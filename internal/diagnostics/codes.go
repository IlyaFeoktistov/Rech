package diagnostics

import "fmt"

// Коды - по КАТЕГОРИЯМ ошибок, не по каждому отдельному сообщению (как
// E0308 в Rust покрывает любое несовпадение типов, а не отдельный код на
// каждую пару типов) - иначе на ~700 существующих мест ошибок
// (internal/lexer+internal/parser+internal/semantic) понадобилось бы ~700
// кодов, что не несёт пользы (два по-разному сформулированных сообщения
// одной и той же природы искали бы по разным кодам). Диапазоны:
//
//	RECH1xxx - лексер (internal/lexer)
//	RECH2xxx - парсер (internal/parser), по конструкциям грамматики
//	RECH30xx - семантика: разрешение имён
//	RECH31xx - семантика: проверка типов
//	RECH32xx - семантика: сущности/методы/конструкторы
//	RECH33xx - семантика: состояния/typestate
//	RECH34xx - семантика: generics
//	RECH35xx - семантика: исключения (попытаться/поймать/выбросить)
//	RECH36xx - семантика: экспорт/FFI/внешние объявления
//	RECH37xx - семантика: анализ потока управления (вернуть/CFG)
//	RECH38xx - семантика: встроенные типы/методы (БД/HTTP/JSON/...)
//	RECH39xx - семантика: прочее
//	RECH9000 - внутренние ошибки cbackend/toolchain (не код пользователя -
//	           см. package doc Diagnostic.Code про диагностики без кода;
//	           этот код - единственное исключение, обёртка "ошибка
//	           компилятора" вокруг ~60 мест cbackend, не мигрированных на
//	           отдельные категории)
const (
	CodeInternal = "RECH9000"
)

// registry - код -> короткое описание категории (не текст конкретного
// сообщения - см. package doc выше). Единственный источник правды: любой
// код, который встречается в кодовой базе, обязан быть здесь
// зарегистрирован через Register() в файле своей категории
// (internal/parser/codes.go, internal/semantic/codes.go и т.п., вызывается
// из их собственного init()) - иначе codes_test.go/TestAllCodesRegistered
// падает.
var registry = map[string]string{}

// Register добавляет код в реестр и возвращает его же (удобно писать
// const-подобные объявления вида
// var CodeUnexpectedToken = diagnostics.Register("RECH2001", "...")) -
// паникует при повторной регистрации с ДРУГИМ описанием (опечатка/
// копипаста при добавлении нового места ошибки, использующего уже занятый
// код по ошибке); при регистрации того же кода с тем же описанием
// (несколько мест ошибок одной категории) молча идемпотентен.
func Register(code, description string) string {
	if existing, ok := registry[code]; ok && existing != description {
		panic(fmt.Sprintf("diagnostics: код %s уже зарегистрирован с описанием %q, повторная регистрация с %q", code, existing, description))
	}
	registry[code] = description
	return code
}

// Describe возвращает зарегистрированное описание категории кода - "" ,
// false, если код не зарегистрирован (internal/lsp/cmd/rech - для
// справочной информации, не для проверки корректности вывода).
func Describe(code string) (string, bool) {
	d, ok := registry[code]
	return d, ok
}

// Codes возвращает все зарегистрированные коды - используется
// codes_test.go и генератором docs/DIAGNOSTICS.md (Фаза 7).
func Codes() map[string]string {
	out := make(map[string]string, len(registry))
	for k, v := range registry {
		out[k] = v
	}
	return out
}

func init() {
	Register(CodeInternal, "внутренняя ошибка компилятора (генерация C/сборка тулчейном)")
}
