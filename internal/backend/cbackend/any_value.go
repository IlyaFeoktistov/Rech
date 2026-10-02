package cbackend

import (
	"fmt"
	"strings"

	"github.com/IlyaFeoktistov/Rech/internal/ir"
)

// Текст и сравнение "Значение" (rech_any_t): "${з}"/"вывести з" печатают
// содержимое так же, как печаталось бы само значение, "==" - то же
// сравнение, что у содержимого (числа/строки - по значению, структура - по
// полям, сущность - по ссылке), разные типы - не равны. Ветвление - по
// дескриптору типа: у каждого типа он ровно один. Те же правила - у
// interp (printText/valuesEqual, ветка KindAny).

// anyValueProtos - прототипы rech_any_to_str/rech_any_eq: "равен" коллекций
// "Значение" генерируется раньше их тел.
func anyValueProtos(ctx *irGenCtx) string {
	var sb strings.Builder
	if ctx.usesAnyText {
		sb.WriteString("static const char *rech_any_to_str(rech_any_t v);\n")
	}
	if ctx.usesAnyEq {
		sb.WriteString("static bool rech_any_eq(rech_any_t a, rech_any_t b);\n")
	}
	return sb.String()
}

// anyValueHelpers - C rech_any_to_str/rech_any_eq; "" - не используются.
func anyValueHelpers(ctx *irGenCtx) string {
	if !ctx.usesAnyText && !ctx.usesAnyEq {
		return ""
	}
	var sb strings.Builder
	meta := func(k ir.TypeKind) string { return "&" + reflectMetaName(ir.Type{Kind: k}.String()) }
	if ctx.usesAnyText {
		sb.WriteString("static const char *rech_any_to_str(rech_any_t v) {\n")
		for _, k := range reflectPrimitiveKinds {
			fmt.Fprintf(&sb, "    if (v.type == %s) return %s;\n", meta(k), anyTextOf(k))
		}
		sb.WriteString(`    size_t n = strlen(v.type->name) + 32;
    char *out = GC_MALLOC_ATOMIC(n);
    snprintf(out, n, v.type->is_value ? "[структура]<%s>" : "[сущность]<%s>", v.type->name);
    return out;
}

`)
	}
	if ctx.usesAnyEq {
		sb.WriteString("static bool rech_any_eq(rech_any_t a, rech_any_t b) {\n    if (a.type != b.type) return false;\n")
		fmt.Fprintf(&sb, "    if (a.type == %s) return true;\n", meta(ir.KindNull))
		fmt.Fprintf(&sb, "    if (a.type == %s) return strcmp(a.v.s, b.v.s) == 0;\n", meta(ir.KindString))
		fmt.Fprintf(&sb, "    if (a.type == %s) return a.v.f64 == b.v.f64;\n", meta(ir.KindFloat))
		fmt.Fprintf(&sb, "    if (a.type == %s) return a.v.f32 == b.v.f32;\n", meta(ir.KindFloat32))
		fmt.Fprintf(&sb, "    if (a.type == %s) return a.v.b == b.v.b;\n", meta(ir.KindBool))
		sb.WriteString(`    if (a.type->is_value) {
        for (int i = 0; i < a.type->field_count; i++) {
            const rech_field_meta_t *f = &a.type->fields[i];
            if (!rech_any_eq(f->get(a.v.ptr), f->get(b.v.ptr))) return false;
        }
        return true;
    }
    return a.v.i == b.v.i;
}

`)
	}
	return sb.String()
}

// anyTextOf - C-текст значения v примитивного вида k внутри rech_any_t.
func anyTextOf(k ir.TypeKind) string {
	switch k {
	case ir.KindNull:
		return `"Ничто"`
	case ir.KindString:
		return "v.v.s"
	case ir.KindBool:
		return `(v.v.b ? "истина" : "ложь")`
	case ir.KindFloat:
		return "rech_cast_float_to_str(v.v.f64)"
	case ir.KindFloat32:
		return "rech_cast_float_to_str((double)v.v.f32)"
	case ir.KindChar:
		return "rech_cast_char_to_str((int32_t)v.v.i)"
	case ir.KindUint, ir.KindUint16, ir.KindUint32:
		return "rech_cast_uint_to_str((unsigned long long)v.v.i)"
	case ir.KindFile:
		return `"[файл]"`
	case ir.KindSocket:
		return `"[сокет]"`
	case ir.KindDB:
		return `"[бд]"`
	case ir.KindHTTP2Session:
		return `"[http2-сессия]"`
	}
	return "rech_cast_int_to_str(v.v.i)"
}
