package ast

// CharLiteral - символьный литерал в одинарных кавычках ('a', 'я' - один
// кодовый пункт Unicode, не байт).
type CharLiteral struct {
	Value rune
	Line  int
}

func (*CharLiteral) node() {}
func (*CharLiteral) expr() {}

// CastExpr - явное приведение типа:
//
//	(Тип) <выражение>
//
// Только между примитивами (Type - одно из фиксированных имён встроенных
// типов, проверяется парсером при разборе, не как имя сущности) -
// допустимость конкретной пары исходный/целевой тип проверяется
// семантическим анализом.
type CastExpr struct {
	Type  string
	Value Expr
	Line  int
}

func (*CastExpr) node() {}
func (*CastExpr) expr() {}
