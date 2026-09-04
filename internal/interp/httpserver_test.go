package interp

import (
	"fmt"
	"io"
	"net"
	"testing"
	"time"
)

// TestRunHTTPServerBuiltins - "принять_http"/"отправить_http_ответ" через
// настоящий TCP-сокет: клиент - обычный net.Dial в фоновой горутине,
// отправляющий вручную-собранный сырой HTTP-запрос (не net/http.Client -
// нужен полный контроль над тем, что именно отправляется по проводу, тем
// же принципом, что и TestRunHTTPRequestBuiltin для клиентской стороны).
// runSrc (сервер на Rech) выполняется синхронно в основной горутине теста
// (t.Fatalf внутри нужно звать именно оттуда - testing.T это требует);
// клиентская горутина сама повторяет подключение (dial retry), пока
// "сетевой_узел" внутри runSrc не свяжет порт - без фиксированной задержки.
func TestRunHTTPServerBuiltins(t *testing.T) {
	probe, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("net.Listen: %v", err)
	}
	port := probe.Addr().(*net.TCPAddr).Port
	probe.Close()

	type clientResult struct {
		raw string
		err error
	}
	clientDone := make(chan clientResult, 1)
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
			clientDone <- clientResult{err: fmt.Errorf("net.Dial: %w", dialErr)}
			return
		}
		defer conn.Close()

		req := "POST /ping HTTP/1.1\r\nHost: localhost\r\nContent-Length: 8\r\n\r\nтест"
		if _, err := io.WriteString(conn, req); err != nil {
			clientDone <- clientResult{err: fmt.Errorf("отправка запроса: %w", err)}
			return
		}
		raw, err := io.ReadAll(conn)
		if err != nil {
			clientDone <- clientResult{err: fmt.Errorf("чтение ответа: %w", err)}
			return
		}
		clientDone <- clientResult{raw: string(raw)}
	}()

	got := runSrc(t, fmt.Sprintf(`
функция главная
{
    переменная слушатель = сетевой_узел("tcp", "", %d)
    переменная соединение = принять(слушатель)
    если соединение != Ничто
    {
        переменная запрос = принять_http(соединение)
        вывести запрос.метод
        вывести запрос.путь
        вывести запрос.тело

        переменная ответ = новый Ответ(200, "Content-Type: text/plain", "привет")
        отправить_http_ответ(соединение, ответ)
        закрыть_сокет(соединение)
    }
    закрыть_сокет(слушатель)
}
`, port))

	res := <-clientDone
	if res.err != nil {
		t.Fatal(res.err)
	}
	if want := "HTTP/1.1 200 OK\r\n"; len(res.raw) < len(want) || res.raw[:len(want)] != want {
		t.Fatalf("ожидался статус 200, получено:\n%s", res.raw)
	}
	if want := "привет"; len(res.raw) < len(want) || res.raw[len(res.raw)-len(want):] != want {
		t.Fatalf("ожидалось тело \"привет\" в конце ответа, получено:\n%s", res.raw)
	}

	want := "POST\n/ping\nтест\n"
	if got != want {
		t.Fatalf("получено %q, хотели %q", got, want)
	}
}

// TestRunHTTPServerDecodesPercentEncodedPath - НАЙДЕННЫЙ В ROADMAP (шаг 41)
// пробел: настоящие HTTP-клиенты (curl, браузеры) percent-encode
// нелатинские байты в ПУТИ (RFC 3986) сами - маршрут "/задачи" никогда не
// совпал бы с тем, что реально пришло по проводу
// ("/%D0%B7%D0%B0%D0%B4%D0%B0%D1%87%D0%B8"), если "принять_http" не
// декодирует сегмент пути. Query-строка после "?" декодируется отдельно
// (параметры_пути) - "запрос.путь" сохраняет её как есть, тем же принципом,
// что описан в маршрутизатор.rech.
func TestRunHTTPServerDecodesPercentEncodedPath(t *testing.T) {
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
		// "/задачи?имя=привет" - путь percent-encoded (кириллица в самом
		// пути, не в query), query-строка - как реальный curl её оставил бы.
		req := "GET /%D0%B7%D0%B0%D0%B4%D0%B0%D1%87%D0%B8?%D0%B8%D0%BC%D1%8F=%D0%BF%D1%80%D0%B8%D0%B2%D0%B5%D1%82 HTTP/1.1\r\nHost: localhost\r\nContent-Length: 0\r\n\r\n"
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
    переменная слушатель = сетевой_узел("tcp", "", %d)
    переменная соединение = принять(слушатель)
    если соединение != Ничто
    {
        переменная запрос = принять_http(соединение)
        вывести запрос.путь
        переменная параметры = параметры_пути(запрос.путь)
        вывести параметры.получить("имя")

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
	want := "/задачи?%D0%B8%D0%BC%D1%8F=%D0%BF%D1%80%D0%B8%D0%B2%D0%B5%D1%82\nпривет\n"
	if got != want {
		t.Fatalf("получено %q, хотели %q", got, want)
	}
}

func TestRunQueryParamsBuiltin(t *testing.T) {
	got := runSrc(t, `
функция главная
{
    переменная параметры = параметры_пути("/ping?имя=Иван%20Петров&возраст=30&пусто=")
    вывести параметры.длина()
    вывести параметры.получить("имя")
    вывести параметры.получить("возраст")
    вывести параметры.содержит("пусто")
    вывести параметры.содержит("нет_такого")

    переменная без_вопроса = параметры_пути("/ping")
    вывести без_вопроса.длина()
}
`)
	want := "3\nИван Петров\n30\nистина\nложь\n0\n"
	if got != want {
		t.Fatalf("получено %q, хотели %q", got, want)
	}
}
