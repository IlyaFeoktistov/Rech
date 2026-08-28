package rtmsg

import "fmt"

// Шаблоны экспортированы отдельно от Format-функций - cbackend встраивает
// их же (escapeCString) буквально в текст C-хелперов рефлексии
// (internal/backend/cbackend/reflect.go), с C printf-аргументами вместо
// Go fmt.Sprintf - гарантирует байт-в-байт совпадение текста паники между
// cbackend и internal/interp по построению, не по совпадению вручную
// продублированной строки.

// AsTypeMismatchTemplate/AsTypeMismatchFormat - сообщение при "значение
// как Тип", если фактический тип боксированного значения (actual) не
// совпадает с целевым (expected).
const AsTypeMismatchTemplate = "каст (Значение -> %s): фактический тип значения - %s"

func AsTypeMismatchFormat(actual, expected string) string {
	return fmt.Sprintf(AsTypeMismatchTemplate, expected, actual)
}

// ReflectIndexOutOfRangeTemplate/ReflectIndexOutOfRangeFormat -
// сообщение при выходе за границы у индексных методов "Тип"
// (имя_поля/тип_поля/имя_метода/...) - kind - "поле"/"метод"/"параметр".
const ReflectIndexOutOfRangeTemplate = "%s: индекс вне границ у типа \"%s\""

func ReflectIndexOutOfRangeFormat(kind, typeName string) string {
	return fmt.Sprintf(ReflectIndexOutOfRangeTemplate, kind, typeName)
}

// ReflectFieldNotFoundTemplate/ReflectFieldNotFoundFormat - сообщение при
// "значение.получить_поле"/"значение.установить_поле" по несуществующему
// имени поля.
const ReflectFieldNotFoundTemplate = "поле \"%s\" не найдено у типа \"%s\""

func ReflectFieldNotFoundFormat(name, typeName string) string {
	return fmt.Sprintf(ReflectFieldNotFoundTemplate, name, typeName)
}

// ReflectMethodNotFoundTemplate/ReflectMethodNotFoundFormat - сообщение
// при "значение.вызвать"/"тип.создать" по несуществующему имени метода/
// недостижимому конструктору.
const ReflectMethodNotFoundTemplate = "метод \"%s\" не найден у типа \"%s\""

func ReflectMethodNotFoundFormat(name, typeName string) string {
	return fmt.Sprintf(ReflectMethodNotFoundTemplate, name, typeName)
}

// ReflectArgCountMismatchTemplate/ReflectArgCountMismatchFormat -
// сообщение при "значение.вызвать"/"тип.создать", если число переданных
// аргументов не совпадает с объявленным числом параметров метода/
// конструктора name.
const ReflectArgCountMismatchTemplate = "метод \"%s\" ожидает аргументов: %d, передано: %d"

func ReflectArgCountMismatchFormat(name string, expected, got int) string {
	return fmt.Sprintf(ReflectArgCountMismatchTemplate, name, expected, got)
}
