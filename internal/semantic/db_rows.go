// Package semantic: "запросить(...) как Список<Тип>"/"выполнить(...) как
// Список<Тип>" (docs/SPEC.md, "БД") - типизированный маппинг результата
// запроса (Список<Словарь<Строка, Значение>>, dbResultType) в Список
// пользовательских сущностей, по имени поля или его тегу `sql:...`
// (внешний ключ колонки, отличный от имени поля - тот же принцип, что и
// `json:...` у "разобрать JSON", parse.go). Обрабатывается ветвлением
// внутри analyzeAsExpr (reflect.go) - тот же синтаксис "<значение> как
// <Тип>", что и у разбоксирования "Значение", но по-другому: источник -
// Список<Словарь<Строка, Значение>>, не единственное боксированное
// значение, и результат - Список<Тип>, не Тип напрямую.
package semantic

import (
	"github.com/IlyaFeoktistov/Rech/internal/diagnostics"
	"github.com/IlyaFeoktistov/Rech/internal/ir"
	"github.com/IlyaFeoktistov/Rech/internal/structtag"
)

// sqlFieldKey - внешний ключ колонки поля i сущности sig: значение тега
// "sql:..." (internal/structtag), если оно задано, иначе - само имя поля -
// тем же принципом, что jsonFieldKey (parse.go).
func sqlFieldKey(sig entitySig, i int) string {
	if i < len(sig.FieldTags) {
		if key := structtag.Parse(sig.FieldTags[i]).Get("sql"); key != "" {
			return key
		}
	}
	return sig.FieldNames[i]
}

// checkSQLKeysDistinct - тем же принципом, что checkJSONKeysDistinct
// (parse.go), но по тегу `sql:...`/имени поля - два поля не могут
// одновременно целиться в одну и ту же колонку результата запроса.
func checkSQLKeysDistinct(sig entitySig) error {
	seen := make(map[string]string, len(sig.FieldNames))
	for i, name := range sig.FieldNames {
		key := sqlFieldKey(sig, i)
		if other, dup := seen[key]; dup {
			return errNoPos(CodeBuiltinTarget, "поля %q и %q конфликтуют - у обоих одинаковая колонка результата запроса %q (проверьте теги `sql:...`)", other, name, key)
		}
		seen[key] = name
	}
	return nil
}

// dbRowFieldTypeOK - t допустим полем цели "запросить(...) как Список<T>" -
// примитив либо nullable-примитив (NULL колонки результата - "Ничто" в
// боксированном "Значение", package doc db.go) - СТРОЖЕ, чем
// structCompatible у JSON/XML (parse.go) в двух отношениях: (1) строка
// результата SQL-запроса плоская, без вложенных сущностей/списков -
// вложенность там просто неоткуда взяться (каждая колонка - один
// боксированный примитив); (2) "Дробное" (32-битное) НЕ входит - обе
// реализации (internal/backend/cbackend/db.go, internal/interp/db.go)
// боксируют числовую колонку результата ВСЕГДА как "Действительное"
// (.v.f64/float64, ширина самого SQL-драйвера/протокола) - поле "Дробное"
// стабильно не совпадало бы с боксированным типом и паниковало бы на
// КАЖДОЙ строке (сравнение типа "как"/AsExpr - точное, не приводящее).
func dbRowFieldTypeOK(entities map[string]entitySig, t ir.Type) bool {
	switch t.Kind {
	case ir.KindInt, ir.KindFloat, ir.KindString, ir.KindBool, ir.KindDecimal:
		return true
	case ir.KindNullable:
		switch entities[t.Entity].ElemType.Kind {
		case ir.KindInt, ir.KindFloat, ir.KindString, ir.KindBool, ir.KindDecimal:
			return true
		}
	}
	return false
}

// dbRowEntityCompatible проверяет, что t годится ЭЛЕМЕНТОМ цели
// "запросить(...) как Список<T>" - обычная (не внешняя, не встроенная
// коллекция) сущность/структура БЕЗ явного конструктора (результат
// заполняет поля напрямую, тем же принципом, что и structCompatible для
// JSON - см. её package doc про несовместимость с пользовательским
// "конструктор(...)"), все поля - примитив/nullable-примитив
// (dbRowFieldTypeOK), колонки различны (checkSQLKeysDistinct).
func dbRowEntityCompatible(entities map[string]entitySig, t ir.Type) error {
	if t.Kind != ir.KindEntity {
		return errNoPos(CodeBuiltinTarget, "%s не поддержан как цель результата запроса (только сущность/структура)", t)
	}
	sig, ok := entities[t.Entity]
	if !ok {
		return errNoPos(CodeNameResolution, "тип %s не найден", t)
	}
	if sig.IsList || sig.IsDict || sig.IsExternOpaque || sig.IsExternStruct || sig.IsNullable || sig.IsPointer {
		return errNoPos(CodeBuiltinTarget, "%s не поддержан как цель результата запроса (только сущность/структура с примитивными полями)", t)
	}
	if sig.HasCtor {
		return errNoPos(CodeBuiltinTarget, "%s не поддержан как цель результата запроса: у сущности есть собственный \"конструктор(...)\" - маппинг заполняет поля напрямую, а не вызывает пользовательский конструктор с неизвестным ему набором аргументов", t)
	}
	for i, ft := range sig.FieldTypes {
		if !dbRowFieldTypeOK(entities, ft) {
			return errNoPos(CodeBuiltinTarget, "%s: поле %q имеет тип %s - цель результата запроса допускает только примитивы (Целое/Действительное/Строка/Логическое) и их nullable-варианты (NULL колонки)", t, sig.FieldNames[i], ft)
		}
	}
	if err := checkSQLKeysDistinct(sig); err != nil {
		return diagnostics.Wrap(err, "%s", t)
	}
	return nil
}

// analyzeDBRowsAsExpr - value уже проверено analyzeAsExpr как выражение
// типа dbResultType ("Список<Словарь<Строка, Значение>>", результат
// "запросить"/"выполнить"). to - разрешённый целевой тип ("Список<Тип>" из
// текста "как Список<Тип>"), elem - его ElemType уже проверен вызывающей
// стороной как настоящая пользовательская сущность (не примитив, не
// вложенная встроенная коллекция).
func analyzeDBRowsAsExpr(sc *ctx, value ir.Expr, to, elem ir.Type, line int) (ir.Expr, error) {
	if err := dbRowEntityCompatible(sc.entities, elem); err != nil {
		return nil, diagnostics.WrapAt(err, diagnostics.Position{File: currentFile, Line: line}, "\"как %s\"", to)
	}
	// markUsesReflection - уже безусловно взведён самим фактом, что value
	// вообще может иметь тип dbResultType (registerDBTypesOnCall у любой из
	// четырёх функций БД, db.go) - вызывается здесь ещё раз для полноты
	// (идемпотентно), не потому что это первое место, где это нужно.
	sc.a.markUsesReflection()
	sc.a.registerDBRowTarget(elem)
	return &ir.DBRowsAsExpr{Rows: value, To: to, Elem: elem}, nil
}
