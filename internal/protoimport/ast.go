package protoimport

// file - результат разбора одного ".proto"-файла (до разрешения ссылок
// типов между сообщениями/импортами - см. resolve.go).
type file struct {
	path     string // путь файла, как передан/разрешён импортом - для сообщений об ошибках и комментариев в сгенерированном коде
	syntax   string // "proto3" - единственная поддержанная (см. parser.go)
	pkg      string
	imports  []importDecl
	messages []*message
	enums    []*enumType
	services []serviceDecl // только для отчёта - не поддержаны (см. generate.go)
}

type importDecl struct {
	path string
	line int
}

type serviceDecl struct {
	name    string
	line    int
	methods []rpcDecl
}

// rpcDecl - "rpc Имя (Запрос) returns (Ответ);" (тело "{ ... }" вместо ";"
// тоже допустимо - только "option ...;" внутри, пропускается целиком,
// generate.go не порождает по нему ничего). reqStreaming/respStreaming -
// "stream" перед типом запроса/ответа - определяет один из четырёх видов
// gRPC-вызова (см. generate.go, rpcKind).
type rpcDecl struct {
	name          string
	reqType       string
	reqStreaming  bool
	respType      string
	respStreaming bool
	line          int
}

// message - "message Имя { ... }", включая вложенные message/enum (плоско
// прибиты к родителю через nested/nestedEnums - собственной вложенной
// области видимости у Rech-сущностей нет, см. generate.go, flattenName).
type message struct {
	name        string
	fields      []*field
	nested      []*message
	nestedEnums []*enumType
	line        int
}

// field - обычное поле сообщения. rawType - как написано в ".proto"
// (может быть квалифицированным именем "пакет.Тип" - см. resolve.go,
// lastSegment). oneof/mapKeyType != "" - явно НЕ поддержанные конструкции
// (см. generate.go) - разобраны (не ломают парсер на реальном файле), но
// генерация по ним отдельно и однозначно отказывает.
type field struct {
	name        string
	rawType     string
	repeated    bool
	number      int64
	line        int
	isOneofPart bool
	oneofName   string
	mapKeyType  string // непусто - поле было "map<K, V> имя = N" (K здесь)
	mapValType  string
}

type enumType struct {
	name   string
	values []enumValue
	line   int
}

type enumValue struct {
	name   string
	number int64
}
