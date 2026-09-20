// Списковая форма "экспортировать" (docs/SPEC.md, "Закрытые (бинарные)
// пакеты") - альтернатива инлайн-модификатору перед самим объявлением
// ("экспортировать функция"/"экспортировать сущность"/"экспортировать
// <метод>"/"экспортировать конструктор"): вместо повторения слова перед
// каждым членом большой сущности можно один раз перечислить пути к уже
// объявленным ВЫШЕ по файлу функциям/сущностям/методам/конструкторам -
// либо построчно повторяя "экспортировать <путь>", либо одним блоком
// "экспортировать (<путь> ...)". Обе формы равноправны и МОГУТ сочетаться
// в одном файле с инлайн-модификатором - список только помечает уже
// готовые ast.FunctionDecl/EntityDecl/MethodDecl/ConstructorDecl тем же
// полем Exported, второго механизма проверки допустимости не заводится
// (internal/semantic.validateExports видит только итоговый флаг, ему
// незачем знать, КАКИМ синтаксисом он выставлен).
package parser

import (
	"github.com/IlyaFeoktistov/Rech/internal/ast"
	"github.com/IlyaFeoktistov/Rech/internal/diagnostics"
	"github.com/IlyaFeoktistov/Rech/internal/lexer"
)

// exportPathItem - один путь в списковой форме - Target: имя функции или
// сущности; Member - "" (сама функция/сущность целиком), "конструктор",
// либо имя метода сущности Target.
type exportPathItem struct {
	Target string
	Member string
	Line   int
}

// isExportListStart - после "экспортировать" следует путь (списковая
// форма), а не "функция"/"абстрактная"/"сущность"/"класс"/"структура"
// (инлайн-форма перед самим объявлением, parseExportedDecl) - LPAREN
// (блок в скобках) или IDENT (голый одиночный путь) отличают её
// однозначно, оба ключевых слова синтаксически исключены на этой позиции.
func isExportListStart(next lexer.TokenType) bool {
	return next == lexer.LPAREN || next == lexer.IDENT
}

// parseExportPathList разбирает "экспортировать <путь>" ИЛИ
// "экспортировать (<путь> ...)" - позиция парсера на входе - на самом
// "экспортировать". Пути внутри скобок разделяются переносом строки и/или
// запятой (оба разделителя допустимы одновременно, тем же принципом, что и
// везде в этом парсере).
func (p *Parser) parseExportPathList() ([]exportPathItem, error) {
	p.advance() // "экспортировать"
	if p.cur().Type != lexer.LPAREN {
		item, err := p.parseExportPath()
		if err != nil {
			return nil, err
		}
		return []exportPathItem{item}, nil
	}
	p.advance() // "("
	p.skipSeparators()
	var items []exportPathItem
	for p.cur().Type != lexer.RPAREN {
		if p.cur().Type == lexer.EOF {
			return nil, errAtLine(p.cur().Line, CodeUnclosedBracket, "незакрытая \"экспортировать(\" - ожидалась \")\"")
		}
		item, err := p.parseExportPath()
		if err != nil {
			return nil, err
		}
		items = append(items, item)
		if p.cur().Type == lexer.COMMA {
			p.advance()
		}
		p.skipSeparators()
	}
	p.advance() // ")"
	return items, nil
}

// parseExportPath разбирает один путь - "Имя" или "Имя.Член". "Член" -
// имя метода сущности (IDENT), либо ключевое слово "конструктор" (лексер
// выдаёт его отдельным типом токена, не IDENT - разбирается отдельной
// веткой).
func (p *Parser) parseExportPath() (exportPathItem, error) {
	name, err := p.expect(lexer.IDENT)
	if err != nil {
		return exportPathItem{}, diagnostics.Wrap(err, "в списке \"экспортировать\"")
	}
	item := exportPathItem{Target: name.Value, Line: name.Line}
	if p.cur().Type != lexer.DOT {
		return item, nil
	}
	p.advance() // "."
	switch p.cur().Type {
	case lexer.KwConstructor:
		p.advance()
		item.Member = "конструктор"
	case lexer.IDENT:
		item.Member = p.advance().Value
	default:
		return exportPathItem{}, errAt(p.cur().Line, p.cur().Col, CodeUnexpectedToken, "после %q ожидалось имя метода или \"конструктор\", получено %q", item.Target+".", p.cur().Value)
	}
	return item, nil
}

// applyExportItem применяет ОДИН разобранный путь к prog, уже накопившему
// все объявления, встреченные РАНЬШЕ в этом файле (список читается как
// обычная топ-уровневая декларация - цель обязана быть объявлена ВЫШЕ по
// файлу, не ниже; в отличие от "использовать"/вызовов функций, forward-
// ссылки в списке "экспортировать" не разрешены - это не вызов, а прямая
// мутация уже разобранного узла AST, второго прохода для неё не заводится).
func applyExportItem(prog *ast.Program, item exportPathItem) error {
	for _, fn := range prog.Functions {
		if fn.Name != item.Target {
			continue
		}
		if item.Member != "" {
			return errAtLine(item.Line, CodeExport, "%q - функция, у неё не может быть члена %q", item.Target, item.Member)
		}
		if fn.Name == entryPointName {
			return errAtLine(item.Line, CodeEntryPoint, "%q - точка входа программы, не может быть экспортирована", entryPointName)
		}
		fn.Exported = true
		return nil
	}
	for _, ent := range prog.Entities {
		if ent.Name != item.Target {
			continue
		}
		switch item.Member {
		case "":
			ent.Exported = true
		case "конструктор":
			if ent.Ctor == nil {
				return errAtLine(item.Line, CodeExport, "у сущности %q нет явного конструктора (нужен \"конструктор(...)\" в теле, чтобы его можно было экспортировать)", item.Target)
			}
			ent.Ctor.Exported = true
		default:
			found := false
			for _, m := range ent.Methods {
				if m.Name == item.Member {
					m.Exported = true
					found = true
					break
				}
			}
			if !found {
				return errAtLine(item.Line, CodeExport, "у сущности %q нет метода %q", item.Target, item.Member)
			}
		}
		return nil
	}
	return errAtLine(item.Line, CodeExport, "%q не объявлено выше в этом файле (список \"экспортировать\" видит только уже разобранные объявления)", item.Target)
}
