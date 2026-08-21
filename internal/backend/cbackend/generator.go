// Package cbackend транслирует ast.Program в исходник на C.
//
// Это первый и на сегодня единственный backend Rech. По архитектуре из
// docs/ROADMAP.md он не единственный на будущее: AST специально не содержит
// ничего C-специфичного, чтобы позже рядом можно было добавить interpreter
// и VM backend без изменения lexer/parser/ast.
package cbackend

import (
	"fmt"
	"strings"

	"github.com/IlyaFeoktistov/Rech/internal/ast"
)

// entryPointName - функция с этим именем становится точкой входа программы
// (C main). Остальные имена транслитерируются в валидные C-идентификаторы.
const entryPointName = "главная"

// translit - таблица транслитерации кириллицы для имён функций.
// Не претендует на лингвистическую точность (это не морфология, см.
// docs/ROADMAP.md), задача - дать читаемый и уникальный C-идентификатор.
var translit = map[rune]string{
	'а': "a", 'б': "b", 'в': "v", 'г': "g", 'д': "d", 'е': "e", 'ё': "e",
	'ж': "zh", 'з': "z", 'и': "i", 'й': "j", 'к': "k", 'л': "l", 'м': "m",
	'н': "n", 'о': "o", 'п': "p", 'р': "r", 'с': "s", 'т': "t", 'у': "u",
	'ф': "f", 'х': "h", 'ц': "c", 'ч': "ch", 'ш': "sh", 'щ': "sch", 'ъ': "",
	'ы': "y", 'ь': "", 'э': "e", 'ю': "yu", 'я': "ya",
}

func mangleName(name string) string {
	if name == entryPointName {
		return "main"
	}
	var sb strings.Builder
	sb.WriteString("rech_")
	for _, r := range strings.ToLower(name) {
		if lat, ok := translit[r]; ok {
			sb.WriteString(lat)
			continue
		}
		sb.WriteRune(r)
	}
	return sb.String()
}

// Generate возвращает исходник на C, соответствующий программе prog.
func Generate(prog *ast.Program) (string, error) {
	var sb strings.Builder
	sb.WriteString("// Автоматически сгенерировано компилятором rech. Не редактировать руками.\n")
	sb.WriteString("#include <stdio.h>\n\n")

	hasEntryPoint := false
	for _, fn := range prog.Functions {
		if fn.Name == entryPointName {
			hasEntryPoint = true
		}
		if err := generateFunction(&sb, fn); err != nil {
			return "", err
		}
	}
	if !hasEntryPoint {
		return "", fmt.Errorf("в программе не найдена функция %q - она служит точкой входа", entryPointName)
	}
	return sb.String(), nil
}

func generateFunction(sb *strings.Builder, fn *ast.FunctionDecl) error {
	cName := mangleName(fn.Name)
	fmt.Fprintf(sb, "// %s\n", fn.Name)
	fmt.Fprintf(sb, "int %s(void) {\n", cName)
	for _, stmt := range fn.Body {
		if err := generateStmt(sb, stmt); err != nil {
			return fmt.Errorf("функция %q: %w", fn.Name, err)
		}
	}
	if cName == "main" {
		sb.WriteString("    return 0;\n")
	}
	sb.WriteString("}\n\n")
	return nil
}

func generateStmt(sb *strings.Builder, stmt ast.Stmt) error {
	switch s := stmt.(type) {
	case *ast.PrintStmt:
		value, err := evalConstString(s.Value)
		if err != nil {
			return err
		}
		fmt.Fprintf(sb, "    printf(\"%s\\n\");\n", escapeCString(value))
		return nil
	default:
		return fmt.Errorf("строка %d: инструкция %T пока не поддерживается C backend'ом", lineOf(stmt), stmt)
	}
}

// evalConstString в v0.1 умеет вычислять только строковые литералы -
// переменные и конкатенация появятся вместе с этапом "Выражения".
func evalConstString(expr ast.Expr) (string, error) {
	switch e := expr.(type) {
	case *ast.StringLiteral:
		return e.Value, nil
	default:
		return "", fmt.Errorf("строка %d: выражение %T пока не поддерживается C backend'ом", lineOf(expr), expr)
	}
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
	case *ast.StringLiteral:
		return v.Line
	default:
		return 0
	}
}
