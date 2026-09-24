package parser

import (
	"github.com/IlyaFeoktistov/Rech/internal/ast"
	"github.com/IlyaFeoktistov/Rech/internal/diagnostics"
	"github.com/IlyaFeoktistov/Rech/internal/lexer"
)

// parseThrowStmt разбирает "выбросить <выражение>".
func (p *Parser) parseThrowStmt() (ast.Stmt, error) {
	kw, err := p.expect(lexer.KwThrow)
	if err != nil {
		return nil, err
	}
	value, err := p.parseExpr()
	if err != nil {
		return nil, diagnostics.Wrap(err, "после \"выбросить\" ожидалось выражение")
	}
	return &ast.ThrowStmt{Value: value, Line: kw.Line}, nil
}

// tryCatchClause - один разобранный "поймать(...) { ... }" из цепочки,
// см. package doc parseTryStmt ниже.
type tryCatchClause struct {
	catchVar   string
	catchTypes []string
	catchBody  []ast.Stmt
	line       int
}

// parseOneCatchClause разбирает ОДИН "поймать(<имя>: <Тип> { "|" <Тип> }) { <обработчик> }" -
// вызывающая сторона (parseTryStmt) уже убедилась (peekPastNewlines), что
// дальше действительно идёт "поймать", и сама отвечает за skipNewlines
// перед вызовом.
func (p *Parser) parseOneCatchClause() (tryCatchClause, error) {
	kw, err := p.expect(lexer.KwCatch)
	if err != nil {
		return tryCatchClause{}, err
	}
	if _, err := p.expect(lexer.LPAREN); err != nil {
		return tryCatchClause{}, diagnostics.Wrap(err, "после \"поймать\" ожидалось \"(\"")
	}
	catchVar, err := p.expect(lexer.IDENT)
	if err != nil {
		return tryCatchClause{}, diagnostics.Wrap(err, "в \"поймать(...)\" ожидалось имя переменной исключения")
	}
	if _, err := p.expect(lexer.COLON); err != nil {
		return tryCatchClause{}, diagnostics.Wrap(err, "после имени %q в \"поймать(...)\" ожидалось \":\"", catchVar.Value)
	}
	p.registerNoun(catchVar.Value)
	var catchTypes []string
	for {
		t, err := p.parseTypeName()
		if err != nil {
			return tryCatchClause{}, diagnostics.Wrap(err, "в \"поймать(...)\" ожидалось имя типа исключения")
		}
		catchTypes = append(catchTypes, t)
		if p.cur().Type != lexer.BITOR {
			break
		}
		p.advance() // "|"
	}
	if _, err := p.expect(lexer.RPAREN); err != nil {
		return tryCatchClause{}, diagnostics.Wrap(err, "после списка типов в \"поймать(...)\" ожидалось \")\"")
	}
	p.skipNewlines()
	catchBody, err := p.parseBlock()
	if err != nil {
		return tryCatchClause{}, err
	}
	return tryCatchClause{
		catchVar: catchVar.Value, catchTypes: catchTypes, catchBody: catchBody, line: kw.Line,
	}, nil
}

// parseTryStmt разбирает:
//
//	попытаться { <тело> }
//	поймать(<имя1>: <Тип> { "|" <Тип> }) { <обработчик1> }
//	[ поймать(<имя2>: <Тип> ...) { <обработчик2> } ... ]
//	[ окончательно { <блок> } ]
//
// Ноль или больше "поймать" подряд (цепочка, как несколько "иначе если") -
// собираются здесь во ВЛОЖЕННЫЕ *ast.TryStmt (package doc ast.TryStmt):
// первый написанный - самый внутренний узел, каждый следующий оборачивает
// предыдущий целиком как свой Body - пробуются в точности в этом порядке во
// время исполнения, ровно как нарисовано в исходнике. Хотя бы один
// "поймать" ИЛИ "окончательно" обязателен - голое "попытаться { ... }" без
// того и другого не имеет смысла. Ноль "поймать" (только "окончательно") -
// NoCatch true у единственного узла.
func (p *Parser) parseTryStmt() (ast.Stmt, error) {
	kw, err := p.expect(lexer.KwTry)
	if err != nil {
		return nil, err
	}
	p.skipNewlines()
	body, err := p.parseBlock()
	if err != nil {
		return nil, err
	}

	var clauses []tryCatchClause
	for p.peekPastNewlines().Type == lexer.KwCatch {
		p.skipNewlines()
		c, err := p.parseOneCatchClause()
		if err != nil {
			return nil, err
		}
		clauses = append(clauses, c)
	}

	var finallyBody []ast.Stmt
	hasFinally := false
	if p.peekPastNewlines().Type == lexer.KwFinally {
		p.skipNewlines()
		p.advance() // "окончательно"
		p.skipNewlines()
		finallyBody, err = p.parseBlock()
		if err != nil {
			return nil, diagnostics.Wrap(err, "после \"окончательно\" ожидался блок")
		}
		hasFinally = true
	}

	if len(clauses) == 0 {
		if !hasFinally {
			return nil, errAtLine(kw.Line, CodeUnexpectedToken, "после блока \"попытаться\" ожидалось \"поймать\" или \"окончательно\"")
		}
		return &ast.TryStmt{Body: body, NoCatch: true, Finally: finallyBody, Line: kw.Line}, nil
	}

	node := &ast.TryStmt{
		Body: body, CatchVar: clauses[0].catchVar, CatchTypes: clauses[0].catchTypes,
		CatchBody: clauses[0].catchBody, Line: clauses[0].line,
	}
	for _, c := range clauses[1:] {
		node = &ast.TryStmt{
			Body: []ast.Stmt{node}, CatchVar: c.catchVar, CatchTypes: c.catchTypes,
			CatchBody: c.catchBody, Line: c.line,
		}
	}
	node.Finally = finallyBody
	return node, nil
}
