package semantic

import (
	"github.com/IlyaFeoktistov/Rech/internal/ast"
	"github.com/IlyaFeoktistov/Rech/internal/ir"
	"github.com/IlyaFeoktistov/Rech/internal/rtmsg"
)

// Встроенная иерархия паник (rtmsg.PanicKinds): корень "Паника" -
// независимый от "Исключение" (поймать(Исключение) паники не ловит) - и
// наследники по сути ошибки. Есть в любой программе: рантайм бросает их
// сам (выход за границы, деление на ноль, ...), а программа может ловить
// по имени и объявлять свои ("сущность X наследует Паника" + "выбросить").
// Единственное поле - "сообщение"; реализует "Строчный" (в_строку() -
// сообщение), поэтому "${о}"/"вывести о" печатают текст паники.

// registerPanicEntities заводит сигнатуры всех встроенных паник.
func registerPanicEntities(entities map[string]entitySig, methods map[string]map[string]funcSig) {
	for _, name := range rtmsg.PanicKinds {
		sig := entitySig{
			FieldNames:      []string{"сообщение"},
			FieldTypes:      []ir.Type{ir.TypeString},
			FieldVisibility: []ast.Visibility{ast.Public},
			HasCtor:         true,
			CtorParams:      []ir.Type{ir.TypeString},
			CtorParamNames:  []string{"сообщение"},
		}
		if name == rtmsg.PanicEntity {
			sig.OwnInterfaces = []string{stringerInterfaceName}
		} else {
			sig.BaseName = rtmsg.PanicEntity
		}
		entities[name] = sig
		methods[name] = map[string]funcSig{
			stringerMethodName: {ReturnType: ir.TypeString, HasReturn: true},
		}
	}
}

// panicIREntities - IR всех встроенных паник (корень первым).
func panicIREntities() []*ir.Entity {
	out := make([]*ir.Entity, 0, len(rtmsg.PanicKinds))
	for _, name := range rtmsg.PanicKinds {
		ent := &ir.Entity{
			Name:   name,
			Fields: []ir.Field{{Name: "сообщение", Type: ir.TypeString}},
			Ctor:   messageCtor(name),
		}
		if name == rtmsg.PanicEntity {
			ent.Interfaces = []string{stringerInterfaceName}
			ent.Methods = []*ir.Method{{
				Name:       stringerMethodName,
				ReturnType: ir.TypeString,
				HasReturn:  true,
				Body: []ir.Stmt{&ir.ReturnStmt{Value: &ir.FieldAccess{
					Receiver: &ir.ThisExpr{Entity: name},
					Field:    "сообщение",
					Type:     ir.TypeString,
				}}},
			}}
		} else {
			ent.BaseName = rtmsg.PanicEntity
		}
		out = append(out, ent)
	}
	return out
}

// messageCtor - конструктор "(сообщение: Строка)" встроенной паники
// entity: "этот.сообщение = сообщение".
func messageCtor(entity string) *ir.Method {
	return &ir.Method{
		Params: []ir.Param{{Name: "сообщение", Type: ir.TypeString}},
		Body: []ir.Stmt{
			&ir.FieldAssign{
				Receiver: &ir.ThisExpr{Entity: entity},
				Field:    "сообщение",
				Value:    &ir.Identifier{Name: "сообщение", Type: ir.TypeString},
			},
		},
	}
}

// isThrowable - entity сама "Исключение"/"Паника" или их наследник.
func isThrowable(entities map[string]entitySig, entity string) bool {
	return entityIsOrExtends(entities, entity, systemExceptionEntity) || entityIsOrExtends(entities, entity, rtmsg.PanicEntity)
}

// throwableRoot - корень иерархии, к которому относится entity
// ("Исключение" или "Паника").
func throwableRoot(entities map[string]entitySig, entity string) string {
	if entityIsOrExtends(entities, entity, rtmsg.PanicEntity) {
		return rtmsg.PanicEntity
	}
	return systemExceptionEntity
}
