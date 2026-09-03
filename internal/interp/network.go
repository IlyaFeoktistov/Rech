package interp

import (
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
// "подключиться" (net.Dial). mode строкой - неизвестный режим (не
// "tcp"/"udp") - паника в рантайме, тем же принципом, что и у cbackend
// (rech_network_node, builtins_shared.go).
func (in *Interp) networkNode(mode, host string, port int64) Value {
	listening := host == ""
	switch mode {
	case "tcp":
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
	case "udp":
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
