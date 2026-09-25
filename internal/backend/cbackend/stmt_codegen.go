// Кодогенерация C для инструкций (Stmt) из internal/ir - выделено из
// ir_generator.go как отдельный домен (см. package doc там же про
// irGenCtx/помощники пакета).
package cbackend

import (
	"fmt"
	"strings"

	"github.com/IlyaFeoktistov/Rech/internal/ir"
	"github.com/IlyaFeoktistov/Rech/internal/rtmsg"
)

// generateIRStmt - тонкая обёртка над generateIRStmtBody: заводит для
// ЭТОЙ инструкции свежий ctx.chainTmpPreamble (см. package doc
// materializeIfNeeded) и локальный буфер, генерирует тело инструкции туда
// (сама инструкция, а если она составная - IfStmt/WhileStmt/... - и всё
// вложенное тело рекурсивными вызовами ЭТОЙ ЖЕ обёртки, каждый со своим
// собственным chainTmpPreamble), затем сбрасывает накопленные объявления
// временных переменных В sb ПЕРЕД строкой самой инструкции - порядок
// важен (переменная должна быть объявлена раньше своего использования).
// Вложенные инструкции (тела if/while и т. п.) пишут не в исходный sb
// напрямую, а в тот же локальный буфер этой функции (см. вызовы
// generateIRStmt(sb, ...) внутри switch generateIRStmtBody - там sb уже
// это локальный буфер, а не аргумент верхнего вызова) - поэтому временная
// переменная вложенной инструкции окажется на своём месте, прямо перед
// НЕЙ, а не выше всего блока.
func generateIRStmt(sb *strings.Builder, ctx *irGenCtx, s ir.Stmt, depth int) error {
	ctx.curIndent = indent(depth)
	ctx.chainTmpPreamble = nil
	var body strings.Builder
	err := generateIRStmtBody(&body, ctx, s, depth)
	for _, p := range ctx.chainTmpPreamble {
		sb.WriteString(p)
	}
	ctx.chainTmpPreamble = nil
	if err != nil {
		return err
	}
	sb.WriteString(body.String())
	return nil
}

func generateIRStmtBody(sb *strings.Builder, ctx *irGenCtx, s ir.Stmt, depth int) error {
	ind := indent(depth)
	// Строка ТЕКУЩЕЙ инструкции обновляет верхний кадр стека вызовов
	// (rech_trace_line) ДО генерации собственно инструкции - если внутри
	// неё (вложенным вызовом или напрямую) случится паника, кадр уже
	// покажет правильную строку. См. stmtLine/trace.go.
	if line := ir.LineOf(s); line > 0 {
		fmt.Fprintf(sb, "%srech_trace_line(%d);\n", ind, line)
	}
	switch s := s.(type) {
	case *ir.PrintStmt:
		code, typ, err := generateIRExpr(ctx, s.Value)
		if err != nil {
			return err
		}
		fmt.Fprintf(sb, "%s%s\n", ind, printCall(ctx, typ, code))
		return nil

	case *ir.ShowStmt:
		ctx.usesShow = true
		code, _, err := generateIRExpr(ctx, s.Value)
		if err != nil {
			return err
		}
		fmt.Fprintf(sb, "%s%s;\n", ind, generateIRShowExpr(ctx, ir.TypeOf(s.Value), code))
		fmt.Fprintf(sb, "%sprintf(\"\\n\");\n", ind)
		return nil

	case *ir.VarDecl:
		if s.IsStatic {
			return generateIRStaticVarDecl(sb, ctx, s, ind)
		}
		code, typ, err := generateIRExpr(ctx, s.Value)
		if err != nil {
			return err
		}
		// Через временную переменную, не напрямую "Тип v_x = <code>;" - в C
		// имя объявляемой переменной входит в область видимости уже в точке
		// её собственного объявления, ДО инициализатора справа от "=" -
		// самоссылающийся инициализатор при затенении внешней переменной с
		// тем же именем ("если истина { переменная х = х + 1 }", где
		// снаружи уже есть своя "х") иначе читал бы ещё не
		// проинициализированную (мусорную) новую "х", а не старую. "_INIT" -
		// заглавными буквами специально: mangleVar/transliterate всегда
		// приводят весь мангл-идентификатор к нижнему регистру, поэтому
		// такое имя не может совпасть ни с одной настоящей переменной Rech.
		if s.DeclType.Kind != ir.KindUnknown {
			typ = fromIRType(s.DeclType)
		}
		cType := typ.cType(ctx)
		mangled := mangleVar(s.Name)
		tmp := mangled + "_INIT"
		fmt.Fprintf(sb, "%s%s %s = %s;\n", ind, cType, tmp, code)
		fmt.Fprintf(sb, "%s%s %s = %s;\n", ind, cType, mangled, tmp)
		return nil

	case *ir.Assign:
		code, _, err := generateIRExpr(ctx, s.Value)
		if err != nil {
			return err
		}
		fmt.Fprintf(sb, "%s%s = %s;\n", ind, varRef(ctx, s.Name), code)
		return nil

	case *ir.NullCoalesceAssignStmt:
		code, _, err := generateIRExpr(ctx, s.Value)
		if err != nil {
			return err
		}
		name := varRef(ctx, s.Name)
		fmt.Fprintf(sb, "%sif (!%s.has) { %s = %s; }\n", ind, name, name, code)
		return nil

	case *ir.IfStmt:
		cond, _, err := generateIRExpr(ctx, s.Cond)
		if err != nil {
			return err
		}
		fmt.Fprintf(sb, "%sif (%s) {\n", ind, cond)
		for _, st := range s.Then {
			if err := generateIRStmt(sb, ctx, st, depth+1); err != nil {
				return err
			}
		}
		if s.Else != nil {
			fmt.Fprintf(sb, "%s} else {\n", ind)
			for _, st := range s.Else {
				if err := generateIRStmt(sb, ctx, st, depth+1); err != nil {
					return err
				}
			}
		}
		fmt.Fprintf(sb, "%s}\n", ind)
		return nil

	case *ir.GerundStmt:
		// "{"/"}" без условия - только чтобы у "результат" (первый элемент
		// Body, см. package doc ir.GerundStmt) была собственная область
		// видимости в C, не любая специальная кодогенерация.
		fmt.Fprintf(sb, "%s{\n", ind)
		for _, st := range s.Body {
			if err := generateIRStmt(sb, ctx, st, depth+1); err != nil {
				return err
			}
		}
		fmt.Fprintf(sb, "%s}\n", ind)
		return nil

	case *ir.BlockStmt:
		// Тот же приём, что и у GerundStmt выше - "{"/"}" только ради
		// собственной C-области видимости.
		fmt.Fprintf(sb, "%s{\n", ind)
		for _, st := range s.Body {
			if err := generateIRStmt(sb, ctx, st, depth+1); err != nil {
				return err
			}
		}
		fmt.Fprintf(sb, "%s}\n", ind)
		return nil

	case *ir.WhileStmt:
		cond, _, err := generateIRExpr(ctx, s.Cond)
		if err != nil {
			return err
		}
		fmt.Fprintf(sb, "%swhile (%s) {\n", ind, cond)
		lf := ctx.pushLoop()
		for _, st := range s.Body {
			if err := generateIRStmt(sb, ctx, st, depth+1); err != nil {
				ctx.popLoop()
				return err
			}
		}
		ctx.popLoop()
		fmt.Fprintf(sb, "%s%s: ;\n", indent(depth+1), lf.continueTarget)
		fmt.Fprintf(sb, "%s}\n", ind)
		fmt.Fprintf(sb, "%s%s: ;\n", ind, lf.breakTarget)
		return nil

	case *ir.BreakStmt:
		return generateIRBreakStmt(sb, ctx, depth, s.Depth)

	case *ir.ContinueStmt:
		return generateIRContinueStmt(sb, ctx, depth, s.Depth)

	case *ir.CStyleForStmt:
		return generateIRCStyleFor(sb, ctx, s, depth)

	case *ir.ReturnStmt:
		if s.Value == nil {
			if len(ctx.cleanupStack) > 0 {
				fmt.Fprintf(sb, "%s{\n", ind)
				if err := emitCleanupUnwind(sb, ctx, depth+1); err != nil {
					return err
				}
				fmt.Fprintf(sb, "%s    %sreturn 0;\n", ind, tracePop)
				fmt.Fprintf(sb, "%s}\n", ind)
				return nil
			}
			fmt.Fprintf(sb, "%s%s%sreturn 0;\n", ind, tracePop, ind)
			return nil
		}
		code, typ, err := generateIRExpr(ctx, s.Value)
		if err != nil {
			return err
		}
		// Значение вычисляется во временную переменную ДО rech_trace_pop() -
		// если сам code (вложенный вызов) паникует, кадр этой функции ещё
		// обязан быть на стеке (см. package doc trace.go). "return code;"
		// сразу после pop() вычислил бы code уже без него. Обёрнуто в
		// собственный "{ }" - имя переменной одно и то же на каждом "вернуть"
		// функции, отдельная область видимости на каждый исключает
		// повторное объявление, даже если "вернуть" встречается более
		// одного раза (разные ветки "если").
		fmt.Fprintf(sb, "%s{\n", ind)
		fmt.Fprintf(sb, "%s    %s %s = %s;\n", ind, typ.cType(ctx), returnTmpVar, code)
		if err := emitCleanupUnwind(sb, ctx, depth+1); err != nil {
			return err
		}
		fmt.Fprintf(sb, "%s    %s%s    return %s;\n", ind, tracePop, ind, returnTmpVar)
		fmt.Fprintf(sb, "%s}\n", ind)
		return nil

	case *ir.FieldAssign:
		receiverCode, _, err := generateIRExpr(ctx, s.Receiver)
		if err != nil {
			return err
		}
		code, _, err := generateIRExpr(ctx, s.Value)
		if err != nil {
			return err
		}
		if valType := ir.TypeOf(s.Value); valType.Kind == ir.KindCallable || valType.Kind == ir.KindNullable {
			// "объект.поле = замыкание" - то же самое, что и поле у "новый
			// Тип(...)" (generateIRNewExpr) - если поле принадлежит
			// "внешняя структура", {fn, env} по значению не то, что
			// ожидает C (голый указатель на функцию).
			if recvEnt, ok := ctx.entities[ir.TypeOf(s.Receiver).Entity]; ok && recvEnt.IsExternStruct {
				marshaled, err := ctx.marshalCallbackArg(valType, code)
				if err != nil {
					return err
				}
				code = marshaled
			}
		}
		op := "."
		if ctx.receiverIsPointer(s.Receiver) {
			op = "->"
		} else {
			// Получатель-"структура" не адресуем напрямую (результат вызова,
			// не переменная/поле/"этот") - "(rvalue).поле = ..." невалиден в
			// C так же, как и "&(rvalue)" у вызова метода. materializeIfNeeded
			// заводит временную переменную (уже lvalue, "." работает).
			receiverCode = ctx.materializeIfNeeded(s.Receiver, receiverCode)
		}
		fmt.Fprintf(sb, "%s(%s)%s%s = %s;\n", ind, receiverCode, op, mangleField(s.Field), code)
		return nil

	case *ir.StaticFieldAssign:
		code, _, err := generateIRExpr(ctx, s.Value)
		if err != nil {
			return err
		}
		fmt.Fprintf(sb, "%s%s = %s;\n", ind, mangleStaticField(s.Entity, s.Field), code)
		return nil

	case *ir.DerefAssignStmt:
		ptrCode, _, err := generateIRExpr(ctx, s.Pointer)
		if err != nil {
			return err
		}
		valCode, _, err := generateIRExpr(ctx, s.Value)
		if err != nil {
			return err
		}
		fmt.Fprintf(sb, "%s*%s = %s;\n", ind, ptrCode, valCode)
		return nil

	case *ir.BecomeStmt:
		// Состояние уже проверено семантическим анализом как объявленное у
		// сущности получателя - здесь только устанавливает бит в
		// рантайм-хранилище ("state_flags", см. package doc
		// ir.Entity.HasStateFlags) - читает его StateCheckExpr там, где
		// established не смог доказать переход на этапе компиляции.
		recvCode, _, err := generateIRExpr(ctx, s.Receiver)
		if err != nil {
			return err
		}
		entName := ir.TypeOf(s.Receiver).Entity
		ent, ok := ctx.entities[entName]
		if !ok {
			return fmt.Errorf("генерация C из IR: сущность %q (\"становится\") не найдена", entName)
		}
		bit, ok := ent.StateBit(s.State)
		if !ok {
			return fmt.Errorf("генерация C из IR: у сущности %q нет состояния %q", entName, s.State)
		}
		fmt.Fprintf(sb, "%s%s->state_flags |= 1ULL << %d;\n", ind, recvCode, bit)
		return nil

	case *ir.ParentCallStmt:
		argCodes := make([]string, len(s.Args)+1)
		// Явное приведение к указателю на родителя: у потомка своя
		// C-структура, GCC 14+ считает неявное приведение ошибкой.
		argCodes[0] = fmt.Sprintf("(%s *)this_", entityCName(ctx, s.BaseEntity))
		for i, a := range s.Args {
			code, _, err := generateIRExpr(ctx, a)
			if err != nil {
				return err
			}
			argCodes[i+1] = code
		}
		fmt.Fprintf(sb, "%s%s(%s);\n", ind, mangleCtorInit(s.BaseEntity), strings.Join(argCodes, ", "))
		return nil

	case *ir.AssertStmt:
		cond, _, err := generateIRExpr(ctx, s.Cond)
		if err != nil {
			return err
		}
		ctx.usesPanic = true
		if s.Message == nil {
			fmt.Fprintf(sb, "%sif (!(%s)) { %s; }\n", ind, cond, panicCallC(rtmsg.PanicAssertion, `"`+escapeCString(rtmsg.AssertFailed(s.Line))+`"`))
			return nil
		}
		// "утверждать <условие>, <сообщение>" (внятные
		// сообщения о несовпадении) - сообщение конкатенируется с обычным
		// текстом ("утверждение не выполнено (строка N): <сообщение>"),
		// вычисляется в тексте самого "if" (C сам не вычисляет его, если
		// условие истинно - обычное короткое замыкание условного блока, тот
		// же принцип, что и у ветки тернарного оператора).
		msgCode, _, err := generateIRExpr(ctx, s.Message)
		if err != nil {
			return err
		}
		ctx.usesConcat = true
		prefix := escapeCString(rtmsg.AssertFailed(s.Line) + ": ")
		fmt.Fprintf(sb, "%sif (!(%s)) { %s; }\n", ind, cond, panicCallC(rtmsg.PanicAssertion, fmt.Sprintf(`rech_str_concat("%s", %s)`, prefix, msgCode)))
		return nil

	case *ir.ExprStmt:
		var call ir.Expr = s.Call
		switch {
		case s.MethodCall != nil:
			call = s.MethodCall
		case s.StaticMethodCall != nil:
			call = s.StaticMethodCall
		case s.IncDec != nil:
			call = s.IncDec
		case s.Await != nil:
			call = s.Await
		case s.CallValue != nil:
			call = s.CallValue
		}
		code, _, err := generateIRExpr(ctx, call)
		if err != nil {
			return err
		}
		fmt.Fprintf(sb, "%s%s;\n", ind, code)
		return nil

	case *ir.ForStmt:
		fromCode, _, err := generateIRExpr(ctx, s.From)
		if err != nil {
			return err
		}
		toCode, _, err := generateIRExpr(ctx, s.To)
		if err != nil {
			return err
		}
		varC := mangleVar(s.Var)
		fmt.Fprintf(sb, "%sfor (long long %s = %s; %s <= %s; %s++) {\n", ind, varC, fromCode, varC, toCode, varC)
		lf := ctx.pushLoop()
		for _, st := range s.Body {
			if err := generateIRStmt(sb, ctx, st, depth+1); err != nil {
				ctx.popLoop()
				return err
			}
		}
		ctx.popLoop()
		fmt.Fprintf(sb, "%s%s: ;\n", indent(depth+1), lf.continueTarget)
		fmt.Fprintf(sb, "%s}\n", ind)
		fmt.Fprintf(sb, "%s%s: ;\n", ind, lf.breakTarget)
		return nil

	case *ir.ThrowStmt:
		return generateIRThrowStmt(sb, ctx, s, depth)

	case *ir.TryStmt:
		return generateIRTryStmt(sb, ctx, s, depth)

	case *ir.ForEachStmt:
		return generateIRForEachStmt(sb, ctx, s, depth)

	default:
		return fmt.Errorf("генерация C из IR: инструкция %T пока не поддерживается", s)
	}
}
