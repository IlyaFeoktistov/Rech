// TLS для "слушать_tcp_защищённо" (docs/ROADMAP.md, изоляция паники сделала
// сервер устойчивым к плохому запросу - TLS делает его пригодным для
// настоящего прода) - биндинг к OpenSSL (libssl/libcrypto), НЕ собственная
// реализация: в отличие от SHA-256/HMAC (crypto.go - самостоятельный C,
// принцип "не тянуть внешнюю библиотеку туда, где хватает стандартного
// алгоритма") настоящий TLS 1.2/1.3 стек с нуля - риск для безопасности,
// не место для этого принципа.
//
// Дизайн: rech_socket_t НЕ меняет представление (остаётся тем же голым
// int/SOCKET, что и в builtins_shared.go) - TLS-состояние живёт в
// ОТДЕЛЬНОМ связном списке rech_tls_registry (fd -> SSL_CTX* у слушателя,
// fd -> SSL* у принятого соединения), под общей блокировкой
// (RECH_TLS_LOCK/RECH_TLS_UNLOCK). Так rech_accept/rech_send/rech_recv/
// rech_close_socket остаются теми же именами/сигнатурами, что и в
// socketHelpers (builtins_shared.go) - ЛЮБОЙ код, построенный поверх них
// (httpHelpers - HTTP-клиент, httpserver.go - "принять_http"/
// "отправить_http_ответ", udp.go) продолжает работать без единой правки:
// TLS прозрачен для всего вышестоящего кода. Плата - O(n) поиск по списку
// при каждом send/recv (n - число одновременных TLS-соединений), приемлемо
// для этого среза языка (не highload-прокси).
package cbackend

import (
	"fmt"

	"github.com/IlyaFeoktistov/Rech/internal/rtmsg"
)

// tlsRegistryCode - структура записи реестра и блокировка вокруг неё (см.
// package doc выше). listen_ctx != NULL - запись слушателя ("слушать_tcp_
// защищённо"), ssl != NULL - запись принятого TLS-соединения (rech_accept)
// - ровно одно из двух ненулевое в любой существующей записи. Блокировка -
// pthread_mutex_t/CRITICAL_SECTION (ir_generator.go подключает <pthread.h>
// на POSIX при ctx.usesTLS, как и при usesThreads/usesMutex) - своя,
// отдельная от "Блокировка" (mutex.go, Rech-facing тип): реестр мутируется
// изнутри rech_accept/rech_close_socket независимо от того, использует ли
// сама ПРОГРАММА потоки/"Блокировка" вообще.
const tlsRegistryCode = `typedef struct rech_tls_entry {
    rech_socket_t fd;
    SSL_CTX *listen_ctx;
    SSL *ssl;
    struct rech_tls_entry *next;
} rech_tls_entry_t;

static rech_tls_entry_t *rech_tls_registry = NULL;
#ifdef _WIN32
static CRITICAL_SECTION rech_tls_lock;
static volatile LONG rech_tls_lock_ready = 0;
static void rech_tls_lock_init(void) {
    if (InterlockedCompareExchange(&rech_tls_lock_ready, 1, 0) == 0) {
        InitializeCriticalSection(&rech_tls_lock);
    }
}
#define RECH_TLS_LOCK() (rech_tls_lock_init(), EnterCriticalSection(&rech_tls_lock))
#define RECH_TLS_UNLOCK() LeaveCriticalSection(&rech_tls_lock)
#else
static pthread_mutex_t rech_tls_lock = PTHREAD_MUTEX_INITIALIZER;
#define RECH_TLS_LOCK() pthread_mutex_lock(&rech_tls_lock)
#define RECH_TLS_UNLOCK() pthread_mutex_unlock(&rech_tls_lock)
#endif

// rech_tls_register - добавляет запись реестра (listen_ctx для слушателя,
// ssl для принятого соединения - вызывающий код передаёт ровно один из
// двух, второй NULL).
static void rech_tls_register(rech_socket_t fd, SSL_CTX *listen_ctx, SSL *ssl) {
    rech_tls_entry_t *e = malloc(sizeof(rech_tls_entry_t));
    e->fd = fd;
    e->listen_ctx = listen_ctx;
    e->ssl = ssl;
    RECH_TLS_LOCK();
    e->next = rech_tls_registry;
    rech_tls_registry = e;
    RECH_TLS_UNLOCK();
}

// rech_tls_listen_ctx - SSL_CTX слушателя fd, или NULL, если fd не
// зарегистрирован как защищённый слушатель ("сетевой_узел"/rech_listen_tcp
// без TLS) - решает, нужно ли rech_accept начинать TLS-рукопожатие.
static SSL_CTX *rech_tls_listen_ctx(rech_socket_t fd) {
    RECH_TLS_LOCK();
    SSL_CTX *found = NULL;
    for (rech_tls_entry_t *e = rech_tls_registry; e != NULL; e = e->next) {
        if (e->fd == fd && e->listen_ctx != NULL) {
            found = e->listen_ctx;
            break;
        }
    }
    RECH_TLS_UNLOCK();
    return found;
}

// rech_tls_conn - SSL принятого соединения fd, или NULL, если fd - обычный
// (не TLS) сокет - решает, идут ли rech_send/rech_recv через SSL_write/
// SSL_read или напрямую через send/recv.
static SSL *rech_tls_conn(rech_socket_t fd) {
    RECH_TLS_LOCK();
    SSL *found = NULL;
    for (rech_tls_entry_t *e = rech_tls_registry; e != NULL; e = e->next) {
        if (e->fd == fd && e->ssl != NULL) {
            found = e->ssl;
            break;
        }
    }
    RECH_TLS_UNLOCK();
    return found;
}

// rech_tls_unregister - снимает запись fd (соединения ИЛИ слушателя) из
// реестра и возвращает её вызывающему коду (rech_close_socket) для
// освобождения SSL/SSL_CTX - NULL, если fd в реестре не было (обычный
// сокет, закрывается как раньше).
static rech_tls_entry_t *rech_tls_unregister(rech_socket_t fd) {
    RECH_TLS_LOCK();
    rech_tls_entry_t **pp = &rech_tls_registry;
    rech_tls_entry_t *found = NULL;
    while (*pp != NULL) {
        if ((*pp)->fd == fd) {
            found = *pp;
            *pp = found->next;
            break;
        }
        pp = &(*pp)->next;
    }
    RECH_TLS_UNLOCK();
    return found;
}

`

// tlsSocketOpsCode - rech_accept/rech_send/rech_recv/rech_close_socket -
// ИМЕНА И СИГНАТУРЫ те же, что и у plainSocketOpsCode (builtins_shared.go),
// эмитируется вместо неё, когда ctx.usesTLS (см. package doc выше и
// ir_generator.go).
var tlsSocketOpsCode = fmt.Sprintf(`static %[6]s rech_accept(rech_socket_t s) {
    rech_socket_t c = accept(s, NULL, NULL);
    if (c == RECH_INVALID_SOCKET) {
#ifdef _WIN32
        if (WSAGetLastError() == WSAETIMEDOUT) {
            return %[6]s_empty();
        }
#else
        // EINTR - см. package doc rech_accept в builtins_shared.go
        // (plainSocketOpsCode) - тот же смысл здесь.
        if (errno == EAGAIN || errno == EWOULDBLOCK || errno == EINTR) {
            return %[6]s_empty();
        }
#endif
        rech_panic("%[1]s");
    }
    SSL_CTX *listen_ctx = rech_tls_listen_ctx(s);
    if (listen_ctx != NULL) {
        SSL *ssl = SSL_new(listen_ctx);
        SSL_set_fd(ssl, (int)c);
        if (SSL_accept(ssl) <= 0) {
            SSL_free(ssl);
#ifdef _WIN32
            closesocket(c);
#else
            close(c);
#endif
            rech_panic("%[2]s");
        }
        rech_tls_register(c, NULL, ssl);
    }
    return %[6]s_wrap(c);
}

static void rech_send(rech_socket_t s, const char *data) {
    SSL *ssl = rech_tls_conn(s);
    size_t len = strlen(data);
    size_t sent = 0;
    while (sent < len) {
        long long n = ssl != NULL
            ? SSL_write(ssl, data + sent, (int)(len - sent))
            : send(s, data + sent, (int)(len - sent), 0);
        if (n <= 0) {
            rech_panic("%[3]s");
        }
        sent += (size_t)n;
    }
}

// rech_socket_read_raw - см. package doc её же в plainSocketOpsCode
// (builtins_shared.go, вариант без TLS) - ТА ЖЕ роль здесь: единственная
// точка настоящего чтения, через которую идут rech_recv ниже,
// rech_recv_http_request (httpserver.go) и rech_recv_until_close
// (httpHelpers) - без этого TLS работал бы только для "получить" напрямую,
// но не для HTTP-сервера/клиента поверх него (найденный и исправленный
// баг - "принять_http" зависал, читая ещё зашифрованные байты raw recv()).
static long long rech_socket_read_raw(rech_socket_t s, char *buf, long long max) {
    SSL *ssl = rech_tls_conn(s);
    return ssl != NULL ? SSL_read(ssl, buf, (int)max) : recv(s, buf, (int)max, 0);
}

static char *rech_recv(rech_socket_t s, long long max) {
    if (max <= 0) {
        rech_panic("%[4]s");
    }
    char *buf = malloc((size_t)max + 1);
    long long n = rech_socket_read_raw(s, buf, max);
    if (n < 0) {
        free(buf);
        rech_panic("%[5]s");
    }
    buf[n] = '\0';
    return buf;
}

static void rech_close_socket(rech_socket_t s) {
    rech_tls_entry_t *entry = rech_tls_unregister(s);
    if (entry != NULL) {
        if (entry->ssl != NULL) {
            SSL_shutdown(entry->ssl);
            SSL_free(entry->ssl);
        }
        if (entry->listen_ctx != NULL) {
            SSL_CTX_free(entry->listen_ctx);
        }
        free(entry);
    }
#ifdef _WIN32
    closesocket(s);
#else
    close(s);
#endif
}

`, rtmsg.SocketAcceptFailed, rtmsg.TLSHandshakeFailed, rtmsg.SocketSendFailed, rtmsg.SocketRecvInvalidMax, rtmsg.SocketRecvFailed, mangleType("?Сокет"))

// tlsListenSecureCode - "слушать_tcp_защищённо(порт, файл_сертификата,
// файл_ключа)": обычный rech_listen_tcp (bind+listen, socketTypeAndConnectCode
// - TLS тут ни при чём, это просто TCP) + SSL_CTX, настроенный на PEM-пару
// сертификат/ключ, зарегистрированный за fd слушателя - rech_accept выше
// сам находит его и делает TLS-рукопожатие на каждом принятом соединении.
// TLS_server_method()/неявная OPENSSL_init_ssl - OpenSSL >= 1.1.0 (более
// старые версии требовали SSLv23_server_method()+явный SSL_library_init() -
// не поддерживаются, современный минимум).
var tlsListenSecureCode = fmt.Sprintf(`static rech_socket_t rech_listen_tcp_secure(long long port, const char *certfile, const char *keyfile) {
#ifndef _WIN32
    // SIGPIPE - см. package doc socketTypeAndConnectCode (builtins_shared.go) -
    // тот же вызов ЕСТЬ и в rech_listen_tcp там (socketHelpersTLS несёт
    // весь socketTypeAndConnectCode безусловно), но КОД функции - не то же
    // самое, что её ВЫЗОВ: программа, слушающая ТОЛЬКО через "слушать_
    // tcp_защищённо", никогда не выполнит rech_listen_tcp вообще - вызов
    // здесь обязателен, не избыточен.
    signal(SIGPIPE, SIG_IGN);
#endif
    SSL_CTX *ctx = SSL_CTX_new(TLS_server_method());
    if (ctx == NULL) {
        rech_panic("%[1]s");
    }
    if (SSL_CTX_use_certificate_file(ctx, certfile, SSL_FILETYPE_PEM) <= 0 ||
        SSL_CTX_use_PrivateKey_file(ctx, keyfile, SSL_FILETYPE_PEM) <= 0) {
        SSL_CTX_free(ctx);
        rech_panic("%[1]s");
    }
    rech_socket_t s = rech_listen_tcp(port);
    rech_tls_register(s, ctx, NULL);
    return s;
}
`, rtmsg.TLSCertLoadFailed)

// socketHelpersTLS - вариант socketHelpers (builtins_shared.go) для
// программы, использующей "слушать_tcp_защищённо" - см. package doc выше.
var socketHelpersTLS = socketTypeAndConnectCode + tlsRegistryCode + tlsSocketOpsCode + tlsListenSecureCode
