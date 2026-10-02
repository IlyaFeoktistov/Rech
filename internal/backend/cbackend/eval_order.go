package cbackend

import (
	"fmt"
	"slices"
	"strings"

	"github.com/IlyaFeoktistov/Rech/internal/ir"
)

// Порядок вычисления: в Rech операнды и аргументы вычисляются слева
// направо (так работает и interp), а в C порядок вычисления аргументов
// функции и операндов бинарного оператора не определён - gcc считает
// аргументы справа налево. Если среди операндов есть побочный эффект
// (вызов, "++"), операнды сначала вычисляются во временные переменные по
// порядку - внутри выражения-инструкции GNU "({ ... })" (gcc/clang, единственные
// компиляторы, которые ищет internal/toolchain), чтобы не выносить их за
// пределы выражения и не нарушать короткое замыкание "и"/"или"/"?:" вокруг.

// sequenceArgs - codes (уже сгенерированный C операндов exprs) в порядке
// слева направо: decls - объявления временных переменных ("" - порядок не
// важен, codes возвращаются как есть), out - коды для подстановки вместо
// codes. Литералы остаются на месте.
func sequenceArgs(ctx *irGenCtx, exprs []ir.Expr, codes []string) (decls string, out []string) {
	if !needsSequencing(ctx, exprs) {
		return "", codes
	}
	var sb strings.Builder
	out = make([]string, len(codes))
	for i, e := range exprs {
		if isConstExpr(e) {
			out[i] = codes[i]
			continue
		}
		ctx.chainTmpCounter++
		name := fmt.Sprintf("rech_ord%d", ctx.chainTmpCounter)
		fmt.Fprintf(&sb, "__typeof__(%s) %s = %s; ", codes[i], name, codes[i])
		out[i] = name
	}
	return sb.String(), out
}

// sequenced - code, перед которым вычисляются decls (см. sequenceArgs).
func sequenced(decls, code string) string {
	if decls == "" {
		return code
	}
	return fmt.Sprintf("({ %s%s; })", decls, code)
}

// needsSequencing - среди exprs есть побочный эффект и ещё хотя бы один
// не-литерал, на который порядок может повлиять.
func needsSequencing(ctx *irGenCtx, exprs []ir.Expr) bool {
	effects, nonConst := 0, 0
	for _, e := range exprs {
		if !isConstExpr(e) {
			nonConst++
		}
		if hasSideEffect(ctx, e) {
			effects++
		}
	}
	return effects > 0 && nonConst > 1
}

func isConstExpr(e ir.Expr) bool {
	switch e.(type) {
	case *ir.IntLiteral, *ir.FloatLiteral, *ir.StringLiteral, *ir.BoolLiteral, *ir.CharLiteral, *ir.NullLiteral:
		return true
	}
	return false
}

// hasSideEffect - e (или вложенное в него выражение) - вызов, "++"/"--",
// "дождаться" или "новый" сущности со своим конструктором: значение других
// операндов может зависеть от того, вычислен ли он раньше. Лямбда-литерал
// своё тело при создании не исполняет.
func hasSideEffect(ctx *irGenCtx, e ir.Expr) bool {
	switch e := e.(type) {
	case nil:
		return false
	case *ir.CallExpr, *ir.MethodCallExpr, *ir.StaticMethodCallExpr, *ir.CallValueExpr, *ir.IncDecExpr, *ir.AwaitExpr:
		return true
	case *ir.NewExpr:
		if ent := ctx.entities[e.Entity]; ent != nil && ent.Ctor != nil {
			return true
		}
	case *ir.LambdaExpr:
		return false
	}
	return slices.ContainsFunc(ir.ChildExprs(e), func(c ir.Expr) bool { return hasSideEffect(ctx, c) })
}
