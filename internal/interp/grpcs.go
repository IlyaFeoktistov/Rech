// grpcs - TLS+ALPN "h2" поверх уже готового HTTP/2 (http2.go) - в отличие
// от cbackend (grpcs.go там), здесь ДВА builtin'а не требуют вообще
// никакой отдельной инфраструктуры: crypto/tls у Go уже даёт ALPN через
// tls.Config.NextProtos, а серверная сторона САМОЙ сессии - уже
// существующая http2AcceptSession (http2.go) - TLS полностью прозрачна
// для неё (Listener.Accept() отдаёт net.Conn независимо от того, TLS он
// или нет, рукопожатие - ленивое, на первом Read/Write, делает его сам
// http2.Server изнутри ServeConn) - никакой отдельной "grpcs_принять_
// сессию" не нужно, см. package doc internal/backend/cbackend/grpcs.go
// (тот же принцип, там - явно, здесь - бесплатно за счёт net.Listener/
// net.Conn абстракции Go).
package interp

import (
	"crypto/tls"
	"fmt"
	"net"
	"strconv"

	"golang.org/x/net/http2"

	"github.com/IlyaFeoktistov/Rech/internal/ir"
	"github.com/IlyaFeoktistov/Rech/internal/rtmsg"
)

// listenGRPCS - "слушать_grpcs(порт, файл_сертификата, файл_ключа): Сокет" -
// как listenTLS (network.go), но с NextProtos: ["h2"] - НЕ модификация
// самой listenTLS (общая точка входа сломала бы обычный "слушать_tcp_
// защищённо", см. package doc grpcs.go в cbackend).
func listenGRPCS(port int64, certFile, keyFile string) Value {
	cert, err := tls.LoadX509KeyPair(certFile, keyFile)
	if err != nil {
		panicRuntime("%s", rtmsg.GRPCSCertLoadFailed)
	}
	cfg := &tls.Config{Certificates: []tls.Certificate{cert}, NextProtos: []string{"h2"}}
	rawL, err := net.Listen("tcp", fmt.Sprintf(":%d", port))
	if err != nil {
		panicRuntime("%s", rtmsg.SocketBindFailed)
	}
	l := tls.NewListener(rawL, cfg)
	return Value{Type: ir.TypeSocket, Listener: l, RawListener: rawL}
}

// grpcsNewSession - "новая_grpcs_сессия(хост, порт): HTTP2Сессия" - как
// http2NewSession (http2.go), но настоящий tls.Dial с ALPN "h2" вместо
// h2c-трюка (AllowHTTP/DialTLSContext-подмена) - проверяет реально
// согласованный протокол (ConnectionState().NegotiatedProtocol), не
// молча продолжает с TLS-сессией, которая ALPN не подтвердила.
func (in *Interp) grpcsNewSession(host string, port int64) Value {
	addr := net.JoinHostPort(host, strconv.FormatInt(port, 10))
	conn, err := tls.Dial("tcp", addr, &tls.Config{NextProtos: []string{"h2"}})
	if err != nil {
		panicRuntime("%s", rtmsg.GRPCSClientHandshakeFailed)
	}
	if conn.ConnectionState().NegotiatedProtocol != "h2" {
		conn.Close()
		panicRuntime("%s", rtmsg.GRPCSALPNRejected)
	}
	transport := &http2.Transport{}
	clientConn, err := transport.NewClientConn(conn)
	if err != nil {
		conn.Close()
		panicRuntime("%s", rtmsg.HTTP2ClientConnFailed)
	}
	return sessionValue(&http2Session{conn: conn, clientConn: clientConn, authority: addr, streams: map[int64]*http2StreamState{}})
}
