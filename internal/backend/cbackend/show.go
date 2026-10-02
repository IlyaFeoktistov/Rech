package cbackend

import (
	"fmt"
	"strings"

	"github.com/IlyaFeoktistov/Rech/internal/ir"
)

// showFuncName - C-имя функции отладочного вывода ("показать") для типа
// name (обычная сущность или generic-инстанциация вроде "Список<Целое>") -
// тот же transliterate, что и у mangleType; отдельный префикс (rech_show_,
// не rech_T_/rech_M_) исключает коллизию с typedef самого типа и с
// C-функциями методов.
func showFuncName(name string) string {
	return "rech_show_" + transliterate(name)
}

// generateIRShowExpr - C-выражение (без завершающего ";"), печатающее
// значение типа t, хранящееся в cExpr, вместе с именем типа - рекурсивно
// для сущностей/generic-коллекций (вызов уже сгенерированной функции показа
// этого типа), одним printf для примитивов. ctx.usesCast взводится для
// Символ - расшифровка кодовой точки в UTF-8 (rech_cast_char_to_str)
// переиспользуется из castHelpers, тот же принцип, что и у printCall.
func generateIRShowExpr(ctx *irGenCtx, t ir.Type, cExpr string) string {
	switch t.Kind {
	case ir.KindString:
		return fmt.Sprintf(`printf("\"%%s\" (Строка)", %s)`, cExpr)
	case ir.KindByte:
		return fmt.Sprintf(`printf("%%lld (Байт)", (long long)(%s))`, cExpr)
	case ir.KindInt:
		return fmt.Sprintf(`printf("%%lld (Целое)", %s)`, cExpr)
	case ir.KindFloat32:
		return fmt.Sprintf(`{ char rech_fb[48]; printf("%%s (Дробное)", rech_fmt_float(rech_fb, (double)(%s), 1)); }`, cExpr)
	case ir.KindFloat:
		return fmt.Sprintf(`{ char rech_fb[48]; printf("%%s (Действительное)", rech_fmt_float(rech_fb, %s, 0)); }`, cExpr)
	case ir.KindDecimal:
		ctx.usesDecimal = true
		return fmt.Sprintf(`{ char rech_db[48]; printf("%%s (Десятичное)", rech_dec_to_text(rech_db, %s)); }`, cExpr)
	case ir.KindChar:
		ctx.usesCast = true
		return fmt.Sprintf(`printf("%%s (Символ)", rech_cast_char_to_str(%s))`, cExpr)
	case ir.KindBool:
		return fmt.Sprintf(`printf("%%s (Логическое)", (%s) ? "истина" : "ложь")`, cExpr)
	case ir.KindEntity:
		return fmt.Sprintf("%s(%s)", showFuncName(t.Entity), cExpr)
	case ir.KindNullable:
		// Возвращает if/else БЛОК, не одно выражение - единственный случай
		// здесь: вызывающая сторона (generateIREntityShowFunc и т. п.)
		// всегда дописывает результат как отдельную инструкцию ("%s;\n"),
		// лишняя ";" после "}" в C - пустая инструкция, синтаксически
		// безобидна.
		inner := ctx.nullableInner[t.Entity]
		return fmt.Sprintf(`if ((%[1]s).has) { %[2]s; } else { printf("Ничто (%[3]s)"); }`,
			cExpr, generateIRShowExpr(ctx, inner, "("+cExpr+").value"), escapeCString(t.String()))
	case ir.KindArray:
		// Блок, как и у KindNullable: массив вычисляется один раз во
		// временную переменную, элементы - по порядку.
		ctx.chainTmpCounter++
		tmp := fmt.Sprintf("rech_show%d", ctx.chainTmpCounter)
		a := ctx.arrays[t.Entity]
		return fmt.Sprintf(`{ %[1]s %[2]s = %[3]s; printf("%[4]s ["); for (long long %[2]s_i = 0; %[2]s_i < %[5]d; %[2]s_i++) { if (%[2]s_i > 0) printf(", "); %[6]s; } printf("]"); }`,
			mangleType(t.Entity), tmp, cExpr, escapeCString(t.String()), a.Len, generateIRShowExpr(ctx, a.Elem, tmp+".a["+tmp+"_i]"))
	case ir.KindAny:
		// Содержимое и его тип (interp showValue - тот же текст).
		ctx.usesAnyText, ctx.usesCast = true, true
		return fmt.Sprintf(`{ rech_any_t rech_sa = %s; printf("%%s (Значение: %%s)", rech_any_to_str(rech_sa), rech_sa.type->name); }`, cExpr)
	default:
		return fmt.Sprintf(`printf("%%s", "?")`)
	}
}

// generateIREntityShowFunc строит определение функции показа
// пользовательской сущности ent - печатает "Имя { поле: значение (Тип), ... }"
// без завершающего переноса строки (его добавляет вызывающая сторона,
// generateIRStmt на *ir.ShowStmt). Параметр v - указатель для ссылочной
// "сущность"/"класс" (ctx.entityIsRef), значение для "структура" - та же
// форма, в которой значение этого типа реально передаётся везде ещё
// (см. ctx.entityIsRef/generateIRNewExpr/cType) - обращение к полю через
// "->"/"." выбирается тем же способом.
func generateIREntityShowFunc(ctx *irGenCtx, ent *ir.Entity) string {
	var sb strings.Builder
	paramType := entityCName(ctx, ent.Name)
	op := "."
	if ctx.entityIsRef(ent.Name) {
		paramType += " *"
		op = "->"
	}
	fmt.Fprintf(&sb, "static void %s(%s v) {\n", showFuncName(ent.Name), paramType)
	fmt.Fprintf(&sb, "    printf(\"%s { \");\n", escapeCString(ent.Name))
	for i, f := range ent.Fields {
		if i > 0 {
			sb.WriteString("    printf(\", \");\n")
		}
		fmt.Fprintf(&sb, "    printf(\"%s: \");\n", escapeCString(f.Name))
		fmt.Fprintf(&sb, "    %s;\n", generateIRShowExpr(ctx, f.Type, "v"+op+mangleField(f.Name)))
	}
	sb.WriteString("    printf(\" }\");\n")
	sb.WriteString("}\n\n")
	return sb.String()
}

// generateIRGenericShowFunc строит определение функции показа встроенной
// generic-коллекции g - печатает "Имя<T> [элемент, элемент, ...]"
// (Список/Стопка/Очередь - все три растущий массив с одинаковым набором
// полей data/len/cap, см. listCollectionCCode) или "Имя<К, В> {ключ:
// значение, ...}" (Словарь - хеш-таблица со слотами/state, см.
// dictCollectionCCode) - перебирая содержимое в настоящем C-цикле (длина
// известна только в рантайме, в отличие от полей сущности).
func generateIRGenericShowFunc(ctx *irGenCtx, g ir.GenericInstantiation) string {
	if g.Template == "Словарь" {
		return generateIRDictShowFunc(ctx, g)
	}
	if g.Template == "Множество" {
		return generateIRSetShowFunc(ctx, g)
	}
	var sb strings.Builder
	fmt.Fprintf(&sb, "static void %s(%s *v) {\n", showFuncName(g.Name), mangleType(g.Name))
	fmt.Fprintf(&sb, "    printf(\"%s [\");\n", escapeCString(g.Name))
	sb.WriteString("    for (long long i = 0; i < v->len; i++) {\n")
	sb.WriteString("        if (i > 0) printf(\", \");\n")
	fmt.Fprintf(&sb, "        %s;\n", generateIRShowExpr(ctx, g.ElemType, "v->data[i]"))
	sb.WriteString("    }\n")
	sb.WriteString("    printf(\"]\");\n")
	sb.WriteString("}\n\n")
	return sb.String()
}

// generateIRDictShowFunc - generateIRGenericShowFunc для "Словарь<К, В>" -
// см. package doc там.
func generateIRDictShowFunc(ctx *irGenCtx, g ir.GenericInstantiation) string {
	var sb strings.Builder
	fmt.Fprintf(&sb, "static void %s(%s *v) {\n", showFuncName(g.Name), mangleType(g.Name))
	fmt.Fprintf(&sb, "    printf(\"%s {\");\n", escapeCString(g.Name))
	sb.WriteString("    bool rech_show_first = true;\n")
	sb.WriteString("    for (long long i = 0; i < v->len; i++) {\n")
	sb.WriteString("        if (v->slots[i].state != 1) continue;\n")
	sb.WriteString("        if (!rech_show_first) printf(\", \");\n")
	sb.WriteString("        rech_show_first = false;\n")
	fmt.Fprintf(&sb, "        %s;\n", generateIRShowExpr(ctx, g.KeyType, "v->slots[i].key"))
	sb.WriteString("        printf(\": \");\n")
	fmt.Fprintf(&sb, "        %s;\n", generateIRShowExpr(ctx, g.ElemType, "v->slots[i].value"))
	sb.WriteString("    }\n")
	sb.WriteString("    printf(\"}\");\n")
	sb.WriteString("}\n\n")
	return sb.String()
}
