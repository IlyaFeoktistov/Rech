// GenerateFromIR - единственный C backend Rech, транслирует
// internal/ir.Program (уже полностью проверенный internal/semantic) в
// исходник на C. Вызывается напрямую из internal/compiler.
//
// Использует общую для пакета инфраструктуру, не зависящую от IR как
// такового (mangleName/mangleVar/mangleType/mangleField/mangleMethod,
// valueType, printCall, escapeCString, formatFloatLiteral, indent,
// builtinCallCode, generic-коллекции - см. соседние файлы пакета).
package cbackend

import (
	"fmt"
	"strings"

	"github.com/IlyaFeoktistov/Rech/internal/ir"
	"github.com/IlyaFeoktistov/Rech/internal/rtmsg"
)

// irGenCtx - состояние генерации из IR: флаги использованных
// runtime-хелперов (как ctx.usesConcat/usesStrCmp),
// сущности программы по имени (нужно NewExpr - её Args позиционны, без
// имён полей, см. package doc ir.NewExpr - имена нужны только здесь, для
// designated initializer в C) и имя сущности-владельца метода, который
// сейчас генерируется ("" - вне метода, thisEntity делает ThisExpr
// типизируемым без внешнего контекста, как ctx.thisEntity в AST-пути).
// Символьной таблицы переменных здесь, в отличие от AST-пути, нет вообще -
// IR-узлы (Identifier.Type, BinaryExpr.Type) уже несут тип, разрешённый
// internal/semantic, генератору достаточно его прочитать, а не искать по
// имени - в этом и смысл шага 5 (тип решается один раз, не в каждом
// backend'е заново).
type irGenCtx struct {
	usesConcat bool
	// builtinFlags - единая таблица диспетчеризации builtinCallCode
	// (usesPanic из неё же покрывает "утверждать"/деление на ноль -
	// generic-коллекции учтены отдельно через usesGenerics/prog.Generics в
	// GenerateFromIR, не через этот флаг).
	builtinFlags
	// usesIntDiv - нужен ли rech_int_div (см. generateIRArithmetic).
	usesIntDiv bool
	// usesShow - нужны ли функции показа (rech_show_...) - генерируются
	// только если "показать" реально встретилось в программе.
	usesShow bool
	entities map[string]*ir.Entity
	// externCNames - Rech-facing имя (Alias) внешней функции -> настоящий
	// C-символ (CName), из prog.ExternFuncs: CallExpr.Callee несёт Alias,
	// вызывается по CName, не по mangleName(Alias) (тот не имеет отношения
	// к настоящему символу).
	externCNames map[string]string
	thisEntity   string
}

// GenerateFromIR возвращает исходник на C, соответствующий программе prog.
func GenerateFromIR(prog *ir.Program) (string, error) {
	if err := checkManglingCollisions(prog); err != nil {
		return "", err
	}

	entities := map[string]*ir.Entity{}
	for _, ent := range prog.Entities {
		entities[ent.Name] = ent
	}
	externCNames := map[string]string{}
	for _, ext := range prog.ExternFuncs {
		externCNames[ext.Alias] = ext.CName
	}

	var typedefs strings.Builder
	for _, ent := range prog.Entities {
		typedefs.WriteString(generateIREntityStruct(ent))
	}

	// genericsCode - typedef struct + функции для встроенных generic-
	// коллекций, использованных в программе - переиспользует
	// listCollectionCCode/stackCollectionCCode/queueCollectionCCode
	// (растущий массив) - IR не хранит собственных сигнатур методов для
	// них, MethodCallExpr уже несёт всё нужное для вызова, эта функция
	// только про C-текст самого типа.
	var genericsCode strings.Builder
	usesGenerics := len(prog.Generics) > 0
	for _, g := range prog.Generics {
		elemC := fromIRType(g.ElemType).cType()
		switch g.Template {
		case "Список":
			genericsCode.WriteString(listCollectionCCode(g.Name, elemC))
		case "Стопка":
			genericsCode.WriteString(stackCollectionCCode(g.Name, elemC))
		case "Очередь":
			genericsCode.WriteString(queueCollectionCCode(g.Name, elemC))
		default:
			return "", fmt.Errorf("неизвестный generic-шаблон %q", g.Template)
		}
	}

	// showFuncs - функции показа ("показать") для каждой сущности и
	// generic-коллекции программы - строятся всегда (дёшево, чистый текст),
	// в итоговый файл попадают только если ctx.usesShow (см. ниже, после
	// генерации тел).
	var showFuncs strings.Builder
	for _, ent := range prog.Entities {
		showFuncs.WriteString(generateIREntityShowFunc(ent))
	}
	for _, g := range prog.Generics {
		showFuncs.WriteString(generateIRGenericShowFunc(g))
	}

	var protos strings.Builder
	for _, ext := range prog.ExternFuncs {
		protos.WriteString(externPrototypeOfIR(ext))
	}
	for _, ent := range prog.Entities {
		for _, m := range ent.Methods {
			fmt.Fprintf(&protos, "%s %s(%s);\n", cReturnTypeFor(m.HasReturn, m.ReturnType), mangleMethod(ent.Name, m.Name), cMethodParamListIR(ent.Name, m.Params))
		}
	}
	hasEntryPoint := false
	for _, fn := range prog.Functions {
		if fn.Name == entryPointName {
			hasEntryPoint = true
			continue
		}
		fmt.Fprintf(&protos, "%s %s(%s);\n", cReturnTypeFor(fn.HasReturn, fn.ReturnType), mangleName(fn.Name), cParamListIR(fn.Params))
	}
	if !hasEntryPoint {
		return "", fmt.Errorf("в программе не найдена функция %q - она служит точкой входа", entryPointName)
	}

	var body strings.Builder
	ctx := &irGenCtx{entities: entities, externCNames: externCNames}
	for _, ent := range prog.Entities {
		for _, m := range ent.Methods {
			mCode, err := generateIRMethod(ctx, ent.Name, m)
			if err != nil {
				return "", fmt.Errorf("сущность %q: метод %q: %w", ent.Name, m.Name, err)
			}
			body.WriteString(mCode)
		}
	}
	for _, fn := range prog.Functions {
		fnCode, err := generateIRFunction(ctx, fn)
		if err != nil {
			return "", fmt.Errorf("функция %q: %w", fn.Name, err)
		}
		body.WriteString(fnCode)
	}

	var sb strings.Builder
	sb.WriteString("// Автоматически сгенерировано компилятором rech. Не редактировать руками.\n")
	sb.WriteString("#include <stdio.h>\n")
	sb.WriteString("#include <stdbool.h>\n")
	if ctx.usesConcat || ctx.usesStrCmp || ctx.usesConsoleInput || ctx.usesFileIO || ctx.usesSockets {
		sb.WriteString("#include <string.h>\n")
	}
	if ctx.usesConcat || usesGenerics || ctx.usesPanic || ctx.usesConsoleInput || ctx.usesFileIO {
		sb.WriteString("#include <stdlib.h>\n")
	}
	if ctx.usesMath {
		sb.WriteString("#include <math.h>\n")
	}
	// windows.h/shellapi.h - rech_init_console/rech_init_args нужны
	// безусловно, файловый ввод-вывод - за MultiByteToWideChar.
	// WIN32_LEAN_AND_MEAN/winsock2.h - нужны при usesSockets.
	sb.WriteString("#ifdef _WIN32\n#define WIN32_LEAN_AND_MEAN\n#include <windows.h>\n#include <shellapi.h>\n#endif\n")
	if ctx.usesSockets {
		sb.WriteString("#ifdef _WIN32\n#include <winsock2.h>\n#include <ws2tcpip.h>\n#else\n#include <sys/socket.h>\n#include <netinet/in.h>\n#include <arpa/inet.h>\n#include <netdb.h>\n#include <unistd.h>\n#endif\n")
	}
	sb.WriteString("\n")
	sb.WriteString(consoleInitHelper)
	sb.WriteString("\n")
	sb.WriteString(argsGlobals)
	sb.WriteString("\n")
	if ctx.usesConcat {
		sb.WriteString(strConcatHelper)
		sb.WriteString("\n")
	}
	if usesGenerics || ctx.usesPanic {
		sb.WriteString(panicHelper)
		sb.WriteString("\n")
	}
	if ctx.usesArgs {
		sb.WriteString(argFuncHelper)
		sb.WriteString("\n")
	}
	if ctx.usesIntDiv {
		sb.WriteString(intDivHelper)
		sb.WriteString("\n")
	}
	if ctx.usesConsoleInput {
		sb.WriteString(readHelpers)
		sb.WriteString("\n")
	}
	if ctx.usesFileIO {
		sb.WriteString(fileHelpers)
		sb.WriteString("\n")
	}
	if ctx.usesSockets {
		sb.WriteString(socketHelpers)
		sb.WriteString("\n")
	}
	if typedefs.Len() > 0 {
		sb.WriteString(typedefs.String())
	}
	if genericsCode.Len() > 0 {
		sb.WriteString(genericsCode.String())
	}
	if ctx.usesShow && showFuncs.Len() > 0 {
		sb.WriteString(showFuncs.String())
	}
	if ctx.usesHTTP {
		// После typedefs - rech_http_request принимает/возвращает Запрос/
		// Ответ по значению, их typedef должен быть уже виден.
		sb.WriteString(httpHelpers)
		sb.WriteString("\n")
	}
	if protos.Len() > 0 {
		sb.WriteString(protos.String())
		sb.WriteString("\n")
	}
	sb.WriteString(body.String())
	return sb.String(), nil
}

// generateIREntityStruct - C typedef struct для ent, как
// C typedef struct для ent, по уже собранным internal/ir.Field.
func generateIREntityStruct(ent *ir.Entity) string {
	var sb strings.Builder
	sb.WriteString("typedef struct {\n")
	if len(ent.Fields) == 0 {
		sb.WriteString("    char _rech_empty;\n")
	}
	for _, f := range ent.Fields {
		fmt.Fprintf(&sb, "    %s %s;\n", fromIRType(f.Type).cType(), mangleField(f.Name))
	}
	fmt.Fprintf(&sb, "} %s;\n\n", mangleType(ent.Name))
	return sb.String()
}

// cReturnTypeFor - C-тип результата функции/метода: сама Type, если он
// объявлен (hasReturn), иначе "int" - технический 0 в конце (не значение
// "результата"), чтобы int-функция не осталась без return (для строгого
// C - undefined behavior), как и в AST-пути.
func cReturnTypeFor(hasReturn bool, returnType ir.Type) string {
	if hasReturn {
		return fromIRType(returnType).cType()
	}
	return "int"
}

func cParamListIR(params []ir.Param) string {
	if len(params) == 0 {
		return "void"
	}
	parts := make([]string, len(params))
	for i, p := range params {
		parts[i] = fmt.Sprintf("%s %s", fromIRType(p.Type).cType(), mangleVar(p.Name))
	}
	return strings.Join(parts, ", ")
}

// externPrototypeOfIR - "extern <тип> <CName>(<типы>);" для внешней
// функции - без имён параметров (в отличие от cbackend.externPrototypeOf,
// C допускает анонимные параметры в прототипе, а сами имена
// параметров Rech после семантического анализа уже не нужны ни для чего,
// кроме сообщений об ошибках, которые уже показаны на этапе Analyze.
func externPrototypeOfIR(ext *ir.ExternFuncDecl) string {
	cReturnType := "void"
	if ext.HasReturn {
		cReturnType = fromIRType(ext.ReturnType).cType()
	}
	types := make([]string, len(ext.Params))
	for i, p := range ext.Params {
		types[i] = fromIRType(p).cType()
	}
	return fmt.Sprintf("extern %s %s(%s);\n", cReturnType, ext.CName, strings.Join(types, ", "))
}

// cMethodParamListIR - как cParamListIR, но с обязательным скрытым первым
// параметром "Тип *this_" (получатель по указателю, как и в AST-пути) -
// у метода он есть всегда, поэтому в отличие от cParamListIR/
// combineParamList не нужен отдельный случай "нет параметров -> void".
func cMethodParamListIR(entityName string, params []ir.Param) string {
	parts := make([]string, len(params)+1)
	parts[0] = fmt.Sprintf("%s *this_", mangleType(entityName))
	for i, p := range params {
		parts[i+1] = fmt.Sprintf("%s %s", fromIRType(p.Type).cType(), mangleVar(p.Name))
	}
	return strings.Join(parts, ", ")
}

func generateIRFunction(ctx *irGenCtx, fn *ir.Function) (string, error) {
	var sb strings.Builder
	fmt.Fprintf(&sb, "// %s\n", fn.Name)
	cName := mangleName(fn.Name)
	paramList := cParamListIR(fn.Params)
	if cName == "main" {
		// "главная" всегда получает настоящую C-сигнатуру main(int argc,
		// char **argv) - как и в AST-пути (generateCallable) - количество_
		// аргументов/аргумент читают их из глобальных rech_argc/rech_argv
		// (argsGlobals), вызываемых из любой функции программы, не только
		// из главная.
		paramList = "int argc, char **argv"
	}
	fmt.Fprintf(&sb, "%s %s(%s) {\n", cReturnTypeFor(fn.HasReturn, fn.ReturnType), cName, paramList)
	if cName == "main" {
		sb.WriteString("    rech_init_console();\n")
		sb.WriteString("    rech_init_args(argc, argv);\n")
	}
	for _, s := range fn.Body {
		if err := generateIRStmt(&sb, ctx, s, 1); err != nil {
			return "", err
		}
	}
	if !fn.HasReturn {
		sb.WriteString("    return 0;\n")
	}
	sb.WriteString("}\n\n")
	return sb.String(), nil
}

// generateIRMethod - как generateIRFunction, но со скрытым параметром
// "Тип *this_" и ctx.thisEntity, включённым на время генерации тела - то,
// что делает ThisExpr типизируемым (generateIRExpr, case *ir.ThisExpr),
// как и ctx.thisEntity в AST-пути (generateMethod).
func generateIRMethod(ctx *irGenCtx, entityName string, m *ir.Method) (string, error) {
	var sb strings.Builder
	fmt.Fprintf(&sb, "// %s.%s\n", entityName, m.Name)
	fmt.Fprintf(&sb, "%s %s(%s) {\n", cReturnTypeFor(m.HasReturn, m.ReturnType), mangleMethod(entityName, m.Name), cMethodParamListIR(entityName, m.Params))
	ctx.thisEntity = entityName
	defer func() { ctx.thisEntity = "" }()
	for _, s := range m.Body {
		if err := generateIRStmt(&sb, ctx, s, 1); err != nil {
			return "", err
		}
	}
	if !m.HasReturn {
		sb.WriteString("    return 0;\n")
	}
	sb.WriteString("}\n\n")
	return sb.String(), nil
}

func generateIRStmt(sb *strings.Builder, ctx *irGenCtx, s ir.Stmt, depth int) error {
	ind := indent(depth)
	switch s := s.(type) {
	case *ir.PrintStmt:
		code, typ, err := generateIRExpr(ctx, s.Value)
		if err != nil {
			return err
		}
		fmt.Fprintf(sb, "%s%s\n", ind, printCall(typ, code))
		return nil

	case *ir.ShowStmt:
		ctx.usesShow = true
		code, _, err := generateIRExpr(ctx, s.Value)
		if err != nil {
			return err
		}
		fmt.Fprintf(sb, "%s%s;\n", ind, generateIRShowExpr(ir.TypeOf(s.Value), code))
		fmt.Fprintf(sb, "%sprintf(\"\\n\");\n", ind)
		return nil

	case *ir.VarDecl:
		code, typ, err := generateIRExpr(ctx, s.Value)
		if err != nil {
			return err
		}
		fmt.Fprintf(sb, "%s%s %s = %s;\n", ind, typ.cType(), mangleVar(s.Name), code)
		return nil

	case *ir.Assign:
		code, _, err := generateIRExpr(ctx, s.Value)
		if err != nil {
			return err
		}
		fmt.Fprintf(sb, "%s%s = %s;\n", ind, mangleVar(s.Name), code)
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

	case *ir.WhileStmt:
		cond, _, err := generateIRExpr(ctx, s.Cond)
		if err != nil {
			return err
		}
		fmt.Fprintf(sb, "%swhile (%s) {\n", ind, cond)
		for _, st := range s.Body {
			if err := generateIRStmt(sb, ctx, st, depth+1); err != nil {
				return err
			}
		}
		fmt.Fprintf(sb, "%s}\n", ind)
		return nil

	case *ir.ReturnStmt:
		if s.Value == nil {
			fmt.Fprintf(sb, "%sreturn 0;\n", ind)
			return nil
		}
		code, _, err := generateIRExpr(ctx, s.Value)
		if err != nil {
			return err
		}
		fmt.Fprintf(sb, "%sreturn %s;\n", ind, code)
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
		fmt.Fprintf(sb, "%s(%s).%s = %s;\n", ind, receiverCode, mangleField(s.Field), code)
		return nil

	case *ir.BecomeStmt:
		// Шаг 4.1, упрощённая версия: состояние уже проверено семантическим
		// анализом как объявленное - typestate чисто компилятивный на этом
		// шаге, "становится" не эмитит C-код, как и в AST-пути.
		return nil

	case *ir.AssertStmt:
		cond, _, err := generateIRExpr(ctx, s.Cond)
		if err != nil {
			return err
		}
		ctx.usesPanic = true
		fmt.Fprintf(sb, "%sif (!(%s)) { rech_panic(\"%s\"); }\n", ind, cond, escapeCString(rtmsg.AssertFailed(s.Line)))
		return nil

	case *ir.ExprStmt:
		var call ir.Expr = s.Call
		switch {
		case s.MethodCall != nil:
			call = s.MethodCall
		case s.IncDec != nil:
			call = s.IncDec
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
		for _, st := range s.Body {
			if err := generateIRStmt(sb, ctx, st, depth+1); err != nil {
				return err
			}
		}
		fmt.Fprintf(sb, "%s}\n", ind)
		return nil

	case *ir.ForEachStmt:
		collCode, collType, err := generateIRExpr(ctx, s.Collection)
		if err != nil {
			return err
		}
		// "для <строка> в <Файл>" - потоковое построчное чтение, отдельная
		// форма от индексированного Список<T> ниже - файл не известной
		// заранее длины (тот же принцип, что и в cbackend.generateStmt,
		// AST-путь).
		if collType == typeFile {
			ctx.usesFileIO = true
			ctx.usesPanic = true
			varC := mangleVar(s.Var)
			okC := "rech_fe_ok_" + varC
			fmt.Fprintf(sb, "%sfor (;;) {\n", ind)
			fmt.Fprintf(sb, "%sbool %s = false;\n", indent(depth+1), okC)
			fmt.Fprintf(sb, "%sconst char *%s = rech_read_line_from_file(%s, &%s);\n", indent(depth+1), varC, collCode, okC)
			fmt.Fprintf(sb, "%sif (!%s) { break; }\n", indent(depth+1), okC)
			for _, st := range s.Body {
				if err := generateIRStmt(sb, ctx, st, depth+1); err != nil {
					return err
				}
			}
			fmt.Fprintf(sb, "%s}\n", ind)
			return nil
		}
		varC := mangleVar(s.Var)
		idxC := "rech_fe_i_" + varC
		fmt.Fprintf(sb, "%sfor (long long %s = 0; %s < %s.len; %s++) {\n", ind, idxC, idxC, collCode, idxC)
		fmt.Fprintf(sb, "%s%s %s = %s.data[%s];\n", indent(depth+1), fromIRType(s.ElemType).cType(), varC, collCode, idxC)
		for _, st := range s.Body {
			if err := generateIRStmt(sb, ctx, st, depth+1); err != nil {
				return err
			}
		}
		fmt.Fprintf(sb, "%s}\n", ind)
		return nil

	default:
		return fmt.Errorf("генерация C из IR: инструкция %T пока не поддерживается", s)
	}
}

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
		return mangleVar(e.Name), fromIRType(e.Type), nil

	case *ir.UnaryExpr:
		return generateIRUnaryExpr(ctx, e)

	case *ir.IncDecExpr:
		name := mangleVar(e.Name)
		if e.Prefix {
			return fmt.Sprintf("(%s%s)", e.Op, name), fromIRType(e.Type), nil
		}
		return fmt.Sprintf("(%s%s)", name, e.Op), fromIRType(e.Type), nil

	case *ir.BinaryExpr:
		return generateIRBinaryExpr(ctx, e)

	case *ir.CallExpr:
		argCodes := make([]string, len(e.Args))
		for i, a := range e.Args {
			code, _, err := generateIRExpr(ctx, a)
			if err != nil {
				return "", typeUnknown, err
			}
			argCodes[i] = code
		}
		// Порядок проверки - тот же, что и в cbackend.genCall (AST-путь):
		// builtin (inline C-выражение) -> внешняя функция (настоящий
		// C-символ, не mangleName) -> обычная функция программы.
		if code, ok := builtinCallCode(&ctx.builtinFlags, e.Callee, argCodes); ok {
			return code, fromIRType(e.Type), nil
		}
		callee := mangleName(e.Callee)
		if cName, ok := ctx.externCNames[e.Callee]; ok {
			callee = cName
		}
		return fmt.Sprintf("%s(%s)", callee, strings.Join(argCodes, ", ")), fromIRType(e.Type), nil

	case *ir.NewExpr:
		return generateIRNewExpr(ctx, e)

	case *ir.FieldAccess:
		code, _, err := generateIRExpr(ctx, e.Receiver)
		if err != nil {
			return "", typeUnknown, err
		}
		return fmt.Sprintf("(%s).%s", code, mangleField(e.Field)), fromIRType(e.Type), nil

	case *ir.ThisExpr:
		// "(*this_)" - разыменованный указатель this_ ведёт себя как
		// lvalue сущностного типа, как и в AST-пути (genExpr, case
		// *ast.ThisExpr) - тем же способом обходится без отдельной ветки
		// на "->" в FieldAccess/FieldAssign.
		return "(*this_)", entityType(e.Entity), nil

	case *ir.MethodCallExpr:
		return generateIRMethodCallExpr(ctx, e)

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
		return fmt.Sprintf("(-%s)", code), typ, nil
	case "не":
		return fmt.Sprintf("(!%s)", code), typeBool, nil
	case "~":
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

	case "-", "*", "/":
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
		return fmt.Sprintf("(%s %s %s)", lc, e.Op, rc), typeInt, nil

	default:
		return "", typeUnknown, fmt.Errorf("генерация C из IR: неизвестная операция %q", e.Op)
	}
}

// generateIRArithmetic - "+" (нестроковый)/"-"/"*"/"/": приведение
// операнда к double при смешении Целое/Число - та же промоция, что и в
// generateIRArithmetic.
func generateIRArithmetic(ctx *irGenCtx, op string, lc string, lt ir.Type, rc string, rt ir.Type) (string, valueType, error) {
	if lt == ir.TypeFloat || rt == ir.TypeFloat {
		if lt == ir.TypeInt {
			lc = fmt.Sprintf("((double)%s)", lc)
		}
		if rt == ir.TypeInt {
			rc = fmt.Sprintf("((double)%s)", rc)
		}
		return fmt.Sprintf("(%s %s %s)", lc, op, rc), typeFloat, nil
	}
	if op == "/" {
		ctx.usesPanic = true
		ctx.usesIntDiv = true
		return fmt.Sprintf("rech_int_div(%s, %s)", lc, rc), typeInt, nil
	}
	return fmt.Sprintf("(%s %s %s)", lc, op, rc), typeInt, nil
}

// generateIRNewExpr генерирует C99 compound literal для "новый Тип(...)" -
// e.Args уже позиционны по полю (см. package doc ir.NewExpr), но без имён
// полей - для designated initializer ("."+имя+"="+значение) имена берутся
// из ctx.entities[e.Entity].Fields, собранных раньше в GenerateFromIR.
func generateIRNewExpr(ctx *irGenCtx, e *ir.NewExpr) (string, valueType, error) {
	ent := ctx.entities[e.Entity]
	parts := make([]string, len(e.Args))
	for i, a := range e.Args {
		code, _, err := generateIRExpr(ctx, a)
		if err != nil {
			return "", typeUnknown, err
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
	return fmt.Sprintf("(%s){%s}", mangleType(e.Entity), initList), entityType(e.Entity), nil
}

// generateIRMethodCallExpr генерирует вызов метода - получатель по адресу
// (первый скрытый параметр this_), как и в AST-пути (genMethodCall):
// мутация поля через "этот.поле = ..." внутри метода видна вызывающему
// коду после возврата.
func generateIRMethodCallExpr(ctx *irGenCtx, e *ir.MethodCallExpr) (string, valueType, error) {
	receiverCode, _, err := generateIRExpr(ctx, e.Receiver)
	if err != nil {
		return "", typeUnknown, err
	}
	argCodes := make([]string, len(e.Args)+1)
	argCodes[0] = fmt.Sprintf("&(%s)", receiverCode)
	for i, a := range e.Args {
		code, _, err := generateIRExpr(ctx, a)
		if err != nil {
			return "", typeUnknown, err
		}
		argCodes[i+1] = code
	}
	return fmt.Sprintf("%s(%s)", mangleMethod(e.Entity, e.Method), strings.Join(argCodes, ", ")), fromIRType(e.Type), nil
}

// fromIRType переводит ir.Type (backend-независимый, internal/ir) в
// valueType (C-специфичный, этот пакет) - единственное место, где IR-путь
// знает про valueType вообще, остальной generateIR* код работает через
// ir.Type/ir.TypeOf.
func fromIRType(t ir.Type) valueType {
	switch t.Kind {
	case ir.KindString:
		return typeString
	case ir.KindInt:
		return typeInt
	case ir.KindFloat:
		return typeFloat
	case ir.KindBool:
		return typeBool
	case ir.KindFile:
		return typeFile
	case ir.KindSocket:
		return typeSocket
	case ir.KindEntity:
		return entityType(t.Entity)
	default:
		return typeUnknown
	}
}
