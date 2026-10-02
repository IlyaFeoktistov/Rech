package cbackend

import (
	"fmt"
	"strings"

	"github.com/IlyaFeoktistov/Rech/internal/ir"
)

// registerStructEqEntity добавляет name (и рекурсивно - каждую вложенную
// "структуру" полем, транзитивно) в ctx.structEqEntities - см. её package
// doc (ir_generator.go) про то, почему это множество, а не "генерируем для
// всех структур программы" (как showFuncs/usesShow) - semantic
// (structFieldsEquatable) проверяет ТОЛЬКО реально сравниваемую "=="
// структуру и её вложенные структуры-поля, транзитивно - та же граница
// здесь, той же рекурсией по тем же полям (Fields, не FieldTypes - ctx.
// entities здесь уже *ir.Entity, не entitySig internal/semantic).
func registerStructEqEntity(ctx *irGenCtx, name string) {
	if ctx.structEqEntities == nil {
		ctx.structEqEntities = map[string]bool{}
	}
	if ctx.structEqEntities[name] {
		return
	}
	ctx.structEqEntities[name] = true
	ent, ok := ctx.entities[name]
	if !ok {
		return
	}
	for _, f := range ent.Fields {
		if f.Type.Kind == ir.KindEntity && !ctx.entityIsRef(f.Type.Entity) {
			registerStructEqEntity(ctx, f.Type.Entity)
		}
	}
}

// structEqFuncName - C-имя функции поле-в-поле сравнения "структуры" name
// для "=="/"!=" - тот же принцип именования, что и у
// showFuncName (show.go), отдельный префикс исключает коллизию с typedef'ом
// самой структуры и с C-функциями методов/показа.
func structEqFuncName(name string) string {
	return "rech_struct_eq_" + transliterate(name)
}

// generateIRStructEqFunc строит определение функции поле-в-поле сравнения
// "структуры" ent - semantic (structFieldsEquatable, internal/semantic/
// cast.go) уже гарантировал ДО кодогенерации, что КАЖДОЕ поле - один из
// трёх видов: Строка (strcmp), уже примитивный equatableTypes-тип (обычное
// "==", в т. ч. и сущность-ссылка полем - в C это указатель, "==" на нём
// уже identity-сравнение, отдельного случая не нужно) или вложенная
// "структура" (рекурсивный вызов ЕЁ собственной rech_struct_eq_..., см.
// structEqFuncName) - эта функция ничего не проверяет заново, только
// эмитирует уже разрешённый семантикой случай для каждого поля.
func generateIRStructEqFunc(ctx *irGenCtx, ent *ir.Entity) string {
	var sb strings.Builder
	cName := entityCName(ctx, ent.Name)
	fmt.Fprintf(&sb, "static bool %s(%s a, %s b) {\n", structEqFuncName(ent.Name), cName, cName)
	if len(ent.Fields) == 0 {
		// Структура без полей - равна сама себе безусловно (тот же случай,
		// что и у пустого класса в C#/Java - "истина" по умолчанию, не
		// ошибка компиляции самой функции из-за пустого "return true &&").
		sb.WriteString("    return true;\n}\n\n")
		return sb.String()
	}
	sb.WriteString("    return true")
	for _, f := range ent.Fields {
		aExpr := "a." + mangleField(f.Name)
		bExpr := "b." + mangleField(f.Name)
		switch {
		case f.Type == ir.TypeString:
			ctx.usesStrCmp = true
			fmt.Fprintf(&sb, "\n        && strcmp(%s, %s) == 0", aExpr, bExpr)
		case f.Type.Kind == ir.KindEntity && !ctx.entityIsRef(f.Type.Entity):
			fmt.Fprintf(&sb, "\n        && %s(%s, %s)", structEqFuncName(f.Type.Entity), aExpr, bExpr)
		case f.Type == ir.TypeDecimal:
			ctx.usesDecimal = true
			fmt.Fprintf(&sb, "\n        && rech_dec_cmp(%s, %s) == 0", aExpr, bExpr)
		default:
			fmt.Fprintf(&sb, "\n        && (%s == %s)", aExpr, bExpr)
		}
	}
	sb.WriteString(";\n}\n\n")
	return sb.String()
}
