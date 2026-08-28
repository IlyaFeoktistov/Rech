package lexer

import "testing"

// TestLexerSkipsLeadingBOM - "UTF-8 with BOM" (типичное сохранение файла на
// Windows, в т. ч. PowerShell "Out-File" по умолчанию) не должен ронять
// разбор первой строки - см. package doc New.
func TestLexerSkipsLeadingBOM(t *testing.T) {
	toks, err := New("\uFEFFвывести 1").Tokenize()
	if err != nil {
		t.Fatalf("неожиданная ошибка: %v", err)
	}
	if len(toks) == 0 || toks[0].Type != KwPrint {
		t.Fatalf("ожидался первый токен KwPrint, получено: %+v", toks)
	}
}
