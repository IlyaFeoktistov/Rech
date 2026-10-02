// "сериализовать YAML <выражение>" - обратное к yaml.go, тот же принцип,
// что json_serialize.go/xml.go, порт по алгоритму
// internal/backend/cbackend/yaml_serialize.go (см. её package doc) -
// каждое поле-не-примитив получает перевод строки после "ключ:" и
// вложенный блок с отступом indent+2; пустой Список<T> - "[]" (блочный
// YAML не может выразить "далее ноль элементов" никаким отступом).
package interp

import (
	"strconv"
	"strings"

	"github.com/IlyaFeoktistov/Rech/internal/ir"
)

func (in *Interp) evalYAMLSerializeExpr(v Value) Value {
	var sb strings.Builder
	in.yamlSerialize(&sb, v, 0)
	return stringValue(sb.String())
}

// yamlSerialize пишет значение v в sb, начиная с колонки indent - тот же
// принцип, что и у cbackend (rech_yaml_serialize_<T>(value, sb, indent)),
// одна функция на все случаи (примитив/сущность/Список<T>), а не разные
// сигнатуры для скаляра и блока - в Go нет необходимости в отдельных
// C-функциях на каждый тип, диспетчеризация просто идёт по v.Type.Kind во
// время исполнения.
func (in *Interp) yamlSerialize(sb *strings.Builder, v Value, indent int) {
	switch v.Type.Kind {
	case ir.KindEntity:
		if v.Obj.coll != nil {
			in.yamlSerializeList(sb, v, indent)
			return
		}
		ent := in.entities[v.Obj.typeName]
		for _, f := range ent.Fields {
			sb.WriteString(strings.Repeat(" ", indent))
			sb.WriteString(f.JSONKey())
			sb.WriteByte(':')
			fv := v.Obj.getField(f.Name)
			if isYAMLPrimitiveKind(fv.Type.Kind) {
				sb.WriteByte(' ')
				yamlWriteScalar(sb, fv)
				sb.WriteByte('\n')
			} else {
				sb.WriteByte('\n')
				in.yamlSerialize(sb, fv, indent+2)
			}
		}
	default:
		yamlWriteScalar(sb, v)
	}
}

func (in *Interp) yamlSerializeList(sb *strings.Builder, v Value, indent int) {
	elems := v.Obj.coll.elems
	if len(elems) == 0 {
		sb.WriteString(strings.Repeat(" ", indent))
		sb.WriteString("[]\n")
		return
	}
	for _, elem := range elems {
		sb.WriteString(strings.Repeat(" ", indent))
		if isYAMLPrimitiveKind(elem.Type.Kind) {
			sb.WriteString("- ")
			yamlWriteScalar(sb, elem)
			sb.WriteByte('\n')
		} else {
			sb.WriteString("-\n")
			in.yamlSerialize(sb, elem, indent+2)
		}
	}
}

func isYAMLPrimitiveKind(k ir.TypeKind) bool {
	switch k {
	case ir.KindInt, ir.KindFloat32, ir.KindFloat, ir.KindString, ir.KindBool:
		return true
	default:
		return false
	}
}

// yamlWriteScalar - тот же набор представлений, что и yamlPrimitiveSerializerCode
// (cbackend) - число как есть, bool как true/false, строка ВСЕГДА в
// двойных кавычках (см. её package doc там же - простое, надёжное
// правило вместо анализа "похоже ли на число/true/false/пусто").
func yamlWriteScalar(sb *strings.Builder, v Value) {
	switch v.Type.Kind {
	case ir.KindInt:
		sb.WriteString(strconv.FormatInt(v.I, 10))
	case ir.KindFloat32, ir.KindFloat:
		sb.WriteString(formatFloat(v))
	case ir.KindBool:
		if v.B {
			sb.WriteString("true")
		} else {
			sb.WriteString("false")
		}
	case ir.KindString:
		yamlWriteDquoted(sb, v.S)
	}
}

// yamlWriteDquoted - тот же набор экранирований, что и
// cbackend.rech_yaml_sb_put_dquoted (yaml_serialize.go) - должны остаться
// согласованными между backend'ами.
func yamlWriteDquoted(sb *strings.Builder, s string) {
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
			sb.WriteRune(r)
		}
	}
	sb.WriteByte('"')
}
