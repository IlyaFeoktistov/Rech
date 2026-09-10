package interp

import (
	"crypto/tls"
	"fmt"
	"net"
	"strconv"

	"github.com/IlyaFeoktistov/Rech/internal/ir"
	"github.com/IlyaFeoktistov/Rech/internal/rtmsg"
)

// networkNode - "сетевой_узел(режим, хост, порт)" - единая точка входа
// вместо прежних "слушать_tcp"/"подключиться_tcp"/"подключиться_udp": host
// == "" решает "слушать" (net.Listen для tcp, net.ListenPacket для udp -
// см. package doc interp.go, Value.PacketConn), непустой host -
// "подключиться" (net.Dial). mode - индекс СетевойПротокол (types.go: TCP=0,
// UDP=1), не строка (была строкой "tcp"/"udp" - НАЙДЕНО по ходу
// examples/проекты/чат_сервер: опечатка молча давала бы рантайм-панику
// вместо ошибки компиляции) - значение вне этих двух достижимо только
// явным целочисленным литералом мимо самого перечисления - паника в
// рантайме, тем же принципом, что и у cbackend (rech_network_node,
// builtins_shared.go).
func (in *Interp) networkNode(mode int64, host string, port int64) Value {
	listening := host == ""
	switch mode {
	case 0:
		if listening {
			l, err := net.Listen("tcp", fmt.Sprintf(":%d", port))
			if err != nil {
				panicRuntime("%s", rtmsg.SocketBindFailed)
			}
			return listenerValue(l)
		}
		c, err := net.Dial("tcp", net.JoinHostPort(host, strconv.FormatInt(port, 10)))
		if err != nil {
			panicRuntime("%s", rtmsg.SocketConnectFailed)
		}
		return connValue(c)
	case 1:
		if listening {
			pc, err := net.ListenPacket("udp", fmt.Sprintf(":%d", port))
			if err != nil {
				panicRuntime("%s", rtmsg.SocketBindFailed)
			}
			return packetConnValue(pc)
		}
		c, err := net.Dial("udp", net.JoinHostPort(host, strconv.FormatInt(port, 10)))
		if err != nil {
			panicRuntime("%s", rtmsg.SocketConnectFailed)
		}
		return connValue(c)
	default:
		panicRuntime("%s", rtmsg.SocketUnknownMode)
		return Value{}
	}
}

// listenTLS - "слушать_tcp_защищённо(порт, файл_сертификата, файл_ключа)" -
// crypto/tls стандартной библиотеки Go, НЕ собственная реализация (в
// отличие от SHA-256/HMAC, crypto.go - настоящий TLS-стек с нуля - риск для
// безопасности, см. package doc internal/backend/cbackend/tls.go, тот же
// принцип на этом backend'е). tls.NewListener(rawL, cfg), не tls.Listen
// напрямую - нужен отдельный указатель на rawL (сырой net.Listen("tcp",
// ...) ДО обёртки), см. package doc Value.RawListener - "принять"
// (builtins.go) дальше вызывает Accept() ТЕМ ЖЕ кодом, что и у обычного
// слушателя (никакого отдельного пути) - явный tc.Handshake() там же
// форсирует рукопожатие СИНХРОННО, в момент "принять" - у Go *tls.Conn оно
// иначе ленивое (на первом Read/Write), а cbackend (rech_accept, tls.go)
// делает SSL_accept сразу - расхождение поведения между backend'ами иначе
// было бы наблюдаемым (когда именно всплывает ошибка рукопожатия).
func listenTLS(port int64, certFile, keyFile string) Value {
	cert, err := tls.LoadX509KeyPair(certFile, keyFile)
	if err != nil {
		panicRuntime("%s", rtmsg.TLSCertLoadFailed)
	}
	cfg := &tls.Config{Certificates: []tls.Certificate{cert}}
	rawL, err := net.Listen("tcp", fmt.Sprintf(":%d", port))
	if err != nil {
		panicRuntime("%s", rtmsg.SocketBindFailed)
	}
	l := tls.NewListener(rawL, cfg)
	return Value{Type: ir.TypeSocket, Listener: l, RawListener: rawL}
}

// recvFrom - "получить_от" - одна входящая UDP-датаграмма вместе с адресом
// ЕЁ ОТПРАВИТЕЛЯ (в отличие от "получить" на подключённом сокете, где
// собеседник уже зафиксирован самим соединением) - см. package doc
// internal/semantic/network.go, datagramFields.
func (in *Interp) recvFrom(sock Value, max int64) Value {
	if sock.PacketConn == nil {
		panicRuntime("%s", rtmsg.SocketWrongKind)
	}
	if max <= 0 {
		panicRuntime("%s", rtmsg.SocketRecvInvalidMax)
	}
	buf := make([]byte, max)
	n, addr, err := sock.PacketConn.ReadFrom(buf)
	if err != nil {
		panicRuntime("%s", rtmsg.SocketRecvFailed)
	}
	host, portStr, err := net.SplitHostPort(addr.String())
	if err != nil {
		panicRuntime("%s", rtmsg.SocketRecvFailed)
	}
	port, err := strconv.ParseInt(portStr, 10, 64)
	if err != nil {
		panicRuntime("%s", rtmsg.SocketRecvFailed)
	}
	return Value{Type: ir.EntityType("Дейтаграмма"), Obj: &entityObj{typeName: "Дейтаграмма", fields: map[string]Value{
		"данные": stringValue(string(buf[:n])), "адрес": stringValue(host), "порт": intValue(port),
	}}}
}

// sendTo - "отправить_в" - ответ конкретному отправителю (addr/port из
// "Дейтаграмма", полученной "получить_от") через тот же udp-узел, без
// отдельного "подключения".
func (in *Interp) sendTo(sock Value, data, addr string, port int64) {
	if sock.PacketConn == nil {
		panicRuntime("%s", rtmsg.SocketWrongKind)
	}
	target, err := net.ResolveUDPAddr("udp", net.JoinHostPort(addr, strconv.FormatInt(port, 10)))
	if err != nil {
		panicRuntime("%s", rtmsg.SocketResolveFailed)
	}
	if _, err := sock.PacketConn.WriteTo([]byte(data), target); err != nil {
		panicRuntime("%s", rtmsg.SocketSendFailed)
	}
}
