package semantic

import (
	"fmt"

	"github.com/IlyaFeoktistov/Rech/internal/ast"
	"github.com/IlyaFeoktistov/Rech/internal/ir"
)

// visibilityAdjective - "приватный"/"защищённый" с окончанием, согласованным
// по роду со словом-контейнером (kindEnding): "ое" у "поле" (средний род),
// "ая" у "константа" (женский), "ый" у "метод" (мужской) - та же схема
// окончаний, что и в internal/parser (visibilityKeywordFor), но здесь нет
// нужды в согласовании с ИМЕНЕМ поля (сообщение об ошибке всегда называет
// поле/константу/метод словом-родом, не собственным именем).
func visibilityAdjective(vis ast.Visibility, kindEnding string) string {
	stem := "приватн"
	if vis == ast.Protected {
		stem = "защищённ"
	}
	return stem + kindEnding
}

// checkVisibility - общая проверка доступа к члену сущности (поле/
// константа/метод) снаружи. Public - всегда можно. Private - только из
// метода той же сущности (sc.thisEntity == entityName). Protected - из
// метода той же сущности ИЛИ метода сущности, которая её наследует
// (напрямую или транзитивно) - членов базовой сущности касается
// содержательно, поэтому подходит и метод самого entityName, и метод
// любого потомка. kindWord/kindEnding/actionWord параметризуют разницу
// между полем/константой/методом (см. обёртки ниже) - kindWord -
// существительное для сообщения, kindEnding - окончание прилагательного
// "приватный"/"защищённый", согласованное по роду с kindWord, actionWord -
// что запрещено ("доступ" у поля/константы, "вызов" у метода).
func checkVisibility(sc *ctx, kindWord, kindEnding, actionWord, entityName, memberName string, vis ast.Visibility, line int) error {
	if vis == ast.Public || sc.thisEntity == entityName {
		return nil
	}
	if vis == ast.Protected && entityIsOrExtends(sc.entities, sc.thisEntity, entityName) {
		return nil
	}
	return fmt.Errorf("строка %d: %s %q сущности %q %s - %s только из методов самой сущности%s", line, kindWord, memberName, entityName,
		visibilityAdjective(vis, kindEnding), actionWord, protectedHint(vis))
}

// protectedHint - у "защищённого" доступ шире, чем у "приватного" (не
// только сама сущность, но и её наследники) - сообщение об ошибке должно
// об этом говорить, а не молча повторять текст, рассчитанный на "приватный".
func protectedHint(vis ast.Visibility) string {
	if vis == ast.Protected {
		return " или сущности, которая её наследует"
	}
	return ""
}

func checkFieldVisibility(sc *ctx, entityName, fieldName string, vis ast.Visibility, line int) error {
	return checkVisibility(sc, "поле", "ое", "доступ", entityName, fieldName, vis, line)
}

func checkConstVisibility(sc *ctx, entityName, constName string, vis ast.Visibility, line int) error {
	return checkVisibility(sc, "константа", "ая", "доступ", entityName, constName, vis, line)
}

func checkStaticFieldVisibility(sc *ctx, entityName, fieldName string, vis ast.Visibility, line int) error {
	return checkVisibility(sc, "статическое поле", "ое", "доступ", entityName, fieldName, vis, line)
}

// resolveEntityConstant возвращает значение константы entityName.constName.
// Значение вычисляется РОВНО один раз на весь Analyze: результат кладётся в
// sc.resolvedConsts (общая для всей программы карта, см. newCtx) и на
// повторных обращениях просто читается оттуда - без этого выражение
// константы (если это не литерал, а что-то посложнее) вычислялось бы заново
// на каждое обращение "Сущность.Константа"/"экземпляр.Константа".
//
// sc.resolvingConsts защищает от циклической ссылки константы саму на себя
// (напрямую или через цепочку других констант, в т.ч. чужих сущностей):
// ключ ставится перед рекурсивным analyzeExpr и снимается сразу после -
// если тот же ключ встретится повторно, пока он ещё не снят, значит
// вычисление зациклилось - без этой проверки analyzeExpr вызывал бы сам
// себя без остановки и падал стек-оверфлоу самого компилятора вместо
// понятной ошибки.
//
// Видимость здесь не проверяется - это забота вызывающей стороны (обеих
// точек доступа в analyzeExpr выше): при построении ir.Entity.Constants
// для собственной сущности видимость проверять не нужно (сущность имеет
// право использовать собственную приватную константу для вычисления
// другой своей константы).
func resolveEntityConstant(sc *ctx, entityName, constName string, line int) (ir.Expr, error) {
	key := entityName + "." + constName
	if v, ok := sc.resolvedConsts[key]; ok {
		return v, nil
	}
	if sc.resolvingConsts[key] {
		return nil, fmt.Errorf("строка %d: константа %q.%q ссылается сама на себя (прямо или через другую константу) - вычислить значение невозможно", line, entityName, constName)
	}
	sig := sc.entities[entityName]
	idx := sig.constIndex(constName)
	sc.resolvingConsts[key] = true
	val, err := analyzeExpr(sc, sig.ConstExprs[idx])
	delete(sc.resolvingConsts, key)
	if err != nil {
		return nil, fmt.Errorf("строка %d: при вычислении константы %q.%q: %w", line, entityName, constName, err)
	}
	sc.resolvedConsts[key] = val
	return val, nil
}

// resolveModuleConstant - то же самое, что resolveEntityConstant, но для
// константы уровня МОДУЛЯ (ast.Program.Consts, sc.a.moduleConstExprs) -
// ключ кэша resolvedConsts/resolvingConsts - голое имя константы (без
// точки, тем же общим кэшем - см. package doc Analyzer.moduleConstExprs).
// name - точно из sc.a.moduleConstExprs (вызывающая сторона -
// analyzeExpr, *ast.Identifier - уже проверила наличие ключа).
func resolveModuleConstant(sc *ctx, name string, line int) (ir.Expr, error) {
	if v, ok := sc.resolvedConsts[name]; ok {
		return v, nil
	}
	if sc.resolvingConsts[name] {
		return nil, fmt.Errorf("строка %d: константа %q уровня модуля ссылается сама на себя (прямо или через другую константу) - вычислить значение невозможно", line, name)
	}
	sc.resolvingConsts[name] = true
	val, err := analyzeExpr(sc, sc.a.moduleConstExprs[name])
	delete(sc.resolvingConsts, name)
	if err != nil {
		return nil, fmt.Errorf("строка %d: при вычислении константы %q уровня модуля: %w", line, name, err)
	}
	sc.resolvedConsts[name] = val
	return val, nil
}

// resolveFieldDefault возвращает значение по умолчанию поля entityName.
// fieldName (см. ast.Field.Default) - тем же принципом, что и
// resolveEntityConstant: вычисляется через analyzeExpr РОВНО один раз на
// весь Analyze, результат кэшируется в sc.a.resolvedFieldDefaults и
// переиспользуется на каждый вызов "новый Тип(...)", пропустивший это поле
// (тот же resolved ir.Expr подставляется в ir.NewExpr.Args каждого такого
// места). sc передаётся как есть (та область видимости, откуда впервые
// потребовалось значение) - thisEntity в ней может быть пустым или чужим,
// значения это не меняет: выражение по умолчанию не видит "этот" и не
// ссылается на другие поля этой же сущности синтаксически (поле - не
// переменная в scopes), только на константы (в т.ч. через "Сущность.
// КОНСТАНТА").
func resolveFieldDefault(sc *ctx, entityName, fieldName string, line int) (ir.Expr, error) {
	key := entityName + "." + fieldName
	if v, ok := sc.a.resolvedFieldDefaults[key]; ok {
		return v, nil
	}
	sig := sc.entities[entityName]
	idx := sig.fieldIndex(fieldName)
	// coerceExpr, не analyzeExpr напрямую - значение по умолчанию nullable-
	// поля правомерно записано как "Ничто" ("имеет: поле: ?Тип = Ничто"),
	// а "Ничто" вообще не типизируется без контекста ожидаемого типа
	// (см. package doc ast.NullLiteral) - тем же принципом, что и у любого
	// другого места языка, ожидающего значение конкретного типа (аргумент
	// вызова/присваивание/"вернуть").
	val, err := coerceExpr(sc, sig.FieldDefaultExprs[idx], sig.FieldTypes[idx])
	if err != nil {
		return nil, fmt.Errorf("строка %d: значение по умолчанию поля %q сущности %q: %w", line, fieldName, entityName, err)
	}
	if typ := ir.TypeOf(val); !isAssignable(sc.entities, sc.interfaces, typ, sig.FieldTypes[idx]) {
		return nil, fmt.Errorf("строка %d: значение по умолчанию поля %q сущности %q должно быть типа %s, получено %s", line, fieldName, entityName, sig.FieldTypes[idx], typ)
	}
	sc.a.resolvedFieldDefaults[key] = val
	return val, nil
}

// resolveStaticFieldDefault - как resolveFieldDefault, но для статического
// поля (entitySig.StaticField*) - используется РОВНО один раз, при сборке
// ir.Entity.StaticFields (см. analyzeEntity), не при каждом "новый Тип(...)"
// (у статического поля нет отдельного экземпляра, которому подставлять
// значение) - поэтому не нуждается в собственном кэше, в отличие от
// resolveFieldDefault.
func resolveStaticFieldDefault(sc *ctx, entityName, fieldName string, line int) (ir.Expr, error) {
	sig := sc.entities[entityName]
	idx := sig.staticFieldIndex(fieldName)
	val, err := coerceExpr(sc, sig.StaticFieldDefaultExprs[idx], sig.StaticFieldTypes[idx])
	if err != nil {
		return nil, fmt.Errorf("строка %d: значение по умолчанию статического поля %q сущности %q: %w", line, fieldName, entityName, err)
	}
	if typ := ir.TypeOf(val); !isAssignable(sc.entities, sc.interfaces, typ, sig.StaticFieldTypes[idx]) {
		return nil, fmt.Errorf("строка %d: значение по умолчанию статического поля %q сущности %q должно быть типа %s, получено %s", line, fieldName, entityName, sig.StaticFieldTypes[idx], typ)
	}
	return val, nil
}

// checkMethodVisibility - то же самое для вызова непубличного метода.
func checkMethodVisibility(sc *ctx, entityName, methodName string, vis ast.Visibility, line int) error {
	return checkVisibility(sc, "метод", "ый", "вызов", entityName, methodName, vis, line)
}

// scope - символьная таблица одной области видимости: имя переменной ->
// тип.
