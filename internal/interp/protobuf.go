// "разобрать PROTOBUF <выражение> как <Тип>"/"сериализовать PROTOBUF
// <выражение>" (internal/ir.ParseExpr/SerializeExpr, Format == "PROTOBUF") -
// ручная реализация двоичного wire-формата Protocol Buffers (varint/
// fixed32/fixed64/length-delimited), совместимая с настоящими protobuf-
// собеседниками (не собственный, только-для-Rech формат) - см. package doc
// internal/semantic/protobuf.go для схемы (теги "protobuf:<номер>" вместо
// ".proto"-файлов) и internal/backend/cbackend/protobuf.go для эквивалентной
// реализации на C.
//
// В отличие от JSON/XML/YAML разбор НЕ бросает "ОшибкаРазбора" на
// несовпадение номера поля/вида значения - protobuf по спецификации молча
// пропускает незнакомые/несовпадающие поля (forward/backward-совместимость,
// см. rtmsg.ProtobufParseError) - исключение бросается только на
// физически оборванные байты.
package interp

import (
	"encoding/binary"
	"math"
	"strconv"

	"github.com/IlyaFeoktistov/Rech/internal/ir"
	"github.com/IlyaFeoktistov/Rech/internal/rtmsg"
	"github.com/IlyaFeoktistov/Rech/internal/structtag"
)

// Wire-типы protobuf (см. https://protobuf.dev/programming-guides/encoding/) -
// 3/4 (group) - устаревшая, не поддержанная форма.
const (
	pbWireVarint  = 0
	pbWireFixed64 = 1
	pbWireBytes   = 2
	pbWireFixed32 = 5
)

func (in *Interp) protobufParseFail() error {
	return newSystemException(parseErrorEntity, rtmsg.ProtobufParseError)
}

// pbFieldNumber - номер поля (тег "protobuf:<номер>") - уже проверен
// internal/semantic/protobuf.go (protobufFieldNumbers) для ЛЮБОГО поля,
// достижимого из места, где вызывается эта функция - ошибка здесь
// невозможна, не обрабатывается отдельно.
func pbFieldNumber(f ir.Field) int {
	n, _ := strconv.Atoi(structtag.Parse(f.Tag).Get("protobuf"))
	return n
}

func (in *Interp) evalProtobufParseExpr(e *ir.ParseExpr, src Value) (Value, error) {
	buf := bytesFromValue(src)
	return in.pbDecodeMessage(buf, e.To)
}

func (in *Interp) evalProtobufSerializeExpr(v Value) Value {
	return bytesValue(in.pbEncodeMessage(v))
}

// ---- Кодирование ----

// pbEncodeMessage сериализует v (сущность/структура) в protobuf-совместимые
// байты - конкатенация полей в порядке их объявления (порядок полей в
// одном сообщении не значим для протокола, но детерминирован для
// воспроизводимости вывода).
func (in *Interp) pbEncodeMessage(v Value) []byte {
	ent := in.entities[v.Obj.typeName]
	var buf []byte
	for _, f := range ent.Fields {
		buf = in.pbEncodeSingularField(buf, pbFieldNumber(f), f.Type, v.Obj.getField(f.Name))
	}
	return buf
}

// pbEncodeSingularField кодирует ОДНО поле сообщения (не элемент Список<T> -
// см. pbEncodeRepeatedField) - proto3-семантика: значение "по умолчанию"
// (0/""/ложь/пустой Список<Байт>) целиком пропускается (implicit presence) -
// вложенное сообщение кодируется всегда (у сущности нет понятия "не
// присвоено" в этой модели - поле всегда сконструировано).
func (in *Interp) pbEncodeSingularField(buf []byte, num int, t ir.Type, val Value) []byte {
	switch t.Kind {
	case ir.KindInt, ir.KindBool, ir.KindFloat32, ir.KindFloat:
		return pbWriteTaggedScalar(buf, num, t, val, true)
	case ir.KindString:
		if val.S == "" {
			return buf
		}
		return pbWriteTaggedBytes(buf, num, []byte(val.S))
	case ir.KindEntity:
		if t.Entity == byteListType {
			data := bytesFromValue(val)
			if len(data) == 0 {
				return buf
			}
			return pbWriteTaggedBytes(buf, num, data)
		}
		if g, ok := in.generics[t.Entity]; ok && g.Template == "Список" {
			return in.pbEncodeRepeatedField(buf, num, g.ElemType, val.Obj.coll.elems)
		}
		return pbWriteTaggedBytes(buf, num, in.pbEncodeMessage(val))
	}
	return buf
}

// pbEncodeRepeatedField кодирует Список<T>-поле - packed (одно
// length-delimited поле, содержащее конкатенацию "сырых" значений без
// собственных тегов) для packable-скаляров (Целое/Логическое/Дробное/
// Действительное - proto3-умолчание для repeated-скаляров), unpacked (тег +
// значение на каждый элемент) для Строка/Список<Байт>/вложенного сообщения -
// в отличие от singular-поля, элементы кодируются ВСЕГДА, включая нулевые
// (значима сама длина списка, не значение элемента).
func (in *Interp) pbEncodeRepeatedField(buf []byte, num int, elemType ir.Type, elems []Value) []byte {
	if len(elems) == 0 {
		return buf
	}
	switch elemType.Kind {
	case ir.KindInt, ir.KindBool, ir.KindFloat32, ir.KindFloat:
		var payload []byte
		for _, e := range elems {
			payload = pbAppendRawScalar(payload, elemType, e)
		}
		return pbWriteTaggedBytes(buf, num, payload)
	case ir.KindString:
		for _, e := range elems {
			buf = pbWriteTaggedBytes(buf, num, []byte(e.S))
		}
		return buf
	case ir.KindEntity:
		if elemType.Entity == byteListType {
			for _, e := range elems {
				buf = pbWriteTaggedBytes(buf, num, bytesFromValue(e))
			}
			return buf
		}
		for _, e := range elems {
			buf = pbWriteTaggedBytes(buf, num, in.pbEncodeMessage(e))
		}
		return buf
	}
	return buf
}

func pbWriteTag(buf []byte, num int, wireType uint64) []byte {
	return pbAppendVarint(buf, uint64(num)<<3|wireType)
}

func pbWriteTaggedScalar(buf []byte, num int, t ir.Type, val Value, skipZero bool) []byte {
	switch t.Kind {
	case ir.KindInt:
		if skipZero && val.I == 0 {
			return buf
		}
		buf = pbWriteTag(buf, num, pbWireVarint)
		return pbAppendVarint(buf, uint64(val.I))
	case ir.KindBool:
		if skipZero && !val.B {
			return buf
		}
		buf = pbWriteTag(buf, num, pbWireVarint)
		if val.B {
			return pbAppendVarint(buf, 1)
		}
		return pbAppendVarint(buf, 0)
	case ir.KindFloat32:
		if skipZero && val.F == 0 {
			return buf
		}
		buf = pbWriteTag(buf, num, pbWireFixed32)
		return binary.LittleEndian.AppendUint32(buf, math.Float32bits(float32(val.F)))
	default: // ir.KindFloat
		if skipZero && val.F == 0 {
			return buf
		}
		buf = pbWriteTag(buf, num, pbWireFixed64)
		return binary.LittleEndian.AppendUint64(buf, math.Float64bits(val.F))
	}
}

func pbWriteTaggedBytes(buf []byte, num int, data []byte) []byte {
	buf = pbWriteTag(buf, num, pbWireBytes)
	buf = pbAppendVarint(buf, uint64(len(data)))
	return append(buf, data...)
}

// pbAppendRawScalar пишет "сырое" (без тега) значение - используется внутри
// packed-payload repeated-поля (pbEncodeRepeatedField).
func pbAppendRawScalar(buf []byte, t ir.Type, val Value) []byte {
	switch t.Kind {
	case ir.KindInt:
		return pbAppendVarint(buf, uint64(val.I))
	case ir.KindBool:
		if val.B {
			return pbAppendVarint(buf, 1)
		}
		return pbAppendVarint(buf, 0)
	case ir.KindFloat32:
		return binary.LittleEndian.AppendUint32(buf, math.Float32bits(float32(val.F)))
	default: // ir.KindFloat
		return binary.LittleEndian.AppendUint64(buf, math.Float64bits(val.F))
	}
}

func pbAppendVarint(buf []byte, v uint64) []byte {
	for v >= 0x80 {
		buf = append(buf, byte(v)|0x80)
		v >>= 7
	}
	return append(buf, byte(v))
}

// ---- Декодирование ----

// pbDecodeMessage разбирает buf (тело сообщения - без внешнего тега/длины,
// он уже снят вызывающей стороной, если сообщение вложенное) в значение
// target (сущность/структура). Поля, отсутствующие в buf, получают
// нулевое значение (in.zeroFieldValue) - proto3-семантика "поле не
// закодировано == значение по умолчанию", а не ошибка (в отличие от JSON,
// где отсутствующее поле - "ОшибкаРазбора"). Незнакомый номер поля/
// несовпадение wire-типа с ожидаемым - молча пропускается (см. package doc)
// - исключение бросается ТОЛЬКО когда сами байты физически обрываются
// раньше конца значения.
func (in *Interp) pbDecodeMessage(buf []byte, target ir.Type) (Value, error) {
	ent := in.entities[target.Entity]
	fields := make(map[string]Value, len(ent.Fields))
	idxByNum := make(map[int]int, len(ent.Fields))
	for i := range ent.Fields {
		fields[ent.Fields[i].Name] = in.zeroFieldValue(ent.Fields[i].Type)
		idxByNum[pbFieldNumber(ent.Fields[i])] = i
	}
	pos := 0
	for pos < len(buf) {
		num, wireType, n, err := pbReadTag(buf[pos:])
		if err != nil {
			return Value{}, in.protobufParseFail()
		}
		pos += n
		idx, known := idxByNum[num]
		if !known {
			skipN, err := pbSkipField(buf[pos:], wireType)
			if err != nil {
				return Value{}, in.protobufParseFail()
			}
			pos += skipN
			continue
		}
		f := ent.Fields[idx]
		val, consumed, err := in.pbDecodeFieldValue(buf[pos:], wireType, f.Type, fields[f.Name])
		if err != nil {
			return Value{}, err
		}
		pos += consumed
		fields[f.Name] = val
	}
	return Value{Type: target, Obj: &entityObj{typeName: target.Entity, fields: fields}}, nil
}

// pbDecodeFieldValue декодирует ОДНО значение с wire (уже без тега) для
// поля статического типа fieldType, current - его текущее значение
// (используется как накопитель для Список<T> - repeated-поле может
// встретиться на wire несколько раз подряд, каждый раз добавляя элемент(ы),
// а не заменяя список целиком). Возвращает (новое значение, число
// потреблённых байт, ошибка) - ошибка только при физически оборванных
// данных; несовпадение wire-типа с ожидаемым по fieldType - тихий пропуск
// (consumed по wireType, current не меняется).
func (in *Interp) pbDecodeFieldValue(data []byte, wireType int, fieldType ir.Type, current Value) (Value, int, error) {
	if fieldType.Kind == ir.KindEntity {
		if g, ok := in.generics[fieldType.Entity]; ok && g.Template == "Список" {
			return in.pbDecodeRepeatedElement(data, wireType, g.ElemType, current)
		}
	}
	switch fieldType.Kind {
	case ir.KindInt:
		if wireType != pbWireVarint {
			n, err := pbSkipField(data, wireType)
			return current, n, err
		}
		v, n, err := pbReadVarint(data)
		if err != nil {
			return Value{}, 0, in.protobufParseFail()
		}
		return intValue(int64(v)), n, nil
	case ir.KindBool:
		if wireType != pbWireVarint {
			n, err := pbSkipField(data, wireType)
			return current, n, err
		}
		v, n, err := pbReadVarint(data)
		if err != nil {
			return Value{}, 0, in.protobufParseFail()
		}
		return boolValue(v != 0), n, nil
	case ir.KindFloat32:
		if wireType != pbWireFixed32 {
			n, err := pbSkipField(data, wireType)
			return current, n, err
		}
		v, n, err := pbReadFixed32(data)
		if err != nil {
			return Value{}, 0, in.protobufParseFail()
		}
		return Value{Type: ir.TypeFloat32, F: float64(math.Float32frombits(v))}, n, nil
	case ir.KindFloat:
		if wireType != pbWireFixed64 {
			n, err := pbSkipField(data, wireType)
			return current, n, err
		}
		v, n, err := pbReadFixed64(data)
		if err != nil {
			return Value{}, 0, in.protobufParseFail()
		}
		return floatValue(math.Float64frombits(v)), n, nil
	case ir.KindString:
		if wireType != pbWireBytes {
			n, err := pbSkipField(data, wireType)
			return current, n, err
		}
		b, n, err := pbReadBytes(data)
		if err != nil {
			return Value{}, 0, in.protobufParseFail()
		}
		return stringValue(string(b)), n, nil
	case ir.KindEntity:
		if wireType != pbWireBytes {
			n, err := pbSkipField(data, wireType)
			return current, n, err
		}
		b, n, err := pbReadBytes(data)
		if err != nil {
			return Value{}, 0, in.protobufParseFail()
		}
		if fieldType.Entity == byteListType {
			return bytesValue(b), n, nil
		}
		sub, err := in.pbDecodeMessage(b, fieldType)
		if err != nil {
			return Value{}, 0, err
		}
		return sub, n, nil
	default:
		n, err := pbSkipField(data, wireType)
		return current, n, err
	}
}

// pbDecodeRepeatedElement добавляет один (или, для packed-скаляров,
// несколько сразу) элемент(ы) в текущий Список<T> current - принимает ОБЕ
// формы, которые допускает сама спецификация protobuf: packed (wireType ==
// pbWireBytes для packable-скаляра elemType - длина + конкатенация сырых
// значений без тегов) и unpacked (wireType совпадает с "родным" wire-типом
// elemType - одно значение, тег которого уже снят вызывающей стороной).
func (in *Interp) pbDecodeRepeatedElement(data []byte, wireType int, elemType ir.Type, current Value) (Value, int, error) {
	appendElem := func(v Value) {
		current.Obj.coll.elems = append(current.Obj.coll.elems, v)
	}
	switch elemType.Kind {
	case ir.KindInt, ir.KindBool:
		if wireType == pbWireBytes {
			payload, n, err := pbReadBytes(data)
			if err != nil {
				return Value{}, 0, in.protobufParseFail()
			}
			for len(payload) > 0 {
				v, m, err := pbReadVarint(payload)
				if err != nil {
					return Value{}, 0, in.protobufParseFail()
				}
				if elemType.Kind == ir.KindBool {
					appendElem(boolValue(v != 0))
				} else {
					appendElem(intValue(int64(v)))
				}
				payload = payload[m:]
			}
			return current, n, nil
		}
		if wireType != pbWireVarint {
			n, err := pbSkipField(data, wireType)
			return current, n, err
		}
		v, n, err := pbReadVarint(data)
		if err != nil {
			return Value{}, 0, in.protobufParseFail()
		}
		if elemType.Kind == ir.KindBool {
			appendElem(boolValue(v != 0))
		} else {
			appendElem(intValue(int64(v)))
		}
		return current, n, nil
	case ir.KindFloat32:
		if wireType == pbWireBytes {
			payload, n, err := pbReadBytes(data)
			if err != nil {
				return Value{}, 0, in.protobufParseFail()
			}
			for len(payload) >= 4 {
				v, _, _ := pbReadFixed32(payload)
				appendElem(Value{Type: ir.TypeFloat32, F: float64(math.Float32frombits(v))})
				payload = payload[4:]
			}
			return current, n, nil
		}
		if wireType != pbWireFixed32 {
			n, err := pbSkipField(data, wireType)
			return current, n, err
		}
		v, n, err := pbReadFixed32(data)
		if err != nil {
			return Value{}, 0, in.protobufParseFail()
		}
		appendElem(Value{Type: ir.TypeFloat32, F: float64(math.Float32frombits(v))})
		return current, n, nil
	case ir.KindFloat:
		if wireType == pbWireBytes {
			payload, n, err := pbReadBytes(data)
			if err != nil {
				return Value{}, 0, in.protobufParseFail()
			}
			for len(payload) >= 8 {
				v, _, _ := pbReadFixed64(payload)
				appendElem(floatValue(math.Float64frombits(v)))
				payload = payload[8:]
			}
			return current, n, nil
		}
		if wireType != pbWireFixed64 {
			n, err := pbSkipField(data, wireType)
			return current, n, err
		}
		v, n, err := pbReadFixed64(data)
		if err != nil {
			return Value{}, 0, in.protobufParseFail()
		}
		appendElem(floatValue(math.Float64frombits(v)))
		return current, n, nil
	case ir.KindString:
		if wireType != pbWireBytes {
			n, err := pbSkipField(data, wireType)
			return current, n, err
		}
		b, n, err := pbReadBytes(data)
		if err != nil {
			return Value{}, 0, in.protobufParseFail()
		}
		appendElem(stringValue(string(b)))
		return current, n, nil
	case ir.KindEntity:
		if wireType != pbWireBytes {
			n, err := pbSkipField(data, wireType)
			return current, n, err
		}
		b, n, err := pbReadBytes(data)
		if err != nil {
			return Value{}, 0, in.protobufParseFail()
		}
		if elemType.Entity == byteListType {
			appendElem(bytesValue(b))
			return current, n, nil
		}
		sub, err := in.pbDecodeMessage(b, elemType)
		if err != nil {
			return Value{}, 0, err
		}
		appendElem(sub)
		return current, n, nil
	default:
		n, err := pbSkipField(data, wireType)
		return current, n, err
	}
}

func pbReadTag(data []byte) (num int, wireType int, consumed int, err error) {
	v, n, err := pbReadVarint(data)
	if err != nil {
		return 0, 0, 0, err
	}
	return int(v >> 3), int(v & 0x7), n, nil
}

func pbReadVarint(data []byte) (uint64, int, error) {
	var v uint64
	for i := range len(data) {
		b := data[i]
		v |= uint64(b&0x7F) << (7 * uint(i))
		if b&0x80 == 0 {
			return v, i + 1, nil
		}
		if i == 9 {
			break
		}
	}
	return 0, 0, errProtobufTruncated
}

func pbReadFixed32(data []byte) (uint32, int, error) {
	if len(data) < 4 {
		return 0, 0, errProtobufTruncated
	}
	return binary.LittleEndian.Uint32(data), 4, nil
}

func pbReadFixed64(data []byte) (uint64, int, error) {
	if len(data) < 8 {
		return 0, 0, errProtobufTruncated
	}
	return binary.LittleEndian.Uint64(data), 8, nil
}

func pbReadBytes(data []byte) ([]byte, int, error) {
	l, n, err := pbReadVarint(data)
	if err != nil {
		return nil, 0, err
	}
	end := n + int(l)
	if end < n || end > len(data) {
		return nil, 0, errProtobufTruncated
	}
	return data[n:end], end, nil
}

// pbSkipField пропускает ОДНО значение неизвестного/несовпадающего поля по
// его wireType - group (3/4) не поддержана (устаревшая форма, ни один
// современный protobuf-собеседник её не эмитит).
func pbSkipField(data []byte, wireType int) (int, error) {
	switch wireType {
	case pbWireVarint:
		_, n, err := pbReadVarint(data)
		return n, err
	case pbWireFixed64:
		_, n, err := pbReadFixed64(data)
		return n, err
	case pbWireBytes:
		_, n, err := pbReadBytes(data)
		return n, err
	case pbWireFixed32:
		_, n, err := pbReadFixed32(data)
		return n, err
	default:
		return 0, errProtobufTruncated
	}
}

var errProtobufTruncated = protobufTruncatedError{}

type protobufTruncatedError struct{}

func (protobufTruncatedError) Error() string { return rtmsg.ProtobufParseError }
