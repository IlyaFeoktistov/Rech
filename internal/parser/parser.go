// Package parser строит ast.Program из потока токенов лексера.
//
// Грамматика v0.1 (расширяется по этапам из docs/ROADMAP.md, подробный
// EBNF-набросок - docs/GRAMMAR.md):
//
//	program     := { newline } { function { newline } }
//	function    := "функция" IDENT newline block
//	block       := "{" newline { statement } "}"
//	statement   := var_decl | assign | print_stmt
//	var_decl    := "переменная" IDENT "=" expr
//	assign      := IDENT "=" expr
//	print_stmt  := "вывести" expr
//
//	expr        := or_expr
//	or_expr     := and_expr { "или" and_expr }
//	and_expr    := not_expr { "и" not_expr }
//	not_expr    := "не" not_expr | comparison
//	comparison  := additive [ ("==" | "!=" | "<" | ">" | "<=" | ">=") additive ]
//	additive    := multiplicative { ("+" | "-") multiplicative }
//	multiplicative := unary { ("*" | "/") unary }
//	unary       := "-" unary | primary
//	primary     := INT | FLOAT | STRING | "истина" | "ложь" | IDENT | "(" expr ")"
package parser

import (
	"fmt"
	"strconv"

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

func (p *Parser) peekAt(offset int) lexer.Token {
	idx := p.pos + offset
	if idx >= len(p.tokens) {
		return p.tokens[len(p.tokens)-1] // EOF
	}
	return p.tokens[idx]
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
	case lexer.KwVariable:
		return p.parseVarDecl()
	case lexer.IDENT:
		if p.peekAt(1).Type == lexer.ASSIGN {
			return p.parseAssign()
		}
		return nil, fmt.Errorf("строка %d, столбец %d: неожиданный токен %q в теле функции",
			p.cur().Line, p.cur().Col, p.cur().Value)
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

func (p *Parser) parseVarDecl() (ast.Stmt, error) {
	kw, err := p.expect(lexer.KwVariable)
	if err != nil {
		return nil, err
	}
	name, err := p.expect(lexer.IDENT)
	if err != nil {
		return nil, fmt.Errorf("после \"переменная\" ожидалось имя переменной: %w", err)
	}
	if _, err := p.expect(lexer.ASSIGN); err != nil {
		return nil, fmt.Errorf("объявление переменной %q должно содержать инициализацию (\"=\"): %w", name.Value, err)
	}
	value, err := p.parseExpr()
	if err != nil {
		return nil, fmt.Errorf("в инициализации переменной %q: %w", name.Value, err)
	}
	return &ast.VarDecl{Name: name.Value, Value: value, Line: kw.Line}, nil
}

func (p *Parser) parseAssign() (ast.Stmt, error) {
	name := p.advance()
	if _, err := p.expect(lexer.ASSIGN); err != nil {
		return nil, err
	}
	value, err := p.parseExpr()
	if err != nil {
		return nil, fmt.Errorf("в присваивании %q: %w", name.Value, err)
	}
	return &ast.Assign{Name: name.Value, Value: value, Line: name.Line}, nil
}

func (p *Parser) parseExpr() (ast.Expr, error) {
	return p.parseOr()
}

func (p *Parser) parseOr() (ast.Expr, error) {
	left, err := p.parseAnd()
	if err != nil {
		return nil, err
	}
	for p.cur().Type == lexer.KwOr {
		op := p.advance()
		right, err := p.parseAnd()
		if err != nil {
			return nil, err
		}
		left = &ast.BinaryExpr{Op: "или", Left: left, Right: right, Line: op.Line}
	}
	return left, nil
}

func (p *Parser) parseAnd() (ast.Expr, error) {
	left, err := p.parseNot()
	if err != nil {
		return nil, err
	}
	for p.cur().Type == lexer.KwAnd {
		op := p.advance()
		right, err := p.parseNot()
		if err != nil {
			return nil, err
		}
		left = &ast.BinaryExpr{Op: "и", Left: left, Right: right, Line: op.Line}
	}
	return left, nil
}

func (p *Parser) parseNot() (ast.Expr, error) {
	if p.cur().Type == lexer.KwNot {
		op := p.advance()
		operand, err := p.parseNot()
		if err != nil {
			return nil, err
		}
		return &ast.UnaryExpr{Op: "не", Operand: operand, Line: op.Line}, nil
	}
	return p.parseComparison()
}

var comparisonOps = map[lexer.TokenType]string{
	lexer.EQ:  "==",
	lexer.NEQ: "!=",
	lexer.LT:  "<",
	lexer.GT:  ">",
	lexer.LE:  "<=",
	lexer.GE:  ">=",
}

func (p *Parser) parseComparison() (ast.Expr, error) {
	left, err := p.parseAdditive()
	if err != nil {
		return nil, err
	}
	if opStr, ok := comparisonOps[p.cur().Type]; ok {
		op := p.advance()
		right, err := p.parseAdditive()
		if err != nil {
			return nil, err
		}
		return &ast.BinaryExpr{Op: opStr, Left: left, Right: right, Line: op.Line}, nil
	}
	return left, nil
}

func (p *Parser) parseAdditive() (ast.Expr, error) {
	left, err := p.parseMultiplicative()
	if err != nil {
		return nil, err
	}
	for p.cur().Type == lexer.PLUS || p.cur().Type == lexer.MINUS {
		op := p.advance()
		right, err := p.parseMultiplicative()
		if err != nil {
			return nil, err
		}
		opStr := "+"
		if op.Type == lexer.MINUS {
			opStr = "-"
		}
		left = &ast.BinaryExpr{Op: opStr, Left: left, Right: right, Line: op.Line}
	}
	return left, nil
}

func (p *Parser) parseMultiplicative() (ast.Expr, error) {
	left, err := p.parseUnary()
	if err != nil {
		return nil, err
	}
	for p.cur().Type == lexer.STAR || p.cur().Type == lexer.SLASH {
		op := p.advance()
		right, err := p.parseUnary()
		if err != nil {
			return nil, err
		}
		opStr := "*"
		if op.Type == lexer.SLASH {
			opStr = "/"
		}
		left = &ast.BinaryExpr{Op: opStr, Left: left, Right: right, Line: op.Line}
	}
	return left, nil
}

func (p *Parser) parseUnary() (ast.Expr, error) {
	if p.cur().Type == lexer.MINUS {
		op := p.advance()
		operand, err := p.parseUnary()
		if err != nil {
			return nil, err
		}
		return &ast.UnaryExpr{Op: "-", Operand: operand, Line: op.Line}, nil
	}
	return p.parsePrimary()
}

func (p *Parser) parsePrimary() (ast.Expr, error) {
	tok := p.cur()
	switch tok.Type {
	case lexer.STRING:
		p.advance()
		return &ast.StringLiteral{Value: tok.Value, Line: tok.Line}, nil
	case lexer.INT:
		p.advance()
		v, err := strconv.ParseInt(tok.Value, 10, 64)
		if err != nil {
			return nil, fmt.Errorf("строка %d, столбец %d: некорректный целочисленный литерал %q", tok.Line, tok.Col, tok.Value)
		}
		return &ast.IntLiteral{Value: v, Line: tok.Line}, nil
	case lexer.FLOAT:
		p.advance()
		v, err := strconv.ParseFloat(tok.Value, 64)
		if err != nil {
			return nil, fmt.Errorf("строка %d, столбец %d: некорректный числовой литерал %q", tok.Line, tok.Col, tok.Value)
		}
		return &ast.FloatLiteral{Value: v, Line: tok.Line}, nil
	case lexer.KwTrue:
		p.advance()
		return &ast.BoolLiteral{Value: true, Line: tok.Line}, nil
	case lexer.KwFalse:
		p.advance()
		return &ast.BoolLiteral{Value: false, Line: tok.Line}, nil
	case lexer.IDENT:
		p.advance()
		return &ast.Identifier{Name: tok.Value, Line: tok.Line}, nil
	case lexer.LPAREN:
		p.advance()
		inner, err := p.parseExpr()
		if err != nil {
			return nil, err
		}
		if _, err := p.expect(lexer.RPAREN); err != nil {
			return nil, err
		}
		return inner, nil
	default:
		return nil, fmt.Errorf("строка %d, столбец %d: ожидалось выражение, получено %q",
			tok.Line, tok.Col, tok.Value)
	}
}
