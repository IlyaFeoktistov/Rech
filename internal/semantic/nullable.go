package semantic

import (
	"fmt"

	"github.com/IlyaFeoktistov/Rech/internal/ast"
	"github.com/IlyaFeoktistov/Rech/internal/ir"
)

// resolveNullableTypeName резолвит "?Тип" (name уже несёт ведущий "?",
// добавленный parser.parseTypeName) - лениво синтезирует entitySig с
// IsNullable/ElemType при первой встрече конкретной инстанциации, тем же
// принципом мемоизации, что и resolveGenericTypeName/resolveDictTypeName.
func resolveNullableTypeName(entities map[string]entitySig, methods map[string]map[string]funcSig, aliases map[string]string, interfaces map[string]interfaceSig, enums map[string]map[string]int64, name string) (ir.Type, error) {
	innerName := name[len("?"):]
	if innerName == "" {
		return ir.TypeUnknown, fmt.Errorf("%q: после \"?\" ожидалось имя типа", name)
	}
	inner, err := resolveTypeName(entities, methods, aliases, interfaces, enums, innerName)
	if err != nil {
		return ir.TypeUnknown, fmt.Errorf("тип %q: %w", name, err)
	}
	if inner.Kind == ir.KindNullable {
		return ir.TypeUnknown, fmt.Errorf("%q: nullable-тип нельзя обернуть повторно (\"??Тип\")", name)
	}
	if _, exists := entities[name]; !exists {
		entities[name] = entitySig{IsNullable: true, ElemType: inner}
	}
	return ir.Type{Kind: ir.KindNullable, Entity: name}, nil
}

// registerOptionalStringOnCall - регистрирует entitySig "?Строка" лениво,
// при первом РЕАЛЬНОМ вызове одной из функций, возвращающих "?Строка"
// (`получить_переменную_окружения`/`значение_заголовка`/`значение_куки`) -
// общая точка для всех трёх (тем же принципом, что registerSocketNullableOnCall,
// network.go, у "принять") вместо трёх копий одного и того же тела в
// env.go/httpheaders.go/cookies.go. НАЙДЕННЫЙ структурный риск (docs/
// "Архитектура компилятора - структурные риски") - раньше все
// три функции регистрировали "?Строка" БЕЗУСЛОВНО при настройке
// анализатора (до разбора программы), значит "?Строка" попадал в
// `Program.Nullables` для КАЖДОЙ программы, даже без единого вызова этих
// функций - для "?Строка" это было безобидно (её C-представление,
// `const char *`, никогда не эмитируется условно, в отличие от
// `rech_socket_t`/подобных - см. registerSocketNullableOnCall), но сам
// механизм ("по факту регистрации", не "по факту использования") -
// потенциальный источник той же ошибки для БУДУЩЕГО builtin'а. Исправлено
// тем же путём, что и "?Сокет".
func registerOptionalStringOnCall(entities map[string]entitySig, methods map[string]map[string]funcSig, aliases map[string]string, interfaces map[string]interfaceSig, enums map[string]map[string]int64) error {
	_, err := resolveTypeName(entities, methods, aliases, interfaces, enums, "?Строка")
	return err
}

// registerOptionalAnyOnCall - тот же приём, что registerOptionalStringOnCall
// выше, но для "?Значение" ("Значение".получить_поле_по_тегу,
// internal/semantic/reflect.go, anyMethodSigs) - в отличие от "?Целое"
// (обёртка поля "код" встроенного "Исключение", зарегистрирована
// безусловно при настройке анализатора, см. analyzer.go) "?Значение" не
// нужен НИ ОДНОЙ другой части языка, поэтому регистрируется лениво, ровно
// тем же путём, что и "?Строка"/"?Сокет" - вызывается из
// analyzeMethodCallExpr (expr.go) при каждом реальном вызове.
func registerOptionalAnyOnCall(entities map[string]entitySig, methods map[string]map[string]funcSig, aliases map[string]string, interfaces map[string]interfaceSig, enums map[string]map[string]int64) error {
	_, err := resolveTypeName(entities, methods, aliases, interfaces, enums, "?Значение")
	return err
}

// optionalStringFuncNames - имена всех функций, возвращающих "?Строка" через
// registerOptionalStringOnCall (analyzeCallExpr, expr.go) - тем же принципом
// lookup-таблицы, что и dbFuncNames (db.go), вместо трёх отдельных "if
// callee ==".
var optionalStringFuncNames = map[string]bool{
	"получить_переменную_окружения": true,
	"значение_заголовка":            true,
	"значение_куки":                 true,
	"http2_заголовок":               true,
}

// nonNullState - синтетическое имя "состояния" (в смысле established, см.
// internal/semantic/ctx.go), которым отслеживается доказанная непустота
// nullable-переменной внутри "если а != Ничто {...}" - переиспользует
// СУЩЕСТВУЮЩИЙ established-механизм typestate (шаг 5/6.4) целиком
// (снапшот/пересечение веток если/иначе, сброс после цикла), а не отдельную
// параллельную структуру: имя начинается с "$", которое не может встретиться
// как настоящее имя состояния сущности (недопустимый первый символ
// идентификатора Rech), поэтому коллизии в принципе исключены.
const nonNullState = "$непусто"

// markNonNull записывает в established, что переменная name (простое имя -
// как и у "становится", см. BecomeStmt) доказанно не Ничто в текущей
// (обычно только что открытой push()) области видимости.
func markNonNull(sc *ctx, name string) {
	next := cloneStates(sc.getEstablished(name))
	next[nonNullState] = true
	sc.setEstablished(name, next)
}

// isNonNull - true, если value - простой идентификатор, established
// которого уже содержит nonNullState (см. markNonNull). Любая другая форма
// выражения (поле, вызов, "этот" и т. п.) - не отслеживается, тем же
// осознанным ограничением, что и established typestate вообще
// (establishedStatesOf).
func isNonNull(sc *ctx, value ir.Expr) bool {
	ident, ok := value.(*ir.Identifier)
	if !ok {
		return false
	}
	return sc.getEstablished(ident.Name)[nonNullState]
}

// nullableInner возвращает внутренний Тип у уже резолвленного "?Тип" t
// (t.Kind == KindNullable).
func nullableInner(sc *ctx, t ir.Type) ir.Type {
	return sc.entities[t.Entity].ElemType
}

// maybeUnwrapNarrowed - если value имеет nullable-тип и доказанно непусто
// (isNonNull), заворачивает его в ir.NullUnwrapExpr (тот же узел, что и у
// явного каста, см. package doc ir.NullUnwrapExpr) - иначе возвращает value
// без изменений, оставляя вызывающей стороне её обычную ошибку
// несовместимости типов ("оператор не поддерживается для типа ?Тип" и т.
// п. - уже достаточно информативно само по себе). Вызывается в каждой
// точке, где значение употребляется как обычный Тип (арифметика/сравнение/
// получатель поля или метода/аргумент вызова), НЕ в "а == Ничто"/"а ?? б" -
// там как раз нужен исходный nullable-тип.
func maybeUnwrapNarrowed(sc *ctx, value ir.Expr) ir.Expr {
	typ := ir.TypeOf(value)
	if typ.Kind != ir.KindNullable || !isNonNull(sc, value) {
		return value
	}
	return &ir.NullUnwrapExpr{Value: value, Type: nullableInner(sc, typ)}
}

// coerceExpr - единственная точка, которая понимает "Ничто" и неявное
// заворачивание/разворачивание nullable-значений в контексте с уже
// известным ожидаемым типом target (объявление с аннотацией/присваивание
// переменной или полю/аргумент вызова/"вернуть"/аргумент конструктора):
//   - "Ничто" (ast.NullLiteral) допустим, только если target - nullable -
//     сам литерал получает Type = target (см. package doc ir.NullLiteral).
//   - значение базового типа, совпадающего с внутренним типом target -
//     заворачивается в ir.NullWrapExpr.
//   - доказанно непустое nullable-значение (см. maybeUnwrapNarrowed) на
//     месте небазового target разворачивается тем же путём, что и везде.
//
// В остальных случаях возвращает проанализированное значение как есть -
// вызывающая сторона делает свою обычную проверку isAssignable(target,
// TypeOf(value)) и сообщает свою же, более специфичную ошибку типа при
// несовпадении.
func coerceExpr(sc *ctx, e ast.Expr, target ir.Type) (ir.Expr, error) {
	if nl, ok := e.(*ast.NullLiteral); ok {
		if target.Kind != ir.KindNullable {
			return nil, fmt.Errorf("строка %d: \"Ничто\" допустимо только для значения nullable-типа (\"?Тип\"), ожидался %s", nl.Line, target)
		}
		return &ir.NullLiteral{Type: target, Line: nl.Line}, nil
	}
	if ll, ok := e.(*ast.ListLiteral); ok && target.Kind == ir.KindEntity {
		if sig, ok := sc.entities[target.Entity]; ok && sig.IsList {
			return analyzeListLiteralExpr(sc, ll, sig.ElemType, true)
		}
	}
	value, err := analyzeExpr(sc, e)
	if err != nil {
		return nil, err
	}
	typ := ir.TypeOf(value)
	if typ == target {
		return value, nil
	}
	if target.Kind == ir.KindNullable && nullableInner(sc, target) == typ {
		return &ir.NullWrapExpr{Value: value, Type: target}, nil
	}
	return maybeUnwrapNarrowed(sc, value), nil
}

// analyzeNullCheck - "<other> == Ничто"/"<other> != Ничто" (isNot=true у
// "!="), вызывается ДО обычного analyzeBinaryExpr именно потому, что
// "Ничто" без контекста типа не разбирается обычным analyzeExpr вообще (см.
// package doc ast.NullLiteral) - other здесь всегда сторона БЕЗ "Ничто".
func analyzeNullCheck(sc *ctx, other ast.Expr, isNot bool, line int) (ir.Expr, error) {
	value, err := analyzeExpr(sc, other)
	if err != nil {
		return nil, err
	}
	if typ := ir.TypeOf(value); typ.Kind != ir.KindNullable {
		return nil, fmt.Errorf("строка %d: сравнение с \"Ничто\" применимо только к значению nullable-типа (\"?Тип\"), получен %s", line, typ)
	}
	return &ir.NullCheckExpr{Value: value, IsNot: isNot, Line: line}, nil
}

// analyzeNullCoalesce - "а ?? б": left уже доказанно nullable (проверяется
// здесь), right - либо тот же nullable-тип (результат остаётся nullable),
// либо его внутренний тип (результат - уже не nullable, "запасное
// значение").
func analyzeNullCoalesce(e *ast.BinaryExpr, sc *ctx, left, right ir.Expr) (ir.Expr, error) {
	lt := ir.TypeOf(left)
	if lt.Kind != ir.KindNullable {
		return nil, fmt.Errorf("строка %d: \"??\" применимо только к nullable-значению слева, получен %s", e.Line, lt)
	}
	inner := nullableInner(sc, lt)
	rt := ir.TypeOf(right)
	switch rt {
	case inner:
		return &ir.NullCoalesceExpr{Left: left, Right: right, Type: inner, Line: e.Line}, nil
	case lt:
		return &ir.NullCoalesceExpr{Left: left, Right: right, Type: lt, Line: e.Line}, nil
	default:
		return nil, fmt.Errorf("строка %d: \"??\" справа ожидает %s или %s, получено %s", e.Line, inner, lt, rt)
	}
}

// analyzeNullCoalesceAssignStmt - "а ?= б" (ast.NullCoalesceAssign) -
// присвоить б переменной а, только если а сейчас Ничто. а обязана быть уже
// объявленной nullable-переменной (не полем, не константой - то же
// ограничение, что у обычного Assign/IncDecExpr).
func analyzeNullCoalesceAssignStmt(sc *ctx, s *ast.NullCoalesceAssign) (ir.Stmt, error) {
	declared, exists := sc.lookup(s.Name)
	if !exists {
		return nil, fmt.Errorf("строка %d: переменная %q не объявлена", s.Line, s.Name)
	}
	if declared.Kind != ir.KindNullable {
		return nil, fmt.Errorf("строка %d: \"?=\" применимо только к nullable-переменной (\"?Тип\"), %q имеет тип %s", s.Line, s.Name, declared)
	}
	if sc.isConst(s.Name) {
		return nil, fmt.Errorf("строка %d: %q объявлено как \"константа\" - нельзя присвоить новое значение", s.Line, s.Name)
	}
	value, err := coerceExpr(sc, s.Value, declared)
	if err != nil {
		return nil, err
	}
	if typ := ir.TypeOf(value); !isAssignable(sc.entities, sc.interfaces, typ, declared) {
		return nil, fmt.Errorf("строка %d: переменной %q типа %s нельзя присвоить значение типа %s", s.Line, s.Name, declared, typ)
	}
	return &ir.NullCoalesceAssignStmt{Name: s.Name, Value: value, Line: s.Line}, nil
}
