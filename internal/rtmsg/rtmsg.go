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

// DictKeyNotFound - сообщение при "получить" по отсутствующему ключу
// Словаря.
func DictKeyNotFound(typeName string) string {
	return fmt.Sprintf("ключ не найден в %s", typeName)
}

// DivisionByZero - сообщение при делении Целое на ноль. Только для
// целочисленного деления - деление Число (float64) на 0.0 по IEEE 754 даёт
// +-Inf/NaN, не аварийное завершение, и одинаково ведёт себя в C и в Go, не
// нуждается в отдельной проверке.
const DivisionByZero = "деление на ноль"

// NullAccess - сообщение при развороте "?Тип" в Тип (явный каст "(Тип)
// значение" или неявное использование доказанно непустого значения),
// значение которого на самом деле - Ничто.
const NullAccess = "обращение к пустому значению (Ничто)"

// ReadEOF - сообщение при попытке прочитать_строку/прочитать_целое/
// прочитать_число, когда стандартный ввод уже закончился.
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

// OpenFailedFormat - сообщение при "открыть"/"открыть_для_записи"/
// "открыть_для_дозаписи", если файл не удалось открыть (не существует, нет
// прав и т. п. - системная ошибка не различается дальше, как и везде в
// этом минимальном срезе); op - имя конкретной вызванной функции (само
// сообщение начинается с него, как и у ReadIntFormat/ReadFloatFormat).
// Формат должен совпадать байт-в-байт с cbackend (internal/backend/
// cbackend.fileHelpers, rech_open_mode).
func OpenFailedFormat(op, path string) string {
	return fmt.Sprintf("%s: не удалось открыть файл \"%s\"", op, path)
}

// ReadInvalidMax - сообщение при "прочитать", если максимум байт <= 0.
const ReadInvalidMax = "прочитать: максимум байт должен быть положительным числом"

// CastStrToIntFailedFormat - сообщение при явном касте "Строка" ->
// "Целое"/"Байт" (`(Тип) значение`), если строка - не целое число.
// typeName - имя целевого типа ("Целое"/"Байт"), как оно написано в касте.
// Формат должен совпадать байт-в-байт с cbackend (internal/backend/
// cbackend/cast.go, rech_cast_str_to_int).
func CastStrToIntFailedFormat(typeName, input string) string {
	return fmt.Sprintf("каст (%s): \"%s\" не является целым числом", typeName, input)
}

// CastStrToFloatFailedFormat - то же самое для касты в "Действительное"/
// "Дробное".
func CastStrToFloatFailedFormat(typeName, input string) string {
	return fmt.Sprintf("каст (%s): \"%s\" не является числом", typeName, input)
}

// CastStrToCharFailedFormat - сообщение при касте "Строка" -> "Символ",
// если строка не состоит ровно из одной кодовой точки Unicode.
func CastStrToCharFailedFormat(input string) string {
	return fmt.Sprintf("каст (Символ): строка \"%s\" не состоит ровно из одного символа", input)
}

// CastStrToBoolFailedFormat - сообщение при касте "Строка" -> "Логическое",
// если строка - не буквально "истина" или "ложь" (тот же текст, что и вывод
// "вывести"/"показать" для Логическое).
func CastStrToBoolFailedFormat(input string) string {
	return fmt.Sprintf("каст (Логическое): \"%s\" не является \"истина\" или \"ложь\"", input)
}

// ReadIOFailed - сообщение при "прочитать", если сам системный вызов
// чтения вернул ошибку - не спутать с концом файла (0 прочитанных байт),
// который не аварийное завершение, а пустая Строка (см. docs/SPEC.md).
const ReadIOFailed = "прочитать: ошибка чтения файла"

// WriteIOFailed - сообщение при "записать", если запись в файл не удалась
// или прошла не полностью (диск заполнен и т. п.).
const WriteIOFailed = "записать: ошибка записи в файл"

// Сетевые примитивы - сокеты, байты через TCP/UDP. Текст должен
// совпадать байт-в-байт между cbackend (internal/backend/cbackend.
// socketHelpers) и internal/interp - тот же принцип, что и у файлового
// ввода-вывода выше.
const (
	SocketCreateFailed   = "сетевой_узел: не удалось создать сокет"
	SocketBindFailed     = "сетевой_узел: не удалось привязаться к порту"
	SocketListenFailed   = "сетевой_узел: не удалось начать прослушивание"
	SocketAcceptFailed   = "принять: не удалось принять соединение"
	SocketResolveFailed  = "сетевой_узел: не удалось разрешить адрес"
	SocketConnectCreate  = "сетевой_узел: не удалось создать сокет"
	SocketConnectFailed  = "сетевой_узел: не удалось подключиться"
	SocketSendFailed     = "отправить: ошибка отправки данных"
	SocketRecvFailed     = "получить: ошибка получения данных"
	SocketRecvInvalidMax = "получить: максимум байт должен быть положительным числом"
	// SocketUnknownMode - "сетевой_узел" с режимом, отличным от "tcp"/
	// "udp" - строковый параметр решается только в рантайме, не на этапе
	// компиляции (тот же компромисс, что и у остальных "режим строкой"
	// API языка).
	SocketUnknownMode = "сетевой_узел: неизвестный режим сети (допустимы tcp/udp)"
	// SocketWrongKind - вызов "отправить"/"получить"/"закрыть_сокет" на
	// значении "сетевой_узел(режим, \"\", порт)" (слушающий, не
	// подключённый сокет) или "принять"/"получить_от"/"отправить_в" на
	// значении не того вида (слушающий tcp против udp против подключённого) -
	// ошибка использования, не ввода-вывода. cbackend полагается на саму ОС
	// (send/recv/accept/recvfrom/sendto на "неправильном" дескрипторе
	// вернут системную ошибку, которая уже покрыта сообщениями выше);
	// интерпретатор различает это на уровне Go-типа (net.Listener/net.Conn/
	// net.PacketConn) раньше, поэтому нужен отдельный текст - единственная
	// точка расхождения текста ошибки между backend'ами в этом срезе,
	// задокументированная явно, а не случайно пропущенная.
	SocketWrongKind = "сокет: операция не подходит для этого вида сокета (слушающий/подключённый/udp-узел)"
	// TLSCertLoadFailed - "слушать_tcp_защищённо" не смог загрузить пару
	// сертификат/приватный ключ (файл не найден, не PEM, ключ не подходит
	// сертификату) - ошибка проверяется один раз при создании слушателя, не
	// при каждом "принять".
	TLSCertLoadFailed = "слушать_tcp_защищённо: не удалось загрузить сертификат/ключ"
	// TLSHandshakeFailed - TLS-рукопожатие после "принять" на защищённом
	// слушателе не завершилось успешно (клиент не поддерживает TLS,
	// оборвал соединение раньше времени и т. п.) - в cbackend проверяется
	// сразу внутри rech_accept (см. tls.go), в interp - лениво при первом
	// Read/Write самим net/tls (тот же принцип отложенного рукопожатия, что
	// и у стандартного net/http Go).
	TLSHandshakeFailed = "принять: не удалось выполнить TLS-рукопожатие"
)

// HTTPRecvFailed - сообщение при "запрос" (шаг 9, HTTP-клиент), если чтение
// ответа с сокета завершилось ошибкой (соединение обрывается, не просто
// закрывается нормально - последнее и есть штатный конец ответа, не
// ошибка).
const HTTPRecvFailed = "запрос: ошибка получения ответа"

// HTTPServerRecvFailed - то же самое, что HTTPRecvFailed, но для
// "принять_http" (минимальный HTTP-сервер) - чтение входящего запроса с
// уже принятого соединения завершилось ошибкой.
const HTTPServerRecvFailed = "принять_http: ошибка получения запроса"

// JSONParseError - сообщение при "разобрать JSON", если текст - не
// корректный JSON (синтаксическая ошибка), без точной позиции: хендрольный
// C-парсер (cbackend) и encoding/json (internal/interp) сообщают о
// синтаксических ошибках совершенно по-разному внутри себя - единый текст
// без деталей парсера - единственный способ сохранить сообщение байт-в-байт
// одинаковым между backend'ами (тот же принцип, что и у остальных сообщений
// этого пакета).
const JSONParseError = "разобрать JSON: текст не является корректным JSON"

// JSONFieldMissing - сообщение, когда у целевой сущности/структуры есть
// поле field, а в разобранном JSON-объекте нет ключа с таким именем.
func JSONFieldMissing(field string) string {
	return fmt.Sprintf("разобрать JSON: отсутствует поле %q", field)
}

// JSONValueMismatch - сообщение, когда JSON-значение (корень разбора,
// значение поля сущности или элемент Список<T>) не того вида, что ожидает
// целевой тип (например, ожидалось число, получена строка). Без имени
// поля/индекса - единое сообщение для любой глубины несовпадения нужно,
// чтобы текст оставался байт-в-байт одинаковым между hand-rolled парсером
// cbackend (json_extract.go, сообщение - C-литерал, зашитый во время
// кодогенерации) и internal/interp/json.go (то же сообщение во время
// исполнения) без рантайм-форматирования на стороне C.
const JSONValueMismatch = "разобрать JSON: значение имеет несовместимый тип"

// XMLParseError/XMLFieldMissing/XMLValueMismatch - те же роли, что
// JSONParseError/JSONFieldMissing/JSONValueMismatch выше, для "разобрать
// XML"/"сериализовать XML" - отдельные сообщения (не общие с JSON), чтобы
// текст ошибки указывал на реально использованный формат.
const XMLParseError = "разобрать XML: текст не является корректным XML"

func XMLFieldMissing(field string) string {
	return fmt.Sprintf("разобрать XML: отсутствует поле %q", field)
}

// XMLFieldDuplicated - сообщение, когда у скалярного (не Список<T>) поля
// нашлось больше одного XML-элемента с совпадающим именем - в отличие от
// JSON (объект физически не может иметь два одинаковых ключа в этом
// разборе), повторяющиеся XML-теги синтаксически допустимы, поэтому это
// отдельная, специфичная для XML ошибка.
func XMLFieldDuplicated(field string) string {
	return fmt.Sprintf("разобрать XML: поле %q повторяется более одного раза", field)
}

const XMLValueMismatch = "разобрать XML: значение имеет несовместимый тип"

// YAMLParseError/YAMLFieldMissing/YAMLValueMismatch - те же роли, что
// JSONParseError/JSONFieldMissing/JSONValueMismatch выше, для "разобрать
// YAML"/"сериализовать YAML" (блочный YAML - отступами, без фигурных
// скобок/кавычек у большинства значений) - отдельные сообщения, чтобы
// текст ошибки указывал на реально использованный формат.
const YAMLParseError = "разобрать YAML: текст не является корректным YAML"

func YAMLFieldMissing(field string) string {
	return fmt.Sprintf("разобрать YAML: отсутствует поле %q", field)
}

const YAMLValueMismatch = "разобрать YAML: значение имеет несовместимый тип"

// UncaughtExceptionFormat - сообщение при "выбросить", которое не поймал ни
// один охватывающий "попытаться"/"поймать" - typeName - фактическая
// сущность выброшенного значения (не обязательно "Исключение" само по
// себе - может быть любой наследник), message - её поле "сообщение".
// Формат должен совпадать байт-в-байт между cbackend (internal/backend/
// cbackend/exceptions.go, rech_throw) и internal/interp.
func UncaughtExceptionFormat(typeName, message string) string {
	return fmt.Sprintf("необработанное исключение %s: %s", typeName, message)
}

// ArgIndexOutOfRangeFormat - сообщение при "аргумент", если индекс вне
// диапазона [0, количество_аргументов()) - тот же принцип, что и у
// IndexOutOfRange для встроенных коллекций, но с самим индексом в
// сообщении (у коллекций - только имя типа, здесь заведомо интереснее
// увидеть, какой именно индекс запрошен).
func ArgIndexOutOfRangeFormat(index int64) string {
	return fmt.Sprintf("аргумент: индекс %d вне диапазона", index)
}

// TraceHeader - строка-заголовок перед стеком вызовов, который печатается
// вслед за сообщением об аварийном завершении/необработанном исключении.
// Формат должен совпадать байт-в-байт между cbackend (internal/backend/
// cbackend/trace.go) и internal/interp (internal/interp/trace.go).
const TraceHeader = "стек вызовов:"

// TraceFrameFormat - printf/fmt-шаблон одной строки стека вызовов: имя
// функции/метода ("Тип.метод" у методов) и номер строки, на которой
// выполнение этого уровня либо само запаниковало, либо вызвало следующий,
// более глубокий уровень. cbackend встраивает этот же текст буквально в
// сгенерированный C (rech_trace_print) вместо повторного вызова
// TraceFrameLine - оба места должны остаться идентичны, если формат
// изменится.
const TraceFrameFormat = "  в %s, строка %d"

// TraceFrameLine форматирует одну строку стека вызовов (см. TraceFrameFormat).
func TraceFrameLine(name string, line int) string {
	return fmt.Sprintf(TraceFrameFormat, name, line)
}

// TraceTruncated - строка, добавляемая в конец стека вызовов cbackend,
// если глубина рекурсии превысила фиксированный размер массива кадров
// (см. internal/backend/cbackend/trace.go) - интерпретатор такого предела
// не имеет (обычный растущий срез Go), поэтому в этом (редком, глубокая
// рекурсия) случае вывод двух backend'ов может разойтись - задокументированное
// исключение из правила "побайтовое совпадение".
const TraceTruncated = "  ... (глубже стек не показан)"

// StateNotEstablishedTemplate/StateNotEstablishedFormat - сообщение
// перехватываемого "Исключение", брошенного рантайм-проверкой typestate
// (internal/backend/cbackend/typestate.go, internal/interp/eval.go), когда
// established на этапе компиляции не смог доказать переход значения в
// требуемое состояние параметра, объявленного производным типом состояния
// (docs/SPEC.md, "Состояния") - typeName/state известны уже на этапе
// компиляции (тип параметра/имя состояния), поэтому cbackend встраивает
// готовую строку как есть, не через snprintf времени исполнения.
const StateNotEstablishedTemplate = "объект типа \"%s\" не находится в требуемом состоянии \"%s\""

func StateNotEstablishedFormat(typeName, state string) string {
	return fmt.Sprintf(StateNotEstablishedTemplate, typeName, state)
}

// RandomIntInvalidRange - "случайное_целое(мин, макс)" с мин > макс -
// пустой диапазон, аварийное завершение (тем же принципом, что и
// ReadInvalidMax - ошибка программиста, не ожидаемый исключительный
// случай).
const RandomIntInvalidRange = "случайное_целое: минимум должен быть не больше максимума"

// SplitEmptySeparator - "разбить(строка, разделитель)" с пустым
// разделителем - неоднозначно (границы по байтам? по code point'ам?),
// аварийное завершение вместо молчаливого выбора одной интерпретации, тем
// же принципом, что и ReadInvalidMax/RandomIntInvalidRange выше.
const SplitEmptySeparator = "разбить: разделитель не может быть пустой строкой"
