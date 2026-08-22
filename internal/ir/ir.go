// Package ir описывает промежуточное представление программы (шаг 5,
// docs/ROADMAP.md) - слой между ast.Program и backend'ами (cbackend,
// будущий interpreter), не зависящий ни от русской грамматики (имена -
// уже проверенные идентификаторы, не произвольные словоформы падежного
// сопоставления), ни от конкретного backend'а (нет понятия mangleName/
// mangleType/C-типов - это решает cbackend или interpreter по-своему).
//
// internal/semantic производит *Program из *ast.Program (семантический
// анализ - проверка типов, разрешение имён, шаг 5.b) - см. пакет semantic
// для деталей.
//
// Реализуется вертикальными срезами по конструкциям языка (стратегия,
// согласованная с пользователем для шага 5, см. docs/ROADMAP.md, Шаг 5) -
// а не всей IR сразу для всех конструкций. Покрытие на данный момент - шаг
// 0 (`вывести <строка>`), шаг 1 (переменные, арифметика, сравнения,
// логические операторы) и шаг 2 (если/пока/для, блочная область
// видимости). Пока покрытие не станет полным, IR НЕ встроен в реальный
// путь компиляции
// (internal/compiler по-прежнему вызывает cbackend.Generate(*ast.Program)
// напрямую) - internal/ir и internal/semantic существуют параллельно,
// проверяются собственными тестами, и заменяют старый путь только когда
// покрытие станет исчерпывающим.
package ir

// Program - корень IR-дерева программы.
type Program struct {
	Functions []*Function
}

// Function - функция программы после семантического анализа. Name - имя
// ровно как в Rech-исходнике (например, "главная") - маппинг на
// C-специфичное имя (например, main) - решение backend'а, не IR.
type Function struct {
	Name string
	Body []Stmt
}

// Stmt - общий маркерный интерфейс инструкций IR (как ast.Stmt).
type Stmt interface {
	stmt()
}

// PrintStmt соответствует "вывести <выражение>".
type PrintStmt struct {
	Value Expr
}

func (*PrintStmt) stmt() {}

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

// ReturnStmt соответствует "вернуть" без значения (шаг 2 - досрочный выход
// из функции; возврат значения появился только в шаге 3, у функции без
// объявленного типа результата возможен только этот, безаргументный вид -
// internal/semantic это проверяет).
type ReturnStmt struct{}

func (*ReturnStmt) stmt() {}

// Type - тип значения выражения IR, уже разрешённый семантическим анализом
// (шаг 5.b) - в отличие от AST, где тип выводится заново на каждом
// обращении (cbackend.genExpr), IR хранит готовый результат прямо в узле
// (Identifier.Type, BinaryExpr.Type, UnaryExpr.Type) - у литералов тип
// известен по самому Go-типу узла, см. TypeOf.
type Type int

const (
	TypeUnknown Type = iota
	TypeString
	TypeInt
	TypeFloat
	TypeBool
)

func (t Type) String() string {
	switch t {
	case TypeString:
		return "Строка"
	case TypeInt:
		return "Целое"
	case TypeFloat:
		return "Число"
	case TypeBool:
		return "Логическое"
	default:
		return "неизвестный тип"
	}
}

// Expr - общий маркерный интерфейс выражений IR (как ast.Expr).
type Expr interface {
	expr()
}

// StringLiteral - строковый литерал.
type StringLiteral struct {
	Value string
}

func (*StringLiteral) expr() {}

// IntLiteral - целочисленный литерал.
type IntLiteral struct {
	Value int64
}

func (*IntLiteral) expr() {}

// FloatLiteral - литерал с плавающей точкой.
type FloatLiteral struct {
	Value float64
}

func (*FloatLiteral) expr() {}

// BoolLiteral - логический литерал ("истина"/"ложь").
type BoolLiteral struct {
	Value bool
}

func (*BoolLiteral) expr() {}

// Identifier - ссылка на переменную; Type - тип, разрешённый семантическим
// анализом по месту объявления (internal/semantic хранит для этого
// собственную символьную таблицу, отдельную от backend'ов - в этом и
// состоит смысл шага 5, см. package doc).
type Identifier struct {
	Name string
	Type Type
}

func (*Identifier) expr() {}

// BinaryExpr - бинарная операция; Op - один из "+ - * / == != < > <= >=
// и или". Type - тип результата, уже разрешённый (и провалидированный по
// типам операндов) семантическим анализом.
type BinaryExpr struct {
	Op    string
	Left  Expr
	Right Expr
	Type  Type
}

func (*BinaryExpr) expr() {}

// UnaryExpr - унарная операция; Op - один из "- не".
type UnaryExpr struct {
	Op      string
	Operand Expr
	Type    Type
}

func (*UnaryExpr) expr() {}

// TypeOf возвращает тип значения выражения e - у литералов известен по
// самому узлу, у Identifier/BinaryExpr/UnaryExpr - уже разрешён
// семантическим анализом и хранится в поле Type. Общий хелпер для всех
// backend'ов (cbackend, будущий interpreter) - не переопределять отдельно
// в каждом.
func TypeOf(e Expr) Type {
	switch e := e.(type) {
	case *StringLiteral:
		return TypeString
	case *IntLiteral:
		return TypeInt
	case *FloatLiteral:
		return TypeFloat
	case *BoolLiteral:
		return TypeBool
	case *Identifier:
		return e.Type
	case *BinaryExpr:
		return e.Type
	case *UnaryExpr:
		return e.Type
	default:
		return TypeUnknown
	}
}
