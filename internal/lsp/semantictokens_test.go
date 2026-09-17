package lsp

import "testing"

// TestSemanticTokensClassifiesCamelCaseFunctionAsFunction - вид имени
// определяется РЕАЛЬНЫМ объявлением (функция), а не регистром первой буквы
// (написана нарочно CamelCase - см. package doc semantictokens.go, "а если я
// захочу функции в CamelCase" - статическая TextMate-подсветка, завязанная
// на регистр, приняла бы это имя за тип/сущность).
func TestSemanticTokensClassifiesCamelCaseFunctionAsFunction(t *testing.T) {
	src := `
функция СложитьЧисла(а: Целое, б: Целое): Целое
{
    вернуть а + б
}
функция главная
{
    вывести СложитьЧисла(1, 2)
}
`
	d := newDoc("test://a.rech", src)
	toks := d.semanticTokens()

	declLine := lineOf(t, src, "функция СложитьЧисла")
	declCol := runeCol(t, "функция СложитьЧисла(а: Целое, б: Целое): Целое", "СложитьЧисла")
	assertSemToken(t, toks, declLine, declCol, stFunction)

	callLine := lineOf(t, src, "вывести СложитьЧисла(1, 2)")
	callCol := runeCol(t, "    вывести СложитьЧисла(1, 2)", "СложитьЧисла")
	assertSemToken(t, toks, callLine, callCol, stFunction)
}

// TestSemanticTokensClassifiesEntityFieldAsProperty - "т.x" внутри метода -
// "x" после точки - поле сущности "Точка" (hoverTestSrc), не переменная и
// не функция.
func TestSemanticTokensClassifiesEntityFieldAsProperty(t *testing.T) {
	d := newDoc("test://a.rech", hoverTestSrc)
	toks := d.semanticTokens()
	line := lineOf(t, hoverTestSrc, "вывести т.x")
	col := runeCol(t, "    вывести т.x", "x")
	assertSemToken(t, toks, line, col, stProperty)
}

// TestSemanticTokensClassifiesEntityConstantAsReadonlyProperty - "ПРЕДЕЛ" в
// "т.ПРЕДЕЛ" (constHoverSrc) - член сущности с модификатором "readonly",
// не обычное поле, даже написанный ЗАГЛАВНЫМИ (declPos/declLine самой
// "константа ПРЕДЕЛ" - keyword-анкерное правило TextMate, никогда не
// зависело от регистра - семантические токены его намеренно НЕ трогают,
// только "т.ПРЕДЕЛ" - см. package doc semantictokens.go).
func TestSemanticTokensClassifiesEntityConstantAsReadonlyProperty(t *testing.T) {
	d := newDoc("test://b.rech", constHoverSrc)
	toks := d.semanticTokens()
	line := lineOf(t, constHoverSrc, "вывести т.ПРЕДЕЛ")
	col := runeCol(t, "    вывести т.ПРЕДЕЛ", "ПРЕДЕЛ")
	tok := findSemToken(t, toks, line, col)
	if tok.Type != stProperty || tok.Mods&modReadonly == 0 {
		t.Fatalf("ожидалось readonly-свойство, получено %+v", tok)
	}
}

// TestSemanticTokensClassifiesGerundCallAsFunction - словоформа "сложив"
// (деепричастие) должна классифицироваться как функция "сложить", тем же
// путём, что definition/hover (wordform_definition.go).
func TestSemanticTokensClassifiesGerundCallAsFunction(t *testing.T) {
	d := newDoc("test://a.rech", gerundCallDefinitionSrc)
	toks := d.semanticTokens()
	line := lineOf(t, gerundCallDefinitionSrc, "сложив(2, 3)")
	col := runeCol(t, "    сложив(2, 3)", "сложив")
	assertSemToken(t, toks, line, col, stFunction)
}

func findSemToken(t *testing.T, toks []semToken, line, col int) semToken {
	t.Helper()
	for _, tok := range toks {
		if tok.Line == line && tok.Col == col {
			return tok
		}
	}
	t.Fatalf("не найден семантический токен на строке %d, столбце %d (все: %+v)", line, col, toks)
	return semToken{}
}

func assertSemToken(t *testing.T, toks []semToken, line, col, wantType int) {
	t.Helper()
	tok := findSemToken(t, toks, line, col)
	if tok.Type != wantType {
		t.Fatalf("строка %d, столбец %d: ожидался тип %d, получено %+v", line, col, wantType, tok)
	}
}

// TestEncodeSemanticTokensDeltaEncodesPositions - LSP-формат: пятёрки int,
// каждая ОТНОСИТЕЛЬНО предыдущего токена (deltaLine 0, если та же строка -
// тогда deltaStartChar тоже относительно предыдущего столбца, иначе
// абсолютный от начала строки) - см. encodeSemanticTokens.
func TestEncodeSemanticTokensDeltaEncodesPositions(t *testing.T) {
	data := encodeSemanticTokens([]semToken{
		{Line: 2, Col: 5, Len: 3, Type: stFunction, Mods: 0},
		{Line: 2, Col: 12, Len: 4, Type: stVariable, Mods: modReadonly},
		{Line: 4, Col: 1, Len: 2, Type: stType, Mods: 0},
	})
	want := []int{
		1, 4, 3, stFunction, 0, // строка 2 (0-индексная 1), столбец 5 (0-индексный 4)
		0, 7, 4, stVariable, modReadonly, // та же строка - deltaStartChar = 12-5 = 7
		2, 0, 2, stType, 0, // строка 4 (0-индексная 3) - deltaLine = 3-1 = 2, столбец 0
	}
	if len(data) != len(want) {
		t.Fatalf("ожидалась длина %d, получено %d (%v)", len(want), len(data), data)
	}
	for i := range want {
		if data[i] != want[i] {
			t.Fatalf("data[%d] = %d, ожидалось %d (весь массив %v, ожидалось %v)", i, data[i], want[i], data, want)
		}
	}
}
