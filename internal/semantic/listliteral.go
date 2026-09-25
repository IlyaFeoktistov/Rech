package semantic

import (
	"github.com/IlyaFeoktistov/Rech/internal/ast"
	"github.com/IlyaFeoktistov/Rech/internal/diagnostics"
	"github.com/IlyaFeoktistov/Rech/internal/ir"
)

// analyzeListLiteralExpr - "[элемент, ...]" (см. package doc
// ast.ListLiteral). hasTarget/targetElem - известный заранее тип элемента
// (из аннотации типа переменной/параметра/поля - см. coerceExpr). Без него
// тип элемента выводится из самих элементов - все одного типа T, а если
// среди них есть "Ничто" или значение "?T" - элемент "?T" (literalElemType).
// С ним - каждый элемент обязан быть этого типа; при "?T" обычное
// значение T и "Ничто" приводятся к нему.
func analyzeListLiteralExpr(sc *ctx, e *ast.ListLiteral, targetElem ir.Type, hasTarget bool) (ir.Expr, error) {
	if len(e.Elems) == 0 {
		if !hasTarget {
			return nil, errAtLine(e.Line, CodeSemanticOther, "пустой литерал списка \"[]\" требует известный тип - объявите тип переменной явно (\"переменная а: Список<Тип> = []\")")
		}
		return buildListLiteral(sc, nil, targetElem, e.Line)
	}

	elems := make([]ir.Expr, len(e.Elems))
	types := make([]ir.Type, len(e.Elems))
	var spread []bool
	for i, raw := range e.Elems {
		if _, isNull := raw.(*ast.NullLiteral); isNull {
			continue
		}
		v, vt, isSpread, err := analyzeListElem(sc, raw, e.Line)
		if err != nil {
			return nil, err
		}
		if isSpread && spread == nil {
			spread = make([]bool, len(e.Elems))
		}
		if isSpread {
			spread[i] = true
		}
		elems[i], types[i] = v, vt
	}

	elemType := targetElem
	if !hasTarget {
		var err error
		elemType, err = literalElemType(sc, e, types)
		if err != nil {
			return nil, err
		}
	}
	for i, raw := range e.Elems {
		v, err := coerceListElem(sc, raw, elems[i], types[i], spread != nil && spread[i], elemType, i, e.Line)
		if err != nil {
			return nil, err
		}
		elems[i] = v
	}
	lit, err := buildListLiteral(sc, elems, elemType, e.Line)
	if err != nil {
		return nil, err
	}
	lit.(*ir.ListLiteralExpr).Spread = spread
	return lit, nil
}

// literalElemType - тип элемента литерала без аннотации: types[i] пуст у
// "Ничто". Базовый тип T - у первого не-"Ничто" элемента ("?T" значения -
// его внутренний T), все остальные обязаны давать тот же T; "?T" -
// если хоть один элемент "Ничто" или сам "?T".
func literalElemType(sc *ctx, e *ast.ListLiteral, types []ir.Type) (ir.Type, error) {
	var base ir.Type
	haveBase, nullable := false, false
	for i, vt := range types {
		if vt == (ir.Type{}) {
			nullable = true
			continue
		}
		inner := vt
		if vt.Kind == ir.KindNullable {
			nullable = true
			inner = nullableInner(sc, vt)
		}
		if !haveBase {
			base, haveBase = inner, true
			continue
		}
		if inner != base {
			return ir.Type{}, errAtLine(e.Line, CodeSemanticOther, "элемент %d литерала списка должен быть типа %s (как первый элемент) - массивы строго типизированы, без неявного расширения, получен %s", i+1, base, vt)
		}
	}
	if !haveBase {
		return ir.Type{}, errAtLine(e.Line, CodeNullable, "литерал списка только из \"Ничто\" - тип элемента неоткуда вывести, объявите тип переменной явно (\"переменная а: Список<?Тип> = [Ничто]\")")
	}
	if !nullable {
		return base, nil
	}
	t, err := resolveNullableTypeName(sc.entities, sc.methods, sc.aliases, sc.interfaces, sc.enums, "?"+base.String())
	if err != nil {
		return ir.Type{}, diagnostics.WithPos(err, diagnostics.Position{File: currentFile, Line: e.Line})
	}
	return t, nil
}

// coerceListElem - элемент литерала (v/vt уже разобраны, кроме "Ничто" -
// тогда v == nil) к типу элемента списка: "Ничто" и T при "?T"
// оборачиваются, "...список" - только с ровно тем же типом элемента.
func coerceListElem(sc *ctx, raw ast.Expr, v ir.Expr, vt ir.Type, isSpread bool, elemType ir.Type, i, line int) (ir.Expr, error) {
	if nl, isNull := raw.(*ast.NullLiteral); isNull {
		if elemType.Kind != ir.KindNullable {
			return nil, errAtLine(line, CodeNullable, "элемент %d литерала списка - \"Ничто\", но элемент списка %s не nullable-тип (нужен \"?Тип\")", i+1, elemType)
		}
		return &ir.NullLiteral{Type: elemType, Line: nl.Line}, nil
	}
	if vt == elemType {
		return v, nil
	}
	if !isSpread {
		inner := elemType
		if elemType.Kind == ir.KindNullable {
			inner = nullableInner(sc, elemType)
		}
		if coerced, ok, err := coerceNumericConst(v, inner, line); err != nil {
			return nil, err
		} else if ok {
			v, vt = coerced, inner
			if vt == elemType {
				return v, nil
			}
		}
	}
	if !isSpread && elemType.Kind == ir.KindNullable && nullableInner(sc, elemType) == vt {
		return &ir.NullWrapExpr{Value: v, Type: elemType}, nil
	}
	return nil, errAtLine(line, CodeSemanticOther, "элемент %d литерала списка должен быть типа %s, получен %s", i+1, elemType, vt)
}

// analyzeListElem - один элемент литерала списка: обычное выражение или
// "...список" (ast.SpreadExpr) - тогда vt - тип элемента этого списка.
func analyzeListElem(sc *ctx, raw ast.Expr, line int) (v ir.Expr, vt ir.Type, isSpread bool, err error) {
	sp, isSpread := raw.(*ast.SpreadExpr)
	if !isSpread {
		v, err = analyzeExpr(sc, raw)
		if err != nil {
			return nil, ir.Type{}, false, err
		}
		v = maybeUnwrapNarrowed(sc, v)
		return v, ir.TypeOf(v), false, nil
	}
	v, err = analyzeExpr(sc, sp.Value)
	if err != nil {
		return nil, ir.Type{}, false, err
	}
	v = maybeUnwrapNarrowed(sc, v)
	t := ir.TypeOf(v)
	sig, ok := sc.entities[t.Entity]
	if t.Kind != ir.KindEntity || !ok || !sig.IsList {
		return nil, ir.Type{}, false, errAtLine(line, CodeSemanticOther, "\"...\" раскрывает только Список, получен %s", t)
	}
	return v, sig.ElemType, true, nil
}

// buildListLiteral резолвит/регистрирует "Список<elemType>" (тем же путём,
// что и обычное написанное явно "Список<Тип>", см. resolveGenericTypeName)
// и строит итоговый IR-узел.
func buildListLiteral(sc *ctx, elems []ir.Expr, elemType ir.Type, line int) (ir.Expr, error) {
	name := "Список<" + elemType.String() + ">"
	listType, err := resolveTypeName(sc.entities, sc.methods, sc.aliases, sc.interfaces, sc.enums, name)
	if err != nil {
		return nil, diagnostics.WithPos(err, diagnostics.Position{File: currentFile, Line: line})
	}
	sc.a.markListLiteralUsed(listType.Entity)
	return &ir.ListLiteralExpr{Elems: elems, ElemType: elemType, CollectionName: listType.Entity, Type: listType, Line: line}, nil
}
