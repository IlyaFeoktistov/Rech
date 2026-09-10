package interp

import (
	"fmt"
	"io"
	"net"
	"testing"
	"time"
)

// TestRunHTTPRequestDecodesChunkedResponse - docs/ROADMAP.md, HTTP
// highload пункт 4 - сервер отвечает без Content-Length, вместо него
// "Transfer-Encoding: chunked" (несколько чанков вперемешку с их
// шестнадцатеричными размерами и терминирующим нулевым чанком) - "запрос"
// обязан вернуть уже РАСКОДИРОВАННОЕ тело, без размеров чанков/CRLF внутри
// него.
func TestRunHTTPRequestDecodesChunkedResponse(t *testing.T) {
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
		c.Write([]byte("HTTP/1.1 200 OK\r\nTransfer-Encoding: chunked\r\n\r\n" +
			"5\r\nHello\r\n" +
			fmt.Sprintf("%x\r\n, мир!\r\n", len(", мир!")) +
			"0\r\n\r\n"))
	}()

	got := runSrc(t, fmt.Sprintf(`
функция главная
{
    переменная р = новый Запрос("GET", "127.0.0.1", %d, "/", "", "")
    переменная о = запрос(р)
    вывести о.код
    вывести о.тело
}
`, port))
	if got != "200\nHello, мир!\n" {
		t.Fatalf("получено %q, хотели \"200\\nHello, мир!\\n\"", got)
	}
}

// TestRunHTTPServerDecodesChunkedRequestBody - зеркально предыдущему, но
// для сервера: клиент шлёт запрос без Content-Length, вместо него
// "Transfer-Encoding: chunked" - "принять_http" обязан вернуть уже
// раскодированное "тело".
func TestRunHTTPServerDecodesChunkedRequestBody(t *testing.T) {
	probe, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("net.Listen: %v", err)
	}
	port := probe.Addr().(*net.TCPAddr).Port
	probe.Close()

	clientDone := make(chan error, 1)
	go func() {
		var conn net.Conn
		var dialErr error
		for range 200 {
			conn, dialErr = net.Dial("tcp", fmt.Sprintf("127.0.0.1:%d", port))
			if dialErr == nil {
				break
			}
			time.Sleep(5 * time.Millisecond)
		}
		if dialErr != nil {
			clientDone <- fmt.Errorf("net.Dial: %w", dialErr)
			return
		}
		defer conn.Close()
		req := "POST /upload HTTP/1.1\r\nHost: localhost\r\nTransfer-Encoding: chunked\r\n\r\n" +
			"4\r\ndata\r\n" +
			"6\r\n-more-\r\n" +
			"0\r\n\r\n"
		if _, err := io.WriteString(conn, req); err != nil {
			clientDone <- fmt.Errorf("отправка запроса: %w", err)
			return
		}
		io.ReadAll(conn)
		clientDone <- nil
	}()

	got := runSrc(t, fmt.Sprintf(`
функция главная
{
    переменная слушатель = сетевой_узел(СетевойПротокол.TCP, "", %d)
    переменная соединение = принять(слушатель)
    если соединение != Ничто
    {
        переменная запрос = принять_http(соединение)
        вывести запрос.метод
        вывести запрос.тело

        переменная ответ = новый Ответ(200, "", "")
        отправить_http_ответ(соединение, ответ)
        закрыть_сокет(соединение)
    }
    закрыть_сокет(слушатель)
}
`, port))

	if err := <-clientDone; err != nil {
		t.Fatal(err)
	}
	want := "POST\ndata-more-\n"
	if got != want {
		t.Fatalf("получено %q, хотели %q", got, want)
	}
}
