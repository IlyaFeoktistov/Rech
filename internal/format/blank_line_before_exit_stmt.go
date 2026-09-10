package format

import (
	"strings"

	"github.com/IlyaFeoktistov/Rech/internal/ast"
	"github.com/IlyaFeoktistov/Rech/internal/lexer"
	"github.com/IlyaFeoktistov/Rech/internal/parser"
)

// insertBlankLineBeforeExitStmt - "вернуть"/"прервать"/"продолжить" сразу
// после ХОТЯ БЫ ОДНОЙ другой инструкции в том же теле УПРАВЛЯЮЩЕЙ
// КОНСТРУКЦИИ (если/иначе/пока/для/попытаться/поймать/окончательно/ветка
// "выбрать" - НЕ тело функции/метода/конструктора НАПРЯМУЮ, тот же
// controlLevel-принцип, что у explodeInlineExitStmt.go выше - "если x < 0 {
// вернуть 0 } вернуть x" - устоявшийся идиоматичный guard clause,
// TestFormatExplodesInlineReturnInsideIf/TestFormatAlreadyExplodedReturnStaysUnchanged
// подтверждают, что верхний уровень функции - НЕ трогать) получает пустую
// строку-разделитель перед собой - тот же мотив, что и у самой
// explodeInlineExitStatements ("легко проглядеть при беглом чтении, если не
// выделена"), только тут инструкция уже на своей строке (эта функция зовётся
// сразу следом), просто СЛИТА с предыдущим кодом без пустой строки между
// ними. Первая инструкция своего блока (сразу после "{") НЕ получает пустую
// строку - там разделять нечего, "если x { вернуть 0 }" уже читается
// однозначно.
//
// Идемпотентно (уже отделённая пустая строка не дублируется) - та же
// проверка "строка над и так пуста или кончается на '{'", что у
// insertBlankLinesBetweenDecls. Ведущий "//"-комментарий прямо над
// инструкцией (без своей пустой строки) сдвигается вместе с ней - пустая
// строка встаёт НАД комментарием, не разлучая его с тем, что он поясняет.
func insertBlankLineBeforeExitStmt(src string) (string, error) {
	toks, err := lexer.New(src).Tokenize()
	if err != nil {
		return "", err
	}
	prog, err := parser.New(toks).ParseProgram()
	if err != nil {
		return "", err
	}

	normalized := strings.ReplaceAll(src, "\r\n", "\n")
	lines := strings.Split(normalized, "\n")

	isPlainCommentLine := func(lineNo int) bool {
		if lineNo < 1 || lineNo > len(lines) {
			return false
		}
		return strings.HasPrefix(strings.TrimSpace(lines[lineNo-1]), "//")
	}
	blockStart := func(line int) int {
		for isPlainCommentLine(line - 1) {
			line--
		}
		return line
	}

	starts := map[int]bool{}
	var exitStmtLine func(s ast.Stmt) (int, bool)
	exitStmtLine = func(s ast.Stmt) (int, bool) {
		switch st := s.(type) {
		case *ast.ReturnStmt:
			return st.Line, true
		case *ast.BreakStmt:
			return st.Line, true
		case *ast.ContinueStmt:
			return st.Line, true
		}
		return 0, false
	}
	var walkBody func(body []ast.Stmt, controlLevel bool)
	walkBody = func(body []ast.Stmt, controlLevel bool) {
		for i, s := range body {
			if controlLevel && i > 0 {
				if line, ok := exitStmtLine(s); ok {
					starts[blockStart(line)] = true
				}
			}
			switch st := s.(type) {
			case *ast.IfStmt:
				walkBody(st.Then, true)
				walkBody(st.Else, true)
			case *ast.WhileStmt:
				walkBody(st.Body, true)
			case *ast.ForStmt:
				walkBody(st.Body, true)
			case *ast.ForEachStmt:
				walkBody(st.Body, true)
			case *ast.CStyleForStmt:
				walkBody(st.Body, true)
			case *ast.TryStmt:
				walkBody(st.Body, true)
				walkBody(st.CatchBody, true)
				walkBody(st.Finally, true)
			case *ast.SwitchStmt:
				for _, c := range st.Cases {
					walkBody(c.Body, true)
				}
				walkBody(st.Default, true)
			}
		}
	}
	for _, fn := range prog.Functions {
		walkBody(fn.Body, false)
	}
	for _, ent := range prog.Entities {
		if ent.Ctor != nil {
			walkBody(ent.Ctor.Body, false)
		}
		for _, m := range ent.Methods {
			if m.Body != nil {
				walkBody(m.Body, false)
			}
		}
	}

	var out []string
	for i, line := range lines {
		lineNo := i + 1
		if starts[lineNo] && lineNo > 1 {
			above := strings.TrimRight(lines[lineNo-2], " \t\r")
			if above != "" && !strings.HasSuffix(above, "{") {
				out = append(out, "")
			}
		}
		out = append(out, line)
	}
	return strings.Join(out, "\n") + "\n", nil
}
