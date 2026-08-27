package parser

import (
	"fmt"
	"strconv"

	"github.com/IlyaFeoktistov/Rech/internal/ast"
	"github.com/IlyaFeoktistov/Rech/internal/lexer"
)

func (p *Parser) parseExpr() (ast.Expr, error) {
	return p.parseOr()
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
	return left, nil
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

var multiplicativeOps = map[lexer.TokenType]string{lexer.STAR: "*", lexer.SLASH: "/"}

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
	}
	return p.parsePostfix()
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

// parsePostfix разбирает postfix := primary { "." IDENT }. Цикл (а не
// одиночная проверка) уже сейчас поддерживает цепочки (`x.поле.поле`),
// даже если шаг 4 сам их ещё не порождает - незачем переписывать этот
// уровень грамматики, когда они понадобятся.
func (p *Parser) parsePostfix() (ast.Expr, error) {
	expr, err := p.parsePrimary()
	if err != nil {
		return nil, err
	}
	for p.cur().Type == lexer.DOT {
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
	}
	return expr, nil
}

func (p *Parser) parsePrimary() (ast.Expr, error) {
	tok := p.cur()
	switch tok.Type {
	case lexer.STRING:
		p.advance()
		return &ast.StringLiteral{Value: tok.Value, Line: tok.Line}, nil
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
	case lexer.IDENT:
		if p.peekAt(1).Type == lexer.LPAREN {
			return p.parseCallExpr()
		}
		if p.peekAt(1).Type == lexer.INCR || p.peekAt(1).Type == lexer.DECR {
			p.advance()
			op := p.advance()
			return &ast.IncDecExpr{Name: tok.Value, Op: op.Value, Prefix: false, Line: tok.Line}, nil
		}
		p.advance()
		return &ast.Identifier{Name: tok.Value, Line: tok.Line}, nil
	case lexer.KwNew:
		return p.parseNewExpr()
	case lexer.KwThis:
		p.advance()
		if !p.inMethodBody {
			return nil, fmt.Errorf("строка %d, столбец %d: \"этот\" валиден только внутри тела метода", tok.Line, tok.Col)
		}
		return &ast.ThisExpr{Line: tok.Line}, nil
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
	default:
		return nil, fmt.Errorf("строка %d, столбец %d: ожидалось выражение, получено %q",
			tok.Line, tok.Col, tok.Value)
	}
}
