// Статические члены сущности/структуры ("статическое поле"/"статический
// метод", доступ "Тип::имя") - принадлежат самому типу, не экземпляру: одно
// общее хранилище на всю программу, доступ только через имя типа перед "::"
// (никогда через получатель-экземпляр, см. package doc ast.FieldAccess.
// Static/ast.MethodCallExpr.Static). Не наследуются (internal/semantic/
// inheritance.go, applyInheritance) - тем же принципом, что и конструктор.
package semantic

import (
	"fmt"

	"github.com/IlyaFeoktistov/Rech/internal/ast"
	"github.com/IlyaFeoktistov/Rech/internal/ir"
)

// staticReceiverEntity разбирает Receiver выражения "Тип::член" - обязан
// быть простым идентификатором, называющим объявленную сущность/структуру
// (не переменную, не выражение) - "::" однозначен по грамматике, в отличие
// от "." никакого разрешения неоднозначности (переменная/тип с тем же
// именем) не нужно: переменная слева от "::" - всегда ошибка, даже если она
// существует.
func staticReceiverEntity(sc *ctx, receiver ast.Expr, line int) (string, entitySig, error) {
	ident, ok := receiver.(*ast.Identifier)
	if !ok {
		return "", entitySig{}, fmt.Errorf("строка %d: слева от \"::\" ожидалось имя сущности/структуры", line)
	}
	if _, isVar := sc.lookup(ident.Name); isVar {
		return "", entitySig{}, fmt.Errorf("строка %d: %q - переменная, не тип - доступ к статическому члену требует имени сущности/структуры слева от \"::\"", line, ident.Name)
	}
	sig, ok := sc.entities[ident.Name]
	if !ok {
		return "", entitySig{}, fmt.Errorf("строка %d: сущность %q не объявлена", line, ident.Name)
	}
	return ident.Name, sig, nil
}

// analyzeStaticFieldAccess - "Тип::поле" как значение (чтение). "Тип" -
// имя объявленной сущности/структуры (обычный случай, staticReceiverEntity
// ниже) ИЛИ один из числовых примитивов ("Целое8::МАКСИМУМ" и т. п., см.
// type_constants.go) - у второго нет записи в sc.entities вовсе, поэтому
// проверяется первым.
func analyzeStaticFieldAccess(sc *ctx, e *ast.FieldAccess) (ir.Expr, error) {
	if ident, ok := e.Receiver.(*ast.Identifier); ok {
		if expr, found, err := primitiveTypeConstant(ident.Name, e.Field, e.Line); found {
			return expr, err
		}
	}
	entityName, sig, err := staticReceiverEntity(sc, e.Receiver, e.Line)
	if err != nil {
		return nil, err
	}
	if idx := sig.staticFieldIndex(e.Field); idx != -1 {
		if err := checkStaticFieldVisibility(sc, entityName, e.Field, sig.StaticFieldVisibility[idx], e.Line); err != nil {
			return nil, err
		}
		return &ir.StaticFieldExpr{Entity: entityName, Field: e.Field, Type: sig.StaticFieldTypes[idx]}, nil
	}
	// Константа тоже статическая по природе (docs/SPEC.md, "Статические
	// члены") - "Тип::КОНСТАНТА" работает наравне с уже существующим
	// "Тип.КОНСТАНТА" (resolveEntityConstant), не только "Тип::поле".
	if idx := sig.constIndex(e.Field); idx != -1 {
		if err := checkConstVisibility(sc, entityName, e.Field, sig.ConstVisibility[idx], e.Line); err != nil {
			return nil, err
		}
		return resolveEntityConstant(sc, entityName, e.Field, e.Line)
	}
	return nil, fmt.Errorf("строка %d: у сущности %q нет статического поля или константы %q", e.Line, entityName, e.Field)
}

// analyzeStaticMethodCallExpr - "Тип::метод(...)".
func analyzeStaticMethodCallExpr(sc *ctx, e *ast.MethodCallExpr) (*ir.StaticMethodCallExpr, error) {
	entityName, _, err := staticReceiverEntity(sc, e.Receiver, e.Line)
	if err != nil {
		return nil, err
	}
	sig, ok := sc.methods[entityName][e.Method]
	if !ok || !sig.IsStatic {
		return nil, fmt.Errorf("строка %d: у сущности %q нет статического метода %q", e.Line, entityName, e.Method)
	}
	if err := checkMethodVisibility(sc, entityName, e.Method, sig.Visibility, e.Line); err != nil {
		return nil, err
	}
	args, err := checkCallArgs(sc, e.Args, sig, e.Line, fmt.Sprintf("статический метод %q", e.Method), fmt.Sprintf("вызова статического метода %q", e.Method))
	if err != nil {
		return nil, err
	}
	return &ir.StaticMethodCallExpr{Entity: entityName, Method: e.Method, Args: args, Type: sig.ReturnType, HasReturn: sig.HasReturn}, nil
}

// analyzeStaticFieldAssign - "Тип::поле = значение".
func analyzeStaticFieldAssign(sc *ctx, s *ast.FieldAssign) (*ir.StaticFieldAssign, error) {
	if ident, ok := s.Receiver.(*ast.Identifier); ok {
		if _, ok := primitiveTypes[ident.Name]; ok {
			return nil, fmt.Errorf("строка %d: %q - константа типа %q, ей нельзя присвоить заново", s.Line, s.Field, ident.Name)
		}
	}
	entityName, sig, err := staticReceiverEntity(sc, s.Receiver, s.Line)
	if err != nil {
		return nil, err
	}
	idx := sig.staticFieldIndex(s.Field)
	if idx == -1 {
		if sig.constIndex(s.Field) != -1 {
			return nil, fmt.Errorf("строка %d: %q - константа сущности %q, ей нельзя присвоить заново", s.Line, s.Field, entityName)
		}
		return nil, fmt.Errorf("строка %d: у сущности %q нет статического поля %q", s.Line, entityName, s.Field)
	}
	if err := checkStaticFieldVisibility(sc, entityName, s.Field, sig.StaticFieldVisibility[idx], s.Line); err != nil {
		return nil, err
	}
	value, err := coerceExpr(sc, s.Value, sig.StaticFieldTypes[idx])
	if err != nil {
		return nil, err
	}
	if typ := ir.TypeOf(value); !isAssignable(sc.entities, sc.interfaces, typ, sig.StaticFieldTypes[idx]) {
		return nil, fmt.Errorf("строка %d: статическому полю %q сущности %q присваивается %s, ожидался %s", s.Line, s.Field, entityName, typ, sig.StaticFieldTypes[idx])
	}
	return &ir.StaticFieldAssign{Entity: entityName, Field: s.Field, Value: value, Line: s.Line}, nil
}
