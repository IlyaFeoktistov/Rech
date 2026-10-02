package semantic

import (
	"strings"

	"github.com/IlyaFeoktistov/Rech/internal/ast"
	"github.com/IlyaFeoktistov/Rech/internal/diagnostics"
	"github.com/IlyaFeoktistov/Rech/internal/ir"
	"github.com/IlyaFeoktistov/Rech/internal/typename"
)

// Массив фиксированной длины "T[N]" - значение (копия при присваивании/
// передаче/возврате, как "структура"), длина - часть типа. "м[i]" -
// элемент T (выход за границы - паника, константный индекс вне границ -
// ошибка компиляции), "м[i] = x" - запись, "&м[i]" - адрес элемента,
// "м.длина()" - N. Объявление без значения заполняет массив нулевыми
// значениями элемента. Представление совпадает с C-массивом "T a[N]" -
// годится полем "внешняя структура".

// resolveArrayTypeName - "T[N]" (имя уже с измерениями, typename.Array):
// регистрирует entitySig{IsArray} под каноническим именем - по уже
// резолвленному T ("Цвет[3]" перечисления - "Целое[3]").
func resolveArrayTypeName(entities map[string]entitySig, methods map[string]map[string]funcSig, aliases map[string]string, interfaces map[string]interfaceSig, enums map[string]map[string]int64, name string) (ir.Type, error) {
	elemName, size, _ := typename.OuterDim(name)
	n, err := arrayLen(name, size)
	if err != nil {
		return ir.TypeUnknown, err
	}
	if n <= 0 {
		return ir.TypeUnknown, errNoPos(CodeArray, "%q: длина массива должна быть больше нуля", name)
	}
	elem, err := resolveTypeName(entities, methods, aliases, interfaces, enums, elemName)
	if err != nil {
		return ir.TypeUnknown, diagnostics.Wrap(err, "тип элемента %q в %q", elemName, name)
	}
	if !arrayElemAllowed(entities, elem) {
		return ir.TypeUnknown, errNoPos(CodeArray, "%q: элементом массива может быть число, Символ, Байт, Логическое, Строка, указатель, внешняя сущность, сущность/структура или другой массив - не %s", name, elem)
	}
	canon := typename.Array(elem.String(), n)
	if _, exists := entities[canon]; !exists {
		entities[canon] = entitySig{IsArray: true, ElemType: elem, ArrayLen: n}
	}
	return ir.Type{Kind: ir.KindArray, Entity: canon}, nil
}

// arrayElemAllowed - T годится элементом "T[N]": у него есть нулевое
// значение и представление в C, не требующее особого порядка объявлений
// (коллекции, "?Тип", "Вызываемый", интерфейсы - нет).
func arrayElemAllowed(entities map[string]entitySig, t ir.Type) bool {
	switch t.Kind {
	case ir.KindPointer, ir.KindExternEntity, ir.KindArray:
		return true
	case ir.KindEntity:
		_, ok := entities[t.Entity]
		return ok && !strings.Contains(t.Entity, "<")
	}
	return isFFIScalar(t) || t == ir.TypeString
}

// arrayTypeFor - "T[N]" по уже известному T.
func arrayTypeFor(sc *ctx, elem ir.Type, n int64) (ir.Type, error) {
	return resolveArrayTypeName(sc.entities, sc.methods, sc.aliases, sc.interfaces, sc.enums, typename.Array(elem.String(), n))
}

// arrayInfo - T и N у массива t (t.Kind == KindArray).
func arrayInfo(sc *ctx, t ir.Type) (elem ir.Type, n int64) {
	sig := sc.entities[t.Entity]
	return sig.ElemType, sig.ArrayLen
}

// analyzeArrayIndex - "массив[индекс]". Константный индекс проверяется
// сразу.
func analyzeArrayIndex(sc *ctx, array ir.Expr, index ast.Expr, line int) (*ir.ArrayIndexExpr, error) {
	typ := ir.TypeOf(array)
	elem, n := arrayInfo(sc, typ)
	idx, err := analyzeExpr(sc, index)
	if err != nil {
		return nil, err
	}
	idx = maybeUnwrapNarrowed(sc, idx)
	if it := ir.TypeOf(idx); it != ir.TypeInt {
		return nil, errAtLine(line, CodeArray, "индекс массива должен быть типа Целое, получен %s", it)
	}
	if v, ok := constIntValue(idx); ok && (v < 0 || v >= n) {
		return nil, errAtLine(line, CodeArray, "индекс %d вне границ %s (допустимо от 0 до %d)", v, typ, n-1)
	}
	return &ir.ArrayIndexExpr{Array: array, Index: idx, Type: elem, Line: line}, nil
}

// analyzeArrayMethodCall - метод массива: "получить(i)" (то же, что
// "[i]"), "длина()" (N, константа). "установить" - инструкция
// (analyzeArrayStore).
func analyzeArrayMethodCall(sc *ctx, receiver ir.Expr, e *ast.MethodCallExpr) (ir.Expr, error) {
	typ := ir.TypeOf(receiver)
	switch e.Method {
	case "получить":
		if len(e.Args) != 1 {
			return nil, errAtLine(e.Line, CodeArray, "\"получить\" у %s принимает 1 аргумент (индекс), получено %d", typ, len(e.Args))
		}
		return analyzeArrayIndex(sc, receiver, e.Args[0], e.Line)
	case "длина":
		if len(e.Args) != 0 {
			return nil, errAtLine(e.Line, CodeArray, "\"длина\" у %s не принимает аргументов", typ)
		}
		_, n := arrayInfo(sc, typ)
		return &ir.IntLiteral{Value: n}, nil
	case "установить":
		return nil, errAtLine(e.Line, CodeArray, "\"установить\" у %s - инструкция, значения у неё нет", typ)
	}
	return nil, errAtLine(e.Line, CodeNameResolution, "у %s нет метода %q - есть индексация \"[i]\" (\"получить\"/\"установить\") и \"длина()\"", typ, e.Method)
}

// analyzeArrayStore - "массив[i] = значение" ("массив.установить(i,
// значение)"). Массив - значение, поэтому писать можно только в
// переменную, поле, элемент такого же массива или по указателю - запись в
// копию (результат вызова) потерялась бы.
func analyzeArrayStore(sc *ctx, receiver ir.Expr, e *ast.MethodCallExpr) (ir.Stmt, error) {
	typ := ir.TypeOf(receiver)
	if len(e.Args) != 2 {
		return nil, errAtLine(e.Line, CodeArray, "\"установить\" у %s принимает 2 аргумента (индекс, значение), получено %d", typ, len(e.Args))
	}
	if !isArrayLvalue(sc, receiver) {
		return nil, errAtLine(e.Line, CodeArray, "запись в элемент %s: массив - значение, писать можно только в переменную, поле или элемент массива, не в результат вызова", typ)
	}
	elemRef, err := analyzeArrayIndex(sc, receiver, e.Args[0], e.Line)
	if err != nil {
		return nil, err
	}
	value, err := coerceExpr(sc, e.Args[1], elemRef.Type)
	if err != nil {
		return nil, err
	}
	if vt := ir.TypeOf(value); !isAssignable(sc.entities, sc.interfaces, vt, elemRef.Type) {
		return nil, errAtLine(e.Line, CodeArray, "в элемент %s нельзя записать значение типа %s", typ, vt)
	}
	return &ir.ArrayAssignStmt{Array: receiver, Index: elemRef.Index, Value: value, Line: e.Line}, nil
}

// isArrayLvalue - e обозначает место хранения, а не временную копию: поле
// ссылочной сущности - всегда (объект один), поле структуры - если сама
// структура - место хранения.
func isArrayLvalue(sc *ctx, e ir.Expr) bool {
	switch e := e.(type) {
	case *ir.Identifier, *ir.DerefExpr, *ir.ThisExpr:
		return true
	case *ir.FieldAccess:
		if rt := ir.TypeOf(e.Receiver); rt.Kind == ir.KindEntity && entityIsRefType(sc.entities, rt.Entity) {
			return true
		}
		return isArrayLvalue(sc, e.Receiver)
	case *ir.ArrayIndexExpr:
		return isArrayLvalue(sc, e.Array)
	}
	return false
}

// analyzeArrayLiteral - "[a, b, ...]" там, где ожидается "T[N]": ровно N
// элементов, каждый приводится к T (вложенный литерал - к вложенному
// массиву).
func analyzeArrayLiteral(sc *ctx, ll *ast.ListLiteral, target ir.Type) (ir.Expr, error) {
	elem, n := arrayInfo(sc, target)
	if int64(len(ll.Elems)) != n {
		return nil, errAtLine(ll.Line, CodeArray, "литерал %s: ожидалось %d элементов, получено %d", target, n, len(ll.Elems))
	}
	elems := make([]ir.Expr, len(ll.Elems))
	for i, el := range ll.Elems {
		v, err := coerceExpr(sc, el, elem)
		if err != nil {
			return nil, err
		}
		if vt := ir.TypeOf(v); !isAssignable(sc.entities, sc.interfaces, vt, elem) {
			return nil, errAtLine(ll.Line, CodeArray, "литерал %s: элемент %d имеет тип %s, ожидался %s", target, i, vt, elem)
		}
		elems[i] = v
	}
	return &ir.ArrayLiteralExpr{Elems: elems, Type: target, Line: ll.Line}, nil
}

// fieldHasDefault - поле idx можно не указывать в "новый": у него есть
// значение по умолчанию, или это массив (по умолчанию - нули).
func (sig entitySig) fieldHasDefault(idx int) bool {
	if idx < len(sig.FieldDefaultExprs) && sig.FieldDefaultExprs[idx] != nil {
		return true
	}
	return sig.FieldTypes[idx].Kind == ir.KindArray
}
