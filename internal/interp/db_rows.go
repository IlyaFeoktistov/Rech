// "запросить(...) как Список<Тип>"/"выполнить(...) как Список<Тип>"
// (internal/ir.DBRowsAsExpr, internal/semantic/db_rows.go) - маппинг
// каждой строки результата (Словарь<Строка, Значение>) в новый экземпляр
// Тип по имени поля/тегу `sql:...` (ir.Field.SQLKey) - тем же принципом,
// что jsonExtract (json.go), но источник - уже готовые Value (никакого
// текста для разбора), поэтому проще: колонка читается прямо из
// entityObj.coll.pairs, без промежуточного any. См. package doc
// internal/backend/cbackend/db_rows.go для эквивалентной реализации на C.
package interp

import (
	"strings"

	"github.com/IlyaFeoktistov/Rech/internal/decimal"
	"github.com/IlyaFeoktistov/Rech/internal/ir"
	"github.com/IlyaFeoktistov/Rech/internal/numfmt"
	"github.com/IlyaFeoktistov/Rech/internal/rtmsg"
)

// dbNullableInnerType - внутренний тип "?Тип" nullableEntity (её
// каноническое имя, "?Целое" и т. п.) - internal/semantic.dbRowFieldTypeOK
// уже ограничила его РОВНО этими четырьмя (не Дробное - см. её package
// doc), поэтому простой switch по имени без обращения к какой-либо общей
// таблице типов (в отличие от cbackend, у interp такой таблицы просто нет -
// Env/Value достаточно динамического Type у самого значения).
func dbNullableInnerType(nullableEntity string) ir.Type {
	switch strings.TrimPrefix(nullableEntity, "?") {
	case "Целое":
		return ir.TypeInt
	case "Действительное":
		return ir.TypeFloat
	case "Строка":
		return ir.TypeString
	case "Логическое":
		return ir.TypeBool
	case "Десятичное":
		return ir.TypeDecimal
	default:
		return ir.Type{}
	}
}

func (in *Interp) evalDBRowsAsExpr(e *ir.DBRowsAsExpr, env *Env, cc *callCtx) (Value, error) {
	rows, err := in.evalExpr(e.Rows, env, cc)
	if err != nil {
		return Value{}, err
	}
	ent := in.entities[e.Elem.Entity]
	elems := make([]Value, len(rows.Obj.coll.elems))
	for i, row := range rows.Obj.coll.elems {
		item, err := in.dbRowToEntity(row, ent)
		if err != nil {
			return Value{}, err
		}
		elems[i] = item
	}
	return Value{Type: e.To, Obj: &entityObj{typeName: e.To.Entity, coll: &collection{template: "Список", elems: elems}}}, nil
}

// dbRowToEntity строит один экземпляр ent из row (Словарь<Строка,
// Значение>, одна строка результата запроса) - panicRuntime при
// несовпадении фактического типа боксированной колонки с объявленным
// типом поля, тем же сообщением (rtmsg.AsTypeMismatchFormat), что и обычное
// "значение как Тип" (evalAsExpr, reflect.go) - тот же механизм
// разбоксирования, просто по одному полю за раз вместо одного явного
// выражения.
func (in *Interp) dbRowToEntity(row Value, ent *ir.Entity) (Value, error) {
	fields := make(map[string]Value, len(ent.Fields))
	for _, f := range ent.Fields {
		key := stringValue(f.SQLKey())
		boxed, ok := row.Obj.coll.pairs.get(key)
		if !ok {
			panicRuntimeAs(rtmsg.PanicKeyNotFound, "%s", rtmsg.DictKeyNotFound(row.Type.String()))
		}
		if f.Type.Kind == ir.KindNullable {
			if boxed.Boxed.Type.Kind == ir.KindNull {
				fields[f.Name] = Value{Type: f.Type}
				continue
			}
			inner := dbNullableInnerType(f.Type.Entity)
			if inner == ir.TypeDecimal {
				d := dbDecimalFromBoxed(*boxed.Boxed)
				fields[f.Name] = Value{Type: f.Type, Nullable: &d}
				continue
			}
			if boxed.Boxed.Type != inner {
				panicRuntimeAs(rtmsg.PanicBadCast, "%s", rtmsg.AsTypeMismatchFormat(boxed.Boxed.Type.String(), inner.String()))
			}
			unwrapped := *boxed.Boxed
			fields[f.Name] = Value{Type: f.Type, Nullable: &unwrapped}
			continue
		}
		if f.Type == ir.TypeDecimal {
			fields[f.Name] = dbDecimalFromBoxed(*boxed.Boxed)
			continue
		}
		if boxed.Boxed.Type != f.Type {
			panicRuntimeAs(rtmsg.PanicBadCast, "%s", rtmsg.AsTypeMismatchFormat(boxed.Boxed.Type.String(), f.Type.String()))
		}
		fields[f.Name] = *boxed.Boxed
	}
	return Value{Type: ir.EntityType(ent.Name), Obj: &entityObj{typeName: ent.Name, fields: fields}}, nil
}

// dbDecimalFromBoxed - колонка в поле Десятичное (cbackend
// rech_db_dec_from_any - то же правило): Целое и Строка - точно,
// Действительное - через кратчайшую запись.
func dbDecimalFromBoxed(v Value) Value {
	switch v.Type {
	case ir.TypeDecimal:
		return v
	case ir.TypeInt:
		return decimalValue(decimal.FromInt(v.I))
	case ir.TypeFloat:
		return decimalResult(decimal.Parse(numfmt.Float(v.F, 64)))
	case ir.TypeString:
		if d, err := decimal.Parse(v.S); err == nil {
			return decimalValue(d)
		}
	}
	panicRuntimeAs(rtmsg.PanicBadCast, "%s", rtmsg.AsTypeMismatchFormat(v.Type.String(), ir.TypeDecimal.String()))
	return Value{}
}
