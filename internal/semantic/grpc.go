// gRPC - тонкий слой поверх HTTP/2 (http2.go): length-prefix фрейминг (1
// байт флага сжатия + 4 байта big-endian длины + protobuf-байты, wire-
// формат gRPC) плюс общепринятые заголовок/трейлеры ("content-type:
// application/grpc", "grpc-status"/"grpc-message"), а не новый транспорт -
// само сообщение (Список<Байт>) собирает/разбирает вызывающая программа
// через уже существующий PROTOBUF-формат (protobuf.go), grpc_* здесь не
// знает о protobuf вообще. Unary и все три вида streaming - ОДНИ И ТЕ ЖЕ
// функции: unary - "вызвать по одному разу", streaming - "вызвать в
// цикле", тем же принципом, что и http2_прочитать_данные/http2_отправить_
// данные ниже них.
//
// Метод/статус в HTTP-смысле у gRPC ФИКСИРОВАНЫ (POST, всегда ":status:
// 200" даже при ошибке уровня приложения) - поэтому grpc_открыть_поток не
// принимает метод, а grpc_начать_ответ не принимает код, в отличие от
// http2_открыть_поток/http2_отправить_заголовки - код ошибки уровня gRPC
// идёт ТОЛЬКО через трейлер "grpc-status" (grpc_завершить/grpc_статус).
package semantic

import "github.com/IlyaFeoktistov/Rech/internal/ir"

// registerGRPCFuncs - см. package doc.
func registerGRPCFuncs(funcs map[string]funcSig) {
	streamType := ir.EntityType(http2StreamType)
	byteList := ir.EntityType(byteListType)

	funcs["grpc_открыть_поток"] = funcSig{Params: []ir.Type{ir.TypeHTTP2Session, ir.TypeString}, ReturnType: streamType, HasReturn: true}
	funcs["grpc_начать_ответ"] = funcSig{Params: []ir.Type{streamType}}

	// "grpc_прочитать_сообщение" возвращает Ничто ровно на чистом конце
	// потока (END_STREAM до префикса длины следующего сообщения) - тем же
	// принципом различения "конец" vs "ошибка", что и у http2_прочитать_
	// данные (package doc http2.go) - обрыв ПОСЕРЕДИНЕ префикса/тела это
	// rtmsg.GRPCFramingError, не Ничто.
	funcs["grpc_прочитать_сообщение"] = funcSig{Params: []ir.Type{streamType}, ReturnType: ir.NullableType(byteList), HasReturn: true}
	funcs["grpc_отправить_сообщение"] = funcSig{Params: []ir.Type{streamType, byteList}}

	funcs["grpc_завершить"] = funcSig{Params: []ir.Type{streamType, ir.TypeInt, ir.TypeString}}
	funcs["grpc_статус"] = funcSig{Params: []ir.Type{streamType}, ReturnType: ir.TypeInt, HasReturn: true}
	funcs["grpc_сообщение_ошибки"] = funcSig{Params: []ir.Type{streamType}, ReturnType: ir.TypeString, HasReturn: true}
}

// grpcFuncNames - имена всех grpc_*-функций, упоминающих HTTP2Поток -
// используется analyzeCallExpr тем же принципом, что и http2FuncNames.
var grpcFuncNames = map[string]bool{
	"grpc_открыть_поток":       true,
	"grpc_начать_ответ":        true,
	"grpc_прочитать_сообщение": true,
	"grpc_отправить_сообщение": true,
	"grpc_завершить":           true,
	"grpc_статус":              true,
	"grpc_сообщение_ошибки":    true,
}

// grpcBytesFuncNames - подмножество grpcFuncNames, чья сигнатура ТАКЖЕ
// упоминает "Список<Байт>" - см. registerByteListTypeOnCall (bytesio.go).
var grpcBytesFuncNames = map[string]bool{
	"grpc_прочитать_сообщение": true,
	"grpc_отправить_сообщение": true,
}

// registerGRPCTypesOnCall - резолвит HTTP2Поток (через registerHTTP2TypesOnCall,
// заодно "?Строка") и дополнительно "?Список<Байт>" (результат
// grpc_прочитать_сообщение) - вызывается из analyzeCallExpr при вызове
// любой grpcFuncNames, тем же принципом, что registerHTTP2TypesOnCall.
func registerGRPCTypesOnCall(entities map[string]entitySig, methods map[string]map[string]funcSig, aliases map[string]string, interfaces map[string]interfaceSig) error {
	if err := registerHTTP2TypesOnCall(entities, methods, aliases, interfaces); err != nil {
		return err
	}
	_, err := resolveTypeName(entities, methods, aliases, interfaces, "?"+byteListType)
	return err
}
