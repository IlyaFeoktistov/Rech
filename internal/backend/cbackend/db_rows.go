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
	sb.WriteString(dbDecimalFromAnyCode())
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
		fn := "rech_db_rows_to_" + transliterate(ent.Name)
		// isRef - "сущность"/"класс" (item - указатель, GC_MALLOC'нутый
		// через rech_new_X, поля - "item->поле") против "структура" (item -
		// значение на стеке, у неё нет rech_new_X вовсе - структуры строятся
		// только compound-literal'ом в месте вызова, см. package doc пакета
		// generateIRNewExpr - зануляется "= {0}" здесь, безопасно, потому что
		// КАЖДОЕ поле явно присваивается ниже, ни одно не остаётся нулевым
		// байтом дольше одной строки этой функции; поля - "item.поле").
		// dbRowEntityCompatible (internal/semantic/db_rows.go) разрешает
		// ОБА - раньше здесь была только ветка для сущности (rech_new_X() +
		// "->"), для структуры это компилировалось в невалидный C
		// (несуществующий rech_new_X, "->" на значении, не указателе) - живая
		// проверка нашла это на examples/бд/orm_легковесный.rech.
		isRef := ctx.entityIsRef(ent.Name)
		access := "."
		itemInit := itemC + " item = {0};"
		if isRef {
			access = "->"
			itemInit = itemC + " item = " + mangleCtor(ent.Name) + "();"
		}

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
				fmt.Fprintf(&body, "        item%s%s = (%s.type == &%s) ? %s() : %s(%s);\n",
					access, mangleField(f.Name), valVar, nullMeta, empty, wrap, dbColumnValueCode(ctx, inner, valVar))
			} else {
				fmt.Fprintf(&body, "        item%s%s = %s;\n", access, mangleField(f.Name), dbColumnValueCode(ctx, f.Type, rowGet))
			}
		}

		fmt.Fprintf(&sb, `static %[1]s *%[2]s(%[3]s *rows) {
    %[1]s *out = %[4]s();
    for (long long i = 0; i < rows->len; i++) {
        %[6]s *row = rows->data[i];
        %[5]s
%[7]s        %[8]s(out, item);
    }
    return out;
}

`, listC, fn, rowsC, listCtor, itemInit, mangleType(dbRowType), body.String(), listAdd)
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

// dbColumnValueCode - значение колонки anyExpr для поля типа t:
// Десятичное - преобразованием (rech_db_dec_from_any), остальное - точной
// распаковкой.
func dbColumnValueCode(ctx *irGenCtx, t ir.Type, anyExpr string) string {
	if t == ir.TypeDecimal {
		ctx.usesDecimal = true
		return fmt.Sprintf("rech_db_dec_from_any(%s)", anyExpr)
	}
	return reflectCheckedUnboxExprCode(ctx, t, anyExpr)
}

// dbDecimalFromAnyCode - rech_db_dec_from_any: колонка в поле
// Десятичное. Драйверы отдают числа как Целое или Действительное, текст -
// как Строка: целое и строка - точно, Действительное - через кратчайшую
// запись (interp dbDecimalFromBoxed - то же правило). Иное - паника
// несовпадения типа, как у обычной распаковки.
func dbDecimalFromAnyCode() string {
	return fmt.Sprintf(`static rech_dec rech_db_dec_from_any(rech_any_t v) {
    rech_dec d;
    if (v.type == &%[1]s) return v.v.d;
    if (v.type == &%[2]s) return rech_dec_from_int(v.v.i);
    if (v.type == &%[3]s) return rech_dec_from_double(v.v.f64, 0);
    if (v.type == &%[4]s && rech_dec_parse(v.v.s, &d)) return d;
    return rech_unbox_check(v, &%[1]s).v.d;
}

`, reflectMetaVarFor(ir.TypeDecimal), reflectMetaVarFor(ir.TypeInt), reflectMetaVarFor(ir.TypeFloat), reflectMetaVarFor(ir.TypeString))
}
