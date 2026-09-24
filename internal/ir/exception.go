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
// CatchTypes - имена сущностей ("Исключение"/"Паника" или наследники,
// проверено семантическим анализом); брошенное значение совпадает, если
// относится хотя бы к одному из них. CatchVarType - тип, под которым
// CatchVar доступен в CatchBody: сам CatchTypes[0], если он один; корень
// ("Исключение"/"Паника"), если все из одной иерархии; "Строка" (поле
// "сообщение" брошенного значения), если список смешивает обе иерархии -
// общего предка у них нет.
type TryStmt struct {
	Body         []Stmt
	CatchVar     string
	CatchTypes   []string
	CatchVarType Type
	CatchBody    []Stmt
	// NoCatch - см. ast.TryStmt.NoCatch: "попытаться { ... } окончательно
	// { ... }" без единого "поймать" - CatchVar/CatchTypes/CatchVarType/
	// CatchBody не имеют смысла, Finally обязан быть непустым.
	NoCatch bool
	// Finally - см. ast.TryStmt.Finally: непусто только у самого внешнего
	// узла цепочки "поймать" (или у единственного узла, если NoCatch) -
	// исполняется ровно один раз после конструкции целиком, независимо от
	// того, какой путь сработал (см. cbackend/exceptions.go,
	// generateIRTryWithFinallyStmt; internal/interp/exception.go,
	// execTryWithFinally).
	Finally []Stmt
	// Line - строка "попытаться" (у вложенных узлов цепочки "поймать" - та
	// же строка узла ast.TryStmt, из которого этот собран, см. package doc
	// ast.TryStmt) - нужна internal/ir/cfg для диагностики "недостижимый
	// код", когда недостижима сама конструкция целиком, а не что-то внутри
	// её тела/обработчика/"окончательно" (там своя Line у каждой инструкции).
	Line int
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
