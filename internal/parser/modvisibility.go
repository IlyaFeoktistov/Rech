// Видимость верхнего уровня файла (модульная инкапсуляция) - необязательный
// "приватный"/"публичный" перед "функция"/"сущность"/"класс"/"структура" (в
// т. ч. вместе с "экспортировать"/"абстрактный"), "перечисление" или
// "константа" (docs/ROADMAP.md - паритет с уже существующей приватностью
// функций/сущностей): без модификатора - публичное (обратная
// совместимость), приватное - не видно для "использовать <имя> из <путь>"
// из другого файла (internal/module, declaredNames) и не резолвится по
// голому имени/через "." из другого файла (internal/module/walk.go,
// rewriteExpr - case *ast.Identifier/*ast.FieldAccess), но не ограничивает
// использование внутри самого файла. "защищённый" здесь не предлагается -
// у файлов нет наследования друг от друга, третий уровень видимости
// (внутри сущности - "виден предкам") не имеет аналога на уровне модуля.
package parser

import (
	"fmt"

	"github.com/IlyaFeoktistov/Rech/internal/ast"
	"github.com/IlyaFeoktistov/Rech/internal/lexer"
	"github.com/IlyaFeoktistov/Rech/internal/morph"
)

// isModuleVisibilityKeyword - true для "приватный"/"публичный" (не
// "защищённый" - см. package doc) в любой форме.
func isModuleVisibilityKeyword(t lexer.TokenType) bool {
	return t == lexer.KwPrivate || t == lexer.KwPublic
}

// governingModuleKeyword - ключевое слово верхнего уровня, с которым
// должен согласоваться уже прочитанный модификатор видимости (p.cur() -
// первый токен после него) - пропускает "экспортировать"/"абстрактный"
// (сами по себе модификаторы, без собственного рода) в поисках
// "функция"/"сущность"/"класс"/"структура".
func (p *Parser) governingModuleKeyword() lexer.TokenType {
	for i := range 3 {
		t := p.peekAt(i).Type
		if t == lexer.KwExport || t == lexer.KwAbstract {
			continue
		}
		return t
	}
	return p.peekAt(0).Type
}

// moduleVisibilityWordFor - ожидаемая форма модификатора видимости перед
// governing - НЕ через morph.NounAgreement (имя объявления - не то, с чем
// согласуется модификатор): "функция"/"сущность"/"структура"/"константа" -
// женский род, "класс" - мужской, "перечисление" - средний, фиксированные
// служебные слова, тем же принципом, что и checkConstVisibilityAgreement
// (константа уровня сущности - тоже всегда женский род).
func moduleVisibilityWordFor(vis ast.Visibility, governing lexer.TokenType) (string, error) {
	switch governing {
	case lexer.KwFunction, lexer.KwEntity, lexer.KwStruct, lexer.KwConst:
		return visibilityKeywordFor(vis, morph.Feminine, morph.Singular), nil
	case lexer.KwClass:
		return visibilityKeywordFor(vis, morph.Masculine, morph.Singular), nil
	case lexer.KwEnum:
		return visibilityKeywordFor(vis, morph.Neuter, morph.Singular), nil
	default:
		return "", fmt.Errorf("модификатор видимости здесь неуместен - ожидалась \"функция\", \"сущность\", \"класс\", \"структура\", \"перечисление\" или \"константа\"")
	}
}

// parseModuleVisibility разбирает необязательный "приватный"/"публичный"
// перед объявлением верхнего уровня файла - см. package doc. Курсор на
// входе - на самом модификаторе или уже на governing keyword (модификатора
// не было); на выходе - всегда на governing keyword.
func (p *Parser) parseModuleVisibility() (ast.Visibility, error) {
	if !isModuleVisibilityKeyword(p.cur().Type) {
		return ast.Public, nil
	}
	tok := p.advance()
	vis := visibilityFromToken(tok)
	governing := p.governingModuleKeyword()
	want, err := moduleVisibilityWordFor(vis, governing)
	if err != nil {
		return ast.Public, fmt.Errorf("строка %d, столбец %d: %w", tok.Line, tok.Col, err)
	}
	if tok.Value != want {
		return ast.Public, fmt.Errorf("строка %d, столбец %d: модификатор видимости согласуется со словом %q - ожидалось %q, получено %q",
			tok.Line, tok.Col, governing.String(), want, tok.Value)
	}
	return vis, nil
}
