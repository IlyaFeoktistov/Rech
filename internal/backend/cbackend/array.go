package cbackend

import (
	"fmt"
	"strings"

	"github.com/IlyaFeoktistov/Rech/internal/ir"
	"github.com/IlyaFeoktistov/Rech/internal/rtmsg"
)

// Массив фиксированной длины "T[N]" - "typedef struct { T a[N]; }
// rech_T_..." : C-массив внутри структуры даёт значение (присваивание,
// передача и возврат копируют), а раскладка в памяти та же, что у "T a[N]"
// - годится полем "внешняя структура". Индекс проверяется rech_arr_idx
// (паника "выход за границы", как у Список<T>), результат - lvalue:
// "м.a[rech_arr_idx(i, N, ...)] = x" и "&м.a[...]" - обычный C.

// arrayIndexHelper - проверка индекса массива.
var arrayIndexHelper = fmt.Sprintf(`static inline long long rech_arr_idx(long long i, long long n, const char *msg) {
    if (i < 0 || i >= n) {
        rech_panic_as(%d, msg);
    }
    return i;
}
`, rtmsg.PanicKind(rtmsg.PanicOutOfRange))

// arrayNeedsEntityTypedef - typedef массива a ссылается на структуру по
// значению (элемент - "структура"/"внешняя структура" или указатель на
// неё, в т. ч. через вложенные массивы): объявляется рядом с typedef'ами
// сущностей, не раньше.
func arrayNeedsEntityTypedef(ctx *irGenCtx, t ir.Type) bool {
	for {
		switch t.Kind {
		case ir.KindArray:
			t = ctx.arrays[t.Entity].Elem
			continue
		case ir.KindPointer:
			t = ctx.pointerInner[t.Entity]
			continue
		case ir.KindEntity:
			ent := ctx.entities[t.Entity]
			return ent != nil && (ent.IsStruct || ent.IsExternStruct)
		}
		return false
	}
}

// ensureArrayTypedefs - typedef'ы массивов, достижимых из t (сам t,
// вложенные массивы, массивы под указателем), в sb, каждый один раз,
// вложенные - раньше.
func ensureArrayTypedefs(ctx *irGenCtx, sb *strings.Builder, t ir.Type) {
	switch t.Kind {
	case ir.KindPointer:
		ensureArrayTypedefs(ctx, sb, ctx.pointerInner[t.Entity])
	case ir.KindArray:
		if ctx.arraysEmitted[t.Entity] {
			return
		}
		a := ctx.arrays[t.Entity]
		ensureArrayTypedefs(ctx, sb, a.Elem)
		ctx.arraysEmitted[t.Entity] = true
		fmt.Fprintf(sb, "typedef struct { %s a[%d]; } %s;\n", fromIRType(a.Elem).cType(ctx), a.Len, mangleType(a.Name))
	}
}

// arrayIndexCode - C элемента "массив[индекс]" (lvalue, если arrayCode -
// lvalue).
func arrayIndexCode(ctx *irGenCtx, e *ir.ArrayIndexExpr) (string, error) {
	arrCode, _, err := generateIRExpr(ctx, e.Array)
	if err != nil {
		return "", err
	}
	idxCode, _, err := generateIRExpr(ctx, e.Index)
	if err != nil {
		return "", err
	}
	n := ctx.arrays[ir.TypeOf(e.Array).Entity].Len
	ctx.usesPanic = true
	ctx.usesArrayIndex = true
	msg := escapeCString(rtmsg.IndexOutOfRange(ir.TypeOf(e.Array).String()))
	return fmt.Sprintf("(%s).a[rech_arr_idx(%s, %d, \"%s\")]", arrCode, idxCode, n, msg), nil
}

// arrayLiteralCode - "[a, b, ...]" как "T[N]" (compound literal); без
// элементов - нули.
func arrayLiteralCode(ctx *irGenCtx, e *ir.ArrayLiteralExpr) (string, error) {
	cType := mangleType(e.Type.Entity)
	if e.Elems == nil {
		return zeroValueCode(ctx, e.Type), nil
	}
	codes := make([]string, len(e.Elems))
	for i, el := range e.Elems {
		code, _, err := generateIRExpr(ctx, el)
		if err != nil {
			return "", err
		}
		codes[i] = code
	}
	decls, codes := sequenceArgs(ctx, e.Elems, codes)
	return sequenced(decls, fmt.Sprintf("((%s){ { %s } })", cType, strings.Join(codes, ", "))), nil
}

// Нулевое значение массива - "{0}" везде, кроме Строка: у неё ноль - "",
// не NULL. Массив (или структура-элемент), где есть строки, получает
// функцию rech_zero_<тип>, заполняющую их (ctx.zeroFuncs, после всех
// typedef'ов).

// zeroNeedsInit - нулевые байты - не нулевое значение t (где-то внутри
// есть Строка).
func zeroNeedsInit(ctx *irGenCtx, t ir.Type) bool {
	switch t.Kind {
	case ir.KindString:
		return true
	case ir.KindArray:
		return zeroNeedsInit(ctx, ctx.arrays[t.Entity].Elem)
	case ir.KindEntity:
		ent := ctx.entities[t.Entity]
		if ent == nil || !(ent.IsStruct || ent.IsExternStruct) {
			return false
		}
		for _, f := range ent.Fields {
			if zeroNeedsInit(ctx, f.Type) {
				return true
			}
		}
	}
	return false
}

// zeroValueCode - C-выражение нулевого значения t (элемент массива,
// массив без значения).
func zeroValueCode(ctx *irGenCtx, t ir.Type) string {
	if t.Kind == ir.KindString {
		return `""`
	}
	cType := fromIRType(t).cType(ctx)
	if !zeroNeedsInit(ctx, t) {
		return fmt.Sprintf("((%s){0})", cType)
	}
	fn := "rech_zero_" + strings.TrimPrefix(mangleType(t.String()), "rech_T_")
	if !ctx.zeroFuncsEmitted[fn] {
		ctx.zeroFuncsEmitted[fn] = true
		var body strings.Builder
		fmt.Fprintf(&body, "static %s %s(void) {\n    %s r = {0};\n", cType, fn, cType)
		if t.Kind == ir.KindArray {
			a := ctx.arrays[t.Entity]
			fmt.Fprintf(&body, "    for (long long i = 0; i < %d; i++) {\n        r.a[i] = %s;\n    }\n", a.Len, zeroValueCode(ctx, a.Elem))
		} else {
			for _, f := range ctx.entities[t.Entity].Fields {
				if zeroNeedsInit(ctx, f.Type) {
					fmt.Fprintf(&body, "    r.%s = %s;\n", mangleField(f.Name), zeroValueCode(ctx, f.Type))
				}
			}
		}
		body.WriteString("    return r;\n}\n")
		ctx.zeroFuncs.WriteString(body.String())
	}
	return fn + "()"
}
