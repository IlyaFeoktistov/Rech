package ir

// StateCheckExpr - рантайм-проверка typestate: Value (сущность типа
// Entity) должна сейчас находиться в состоянии State - иначе бросает
// перехватываемое "Исключение". Заменяет собой параметр, объявленный
// производным типом состояния, аргумент под который не удалось доказать
// established на этапе компиляции (internal/semantic, checkCallArgs) -
// раньше это была жёсткая ошибка компиляции, теперь недоказуемость
// откладывается до рантайма. Прозрачна для типа: TypeOf(e) == TypeOf(e.
// Value) - составляется на месте самого Value везде, где ожидалось бы
// обычное выражение.
type StateCheckExpr struct {
	Value  Expr
	Entity string
	State  string
	Line   int
}

func (*StateCheckExpr) expr() {}

// HasStateExpr - "<выражение> относится к <ПроизводныйТипСостояния>":
// Логическое, есть ли у объекта СЕЙЧАС, во время выполнения, состояние
// State - в отличие от StateCheckExpr, никогда не бросает исключение,
// только сообщает результат (то же "относится к", что и обычная проверка
// типа/интерфейса, ast.IsExpr - internal/semantic, analyzeIsExpr, решает
// по Target, что именно строить: голое имя типа состояния - HasStateExpr,
// иначе обычный ir.IsExpr).
type HasStateExpr struct {
	Value  Expr
	Entity string
	State  string
}

func (*HasStateExpr) expr() {}
