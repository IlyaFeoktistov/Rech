// Package parser строит ast.Program из потока токенов лексера.
//
// Грамматика v0.1 (расширяется по этапам из docs/ROADMAP.md):
//
//	program    := { newline } { function { newline } }
//	function   := "функция" IDENT newline block
//	block      := "{" newline { statement } "}"
//	statement  := print_stmt
//	print_stmt := "вывести" STRING newline
package parser

import (
	"fmt"

	"github.com/IlyaFeoktistov/Rech/internal/ast"
	"github.com/IlyaFeoktistov/Rech/internal/lexer"
)

type Parser struct {
	tokens []lexer.Token
	pos    int
}

func New(tokens []lexer.Token) *Parser {
	return &Parser{tokens: tokens}
}

func (p *Parser) cur() lexer.Token {
	return p.tokens[p.pos]
}

func (p *Parser) advance() lexer.Token {
	t := p.tokens[p.pos]
	if p.pos < len(p.tokens)-1 {
		p.pos++
	}
	return t
}

func (p *Parser) expect(tt lexer.TokenType) (lexer.Token, error) {
	if p.cur().Type != tt {
		return lexer.Token{}, fmt.Errorf("строка %d, столбец %d: ожидался %s, получен %q",
			p.cur().Line, p.cur().Col, tt, p.cur().Value)
	}
	return p.advance(), nil
}

func (p *Parser) skipNewlines() {
	for p.cur().Type == lexer.NEWLINE {
		p.advance()
	}
}

// ParseProgram - точка входа парсера.
func (p *Parser) ParseProgram() (*ast.Program, error) {
	prog := &ast.Program{}
	p.skipNewlines()
	for p.cur().Type != lexer.EOF {
		fn, err := p.parseFunction()
		if err != nil {
			return nil, err
		}
		prog.Functions = append(prog.Functions, fn)
		p.skipNewlines()
	}
	if len(prog.Functions) == 0 {
		return nil, fmt.Errorf("программа не содержит ни одной функции")
	}
	return prog, nil
}

func (p *Parser) parseFunction() (*ast.FunctionDecl, error) {
	kw, err := p.expect(lexer.KwFunction)
	if err != nil {
		return nil, err
	}
	name, err := p.expect(lexer.IDENT)
	if err != nil {
		return nil, fmt.Errorf("после \"функция\" ожидалось имя функции: %w", err)
	}
	p.skipNewlines()
	body, err := p.parseBlock()
	if err != nil {
		return nil, err
	}
	return &ast.FunctionDecl{Name: name.Value, Body: body, Line: kw.Line}, nil
}

func (p *Parser) parseBlock() ([]ast.Stmt, error) {
	if _, err := p.expect(lexer.LBRACE); err != nil {
		return nil, err
	}
	p.skipNewlines()

	var stmts []ast.Stmt
	for p.cur().Type != lexer.RBRACE {
		if p.cur().Type == lexer.EOF {
			return nil, fmt.Errorf("строка %d: не найдена закрывающая }", p.cur().Line)
		}
		stmt, err := p.parseStatement()
		if err != nil {
			return nil, err
		}
		stmts = append(stmts, stmt)
		p.skipNewlines()
	}
	if _, err := p.expect(lexer.RBRACE); err != nil {
		return nil, err
	}
	return stmts, nil
}

func (p *Parser) parseStatement() (ast.Stmt, error) {
	switch p.cur().Type {
	case lexer.KwPrint:
		return p.parsePrintStmt()
	default:
		return nil, fmt.Errorf("строка %d, столбец %d: неожиданный токен %q в теле функции",
			p.cur().Line, p.cur().Col, p.cur().Value)
	}
}

func (p *Parser) parsePrintStmt() (ast.Stmt, error) {
	kw, err := p.expect(lexer.KwPrint)
	if err != nil {
		return nil, err
	}
	value, err := p.parseExpr()
	if err != nil {
		return nil, fmt.Errorf("после \"вывести\" ожидалось выражение: %w", err)
	}
	return &ast.PrintStmt{Value: value, Line: kw.Line}, nil
}

// parseExpr в v0.1 понимает только строковые литералы.
// Арифметика, переменные и сравнения - этап "Выражения" в docs/ROADMAP.md.
func (p *Parser) parseExpr() (ast.Expr, error) {
	switch p.cur().Type {
	case lexer.STRING:
		tok := p.advance()
		return &ast.StringLiteral{Value: tok.Value, Line: tok.Line}, nil
	default:
		return nil, fmt.Errorf("строка %d, столбец %d: ожидался строковый литерал, получен %q",
			p.cur().Line, p.cur().Col, p.cur().Value)
	}
}
