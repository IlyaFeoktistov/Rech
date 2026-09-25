package interp

import (
	"crypto/tls"
	"net"
	"sync"
	"time"

	"github.com/IlyaFeoktistov/Rech/internal/ir"
	"github.com/IlyaFeoktistov/Rech/internal/rtmsg"
)

// deadlineSetter - подмножество net.Conn/net.Listener, у которых есть
// SetDeadline (net.Conn несёт его в самом интерфейсе, *net.TCPListener -
// тоже, но через конкретный тип, не через net.Listener - см. package doc
// Value.RawListener, почему "принять" не может просто спросить его у
// произвольного net.Listener).
type deadlineSetter interface {
	SetDeadline(time.Time) error
}

// socketTimeouts - "установить_таймаут(сокет, миллисекунды)" ставит
// СТОЯЩИЙ (не разовый) таймаут - как SO_RCVTIMEO в cbackend (builtins_
// shared.go, rech_set_timeout), не как разовый net.Conn.SetDeadline: он
// применяется заново к КАЖДОМУ следующему "получить"/"принять_http"/
// "принять" на этом сокете, не только к следующему вызову. Ключ - Listener/
// Conn как есть (интерфейсные значения по указателю на конкретный тип -
// сравнимы, разные Value могут держать один и тот же net.Conn/net.Listener
// - тот же сокет). PacketConn (udp-сервер) таймаута не имеет вообще - ни
// "получить_от", ни cbackend-эквивалент его не поддерживают.
var (
	socketTimeoutsMu sync.Mutex
	socketTimeouts   = map[any]time.Duration{}
)

// setSocketTimeout - "установить_таймаут". 0 или отрицательное значение
// снимает таймаут (сокет снова блокируется бессрочно) - тот же смысл, что
// у SO_RCVTIMEO{0} в cbackend.
func setSocketTimeout(sock Value, ms int64) {
	var key any
	switch {
	case sock.Listener != nil:
		key = sock.Listener
	case sock.Conn != nil:
		key = sock.Conn
	default:
		panicRuntime("%s", rtmsg.SocketWrongKind)
		return
	}
	socketTimeoutsMu.Lock()
	defer socketTimeoutsMu.Unlock()
	if ms <= 0 {
		delete(socketTimeouts, key)
		return
	}
	socketTimeouts[key] = time.Duration(ms) * time.Millisecond
}

// applyDeadline - если для key установлен стоящий таймаут, переносит его в
// разовый абсолютный дедлайн "сейчас + таймаут" на target (см. package doc
// socketTimeouts). target - Value.RawListener для TLS-слушателя (см. её
// package doc), иначе сам Listener/Conn.
func applyDeadline(key any, target deadlineSetter) {
	socketTimeoutsMu.Lock()
	d, ok := socketTimeouts[key]
	socketTimeoutsMu.Unlock()
	if !ok {
		return
	}
	target.SetDeadline(time.Now().Add(d))
}

// acceptWithTimeout - "принять": Ничто по истечении установленного
// таймаута (см. socketTimeouts), иначе - новый Сокет, как раньше. net.Error.
// Timeout() отличает истечение дедлайна от настоящей ошибки accept - только
// первое превращается в Ничто, второе по-прежнему паникует.
func (in *Interp) acceptWithTimeout(sock Value) Value {
	if sock.Listener == nil {
		panicRuntime("%s", rtmsg.SocketWrongKind)
	}
	deadlineTarget := sock.Listener
	if sock.RawListener != nil {
		deadlineTarget = sock.RawListener
	}
	if ds, ok := deadlineTarget.(deadlineSetter); ok {
		applyDeadline(sock.Listener, ds)
	}
	nullableType := ir.NullableType(ir.TypeSocket)
	c, err := sock.Listener.Accept()
	if err != nil {
		if ne, ok := err.(net.Error); ok && ne.Timeout() {
			return Value{Type: nullableType}
		}
		throwRuntimeError(rtmsg.ErrorNetwork, "%s", rtmsg.SocketAcceptFailed)
	}
	if tc, ok := c.(*tls.Conn); ok {
		if err := tc.Handshake(); err != nil {
			c.Close()
			throwRuntimeError(rtmsg.ErrorNetwork, "%s", rtmsg.TLSHandshakeFailed)
		}
	}
	v := connValue(c)
	return Value{Type: nullableType, Nullable: &v}
}
