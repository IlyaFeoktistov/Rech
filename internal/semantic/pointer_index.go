package semantic

import (
	"github.com/IlyaFeoktistov/Rech/internal/ast"
	"github.com/IlyaFeoktistov/Rech/internal/ir"
)

// Индексация "Указатель<T>" - указатель как начало C-массива T:
// "п[i]"/"п.получить(i)" читает i-й элемент, "п[i] = значение"/
// "п.установить(i, значение)" пишет его, "&п[i]" - адрес i-го элемента.
// Границы не проверяются (длины у указателя нет), как и в C.

// pointerElementAddr - адрес элемента index массива по адресу pointer.
func pointerElementAddr(sc *ctx, pointer ir.Expr, index ast.Expr, line int) (*ir.PointerOffsetExpr, error) {
	idx, err := analyzeExpr(sc, index)
	if err != nil {
		return nil, err
	}
	idx = maybeUnwrapNarrowed(sc, idx)
	if it := ir.TypeOf(idx); it != ir.TypeInt {
		return nil, errAtLine(line, CodePointer, "индекс указателя должен быть типа Целое, получен %s", it)
	}
	return &ir.PointerOffsetExpr{Pointer: pointer, Index: idx, Type: ir.TypeOf(pointer), Line: line}, nil
}

// pointerElementRead - "п[i]": значение T, не "?T", в отличие от Список<T>.
func pointerElementRead(sc *ctx, pointer ir.Expr, index ast.Expr, line int) (ir.Expr, error) {
	addr, err := pointerElementAddr(sc, pointer, index, line)
	if err != nil {
		return nil, err
	}
	return &ir.DerefExpr{Value: addr, Type: pointerInner(sc, addr.Type), Line: line}, nil
}

// analyzePointerMethodCall - метод у "Указатель<T>": только "получить".
// "установить" - инструкция (analyzePointerStore), значения у неё нет.
func analyzePointerMethodCall(sc *ctx, receiver ir.Expr, e *ast.MethodCallExpr) (ir.Expr, error) {
	typ := ir.TypeOf(receiver)
	switch e.Method {
	case "получить":
		if len(e.Args) != 1 {
			return nil, errAtLine(e.Line, CodePointer, "\"получить\" у %s принимает 1 аргумент (индекс), получено %d", typ, len(e.Args))
		}
		return pointerElementRead(sc, receiver, e.Args[0], e.Line)
	case "установить":
		return nil, errAtLine(e.Line, CodeExprCheck, "\"установить\" у %s - инструкция, значения у неё нет", typ)
	}
	return nil, errAtLine(e.Line, CodeNameResolution, "у %s нет метода %q - есть только индексация \"[i]\" (\"получить\"/\"установить\")", typ, e.Method)
}

// analyzePointerStore - "п[i] = значение" ("п.установить(i, значение)").
func analyzePointerStore(sc *ctx, receiver ir.Expr, e *ast.MethodCallExpr) (ir.Stmt, error) {
	typ := ir.TypeOf(receiver)
	if len(e.Args) != 2 {
		return nil, errAtLine(e.Line, CodePointer, "\"установить\" у %s принимает 2 аргумента (индекс, значение), получено %d", typ, len(e.Args))
	}
	addr, err := pointerElementAddr(sc, receiver, e.Args[0], e.Line)
	if err != nil {
		return nil, err
	}
	inner := pointerInner(sc, typ)
	value, err := coerceExpr(sc, e.Args[1], inner)
	if err != nil {
		return nil, err
	}
	if vt := ir.TypeOf(value); !isAssignable(sc.entities, sc.interfaces, vt, inner) {
		return nil, errAtLine(e.Line, CodePointer, "через указатель на %s нельзя записать значение типа %s", inner, vt)
	}
	return &ir.DerefAssignStmt{Pointer: addr, Value: value, Line: e.Line}, nil
}
