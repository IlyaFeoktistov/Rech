package semantic

import (
	"github.com/IlyaFeoktistov/Rech/internal/ast"
	"github.com/IlyaFeoktistov/Rech/internal/diagnostics"
	"github.com/IlyaFeoktistov/Rech/internal/ir"
)

// bareCallableType - "Вызываемый" без "(...)" в объявлении переменной с
// инициализатором ("Вызываемый фн = функция () {...}"): сигнатура - та же,
// что у значения справа.
const bareCallableType = "Вызываемый"

func analyzeBareCallableVarDecl(sc *ctx, s *ast.VarDecl) (ir.Stmt, error) {
	if s.IsStatic {
		return nil, errAtLine(s.Line, CodeStmtCheck, "у статической переменной %q тип \"Вызываемый\" пишется с сигнатурой: \"Вызываемый(...)\"", s.Name)
	}
	value, err := analyzeExpr(sc, s.Value)
	if err != nil {
		return nil, err
	}
	typ := ir.TypeOf(value)
	if typ.Kind != ir.KindCallable {
		return nil, errAtLine(s.Line, CodeStmtCheck, "переменная %q объявлена типом \"Вызываемый\", но инициализирована значением типа %s", s.Name, typ)
	}
	newEstablished := establishedForNewBinding(sc, s.Name, value)
	if err := sc.declare(s.Name, typ); err != nil {
		return nil, diagnostics.WithPos(err, diagnostics.Position{File: currentFile, Line: s.Line})
	}
	sc.setEstablished(s.Name, newEstablished)
	return &ir.VarDecl{Name: s.Name, Value: value, Line: s.Line}, nil
}
