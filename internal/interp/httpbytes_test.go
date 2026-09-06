package interp

import (
	"bytes"
	"fmt"
	"io"
	"net"
	"testing"
	"time"
)

// TestRunHTTPServerBytesRoundTripsBinaryBody - docs/ROADMAP.md, загрузка/
// раздача файлов - "принять_http_байты" обязан сохранять тело запроса
// побайтово, включая нулевые байты (в отличие от "принять_http", у
// которого "тело: Строка" может их обрезать - хотя в interp, чистом Go,
// строки и так бинарно-безопасны; проверка здесь - на симметрию с
// cbackend и на то, что "тело" действительно приходит как "Список<Байт>",
// не как что-то другое).
func TestRunHTTPServerBytesRoundTripsBinaryBody(t *testing.T) {
	probe, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("net.Listen: %v", err)
	}
	port := probe.Addr().(*net.TCPAddr).Port
	probe.Close()

	body := []byte{0, 1, 2, 255, 254, 0, 128, 10, 13, 0, 200, 201}
	clientDone := make(chan error, 1)
	var respBody []byte
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
		req := fmt.Sprintf("POST /upload HTTP/1.1\r\nHost: localhost\r\nContent-Length: %d\r\n\r\n", len(body))
		if _, err := io.WriteString(conn, req); err != nil {
			clientDone <- fmt.Errorf("отправка заголовков: %w", err)
			return
		}
		if _, err := conn.Write(body); err != nil {
			clientDone <- fmt.Errorf("отправка тела: %w", err)
			return
		}
		raw, err := io.ReadAll(conn)
		if err != nil {
			clientDone <- fmt.Errorf("чтение ответа: %w", err)
			return
		}
		if _, after, ok := bytes.Cut(raw, []byte("\r\n\r\n")); ok {
			respBody = after
		}
		clientDone <- nil
	}()

	got := runSrc(t, fmt.Sprintf(`
функция главная
{
    переменная слушатель = сетевой_узел("tcp", "", %d)
    переменная соединение = принять(слушатель)
    если соединение != Ничто
    {
        переменная запрос = принять_http_байты(соединение)
        вывести запрос.метод
        вывести запрос.тело.длина()

        переменная ответ = новый ОтветБайты(200, "", запрос.тело)
        отправить_http_ответ_байты(соединение, ответ)
        закрыть_сокет(соединение)
    }
    закрыть_сокет(слушатель)
}
`, port))

	if err := <-clientDone; err != nil {
		t.Fatal(err)
	}
	want := fmt.Sprintf("POST\n%d\n", len(body))
	if got != want {
		t.Fatalf("получено %q, хотели %q", got, want)
	}
	if !bytes.Equal(respBody, body) {
		t.Fatalf("тело ответа: получено % x, хотели % x", respBody, body)
	}
}
