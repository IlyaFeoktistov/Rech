package cbackend

import (
	"fmt"
	"strings"

	"github.com/IlyaFeoktistov/Rech/internal/ir"
)

// dbRowsRuntimeCode - C-функции "маппинг строки результата запроса в
// сущность" (rech_db_rows_to_<Тип>), по одной на каждый элемент
// prog.DBRowTargets (internal/semantic/db_rows.go) - см. package doc
// ir.DBRowsAsExpr. Сигнатура - ResolveFn (ir_generator.go,
// resolveRuntimeCode), тем же принципом, что jsonRuntimeCode.
//
// Каждая функция берёт rows (уже готовый "Список<Словарь<Строка,
// Значение>>", результат "запросить"/"выполнить"), строит новый
// "Список<Тип>" и для каждой строки - новый экземпляр Тип (rech_new_<Тип>,
// без аргументов - dbRowEntityCompatible в internal/semantic уже
// гарантировала отсутствие собственного "конструктор(...)"), заполняя поля
// одно за другим значением из строки по колонке (имя поля или тег
// `sql:...`, ir.Field.SQLKey). Обычное поле распаковывается уже
// существующим reflectCheckedUnboxExprCode (та же рантайм-проверка типа и
// паника при несовпадении, что и у "значение как Тип" - NULL-колонка
// (боксирована как "Ничто") паникует здесь точно так же, как несовпадение
// любого другого типа - потому что поле НЕ nullable, значит NULL для него
// в принципе неожиданное значение). Nullable-поле - отдельная ветка:
// сравнивает боксированный тип строки с дескриптором "Ничто" (reflectMetaVarFor
// ir.TypeNullValue) НАПРЯМУЮ (без паники) и заворачивает/пустит
// соответствующий "?T" (nullableCollectionCCode, _wrap/_empty).
func dbRowsRuntimeCode(ctx *irGenCtx, prog *ir.Program, targets []ir.Type) (string, error) {
	entities := make(map[string]*ir.Entity, len(prog.Entities))
	for _, ent := range prog.Entities {
		entities[ent.Name] = ent
	}
	rowsC := mangleType(dbResultType)
	dictGet := mangleMethod(dbRowType, "получить")
	nullMeta := reflectMetaVarFor(ir.TypeNullValue)

	var sb strings.Builder
	for _, t := range targets {
		ent, ok := entities[t.Entity]
		if !ok {
			return "", fmt.Errorf("генерация C из IR: \"запросить(...) как Список<%s>\" в неизвестный тип", t.Entity)
		}
		listName := "Список<" + ent.Name + ">"
		listC := mangleType(listName)
		listCtor := mangleCtor(listName)
		listAdd := mangleMethod(listName, "добавить")
		itemC := fromIRType(ir.EntityType(ent.Name)).cType(ctx)
		itemCtor := mangleCtor(ent.Name)
		fn := "rech_db_rows_to_" + transliterate(ent.Name)

		var body strings.Builder
		for i, f := range ent.Fields {
			key := escapeCString(f.SQLKey())
			rowGet := fmt.Sprintf("%s(row, \"%s\")", dictGet, key)
			if f.Type.Kind == ir.KindNullable {
				nullableC := mangleType(f.Type.Entity)
				wrap := nullableC + "_wrap"
				empty := nullableC + "_empty"
				inner := ctx.nullableInner[f.Type.Entity]
				valVar := fmt.Sprintf("v%d", i)
				fmt.Fprintf(&body, "        rech_any_t %s = %s;\n", valVar, rowGet)
				fmt.Fprintf(&body, "        item->%s = (%s.type == &%s) ? %s() : %s(%s);\n",
					mangleField(f.Name), valVar, nullMeta, empty, wrap, reflectCheckedUnboxExprCode(ctx, inner, valVar))
			} else {
				fmt.Fprintf(&body, "        item->%s = %s;\n", mangleField(f.Name), reflectCheckedUnboxExprCode(ctx, f.Type, rowGet))
			}
		}

		fmt.Fprintf(&sb, `static %[1]s *%[2]s(%[3]s *rows) {
    %[1]s *out = %[4]s();
    for (long long i = 0; i < rows->len; i++) {
        %[6]s *row = rows->data[i];
        %[5]s item = %[7]s();
%[8]s        %[9]s(out, item);
    }
    return out;
}

`, listC, fn, rowsC, listCtor, itemC, mangleType(dbRowType), itemCtor, body.String(), listAdd)
	}
	return sb.String(), nil
}

// dbRowsFuncName - имя сгенерированной функции маппинга для elemEntity
// (ent.Name), см. package doc dbRowsRuntimeCode выше - согласовано с "fn"
// там (тот же transliterate(ent.Name)).
func dbRowsFuncName(elemEntity string) string {
	return "rech_db_rows_to_" + transliterate(elemEntity)
}

// generateIRDBRowsAsExpr - "запросить(...) как Список<Тип>" (ir.DBRowsAsExpr) -
// вызов уже сгенерированной dbRowsRuntimeCode-функции для elemEntity.
func generateIRDBRowsAsExpr(ctx *irGenCtx, e *ir.DBRowsAsExpr) (string, valueType, error) {
	code, _, err := generateIRExpr(ctx, e.Rows)
	if err != nil {
		return "", typeUnknown, err
	}
	return fmt.Sprintf("%s(%s)", dbRowsFuncName(e.Elem.Entity), code), fromIRType(e.To), nil
}
