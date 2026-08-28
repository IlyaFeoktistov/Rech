package cbackend

import (
	"fmt"
	"strconv"
	"strings"
)

func indent(depth int) string {
	return strings.Repeat("    ", depth)
}

// printCall - "вывести <выражение>": один printf на примитив, дружелюбный
// плейсхолдер (без обращения к самому значению) на сущность/файл/сокет -
// вместо ошибки компиляции или неопределённого поведения (printf("%s", ...)
// на структуре) значение просто помечается своим видом, подробности - через
// "показать" (см. generateIRShowExpr).
func printCall(ctx *irGenCtx, typ valueType, code string) string {
	switch typ {
	case typeString:
		return fmt.Sprintf(`printf("%%s\n", %s);`, code)
	case typeInt, typeByte, typeInt8, typeInt16, typeInt32, typeInt64:
		return fmt.Sprintf(`printf("%%lld\n", (long long)(%s));`, code)
	case typeUint, typeUint8, typeUint16, typeUint32, typeUint64:
		// %llu, не %lld - Натуральное64 может нести значение выше
		// INT64_MAX, которое (long long) отобразил бы как отрицательное.
		return fmt.Sprintf(`printf("%%llu\n", (unsigned long long)(%s));`, code)
	case typeFloat, typeFloat32:
		return fmt.Sprintf(`printf("%%g\n", (double)(%s));`, code)
	case typeBool:
		return fmt.Sprintf(`printf("%%s\n", (%s) ? "истина" : "ложь");`, code)
	case typeChar:
		// Символ печатается как сам символ (UTF-8), не как числовой код -
		// та же кодировка кодовой точки в байты, что и у явного каста
		// Символ -> Строка (rech_cast_char_to_str, castHelpers).
		ctx.usesCast = true
		return fmt.Sprintf(`printf("%%s\n", rech_cast_char_to_str(%s));`, code)
	case typeFile:
		return `printf("[файл]\n");`
	case typeSocket:
		return `printf("[сокет]\n");`
	case typeDB:
		return `printf("[бд]\n");`
	default:
		if typ.kind == kindEntity {
			kind := "сущность"
			if !ctx.entityIsRef(typ.entity) {
				kind = "структура"
			}
			return fmt.Sprintf(`printf("[%s]<%s>\n");`, kind, escapeCString(typ.entity))
		}
		return fmt.Sprintf(`printf("%%s\n", %s);`, code)
	}
}

func formatFloatLiteral(v float64) string {
	s := strconv.FormatFloat(v, 'g', -1, 64)
	if !strings.ContainsAny(s, ".eE") {
		s += ".0"
	}
	return s
}

func escapeCString(s string) string {
	var sb strings.Builder
	for _, r := range s {
		switch r {
		case '"':
			sb.WriteString(`\"`)
		case '\\':
			sb.WriteString(`\\`)
		case '\n':
			sb.WriteString(`\n`)
		case '\t':
			sb.WriteString(`\t`)
		default:
			sb.WriteRune(r)
		}
	}
	return sb.String()
}
