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
// этого типа), одним printf для примитивов.
func generateIRShowExpr(t ir.Type, cExpr string) string {
	switch t.Kind {
	case ir.KindString:
		return fmt.Sprintf(`printf("\"%%s\" (Строка)", %s)`, cExpr)
	case ir.KindInt:
		return fmt.Sprintf(`printf("%%lld (Целое)", %s)`, cExpr)
	case ir.KindFloat:
		return fmt.Sprintf(`printf("%%g (Действительное)", %s)`, cExpr)
	case ir.KindBool:
		return fmt.Sprintf(`printf("%%s (Логическое)", (%s) ? "истина" : "ложь")`, cExpr)
	case ir.KindEntity:
		return fmt.Sprintf("%s(%s)", showFuncName(t.Entity), cExpr)
	default:
		return fmt.Sprintf(`printf("%%s", "?")`)
	}
}

// generateIREntityShowFunc строит определение функции показа
// пользовательской сущности ent - печатает "Имя { поле: значение (Тип), ... }"
// без завершающего переноса строки (его добавляет вызывающая сторона,
// generateIRStmt на *ir.ShowStmt).
func generateIREntityShowFunc(ent *ir.Entity) string {
	var sb strings.Builder
	fmt.Fprintf(&sb, "static void %s(%s v) {\n", showFuncName(ent.Name), mangleType(ent.Name))
	fmt.Fprintf(&sb, "    printf(\"%s { \");\n", escapeCString(ent.Name))
	for i, f := range ent.Fields {
		if i > 0 {
			sb.WriteString("    printf(\", \");\n")
		}
		fmt.Fprintf(&sb, "    printf(\"%s: \");\n", escapeCString(f.Name))
		fmt.Fprintf(&sb, "    %s;\n", generateIRShowExpr(f.Type, "v."+mangleField(f.Name)))
	}
	sb.WriteString("    printf(\" }\");\n")
	sb.WriteString("}\n\n")
	return sb.String()
}

// generateIRGenericShowFunc строит определение функции показа встроенной
// generic-коллекции g (Список/Стопка/Очередь - все три растущий массив с
// одинаковым набором полей data/len/cap, см. listCollectionCCode) -
// печатает "Имя<T> [элемент, элемент, ...]", перебирая data[0..len) в
// настоящем C-цикле (длина известна только в рантайме, в отличие от полей
// сущности).
func generateIRGenericShowFunc(g ir.GenericInstantiation) string {
	var sb strings.Builder
	fmt.Fprintf(&sb, "static void %s(%s v) {\n", showFuncName(g.Name), mangleType(g.Name))
	fmt.Fprintf(&sb, "    printf(\"%s [\");\n", escapeCString(g.Name))
	sb.WriteString("    for (long long i = 0; i < v.len; i++) {\n")
	sb.WriteString("        if (i > 0) printf(\", \");\n")
	fmt.Fprintf(&sb, "        %s;\n", generateIRShowExpr(g.ElemType, "v.data[i]"))
	sb.WriteString("    }\n")
	sb.WriteString("    printf(\"]\");\n")
	sb.WriteString("}\n\n")
	return sb.String()
}
