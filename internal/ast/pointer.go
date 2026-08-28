package ast

// AddressOfExpr - "&получатель.поле" / "&получатель[индекс]" - адрес поля
// сущности/элемента списка (docs/SPEC.md, "Указатели"). Ровно одно из
// Field/Index занято. Простое "&имя" (адрес переменной/параметра ИЛИ ссылка
// на функцию как значение - неоднозначность разрешается семантическим
// анализом по тому, чем на самом деле оказывается имя) - уже существующий
// узел FuncRefExpr (callable.go), не этот - парсер строит AddressOfExpr,
// только когда сразу после идентификатора действительно следует "."/"[".
type AddressOfExpr struct {
	Receiver string
	Field    string
	Index    Expr
	Line     int
}

func (*AddressOfExpr) node() {}
func (*AddressOfExpr) expr() {}

// DerefExpr - "*значение" - чтение через указатель ("Указатель<T>" -> T).
// Многоуровневые указатели ("**x") - обычная рекурсивная вложенность
// (DerefExpr{Value: DerefExpr{...}}), без отдельного узла на каждый
// уровень.
type DerefExpr struct {
	Value Expr
	Line  int
}

func (*DerefExpr) node() {}
func (*DerefExpr) expr() {}

// DerefAssignStmt - "*указатель = значение" - запись через указатель.
// Pointer - выражение, дающее сам указатель (не то, на что он указывает) -
// для "**p = значение" это "*p" (DerefExpr), не голое "p".
type DerefAssignStmt struct {
	Pointer Expr
	Value   Expr
	Line    int
}

func (*DerefAssignStmt) node() {}
func (*DerefAssignStmt) stmt() {}
