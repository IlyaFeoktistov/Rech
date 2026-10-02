package parser

import (
	"strings"
	"testing"

	"github.com/IlyaFeoktistov/Rech/internal/ast"
	"github.com/IlyaFeoktistov/Rech/internal/lexer"
)

// TestParseTemplateLiteralNoInterpolation - без "${...}" шаблонный литерал -
// просто StringLiteral, без BinaryExpr("+")-обёртки.
func TestParseTemplateLiteralNoInterpolation(t *testing.T) {
	prog, err := parseSource(t, "функция главная {\n    переменная а = `просто текст`\n}\n")
	if err != nil {
		t.Fatalf("неожиданная ошибка: %v", err)
	}
	decl := prog.Functions[0].Body[0].(*ast.VarDecl)
	lit, ok := decl.Value.(*ast.StringLiteral)
	if !ok || lit.Value != "просто текст" {
		t.Fatalf("ожидался StringLiteral(\"просто текст\"), получено %+v", decl.Value)
	}
}

// TestParseTemplateLiteralSingleInterpolation - "текст ${выражение}"
// разбирается в StringLiteral + CastExpr(Строка), склеенные BinaryExpr("+").
func TestParseTemplateLiteralSingleInterpolation(t *testing.T) {
	prog, err := parseSource(t, "функция главная {\n    переменная а = `привет, ${имя}!`\n}\n")
	if err != nil {
		t.Fatalf("неожиданная ошибка: %v", err)
	}
	decl := prog.Functions[0].Body[0].(*ast.VarDecl)
	outer, ok := decl.Value.(*ast.BinaryExpr)
	if !ok || outer.Op != "+" {
		t.Fatalf("ожидался внешний BinaryExpr(\"+\"), получено %+v", decl.Value)
	}
	inner, ok := outer.Left.(*ast.BinaryExpr)
	if !ok || inner.Op != "+" {
		t.Fatalf("ожидался внутренний BinaryExpr(\"+\"), получено %+v", outer.Left)
	}
	prefix, ok := inner.Left.(*ast.StringLiteral)
	if !ok || prefix.Value != "привет, " {
		t.Fatalf("ожидался StringLiteral(\"привет, \"), получено %+v", inner.Left)
	}
	cast, ok := inner.Right.(*ast.CastExpr)
	if !ok || cast.Type != "Строка" {
		t.Fatalf("ожидался CastExpr(Строка), получено %+v", inner.Right)
	}
	ident, ok := cast.Value.(*ast.Identifier)
	if !ok || ident.Name != "имя" {
		t.Fatalf("ожидался Identifier(имя) внутри вставки, получено %+v", cast.Value)
	}
	suffix, ok := outer.Right.(*ast.StringLiteral)
	if !ok || suffix.Value != "!" {
		t.Fatalf("ожидался StringLiteral(\"!\"), получено %+v", outer.Right)
	}
}

// TestParseTemplateLiteralPreservesNewlinesVerbatim - настоящий перенос
// строки в исходнике переходит в значение как есть, без какой-либо
// нормализации отступов/пробелов.
func TestParseTemplateLiteralPreservesNewlinesVerbatim(t *testing.T) {
	prog, err := parseSource(t, "функция главная {\n    переменная а = `первая\n    вторая`\n}\n")
	if err != nil {
		t.Fatalf("неожиданная ошибка: %v", err)
	}
	decl := prog.Functions[0].Body[0].(*ast.VarDecl)
	lit, ok := decl.Value.(*ast.StringLiteral)
	want := "первая\n    вторая"
	if !ok || lit.Value != want {
		t.Fatalf("хотели StringLiteral(%q), получили %+v", want, decl.Value)
	}
}

// TestParseTemplateLiteralExpressionWithNestedQuotesAndBraces - вставка
// вправе содержать свои собственные "..."/{...} (например строковый
// литерал или литерал сущности) - splitTemplateParts не должен спутать их
// с границей вставки.
func TestParseTemplateLiteralExpressionWithNestedQuotesAndBraces(t *testing.T) {
	prog, err := parseSource(t, `функция главная {
    переменная а = `+"`{\"id\": ${айди}, \"имя\": \"${имя}\"}`"+`
}
`)
	if err != nil {
		t.Fatalf("неожиданная ошибка: %v", err)
	}
	decl := prog.Functions[0].Body[0].(*ast.VarDecl)
	if _, ok := decl.Value.(*ast.BinaryExpr); !ok {
		t.Fatalf("ожидался BinaryExpr, получено %+v", decl.Value)
	}
}

// TestParseTemplateLiteralUnclosedInterpolationFails - "${" без парной "}"
// должно быть синтаксической ошибкой, а не молча съеденным текстом.
func TestParseTemplateLiteralUnclosedInterpolationFails(t *testing.T) {
	_, err := lexer.New("функция главная {\n    переменная а = `текст ${незакрыто`\n}\n").Tokenize()
	if err == nil || !strings.Contains(err.Error(), "не закрыта символом") {
		t.Fatalf("ожидалась ошибка незакрытой вставки, получено: %v", err)
	}
}

// TestParseTemplateLiteralEscapesProcessed - экранирования \n/\t/\`/\$
// внутри шаблонного литерала обрабатываются лексером (readTag), парсер
// шаблонных литералов их уже не видит.
func TestParseTemplateLiteralEscapesProcessed(t *testing.T) {
	prog, err := parseSource(t, "функция главная {\n    переменная а = `цена: \\$5, таб:\\t`\n}\n")
	if err != nil {
		t.Fatalf("неожиданная ошибка: %v", err)
	}
	decl := prog.Functions[0].Body[0].(*ast.VarDecl)
	lit, ok := decl.Value.(*ast.StringLiteral)
	want := "цена: $5, таб:\t"
	if !ok || lit.Value != want {
		t.Fatalf("хотели StringLiteral(%q), получили %+v", want, decl.Value)
	}
}

// TestParseTemplateLiteralInterpolationKeepsSourceLine - узлы внутри
// "${...}" несут строку самого литерала, не строку внутри вставки (1).
func TestParseTemplateLiteralInterpolationKeepsSourceLine(t *testing.T) {
	prog, err := parseSource(t, "функция главная {\n    переменная а = 1\n\n    переменная б = `x ${ф(а)}`\n}\n")
	if err != nil {
		t.Fatalf("неожиданная ошибка: %v", err)
	}
	decl := prog.Functions[0].Body[1].(*ast.VarDecl)
	cast := decl.Value.(*ast.BinaryExpr).Right.(*ast.CastExpr)
	call, ok := cast.Value.(*ast.CallExpr)
	if !ok || call.Line != 4 {
		t.Fatalf("ожидался CallExpr на строке 4, получено %+v", cast.Value)
	}
}

// TestParseTemplateLiteralThisInsideMethod - вставка в теле метода видит
// "этот" (контекст объемлющего кода).
func TestParseTemplateLiteralThisInsideMethod(t *testing.T) {
	_, err := parseSource(t, "сущность Т\n{\n    имеет:\n        Целое х\n\n    публичный показ(): Строка\n    {\n        вернуть `х=${этот.х}`\n    }\n}\n")
	if err != nil {
		t.Fatalf("неожиданная ошибка: %v", err)
	}
}

// TestParseTemplateNestedInsertionErrorPosition - ошибка во вставке
// вложенного шаблона указывает на её место в исходнике, а не на начало
// файла.
func TestParseTemplateNestedInsertionErrorPosition(t *testing.T) {
	_, err := parseSource(t, "функция главная\n{\n    переменная х = 5\n    вывести `a ${`b ${х +} c`} d`\n}\n")
	if err == nil || !strings.Contains(err.Error(), "строка 4") {
		t.Fatalf("ожидалась ошибка на строке 4, получено %v", err)
	}
}
