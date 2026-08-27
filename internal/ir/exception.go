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
	Body         []Stmt
	CatchVar     string
	CatchTypes   []string
	CatchVarType Type
	CatchBody    []Stmt
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
