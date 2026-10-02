package semantic

import "github.com/IlyaFeoktistov/Rech/internal/ast"

// uninferableVarError - "переменная имя = значение", тип которого из самого
// значения не вычислить: "Ничто", "[]", список только из "Ничто". Подсказка -
// та же строка с явным типом ("Тип имя = ..."). nil - тип вычисляется.
func uninferableVarError(s *ast.VarDecl) error {
	var what, typeHint string
	switch v := s.Value.(type) {
	case *ast.NullLiteral:
		what, typeHint = "\"Ничто\"", "?Тип"
	case *ast.ListLiteral:
		if len(v.Elems) == 0 {
			what, typeHint = "пустого списка \"[]\"", "Список<Тип>"
		} else if allNullLiterals(v.Elems) {
			what, typeHint = "списка только из \"Ничто\"", "Список<?Тип>"
		}
	}
	if what == "" {
		return nil
	}
	return errAtLine(s.Line, CodeVarInference, "невозможно вычислить тип переменной %q из %s - укажите тип явно", s.Name, what).
		WithHint("%s %s = ...", typeHint, s.Name)
}

func allNullLiterals(elems []ast.Expr) bool {
	for _, e := range elems {
		if _, ok := e.(*ast.NullLiteral); !ok {
			return false
		}
	}
	return true
}
