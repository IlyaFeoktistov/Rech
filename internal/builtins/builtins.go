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
	// KindFile - непрозрачный дескриптор открытого файла (шаг 8, файловый
	// ввод-вывод) - как и остальные Kind, ничего не знает о конкретном
	// C/Go-представлении (int/HANDLE/*os.File) - это решает каждый
	// потребитель (cbackend/interp) сам.
	KindFile
)

// Func - сигнатура одной встроенной функции. HasReturn различает функцию со
// значением (математика, ввод, "открыть"/"прочитать") от функции-действия
// без результата ("закрыть") - без явного поля ReturnKind == KindInt (его
// нулевое значение) было бы неотличимо от настоящего Целое-результата.
type Func struct {
	Name       string
	Params     []Kind
	ReturnKind Kind
	HasReturn  bool
}

// All - полный список встроенных функций (порядок значим только для
// вывода документации/ошибок, не для логики).
var All = []Func{
	// Математика (обёртки над <math.h>).
	{Name: "корень", Params: []Kind{KindFloat}, ReturnKind: KindFloat, HasReturn: true},
	{Name: "степень", Params: []Kind{KindFloat, KindFloat}, ReturnKind: KindFloat, HasReturn: true},
	{Name: "округлить", Params: []Kind{KindFloat}, ReturnKind: KindInt, HasReturn: true},
	{Name: "потолок", Params: []Kind{KindFloat}, ReturnKind: KindInt, HasReturn: true},
	{Name: "пол", Params: []Kind{KindFloat}, ReturnKind: KindInt, HasReturn: true},
	{Name: "модуль", Params: []Kind{KindFloat}, ReturnKind: KindFloat, HasReturn: true},

	// Ввод консоли (обёртки над <stdio.h> в C, os.Stdin в интерпретаторе).
	{Name: "прочитать_строку", Params: nil, ReturnKind: KindString, HasReturn: true},
	{Name: "прочитать_целое", Params: nil, ReturnKind: KindInt, HasReturn: true},
	{Name: "прочитать_число", Params: nil, ReturnKind: KindFloat, HasReturn: true},

	// Низкоуровневый файловый ввод-вывод (шаг 8, только чтение в этом
	// срезе - запись/дозапись и высокоуровневые обёртки - следующая
	// итерация). "открыть" - только на чтение, ошибка открытия - аварийное
	// завершение (в языке ещё нет исключений, шаг 14). "прочитать" - не
	// более max байт (POSIX read(2), не строка/не до перевода строки) -
	// пустая Строка сигнализирует конец файла, а не ошибку.
	{Name: "открыть", Params: []Kind{KindString}, ReturnKind: KindFile, HasReturn: true},
	{Name: "прочитать", Params: []Kind{KindFile, KindInt}, ReturnKind: KindString, HasReturn: true},
	{Name: "закрыть", Params: []Kind{KindFile}, HasReturn: false},
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
