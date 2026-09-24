package diagnostics

import (
	"fmt"
	"strconv"
	"strings"

	"github.com/IlyaFeoktistov/Rech/internal/ir"
)

// sourceLine возвращает line-ю строку (1-индексную) src без завершающего
// "\n"/"\r\n" - "", false, если файл короче line строк (позиция могла
// оказаться указывающей на несуществующую строку из-за расхождения между
// тем, что реально лежит на диске, и тем, что видел компилятор - например,
// файл изменился между чтением и печатью диагностики - тогда Render просто
// не печатает сниппет, а не паникует/печатает мусор).
func sourceLine(src string, line int) (string, bool) {
	if line < 1 {
		return "", false
	}
	// Разбивка построчно (не sort.Search по индексам "\n") - исходники
	// Rech-программ - это файлы с исходным кодом, не гигабайтные логи,
	// цена одного strings.Split на печать диагностики не имеет значения.
	lines := strings.Split(src, "\n")
	idx := line - 1
	if idx >= len(lines) {
		return "", false
	}
	return strings.TrimSuffix(lines[idx], "\r"), true
}

// Render печатает одну диагностику в стиле "rustc"/"go vet": заголовок с
// кодом и позицией, сниппет исходной строки, каретка-указатель под
// колонкой (в рунах - см. Position.Col), необязательная строка
// "подсказка: ...". src - содержимое ИМЕННО того файла, что назван в
// d.Pos.File ("" допустимо, если позиция не привязана к файлу - код без
// файла-источника, см. package doc Position.File) - вызывающая сторона
// сама решает, что читать, Render файлов не открывает (см. RenderAll,
// которая берёт src из уже прочитанной карты).
func Render(d *Diagnostic, src string) string {
	var b strings.Builder
	b.WriteString(header(d))
	b.WriteByte('\n')

	if d.Pos.Line > 0 {
		if line, ok := sourceLine(src, d.Pos.Line); ok {
			writeSnippet(&b, d.Pos.Line, d.Pos.Col, line)
		}
	}

	if d.Hint != "" {
		b.WriteString("подсказка: ")
		b.WriteString(ir.DisplayTypes(d.Hint))
		b.WriteByte('\n')
	}
	return b.String()
}

func header(d *Diagnostic) string {
	sev := d.Severity.String()
	code := ""
	if d.Code != "" {
		code = "[" + d.Code + "]"
	}
	loc := ""
	if d.Pos.Line > 0 {
		file := d.Pos.File
		if file == "" {
			file = "<код>"
		}
		if d.Pos.Col > 0 {
			loc = fmt.Sprintf("\n  --> %s:%d:%d", file, d.Pos.Line, d.Pos.Col)
		} else {
			loc = fmt.Sprintf("\n  --> %s:%d", file, d.Pos.Line)
		}
	}
	return fmt.Sprintf("%s%s: %s%s", sev, code, ir.DisplayTypes(d.Message), loc)
}

// writeSnippet пишет строку исходника с префиксом номера строки (тем же
// приёмом, что и "go vet"/rustc - гутер выровнен по ширине самого большого
// номера строки, здесь он один - ширина фиксирована по этому единственному
// номеру) и, если col > 0, строку-каретку под нужной колонкой. col - в
// рунах (см. package doc Position.Col) - строка режется на []rune, чтобы
// каретка совпала с реальным символом при кириллице/любой многобайтовой
// UTF-8 (индекс руны в срезе строки ≠ байтовый offset).
func writeSnippet(b *strings.Builder, lineNo, col int, line string) {
	gutter := strconv.Itoa(lineNo)
	pad := strings.Repeat(" ", len(gutter))

	fmt.Fprintf(b, " %s |\n", pad)
	fmt.Fprintf(b, " %s | %s\n", gutter, line)

	if col <= 0 {
		return
	}
	runes := []rune(line)
	if col > len(runes)+1 {
		return
	}
	caret := strings.Repeat(" ", col-1) + "^"
	fmt.Fprintf(b, " %s | %s\n", pad, caret)
}

// RenderAll печатает несколько диагностик подряд с заголовком "N ошибок" -
// используется, когда парсер/семантика накопили больше одной (см.
// internal/diagnostics.Bag) - sources - содержимое исходников по
// Position.File, прочитанных ОДИН раз вызывающей стороной (cmd/rech)
// заранее, не заново под каждую диагностику; "" - код без файла-источника
// (см. package doc Position.File), sources[""] тогда не используется - у
// такой диагностики Render всё равно не печатает сниппет без файла
// (BuildSource передаёт исходник только диагностикам ПЕРВОГО файла, у
// -c "<код>" файла как такового нет).
func RenderAll(diags []*Diagnostic, sources map[string]string) string {
	var b strings.Builder
	for i, d := range diags {
		if i > 0 {
			b.WriteByte('\n')
		}
		b.WriteString(Render(d, sources[d.Pos.File]))
	}
	if len(diags) > 1 {
		fmt.Fprintf(&b, "\n%s\n", pluralDiagCount(len(diags)))
	}
	return b.String()
}

// pluralDiagCount склоняет "ошибка" по числу (1 ошибка / 2 ошибки / 5
// ошибок) - тем же принципом, что и остальной русскоязычный вывод
// компилятора (internal/morph занимается склонением идентификаторов
// программы, не служебных сообщений CLI - здесь достаточно ручного
// правила по последней цифре, диагностик за один прогон никогда не
// тысячи).
func pluralDiagCount(n int) string {
	word := "ошибок"
	switch {
	case n%10 == 1 && n%100 != 11:
		word = "ошибка"
	case n%10 >= 2 && n%10 <= 4 && (n%100 < 10 || n%100 >= 20):
		word = "ошибки"
	}
	return fmt.Sprintf("%d %s", n, word)
}
