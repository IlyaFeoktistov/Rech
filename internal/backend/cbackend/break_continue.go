package cbackend

import (
	"fmt"
	"strings"

	"github.com/IlyaFeoktistov/Rech/internal/ir"
)

// pushLoop/popLoop - см. package doc irGenCtx.loopStack. Вызывается вокруг
// генерации ТЕЛА каждого из четырёх видов цикла (WhileStmt/ForStmt/
// ForEachStmt/CStyleForStmt), в ir_generator.go/generateIRCStyleFor.
func (ctx *irGenCtx) pushLoop(continueTarget string) {
	ctx.loopStack = append(ctx.loopStack, loopFrame{cleanupBase: len(ctx.cleanupStack), continueTarget: continueTarget})
}

func (ctx *irGenCtx) popLoop() {
	ctx.loopStack = ctx.loopStack[:len(ctx.loopStack)-1]
}

// emitLoopJump - общая генерация "прервать"/"продолжить": jump - уже готовый
// C-переход ("break;\n" или "continue;\n"/"goto <метка>;\n"). Семантический
// анализ (ctx.loopDepth) гарантирует непустой ctx.loopStack здесь - "прервать"/
// "продолжить" вне цикла отвергнуты ДО кодогенерации.
//
// Если cleanupStack с момента входа в цикл не вырос (самый частый случай -
// нет вложенного "попытаться" между началом цикла и этой инструкцией) - jump
// испускается как есть. Иначе сначала прогоняются НОВЫЕ действия (см.
// package doc irGenCtx.loopStack про cleanupBase) - restoreCatchTop и, если
// есть, "окончательно" вложенного "попытаться", которое иначе осталось бы не
// выполненным (тот же баг класс, что решён для раннего "вернуть", см.
// exceptions.go/generateIRReturnStmt) - раньше самого jump, в собственных
// "{ }", чтобы объявления временных переменных внутри cleanup-действий не
// конфликтовали при нескольких "прервать"/"продолжить" в одной функции.
func emitLoopJump(sb *strings.Builder, ctx *irGenCtx, depth int, jump string) error {
	ind := indent(depth)
	lc := ctx.loopStack[len(ctx.loopStack)-1]
	if len(ctx.cleanupStack) == lc.cleanupBase {
		fmt.Fprintf(sb, "%s%s", ind, jump)
		return nil
	}
	fmt.Fprintf(sb, "%s{\n", ind)
	if err := emitCleanupUnwindFrom(sb, ctx, depth+1, lc.cleanupBase); err != nil {
		return err
	}
	fmt.Fprintf(sb, "%s%s", indent(depth+1), jump)
	fmt.Fprintf(sb, "%s}\n", ind)
	return nil
}

// generateIRBreakStmt/generateIRContinueStmt - см. package doc emitLoopJump.
func generateIRBreakStmt(sb *strings.Builder, ctx *irGenCtx, depth int) error {
	return emitLoopJump(sb, ctx, depth, "break;\n")
}

func generateIRContinueStmt(sb *strings.Builder, ctx *irGenCtx, depth int) error {
	lc := ctx.loopStack[len(ctx.loopStack)-1]
	jump := "continue;\n"
	if lc.continueTarget != "" {
		jump = fmt.Sprintf("goto %s;\n", lc.continueTarget)
	}
	return emitLoopJump(sb, ctx, depth, jump)
}

// generateIRCStyleFor - "для <init>; <условие>; <шаг> { <тело> }"
// (ir.CStyleForStmt) - Init/Cond, в отличие от Post, - обычные C-выражения
// (объявление/условие), помещаются прямо в заголовок C `for`, тем же
// приёмом, что и у ir.ForStmt (`for (long long i = from; i <= to; i++)`) -
// пустая секция (Init/Cond == nil, package doc ir.CStyleForStmt) переходит
// в пустую секцию заголовка C `for` (валидно сам по себе - "для ;; {...}"
// становится "for (;;) {...}", т. е. бесконечным).
//
// Post - ПРОИЗВОЛЬНАЯ Rech-инструкция (обычно присваивание/инкремент, но
// синтаксически - что угодно, что умеет parseStatement), не одно C-выражение -
// в C третья секция заголовка `for` обязана быть выражением, туда
// произвольную инструкцию не поместить. Поэтому Post генерируется как
// обычная инструкция ПОСЛЕ тела, перед меткой цикла нет - метка СРАЗУ
// перед Post (rech_for_continue_N) - иначе raw C `continue` изнутри тела
// (если бы третья секция заголовка была пуста, что она и есть) прыгнул бы
// прямо к повторной проверке условия, ПРОПУСТИВ Post - именно поэтому
// "продолжить" внутри CStyleForStmt эмитится как `goto` на эту метку, а не
// raw `continue` (см. package doc irGenCtx.loopStack.continueTarget).
func generateIRCStyleFor(sb *strings.Builder, ctx *irGenCtx, s *ir.CStyleForStmt, depth int) error {
	ind := indent(depth)
	var initCode string
	if s.Init != nil {
		code, typ, err := generateIRExpr(ctx, s.Init.Value)
		if err != nil {
			return err
		}
		initCode = fmt.Sprintf("%s %s = %s", typ.cType(ctx), mangleVar(s.Init.Name), code)
	}
	var condCode string
	if s.Cond != nil {
		code, _, err := generateIRExpr(ctx, s.Cond)
		if err != nil {
			return err
		}
		condCode = code
	}
	ctx.forLabelCounter++
	label := fmt.Sprintf("rech_for_continue_%d", ctx.forLabelCounter)

	fmt.Fprintf(sb, "%sfor (%s; %s;) {\n", ind, initCode, condCode)
	ctx.pushLoop(label)
	for _, st := range s.Body {
		if err := generateIRStmt(sb, ctx, st, depth+1); err != nil {
			ctx.popLoop()
			return err
		}
	}
	ctx.popLoop()
	fmt.Fprintf(sb, "%s%s: ;\n", indent(depth+1), label)
	if s.Post != nil {
		if err := generateIRStmt(sb, ctx, s.Post, depth+1); err != nil {
			return err
		}
	}
	fmt.Fprintf(sb, "%s}\n", ind)
	return nil
}
