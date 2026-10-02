package cbackend

import (
	"fmt"
	"strings"

	"github.com/IlyaFeoktistov/Rech/internal/backend/cbackend/runtime/collections"
	"github.com/IlyaFeoktistov/Rech/internal/ir"
	"github.com/IlyaFeoktistov/Rech/internal/rtmsg"
)

// Множество<T> - хеш-таблица словаря (collections.DictCollectionCode) с
// ключом T и значением bool: элемент - ключ. Свои методы - обёртки над
// внутренними методами словаря (см. internal/semantic/set_type.go).

// setCollectionCode - C-код Множество<T> g: таблица, конструктор, методы,
// "равен".
func setCollectionCode(ctx *irGenCtx, g ir.GenericInstantiation) string {
	keyC := fromIRType(g.KeyType).cType(ctx)
	structC := mangleType(g.Name)
	put, get := mangleMethod(g.Name, "__установить"), mangleMethod(g.Name, "__получить")
	var sb strings.Builder
	sb.WriteString(collections.DictCollectionCode(keyC, "bool", structC,
		put, get, mangleMethod(g.Name, "содержит"),
		mangleMethod(g.Name, "удалить"), mangleMethod(g.Name, "длина"), mangleMethod(g.Name, "пусто"),
		escapeCString(rtmsg.DictKeyNotFound(g.Name)), g.KeyType.Kind == ir.KindString))
	sb.WriteString(collections.GenericCtorCode(structC, mangleCtor(g.Name)))
	fmt.Fprintf(&sb, "static void %s(%s *this_, %s x) {\n    %s(this_, x, true);\n}\n\n", mangleMethod(g.Name, "добавить"), structC, keyC, put)
	// "равен": одинаковые наборы элементов (значения - всегда истина).
	eq := g
	eq.ElemType = ir.TypeBool
	sb.WriteString(dictEqualCCode(ctx, eq))
	return sb.String()
}

// generateIRSetShowFunc - "показать" для Множество<T>: "Имя {a, b}".
func generateIRSetShowFunc(ctx *irGenCtx, g ir.GenericInstantiation) string {
	var sb strings.Builder
	fmt.Fprintf(&sb, "static void %s(%s *v) {\n", showFuncName(g.Name), mangleType(g.Name))
	fmt.Fprintf(&sb, "    printf(\"%s {\");\n", escapeCString(g.Name))
	sb.WriteString("    bool rech_show_first = true;\n")
	sb.WriteString("    for (long long i = 0; i < v->len; i++) {\n")
	sb.WriteString("        if (v->slots[i].state != 1) continue;\n")
	sb.WriteString("        if (!rech_show_first) printf(\", \");\n")
	sb.WriteString("        rech_show_first = false;\n")
	fmt.Fprintf(&sb, "        %s;\n", generateIRShowExpr(ctx, g.KeyType, "v->slots[i].key"))
	sb.WriteString("    }\n")
	sb.WriteString("    printf(\"}\");\n")
	sb.WriteString("}\n\n")
	return sb.String()
}
