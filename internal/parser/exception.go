package parser

import (
	"fmt"

	"github.com/IlyaFeoktistov/Rech/internal/ast"
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
		return nil, fmt.Errorf("после \"выбросить\" ожидалось выражение: %w", err)
	}
	return &ast.ThrowStmt{Value: value, Line: kw.Line}, nil
}

// tryCatchClause - один разобранный "поймать(...) { ... }" из цепочки,
// см. package doc parseTryStmt ниже.
type tryCatchClause struct {
	catchVar   string
	catchTypes []string
	catchPanic bool
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
		return tryCatchClause{}, fmt.Errorf("после \"поймать\" ожидалось \"(\": %w", err)
	}
	catchVar, err := p.expect(lexer.IDENT)
	if err != nil {
		return tryCatchClause{}, fmt.Errorf("в \"поймать(...)\" ожидалось имя переменной исключения: %w", err)
	}
	if _, err := p.expect(lexer.COLON); err != nil {
		return tryCatchClause{}, fmt.Errorf("после имени %q в \"поймать(...)\" ожидалось \":\": %w", catchVar.Value, err)
	}
	p.registerNoun(catchVar.Value)
	var catchTypes []string
	for {
		t, err := p.parseTypeName()
		if err != nil {
			return tryCatchClause{}, fmt.Errorf("в \"поймать(...)\" ожидалось имя типа исключения: %w", err)
		}
		catchTypes = append(catchTypes, t)
		if p.cur().Type != lexer.BITOR {
			break
		}
		p.advance() // "|"
	}
	if _, err := p.expect(lexer.RPAREN); err != nil {
		return tryCatchClause{}, fmt.Errorf("после списка типов в \"поймать(...)\" ожидалось \")\": %w", err)
	}
	p.skipNewlines()
	catchBody, err := p.parseBlock()
	if err != nil {
		return tryCatchClause{}, err
	}
	// "Паника" (docs/ROADMAP.md, изоляция паники) - НЕ настоящий тип
	// исключения (см. package doc ast.TryStmt.CatchPanic) - распознаётся
	// здесь ПОСЛЕ полного разбора списка типов, а не как отдельная ветка
	// грамматики, и вырезается из catchTypes перед возвратом (семантика
	// резолвит только оставшиеся имена как настоящие типы, "Паника" саму по
	// себе резолвить не пытается): "поймать (x: Паника | Паника)" - явная
	// ошибка (дубликат), "поймать (x: Паника | ОшибкаХ)" - валидный
	// смешанный обработчик (CatchPanic true И CatchTypes непусто).
	panicCount := 0
	otherTypes := make([]string, 0, len(catchTypes))
	for _, t := range catchTypes {
		if t == "Паника" {
			panicCount++
			continue
		}
		otherTypes = append(otherTypes, t)
	}
	if panicCount > 1 {
		return tryCatchClause{}, fmt.Errorf("строка %d: \"Паника\" в \"поймать(...)\" перечислена более одного раза", kw.Line)
	}
	return tryCatchClause{
		catchVar: catchVar.Value, catchTypes: otherTypes, catchPanic: panicCount == 1, catchBody: catchBody, line: kw.Line,
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
			return nil, fmt.Errorf("после \"окончательно\" ожидался блок: %w", err)
		}
		hasFinally = true
	}

	if len(clauses) == 0 {
		if !hasFinally {
			return nil, fmt.Errorf("строка %d: после блока \"попытаться\" ожидалось \"поймать\" или \"окончательно\"", kw.Line)
		}
		return &ast.TryStmt{Body: body, NoCatch: true, Finally: finallyBody, Line: kw.Line}, nil
	}

	node := &ast.TryStmt{
		Body: body, CatchVar: clauses[0].catchVar, CatchTypes: clauses[0].catchTypes,
		CatchPanic: clauses[0].catchPanic, CatchBody: clauses[0].catchBody, Line: clauses[0].line,
	}
	for _, c := range clauses[1:] {
		node = &ast.TryStmt{
			Body: []ast.Stmt{node}, CatchVar: c.catchVar, CatchTypes: c.catchTypes,
			CatchPanic: c.catchPanic, CatchBody: c.catchBody, Line: c.line,
		}
	}
	node.Finally = finallyBody
	return node, nil
}
