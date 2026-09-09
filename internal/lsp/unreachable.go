// Недостижимый код - диагностика (hintSeverity + unnecessaryTag, тот же
// принцип рендера "выцветшим текстом", что и у TypeScript "unreachable
// code detected") на каждой инструкции ПОСЛЕ "вернуть"/"выбросить"/
// "прервать"/"продолжить" в ОДНОМ И ТОМ ЖЕ списке инструкций - эти четыре
// безусловно передают управление ДАЛЬШЕ этого блока (вернуть/выбросить -
// из функции, прервать/продолжить - из цикла), значит всё, что написано
// следом в ТОМ ЖЕ блоке, никогда не исполнится.
//
// НЕ анализирует более сложную исчерпываемость (например, "если x {
// вернуть 1 } иначе { вернуть 2 }" - обе ветки всегда завершаются, значит
// код ПОСЛЕ этого "если" тоже недостижим - это уже настоящий анализ потока
// управления через ветвление, вне объёма этого среза) - только прямое,
// нередкое на практике "инструкция сразу после return/throw/break/continue
// в одном блоке" - пользователь попросил именно этот случай явно.
package lsp

import (
	"strings"

	"github.com/IlyaFeoktistov/Rech/internal/ast"
)

// unnecessaryTag - LSP DiagnosticTag.Unnecessary (1, см. LSP spec) - тот
// же смысл, что и у deprecatedTag (2, deprecated.go) для другого случая:
// редактор рисует помеченный текст выцветшим, не подчёркивает как ошибку.
const unnecessaryTag = 1

// unreachableDiagnostics - см. package doc. Пусто, если разбор не удался
// вовсе (d.astProg == nil) - те же условия, что и у codeActions/
// suggestions/deprecatedDiagnostics.
func (d *doc) unreachableDiagnostics() []Diagnostic {
	if d.astProg == nil {
		return nil
	}
	lines := strings.Split(d.text, "\n")
	var lineNos []int

	var walkBody func(body []ast.Stmt)
	walkBody = func(body []ast.Stmt) {
		terminated := false
		for _, s := range body {
			if terminated {
				lineNos = append(lineNos, astStmtLine(s))
				continue // весь этот блок уже недостижим - не разбираем вложенность отдельно
			}
			switch st := s.(type) {
			case *ast.ReturnStmt, *ast.ThrowStmt, *ast.BreakStmt, *ast.ContinueStmt:
				terminated = true
			case *ast.IfStmt:
				walkBody(st.Then)
				walkBody(st.Else)
			case *ast.WhileStmt:
				walkBody(st.Body)
			case *ast.ForStmt:
				walkBody(st.Body)
			case *ast.ForEachStmt:
				walkBody(st.Body)
			case *ast.CStyleForStmt:
				walkBody(st.Body)
			case *ast.TryStmt:
				walkBody(st.Body)
				walkBody(st.CatchBody)
				walkBody(st.Finally)
			case *ast.SwitchStmt:
				for _, c := range st.Cases {
					walkBody(c.Body)
				}
				walkBody(st.Default)
			case *ast.GerundStmt:
				walkBody(st.Body)
			}
			// Лямбда-колбэк, спрятанная в выражении этой инструкции
			// (аргумент вызова и т. п.) - тоже отдельное тело, со своей
			// собственной недостижимостью (та же дыра, что нашёл
			// пользователь для hints.go - см. package doc walkLambdaBodies).
			for _, e := range immediateExprs(s) {
				walkLambdaBodies(e, walkBody)
			}
		}
	}

	for _, fn := range d.astProg.Functions {
		walkBody(fn.Body)
	}
	for _, ent := range d.astProg.Entities {
		if ent.Ctor != nil {
			walkBody(ent.Ctor.Body)
		}
		for _, m := range ent.Methods {
			if m.Body != nil {
				walkBody(m.Body)
			}
		}
	}

	var out []Diagnostic
	for _, l := range lineNos {
		if l < 1 || l > len(lines) {
			continue
		}
		runes := []rune(lines[l-1])
		start, end := trimmedRuneRange(runes)
		if end <= start {
			continue
		}
		out = append(out, Diagnostic{
			Line: l, Col: start + 1, Len: end - start,
			Message: "недостижимый код - это выражение не выполнится, потому что раньше в этом же блоке уже стоит вернуть/выбросить/прервать/продолжить",
			Severity: hintSeverity, Tags: []int{unnecessaryTag},
		})
	}
	return out
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
