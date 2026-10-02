// "сериализовать JSON <выражение>" (internal/ir.SerializeExpr) - обратное к
// json.go: обходит Value по её собственному типу (уже проверенному
// семантическим анализом на JSON-совместимость, internal/semantic/parse.go,
// jsonCompatible) и пишет JSON-текст напрямую в strings.Builder - в отличие
// от разбора, сериализация корректно типизированного значения не может
// провалиться во время исполнения, поэтому нет отдельного пути ошибки.
package interp

import (
	"fmt"
	"strconv"
	"strings"

	"github.com/IlyaFeoktistov/Rech/internal/ir"
)

func (in *Interp) evalSerializeExpr(e *ir.SerializeExpr, env *Env, cc *callCtx) (Value, error) {
	v, err := in.evalExpr(e.Value, env, cc)
	if err != nil {
		return Value{}, err
	}
	if e.Format == "XML" {
		return in.evalXMLSerializeExpr(v), nil
	}
	if e.Format == "YAML" {
		return in.evalYAMLSerializeExpr(v), nil
	}
	if e.Format == "PROTOBUF" {
		return in.evalProtobufSerializeExpr(v), nil
	}
	var sb strings.Builder
	in.jsonSerialize(&sb, v)
	return stringValue(sb.String()), nil
}

func (in *Interp) jsonSerialize(sb *strings.Builder, v Value) {
	switch v.Type.Kind {
	case ir.KindInt:
		sb.WriteString(strconv.FormatInt(v.I, 10))
	case ir.KindFloat32, ir.KindFloat:
		sb.WriteString(formatFloat(v))
	case ir.KindDecimal:
		sb.WriteString(v.Dec.String())
	case ir.KindString:
		jsonWriteEscapedString(sb, v.S)
	case ir.KindBool:
		if v.B {
			sb.WriteString("true")
		} else {
			sb.WriteString("false")
		}
	case ir.KindEntity:
		if v.Obj.coll != nil {
			sb.WriteByte('[')
			for i, elem := range v.Obj.coll.elems {
				if i > 0 {
					sb.WriteByte(',')
				}
				in.jsonSerialize(sb, elem)
			}
			sb.WriteByte(']')
			return
		}
		ent := in.entities[v.Obj.typeName]
		sb.WriteByte('{')
		for i, f := range ent.Fields {
			if i > 0 {
				sb.WriteByte(',')
			}
			jsonWriteEscapedString(sb, f.JSONKey())
			sb.WriteByte(':')
			in.jsonSerialize(sb, v.Obj.getField(f.Name))
		}
		sb.WriteByte('}')
	}
}

// jsonWriteEscapedString - тот же набор экранирований, что и
// cbackend.rech_json_sb_put_escaped (json_serialize.go) - должны остаться
// согласованными, иначе сериализованный текст разошёлся бы между
// backend'ами для строк со спецсимволами.
func jsonWriteEscapedString(sb *strings.Builder, s string) {
	sb.WriteByte('"')
	for _, r := range s {
		switch r {
		case '"':
			sb.WriteString(`\"`)
		case '\\':
			sb.WriteString(`\\`)
		case '\n':
			sb.WriteString(`\n`)
		case '\r':
			sb.WriteString(`\r`)
		case '\t':
			sb.WriteString(`\t`)
		default:
			if r < 0x20 {
				fmt.Fprintf(sb, `\u%04x`, r)
			} else {
				sb.WriteRune(r)
			}
		}
	}
	sb.WriteByte('"')
}
