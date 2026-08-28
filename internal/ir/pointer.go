package ir

// PointerInstantiation - конкретный "Указатель<T>", встретившийся в
// программе - тем же принципом, что NullableInstantiation для "?Тип":
// backend сам знает, как представить адрес значения T (в cbackend - "T *"
// буквально; в interp - Go-указатель *Value), IR только называет, какая
// инстанциация встретилась.
type PointerInstantiation struct {
	Name  string // "Указатель<Целое>" - он же Type.Entity
	Inner Type
}

// AddressOfExpr - "&получатель" ("Указатель<T>", адрес значения T) -
// Receiver - выражение, из которого берётся адрес, ограничено на этапе
// семантического анализа (см. internal/semantic/pointer.go) тремя формами:
// простая переменная/параметр (Identifier), поле сущности (FieldAccess,
// ровно один уровень - то же ограничение, что у левой части присваивания
// полю) или элемент списка (MethodCallExpr{Method: "получить"} - берёт
// адрес РЕАЛЬНОЙ ячейки хранения, паникует при выходе за границу, тем же
// принципом, что и обычное "получить"). Адрес, взятый у элемента списка,
// теряет силу при следующем "добавить" в тот же список, реаллоцирующем
// буфер - ответственность программиста, тем же принципом, что и
// отсутствие проверки гонок у потоков (Шаг 15) - явная, осознанная
// небезопасность, не скрытая от программиста.
type AddressOfExpr struct {
	Receiver Expr
	Type     Type // "Указатель<T>"
	Line     int
}

func (*AddressOfExpr) expr() {}

// DerefExpr - "*указатель" (чтение T через "Указатель<T>") - Type уже без
// "Указатель<...>". Многоуровневые указатели ("**x") - обычная
// рекурсивная вложенность DerefExpr{Value: DerefExpr{...}}, без отдельного
// узла на каждый уровень.
type DerefExpr struct {
	Value Expr
	Type  Type // T
	Line  int
}

func (*DerefExpr) expr() {}

// DerefAssignStmt - "*указатель = значение" (запись T через "Указатель<T>").
type DerefAssignStmt struct {
	Pointer Expr
	Value   Expr
	Line    int
}

func (*DerefAssignStmt) stmt() {}
