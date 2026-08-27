package ir

type Stmt interface {
	stmt()
}

// PrintStmt соответствует "вывести <выражение>".
type PrintStmt struct {
	Value Expr
}

func (*PrintStmt) stmt() {}

// ShowStmt соответствует "показать <выражение>" - отладочный вывод значения
// вместе с именем его типа, рекурсивно для сущностей (поля) и встроенных
// коллекций (элементы). Тип значения - TypeOf(Value), как и у PrintStmt;
// имена полей сущности codegen берёт из Program.Entities по имени типа.
type ShowStmt struct {
	Value Expr
}

func (*ShowStmt) stmt() {}

// VarDecl соответствует "переменная <имя> = <выражение>". Тип переменной -
// тип Value (literal-based inference, шаг 1) - разрешён и провалидирован
// семантическим анализом (internal/semantic), в самом узле не хранится
// отдельно - Value.Type()/TypeOf(Value) и есть тип переменной.
type VarDecl struct {
	Name  string
	Value Expr
}

func (*VarDecl) stmt() {}

// Assign соответствует "<имя> = <выражение>" для уже объявленной
// переменной - совпадение типа с объявленным при первом "переменная" уже
// проверено семантическим анализом.
type Assign struct {
	Name  string
	Value Expr
}

func (*Assign) stmt() {}

// FieldAssign соответствует "receiver.поле = выражение" (шаг 4.c) -
// Receiver уже проверен на адресуемость (переменная/"этот"/доступ к полю,
// как и получатель вызова метода) семантическим анализом.
type FieldAssign struct {
	Receiver Expr
	Field    string
	Value    Expr
}

func (*FieldAssign) stmt() {}

// BecomeStmt соответствует "receiver становится <состояние>" (шаг 4.1,
// упрощённая версия - состояние без падежного согласования, ровно как в
// объявлении "состояние:"). Не несёт типа - "становится" не эмитит C-код
// (typestate чисто компилятивный на этом шаге), состояние уже проверено
// семантическим анализом как объявленное у сущности Receiver'а.
type BecomeStmt struct {
	Receiver Expr
	State    string
}

func (*BecomeStmt) stmt() {}

// ParentCallStmt соответствует "родитель(<арг>, ...)" внутри тела
// конструктора - вызывает Entity.Ctor базовой сущности целиком на том же
// объекте ("этот" вызывающего конструктора). BaseEntity - имя базовой
// сущности (семантический анализ уже проверил, что она объявлена, что у
// неё есть собственный Ctor, и что это первая инструкция тела - см. package
// doc internal/semantic/inheritance.go) - backend просто вызывает её Ctor
// напрямую по имени, без повторного поиска по цепочке BaseName.
type ParentCallStmt struct {
	BaseEntity string
	Args       []Expr
}

func (*ParentCallStmt) stmt() {}

// AssertStmt соответствует "утверждать <условие>" - Cond уже провалидирован
// (тип Логическое) семантическим анализом. Line - единственное поле Line
// среди узлов IR (остальные его не несут, см. package doc) - здесь оно
// нужно backend'у/интерпретатору для сообщения об ошибке во время
// исполнения ("утверждение не выполнено (строка N)"), которое иначе
// указывало бы только "где-то в программе".
type AssertStmt struct {
	Cond Expr
	Line int
}

func (*AssertStmt) stmt() {}

// IfStmt соответствует "если <условие> { <then> } [иначе { <else> }]" -
// Cond уже провалидирован (тип Логическое) семантическим анализом.
type IfStmt struct {
	Cond Expr
	Then []Stmt
	Else []Stmt
}

func (*IfStmt) stmt() {}

// WhileStmt соответствует "пока <условие> { <тело> }".
type WhileStmt struct {
	Cond Expr
	Body []Stmt
}

func (*WhileStmt) stmt() {}

// ForStmt соответствует "для <имя> от <from> до <to> { <тело> }" - цикл по
// целочисленному диапазону (включая обе границы, шаг 2). Var - переменная
// цикла, всегда типа Целое.
type ForStmt struct {
	Var  string
	From Expr
	To   Expr
	Body []Stmt
}

func (*ForStmt) stmt() {}

// ForEachStmt соответствует "для <имя> в <коллекция> { <тело> }" - цикл по
// Список<T> (шаг 4.2.b) - Collection уже проверен как переменная-Список<T>
// семантическим анализом, ElemType - тип её элемента (нужен backend'у для
// C-типа переменной цикла, чтобы не искать сигнатуру коллекции заново).
type ForEachStmt struct {
	Var        string
	Collection Expr
	ElemType   Type
	Body       []Stmt
}

func (*ForEachStmt) stmt() {}

// ReturnStmt соответствует "вернуть [<выражение>]" - Value == nil у функции
// без объявленного типа результата (только этот вид - шаг 2); с шага 3 у
// функции с объявленным типом результата Value обязателен и его тип уже
// проверен internal/semantic на совпадение с ReturnType функции/метода.
type ReturnStmt struct {
	Value Expr
}

func (*ReturnStmt) stmt() {}

type ExprStmt struct {
	Call       *CallExpr
	CallValue  *CallValueExpr
	MethodCall *MethodCallExpr
	IncDec     *IncDecExpr
	Await      *AwaitExpr
}

func (*ExprStmt) stmt() {}

// TypeKind - "форма" типа значения: один из встроенных примитивов или
// KindEntity (сущность, чьё имя несёт сама Type.Entity - без этого
// сравнение двух Type/вывод C-типа нуждались бы во внешнем реестре, тот же
// принцип, что и у valueType в cbackend).
