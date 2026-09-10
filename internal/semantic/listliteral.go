package semantic

import (
	"fmt"

	"github.com/IlyaFeoktistov/Rech/internal/ast"
	"github.com/IlyaFeoktistov/Rech/internal/ir"
)

// analyzeListLiteralExpr - "[элемент, ...]" (см. package doc
// ast.ListLiteral). hasTarget/targetElem - известный заранее тип элемента
// (из аннотации типа переменной/параметра/поля - см. coerceExpr), нужен
// только для ПУСТОГО литерала (типа иначе неоткуда взять) - непустой сам
// себя типизирует из первого элемента независимо от того, есть ли target
// (при наличии - дополнительно сверяется с ним).
func analyzeListLiteralExpr(sc *ctx, e *ast.ListLiteral, targetElem ir.Type, hasTarget bool) (ir.Expr, error) {
	if len(e.Elems) == 0 {
		if !hasTarget {
			return nil, fmt.Errorf("строка %d: пустой литерал списка \"[]\" требует известный тип - объявите тип переменной явно (\"переменная а: Список<Тип> = []\")", e.Line)
		}
		return buildListLiteral(sc, nil, targetElem, e.Line)
	}

	elems := make([]ir.Expr, len(e.Elems))
	var elemType ir.Type
	for i, raw := range e.Elems {
		v, err := analyzeExpr(sc, raw)
		if err != nil {
			return nil, err
		}
		v = maybeUnwrapNarrowed(sc, v)
		vt := ir.TypeOf(v)
		if i == 0 {
			elemType = vt
			if hasTarget && elemType != targetElem {
				return nil, fmt.Errorf("строка %d: элемент 1 литерала списка должен быть типа %s, получен %s", e.Line, targetElem, elemType)
			}
		} else if vt != elemType {
			return nil, fmt.Errorf("строка %d: элемент %d литерала списка должен быть типа %s (как первый элемент) - массивы строго типизированы, без неявного расширения, получен %s", e.Line, i+1, elemType, vt)
		}
		elems[i] = v
	}
	return buildListLiteral(sc, elems, elemType, e.Line)
}

// buildListLiteral резолвит/регистрирует "Список<elemType>" (тем же путём,
// что и обычное написанное явно "Список<Тип>" - resolveTypeName сама
// отклонит nullable-тип/пользовательскую сущность как элемент, см.
// resolveGenericTypeName) и строит итоговый IR-узел.
func buildListLiteral(sc *ctx, elems []ir.Expr, elemType ir.Type, line int) (ir.Expr, error) {
	name := "Список<" + elemType.String() + ">"
	listType, err := resolveTypeName(sc.entities, sc.methods, sc.aliases, sc.interfaces, sc.enums, name)
	if err != nil {
		return nil, fmt.Errorf("строка %d: %w", line, err)
	}
	sc.a.markListLiteralUsed(listType.Entity)
	return &ir.ListLiteralExpr{Elems: elems, ElemType: elemType, CollectionName: listType.Entity, Type: listType, Line: line}, nil
}
