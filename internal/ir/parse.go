package ir

// ParseExpr - "разобрать JSON <выражение> как <Тип>" (Format - "JSON",
// единственный поддержанный формат в этом срезе). Source - уже
// проверенное семантическим анализом выражение типа Строка, To - целевой
// тип (примитив/сущность/структура/Список<T>, рекурсивно), уже проверенный
// на JSON-совместимость (internal/semantic/parse.go, jsonCompatible). При
// ошибке разбора кодогенерация выбрасывает "ОшибкаРазбора" (наследник
// "Исключение") - см. package doc internal/backend/cbackend/json_extract.go
// и internal/interp/json.go.
type ParseExpr struct {
	Format string
	Source Expr
	To     Type
}

func (*ParseExpr) expr() {}

// SerializeExpr - "сериализовать JSON <выражение>" - обратное к ParseExpr,
// Value любого JSON-совместимого типа сериализуется в Строка (нет
// отдельного целевого типа - им и так является тип Value).
type SerializeExpr struct {
	Format string
	Value  Expr
}

func (*SerializeExpr) expr() {}
