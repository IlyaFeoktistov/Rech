package semantic

import (
	"github.com/IlyaFeoktistov/Rech/internal/ast"
	"github.com/IlyaFeoktistov/Rech/internal/diagnostics"
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
		return nil, errAtLine(e.Line, CodeSemanticOther, "получателем индексации \"[]\" может быть только переменная, поле или \"этот\"")
	}
	receiver, err := analyzeExpr(sc, e.Receiver)
	if err != nil {
		return nil, err
	}
	receiver = maybeUnwrapNarrowed(sc, receiver)
	typ := ir.TypeOf(receiver)
	sig, ok := sc.entities[typ.Entity]
	if typ.Kind != ir.KindEntity || !ok || (!sig.IsList && !sig.IsDict) {
		return nil, errAtLine(e.Line, CodeSemanticOther, "индексация \"[]\" применима только к Список<T>/Словарь<К, В>, получен %s", typ)
	}

	resultType, err := resolveNullableTypeName(sc.entities, sc.methods, sc.aliases, sc.interfaces, sc.enums, "?"+sig.ElemType.String())
	if err != nil {
		return nil, diagnostics.WithPos(err, diagnostics.Position{File: currentFile, Line: e.Line})
	}

	if sig.IsList {
		index, err := analyzeExpr(sc, e.Index)
		if err != nil {
			return nil, err
		}
		index = maybeUnwrapNarrowed(sc, index)
		if it := ir.TypeOf(index); it != ir.TypeInt {
			return nil, errAtLine(e.Line, CodeSemanticOther, "индекс \"[]\" должен быть типа Целое, получен %s", it)
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
		return nil, errAtLine(e.Line, CodeSemanticOther, "ключ \"[]\" должен быть типа %s, получен %s", sig.KeyType, kt)
	}
	sc.a.markIndexUsed(typ.Entity)
	return &ir.DictIndexExpr{Receiver: receiver, Key: key, CollectionName: typ.Entity, Type: resultType, Line: e.Line}, nil
}
