package format

import (
	"fmt"
	"strings"
	"text/tabwriter"

	"github.com/IlyaFeoktistov/Rech/internal/ast"
	"github.com/IlyaFeoktistov/Rech/internal/lexer"
	"github.com/IlyaFeoktistov/Rech/internal/parser"
)

// alignVarDecls выравнивает соседние "переменная имя[: Тип] = значение" в
// таблицу по "=" - тот же принцип, что gofmt применяет к соседним
// "имя = значение"/"имя Тип = значение" внутри одного "var (...)"/"const
// (...)" блока (пользователь попросил явно - "как в Go... выравнивание
// переменных", см. [[alignEntityFields]] про то же самое для полей
// сущности). "Соседние" - подряд идущие ast.VarDecl в ОДНОМ списке
// инструкций (тело функции/метода/если/пока/для/... - любой блок), без
// пустой строки и без ЛЮБОЙ другой инструкции между ними - как только
// встречается что-то ещё, текущий пробег выравнивания завершается, копится
// новый после.
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

	var walkBody func(body []ast.Stmt)
	walkBody = func(body []ast.Stmt) {
		var run []varRow
		flush := func() {
			if len(run) >= 2 {
				renderVarRun(lines, run)
			}
			run = nil
		}
		prevLine := 0
		for _, s := range body {
			v, ok := s.(*ast.VarDecl)
			if !ok {
				flush()
				prevLine = 0
			} else {
				if prevLine != 0 && hasBlankLineBetween(lines, prevLine, v.Line) {
					flush()
				}
				row, ok := buildVarRow(lines, lineToks, v)
				if !ok {
					flush()
				} else {
					run = append(run, row)
				}
				prevLine = v.Line
			}
			switch st := s.(type) {
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
		flush()
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
	}

	return strings.Join(lines, "\n") + "\n", nil
}

// varRow - один ряд будущей таблицы "переменная ... = ..." - cells[0] -
// "переменная имя[: Тип]" (включая ведущий отступ строки - НЕ обрезается,
// тем же принципом и по той же причине, что и у fieldRow.cells[0],
// field_align.go), cells[1] - "= значение" (сырой текст правой части, как
// в исходнике).
type varRow struct {
	line  int
	cells [2]string
}

// buildVarRow разбирает "переменная ... = ..." на строке v.Line на две
// ячейки - false, если объявление растянуто на несколько строк (значение
// само многострочное - редкость) - тогда выравниванием эта строка не
// трогается и обрывает текущий пробег, как и пустая строка.
func buildVarRow(lines []string, lineToks map[int][]lexer.Token, v *ast.VarDecl) (varRow, bool) {
	row := varRow{line: v.Line}
	if v.Line < 1 || v.Line > len(lines) {
		return row, false
	}
	toks := lineToks[v.Line]
	assignIdx := -1
	for i, t := range toks {
		if t.Type == lexer.ASSIGN {
			assignIdx = i
			break
		}
	}
	if assignIdx == -1 {
		return row, false
	}
	// Значение обязано завершиться на ЭТОЙ ЖЕ строке - все токены после "="
	// на строке v.Line.
	for i := assignIdx + 1; i < len(toks); i++ {
		if toks[i].Line != v.Line {
			return row, false
		}
	}

	lineText := lines[v.Line-1]
	runeLine := []rune(lineText)
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

	row.cells[0] = strings.TrimRight(slice(1, toks[assignIdx].Col), " \t")
	row.cells[1] = strings.TrimRight(slice(toks[assignIdx].Col, lineEndCol), " \t\r")
	return row, true
}

// renderVarRun - тот же text/tabwriter, что и renderFieldRun (field_align.go) -
// padding=1, без выравнивания по правому краю.
func renderVarRun(lines []string, run []varRow) {
	var buf strings.Builder
	tw := tabwriter.NewWriter(&buf, 0, 0, 1, ' ', 0)
	for _, row := range run {
		fmt.Fprintln(tw, row.cells[0]+"\t"+row.cells[1])
	}
	tw.Flush()

	rendered := strings.Split(strings.TrimSuffix(buf.String(), "\n"), "\n")
	if len(rendered) != len(run) {
		return
	}
	for i, row := range run {
		lines[row.line-1] = rendered[i]
	}
}
