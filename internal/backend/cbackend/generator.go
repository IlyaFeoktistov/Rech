// Package cbackend транслирует ast.Program в исходник на C.
//
// Это первый и на сегодня единственный backend Rech. По архитектуре из
// docs/ROADMAP.md он не единственный на будущее: AST специально не содержит
// ничего C-специфичного, чтобы позже рядом можно было добавить interpreter
// и VM backend без изменения lexer/parser/ast.
//
// Типы (valueType) выводятся здесь же, прямо во время генерации кода -
// простой literal-based inference по docs/ROADMAP.md, шаг 1. Настоящая
// проверка типов на отдельном IR-слое появится на шаге 5 ("Semantic
// analysis"); до тех пор эта логика намеренно живёт в backend'е, а не в AST.
package cbackend

import (
	"fmt"
	"slices"
	"strconv"
	"strings"

	"github.com/IlyaFeoktistov/Rech/internal/ast"
)

// entryPointName - функция с этим именем становится точкой входа программы
// (C main). Остальные имена транслитерируются в валидные C-идентификаторы.
const entryPointName = "главная"

// translit - таблица транслитерации кириллицы для имён функций и переменных.
// Не претендует на лингвистическую точность (это не морфология, см.
// docs/ROADMAP.md), задача - дать читаемый и уникальный C-идентификатор.
var translit = map[rune]string{
	'а': "a", 'б': "b", 'в': "v", 'г': "g", 'д': "d", 'е': "e", 'ё': "e",
	'ж': "zh", 'з': "z", 'и': "i", 'й': "j", 'к': "k", 'л': "l", 'м': "m",
	'н': "n", 'о': "o", 'п': "p", 'р': "r", 'с': "s", 'т': "t", 'у': "u",
	'ф': "f", 'х': "h", 'ц': "c", 'ч': "ch", 'ш': "sh", 'щ': "sch", 'ъ': "",
	'ы': "y", 'ь': "", 'э': "e", 'ю': "yu", 'я': "ya",
}

func transliterate(name string) string {
	var sb strings.Builder
	for _, r := range strings.ToLower(name) {
		if lat, ok := translit[r]; ok {
			sb.WriteString(lat)
			continue
		}
		sb.WriteRune(r)
	}
	return sb.String()
}

func mangleName(name string) string {
	if name == entryPointName {
		return "main"
	}
	return "rech_" + transliterate(name)
}

func mangleVar(name string) string {
	return "v_" + transliterate(name)
}

// valueType - тип значения выражения, выведенный во время генерации C-кода.
type valueType int

const (
	typeUnknown valueType = iota
	typeString
	typeInt
	typeFloat
	typeBool
)

func (t valueType) String() string {
	switch t {
	case typeString:
		return "Строка"
	case typeInt:
		return "Целое"
	case typeFloat:
		return "Число"
	case typeBool:
		return "Логическое"
	default:
		return "неизвестный тип"
	}
}

func (t valueType) cType() string {
	switch t {
	case typeString:
		return "const char *"
	case typeInt:
		return "long long"
	case typeFloat:
		return "double"
	case typeBool:
		return "bool"
	default:
		return "void *"
	}
}

// genCtx - состояние генерации одной функции: стек областей видимости
// (шаг 2 - если/иначе/пока/для вводят вложенные блоки, как в C) и флаги
// использованных runtime-хелперов (влияют на пролог файла).
type genCtx struct {
	scopes     []map[string]valueType
	usesConcat bool
	usesStrCmp bool
}

func (ctx *genCtx) pushScope() {
	ctx.scopes = append(ctx.scopes, map[string]valueType{})
}

func (ctx *genCtx) popScope() {
	ctx.scopes = ctx.scopes[:len(ctx.scopes)-1]
}

// declare добавляет переменную в текущую (самую внутреннюю) область
// видимости. Возвращает false, если переменная с этим именем уже объявлена
// в этой же области - повторное объявление того же имени во вложенном
// блоке разрешено (затенение, как в C).
func (ctx *genCtx) declare(name string, typ valueType) bool {
	top := ctx.scopes[len(ctx.scopes)-1]
	if _, exists := top[name]; exists {
		return false
	}
	top[name] = typ
	return true
}

// lookup ищет переменную от самой внутренней области видимости к внешним.
func (ctx *genCtx) lookup(name string) (valueType, bool) {
	for _, scope := range slices.Backward(ctx.scopes) {
		if typ, ok := scope[name]; ok {
			return typ, true
		}
	}
	return typeUnknown, false
}

func indent(depth int) string {
	return strings.Repeat("    ", depth)
}

// Generate возвращает исходник на C, соответствующий программе prog.
func Generate(prog *ast.Program) (string, error) {
	var body strings.Builder
	ctx := &genCtx{}

	hasEntryPoint := false
	for _, fn := range prog.Functions {
		if fn.Name == entryPointName {
			hasEntryPoint = true
		}
		if err := generateFunction(&body, ctx, fn); err != nil {
			return "", err
		}
	}
	if !hasEntryPoint {
		return "", fmt.Errorf("в программе не найдена функция %q - она служит точкой входа", entryPointName)
	}

	var sb strings.Builder
	sb.WriteString("// Автоматически сгенерировано компилятором rech. Не редактировать руками.\n")
	sb.WriteString("#include <stdio.h>\n")
	sb.WriteString("#include <stdbool.h>\n")
	if ctx.usesConcat || ctx.usesStrCmp {
		sb.WriteString("#include <string.h>\n")
	}
	if ctx.usesConcat {
		sb.WriteString("#include <stdlib.h>\n")
	}
	sb.WriteString("\n")
	if ctx.usesConcat {
		sb.WriteString(strConcatHelper)
		sb.WriteString("\n")
	}
	sb.WriteString(body.String())
	return sb.String(), nil
}

const strConcatHelper = `static char *rech_str_concat(const char *a, const char *b) {
    size_t la = strlen(a), lb = strlen(b);
    char *out = malloc(la + lb + 1);
    memcpy(out, a, la);
    memcpy(out + la, b, lb);
    out[la + lb] = '\0';
    return out;
}
`

func generateFunction(sb *strings.Builder, ctx *genCtx, fn *ast.FunctionDecl) error {
	cName := mangleName(fn.Name)
	ctx.scopes = nil
	ctx.pushScope()

	fmt.Fprintf(sb, "// %s\n", fn.Name)
	fmt.Fprintf(sb, "int %s(void) {\n", cName)
	for _, stmt := range fn.Body {
		if err := generateStmt(sb, ctx, stmt, 1); err != nil {
			return fmt.Errorf("функция %q: %w", fn.Name, err)
		}
	}
	// Функции в v0.1 не имеют объявленного типа результата (шаг 3), поэтому
	// 0 в конце - не значение "результата", а просто способ не оставлять
	// int-функцию без return (что для строгого C - undefined behavior).
	sb.WriteString("    return 0;\n")
	sb.WriteString("}\n\n")
	return nil
}

func generateStmt(sb *strings.Builder, ctx *genCtx, stmt ast.Stmt, depth int) error {
	ind := indent(depth)
	switch s := stmt.(type) {
	case *ast.PrintStmt:
		code, typ, err := genExpr(ctx, s.Value)
		if err != nil {
			return err
		}
		fmt.Fprintf(sb, "%s%s\n", ind, printCall(typ, code))
		return nil

	case *ast.VarDecl:
		code, typ, err := genExpr(ctx, s.Value)
		if err != nil {
			return err
		}
		if !ctx.declare(s.Name, typ) {
			return fmt.Errorf("строка %d: переменная %q уже объявлена в этой области видимости", s.Line, s.Name)
		}
		fmt.Fprintf(sb, "%s%s %s = %s;\n", ind, typ.cType(), mangleVar(s.Name), code)
		return nil

	case *ast.Assign:
		declared, exists := ctx.lookup(s.Name)
		if !exists {
			return fmt.Errorf("строка %d: переменная %q не объявлена - используйте \"переменная\" для объявления", s.Line, s.Name)
		}
		code, typ, err := genExpr(ctx, s.Value)
		if err != nil {
			return err
		}
		if typ != declared {
			return fmt.Errorf("строка %d: переменной %q типа %s нельзя присвоить значение типа %s", s.Line, s.Name, declared, typ)
		}
		fmt.Fprintf(sb, "%s%s = %s;\n", ind, mangleVar(s.Name), code)
		return nil

	case *ast.IfStmt:
		cond, typ, err := genExpr(ctx, s.Cond)
		if err != nil {
			return err
		}
		if typ != typeBool {
			return fmt.Errorf("строка %d: условие \"если\" должно быть логическим, получено %s", s.Line, typ)
		}
		fmt.Fprintf(sb, "%sif (%s) {\n", ind, cond)
		ctx.pushScope()
		for _, st := range s.Then {
			if err := generateStmt(sb, ctx, st, depth+1); err != nil {
				return err
			}
		}
		ctx.popScope()
		if s.Else != nil {
			fmt.Fprintf(sb, "%s} else {\n", ind)
			ctx.pushScope()
			for _, st := range s.Else {
				if err := generateStmt(sb, ctx, st, depth+1); err != nil {
					return err
				}
			}
			ctx.popScope()
		}
		fmt.Fprintf(sb, "%s}\n", ind)
		return nil

	case *ast.WhileStmt:
		cond, typ, err := genExpr(ctx, s.Cond)
		if err != nil {
			return err
		}
		if typ != typeBool {
			return fmt.Errorf("строка %d: условие \"пока\" должно быть логическим, получено %s", s.Line, typ)
		}
		fmt.Fprintf(sb, "%swhile (%s) {\n", ind, cond)
		ctx.pushScope()
		for _, st := range s.Body {
			if err := generateStmt(sb, ctx, st, depth+1); err != nil {
				return err
			}
		}
		ctx.popScope()
		fmt.Fprintf(sb, "%s}\n", ind)
		return nil

	case *ast.ForStmt:
		fromCode, fromType, err := genExpr(ctx, s.From)
		if err != nil {
			return err
		}
		if fromType != typeInt {
			return fmt.Errorf("строка %d: граница диапазона после \"от\" должна быть целым числом, получено %s", s.Line, fromType)
		}
		toCode, toType, err := genExpr(ctx, s.To)
		if err != nil {
			return err
		}
		if toType != typeInt {
			return fmt.Errorf("строка %d: граница диапазона после \"до\" должна быть целым числом, получено %s", s.Line, toType)
		}
		ctx.pushScope()
		ctx.declare(s.Var, typeInt) // область свежая - имя точно свободно
		varC := mangleVar(s.Var)
		fmt.Fprintf(sb, "%sfor (long long %s = %s; %s <= %s; %s++) {\n", ind, varC, fromCode, varC, toCode, varC)
		for _, st := range s.Body {
			if err := generateStmt(sb, ctx, st, depth+1); err != nil {
				return err
			}
		}
		ctx.popScope()
		fmt.Fprintf(sb, "%s}\n", ind)
		return nil

	case *ast.ReturnStmt:
		fmt.Fprintf(sb, "%sreturn 0;\n", ind)
		return nil

	default:
		return fmt.Errorf("строка %d: инструкция %T пока не поддерживается C backend'ом", lineOf(stmt), stmt)
	}
}

func printCall(typ valueType, code string) string {
	switch typ {
	case typeString:
		return fmt.Sprintf(`printf("%%s\n", %s);`, code)
	case typeInt:
		return fmt.Sprintf(`printf("%%lld\n", %s);`, code)
	case typeFloat:
		return fmt.Sprintf(`printf("%%g\n", %s);`, code)
	case typeBool:
		return fmt.Sprintf(`printf("%%s\n", (%s) ? "истина" : "ложь");`, code)
	default:
		return fmt.Sprintf(`printf("%%s\n", %s);`, code)
	}
}

// genExpr генерирует C-выражение для expr и возвращает вместе с ним
// выведенный тип значения.
func genExpr(ctx *genCtx, expr ast.Expr) (string, valueType, error) {
	switch e := expr.(type) {
	case *ast.StringLiteral:
		return fmt.Sprintf(`"%s"`, escapeCString(e.Value)), typeString, nil

	case *ast.IntLiteral:
		return fmt.Sprintf("%dLL", e.Value), typeInt, nil

	case *ast.FloatLiteral:
		return formatFloatLiteral(e.Value), typeFloat, nil

	case *ast.BoolLiteral:
		if e.Value {
			return "true", typeBool, nil
		}
		return "false", typeBool, nil

	case *ast.Identifier:
		typ, ok := ctx.lookup(e.Name)
		if !ok {
			return "", typeUnknown, fmt.Errorf("строка %d: переменная %q не объявлена", e.Line, e.Name)
		}
		return mangleVar(e.Name), typ, nil

	case *ast.UnaryExpr:
		return genUnaryExpr(ctx, e)

	case *ast.BinaryExpr:
		return genBinaryExpr(ctx, e)

	default:
		return "", typeUnknown, fmt.Errorf("строка %d: выражение %T пока не поддерживается C backend'ом", lineOf(expr), expr)
	}
}

func genUnaryExpr(ctx *genCtx, e *ast.UnaryExpr) (string, valueType, error) {
	code, typ, err := genExpr(ctx, e.Operand)
	if err != nil {
		return "", typeUnknown, err
	}
	switch e.Op {
	case "-":
		if typ != typeInt && typ != typeFloat {
			return "", typeUnknown, fmt.Errorf("строка %d: унарный \"-\" применим только к числам, получен %s", e.Line, typ)
		}
		return fmt.Sprintf("(-%s)", code), typ, nil
	case "не":
		if typ != typeBool {
			return "", typeUnknown, fmt.Errorf("строка %d: \"не\" применимо только к логическому значению, получен %s", e.Line, typ)
		}
		return fmt.Sprintf("(!%s)", code), typeBool, nil
	default:
		return "", typeUnknown, fmt.Errorf("строка %d: неизвестная унарная операция %q", e.Line, e.Op)
	}
}

func genBinaryExpr(ctx *genCtx, e *ast.BinaryExpr) (string, valueType, error) {
	lc, lt, err := genExpr(ctx, e.Left)
	if err != nil {
		return "", typeUnknown, err
	}
	rc, rt, err := genExpr(ctx, e.Right)
	if err != nil {
		return "", typeUnknown, err
	}

	switch e.Op {
	case "+":
		if lt == typeString && rt == typeString {
			ctx.usesConcat = true
			return fmt.Sprintf("rech_str_concat(%s, %s)", lc, rc), typeString, nil
		}
		return genArithmetic(e, "+", lc, lt, rc, rt)
	case "-", "*", "/":
		return genArithmetic(e, e.Op, lc, lt, rc, rt)

	case "==", "!=":
		if lt == typeString && rt == typeString {
			ctx.usesStrCmp = true
			cmp := fmt.Sprintf("strcmp(%s, %s)", lc, rc)
			if e.Op == "==" {
				return fmt.Sprintf("(%s == 0)", cmp), typeBool, nil
			}
			return fmt.Sprintf("(%s != 0)", cmp), typeBool, nil
		}
		if lt != rt || (lt != typeInt && lt != typeFloat && lt != typeBool) {
			return "", typeUnknown, fmt.Errorf("строка %d: оператор %q не поддерживается для типов %s и %s", e.Line, e.Op, lt, rt)
		}
		return fmt.Sprintf("(%s %s %s)", lc, e.Op, rc), typeBool, nil

	case "<", ">", "<=", ">=":
		if lt != rt || (lt != typeInt && lt != typeFloat) {
			return "", typeUnknown, fmt.Errorf("строка %d: оператор %q применим только к числам одного типа, получены %s и %s", e.Line, e.Op, lt, rt)
		}
		return fmt.Sprintf("(%s %s %s)", lc, e.Op, rc), typeBool, nil

	case "и", "или":
		if lt != typeBool || rt != typeBool {
			return "", typeUnknown, fmt.Errorf("строка %d: оператор %q применим только к логическим значениям, получены %s и %s", e.Line, e.Op, lt, rt)
		}
		cOp := "&&"
		if e.Op == "или" {
			cOp = "||"
		}
		return fmt.Sprintf("(%s %s %s)", lc, cOp, rc), typeBool, nil

	default:
		return "", typeUnknown, fmt.Errorf("строка %d: неизвестная операция %q", e.Line, e.Op)
	}
}

func genArithmetic(e *ast.BinaryExpr, op string, lc string, lt valueType, rc string, rt valueType) (string, valueType, error) {
	if lt != typeInt && lt != typeFloat {
		return "", typeUnknown, fmt.Errorf("строка %d: оператор %q не поддерживается для типа %s", e.Line, op, lt)
	}
	if rt != typeInt && rt != typeFloat {
		return "", typeUnknown, fmt.Errorf("строка %d: оператор %q не поддерживается для типа %s", e.Line, op, rt)
	}
	if lt == typeFloat || rt == typeFloat {
		if lt == typeInt {
			lc = fmt.Sprintf("((double)%s)", lc)
		}
		if rt == typeInt {
			rc = fmt.Sprintf("((double)%s)", rc)
		}
		return fmt.Sprintf("(%s %s %s)", lc, op, rc), typeFloat, nil
	}
	return fmt.Sprintf("(%s %s %s)", lc, op, rc), typeInt, nil
}

func formatFloatLiteral(v float64) string {
	s := strconv.FormatFloat(v, 'g', -1, 64)
	if !strings.ContainsAny(s, ".eE") {
		s += ".0"
	}
	return s
}

func escapeCString(s string) string {
	var sb strings.Builder
	for _, r := range s {
		switch r {
		case '"':
			sb.WriteString(`\"`)
		case '\\':
			sb.WriteString(`\\`)
		case '\n':
			sb.WriteString(`\n`)
		case '\t':
			sb.WriteString(`\t`)
		default:
			sb.WriteRune(r)
		}
	}
	return sb.String()
}

func lineOf(n ast.Node) int {
	switch v := n.(type) {
	case *ast.PrintStmt:
		return v.Line
	case *ast.VarDecl:
		return v.Line
	case *ast.Assign:
		return v.Line
	case *ast.IfStmt:
		return v.Line
	case *ast.WhileStmt:
		return v.Line
	case *ast.ForStmt:
		return v.Line
	case *ast.ReturnStmt:
		return v.Line
	case *ast.StringLiteral:
		return v.Line
	case *ast.IntLiteral:
		return v.Line
	case *ast.FloatLiteral:
		return v.Line
	case *ast.BoolLiteral:
		return v.Line
	case *ast.Identifier:
		return v.Line
	case *ast.UnaryExpr:
		return v.Line
	case *ast.BinaryExpr:
		return v.Line
	default:
		return 0
	}
}
