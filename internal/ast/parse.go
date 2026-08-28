package ast

// ParseExpr соответствует выражению:
//
//	разобрать JSON <выражение> как <Тип>
//
// Source - Строка с текстом в формате Format ("JSON" - единственный
// поддержанный формат в этом срезе), TypeName - имя целевого типа
// (примитив/сущность/структура/Список<T>, рекурсивно, как у "новый Тип" -
// проверяется семантическим анализом на соответствие полю-в-поле формату,
// не парсером). При ошибке разбора (некорректный текст, отсутствующее
// поле, несовпадение типа) выбрасывается перехватываемое "ОшибкаРазбора"
// (наследник "Исключение", см. internal/ast/exception.go), не аварийное
// завершение процесса.
type ParseExpr struct {
	Format   string
	Source   Expr
	TypeName string
	Line     int
}

func (*ParseExpr) node() {}
func (*ParseExpr) expr() {}

// SerializeExpr соответствует выражению:
//
//	сериализовать JSON <выражение>
//
// Обратное к ParseExpr - Value любого JSON-совместимого типа (та же
// проверка, jsonCompatible) сериализуется в Строка. Нет "как <Тип>" -
// целевой тип и так известен (тип самого Value), нет отдельного пути
// ошибки (см. package doc internal/backend/cbackend/json_extract.go) -
// JSON-совместимость проверена статически, сериализация корректного
// значения не может провалиться во время исполнения.
type SerializeExpr struct {
	Format string
	Value  Expr
	Line   int
}

func (*SerializeExpr) node() {}
func (*SerializeExpr) expr() {}
