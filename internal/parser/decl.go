package parser

import (
	"fmt"
	"strconv"
	"strings"

	"github.com/IlyaFeoktistov/Rech/internal/ast"
	"github.com/IlyaFeoktistov/Rech/internal/lexer"
	"github.com/IlyaFeoktistov/Rech/internal/morph"
)

// parseOptionalAlias разбирает необязательное "как <Псевдоним>" после уже
// разобранного имени - общая часть "использовать <имя> [как <псевдоним>]"
// (parseUseDecl) и "внешняя функция <ИмяC> [как <Псевдоним>]"
// (parseExternFuncDecl). "" - альтернативного имени нет, вызывающая сторона
// использует исходное имя как есть.
func (p *Parser) parseOptionalAlias() (string, error) {
	if p.cur().Type != lexer.KwAs {
		return "", nil
	}
	p.advance() // "как"
	aliasTok, err := p.expect(lexer.IDENT)
	if err != nil {
		return "", fmt.Errorf("после \"как\" ожидался псевдоним: %w", err)
	}
	return aliasTok.Value, nil
}

// parseUseDecl разбирает "использовать <имя> [как <псевдоним>], ... из
// "<путь>"" (шаг 11, docs/ROADMAP.md) - список имён через запятую (перенос
// строки сразу после запятой допустим, чтобы длинный список можно было
// разбить на несколько строк), каждое опционально с "как <псевдоним>"
// (переименование при импорте - единственный способ ввести два одноимённых
// в исходных файлах импорта одновременно, не сталкиваясь друг с другом),
// затем "из" и путь строковым литералом. Разрешение пути (относительно
// каталога текущего файла), проверка существования импортируемых имён в
// целевом файле и видимости - вне парсера, см. internal/module.
func (p *Parser) parseUseDecl() (*ast.UseDecl, error) {
	kw := p.advance() // "использовать"
	var names []ast.UseName
	for {
		name, err := p.expect(lexer.IDENT)
		if err != nil {
			return nil, fmt.Errorf("после %q ожидалось имя: %w", kw.Value, err)
		}
		alias := name.Value
		if a, err := p.parseOptionalAlias(); err != nil {
			return nil, err
		} else if a != "" {
			alias = a
		}
		names = append(names, ast.UseName{Name: name.Value, Alias: alias, Line: name.Line})
		if p.cur().Type != lexer.COMMA {
			break
		}
		p.advance() // ","
		p.skipNewlines()
	}
	if _, err := p.expect(lexer.KwOf); err != nil {
		return nil, fmt.Errorf("после списка имён ожидалось \"из\": %w", err)
	}
	path, err := p.expect(lexer.STRING)
	if err != nil {
		return nil, fmt.Errorf("после \"из\" ожидался путь строковым литералом: %w", err)
	}
	return &ast.UseDecl{Names: names, Path: path.Value, Line: kw.Line}, nil
}

// parseExternFuncDecl разбирает "внешняя функция <ИмяC> [как <Псевдоним>]
// (<парам>: <Тип>, ...) [: <ТипВозврата>]" (шаг 7, FFI) - объявление без
// тела: реализация - уже существующий C-символ ИмяC. "как" - тот же
// принцип переименования, что и у "использовать" (parseUseDecl) - без него
// Alias == ИмяC. Проверка допустимости типов параметров/результата (только
// Целое/Число/Строка/Логическое в этом срезе) - в internal/semantic
// (isFFIScalar), не здесь.
func (p *Parser) parseExternFuncDecl() (*ast.ExternFuncDecl, error) {
	kw, err := p.expect(lexer.KwExternal)
	if err != nil {
		return nil, err
	}
	if _, err := p.expect(lexer.KwFunction); err != nil {
		return nil, fmt.Errorf("после %q ожидалось \"функция\": %w", kw.Value, err)
	}
	cName, err := p.expect(lexer.IDENT)
	if err != nil {
		return nil, fmt.Errorf("после \"внешняя функция\" ожидалось имя C-функции: %w", err)
	}
	alias := cName.Value
	if a, err := p.parseOptionalAlias(); err != nil {
		return nil, err
	} else if a != "" {
		alias = a
	}
	if alias == entryPointName {
		return nil, fmt.Errorf("строка %d: %q - точка входа программы, внешняя функция не может так называться", kw.Line, entryPointName)
	}

	var params []ast.Param
	if p.cur().Type == lexer.LPAREN {
		params, err = p.parseParams()
		if err != nil {
			return nil, fmt.Errorf("в списке параметров внешней функции %q: %w", alias, err)
		}
	}

	returnType := ""
	if p.cur().Type == lexer.COLON {
		p.advance()
		returnType, err = p.parseTypeName()
		if err != nil {
			return nil, fmt.Errorf("после \":\" у внешней функции %q ожидалось имя типа результата: %w", alias, err)
		}
	}

	return &ast.ExternFuncDecl{CName: cName.Value, Alias: alias, Params: params, ReturnType: returnType, Line: kw.Line}, nil
}

// parseEnumDecl разбирает "перечисление <Имя> { Член1, Член2 = 5, ... }"
// (шаг 12) - список членов через запятую и/или перенос строки (оба
// разделителя допустимы, тем же принципом, что и везде в этом парсере, а
// не единственно допустимой запятой, как у "использовать"). Член без явного
// "= <целое>" получает значение "предыдущее + 1" (0 у самого первого члена
// без явного значения) - вычисляется прямо здесь, чтобы internal/ast.
// EnumMember.Value уже нёс готовое число, без разницы явное оно или нет.
func (p *Parser) parseEnumDecl() (*ast.EnumDecl, error) {
	kw, err := p.expect(lexer.KwEnum)
	if err != nil {
		return nil, err
	}
	name, err := p.expect(lexer.IDENT)
	if err != nil {
		return nil, fmt.Errorf("после %q ожидалось имя перечисления: %w", kw.Value, err)
	}
	p.skipNewlines()
	if _, err := p.expect(lexer.LBRACE); err != nil {
		return nil, err
	}
	p.skipSeparators()

	var members []ast.EnumMember
	seen := map[string]bool{}
	next := int64(0)
	for p.cur().Type != lexer.RBRACE {
		memberName, err := p.expect(lexer.IDENT)
		if err != nil {
			return nil, fmt.Errorf("перечисление %q: %w", name.Value, err)
		}
		if seen[memberName.Value] {
			return nil, fmt.Errorf("строка %d: перечисление %q: член %q объявлен более одного раза", memberName.Line, name.Value, memberName.Value)
		}
		seen[memberName.Value] = true

		value := next
		if p.cur().Type == lexer.ASSIGN {
			p.advance()
			neg := false
			if p.cur().Type == lexer.MINUS {
				neg = true
				p.advance()
			}
			numTok, err := p.expect(lexer.INT)
			if err != nil {
				return nil, fmt.Errorf("перечисление %q: член %q: после \"=\" ожидалось целое число: %w", name.Value, memberName.Value, err)
			}
			v, err := strconv.ParseInt(numTok.Value, 10, 64)
			if err != nil {
				return nil, fmt.Errorf("строка %d: перечисление %q: член %q: некорректное число %q", numTok.Line, name.Value, memberName.Value, numTok.Value)
			}
			if neg {
				v = -v
			}
			value = v
		}
		members = append(members, ast.EnumMember{Name: memberName.Value, Value: value, Line: memberName.Line})
		next = value + 1

		if p.cur().Type == lexer.COMMA {
			p.advance()
			p.skipSeparators()
			continue
		}
		if p.skipSeparators() > 0 {
			continue
		}
		break
	}
	if _, err := p.expect(lexer.RBRACE); err != nil {
		return nil, fmt.Errorf("перечисление %q: %w", name.Value, err)
	}
	if len(members) == 0 {
		return nil, fmt.Errorf("строка %d: перечисление %q должно содержать хотя бы один член", kw.Line, name.Value)
	}
	return &ast.EnumDecl{Name: name.Value, Members: members, Line: kw.Line}, nil
}

// parseStatesSection разбирает "состояние:" и краткие формы состояний,
// перечисленные через пробел на той же строке (шаг 4.1, упрощённая
// версия) - в отличие от "имеет:", где каждое поле на своей строке с
// отступом, здесь ровно одна строка, как во всех остальных
// "перечислениях через пробел" в языке (docs/GRAMMAR.md). Для каждого
// слова сразу синтезирует производный тип (StateDecl.TypeName) - полная,
// согласованная по роду сущности entityName форма слова (см.
// stateWordAgreement) + само имя сущности ("авторизован" + "Пользователь"
// (муж.) -> "авторизованныйПользователь" с большой буквы ->
// "АвторизованныйПользователь"). Если род сущности не определяется
// автоматически (morph.NounAgreement) - явная ошибка, а не угаданная
// форма: в отличие от согласования "новый"/"новая" у конструктора
// (checkNewKeywordAgreement), здесь нет "любая форма принимается"
// запасного варианта - производному типу нужно какое-то одно конкретное имя.

func (p *Parser) parseFunction() (*ast.FunctionDecl, error) {
	kw, err := p.expect(lexer.KwFunction)
	if err != nil {
		return nil, err
	}
	name, err := p.expect(lexer.IDENT)
	if err != nil {
		return nil, fmt.Errorf("после \"функция\" ожидалось имя функции: %w", err)
	}
	typeParams, err := p.parseGenericParams()
	if err != nil {
		return nil, fmt.Errorf("функция %q: %w", name.Value, err)
	}
	if name.Value == entryPointName && typeParams != nil {
		return nil, fmt.Errorf("строка %d: %q - точка входа программы, не может быть generic", kw.Line, entryPointName)
	}

	var params []ast.Param
	if p.cur().Type == lexer.LPAREN {
		params, err = p.parseParams()
		if err != nil {
			return nil, fmt.Errorf("в списке параметров функции %q: %w", name.Value, err)
		}
	}

	returnType := ""
	if p.cur().Type == lexer.COLON {
		p.advance()
		var err error
		returnType, err = p.parseTypeName()
		if err != nil {
			return nil, fmt.Errorf("после \":\" у функции %q ожидалось имя типа результата: %w", name.Value, err)
		}
	}

	if name.Value == entryPointName && (len(params) > 0 || returnType != "") {
		return nil, fmt.Errorf("строка %d: %q - точка входа программы, у неё фиксированная сигнатура: нельзя объявить параметры или тип результата", kw.Line, entryPointName)
	}

	p.skipNewlines()
	body, err := p.parseBlock()
	if err != nil {
		return nil, err
	}
	return &ast.FunctionDecl{Name: name.Value, Params: params, ReturnType: returnType, Body: body, TypeParams: typeParams, Line: kw.Line}, nil
}

// parseParams разбирает "(" [ param { "," param } ] ")". Позиция парсера
// на входе - на "(".
func (p *Parser) parseParams() ([]ast.Param, error) {
	p.advance() // "("
	var params []ast.Param
	seenCases := map[morph.Case]string{}
	if p.cur().Type != lexer.RPAREN {
		for {
			param, err := p.parseParam()
			if err != nil {
				return nil, err
			}
			if param.CaseWord != "" {
				c, _ := morph.CaseOfParamName(param.CaseWord)
				if prev, exists := seenCases[c]; exists {
					return nil, fmt.Errorf("строка %d: параметр %q дублирует падежную роль параметра %q - у каждого падежа может быть только один параметр",
						param.Line, param.Name, prev)
				}
				seenCases[c] = param.Name
			}
			params = append(params, param)
			if p.cur().Type != lexer.COMMA {
				break
			}
			p.advance() // ","
		}
	}
	if _, err := p.expect(lexer.RPAREN); err != nil {
		return nil, err
	}
	return params, nil
}

// parseTypeName разбирает имя типа: IDENT [ "<" type_name { "," type_name } ">" ] -
// обычное имя (примитив или сущность) либо generic-инстанциация вроде
// "Список<Целое>" (встроенные коллекции, шаг 4.2) или "Коробка<Целое,
// Строка>" (пользовательские generic-сущности/функции, несколько
// параметров типа). Кодируется в одну строку - AST продолжает
// хранить тип как string везде (Param.Type, Field.Type, ReturnType,
// NewExpr.Type), backend сам разбирает и резолвит эту строку
// (internal/backend/cbackend, resolveType). "<"/">" - уже существующие
// токены LT/GT (операторы сравнения) - неоднозначности с выражениями нет,
// это отдельная синтаксическая позиция (сразу после имени типа).
func (p *Parser) parseTypeName() (string, error) {
	name, err := p.expect(lexer.IDENT)
	if err != nil {
		return "", err
	}
	if p.cur().Type != lexer.LT {
		return name.Value, nil
	}
	p.advance() // "<"
	var args []string
	for {
		arg, err := p.parseTypeName()
		if err != nil {
			return "", fmt.Errorf("в аргументе типа %q: %w", name.Value, err)
		}
		args = append(args, arg)
		if p.cur().Type != lexer.COMMA {
			break
		}
		p.advance() // ","
	}
	if err := p.expectCloseAngle(); err != nil {
		return "", fmt.Errorf("после аргументов типа %q<%s>: %w", name.Value, strings.Join(args, ", "), err)
	}
	return fmt.Sprintf("%s<%s>", name.Value, strings.Join(args, ", ")), nil
}

// parseParam разбирает параметр в одной из двух форм:
//
//	падеж: Тип             - падежное слово одновременно и роль, и имя
//	падеж имя: Тип         - падежная роль отдельно от отображаемого имени
//	имя: Тип               - без падежной роли, только позиционный параметр
//
// Различаются лукахедом: если первое слово - падежное (morph.CaseOfParamName)
// и следом идёт ещё один IDENT (а не ":"), это двусловная форма; иначе
// первое слово - и есть Name (падежное или нет).
func (p *Parser) parseParam() (ast.Param, error) {
	first, err := p.expect(lexer.IDENT)
	if err != nil {
		return ast.Param{}, fmt.Errorf("ожидалось имя параметра: %w", err)
	}
	caseWord := ""
	name := first
	if _, ok := morph.CaseOfParamName(first.Value); ok {
		caseWord = first.Value
		if p.cur().Type == lexer.IDENT {
			name, err = p.expect(lexer.IDENT)
			if err != nil {
				return ast.Param{}, err
			}
		}
	}
	if _, err := p.expect(lexer.COLON); err != nil {
		return ast.Param{}, fmt.Errorf("после имени параметра %q ожидалось \":\" и тип: %w", name.Value, err)
	}
	typeName, err := p.parseTypeName()
	if err != nil {
		return ast.Param{}, fmt.Errorf("после \":\" у параметра %q ожидалось имя типа: %w", name.Value, err)
	}
	return ast.Param{Name: name.Value, CaseWord: caseWord, Type: typeName, Line: first.Line}, nil
}
