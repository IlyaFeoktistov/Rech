// Недостижимый код - затенение (hintSeverity + unnecessaryTag, тот же
// рендер "выцветшим текстом", что и у TypeScript "unreachable code
// detected") всего недостижимого куска: от первой недостижимой инструкции
// до конца её блока. Источник - предупреждения компилятора
// (ir.Program.Warnings, internal/semantic/flow.go): граф потока управления
// видит и код после "если/иначе", где обе ветки выходят, и после вечного
// цикла. Плюс то же предупреждение уровнем warningSeverity на первой
// строке куска - чтобы оно было в списке проблем редактора.
package lsp

import (
	"errors"
	"strings"

	"github.com/IlyaFeoktistov/Rech/internal/ast"
	"github.com/IlyaFeoktistov/Rech/internal/diagnostics"
	"github.com/IlyaFeoktistov/Rech/internal/lexer"
	"github.com/IlyaFeoktistov/Rech/internal/semantic"
)

// unnecessaryTag - LSP DiagnosticTag.Unnecessary (1, см. LSP spec) - тот
// же смысл, что и у deprecatedTag (2, deprecated.go) для другого случая:
// редактор рисует помеченный текст выцветшим, не подчёркивает как ошибку.
const unnecessaryTag = 1

// unreachableDiagnostics - см. package doc. Пусто без IR (d.irProg == nil).
func (d *doc) unreachableDiagnostics() []Diagnostic {
	if d.irProg == nil {
		return nil
	}
	lines := strings.Split(d.text, "\n")
	var out []Diagnostic
	for _, w := range d.irProg.Warnings {
		var dg *diagnostics.Diagnostic
		if !errors.As(w, &dg) || !strings.HasSuffix(dg.Message, semantic.UnreachableMessage) || !d.isOwnFile(dg.Pos.File) {
			continue
		}
		line := dg.Pos.Line
		if line < 1 || line > len(lines) {
			continue
		}
		start, end := trimmedRuneRange([]rune(lines[line-1]))
		if end <= start {
			continue
		}
		endLine := d.blockEndLine(line)
		_, endCol := trimmedRuneRange([]rune(lines[endLine-1]))
		out = append(out,
			Diagnostic{
				Line: line, Col: start + 1, EndLine: endLine, EndCol: endCol + 1,
				Message: dg.Message, Severity: hintSeverity, Tags: []int{unnecessaryTag},
			},
			Diagnostic{
				Line: line, Col: start + 1, Len: end - start,
				Message: dg.Message, Severity: warningSeverity,
			})
	}
	return out
}

// isOwnFile - позиция file относится к этому документу (при многофайловой
// линковке предупреждения других файлов графа сюда не относятся).
func (d *doc) isOwnFile(file string) bool {
	return file == "" || file == d.uri || (d.path != "" && file == d.path)
}

// blockEndLine - последняя строка блока, в котором стоит инструкция со
// строки line: строка последнего токена перед "}", закрывающей этот блок
// (вложенные скобки пропускаются), или конец файла.
func (d *doc) blockEndLine(line int) int {
	last := line
	depth := 0
	started := false
	for _, tok := range d.tokens {
		if tok.Line < line {
			continue
		}
		switch tok.Type {
		case lexer.EOF:
			return last
		case lexer.LBRACE, lexer.LPAREN, lexer.LBRACKET:
			depth++
		case lexer.RBRACE, lexer.RPAREN, lexer.RBRACKET:
			if depth == 0 && started {
				return last
			}
			depth--
		case lexer.NEWLINE:
			continue
		}
		started = true
		last = max(tok.Line, tok.EndLine)
	}
	return last
}

// astStmtLine - строка объявления s (ast.Stmt не несёт общего метода
// Line() - у каждого конкретного типа своё поле).
func astStmtLine(s ast.Stmt) int {
	switch st := s.(type) {
	case *ast.PrintStmt:
		return st.Line
	case *ast.ShowStmt:
		return st.Line
	case *ast.BecomeStmt:
		return st.Line
	case *ast.AssertStmt:
		return st.Line
	case *ast.IfStmt:
		return st.Line
	case *ast.WhileStmt:
		return st.Line
	case *ast.ForStmt:
		return st.Line
	case *ast.ForEachStmt:
		return st.Line
	case *ast.CStyleForStmt:
		return st.Line
	case *ast.BreakStmt:
		return st.Line
	case *ast.ContinueStmt:
		return st.Line
	case *ast.ReturnStmt:
		return st.Line
	case *ast.ExprStmt:
		return st.Line
	case *ast.GerundStmt:
		return st.Line
	case *ast.Assign:
		return st.Line
	case *ast.FieldAssign:
		return st.Line
	case *ast.DerefAssignStmt:
		return st.Line
	case *ast.VarDecl:
		return st.Line
	case *ast.ThrowStmt:
		return st.Line
	case *ast.ParentCallStmt:
		return st.Line
	case *ast.TryStmt:
		return st.Line
	case *ast.SwitchStmt:
		return st.Line
	case *ast.ConstDecl:
		return st.Line
	default:
		return 0
	}
}
