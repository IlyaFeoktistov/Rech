package ir

type Stmt interface {
	stmt()
}

// Line - номер исходной строки Rech, которую представляет инструкция.
// Несёт его каждый вид Stmt, исполнение которого может само по себе
// привести к панике или уйти вложенным вызовом (backend/интерпретатор
// используют его для стека вызовов при аварийном завершении/необработанном
// исключении - internal/backend/cbackend/trace.go, internal/interp/trace.go;
// internal/lsp - чтобы определить область видимости локальных переменных по
// позиции курсора). BecomeStmt его не несёт - "становится" не исполняется
// во время выполнения (чисто компилятивная проверка).

// LineOf возвращает Line инструкции s, 0 - если её вид Line не несёт
// (BecomeStmt или неизвестный внешнему коду вид). Общий хелпер для всех
// потребителей (см. package doc Line выше) - раньше был отдельной копией
// в internal/backend/cbackend и internal/interp по отдельности.
func LineOf(s Stmt) int {
	switch s := s.(type) {
	case *PrintStmt:
		return s.Line
	case *ShowStmt:
		return s.Line
	case *VarDecl:
		return s.Line
	case *Assign:
		return s.Line
	case *FieldAssign:
		return s.Line
	case *StaticFieldAssign:
		return s.Line
	case *DerefAssignStmt:
		return s.Line
	case *ParentCallStmt:
		return s.Line
	case *AssertStmt:
		return s.Line
	case *IfStmt:
		return s.Line
	case *WhileStmt:
		return s.Line
	case *ForStmt:
		return s.Line
	case *ForEachStmt:
		return s.Line
	case *CStyleForStmt:
		return s.Line
	case *BreakStmt:
		return s.Line
	case *ContinueStmt:
		return s.Line
	case *ReturnStmt:
		return s.Line
	case *ExprStmt:
		return s.Line
	case *ThrowStmt:
		return s.Line
	case *TryStmt:
		return s.Line
	default:
		return 0
	}
}

// PrintStmt соответствует "вывести <выражение>". Line - см. package doc,
// "Строка исполнения для стека вызовов" ниже.
type PrintStmt struct {
	Value Expr
	Line  int
}

func (*PrintStmt) stmt() {}

// ShowStmt соответствует "показать <выражение>" - отладочный вывод значения
// вместе с именем его типа, рекурсивно для сущностей (поля) и встроенных
// коллекций (элементы). Тип значения - TypeOf(Value), как и у PrintStmt;
// имена полей сущности codegen берёт из Program.Entities по имени типа.
type ShowStmt struct {
	Value Expr
	Line  int
}

func (*ShowStmt) stmt() {}

// VarDecl соответствует "переменная <имя> = <выражение>". Тип переменной -
// тип Value (literal-based inference) - разрешён и провалидирован
// семантическим анализом (internal/semantic), в самом узле не хранится
// отдельно - Value.Type()/TypeOf(Value) и есть тип переменной.
type VarDecl struct {
	Name  string
	Value Expr
	// IsStatic - объявлена "статическая переменная" (ast.VarDecl.IsStatic):
	// Value вычисляется и запоминается РОВНО один раз за всё время работы
	// программы (при первом достижении этой инструкции), при повторных
	// достижениях (новый вызов функции, следующая итерация цикла) backend
	// пропускает вычисление и подставляет уже сохранённое значение - как
	// "static" локальная переменная в C. StaticID при этом всегда заполнен.
	IsStatic bool
	// StaticID - уникальный на всю программу номер этого объявления,
	// назначенный семантическим анализом (Analyzer.nextStaticVarID) только
	// когда IsStatic - хранилище для значения ключуется им, а не именем
	// переменной (два разных "статическая переменная x" в разных функциях -
	// два независимых хранилища, как и должно быть у "static" в C). 0 -
	// IsStatic == false.
	StaticID int
	Line     int
	// DeclType - тип из аннотации ("Фигура ф = ...", "переменная ф: Фигура
	// = ..."), тип переменной; нулевое значение (KindUnknown) - аннотации
	// не было, тип переменной - TypeOf(Value).
	DeclType Type
	// IsConst - объявлена "константа" (для инструментов: hover пишет
	// "константа", не "переменная"); backend'ы её не различают.
	IsConst bool
	// Doc - doc-комментарий объявления (ast.VarDecl/ConstDecl.Doc).
	Doc string
}

func (*VarDecl) stmt() {}

// Assign соответствует "<имя> = <выражение>" для уже объявленной
// переменной - совпадение типа с объявленным при первом "переменная" уже
// проверено семантическим анализом.
type Assign struct {
	Name  string
	Value Expr
	Line  int
}

func (*Assign) stmt() {}

// FieldAssign соответствует "receiver.поле = выражение" - Receiver уже
// проверен на адресуемость (переменная/"этот"/доступ к полю, как и
// получатель вызова метода) семантическим анализом.
type FieldAssign struct {
	Receiver Expr
	Field    string
	Value    Expr
	Line     int
}

func (*FieldAssign) stmt() {}

// StaticFieldAssign соответствует "Тип::поле = выражение" - как
// FieldAssign, но без получателя-значения (см. package doc
// ir.StaticFieldExpr).
type StaticFieldAssign struct {
	Entity string
	Field  string
	Value  Expr
	Line   int
}

func (*StaticFieldAssign) stmt() {}

// BecomeStmt соответствует "receiver становится <состояние>" - состояние
// без падежного согласования, ровно как в объявлении "состояние:". Не несёт
// типа - "становится" не эмитит C-код (typestate чисто компилятивный),
// состояние уже проверено семантическим анализом как объявленное у сущности
// Receiver'а.
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
	Line       int
}

func (*ParentCallStmt) stmt() {}

// AssertStmt соответствует "утверждать <условие>[, <сообщение>]" - Cond уже
// провалидирован (тип Логическое) семантическим анализом. Line нужен
// backend'у/интерпретатору для сообщения об ошибке во время исполнения
// ("утверждение не выполнено (строка N)"), которое иначе указывало бы
// только "где-то в программе". Message - необязательное (nil - его не было)
// выражение типа Строка (внятные сообщения о
// несовпадении, тот же принцип, что и у Go "t.Errorf(сообщение)") -
// дописывается к стандартному тексту через ": " при провале, см.
// generateIRStmtBody/*ir.AssertStmt (cbackend) и exec.go/*ir.AssertStmt
// (interp).
type AssertStmt struct {
	Cond    Expr
	Message Expr
	Line    int
}

func (*AssertStmt) stmt() {}

// IfStmt соответствует "если <условие> { <then> } [иначе { <else> }]" -
// Cond уже провалидирован (тип Логическое) семантическим анализом.
type IfStmt struct {
	Cond Expr
	Then []Stmt
	Else []Stmt
	Line int
}

func (*IfStmt) stmt() {}

// WhileStmt соответствует "пока <условие> { <тело> }".
type WhileStmt struct {
	Cond Expr
	Body []Stmt
	Line int
}

func (*WhileStmt) stmt() {}

// ForStmt соответствует "для <имя> от <from> до <to> { <тело> }" - цикл по
// целочисленному диапазону (включая обе границы). Var - переменная цикла,
// всегда типа Целое.
type ForStmt struct {
	Var  string
	From Expr
	To   Expr
	Body []Stmt
	Line int
}

func (*ForStmt) stmt() {}

// ForEachKind - вид коллекции, по которой идёт ForEachStmt: определяет
// порядок обхода и представление коллекции в backend'е.
type ForEachKind int

const (
	// ForEachList - Список<T>, от первого элемента к последнему.
	ForEachList ForEachKind = iota
	// ForEachStack - Стопка<T>, сверху вниз (в порядке "снять").
	ForEachStack
	// ForEachQueue - Очередь<T>, от первого к последнему (в порядке
	// "извлечь").
	ForEachQueue
	// ForEachDict - Словарь<К, В>, в порядке вставки ключей.
	ForEachDict
	// ForEachFile - Файл, построчно до конца файла.
	ForEachFile
)

// ForEachStmt соответствует "для [<ключ>,] <имя> в <коллекция> { <тело> }" -
// цикл по коллекции вида Kind. Collection уже проверен семантическим
// анализом, ElemType - тип элемента (значения у Словаря), нужен backend'у
// для C-типа переменной цикла, чтобы не искать сигнатуру коллекции заново.
// KeyVar (пусто у формы с одной переменной) - ключ у Словаря (тип KeyType),
// порядковый номер элемента в обходе с 0 у остальных видов (KeyType -
// Целое).
type ForEachStmt struct {
	Kind       ForEachKind
	KeyVar     string
	KeyType    Type
	Var        string
	Collection Expr
	ElemType   Type
	Body       []Stmt
	Line       int
}

func (*ForEachStmt) stmt() {}

// CStyleForStmt соответствует общему циклу "для <init>; <условие>; <шаг>
// { <тело> }" - обобщение ForStmt для произвольного шага/направления. Init -
// переменная цикла, объявленная заново в его собственной области видимости
// (тем же принципом, что и ForStmt.Var/ForEachStmt.Var); Post - инструкция,
// исполняемая после Body на каждой итерации, перед повторной проверкой
// Cond, ровно как в C. Init/Cond/Post - каждый независимо может быть nil
// (пустая секция, как в C) - Cond == nil равносильно "истина" (бесконечный
// цикл, "для ;; { ... }").
type CStyleForStmt struct {
	Init *VarDecl
	Cond Expr
	Post Stmt
	Body []Stmt
	Line int
}

func (*CStyleForStmt) stmt() {}

// BreakStmt соответствует "прервать"/"прервать N" - немедленный выход из
// Depth-го по счёту (считая текущий, 1 - ближайший) охватывающего цикла.
// Depth уже проверен семантическим анализом (1..глубина вложенности циклов
// в этой точке) - оба backend'а доверяют этому, не перепроверяют.
type BreakStmt struct {
	Depth int
	Line  int
}

func (*BreakStmt) stmt() {}

// ContinueStmt соответствует "продолжить"/"продолжить N" - переход к
// следующей итерации Depth-го по счёту (считая текущий, 1 - ближайший)
// охватывающего цикла (у CStyleForStmt - через Post). Depth уже проверен
// семантическим анализом, см. BreakStmt.
type ContinueStmt struct {
	Depth int
	Line  int
}

func (*ContinueStmt) stmt() {}

// ReturnStmt соответствует "вернуть [<выражение>]" - Value == nil у функции
// без объявленного типа результата; у функции с объявленным типом
// результата Value обязателен и его тип уже проверен internal/semantic на
// совпадение с ReturnType функции/метода.
type ReturnStmt struct {
	Value Expr
	Line  int
}

func (*ReturnStmt) stmt() {}

type ExprStmt struct {
	Call             *CallExpr
	CallValue        *CallValueExpr
	MethodCall       *MethodCallExpr
	StaticMethodCall *StaticMethodCallExpr
	IncDec           *IncDecExpr
	Await            *AwaitExpr
	Line             int
}

func (*ExprStmt) stmt() {}

// GerundStmt - деепричастный вызов с блоком (ast.GerundStmt) -
// Body уже несёт САМ вызов первым элементом (VarDecl{Name: "результат",
// Value: CallExpr{...}} - см. internal/semantic/gerund.go, analyzeGerundStmt),
// GerundStmt здесь - только маркер "это отдельная область видимости"
// (backend'у достаточно исполнить/сгенерировать Body в новой вложенной
// области, как тело если/пока - см. package doc там же).
type GerundStmt struct {
	Body []Stmt
	Line int
}

func (*GerundStmt) stmt() {}

// BlockStmt - произвольная последовательность инструкций в собственной
// области видимости, без условия/цикла/деепричастия вокруг неё - backend'у
// достаточно исполнить/сгенерировать Body в новой вложенной области, тем же
// принципом, что и у GerundStmt (см. package doc там же), но без семантики
// "результат"/деепричастного вызова. Используется десахариванием "выбрать
// ... когда ..." (internal/semantic/switch.go) - Body несёт временную
// переменную под однократно вычисленное значение выбора, затем цепочку
// "если/иначе если", в которую semantic analysis развернул ветки "когда".
type BlockStmt struct {
	Body []Stmt
	Line int
}

func (*BlockStmt) stmt() {}

// TypeKind - "форма" типа значения: один из встроенных примитивов или
// KindEntity (сущность, чьё имя несёт сама Type.Entity - без этого
// сравнение двух Type/вывод C-типа нуждались бы во внешнем реестре, тот же
// принцип, что и у valueType в cbackend).
