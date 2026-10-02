// Package protoimport, продолжение (см. package doc generate.go) - service/
// rpc ТЕПЕРЬ порождают Rech-код (gRPC, Шаги 92-95, реализован): для unary
// rpc - полная клиентская обёртка (сериализация запроса/чтение и разбор
// ответа/проверка grpc-status с выбросом GRPCОшибка); для любого из трёх
// видов streaming - только "открыть поток" (grpc_открыть_поток на нужном
// пути) - сериализацию/цикл чтения-записи streaming-сообщений вызывающая
// программа ведёт сама через уже готовые grpc_*-функции (docs/SPEC.md,
// PROTOBUF/gRPC) - НЕ сгенерированные типобезопасные обёртки для
// streaming (следующий шаг, не начат).
package protoimport

import (
	"fmt"
	"strings"
)

type rpcKind int

const (
	rpcUnary rpcKind = iota
	rpcClientStreaming
	rpcServerStreaming
	rpcBidiStreaming
)

func (k rpcKind) comment() string {
	switch k {
	case rpcClientStreaming:
		return "client-streaming"
	case rpcServerStreaming:
		return "server-streaming"
	case rpcBidiStreaming:
		return "bidi-streaming"
	default:
		return "unary"
	}
}

// generatedRPC - один rpc-метод после разрешения типов запроса/ответа,
// готовый к печати.
type generatedRPC struct {
	funcName     string // "Сервис_Метод" - Rech не даёт функциям вложенную область видимости, поэтому имя сервиса - префикс (та же причина, что flattenName у message)
	path         string // "/пакет.Сервис/Метод" - см. package doc grpc.go (internal/semantic), тот же путь, что и http2_открыть_поток
	kind         rpcKind
	reqRechType  string
	respRechType string
	sourceFile   string
	protoService string
	protoMethod  string
}

// grpcErrorEntityName - имя исключения, которое бросают сгенерированные
// unary-обёртки при grpc-status != 0 - "GRPC" латиницей + "Ошибка"
// кириллицей: род определяется по кириллическому хвосту ("Ошибка" -
// женский), проверено живым прогоном ("новая GRPCОшибка(...)" принято
// компилятором, "новый" - нет) - тот же принцип смешанных имён, что и у
// "HTTP2Сессия"/"HTTP2Поток".
const grpcErrorEntityName = "GRPCОшибка"

const grpcErrorEntityCode = `// Бросается сгенерированными unary-обёртками ниже при "grpc-status" != 0.
сущность ` + grpcErrorEntityName + ` наследует Исключение
{
    имеет:
        Целое статус
    конструктор(статус: Целое, сообщение: Строка)
    {
        родитель(сообщение)
        этот.статус = статус
    }
}

`

// resolveRPCs - собирает generatedRPC по services всех files (тем же
// принципом, что и walkMessages в generate.go: reg уже построен целиком по
// ВСЕМ files графа импортов, поэтому вперёд-ссылки на ещё не напечатанный
// ниже message работают так же, как и на уже напечатанный). Запрос/ответ
// rpc ОБЯЗАНЫ разрешаться в message (не скаляр/enum) - настоящий protobuf
// тоже требует этого, у proto3-грамматики ".proto" синтаксически
// невозможно написать rpc со скалярным запросом/ответом, но локальный
// импорт мог сослаться на message из ДРУГОГО файла графа - проверка здесь
// защищает от опечатки в имени типа, ошибочно разрешившейся на enum с тем
// же именем.
func resolveRPCs(files []*file, reg *registry) ([]generatedRPC, bool, error) {
	var rpcs []generatedRPC
	usesUnary := false
	for _, f := range files {
		for _, svc := range f.services {
			for _, m := range svc.methods {
				reqRechType, reqKind, _, err := reg.resolveFieldType(m.reqType, nil)
				if err != nil {
					return nil, false, fmt.Errorf("%s:%d: rpc %q: запрос: %w", f.path, m.line, m.name, err)
				}
				if reqKind != refMessage {
					return nil, false, fmt.Errorf("%s:%d: rpc %q: тип запроса %q - не message (rpc допускает только message в запросе/ответе)", f.path, m.line, m.name, m.reqType)
				}
				respRechType, respKind, _, err := reg.resolveFieldType(m.respType, nil)
				if err != nil {
					return nil, false, fmt.Errorf("%s:%d: rpc %q: ответ: %w", f.path, m.line, m.name, err)
				}
				if respKind != refMessage {
					return nil, false, fmt.Errorf("%s:%d: rpc %q: тип ответа %q - не message (rpc допускает только message в запросе/ответе)", f.path, m.line, m.name, m.respType)
				}
				kind := rpcUnary
				switch {
				case m.reqStreaming && m.respStreaming:
					kind = rpcBidiStreaming
				case m.reqStreaming:
					kind = rpcClientStreaming
				case m.respStreaming:
					kind = rpcServerStreaming
				}
				path := "/" + svc.name + "/" + m.name
				if f.pkg != "" {
					path = "/" + f.pkg + "." + svc.name + "/" + m.name
				}
				if kind == rpcUnary {
					usesUnary = true
				}
				rpcs = append(rpcs, generatedRPC{
					funcName:     svc.name + "_" + m.name,
					path:         path,
					kind:         kind,
					reqRechType:  reqRechType,
					respRechType: respRechType,
					sourceFile:   f.path,
					protoService: svc.name,
					protoMethod:  m.name,
				})
			}
		}
	}
	return rpcs, usesUnary, nil
}

func renderRPCs(sb *strings.Builder, rpcs []generatedRPC, usesUnary bool) {
	if usesUnary {
		sb.WriteString(grpcErrorEntityCode)
	}
	for _, r := range rpcs {
		fmt.Fprintf(sb, "// rpc %s.%s (%s, %s) - путь %q\n", r.protoService, r.protoMethod, r.sourceFile, r.kind.comment(), r.path)
		if r.kind == rpcUnary {
			fmt.Fprintf(sb, `функция %s(сессия: HTTP2Сессия, запрос: %s): %s
{
    переменная поток = grpc_открыть_поток(сессия, "%s")
    grpc_отправить_сообщение(поток, сериализовать PROTOBUF запрос)
    http2_завершить_поток(поток)
    переменная тело = grpc_прочитать_сообщение(поток)
    если тело == Ничто {
        выбросить новая %s(0, "пустой ответ")
    }
    переменная ответ = разобрать PROTOBUF тело как %s
    grpc_прочитать_сообщение(поток)
    переменная статус = grpc_статус(поток)
    если статус != 0 {
        выбросить новая %s(статус, grpc_сообщение_ошибки(поток))
    }
    вернуть ответ
}

`, r.funcName, r.reqRechType, r.respRechType, r.path, grpcErrorEntityName, r.respRechType, grpcErrorEntityName)
			continue
		}
		fmt.Fprintf(sb, `// %s - запрос %s, ответ %s - откройте поток и ведите
// обмен сообщениями сами через grpc_отправить_сообщение/grpc_прочитать_
// сообщение/http2_завершить_поток/grpc_статус (см. docs/SPEC.md, PROTOBUF/gRPC).
функция %s(сессия: HTTP2Сессия): HTTP2Поток
{
    вернуть grpc_открыть_поток(сессия, "%s")
}

`, r.kind.comment(), r.reqRechType, r.respRechType, r.funcName, r.path)
	}
}
