package parser

import (
	"fmt"
	"slices"

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

// parseTryStmt разбирает:
//
//	попытаться { <тело> } поймать(<имя>: <Тип> { "|" <Тип> }) { <обработчик> }
//
// Ровно один "поймать" сразу после блока "попытаться" - не цепочка (см.
// package doc ast.TryStmt).
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
	p.skipNewlines()
	if _, err := p.expect(lexer.KwCatch); err != nil {
		return nil, fmt.Errorf("после блока \"попытаться\" ожидалось \"поймать\": %w", err)
	}
	if _, err := p.expect(lexer.LPAREN); err != nil {
		return nil, fmt.Errorf("после \"поймать\" ожидалось \"(\": %w", err)
	}
	catchVar, err := p.expect(lexer.IDENT)
	if err != nil {
		return nil, fmt.Errorf("в \"поймать(...)\" ожидалось имя переменной исключения: %w", err)
	}
	if _, err := p.expect(lexer.COLON); err != nil {
		return nil, fmt.Errorf("после имени %q в \"поймать(...)\" ожидалось \":\": %w", catchVar.Value, err)
	}
	p.registerNoun(catchVar.Value)
	var catchTypes []string
	for {
		t, err := p.parseTypeName()
		if err != nil {
			return nil, fmt.Errorf("в \"поймать(...)\" ожидалось имя типа исключения: %w", err)
		}
		catchTypes = append(catchTypes, t)
		if p.cur().Type != lexer.BITOR {
			break
		}
		p.advance() // "|"
	}
	if _, err := p.expect(lexer.RPAREN); err != nil {
		return nil, fmt.Errorf("после списка типов в \"поймать(...)\" ожидалось \")\": %w", err)
	}
	p.skipNewlines()
	catchBody, err := p.parseBlock()
	if err != nil {
		return nil, err
	}
	// "Паника" (docs/ROADMAP.md, изоляция паники) - НЕ настоящий тип
	// исключения (см. package doc ast.TryStmt.CatchPanic) - распознаётся
	// здесь ПОСЛЕ полного разбора списка типов, а не как отдельная ветка
	// грамматики: "поймать (x: Паника | ОшибкаХ)" - явная ошибка (не тихое
	// падение в резолюцию несуществующей сущности "Паника" где-то дальше в
	// семантике), "Паника" обязана быть ЕДИНСТВЕННЫМ элементом списка.
	if len(catchTypes) > 1 && slices.Contains(catchTypes, "Паника") {
		return nil, fmt.Errorf("строка %d: \"Паника\" в \"поймать(...)\" не может сочетаться с другими типами через \"|\"", kw.Line)
	}
	if len(catchTypes) == 1 && catchTypes[0] == "Паника" {
		return &ast.TryStmt{
			Body: body, CatchVar: catchVar.Value, CatchPanic: true, CatchBody: catchBody, Line: kw.Line,
		}, nil
	}
	return &ast.TryStmt{
		Body: body, CatchVar: catchVar.Value, CatchTypes: catchTypes, CatchBody: catchBody, Line: kw.Line,
	}, nil
}
