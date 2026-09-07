// HTTP/2-транспорт (internal/semantic/http2.go) - биндинг к nghttp2 (C не
// даёт HTTP/2 "бесплатно", в отличие от Go - см. package doc internal/
// interp/http2.go для объяснения асимметрии между backend'ами).
//
// Архитектура: у каждой rech_http2_session_t есть СОБСТВЕННЫЙ поток ОС
// ("IO-поток"), который крутит recv() (с коротким таймаутом, чтобы не
// блокироваться навечно) -> nghttp2_session_mem_recv -> отправка того, что
// накопилось (nghttp2_session_send). Вызовы из основного потока Rech-
// программы (http2_отправить_данные/http2_принять_поток/...) НЕ ждут этот
// цикл - большинство из них (submit_request/submit_response/submit_trailer/
// resume_data + немедленный nghttp2_session_send) сразу сами вызывают
// nghttp2-функции ПОД ТЕМ ЖЕ МЬЮТЕКСОМ, что и IO-поток (nghttp2_session
// НЕ потокобезопасна для одновременных вызовов - мьютекс сериализует
// доступ, не более того) - задержка "до 50мс" остаётся только у ЧТЕНИЯ
// входящих данных с сокета, не у отправки.
//
// Поиск состояния потока (rech_http2_stream_t) по stream_id - через
// СОБСТВЕННОЕ хранилище nghttp2 (nghttp2_session_set/get_stream_user_data),
// не через отдельный список/карту - сама библиотека уже даёт то, что
// иначе пришлось бы реализовывать заново.
//
// Windows не поддержан явно (см. #error ниже) - реализация выше опирается
// на pthread/POSIX-сокеты напрямую, не через уже существующую Windows-ветку
// остального рантайма (concurrency.go/tls.go) - отдельная, непроверенная
// работа, оставлена как явный, а не молчаливый пробел (docs/ROADMAP.md).
package cbackend

import (
	"fmt"

	"github.com/IlyaFeoktistov/Rech/internal/rtmsg"
)

// mangleField применяется здесь к статически известным именам полей
// "HTTP2Поток" (internal/semantic/http2.go, http2StreamFields) - та же
// функция, что и для полей пользовательских сущностей, но сущность сама
// не проходит через generateIREntityStruct (нет ast.EntityDecl) - только
// имена полей мангланы тем же способом, чтобы совпасть с typedef'ом,
// который эмитирует общий механизм регистрации встроенных сущностей.
var http2StreamSessionField = mangleField("сессия")
var http2StreamIDField = mangleField("ид")

// http2SessionTypedefCode - typedef rech_http2_session_t/rech_http2_stream_t
// (внутреннее устройство сессии/потока) и вспомогательные растущие буферы -
// C-структуры, о которых Rech-программа не знает вовсе (видит только
// непрозрачный указатель, см. valuetype.go, typeHTTP2Session).
const http2SessionTypedefCode = `#if defined(_WIN32)
#error "HTTP/2 (nghttp2) пока не поддержан на Windows - см. docs/ROADMAP.md"
#endif

typedef struct rech_http2_stream {
    int32_t id;
    char *path;
    char *method;
    long long status;

    char **hdr_names;
    char **hdr_values;
    long long hdr_count;
    long long hdr_cap;

    unsigned char *recv_buf;
    long long recv_len;
    long long recv_cap;
    long long recv_off;
    int recv_eof;

    unsigned char *send_buf;
    long long send_len;
    long long send_cap;
    long long send_pos;
    int send_eof;

    char **trailer_names;
    char **trailer_values;
    long long trailer_count;
    long long trailer_cap;
    int trailers_sent;

    int headers_sent;
    int headers_ready;
    int closed;
} rech_http2_stream_t;

typedef struct rech_http2_session {
    nghttp2_session *ng;
    int fd;
    int is_client;
    char *authority;

    pthread_t thread;
    pthread_mutex_t lock;
    pthread_cond_t cond;

    int32_t *incoming;
    long long incoming_len;
    long long incoming_cap;
    long long incoming_off;

    // all_streams - СОБСТВЕННЫЙ реестр потоков (id -> rech_http2_stream_t*),
    // НЕ nghttp2_session_get/set_stream_user_data - nghttp2 забывает эту
    // связь, как только поток ПОЛНОСТЬЮ закрывается (обе стороны отправили
    // END_STREAM, см. on_stream_close) - а Rech-программа читает финальный
    // ответ/статус ПОСЛЕ этого момента совершенно штатно (например, сразу
    // после http2_завершить_поток на клиенте, когда сервер уже успел
    // ответить и закрыть свою сторону) - найдено живым прогоном, не
    // гипотетически. Записи никогда не удаляются (простой линейный список,
    // не карта - количество потоков на одну сессию в этом срезе невелико) -
    // GC освободит rech_http2_stream_t, когда сама сессия станет
    // недостижима, не раньше.
    rech_http2_stream_t **all_streams;
    long long all_streams_len;
    long long all_streams_cap;

    int closed;
} rech_http2_session_t;

static void rech_http2_register_stream(rech_http2_session_t *sess, rech_http2_stream_t *st) {
    if (sess->all_streams_len >= sess->all_streams_cap) {
        long long ncap = sess->all_streams_cap == 0 ? 8 : sess->all_streams_cap * 2;
        rech_http2_stream_t **na = GC_MALLOC(sizeof(rech_http2_stream_t *) * (size_t)ncap);
        if (sess->all_streams_len > 0) {
            memcpy(na, sess->all_streams, sizeof(rech_http2_stream_t *) * (size_t)sess->all_streams_len);
        }
        sess->all_streams = na;
        sess->all_streams_cap = ncap;
    }
    sess->all_streams[sess->all_streams_len++] = st;
}

static rech_http2_stream_t *rech_http2_find_stream(rech_http2_session_t *sess, int32_t id) {
    for (long long i = 0; i < sess->all_streams_len; i++) {
        if (sess->all_streams[i]->id == id) {
            return sess->all_streams[i];
        }
    }
    return NULL;
}

static void rech_http2_bytes_append(unsigned char **buf, long long *len, long long *cap, const unsigned char *data, long long n) {
    if (n <= 0) return;
    if (*len + n > *cap) {
        long long ncap = (*cap == 0) ? 256 : *cap;
        while (ncap < *len + n) ncap *= 2;
        unsigned char *nb = GC_MALLOC((size_t)ncap);
        if (*len > 0) memcpy(nb, *buf, (size_t)*len);
        *buf = nb;
        *cap = ncap;
    }
    memcpy(*buf + *len, data, (size_t)n);
    *len += n;
}

static void rech_http2_header_append(rech_http2_stream_t *st, char *name, char *value) {
    if (st->hdr_count >= st->hdr_cap) {
        long long ncap = st->hdr_cap == 0 ? 8 : st->hdr_cap * 2;
        char **nn = GC_MALLOC(sizeof(char *) * (size_t)ncap);
        char **nv = GC_MALLOC(sizeof(char *) * (size_t)ncap);
        if (st->hdr_count > 0) {
            memcpy(nn, st->hdr_names, sizeof(char *) * (size_t)st->hdr_count);
            memcpy(nv, st->hdr_values, sizeof(char *) * (size_t)st->hdr_count);
        }
        st->hdr_names = nn;
        st->hdr_values = nv;
        st->hdr_cap = ncap;
    }
    st->hdr_names[st->hdr_count] = name;
    st->hdr_values[st->hdr_count] = value;
    st->hdr_count++;
}

// rech_http2_is_token - см. rtmsg.HTTP2InvalidTrailerName - RFC 7230
// "token" (field-name): непустая строка из ASCII-букв/цифр и набора
// "!#$%&'*+-.^_|~" плюс обратная кавычка (0x60 - см. strchr ниже, не
// вставлена сюда буквально - разорвала бы Go raw string, у которого backtick
// сам служит разделителем).
static int rech_http2_is_token(const char *s) {
    if (!s || !s[0]) return 0;
    for (const char *p = s; *p; p++) {
        unsigned char c = (unsigned char)*p;
        int ok = (c >= 'a' && c <= 'z') || (c >= 'A' && c <= 'Z') || (c >= '0' && c <= '9') ||
                 strchr("!#$%&'*+-.^_\x60|~", c) != NULL;
        if (!ok) return 0;
    }
    return 1;
}

static void rech_http2_trailer_append(rech_http2_stream_t *st, const char *name, const char *value) {
    if (st->trailer_count >= st->trailer_cap) {
        long long ncap = st->trailer_cap == 0 ? 4 : st->trailer_cap * 2;
        char **nn = GC_MALLOC(sizeof(char *) * (size_t)ncap);
        char **nv = GC_MALLOC(sizeof(char *) * (size_t)ncap);
        if (st->trailer_count > 0) {
            memcpy(nn, st->trailer_names, sizeof(char *) * (size_t)st->trailer_count);
            memcpy(nv, st->trailer_values, sizeof(char *) * (size_t)st->trailer_count);
        }
        st->trailer_names = nn;
        st->trailer_values = nv;
        st->trailer_cap = ncap;
    }
    st->trailer_names[st->trailer_count] = (char *)name;
    st->trailer_values[st->trailer_count] = (char *)value;
    st->trailer_count++;
}

static nghttp2_nv *rech_http2_build_nv(char **names, char **values, long long count) {
    nghttp2_nv *nva = GC_MALLOC(sizeof(nghttp2_nv) * (size_t)(count > 0 ? count : 1));
    for (long long i = 0; i < count; i++) {
        nva[i].name = (uint8_t *)names[i];
        nva[i].value = (uint8_t *)values[i];
        nva[i].namelen = strlen(names[i]);
        nva[i].valuelen = strlen(values[i]);
        nva[i].flags = NGHTTP2_NV_FLAG_NONE;
    }
    return nva;
}

static void rech_http2_enqueue(rech_http2_session_t *sess, int32_t stream_id) {
    if (sess->incoming_len >= sess->incoming_cap) {
        long long ncap = sess->incoming_cap == 0 ? 16 : sess->incoming_cap * 2;
        int32_t *ni = GC_MALLOC(sizeof(int32_t) * (size_t)ncap);
        long long n = sess->incoming_len - sess->incoming_off;
        if (n > 0) memcpy(ni, sess->incoming + sess->incoming_off, sizeof(int32_t) * (size_t)n);
        sess->incoming = ni;
        sess->incoming_len = n;
        sess->incoming_off = 0;
        sess->incoming_cap = ncap;
    }
    sess->incoming[sess->incoming_len++] = stream_id;
}

`

// http2CallbacksCode - колбэки nghttp2 (см. package doc) - разбор входящих
// заголовков/данных/закрытия потока, отправка исходящих байт на сокет,
// поставщик данных для DATA-фреймов (читает из stream->send_buf, отдаёт
// EOF+трейлеры, когда http2_завершить_поток пометил send_eof и буфер
// вычерпан).
const http2CallbacksCode = `static int rech_http2_on_begin_headers(nghttp2_session *ng, const nghttp2_frame *frame, void *user_data) {
    rech_http2_session_t *sess = (rech_http2_session_t *)user_data;
    if (frame->hd.type != NGHTTP2_HEADERS) return 0;
    if (frame->headers.cat == NGHTTP2_HCAT_REQUEST) {
        rech_http2_stream_t *st = GC_MALLOC(sizeof(rech_http2_stream_t));
        st->id = frame->hd.stream_id;
        st->status = -1;
        nghttp2_session_set_stream_user_data(ng, frame->hd.stream_id, st);
        rech_http2_register_stream(sess, st);
    }
    return 0;
}

static int rech_http2_on_header(nghttp2_session *ng, const nghttp2_frame *frame,
                                 const uint8_t *name, size_t namelen,
                                 const uint8_t *value, size_t valuelen,
                                 uint8_t flags, void *user_data) {
    (void)flags;
    (void)user_data;
    if (frame->hd.type != NGHTTP2_HEADERS) return 0;
    rech_http2_stream_t *st = nghttp2_session_get_stream_user_data(ng, frame->hd.stream_id);
    if (!st) return 0;
    char *n = GC_MALLOC(namelen + 1);
    memcpy(n, name, namelen);
    n[namelen] = '\0';
    char *v = GC_MALLOC(valuelen + 1);
    memcpy(v, value, valuelen);
    v[valuelen] = '\0';
    if (strcmp(n, ":path") == 0) {
        st->path = v;
    } else if (strcmp(n, ":method") == 0) {
        st->method = v;
    } else if (strcmp(n, ":status") == 0) {
        st->status = atoll(v);
    } else if (n[0] != ':') {
        rech_http2_header_append(st, n, v);
    }
    return 0;
}

static int rech_http2_on_frame_recv(nghttp2_session *ng, const nghttp2_frame *frame, void *user_data) {
    rech_http2_session_t *sess = (rech_http2_session_t *)user_data;
    rech_http2_stream_t *st = nghttp2_session_get_stream_user_data(ng, frame->hd.stream_id);
    if (frame->hd.type == NGHTTP2_HEADERS && st) {
        if (frame->headers.cat == NGHTTP2_HCAT_REQUEST) {
            rech_http2_enqueue(sess, frame->hd.stream_id);
        } else {
            st->headers_ready = 1;
        }
    }
    // END_STREAM может прийти и на DATA-фрейме (тело запроса без трейлеров
    // - трейлеры только у ОТВЕТА, см. http2_отправить_трейлер), не только
    // на HEADERS (трейлерном) - ЖИВОЙ НАЙДЕННЫЙ БАГ: пока recv_eof
    // выставлялся только по HEADERS, http2_прочитать_всё (ждёт именно
    // recv_eof, в отличие от http2_прочитать_данные, которой достаточно
    // уже накопленных байт) вешалось навечно на чтении тела запроса.
    if (st && (frame->hd.type == NGHTTP2_HEADERS || frame->hd.type == NGHTTP2_DATA) &&
        (frame->hd.flags & NGHTTP2_FLAG_END_STREAM)) {
        st->recv_eof = 1;
    }
    pthread_cond_broadcast(&sess->cond);
    return 0;
}

static int rech_http2_on_data_chunk(nghttp2_session *ng, uint8_t flags, int32_t stream_id,
                                     const uint8_t *data, size_t len, void *user_data) {
    (void)flags;
    rech_http2_session_t *sess = (rech_http2_session_t *)user_data;
    rech_http2_stream_t *st = nghttp2_session_get_stream_user_data(ng, stream_id);
    if (st) {
        rech_http2_bytes_append(&st->recv_buf, &st->recv_len, &st->recv_cap, data, (long long)len);
        pthread_cond_broadcast(&sess->cond);
    }
    return 0;
}

static int rech_http2_on_stream_close(nghttp2_session *ng, int32_t stream_id, uint32_t error_code, void *user_data) {
    (void)error_code;
    rech_http2_session_t *sess = (rech_http2_session_t *)user_data;
    rech_http2_stream_t *st = nghttp2_session_get_stream_user_data(ng, stream_id);
    if (st) {
        st->closed = 1;
        st->recv_eof = 1;
        st->headers_ready = 1;
    }
    pthread_cond_broadcast(&sess->cond);
    return 0;
}

static ssize_t rech_http2_send_cb(nghttp2_session *ng, const uint8_t *data, size_t length, int flags, void *user_data) {
    (void)ng;
    (void)flags;
    rech_http2_session_t *sess = (rech_http2_session_t *)user_data;
    ssize_t n = send(sess->fd, data, length, 0);
    if (n < 0) {
        if (errno == EAGAIN || errno == EWOULDBLOCK) return NGHTTP2_ERR_WOULDBLOCK;
        return NGHTTP2_ERR_CALLBACK_FAILURE;
    }
    return n;
}

static ssize_t rech_http2_data_read_cb(nghttp2_session *ng, int32_t stream_id, uint8_t *buf, size_t length,
                                        uint32_t *data_flags, nghttp2_data_source *source, void *user_data) {
    (void)ng;
    (void)stream_id;
    (void)user_data;
    rech_http2_stream_t *st = (rech_http2_stream_t *)source->ptr;
    long long avail = st->send_len - st->send_pos;
    if (avail <= 0) {
        if (st->send_eof) {
            *data_flags |= NGHTTP2_DATA_FLAG_EOF;
            if (st->trailer_count > 0 && !st->trailers_sent) {
                // NGHTTP2_DATA_FLAG_NO_END_STREAM - ЖИВОЙ НАЙДЕННЫЙ БАГ: без
                // него nghttp2 сам ставит END_STREAM на этот DATA-фрейм, и
                // последующий nghttp2_submit_trailer молча не долетает до
                // получателя (поток уже закрыт с его точки зрения) - см.
                // nghttp2.h, доку у nghttp2_submit_trailer.
                *data_flags |= NGHTTP2_DATA_FLAG_NO_END_STREAM;
                nghttp2_nv *nva = rech_http2_build_nv(st->trailer_names, st->trailer_values, st->trailer_count);
                nghttp2_submit_trailer(ng, stream_id, nva, (size_t)st->trailer_count);
                st->trailers_sent = 1;
            }
            return 0;
        }
        return NGHTTP2_ERR_DEFERRED;
    }
    long long n = avail < (long long)length ? avail : (long long)length;
    memcpy(buf, st->send_buf + st->send_pos, (size_t)n);
    st->send_pos += n;
    return n;
}

static void *rech_http2_io_thread(void *arg) {
    rech_http2_session_t *sess = (rech_http2_session_t *)arg;
    unsigned char buf[16384];
    for (;;) {
        pthread_mutex_lock(&sess->lock);
        int closed = sess->closed;
        pthread_mutex_unlock(&sess->lock);
        if (closed) break;

        ssize_t n = recv(sess->fd, buf, sizeof(buf), 0);
        int broken = 0;
        if (n > 0) {
            pthread_mutex_lock(&sess->lock);
            ssize_t rv = nghttp2_session_mem_recv(sess->ng, buf, (size_t)n);
            if (rv < 0) broken = 1;
            pthread_mutex_unlock(&sess->lock);
        } else if (n == 0) {
            broken = 1;
        } else if (errno != EAGAIN && errno != EWOULDBLOCK && errno != EINTR) {
            broken = 1;
        }

        pthread_mutex_lock(&sess->lock);
        if (!broken && nghttp2_session_want_write(sess->ng)) {
            if (nghttp2_session_send(sess->ng) != 0) broken = 1;
        }
        if (broken) {
            sess->closed = 1;
        }
        pthread_cond_broadcast(&sess->cond);
        pthread_mutex_unlock(&sess->lock);
        if (broken) break;
    }
    return NULL;
}

`

// http2StreamTypeC - см. internal/semantic/http2.go, http2StreamType - тот
// же принцип дублирования имени константой в нескольких пакетах, что и
// byteListType (bytesio.go).
const http2StreamTypeC = "HTTP2Поток"

// http2SessionOpsCode - публичные C-функции, соответствующие каждой
// http2_*-функции Rech (см. internal/semantic/http2.go) - генерируется как
// fmt.Sprintf-шаблон, чтобы встроить готовые (уже экранированные) тексты
// сообщений rtmsg и настоящие манглированные C-имена (не угаданные строки -
// mangleType/mangleField решают транслитерацию сами) прямо в тело функций,
// тем же приёмом, что и у socketHelpers/plainSocketOpsCode.
func http2SessionOpsCode() string {
	streamC := mangleType(http2StreamTypeC)
	optStringC := mangleType("?Строка")
	byteListC := mangleType(byteListType)
	return fmt.Sprintf(`static void rech_http2_start_thread(rech_http2_session_t *sess) {
    pthread_mutex_init(&sess->lock, NULL);
    pthread_cond_init(&sess->cond, NULL);
    struct timeval tv;
    tv.tv_sec = 0;
    tv.tv_usec = 50000;
    setsockopt(sess->fd, SOL_SOCKET, SO_RCVTIMEO, &tv, sizeof(tv));
    GC_pthread_create(&sess->thread, NULL, rech_http2_io_thread, sess);
}

// rech_http2_new_callbacks - СВЕЖИЙ объект колбэков на каждую сессию, не
// разделяемый лениво-инициализированный синглтон - nghttp2_session_*_new
// КОПИРУЕТ все поля callbacks в саму сессию (см. её документацию - "session
// does not store callbacks"), поэтому объект нужен ровно на время самого
// вызова и сразу удаляется. НАЙДЕННЫЙ ЖИВОЙ БАГ (не гипотетический) - более
// ранняя версия использовала общий "static nghttp2_session_callbacks *cbs"
// с ленивой инициализацией БЕЗ мьютекса - клиентская и серверная сессии
// создаются практически одновременно, каждая в своём потоке ОС, и вторая
// могла получить указатель на ЕЩЁ НЕ ДОЗАПОЛНЕННЫЙ объект (гонка на
// "cbs != NULL" против последовательности nghttp2_session_callbacks_set_*)-
// колбэки на данные/заголовки молча не срабатывали, HTTP/2-сессия зависала
// без единого события. Свежий объект на каждый вызов стоит недорого
// (маленькая структура, создаётся один раз на всю жизнь сессии) и полностью
// исключает эту гонку.
static nghttp2_session_callbacks *rech_http2_new_callbacks(void) {
    nghttp2_session_callbacks *cbs;
    nghttp2_session_callbacks_new(&cbs);
    nghttp2_session_callbacks_set_send_callback(cbs, rech_http2_send_cb);
    nghttp2_session_callbacks_set_on_begin_headers_callback(cbs, rech_http2_on_begin_headers);
    nghttp2_session_callbacks_set_on_header_callback(cbs, rech_http2_on_header);
    nghttp2_session_callbacks_set_on_frame_recv_callback(cbs, rech_http2_on_frame_recv);
    nghttp2_session_callbacks_set_on_data_chunk_recv_callback(cbs, rech_http2_on_data_chunk);
    nghttp2_session_callbacks_set_on_stream_close_callback(cbs, rech_http2_on_stream_close);
    return cbs;
}

static rech_http2_session_t *rech_http2_accept_session(rech_socket_t listener) {
    int fd = accept(listener, NULL, NULL);
    if (fd < 0) {
        rech_panic("%[1]s");
    }
    rech_http2_session_t *sess = GC_MALLOC(sizeof(rech_http2_session_t));
    sess->fd = fd;
    sess->is_client = 0;
    nghttp2_session_callbacks *cbs = rech_http2_new_callbacks();
    int rv = nghttp2_session_server_new(&sess->ng, cbs, sess);
    nghttp2_session_callbacks_del(cbs);
    if (rv != 0) {
        rech_panic("%[2]s");
    }
    nghttp2_submit_settings(sess->ng, NGHTTP2_FLAG_NONE, NULL, 0);
    nghttp2_session_send(sess->ng);
    rech_http2_start_thread(sess);
    return sess;
}

static rech_http2_session_t *rech_http2_new_session(const char *host, long long port) {
    rech_socket_t fd = rech_connect_tcp(host, port);
    rech_http2_session_t *sess = GC_MALLOC(sizeof(rech_http2_session_t));
    sess->fd = fd;
    sess->is_client = 1;
    long long authlen = (long long)strlen(host) + 32;
    sess->authority = GC_MALLOC((size_t)authlen);
    snprintf(sess->authority, (size_t)authlen, "%%s:%%lld", host, port);
    nghttp2_session_callbacks *cbs = rech_http2_new_callbacks();
    int rv = nghttp2_session_client_new(&sess->ng, cbs, sess);
    nghttp2_session_callbacks_del(cbs);
    if (rv != 0) {
        rech_panic("%[3]s");
    }
    // nghttp2_session_client_new САМА ставит клиентский connection preface
    // ("PRI * HTTP/2.0\r\n\r\nSM\r\n\r\n") в очередь на отправку - НАЙДЕННЫЙ
    // ЖИВОЙ БАГ (не по документации, а по факту): более ранняя версия ЕЩЁ
    // РАЗ отправляла его вручную ("сырым" send() до submit_settings) -
    // сервер получал preface ДВАЖДЫ подряд и рвал соединение
    // (NGHTTP2_PROTOCOL_ERROR), приняв вторую копию за испорченный
    // SETTINGS-фрейм. Ручная отправка не нужна вовсе - nghttp2_session_send
    // ниже сама пришлёт preface + SETTINGS одним вызовом send_callback.
    nghttp2_submit_settings(sess->ng, NGHTTP2_FLAG_NONE, NULL, 0);
    nghttp2_session_send(sess->ng);
    rech_http2_start_thread(sess);
    return sess;
}

static void rech_http2_close_session(rech_http2_session_t *sess) {
    pthread_mutex_lock(&sess->lock);
    sess->closed = 1;
    pthread_mutex_unlock(&sess->lock);
    close(sess->fd);
}

static %[13]s rech_http2_accept_stream(rech_http2_session_t *sess) {
    pthread_mutex_lock(&sess->lock);
    while (sess->incoming_off >= sess->incoming_len && !sess->closed) {
        pthread_cond_wait(&sess->cond, &sess->lock);
    }
    if (sess->incoming_off >= sess->incoming_len) {
        pthread_mutex_unlock(&sess->lock);
        rech_panic("%[4]s");
    }
    int32_t id = sess->incoming[sess->incoming_off++];
    pthread_mutex_unlock(&sess->lock);
    %[13]s out;
    out.%[5]s = sess;
    out.%[6]s = id;
    return out;
}

static %[13]s rech_http2_open_stream(rech_http2_session_t *sess, const char *method, const char *path) {
    rech_http2_stream_t *st = GC_MALLOC(sizeof(rech_http2_stream_t));
    st->status = -1;

    nghttp2_nv nva[4];
    nva[0].name = (uint8_t *)":method"; nva[0].namelen = 7;
    nva[0].value = (uint8_t *)method; nva[0].valuelen = strlen(method);
    nva[0].flags = NGHTTP2_NV_FLAG_NONE;
    nva[1].name = (uint8_t *)":scheme"; nva[1].namelen = 7;
    nva[1].value = (uint8_t *)"http"; nva[1].valuelen = 4;
    nva[1].flags = NGHTTP2_NV_FLAG_NONE;
    nva[2].name = (uint8_t *)":authority"; nva[2].namelen = 10;
    nva[2].value = (uint8_t *)sess->authority; nva[2].valuelen = strlen(sess->authority);
    nva[2].flags = NGHTTP2_NV_FLAG_NONE;
    nva[3].name = (uint8_t *)":path"; nva[3].namelen = 5;
    nva[3].value = (uint8_t *)path; nva[3].valuelen = strlen(path);
    nva[3].flags = NGHTTP2_NV_FLAG_NONE;

    nghttp2_data_provider prd;
    prd.source.ptr = st;
    prd.read_callback = rech_http2_data_read_cb;

    pthread_mutex_lock(&sess->lock);
    int32_t id = nghttp2_submit_request(sess->ng, NULL, nva, 4, &prd, st);
    if (id < 0) {
        pthread_mutex_unlock(&sess->lock);
        rech_panic("%[7]s");
    }
    st->id = id;
    rech_http2_register_stream(sess, st);
    nghttp2_session_send(sess->ng);
    pthread_mutex_unlock(&sess->lock);

    %[13]s out;
    out.%[5]s = sess;
    out.%[6]s = id;
    return out;
}

static rech_http2_stream_t *rech_http2_stream_of(%[13]s potok) {
    rech_http2_session_t *sess = potok.%[5]s;
    pthread_mutex_lock(&sess->lock);
    rech_http2_stream_t *st = rech_http2_find_stream(sess, (int32_t)potok.%[6]s);
    pthread_mutex_unlock(&sess->lock);
    if (!st) {
        rech_panic("%[8]s");
    }
    return st;
}

static const char *rech_http2_path(%[13]s potok) {
    rech_http2_stream_t *st = rech_http2_stream_of(potok);
    return st->path ? st->path : "";
}

static const char *rech_http2_method(%[13]s potok) {
    rech_http2_stream_t *st = rech_http2_stream_of(potok);
    return st->method ? st->method : "";
}

static long long rech_http2_status(%[13]s potok) {
    rech_http2_session_t *sess = potok.%[5]s;
    rech_http2_stream_t *st = rech_http2_stream_of(potok);
    pthread_mutex_lock(&sess->lock);
    while (!st->headers_ready) {
        pthread_cond_wait(&sess->cond, &sess->lock);
    }
    pthread_mutex_unlock(&sess->lock);
    return st->status;
}

static %[9]s rech_http2_header(%[13]s potok, const char *name) {
    // Трейлер - ОТДЕЛЬНЫЙ, более ПОЗДНИЙ HEADERS-фрейм, приходящий уже
    // ПОСЛЕ данных (не вместе с headers_ready, который взводится на
    // ПЕРВЫХ заголовках) - ждать нужно ДО ТЕХ ПОР, пока либо искомое имя
    // не найдётся, либо поток не завершится целиком (recv_eof - дальше
    // точно ничего не придёт) - иначе чтение трейлера сразу после
    // "http2_прочитать_данные" гонялось бы с ещё не обработанным IO-потоком
    // фреймом (найдено живым прогоном, не гипотетически).
    rech_http2_session_t *sess = potok.%[5]s;
    rech_http2_stream_t *st = rech_http2_stream_of(potok);
    pthread_mutex_lock(&sess->lock);
    for (;;) {
        for (long long i = 0; i < st->hdr_count; i++) {
            if (strcmp(st->hdr_names[i], name) == 0) {
                %[9]s found = %[10]s(st->hdr_values[i]);
                pthread_mutex_unlock(&sess->lock);
                return found;
            }
        }
        if (st->recv_eof) {
            break;
        }
        pthread_cond_wait(&sess->cond, &sess->lock);
    }
    pthread_mutex_unlock(&sess->lock);
    return %[9]s_empty();
}

static %[11]s *rech_http2_read_data(%[13]s potok, long long max) {
    if (max <= 0) {
        rech_panic("%[12]s");
    }
    rech_http2_session_t *sess = potok.%[5]s;
    rech_http2_stream_t *st = rech_http2_stream_of(potok);
    pthread_mutex_lock(&sess->lock);
    while (st->recv_off >= st->recv_len && !st->recv_eof) {
        pthread_cond_wait(&sess->cond, &sess->lock);
    }
    long long avail = st->recv_len - st->recv_off;
    long long n = avail < max ? avail : max;
    unsigned char *tmp = n > 0 ? GC_MALLOC((size_t)n) : NULL;
    if (n > 0) {
        memcpy(tmp, st->recv_buf + st->recv_off, (size_t)n);
        st->recv_off += n;
    }
    pthread_mutex_unlock(&sess->lock);
    return rech_bytes_wrap(tmp, n);
}

// rech_http2_read_all - "http2_прочитать_всё" - ждёт recv_eof (END_STREAM) и
// отдаёт НАКОПЛЕННЫЙ буфер целиком одним куском - буфер и так уже собирает
// io-поток колбэком rech_http2_on_data_chunk по мере прихода данных, поэтому
// в отличие от Go-стороны (interp/http2.go, http2ReadAll) здесь не нужен
// цикл чтения - достаточно один раз дождаться конца и скопировать всё сразу.
static %[11]s *rech_http2_read_all(%[13]s potok) {
    rech_http2_session_t *sess = potok.%[5]s;
    rech_http2_stream_t *st = rech_http2_stream_of(potok);
    pthread_mutex_lock(&sess->lock);
    while (!st->recv_eof) {
        pthread_cond_wait(&sess->cond, &sess->lock);
    }
    long long n = st->recv_len - st->recv_off;
    unsigned char *tmp = n > 0 ? GC_MALLOC((size_t)n) : NULL;
    if (n > 0) {
        memcpy(tmp, st->recv_buf + st->recv_off, (size_t)n);
        st->recv_off += n;
    }
    pthread_mutex_unlock(&sess->lock);
    return rech_bytes_wrap(tmp, n);
}

static void rech_http2_send_headers(%[13]s potok, long long code) {
    rech_http2_session_t *sess = potok.%[5]s;
    rech_http2_stream_t *st = rech_http2_stream_of(potok);
    char status[16];
    snprintf(status, sizeof status, "%%lld", code);
    nghttp2_nv nva[1];
    nva[0].name = (uint8_t *)":status"; nva[0].namelen = 7;
    nva[0].value = (uint8_t *)status; nva[0].valuelen = strlen(status);
    nva[0].flags = NGHTTP2_NV_FLAG_NONE;

    nghttp2_data_provider prd;
    prd.source.ptr = st;
    prd.read_callback = rech_http2_data_read_cb;

    pthread_mutex_lock(&sess->lock);
    nghttp2_submit_response(sess->ng, st->id, nva, 1, &prd);
    st->headers_sent = 1;
    nghttp2_session_send(sess->ng);
    pthread_mutex_unlock(&sess->lock);
}

static void rech_http2_send_data(%[13]s potok, %[11]s *data) {
    rech_http2_session_t *sess = potok.%[5]s;
    rech_http2_stream_t *st = rech_http2_stream_of(potok);
    pthread_mutex_lock(&sess->lock);
    rech_http2_bytes_append(&st->send_buf, &st->send_len, &st->send_cap, data->data, data->len);
    nghttp2_session_resume_data(sess->ng, st->id);
    nghttp2_session_send(sess->ng);
    pthread_mutex_unlock(&sess->lock);
}

static void rech_http2_send_trailer(%[13]s potok, const char *name, const char *value) {
    if (!rech_http2_is_token(name)) {
        rech_panic("%[14]s");
    }
    rech_http2_stream_t *st = rech_http2_stream_of(potok);
    rech_http2_trailer_append(st, name, value);
}

static void rech_http2_finish_stream(%[13]s potok) {
    rech_http2_session_t *sess = potok.%[5]s;
    rech_http2_stream_t *st = rech_http2_stream_of(potok);
    pthread_mutex_lock(&sess->lock);
    st->send_eof = 1;
    nghttp2_session_resume_data(sess->ng, st->id);
    nghttp2_session_send(sess->ng);
    pthread_mutex_unlock(&sess->lock);
}

`, rtmsg.HTTP2AcceptFailed, rtmsg.HTTP2HandshakeFailed, rtmsg.HTTP2DialFailed,
		rtmsg.HTTP2SessionClosed, http2StreamSessionField, http2StreamIDField,
		rtmsg.HTTP2OpenStreamFailed, rtmsg.HTTP2StreamNotFound, optStringC,
		optStringC+"_wrap", byteListC, rtmsg.HTTP2RecvInvalidMax, streamC,
		rtmsg.HTTP2InvalidTrailerName)
}
