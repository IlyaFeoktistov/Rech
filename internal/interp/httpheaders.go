package interp

import (
	"strings"

	"github.com/IlyaFeoktistov/Rech/internal/ir"
)

// headerValue - "значение_заголовка(заголовки, имя)": ищет строку вида
// "имя: значение" в заголовках ("\r\n"-разделённых, тем же форматом, что
// "заголовки" у Запрос/Ответ) - совпадение имени ТОЧНОЕ, с учётом регистра
// (тот же принцип, что и у cookieValue/"значение_куки" - реальные HTTP-
// клиенты/серверы присылают имена заголовков в известном написании,
// case-insensitive сравнение здесь намеренно не реализовано).
func headerValue(headers, name string) Value {
	nullableType := ir.NullableType(ir.TypeString)
	prefix := name + ":"
	for line := range strings.SplitSeq(headers, "\r\n") {
		rest, ok := strings.CutPrefix(line, prefix)
		if !ok {
			continue
		}
		v := stringValue(strings.TrimSpace(rest))
		return Value{Type: nullableType, Nullable: &v}
	}
	return Value{Type: nullableType}
}
