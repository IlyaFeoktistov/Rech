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
	// KindSocket - непрозрачный дескриптор сетевого сокета (шаг 8, сетевые
	// примитивы) - тот же принцип, что и KindFile.
	KindSocket
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
	{Name: "экспонента", Params: []Kind{KindFloat}, ReturnKind: KindFloat, HasReturn: true},
	{Name: "синус", Params: []Kind{KindFloat}, ReturnKind: KindFloat, HasReturn: true},
	{Name: "косинус", Params: []Kind{KindFloat}, ReturnKind: KindFloat, HasReturn: true},
	{Name: "тангенс", Params: []Kind{KindFloat}, ReturnKind: KindFloat, HasReturn: true},
	{Name: "арксинус", Params: []Kind{KindFloat}, ReturnKind: KindFloat, HasReturn: true},
	{Name: "арккосинус", Params: []Kind{KindFloat}, ReturnKind: KindFloat, HasReturn: true},
	{Name: "арктангенс", Params: []Kind{KindFloat}, ReturnKind: KindFloat, HasReturn: true},
	{Name: "арктангенс2", Params: []Kind{KindFloat, KindFloat}, ReturnKind: KindFloat, HasReturn: true},
	{Name: "логарифм", Params: []Kind{KindFloat}, ReturnKind: KindFloat, HasReturn: true},
	{Name: "логарифм2", Params: []Kind{KindFloat}, ReturnKind: KindFloat, HasReturn: true},
	{Name: "логарифм10", Params: []Kind{KindFloat}, ReturnKind: KindFloat, HasReturn: true},
	{Name: "минимум", Params: []Kind{KindFloat, KindFloat}, ReturnKind: KindFloat, HasReturn: true},
	{Name: "максимум", Params: []Kind{KindFloat, KindFloat}, ReturnKind: KindFloat, HasReturn: true},
	{Name: "пи", Params: nil, ReturnKind: KindFloat, HasReturn: true},

	// Случайные числа (криптографически стойкий источник - arc4random* в
	// cbackend, crypto/rand в интерпретаторе - НЕ math/rand/rand(), см.
	// package doc random.go в обоих backend'ах).
	{Name: "случайное", Params: nil, ReturnKind: KindFloat, HasReturn: true},
	{Name: "случайное_целое", Params: []Kind{KindInt, KindInt}, ReturnKind: KindInt, HasReturn: true},

	// Строки - индексация/длина/поиск по КОДОВЫМ ТОЧКАМ Unicode, не байтам
	// (тем же принципом, что и "Символ" - см. string.go в обоих backend'ах).
	{Name: "длина_строки", Params: []Kind{KindString}, ReturnKind: KindInt, HasReturn: true},
	{Name: "подстрока", Params: []Kind{KindString, KindInt, KindInt}, ReturnKind: KindString, HasReturn: true},
	{Name: "позиция_в_строке", Params: []Kind{KindString, KindString}, ReturnKind: KindInt, HasReturn: true},

	// Ввод консоли (обёртки над <stdio.h> в C, os.Stdin в интерпретаторе).
	{Name: "прочитать_строку", Params: nil, ReturnKind: KindString, HasReturn: true},
	{Name: "прочитать_целое", Params: nil, ReturnKind: KindInt, HasReturn: true},
	{Name: "прочитать_число", Params: nil, ReturnKind: KindFloat, HasReturn: true},

	// Низкоуровневый файловый ввод-вывод (шаг 8). "открыть" - на чтение,
	// "открыть_для_записи" - на запись (существующее содержимое обрезается),
	// "открыть_для_дозаписи" - на запись в конец существующего содержимого;
	// отдельные функции, а не одна с параметром-режимом - ошибка открытия
	// у всех трёх - аварийное завершение, не выброс исключения (открыть
	// несуществующий файл - обычно ошибка программиста в этом простом
	// срезе, не ожидаемый исключительный случай). "прочитать" - не более
	// max байт (не строка/не до перевода
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

	// Сетевые примитивы (шаг 8, единый узел - docs/ROADMAP.md) - байты
	// через TCP/UDP, не HTTP (шаг 9). Одна "сетевой_узел(режим, хост,
	// порт)" вместо трёх отдельных функций (были "слушать_tcp"/
	// "подключиться_tcp"/"подключиться_udp") - хост == "" решает "слушать"
	// (bind+listen для tcp, bind для udp), непустой хост - "подключиться"
	// (connect). Режим - "tcp"/"udp"; неизвестный режим - паника в
	// рантайме (строковый параметр решается только там, не на этапе
	// компиляции, тот же компромисс, что и у остальных "режим строкой" API
	// языка, например БД). "принять" - только для tcp-узла, полученного
	// БЕЗ хоста (слушающего) - один "принять" на каждое входящее
	// соединение, возвращает НОВЫЙ Сокет для этого клиента, или Ничто по
	// истечении таймаута, если он установлен ("установить_таймаут" ниже) -
	// nullable-возврат, поэтому регистрируется отдельно от этой таблицы
	// (см. internal/semantic - registerNetworkFuncs, тем же принципом, что
	// и "получить_переменную_окружения"). "отправить"/"получить" -
	// подключённый сокет (клиентский tcp/udp или результат "принять") -
	// байты без адреса, собеседник уже зафиксирован самим соединением; для
	// udp-узла БЕЗ хоста (слушающего, отвечающего произвольным
	// отправителям) - вместо них "получить_от"/"отправить_в" (recvfrom/
	// sendto с явным адресом, см. internal/semantic - registerNetworkFuncs).
	{Name: "сетевой_узел", Params: []Kind{KindString, KindString, KindInt}, ReturnKind: KindSocket, HasReturn: true},
	{Name: "отправить", Params: []Kind{KindSocket, KindString}, HasReturn: false},
	{Name: "получить", Params: []Kind{KindSocket, KindInt}, ReturnKind: KindString, HasReturn: true},
	{Name: "закрыть_сокет", Params: []Kind{KindSocket}, HasReturn: false},

	// "установить_таймаут(сокет, миллисекунды)" - ограничивает, сколько
	// "получить"/"принять_http" (чтение) и "принять" (на слушающем Сокете)
	// готовы блокироваться на этом сокете, прежде чем вернуть ошибку/Ничто
	// вместо того, чтобы ждать вечно - защита от клиента, который
	// подключился и намеренно/случайно не шлёт данные (иначе с потоком на
	// каждое соединение держал бы этот поток бесконечно). На слушающем
	// Сокете - тот же смысл для "принять": не блокироваться, пока не
	// появится хоть один клиент, а периодически возвращать Ничто, чтобы
	// цикл сервера мог проверить "запрошена_остановка" ниже.
	{Name: "установить_таймаут", Params: []Kind{KindSocket, KindInt}, HasReturn: false},

	// "запрошена_остановка(): Логическое" - истина после того, как процесс
	// получил SIGINT/SIGTERM (Ctrl+C или обычный сигнал остановки от
	// системы управления процессами/оркестратора) - не прерывает
	// выполнение сама по себе, только взводит флаг: цикл сервера сам
	// решает, когда и как безопасно остановиться (обычно - выйти из "пока
	// не запрошена_остановка() { принять... }" и доделать cleanup обычным
	// кодом после цикла, никакой отдельной "магии" остановки в рантайме
	// нет).
	{Name: "запрошена_остановка", Params: nil, ReturnKind: KindBool, HasReturn: true},

	// "дождаться_всех_задач(миллисекунды): Логическое" - блокируется, пока
	// не завершатся ВСЕ когда-либо запущенные ("запустить") задачи
	// программы, включая те, чей результат никогда не был передан
	// "дождаться" (фоновые задачи - поток на HTTP-соединение), или пока не
	// истечёт таймаут - истина означает "все завершились", ложь - "истёк
	// таймаут, что-то ещё работает". Пара с "запрошена_остановка" -
	// типичный сценарий graceful shutdown: выйти из цикла приёма новых
	// соединений по сигналу, затем дождаться уже принятых, прежде чем
	// процесс завершится сам (обычным возвратом из "главная").
	{Name: "дождаться_всех_задач", Params: []Kind{KindInt}, ReturnKind: KindBool, HasReturn: true},

	// "слушать_tcp_защищённо(порт, файл_сертификата, файл_ключа)" -
	// изоляция паники делает сервер устойчивым к плохому запросу, TLS -
	// пригодным для настоящего прода: слушающий Сокет, у которого
	// "принять" сразу выполняет TLS-рукопожатие (TLS 1.2/1.3, через
	// OpenSSL в cbackend/crypto/tls в interp - НЕ через собственную
	// реализацию, в отличие от SHA-256/HMAC выше: настоящий TLS-стек с
	// нуля - риск для безопасности, не место для "не тянуть внешнюю
	// библиотеку"). Только серверная сторона (только "слушать", не
	// "подключиться" с TLS) - HTTP-клиент "запрос" TLS пока не
	// поддерживает. Дальше - ОБЫЧНЫЕ "принять"/"отправить"/"получить"/
	// "закрыть_сокет"/"принять_http"/"отправить_http_ответ" на полученном
	// Сокете - TLS прозрачен для всего вышестоящего кода (см. package doc
	// internal/backend/cbackend/tls.go).
	{Name: "слушать_tcp_защищённо", Params: []Kind{KindInt, KindString, KindString}, ReturnKind: KindSocket, HasReturn: true},

	// Время - текущий момент как unix-метка (секунды с эпохи 1970-01-01
	// UTC). Распаковка в календарные поля - "время_как_дата" (сущность
	// "ДатаВремя", не примитив - зарегистрирована отдельно, см.
	// ReservedEntityNames/ReservedFuncNames и internal/semantic/datetime.go).
	{Name: "время", Params: nil, ReturnKind: KindInt, HasReturn: true},

	// Криптографические примитивы (docs/ROADMAP.md, "Нет крипто-хеш-
	// функции" - пароли в examples/проекты/http_сервер сравнивались как
	// есть). "хеш_sha256"/"подпись_hmac_sha256" возвращают HEX-строку (64
	// символа), не сырые байты дайджеста - Rech "Строка" в cbackend это
	// "const char*" без отдельно хранимой длины, а сырой 32-байтовый
	// дайджест может содержать 0x00 внутри и потерять хвост. "подпись_
	// hmac_sha256(данные, ключ)" - тот же примитив (HMAC-SHA256), которым
	// подписан JWT (алгоритм HS256) - основа для собственных сессионных
	// токенов вместо случайной строки без подписи.
	// "закодировать_base64url"/"декодировать_base64url" - RFC 4648 §5, БЕЗ
	// "="-паддинга (тот же алфавит, что у частей JWT) - только для текста
	// без встроенных нулевых байт (JSON заголовка/тела токена), той же
	// оговоркой, что и у самого HEX-выбора выше.
	{Name: "хеш_sha256", Params: []Kind{KindString}, ReturnKind: KindString, HasReturn: true},
	{Name: "подпись_hmac_sha256", Params: []Kind{KindString, KindString}, ReturnKind: KindString, HasReturn: true},
	{Name: "закодировать_base64url", Params: []Kind{KindString}, ReturnKind: KindString, HasReturn: true},
	{Name: "декодировать_base64url", Params: []Kind{KindString}, ReturnKind: KindString, HasReturn: true},
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

// ReservedFuncNames - зарезервированные имена функций, не входящие в All -
// для них Kind (только примитивы) недостаточен, чтобы описать сигнатуру:
// "запрос" (HTTP-клиент) принимает и возвращает сущности (Запрос/Ответ),
// не примитив; "подключиться_к_бд"/"запросить"/"выполнить"/"закрыть_бд"
// (БД) - Список<Значение>/Список<Словарь<Строка, Значение>>, тоже не
// примитив; "разбить" - Список<Строка> (internal/semantic/stringsplit.go).
// Регистрация самой сигнатуры и реализация - в каждом
// потребителе отдельно (internal/semantic/internal/backend/cbackend/
// internal/interp, см. их package doc), здесь - только имя, для коллизий
// (internal/module.resolveBareName, internal/semantic - объявление
// пользовательской функции/внешней функции с этим именем).
var ReservedFuncNames = map[string]bool{
	"запрос":               true,
	"время_как_дата":       true,
	"дата_как_время":       true,
	"дату_в_строку":        true,
	"принять_http":         true,
	"отправить_http_ответ": true,
	"параметры_пути":       true,
	"разбить":              true,
	"подключиться_к_бд":    true,
	"запросить":            true,
	"выполнить":            true,
	"закрыть_бд":           true,
	"начать_транзакцию":    true,
	"значение_куки":        true,
	"строка_куки":          true,
	"получить_переменную_окружения":   true,
	"получить_от":          true,
	"отправить_в":          true,
	"принять":              true,
	"регулярное":           true,
	"значение_заголовка":   true,
}

// ReservedEntityNames - зарезервированные имена сущностей (Запрос/Ответ,
// шаг 9) - фиксированный набор полей, вводится компилятором автоматически
// при первом использовании (см. internal/semantic.resolveTypeName), нельзя
// объявить свою сущность с этим именем.
var ReservedEntityNames = map[string]bool{
	"Запрос":      true,
	"Ответ":       true,
	"Вызываемый":  true,
	"ДатаВремя":   true,
	"Блокировка":  true,
	"Семафор":     true,
	"Транзакция":  true,
	"ОпцииКуки":   true,
	"Дейтаграмма": true,
	"Совпадение":  true,
	// "Паника" - НЕ настоящая сущность (в отличие от "Исключение" -
	// наследуется/боксируется/несёт поля) - зарезервированное псевдо-имя
	// типа, распознаваемое ТОЛЬКО в позиции "поймать (<имя>: Паника)"
	// (docs/ROADMAP.md, изоляция паники) - ловит "rech_panic"-класс ошибок
	// (выход за границы, несовпадение "как"/рефлексии, "утверждать" и
	// т. п.), а не "выбросить"-исключения. Зарезервировано здесь, чтобы
	// нельзя было объявить СВОЮ сущность с этим именем и получить
	// неоднозначность в "поймать (x: Паника)".
	"Паника": true,
}

// IsReservedName - true для обычной встроенной функции (All) или для имени
// из ReservedFuncNames - общая проверка для internal/module
// (resolveBareName - не требует "использовать") и internal/semantic
// (коллизия имени пользовательской функции/внешней функции).
func IsReservedName(name string) bool {
	if _, ok := ByName(name); ok {
		return true
	}
	return ReservedFuncNames[name]
}
