// Package rtmsg содержит текст сообщений об ошибках времени выполнения
// программы Rech (не компилятора): выход за границы/операция на пустой
// встроенной коллекции, ложное условие "утверждать". Три независимых
// потребителя одного и того же internal/ir (cbackend.Generate, AST-путь;
// cbackend.GenerateFromIR, IR-путь; internal/interp) должны печатать
// одинаковый текст для одинаковой ошибки - отсюда пакет без зависимостей,
// который может импортировать любой из них.
package rtmsg

import "fmt"

// PanicPrefix - префикс перед текстом ошибки при аварийном завершении
// программы (rech_panic в сгенерированном C, panicRuntime в
// internal/interp).
const PanicPrefix = "выполнение прервано: "

// AssertFailed - сообщение при ложном условии "утверждать".
func AssertFailed(line int) string {
	return fmt.Sprintf("утверждение не выполнено (строка %d)", line)
}

// IndexOutOfRange - сообщение при индексе вне границ у Список<T>.
func IndexOutOfRange(typeName string) string {
	return fmt.Sprintf("индекс вне границ у %s", typeName)
}

// EmptyStackOp - сообщение при "снять"/"верх" на пустой Стопке.
func EmptyStackOp(typeName string) string {
	return fmt.Sprintf("нельзя снять/получить верх пустой %s", typeName)
}

// EmptyQueueOp - сообщение при "извлечь"/"первый" на пустой Очереди.
func EmptyQueueOp(typeName string) string {
	return fmt.Sprintf("нельзя извлечь/получить первый элемент пустой %s", typeName)
}

// DivisionByZero - сообщение при делении Целое на ноль. Только для
// целочисленного деления - деление Число (float64) на 0.0 по IEEE 754 даёт
// +-Inf/NaN, не аварийное завершение, и одинаково ведёт себя в C и в Go, не
// нуждается в отдельной проверке.
const DivisionByZero = "деление на ноль"

// ReadEOF - сообщение при попытке прочитатьСтроку/прочитатьЦелое/
// прочитатьЧисло, когда стандартный ввод уже закончился (шаг 8).
const ReadEOF = "не удалось прочитать: конец ввода"

// ReadIntFormat - сообщение при прочитатьЦелое, если введённая строка - не
// целое число. Кавычки вокруг input - буквальные "\"%s\"", не Go-verb %q
// (который экранировал бы спецсимволы иначе, чем C printf "%s") - формат
// должен совпадать байт-в-байт с cbackend (internal/backend/cbackend.
// readHelpers, rech_read_int).
func ReadIntFormat(input string) string {
	return fmt.Sprintf("прочитатьЦелое: \"%s\" не является целым числом", input)
}

// ReadFloatFormat - то же самое для прочитатьЧисло.
func ReadFloatFormat(input string) string {
	return fmt.Sprintf("прочитатьЧисло: \"%s\" не является числом", input)
}
