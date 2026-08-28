package semantic

import (
	"fmt"

	"github.com/IlyaFeoktistov/Rech/internal/ast"
	"github.com/IlyaFeoktistov/Rech/internal/ir"
)

// analyzeIndexExpr - "receiver[index]" (безопасная индексация, см. package
// doc ast.IndexExpr) - Список<T> по целочисленному индексу, Словарь<К, В>
// по ключу К - результат всегда "?T"/"?В" (Ничто при промахе/выходе за
// границу), не паника, в отличие от ".получить(...)". Отдельно от них -
// РЯДОМ, не вместо: ".получить"/".установить" продолжают паниковать как
// раньше, "[]" - новый безопасный путь.
func analyzeIndexExpr(sc *ctx, e *ast.IndexExpr) (ir.Expr, error) {
	if !receiverAddressable(e.Receiver) {
		return nil, fmt.Errorf("строка %d: получателем индексации \"[]\" может быть только переменная, поле или \"этот\"", e.Line)
	}
	receiver, err := analyzeExpr(sc, e.Receiver)
	if err != nil {
		return nil, err
	}
	receiver = maybeUnwrapNarrowed(sc, receiver)
	typ := ir.TypeOf(receiver)
	sig, ok := sc.entities[typ.Entity]
	if typ.Kind != ir.KindEntity || !ok || (!sig.IsList && !sig.IsDict) {
		return nil, fmt.Errorf("строка %d: индексация \"[]\" применима только к Список<T>/Словарь<К, В>, получен %s", e.Line, typ)
	}

	resultType, err := resolveNullableTypeName(sc.entities, sc.methods, sc.aliases, sc.interfaces, "?"+sig.ElemType.String())
	if err != nil {
		return nil, fmt.Errorf("строка %d: %w", e.Line, err)
	}

	if sig.IsList {
		index, err := analyzeExpr(sc, e.Index)
		if err != nil {
			return nil, err
		}
		index = maybeUnwrapNarrowed(sc, index)
		if it := ir.TypeOf(index); it != ir.TypeInt {
			return nil, fmt.Errorf("строка %d: индекс \"[]\" должен быть типа Целое, получен %s", e.Line, it)
		}
		sc.a.markIndexUsed(typ.Entity)
		return &ir.ListIndexExpr{Receiver: receiver, Index: index, CollectionName: typ.Entity, Type: resultType, Line: e.Line}, nil
	}

	key, err := analyzeExpr(sc, e.Index)
	if err != nil {
		return nil, err
	}
	key = maybeUnwrapNarrowed(sc, key)
	if kt := ir.TypeOf(key); kt != sig.KeyType {
		return nil, fmt.Errorf("строка %d: ключ \"[]\" должен быть типа %s, получен %s", e.Line, sig.KeyType, kt)
	}
	sc.a.markIndexUsed(typ.Entity)
	return &ir.DictIndexExpr{Receiver: receiver, Key: key, CollectionName: typ.Entity, Type: resultType, Line: e.Line}, nil
}
