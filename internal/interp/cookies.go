// Cookies поверх уже существующего HTTP (Шаг 9) - см. package doc
// internal/backend/cbackend/cookies.go: то же самое поведение (точное,
// с учётом регистра совпадение имени заголовка, значение без URL-кодирования),
// реализовано через strings.* вместо ручного разбора C-строки.
package interp

import (
	"fmt"
	"strconv"
	"strings"

	"github.com/IlyaFeoktistov/Rech/internal/ir"
)

// cookieValue - "значение_куки(запрос, имя)": разбирает заголовок
// "Cookie: a=1; b=2" (если он есть) и ищет пару с именем name.
func cookieValue(req Value, name string) Value {
	headers := req.Obj.getField("заголовки").S
	nullableType := ir.NullableType(ir.TypeString)
	for line := range strings.SplitSeq(headers, "\r\n") {
		rest, ok := strings.CutPrefix(line, "Cookie:")
		if !ok {
			continue
		}
		for pair := range strings.SplitSeq(strings.TrimSpace(rest), ";") {
			pair = strings.TrimSpace(pair)
			key, value, ok := strings.Cut(pair, "=")
			if ok && key == name {
				v := stringValue(value)
				return Value{Type: nullableType, Nullable: &v}
			}
		}
	}
	return Value{Type: nullableType}
}

// cookieString - "строка_куки(имя, значение, опции)": строит одну строку
// "Set-Cookie: ..." (без завершающего "\r\n" - тем же принципом, что и
// "заголовки" вообще).
func cookieString(name, value string, opts Value) string {
	var sb strings.Builder
	fmt.Fprintf(&sb, "Set-Cookie: %s=%s", name, value)
	if p := opts.Obj.getField("путь").S; p != "" {
		fmt.Fprintf(&sb, "; Path=%s", p)
	}
	if d := opts.Obj.getField("домен").S; d != "" {
		fmt.Fprintf(&sb, "; Domain=%s", d)
	}
	if maxAge := opts.Obj.getField("максимальный_возраст"); maxAge.Nullable != nil {
		fmt.Fprintf(&sb, "; Max-Age=%s", strconv.FormatInt(maxAge.Nullable.I, 10))
	}
	if opts.Obj.getField("только_http").B {
		sb.WriteString("; HttpOnly")
	}
	if opts.Obj.getField("безопасный").B {
		sb.WriteString("; Secure")
	}
	if s := opts.Obj.getField("тот_же_сайт").S; s != "" {
		fmt.Fprintf(&sb, "; SameSite=%s", s)
	}
	return sb.String()
}
