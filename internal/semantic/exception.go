// Исключения ("выбросить"/"попытаться"/"поймать", "относится к") - см.
// docs/SPEC.md. Системная сущность "Исключение" засеяна в collectEntities
// (internal/semantic/analyzer.go) до разбора программы - любой её
// наследник проверяется здесь тем же entityIsOrExtends, что и обычная
// наследование/интерфейсы (internal/semantic/inheritance.go).
package semantic

import (
	"fmt"

	"github.com/IlyaFeoktistov/Rech/internal/ast"
	"github.com/IlyaFeoktistov/Rech/internal/ir"
)

// systemExceptionEntity - имя базовой сущности, которую обязаны наследовать
// (напрямую или транзитивно) все типы, участвующие в "выбросить"/"поймать".
const systemExceptionEntity = "Исключение"

// systemExceptionFields - поля базовой "Исключение" ("сообщение",
// "код" - необязательный целочисленный код, "Ничто" по умолчанию,
// docs/SPEC.md) - общая для ir.Entity "Исключение" самой и для КАЖДОГО
// системного наследника (ОшибкаРазбора/ОшибкаДеленияНаНоль/...),
// который не проходит через applyInheritance (см. package doc там) и
// поэтому обязан сам флаттенить унаследованные поля в собственный
// ir.Entity.Fields - единственное место, которое их перечисляет, чтобы не
// разойтись, если состав полей "Исключение" когда-нибудь изменится.
func systemExceptionFields() []ir.Field {
	return []ir.Field{
		{Name: "сообщение", Type: ir.TypeString},
		{Name: "код", Type: ir.NullableType(ir.TypeInt)},
	}
}

// systemExceptionSigFields - то же самое, что systemExceptionFields, в
// форме параллельных массивов entitySig (FieldNames/FieldTypes/
// FieldVisibility), а не []ir.Field - используется при сборке entitySig
// (собственная форма, отдельная от ir.Entity, см. package doc entitySig)
// для "Исключение" самой и для каждого системного наследника.
func systemExceptionSigFields() ([]string, []ir.Type, []ast.Visibility) {
	return []string{"сообщение", "код"},
		[]ir.Type{ir.TypeString, ir.NullableType(ir.TypeInt)},
		[]ast.Visibility{ast.Public, ast.Public}
}

// systemExceptionCtor - конструктор "Исключение(сообщение: Строка)",
// эквивалентный ровно тому, что написал бы пользователь сам ("этот.сообщение
// = сообщение") - в отличие от обычной пользовательской сущности без
// собственного "конструктор(...)" (docs/SPEC.md, "Явные конструкторы"),
// "Исключение" не остаётся с пустым синтетическим - "новое Исключение(...)"
// обязано продолжать работать без единой строки пользовательского кода, тем
// же принципом, что и у `Exception(String message)` в Java/C#. Наследник
// без своего "конструктор" (`сущность ОшибкаХ наследует Исключение {}`)
// этот конструктор НЕ наследует (HasCtor никогда не наследуется, см.
// package doc entitySig.HasCtor) - ему, как и у Java/C#, нужен свой,
// пробрасывающий сообщение через "родитель(сообщение)" - см.
// docs/SPEC.md, "Явные конструкторы".
func systemExceptionCtor() *ir.Method {
	return &ir.Method{
		Params: []ir.Param{{Name: "сообщение", Type: ir.TypeString}},
		Body: []ir.Stmt{
			&ir.FieldAssign{
				Receiver: &ir.ThisExpr{Entity: systemExceptionEntity},
				Field:    "сообщение",
				Value:    &ir.Identifier{Name: "сообщение", Type: ir.TypeString},
			},
		},
	}
}

// divByZeroExceptionEntity - встроенное исключение, выбрасываемое целочисленным
// делением на ноль ("/") - наследник systemExceptionEntity. Регистрируется
// НЕ безусловно (тем же принципом, что и parseErrorEntity, см. package doc
// parse.go, registerParseErrorEntity) - только если программа хоть раз
// использовала целочисленное деление (analyzeArithmetic), иначе любая
// программа без единого "/" платила бы за участие "Исключение" в
// диспетчеризации без всякой причины.
const divByZeroExceptionEntity = "ОшибкаДеленияНаНоль"

// registerDivByZeroException - см. package doc divByZeroExceptionEntity.
func registerDivByZeroException(entities map[string]entitySig) {
	if _, exists := entities[divByZeroExceptionEntity]; exists {
		return
	}
	names, types, vis := systemExceptionSigFields()
	entities[divByZeroExceptionEntity] = entitySig{
		BaseName:        systemExceptionEntity,
		FieldNames:      names,
		FieldTypes:      types,
		FieldVisibility: vis,
		HasCtor:         true,
		CtorParams:      []ir.Type{ir.TypeString},
		CtorParamNames:  []string{"сообщение"},
	}
}

// analyzeThrowStmt - "выбросить <выражение>": значение обязано быть
// сущностью "Исключение" или её наследником - не структурой/интерфейсом/
// примитивом (нелокальный выход, реализуемый как передача указателя
// дальше по стеку вызовов, cbackend/interp).
func analyzeThrowStmt(sc *ctx, s *ast.ThrowStmt) (ir.Stmt, error) {
	value, err := analyzeExpr(sc, s.Value)
	if err != nil {
		return nil, err
	}
	typ := ir.TypeOf(value)
	if typ.Kind != ir.KindEntity || !entityIsOrExtends(sc.entities, typ.Entity, systemExceptionEntity) {
		return nil, fmt.Errorf("строка %d: \"выбросить\" ожидает значение сущности %q или наследника, получено %s", s.Line, systemExceptionEntity, typ)
	}
	return &ir.ThrowStmt{Value: value, Line: s.Line}, nil
}

// checkCatchTypesAreExceptions проверяет, что каждое имя из "поймать(...)"
// (кроме "Паника" - вырезанной парсером ещё до этого места, см. package doc
// ast.TryStmt.CatchPanic) резолвится в "Исключение" саму или наследника, и
// что список без дубликатов - общая проверка и для обычного поймать(Тип),
// и для смешанного поймать(Паника|Тип) (analyzeTryStmt, обе ветки ниже).
// Пустой names - нет ошибки (единственная точка вызова, где это возможно, -
// смешанная ветка с одной лишь "Паника" в списке, т. е. otherTypes пуст).
func checkCatchTypesAreExceptions(sc *ctx, names []string) error {
	seen := map[string]bool{}
	for _, name := range names {
		typ, err := resolveTypeName(sc.entities, sc.methods, sc.aliases, sc.interfaces, sc.enums, name)
		if err != nil {
			return fmt.Errorf("\"поймать(...)\": %w", err)
		}
		if typ.Kind != ir.KindEntity || !entityIsOrExtends(sc.entities, typ.Entity, systemExceptionEntity) {
			return fmt.Errorf("\"поймать(...)\" ожидает сущность %q или наследника, получено %s", systemExceptionEntity, typ)
		}
		if seen[typ.Entity] {
			return fmt.Errorf("\"поймать(...)\": тип %q перечислен более одного раза", typ.Entity)
		}
		seen[typ.Entity] = true
	}
	return nil
}

// analyzeTryStmt - "попытаться { ... } поймать(<имя>: <Тип>[|<Тип>...]) { ... }".
// Established (шаг 4.1/6.4 typestate) внутри Body/CatchBody не гарантирован
// после всей конструкции целиком - "выбросить" может прервать тело
// "попытаться" в произвольной точке, а сам "поймать" может не сработать
// вовсе (конкретное исключение не относится к перечисленным типам) - тот
// же консервативный откат к состоянию до конструкции, что и у тела цикла
// (analyzeLoopBody), не fixed-point анализ по всем путям выполнения.
func analyzeTryStmt(sc *ctx, s *ast.TryStmt) (ir.Stmt, error) {
	preTry := snapshotEstablished(sc.established)

	sc.push()
	body, err := analyzeBlock(sc, s.Body)
	sc.pop()
	if err != nil {
		return nil, err
	}
	sc.established = preTry

	// NoCatch - "попытаться { ... } окончательно { ... }" без единого
	// "поймать" (package doc ast.TryStmt.NoCatch) - CatchVar/CatchTypes и
	// остальные поля поймать не существуют для этого узла вовсе.
	if s.NoCatch {
		finally, err := analyzeFinally(sc, s, preTry)
		if err != nil {
			return nil, err
		}
		return &ir.TryStmt{Body: body, NoCatch: true, Finally: finally, Line: s.Line}, nil
	}

	// CatchPanic - "Паника" была в списке типов (изоляция
	// паники) - CatchVar несёт "Строка" (сообщение), не сущность-исключение,
	// её типовая проверка/entityIsOrExtends ниже к ней не применяется. Если
	// s.CatchTypes при этом непусто ("поймать (x: Паника|Исключение)" и
	// подобные, см. package doc ast.TryStmt.CatchPanic) - остальные типы
	// списка ВСЁ РАВНО проверяются как настоящие типы исключений (та же
	// проверка, что и в обычной ветке ниже), просто CatchVarType остаётся
	// "Строка" в любом случае - смешение с "Паника" уже не оставляет общего
	// типа шире неё.
	if s.CatchPanic {
		if err := checkCatchTypesAreExceptions(sc, s.CatchTypes); err != nil {
			return nil, fmt.Errorf("строка %d: %w", s.Line, err)
		}
		sc.push()
		if err := sc.declare(s.CatchVar, ir.TypeString); err != nil {
			sc.pop()
			return nil, fmt.Errorf("строка %d: %w", s.Line, err)
		}
		catchBody, err := analyzeBlock(sc, s.CatchBody)
		sc.pop()
		if err != nil {
			return nil, err
		}
		sc.established = preTry
		finally, err := analyzeFinally(sc, s, preTry)
		if err != nil {
			return nil, err
		}
		return &ir.TryStmt{
			Body: body, CatchVar: s.CatchVar, CatchPanic: true, CatchTypes: s.CatchTypes, CatchVarType: ir.TypeString, CatchBody: catchBody, Finally: finally, Line: s.Line,
		}, nil
	}

	if len(s.CatchTypes) == 0 {
		return nil, fmt.Errorf("строка %d: \"поймать(...)\" должно перечислять хотя бы один тип исключения", s.Line)
	}
	if err := checkCatchTypesAreExceptions(sc, s.CatchTypes); err != nil {
		return nil, fmt.Errorf("строка %d: %w", s.Line, err)
	}

	// CatchVarType - единственный перечисленный тип, если он один (полный
	// доступ к его полям/методам в CatchBody); при нескольких - только
	// "Исключение", единственный гарантированный общий предок списка (все
	// элементы CatchTypes уже проверены выше как он сам или наследники) -
	// различение конкретных типов внутри CatchBody, если нужно, через
	// "относится к" (IsExpr), а не через доступ к специфичным полям.
	catchVarType := ir.EntityType(systemExceptionEntity)
	if len(s.CatchTypes) == 1 {
		catchVarType = ir.EntityType(s.CatchTypes[0])
	}

	sc.push()
	if err := sc.declare(s.CatchVar, catchVarType); err != nil {
		sc.pop()
		return nil, fmt.Errorf("строка %d: %w", s.Line, err)
	}
	catchBody, err := analyzeBlock(sc, s.CatchBody)
	sc.pop()
	if err != nil {
		return nil, err
	}
	sc.established = preTry
	finally, err := analyzeFinally(sc, s, preTry)
	if err != nil {
		return nil, err
	}

	return &ir.TryStmt{
		Body: body, CatchVar: s.CatchVar, CatchTypes: s.CatchTypes, CatchVarType: catchVarType, CatchBody: catchBody, Finally: finally, Line: s.Line,
	}, nil
}

// analyzeFinally анализирует "окончательно { ... }" узла s (nil, если его
// нет - непусто только у самого внешнего узла цепочки "поймать", см.
// package doc ast.TryStmt.Finally) в собственной, отдельной от CatchBody
// области видимости (никакого CatchVar внутри - окончательно не привязан
// ни к какому конкретному "поймать"). established сбрасывается к preTry
// после (тот же консервативный принцип, что и у Body/CatchBody выше -
// "окончательно" не гарантированно исполнится ПОСЛЕ какого-то конкретного
// перехода состояния, доказанного только в одной из веток).
func analyzeFinally(sc *ctx, s *ast.TryStmt, preTry []map[string]map[string]bool) ([]ir.Stmt, error) {
	if len(s.Finally) == 0 {
		return nil, nil
	}
	sc.push()
	finally, err := analyzeBlock(sc, s.Finally)
	sc.pop()
	if err != nil {
		return nil, err
	}
	sc.established = preTry
	return finally, nil
}

// analyzeIsExpr - "<выражение> относится к <Тип>": допустимо между
// сущностью/интерфейсом и сущностью/интерфейсом, только если они вообще
// могут быть связаны наследованием или соответствием интерфейсу
// (relatedForIsCheck) - иначе результат заведомо известен статически
// (гарантированно ложь) и сама проверка, скорее всего, опечатка, а не
// осмысленный runtime-тест.
func analyzeIsExpr(sc *ctx, e *ast.IsExpr) (ir.Expr, error) {
	value, err := analyzeExpr(sc, e.Value)
	if err != nil {
		return nil, err
	}
	vt := ir.TypeOf(value)
	if vt.Kind != ir.KindEntity && vt.Kind != ir.KindInterface {
		return nil, fmt.Errorf("строка %d: \"относится к\" применимо только к сущности/интерфейсу, получено %s", e.Line, vt)
	}
	// Target, зарегистрированный как производный тип состояния (sc.a.
	// aliasState - "АвторизованныйПользователь" -> "авторизован"), - особый
	// случай: не проверка типа (им бы он и резолвился через resolveTypeName -
	// это ровно тот же синоним, что и в "параметр: АвторизованныйПользователь"),
	// а запрос текущего, времени исполнения, состояния объекта -
	// ir.HasStateExpr вместо ir.IsExpr, никогда не бросает (в отличие от
	// ir.StateCheckExpr, которым компилятор САМ подстраховывает недоказанный
	// на этапе компиляции переход, docs/SPEC.md, "Состояния").
	if state, isStateAlias := sc.a.aliasState[e.Target]; isStateAlias {
		owner := sc.aliases[e.Target]
		if vt.Kind != ir.KindEntity || !entityIsOrExtends(sc.entities, vt.Entity, owner) {
			return nil, fmt.Errorf("строка %d: \"относится к\" %q: %s не является %q (сущностью, объявляющей это состояние) или её наследником", e.Line, e.Target, vt, owner)
		}
		return &ir.HasStateExpr{Value: value, Entity: owner, State: state}, nil
	}
	target, err := resolveTypeName(sc.entities, sc.methods, sc.aliases, sc.interfaces, sc.enums, e.Target)
	if err != nil {
		return nil, fmt.Errorf("строка %d: \"относится к\": %w", e.Line, err)
	}
	if target.Kind != ir.KindEntity && target.Kind != ir.KindInterface {
		return nil, fmt.Errorf("строка %d: \"относится к\" ожидает сущность/интерфейс, получено %s", e.Line, target)
	}
	if !relatedForIsCheck(sc.entities, sc.interfaces, vt, target) {
		return nil, fmt.Errorf("строка %d: \"относится к\": %s и %s не связаны наследованием/интерфейсом", e.Line, vt, target)
	}
	return &ir.IsExpr{Value: value, Target: target.Entity}, nil
}

// relatedForIsCheck - true, если между a и b в принципе возможно отношение
// подтипа в ЛЮБУЮ сторону (a - предок/потомок b, или один из них -
// интерфейс, которому соответствует/может соответствовать другой) - та же
// пара отношений, что и isAssignable, но проверяется в обе стороны сразу
// (в отличие от isAssignable: там направление всегда "from -> to" одной
// присваиваемой стороны, здесь both operands могут быть как более общим,
// так и более узким статическим типом места использования).
func relatedForIsCheck(entities map[string]entitySig, interfaces map[string]interfaceSig, a, b ir.Type) bool {
	switch {
	case a.Kind == ir.KindEntity && b.Kind == ir.KindEntity:
		return entityIsOrExtends(entities, a.Entity, b.Entity) || entityIsOrExtends(entities, b.Entity, a.Entity)
	case a.Kind == ir.KindEntity && b.Kind == ir.KindInterface:
		return entitySatisfies(entities, interfaces, a.Entity, b.Entity)
	case a.Kind == ir.KindInterface && b.Kind == ir.KindEntity:
		return entitySatisfies(entities, interfaces, b.Entity, a.Entity)
	case a.Kind == ir.KindInterface && b.Kind == ir.KindInterface:
		return interfaceIsOrExtends(interfaces, a.Entity, b.Entity) || interfaceIsOrExtends(interfaces, b.Entity, a.Entity)
	default:
		return false
	}
}
