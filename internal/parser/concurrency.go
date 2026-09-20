package parser

import (
	"github.com/IlyaFeoktistov/Rech/internal/ast"
	"github.com/IlyaFeoktistov/Rech/internal/diagnostics"
	"github.com/IlyaFeoktistov/Rech/internal/lexer"
)

// parseLaunchExpr разбирает "запустить <имя>(<арг>, ...)" - операнд
// ограничен ровно строгим вызовом обычной функции (не методом, не
// естественным вызовом) - см. package doc ast.LaunchExpr.
func (p *Parser) parseLaunchExpr() (ast.Expr, error) {
	kw := p.advance() // "запустить"
	if p.cur().Type != lexer.IDENT || p.peekAt(1).Type != lexer.LPAREN {
		return nil, errAt(p.cur().Line, p.cur().Col, CodeUnexpectedToken, "после \"запустить\" ожидался вызов функции (\"имя(...)\")")
	}
	call, err := p.parseCallExpr()
	if err != nil {
		return nil, err
	}
	return &ast.LaunchExpr{Call: call, Line: kw.Line}, nil
}

// parseCoroLaunchExpr разбирает "поручить <имя>(<арг>, ...)" - та же
// грамматическая форма, что и "запустить" (см. package doc
// ast.CoroLaunchExpr) - принадлежность к "асинхронная функция" проверяет
// семантика, не парсер (тем же принципом, что и разбор запустить).
func (p *Parser) parseCoroLaunchExpr() (ast.Expr, error) {
	kw := p.advance() // "поручить"
	if p.cur().Type != lexer.IDENT || p.peekAt(1).Type != lexer.LPAREN {
		return nil, errAt(p.cur().Line, p.cur().Col, CodeUnexpectedToken, "после \"поручить\" ожидался вызов асинхронной функции (\"имя(...)\")")
	}
	call, err := p.parseCallExpr()
	if err != nil {
		return nil, err
	}
	return &ast.CoroLaunchExpr{Call: call, Line: kw.Line}, nil
}

// parseAwaitExpr разбирает "дождаться <выражение>" - выражение задачи
// берётся на уровне postfix (переменная, доступ к полю) - не полное expr,
// чтобы "дождаться а + б" не читалось неоднозначно ("дождаться а", потом
// "+ б", или "дождаться (а + б)"?).
func (p *Parser) parseAwaitExpr() (ast.Expr, error) {
	kw := p.advance() // "дождаться"
	task, err := p.parsePostfix()
	if err != nil {
		return nil, diagnostics.Wrap(err, "после \"дождаться\" ожидалось выражение задачи")
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

// parseAsyncModifier разбирает необязательное "асинхронная" перед
// "функция" (после видимости модуля, если она есть) - см.
// ast.FunctionDecl.IsAsync. Согласуется только с "функция" (женский род) -
// метод/сущность/"экспортировать" после "асинхронная" не поддерживаются в
// этой версии (та же граница, что у ast.LaunchExpr.Call - только обычные
// функции).
func (p *Parser) parseAsyncModifier() (bool, error) {
	if p.cur().Type != lexer.KwAsync {
		return false, nil
	}
	tok := p.advance()
	if tok.Value != "асинхронная" {
		return false, errAt(tok.Line, tok.Col, CodeModifierAgree, "модификатор согласуется со словом \"функция\" (женский род) - ожидалось \"асинхронная\", получено %q", tok.Value)
	}
	if p.cur().Type != lexer.KwFunction {
		return false, errAt(p.cur().Line, p.cur().Col, CodeUnexpectedToken, "после \"асинхронная\" ожидалось \"функция\" (модификатор пока поддерживается только для обычных функций - не методов, не сущностей, не \"экспортировать\"), получено %q", p.cur().Value)
	}
	return true, nil
}
