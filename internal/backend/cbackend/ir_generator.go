// GenerateFromIR - C backend поверх internal/ir (шаг 5.c, начало) -
// параллельный путь к Generate(*ast.Program): пока не встроен в реальную
// компиляцию (internal/compiler по-прежнему вызывает Generate напрямую),
// покрывает ровно то же подмножество, что и internal/semantic.Analyze на
// данный момент - "вывести <строка>", функция без параметров/типа
// результата. Расширяется вместе с покрытием internal/semantic, вертикальным
// срезом за срезом (см. package doc internal/ir), а не отдельно от него -
// не имеет смысла генерировать код для IR-узла, которого семантический
// анализ ещё не производит.
//
// Переиспользует то, что уже есть в этом пакете для генерации из AST
// (entryPointName, mangleName, valueType, printCall, escapeCString,
// indent) - IR-путь не имеет собственного понятия C-типа/мангла, это
// решение backend'а в обоих случаях одно и то же.
package cbackend

import (
	"fmt"
	"strings"

	"github.com/IlyaFeoktistov/Rech/internal/ir"
)

// GenerateFromIR возвращает исходник на C, соответствующий программе prog.
func GenerateFromIR(prog *ir.Program) (string, error) {
	hasEntryPoint := false
	var body strings.Builder
	for _, fn := range prog.Functions {
		if fn.Name == entryPointName {
			hasEntryPoint = true
		}
		fnCode, err := generateIRFunction(fn)
		if err != nil {
			return "", fmt.Errorf("функция %q: %w", fn.Name, err)
		}
		body.WriteString(fnCode)
	}
	if !hasEntryPoint {
		return "", fmt.Errorf("в программе не найдена функция %q - она служит точкой входа", entryPointName)
	}

	var sb strings.Builder
	sb.WriteString("// Автоматически сгенерировано компилятором rech. Не редактировать руками.\n")
	sb.WriteString("#include <stdio.h>\n\n")
	sb.WriteString(body.String())
	return sb.String(), nil
}

func generateIRFunction(fn *ir.Function) (string, error) {
	var sb strings.Builder
	fmt.Fprintf(&sb, "int %s(void) {\n", mangleName(fn.Name))
	for _, s := range fn.Body {
		if err := generateIRStmt(&sb, s, 1); err != nil {
			return "", err
		}
	}
	sb.WriteString("    return 0;\n}\n")
	return sb.String(), nil
}

func generateIRStmt(sb *strings.Builder, s ir.Stmt, depth int) error {
	ind := indent(depth)
	switch s := s.(type) {
	case *ir.PrintStmt:
		code, typ, err := generateIRExpr(s.Value)
		if err != nil {
			return err
		}
		fmt.Fprintf(sb, "%s%s\n", ind, printCall(typ, code))
		return nil
	default:
		return fmt.Errorf("генерация C из IR: инструкция %T пока не поддерживается", s)
	}
}

func generateIRExpr(e ir.Expr) (string, valueType, error) {
	switch e := e.(type) {
	case *ir.StringLiteral:
		return fmt.Sprintf(`"%s"`, escapeCString(e.Value)), typeString, nil
	default:
		return "", typeUnknown, fmt.Errorf("генерация C из IR: выражение %T пока не поддерживается", e)
	}
}
