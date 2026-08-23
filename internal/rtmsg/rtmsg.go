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

// ReadEOF - сообщение при попытке прочитать_строку/прочитать_целое/
// прочитать_число, когда стандартный ввод уже закончился (шаг 8).
const ReadEOF = "не удалось прочитать: конец ввода"

// ReadIntFormat - сообщение при прочитать_целое, если введённая строка - не
// целое число. Кавычки вокруг input - буквальные "\"%s\"", не Go-verb %q
// (который экранировал бы спецсимволы иначе, чем C printf "%s") - формат
// должен совпадать байт-в-байт с cbackend (internal/backend/cbackend.
// readHelpers, rech_read_int).
func ReadIntFormat(input string) string {
	return fmt.Sprintf("прочитать_целое: \"%s\" не является целым числом", input)
}

// ReadFloatFormat - то же самое для прочитать_число.
func ReadFloatFormat(input string) string {
	return fmt.Sprintf("прочитать_число: \"%s\" не является числом", input)
}

// OpenFailedFormat - сообщение при "открыть", если файл не удалось открыть
// (не существует, нет прав и т. п. - системная ошибка не различается
// дальше, как и везде в этом минимальном срезе). Формат должен совпадать
// байт-в-байт с cbackend (internal/backend/cbackend.fileHelpers,
// rech_open) - тот же принцип, что и у ReadIntFormat/ReadFloatFormat.
func OpenFailedFormat(path string) string {
	return fmt.Sprintf("открыть: не удалось открыть файл \"%s\"", path)
}

// ReadInvalidMax - сообщение при "прочитать", если максимум байт <= 0.
const ReadInvalidMax = "прочитать: максимум байт должен быть положительным числом"

// ReadIOFailed - сообщение при "прочитать", если сам системный вызов
// чтения вернул ошибку - не спутать с концом файла (0 прочитанных байт),
// который не аварийное завершение, а пустая Строка (см. docs/SPEC.md).
const ReadIOFailed = "прочитать: ошибка чтения файла"
