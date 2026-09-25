// Разбор multipart/form-data - см. package doc internal/semantic/
// multipart.go для полного описания решения (курируемый минимум, не
// полный RFC-парсер).
package interp

import (
	"bytes"
	"strings"

	"github.com/IlyaFeoktistov/Rech/internal/ir"
	"github.com/IlyaFeoktistov/Rech/internal/rtmsg"
)

// formPartType/multipartResultType - см. internal/semantic/multipart.go,
// package doc queryParamsDictType (cbackend/httpserver.go) для того же
// принципа дублирования имени константой в нескольких пакетах.
const (
	formPartType        = "ЧастьФормы"
	multipartResultType = "Список<ЧастьФормы>"
)

// extractBoundary ищет "Content-Type" в headers (текст заголовков
// Запрос/ЗапросБайты) регистронезависимо и возвращает значение параметра
// "boundary=" из него (кавычки, если были, сняты) - ok=false, если
// Content-Type не найден вовсе или не несёт "boundary=".
func extractBoundary(headers string) (boundary string, ok bool) {
	lower := strings.ToLower(headers)
	idx := strings.Index(lower, "content-type:")
	if idx < 0 {
		return "", false
	}
	rest := headers[idx+len("content-type:"):]
	if nl := strings.IndexAny(rest, "\r\n"); nl >= 0 {
		rest = rest[:nl]
	}
	lowerRest := strings.ToLower(rest)
	bidx := strings.Index(lowerRest, "boundary=")
	if bidx < 0 {
		return "", false
	}
	val := strings.TrimSpace(rest[bidx+len("boundary="):])
	if semi := strings.IndexByte(val, ';'); semi >= 0 {
		val = strings.TrimSpace(val[:semi])
	}
	val = strings.Trim(val, `"`)
	if val == "" {
		return "", false
	}
	return val, true
}

// extractDispositionParam ищет параметр name="значение" (или
// filename="значение") в блоке заголовков ОДНОЙ части формы
// (Content-Disposition: form-data; name="фото"; filename="кот.jpg") -
// регистронезависимо по имени параметра, значение - как есть между
// кавычками (без раскодирования percent-encoding - реальные браузеры не
// кодируют имя/имя_файла в form-data, в отличие от query-строки).
func extractDispositionParam(partHeaders, param string) string {
	lower := strings.ToLower(partHeaders)
	want := param + `="`
	idx := strings.Index(lower, want)
	if idx < 0 {
		return ""
	}
	rest := partHeaders[idx+len(want):]
	end := strings.IndexByte(rest, '"')
	if end < 0 {
		return ""
	}
	return rest[:end]
}

// extractPartContentType ищет "Content-Type" в блоке заголовков ОДНОЙ
// части формы - "", если не указан (обычное текстовое поле формы).
func extractPartContentType(partHeaders string) string {
	lower := strings.ToLower(partHeaders)
	idx := strings.Index(lower, "content-type:")
	if idx < 0 {
		return ""
	}
	rest := partHeaders[idx+len("content-type:"):]
	if nl := strings.IndexAny(rest, "\r\n"); nl >= 0 {
		rest = rest[:nl]
	}
	return strings.TrimSpace(rest)
}

// parseMultipart - "разобрать_multipart(тело, заголовки): Список<ЧастьФормы>".
// Формат (RFC 2046): каждая часть отделена строкой "--<boundary>\r\n",
// поток заканчивается "--<boundary>--" - между частями и телом всего
// запроса СТРОГОГО совпадения с RFC не проверяется (курируемый минимум,
// см. package doc), только достаточно для реальных браузеров/curl.
func parseMultipart(body []byte, headers string) Value {
	boundary, ok := extractBoundary(headers)
	if !ok {
		throwRuntimeError(rtmsg.ErrorParse, "%s", rtmsg.MultipartMissingBoundary)
	}
	delim := []byte("--" + boundary)
	var parts []Value

	rest := body
	idx := bytes.Index(rest, delim)
	if idx < 0 {
		return Value{Type: ir.EntityType(multipartResultType), Obj: &entityObj{typeName: multipartResultType, coll: &collection{template: "Список", elems: parts}}}
	}
	rest = rest[idx+len(delim):]

	for {
		if bytes.HasPrefix(rest, []byte("--")) {
			break
		}
		rest = bytes.TrimPrefix(rest, []byte("\r\n"))

		hdrEnd := bytes.Index(rest, []byte("\r\n\r\n"))
		if hdrEnd < 0 {
			break
		}
		partHeaders := string(rest[:hdrEnd])
		content := rest[hdrEnd+4:]

		nextIdx := bytes.Index(content, delim)
		var data []byte
		if nextIdx < 0 {
			data = bytes.TrimSuffix(content, []byte("\r\n"))
			rest = nil
		} else {
			data = bytes.TrimSuffix(content[:nextIdx], []byte("\r\n"))
			rest = content[nextIdx+len(delim):]
		}

		parts = append(parts, Value{Type: ir.EntityType(formPartType), Obj: &entityObj{typeName: formPartType, fields: map[string]Value{
			"имя":             stringValue(extractDispositionParam(partHeaders, "name")),
			"имя_файла":       stringValue(extractDispositionParam(partHeaders, "filename")),
			"тип_содержимого": stringValue(extractPartContentType(partHeaders)),
			"данные":          bytesValue(append([]byte(nil), data...)),
		}}})

		if rest == nil {
			break
		}
	}

	return Value{Type: ir.EntityType(multipartResultType), Obj: &entityObj{typeName: multipartResultType, coll: &collection{template: "Список", elems: parts}}}
}
