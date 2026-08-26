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
func printCall(typ valueType, code string) string {
	switch typ {
	case typeString:
		return fmt.Sprintf(`printf("%%s\n", %s);`, code)
	case typeInt:
		return fmt.Sprintf(`printf("%%lld\n", %s);`, code)
	case typeFloat:
		return fmt.Sprintf(`printf("%%g\n", %s);`, code)
	case typeBool:
		return fmt.Sprintf(`printf("%%s\n", (%s) ? "истина" : "ложь");`, code)
	case typeFile:
		return `printf("[файл]\n");`
	case typeSocket:
		return `printf("[сокет]\n");`
	default:
		if typ.kind == kindEntity {
			return fmt.Sprintf(`printf("[сущность]<%s>\n");`, escapeCString(typ.entity))
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
