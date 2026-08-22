// Package builtins перечисляет функции, встроенные в рантайм Rech, а не
// объявленные в исходнике программы (шаг 8, docs/ROADMAP.md - ввод консоли
// и базовая математика, без ожидания общего FFI-механизма шага 7: и
// математика, и стандартный ввод доступны C-рантайму программы уже сейчас,
// как rech_panic/rech_int_div).
//
// Три независимых потребителя (cbackend - генерация вызова в C,
// internal/semantic - проверка сигнатуры вызова на IR-пути, internal/interp
// - исполнение) должны знать один и тот же список имён/сигнатур - отсюда
// пакет без зависимостей (как internal/rtmsg для текста сообщений), а не
// три отдельных списка. Каждый потребитель сам решает, как ИСПОЛНИТЬ
// вызов (C-код против Go-функции) - здесь только имена и типы параметров/
// результата, этого достаточно для регистрации сигнатуры и для проверки
// коллизии с именем, объявленным в самой программе.
package builtins

// Kind - тип параметра/результата встроенной функции: только примитивы -
// ни одна встроенная функция этого шага не работает с сущностями.
type Kind int

const (
	KindInt Kind = iota
	KindFloat
	KindString
)

// Func - сигнатура одной встроенной функции.
type Func struct {
	Name       string
	Params     []Kind
	ReturnKind Kind
}

// All - полный список встроенных функций (порядок значим только для
// вывода документации/ошибок, не для логики).
var All = []Func{
	// Математика (обёртки над <math.h>).
	{Name: "корень", Params: []Kind{KindFloat}, ReturnKind: KindFloat},
	{Name: "степень", Params: []Kind{KindFloat, KindFloat}, ReturnKind: KindFloat},
	{Name: "округлить", Params: []Kind{KindFloat}, ReturnKind: KindInt},
	{Name: "потолок", Params: []Kind{KindFloat}, ReturnKind: KindInt},
	{Name: "пол", Params: []Kind{KindFloat}, ReturnKind: KindInt},
	{Name: "модуль", Params: []Kind{KindFloat}, ReturnKind: KindFloat},

	// Ввод консоли (обёртки над <stdio.h> в C, os.Stdin в интерпретаторе).
	{Name: "прочитатьСтроку", Params: nil, ReturnKind: KindString},
	{Name: "прочитатьЦелое", Params: nil, ReturnKind: KindInt},
	{Name: "прочитатьЧисло", Params: nil, ReturnKind: KindFloat},
}

// ByName ищет встроенную функцию по имени.
func ByName(name string) (Func, bool) {
	for _, f := range All {
		if f.Name == name {
			return f, true
		}
	}
	return Func{}, false
}
