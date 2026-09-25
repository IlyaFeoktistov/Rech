package parser

import (
	"fmt"
	"strings"

	"github.com/IlyaFeoktistov/Rech/internal/ast"
	"github.com/IlyaFeoktistov/Rech/internal/diagnostics"
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
		return "", diagnostics.Wrap(err, "после \"как\" ожидался псевдоним")
	}
	return aliasTok.Value, nil
}

// parseUseDecl разбирает "использовать <имя> [как <псевдоним>], ... из
// "<путь>"" (шаг 11) - список имён через запятую (перенос
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
			return nil, diagnostics.Wrap(err, "после %q ожидалось имя", kw.Value)
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
	p.skipNewlines()
	if _, err := p.expect(lexer.KwOf); err != nil {
		return nil, diagnostics.Wrap(err, "после списка имён ожидалось \"из\"")
	}
	path, err := p.expect(lexer.STRING)
	if err != nil {
		return nil, diagnostics.Wrap(err, "после \"из\" ожидался путь строковым литералом")
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
		return nil, diagnostics.Wrap(err, "после %q ожидалось \"функция\"", kw.Value)
	}
	cName, err := p.expect(lexer.IDENT)
	if err != nil {
		return nil, diagnostics.Wrap(err, "после \"внешняя функция\" ожидалось имя C-функции")
	}
	alias := cName.Value
	if a, err := p.parseOptionalAlias(); err != nil {
		return nil, err
	} else if a != "" {
		alias = a
	}
	if alias == entryPointName {
		return nil, errAtLine(kw.Line, CodeEntryPoint, "%q - точка входа программы, внешняя функция не может так называться", entryPointName)
	}

	var params []ast.Param
	if p.cur().Type == lexer.LPAREN {
		params, err = p.parseParams()
		if err != nil {
			return nil, diagnostics.Wrap(err, "в списке параметров внешней функции %q", alias)
		}
		if err := rejectVariadicParam(params, "внешней функции"); err != nil {
			return nil, err
		}
	}

	returnType := ""
	if p.cur().Type == lexer.COLON {
		p.advance()
		returnType, err = p.parseTypeName()
		if err != nil {
			return nil, diagnostics.Wrap(err, "после \":\" у внешней функции %q ожидалось имя типа результата", alias)
		}
		returnType = normalizeReturnType(returnType)
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
		return nil, diagnostics.Wrap(err, "после %q ожидалось имя перечисления", kw.Value)
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
			return nil, diagnostics.Wrap(err, "перечисление %q", name.Value)
		}
		if seen[memberName.Value] {
			return nil, errAtLine(memberName.Line, CodeDuplicateDecl, "перечисление %q: член %q объявлен более одного раза", name.Value, memberName.Value)
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
				return nil, diagnostics.Wrap(err, "перечисление %q: член %q: после \"=\" ожидалось целое число", name.Value, memberName.Value)
			}
			v, err := parseIntTokenValue(numTok.Value)
			if err != nil {
				return nil, errAtLine(numTok.Line, CodeEntityInvariant, "перечисление %q: член %q: некорректное число %q", name.Value, memberName.Value, numTok.Value)
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
		return nil, diagnostics.Wrap(err, "перечисление %q", name.Value)
	}
	if len(members) == 0 {
		return nil, errAtLine(kw.Line, CodeEntityInvariant, "перечисление %q должно содержать хотя бы один член", name.Value)
	}
	return &ast.EnumDecl{Name: name.Value, Members: members, Line: kw.Line}, nil
}

// parseStatesSection разбирает "состояние:" и краткие формы состояний,
// перечисленные через пробел на той же строке (шаг 4.1, упрощённая
// версия) - в отличие от "имеет:", где каждое поле на своей строке с
// отступом, здесь ровно одна строка, как во всех остальных
// "перечислениях через пробел" в языке (docs/GRAMMAR_DRAFT.md). Для каждого
// слова сразу синтезирует производный тип (StateDecl.TypeName) - полная,
// согласованная по роду сущности entityName форма слова (см.
// stateWordAgreement) + само имя сущности ("авторизован" + "Пользователь"
// (муж.) -> "авторизованныйПользователь" с большой буквы ->
// "АвторизованныйПользователь"). Если род сущности не определяется
// автоматически (morph.NounAgreement) - явная ошибка, а не угаданная
// форма: в отличие от согласования "новый"/"новая" у конструктора
// (checkNewKeywordAgreement), здесь нет "любая форма принимается"
// запасного варианта - производному типу нужно какое-то одно конкретное имя.

func (p *Parser) parseFunction(isAsync bool) (*ast.FunctionDecl, error) {
	p.enterDecl()
	kw, err := p.expect(lexer.KwFunction)
	if err != nil {
		return nil, err
	}
	name, err := p.expect(lexer.IDENT)
	if err != nil {
		return nil, diagnostics.Wrap(err, "после \"функция\" ожидалось имя функции")
	}
	if isAsync && name.Value == entryPointName {
		return nil, errAtLine(kw.Line, CodeEntryPoint, "%q - точка входа программы, не может быть \"асинхронная\"", entryPointName)
	}
	typeParams, err := p.parseGenericParams()
	if err != nil {
		return nil, diagnostics.Wrap(err, "функция %q", name.Value)
	}
	if name.Value == entryPointName && typeParams != nil {
		return nil, errAtLine(kw.Line, CodeEntryPoint, "%q - точка входа программы, не может быть generic", entryPointName)
	}

	var params []ast.Param
	if p.cur().Type == lexer.LPAREN {
		params, err = p.parseParams()
		if err != nil {
			return nil, diagnostics.Wrap(err, "в списке параметров функции %q", name.Value)
		}
	}

	returnType := ""
	if p.cur().Type == lexer.COLON {
		p.advance()
		var err error
		returnType, err = p.parseTypeName()
		if err != nil {
			return nil, diagnostics.Wrap(err, "после \":\" у функции %q ожидалось имя типа результата", name.Value)
		}
		returnType = normalizeReturnType(returnType)
	}

	if name.Value == entryPointName && (len(params) > 0 || returnType != "") {
		return nil, errAtLine(kw.Line, CodeEntryPoint, "%q - точка входа программы, у неё фиксированная сигнатура: нельзя объявить параметры или тип результата", entryPointName)
	}

	var tag string
	if p.cur().Type == lexer.TAG {
		tag = p.advance().Value
	}

	p.skipNewlines()
	restoreVarTypes := p.beginVarTypeScope(params)
	body, err := p.parseBlock()
	restoreVarTypes()
	if err != nil {
		return nil, err
	}
	return &ast.FunctionDecl{Name: name.Value, Params: params, ReturnType: returnType, Body: body, TypeParams: typeParams, Line: kw.Line, Tag: tag, IsAsync: isAsync}, nil
}

// parseParams разбирает "(" [ param { "," param } ] ")". Позиция парсера
// на входе - на "(".
func (p *Parser) parseParams() ([]ast.Param, error) {
	p.advance() // "("
	// Перенос строки допустим сразу после "(" и после каждой "," - тот же
	// приём, что и у списка имён "использовать" (parseUseDecl) - длинный
	// список параметров можно разбить на несколько строк, каждая со своим
	// параметром.
	p.skipNewlines()
	var params []ast.Param
	seenCases := map[morph.Case]string{}
	if p.cur().Type != lexer.RPAREN {
		for {
			param, err := p.parseParam()
			if err != nil {
				return nil, err
			}
			// Параметр - существительное (падежная роль или обычное имя),
			// естественному вызову нужно уметь опознать его словоформу в
			// теле функции/метода так же, как и у любой обычной переменной
			// (registerNoun) - раньше не регистрировался вовсе, поэтому
			// параметр нельзя было передать в НАТУРАЛЬНЫЙ вызов (ни
			// свободной функции, ни метода) без предварительного
			// присваивания его локальной переменной.
			p.registerNoun(param.Name)
			if param.CaseWord != "" {
				c, _ := morph.CaseOfParamName(param.CaseWord)
				if prev, exists := seenCases[c]; exists {
					return nil, errAtLine(param.Line, CodeNaturalCall, "параметр %q дублирует падежную роль параметра %q - у каждого падежа может быть только один параметр", param.Name, prev)
				}
				seenCases[c] = param.Name
			}
			params = append(params, param)
			if p.cur().Type != lexer.COMMA {
				break
			}
			p.advance() // ","
			atClose, err := p.skipNewlinesAfterListComma(lexer.RPAREN)
			if err != nil {
				return nil, err
			}
			if atClose {
				break // висячая запятая перед ")"
			}
		}
	}
	p.skipNewlines()
	if _, err := p.expect(lexer.RPAREN); err != nil {
		return nil, err
	}
	if err := checkVariadicParam(params); err != nil {
		return nil, err
	}
	if err := checkTrailingDefaults(params); err != nil {
		return nil, err
	}
	return params, nil
}

// checkVariadicParam проверяет вариативный параметр ("...имя: Тип"): он
// единственный, последний и без значения по умолчанию. Его Type
// переписывается в "Список<Тип>" - так параметр виден в теле функции.
func checkVariadicParam(params []ast.Param) error {
	for i := range params {
		if !params[i].Variadic {
			continue
		}
		if i != len(params)-1 {
			return errAtLine(params[i].Line, CodeUnexpectedToken, "вариативный параметр %q должен быть последним", params[i].Name)
		}
		for _, other := range params {
			if other.Default != nil {
				return errAtLine(other.Line, CodeUnexpectedToken, "параметр %q: значения по умолчанию нельзя сочетать с вариативным параметром %q", other.Name, params[i].Name)
			}
		}
		params[i].Type = "Список<" + params[i].Type + ">"
	}
	return nil
}

// rejectVariadicParam - вариативный параметр допустим только у функций и
// методов; where - что объявляется ("лямбды", "конструктора", ...).
func rejectVariadicParam(params []ast.Param, where string) error {
	for _, param := range params {
		if param.Variadic {
			return errAtLine(param.Line, CodeUnexpectedToken, "у %s не может быть вариативного параметра %q - только у функций и методов", where, param.Name)
		}
	}
	return nil
}

// checkTrailingDefaults проверяет, что значения по умолчанию (Param.Default)
// идут только ХВОСТОМ списка параметров - раз параметр получил default,
// каждый следующий за ним обязан иметь его тоже (иначе непонятно, что
// подставлять на пропущенное место при вызове с меньшим числом аргументов,
// чем объявлено) - тот же принцип, что и у полей сущности
// (docs/SPEC.md, "Явные конструкторы").
func checkTrailingDefaults(params []ast.Param) error {
	seenDefault := false
	for _, p := range params {
		if p.Default != nil {
			seenDefault = true
			continue
		}
		if seenDefault {
			return errAtLine(p.Line, CodeUnexpectedToken, "параметр %q без значения по умолчанию идёт после параметра со значением по умолчанию - значения по умолчанию допустимы только хвостом списка параметров", p.Name)
		}
	}
	return nil
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
// normalizeReturnType - "Пусто" как тип результата - чисто синтаксическая
// форма "нет типа результата", эквивалентная полностью пропущенной
// аннотации ": Тип" - схлопывается сюда же, в самом парсере, чтобы дальше
// по конвейеру (семантика, backend'ы) вообще не знали об этом имени.
// Только для ВОЗВРАЩАЕМОГО типа - как обычное имя типа параметра/поля
// "Пусто" не имеет смысла и не нормализуется (см. вызовы parseTypeName без
// этой обёртки).
func normalizeReturnType(t string) string {
	if t == "Пусто" {
		return ""
	}
	return t
}

// parseTypeName разбирает имя типа - IDENT, необязательно с аргументами
// типа ("Список<Целое>"), необязательно с ведущим "?" (nullable, "?Тип") или
// "*" (указатель, "*Тип") и необязательно с одним и более ЗАВЕРШАЮЩИМИ "[]" -
// "Тип[]"/"Тип[][]" - чистый синтаксический сахар над
// "Список<Тип>"/"Список<Список<Тип>>" (массивоподобная запись, привычная по
// C#/Java/TypeScript) и над "Указатель<Тип>" (запись через "*", привычная по
// C/C++/Go/Rust) - раскрывается прямо здесь, в строку имени типа, до того,
// как её вообще увидит resolveTypeName - ни один потребитель ниже
// (семантика/backend'ы) не отличает "Целое[]"/"*Целое" от буквально
// написанного "Список<Целое>"/"Указатель<Целое>".
func (p *Parser) parseTypeName() (string, error) {
	if p.cur().Type == lexer.QUESTION {
		p.advance()
		inner, err := p.parseTypeName()
		if err != nil {
			return "", diagnostics.Wrap(err, "после \"?\"")
		}
		return "?" + inner, nil
	}
	if p.cur().Type == lexer.STAR {
		p.advance()
		inner, err := p.parseTypeName()
		if err != nil {
			return "", diagnostics.Wrap(err, "после \"*\"")
		}
		return fmt.Sprintf("Указатель<%s>", inner), nil
	}
	name, err := p.expect(lexer.IDENT)
	if err != nil {
		return "", err
	}
	if name.Value == callableTypeName {
		return p.parseCallableTypeName(name)
	}
	result, err := p.parseTypeArgsSuffix(name.Value)
	if err != nil {
		return "", err
	}
	// Лукахед на ОБА токена ("[" и сразу за ним "]"), не только на "[" -
	// "[" сразу после имени типа результата у лямбды - синтаксически то же
	// место, что и список захвата ("функция(...): Тип [x, &y] { ... }",
	// callable.go, parseCaptureList): "[x" - список захвата, не "Тип[]".
	// Пустой "[]" различается пробелом: слитно ("Целое[]") - сокращение
	// "Список<Целое>", через пробел ("Целое []") - пустой список захвата
	// (лямбда, закрытая ото всех внешних переменных) - "[" тогда остаётся
	// нетронутым для parseLambdaExpr.
	for p.cur().Type == lexer.LBRACKET && !p.cur().SpaceBefore && p.peekAt(1).Type == lexer.RBRACKET {
		p.advance() // "["
		p.advance() // "]"
		result = fmt.Sprintf("Список<%s>", result)
	}
	return result, nil
}

// parseTypeArgsSuffix - "<Тип, ...>" сразу после имени типа name (уже
// прочитанного), если он есть: "Коробка<Целое>"; иначе name как есть.
func (p *Parser) parseTypeArgsSuffix(name string) (string, error) {
	if p.cur().Type != lexer.LT {
		return name, nil
	}
	p.advance() // "<"
	p.skipNewlines()
	var args []string
	for {
		arg, err := p.parseTypeName()
		if err != nil {
			return "", diagnostics.Wrap(err, "в аргументе типа %q", name)
		}
		args = append(args, arg)
		if p.cur().Type != lexer.COMMA {
			break
		}
		p.advance() // ","
		atClose, err := p.skipNewlinesAfterAngleComma()
		if err != nil {
			return "", err
		}
		if atClose {
			break // висячая запятая перед ">"
		}
	}
	p.skipNewlines()
	if err := p.expectCloseAngle(); err != nil {
		return "", diagnostics.Wrap(err, "после аргументов типа %q<%s>", name, strings.Join(args, ", "))
	}
	return fmt.Sprintf("%s<%s>", name, strings.Join(args, ", ")), nil
}

// parseParam разбирает параметр в одной из форм:
//
//	падеж: Тип             - падежное слово одновременно и роль, и имя
//	падеж имя: Тип         - падежная роль отдельно от отображаемого имени
//	имя: Тип               - без падежной роли, только позиционный параметр
//	с кем/с чем [имя]: Тип - двусловная запись ParamWithWhom/ParamWithWhat
//	                          (morph.RequiresMandatoryPreposition) - "с" сам
//	                          по себе не IDENT (лексер отдаёт его как
//	                          KwWith), поэтому разбирается отдельно, до
//	                          обычного expect(IDENT) ниже. "с_кем"/"с_чем"
//	                          слитно - та же роль, обычным путём (это уже
//	                          просто IDENT, morph.CaseOfParamName их знает).
//
// Различаются лукахедом: если первое слово - падежное (morph.CaseOfParamName)
// и следом идёт ещё один IDENT (а не ":"), это двусловная форма; иначе
// первое слово - и есть Name (падежное или нет).
func (p *Parser) parseParam() (ast.Param, error) {
	if p.cur().Type == lexer.ELLIPSIS {
		p.advance() // "..."
		param, err := p.parseParam()
		if err != nil {
			return param, err
		}
		param.Variadic = true
		return param, nil
	}
	if p.cur().Type == lexer.KwWith && p.peekAt(1).Type == lexer.IDENT &&
		(p.peekAt(1).Value == "кем" || p.peekAt(1).Value == "чем") {
		return p.parseTwoWordCaseParam(morph.ParamWithWhom, morph.ParamWithWhat)
	}
	if p.cur().Type == lexer.KwFor && p.peekAt(1).Type == lexer.IDENT &&
		(p.peekAt(1).Value == "кого" || p.peekAt(1).Value == "чего") {
		return p.parseTwoWordCaseParam(morph.ParamForWhom, morph.ParamForWhat)
	}
	first, err := p.expect(lexer.IDENT)
	if err != nil {
		return ast.Param{}, diagnostics.Wrap(err, "ожидалось имя параметра")
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
		return ast.Param{}, diagnostics.Wrap(err, "после имени параметра %q ожидалось \":\" и тип", name.Value)
	}
	typeName, err := p.parseTypeName()
	if err != nil {
		return ast.Param{}, diagnostics.Wrap(err, "после \":\" у параметра %q ожидалось имя типа", name.Value)
	}
	def, err := p.parseParamDefault(name.Value)
	if err != nil {
		return ast.Param{}, err
	}
	return ast.Param{Name: name.Value, CaseWord: caseWord, Type: typeName, Line: first.Line, Default: def}, nil
}

// parseParamDefault разбирает необязательное "= <выражение>" после типа
// параметра ("параметр: Тип = выражение") - nil, если "=" нет (ничего не
// потребляет). Тем же принципом, что и значение по умолчанию поля сущности
// (parseField, entity.go).
func (p *Parser) parseParamDefault(paramName string) (ast.Expr, error) {
	if p.cur().Type != lexer.ASSIGN {
		return nil, nil
	}
	p.advance()
	def, err := p.parseExpr()
	if err != nil {
		return nil, diagnostics.Wrap(err, "в значении по умолчанию параметра %q", paramName)
	}
	return def, nil
}

// parseTwoWordCaseParam разбирает двухсловную запись падежной роли с
// обязательным предлогом (сам предлог уже проверен лукахедом в parseParam) -
// "с кем"/"с чем" (animateWord=ParamWithWhom, inanimateWord=ParamWithWhat)
// или "для кого"/"для чего" (ParamForWhom/ParamForWhat) - [имя]: Тип.
func (p *Parser) parseTwoWordCaseParam(animateWord, inanimateWord string) (ast.Param, error) {
	withTok := p.advance() // "с"/"для"
	word := p.advance()    // "кем"/"чем" или "кого"/"чего"
	caseWord := animateWord
	if word.Value == "чем" || word.Value == "чего" {
		caseWord = inanimateWord
	}
	name := word
	if p.cur().Type == lexer.IDENT {
		var err error
		name, err = p.expect(lexer.IDENT)
		if err != nil {
			return ast.Param{}, err
		}
	}
	if _, err := p.expect(lexer.COLON); err != nil {
		return ast.Param{}, diagnostics.Wrap(err, "после имени параметра %q ожидалось \":\" и тип", name.Value)
	}
	typeName, err := p.parseTypeName()
	if err != nil {
		return ast.Param{}, diagnostics.Wrap(err, "после \":\" у параметра %q ожидалось имя типа", name.Value)
	}
	def, err := p.parseParamDefault(name.Value)
	if err != nil {
		return ast.Param{}, err
	}
	return ast.Param{Name: name.Value, CaseWord: caseWord, Type: typeName, Line: withTok.Line, Default: def}, nil
}
