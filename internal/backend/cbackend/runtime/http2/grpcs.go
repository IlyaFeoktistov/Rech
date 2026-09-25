// grpcs - TLS+ALPN "h2" поверх уже готового HTTP/2 (http2.go) и уже
// готового TLS-биндинга (tls.go, OpenSSL) - НЕ трогает существующие
// "слушать_tcp_защищённо"/"запрос"/rech_connect_tcp_tls вовсе (разные,
// параллельные функции с собственными SSL_CTX): ALPN-колбэк ниже требует
// именно "h2" - если бы он сидел на ОБЩЕМ SSL_CTX с обычным HTTPS-сервером
// ("принять_http"), настоящий браузер, предлагающий И "h2", И "http/1.1",
// получил бы "h2" от ЭТОГО колбэка, хотя сервер реально понимает только
// HTTP/1.1-фрейминг ("принять_http") - живая находка при проектировании
// (не гипотетическая), решённая тем же приёмом, что и весь остальной
// TLS-рантайм: НОВАЯ функция/СВОЙ SSL_CTX на fd, не общая точка входа.
//
// Серверная сторона gRPC-СЕССИИ переиспользует "http2_принять_сессию" без
// единой правки на уровне Rech-программы (см. acceptSessionAcceptCode,
// http2.go) - тот listener, что вернула "слушать_grpcs" ниже, уже
// зарегистрирован в rech_tls_registry (tls.go) ТОЧНО так же, как у обычной
// "слушать_tcp_защищённо" - rech_accept сама делает SSL_accept, находит
// уже согласованный ALPN не нужно вовсе (он УЖЕ подразумевается тем, что
// вызывающая программа осознанно позвала "слушать_grpcs", не "слушать_
// tcp_защищённо"). Клиентская сторона - ОТДЕЛЬНАЯ функция ниже
// (rech_connect_tcp_tls, tls.go, ALPN не предлагает вовсе - см. её же
// package doc про риск того же рода для "запрос").
package http2

import "fmt"

// GRPCSALPNProtoListC - wire-формат ALPN protocol list (RFC 7301): один
// байт длины + сами байты, повторить на каждый протокол - здесь только
// "h2" (2 байта) - НЕ "h2c" (cleartext, не имеет смысла внутри TLS) и НЕ
// "http/1.1" (эта функция - только для gRPC, http2_принять_сессию не
// умеет обслуживать HTTP/1.1-фрейминг).
const GRPCSALPNProtoListC = `"\x02h2"`

// GRPCSListenAndConnectCode - "слушать_grpcs(порт, серт, ключ): Сокет" +
// "новая_grpcs_сессия(хост, порт): HTTP2Сессия" - см. package doc.
// Сообщения - уже экранированные (escapeCString - внутренняя,
// неэкспортированная инфраструктура cbackend, этот пакет от неё не
// зависит).
func GRPCSListenAndConnectCode(certLoadFailedMsg, clientHandshakeFailedMsg, alpnRejectedMsg string) string {
	return fmt.Sprintf(`static int rech_grpcs_alpn_select_cb(SSL *ssl, const unsigned char **out, unsigned char *outlen,
                                      const unsigned char *in, unsigned int inlen, void *arg) {
    (void)ssl;
    (void)arg;
    static const unsigned char h2[] = %[1]s;
    if (SSL_select_next_proto((unsigned char **)out, outlen, h2, sizeof(h2) - 1, in, inlen) != OPENSSL_NPN_NEGOTIATED) {
        return SSL_TLSEXT_ERR_NOACK;
    }
    return SSL_TLSEXT_ERR_OK;
}

static rech_socket_t rech_listen_tcp_grpcs(long long port, const char *certfile, const char *keyfile) {
#ifndef _WIN32
    signal(SIGPIPE, SIG_IGN);
#endif
    SSL_CTX *ctx = SSL_CTX_new(TLS_server_method());
    if (ctx == NULL) {
        rech_error_as(RECH_ERR_NETWORK, "%[2]s");
    }
    if (SSL_CTX_use_certificate_file(ctx, certfile, SSL_FILETYPE_PEM) <= 0 ||
        SSL_CTX_use_PrivateKey_file(ctx, keyfile, SSL_FILETYPE_PEM) <= 0) {
        SSL_CTX_free(ctx);
        rech_error_as(RECH_ERR_NETWORK, "%[2]s");
    }
    SSL_CTX_set_alpn_select_cb(ctx, rech_grpcs_alpn_select_cb, NULL);
    rech_socket_t s = rech_listen_tcp(port);
    rech_tls_register(s, ctx, NULL);
    return s;
}

static rech_http2_session_t *rech_grpcs_new_session(const char *host, long long port) {
    rech_socket_t fd = rech_connect_tcp(host, port);
    SSL_CTX *ctx = SSL_CTX_new(TLS_client_method());
    if (ctx == NULL) {
        rech_error_as(RECH_ERR_NETWORK, "%[3]s");
    }
    static const unsigned char h2[] = %[1]s;
    SSL_CTX_set_alpn_protos(ctx, h2, sizeof(h2) - 1);
    SSL *ssl = SSL_new(ctx);
    SSL_CTX_free(ctx);
    if (ssl == NULL) {
        rech_error_as(RECH_ERR_NETWORK, "%[3]s");
    }
    SSL_set_fd(ssl, (int)fd);
    if (SSL_connect(ssl) <= 0) {
        SSL_free(ssl);
#ifdef _WIN32
        closesocket(fd);
#else
        close(fd);
#endif
        rech_error_as(RECH_ERR_NETWORK, "%[3]s");
    }
    const unsigned char *negotiated = NULL;
    unsigned int negotiated_len = 0;
    SSL_get0_alpn_selected(ssl, &negotiated, &negotiated_len);
    if (negotiated == NULL || negotiated_len != 2 || memcmp(negotiated, "h2", 2) != 0) {
        SSL_free(ssl);
#ifdef _WIN32
        closesocket(fd);
#else
        close(fd);
#endif
        rech_error_as(RECH_ERR_NETWORK, "%[4]s");
    }

    rech_http2_session_t *sess = GC_MALLOC(sizeof(rech_http2_session_t));
    sess->fd = fd;
    sess->is_client = 1;
    sess->tls_ssl = ssl;
    long long authlen = (long long)strlen(host) + 32;
    sess->authority = GC_MALLOC((size_t)authlen);
    snprintf(sess->authority, (size_t)authlen, "%%s:%%lld", host, port);
    nghttp2_session_callbacks *cbs = rech_http2_new_callbacks();
    int rv = nghttp2_session_client_new(&sess->ng, cbs, sess);
    nghttp2_session_callbacks_del(cbs);
    if (rv != 0) {
        rech_error_as(RECH_ERR_NETWORK, "%[3]s");
    }
    nghttp2_submit_settings(sess->ng, NGHTTP2_FLAG_NONE, NULL, 0);
    nghttp2_session_send(sess->ng);
    rech_http2_start_thread(sess);
    return sess;
}

`, GRPCSALPNProtoListC, certLoadFailedMsg, clientHandshakeFailedMsg, alpnRejectedMsg)
}
