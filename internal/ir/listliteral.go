package ir

// ListLiteralExpr - "[элемент, ...]" (в т.ч. пустой) - создаёт Список<T> из
// заданных элементов в порядке записи. CollectionName - имя инстанциации
// ("Список<Целое>"), тем же принципом, что у ListIndexExpr - backend
// вызывает по нему сгенерированную функцию построения из литерала (см.
// package doc GenericInstantiation.NeedsListLiteral).
type ListLiteralExpr struct {
	Elems          []Expr
	ElemType       Type
	CollectionName string
	Type           Type // "Список<ElemType>"
	// Spread[i] - Elems[i] записан как "...список" и раскрывается на своём
	// месте всеми элементами (сам Elems[i] - типа Список<ElemType>). nil -
	// спредов в литерале нет.
	Spread []bool
	Line   int
}

func (*ListLiteralExpr) expr() {}
