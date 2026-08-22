// Package ir описывает промежуточное представление программы (шаг 5,
// docs/ROADMAP.md) - слой между ast.Program и backend'ами (cbackend,
// будущий interpreter), не зависящий ни от русской грамматики (имена -
// уже проверенные идентификаторы, не произвольные словоформы падежного
// сопоставления), ни от конкретного backend'а (нет понятия mangleName/
// mangleType/C-типов - это решает cbackend или interpreter по-своему).
//
// internal/semantic производит *Program из *ast.Program (семантический
// анализ - проверка типов, разрешение имён, шаг 5.b) - см. пакет semantic
// для деталей.
//
// Реализуется вертикальными срезами по конструкциям языка (стратегия,
// согласованная с пользователем для шага 5, см. docs/ROADMAP.md, Шаг 5) -
// а не всей IR сразу для всех конструкций. Первый срез - буквально то же
// подмножество, что и шаг 0 ("функция главная { вывести "..." }"). Пока
// покрытие не станет полным, IR НЕ встроен в реальный путь компиляции
// (internal/compiler по-прежнему вызывает cbackend.Generate(*ast.Program)
// напрямую) - internal/ir и internal/semantic существуют параллельно,
// проверяются собственными тестами, и заменяют старый путь только когда
// покрытие станет исчерпывающим.
package ir

// Program - корень IR-дерева программы.
type Program struct {
	Functions []*Function
}

// Function - функция программы после семантического анализа. Name - имя
// ровно как в Rech-исходнике (например, "главная") - маппинг на
// C-специфичное имя (например, main) - решение backend'а, не IR.
type Function struct {
	Name string
	Body []Stmt
}

// Stmt - общий маркерный интерфейс инструкций IR (как ast.Stmt).
type Stmt interface {
	stmt()
}

// PrintStmt соответствует "вывести <выражение>".
type PrintStmt struct {
	Value Expr
}

func (*PrintStmt) stmt() {}

// Expr - общий маркерный интерфейс выражений IR (как ast.Expr).
type Expr interface {
	expr()
}

// StringLiteral - строковый литерал.
type StringLiteral struct {
	Value string
}

func (*StringLiteral) expr() {}
