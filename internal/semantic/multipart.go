// Разбор multipart/form-data (RFC 2046/7578) - "разобрать_multipart" -
// последний кусок поддержки файлов по HTTP (docs/ROADMAP.md): реальная
// HTML-форма с "<input type=file>" кодирует загружаемый файл именно так
// (несколько частей формы, разделённых границей - "boundary" из заголовка
// Content-Type, - каждая со своими заголовками Content-Disposition/
// Content-Type и телом). Работает над уже бинарно-безопасным телом
// (Список<Байт>, принять_http_байты/httpbytes.go) - тело части формы
// (например, содержимое файла) может быть произвольным бинарным
// содержимым, "Строка" для него не подошла бы по той же причине, что и
// для тела всего запроса.
//
// "ЧастьФормы" - лениво регистрируемая сущность (тем же принципом, что
// "Совпадение"/regex.go): "имя" (значение "name=" из Content-Disposition),
// "имя_файла" (значение "filename=", пустая Строка - обычное текстовое
// поле формы, не файл), "тип_содержимого" (Content-Type ЭТОЙ части,
// пустая Строка - не указан), "данные" (тело части, Список<Байт>).
//
// Курируемый минимум, не полный RFC-парсер: не поддержаны вложенные
// multipart (multipart/mixed внутри multipart/form-data), Content-
// Transfer-Encoding (base64/quoted-printable - реальные браузеры/curl не
// используют его для form-data), несколько значений одного "name" как
// массив - вызывающий Rech-код сам решает, что делать с несколькими
// частями одного имени (Список<ЧастьФормы> несёт их все по порядку).
package semantic

import "github.com/IlyaFeoktistov/Rech/internal/ir"

const (
	formPartType        = "ЧастьФормы"
	multipartResultType = "Список<ЧастьФормы>"
)

// formPartFields - поля "ЧастьФормы".
var formPartFields = []ir.Field{
	{Name: "имя", Type: ir.TypeString},
	{Name: "имя_файла", Type: ir.TypeString},
	{Name: "тип_содержимого", Type: ir.TypeString},
	{Name: "данные", Type: ir.EntityType(byteListType)},
}

// registerMultipartFunc - только сигнатура функции, без регистрации самой
// сущности/generic-коллекций - те лениво регистрируются при первом
// реальном вызове (registerMultipartTypesOnCall), тем же принципом, что и
// registerRegexFunc.
func registerMultipartFunc(funcs map[string]funcSig) {
	funcs["разобрать_multipart"] = funcSig{
		Params:     []ir.Type{ir.EntityType(byteListType), ir.TypeString},
		ReturnType: ir.EntityType(multipartResultType), HasReturn: true,
	}
}

// registerMultipartTypesOnCall - вызывается из analyzeCallExpr при вызове
// "разобрать_multipart" - резолвит "ЧастьФормы" (которая сама резолвит
// byteListType раньше себя, см. её case в resolveTypeName) и
// multipartResultType, тем же порядком, что registerRegexTypesOnCall
// резолвит "Совпадение" и regexResultType.
func registerMultipartTypesOnCall(entities map[string]entitySig, methods map[string]map[string]funcSig, aliases map[string]string, interfaces map[string]interfaceSig, enums map[string]map[string]int64) error {
	if _, err := resolveTypeName(entities, methods, aliases, interfaces, enums, formPartType); err != nil {
		return err
	}
	_, err := resolveTypeName(entities, methods, aliases, interfaces, enums, multipartResultType)
	return err
}
