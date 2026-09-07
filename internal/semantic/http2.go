// HTTP/2 - транспортный слой поверх уже существующего "Сокет"
// (сетевой_узел/принять) - минимальный, симметричный клиент/сервер API,
// достаточный для того, чтобы gRPC (следующий шаг) свёлся к простому
// фреймингу поверх него (5-байтный префикс длины + protobuf в теле,
// grpc-status/grpc-message - через "http2_отправить_трейлер"), без
// экспонирования самого HTTP/2-фрейминга в Rech.
//
// Один новый непрозрачный тип - "HTTP2Сессия" (ir.KindHTTP2Session,
// internal/ir/type.go) - активное HTTP/2-соединение, клиентское или
// серверное, поверх уже принятого/установленного "Сокет". Один RPC/запрос
// внутри сессии - НЕ отдельный непрозрачный тип, а обычная (лениво
// регистрируемая, тем же принципом, что "ЗапросБайты"/"ОтветБайты",
// httpbytes.go) структура "HTTP2Поток" - у HTTP/2 одна сессия
// мультиплексирует много потоков, поэтому непрозрачна именно сессия, а
// поток - обычные данные (номер потока внутри неё), с которыми работают
// свободные функции ниже.
package semantic

import "github.com/IlyaFeoktistov/Rech/internal/ir"

// http2StreamType - см. package doc.
const http2StreamType = "HTTP2Поток"

// http2StreamFields - "сессия" (та, что вернула http2_принять_сессию/
// новая_http2_сессия) + "ид" (номер потока внутри неё, назначенный
// http2_принять_поток/http2_открыть_поток) - вместе однозначно
// идентифицируют один запрос/ответ для остальных http2_*-функций.
var http2StreamFields = []ir.Field{
	{Name: "сессия", Type: ir.TypeHTTP2Session},
	{Name: "ид", Type: ir.TypeInt},
}

// registerHTTP2Funcs - только сигнатуры функций (funcs) - сама сущность
// "HTTP2Поток" регистрируется лениво, при первом реальном вызове одной из
// них (registerHTTP2TypesOnCall, analyzeCallExpr) - тем же принципом, что и
// registerHTTPBytesFuncs/httpbytes.go.
func registerHTTP2Funcs(funcs map[string]funcSig) {
	streamType := ir.EntityType(http2StreamType)
	byteList := ir.EntityType(byteListType)

	funcs["http2_принять_сессию"] = funcSig{Params: []ir.Type{ir.TypeSocket}, ReturnType: ir.TypeHTTP2Session, HasReturn: true}
	funcs["новая_http2_сессия"] = funcSig{Params: []ir.Type{ir.TypeString, ir.TypeInt}, ReturnType: ir.TypeHTTP2Session, HasReturn: true}
	funcs["http2_закрыть_сессию"] = funcSig{Params: []ir.Type{ir.TypeHTTP2Session}}

	funcs["http2_принять_поток"] = funcSig{Params: []ir.Type{ir.TypeHTTP2Session}, ReturnType: streamType, HasReturn: true}
	funcs["http2_открыть_поток"] = funcSig{Params: []ir.Type{ir.TypeHTTP2Session, ir.TypeString, ir.TypeString}, ReturnType: streamType, HasReturn: true}

	funcs["http2_путь"] = funcSig{Params: []ir.Type{streamType}, ReturnType: ir.TypeString, HasReturn: true}
	funcs["http2_метод"] = funcSig{Params: []ir.Type{streamType}, ReturnType: ir.TypeString, HasReturn: true}
	funcs["http2_статус"] = funcSig{Params: []ir.Type{streamType}, ReturnType: ir.TypeInt, HasReturn: true}
	funcs["http2_заголовок"] = funcSig{Params: []ir.Type{streamType, ir.TypeString}, ReturnType: ir.NullableType(ir.TypeString), HasReturn: true}

	// "http2_прочитать_данные" - тем же принципом, что "прочитать(файл,
	// максимум)" (шаг 8), не "прочитать тело целиком": HTTP/2-поток может
	// нести МНОГО отдельных gRPC-сообщений одно за другим (streaming,
	// следующий шаг) - чтение "всё, что есть, до конца потока" сделало бы
	// server-streaming/bidi невозможными (сервер обязан отвечать ПОКА
	// клиент ещё пишет, а не только после того, как клиент закончит
	// писать). Пустой результат - конец потока (END_STREAM), не 0 доступных
	// сейчас байт - вызывающая сторона отличает их так же, как и у
	// "прочитать" (внутренняя блокировка до появления хотя бы одного байта
	// ИЛИ конца потока).
	funcs["http2_прочитать_данные"] = funcSig{Params: []ir.Type{streamType, ir.TypeInt}, ReturnType: byteList, HasReturn: true}

	// "http2_прочитать_всё" - ждёт END_STREAM и отдаёт тело целиком, в
	// отличие от "http2_прочитать_данные" выше - удобно, когда сторона
	// гарантированно шлёт РОВНО ОДНО сообщение за весь поток (как в
	// echo-примере), но НЕ годится для gRPC server-streaming/bidi, где один
	// поток несёт несколько сообщений подряд - там нужен именно цикл на
	// "http2_прочитать_данные" с известной длиной каждого сообщения.
	funcs["http2_прочитать_всё"] = funcSig{Params: []ir.Type{streamType}, ReturnType: byteList, HasReturn: true}
	funcs["http2_отправить_заголовки"] = funcSig{Params: []ir.Type{streamType, ir.TypeInt}}
	funcs["http2_отправить_данные"] = funcSig{Params: []ir.Type{streamType, byteList}}
	funcs["http2_отправить_трейлер"] = funcSig{Params: []ir.Type{streamType, ir.TypeString, ir.TypeString}}
	funcs["http2_завершить_поток"] = funcSig{Params: []ir.Type{streamType}}
}

// http2FuncNames - имена всех http2_*-функций, чья сигнатура упоминает
// "HTTP2Поток" (параметром или результатом) - используется
// analyzeCallExpr для решения, нужно ли звать registerHTTP2TypesOnCall
// (тем же принципом lookup-таблицы, что dbFuncNames/bytesIOFuncNames).
// "http2_принять_сессию"/"новая_http2_сессия"/"http2_закрыть_сессию" не
// входят - они упоминают только "HTTP2Сессия" (примитив, не нуждается в
// ленивой регистрации).
var http2FuncNames = map[string]bool{
	"http2_принять_поток":       true,
	"http2_открыть_поток":       true,
	"http2_путь":                true,
	"http2_метод":               true,
	"http2_статус":              true,
	"http2_заголовок":           true,
	"http2_прочитать_данные":    true,
	"http2_прочитать_всё":       true,
	"http2_отправить_заголовки": true,
	"http2_отправить_данные":    true,
	"http2_отправить_трейлер":   true,
	"http2_завершить_поток":     true,
}

// http2BytesFuncNames - подмножество http2FuncNames, чья сигнатура ТАКЖЕ
// упоминает "Список<Байт>" - см. registerByteListTypeOnCall (bytesio.go).
var http2BytesFuncNames = map[string]bool{
	"http2_прочитать_данные": true,
	"http2_прочитать_всё":    true,
	"http2_отправить_данные": true,
}

// registerHTTP2TypesOnCall резолвит "HTTP2Поток" (idempotent, как и
// resolveTypeName вообще) - вызывается из analyzeCallExpr при вызове любой
// http2FuncNames. Заодно резолвит "?Строка" БЕЗУСЛОВНО (не только когда
// реально вызвана именно "http2_заголовок") - C-рантайм HTTP/2 (http2.go,
// http2SessionOpsCode) эмитируется ОДНИМ неделимым блоком на всю тему сразу
// (в отличие от JSON/XML/YAML, где каждая функция получает СВОЮ generated-
// функцию) - "rech_http2_header" внутри него ссылается на обёртку "?Строка"
// всегда, независимо от того, какую именно http2_*-функцию вызвала
// программа - без этой безусловной регистрации сборка настоящим cc падала
// бы на "unknown type name", если программа использует HTTP/2, но ни разу
// не вызывает именно "http2_заголовок" (найдено живой проверкой).
//
// ТЕМ ЖЕ путём резолвит "?Сокет" - ЖИВОЙ НАЙДЕННЫЙ БАГ (Шаг 96/97): любая
// http2_*/grpc_*-функция (не только http2_принять_сессию/новая_http2_
// сессия) заставляет ir_generator.go включить usesSockets (rech_http2_
// io_thread/accept_session зовут accept()/recv() напрямую - см. её же
// комментарий, "ctx.usesHTTP2 => ctx.usesSockets") - а socketHelpers/
// socketHelpersTLS БЕЗУСЛОВНО несут rech_accept, возвращающую "?Сокет" -
// программа/библиотека, вызывающая ТОЛЬКО функции над уже открытой
// сессией (например, сгенерированная "rech proto" клиентская обёртка
// Сервис_Метод(сессия: HTTP2Сессия, ...) - саму сессию открывает
// ВЫЗЫВАЮЩАЯ программа, не эта функция), ни разу не звавшая "http2_
// принять_сессию"/"новая_http2_сессия"/"запрос" (единственные места,
// раньше резолвившие "?Сокет" - см. registerSocketNullableOnCall,
// funcSigTouchesSocket), падала на "unknown type name" у "rech build --lib".
func registerHTTP2TypesOnCall(entities map[string]entitySig, methods map[string]map[string]funcSig, aliases map[string]string, interfaces map[string]interfaceSig) error {
	if _, err := resolveTypeName(entities, methods, aliases, interfaces, "?Строка"); err != nil {
		return err
	}
	if err := registerSocketNullableOnCall(entities, methods, aliases, interfaces); err != nil {
		return err
	}
	_, err := resolveTypeName(entities, methods, aliases, interfaces, http2StreamType)
	return err
}
