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
//	    метод(<парам>: <Тип>, ...): <ТипВозврата>
//	    {
//	        <тело>
//	    }
//	}
//
// ("класс" - синоним "сущность", docs/GRAMMAR.md).
type EntityDecl struct {
	Name    string
	States  []StateDecl
	Fields  []Field
	Methods []*MethodDecl
	Line    int
}

func (*EntityDecl) node() {}

// Field - одно поле сущности, объявленное под "имеет:". Type - имя типа,
// как и у Param (проверка допустимости - в backend'е).
type Field struct {
	Name string
	Type string
	Line int
}

// StateDecl - одно состояние, объявленное под "состояние:" (шаг 4.1,
// упрощённая версия - см. docs/ROADMAP.md/GRAMMAR.md). Name - краткая
// форма причастия/прилагательного ровно как написана в объявлении
// ("авторизован") - именно её должна повторить инструкция "становится
// <Name>" (BecomeStmt), без падежного согласования (это открытый вопрос,
// не решённый на этом шаге). TypeName - производный тип, синтезированный
// парсером при разборе сущности (полная форма по роду сущности + имя
// сущности, "АвторизованныйПользователь") - готовая строка, как и у
// generic-инстанциаций ("Список<Целое>"), backend не занимается
// морфологией, только регистрирует TypeName как синоним типа сущности.
type StateDecl struct {
	Name     string
	TypeName string
	Line     int
}

// MethodDecl - метод сущности: как FunctionDecl (те же параметры/тип
// результата/тело), но без ключевого слова "функция" - объявляется прямо в
// теле сущности (docs/GRAMMAR.md, "method"). Внутри тела метода доступно
// зарезервированное слово "этот" (ThisExpr) - ссылка на объект, у которого
// вызван метод.
type MethodDecl struct {
	Name       string
	Params     []Param
	ReturnType string
	Body       []Stmt
	Line       int
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

// Param - один параметр функции. Name - отображаемое имя (используется в
// теле функции). CaseWord - падежная роль параметра при естественном
// вызове (одно из кто/кого/кому/кем/где/что, см.
// internal/morph.CaseOfParamName) или "" - без падежной роли, тогда
// параметр доступен только позиционно (естественный вызов заполняет такие
// параметры "по остатку" - см. docs/SPEC.md, "Вызов функции (естественный
// синтаксис)"). Name == CaseWord, когда параметр объявлен одним словом
// (`кого: Тип`, исторический и по-прежнему основной случай) - падежное
// слово тогда одновременно и имя, и роль; двусловная форма (`кого
// пользователь: Тип`) даёт отдельное отображаемое имя при той же роли.
// Type - имя типа (проверка допустимости - в backend'е, как и для типа
// переменных, а не здесь, см. заголовочный комментарий cbackend/generator.go).
type Param struct {
	Name     string
	CaseWord string
	Type     string
	Line     int
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

// FieldAssign соответствует инструкции:
//
//	receiver.поле = выражение
//
// Отдельно от Assign (не переиспользует его), чтобы не усложнять уже
// рабочую логику присваивания переменной. Receiver в шаге 4.c - всегда
// *Identifier (получатель-переменная); "этот.поле = ..." внутри метода
// появится в 4.d, когда Receiver сможет быть и *ThisExpr - тип поля Expr
// оставлен общим уже сейчас, чтобы не переделывать этот узел ещё раз.
type FieldAssign struct {
	Receiver Expr
	Field    string
	Value    Expr
	Line     int
}

func (*FieldAssign) node() {}
func (*FieldAssign) stmt() {}

// BecomeStmt соответствует инструкции смены состояния (шаг 4.1, упрощённая
// версия):
//
//	receiver становится <состояние>
//
// State - краткая форма ровно как в StateDecl.Name (без падежного
// согласования - открытый вопрос, docs/GRAMMAR.md). Receiver - тот же
// класс выражений, что и у получателя вызова метода (переменная/"этот"/
// доступ к полю) - проверяется в backend'е.
type BecomeStmt struct {
	Receiver Expr
	State    string
	Line     int
}

func (*BecomeStmt) node() {}
func (*BecomeStmt) stmt() {}

// AssertStmt соответствует инструкции проверки условия:
//
//	утверждать <условие>
//
// Cond должно быть типа Логическое (проверяется backend'ом, как и условие
// "если"/"пока"). Если во время исполнения Cond ложно - аварийное
// завершение процесса (сообщение на stderr, exit(1)), как и rech_panic у
// встроенных коллекций (docs/SPEC.md, раздел 8) - не исключение (в языке
// пока нет исключений, шаг 14). Не привязано к тестам как отдельной
// концепции языка - обычная инструкция, доступная в любой функции/методе;
// "тестовый файл" - это соглашение имени файла на уровне CLI (`rech test`,
// docs/ROADMAP.md), не отдельный синтаксис.
type AssertStmt struct {
	Cond Expr
	Line int
}

func (*AssertStmt) node() {}
func (*AssertStmt) stmt() {}

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

// ForEachStmt соответствует циклу по коллекции (Список<T>, шаг 4.2.b):
//
//	для <имя> в <коллекция> { <тело> }
//
// Collection - Identifier, полученный разбором словоформы в предложном
// падеже после "в" (согласование с предлогом) и обратной резолюцией через
// морфологический словарь к лемме переменной (internal/parser,
// resolvePrepositionalNoun) - тем же механизмом, что и падежное
// сопоставление аргументов естественного вызова (шаг 3.4), а не голым
// совпадением текста словоформы с именем переменной.
type ForEachStmt struct {
	Var        string
	Collection Expr
	Body       []Stmt
	Line       int
}

func (*ForEachStmt) node() {}
func (*ForEachStmt) stmt() {}

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
// Ровно одно из Call/MethodCall заполнено - строгий вызов функции или
// вызов метода сущности (шаг 4.d) как самостоятельная инструкция.
type ExprStmt struct {
	Call       *CallExpr
	MethodCall *MethodCallExpr
	IncDec     *IncDecExpr
	Line       int
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

// NewExpr - конструктор сущности, дающий значение:
//
//	новый Тип(<арг>, ...)                       - позиционная форма
//	новый Тип(<поле>: <арг>, ...)                - именованная форма
//
// Ключевое слово ("новый"/"новая"/"новое"/"новые") согласуется по роду/
// числу с Type (проверяется парсером, internal/morph.NounAgreement) - само
// AST хранит только Type, форма ключевого слова уже проверена и дальше не
// нужна.
//
// FieldNames == nil - позиционная форма: Args[i] - значение поля с
// индексом i в EntityDecl.Fields. FieldNames != nil (той же длины, что
// Args) - именованная форма: FieldNames[i] - имя поля для значения Args[i],
// в любом порядке относительно объявления полей.
type NewExpr struct {
	Type       string
	Args       []Expr
	FieldNames []string
	Line       int
}

func (*NewExpr) node() {}
func (*NewExpr) expr() {}

// FieldAccess - чтение поля сущности:
//
//	receiver.поле
//
// Receiver - произвольное выражение (переменная, this и т.д.) - не только
// IDENT, чтобы в будущем работали цепочки (`x.поле.поле`), даже если шаг 4
// сам эти цепочки ещё не порождает.
type FieldAccess struct {
	Receiver Expr
	Field    string
	Line     int
}

func (*FieldAccess) node() {}
func (*FieldAccess) expr() {}

// ThisExpr - "этот": ссылка на объект, у которого вызван текущий метод.
// Валидно только внутри тела метода (проверяется парсером - структурное,
// не зависящее от типов свойство).
type ThisExpr struct {
	Line int
}

func (*ThisExpr) node() {}
func (*ThisExpr) expr() {}

// MethodCallExpr - вызов метода сущности, дающий значение:
//
//	receiver.метод(<арг>, ...)
//
// Только строгий синтаксис в шаге 4 - естественный вызов метода
// (`receiver метод ...`) вне скоупа (docs/GRAMMAR.md).
type MethodCallExpr struct {
	Receiver Expr
	Method   string
	Args     []Expr
	Line     int
}

func (*MethodCallExpr) node() {}
func (*MethodCallExpr) expr() {}

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

// IncDecExpr соответствует "имя++"/"имя--" (постфикс, Prefix == false) или
// "++имя"/"--имя" (префикс, Prefix == true) - применимо только к уже
// объявленной переменной типа Целое/Число (не к полю сущности, не к
// результату вызова - только простое имя переменной, как и Assign).
// Полноценное выражение со значением (постфикс возвращает старое
// значение, префикс - новое, как в C), а не только самостоятельная
// инструкция - ExprStmt.IncDec покрывает форму "на своей строке".
type IncDecExpr struct {
	Name   string
	Op     string // "++" | "--"
	Prefix bool
	Line   int
}

func (*IncDecExpr) node() {}
func (*IncDecExpr) expr() {}
