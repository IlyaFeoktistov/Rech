// Перегрузка функций/методов (по эталону C#, объём v1 - см. OVERLOAD_PLAN.md
// в истории коммитов) - несколько функций верхнего уровня (или простых,
// не абстрактных/не статических методов сущности) с одним и тем же именем,
// но разными списками типов параметров. Решается ЦЕЛИКОМ на этапе
// semantic: ir.CallExpr.Callee/ir.MethodCallExpr.Method остаются простой
// строкой (см. их package doc) - каждому объявлению группы присваивается
// собственное мангл-имя вида "имя(Тип1,Тип2)" ("()" - без параметров),
// используемое как ключ в funcs/methods[Сущность] и как Callee/Method в
// самом IR - cbackend/interp резолвят функцию/метод по этой строке как
// есть, без понятия "сигнатура", тем же приёмом, что уже применяется к
// generic-инстанциациям (mangleGenericCallName, internal/semantic/
// generics.go). Скобки и запятая не могут встретиться в обычном
// идентификаторе Rech - мангл-имя не столкнётся ни с одним реальным
// объявленным именем.
package semantic

import (
	"strings"

	"github.com/IlyaFeoktistov/Rech/internal/ast"
	"github.com/IlyaFeoktistov/Rech/internal/ir"
)

// overloadMangledName строит мангл-имя одной конкретной перегрузки name с
// данным списком типов параметров - см. package doc выше.
func overloadMangledName(name string, params []ir.Type) string {
	var b strings.Builder
	b.WriteString(name)
	b.WriteByte('(')
	for i, p := range params {
		if i > 0 {
			b.WriteByte(',')
		}
		b.WriteString(p.String())
	}
	b.WriteByte(')')
	return b.String()
}

// paramTypesString - список типов параметров через запятую, для текста
// ошибок ("две перегрузки с одинаковым списком типов параметров").
func paramTypesString(params []ir.Type) string {
	if len(params) == 0 {
		return "без параметров"
	}
	parts := make([]string, len(params))
	for i, p := range params {
		parts[i] = p.String()
	}
	return strings.Join(parts, ", ")
}

// sameParamTypeList - true, если a и b - один и тот же список типов
// параметров позиция-в-позицию: две перегрузки с одинаковым списком
// неотличимы ни на одном вызове, отвергается уже на сборе сигнатур
// (collectFuncSigs/collectEntitySigs), как и в C#.
func sameParamTypeList(a, b []ir.Type) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}

// resolveFuncSigForCall резолвит вызов "callee(args...)" против группы
// перегрузок candidates (funcOverloads[callee] - свободные функции, или
// methodOverloads[Сущность][callee] - методы, вызывающая сторона сама
// решает, какую карту передать) - ok=false означает "callee вообще не
// перегруженное имя в этой карте", вызывающая сторона продолжает обычную
// резолюцию как раньше (например, funcs[callee] напрямую, или "может это
// переменная типа Вызываемый"). ok=true, err!=nil - callee ТОЧНО
// перегружено, но ни один кандидат не подошёл, или подошло несколько сразу
// (неоднозначно) - ошибка обязана пропагироваться немедленно, дальнейшая
// резолюция уже не имеет смысла.
func resolveFuncSigForCall(sc *ctx, name string, candidates []funcSig, args []ast.Expr, line int) (sig funcSig, mangledCallee string, ok bool, err error) {
	if candidates == nil {
		return funcSig{}, "", false, nil
	}
	best, perr := pickOverload(sc, name, candidates, args, line)
	if perr != nil {
		return funcSig{}, "", true, perr
	}
	return best, best.MangledName, true, nil
}

// pickOverload выбирает единственную подходящую сигнатуру из candidates по
// естественным типам args. Каждый args[i] пробно разбирается БЕЗ контекста
// целевого типа (analyzeExpr(sc, args[i])) - ошибку такого разбора (в
// первую очередь "Ничто" без контекста типа, analyzeExpr(*ast.NullLiteral),
// не имеющее смысла без цели) превращает в "тип этой позиции неизвестен"
// (совместим с любым параметром-кандидатом, без бонуса точности), а не
// пропагирует - настоящий разбор аргумента произойдёт ещё раз, уже против
// выигравшей сигнатуры, через checkCallArgs (двойной разбор одного и того
// же ast.Expr предполагается безопасным - весь анализ выражений в этом
// компиляторе idempotent, тот же принцип, что и у ленивых "ensure"-регистраций
// generic/reflect/regex-целей).
func pickOverload(sc *ctx, name string, candidates []funcSig, args []ast.Expr, line int) (funcSig, error) {
	natural := make([]*ir.Type, len(args))
	for i, arg := range args {
		expr, err := analyzeExpr(sc, arg)
		if err != nil {
			continue
		}
		t := ir.TypeOf(expr)
		natural[i] = &t
	}

	var best funcSig
	bestScore := -1
	ambiguous := false
	found := false
	for _, cand := range candidates {
		score, ok := scoreOverloadCandidate(sc, cand, len(args), natural)
		if !ok {
			continue
		}
		switch {
		case !found || score > bestScore:
			best = cand
			bestScore = score
			ambiguous = false
			found = true
		case score == bestScore:
			ambiguous = true
		}
	}
	if !found {
		return funcSig{}, errAtLine(line, CodeSemanticOther, "нет подходящей перегрузки %q для переданных аргументов (%d шт.)", name, len(args))
	}
	if ambiguous {
		return funcSig{}, errAtLine(line, CodeSemanticOther, "неоднозначный вызов перегруженной функции %q - несколько перегрузок одинаково подходят переданным аргументам", name)
	}
	return best, nil
}

// scoreOverloadCandidate - совместимость и "качество" кандидата cand для
// вызова с argc аргументами и естественными типами natural (nil на позиции -
// тип не удалось определить без контекста, см. package doc pickOverload).
// ok=false - кандидат в принципе не годится (арность не совпадает с учётом
// ParamDefaults - тот же minRequired, что и checkCallArgs, либо на позиции с
// известным типом натуральный тип несовместим с параметром) - не участвует
// в выборе. Числового приведения нет вообще нигде в языке (Целое НЕ
// приводится к Действительное, isAssignable) - счёт не нуждается в
// C#-подобной лестнице промоций, только "точное совпадение" (2 очка) /
// "совместимо через isAssignable - сущность-подтип или сущность-интерфейс"
// (1 очко) на каждой известной позиции.
func scoreOverloadCandidate(sc *ctx, cand funcSig, argc int, natural []*ir.Type) (int, bool) {
	if argc > len(cand.Params) {
		return 0, false
	}
	minRequired := len(cand.Params)
	for i := len(cand.Params) - 1; i >= 0 && i < len(cand.ParamDefaults) && cand.ParamDefaults[i] != nil; i-- {
		minRequired = i
	}
	if argc < minRequired {
		return 0, false
	}
	score := 0
	for i := 0; i < argc; i++ {
		nt := natural[i]
		if nt == nil {
			continue
		}
		pt := cand.Params[i]
		switch {
		case *nt == pt:
			score += 2
		case isAssignable(sc.entities, sc.interfaces, *nt, pt):
			score++
		default:
			return 0, false
		}
	}
	return score, true
}
