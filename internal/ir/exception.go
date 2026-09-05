package ir

// ThrowStmt соответствует "выбросить <выражение>" - Value уже
// провалидирован семантическим анализом как значение сущности "Исключение"
// или наследника (internal/semantic - systemExceptionEntity).
type ThrowStmt struct {
	Value Expr
	Line  int
}

func (*ThrowStmt) stmt() {}

// TryStmt соответствует "попытаться { <тело> } поймать(<имя>: <Тип>[|<Тип>...]) { <обработчик> }".
// CatchTypes - имена сущностей ("Исключение" или наследники, проверено
// семантическим анализом). CatchVarType - тип, под которым CatchVar
// доступен в CatchBody: сам CatchTypes[0], если он один, иначе
// "Исключение" - единственный гарантированный общий предок всего списка
// (все элементы CatchTypes обязаны быть им самим или его наследником).
type TryStmt struct {
	Body       []Stmt
	CatchVar   string
	CatchTypes []string
	// CatchPanic - см. ast.TryStmt.CatchPanic (docs/ROADMAP.md, изоляция
	// паники) - "Паника" была в списке типов - CatchVarType всегда
	// TypeString, ловит "rech_panic"-класс ошибок. CatchTypes пуст -
	// "поймать (x: Паника)" в одиночку (не пересекается с rech_throw вовсе).
	// CatchTypes непуст - смешанный "поймать (x: Паника|Тип|...)": ловит И
	// панику, И перечисленные исключения ОДНИМ обработчиком (см. cbackend/
	// exceptions.go, generateIRTryCatchMixedStmt; internal/interp/
	// exception.go, execTryCatchMixed).
	CatchPanic   bool
	CatchVarType Type
	CatchBody    []Stmt
	// NoCatch - см. ast.TryStmt.NoCatch: "попытаться { ... } окончательно
	// { ... }" без единого "поймать" - CatchVar/CatchTypes/CatchPanic/
	// CatchVarType/CatchBody не имеют смысла, Finally обязан быть непустым.
	NoCatch bool
	// Finally - см. ast.TryStmt.Finally: непусто только у самого внешнего
	// узла цепочки "поймать" (или у единственного узла, если NoCatch) -
	// исполняется ровно один раз после конструкции целиком, независимо от
	// того, какой путь сработал (см. cbackend/exceptions.go,
	// generateIRTryWithFinallyStmt; internal/interp/exception.go,
	// execTryWithFinally).
	Finally []Stmt
}

func (*TryStmt) stmt() {}

// IsExpr соответствует "<выражение> относится к <Тип>" - логическая
// проверка фактического (времени исполнения) типа значения Value против
// Target. Не только для исключений - любая сущность/интерфейс, связанные
// наследованием или соответствием интерфейсу (проверено семантическим
// анализом, см. package doc internal/semantic - analyzeIsExpr).
type IsExpr struct {
	Value  Expr
	Target string
}

func (*IsExpr) expr() {}
