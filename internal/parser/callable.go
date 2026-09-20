package parser

import (
	"fmt"
	"strings"

	"github.com/IlyaFeoktistov/Rech/internal/ast"
	"github.com/IlyaFeoktistov/Rech/internal/diagnostics"
	"github.com/IlyaFeoktistov/Rech/internal/lexer"
)

// parseLambdaExpr разбирает "функция(<парам>: <Тип>, ...): <ТипРезультата>
// [<список захвата>] { <тело> }" без имени, в позиции выражения - та же
// грамматика параметров/тела, что и у именованного parseFunction, кроме
// имени и параметров типа (лямбда не может быть generic). Список захвата -
// необязательный "[...]" СРАЗУ ПЕРЕД телом (после типа результата, если
// есть) - см. package doc ast.LambdaExpr/parseCaptureList. Позиция парсера
// на входе - на "функция".
func (p *Parser) parseLambdaExpr() (*ast.LambdaExpr, error) {
	kw, err := p.expect(lexer.KwFunction)
	if err != nil {
		return nil, err
	}

	var params []ast.Param
	if p.cur().Type == lexer.LPAREN {
		params, err = p.parseParams()
		if err != nil {
			return nil, diagnostics.Wrap(err, "в списке параметров лямбды")
		}
	}

	returnType := ""
	if p.cur().Type == lexer.COLON {
		p.advance()
		returnType, err = p.parseTypeName()
		if err != nil {
			return nil, diagnostics.Wrap(err, "после \":\" у лямбды ожидалось имя типа результата")
		}
		returnType = normalizeReturnType(returnType)
	}

	hasCaptures := false
	var captures []ast.CaptureSpec
	if p.cur().Type == lexer.LBRACKET {
		hasCaptures = true
		captures, err = p.parseCaptureList()
		if err != nil {
			return nil, err
		}
	}

	p.skipNewlines()
	body, err := p.parseBlock()
	if err != nil {
		return nil, err
	}
	return &ast.LambdaExpr{Params: params, ReturnType: returnType, HasCaptures: hasCaptures, Captures: captures, Body: body, Line: kw.Line}, nil
}

// parseCaptureList разбирает "[x, &y, ...]" (в т.ч. пустой "[]" - лямбда,
// закрытая ото всех внешних переменных) - список имён через запятую, у
// каждого - необязательный ведущий "&" (захват по ссылке, см. package doc
// ast.CaptureSpec). Позиция парсера на входе - на "[".
func (p *Parser) parseCaptureList() ([]ast.CaptureSpec, error) {
	p.advance() // "["
	p.skipNewlines()
	var captures []ast.CaptureSpec
	if p.cur().Type != lexer.RBRACKET {
		for {
			byRef := false
			if p.cur().Type == lexer.BITAND {
				byRef = true
				p.advance()
			}
			name, err := p.expect(lexer.IDENT)
			if err != nil {
				return nil, diagnostics.Wrap(err, "в списке захвата лямбды")
			}
			captures = append(captures, ast.CaptureSpec{Name: name.Value, ByRef: byRef, Line: name.Line})
			if p.cur().Type != lexer.COMMA {
				break
			}
			p.advance() // ","
			atClose, err := p.skipNewlinesAfterListComma(lexer.RBRACKET)
			if err != nil {
				return nil, err
			}
			if atClose {
				break // висячая запятая перед "]"
			}
		}
	}
	p.skipNewlines()
	if _, err := p.expect(lexer.RBRACKET); err != nil {
		return nil, diagnostics.Wrap(err, "после списка захвата лямбды ожидалось \"]\"")
	}
	return captures, nil
}

// callableTypeName - зарезервированное имя типа "функция как значение".
const callableTypeName = "Вызываемый"

// parseCallableTypeName разбирает тип "функция как значение" после уже
// считанного имени "Вызываемый":
//
//	[ "<" generic_param { "," generic_param } ">" ] "(" [ type_name { "," type_name } ] ")" [ ":" type_name ]
//
// Список параметров в скобках - только ТИПЫ через запятую (без имён - тип
// значения не несёт собственных имён параметров, они появляются только у
// конкретной лямбды/функции, которая этому типу соответствует), тип
// результата - через ":" после скобок (пусто - без результата), вместо
// прежней формы "Вызываемый<Тип1, Тип2>Результат" (типы сразу внутри
// "<...>", результат сразу за закрывающей ">" без разделителя).
//
// Необязательный "<...>" перед скобками - параметры типа самого этого
// значения ("Вызываемый<T>(T): T" - обратный вызов, полиморфный по T), той
// же грамматикой, что у generic-сущности/функции (parseGenericParams). В
// отличие от Коробка<T>/первый<T>, у "Вызываемый" нет своего шаблонного
// тела для инстанциации - весь его "шаблон" (типы параметров/результата) и
// так виден целиком тут же, в скобках, поэтому это НЕ отдельный механизм
// мономорфизации (в дальнейшем возможно расширение, "вдруг понадобится"):
// имена внутри "<...>" не попадают в каноническую строку - каждое из них
// годится как имя типа внутри скобок/типа результата, только если такое
// имя УЖЕ разрешимо в окружающей области видимости (типичный случай -
// собственный параметр типа объемлющей generic-функции/сущности с тем же
// именем; substituteTypeString уже рекурсивно подставляет его внутрь
// строки типа "Вызываемый" при мономорфизации объемлющего шаблона, см.
// package doc internal/semantic/generics.go, - отдельного связывания не
// требуется).
//
// Позиция парсера на входе - сразу после IDENT "Вызываемый".
func (p *Parser) parseCallableTypeName(name lexer.Token) (string, error) {
	if _, err := p.parseGenericParams(); err != nil {
		return "", diagnostics.Wrap(err, "в параметрах типа %q", name.Value)
	}
	if p.cur().Type != lexer.LPAREN {
		return "", errAt(p.cur().Line, p.cur().Col, CodeUnexpectedToken, "после %q ожидался список параметров \"(...)\"", name.Value)
	}
	p.advance() // "("
	p.skipNewlines()
	var params []string
	if p.cur().Type != lexer.RPAREN {
		for {
			t, err := p.parseTypeName()
			if err != nil {
				return "", diagnostics.Wrap(err, "в параметрах типа %q", name.Value)
			}
			params = append(params, t)
			if p.cur().Type != lexer.COMMA {
				break
			}
			p.advance() // ","
			atClose, err := p.skipNewlinesAfterListComma(lexer.RPAREN)
			if err != nil {
				return "", err
			}
			if atClose {
				break // висячая запятая перед ")"
			}
		}
	}
	p.skipNewlines()
	if _, err := p.expect(lexer.RPAREN); err != nil {
		return "", diagnostics.Wrap(err, "после параметров типа %q", name.Value)
	}
	result := ""
	if p.cur().Type == lexer.COLON {
		p.advance()
		r, err := p.parseTypeName()
		if err != nil {
			return "", diagnostics.Wrap(err, "после \":\" в типе результата %q", name.Value)
		}
		result = normalizeReturnType(r)
	}
	return fmt.Sprintf("%s<%s>%s", name.Value, strings.Join(params, ", "), result), nil
}
