package ir

// CharLiteral - символьный литерал ('a', 'я' - один кодовый пункт Unicode,
// не байт).
type CharLiteral struct {
	Value rune
}

func (*CharLiteral) expr() {}

// CastExpr - явное приведение типа ("(Тип) значение") - To - целевой тип,
// тип исходного значения - TypeOf(Value). Допустимость конкретной пары
// (From, To) уже проверена семантическим анализом (internal/semantic) -
// сам узел IR несёт только то, что нужно кодогенерации.
type CastExpr struct {
	Value Expr
	To    Type
}

func (*CastExpr) expr() {}
