package rtmsg

// Встроенная иерархия паник: корень PanicEntity и наследники по сути
// ошибки. Одни и те же имена у семантики (регистрирует сущности) и обоих
// backend'ов (бросают паники этих типов).
const (
	PanicEntity          = "Паника"
	PanicDivisionByZero  = "ДелениеНаНоль"
	PanicOutOfRange      = "ВыходЗаГраницы"
	PanicEmptyCollection = "ПустаяКоллекция"
	PanicKeyNotFound     = "КлючНеНайден"
	PanicBadCast         = "НеверноеПриведение"
	PanicAssertion       = "НарушениеУтверждения"
)

// PanicKinds - все встроенные типы паник; индекс - код вида в C-рантайме
// (rech_panic_as), 0 - сам корень.
var PanicKinds = []string{
	PanicEntity,
	PanicDivisionByZero,
	PanicOutOfRange,
	PanicEmptyCollection,
	PanicKeyNotFound,
	PanicBadCast,
	PanicAssertion,
}

// PanicKind - код вида паники name в PanicKinds (0, если name не встроенный).
func PanicKind(name string) int {
	for i, n := range PanicKinds {
		if n == name {
			return i
		}
	}
	return 0
}
