package ir

import (
	"strings"
	"unicode/utf8"
)

// callableCanon - начало канонического имени типа "функция как значение"
// (ir.CallableType): "Вызываемый<Параметр, ...>Результат" - ключ типа
// внутри компилятора, не то, что пишет программист.
const callableCanon = CallableTypeName + "<"

// DisplayTypes переписывает в тексте сообщения каждое каноническое имя
// "Вызываемый<A, B>R" в исходную запись "Вызываемый(A, B): R" (без
// результата - "Вызываемый(A, B)"), в т.ч. вложенные ("?Вызываемый<...>",
// "Список<Вызываемый<...>>"). Остальной текст не меняется. Для
// сообщений/подсказок пользователю - сам Type.String() остаётся ключом.
func DisplayTypes(msg string) string {
	if !strings.Contains(msg, callableCanon) {
		return msg
	}
	var b strings.Builder
	for {
		i := strings.Index(msg, callableCanon)
		if i < 0 {
			b.WriteString(msg)
			return b.String()
		}
		b.WriteString(msg[:i])
		text, n := callableDisplay(msg[i:])
		b.WriteString(text)
		msg = msg[i+n:]
	}
}

// callableDisplay - s начинается с callableCanon; возвращает исходную
// запись и длину разобранного канонического имени в s. Несбалансированные
// скобки - текст как есть до конца s.
func callableDisplay(s string) (string, int) {
	pos := len(callableCanon)
	depth := 1
	start := pos
	var params []string
	for pos < len(s) && depth > 0 {
		switch s[pos] {
		case '<':
			depth++
		case '>':
			depth--
		case ',':
			if depth == 1 {
				params = append(params, s[start:pos])
				start = pos + 1
			}
		}
		pos++
	}
	if depth != 0 {
		return s, len(s)
	}
	if last := s[start : pos-1]; strings.TrimSpace(last) != "" || len(params) > 0 {
		params = append(params, last)
	}
	for i, p := range params {
		params[i] = DisplayTypes(strings.TrimSpace(p))
	}
	text := "Вызываемый(" + strings.Join(params, ", ") + ")"
	n := typeTokenLen(s[pos:])
	if n > 0 {
		text += ": " + DisplayTypes(s[pos:pos+n])
	}
	return text, pos + n
}

// typeTokenLen - длина имени типа в начале s: "?" впереди, буквы/цифры/
// "_", затем сбалансированный "<...>" generic-аргументов и, у
// "Вызываемый<...>", его собственный результат.
func typeTokenLen(s string) int {
	pos := 0
	if strings.HasPrefix(s, "?") {
		pos++
	}
	nameStart := pos
	for pos < len(s) {
		r, size := utf8.DecodeRuneInString(s[pos:])
		if !isTypeNameRune(r) {
			break
		}
		pos += size
	}
	if pos == nameStart {
		return 0
	}
	if pos < len(s) && s[pos] == '<' {
		if strings.HasPrefix(s[nameStart:], callableCanon) {
			_, n := callableDisplay(s[nameStart:])
			return nameStart + n
		}
		depth := 0
		for pos < len(s) {
			switch s[pos] {
			case '<':
				depth++
			case '>':
				depth--
			}
			pos++
			if depth == 0 {
				break
			}
		}
	}
	return pos
}

func isTypeNameRune(r rune) bool {
	return r == '_' || (r >= '0' && r <= '9') || (r >= 'a' && r <= 'z') || (r >= 'A' && r <= 'Z') || (r >= 0x400 && r <= 0x4FF)
}
