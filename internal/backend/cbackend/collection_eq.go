package cbackend

import (
	"fmt"

	"github.com/IlyaFeoktistov/Rech/internal/ir"
)

// elemCompareExpr строит C-выражение сравнения ДВУХ элементов типа t
// (aExpr/bExpr - уже готовый C-код доступа к каждому) для метода "равен" у
// "Список<T>"/"Словарь<К, В>" (see listEqualCCode/dictEqualCCode ниже) -
// той же трёхчастной логикой, что и у generateIRStructEqFunc (struct_eq.go)
// для полей "структуры": Строка - strcmp, вложенная "структура" -
// рекурсивный rech_struct_eq_..., иначе (примитив ИЛИ ссылочная сущность -
// identity) - обычное "==". Единственное отличие от struct_eq.go -
// ЧЕТВЁРТЫЙ случай: t сам оказался ДРУГОЙ generic-инстанциацией
// (ctx.generics[t.Entity] - Список<Список<Целое>> и т. п.) - тогда вместо
// "==" (что дало бы сравнение указателей, не содержимого) вызывается ЕЁ
// СОБСТВЕННЫЙ метод "равен" - internal/semantic уже гарантировал
// (elemEquatable, cast.go), что у вложенной инстанциации он существует,
// прежде чем разрешить внешней "равен" вообще появиться.
func elemCompareExpr(ctx *irGenCtx, t ir.Type, aExpr, bExpr string) string {
	switch {
	case t == ir.TypeString:
		ctx.usesStrCmp = true
		return fmt.Sprintf("strcmp(%s, %s) == 0", aExpr, bExpr)
	case t.Kind == ir.KindEntity && ctx.generics[t.Entity]:
		return fmt.Sprintf("%s(%s, %s)", mangleMethod(t.Entity, "равен"), aExpr, bExpr)
	case t.Kind == ir.KindEntity && !ctx.entityIsRef(t.Entity):
		registerStructEqEntity(ctx, t.Entity)
		return fmt.Sprintf("%s(%s, %s)", structEqFuncName(t.Entity), aExpr, bExpr)
	default:
		return fmt.Sprintf("(%s == %s)", aExpr, bExpr)
	}
}

// listEqualCCode строит C-текст метода "равен" для конкретной инстанциации
// "Список<T>" g (g.Equatable уже проверено вызывающей стороной, ir_
// generator.go) - длины и поэлементно, в порядке индексов (Список -
// упорядоченная коллекция, в отличие от Словаря ниже).
func listEqualCCode(ctx *irGenCtx, g ir.GenericInstantiation) string {
	structName := mangleType(g.Name)
	fn := mangleMethod(g.Name, "равен")
	cmp := elemCompareExpr(ctx, g.ElemType, "this_->data[i]", "other->data[i]")
	return fmt.Sprintf(`static bool %[1]s(%[2]s *this_, %[2]s *other) {
    if (this_->len != other->len) {
        return false;
    }
    for (long long i = 0; i < this_->len; i++) {
        if (!(%[3]s)) {
            return false;
        }
    }
    return true;
}

`, fn, structName, cmp)
}

// dictEqualCCode строит C-текст метода "равен" для конкретной инстанциации
// "Словарь<К, В>" g - Словарь неупорядочен (открытая адресация,
// dictCollectionCCode), поэтому сравнение идёт по КЛЮЧАМ: для каждого
// занятого слота this_ ищет тот же ключ в other через её собственную
// "<structName>_find" (уже определена dictCollectionCCode тем же именем,
// пересчитывать не нужно - тот же файл трансляции) и сравнивает значения.
func dictEqualCCode(ctx *irGenCtx, g ir.GenericInstantiation) string {
	structName := mangleType(g.Name)
	findFn := structName + "_find"
	fn := mangleMethod(g.Name, "равен")
	cmp := elemCompareExpr(ctx, g.ElemType, "this_->slots[i].value", "other->slots[idx].value")
	return fmt.Sprintf(`static bool %[1]s(%[2]s *this_, %[2]s *other) {
    if (this_->count != other->count) {
        return false;
    }
    for (long long i = 0; i < this_->cap; i++) {
        if (this_->slots[i].state != 1) {
            continue;
        }
        long long idx = %[3]s(other, this_->slots[i].key);
        if (idx < 0 || !(%[4]s)) {
            return false;
        }
    }
    return true;
}

`, fn, structName, findFn, cmp)
}
