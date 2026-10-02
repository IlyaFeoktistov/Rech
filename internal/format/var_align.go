package format

import (
	"fmt"
	"sort"
	"strings"
	"text/tabwriter"

	"github.com/IlyaFeoktistov/Rech/internal/ast"
	"github.com/IlyaFeoktistov/Rech/internal/lexer"
	"github.com/IlyaFeoktistov/Rech/internal/parser"
)

// alignVarDecls выравнивает соседние объявления переменных и констант в
// таблицу из трёх столбцов - тот же принцип, что gofmt применяет к "var
// (...)"/"const (...)", и тот же, что у полей сущности (alignEntityFields):
//
//	Целое          число          = 2
//	Действительное действительное = 2.10
//	переменная     доля           = 0.5
//
// Первый столбец - всё до имени (модификаторы с типом, "переменная",
// "константа"), второй - имя, третий - "= значение" (у объявления без
// значения его нет). Охватывает локальные объявления в любом блоке,
// переменные и константы модуля, константы сущности.
//
// "Соседние" - объявления на подряд идущих строках с одинаковым отступом:
// пустая строка, комментарий или любая другая инструкция между ними
// завершает пробег. Объявление, чьё значение не уместилось в одну строку,
// выравниванием не трогается и тоже обрывает пробег.
func alignVarDecls(src string) (string, error) {
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

	normalized := strings.ReplaceAll(src, "\r\n", "\n")
	lines := strings.Split(normalized, "\n")

	decls := map[int]string{} // строка -> имя объявленного
	var walkBody func(body []ast.Stmt)
	walkBody = func(body []ast.Stmt) {
		for _, s := range body {
			switch st := s.(type) {
			case *ast.VarDecl:
				decls[st.Line] = st.Name
			case *ast.ConstDecl:
				decls[st.Line] = st.Name
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
		}
	}

	for _, fn := range prog.Functions {
		walkBody(fn.Body)
	}
	for _, ent := range prog.Entities {
		if ent.Ctor != nil {
			walkBody(ent.Ctor.Body)
		}
		for _, m := range ent.Methods {
			if m.Body != nil {
				walkBody(m.Body)
			}
		}
		for _, c := range ent.Constants {
			decls[c.Line] = c.Name
		}
	}
	for _, c := range prog.Consts {
		decls[c.Line] = c.Name
	}
	for _, v := range prog.Vars {
		decls[v.Line] = v.Name
	}

	declLines := make([]int, 0, len(decls))
	for l := range decls {
		declLines = append(declLines, l)
	}
	sort.Ints(declLines)

	var run []varRow
	flush := func() {
		if len(run) >= 2 {
			renderVarRun(lines, run)
		}
		run = nil
	}
	for _, l := range declLines {
		row, ok := buildVarRow(lines, lineToks, l, decls[l])
		if !ok {
			flush()
			continue
		}
		if len(run) > 0 {
			prev := run[len(run)-1]
			if prev.line+1 != l || leadingIndent(lines[prev.line-1]) != leadingIndent(lines[l-1]) {
				flush()
			}
		}
		run = append(run, row)
	}
	flush()

	return strings.Join(lines, "\n") + "\n", nil
}

func leadingIndent(line string) string {
	return line[:len(line)-len(strings.TrimLeft(line, " \t"))]
}

// varRow - один ряд будущей таблицы: cells[0] - всё до имени (включая
// ведущий отступ строки - НЕ обрезается, тем же принципом и по той же
// причине, что и у fieldRow.cells[0], field_align.go), cells[1] - имя,
// cells[2] - "= значение" (сырой текст правой части, как в исходнике; ""
// у объявления без значения).
type varRow struct {
	line  int
	cells [3]string
}

// buildVarRow разбирает объявление name на строке line на ячейки - false,
// если строка не начинается этим объявлением или значение растянуто на
// несколько строк.
func buildVarRow(lines []string, lineToks map[int][]lexer.Token, line int, name string) (varRow, bool) {
	row := varRow{line: line}
	if line < 1 || line > len(lines) {
		return row, false
	}
	toks := lineToks[line]
	assignIdx := len(toks)
	for i, t := range toks {
		if t.Type == lexer.ASSIGN {
			assignIdx = i
			break
		}
	}
	nameIdx := assignIdx - 1
	if nameIdx < 1 || toks[nameIdx].Type != lexer.IDENT || toks[nameIdx].Value != name {
		return row, false
	}
	// Значение обязано завершиться на ЭТОЙ ЖЕ строке.
	for i := assignIdx; i < len(toks); i++ {
		if toks[i].Line != line {
			return row, false
		}
	}

	runeLine := []rune(lines[line-1])
	lineEndCol := len(runeLine) + 1
	slice := func(fromCol, toCol int) string {
		from, to := fromCol-1, toCol-1
		if from < 0 {
			from = 0
		}
		if to > len(runeLine) {
			to = len(runeLine)
		}
		if from >= to {
			return ""
		}
		return string(runeLine[from:to])
	}

	nameCol := toks[nameIdx].Col
	nameEnd := lineEndCol
	if assignIdx < len(toks) {
		nameEnd = toks[assignIdx].Col
	}
	row.cells[0] = strings.TrimRight(slice(1, nameCol), " \t")
	row.cells[1] = strings.TrimSpace(slice(nameCol, nameEnd))
	if assignIdx < len(toks) {
		row.cells[2] = strings.TrimRight(slice(toks[assignIdx].Col, lineEndCol), " \t\r")
	}
	return row, true
}

// renderVarRun - тот же text/tabwriter, что и renderFieldRun (field_align.go) -
// padding=1, без выравнивания по правому краю; столбец "=" пустой у всего
// пробега не добавляет хвостовых пробелов.
func renderVarRun(lines []string, run []varRow) {
	var buf strings.Builder
	tw := tabwriter.NewWriter(&buf, 0, 0, 1, ' ', 0)
	for _, row := range run {
		cells := row.cells[:]
		if cells[2] == "" {
			cells = cells[:2]
		}
		fmt.Fprintln(tw, strings.Join(cells, "\t"))
	}
	tw.Flush()

	rendered := strings.Split(strings.TrimSuffix(buf.String(), "\n"), "\n")
	if len(rendered) != len(run) {
		return
	}
	for i, row := range run {
		lines[row.line-1] = strings.TrimRight(rendered[i], " ")
	}
}
