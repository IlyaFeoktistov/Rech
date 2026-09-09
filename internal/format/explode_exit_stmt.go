package format

import (
	"sort"
	"strings"

	"github.com/IlyaFeoktistov/Rech/internal/ast"
	"github.com/IlyaFeoktistov/Rech/internal/lexer"
	"github.com/IlyaFeoktistov/Rech/internal/parser"
)

// explodeInlineExitStatements - "вернуть"/"прервать"/"продолжить", зажатые
// на одной физической строке с окружающим кодом ("если x { вернуть 0 }" -
// целиком одной строкой, или только "{ вернуть 0 }"/"вернуть 0 }" - часть
// строки одна) внутри УПРАВЛЯЮЩЕЙ конструкции (если/иначе/для/пока/
// попытаться/поймать/окончательно/ветка "выбрать") - переносятся на
// собственную строку: такую инструкцию легко проглядеть при беглом чтении,
// если она не выделена отдельной строкой (пользователь попросил явно, по
// аналогии с break/continue - "как они там в речь").
//
// НЕ трогает то же самое внутри тела функции/метода/конструктора/лямбды
// напрямую (не внутри вложенной управляющей конструкции) - однострочное
// "функция ф(): Целое { вернуть 1 }" уже устоявшийся стиль (см.
// internal/format/format_test.go, TestFormatFunctionKeepsAllmanBraceOnOwnLine
// - v1 форматтера сознательно не переносит однострочные объявления).
//
// Работает НАД ИСХОДНЫМ ТЕКСТОМ, ДО normalizeBraceStyle (brace_style.go) -
// после разбивки "если x { вернуть 0 }" на "если x {"/"вернуть 0"/"}"
// заголовок с "{" уже и так K&R-совместимы (остаются на одной строке без
// вмешательства normalizeBraceStyle), закрывающая "}" - на собственной,
// normalizeBraceStyle обрабатывает её дальше как обычно (в т. ч. цепочку
// "} иначе {", если следом идёт продолжение).
func explodeInlineExitStatements(src string) (string, error) {
	toks, err := lexer.New(src).Tokenize()
	if err != nil {
		return "", err
	}
	prog, err := parser.New(toks).ParseProgram()
	if err != nil {
		return "", err
	}

	lineToks := map[int][]lexer.Token{}
	for _, t := range toks {
		if t.Type == lexer.NEWLINE || t.Type == lexer.EOF {
			continue
		}
		lineToks[t.Line] = append(lineToks[t.Line], t)
	}

	splitCols := map[int][]int{}
	var walkBody func(body []ast.Stmt, controlLevel bool)
	walkBody = func(body []ast.Stmt, controlLevel bool) {
		if controlLevel && len(body) == 1 {
			registerExitSplit(body[0], lineToks, splitCols)
		}
		for _, s := range body {
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

	normalized := strings.ReplaceAll(src, "\r\n", "\n")
	lines := strings.Split(normalized, "\n")
	return renderSplitColumns(lines, splitCols), nil
}

// registerExitSplit - для ЕДИНСТВЕННОЙ инструкции s тела управляющей
// конструкции (см. package doc), если это вернуть/прервать/продолжить,
// отмечает столбцы (1-индексные - "разбить строку ПЕРЕД этим столбцом"),
// на которых её нужно отделить: перед её собственным первым токеном, если
// перед ним есть что-то ЕЩЁ на этой же строке (не первая на строке), и
// перед первой "}", если она идёт следом на этой же строке (закрывает этот
// же блок). Уже правильно оформленная инструкция (одна на своей строке,
// ничего не добавляется) - НИЧЕГО не отмечает, ключевое для идемпотентности.
func registerExitSplit(s ast.Stmt, lineToks map[int][]lexer.Token, splitCols map[int][]int) {
	var kw lexer.TokenType
	var line int
	switch st := s.(type) {
	case *ast.ReturnStmt:
		kw, line = lexer.KwReturn, st.Line
	case *ast.BreakStmt:
		kw, line = lexer.KwBreak, st.Line
	case *ast.ContinueStmt:
		kw, line = lexer.KwContinue, st.Line
	default:
		return
	}
	toks := lineToks[line]
	idx := -1
	for i, t := range toks {
		if t.Type == kw {
			idx = i
			break
		}
	}
	if idx == -1 {
		return
	}
	if idx > 0 {
		splitCols[line] = append(splitCols[line], toks[idx].Col)
	}
	end := len(toks) - 1
	for i := idx; i < len(toks); i++ {
		if toks[i].Type == lexer.RBRACE {
			end = i - 1
			break
		}
	}
	if end+1 < len(toks) {
		splitCols[line] = append(splitCols[line], toks[end+1].Col)
	}
}

// renderSplitColumns разбивает КАЖДУЮ строку lines на несколько выходных
// строк по столбцам splitCols[номер строки] (1-индексные, по возрастанию -
// "разбить перед этим столбцом") - строки без отметок остаются как есть.
func renderSplitColumns(lines []string, splitCols map[int][]int) string {
	var out []string
	for i, line := range lines {
		cols := splitCols[i+1]
		if len(cols) == 0 {
			out = append(out, line)
			continue
		}
		sort.Ints(cols)
		runes := []rune(line)
		prev := 0
		for _, c := range cols {
			idx := c - 1
			if idx < prev || idx > len(runes) {
				continue
			}
			out = append(out, strings.TrimRight(string(runes[prev:idx]), " \t"))
			prev = idx
		}
		out = append(out, strings.TrimSpace(string(runes[prev:])))
	}
	return strings.Join(out, "\n") + "\n"
}
