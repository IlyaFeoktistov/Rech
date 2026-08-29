// Деепричастный вызов с блоком (шаг 6.5) - см. package doc
// internal/parser/gerund.go/internal/morph/gerund.go для полной картины;
// здесь - только проверка типов и построение IR.
package semantic

import (
	"fmt"

	"github.com/IlyaFeoktistov/Rech/internal/ast"
	"github.com/IlyaFeoktistov/Rech/internal/ir"
)

// analyzeGerundStmt разбирает ast.GerundStmt в ir.GerundStmt - Body
// результата несёт САМ вызов первым элементом (VarDecl "результат" +
// CallExpr), затем проанализированные инструкции s.Body - обе части в
// ОДНОЙ новой области видимости (sc.push()/sc.pop() вокруг обеих, не
// только вокруг s.Body) - "результат" не должна быть видна снаружи этого
// блока, тем же принципом, что и переменная цикла "для".
func analyzeGerundStmt(sc *ctx, s *ast.GerundStmt) (ir.Stmt, error) {
	sig, ok := sc.funcs[s.Callee]
	if !ok {
		return nil, fmt.Errorf("строка %d: функция %q не объявлена", s.Line, s.Callee)
	}
	if !sig.HasReturn {
		return nil, fmt.Errorf("строка %d: деепричастие требует функцию с объявленным типом результата - у %q его нет, \"результат\" не от чего образовать",
			s.Line, s.Callee)
	}
	args, err := checkCallArgs(sc, s.Args, sig, s.Line, fmt.Sprintf("функция %q", s.Callee), fmt.Sprintf("деепричастного вызова %q", s.Callee))
	if err != nil {
		return nil, err
	}
	call := &ir.CallExpr{Callee: s.Callee, Args: args, Type: sig.ReturnType, HasReturn: true}

	sc.push()
	if err := sc.declare("результат", sig.ReturnType); err != nil {
		sc.pop()
		return nil, fmt.Errorf("строка %d: %w", s.Line, err)
	}
	bodyIR, err := analyzeBlock(sc, s.Body)
	sc.pop()
	if err != nil {
		return nil, err
	}

	body := make([]ir.Stmt, 0, len(bodyIR)+1)
	body = append(body, &ir.VarDecl{Name: "результат", Value: call, Line: s.Line})
	body = append(body, bodyIR...)
	return &ir.GerundStmt{Body: body, Line: s.Line}, nil
}
