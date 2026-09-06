// Бинарно-безопасный HTTP-запрос/ответ - см. package doc internal/
// semantic/httpbytes.go для полного описания решения.
package interp

// httpBytesRequestType/httpBytesResponseType - см. internal/semantic/
// httpbytes.go, package doc queryParamsDictType (cbackend/httpserver.go)
// для того же принципа дублирования имени константой в нескольких
// пакетах.
const (
	httpBytesRequestType  = "ЗапросБайты"
	httpBytesResponseType = "ОтветБайты"
)
