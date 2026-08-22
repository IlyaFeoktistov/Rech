// GenerateFromIR - C backend поверх internal/ir (шаг 5.c) - параллельный
// путь к Generate(*ast.Program): пока не встроен в реальную компиляцию
// (internal/compiler по-прежнему вызывает Generate напрямую), покрывает
// ровно то же подмножество, что и internal/semantic.Analyze на данный
// момент - шаги 0-3 (`вывести <строка>`, переменные/арифметика,
// если/пока/для, функции с параметрами/типом результата и их вызов).
// Расширяется вместе с покрытием internal/semantic,
// вертикальным срезом за срезом (см. package doc internal/ir), а не
// отдельно от него - не имеет смысла генерировать код для IR-узла, которого
// семантический анализ ещё не производит.
//
// Переиспользует то, что уже есть в этом пакете для генерации из AST
// (entryPointName, mangleName, mangleVar, valueType, printCall,
// escapeCString, formatFloatLiteral, indent) - IR-путь не имеет
// собственного понятия C-типа/мангла, это решение backend'а в обоих
// случаях одно и то же.
package cbackend

import (
	"fmt"
	"strings"

	"github.com/IlyaFeoktistov/Rech/internal/ir"
)

// irGenCtx - состояние генерации из IR: флаги использованных
// runtime-хелперов (как ctx.usesConcat/usesStrCmp в genCtx, AST-путь).
// Символьной таблицы здесь, в отличие от AST-пути, нет вообще - IR-узлы
// (Identifier.Type, BinaryExpr.Type) уже несут тип, разрешённый
// internal/semantic, генератору достаточно его прочитать, а не искать по
// имени - в этом и смысл шага 5 (тип решается один раз, не в каждом
// backend'е заново).
type irGenCtx struct {
	usesConcat bool
	usesStrCmp bool
}

// GenerateFromIR возвращает исходник на C, соответствующий программе prog.
func GenerateFromIR(prog *ir.Program) (string, error) {
	hasEntryPoint := false
	var protos strings.Builder
	for _, fn := range prog.Functions {
		if fn.Name == entryPointName {
			hasEntryPoint = true
			continue
		}
		fmt.Fprintf(&protos, "%s %s(%s);\n", cReturnTypeIR(fn), mangleName(fn.Name), cParamListIR(fn.Params))
	}
	if !hasEntryPoint {
		return "", fmt.Errorf("в программе не найдена функция %q - она служит точкой входа", entryPointName)
	}

	var body strings.Builder
	ctx := &irGenCtx{}
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
	if ctx.usesConcat || ctx.usesStrCmp {
		sb.WriteString("#include <string.h>\n")
	}
	sb.WriteString("\n")
	if ctx.usesConcat {
		sb.WriteString(strConcatHelper)
		sb.WriteString("\n")
	}
	if protos.Len() > 0 {
		sb.WriteString(protos.String())
		sb.WriteString("\n")
	}
	sb.WriteString(body.String())
	return sb.String(), nil
}

// cReturnTypeIR - C-тип результата функции: сама Type, если у функции
// объявлен тип результата (fn.HasReturn), иначе "int" - технический 0 в
// конце (не значение "результата"), чтобы int-функция не осталась без
// return (для строгого C - undefined behavior), как и в AST-пути.
func cReturnTypeIR(fn *ir.Function) string {
	if fn.HasReturn {
		return fromIRType(fn.ReturnType).cType()
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

func generateIRFunction(ctx *irGenCtx, fn *ir.Function) (string, error) {
	var sb strings.Builder
	fmt.Fprintf(&sb, "// %s\n", fn.Name)
	fmt.Fprintf(&sb, "%s %s(%s) {\n", cReturnTypeIR(fn), mangleName(fn.Name), cParamListIR(fn.Params))
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

	case *ir.ExprStmt:
		code, _, err := generateIRExpr(ctx, s.Call)
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
		return fmt.Sprintf("%s(%s)", mangleName(e.Callee), strings.Join(argCodes, ", ")), fromIRType(e.Type), nil

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
	default:
		return "", typeUnknown, fmt.Errorf("генерация C из IR: неизвестная унарная операция %q", e.Op)
	}
}

// generateIRBinaryExpr доверяет типам, уже разрешённым internal/semantic
// (e.Type, ir.TypeOf(e.Left/e.Right)) - в отличие от cbackend.genBinaryExpr
// (AST-путь), сам не перепроверяет допустимость операции для этих типов,
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
		return generateIRArithmetic(e.Op, lc, lt, rc, rt)

	case "-", "*", "/":
		return generateIRArithmetic(e.Op, lc, lt, rc, rt)

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

	default:
		return "", typeUnknown, fmt.Errorf("генерация C из IR: неизвестная операция %q", e.Op)
	}
}

// generateIRArithmetic - "+" (нестроковый)/"-"/"*"/"/": приведение
// операнда к double при смешении Целое/Число - та же промоция, что и в
// cbackend.genArithmetic (AST-путь).
func generateIRArithmetic(op string, lc string, lt ir.Type, rc string, rt ir.Type) (string, valueType, error) {
	if lt == ir.TypeFloat || rt == ir.TypeFloat {
		if lt == ir.TypeInt {
			lc = fmt.Sprintf("((double)%s)", lc)
		}
		if rt == ir.TypeInt {
			rc = fmt.Sprintf("((double)%s)", rc)
		}
		return fmt.Sprintf("(%s %s %s)", lc, op, rc), typeFloat, nil
	}
	return fmt.Sprintf("(%s %s %s)", lc, op, rc), typeInt, nil
}

// fromIRType переводит ir.Type (backend-независимый, internal/ir) в
// valueType (C-специфичный, этот пакет) - единственное место, где IR-путь
// знает про valueType вообще, остальной generateIR* код работает через
// ir.Type/ir.TypeOf.
func fromIRType(t ir.Type) valueType {
	switch t {
	case ir.TypeString:
		return typeString
	case ir.TypeInt:
		return typeInt
	case ir.TypeFloat:
		return typeFloat
	case ir.TypeBool:
		return typeBool
	default:
		return typeUnknown
	}
}
