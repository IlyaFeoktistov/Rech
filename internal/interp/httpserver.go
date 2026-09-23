// "принять_http"/"отправить_http_ответ" - минимальный HTTP-сервер поверх
// уже существующих сетевых примитивов (net.Conn из "принять") - см. package
// doc internal/semantic/httpserver.go и internal/backend/cbackend/
// httpserver.go (тот же алгоритм чтения запроса, здесь на Go).
package interp

import (
	"bytes"
	"fmt"
	"io"
	"net"
	"strconv"
	"strings"

	"github.com/IlyaFeoktistov/Rech/internal/ir"
	"github.com/IlyaFeoktistov/Rech/internal/rtmsg"
)

// queryParamsDictType - имя generic-инстанциации, зарегистрированной для
// "параметры_пути" принудительно (internal/semantic/httpserver.go,
// registerHTTPServerFuncs) - тот же принцип, что typeName "Запрос"/"Ответ"
// у httpRequest.
const queryParamsDictType = "Словарь<Строка, Строка>"

// queryParams - "параметры_пути" - разбирает часть строки path после "?"
// (если "?" нет - результат пуст) в пары "ключ=значение" через "&", с
// процент-декодированием (urlDecode) - тот же алгоритм, что и
// cbackend.rech_query_params (httpserver.go).
func queryParams(path string) Value {
	pairs := newDictPairs()
	_, query, found := strings.Cut(path, "?")
	if found {
		for pair := range strings.SplitSeq(query, "&") {
			if pair == "" {
				continue
			}
			key, value, _ := strings.Cut(pair, "=")
			key = urlDecode(key)
			if key == "" {
				continue
			}
			pairs.set(stringValue(key), stringValue(urlDecode(value)))
		}
	}
	return Value{Type: ir.EntityType(queryParamsDictType), Obj: &entityObj{typeName: queryParamsDictType, coll: &collection{template: "Словарь", pairs: pairs}}}
}

// urlDecode - "+" как пробел, "%XX" как байт по шестнадцатеричному коду
// (нераспознанный "%" без двух валидных hex-цифр следом - оставлен как
// есть, без ошибки - типичное снисходительное поведение разбора
// query-строки).
func urlDecode(s string) string {
	var sb strings.Builder
	for i := 0; i < len(s); i++ {
		switch {
		case s[i] == '+':
			sb.WriteByte(' ')
		case s[i] == '%' && i+2 < len(s):
			hi, hiOK := hexDigit(s[i+1])
			lo, loOK := hexDigit(s[i+2])
			if hiOK && loOK {
				sb.WriteByte(byte(hi<<4 | lo))
				i += 2
			} else {
				sb.WriteByte(s[i])
			}
		default:
			sb.WriteByte(s[i])
		}
	}
	return sb.String()
}

// urlDecodePath - процент-декодирование СЕГМЕНТА пути (не query-строки) -
// настоящие HTTP-клиенты (curl, браузеры) сами percent-encode нелатинские
// байты в пути (RFC 3986). В отличие от urlDecode (query-строка), "+" здесь
// НЕ означает пробел - путь не application/x-www-form-urlencoded.
func urlDecodePath(s string) string {
	var sb strings.Builder
	for i := 0; i < len(s); i++ {
		if s[i] == '%' && i+2 < len(s) {
			hi, hiOK := hexDigit(s[i+1])
			lo, loOK := hexDigit(s[i+2])
			if hiOK && loOK {
				sb.WriteByte(byte(hi<<4 | lo))
				i += 2
				continue
			}
		}
		sb.WriteByte(s[i])
	}
	return sb.String()
}

// decodeHTTPPath - декодирует только СЕГМЕНТ пути rawPath (до "?", если
// есть) - query-строка после "?" остаётся как есть, её отдельно декодирует
// queryParams (urlDecode) по своим правилам.
func decodeHTTPPath(rawPath string) string {
	seg, query, hasQuery := strings.Cut(rawPath, "?")
	seg = urlDecodePath(seg)
	if hasQuery {
		return seg + "?" + query
	}
	return seg
}

func hexDigit(c byte) (int, bool) {
	switch {
	case c >= '0' && c <= '9':
		return int(c - '0'), true
	case c >= 'a' && c <= 'f':
		return int(c-'a') + 10, true
	case c >= 'A' && c <= 'F':
		return int(c-'A') + 10, true
	default:
		return 0, false
	}
}

// recvHTTPRequest читает сырые байты входящего HTTP-запроса с conn - не до
// закрытия соединения (клиент держит его открытым, ожидая ответ, в отличие
// от httpRequest выше, где сервер закрывает соединение сам), а до конца
// заголовков ("\r\n\r\n") плюс ровно Content-Length байт тела (0, если
// заголовка Content-Length нет). maxRequestBytes - ir.Program.
// HTTPMaxRequestBytes (Interp.httpMaxRequestBytes) - запрос (заголовки и
// тело вместе, тот же буфер buf) сверх этого предела аварийно завершает
// чтение (rtmsg.HTTPServerRequestTooLarge), тем же приёмом, что и
// cbackend.rech_recv_http_request: как только известна Content-Length,
// сравнивается СУММА "уже прочитанные заголовки + заявленное тело" с
// пределом (не одна Content-Length - огромные заголовки сами по себе,
// без всякого тела, тоже обязаны быть отклонены), не читая тело вовсе,
// если сумма уже больше предела; и на каждой итерации накопления
// заголовков ДО терминатора (клиент, никогда не заканчивающий заголовки,
// иначе рос бы в памяти без предела тем же самым буфером).
func recvHTTPRequest(conn net.Conn, maxRequestBytes int) []byte {
	var buf []byte
	chunk := make([]byte, 4096)
	bodyStart := -1
	var contentLength int64
	isChunked := false
	for {
		if bodyStart < 0 {
			if idx := bytes.Index(buf, []byte("\r\n\r\n")); idx >= 0 {
				bodyStart = idx + 4
				isChunked = isChunkedHeaders(buf[:idx])
				if !isChunked {
					contentLength = httpContentLength(buf[:idx])
					// Заголовки + тело ВМЕСТЕ против предела, тем же
					// принципом, что cbackend.rech_recv_http_request - см.
					// её комментарий. Chunked-путь сюда не входит - общая
					// проверка len(buf) ниже страхует оба пути одинаково.
					if int64(bodyStart)+contentLength > int64(maxRequestBytes) {
						panicRuntime("%s", rtmsg.HTTPServerRequestTooLarge)
					}
				}
			} else if len(buf) >= maxRequestBytes {
				panicRuntime("%s", rtmsg.HTTPServerRequestTooLarge)
			}
		}
		if bodyStart >= 0 {
			if isChunked {
				if decoded, complete := tryDechunk(buf[bodyStart:]); complete {
					return append(append([]byte{}, buf[:bodyStart]...), decoded...)
				}
			} else if int64(len(buf)-bodyStart) >= contentLength {
				break
			}
		}
		// Общий предел на ВЕСЬ принятый буфер - для content_length-пути он
		// избыточен (уже проверен выше), для chunked-пути - единственная
		// защита от неограниченного роста (заранее известной длины тела
		// нет), тем же принципом, что и cbackend.rech_recv_http_request.
		if len(buf) >= maxRequestBytes {
			panicRuntime("%s", rtmsg.HTTPServerRequestTooLarge)
		}
		applyDeadline(conn, conn)
		n, err := conn.Read(chunk)
		if n > 0 {
			buf = append(buf, chunk[:n]...)
		}
		if err != nil {
			if err != io.EOF {
				panicRuntime("%s", rtmsg.HTTPServerRecvFailed)
			}
			break
		}
	}
	if isChunked && bodyStart >= 0 {
		// Соединение оборвалось раньше терминирующего чанка - best-effort,
		// тем же принципом, что и ранний EOF на content_length-пути.
		decoded, _ := tryDechunk(buf[bodyStart:])
		return append(append([]byte{}, buf[:bodyStart]...), decoded...)
	}
	return buf
}

// httpContentLength ищет заголовок "Content-Length" регистронезависимо в
// headers (байты до "\r\n\r\n") - тот же ручной регистронезависимый поиск,
// что и cbackend.rech_http_content_length, не strings.ToLower на весь текст
// заранее (не нужно - искомое слово короткое, а неймспейс заголовков может
// содержать произвольный текст тела ниже, если поиск вести по всему буферу
// вместо только headers).
func httpContentLength(headers []byte) int64 {
	const want = "content-length"
	for i := 0; i+len(want) <= len(headers); i++ {
		match := true
		for j := range len(want) {
			c := headers[i+j]
			if c >= 'A' && c <= 'Z' {
				c += 32
			}
			if c != want[j] {
				match = false
				break
			}
		}
		if !match {
			continue
		}
		rest := headers[i+len(want):]
		k := 0
		for k < len(rest) && (rest[k] == ':' || rest[k] == ' ' || rest[k] == '\t') {
			k++
		}
		rest = rest[k:]
		end := 0
		for end < len(rest) && rest[end] >= '0' && rest[end] <= '9' {
			end++
		}
		n, _ := strconv.ParseInt(string(rest[:end]), 10, 64)
		return n
	}
	return 0
}

// parseHTTPHead разбирает СТРОКУ запроса (метод, путь - с процент-
// декодированием сегмента до "?") и блок заголовков из raw - общая часть
// acceptHTTPRequest/acceptHTTPRequestBytes, различающихся только тем, КАК
// получить "тело" из raw[bodySep+4:] (Строка против Список<Байт>) - тем же
// принципом, что и cbackend.rech_http_parse_head. bodySep < 0 - "\r\n\r\n"
// не нашёлся (нет заголовков вовсе - в норме недостижимо для настоящего
// HTTP-запроса, но recvHTTPRequest не гарантирует это сама).
func parseHTTPHead(raw []byte) (method, path, headers string, bodySep int) {
	lineEnd := bytes.Index(raw, []byte("\r\n"))
	sp1 := bytes.IndexByte(raw, ' ')
	if sp1 >= 0 && lineEnd >= 0 && sp1 < lineEnd {
		method = string(raw[:sp1])
		if sp2 := bytes.IndexByte(raw[sp1+1:], ' '); sp2 >= 0 && sp1+1+sp2 < lineEnd {
			path = decodeHTTPPath(string(raw[sp1+1 : sp1+1+sp2]))
		}
	}
	hdrStart := 0
	if lineEnd >= 0 {
		hdrStart = lineEnd + 2
	}
	bodySep = -1
	if sep := bytes.Index(raw, []byte("\r\n\r\n")); sep >= 0 && sep >= hdrStart {
		headers = string(raw[hdrStart:sep])
		bodySep = sep
	}
	return method, path, headers, bodySep
}

// acceptHTTPRequest - "принять_http" - разбирает сырые байты запроса
// (recvHTTPRequest) в "Запрос" (метод/путь/заголовки/тело) - "адрес"/"порт"
// (поля клиентского назначения) остаются пустыми, тем же принципом, что и
// cbackend.rech_http_accept_request.
func (in *Interp) acceptHTTPRequest(conn net.Conn) Value {
	raw := recvHTTPRequest(conn, in.httpMaxRequestBytes)
	method, path, headers, bodySep := parseHTTPHead(raw)
	body := ""
	if bodySep >= 0 {
		body = string(raw[bodySep+4:])
	}

	return Value{Type: ir.EntityType("Запрос"), Obj: &entityObj{typeName: "Запрос", fields: map[string]Value{
		"метод": stringValue(method), "адрес": stringValue(""), "порт": intValue(0),
		"путь": stringValue(path), "заголовки": stringValue(headers), "тело": stringValue(body),
	}}}
}

// acceptHTTPRequestBytes - "принять_http_байты" (internal/semantic/
// httpbytes.go) - как acceptHTTPRequest, но "тело" - "Список<Байт>", не
// "Строка" (бинарно-безопасно) - тем же принципом, что и
// cbackend.rech_http_accept_request_bytes.
func (in *Interp) acceptHTTPRequestBytes(conn net.Conn) Value {
	raw := recvHTTPRequest(conn, in.httpMaxRequestBytes)
	method, path, headers, bodySep := parseHTTPHead(raw)
	body := []byte(nil)
	if bodySep >= 0 {
		body = raw[bodySep+4:]
	}

	return Value{Type: ir.EntityType(httpBytesRequestType), Obj: &entityObj{typeName: httpBytesRequestType, fields: map[string]Value{
		"метод": stringValue(method), "адрес": stringValue(""), "порт": intValue(0),
		"путь": stringValue(path), "заголовки": stringValue(headers), "тело": bytesValue(body),
	}}}
}

// httpResponseHead - статус-строка + заголовки + разделительная пустая
// строка, общая часть sendHTTPResponse/sendHTTPResponseBytes ниже - тот же
// текст, что и у cbackend.rech_http_send_response (внутри rech_http_send_
// response, httpserver.go), включая исправления от 2026-09-20:
//   - фраза-причина - настоящая (rtmsg.HTTPReasonPhrase), не буквальная
//     "OK" для любого кода, как было раньше (реальные HTTP-клиенты
//     ориентируются на числовой код, фраза - для человека/лога, но раз
//     она уже известна - нет причины врать).
//   - "Connection"/"Content-Length" добавляются ТОЛЬКО если Rech-программа
//     сама их не задала в "заголовки" ответа - раньше добавлялись
//     безусловно поверх пользовательских, давая ДВА одинаковых заголовка
//     при коллизии (неопределённое поведение клиента) - в частности, это
//     единственный способ РЕАЛЬНО известить клиента о закрытии соединения
//     (свой "Connection" со значением close) - "Connection: keep-alive"
//     по умолчанию лишь заявляет готовность, решение - за вызывающим
//     Rech-кодом (после каждого "отправить_http_ответ" явно проверяет
//     заголовок "Connection" ВХОДЯЩЕГО запроса через "значение_заголовка" -
//     см. examples/проекты/http_сервер, "обработать_соединение").
func httpResponseHead(code int64, headers string, bodyLen int) string {
	hdrSep := ""
	if headers != "" {
		hdrSep = "\r\n"
	}
	connLine := "Connection: keep-alive\r\n"
	if headerValue(headers, "Connection").Nullable != nil {
		connLine = ""
	}
	lenLine := fmt.Sprintf("Content-Length: %d\r\n", bodyLen)
	if headerValue(headers, "Content-Length").Nullable != nil {
		lenLine = ""
	}
	reason := rtmsg.HTTPReasonPhrase(code)
	return fmt.Sprintf("HTTP/1.1 %d %s\r\n%s%s%s%s\r\n", code, reason, headers, hdrSep, connLine, lenLine)
}

// sendHTTPResponse - "отправить_http_ответ" - сериализует "Ответ" в сырой
// HTTP-ответ и отправляет на conn.
func (in *Interp) sendHTTPResponse(conn net.Conn, resp Value) {
	f := resp.Obj.fields
	code, headers, body := f["код"].I, f["заголовки"].S, f["тело"].S
	head := httpResponseHead(code, headers, len(body))
	if _, err := io.WriteString(conn, head+body); err != nil {
		panicRuntime("%s", rtmsg.SocketSendFailed)
	}
}

// sendHTTPResponseBytes - "отправить_http_ответ_байты" (internal/semantic/
// httpbytes.go) - как sendHTTPResponse, но "тело" - "Список<Байт>", не
// "Строка" (бинарно-безопасно, тело шлётся через conn.Write([]byte), не
// строковую конкатенацию) - тем же принципом, что и cbackend.rech_http_
// send_response_bytes.
func (in *Interp) sendHTTPResponseBytes(conn net.Conn, resp Value) {
	f := resp.Obj.fields
	code, headers, body := f["код"].I, f["заголовки"].S, bytesFromValue(f["тело"])
	head := httpResponseHead(code, headers, len(body))
	if _, err := io.WriteString(conn, head); err != nil {
		panicRuntime("%s", rtmsg.SocketSendFailed)
	}
	if _, err := conn.Write(body); err != nil {
		panicRuntime("%s", rtmsg.SocketSendFailed)
	}
}
