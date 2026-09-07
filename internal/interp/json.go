// "разобрать JSON <выражение> как <Тип>" (internal/ir.ParseExpr) - разбор
// текста через encoding/json в универсальное Go-значение (map[string]any/
// []any/string/float64/bool/nil), затем структурное извлечение по
// целевому ir.Type (jsonExtract) - типовая совместимость уже проверена
// семантическим анализом (internal/semantic/parse.go, jsonCompatible),
// здесь остаётся только фактическое совпадение с формой РЕАЛЬНОГО текста
// (отсутствующее поле/несовпадение JSON-вида - обычная ошибка данных, не
// баг компилятора). Любое несовпадение выбрасывает "ОшибкаРазбора" тем же
// каналом, что и обычное "выбросить" (thrownException) - см. package doc
// internal/backend/cbackend/json_extract.go для эквивалентной реализации
// на C (ручной JSON-парсер, никакой библиотеки не подключается).
package interp

import (
	"encoding/json"

	"github.com/IlyaFeoktistov/Rech/internal/ir"
	"github.com/IlyaFeoktistov/Rech/internal/rtmsg"
)

const parseErrorEntity = "ОшибкаРазбора"

func (in *Interp) jsonParseFail(msg string) error {
	return newSystemException(parseErrorEntity, msg)
}

func (in *Interp) evalParseExpr(e *ir.ParseExpr, env *Env, cc *callCtx) (Value, error) {
	src, err := in.evalExpr(e.Source, env, cc)
	if err != nil {
		return Value{}, err
	}
	if e.Format == "XML" {
		return in.evalXMLParseExpr(e, src)
	}
	if e.Format == "YAML" {
		return in.evalYAMLParseExpr(e, src)
	}
	if e.Format == "PROTOBUF" {
		return in.evalProtobufParseExpr(e, src)
	}
	var raw any
	if err := json.Unmarshal([]byte(src.S), &raw); err != nil {
		return Value{}, in.jsonParseFail(rtmsg.JSONParseError)
	}
	return in.jsonExtract(raw, e.To)
}

// jsonExtract извлекает значение типа target из raw (результат
// json.Unmarshal в any). Сообщение об ошибке несовпадения вида JSON-
// значения (rtmsg.JSONValueMismatch) намеренно без имени поля/индекса - см.
// её package doc: единый текст на любой глубине вложенности нужен, чтобы
// оставаться байт-в-байт одинаковым со сгенерированным C (там это
// C-литерал, зашитый при кодогенерации, без рантайм-форматирования).
func (in *Interp) jsonExtract(raw any, target ir.Type) (Value, error) {
	mismatch := func() error { return in.jsonParseFail(rtmsg.JSONValueMismatch) }
	switch target.Kind {
	case ir.KindInt:
		n, ok := raw.(float64)
		if !ok || n != float64(int64(n)) {
			return Value{}, mismatch()
		}
		return intValue(int64(n)), nil
	case ir.KindFloat32:
		n, ok := raw.(float64)
		if !ok {
			return Value{}, mismatch()
		}
		return Value{Type: ir.TypeFloat32, F: n}, nil
	case ir.KindFloat:
		n, ok := raw.(float64)
		if !ok {
			return Value{}, mismatch()
		}
		return floatValue(n), nil
	case ir.KindString:
		s, ok := raw.(string)
		if !ok {
			return Value{}, mismatch()
		}
		return stringValue(s), nil
	case ir.KindBool:
		b, ok := raw.(bool)
		if !ok {
			return Value{}, mismatch()
		}
		return boolValue(b), nil
	case ir.KindEntity:
		if g, ok := in.generics[target.Entity]; ok && g.Template == "Список" {
			arr, ok := raw.([]any)
			if !ok {
				return Value{}, mismatch()
			}
			elems := make([]Value, len(arr))
			for i, item := range arr {
				v, err := in.jsonExtract(item, g.ElemType)
				if err != nil {
					return Value{}, err
				}
				elems[i] = v
			}
			return Value{Type: target, Obj: &entityObj{typeName: target.Entity, coll: &collection{template: "Список", elems: elems}}}, nil
		}
		ent, ok := in.entities[target.Entity]
		if !ok {
			return Value{}, mismatch()
		}
		obj, ok := raw.(map[string]any)
		if !ok {
			return Value{}, mismatch()
		}
		fields := make(map[string]Value, len(ent.Fields))
		for _, f := range ent.Fields {
			raw, present := obj[f.JSONKey()]
			if !present {
				return Value{}, in.jsonParseFail(rtmsg.JSONFieldMissing(f.JSONKey()))
			}
			v, err := in.jsonExtract(raw, f.Type)
			if err != nil {
				return Value{}, err
			}
			fields[f.Name] = v
		}
		return Value{Type: target, Obj: &entityObj{typeName: target.Entity, fields: fields}}, nil
	default:
		return Value{}, mismatch()
	}
}
