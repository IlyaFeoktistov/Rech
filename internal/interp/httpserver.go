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
	pairs := map[Value]Value{}
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
			pairs[stringValue(key)] = stringValue(urlDecode(value))
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
// заголовка Content-Length нет).
func recvHTTPRequest(conn net.Conn) []byte {
	var buf []byte
	chunk := make([]byte, 4096)
	bodyStart := -1
	var contentLength int64
	for {
		if bodyStart < 0 {
			if idx := bytes.Index(buf, []byte("\r\n\r\n")); idx >= 0 {
				bodyStart = idx + 4
				contentLength = httpContentLength(buf[:idx])
			}
		}
		if bodyStart >= 0 && int64(len(buf)-bodyStart) >= contentLength {
			break
		}
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

// acceptHTTPRequest - "принять_http" - разбирает сырые байты запроса
// (recvHTTPRequest) в "Запрос" (метод/путь/заголовки/тело) - "адрес"/"порт"
// (поля клиентского назначения) остаются пустыми, тем же принципом, что и
// cbackend.rech_http_accept_request.
func (in *Interp) acceptHTTPRequest(conn net.Conn) Value {
	raw := recvHTTPRequest(conn)
	method, path, headers, body := "", "", "", ""

	lineEnd := bytes.Index(raw, []byte("\r\n"))
	sp1 := bytes.IndexByte(raw, ' ')
	if sp1 >= 0 && lineEnd >= 0 && sp1 < lineEnd {
		method = string(raw[:sp1])
		if sp2 := bytes.IndexByte(raw[sp1+1:], ' '); sp2 >= 0 && sp1+1+sp2 < lineEnd {
			path = string(raw[sp1+1 : sp1+1+sp2])
		}
	}
	hdrStart := 0
	if lineEnd >= 0 {
		hdrStart = lineEnd + 2
	}
	if bodySep := bytes.Index(raw, []byte("\r\n\r\n")); bodySep >= 0 && bodySep >= hdrStart {
		headers = string(raw[hdrStart:bodySep])
		body = string(raw[bodySep+4:])
	}

	return Value{Type: ir.EntityType("Запрос"), Obj: &entityObj{typeName: "Запрос", fields: map[string]Value{
		"метод": stringValue(method), "адрес": stringValue(""), "порт": intValue(0),
		"путь": stringValue(path), "заголовки": stringValue(headers), "тело": stringValue(body),
	}}}
}

// sendHTTPResponse - "отправить_http_ответ" - сериализует "Ответ" в сырой
// HTTP-ответ и отправляет на conn, тем же принципом, что и
// cbackend.rech_http_send_response (единая фраза-причина "OK" для любого
// кода - реальные HTTP-клиенты ориентируются на числовой код, не на неё).
func (in *Interp) sendHTTPResponse(conn net.Conn, resp Value) {
	f := resp.Obj.fields
	code, headers, body := f["код"].I, f["заголовки"].S, f["тело"].S
	hdrSep := ""
	if headers != "" {
		hdrSep = "\r\n"
	}
	head := fmt.Sprintf("HTTP/1.1 %d OK\r\n%s%sConnection: close\r\nContent-Length: %d\r\n\r\n", code, headers, hdrSep, len(body))
	if _, err := io.WriteString(conn, head+body); err != nil {
		panicRuntime("%s", rtmsg.SocketSendFailed)
	}
}
