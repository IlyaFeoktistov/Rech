// Перегрузка бинарного оператора для пользовательской сущности/структуры
// ("статический публичный оператор +(a: Тип, b: Тип): Тип", internal/
// parser/entity.go, parseMethod) - прямое продолжение перегрузки функций/
// методов (internal/semantic/overload.go): сам синтаксис уже развёл
// операторный метод в обычный ast.MethodDecl с синтетическим именем
// (ast.OperatorMethodName), поэтому здесь не нужно ничего знать про
// лексер/парсер - достаточно поискать статический метод с этим именем на
// типе ЛЕВОГО операнда, как обычную статическую сигнатуру (methods[Entity]).
package semantic

import (
	"fmt"

	"github.com/IlyaFeoktistov/Rech/internal/ast"
	"github.com/IlyaFeoktistov/Rech/internal/ir"
)

// tryOperatorOverload резолвит "left <op> right" через объявленный
// сущностью/структурой типа left оператор, если он есть. found=false -
// такой оператор не объявлен (или left - не сущность вовсе) - вызывающая
// сторона (analyzeBinaryExpr) продолжает обычную резолюцию для встроенных
// типов, как раньше. found=true, err!=nil - оператор объявлен, но с другим
// списком параметров, чем (left, right) - ошибка обязана пропагироваться
// немедленно, а не тихо провалиться в обычную резолюцию (та выдала бы куда
// менее полезное "оператор не поддерживается для сущности X").
//
// Только ЛЕВЫЙ операнд может быть объявляющей стороной - симметричная
// форма (правый операнд - сущность, левый - примитив, "2 * Вектор") не
// поддерживается в этой версии: перегрузка всегда статический метод САМОЙ
// сущности, у которой в этой версии нет способа "вклиниться" в оператор,
// если она справа. См. docs/SPEC.md, "Перегрузка операторов".
func tryOperatorOverload(sc *ctx, op string, left, right ir.Expr, lt ir.Type, line int) (ir.Expr, bool, error) {
	if lt.Kind != ir.KindEntity {
		return nil, false, nil
	}
	name := ast.OperatorMethodName(op)
	sig, ok := sc.methods[lt.Entity][name]
	if !ok {
		return nil, false, nil
	}
	rt := ir.TypeOf(right)
	if len(sig.Params) != 2 ||
		!isAssignable(sc.entities, sc.interfaces, lt, sig.Params[0]) ||
		!isAssignable(sc.entities, sc.interfaces, rt, sig.Params[1]) {
		return nil, true, fmt.Errorf("строка %d: у сущности %q оператор %q объявлен для (%s, %s), не подходит для операндов (%s, %s)",
			line, lt.Entity, op, sig.Params[0], sig.Params[1], lt, rt)
	}
	if err := checkMethodVisibility(sc, lt.Entity, name, sig.Visibility, line); err != nil {
		return nil, true, err
	}
	return &ir.StaticMethodCallExpr{Entity: lt.Entity, Method: name, Args: []ir.Expr{left, right}, Type: sig.ReturnType, HasReturn: sig.HasReturn}, true, nil
}
