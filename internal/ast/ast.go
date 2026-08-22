// Package ast описывает синтаксическое дерево Rech.
//
// Дерево не знает ничего о C, LLVM или интерпретаторе - оно описывает
// только структуру программы на русском языке. Backend'ы (cbackend и
// будущие interpreter/VM) читают это дерево и решают, во что его превратить.
// См. docs/ROADMAP.md, этап "IR": позже между AST и backend'ами появится
// отдельный слой IR, но интерфейс Stmt/Expr рассчитан так, чтобы это не
// потребовало переписывать парсер.
package ast

// Node - общий маркерный интерфейс всех узлов дерева.
type Node interface {
	node()
}

// Program - корень дерева: список объявленных функций и сущностей.
type Program struct {
	Functions []*FunctionDecl
	Entities  []*EntityDecl
}

func (*Program) node() {}

// EntityDecl соответствует конструкции:
//
//	сущность Имя
//	{
//	    имеет:
//	        поле: Тип
//	        ...
//	}
//
// ("класс" - синоним "сущность", docs/GRAMMAR.md). Methods добавятся в
// оставшейся части шага 4 - на первой итерации сущность несёт только поля.
type EntityDecl struct {
	Name   string
	Fields []Field
	Line   int
}

func (*EntityDecl) node() {}

// Field - одно поле сущности, объявленное под "имеет:". Type - имя типа,
// как и у Param (проверка допустимости - в backend'е).
type Field struct {
	Name string
	Type string
	Line int
}

// FunctionDecl соответствует конструкции:
//
//	функция <имя>(<парам>: <Тип>, ...): <ТипВозврата>
//	{
//	    <тело>
//	}
//
// Параметры и тип возврата необязательны (Params == nil, ReturnType == "") -
// это форма из шагов 0-2, всё ещё допустимая. Params и ReturnType появились
// в шаге 3, см. docs/ROADMAP.md.
type FunctionDecl struct {
	Name       string
	Params     []Param
	ReturnType string // "" - тип результата не объявлен
	Body       []Stmt
	Line       int
}

func (*FunctionDecl) node() {}

// Param - один параметр функции. Name - одно из падежных вопросительных
// слов (кто, кого, кому, кем, где, что, см. internal/morph.CaseOfParamName) -
// это одновременно имя параметра и его падежная роль при естественном
// вызове (шаг 3). Type - имя типа (в шаге 3 - один из Строка/Целое/Число/
// Логическое; проверка допустимости имени - в backend'е, как и для типа
// переменных, а не здесь, см. заголовочный комментарий cbackend/generator.go).
type Param struct {
	Name string
	Type string
	Line int
}

// Stmt - инструкция внутри тела функции.
type Stmt interface {
	Node
	stmt()
}

// PrintStmt соответствует инструкции:
//
//	вывести <выражение>
type PrintStmt struct {
	Value Expr
	Line  int
}

func (*PrintStmt) node() {}
func (*PrintStmt) stmt() {}

// VarDecl соответствует инструкции объявления переменной с инициализацией:
//
//	переменная <имя> = <выражение>
//
// Тип переменной не аннотируется явно - выводится из выражения-инициализатора
// (простой literal-based inference, см. docs/ROADMAP.md, шаг 1).
type VarDecl struct {
	Name  string
	Value Expr
	Line  int
}

func (*VarDecl) node() {}
func (*VarDecl) stmt() {}

// Assign соответствует инструкции присваивания уже объявленной переменной:
//
//	<имя> = <выражение>
type Assign struct {
	Name  string
	Value Expr
	Line  int
}

func (*Assign) node() {}
func (*Assign) stmt() {}

// IfStmt соответствует конструкции:
//
//	если <условие> { <then> } [ иначе { <else> } ]
//
// Else == nil, если ветки "иначе" не было. Цепочки "иначе если" в v0.1 нет -
// вложенность через отдельный if внутри блока Else.
type IfStmt struct {
	Cond Expr
	Then []Stmt
	Else []Stmt
	Line int
}

func (*IfStmt) node() {}
func (*IfStmt) stmt() {}

// WhileStmt соответствует конструкции:
//
//	пока <условие> { <тело> }
type WhileStmt struct {
	Cond Expr
	Body []Stmt
	Line int
}

func (*WhileStmt) node() {}
func (*WhileStmt) stmt() {}

// ForStmt соответствует конструкции цикла по целочисленному диапазону
// (включая обе границы):
//
//	для <имя> от <from> до <to> { <тело> }
type ForStmt struct {
	Var  string
	From Expr
	To   Expr
	Body []Stmt
	Line int
}

func (*ForStmt) node() {}
func (*ForStmt) stmt() {}

// ReturnStmt соответствует инструкции:
//
//	вернуть
//	вернуть <выражение>
//
// Value == nil - "вернуть" без значения (допустимо только у функции без
// объявленного типа результата); Value != nil - "вернуть <выражение>"
// (допустимо только у функции с объявленным типом результата, шаг 3).
type ReturnStmt struct {
	Value Expr
	Line  int
}

func (*ReturnStmt) node() {}
func (*ReturnStmt) stmt() {}

// ExprStmt - вызов функции как самостоятельная инструкция (ради побочного
// эффекта, результат отбрасывается):
//
//	<имя>(<арг>, ...)
//
// Единственное выражение, которое в шаге 3 разрешено как самостоятельная
// инструкция, - вызов функции, отсюда явное поле Call вместо общего Expr:
// строгий и естественный вызов (docs/GRAMMAR.md) производят один и тот же
// CallExpr, а ExprStmt - только тонкая обёртка вокруг него для позиции
// инструкции.
type ExprStmt struct {
	Call *CallExpr
	Line int
}

func (*ExprStmt) node() {}
func (*ExprStmt) stmt() {}

// Expr - выражение, вычисляемое в значение.
type Expr interface {
	Node
	expr()
}

// StringLiteral - строковый литерал в двойных кавычках.
type StringLiteral struct {
	Value string
	Line  int
}

func (*StringLiteral) node() {}
func (*StringLiteral) expr() {}

// IntLiteral - целочисленный литерал.
type IntLiteral struct {
	Value int64
	Line  int
}

func (*IntLiteral) node() {}
func (*IntLiteral) expr() {}

// FloatLiteral - литерал с плавающей точкой.
type FloatLiteral struct {
	Value float64
	Line  int
}

func (*FloatLiteral) node() {}
func (*FloatLiteral) expr() {}

// BoolLiteral - логический литерал ("истина" / "ложь").
type BoolLiteral struct {
	Value bool
	Line  int
}

func (*BoolLiteral) node() {}
func (*BoolLiteral) expr() {}

// Identifier - ссылка на переменную в выражении.
type Identifier struct {
	Name string
	Line int
}

func (*Identifier) node() {}
func (*Identifier) expr() {}

// CallExpr - вызов функции, дающий значение:
//
//	<имя>(<арг>, ...)
//
// Строгий и естественный синтаксис вызова (docs/GRAMMAR.md, "Два
// представления одного AST") - это два способа получить один и тот же
// узел: Args всегда упорядочен по позиции параметра в объявлении функции
// (FunctionDecl.Params), а не по порядку слов в исходнике - естественный
// вызов раскладывает падежные аргументы по этому порядку при разборе.
type CallExpr struct {
	Callee string
	Args   []Expr
	Line   int
}

func (*CallExpr) node() {}
func (*CallExpr) expr() {}

// BinaryExpr - бинарная операция. Op - один из:
// "+" "-" "*" "/" "==" "!=" "<" ">" "<=" ">=" "и" "или".
type BinaryExpr struct {
	Op    string
	Left  Expr
	Right Expr
	Line  int
}

func (*BinaryExpr) node() {}
func (*BinaryExpr) expr() {}

// UnaryExpr - унарная операция. Op - один из: "-" "не".
type UnaryExpr struct {
	Op      string
	Operand Expr
	Line    int
}

func (*UnaryExpr) node() {}
func (*UnaryExpr) expr() {}
