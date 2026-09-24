package parser

import (
	"github.com/IlyaFeoktistov/Rech/internal/ast"
	"github.com/IlyaFeoktistov/Rech/internal/diagnostics"
	"github.com/IlyaFeoktistov/Rech/internal/lexer"
)

// parseSynonymDecl разбирает "синоним [?]<Имя>[<T, ...>] = <тип>" (см.
// ast.SynonymDecl). Параметры generic-синонима - только имена: ограничения
// ("T как Строчный") проверяются там, где синоним раскрыт, у самого типа.
func (p *Parser) parseSynonymDecl() (*ast.SynonymDecl, error) {
	kw, err := p.expect(lexer.KwSynonym)
	if err != nil {
		return nil, err
	}
	nullable := false
	if p.cur().Type == lexer.QUESTION {
		p.advance()
		nullable = true
	}
	name, err := p.expect(lexer.IDENT)
	if err != nil {
		return nil, diagnostics.Wrap(err, "после %q ожидалось имя синонима", kw.Value)
	}
	generic, err := p.parseGenericParams()
	if err != nil {
		return nil, diagnostics.Wrap(err, "синоним %q", name.Value)
	}
	var params []string
	for _, g := range generic {
		if len(g.Constraints) > 0 {
			return nil, errAtLine(g.Line, CodeUnexpectedToken, "синоним %q: у параметра типа %q не бывает ограничений - синоним только называет тип", name.Value, g.Name)
		}
		params = append(params, g.Name)
	}
	if _, err := p.expect(lexer.ASSIGN); err != nil {
		return nil, diagnostics.Wrap(err, "синоним %q: ожидалось \"=\" и тип", name.Value)
	}
	target, err := p.parseTypeName()
	if err != nil {
		return nil, diagnostics.Wrap(err, "синоним %q", name.Value)
	}
	return &ast.SynonymDecl{Name: name.Value, Nullable: nullable, TypeParams: params, Target: target, Line: kw.Line}, nil
}
