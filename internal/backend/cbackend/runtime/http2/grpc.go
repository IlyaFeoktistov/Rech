// gRPC - см. package doc internal/semantic/grpc.go. Реализация - тонкая
// надстройка над уже сгенерированным HTTP/2-рантаймом (http2.go, тот же
// nghttp2-биндинг, тот же rech_http2_stream_t/rech_http2_session_t) -
// повторно использует rech_http2_read_data/rech_http2_send_data/
// rech_http2_send_trailer/rech_http2_finish_stream/rech_http2_header
// (SessionOpsCode) по имени, поэтому эмитируется В ТОМ ЖЕ файле ПОСЛЕ
// SessionOpsCode (см. ir_generator.go) - C требует видеть определение
// статической функции раньше её вызова.
package http2

import "fmt"

// GRPCContentType - "content-type" gRPC-запросов/ответов.
const GRPCContentType = "application/grpc"

// GRPCSessionOpsCode - публичные C-функции, соответствующие каждой
// grpc_*-функции Rech (см. internal/semantic/grpc.go) - тот же приём
// fmt.Sprintf-шаблона, что и у SessionOpsCode. Параметры - уже вычисленные
// вызывающей стороной значения (см. package doc http2.go).
func GRPCSessionOpsCode(streamC, byteListC, nullableByteListC, optStringC string,
	streamSessionField, streamIDField string,
	openStreamFailedMsg, framingErrorMsg, compressedUnsupportedMsg, statusMissingMsg, statusInvalidMsg, compressFailedMsg string) string {
	return fmt.Sprintf(`static %[1]s rech_grpc_open_stream(rech_http2_session_t *sess, const char *path) {
    rech_http2_stream_t *st = GC_MALLOC(sizeof(rech_http2_stream_t));
    st->status = -1;

    nghttp2_nv nva[5];
    nva[0].name = (uint8_t *)":method"; nva[0].namelen = 7;
    nva[0].value = (uint8_t *)"POST"; nva[0].valuelen = 4;
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
    nva[4].name = (uint8_t *)"content-type"; nva[4].namelen = 12;
    nva[4].value = (uint8_t *)"%[9]s"; nva[4].valuelen = %[10]d;
    nva[4].flags = NGHTTP2_NV_FLAG_NONE;

    nghttp2_data_provider prd;
    prd.source.ptr = st;
    prd.read_callback = rech_http2_data_read_cb;

    pthread_mutex_lock(&sess->lock);
    int32_t id = nghttp2_submit_request(sess->ng, NULL, nva, 5, &prd, st);
    if (id < 0) {
        pthread_mutex_unlock(&sess->lock);
        rech_panic("%[5]s");
    }
    st->id = id;
    rech_http2_register_stream(sess, st);
    nghttp2_session_send(sess->ng);
    pthread_mutex_unlock(&sess->lock);

    %[1]s out;
    out.%[3]s = sess;
    out.%[4]s = id;
    return out;
}

static void rech_grpc_send_headers(%[1]s potok) {
    rech_http2_session_t *sess = potok.%[3]s;
    rech_http2_stream_t *st = rech_http2_stream_of(potok);

    nghttp2_nv nva[2];
    nva[0].name = (uint8_t *)":status"; nva[0].namelen = 7;
    nva[0].value = (uint8_t *)"200"; nva[0].valuelen = 3;
    nva[0].flags = NGHTTP2_NV_FLAG_NONE;
    nva[1].name = (uint8_t *)"content-type"; nva[1].namelen = 12;
    nva[1].value = (uint8_t *)"%[9]s"; nva[1].valuelen = %[10]d;
    nva[1].flags = NGHTTP2_NV_FLAG_NONE;

    nghttp2_data_provider prd;
    prd.source.ptr = st;
    prd.read_callback = rech_http2_data_read_cb;

    pthread_mutex_lock(&sess->lock);
    nghttp2_submit_response(sess->ng, st->id, nva, 2, &prd);
    st->headers_sent = 1;
    nghttp2_session_send(sess->ng);
    pthread_mutex_unlock(&sess->lock);
}

// rech_gzip_compress - тело в формате gzip (RFC 1952 - windowBits 15+16
// deflateInit2 сигнализирует именно gzip-обёртку, не "сырой" deflate и не
// zlib/RFC 1950 - grpc-encoding: gzip требует ровно этот формат). deflateBound
// - верхняя граница размера сжатых данных ЛЮБОГО входа этой длины (включая
// несжимаемый) - однопроходно, без роста буфера по ходу, в отличие от
// rech_gzip_decompress ниже (там итоговый размер заранее не известен).
static unsigned char *rech_gzip_compress(const unsigned char *data, long long len, long long *out_len) {
    z_stream strm;
    memset(&strm, 0, sizeof(strm));
    if (deflateInit2(&strm, Z_DEFAULT_COMPRESSION, Z_DEFLATED, 15 + 16, 8, Z_DEFAULT_STRATEGY) != Z_OK) {
        rech_panic("%[16]s");
    }
    uLong cap = deflateBound(&strm, (uLong)len);
    unsigned char *out = GC_MALLOC((size_t)cap);
    strm.next_in   = (Bytef *)data;
    strm.avail_in  = (uInt)len;
    strm.next_out  = out;
    strm.avail_out = (uInt)cap;
    int rc = deflate(&strm, Z_FINISH);
    if (rc != Z_STREAM_END) {
        deflateEnd(&strm);
        rech_panic("%[16]s");
    }
    *out_len = (long long)strm.total_out;
    deflateEnd(&strm);
    return out;
}

// rech_gzip_decompress - обратное rech_gzip_compress. Итоговый размер
// неизвестен заранее (в отличие от сжатия) - буфер удваивается по ходу,
// когда inflate() исчерпывает avail_out, не дочитав до Z_STREAM_END.
static unsigned char *rech_gzip_decompress(const unsigned char *data, long long len, long long *out_len) {
    z_stream strm;
    memset(&strm, 0, sizeof(strm));
    if (inflateInit2(&strm, 15 + 16) != Z_OK) {
        return NULL;
    }
    size_t cap = (size_t)len * 4 + 64;
    unsigned char *out = GC_MALLOC(cap);
    strm.next_in   = (Bytef *)data;
    strm.avail_in  = (uInt)len;
    strm.next_out  = out;
    strm.avail_out = (uInt)cap;
    for (;;) {
        int rc = inflate(&strm, Z_NO_FLUSH);
        if (rc == Z_STREAM_END) {
            break;
        }
        if (rc != Z_OK) {
            inflateEnd(&strm);
            return NULL;
        }
        if (strm.avail_out == 0) {
            size_t used = cap;
            cap *= 2;
            unsigned char *bigger = GC_MALLOC(cap);
            memcpy(bigger, out, used);
            out = bigger;
            strm.next_out  = bigger + used;
            strm.avail_out = (uInt)(cap - used);
        }
    }
    *out_len = (long long)strm.total_out;
    inflateEnd(&strm);
    return out;
}

// rech_grpc_send_message - "grpc_отправить_сообщение(поток, тело, сжато)" -
// compressed==0 (по умолчанию, docs/SPEC.md) - 1 байт флага сжатия
// "identity" + 4 байта big-endian длины + тело как есть, ОДНОЙ записью
// через rech_http2_send_data (не дробить маленькое сообщение на два
// DATA-фрейма). compressed!=0 - тело сначала gzip-сжимается
// (rech_gzip_compress), флаг - 1. Пустое тело ВСЕГДА отправляется как
// identity, даже при compressed!=0 - сжимать 0 байт бессмысленно, а
// gzip-обёртка "пустых" данных не пуста (заголовок/трейлер формата).
static void rech_grpc_send_message(%[1]s potok, %[2]s *body, int compressed) {
    long long n = body ? body->len : 0;
    if (!compressed || n == 0) {
        unsigned char *framed = GC_MALLOC((size_t)(5 + n));
        framed[0] = 0;
        framed[1] = (unsigned char)((n >> 24) & 0xff);
        framed[2] = (unsigned char)((n >> 16) & 0xff);
        framed[3] = (unsigned char)((n >> 8) & 0xff);
        framed[4] = (unsigned char)(n & 0xff);
        if (n > 0) {
            memcpy(framed + 5, body->data, (size_t)n);
        }
        rech_http2_send_data(potok, rech_bytes_wrap(framed, 5 + n));
        return;
    }
    long long clen = 0;
    unsigned char *compressed_body = rech_gzip_compress(body->data, n, &clen);
    unsigned char *framed = GC_MALLOC((size_t)(5 + clen));
    framed[0] = 1;
    framed[1] = (unsigned char)((clen >> 24) & 0xff);
    framed[2] = (unsigned char)((clen >> 16) & 0xff);
    framed[3] = (unsigned char)((clen >> 8) & 0xff);
    framed[4] = (unsigned char)(clen & 0xff);
    memcpy(framed + 5, compressed_body, (size_t)clen);
    rech_http2_send_data(potok, rech_bytes_wrap(framed, 5 + clen));
}

// rech_grpc_read_message - читает ровно один length-prefixed кадр через
// rech_http2_read_data В ЦИКЛЕ (та может вернуть меньше запрошенного за
// одно обращение) - см. rtmsg.GRPCFramingError/GRPCCompressedUnsupported:
// %[6]s (пустая обёртка) - ТОЛЬКО чистый конец потока (первый же читаемый
// байт - EOF), обрыв посреди префикса/тела - паника, не пустая обёртка.
// Флаг сжатия (header[0]) 1 - тело распаковывается как gzip (единственный
// поддержанный grpc-encoding, см. package doc rtmsg.GRPCCompressedUnsupported) -
// неудачная распаковка (не gzip/испорчено) - паника, не тихий пропуск.
static %[7]s rech_grpc_read_message(%[1]s potok) {
    unsigned char header[5];
    long long got = 0;
    while (got < 5) {
        %[2]s *chunk = rech_http2_read_data(potok, 5 - got);
        if (chunk->len == 0) {
            if (got == 0) {
                return %[6]s();
            }
            rech_panic("%[11]s");
        }
        memcpy(header + got, chunk->data, (size_t)chunk->len);
        got += chunk->len;
    }
    int compressed = header[0] != 0;
    long long msg_len = ((long long)header[1] << 24) | ((long long)header[2] << 16) |
                         ((long long)header[3] << 8) | (long long)header[4];
    unsigned char *body = msg_len > 0 ? GC_MALLOC((size_t)msg_len) : NULL;
    long long bgot = 0;
    while (bgot < msg_len) {
        %[2]s *chunk = rech_http2_read_data(potok, msg_len - bgot);
        if (chunk->len == 0) {
            rech_panic("%[11]s");
        }
        memcpy(body + bgot, chunk->data, (size_t)chunk->len);
        bgot += chunk->len;
    }
    if (!compressed) {
        return %[8]s(rech_bytes_wrap(body, msg_len));
    }
    long long plain_len = 0;
    unsigned char *plain = msg_len > 0 ? rech_gzip_decompress(body, msg_len, &plain_len) : NULL;
    if (msg_len > 0 && plain == NULL) {
        rech_panic("%[12]s");
    }
    return %[8]s(rech_bytes_wrap(plain, plain_len));
}

// rech_grpc_finish - серверная сторона: трейлеры "grpc-status"/
// "grpc-message" (пустое сообщение не отправляется вовсе - легитимный
// успех без текста) + завершает поток.
static void rech_grpc_finish(%[1]s potok, long long code, const char *message) {
    char code_str[32];
    snprintf(code_str, sizeof code_str, "%%lld", code);
    rech_http2_send_trailer(potok, "grpc-status", code_str);
    if (message && message[0]) {
        rech_http2_send_trailer(potok, "grpc-message", message);
    }
    rech_http2_finish_stream(potok);
}

// rech_grpc_status - клиентская сторона, см. rtmsg.GRPCStatusMissing -
// вызывающая программа обязана дочитать поток (rech_grpc_read_message до
// пустой обёртки) РАНЬШЕ - иначе трейлер ещё не гарантированно виден.
static long long rech_grpc_status(%[1]s potok) {
    %[13]s raw = rech_http2_header(potok, "grpc-status");
    if (!raw.has) {
        rech_panic("%[14]s");
    }
    char *endptr = NULL;
    long long code = strtoll(raw.value, &endptr, 10);
    if (endptr == raw.value || *endptr != '\x00') {
        rech_panic("%[15]s");
    }
    return code;
}

static const char *rech_grpc_error_message(%[1]s potok) {
    %[13]s raw = rech_http2_header(potok, "grpc-message");
    return raw.has ? raw.value : "";
}

`, streamC, byteListC, streamSessionField, streamIDField, openStreamFailedMsg,
		nullableByteListC+"_empty", nullableByteListC, nullableByteListC+"_wrap",
		GRPCContentType, len(GRPCContentType), framingErrorMsg, compressedUnsupportedMsg,
		optStringC, statusMissingMsg, statusInvalidMsg, compressFailedMsg)
}
