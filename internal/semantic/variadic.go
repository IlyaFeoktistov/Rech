package semantic

import (
	"strings"

	"github.com/IlyaFeoktistov/Rech/internal/ast"
	"github.com/IlyaFeoktistov/Rech/internal/ir"
)

// hasVariadicParam - последний параметр объявлен как "...имя: Тип"
// (parser.checkVariadicParam гарантирует, что только последний).
func hasVariadicParam(params []ast.Param) bool {
	return len(params) > 0 && params[len(params)-1].Variadic
}

// variadicInferArgs - типы параметров, против которых выводятся аргументы
// типа шаблона функции, по одному на аргумент: у вариативной части это
// тип элемента ("...список" - сам Список, сверяется его значение).
func variadicInferArgs(params []ast.Param, args []ast.Expr) ([]string, []ast.Expr) {
	types := make([]string, len(params))
	for i, p := range params {
		types[i] = p.Type
	}
	if !hasVariadicParam(params) || len(args) < len(params)-1 {
		return types, args
	}
	fixed := len(params) - 1
	listType := types[fixed]
	elemType := strings.TrimSuffix(strings.TrimPrefix(listType, "Список<"), ">")
	types = types[:fixed]
	outArgs := append([]ast.Expr(nil), args[:fixed]...)
	for _, a := range args[fixed:] {
		if sp, ok := a.(*ast.SpreadExpr); ok {
			types = append(types, listType)
			outArgs = append(outArgs, sp.Value)
			continue
		}
		types = append(types, elemType)
		outArgs = append(outArgs, a)
	}
	return types, outArgs
}

// checkSpreadPlacement - "...список" среди аргументов допустим только в
// вариативной части вызова (с позиции вариативного параметра).
func checkSpreadPlacement(args []ast.Expr, sig funcSig, line int, callee string) error {
	for i, arg := range args {
		if _, ok := arg.(*ast.SpreadExpr); !ok {
			continue
		}
		if sig.CVariadic {
			return errAtLine(line, CodeExprCheck, "аргумент %d: \"...список\" не раскрывается в аргументы C-функции %s - передайте значения по одному", i+1, callee)
		}
		if !sig.Variadic {
			return errAtLine(line, CodeExprCheck, "аргумент %d: \"...\" допустимо только в вариативной части вызова, а %s не вариативна", i+1, callee)
		}
		if i < len(sig.Params)-1 {
			return errAtLine(line, CodeExprCheck, "аргумент %d: \"...\" допустимо только в вариативной части вызова (с аргумента %d)", i+1, len(sig.Params))
		}
	}
	return nil
}

// analyzeVariadicTail собирает аргументы вариативной части вызова в один
// Список (listType - тип вариативного параметра): обычный аргумент
// приводится к типу элемента, как любой аргумент вызова, "...список"
// раскрывается на своём месте. pos - индекс вариативного параметра.
func analyzeVariadicTail(sc *ctx, tail []ast.Expr, listType ir.Type, pos, line int, argLabel string) (ir.Expr, error) {
	elemType := sc.entities[listType.Entity].ElemType
	elems := make([]ir.Expr, len(tail))
	var spread []bool
	for i, raw := range tail {
		n := pos + i + 1
		if sp, ok := raw.(*ast.SpreadExpr); ok {
			v, err := analyzeExpr(sc, sp.Value)
			if err != nil {
				return nil, err
			}
			v = maybeUnwrapNarrowed(sc, v)
			if t := ir.TypeOf(v); t != listType {
				return nil, errAtLine(line, CodeExprCheck, "аргумент %d %s: \"...\" должно раскрывать %s, получен %s", n, argLabel, listType, t)
			}
			if spread == nil {
				spread = make([]bool, len(tail))
			}
			spread[i] = true
			elems[i] = v
			continue
		}
		v, err := coerceExpr(sc, raw, elemType)
		if err != nil {
			return nil, err
		}
		if t := ir.TypeOf(v); !isAssignable(sc.entities, sc.interfaces, t, elemType) {
			return nil, errAtLine(line, CodeExprCheck, "аргумент %d %s должен быть типа %s, получен %s", n, argLabel, elemType, t)
		}
		invalidateEstablishedForOpaqueArg(sc, v)
		elems[i] = v
	}
	lit, err := buildListLiteral(sc, elems, elemType, line)
	if err != nil {
		return nil, err
	}
	lit.(*ir.ListLiteralExpr).Spread = spread
	return lit, nil
}
