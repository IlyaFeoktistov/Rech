package format

import (
	"strings"

	"github.com/IlyaFeoktistov/Rech/internal/lexer"
)

// canonicalizeDocComments приводит МНОГОСТРОЧНЫЙ doc-комментарий
// ("/** ... */", хотя бы один перенос строки внутри) к канонической форме
// Javadoc/JSDoc - "/**" само по себе на первой строке, каждая внутренняя
// строка - " * <текст>" (пустая - просто " *", без хвостового пробела),
// закрывающая "*/" - на своей собственной строке ("     */" получит
// правильный отступ позже, при пересчёте отступов - здесь важен только сам
// факт, что она отдельная строка):
//
//	/**
//	 * <текст 1>
//	 * <текст 2>
//	 */
//
// Пользователь попросил явно - "докблоки должны тоже сами
// форматироваться". ОДНОСТРОЧНЫЙ doc-комментарий ("/** Коротко. */", без
// единого переноса строки внутри - подавляющее большинство комментариев в
// этом репозитории) НЕ трогается вовсе - взрывать их в блок ради
// единообразия было бы масштабным переформатированием, которого не просили
// (см. package doc format.go про "не переформатировать походя").
//
// Работает НАД ИСХОДНЫМ ТЕКСТОМ, ДО остальных проходов - заменяет диапазон
// физических строк, которые занимал токен, на новый (другой длины) набор
// строк; собственный отступ каждой новой строки не важен -
// computeLineDepths пересчитывает его с нуля позже, как и у остальных
// проходов этого пакета.
func canonicalizeDocComments(src string) (string, error) {
	toks, err := lexer.New(src).Tokenize()
	if err != nil {
		return "", err
	}

	normalized := strings.ReplaceAll(src, "\r\n", "\n")
	lines := strings.Split(normalized, "\n")

	type replacement struct {
		startLine, endLine int
		newLines           []string
	}
	var reps []replacement
	for _, t := range toks {
		if t.Type != lexer.DOC_COMMENT || !strings.Contains(t.Value, "\n") {
			continue
		}
		endLine := t.Line + strings.Count(t.Value, "\n")
		reps = append(reps, replacement{
			startLine: t.Line, endLine: endLine,
			newLines: strings.Split(canonicalDocCommentText(t.Value), "\n"),
		})
	}
	if len(reps) == 0 {
		return src, nil
	}

	var out []string
	cur := 1
	for _, r := range reps {
		for cur < r.startLine {
			out = append(out, lines[cur-1])
			cur++
		}
		out = append(out, r.newLines...)
		cur = r.endLine + 1
	}
	for cur <= len(lines) {
		out = append(out, lines[cur-1])
		cur++
	}
	return strings.Join(out, "\n") + "\n", nil
}

// canonicalDocCommentText - полный текст док-комментария (включая "/**"/
// "*/" самих по себе) в канонической форме - см. package doc выше. value -
// сырой текст МЕЖДУ "/**" и "*/" (lexer.Token.Value, internal/lexer/
// lexer.go, readDocComment) - каждая его физическая строка (включая
// первую, идущую сразу за "/**" в исходнике, и последнюю, идущую сразу
// перед "*/") становится одной "содержательной" строкой блока, отбросив
// прежнее авторское оформление ("*"/пробелы в начале) и наложив заново
// единообразное.
func canonicalDocCommentText(value string) string {
	raw := strings.Split(value, "\n")
	var b strings.Builder
	b.WriteString("/**\n")
	for i, l := range raw {
		content := normalizeDocCommentLine(l)
		// Первая строка (сразу за "/**" в исходнике) и последняя (сразу
		// перед "*/") - если ПУСТА, это просто перенос строки у самого
		// разделителя, не настоящий пустой абзац содержимого - её незачем
		// рендерить отдельной " *" строкой (иначе появлялась бы лишняя
		// пустая строка, которой в исходнике не было). Пустая строка
		// СРЕДИ содержимого - настоящий разрыв абзаца автора, сохраняется.
		if content == "" && (i == 0 || i == len(raw)-1) {
			continue
		}
		if content == "" {
			b.WriteString(" *\n")
		} else {
			b.WriteString(" * ")
			b.WriteString(content)
			b.WriteString("\n")
		}
	}
	b.WriteString(" */")
	return b.String()
}

// normalizeDocCommentLine - "текст" одной строки doc-комментария, без
// прежнего авторского оформления: ведущие пробелы/табы, затем один ведущий
// "*" (если был) и один пробел сразу после него (если был) - отброшены;
// хвостовые пробелы - тоже. Работает одинаково для ЛЮБОЙ строки блока
// (первой/внутренней/последней) - у всех этот же смысл "что автор здесь
// написал", просто первая до этого стояла сразу за "/**" в исходнике, а
// не под своим собственным "*".
func normalizeDocCommentLine(l string) string {
	t := strings.TrimLeft(l, " \t")
	t = strings.TrimRight(t, " \t\r")
	if strings.HasPrefix(t, "*") {
		t = strings.TrimPrefix(t[1:], " ")
	}
	return t
}
