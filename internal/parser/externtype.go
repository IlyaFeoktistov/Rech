package parser

import (
	"fmt"

	"github.com/IlyaFeoktistov/Rech/internal/ast"
	"github.com/IlyaFeoktistov/Rech/internal/lexer"
)

// parseExternEntityDecl разбирает "внешняя сущность <ИмяC> [как
// <Псевдоним>]" - непрозрачный внешний тип (указатель на структуру чужой
// C-библиотеки, например CURL* из libcurl), без тела: у него нет ни
// полей, ни методов, значение можно только получить от внешней функции.
func (p *Parser) parseExternEntityDecl() (*ast.ExternEntityDecl, error) {
	kw, err := p.expect(lexer.KwExternal)
	if err != nil {
		return nil, err
	}
	if _, err := p.expect(lexer.KwEntity); err != nil {
		return nil, fmt.Errorf("после %q ожидалось \"сущность\": %w", kw.Value, err)
	}
	cName, err := p.expect(lexer.IDENT)
	if err != nil {
		return nil, fmt.Errorf("после \"внешняя сущность\" ожидалось имя C-типа: %w", err)
	}
	alias := cName.Value
	if a, err := p.parseOptionalAlias(); err != nil {
		return nil, err
	} else if a != "" {
		alias = a
	}
	p.registerNoun(alias)
	return &ast.ExternEntityDecl{CName: cName.Value, Alias: alias, Line: kw.Line}, nil
}

// parseExternStructDecl разбирает "внешняя структура <ИмяC> [как
// <Псевдоним>] { имеет: ... }" - прозрачная C-структура по значению,
// повторяющая layout настоящей C-структуры (см. package doc
// ast.EntityDecl.IsExternStruct). Возвращает обычный *ast.EntityDecl
// (IsStruct/IsExternStruct == true) - дальше по конвейеру (семантика,
// cbackend) это неотличимо от написанной вручную "структура", кроме
// дополнительной проверки типов полей (internal/semantic, isFFIType).
func (p *Parser) parseExternStructDecl() (*ast.EntityDecl, error) {
	kw, err := p.expect(lexer.KwExternal)
	if err != nil {
		return nil, err
	}
	if _, err := p.expect(lexer.KwStruct); err != nil {
		return nil, fmt.Errorf("после %q ожидалось \"структура\": %w", kw.Value, err)
	}
	cName, err := p.expect(lexer.IDENT)
	if err != nil {
		return nil, fmt.Errorf("после \"внешняя структура\" ожидалось имя C-типа: %w", err)
	}
	alias := cName.Value
	if a, err := p.parseOptionalAlias(); err != nil {
		return nil, err
	} else if a != "" {
		alias = a
	}
	p.registerNoun(alias)

	p.skipNewlines()
	if _, err := p.expect(lexer.LBRACE); err != nil {
		return nil, err
	}
	p.skipSeparators()
	fields, err := p.parseFieldsSection()
	if err != nil {
		return nil, fmt.Errorf("внешняя структура %q: %w", alias, err)
	}
	p.skipSeparators()
	if _, err := p.expect(lexer.RBRACE); err != nil {
		return nil, fmt.Errorf("внешняя структура %q: %w", alias, err)
	}
	if len(fields) == 0 {
		return nil, fmt.Errorf("строка %d: внешняя структура %q должна содержать хотя бы одно поле", kw.Line, alias)
	}
	return &ast.EntityDecl{
		Name: alias, IsStruct: true, IsExternStruct: true, ExternCName: cName.Value,
		Fields: fields, Line: kw.Line,
	}, nil
}
