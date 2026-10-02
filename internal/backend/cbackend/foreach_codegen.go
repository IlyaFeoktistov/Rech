package cbackend

import (
	"fmt"
	"strings"

	"github.com/IlyaFeoktistov/Rech/internal/backend/cbackend/runtime/collections"
	"github.com/IlyaFeoktistov/Rech/internal/ir"
)

// generateIRForEachStmt - "для [<ключ>,] <имя> в <коллекция>". Файл -
// потоковое построчное чтение (for(;;) до конца файла), остальные виды -
// индексированный проход по внутреннему буферу коллекции: Список/Очередь -
// data[0..len), Стопка - тот же буфер с конца (вершина - data[len-1]),
// Словарь - плотный массив записей slots[0..len) в порядке вставки с
// пропуском удалённых (state != 1, см. collections.DictCollectionCode).
// Счётчик и границы перечитываются на каждой итерации - та же семантика
// изменения коллекции внутри тела, что и у Список<T> раньше.
func generateIRForEachStmt(sb *strings.Builder, ctx *irGenCtx, s *ir.ForEachStmt, depth int) error {
	collCode, _, err := generateIRExpr(ctx, s.Collection)
	if err != nil {
		return err
	}
	ind := indent(depth)
	inner := indent(depth + 1)
	varC := mangleVar(s.Var)
	if s.Kind == ir.ForEachFile {
		ctx.usesFileIO = true
		ctx.usesPanic = true
		okC := "rech_fe_ok_" + varC
		fmt.Fprintf(sb, "%sfor (;;) {\n", ind)
		fmt.Fprintf(sb, "%sbool %s = false;\n", inner, okC)
		fmt.Fprintf(sb, "%sconst char *%s = rech_read_line_from_file(%s, &%s);\n", inner, varC, collCode, okC)
		fmt.Fprintf(sb, "%sif (!%s) { break; }\n", inner, okC)
		return generateIRForEachBody(sb, ctx, s, depth)
	}
	idxC := "rech_fe_i_" + varC
	elemC := fromIRType(s.ElemType).cType(ctx)
	fmt.Fprintf(sb, "%sfor (long long %s = 0; %s < %s->len; %s++) {\n", ind, idxC, idxC, collCode, idxC)
	switch s.Kind {
	case ir.ForEachDict:
		fmt.Fprintf(sb, "%sif (%s->slots[%s].state != 1) { continue; }\n", inner, collCode, idxC)
		if s.KeyVar != "" {
			fmt.Fprintf(sb, "%s%s %s = %s->slots[%s].key;\n", inner, fromIRType(s.KeyType).cType(ctx), mangleVar(s.KeyVar), collCode, idxC)
		}
		fmt.Fprintf(sb, "%s%s %s = %s->slots[%s].value;\n", inner, elemC, varC, collCode, idxC)
	case ir.ForEachSet:
		fmt.Fprintf(sb, "%sif (%s->slots[%s].state != 1) { continue; }\n", inner, collCode, idxC)
		writeForEachIndexVar(sb, s, inner, idxC)
		fmt.Fprintf(sb, "%s%s %s = %s->slots[%s].key;\n", inner, elemC, varC, collCode, idxC)
	case ir.ForEachStack:
		writeForEachIndexVar(sb, s, inner, idxC)
		fmt.Fprintf(sb, "%s%s %s = %s->data[%s->len - 1 - %s];\n", inner, elemC, varC, collCode, collCode, idxC)
	default:
		writeForEachIndexVar(sb, s, inner, idxC)
		fmt.Fprintf(sb, "%s%s %s = %s->data[%s];\n", inner, elemC, varC, collCode, idxC)
	}
	return generateIRForEachBody(sb, ctx, s, depth)
}

// writeForEachIndexVar объявляет переменную-ключ формы "для к, з в ..."
// у последовательных коллекций - порядковый номер элемента в обходе.
func writeForEachIndexVar(sb *strings.Builder, s *ir.ForEachStmt, inner, idxC string) {
	if s.KeyVar != "" {
		fmt.Fprintf(sb, "%slong long %s = %s;\n", inner, mangleVar(s.KeyVar), idxC)
	}
}

// generateIRForEachBody - тело цикла и метки "продолжить"/"прервать"
// (ctx.pushLoop) после уже открытого заголовка for.
func generateIRForEachBody(sb *strings.Builder, ctx *irGenCtx, s *ir.ForEachStmt, depth int) error {
	lf := ctx.pushLoop()
	for _, st := range s.Body {
		if err := generateIRStmt(sb, ctx, st, depth+1); err != nil {
			ctx.popLoop()
			return err
		}
	}
	ctx.popLoop()
	fmt.Fprintf(sb, "%s%s: ;\n", indent(depth+1), lf.continueTarget)
	fmt.Fprintf(sb, "%s}\n", indent(depth))
	fmt.Fprintf(sb, "%s%s: ;\n", indent(depth), lf.breakTarget)
	return nil
}

// writeToListCode дописывает в dest C-текст метода method ("ключи"/
// "значения") коллекции g - только если в программе есть инстанциация
// Список<elem> результата (semantic регистрирует её лениво, при первом
// вызове метода - нет инстанциации, значит метод не вызывается).
func writeToListCode(dest *strings.Builder, generics map[string]bool, kind string, g ir.GenericInstantiation, method string, elem ir.Type) {
	listName := "Список<" + elem.String() + ">"
	if !generics[listName] {
		return
	}
	dest.WriteString(collections.ToListCode(kind, mangleType(g.Name), mangleMethod(g.Name, method),
		mangleType(listName), mangleCtor(listName), mangleMethod(listName, "добавить")))
}
