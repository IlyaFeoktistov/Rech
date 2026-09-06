// Chunked transfer encoding (RFC 7230 §4.1) - см. package doc
// internal/backend/cbackend/chunked.go для полного описания решения -
// здесь тот же алгоритм на чистом Go, для HTTP-СЕРВЕРА (recvHTTPRequest,
// httpserver.go) и HTTP-КЛИЕНТА (httpRequest, builtins.go) одинаково -
// docs/ROADMAP.md, HTTP highload пункт 4.
package interp

import (
	"strconv"
)

// isChunkedHeaders ищет "transfer-encoding" в headers (байты заголовков,
// без завершающего "\r\n\r\n") регистронезависимо вручную - тот же приём,
// что и httpContentLength выше в этом пакете (единообразие с
// cbackend.rech_http_is_chunked, а не strings.ToLower - оба backend'а
// обязаны разбирать одни и те же байты одинаково) - и проверяет, есть ли
// "chunked" где-то в значении этого заголовка (до конца строки). Курируемый
// минимум, не полный RFC-парсер списка кодировок - см. её package doc в
// cbackend/chunked.go.
func isChunkedHeaders(headers []byte) bool {
	const want = "transfer-encoding"
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
		lineEnd := k
		for lineEnd < len(rest) && !(rest[lineEnd] == '\r' && lineEnd+1 < len(rest) && rest[lineEnd+1] == '\n') {
			lineEnd++
		}
		value := rest[k:lineEnd]
		const chunkedWord = "chunked"
		for m := 0; m+len(chunkedWord) <= len(value); m++ {
			found := true
			for j := range len(chunkedWord) {
				c := value[m+j]
				if c >= 'A' && c <= 'Z' {
					c += 32
				}
				if c != chunkedWord[j] {
					found = false
					break
				}
			}
			if found {
				return true
			}
		}
		return false
	}
	return false
}

// dechunk разбирает body (уже НАВЕРНЯКА содержит весь чанкованный поток
// целиком, включая терминирующий чанк нулевого размера - HTTP-клиент уже
// дочитал до закрытия соединения сервером) за один проход, останавливаясь
// на первом некорректном/незавершённом чанке (тот же best-effort принцип,
// что и у процент-декодирования query-строки) - трейлеры игнорируются.
func dechunk(body []byte) []byte {
	out := make([]byte, 0, len(body))
	p := body
	for {
		size, rest, ok := parseChunkSize(p)
		if !ok {
			break
		}
		p = rest
		if size == 0 {
			break
		}
		if int64(len(p)) < size {
			break
		}
		out = append(out, p[:size]...)
		p = p[size:]
		if len(p) >= 2 && p[0] == '\r' && p[1] == '\n' {
			p = p[2:]
		}
	}
	return out
}

// tryDechunk - ИНКРЕМЕНТАЛЬНЫЙ вариант dechunk для HTTP-СЕРВЕРА (в отличие
// от клиента, сервер не может ждать закрытия соединения - клиент держит
// его открытым в ожидании ответа): body - сколько байт реально принято к
// этому моменту, может быть неполным потоком. complete=true, только когда
// разобран терминирующий чанк (включая завершающий "\r\n" трейлеров) -
// иначе false (нужно читать ещё), разбирает заново с начала каждый раз -
// см. package doc cbackend.rech_http_try_dechunk про этот же компромисс.
func tryDechunk(body []byte) (decoded []byte, complete bool) {
	out := make([]byte, 0, len(body))
	p := body
	for {
		size, rest, ok := parseChunkSize(p)
		if !ok {
			return nil, false
		}
		p = rest
		if size == 0 {
			// Терминирующий чанк - трейлеры (обычно нет ни одного) до
			// финального "\r\n" уже должны были прийти целиком.
			if len(p) < 2 || p[0] != '\r' || p[1] != '\n' {
				return nil, false
			}
			return out, true
		}
		if int64(len(p)) < size+2 {
			return nil, false
		}
		out = append(out, p[:size]...)
		p = p[size:]
		if p[0] == '\r' && p[1] == '\n' {
			p = p[2:]
		}
	}
}

// parseChunkSize разбирает шестнадцатеричный размер одного чанка в начале
// p (до "\r\n", chunk-extension после ";" не поддержан - курируемый
// минимум, тем же принципом, что и у cbackend) - ok=false, если p не
// начинается с валидного hex-числа или "\r\n" после него ещё не пришёл
// целиком (неполные данные, не ошибка).
func parseChunkSize(p []byte) (size int64, rest []byte, ok bool) {
	end := 0
	for end < len(p) && isHexDigit(p[end]) {
		end++
	}
	if end == 0 {
		return 0, nil, false
	}
	n, err := strconv.ParseInt(string(p[:end]), 16, 64)
	if err != nil || n < 0 {
		return 0, nil, false
	}
	lineEnd := -1
	for i := end; i+1 < len(p); i++ {
		if p[i] == '\r' && p[i+1] == '\n' {
			lineEnd = i
			break
		}
	}
	if lineEnd < 0 {
		return 0, nil, false
	}
	return n, p[lineEnd+2:], true
}

func isHexDigit(c byte) bool {
	return (c >= '0' && c <= '9') || (c >= 'a' && c <= 'f') || (c >= 'A' && c <= 'F')
}
