package semantic

import (
	"github.com/IlyaFeoktistov/Rech/internal/ast"
	"github.com/IlyaFeoktistov/Rech/internal/diagnostics"
	"github.com/IlyaFeoktistov/Rech/internal/ir"
)

// Явный тип переменной цикла: "для Целое32 i от 0 до 9", "для Байт б в
// байтах", "для Строка к, Целое з в словаре".

// rangeVarType - тип переменной "для [Тип] i от ... до ...": Целое, если
// не указан; указан - любой целый тип, Байт или Символ.
func rangeVarType(sc *ctx, s *ast.ForStmt) (ir.Type, error) {
	if s.VarType == "" {
		return ir.TypeInt, nil
	}
	t, err := resolveTypeName(sc.entities, sc.methods, sc.aliases, sc.interfaces, sc.enums, s.VarType)
	if err != nil {
		return ir.TypeUnknown, diagnostics.WithPos(err, diagnostics.Position{File: currentFile, Line: s.Line})
	}
	if _, _, isInt := t.Kind.IntWidthSigned(); !isInt && t != ir.TypeByte && t != ir.TypeChar {
		return ir.TypeUnknown, errAtLine(s.Line, CodeStmtCheck, "переменная цикла \"для %s от ... до\" - целого типа, Байт или Символ, а не %s", s.Var, t)
	}
	return t, nil
}

// rangeBound - граница диапазона типа t (константа приводится к t).
func rangeBound(sc *ctx, e ast.Expr, t ir.Type, word string, line int) (ir.Expr, error) {
	v, err := analyzeExpr(sc, e)
	if err != nil {
		return nil, err
	}
	if coerced, ok, err := coerceNumericConst(v, t, line); err != nil {
		return nil, err
	} else if ok {
		v = coerced
	}
	if typ := ir.TypeOf(v); typ != t {
		if t == ir.TypeInt {
			return nil, errAtLine(line, CodeStmtCheck, "граница диапазона после %q должна быть целым числом, получено %s", word, typ)
		}
		return nil, errAtLine(line, CodeStmtCheck, "граница диапазона после %q должна быть типа %s, как переменная цикла, получено %s", word, t, typ)
	}
	return v, nil
}

// checkLoopVarType - объявленный тип declared переменной name цикла "для
// ... в ..." совпадает с типом actual элемента/ключа ("" - не объявлен).
func checkLoopVarType(sc *ctx, declared string, actual ir.Type, name string, line int) error {
	if declared == "" {
		return nil
	}
	t, err := resolveTypeName(sc.entities, sc.methods, sc.aliases, sc.interfaces, sc.enums, declared)
	if err != nil {
		return diagnostics.WithPos(err, diagnostics.Position{File: currentFile, Line: line})
	}
	if t != actual {
		return errAtLine(line, CodeStmtCheck, "переменная цикла %q объявлена типом %s, а элементы - типа %s", name, t, actual)
	}
	return nil
}
