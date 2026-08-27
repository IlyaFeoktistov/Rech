package ir

// LaunchExpr соответствует "запустить <имя>(<арг>, ...)" - запускает
// обычную функцию программы в отдельном потоке ОС, возвращает значение
// типа TaskType(Callee) немедленно, не дожидаясь завершения (см. package
// doc internal/backend/cbackend/concurrency.go про то, почему обёртка -
// одна на каждую КОГДА-ЛИБО запущенную функцию, не одна на тип
// результата).
type LaunchExpr struct {
	Callee string
	Args   []Expr
}

func (*LaunchExpr) expr() {}

// AwaitExpr соответствует "дождаться <выражение>" - блокирует до
// завершения задачи Task; необработанное внутри неё исключение
// выбрасывается заново в потоке, исполняющем AwaitExpr (см. package doc
// ast.AwaitExpr). Callee/Type/HasReturn - скопированы из сигнатуры
// обёрнутой функции семантическим анализом (тот же принцип, что и у
// CallExpr.Type/HasReturn), чтобы backend не искал её сигнатуру заново.
type AwaitExpr struct {
	Task      Expr
	Callee    string
	Type      Type
	HasReturn bool
}

func (*AwaitExpr) expr() {}
