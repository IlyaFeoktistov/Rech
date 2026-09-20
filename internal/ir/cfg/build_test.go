package cfg

import (
	"testing"

	"github.com/IlyaFeoktistov/Rech/internal/ir"
)

func lit(n int) ir.Expr { return &ir.IntLiteral{Value: int64(n)} }

func ret(line int, hasValue bool) *ir.ReturnStmt {
	if !hasValue {
		return &ir.ReturnStmt{Line: line}
	}
	return &ir.ReturnStmt{Value: lit(0), Line: line}
}

func throwStmt(line int) *ir.ThrowStmt {
	return &ir.ThrowStmt{Value: lit(0), Line: line}
}

func printStmt(line int) *ir.PrintStmt {
	return &ir.PrintStmt{Value: lit(0), Line: line}
}

func TestFallsThroughEmptyBody(t *testing.T) {
	g := Build(nil)
	if !g.FallsThrough() {
		t.Fatal("пустое тело должно проваливаться в конец")
	}
}

func TestFallsThroughSimpleReturn(t *testing.T) {
	g := Build([]ir.Stmt{ret(1, true)})
	if g.FallsThrough() {
		t.Fatal("тело, заканчивающееся вернуть, не должно проваливаться")
	}
}

func TestFallsThroughIfWithoutElseFallsThrough(t *testing.T) {
	// если истина { вернуть 0 }
	// код после if - код "падает" через ложную ветвь.
	body := []ir.Stmt{
		&ir.IfStmt{Cond: &ir.BoolLiteral{Value: true}, Then: []ir.Stmt{ret(1, true)}, Line: 1},
	}
	g := Build(body)
	if !g.FallsThrough() {
		t.Fatal("если без иначе должен проваливаться через ложную ветвь")
	}
}

func TestFallsThroughIfElseBothReturnDoesNotFallThrough(t *testing.T) {
	body := []ir.Stmt{
		&ir.IfStmt{Cond: &ir.BoolLiteral{Value: true}, Then: []ir.Stmt{ret(1, true)}, Else: []ir.Stmt{ret(2, true)}, Line: 1},
	}
	g := Build(body)
	if g.FallsThrough() {
		t.Fatal("обе ветви возвращают значение - не должно проваливаться")
	}
}

func TestFallsThroughWhileTrueWithoutBreakDiverges(t *testing.T) {
	body := []ir.Stmt{
		&ir.WhileStmt{Cond: &ir.BoolLiteral{Value: true}, Body: []ir.Stmt{printStmt(2)}, Line: 1},
	}
	g := Build(body)
	if g.FallsThrough() {
		t.Fatal("\"пока истина\" без прервать обязано расходиться целиком")
	}
}

func TestFallsThroughWhileTrueWithBreakFallsThrough(t *testing.T) {
	body := []ir.Stmt{
		&ir.WhileStmt{Cond: &ir.BoolLiteral{Value: true}, Body: []ir.Stmt{
			&ir.IfStmt{Cond: &ir.BoolLiteral{Value: true}, Then: []ir.Stmt{&ir.BreakStmt{Depth: 1, Line: 2}}, Line: 2},
		}, Line: 1},
	}
	g := Build(body)
	if !g.FallsThrough() {
		t.Fatal("\"пока истина\" с достижимым прервать должно проваливаться после цикла")
	}
}

func TestFallsThroughWhileNonConstFallsThroughEvenWithoutBreak(t *testing.T) {
	body := []ir.Stmt{
		&ir.WhileStmt{Cond: &ir.BoolLiteral{Value: false}, Body: []ir.Stmt{printStmt(2)}, Line: 1},
	}
	g := Build(body)
	if !g.FallsThrough() {
		t.Fatal("обычное условие (не буквальная \"истина\") всегда может стать ложным - должно проваливаться")
	}
}

func TestFallsThroughCStyleForEverForeverWithoutBreakDiverges(t *testing.T) {
	body := []ir.Stmt{
		&ir.CStyleForStmt{Cond: nil, Body: []ir.Stmt{printStmt(2)}, Line: 1},
	}
	g := Build(body)
	if g.FallsThrough() {
		t.Fatal("\"для(;;)\" без прервать обязано расходиться")
	}
}

func TestFallsThroughForRangeAlwaysFallsThrough(t *testing.T) {
	body := []ir.Stmt{
		&ir.ForStmt{Var: "i", From: lit(0), To: lit(10), Body: []ir.Stmt{printStmt(2)}, Line: 1},
	}
	g := Build(body)
	if !g.FallsThrough() {
		t.Fatal("числовой диапазон не статически бесконечен - должен проваливаться")
	}
}

func TestFallsThroughTryFinallyReturnsOverridesResult(t *testing.T) {
	// попытаться { вывести } окончательно { вернуть 0 } - вся конструкция не проваливается.
	body := []ir.Stmt{
		&ir.TryStmt{Body: []ir.Stmt{printStmt(1)}, NoCatch: true, Finally: []ir.Stmt{ret(2, true)}, Line: 1},
	}
	g := Build(body)
	if g.FallsThrough() {
		t.Fatal("\"окончательно\" с вернуть переопределяет результат - не должно проваливаться")
	}
}

func TestFallsThroughTryFinallyNormalFallsThrough(t *testing.T) {
	body := []ir.Stmt{
		&ir.TryStmt{Body: []ir.Stmt{ret(1, true)}, NoCatch: true, Finally: []ir.Stmt{printStmt(2)}, Line: 1},
		printStmt(3),
	}
	g := Build(body)
	if g.FallsThrough() {
		t.Fatal("тело всегда возвращает, окончательно проваливается нормально - вся попытка тоже не должна падать после себя")
	}
}

func TestFallsThroughTryCatchBothDivergeDoesNotFallThrough(t *testing.T) {
	body := []ir.Stmt{
		&ir.TryStmt{Body: []ir.Stmt{ret(1, true)}, CatchVar: "e", CatchTypes: []string{"Исключение"}, CatchBody: []ir.Stmt{ret(2, true)}, Line: 1},
	}
	g := Build(body)
	if g.FallsThrough() {
		t.Fatal("и тело, и обработчик возвращают значение - не должно проваливаться")
	}
}

func TestUnreachableAfterReturn(t *testing.T) {
	body := []ir.Stmt{ret(1, true), printStmt(2)}
	g := Build(body)
	line, ok := g.UnreachableLine()
	if !ok || line != 2 {
		t.Fatalf("UnreachableLine() = %d, %v - ожидалась строка 2", line, ok)
	}
}

func TestUnreachableAfterBreak(t *testing.T) {
	body := []ir.Stmt{
		&ir.WhileStmt{Cond: &ir.BoolLiteral{Value: true}, Body: []ir.Stmt{
			&ir.BreakStmt{Depth: 1, Line: 2},
			printStmt(3),
		}, Line: 1},
	}
	g := Build(body)
	line, ok := g.UnreachableLine()
	if !ok || line != 3 {
		t.Fatalf("UnreachableLine() = %d, %v - ожидалась строка 3", line, ok)
	}
}

func TestUnreachableAfterIfElseBothDiverge(t *testing.T) {
	body := []ir.Stmt{
		&ir.IfStmt{Cond: &ir.BoolLiteral{Value: true}, Then: []ir.Stmt{ret(1, true)}, Else: []ir.Stmt{ret(2, true)}, Line: 1},
		printStmt(3),
	}
	g := Build(body)
	line, ok := g.UnreachableLine()
	if !ok || line != 3 {
		t.Fatalf("UnreachableLine() = %d, %v - ожидалась строка 3", line, ok)
	}
}

func TestNoFalsePositiveUnreachableAfterIfWithoutElse(t *testing.T) {
	body := []ir.Stmt{
		&ir.IfStmt{Cond: &ir.BoolLiteral{Value: true}, Then: []ir.Stmt{ret(1, true)}, Line: 1},
		printStmt(2),
	}
	g := Build(body)
	if _, ok := g.UnreachableLine(); ok {
		t.Fatal("код после \"если\" без \"иначе\" достижим через ложную ветвь - ложное срабатывание")
	}
}

func TestNoFalsePositiveCatchWithoutExplicitThrow(t *testing.T) {
	// поймать без единого явного "выбросить" в теле - легитимно (паника
	// произойти может от чего угодно) - не должен считаться недостижимым.
	body := []ir.Stmt{
		&ir.TryStmt{Body: []ir.Stmt{printStmt(1)}, CatchVar: "e", CatchTypes: []string{"Исключение"}, CatchBody: []ir.Stmt{printStmt(2)}, Line: 1},
	}
	g := Build(body)
	if _, ok := g.UnreachableLine(); ok {
		t.Fatal("\"поймать\" без явного \"выбросить\" в теле ложно посчитан недостижимым")
	}
}

func TestNoFalsePositiveFinallyReachedOnlyViaEarlyReturn(t *testing.T) {
	// попытаться { вернуть 0 } окончательно { вывести } - тело сразу
	// возвращает, "окончательно" всё равно должно исполняться (реально).
	body := []ir.Stmt{
		&ir.TryStmt{Body: []ir.Stmt{ret(1, true)}, NoCatch: true, Finally: []ir.Stmt{printStmt(2)}, Line: 1},
	}
	g := Build(body)
	if _, ok := g.UnreachableLine(); ok {
		t.Fatal("\"окончательно\", достижимое только через ранний вернуть, ложно посчитано недостижимым")
	}
}

func TestUnreachableAfterThrowUncaught(t *testing.T) {
	body := []ir.Stmt{throwStmt(1), printStmt(2)}
	g := Build(body)
	line, ok := g.UnreachableLine()
	if !ok || line != 2 {
		t.Fatalf("UnreachableLine() = %d, %v - ожидалась строка 2", line, ok)
	}
}

func TestNestedMultiCatchChainFallsThroughWhenNeitherHandlesFully(t *testing.T) {
	// Цепочка "поймать" представлена вложенными TryStmt (internal/parser) -
	// первый написанный поймать - самый внутренний. Один из обработчиков не
	// возвращает значение - вся цепочка тоже должна проваливаться.
	inner := &ir.TryStmt{
		Body:       []ir.Stmt{ret(1, true)},
		CatchVar:   "a",
		CatchTypes: []string{"А"},
		CatchBody:  []ir.Stmt{ret(2, true)},
		Line:       1,
	}
	outer := &ir.TryStmt{
		Body:       []ir.Stmt{inner},
		CatchVar:   "b",
		CatchTypes: []string{"Б"},
		CatchBody:  []ir.Stmt{printStmt(3)},
		Line:       1,
	}
	g := Build([]ir.Stmt{outer})
	if !g.FallsThrough() {
		t.Fatal("внешний поймать не возвращает значение - вся цепочка должна проваливаться")
	}
}
