package interp

import (
	"fmt"
	"net"
	"strings"
	"testing"

	"github.com/IlyaFeoktistov/Rech/internal/rtmsg"
)

// TestRunHTTPRequestHTTPSAttemptsRealTLSHandshake - HTTP
// highload пункт 5 - "запрос" по адресу с "https://" обязан ДЕЙСТВИТЕЛЬНО
// попытаться TLS-рукопожатие, а не молча свестись к обычному HTTP на порту
// 443 (иначе "https://" ничего не значило бы). Сервер здесь - самый
// обычный (не TLS) TCP-слушатель: клиент, попытавшийся начать TLS-
// рукопожатие с ним, обязан провалиться (не получить обычный HTTP-ответ,
// как получил бы, будь "https://" проигнорирован).
func TestRunHTTPRequestHTTPSAttemptsRealTLSHandshake(t *testing.T) {
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("net.Listen: %v", err)
	}
	defer ln.Close()
	port := ln.Addr().(*net.TCPAddr).Port
	go func() {
		c, err := ln.Accept()
		if err != nil {
			return
		}
		defer c.Close()
		buf := make([]byte, 4096)
		c.Read(buf)
		// Обычный (не TLS) HTTP-ответ - клиент не должен его увидеть, если
		// TLS-рукопожатие действительно попытались (сервер не понимает
		// TLS-запись, соединение оборвётся раньше).
		c.Write([]byte("HTTP/1.1 200 OK\r\n\r\nне-TLS ответ"))
	}()

	err = runSrcExpectRuntimeError(t, fmt.Sprintf(`
функция главная
{
    переменная р = новый Запрос("GET", "https://127.0.0.1", %d, "/", "", "")
    переменная о = запрос(р)
    вывести о.код
}
`, port))
	if err == nil || !strings.Contains(err.Error(), rtmsg.TLSClientHandshakeFailed) {
		t.Fatalf("ожидалась ошибка %q (TLS-рукопожатие с не-TLS сервером обязано провалиться), получено: %v", rtmsg.TLSClientHandshakeFailed, err)
	}
}
