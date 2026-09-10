package cbackend

import (
	"fmt"
	"strings"

	"github.com/IlyaFeoktistov/Rech/internal/ir"
)

// pushLoop/popLoop - см. package doc irGenCtx.loopStack. Вызывается вокруг
// генерации ТЕЛА каждого из четырёх видов цикла (WhileStmt/ForStmt/
// ForEachStmt/CStyleForStmt), в ir_generator.go/generateIRCStyleFor.
// pushLoop сам генерирует уникальную пару меток (continueTarget/
// breakTarget) и возвращает готовый кадр - вызывающий код помещает сами
// метки в C-код в нужном ему месте (см. package doc irGenCtx.loopStack про
// разницу мест для continueTarget/breakTarget между CStyleForStmt и
// остальными тремя видами).
func (ctx *irGenCtx) pushLoop() loopFrame {
	ctx.loopLabelCounter++
	n := ctx.loopLabelCounter
	lf := loopFrame{
		cleanupBase:    len(ctx.cleanupStack),
		continueTarget: fmt.Sprintf("rech_loop_continue_%d", n),
		breakTarget:    fmt.Sprintf("rech_loop_break_%d", n),
	}
	ctx.loopStack = append(ctx.loopStack, lf)
	return lf
}

func (ctx *irGenCtx) popLoop() {
	ctx.loopStack = ctx.loopStack[:len(ctx.loopStack)-1]
}

// emitLoopJump - общая генерация "прервать N"/"продолжить N": target - уже
// готовая C-метка кадра, на которую переходит "goto" (continueTarget или
// breakTarget НУЖНОГО кадра loopStack, см. generateIRBreakStmt/
// generateIRContinueStmt). Семантический анализ (ctx.loopDepth,
// checkLoopJumpDepth) гарантирует, что нужный кадр существует в
// ctx.loopStack здесь - "прервать"/"продолжить" вне цикла или глубже
// реальной вложенности отвергнуты ДО кодогенерации.
//
// Если cleanupStack с момента входа в ЦЕЛЕВОЙ цикл не вырос (самый частый
// случай - нет вложенного "попытаться"/цикла между его началом и этой
// инструкцией) - "goto" испускается как есть. Иначе сначала прогоняются
// НОВЫЕ действия (см. package doc irGenCtx.loopStack про cleanupBase) -
// restoreCatchTop и, если есть, "окончательно" вложенного "попытаться",
// которое иначе осталось бы не выполненным (тот же баг класс, что решён для
// раннего "вернуть", см. exceptions.go/generateIRReturnStmt) - раньше
// самого "goto", в собственных "{ }", чтобы объявления временных переменных
// внутри cleanup-действий не конфликтовали при нескольких "прервать"/
// "продолжить" в одной функции.
func emitLoopJump(sb *strings.Builder, ctx *irGenCtx, indentDepth int, cleanupBase int, target string) error {
	ind := indent(indentDepth)
	jump := fmt.Sprintf("goto %s;\n", target)
	if len(ctx.cleanupStack) == cleanupBase {
		fmt.Fprintf(sb, "%s%s", ind, jump)
		return nil
	}
	fmt.Fprintf(sb, "%s{\n", ind)
	if err := emitCleanupUnwindFrom(sb, ctx, indentDepth+1, cleanupBase); err != nil {
		return err
	}
	fmt.Fprintf(sb, "%s%s", indent(indentDepth+1), jump)
	fmt.Fprintf(sb, "%s}\n", ind)
	return nil
}

// generateIRBreakStmt/generateIRContinueStmt - см. package doc emitLoopJump.
// jumpDepth - ir.BreakStmt.Depth/ir.ContinueStmt.Depth (1 - ближайший
// охватывающий цикл, N - N-й по счёту, считая текущий).
func generateIRBreakStmt(sb *strings.Builder, ctx *irGenCtx, indentDepth int, jumpDepth int) error {
	lc := ctx.loopStack[len(ctx.loopStack)-jumpDepth]
	return emitLoopJump(sb, ctx, indentDepth, lc.cleanupBase, lc.breakTarget)
}

func generateIRContinueStmt(sb *strings.Builder, ctx *irGenCtx, indentDepth int, jumpDepth int) error {
	lc := ctx.loopStack[len(ctx.loopStack)-jumpDepth]
	return emitLoopJump(sb, ctx, indentDepth, lc.cleanupBase, lc.continueTarget)
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
// обычная инструкция ПОСЛЕ тела, перед закрывающей "}" самого C `for` -
// continueTarget-метка СРАЗУ перед Post - иначе "продолжить" изнутри тела
// прыгнул бы прямо к повторной проверке условия, ПРОПУСТИВ Post (третья
// секция заголовка пуста, raw C `continue` там бы сделал именно так) - см.
// package doc irGenCtx.loopStack.continueTarget. breakTarget-метка - ПОСЛЕ
// закрывающей "}" (вне C `for` целиком) - "прервать" не должен исполнять
// Post, в отличие от "продолжить".
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

	fmt.Fprintf(sb, "%sfor (%s; %s;) {\n", ind, initCode, condCode)
	lf := ctx.pushLoop()
	for _, st := range s.Body {
		if err := generateIRStmt(sb, ctx, st, depth+1); err != nil {
			ctx.popLoop()
			return err
		}
	}
	ctx.popLoop()
	fmt.Fprintf(sb, "%s%s: ;\n", indent(depth+1), lf.continueTarget)
	if s.Post != nil {
		if err := generateIRStmt(sb, ctx, s.Post, depth+1); err != nil {
			return err
		}
	}
	fmt.Fprintf(sb, "%s}\n", ind)
	fmt.Fprintf(sb, "%s%s: ;\n", ind, lf.breakTarget)
	return nil
}
