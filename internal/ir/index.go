package ir

// ListIndexExpr - "список[i]" (Список<T>) - безопасная индексация, в
// отличие от MethodCallExpr{Method: "получить"} (которая паникует при
// выходе за границу) всегда даёт "?T" - Ничто при выходе за границу.
// CollectionName - имя инстанциации ("Список<Целое>"), тем же принципом,
// что MethodCallExpr.Entity - backend вызывает по нему сгенерированную
// функцию безопасного доступа (см. package doc GenericInstantiation.
// NeedsSafeIndex).
type ListIndexExpr struct {
	Receiver       Expr
	Index          Expr
	CollectionName string
	Type           Type // "?T"
	Line           int
}

func (*ListIndexExpr) expr() {}

// DictIndexExpr - "словарь[ключ]" (Словарь<К, В>) - тот же принцип, что
// ListIndexExpr, но по ключу - "?В", Ничто при отсутствии ключа (не
// панике, в отличие от MethodCallExpr{Method: "получить"}).
type DictIndexExpr struct {
	Receiver       Expr
	Key            Expr
	CollectionName string
	Type           Type // "?В"
	Line           int
}

func (*DictIndexExpr) expr() {}

// ListElementRef - "список[i]" как АДРЕСУЕМАЯ ячейка хранения, не как
// значение - встречается только внутри AddressOfExpr ("&список[i]",
// internal/ir/pointer.go), паникует при выходе за границу, тем же
// принципом, что MethodCallExpr{Method: "получить"} (не безопасный
// ListIndexExpr выше, дающий "?T"). Type - T адресуемого элемента, БЕЗ
// "Указатель<...>" - обёртку добавляет AddressOfExpr сверху.
type ListElementRef struct {
	Receiver       Expr
	Index          Expr
	CollectionName string
	Type           Type
	Line           int
}

func (*ListElementRef) expr() {}
