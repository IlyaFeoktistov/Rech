// Package protoimport, продолжение (см. package doc lexer.go) - что именно
// поддержано генерацией, а что - явная ошибка вместо тихого несовпадения
// wire-формата с настоящим protobuf-собеседником:
//
// ПОДДЕРЖАНО: message (включая вложенные - flattenName), enum (top-level и
// вложенный), скаляры double/float/int32/int64/uint32/uint64/bool/string/
// bytes, repeated любого из них (включая repeated message), ссылки на
// другое message/enum того же файла или (локально) импортированного,
// syntax = "proto3".
//
// ЯВНО НЕ ПОДДЕРЖАНО (ошибка импорта, не тихая генерация несовместимого
// кода):
//   - proto2 (другая семантика presence).
//   - sint32/sint64/fixed32/fixed64/sfixed32/sfixed64 - см. package doc
//     parser.go, scalarTypes - их wire-представление не совпадает с тем,
//     что уже реализовано в internal/interp/protobuf.go / internal/
//     backend/cbackend/protobuf.go для Целое/Дробное/Действительное.
//   - oneof - у Rech нет объединения полей с таким же поведением
//     ("только одно из набора установлено, чтение остальных недоступно") -
//     сгенерировать N обычных полей означало бы молча потерять инвариант
//     "ровно одно из них", который as-is код может понять неверно.
//   - map<K, V> - в Rech есть Словарь<К, В>, но wire-формат "map" - на
//     самом деле repeated-сообщение из пары ключ/значение
//     (protobuf.dev/programming-guides/proto3#backwards-compatibility) -
//     кодек этого компилятора (internal/semantic/protobuf.go,
//     protobufCompatible) явно отвергает Словарь как цель PROTOBUF -
//     потребовался бы синтетический тип "Entry" и особый разбор, не
//     сделано в этом срезе.
//   - "google/protobuf/..." (well-known types - Timestamp/Duration/Any/...) -
//     не разрешаются как локальный импорт (см. protoimport.go,
//     collectFiles) - потребовали бы специального соответствия встроенным
//     типам Rech (например, Timestamp <-> ДатаВремя), не сделано.
//
// service/rpc (Шаг 96, generate_rpc.go) - unary rpc порождает полную
// клиентскую обёртку (grpc_* + PROTOBUF), streaming rpc - только "открыть
// поток" (сериализацию/цикл streaming-сообщений вызывающая программа ведёт
// сама) - см. package doc generate_rpc.go.
package protoimport

import (
	"fmt"
	"strings"
)

// generatedMessage - message/enum после разрешения ссылок типов полей,
// готовый к печати - плоский список (без вложенности - см. flattenName),
// поэтому дальше достаточно печатать их подряд.
type generatedField struct {
	name     string
	rechType string
	number   int64
	enumRef  string // непусто, если поле - ссылка на enum (комментарий "исходный тип - enum X")
}

type generatedMessage struct {
	rechName   string
	protoName  string // квалифицированное имя в исходном ".proto", для комментария
	sourceFile string
	fields     []generatedField
}

type generatedEnum struct {
	rechName  string
	protoName string
	values    []enumValue
}

// buildAndGenerate - реестр по files (в порядке, в котором они встретились
// при обходе импортов - см. protoimport.go) целиком, затем разрешает поля
// каждого message и печатает итоговый Rech-исходник.
func buildAndGenerate(files []*file) (string, error) {
	reg := newRegistry()
	for _, f := range files {
		if err := reg.register(f); err != nil {
			return "", err
		}
	}

	var messages []generatedMessage
	var enums []generatedEnum

	// walkMessages - обходит messages (и их nested/nestedEnums) файла f в
	// порядке объявления, ПРЕДПОРЯДКОМ (сам message раньше своих
	// вложенных) - определяет порядок печати, не порядок разрешения типов
	// (реестр уже построен целиком выше, поэтому вперёд-ссылки на ещё "не
	// напечатанный" ниже тип работают так же, как и на уже напечатанный -
	// Rech разрешает верхнеуровневые имена модуля независимо от порядка
	// объявления в файле).
	var walkMessages func(msgs []*message, chain []string, sourceFile string) error
	walkMessages = func(msgs []*message, chain []string, sourceFile string) error {
		for _, m := range msgs {
			qualified := m.name
			if len(chain) > 0 {
				qualified = strings.Join(chain, ".") + "." + m.name
			}
			gm := generatedMessage{rechName: flattenName(qualified), protoName: qualified, sourceFile: sourceFile}
			nextChain := append(append([]string{}, chain...), m.name)
			for _, f := range m.fields {
				if f.isOneofPart {
					return fmt.Errorf("%s:%d: поле %q входит в \"oneof %s\" - oneof не поддержан этим импортёром (см. package doc generate.go)", sourceFile, f.line, f.name, f.oneofName)
				}
				if f.mapKeyType != "" {
					return fmt.Errorf("%s:%d: поле %q - \"map<%s, %s>\" не поддержан этим импортёром (см. package doc generate.go)", sourceFile, f.line, f.name, f.mapKeyType, f.mapValType)
				}
				rechType, kind, _, err := reg.resolveFieldType(f.rawType, chain)
				if err != nil {
					return fmt.Errorf("%s:%d: поле %q: %w", sourceFile, f.line, f.name, err)
				}
				gf := generatedField{name: f.name, number: f.number}
				switch kind {
				case refEnum:
					gf.rechType = "Целое"
					gf.enumRef = rechType
				default:
					gf.rechType = rechType
				}
				if f.repeated {
					gf.rechType = "Список<" + gf.rechType + ">"
				}
				gm.fields = append(gm.fields, gf)
			}
			messages = append(messages, gm)
			if err := walkMessages(m.nested, nextChain, sourceFile); err != nil {
				return err
			}
			for _, e := range m.nestedEnums {
				enumQualified := strings.Join(nextChain, ".") + "." + e.name
				enums = append(enums, generatedEnum{rechName: flattenName(enumQualified), protoName: enumQualified, values: e.values})
			}
		}
		return nil
	}

	for _, f := range files {
		if err := walkMessages(f.messages, nil, f.path); err != nil {
			return "", err
		}
		for _, e := range f.enums {
			enums = append(enums, generatedEnum{rechName: flattenName(e.name), protoName: e.name, values: e.values})
		}
	}

	rpcs, usesUnaryRPC, err := resolveRPCs(files, reg)
	if err != nil {
		return "", err
	}

	return renderRech(enums, messages, rpcs, usesUnaryRPC), nil
}

func renderRech(enums []generatedEnum, messages []generatedMessage, rpcs []generatedRPC, usesUnaryRPC bool) string {
	var sb strings.Builder
	sb.WriteString("/** Сгенерировано \"rech proto\" из .proto-контракта - не редактировать вручную,\nперегенерировать при изменении контракта. См. docs/SPEC.md, раздел про PROTOBUF. */\n\n")

	for _, e := range enums {
		fmt.Fprintf(&sb, "// исходный enum \"%s\"\n", e.protoName)
		fmt.Fprintf(&sb, "перечисление %s\n{\n", e.rechName)
		for i, v := range e.values {
			comma := ","
			if i == len(e.values)-1 {
				comma = ""
			}
			fmt.Fprintf(&sb, "    %s = %d%s\n", v.name, v.number, comma)
		}
		sb.WriteString("}\n\n")
	}

	for _, m := range messages {
		fmt.Fprintf(&sb, "// исходное сообщение \"%s\" (%s)\n", m.protoName, m.sourceFile)
		fmt.Fprintf(&sb, "структура %s\n{\n", m.rechName)
		sb.WriteString("    имеет:\n")
		for _, f := range m.fields {
			comment := ""
			if f.enumRef != "" {
				comment = fmt.Sprintf(" // enum %s (перечисление объявлено выше, поле хранит целочисленное значение)", f.enumRef)
			}
			fmt.Fprintf(&sb, "        %s: %s `protobuf:%d`%s\n", f.name, f.rechType, f.number, comment)
		}
		sb.WriteString("}\n\n")
	}

	renderRPCs(&sb, rpcs, usesUnaryRPC)

	return sb.String()
}
