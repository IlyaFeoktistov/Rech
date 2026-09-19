package interp

import "testing"

// TestRunTemplateLiteral - шаблонные литералы (обратные кавычки) целиком
// десахариваются парсером в StringLiteral/CastExpr/BinaryExpr (см.
// internal/parser/template.go) - интерпретатор не знает о них отдельно,
// этот тест проверяет результат исполнения этой десахаризации.
func TestRunTemplateLiteral(t *testing.T) {
	got := runSrc(t, "функция главная\n{\n    Целое айди = 42\n    Строка имя = \"Илья\"\n    вывести `Привет, ${имя}! айди=${айди}, дважды=${айди * 2}, булево=${айди > 0}\\n`\n}\n")
	want := "Привет, Илья! айди=42, дважды=84, булево=истина\n"
	if got != want {
		t.Fatalf("хотели %q, получили %q", want, got)
	}
}

// TestRunTemplateLiteralMultilineVerbatim - настоящий перенос строки внутри
// обратных кавычек сохраняется как есть (без нормализации отступов).
func TestRunTemplateLiteralMultilineVerbatim(t *testing.T) {
	got := runSrc(t, "функция главная\n{\n    вывести `первая\n    вторая\\n`\n}\n")
	want := "первая\n    вторая\n"
	if got != want {
		t.Fatalf("хотели %q, получили %q", want, got)
	}
}
