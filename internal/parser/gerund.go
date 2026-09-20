// Деепричастный вызов с блоком (шаг 6.5):
//
//	сложив(2, 3)
//	    вывести результат
//
// Деепричастная форма ("сложив") функции ("сложить") определяется в
// таком порядке: (1) явный тег `деепричастие:слово` у объявления функции
// (internal/ast.FunctionDecl.Tag) - переопределяет всё остальное; (2)
// автоматически через morph.GerundOf(имяФункции) - широкое использование
// gomorphy (в отличие от узкого, только род/одушевлённость, использования
// в остальном движке), надёжно совпадает с уже курируемыми значениями
// SystemVerbs на проверенных вручную глаголах (см. internal/morph/gerund.go
// и internal/morph/gerund_test.go). Функция, чьё имя не опознано ни одним
// из двух источников, просто не участвует в этом механизме - вызывается
// как обычно, без деепричастной формы вообще.
package parser

import (
	"github.com/IlyaFeoktistov/Rech/internal/ast"
	"github.com/IlyaFeoktistov/Rech/internal/diagnostics"
	"github.com/IlyaFeoktistov/Rech/internal/lexer"
	"github.com/IlyaFeoktistov/Rech/internal/morph"
	"github.com/IlyaFeoktistov/Rech/internal/structtag"
)

// registerGerundWord - см. package doc: вызывается для КАЖДОЙ объявленной
// функции (collectFunctionSignatures), не только для тех, что явно
// претендуют на деепричастную форму - молча ничего не делает, если ни тег,
// ни morph.GerundOf ничего не дали.
func (p *Parser) registerGerundWord(funcName, tag string) error {
	word := structtag.Parse(tag).Get("деепричастие")
	if word == "" {
		auto, ok := morph.GerundOf(funcName)
		if !ok {
			return nil
		}
		word = auto
	}
	if existing, exists := p.gerundWords[word]; exists && existing != funcName {
		return diagnostics.New(CodeDuplicateDecl, diagnostics.Position{}, "деепричастие %q уже зарегистрировано функцией %q - нельзя использовать его же для %q (переопределите одну из форм тегом `деепричастие:...`)",
			word, existing, funcName)
	}
	if p.gerundWords == nil {
		p.gerundWords = map[string]string{}
	}
	p.gerundWords[word] = funcName
	return nil
}

// parseGerundStmt разбирает "<деепричастие>(<арг>, ...)" и следующий за
// ним блок инструкций с бОльшим отступом (по столбцу - тот же приём, что и
// у секции "имеет:", parseFieldsSection). Позиция парсера на входе - на
// самой деепричастной форме (IDENT, уже проверенной вызывающей стороной
// как ключ p.gerundWords), lemma - имя функции, на которое она резолвится.
func (p *Parser) parseGerundStmt(lemma string) (ast.Stmt, error) {
	word := p.advance() // деепричастная форма
	if p.cur().Type != lexer.LPAREN {
		return nil, errAt(p.cur().Line, p.cur().Col, CodeUnexpectedToken, "после деепричастия %q ожидалось \"(\" (даже без аргументов - \"()\")", word.Value)
	}
	args, err := p.parseArgList()
	if err != nil {
		return nil, diagnostics.Wrap(err, "в аргументах деепричастного вызова %q", word.Value)
	}

	var body []ast.Stmt
	for {
		next := p.peekPastNewlines()
		if next.Col <= word.Col || next.Type == lexer.RBRACE || next.Type == lexer.EOF {
			break
		}
		if err := p.requireSeparator(); err != nil {
			return nil, diagnostics.Wrap(err, "после деепричастного вызова %q", word.Value)
		}
		stmt, err := p.parseStatement()
		if err != nil {
			return nil, err
		}
		body = append(body, stmt)
	}
	if len(body) == 0 {
		return nil, errAt(word.Line, word.Col, CodeUnexpectedToken, "после деепричастного вызова %q ожидался блок инструкций с бОльшим отступом", word.Value)
	}
	return &ast.GerundStmt{Callee: lemma, Args: args, Body: body, Line: word.Line}, nil
}
