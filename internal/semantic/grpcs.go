// grpcs - TLS+ALPN "h2" поверх уже готового HTTP/2 (http2.go)/gRPC
// (grpc.go) и уже готового TLS-биндинга (tls.go/network.go) - ДВЕ новые
// функции, а не переиспользование существующих "слушать_tcp_защищённо"/
// "запрос": ALPN у gRPC обязан согласовать именно "h2" (не "http/1.1"), а
// делать это на ОБЩЕМ SSL_CTX с обычным HTTPS-сервером/клиентом сломало бы
// их (см. package doc internal/backend/cbackend/grpcs.go).
//
// Серверная сторона САМОЙ gRPC-сессии - уже существующая "http2_принять_
// сессию" (TLS полностью прозрачна для неё, тем же принципом, что и у
// остального рантайма, см. package doc tls.go) - "слушать_grpcs" только
// заводит TLS-слушающий "Сокет" с нужным ALPN, никакой отдельной "grpcs_
// принять_сессию" не нужно.
package semantic

import "github.com/IlyaFeoktistov/Rech/internal/ir"

func registerGRPCSFuncs(funcs map[string]funcSig) {
	funcs["слушать_grpcs"] = funcSig{Params: []ir.Type{ir.TypeInt, ir.TypeString, ir.TypeString}, ReturnType: ir.TypeSocket, HasReturn: true}
	funcs["новая_grpcs_сессия"] = funcSig{Params: []ir.Type{ir.TypeString, ir.TypeInt}, ReturnType: ir.TypeHTTP2Session, HasReturn: true}
}
