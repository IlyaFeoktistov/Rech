// Кодогенерация C для выражений (Expr) из internal/ir - выделено из
// ir_generator.go как отдельный домен (см. package doc там же про
// irGenCtx/помощники пакета).
package cbackend

import (
	"fmt"
	"strings"

	"github.com/IlyaFeoktistov/Rech/internal/ir"
	"github.com/IlyaFeoktistov/Rech/internal/rtmsg"
)

func generateIRExpr(ctx *irGenCtx, e ir.Expr) (string, valueType, error) {
	switch e := e.(type) {
	case *ir.StringLiteral:
		return fmt.Sprintf(`"%s"`, escapeCString(e.Value)), typeString, nil

	case *ir.IntLiteral:
		return fmt.Sprintf("%dLL", e.Value), typeInt, nil

	case *ir.FloatLiteral:
		return formatFloatLiteral(e.Value), typeFloat, nil

	case *ir.BoolLiteral:
		if e.Value {
			return "true", typeBool, nil
		}
		return "false", typeBool, nil

	case *ir.Identifier:
		return varRef(ctx, e.Name), fromIRType(e.Type), nil

	case *ir.UnaryExpr:
		return generateIRUnaryExpr(ctx, e)

	case *ir.IncDecExpr:
		name := varRef(ctx, e.Name)
		if e.Prefix {
			return fmt.Sprintf("(%s%s)", e.Op, name), fromIRType(e.Type), nil
		}
		return fmt.Sprintf("(%s%s)", name, e.Op), fromIRType(e.Type), nil

	case *ir.BinaryExpr:
		return generateIRBinaryExpr(ctx, e)

	case *ir.TernaryExpr:
		// Родной тернарный оператор C - уже короткое замыкание (ровно одна
		// из веток вычисляется), Then/Else семантически проверены
		// одинакового типа (analyzeTernaryExpr) - никакого хелпера не
		// нужно, в отличие от "??" ниже (там Left - nullable-обёртка,
		// нужно её развернуть).
		condCode, _, err := generateIRExpr(ctx, e.Cond)
		if err != nil {
			return "", typeUnknown, err
		}
		thenCode, _, err := generateIRExpr(ctx, e.Then)
		if err != nil {
			return "", typeUnknown, err
		}
		elseCode, _, err := generateIRExpr(ctx, e.Else)
		if err != nil {
			return "", typeUnknown, err
		}
		return fmt.Sprintf("((%s) ? (%s) : (%s))", condCode, thenCode, elseCode), fromIRType(e.Type), nil

	case *ir.CallExpr:
		cName, isExtern := ctx.externCNames[e.Callee]
		argCodes := make([]string, len(e.Args))
		for i, a := range e.Args {
			code, _, err := generateIRExpr(ctx, a)
			if err != nil {
				return "", typeUnknown, err
			}
			// isExtern && KindCallable - "Вызываемый(...)" аргумент внешней
			// функции ("FFI: маршалинг колбэков") - {fn, env}
			// по значению не то, что ожидает C API (голый указатель на
			// функцию, без слота под env) - marshalCallbackArg подменяет code
			// на C-имя сгенерированного trampoline (см. ffi_callback.go).
			if isExtern && ir.TypeOf(a).Kind == ir.KindCallable {
				marshaled, err := ctx.marshalCallbackArg(ir.TypeOf(a), code)
				if err != nil {
					return "", typeUnknown, err
				}
				code = marshaled
			}
			// Настоящий прототип из заголовка (не собственная догадка
			// cbackend) почти наверняка объявляет параметр непрозрачным
			// структурным указателем своего имени (HWND/HDC/...) - см.
			// package doc bridgeExternHeaderArgCode (extern_headers.go).
			if isExtern && externPrototypeAlreadyProvided(ctx, cName) {
				code = bridgeExternHeaderArgCode(ctx, ir.TypeOf(a), code)
			}
			argCodes[i] = code
		}
		// Порядок проверки: builtin (inline C-выражение) -> внешняя функция
		// (настоящий C-символ, не mangleName) -> обычная функция программы.
		if code, ok := builtinCallCode(&ctx.builtinFlags, e.Callee, argCodes); ok {
			return code, fromIRType(e.Type), nil
		}
		callee := funcCName(ctx, e.Callee)
		if isExtern {
			callee = cName
		}
		if e.Callee == entryPointName {
			// "главная" манглится в настоящий C "main" (mangleName) - у НЕГО,
			// в отличие от любой другой функции программы, сигнатура ВСЕГДА
			// "(int argc, char **argv)" (см. package doc про paramList "int
			// argc, char **argv" в generateIRFunction), а не та, что вывелась
			// бы из ir.Function.Params самой "главная" (у неё их нет вовсе,
			// docs/SPEC.md - "главная" не принимает аргументы командной
			// строки, они читаются отдельными builtin'ами). Аргументы самого
			// Rech-вызова ("главная()") поэтому всегда пусты - C-вызов обязан
			// передать (rech_argc, rech_argv), не "()" - иначе "main()" не
			// совпадает с собственным прототипом "main(int, char**)" и cc
			// отказывается собирать (найдено живьём при проверке, можно ли
			// вообще вызвать "главная" из самого же Rech-кода).
			return fmt.Sprintf("%s(rech_argc, rech_argv)", callee), fromIRType(e.Type), nil
		}
		callCode := fmt.Sprintf("%s(%s)", callee, strings.Join(argCodes, ", "))
		if _, _, isFn := ctx.ffiFuncPointerSig(e.Type); isExtern && e.HasReturn && isFn {
			code, err := ctx.unmarshalFuncPointer(e.Type, callCode, fmt.Sprintf(rtmsg.FFINullFuncPointerResultTemplate, cName))
			if err != nil {
				return "", typeUnknown, err
			}
			return code, fromIRType(e.Type), nil
		}
		if isExtern && e.HasReturn && externPrototypeAlreadyProvided(ctx, cName) {
			callCode = bridgeExternHeaderReturnCode(ctx, e.Type, callCode)
		}
		return callCode, fromIRType(e.Type), nil

	case *ir.NewExpr:
		return generateIRNewExpr(ctx, e)

	case *ir.FieldAccess:
		code, _, err := generateIRExpr(ctx, e.Receiver)
		if err != nil {
			return "", typeUnknown, err
		}
		op := "."
		if ctx.receiverIsPointer(e.Receiver) {
			op = "->"
		}
		access := fmt.Sprintf("(%s)%s%s", code, op, mangleField(e.Field))
		// Поле-колбэк внешней структуры хранит голый указатель на функцию.
		recvName := ir.TypeOf(e.Receiver).Entity
		if ent, ok := ctx.entities[recvName]; ok && ent.IsExternStruct {
			if _, _, isFn := ctx.ffiFuncPointerSig(e.Type); isFn {
				unmarshaled, err := ctx.unmarshalFuncPointer(e.Type, access, fmt.Sprintf(rtmsg.FFINullFuncPointerFieldTemplate, e.Field, ent.ExternCName))
				if err != nil {
					return "", typeUnknown, err
				}
				return unmarshaled, fromIRType(e.Type), nil
			}
		}
		return access, fromIRType(e.Type), nil

	case *ir.ThisExpr:
		// this_ - параметр метода, указатель всегда, вне зависимости от
		// того, "сущность" её тип или "структура" (получатель метода -
		// общее исключение из копирования, шаг 12). Но
		// "видимая" форма ЗНАЧЕНИЯ этого типа - то, что ожидают FieldAccess/
		// присваивание/возврат/передача параметром - должна совпадать с
		// формой, в которой этот тип представлен everywhere: указатель для
		// ссылочной сущности ("this_" как есть - тот же указатель, что и
		// любая другая переменная этого типа), разыменованное значение для
		// структуры ("(*this_)" - this_ здесь просто временный адрес для
		// мутации через метод, не "истинное" представление значения).
		if ctx.entityIsRef(e.Entity) {
			return "this_", entityType(e.Entity), nil
		}
		return "(*this_)", entityType(e.Entity), nil

	case *ir.MethodCallExpr:
		return generateIRMethodCallExpr(ctx, e)

	case *ir.StaticFieldExpr:
		return mangleStaticField(e.Entity, e.Field), fromIRType(e.Type), nil

	case *ir.StaticMethodCallExpr:
		return generateIRStaticMethodCallExpr(ctx, e)

	case *ir.CharLiteral:
		return fmt.Sprintf("%d", e.Value), typeChar, nil

	case *ir.CastExpr:
		return generateIRCastExpr(ctx, e)

	case *ir.ParseExpr:
		return generateIRParseExpr(ctx, e)

	case *ir.SerializeExpr:
		return generateIRSerializeExpr(ctx, e)

	case *ir.IsExpr:
		return generateIRIsExpr(ctx, e)

	case *ir.StateCheckExpr:
		return generateIRStateCheckExpr(ctx, e)

	case *ir.HasStateExpr:
		return generateIRHasStateExpr(ctx, e)

	case *ir.LaunchExpr:
		return generateIRLaunchExpr(ctx, e)

	case *ir.AwaitExpr:
		return generateIRAwaitExpr(ctx, e)

	case *ir.FuncRefExpr:
		return generateIRFuncRefExpr(ctx, e)

	case *ir.LambdaExpr:
		return generateIRLambdaExpr(ctx, e)

	case *ir.CallValueExpr:
		return generateIRCallValueExpr(ctx, e)

	case *ir.TypeOfExpr:
		return fmt.Sprintf("(&%s)", reflectMetaVarFor(e.Type)), typeTypeInfo, nil

	case *ir.AsExpr:
		return generateIRAsExpr(ctx, e)

	case *ir.DBRowsAsExpr:
		return generateIRDBRowsAsExpr(ctx, e)

	case *ir.NullLiteral:
		t := fromIRType(e.Type)
		if e.Type.Kind == ir.KindPointer {
			// "Указатель<T>" - не "?Тип": сам C-указатель уже нативно
			// нулевой ("T *", не has/value-структура) - "Ничто" здесь
			// буквальный NULL (см. package doc coerceExpr,
			// internal/semantic/nullable.go). <stdio.h> включается
			// безусловно (см. generateSource) - NULL гарантированно виден.
			return "NULL", t, nil
		}
		return fmt.Sprintf("%s_empty()", mangleType(t.entity)), t, nil

	case *ir.UpcastExpr:
		code, _, err := generateIRExpr(ctx, e.Value)
		if err != nil {
			return "", typeUnknown, err
		}
		t := fromIRType(e.Type)
		return fmt.Sprintf("((%s)(%s))", t.cType(ctx), code), t, nil

	case *ir.NullWrapExpr:
		code, _, err := generateIRExpr(ctx, e.Value)
		if err != nil {
			return "", typeUnknown, err
		}
		t := fromIRType(e.Type)
		return fmt.Sprintf("%s_wrap(%s)", mangleType(t.entity), code), t, nil

	case *ir.NullUnwrapExpr:
		code, _, err := generateIRExpr(ctx, e.Value)
		if err != nil {
			return "", typeUnknown, err
		}
		src := fromIRType(ir.TypeOf(e.Value))
		return fmt.Sprintf("%s_unwrap(%s)", mangleType(src.entity), code), fromIRType(e.Type), nil

	case *ir.NullCheckExpr:
		code, _, err := generateIRExpr(ctx, e.Value)
		if err != nil {
			return "", typeUnknown, err
		}
		if ir.TypeOf(e.Value).Kind == ir.KindPointer {
			// "Указатель<T> ==/!= Ничто" - прямое сравнение с NULL, не
			// has/value nullable-обёртка (см. package doc ir.NullLiteral
			// выше).
			if e.IsNot {
				return fmt.Sprintf("((%s) != NULL)", code), typeBool, nil
			}
			return fmt.Sprintf("((%s) == NULL)", code), typeBool, nil
		}
		if e.IsNot {
			return fmt.Sprintf("((%s).has)", code), typeBool, nil
		}
		return fmt.Sprintf("(!(%s).has)", code), typeBool, nil

	case *ir.ListIndexExpr:
		_, recvPtr, err := generateIRAddressableExpr(ctx, e.Receiver)
		if err != nil {
			return "", typeUnknown, err
		}
		idxCode, _, err := generateIRExpr(ctx, e.Index)
		if err != nil {
			return "", typeUnknown, err
		}
		fn := mangleType(e.CollectionName) + "_get_safe"
		return fmt.Sprintf("%s(%s, %s)", fn, recvPtr, idxCode), fromIRType(e.Type), nil

	case *ir.DictIndexExpr:
		_, recvPtr, err := generateIRAddressableExpr(ctx, e.Receiver)
		if err != nil {
			return "", typeUnknown, err
		}
		keyCode, _, err := generateIRExpr(ctx, e.Key)
		if err != nil {
			return "", typeUnknown, err
		}
		fn := mangleType(e.CollectionName) + "_get_safe"
		return fmt.Sprintf("%s(%s, %s)", fn, recvPtr, keyCode), fromIRType(e.Type), nil

	case *ir.AddressOfExpr:
		switch recv := e.Receiver.(type) {
		case *ir.Identifier:
			return fmt.Sprintf("(&%s)", varRef(ctx, recv.Name)), fromIRType(e.Type), nil
		case *ir.FieldAccess:
			code, _, err := generateIRExpr(ctx, recv)
			if err != nil {
				return "", typeUnknown, err
			}
			return fmt.Sprintf("(&%s)", code), fromIRType(e.Type), nil
		case *ir.ListElementRef:
			_, listPtr, err := generateIRAddressableExpr(ctx, recv.Receiver)
			if err != nil {
				return "", typeUnknown, err
			}
			idxCode, _, err := generateIRExpr(ctx, recv.Index)
			if err != nil {
				return "", typeUnknown, err
			}
			fn := mangleType(recv.CollectionName) + "_addr"
			return fmt.Sprintf("%s(%s, %s)", fn, listPtr, idxCode), fromIRType(e.Type), nil
		default:
			return "", typeUnknown, fmt.Errorf("генерация C из IR: неподдержанный получатель \"&\" %T", e.Receiver)
		}

	case *ir.DerefExpr:
		code, _, err := generateIRExpr(ctx, e.Value)
		if err != nil {
			return "", typeUnknown, err
		}
		return fmt.Sprintf("(*%s)", code), fromIRType(e.Type), nil

	case *ir.ListLiteralExpr:
		fn := mangleType(e.CollectionName) + "_from_literal"
		if len(e.Elems) == 0 {
			return fmt.Sprintf("%s(0, NULL)", fn), fromIRType(e.Type), nil
		}
		elemC := fromIRType(e.ElemType).cType(ctx)
		codes := make([]string, len(e.Elems))
		for i, el := range e.Elems {
			code, _, err := generateIRExpr(ctx, el)
			if err != nil {
				return "", typeUnknown, err
			}
			codes[i] = code
		}
		arr := fmt.Sprintf("(%s[]){%s}", elemC, strings.Join(codes, ", "))
		return fmt.Sprintf("%s(%dLL, %s)", fn, len(e.Elems), arr), fromIRType(e.Type), nil

	case *ir.NullCoalesceExpr:
		leftCode, _, err := generateIRExpr(ctx, e.Left)
		if err != nil {
			return "", typeUnknown, err
		}
		rightCode, _, err := generateIRExpr(ctx, e.Right)
		if err != nil {
			return "", typeUnknown, err
		}
		src := fromIRType(ir.TypeOf(e.Left))
		structName := mangleType(src.entity)
		result := fromIRType(e.Type)
		if result.kind == kindNullable {
			return fmt.Sprintf("%s_coalesce_n(%s, %s)", structName, leftCode, rightCode), result, nil
		}
		return fmt.Sprintf("%s_coalesce(%s, %s)", structName, leftCode, rightCode), result, nil

	default:
		return "", typeUnknown, fmt.Errorf("генерация C из IR: выражение %T пока не поддерживается", e)
	}
}

func generateIRUnaryExpr(ctx *irGenCtx, e *ir.UnaryExpr) (string, valueType, error) {
	code, _, err := generateIRExpr(ctx, e.Operand)
	if err != nil {
		return "", typeUnknown, err
	}
	typ := fromIRType(e.Type)
	switch e.Op {
	case "-":
		// Целое8/16/32/64, Натуральное/8/16/32/64 - явный каст обратно
		// к своему C-типу: обычные арифметические преобразования C иначе
		// оставили бы результат в более широком типе (int), не усекая/
		// оборачивая его на переполнении - тот же принцип, что и в
		// generateIRArithmetic.
		if _, _, ok := e.Type.Kind.IntWidthSigned(); ok && e.Type.Kind != ir.KindInt {
			return fmt.Sprintf("((%s)(-%s))", typ.cType(ctx), code), typ, nil
		}
		return fmt.Sprintf("(-%s)", code), typ, nil
	case "не":
		return fmt.Sprintf("(!%s)", code), typeBool, nil
	case "~":
		// Целое8/16/32/64, Натуральное/8/16/32/64 и Байт - C продвигает их
		// до int для "~" (обычные арифметические преобразования) - явный
		// каст обратно к своему C-типу, тот же приём, что и у унарного "-"
		// выше.
		if _, _, ok := e.Type.Kind.IntWidthSigned(); ok && e.Type.Kind != ir.KindInt {
			return fmt.Sprintf("((%s)(~%s))", typ.cType(ctx), code), typ, nil
		}
		if e.Type == ir.TypeByte {
			return fmt.Sprintf("((unsigned char)(~%s))", code), typeByte, nil
		}
		return fmt.Sprintf("(~%s)", code), typeInt, nil
	default:
		return "", typeUnknown, fmt.Errorf("генерация C из IR: неизвестная унарная операция %q", e.Op)
	}
}

// generateIRBinaryExpr доверяет типам, уже разрешённым internal/semantic
// (e.Type, ir.TypeOf(e.Left/e.Right)) - в отличие от cbackend.genBinaryExpr
// сам не перепроверяет допустимость операции для этих типов,
// это уже сделано семантическим анализом один раз (шаг 5.b), а не заново в
// каждом backend'е.
func generateIRBinaryExpr(ctx *irGenCtx, e *ir.BinaryExpr) (string, valueType, error) {
	lc, _, err := generateIRExpr(ctx, e.Left)
	if err != nil {
		return "", typeUnknown, err
	}
	rc, _, err := generateIRExpr(ctx, e.Right)
	if err != nil {
		return "", typeUnknown, err
	}
	lt, rt := ir.TypeOf(e.Left), ir.TypeOf(e.Right)

	switch e.Op {
	case "+":
		if lt == ir.TypeString && rt == ir.TypeString {
			ctx.usesConcat = true
			return fmt.Sprintf("rech_str_concat(%s, %s)", lc, rc), typeString, nil
		}
		return generateIRArithmetic(ctx, e.Op, lc, lt, rc, rt)

	case "-", "*", "/", "%":
		return generateIRArithmetic(ctx, e.Op, lc, lt, rc, rt)

	case "==", "!=":
		if lt == ir.TypeString && rt == ir.TypeString {
			ctx.usesStrCmp = true
			cmp := fmt.Sprintf("strcmp(%s, %s)", lc, rc)
			if e.Op == "==" {
				return fmt.Sprintf("(%s == 0)", cmp), typeBool, nil
			}
			return fmt.Sprintf("(%s != 0)", cmp), typeBool, nil
		}
		if lt.Kind == ir.KindEntity && lt == rt && !ctx.entityIsRef(lt.Entity) {
			// "структура" (значение) - поле-в-поле через сгенерированную
			// rech_struct_eq_... (struct_eq.go) - semantic
			// (structFieldsEquatable) уже проверил, что это разрешимо.
			// Ссылочная сущность/класс идёт ниже, обычным пойнтерным "==".
			registerStructEqEntity(ctx, lt.Entity)
			cmp := fmt.Sprintf("%s(%s, %s)", structEqFuncName(lt.Entity), lc, rc)
			if e.Op == "==" {
				return cmp, typeBool, nil
			}
			return fmt.Sprintf("(!%s)", cmp), typeBool, nil
		}
		return fmt.Sprintf("(%s %s %s)", lc, e.Op, rc), typeBool, nil

	case "<", ">", "<=", ">=":
		return fmt.Sprintf("(%s %s %s)", lc, e.Op, rc), typeBool, nil

	case "и", "или":
		cOp := "&&"
		if e.Op == "или" {
			cOp = "||"
		}
		return fmt.Sprintf("(%s %s %s)", lc, cOp, rc), typeBool, nil

	case "&", "|", "^", "<<", ">>":
		// Целое8/16/32/64, Натуральное/8/16/32/64 и Байт продвигаются до int
		// обычными арифметическими преобразованиями C - явный каст назад к
		// своему C-типу, тот же приём, что и у арифметики фикс-ширинных
		// типов (generateIRArithmetic). Обычное Целое (int64_t) уже
		// достаточно широкое - каста не требует.
		if _, _, ok := lt.Kind.IntWidthSigned(); ok && lt.Kind != ir.KindInt {
			vt := fromIRType(lt)
			return fmt.Sprintf("((%s)(%s %s %s))", vt.cType(ctx), lc, e.Op, rc), vt, nil
		}
		if lt == ir.TypeByte {
			return fmt.Sprintf("((unsigned char)(%s %s %s))", lc, e.Op, rc), typeByte, nil
		}
		return fmt.Sprintf("(%s %s %s)", lc, e.Op, rc), typeInt, nil

	default:
		return "", typeUnknown, fmt.Errorf("генерация C из IR: неизвестная операция %q", e.Op)
	}
}

// generateIRArithmetic - "+" (нестроковый)/"-"/"*"/"/"/"%": приведение
// операнда к double при смешении Целое/Число - та же промоция, что и в
// generateIRArithmetic.
func generateIRArithmetic(ctx *irGenCtx, op string, lc string, lt ir.Type, rc string, rt ir.Type) (string, valueType, error) {
	// Целое8/16/32/64, Натуральное/8/16/32/64 - строго между операндами
	// одного и того же Kind (internal/semantic уже это гарантировал,
	// analyzeArithmetic) - результат явно приводится к
	// C-типу самого Kind (cType), чтобы усечение/оборачивание при
	// переполнении произошло НА УРОВНЕ ВЫРАЖЕНИЯ (не только при
	// присваивании) - иначе результат промежуточного `a + b` без
	// присваивания остался бы в более широком типе после обычных
	// арифметических преобразований C. "/" - отдельный рантайм-хелпер с
	// панике на ноль (как и у Целое, rech_int_div) - rech_uint_div для
	// беззнаковых, чтобы не интерпретировать Натуральное64 > INT64_MAX как
	// отрицательное значение через rech_int_div.
	if lt == rt {
		if bits, signed, ok := lt.Kind.IntWidthSigned(); ok && lt.Kind != ir.KindInt {
			vt := fromIRType(lt)
			cType := vt.cType(ctx)
			if op == "/" || op == "%" {
				ctx.usesExceptions = true
				isMod := op == "%"
				fn := intDivModFuncName(signed, isMod, bits)
				if signed {
					ctx.intDivMod.mark(isMod, bits)
				} else {
					ctx.uintDivMod.mark(isMod, bits)
				}
				// Хелпер сам возвращает НАСТОЯЩИЙ C-тип своей ширины
				// (int8_t/.../uint64_t) - никакого внешнего "((cType)...)"
				// не нужно, в отличие от +/-/* ниже, где C сам продвигает
				// операнды до int и результат приходится усекать вручную.
				return fmt.Sprintf("%s(%s, %s)", fn, lc, rc), vt, nil
			}
			return fmt.Sprintf("((%s)(%s %s %s))", cType, lc, op, rc), vt, nil
		}
	}
	if lt == ir.TypeFloat || rt == ir.TypeFloat {
		if lt == ir.TypeInt {
			lc = fmt.Sprintf("((double)%s)", lc)
		}
		if rt == ir.TypeInt {
			rc = fmt.Sprintf("((double)%s)", rc)
		}
		// Дробное (float) - слабее Действительное (double) в numericRanks -
		// C сам продвигает float до double при смешении, явный каст не
		// нужен (в отличие от Целое выше, которому явный "(double)" нужен
		// всегда - у него нет "родного" плавающего представления).
		if op == "%" {
			ctx.usesMath = true
			return fmt.Sprintf("fmod(%s, %s)", lc, rc), typeFloat, nil
		}
		return fmt.Sprintf("(%s %s %s)", lc, op, rc), typeFloat, nil
	}
	if lt == ir.TypeFloat32 || rt == ir.TypeFloat32 {
		// Целое смешанное с Дробное - C сам продвигает int до float при
		// обычной арифметике; "%" на float/double в C не определён вовсе -
		// нужен явный fmodf, не оператор.
		if op == "%" {
			ctx.usesMath = true
			return fmt.Sprintf("fmodf(%s, %s)", lc, rc), typeFloat32, nil
		}
		return fmt.Sprintf("(%s %s %s)", lc, op, rc), typeFloat32, nil
	}
	if op == "/" || op == "%" {
		ctx.usesExceptions = true
		isMod := op == "%"
		// Обычное 64-битное "Целое" делит хелпер с "Целое64" (см. package
		// doc irGenCtx.intDivMod) - оба реально 64 бита в C, разница только
		// в спеллинге cType ("long long" против "int64_t").
		ctx.intDivMod.mark(isMod, 64)
		return fmt.Sprintf("%s(%s, %s)", intDivModFuncName(true, isMod, 64), lc, rc), typeInt, nil
	}
	return fmt.Sprintf("(%s %s %s)", lc, op, rc), typeInt, nil
}

// intDivModUsage - какие ширины/операции целочисленного деления/остатка
// реально понадобились программе (generateIRArithmetic) - по одному биту на
// комбинацию (ширина × "/" или "%"), не общий флаг на всё семейство: каждый
// rech_int_div<N>/rech_int_mod<N> - НАСТОЯЩИЙ C-тип своей ширины
// (int8_t/int16_t/int32_t/int64_t), не общий "продвинуть до 64 бит и усечь
// обратно" (в отличие от более раннего среза) - неиспользуемая static-
// функция другой ширины/операции была бы предупреждением (потенциально
// ошибкой при -Werror) у настоящего cc, значит эмитировать нужно только то,
// что реально вызвано. Обычное 64-битное "Целое" и "Целое64" (аналогично
// "Натуральное"/"Натуральное64") делят один и тот же 64-битный хелпер - оба
// реально 64 бита в C.
type intDivModUsage struct {
	div8, div16, div32, div64 bool
	mod8, mod16, mod32, mod64 bool
}

func (u *intDivModUsage) mark(isMod bool, bits int) {
	switch {
	case !isMod && bits == 8:
		u.div8 = true
	case !isMod && bits == 16:
		u.div16 = true
	case !isMod && bits == 32:
		u.div32 = true
	case !isMod && bits == 64:
		u.div64 = true
	case isMod && bits == 8:
		u.mod8 = true
	case isMod && bits == 16:
		u.mod16 = true
	case isMod && bits == 32:
		u.mod32 = true
	case isMod && bits == 64:
		u.mod64 = true
	}
}

func (u intDivModUsage) wants(isMod bool, bits int) bool {
	switch {
	case !isMod && bits == 8:
		return u.div8
	case !isMod && bits == 16:
		return u.div16
	case !isMod && bits == 32:
		return u.div32
	case !isMod && bits == 64:
		return u.div64
	case isMod && bits == 8:
		return u.mod8
	case isMod && bits == 16:
		return u.mod16
	case isMod && bits == 32:
		return u.mod32
	case isMod && bits == 64:
		return u.mod64
	}
	return false
}

func (u intDivModUsage) any() bool {
	return u.div8 || u.div16 || u.div32 || u.div64 || u.mod8 || u.mod16 || u.mod32 || u.mod64
}

// intDivModFuncName - единая точка сборки имени rech_int_div<N>/
// rech_int_mod<N>/rech_uint_div<N>/rech_uint_mod<N>, чтобы вызывающий код
// (generateIRArithmetic) и генератор текста хелпера (builtins_shared.go,
// intDivModHelperCode) не могли разойтись в написании.
func intDivModFuncName(signed bool, isMod bool, bits int) string {
	sign := "int"
	if !signed {
		sign = "uint"
	}
	kind := "div"
	if isMod {
		kind = "mod"
	}
	return fmt.Sprintf("rech_%s_%s%d", sign, kind, bits)
}

// generateIRNewExpr генерирует "новый Тип(...)" - e.Args уже позиционны по
// полю (см. package doc ir.NewExpr), имена берутся из ctx.entities[e.Entity]
// .Fields, собранных раньше в GenerateFromIR. "структура" - C99 compound
// literal прямо в месте вызова (значение, копируется дальше как обычно
// присваиванием/передачей параметра); ссылочная "сущность"/"класс" - вызов
// сгенерированного конструктора (generateIREntityCtor, GC_MALLOC), который
// возвращает указатель - тот же аргументы, но позиционные параметры функции,
// не designated initializer.
func generateIRNewExpr(ctx *irGenCtx, e *ir.NewExpr) (string, valueType, error) {
	ent := ctx.entities[e.Entity]
	argCodes := make([]string, len(e.Args))
	for i, a := range e.Args {
		code, _, err := generateIRExpr(ctx, a)
		if err != nil {
			return "", typeUnknown, err
		}
		argCodes[i] = code
	}
	if ctx.entityIsRef(e.Entity) {
		return fmt.Sprintf("%s(%s)", ctorCName(ctx, e.Entity), strings.Join(argCodes, ", ")), entityType(e.Entity), nil
	}
	parts := make([]string, len(argCodes))
	for i, code := range argCodes {
		if _, _, isFn := ctx.ffiFuncPointerSig(ent.Fields[i].Type); ent.IsExternStruct && isFn {
			// "Вызываемый(...)" полем "внешняя структура" (WNDCLASS.
			// lpfnWndProc-стиль) - тот же приём, что и у аргумента внешней
			// функции (case *ir.CallExpr выше) - {fn, env} по значению не
			// то, что ожидает C (голый указатель на функцию, тип поля уже
			// объявлен так generateIREntityStruct'ом) - marshalCallbackArg
			// подменяет code на C-имя trampoline.
			marshaled, err := ctx.marshalCallbackArg(ent.Fields[i].Type, code)
			if err != nil {
				return "", typeUnknown, err
			}
			code = marshaled
		}
		parts[i] = fmt.Sprintf(".%s = %s", mangleField(ent.Fields[i].Name), code)
	}
	initList := strings.Join(parts, ", ")
	if initList == "" {
		// Сущность без полей - зануляем техническую заглушку структуры
		// (generateIREntityStruct), а не полагаемся на "{}" (расширение
		// GNU, не входящее в стандартный C99) - как и в AST-пути.
		initList = "0"
	}
	return fmt.Sprintf("(%s){%s}", entityCName(ctx, e.Entity), initList), entityType(e.Entity), nil
}

// generateIRMethodCallExpr генерирует вызов метода - получатель по указателю
// (первый скрытый параметр this_): мутация поля через "этот.поле = ..."
// внутри метода видна вызывающему коду после возврата. Получатель ссылочной
// сущности - уже указатель сам по себе (передаётся как есть, receiverIsPointer);
// получатель "структура" (или получатель, порождённый ThisExpr) - значение,
// нужен "&", чтобы получить указатель для this_ - тот же принцип, что и у
// выбора "."/"->" в FieldAccess/FieldAssign.
// generateIRAddressableExpr генерирует receiver ОДИН раз и возвращает и
// обычный C-код значения (code), и код указателя на него (ptr) - для
// вызова метода/безопасной индексации, которым нужен указатель на
// получателя (this_/this_ у *_get_safe). Для ссылочной сущности/
// интерфейса/generic-коллекции (receiverIsPointer - истина) получателем
// годится ЛЮБОЕ выражение, включая результат другого вызова
// (`x.метод1().метод2()`) - "&" не берётся вовсе, code уже указатель. Для
// "структура" (значение) - materializeIfNeeded: если receiver уже
// адресуем (переменная/поле/"этот") - "&(code)" синтаксически валиден
// напрямую; иначе (результат вызова - rvalue, "&" от него невалиден в C) -
// материализуется временная переменная (см. package doc там же), и "&"
// берётся уже от неё. Не пересчитывает code дважды, в отличие от вызова
// generateIRExpr отдельно для каждой формы.
func generateIRAddressableExpr(ctx *irGenCtx, receiver ir.Expr) (code, ptr string, err error) {
	code, _, err = generateIRExpr(ctx, receiver)
	if err != nil {
		return "", "", err
	}
	if ctx.receiverIsPointer(receiver) {
		return code, fmt.Sprintf("(%s)", code), nil
	}
	code = ctx.materializeIfNeeded(receiver, code)
	return code, fmt.Sprintf("&(%s)", code), nil
}

func generateIRMethodCallExpr(ctx *irGenCtx, e *ir.MethodCallExpr) (string, valueType, error) {
	switch ir.TypeOf(e.Receiver).Kind {
	case ir.KindAny:
		return generateIRAnyMethodCall(ctx, e)
	case ir.KindTypeInfo:
		return generateIRTypeInfoMethodCall(ctx, e)
	}
	receiverCode, receiverPtr, err := generateIRAddressableExpr(ctx, e.Receiver)
	if err != nil {
		return "", typeUnknown, err
	}
	if len(e.RevokesStates) > 0 {
		wrapped, err := wrapRevokeStates(ctx, e.Entity, e.RevokesStates, receiverPtr)
		if err != nil {
			return "", typeUnknown, err
		}
		receiverPtr = wrapped
	}
	argCodes := make([]string, len(e.Args)+1)
	argCodes[0] = receiverPtr
	for i, a := range e.Args {
		code, _, err := generateIRExpr(ctx, a)
		if err != nil {
			return "", typeUnknown, err
		}
		argCodes[i+1] = code
	}
	if ctx.needsDispatch(e.Entity) {
		return generateDispatchCall(ctx, e, receiverCode, argCodes)
	}
	return fmt.Sprintf("%s(%s)", methodCName(ctx, e.Entity, e.Method), strings.Join(argCodes, ", ")), fromIRType(e.Type), nil
}

// generateIRStaticMethodCallExpr - "Тип::метод(...)" - как
// generateIRMethodCallExpr, но без получателя (см. package doc
// ir.StaticMethodCallExpr) - статический метод не участвует в
// диспетчеризации (needsDispatch), вызывается напрямую по мангл-имени.
func generateIRStaticMethodCallExpr(ctx *irGenCtx, e *ir.StaticMethodCallExpr) (string, valueType, error) {
	argCodes := make([]string, len(e.Args))
	for i, a := range e.Args {
		code, _, err := generateIRExpr(ctx, a)
		if err != nil {
			return "", typeUnknown, err
		}
		argCodes[i] = code
	}
	return fmt.Sprintf("%s(%s)", mangleMethod(e.Entity, e.Method), strings.Join(argCodes, ", ")), fromIRType(e.Type), nil
}

// fromIRType переводит ir.Type (backend-независимый, internal/ir) в
// valueType (C-специфичный, этот пакет) - единственное место, где IR-путь
// знает про valueType вообще, остальной generateIR* код работает через
// ir.Type/ir.TypeOf.
// irKindToValueType - ir.Kind -> valueType для всех примитивов (не
// KindEntity - у него есть переменная часть, имя сущности, см. fromIRType).
var irKindToValueType = map[ir.TypeKind]valueType{
	ir.KindByte:         typeByte,
	ir.KindInt:          typeInt,
	ir.KindFloat32:      typeFloat32,
	ir.KindFloat:        typeFloat,
	ir.KindChar:         typeChar,
	ir.KindString:       typeString,
	ir.KindBool:         typeBool,
	ir.KindFile:         typeFile,
	ir.KindSocket:       typeSocket,
	ir.KindDB:           typeDB,
	ir.KindHTTP2Session: typeHTTP2Session,
	ir.KindInt8:         typeInt8,
	ir.KindInt16:        typeInt16,
	ir.KindInt32:        typeInt32,
	ir.KindInt64:        typeInt64,
	ir.KindUint:         typeUint,
	ir.KindUint8:        typeUint8,
	ir.KindUint16:       typeUint16,
	ir.KindUint32:       typeUint32,
	ir.KindUint64:       typeUint64,
	ir.KindAny:          typeAny,
	ir.KindTypeInfo:     typeTypeInfo,
}

func fromIRType(t ir.Type) valueType {
	if t.Kind == ir.KindEntity {
		return entityType(t.Entity)
	}
	if t.Kind == ir.KindTask {
		return taskType(t.Entity)
	}
	if t.Kind == ir.KindCallable {
		return callableType(t.Entity)
	}
	if t.Kind == ir.KindExternEntity {
		return externEntityType(t.Entity)
	}
	if t.Kind == ir.KindNullable {
		return nullableType(t.Entity)
	}
	if t.Kind == ir.KindPointer {
		return pointerType(t.Entity)
	}
	if vt, ok := irKindToValueType[t.Kind]; ok {
		return vt
	}
	return typeUnknown
}
