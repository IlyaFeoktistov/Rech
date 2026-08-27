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
	return &ir.ThrowStmt{Value: value}, nil
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

	if len(s.CatchTypes) == 0 {
		return nil, fmt.Errorf("строка %d: \"поймать(...)\" должно перечислять хотя бы один тип исключения", s.Line)
	}
	seen := map[string]bool{}
	for _, name := range s.CatchTypes {
		typ, err := resolveTypeName(sc.entities, sc.methods, sc.aliases, sc.interfaces, name)
		if err != nil {
			return nil, fmt.Errorf("строка %d: \"поймать(...)\": %w", s.Line, err)
		}
		if typ.Kind != ir.KindEntity || !entityIsOrExtends(sc.entities, typ.Entity, systemExceptionEntity) {
			return nil, fmt.Errorf("строка %d: \"поймать(...)\" ожидает сущность %q или наследника, получено %s", s.Line, systemExceptionEntity, typ)
		}
		if seen[typ.Entity] {
			return nil, fmt.Errorf("строка %d: \"поймать(...)\": тип %q перечислен более одного раза", s.Line, typ.Entity)
		}
		seen[typ.Entity] = true
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

	return &ir.TryStmt{
		Body: body, CatchVar: s.CatchVar, CatchTypes: s.CatchTypes, CatchVarType: catchVarType, CatchBody: catchBody,
	}, nil
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
	target, err := resolveTypeName(sc.entities, sc.methods, sc.aliases, sc.interfaces, e.Target)
	if err != nil {
		return nil, fmt.Errorf("строка %d: \"относится к\": %w", e.Line, err)
	}
	if target.Kind != ir.KindEntity && target.Kind != ir.KindInterface {
		return nil, fmt.Errorf("строка %d: \"относится к\" ожидает сущность/интерфейс, получено %s", e.Line, target)
	}
	if !relatedForIsCheck(sc.entities, vt, target) {
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
func relatedForIsCheck(entities map[string]entitySig, a, b ir.Type) bool {
	switch {
	case a.Kind == ir.KindEntity && b.Kind == ir.KindEntity:
		return entityIsOrExtends(entities, a.Entity, b.Entity) || entityIsOrExtends(entities, b.Entity, a.Entity)
	case a.Kind == ir.KindEntity && b.Kind == ir.KindInterface:
		return entitySatisfies(entities, a.Entity, b.Entity)
	case a.Kind == ir.KindInterface && b.Kind == ir.KindEntity:
		return entitySatisfies(entities, b.Entity, a.Entity)
	case a.Kind == ir.KindInterface && b.Kind == ir.KindInterface:
		return a.Entity == b.Entity
	default:
		return false
	}
}
