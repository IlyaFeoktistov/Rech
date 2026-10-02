package cbackend

import (
	"fmt"
	"strings"

	"github.com/IlyaFeoktistov/Rech/internal/ir"
)

// generateIRTypedRangeFor - "для Байт б от 0 до 255": переменная цикла
// своего типа, конец проверяется ПОСЛЕ тела ("б == до" - выход), поэтому
// диапазон до максимума типа не зацикливается переполнением, как у "б <=
// 255" в C. Границы вычисляются один раз.
func generateIRTypedRangeFor(sb *strings.Builder, ctx *irGenCtx, s *ir.ForStmt, fromCode, toCode string, depth int) error {
	ind := indent(depth)
	cType := fromIRType(s.VarType).cType(ctx)
	varC := mangleVar(s.Var)
	ctx.chainTmpCounter++
	toC := fmt.Sprintf("rech_range_to%d", ctx.chainTmpCounter)
	fmt.Fprintf(sb, "%s{\n", ind)
	fmt.Fprintf(sb, "%s    %s %s = %s;\n", ind, cType, varC, fromCode)
	fmt.Fprintf(sb, "%s    %s %s = %s;\n", ind, cType, toC, toCode)
	fmt.Fprintf(sb, "%s    if (%s <= %s) for (;; %s++) {\n", ind, varC, toC, varC)
	lf := ctx.pushLoop()
	for _, st := range s.Body {
		if err := generateIRStmt(sb, ctx, st, depth+2); err != nil {
			ctx.popLoop()
			return err
		}
	}
	ctx.popLoop()
	fmt.Fprintf(sb, "%s%s: ;\n", indent(depth+2), lf.continueTarget)
	fmt.Fprintf(sb, "%s        if (%s == %s) break;\n", ind, varC, toC)
	fmt.Fprintf(sb, "%s    }\n", ind)
	fmt.Fprintf(sb, "%s}\n", ind)
	fmt.Fprintf(sb, "%s%s: ;\n", ind, lf.breakTarget)
	return nil
}
