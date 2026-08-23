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
	KindBool
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

	// Низкоуровневый файловый ввод-вывод (шаг 8). "открыть" - на чтение,
	// "открыть_для_записи" - на запись (существующее содержимое обрезается),
	// "открыть_для_дозаписи" - на запись в конец существующего содержимого;
	// отдельные функции, а не одна с параметром-режимом - ошибка открытия
	// у всех трёх - аварийное завершение (в языке ещё нет исключений, шаг
	// 14). "прочитать" - не более max байт (не строка/не до перевода
	// строки) - пустая Строка сигнализирует конец файла, а не ошибку.
	// "записать" - записывает Строку целиком, без результата. Построчное
	// чтение - не отдельная функция, а поддержка "Файл" в "для X в Y"
	// (internal/backend/cbackend.generateStmt/internal/interp - см.
	// rech_read_line_from_file).
	{Name: "открыть", Params: []Kind{KindString}, ReturnKind: KindFile, HasReturn: true},
	{Name: "открыть_для_записи", Params: []Kind{KindString}, ReturnKind: KindFile, HasReturn: true},
	{Name: "открыть_для_дозаписи", Params: []Kind{KindString}, ReturnKind: KindFile, HasReturn: true},
	{Name: "прочитать", Params: []Kind{KindFile, KindInt}, ReturnKind: KindString, HasReturn: true},
	{Name: "записать", Params: []Kind{KindFile, KindString}, HasReturn: false},
	{Name: "закрыть", Params: []Kind{KindFile}, HasReturn: false},

	// Высокоуровневые обёртки без ручного открыть/закрыть (шаг 8,
	// "ещё не сделано" в первом срезе файлового ввода-вывода) - типичный
	// сценарий "прочитать/записать файл целиком одной функцией" не должен
	// требовать явного открыть/прочитать в цикле/закрыть. Построчный обход
	// уже покрыт "для X в открыть(путь)" (потоковое чтение) - отдельной
	// высокоуровневой функции для него не нужно.
	{Name: "прочитать_файл", Params: []Kind{KindString}, ReturnKind: KindString, HasReturn: true},
	{Name: "записать_в_файл", Params: []Kind{KindString, KindString}, HasReturn: false},
	{Name: "дописать_в_файл", Params: []Kind{KindString, KindString}, HasReturn: false},

	// Стандартные потоки процесса - тот же тип "Файл", что и у обычных
	// открытых файлов: работают с "прочитать"/"записать"/"для X в Y" точно
	// так же, без отдельного механизма. Позволяют писать типичные Unix-
	// утилиты вида "прочитать построчно стандартный_ввод(), отфильтровать,
	// записать в стандартный_вывод()" - перенаправление ("|", ">") делает
	// сам вызывающий шелл, Rech-программе достаточно знать о трёх готовых
	// потоках, а не о механизме перенаправления.
	{Name: "стандартный_ввод", Params: nil, ReturnKind: KindFile, HasReturn: true},
	{Name: "стандартный_вывод", Params: nil, ReturnKind: KindFile, HasReturn: true},
	{Name: "стандартный_вывод_ошибок", Params: nil, ReturnKind: KindFile, HasReturn: true},

	// Аргументы командной строки, переданные самому собранному бинарнику
	// (argv, без argv[0] - имени самой программы) - без этого нельзя
	// написать типичную Unix-утилиту вида "mygrep <шаблон>". Индексация с
	// 0, как и у "получить" встроенных коллекций. "rech test" (интерпретатор)
	// не имеет способа передать программе свои аргументы (аргументы после
	// "rech test <файл>" - аргументы самого rech, не интерпретируемой
	// программы) - там "количество_аргументов" всегда 0, "аргумент" всегда
	// паникует диапазоном.
	{Name: "количество_аргументов", Params: nil, ReturnKind: KindInt, HasReturn: true},
	{Name: "аргумент", Params: []Kind{KindInt}, ReturnKind: KindString, HasReturn: true},

	// Минимальная строковая операция, без которой типичный текстовый
	// фильтр (например, grep-подобная утилита поверх стандартных потоков
	// выше) невозможен вообще - "содержит(строка, образец)": есть ли
	// образец где-то внутри строки.
	{Name: "содержит", Params: []Kind{KindString, KindString}, ReturnKind: KindBool, HasReturn: true},
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
