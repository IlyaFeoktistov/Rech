// Встроенный интерфейс "Строчный" (аналог fmt.Stringer/toString()) -
// сущность, которая его реализует, печатается через "вывести значение"
// вызовом её в_строку() вместо стандартного "[сущность]<Имя>"/
// "[структура]<Имя>" (cbackend/format.go, printCall). Ведёт себя как
// обычный объявленный интерфейс (checkImplements, EntityInterfaces,
// vtable-диспетчеризация для интерфейсных значений) - единственное отличие
// от пользовательского: он существует всегда, даже если ни одна сущность
// программы его не реализует (см. injectStringerInterface).
package semantic

import (
	"github.com/IlyaFeoktistov/Rech/internal/ast"
	"github.com/IlyaFeoktistov/Rech/internal/ir"
)

const (
	stringerInterfaceName = "Строчный"
	stringerMethodName    = "в_строку"
)

// injectStringerInterface добавляет "Строчный" в начало prog.Interfaces ДО
// collectInterfaces - дальше он неотличим от интерфейса, объявленного в
// исходнике программы. Если пользователь сам объявит "интерфейс Строчный",
// collectInterfaces откажет обычной проверкой "объявлен более одного раза" -
// отдельного механизма зарезервированных имён для интерфейсов не нужно,
// этой ей и так хватает.
func injectStringerInterface(prog *ast.Program) {
	prog.Interfaces = append([]*ast.InterfaceDecl{{
		Name: stringerInterfaceName,
		Methods: []ast.InterfaceMethod{
			{Name: stringerMethodName, ReturnType: "Строка"},
		},
	}}, prog.Interfaces...)
}

// wrapStringerPrint - если typ (тип уже разобранного value) сущность или
// интерфейс, реализующий "Строчный", оборачивает value в вызов её
// в_строку() - "вывести значение" в analyzeStmt печатает результат ЭТОГО
// вызова вместо value как есть; тот же вызов - у "(Строка) value"
// (analyzeCastExpr). nil - Строчный не реализован (или value не
// сущность/интерфейс), значение печатается прежним способом.
func wrapStringerPrint(sc *ctx, value ir.Expr, typ ir.Type) ir.Expr {
	switch typ.Kind {
	case ir.KindEntity:
		if !entitySatisfies(sc.entities, sc.interfaces, typ.Entity, stringerInterfaceName) {
			return nil
		}
	case ir.KindInterface:
		if !interfaceIsOrExtends(sc.interfaces, typ.Entity, stringerInterfaceName) {
			return nil
		}
	default:
		return nil
	}
	return &ir.MethodCallExpr{Receiver: value, Entity: typ.Entity, Method: stringerMethodName, Type: ir.TypeString, HasReturn: true}
}
