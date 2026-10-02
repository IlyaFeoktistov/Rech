// Множественный выбор:
//
//	выбрать число
//	{
//	    когда 1:
//	        вывести "один"
//	    когда 2:
//	        вывести "два"
//	    иначе:
//	        вывести "неизвестное число"
//	}
package parser

import (
	"github.com/IlyaFeoktistov/Rech/internal/ast"
	"github.com/IlyaFeoktistov/Rech/internal/diagnostics"
	"github.com/IlyaFeoktistov/Rech/internal/lexer"
)

// parseSwitchStmt разбирает "выбрать <выражение> { когда ... иначе ... }".
// Позиция парсера на входе - на "выбрать".
func (p *Parser) parseSwitchStmt() (ast.Stmt, error) {
	kw, err := p.expect(lexer.KwSwitch)
	if err != nil {
		return nil, err
	}
	subject, err := p.parseExpr()
	if err != nil {
		return nil, diagnostics.Wrap(err, "после \"выбрать\" ожидалось выражение")
	}
	p.skipNewlines()
	if _, err := p.expect(lexer.LBRACE); err != nil {
		return nil, err
	}
	p.skipSeparators()

	var cases []ast.SwitchCase
	var defaultBody []ast.Stmt
	sawDefault := false
	for p.cur().Type != lexer.RBRACE {
		switch p.cur().Type {
		case lexer.KwWhen:
			whenTok := p.advance()
			values, err := p.parseSwitchValues()
			if err != nil {
				return nil, err
			}
			if _, err := p.expect(lexer.COLON); err != nil {
				return nil, diagnostics.Wrap(err, "после условия \"когда\" ожидалось \":\"")
			}
			p.skipSeparators()
			body, err := p.parseSwitchCaseBody()
			if err != nil {
				return nil, err
			}
			if len(body) == 0 {
				return nil, errAtLine(whenTok.Line, CodeSwitch, "пустая ветка \"когда\" - нужна хотя бы одна инструкция")
			}
			cases = append(cases, ast.SwitchCase{Values: values, Body: body, Line: whenTok.Line})

		case lexer.KwElse:
			if sawDefault {
				return nil, errAtLine(p.cur().Line, CodeDuplicateDecl, "повторное \"иначе\" внутри \"выбрать\" - допустима только одна ветка по умолчанию")
			}
			sawDefault = true
			p.advance()
			if _, err := p.expect(lexer.COLON); err != nil {
				return nil, diagnostics.Wrap(err, "после \"иначе\" внутри \"выбрать\" ожидалось \":\"")
			}
			p.skipSeparators()
			body, err := p.parseSwitchCaseBody()
			if err != nil {
				return nil, err
			}
			defaultBody = body

		case lexer.EOF:
			return nil, errAtLine(p.cur().Line, CodeUnclosedBracket, "не найдена закрывающая } у \"выбрать\"")

		default:
			return nil, errAt(p.cur().Line, p.cur().Col, CodeSwitch, "внутри \"выбрать { ... }\" ожидалось \"когда\" или \"иначе\", получено %q", p.cur().Value)
		}
		p.skipSeparators()
	}
	if _, err := p.expect(lexer.RBRACE); err != nil {
		return nil, err
	}
	if len(cases) == 0 {
		return nil, errAtLine(kw.Line, CodeSwitch, "\"выбрать\" должен содержать хотя бы одну ветку \"когда\"")
	}
	return &ast.SwitchStmt{Subject: subject, Cases: cases, Default: defaultBody, Line: kw.Line}, nil
}

// parseSwitchValues разбирает список значений одной ветки "когда",
// разделённых "|" ("когда 1|2|3:"). Каждое значение разбирается через
// parseBitXor, на один уровень приоритета ВЫШЕ побитового "|"
// (parseBitOr) - иначе "1|2|3" целиком поглотился бы обычной грамматикой
// выражений как одно побитовое ИЛИ (тот же токен lexer.BITOR обслуживает обе
// роли - разделитель списка здесь и оператор внутри обычного выражения,
// поэтому различить их можно только выбором уровня, с которого начинается
// разбор одного элемента, - ровно как список типов в "поймать(имя: Тип|Тип)",
// internal/parser/exception.go, только там элемент - имя типа, а не
// произвольное выражение).
func (p *Parser) parseSwitchValues() ([]ast.Expr, error) {
	first, err := p.parseBitXor()
	if err != nil {
		return nil, diagnostics.Wrap(err, "после \"когда\" ожидалось значение")
	}
	values := []ast.Expr{first}
	for {
		// Перенос строки перед "|" допустим (длинный список значений одной
		// ветки "когда" удобно разбивать по одному на строку) - только
		// если СЛЕДУЮЩИЙ значимый токен действительно "|", иначе перенос
		// принадлежит завершению самой ветки (после значения сразу должно
		// идти ":") и съедать его нельзя - иначе настоящая ошибка
		// (пропущенное ":") превратилась бы в непонятную из другого места.
		if p.cur().Type != lexer.BITOR {
			if p.cur().Type != lexer.NEWLINE || p.peekPastNewlines().Type != lexer.BITOR {
				break
			}
			p.skipNewlines()
		}
		p.advance() // "|"
		p.skipNewlines()
		v, err := p.parseBitXor()
		if err != nil {
			return nil, diagnostics.Wrap(err, "после \"|\" в \"когда\" ожидалось ещё одно значение")
		}
		values = append(values, v)
	}
	return values, nil
}

// parseSwitchCaseBody разбирает инструкции одной ветки "когда"/"иначе" -
// до следующей такой ветки или закрывающей "}" самого "выбрать" (без
// собственных фигурных скобок у ветки, см. package doc).
func (p *Parser) parseSwitchCaseBody() ([]ast.Stmt, error) {
	var stmts []ast.Stmt
	for {
		t := p.cur().Type
		if t == lexer.KwWhen || t == lexer.KwElse || t == lexer.RBRACE || t == lexer.EOF {
			break
		}
		var stmt ast.Stmt
		var err error
		if p.cur().Type == lexer.DOC_COMMENT {
			stmt, err = p.parseDocumentedStatement()
		} else {
			stmt, err = p.parseStatement()
		}
		if err != nil {
			return nil, err
		}
		if stmt != nil {
			stmts = append(stmts, stmt)
		}
		t = p.cur().Type
		if t == lexer.KwWhen || t == lexer.KwElse || t == lexer.RBRACE {
			break
		}
		if err := p.requireSeparator(); err != nil {
			return nil, err
		}
	}
	return stmts, nil
}
