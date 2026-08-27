package ir

// Expr - общий маркерный интерфейс выражений IR (как ast.Expr).
type Expr interface {
	expr()
}

// CallExpr - вызов функции по имени, аргументы уже разложены по позиции
// параметра (см. package doc Function.Params) - строгий и естественный
// синтаксис вызова уже неразличимы на этом уровне. Type/HasReturn - тип
// результата вызванной функции, скопированный из её Function при
// семантическом анализе (шаг 5.b), чтобы TypeOf не искал сигнатуру функции
// заново.
type CallExpr struct {
	Callee    string
	Args      []Expr
	Type      Type
	HasReturn bool
}

func (*CallExpr) expr() {}

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

// IncDecExpr - "имя++"/"имя--"/"++имя"/"--имя" (см. ast.IncDecExpr) - Name
// уже проверено семантическим анализом как объявленная переменная типа
// Целое/Число, Type - её тип.
type IncDecExpr struct {
	Name   string
	Op     string
	Prefix bool
	Type   Type
}

func (*IncDecExpr) expr() {}

// NewExpr - конструктор сущности ("новый Тип(...)" в любой из форм -
// позиционной или именованной, шаг 4.b) - Args уже разложены по позиции
// поля (см. package doc Function.Params - тот же принцип), независимо от
// того, какой формой они были написаны в исходнике.
type NewExpr struct {
	Entity string
	Args   []Expr
}

func (*NewExpr) expr() {}

// FieldAccess - чтение поля сущности ("receiver.поле", шаг 4.b). Type -
// тип поля, разрешённый семантическим анализом по сигнатуре сущности
// Receiver'а.
type FieldAccess struct {
	Receiver Expr
	Field    string
	Type     Type
}

func (*FieldAccess) expr() {}

// ThisExpr - "этот" внутри тела метода (шаг 4.d); Entity - имя сущности-
// владельца метода, где встречен "этот" - парсер уже гарантирует, что
// ThisExpr встречается только внутри метода, так что Entity всегда
// заполнен.
type ThisExpr struct {
	Entity string
}

func (*ThisExpr) expr() {}

// MethodCallExpr - вызов метода ("receiver.метод(...)", шаг 4.d) - Args
// уже разложены по позиции параметра, как и у CallExpr. Entity - имя
// сущности Receiver'а (нужно backend'у для мангла имени метода без
// повторного вывода типа).
type MethodCallExpr struct {
	Receiver  Expr
	Entity    string
	Method    string
	Args      []Expr
	Type      Type
	HasReturn bool
}

func (*MethodCallExpr) expr() {}

// TypeOf возвращает тип значения выражения e - у литералов известен по
// самому узлу, у остальных - уже разрешён семантическим анализом и
// хранится в поле Type (или выводится из Entity/Receiver, как у
// NewExpr/ThisExpr). Общий хелпер для всех backend'ов (cbackend, будущий
// interpreter) - не переопределять отдельно в каждом.
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
	case *CallExpr:
		return e.Type
	case *NewExpr:
		return EntityType(e.Entity)
	case *FieldAccess:
		return e.Type
	case *ThisExpr:
		return EntityType(e.Entity)
	case *MethodCallExpr:
		return e.Type
	case *IncDecExpr:
		return e.Type
	case *CharLiteral:
		return TypeChar
	case *CastExpr:
		return e.To
	default:
		return TypeUnknown
	}
}
