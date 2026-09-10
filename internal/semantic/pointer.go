package semantic

import (
	"fmt"

	"github.com/IlyaFeoktistov/Rech/internal/ast"
	"github.com/IlyaFeoktistov/Rech/internal/ir"
)

// resolvePointerTypeName резолвит "Указатель<T>" - лениво синтезирует
// entitySig с IsPointer/ElemType при первой встрече конкретной
// инстанциации, тем же принципом мемоизации, что и resolveNullableTypeName/
// resolveGenericTypeName. argName - уже отделённый parseGenericTypeName
// аргумент внутри "<...>" (тем же способом, что и у Список<T>/Стопка<T>/
// Очередь<T> - единственного типового аргумента).
func resolvePointerTypeName(entities map[string]entitySig, methods map[string]map[string]funcSig, aliases map[string]string, interfaces map[string]interfaceSig, enums map[string]map[string]int64, name, argName string) (ir.Type, error) {
	inner, err := resolveTypeName(entities, methods, aliases, interfaces, enums, argName)
	if err != nil {
		return ir.TypeUnknown, fmt.Errorf("тип назначения %q в %q: %w", argName, name, err)
	}
	if inner.Kind == ir.KindNullable {
		return ir.TypeUnknown, fmt.Errorf("%q: указатель на nullable-тип (\"?Тип\") не поддержан", name)
	}
	if _, exists := entities[name]; !exists {
		entities[name] = entitySig{IsPointer: true, ElemType: inner}
	}
	return ir.Type{Kind: ir.KindPointer, Entity: name}, nil
}

// pointerInner возвращает внутренний Тип у уже резолвленного "Указатель<T>"
// t (t.Kind == KindPointer) - тем же принципом, что nullableInner.
func pointerInner(sc *ctx, t ir.Type) ir.Type {
	return sc.entities[t.Entity].ElemType
}

// pointerTypeFor резолвит "Указатель<T>" по уже известному T (не по
// строке из исходника - вызывающая сторона уже знает T как ir.Type,
// например тип переменной/поля/элемента списка, у которого берут адрес) -
// собирает каноническое имя тем же способом, что и любая другая generic-
// инстанциация ("Шаблон<Аргумент>", parseGenericTypeName/
// resolveGenericTypeName), поэтому resolvePointerTypeName её узнаёт.
func pointerTypeFor(sc *ctx, inner ir.Type) (ir.Type, error) {
	name := fmt.Sprintf("Указатель<%s>", inner.String())
	return resolvePointerTypeName(sc.entities, sc.methods, sc.aliases, sc.interfaces, sc.enums, name, inner.String())
}

// analyzeAddressOfExpr разбирает "&получатель.поле"/"&получатель[индекс]"
// (см. ast.AddressOfExpr - простое "&имя" остаётся FuncRefExpr, сюда не
// попадает). Получатель - обязательно уже объявленная переменная/параметр
// (простое имя, без своей цепочки "." - та же граница, что и везде у
// однократного доступа к полю/индексу).
func analyzeAddressOfExpr(sc *ctx, e *ast.AddressOfExpr) (ir.Expr, error) {
	recvType, ok := sc.lookup(e.Receiver)
	if !ok {
		return nil, fmt.Errorf("строка %d: %q не объявлено", e.Line, e.Receiver)
	}
	receiver := &ir.Identifier{Name: e.Receiver, Type: recvType}

	if e.Field != "" {
		if recvType.Kind != ir.KindEntity {
			return nil, fmt.Errorf("строка %d: нельзя взять адрес поля %q - тип %s не сущность", e.Line, e.Field, recvType)
		}
		sig := sc.entities[recvType.Entity]
		idx := sig.fieldIndex(e.Field)
		if idx == -1 {
			return nil, fmt.Errorf("строка %d: у сущности %q нет поля %q", e.Line, recvType.Entity, e.Field)
		}
		if err := checkFieldVisibility(sc, recvType.Entity, e.Field, sig.FieldVisibility[idx], e.Line); err != nil {
			return nil, err
		}
		fieldType := sig.FieldTypes[idx]
		ptrType, err := pointerTypeFor(sc, fieldType)
		if err != nil {
			return nil, fmt.Errorf("строка %d: %w", e.Line, err)
		}
		fieldAccess := &ir.FieldAccess{Receiver: receiver, Field: e.Field, Type: fieldType}
		return &ir.AddressOfExpr{Receiver: fieldAccess, Type: ptrType, Line: e.Line}, nil
	}

	if e.Index != nil {
		sig, ok := sc.entities[recvType.Entity]
		if recvType.Kind != ir.KindEntity || !ok || !sig.IsList {
			return nil, fmt.Errorf("строка %d: \"&%s[...]\" применимо только к Список<T>, получен %s", e.Line, e.Receiver, recvType)
		}
		index, err := analyzeExpr(sc, e.Index)
		if err != nil {
			return nil, err
		}
		index = maybeUnwrapNarrowed(sc, index)
		if it := ir.TypeOf(index); it != ir.TypeInt {
			return nil, fmt.Errorf("строка %d: индекс \"&%s[...]\" должен быть типа Целое, получен %s", e.Line, e.Receiver, it)
		}
		elemType := sig.ElemType
		ptrType, err := pointerTypeFor(sc, elemType)
		if err != nil {
			return nil, fmt.Errorf("строка %d: %w", e.Line, err)
		}
		elemRef := &ir.ListElementRef{Receiver: receiver, Index: index, CollectionName: recvType.Entity, Type: elemType, Line: e.Line}
		return &ir.AddressOfExpr{Receiver: elemRef, Type: ptrType, Line: e.Line}, nil
	}

	// Не должно происходить - парсер строит AddressOfExpr только когда
	// сразу после имени следует "."/"[" (см. parser.parseAddressOf), голое
	// "&имя" остаётся FuncRefExpr.
	return nil, fmt.Errorf("строка %d: некорректный \"&%s\"", e.Line, e.Receiver)
}

// analyzeDerefExpr разбирает "*значение" - чтение через "Указатель<T>".
func analyzeDerefExpr(sc *ctx, e *ast.DerefExpr) (ir.Expr, error) {
	value, err := analyzeExpr(sc, e.Value)
	if err != nil {
		return nil, err
	}
	value = maybeUnwrapNarrowed(sc, value)
	typ := ir.TypeOf(value)
	if typ.Kind != ir.KindPointer {
		return nil, fmt.Errorf("строка %d: \"*\" применимо только к \"Указатель<T>\", получен %s", e.Line, typ)
	}
	return &ir.DerefExpr{Value: value, Type: pointerInner(sc, typ), Line: e.Line}, nil
}

// analyzeDerefAssignStmt разбирает "*указатель = значение" - запись через
// "Указатель<T>".
func analyzeDerefAssignStmt(sc *ctx, s *ast.DerefAssignStmt) (ir.Stmt, error) {
	pointer, err := analyzeExpr(sc, s.Pointer)
	if err != nil {
		return nil, err
	}
	pointer = maybeUnwrapNarrowed(sc, pointer)
	typ := ir.TypeOf(pointer)
	if typ.Kind != ir.KindPointer {
		return nil, fmt.Errorf("строка %d: \"*<...> = значение\" применимо только к \"Указатель<T>\", получен %s", s.Line, typ)
	}
	inner := pointerInner(sc, typ)
	value, err := coerceExpr(sc, s.Value, inner)
	if err != nil {
		return nil, err
	}
	if vt := ir.TypeOf(value); !isAssignable(sc.entities, sc.interfaces, vt, inner) {
		return nil, fmt.Errorf("строка %d: через указатель на %s нельзя записать значение типа %s", s.Line, inner, vt)
	}
	return &ir.DerefAssignStmt{Pointer: pointer, Value: value, Line: s.Line}, nil
}
