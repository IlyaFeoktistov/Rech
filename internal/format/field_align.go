package format

import (
	"fmt"
	"strings"
	"text/tabwriter"

	"github.com/IlyaFeoktistov/Rech/internal/ast"
	"github.com/IlyaFeoktistov/Rech/internal/lexer"
	"github.com/IlyaFeoktistov/Rech/internal/parser"
)

// alignEntityFields выравнивает соседние объявления полей сущности/
// структуры ("имеет:") в таблицу - тем же принципом, что gofmt выравнивает
// теги структуры Go: "имя:" каждого поля дополняется пробелами до
// столбца, где начинается Тип (общий для всей группы), значение по
// умолчанию после "=" - до общего столбца тега. Поле без "="/тега просто
// заканчивается раньше - text/tabwriter сам не растягивает отсутствующую
// ячейку (рваные строки, ragged rows), значит на строке без значения по
// умолчанию не появляется висячих пробелов.
//
// "Соседние" - непрерывный пробег полей одной сущности БЕЗ пустой строки
// между ними (doc-комментарий поля пробег не прерывает - он на СВОЕЙ
// строке, не участвует в таблице вовсе). Поле, чьё объявление физически
// не уместилось в одну строку (растянутое значение по умолчанию -
// редкость), выравниванием не трогается и обрывает текущий пробег, как и
// пустая строка.
//
// Как и normalizeBraceStyle - работает НАД ИСХОДНЫМ ТЕКСТОМ (не через
// AST), собственный отступ выровненной строки не важен - computeLineDepths
// пересчитывает его позже заново.
func alignEntityFields(src string) (string, error) {
	toks, err := lexer.New(src).Tokenize()
	if err != nil {
		return "", fmt.Errorf("лексер: %w", err)
	}
	prog, err := parser.New(toks).ParseProgram()
	if err != nil {
		return "", fmt.Errorf("парсер: %w", err)
	}
	if len(prog.Entities) == 0 {
		return src, nil
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

	for _, ent := range prog.Entities {
		applyFieldAlignment(lines, lineToks, ent.Fields)
	}

	return strings.Join(lines, "\n"), nil
}

// fieldRow - один field ряда таблицы, уже разрезанный на ячейки
// (имя_с_модификаторами+":", Тип, "= значение_по_умолчанию", тег) -
// пустая строка ячейки, если этой части у поля нет (см. package doc
// alignEntityFields про рваные строки).
type fieldRow struct {
	line  int
	cells [4]string
}

// applyFieldAlignment находит в fields непрерывные пробеги (без пустой
// строки между полями и без полей, чьё объявление растянуто на
// несколько строк) и выравнивает каждый пробег отдельно.
func applyFieldAlignment(lines []string, lineToks map[int][]lexer.Token, fields []ast.Field) {
	var run []fieldRow
	flush := func() {
		if len(run) >= 2 {
			renderFieldRun(lines, run)
		}
		run = nil
	}
	prevLine := 0
	for _, f := range fields {
		if prevLine != 0 && hasBlankLineBetween(lines, prevLine, f.Line) {
			flush()
		}
		row, ok := buildFieldRow(lines, lineToks, f)
		if !ok {
			flush()
			prevLine = f.Line
			continue
		}
		run = append(run, row)
		prevLine = f.Line
	}
	flush()
}

func hasBlankLineBetween(lines []string, fromLine, toLine int) bool {
	for l := fromLine + 1; l < toLine; l++ {
		if l < 1 || l > len(lines) {
			continue
		}
		if strings.TrimSpace(lines[l-1]) == "" {
			return true
		}
	}
	return false
}

// buildFieldRow разбирает объявление поля f на ячейки будущей таблицы -
// граница между Тип/"= значение"/тег ищется по САМИМ токенам (первый "="
// после ":" - начало значения по умолчанию, TAG - всегда последний), а не
// по значению ast.Field.Type/Default (те - уже разобранные данные без
// исходного текста/отступов) - см. package doc alignEntityFields.
func buildFieldRow(lines []string, lineToks map[int][]lexer.Token, f ast.Field) (fieldRow, bool) {
	row := fieldRow{line: f.Line}
	if f.Line < 1 || f.Line > len(lines) {
		return row, false
	}
	toks := lineToks[f.Line]
	colonIdx := -1
	for i, t := range toks {
		if t.Type == lexer.COLON {
			colonIdx = i
			break
		}
	}
	if colonIdx == -1 {
		return row, false
	}
	assignIdx, tagIdx := -1, -1
	for i := colonIdx + 1; i < len(toks); i++ {
		switch toks[i].Type {
		case lexer.ASSIGN:
			if assignIdx == -1 {
				assignIdx = i
			}
		case lexer.TAG:
			tagIdx = i
		}
	}

	lineText := lines[f.Line-1]
	lineEndCol := len([]rune(lineText)) + 1

	typeEndCol := lineEndCol
	if assignIdx != -1 {
		typeEndCol = toks[assignIdx].Col
	} else if tagIdx != -1 {
		typeEndCol = toks[tagIdx].Col
	}
	defaultEndCol := lineEndCol
	if tagIdx != -1 {
		defaultEndCol = toks[tagIdx].Col
	}

	runeLine := []rune(lineText)
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

	// Ведущий отступ строки - ЧАСТЬ первой ячейки, не обрезается: "имеет:"
	// - единственная секция со значимым отступом (docs/SPEC.md,
	// internal/parser/entity.go, parseFieldsSection) - парсер (при
	// повторном разборе уже ВЫРОВНЕННОГО текста, см. Format) проверяет её
	// СРАЗУ, раньше, чем что-либо успевает пересчитать отступы заново -
	// обрезать этот отступ здесь значило бы сломать разбор файла, если он
	// был меньше "запаса", который потом восстановит computeLineDepths.
	row.cells[0] = slice(1, toks[colonIdx].Col+1)
	row.cells[1] = strings.TrimSpace(slice(toks[colonIdx].Col+1, typeEndCol))
	if assignIdx != -1 {
		row.cells[2] = strings.TrimSpace(slice(toks[assignIdx].Col, defaultEndCol))
	}
	if tagIdx != -1 {
		row.cells[3] = strings.TrimRight(slice(toks[tagIdx].Col, lineEndCol), " \t\r")
	}
	return row, true
}

// renderFieldRun выравнивает один пробег через text/tabwriter (padding=1,
// без выравнивания по правому краю) - именно он умеет рваные строки
// (ragged rows, разное число ячеек в разных строках пробега) корректно:
// столбец выравнивается по САМЫМ ДЛИННЫМ ячейкам среди строк, которые в
// нём вообще участвуют, у более коротких строк не появляется хвостовых
// пробелов сверх того, что нужно для сдвига следующей реальной ячейки.
func renderFieldRun(lines []string, run []fieldRow) {
	var buf strings.Builder
	tw := tabwriter.NewWriter(&buf, 0, 0, 1, ' ', 0)
	for _, row := range run {
		cells := row.cells[:]
		for len(cells) > 0 && cells[len(cells)-1] == "" {
			cells = cells[:len(cells)-1]
		}
		fmt.Fprintln(tw, strings.Join(cells, "\t"))
	}
	tw.Flush()

	rendered := strings.Split(strings.TrimSuffix(buf.String(), "\n"), "\n")
	if len(rendered) != len(run) {
		return // расхождение в числе строк - не рискуем испортить текст
	}
	for i, row := range run {
		lines[row.line-1] = rendered[i]
	}
}
