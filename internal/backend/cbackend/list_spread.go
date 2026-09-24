package cbackend

import (
	"fmt"
	"strings"

	"github.com/IlyaFeoktistov/Rech/internal/ir"
)

// listLiteralFromParts - литерал списка со спредом ("[a, b, ...c, d]"):
// подряд идущие обычные элементы собираются во временный список
// (_from_literal), "...c" - сам список c; итог склеивает части по порядку
// (_from_parts, collections.ListLiteralCode). codes - уже сгенерированный
// C-код элементов e.Elems.
func listLiteralFromParts(e *ir.ListLiteralExpr, codes []string, elemC string) string {
	structC := mangleType(e.CollectionName)
	var parts, run []string
	flush := func() {
		if len(run) == 0 {
			return
		}
		parts = append(parts, fmt.Sprintf("%s_from_literal(%dLL, (%s[]){%s})", structC, len(run), elemC, strings.Join(run, ", ")))
		run = nil
	}
	for i, code := range codes {
		if e.Spread[i] {
			flush()
			parts = append(parts, code)
			continue
		}
		run = append(run, code)
	}
	flush()
	return fmt.Sprintf("%s_from_parts(%dLL, (%s *[]){%s})", structC, len(parts), structC, strings.Join(parts, ", "))
}
