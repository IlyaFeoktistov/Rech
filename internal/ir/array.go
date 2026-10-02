package ir

// ArrayInstantiation - конкретный "T[N]", встретившийся в программе.
type ArrayInstantiation struct {
	Name string // "Целое[3]" - он же Type.Entity
	Elem Type
	Len  int64
}

// ArrayIndexExpr - "массив[индекс]": элемент массива фиксированной длины
// (тип T, не "?T"), выход за границы - паника. Lvalue, если Array -
// lvalue: запись - ArrayAssignStmt, адрес - AddressOfExpr над ним.
type ArrayIndexExpr struct {
	Array Expr
	Index Expr
	Type  Type // T
	Line  int
}

func (*ArrayIndexExpr) expr() {}

// ArrayLiteralExpr - "[a, b, c]" там, где ожидается массив "T[N]"
// (ровно N элементов). Elems == nil - массив из нулевых значений (поле-
// массив без значения по умолчанию).
type ArrayLiteralExpr struct {
	Elems []Expr
	Type  Type // "T[N]"
	Line  int
}

func (*ArrayLiteralExpr) expr() {}

// ArrayAssignStmt - "массив[индекс] = значение" - Array обязан быть
// lvalue (переменная, поле, элемент массива-lvalue, разыменование).
type ArrayAssignStmt struct {
	Array Expr
	Index Expr
	Value Expr
	Line  int
}

func (*ArrayAssignStmt) stmt() {}
