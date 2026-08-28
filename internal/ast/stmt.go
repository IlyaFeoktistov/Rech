package ast

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

// ShowStmt соответствует инструкции:
//
//	показать <выражение>
//
// Отладочный вывод: значение вместе с именем его типа, рекурсивно для
// сущностей (каждое поле) и встроенных коллекций (каждый элемент) - в
// отличие от PrintStmt, который поддерживает только примитивы.
type ShowStmt struct {
	Value Expr
	Line  int
}

func (*ShowStmt) node() {}
func (*ShowStmt) stmt() {}

// VarDecl соответствует инструкции объявления переменной с инициализацией:
//
//	переменная <имя> = <выражение>
//
// Тип переменной не аннотируется явно - выводится из выражения-инициализатора
// (простой literal-based inference, см. docs/ROADMAP.md, шаг 1).
type VarDecl struct {
	Name string
	// Type - явная аннотация типа ("переменная а: ?Целое = Ничто"), "" -
	// без аннотации (обычный вывод из Value, как раньше). Нужна там, где
	// вывести тип из Value невозможно - у голого "Ничто" (nullable-тип
	// иначе неоткуда взять) и у пустого литерала списка "[]".
	Type  string
	Value Expr
	Line  int
}

func (*VarDecl) node() {}
func (*VarDecl) stmt() {}

// ConstDecl - "константа <имя> = <значение>" - как VarDecl, но повторное
// присваивание этому имени (Assign) - ошибка компиляции
// (internal/semantic), не времени выполнения; локальная/верхнеуровневая
// константа вычисляется и хранится тем же кодом, что и обычная переменная
// (internal/ir не отличает их вообще - см. package doc internal/semantic).
//
// Visibility осмыслена только для константы внутри тела "сущность"
// (EntityDecl.Constants) - модификатор "приватная"/"публичная"/
// "защищённая" перед словом "константа", согласованный по роду/числу с
// самим словом "константа" (женский род, единственное число), а не с Name
// (имя константы обычно КАПС_ЛОКОМ и не является обычным
// существительным). У локальной/верхнеуровневой константы (тело функции/
// метода) видимость не имеет смысла - парсер модификатор перед ней не
// принимает, Visibility всегда Public.
type ConstDecl struct {
	Name       string
	Value      Expr
	Visibility Visibility
	Line       int
	// Doc - текст doc-комментария ("/** ... */") прямо перед объявлением,
	// "" - его не было. Осмысленно только у константы уровня сущности
	// (EntityDecl.Constants) - см. ast.EntityDecl.Doc; у ConstDecl,
	// встреченного как обычная инструкция внутри тела функции/метода,
	// парсер его так же разбирает, но там он не несёт смысла (см.
	// EntityDecl.Doc про локальные переменные).
	Doc string
}

func (*ConstDecl) node() {}
func (*ConstDecl) stmt() {}

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

// NullCoalesceAssign соответствует "а ?= б" (null-coalescing присваивание) -
// присвоить б переменной а, только если а сейчас Ничто, иначе оставить как
// есть. Только простое имя переменной (как у Assign), не поле сущности.
type NullCoalesceAssign struct {
	Name  string
	Value Expr
	Line  int
}

func (*NullCoalesceAssign) node() {}
func (*NullCoalesceAssign) stmt() {}

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
// встроенных коллекций (docs/SPEC.md, раздел 8) - не выброс исключения,
// который пользовательский код мог бы перехватить "попытаться"/"поймать" -
// ложное "утверждать" сигнализирует ошибку в самой программе, а не
// ожидаемое исключительное состояние. Не привязано к тестам как отдельной
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
	Await      *AwaitExpr
	Line       int
}

func (*ExprStmt) node() {}
func (*ExprStmt) stmt() {}
