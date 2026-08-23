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
// а не всей IR сразу для всех конструкций. Покрытие на данный момент - шаги
// 0-4 (`вывести <строка>`, переменные/арифметика, если/пока/для, функции с
// параметрами/типом результата и их вызов, сущности/поля/конструктор/
// методы/"этот"), шаг 4.1 (состояния, упрощённая версия) и шаг 4.2
// (встроенные generic-коллекции - Список/Стопка/Очередь, цикл "для x в
// списке"). Пока покрытие не станет полным, IR НЕ встроен в реальный путь
// компиляции
// (internal/compiler по-прежнему вызывает cbackend.
// Generate(*ast.Program) напрямую) - internal/ir и internal/semantic
// существуют параллельно, проверяются собственными тестами, и заменяют
// старый путь только когда покрытие станет исчерпывающим.
package ir

// Program - корень IR-дерева программы.
type Program struct {
	Functions   []*Function
	Entities    []*Entity
	Generics    []GenericInstantiation
	ExternFuncs []*ExternFuncDecl
}

// ExternFuncDecl соответствует "внешняя функция <CName> [как <Alias>](...)"
// (шаг 7, FFI) - объявление сигнатуры уже существующей C-функции без тела,
// см. ast.ExternFuncDecl. Params - только типы (без имён - C-прототип не
// нуждается в именах параметров, а внутри Rech-программы к ним обращаются
// только по имени параметра из AST, которое уже не нужно после того, как
// семантический анализ проверил число/типы аргументов вызова).
// internal/backend/cbackend.GenerateFromIR - единственный потребитель, у
// которого это реально работает: internal/interp не умеет вызывать
// произвольный C-символ без настоящей динамической линковки (dlopen/dlsym),
// поэтому Run явно отказывается сам, увидев непустой ExternFuncs -
// semantic.Analyze производит IR с ним как обычно (это ограничение только
// interpreter-пути, не самого IR).
type ExternFuncDecl struct {
	CName      string
	Alias      string
	Params     []Type
	ReturnType Type
	HasReturn  bool
}

// GenericInstantiation - конкретная инстанциация встроенной generic-
// коллекции (шаг 4.2, "Список"/"Стопка"/"Очередь"), использованная в
// программе - "Список<Целое>" и т. п. Backend синтезирует по ней
// C-структуру и функции сам (growable array, та же реализация, что и в
// AST-пути, cbackend.listCollectionCCode/stackCollectionCCode/
// queueCollectionCCode) - IR только называет, какой шаблон и с каким
// элементом встретился, реализация не видна в Rech (нет доступных полей),
// поэтому IR-узла Entity для инстанциации не заводится - в отличие от
// пользовательских сущностей, тут нечего показывать через Fields.
type GenericInstantiation struct {
	Name     string // "Список<Целое>"
	Template string // "Список" | "Стопка" | "Очередь"
	ElemType Type
}

// Function - функция программы после семантического анализа. Name - имя
// ровно как в Rech-исходнике (например, "главная") - маппинг на
// C-специфичное имя (например, main) - решение backend'а, не IR.
//
// Строгий и естественный синтаксис вызова (docs/SPEC.md) уже неразличимы
// на уровне AST (парсер раскладывает падежные аргументы естественного
// вызова по позиции параметра, как и строгий) - Params здесь просто по
// позиции, падежных ролей (ast.Param.CaseWord) IR не хранит вообще, они
// нужны были только парсеру для сопоставления, не после него.
type Function struct {
	Name       string
	Params     []Param
	ReturnType Type
	HasReturn  bool // false - нет объявленного типа результата
	Body       []Stmt
}

// Param - один параметр функции/метода по позиции.
type Param struct {
	Name string
	Type Type
}

// Entity - сущность программы после семантического анализа (шаг 4).
// Состояния (шаг 4.1) и generic-коллекции (шаг 4.2) сюда пока не
// перенесены - internal/semantic явно отказывает на них, а не тихо
// пропускает.
type Entity struct {
	Name    string
	Fields  []Field
	Methods []*Method
}

// Field - одно поле сущности, в порядке объявления (порядок значим -
// позиционный конструктор и порядок полей в C-структуре зависят от него).
type Field struct {
	Name string
	Type Type
}

// Method - метод сущности: как Function, но получатель ("этот") неявный -
// его тип есть Type{Kind: KindEntity, Entity: <имя сущности-владельца>},
// backend передаёт его отдельным, скрытым от Rech параметром (в cbackend -
// указатель, чтобы мутация поля через "этот.поле = ..." была видна после
// возврата).
type Method struct {
	Name       string
	Params     []Param
	ReturnType Type
	HasReturn  bool
	Body       []Stmt
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

// ExprStmt - вызов функции или метода как самостоятельная инструкция,
// результат отбрасывается (в отличие от использования как значения -
// PrintStmt.Value, VarDecl.Value и т. п.) - ровно один из Call/MethodCall
// заполнен, как и в ast.ExprStmt.
type ExprStmt struct {
	Call       *CallExpr
	MethodCall *MethodCallExpr
	IncDec     *IncDecExpr
}

func (*ExprStmt) stmt() {}

// TypeKind - "форма" типа значения: один из встроенных примитивов или
// KindEntity (сущность, чьё имя несёт сама Type.Entity - без этого
// сравнение двух Type/вывод C-типа нуждались бы во внешнем реестре, тот же
// принцип, что и у valueType в cbackend).
type TypeKind int

const (
	KindUnknown TypeKind = iota
	KindString
	KindInt
	KindFloat
	KindBool
	KindEntity
	// KindFile - непрозрачный дескриптор открытого файла (шаг 8, файловый
	// ввод-вывод) - без операторов, как и KindString.
	KindFile
	// KindSocket - непрозрачный дескриптор сетевого сокета (шаг 8, сетевые
	// примитивы) - как и KindFile, без операторов, платформенно-разное
	// представление (POSIX fd/Windows SOCKET) решает каждый потребитель сам.
	KindSocket
)

// Type - тип значения выражения IR, уже разрешённый семантическим анализом
// (шаг 5.b) - в отличие от AST, где тип выводится заново на каждом
// обращении (cbackend.genExpr), IR хранит готовый результат прямо в узле
// (Identifier.Type, BinaryExpr.Type, UnaryExpr.Type и т. п.) - у литералов
// тип известен по самому Go-типу узла, см. TypeOf.
type Type struct {
	Kind   TypeKind
	Entity string // валиден только при Kind == KindEntity
}

var (
	TypeUnknown = Type{Kind: KindUnknown}
	TypeString  = Type{Kind: KindString}
	TypeInt     = Type{Kind: KindInt}
	TypeFloat   = Type{Kind: KindFloat}
	TypeBool    = Type{Kind: KindBool}
	TypeFile    = Type{Kind: KindFile}
	TypeSocket  = Type{Kind: KindSocket}
)

// EntityType возвращает Type, соответствующий значению сущности name.
func EntityType(name string) Type {
	return Type{Kind: KindEntity, Entity: name}
}

func (t Type) String() string {
	if t.Kind == KindEntity {
		return t.Entity
	}
	switch t.Kind {
	case KindString:
		return "Строка"
	case KindInt:
		return "Целое"
	case KindFloat:
		return "Число"
	case KindBool:
		return "Логическое"
	case KindFile:
		return "Файл"
	case KindSocket:
		return "Сокет"
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
	default:
		return TypeUnknown
	}
}
