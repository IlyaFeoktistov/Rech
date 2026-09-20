package parser

import (
	"github.com/IlyaFeoktistov/Rech/internal/ast"
	"github.com/IlyaFeoktistov/Rech/internal/diagnostics"
	"github.com/IlyaFeoktistov/Rech/internal/lexer"
)

// parsePlatformBlock разбирает "#если ОС.<Член> { ... } [#иначе если
// ОС.<Член> { ... }]... [#иначе { ... }]" - см. package doc
// ast.PlatformBlockDecl (условная компиляция по целевой платформе,
// разрешается ДО семантического анализа, internal/module,
// ResolvePlatformBlocks). Вызывается из parseTopLevelItem при виде HASH.
func (p *Parser) parsePlatformBlock() (*ast.PlatformBlockDecl, error) {
	block := &ast.PlatformBlockDecl{Line: p.cur().Line}
	p.advance() // "#"
	if p.cur().Type != lexer.KwIf {
		return nil, errAtLine(block.Line, CodeUnexpectedToken, "после \"#\" ожидалось \"если\", получено %q", p.cur().Value)
	}
	for {
		p.advance() // "если"
		platform, err := p.parsePlatformCondition()
		if err != nil {
			return nil, err
		}
		arm, err := p.parsePlatformArmBody()
		if err != nil {
			return nil, err
		}
		arm.Platform = platform
		block.Arms = append(block.Arms, arm)

		if p.cur().Type != lexer.HASH || p.peekAt(1).Type != lexer.KwElse {
			return block, nil
		}
		p.advance() // "#"
		p.advance() // "иначе"
		if p.cur().Type == lexer.KwIf {
			// "#иначе если ОС.<Член>" - ещё одна условная рука, продолжаем
			// тот же цикл (сознательно допускаем цепочку здесь, в отличие
			// от обычного рантайм "если"/"иначе" - см. package doc
			// ast.PlatformBlockDecl).
			continue
		}
		elseArm, err := p.parsePlatformArmBody()
		if err != nil {
			return nil, err
		}
		// elseArm.Platform остаётся "" (безусловный "#иначе").
		block.Arms = append(block.Arms, elseArm)
		return block, nil
	}
}

// parsePlatformCondition разбирает "ОС.<Член>" после "если"/"иначе если"
// внутри "#если" - тот же список платформ, что и StandardEnums["ОС"]
// (internal/semantic/types.go) - общий источник, ast.PlatformNames, чтобы
// не разъехались.
func (p *Parser) parsePlatformCondition() (string, error) {
	tok := p.cur()
	if tok.Type != lexer.IDENT || tok.Value != "ОС" {
		return "", errAtLine(tok.Line, CodeConditionalBuild, "после \"если\" внутри \"#если\" ожидалось \"ОС.<Член>\", получено %q", tok.Value)
	}
	p.advance() // "ОС"
	if _, err := p.expect(lexer.DOT); err != nil {
		return "", diagnostics.WrapAt(err, diagnostics.Position{Line: tok.Line}, "ожидалось \".\" после \"ОС\"")
	}
	member, err := p.expect(lexer.IDENT)
	if err != nil {
		return "", diagnostics.WrapAt(err, diagnostics.Position{Line: tok.Line}, "ожидалось имя платформы после \"ОС.\"")
	}
	for _, name := range ast.PlatformNames {
		if name == member.Value {
			return name, nil
		}
	}
	return "", errAtLine(member.Line, CodeConditionalBuild, "\"ОС.%s\" не существует - допустимо \"ОС.Linux\"/\"ОС.Windows\"/\"ОС.MacOS\"", member.Value)
}

// parsePlatformArmBody разбирает "{ <объявления> }" одной руки - тот же
// разбор одного верхнеуровневого объявления, что и у самого файла
// (parseTopLevelItem, вызывается на временный *ast.Program), просто
// ограниченный "}", не EOF, и без топ-уровневых ИНСТРУКЦИЙ (implicit
// "главная") - внутри "#если" осмысленны только объявления, не обычный
// код тела функции.
func (p *Parser) parsePlatformArmBody() (ast.PlatformArm, error) {
	openTok, err := p.expect(lexer.LBRACE)
	if err != nil {
		return ast.PlatformArm{}, diagnostics.Wrap(err, "после условия \"#если\"/\"#иначе если\"/\"#иначе\" ожидалось \"{\"")
	}
	armProg := &ast.Program{}
	var stmts []ast.Stmt
	var explicitMain *ast.FunctionDecl
	p.skipSeparators()
	for p.cur().Type != lexer.RBRACE {
		if p.cur().Type == lexer.EOF {
			return ast.PlatformArm{}, errAtLine(openTok.Line, CodeConditionalBuild, "не закрыта \"{\" блока условной компиляции")
		}
		itemLine := p.cur().Line
		skip, err := p.parseTopLevelItem(armProg, &stmts, &explicitMain)
		if err != nil {
			return ast.PlatformArm{}, err
		}
		if skip {
			continue
		}
		if len(stmts) > 0 {
			return ast.PlatformArm{}, errAtLine(itemLine, CodeConditionalBuild, "внутри \"#если\" разрешены только объявления (функция/сущность/внешняя функция/использовать/...), не обычные инструкции")
		}
		if p.cur().Type == lexer.RBRACE {
			break
		}
		if err := p.requireSeparator(); err != nil {
			return ast.PlatformArm{}, err
		}
	}
	if _, err := p.expect(lexer.RBRACE); err != nil {
		return ast.PlatformArm{}, err
	}
	return ast.PlatformArm{
		Functions:      armProg.Functions,
		Entities:       armProg.Entities,
		Uses:           armProg.Uses,
		ExternFuncs:    armProg.ExternFuncs,
		ExternEntities: armProg.ExternEntities,
		Enums:          armProg.Enums,
		Interfaces:     armProg.Interfaces,
		Consts:         armProg.Consts,
	}, nil
}
