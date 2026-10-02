package ast

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
// Строгий и естественный синтаксис вызова (docs/GRAMMAR_DRAFT.md, "Два
// представления одного AST") - это два способа получить один и тот же
// узел: Args всегда упорядочен по позиции параметра в объявлении функции
// (FunctionDecl.Params), а не по порядку слов в исходнике - естественный
// вызов раскладывает падежные аргументы по этому порядку при разборе.
type CallExpr struct {
	Callee string
	Args   []Expr
	// TypeArgs - явные аргументы типа ("первый<Целое>(...)") - nil у вызова
	// обычной (не generic) функции.
	TypeArgs []string
	Line     int
	// ID - номер узла в программе (semantic, generic_infer.go) - ключ
	// выведенных аргументов типа; копия тела generic-шаблона сохраняет ID
	// своего узла.
	ID int
}

func (*CallExpr) node() {}
func (*CallExpr) expr() {}

// CallValueExpr - вызов РЕЗУЛЬТАТА выражения типа "Вызываемый":
//
//	сп.получить(0)(3)
//	создать_обработчик()(запрос)
//
// В отличие от CallExpr (вызов по имени функции или переменной), Callee -
// произвольное выражение: результат вызова, метода или индексации.
type CallValueExpr struct {
	Callee Expr
	Args   []Expr
	Line   int
}

func (*CallValueExpr) node() {}
func (*CallValueExpr) expr() {}

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
	// ID - см. CallExpr.ID.
	ID int
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
	// Static - разобрано через "::" ("Тип::ИМЯ"), а не "." - Receiver
	// обязан резолвиться в имя типа (сущности/структуры), а не значение, и
	// Field - обязан быть статическим членом (см. docs/SPEC.md,
	// "Статические члены"). false - обычное "." (получатель - значение,
	// поле/константа читается через него как раньше).
	Static bool
	Line   int
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
// (`receiver метод ...`) вне скоупа (docs/GRAMMAR_DRAFT.md).
type MethodCallExpr struct {
	Receiver Expr
	Method   string
	Args     []Expr
	// Static - разобрано через "::" ("Тип::метод(...)") - см. package doc
	// FieldAccess.Static, тот же смысл, для вызова метода вместо чтения
	// поля.
	Static bool
	Line   int
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
	// Bang - узел написан как "!x" (парсер десахаривает в "x == Ничто"):
	// у Логическое операнда это отрицание, как "не x".
	Bang bool
}

func (*BinaryExpr) node() {}
func (*BinaryExpr) expr() {}

// TernaryExpr - "условие ? тогда : иначе" (тернарный условный оператор,
// docs/SPEC.md) - Cond обязан разбираться в Логическое, Then/Else -
// строго одинакового типа. Правоассоциативен (`а ? б : в ? г : д` -
// `а ? б : (в ? г : д)`), тем же принципом, что и в C/большинстве
// C-подобных языков.
type TernaryExpr struct {
	Cond Expr
	Then Expr
	Else Expr
	Line int
}

func (*TernaryExpr) node() {}
func (*TernaryExpr) expr() {}

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

// NullLiteral - "Ничто", отсутствие значения у nullable-переменной
// ("?Тип") - сам по себе не несёт типа, internal/semantic резолвит его
// ровно там, где известен ожидаемый nullable-тип (объявление/присваивание/
// сравнение/аргумент/поле/результат).
type NullLiteral struct {
	Line int
}

func (*NullLiteral) node() {}
func (*NullLiteral) expr() {}

// IndexExpr - безопасная индексация "receiver[index]" (Список<T>/
// Словарь<К, В>) - в отличие от ".получить(i)"/".получить(ключ)" (которые
// паникуют при промахе), значение всегда "?T"/"?В" - Ничто при выходе за
// границу/отсутствии ключа, см. docs/SPEC.md. Только чтение - запись
// "receiver[index] = значение" разбирается сразу как сахар над
// "receiver.установить(index, значение)" (ast.MethodCallExpr), этот узел
// для неё не используется вовсе.
type IndexExpr struct {
	Receiver Expr
	Index    Expr
	Line     int
}

func (*IndexExpr) node() {}
func (*IndexExpr) expr() {}

// ListLiteral - "[элемент, ...]" (в т.ч. пустой "[]") - создаёт
// Список<T>. Пустой литерал типизируется только из контекста (переменная с
// явной аннотацией типа) - сам по себе не несёт типа, как и ast.NullLiteral.
// Непустой - тип элемента выводится из ПЕРВОГО элемента, остальные обязаны
// быть РОВНО того же типа (массивы строго типизированы, без неявного
// расширения) - тогда работает и без аннотации типа переменной, как
// обычный литерал.
type ListLiteral struct {
	Elems []Expr
	Line  int
}

func (*ListLiteral) node() {}
func (*ListLiteral) expr() {}

// SpreadExpr - "...список" среди аргументов вызова вариативной функции или
// элементов литерала списка: элементы Value (Список) подставляются по
// одному на это место.
type SpreadExpr struct {
	Value Expr
	Line  int
}

func (*SpreadExpr) node() {}
func (*SpreadExpr) expr() {}
