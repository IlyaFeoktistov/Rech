// Отложенное объявление переменной (docs/GRAMMAR_DRAFT.md, "Естественный вызов
// как значение"): "переменная X" без "=" - как раньше, требуется имя, но
// значение/тип можно не указывать сразу, если ОНИ появятся из первого
// присваивания X дальше в том же блоке ("X = значение" или "... и положить
// в X" - естественный вызов как значение, call.go). Само "переменная X" не
// порождает ни одного узла AST - настоящий ast.VarDecl строится на месте
// ПЕРВОГО присваивания X (resolveAssignTarget), с типом, выведенным из его
// значения, как у обычного "переменная X = значение".
package parser

import (
	"github.com/IlyaFeoktistov/Rech/internal/ast"
	"github.com/IlyaFeoktistov/Rech/internal/lexer"
)

type pendingVarDecl struct {
	name string
	tok  lexer.Token
}

// beginPendingScope открывает новую область отложенных объявлений - см.
// parseBlock/ParseProgram. Разрешение работает только внутри блока, где
// написано "переменная X" (включая любые вложенные if/while/for в нём) -
// не наружу и не внутрь смежных блоков, тем же принципом области
// видимости, что и у "{...}" везде в языке.
func (p *Parser) beginPendingScope() []pendingVarDecl {
	saved := p.pendingVarDecls
	p.pendingVarDecls = nil
	return saved
}

// endPendingScope закрывает область, открытую beginPendingScope - ошибка,
// если в ней остались незаполненные объявления ("переменная X" без единого
// присваивания X до конца блока была бы просто неиспользуемым именем).
func (p *Parser) endPendingScope(saved []pendingVarDecl) error {
	if len(p.pendingVarDecls) > 0 {
		first := p.pendingVarDecls[0]
		return errAt(first.tok.Line, first.tok.Col, CodeStmtRestrict, "переменная %q объявлена без значения и ни разу не заполнена (\"%s = ...\" или \"... и положить в %s\") до конца блока", first.name, first.name, first.name)
	}
	p.pendingVarDecls = saved
	return nil
}

func (p *Parser) registerPendingVarDecl(name lexer.Token) {
	p.pendingVarDecls = append(p.pendingVarDecls, pendingVarDecl{name: name.Value, tok: name})
}

// resolveAssignTarget строит ast.Stmt для "name = value" - ast.VarDecl,
// если name - отложенное объявление текущего блока (первое присваивание
// такому имени и есть его объявление - тип выводится из value, как у
// обычного "переменная X = значение"), иначе обычное ast.Assign
// (присваивание уже существующей переменной). Общая точка для
// parseAssign ("X = значение") и parseNaturalCallContinuation
// ("... и положить в X", call.go) - оба разрешают отложенное объявление
// одинаково.
func (p *Parser) resolveAssignTarget(name lexer.Token, value ast.Expr) ast.Stmt {
	for i, pending := range p.pendingVarDecls {
		if pending.name == name.Value {
			p.pendingVarDecls = append(p.pendingVarDecls[:i], p.pendingVarDecls[i+1:]...)
			p.registerNoun(name.Value)
			return &ast.VarDecl{Name: name.Value, Value: value, Line: name.Line}
		}
	}
	return &ast.Assign{Name: name.Value, Value: value, Line: name.Line}
}
