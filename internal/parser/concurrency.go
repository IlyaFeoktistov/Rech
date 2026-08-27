package parser

import (
	"fmt"

	"github.com/IlyaFeoktistov/Rech/internal/ast"
	"github.com/IlyaFeoktistov/Rech/internal/lexer"
)

// parseLaunchExpr разбирает "запустить <имя>(<арг>, ...)" - операнд
// ограничен ровно строгим вызовом обычной функции (не методом, не
// естественным вызовом) - см. package doc ast.LaunchExpr.
func (p *Parser) parseLaunchExpr() (ast.Expr, error) {
	kw := p.advance() // "запустить"
	if p.cur().Type != lexer.IDENT || p.peekAt(1).Type != lexer.LPAREN {
		return nil, fmt.Errorf("строка %d, столбец %d: после \"запустить\" ожидался вызов функции (\"имя(...)\")",
			p.cur().Line, p.cur().Col)
	}
	call, err := p.parseCallExpr()
	if err != nil {
		return nil, err
	}
	return &ast.LaunchExpr{Call: call, Line: kw.Line}, nil
}

// parseAwaitExpr разбирает "дождаться <выражение>" - выражение задачи
// берётся на уровне postfix (переменная, доступ к полю) - не полное expr,
// чтобы "дождаться а + б" не читалось неоднозначно ("дождаться а", потом
// "+ б", или "дождаться (а + б)"?).
func (p *Parser) parseAwaitExpr() (ast.Expr, error) {
	kw := p.advance() // "дождаться"
	task, err := p.parsePostfix()
	if err != nil {
		return nil, fmt.Errorf("после \"дождаться\" ожидалось выражение задачи: %w", err)
	}
	return &ast.AwaitExpr{Task: task, Line: kw.Line}, nil
}

// parseAwaitStmt разбирает "дождаться <задача>" как самостоятельную
// инструкцию - валидно, когда обёрнутая функция не объявляет тип
// результата (тот же принцип, что и у обычного вызова функции без
// результата, используемого как ExprStmt).
func (p *Parser) parseAwaitStmt() (ast.Stmt, error) {
	expr, err := p.parseAwaitExpr()
	if err != nil {
		return nil, err
	}
	await := expr.(*ast.AwaitExpr)
	return &ast.ExprStmt{Await: await, Line: await.Line}, nil
}
