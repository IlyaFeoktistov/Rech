// Package semantic: "разобрать PROTOBUF <выражение> как <Тип>"/
// "сериализовать PROTOBUF <выражение>" - schema-first типизированное
// кодирование/декодирование двоичного wire-формата Protocol Buffers прямо в
// значение <Тип>, тем же принципом, что и JSON/XML/YAML (internal/semantic/
// parse.go) - но с двумя структурными отличиями от них:
//
//  1. Источник/результат - "Список<Байт>" (см. internal/semantic/bytesio.go),
//     не "Строка": protobuf - двоичный формат, произвольный байт 0x00-0xFF
//     внутри - обычное дело (varint/fixed32/fixed64), а "Строка" в cbackend -
//     null-terminated C-строка (см. package doc bytesio.go).
//  2. Номер поля ОБЯЗАН быть задан явно тегом "protobuf:<номер>"
//     (internal/structtag) - в отличие от JSON, где тег "json:..."
//     необязателен (по умолчанию берётся имя поля), у protobuf имя поля
//     Rech никак не участвует в wire-формате вообще - реальный protobuf-
//     собеседник (написанный на любом другом языке) знает только номера
//     полей, не русские имена. Поле сущности/структуры без тега
//     "protobuf:..." - ошибка компиляции, не молчаливый пропуск.
//
// Схемы (".proto"-файлы) не парсятся и не генерируются - пользователь сам
// объявляет обычную Rech-сущность/структуру с тегами "protobuf:N" на каждом
// поле (тот же приём, что уже применён для "sql:..." в examples/бд/
// orm_легковесный.rech) - см. также docs/ROADMAP.md, развилка при взятии в
// работу.
package semantic

import (
	"fmt"
	"strconv"

	"github.com/IlyaFeoktistov/Rech/internal/ast"
	"github.com/IlyaFeoktistov/Rech/internal/ir"
	"github.com/IlyaFeoktistov/Rech/internal/structtag"
)

// protobufFormat - см. supportedFormats (parse.go).
const protobufFormat = "PROTOBUF"

// analyzeProtobufParseExpr - "разобрать PROTOBUF <выражение> как <Тип>".
// Source обязан быть "Список<Байт>" (не "Строка", см. package doc), целевой
// Тип - совместим с protobufCompatible.
func analyzeProtobufParseExpr(sc *ctx, e *ast.ParseExpr, source ir.Expr) (ir.Expr, error) {
	source = maybeUnwrapNarrowed(sc, source)
	if err := requireByteListType(ir.TypeOf(source), e.Line, "разобрать PROTOBUF"); err != nil {
		return nil, err
	}
	target, err := resolveTypeName(sc.entities, sc.methods, sc.aliases, sc.interfaces, e.TypeName)
	if err != nil {
		return nil, fmt.Errorf("строка %d: \"разобрать %s ... как\": %w", e.Line, protobufFormat, err)
	}
	if err := protobufTopLevelMessage(sc.entities, target); err != nil {
		return nil, fmt.Errorf("строка %d: \"разобрать %s\" в %s: %w", e.Line, protobufFormat, target, err)
	}
	registerParseErrorEntity(sc.entities)
	if err := registerByteListTypeOnCall(sc.entities, sc.methods, sc.aliases, sc.interfaces); err != nil {
		return nil, err
	}
	sc.a.registerProtobufParseTarget(target)
	return &ir.ParseExpr{Format: protobufFormat, Source: source, To: target}, nil
}

// analyzeProtobufSerializeExpr - "сериализовать PROTOBUF <выражение>" -
// обратное к analyzeProtobufParseExpr, результат - "Список<Байт>".
func analyzeProtobufSerializeExpr(sc *ctx, value ir.Expr, line int) (ir.Expr, error) {
	value = maybeUnwrapNarrowed(sc, value)
	t := ir.TypeOf(value)
	if err := protobufTopLevelMessage(sc.entities, t); err != nil {
		return nil, fmt.Errorf("строка %d: \"сериализовать %s\" для %s: %w", line, protobufFormat, t, err)
	}
	if err := registerByteListTypeOnCall(sc.entities, sc.methods, sc.aliases, sc.interfaces); err != nil {
		return nil, err
	}
	sc.a.registerProtobufSerializeTarget(t)
	return &ir.SerializeExpr{Format: protobufFormat, Value: value}, nil
}

// requireByteListType - Source/значение "разобрать PROTOBUF"/"сериализовать
// PROTOBUF" обязаны быть "Список<Байт>" - в отличие от JSON/XML/YAML
// (требуют "Строка"), см. package doc.
func requireByteListType(t ir.Type, line int, what string) error {
	if t.Kind == ir.KindEntity && t.Entity == byteListType {
		return nil
	}
	return fmt.Errorf("строка %d: %q ожидает источник типа Список<Байт>, получено %s", line, what, t)
}

// protobufTopLevelMessage - настоящий protobuf не знает понятия "верхний
// уровень - примитив/список" вовсе (wire-формат всегда описывает ОДНО
// сообщение, поля которого несут номера) - "разобрать PROTOBUF ... как
// <Тип>"/"сериализовать PROTOBUF <значение>" целиком ограничены сущностью/
// структурой (protobuf message), не примитивом и не голым Список<T> - в
// отличие от JSON/XML/YAML (structCompatible выше), которые допускают любой
// из трёх на верхнем уровне. Внутри сообщения (полем) примитив/Список<T>/
// вложенное сообщение/Список<Байт> - как обычно, через protobufCompatible.
func protobufTopLevelMessage(entities map[string]entitySig, t ir.Type) error {
	if t.Kind != ir.KindEntity || t.Entity == byteListType {
		return fmt.Errorf("%s не поддержан как верхний уровень %s (protobuf message - только сущность/структура, не примитив и не голый список)", t, protobufFormat)
	}
	if sig, ok := entities[t.Entity]; ok && sig.IsList {
		return fmt.Errorf("%s не поддержан как верхний уровень %s (protobuf message - только сущность/структура, не голый список - оберните его полем сообщения)", t, protobufFormat)
	}
	return protobufCompatible(entities, t, nil)
}

// protobufCompatible - аналог structCompatible (parse.go) для формата
// PROTOBUF: примитив из набора {Целое, Дробное, Действительное, Строка,
// Логическое} (та же арифметика типов, что и JSON - protobuf сам различает
// куда больше числовых представлений, но у Rech их ровно эти), "Список<Байт>"
// как единственный способ выразить protobuf-тип "bytes" (не "repeated
// Байт"), Список<T> с совместимым НЕ-списочным T (одна вложенность -
// protobuf не поддерживает repeated-of-repeated напрямую без sub-message-
// обёртки, которую эта реализация не генерирует), либо сущность/структура
// (protobuf message) - ВСЕ поля которой (включая унаследованные) несут
// обязательный тег "protobuf:<номер>" с попарно различными номерами.
func protobufCompatible(entities map[string]entitySig, t ir.Type, seen map[string]bool) error {
	switch t.Kind {
	case ir.KindInt, ir.KindFloat32, ir.KindFloat, ir.KindString, ir.KindBool:
		return nil
	case ir.KindEntity:
		if t.Entity == byteListType {
			return nil
		}
		sig, ok := entities[t.Entity]
		if !ok {
			return fmt.Errorf("тип %s не найден", t)
		}
		if sig.IsList {
			if sig.ElemType.Kind == ir.KindEntity {
				if elemSig, ok := entities[sig.ElemType.Entity]; ok && elemSig.IsList {
					return fmt.Errorf("Список<Список<...>> не поддержан как цель %s (protobuf не выражает repeated-of-repeated без промежуточного message)", protobufFormat)
				}
			}
			return protobufCompatible(entities, sig.ElemType, seen)
		}
		if sig.IsDict || sig.IsExternOpaque || sig.IsExternStruct {
			return fmt.Errorf("%s не поддержан как цель %s (только примитивы, сущности/структуры и Список<T>)", t, protobufFormat)
		}
		if sig.HasCtor {
			return fmt.Errorf("%s не поддержан как цель %s: у сущности есть собственный \"конструктор(...)\" - разбор заполняет поля напрямую, а не вызывает пользовательский конструктор с неизвестным ему набором аргументов", t, protobufFormat)
		}
		if seen[t.Entity] {
			return fmt.Errorf("%s ссылается сам на себя - самоссылающиеся типы не поддержаны как цель %s", t, protobufFormat)
		}
		if _, err := protobufFieldNumbers(sig); err != nil {
			return fmt.Errorf("%s: %w", t, err)
		}
		next := make(map[string]bool, len(seen)+1)
		for k := range seen {
			next[k] = true
		}
		next[t.Entity] = true
		for i, ft := range sig.FieldTypes {
			if err := protobufCompatible(entities, ft, next); err != nil {
				return fmt.Errorf("поле %q: %w", sig.FieldNames[i], err)
			}
		}
		return nil
	default:
		return fmt.Errorf("%s не поддержан как цель %s (только примитивы, сущности/структуры и Список<T>)", t, protobufFormat)
	}
}

// protobufFieldNumbers возвращает номер поля (тег "protobuf:<номер>") для
// каждого поля sig, в том же порядке, что FieldNames/FieldTypes - ошибка,
// если у какого-то поля тег отсутствует, номер не целое положительное число,
// или два поля сталкиваются на один и тот же номер (см. package doc - в
// отличие от JSON, здесь тег не опционален).
func protobufFieldNumbers(sig entitySig) ([]int, error) {
	nums := make([]int, len(sig.FieldNames))
	seen := make(map[int]string, len(sig.FieldNames))
	for i, name := range sig.FieldNames {
		var raw string
		if i < len(sig.FieldTags) {
			raw = structtag.Parse(sig.FieldTags[i]).Get("protobuf")
		}
		if raw == "" {
			return nil, fmt.Errorf("поле %q не помечено тегом `protobuf:<номер>` - обязателен для каждого поля цели PROTOBUF", name)
		}
		n, err := strconv.Atoi(raw)
		if err != nil || n <= 0 {
			return nil, fmt.Errorf("поле %q: тег `protobuf:%s` - номер поля обязан быть целым положительным числом", name, raw)
		}
		if other, dup := seen[n]; dup {
			return nil, fmt.Errorf("поля %q и %q конфликтуют - у обоих одинаковый номер protobuf-поля %d", other, name, n)
		}
		seen[n] = name
		nums[i] = n
	}
	return nums, nil
}
