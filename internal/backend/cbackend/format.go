package cbackend

import (
	"fmt"
	"math"
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
// "показать" (см. generateIRShowExpr). Без "\n" в конце формата - перенос
// строки после значения не добавляется, управляет им сам Rech-код через
// "\n" в строке.
func printCall(ctx *irGenCtx, typ valueType, code string) string {
	switch typ {
	case typeString:
		return fmt.Sprintf(`printf("%%s", %s);`, code)
	case typeInt, typeByte, typeInt8, typeInt16, typeInt32:
		return fmt.Sprintf(`printf("%%lld", (long long)(%s));`, code)
	case typeUint, typeUint16, typeUint32:
		// %llu, не %lld - Натуральное64 может нести значение выше
		// INT64_MAX, которое (long long) отобразил бы как отрицательное.
		return fmt.Sprintf(`printf("%%llu", (unsigned long long)(%s));`, code)
	case typeFloat, typeFloat32:
		// NaN - "nan" без знака: знак NaN из вычисления (0.0/0.0 даёт
		// "-nan" у glibc на x86) не часть значения для языка, и interp
		// печатает так же (formatFloatForPrint).
		return fmt.Sprintf(`{ double rech_pf = (double)(%s); printf(rech_pf != rech_pf ? "nan" : "%%g", rech_pf); }`, code)
	case typeBool:
		return fmt.Sprintf(`printf("%%s", (%s) ? "истина" : "ложь");`, code)
	case typeChar:
		// Символ печатается как сам символ (UTF-8), не как числовой код -
		// та же кодировка кодовой точки в байты, что и у явного каста
		// Символ -> Строка (rech_cast_char_to_str, castHelpers).
		ctx.usesCast = true
		return fmt.Sprintf(`printf("%%s", rech_cast_char_to_str(%s));`, code)
	default:
		if text, ok := printedLabel(ctx, typ); ok {
			return fmt.Sprintf(`printf("%s");`, text)
		}
		if typ.kind == kindPointer {
			// Адрес, не разыменованное значение - "%p" (не "%s": code -
			// указатель на T, не на char, printf("%s", ...) на нём было бы
			// неопределённым поведением). (void *) - C требует именно этот
			// тип аргумента для "%p", каким бы ни было T.
			return fmt.Sprintf(`printf("%%p", (void *)(%s));`, code)
		}
		return fmt.Sprintf(`printf("%%s", %s);`, code)
	}
}

// printedLabel - плейсхолдер, которым "вывести" печатает значение вида
// typ без собственного текстового представления (файл, сокет, БД,
// HTTP/2-сессия, сущность/структура) - уже экранированный для C-строки.
func printedLabel(ctx *irGenCtx, typ valueType) (string, bool) {
	switch typ {
	case typeFile:
		return "[файл]", true
	case typeSocket:
		return "[сокет]", true
	case typeDB:
		return "[бд]", true
	case typeHTTP2Session:
		return "[http2-сессия]", true
	}
	if typ.kind == kindEntity {
		kind := "сущность"
		if !ctx.entityIsRef(typ.entity) {
			kind = "структура"
		}
		return fmt.Sprintf("[%s]<%s>", kind, escapeCString(typ.entity)), true
	}
	return "", false
}

// printedTextCode - C-выражение "const char *" с тем же текстом, что
// печатает printCall для значения code вида typ (вставка "${...}",
// ir.CastExpr.AsPrinted): плейсхолдер - после вычисления code ради его
// побочных эффектов, указатель - адрес "%p".
func printedTextCode(ctx *irGenCtx, typ valueType, code string) string {
	if text, ok := printedLabel(ctx, typ); ok {
		return fmt.Sprintf(`((void)(%s), "%s")`, code, text)
	}
	ctx.usesCast = true
	return fmt.Sprintf("rech_cast_ptr_to_str((void *)(%s))", code)
}

func formatFloatLiteral(v float64) string {
	switch {
	case math.IsInf(v, 1):
		return "__builtin_inf()"
	case math.IsInf(v, -1):
		return "(-__builtin_inf())"
	case math.IsNaN(v):
		return "__builtin_nan(\"\")"
	}
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
		case '\r':
			sb.WriteString(`\r`)
		default:
			sb.WriteRune(r)
		}
	}
	return sb.String()
}
