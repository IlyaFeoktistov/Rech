package parser

import (
	"fmt"
	"strconv"

	"github.com/IlyaFeoktistov/Rech/internal/ast"
	"github.com/IlyaFeoktistov/Rech/internal/lexer"
)

func (p *Parser) parseExpr() (ast.Expr, error) {
	return p.parseCoalesce()
}

// parseCoalesce - "а ?? б" (null-coalescing) - самый низкий приоритет
// (слабее "или"), левоассоциативно, тем же шаблоном, что и остальные
// левоассоциативные уровни (parseLeftAssoc), но отдельной функцией - "??" не
// делит токен ни с чем другим (в отличие от, например, "&"/"&&"), заводить
// под него запись в общей таблице ops не нужно.
var coalesceOps = map[lexer.TokenType]string{lexer.QUESTIONQUESTION: "??"}

func (p *Parser) parseCoalesce() (ast.Expr, error) {
	return p.parseLeftAssoc(p.parseOr, coalesceOps)
}

// ParseExpr разбирает p.tokens как одно самостоятельное выражение (не всю
// программу) - нужно отладчику (internal/interp, вычисление выражений в
// остановленном кадре, "Debug Console") для разбора текста, введённого
// пользователем. Естественный (падежный) вызов функции синтаксически
// возможен только как отдельная инструкция, не как выражение (docs/SPEC.md,
// "Вызов функции") - поэтому, в отличие от ParseProgram, здесь не нужен
// предварительный проход collectFunctionSignatures (он существует только
// ради разбора естественного вызова).
func (p *Parser) ParseExpr() (ast.Expr, error) {
	e, err := p.parseExpr()
	if err != nil {
		return nil, err
	}
	p.skipNewlines()
	if p.cur().Type != lexer.EOF {
		return nil, fmt.Errorf("строка %d, столбец %d: лишние символы после выражения: %q", p.cur().Line, p.cur().Col, p.cur().Value)
	}
	return e, nil
}

// parseLeftAssoc - общий шаблон левоассоциативного бинарного оператора:
// разобрать операнд через next, затем в цикле, пока текущий токен есть в
// ops, разобрать оператор и правый операнд тем же next, сворачивая в
// левоассоциативное дерево. Общий для всех уровней приоритета ниже
// (или/и/побитовые/аддитивный/мультипликативный) - отличаются только
// следующим уровнем (next) и таблицей операторов (ops). parseComparison -
// отдельная функция (сравнения в Rech не ассоциативны - "a < b < c" не
// разбирается, значит это не цикл, а одиночная проверка).
func (p *Parser) parseLeftAssoc(next func() (ast.Expr, error), ops map[lexer.TokenType]string) (ast.Expr, error) {
	left, err := next()
	if err != nil {
		return nil, err
	}
	for {
		opStr, ok := ops[p.cur().Type]
		if !ok {
			return left, nil
		}
		op := p.advance()
		right, err := next()
		if err != nil {
			return nil, err
		}
		left = &ast.BinaryExpr{Op: opStr, Left: left, Right: right, Line: op.Line}
	}
}

var orOps = map[lexer.TokenType]string{lexer.KwOr: "или"}

func (p *Parser) parseOr() (ast.Expr, error) {
	return p.parseLeftAssoc(p.parseAnd, orOps)
}

var andOps = map[lexer.TokenType]string{lexer.KwAnd: "и"}

func (p *Parser) parseAnd() (ast.Expr, error) {
	return p.parseLeftAssoc(p.parseNot, andOps)
}

func (p *Parser) parseNot() (ast.Expr, error) {
	if p.cur().Type == lexer.KwNot {
		op := p.advance()
		operand, err := p.parseNot()
		if err != nil {
			return nil, err
		}
		return &ast.UnaryExpr{Op: "не", Operand: operand, Line: op.Line}, nil
	}
	return p.parseComparison()
}

var comparisonOps = map[lexer.TokenType]string{
	lexer.EQ:  "==",
	lexer.NEQ: "!=",
	lexer.LT:  "<",
	lexer.GT:  ">",
	lexer.LE:  "<=",
	lexer.GE:  ">=",
}

func (p *Parser) parseComparison() (ast.Expr, error) {
	left, err := p.parseBitOr()
	if err != nil {
		return nil, err
	}
	if opStr, ok := comparisonOps[p.cur().Type]; ok {
		op := p.advance()
		right, err := p.parseBitOr()
		if err != nil {
			return nil, err
		}
		return &ast.BinaryExpr{Op: opStr, Left: left, Right: right, Line: op.Line}, nil
	}
	if p.cur().Type == lexer.KwRelates {
		return p.parseIsExpr(left)
	}
	if p.cur().Type == lexer.KwAs {
		return p.parseAsExpr(left)
	}
	return left, nil
}

// parseIsExpr разбирает хвост "относится к <Тип>" уже после того, как left
// (проверяемое выражение) разобран - та же не цепочная позиция, что и у
// обычного сравнения (docs/SPEC.md). "к" грамматически требует дательного
// падежа ("относиться к кому/чему"), но Тип берётся словарной формой как
// есть, БЕЗ проверки склонения (в отличие от "наследует"/"реализует",
// resolveDeclinedTypeRef) - курируемый классификатор Фазы A слепо склоняет
// хвост всей строки составного имени как одно простое слово, что для
// частого в языке паттерна "Голова+РодительныйДовесок" (например,
// "ОшибкаДеления") даёт грамматически бессмысленную форму ("ОшибкаДелении"
// вместо осмысленного "Ошибке Деления" - голова должна склоняться, а не
// хвост) - решение пользователя: не подключать здесь заведомо неверное
// склонение, дождаться настоящего морфологического движка, который умеет
// разбирать составные имена. См. docs/GRAMMAR.md, "Открытый вопрос:
// склонение составных имён".
func (p *Parser) parseIsExpr(left ast.Expr) (ast.Expr, error) {
	kw := p.advance() // "относится"
	if _, err := p.expect(lexer.KwToward); err != nil {
		return nil, fmt.Errorf("после \"относится\" ожидалось \"к\": %w", err)
	}
	target, err := p.expect(lexer.IDENT)
	if err != nil {
		return nil, fmt.Errorf("после \"относится к\" ожидалось имя типа: %w", err)
	}
	return &ast.IsExpr{Value: left, Target: target.Value, Line: kw.Line}, nil
}

// parseBitOr/parseBitXor/parseBitAnd/parseShift - побитовые операторы
// (только Целое, docs/SPEC.md) - вставлены между сравнениями и "+"/"-" в
// том же относительном порядке, что и в C ("|" слабее "^" слабее "&"
// слабее сдвигов слабее "+"/"-"), хотя сам верхний уровень (или/и/не)
// у Rech стоит иначе, чем в C - это уже существующее, отдельное решение,
// здесь не пересматривается.
var bitOrOps = map[lexer.TokenType]string{lexer.BITOR: "|"}

func (p *Parser) parseBitOr() (ast.Expr, error) {
	return p.parseLeftAssoc(p.parseBitXor, bitOrOps)
}

var bitXorOps = map[lexer.TokenType]string{lexer.BITXOR: "^"}

func (p *Parser) parseBitXor() (ast.Expr, error) {
	return p.parseLeftAssoc(p.parseBitAnd, bitXorOps)
}

var bitAndOps = map[lexer.TokenType]string{lexer.BITAND: "&"}

func (p *Parser) parseBitAnd() (ast.Expr, error) {
	return p.parseLeftAssoc(p.parseShift, bitAndOps)
}

var shiftOps = map[lexer.TokenType]string{lexer.SHL: "<<", lexer.SHR: ">>"}

func (p *Parser) parseShift() (ast.Expr, error) {
	return p.parseLeftAssoc(p.parseAdditive, shiftOps)
}

var additiveOps = map[lexer.TokenType]string{lexer.PLUS: "+", lexer.MINUS: "-"}

func (p *Parser) parseAdditive() (ast.Expr, error) {
	return p.parseLeftAssoc(p.parseMultiplicative, additiveOps)
}

var multiplicativeOps = map[lexer.TokenType]string{lexer.STAR: "*", lexer.SLASH: "/", lexer.PERCENT: "%"}

func (p *Parser) parseMultiplicative() (ast.Expr, error) {
	return p.parseLeftAssoc(p.parseUnary, multiplicativeOps)
}

func (p *Parser) parseUnary() (ast.Expr, error) {
	if cast, ok, err := p.tryParseCast(); err != nil {
		return nil, err
	} else if ok {
		return cast, nil
	}
	switch p.cur().Type {
	case lexer.MINUS:
		op := p.advance()
		operand, err := p.parseUnary()
		if err != nil {
			return nil, err
		}
		return &ast.UnaryExpr{Op: "-", Operand: operand, Line: op.Line}, nil
	case lexer.BITNOT:
		op := p.advance()
		operand, err := p.parseUnary()
		if err != nil {
			return nil, err
		}
		return &ast.UnaryExpr{Op: "~", Operand: operand, Line: op.Line}, nil
	case lexer.INCR, lexer.DECR:
		return p.parsePrefixIncDec()
	case lexer.BITAND:
		return p.parseAddressOf()
	case lexer.STAR:
		op := p.advance()
		operand, err := p.parseUnary()
		if err != nil {
			return nil, fmt.Errorf("после \"*\" ожидалось выражение: %w", err)
		}
		return &ast.DerefExpr{Value: operand, Line: op.Line}, nil
	}
	return p.parsePostfix()
}

// parseAddressOf разбирает "&имя" (ссылка на функцию как значение ИЛИ
// адрес переменной/параметра - неоднозначность разрешается семантическим
// анализом, см. ast.FuncRefExpr), "&имя.поле" (адрес поля сущности) и
// "&имя[индекс]" (адрес элемента списка) - docs/SPEC.md, "Указатели".
// Ровно один уровень после имени (та же граница, что у левой части
// присваивания полю, docs/SPEC.md, раздел 6) - "&а.б.в"/"&а[i].поле" не
// поддержаны.
func (p *Parser) parseAddressOf() (ast.Expr, error) {
	op := p.advance() // "&"
	name, err := p.expect(lexer.IDENT)
	if err != nil {
		return nil, fmt.Errorf("после \"&\" ожидалось имя переменной или функции: %w", err)
	}
	switch p.cur().Type {
	case lexer.DOT:
		p.advance()
		field, err := p.expect(lexer.IDENT)
		if err != nil {
			return nil, fmt.Errorf("после \"&%s.\" ожидалось имя поля: %w", name.Value, err)
		}
		return &ast.AddressOfExpr{Receiver: name.Value, Field: field.Value, Line: op.Line}, nil
	case lexer.LBRACKET:
		p.advance()
		index, err := p.parseExpr()
		if err != nil {
			return nil, fmt.Errorf("в индексе \"&%s[...]\": %w", name.Value, err)
		}
		if _, err := p.expect(lexer.RBRACKET); err != nil {
			return nil, fmt.Errorf("после индекса \"&%s[...]\": %w", name.Value, err)
		}
		return &ast.AddressOfExpr{Receiver: name.Value, Index: index, Line: op.Line}, nil
	default:
		return &ast.FuncRefExpr{Name: name.Value, Line: op.Line}, nil
	}
}

// parsePrefixIncDec разбирает "++имя"/"--имя" - операнд обязан быть простым
// именем переменной (как у Assign), не произвольным унарным выражением -
// "++(а)" или "++-а" не имеют смысла (++ не применим к результату другого
// оператора), поэтому в отличие от "-"/"~" здесь нет рекурсии в parseUnary.
func (p *Parser) parsePrefixIncDec() (ast.Expr, error) {
	op := p.advance()
	name, err := p.expect(lexer.IDENT)
	if err != nil {
		return nil, fmt.Errorf("после %q ожидалось имя переменной: %w", op.Value, err)
	}
	return &ast.IncDecExpr{Name: name.Value, Op: op.Value, Prefix: true, Line: op.Line}, nil
}

// parsePostfix разбирает postfix := primary { ("." IDENT) | ("[" expr "]") }.
// Цикл поддерживает цепочки (`x.поле.поле`, `матрица[i]`, в будущем
// `x.поле[i]` и т. п.) уже сейчас, даже если конкретная конструкция сама их
// ещё не порождает - незачем переписывать этот уровень грамматики, когда
// они понадобятся. "[expr]" - только ЧТЕНИЕ (безопасная индексация,
// ast.IndexExpr, см. package doc там) - запись "receiver[index] = значение"
// разбирается отдельно, на уровне инструкции (parseIndexAssign), как сахар
// над "receiver.установить(...)", сюда не доходит вообще.
func (p *Parser) parsePostfix() (ast.Expr, error) {
	expr, err := p.parsePrimary()
	if err != nil {
		return nil, err
	}
	return p.parsePostfixTail(expr)
}

// parsePostfixTail - хвост postfix-цикла (см. package doc parsePostfix),
// начатый уже разобранным expr вместо parsePrimary() - переиспользуется
// там, где база цепочки не примарное выражение, а уже готовый вызов
// функции/естественный вызов/деепричастная форма (parseStatement,
// "создать_логгер().ошибка(...)" - результат вызова функции как
// самостоятельной инструкции ТОЖЕ может продолжаться цепочкой "."/"::",
// как и результат вызова метода).
func (p *Parser) parsePostfixTail(expr ast.Expr) (ast.Expr, error) {
	for {
		switch p.cur().Type {
		case lexer.DOT:
			dot := p.advance()
			field, err := p.expect(lexer.IDENT)
			if err != nil {
				return nil, fmt.Errorf("после \".\" ожидалось имя поля или метода: %w", err)
			}
			if p.cur().Type == lexer.LPAREN {
				args, err := p.parseArgList()
				if err != nil {
					return nil, fmt.Errorf("в аргументах вызова метода %q: %w", field.Value, err)
				}
				expr = &ast.MethodCallExpr{Receiver: expr, Method: field.Value, Args: args, Line: dot.Line}
				continue
			}
			expr = &ast.FieldAccess{Receiver: expr, Field: field.Value, Line: dot.Line}
		case lexer.DBLCOLON:
			dc := p.advance()
			field, err := p.expect(lexer.IDENT)
			if err != nil {
				return nil, fmt.Errorf("после \"::\" ожидалось имя статического поля или метода: %w", err)
			}
			if p.cur().Type == lexer.LPAREN {
				args, err := p.parseArgList()
				if err != nil {
					return nil, fmt.Errorf("в аргументах вызова статического метода %q: %w", field.Value, err)
				}
				expr = &ast.MethodCallExpr{Receiver: expr, Method: field.Value, Args: args, Static: true, Line: dc.Line}
				continue
			}
			expr = &ast.FieldAccess{Receiver: expr, Field: field.Value, Static: true, Line: dc.Line}
		case lexer.LBRACKET:
			lb := p.advance()
			index, err := p.parseExpr()
			if err != nil {
				return nil, fmt.Errorf("в индексе \"[...]\": %w", err)
			}
			if _, err := p.expect(lexer.RBRACKET); err != nil {
				return nil, fmt.Errorf("после индекса ожидалось \"]\": %w", err)
			}
			expr = &ast.IndexExpr{Receiver: expr, Index: index, Line: lb.Line}
		default:
			return expr, nil
		}
	}
}

// parseListLiteral разбирает "[элемент, ...]" (в т.ч. пустой "[]") -
// ast.ListLiteral. Позиция парсера на входе - на "[".
func (p *Parser) parseListLiteral() (ast.Expr, error) {
	lb := p.advance() // "["
	var elems []ast.Expr
	if p.cur().Type != lexer.RBRACKET {
		for {
			el, err := p.parseExpr()
			if err != nil {
				return nil, fmt.Errorf("в литерале списка: %w", err)
			}
			elems = append(elems, el)
			if p.cur().Type != lexer.COMMA {
				break
			}
			p.advance() // ","
		}
	}
	if _, err := p.expect(lexer.RBRACKET); err != nil {
		return nil, fmt.Errorf("после элементов литерала списка ожидалось \"]\": %w", err)
	}
	return &ast.ListLiteral{Elems: elems, Line: lb.Line}, nil
}

func (p *Parser) parsePrimary() (ast.Expr, error) {
	tok := p.cur()
	switch tok.Type {
	case lexer.STRING:
		p.advance()
		return &ast.StringLiteral{Value: tok.Value, Line: tok.Line}, nil
	case lexer.TAG:
		p.advance()
		return p.parseTemplateLiteral(tok)
	case lexer.CHAR:
		p.advance()
		r := []rune(tok.Value)[0]
		return &ast.CharLiteral{Value: r, Line: tok.Line}, nil
	case lexer.INT:
		p.advance()
		v, err := strconv.ParseInt(tok.Value, 10, 64)
		if err != nil {
			return nil, fmt.Errorf("строка %d, столбец %d: некорректный целочисленный литерал %q", tok.Line, tok.Col, tok.Value)
		}
		return &ast.IntLiteral{Value: v, Line: tok.Line}, nil
	case lexer.FLOAT:
		p.advance()
		v, err := strconv.ParseFloat(tok.Value, 64)
		if err != nil {
			return nil, fmt.Errorf("строка %d, столбец %d: некорректный числовой литерал %q", tok.Line, tok.Col, tok.Value)
		}
		return &ast.FloatLiteral{Value: v, Line: tok.Line}, nil
	case lexer.KwTrue:
		p.advance()
		return &ast.BoolLiteral{Value: true, Line: tok.Line}, nil
	case lexer.KwFalse:
		p.advance()
		return &ast.BoolLiteral{Value: false, Line: tok.Line}, nil
	case lexer.KwNull:
		p.advance()
		return &ast.NullLiteral{Line: tok.Line}, nil
	case lexer.IDENT:
		if p.peekAt(1).Type == lexer.LPAREN {
			return p.parseCallExpr()
		}
		if p.genericFuncs[tok.Value] && p.peekAt(1).Type == lexer.LT {
			return p.parseGenericCallExpr()
		}
		if p.peekAt(1).Type == lexer.INCR || p.peekAt(1).Type == lexer.DECR {
			p.advance()
			op := p.advance()
			return &ast.IncDecExpr{Name: tok.Value, Op: op.Value, Prefix: false, Line: tok.Line}, nil
		}
		if params, ok := p.funcParams[tok.Value]; ok && len(params) > 0 && startsNaturalCallArg(p.peekAt(1)) {
			// Естественный вызов как значение (docs/GRAMMAR.md, "Естественный
			// вызов как значение") - тот же parseNaturalCall, что и на месте
			// самостоятельной инструкции (stmt.go): число слов фразы известно
			// заранее (len(params), из предварительного прохода
			// collectFunctionSignatures), поэтому граница фразы не гадается по
			// следующему токену - ровно len(params) словоформ, и всё. После
			// них управление возвращается в обычный разбор выражения
			// (бинарные операторы/",'/")"/"]" и т. п.), как после любого
			// другого primary - естественный вызов свободно участвует в
			// более крупном выражении ("1 + позвать пользователя").
			//
			// startsNaturalCallArg - без неё голый идентификатор, совпавший
			// именем с какой-то функцией файла (например, параметр
			// конструктора, названный так же, как отдельная generic-функция),
			// ошибочно принимался бы за начало вызова, даже когда сразу за
			// ним идёт то, что явно завершает выражение (","/")"/оператор/
			// конец строки) - естественному вызову неоткуда взять аргументы,
			// и parseNaturalCall падал с "не хватает аргументов".
			return p.parseNaturalCall(params)
		}
		p.advance()
		return &ast.Identifier{Name: tok.Value, Line: tok.Line}, nil
	case lexer.KwNew:
		return p.parseNewExpr()
	case lexer.KwLaunch:
		return p.parseLaunchExpr()
	case lexer.KwAwait:
		return p.parseAwaitExpr()
	case lexer.KwThis:
		p.advance()
		if !p.inMethodBody {
			return nil, fmt.Errorf("строка %d, столбец %d: \"этот\" валиден только внутри тела метода", tok.Line, tok.Col)
		}
		return &ast.ThisExpr{Line: tok.Line}, nil
	case lexer.KwFunction:
		return p.parseLambdaExpr()
	case lexer.KwParse:
		return p.parseParseExpr()
	case lexer.KwSerialize:
		return p.parseSerializeExpr()
	case lexer.KwType:
		return p.parseTypeOfExpr()
	case lexer.LPAREN:
		p.advance()
		inner, err := p.parseExpr()
		if err != nil {
			return nil, err
		}
		if _, err := p.expect(lexer.RPAREN); err != nil {
			return nil, err
		}
		return inner, nil
	case lexer.LBRACKET:
		return p.parseListLiteral()
	default:
		return nil, fmt.Errorf("строка %d, столбец %d: ожидалось выражение, получено %q",
			tok.Line, tok.Col, tok.Value)
	}
}
