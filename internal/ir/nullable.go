package ir

// NullLiteral - "Ничто", отсутствие значения у nullable-переменной ("?Тип").
// Type - конкретный "?Тип" контекста, куда литерал попал (переменная a: ?Тип
// = Ничто, аргумент/поле/результат nullable-типа) - сам литерал без
// контекста типа не несёт (internal/semantic резолвит его ровно там, где
// известен ожидаемый тип - см. package doc internal/semantic/nullable.go).
type NullLiteral struct {
	Type Type
	Line int
}

func (*NullLiteral) expr() {}

// NullWrapExpr - неявное заворачивание значения базового типа в "?Тип"
// (переменная a: ?Целое = 10 - Value несёт "10" типа Целое, Type - "?Целое") -
// вставляется семантическим анализом на каждой границе, куда значение
// базового типа попадает в nullable-контекст (объявление/присваивание/
// аргумент/поле/результат) - ни один backend не производит его сам.
type NullWrapExpr struct {
	Value Expr
	Type  Type
	Line  int
}

func (*NullWrapExpr) expr() {}

// NullUnwrapExpr - разворачивание "?Тип" в Тип, Type - результирующий (уже
// БЕЗ "?"). Два источника одного и того же узла: явный каст "(Тип)
// значение" (может паниковать в рантайме на Ничто) и неявное использование
// значения, для которого компилятор уже доказал непустоту через "если а !=
// Ничто {...}" (см. package doc internal/semantic/nullable.go) - в обоих
// случаях backend генерирует одну и ту же проверку/паникующий доступ, разницы
// между ними на уровне IR нет (доказанная непустота просто не должна
// сработать на паническую ветку во время исполнения).
type NullUnwrapExpr struct {
	Value Expr
	Type  Type
	Line  int
}

func (*NullUnwrapExpr) expr() {}

// NullCheckExpr - "а == Ничто"/"а != Ничто" (IsNot=true у "!="). Value -
// проверяемое nullable-значение, результат всегда Логическое. Отдельный
// узел, не обычный BinaryExpr - "Ничто" в исходнике не имеет собственного
// типа/представления, сравнивать его как обычный операнд бессмысленно, здесь
// проверяется только тег занятости nullable-обёртки.
type NullCheckExpr struct {
	Value Expr
	IsNot bool
	Line  int
}

func (*NullCheckExpr) expr() {}

// NullCoalesceExpr - "а ?? б": значение a, если оно не Ничто, иначе
// значение b. Type - тип результата (Тип - если Right не nullable, иначе
// снова "?Тип", см. package doc internal/semantic/nullable.go).
type NullCoalesceExpr struct {
	Left  Expr
	Right Expr
	Type  Type
	Line  int
}

func (*NullCoalesceExpr) expr() {}

// NullCoalesceAssignStmt - "а ?= б": присвоить b переменной a, только если a
// сейчас Ничто (иначе оставить как есть) - применимо только к простому имени
// уже объявленной nullable-переменной (тем же ограничением, что у Assign).
type NullCoalesceAssignStmt struct {
	Name  string
	Value Expr
	Line  int
}

func (*NullCoalesceAssignStmt) stmt() {}

// NullableInstantiation - конкретный "?Тип", встретившийся в программе -
// тем же принципом, что GenericInstantiation для Список/Стопка/Очередь/
// Словарь: backend сам генерирует C-структуру (тег занятости + значение) и
// вспомогательные функции (обернуть/пусто/развернуть) по Inner, IR только
// называет, какая инстанциация встретилась.
type NullableInstantiation struct {
	Name  string // "?Целое" - он же Type.Entity
	Inner Type
}
