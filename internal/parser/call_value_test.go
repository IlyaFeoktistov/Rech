package parser

import (
	"testing"

	"github.com/IlyaFeoktistov/Rech/internal/ast"
)

// TestParseCallOfCallResult - "(" вплотную после вызова/индексации - вызов
// результата; как выражение и как инструкция.
func TestParseCallOfCallResult(t *testing.T) {
	prog, err := parseSource(t, "функция главная\n{\n    переменная р = сп.получить(0)(3)\n    создать()(\"а\")\n}\n")
	if err != nil {
		t.Fatalf("неожиданная ошибка: %v", err)
	}
	body := prog.Functions[0].Body
	if _, ok := body[0].(*ast.VarDecl).Value.(*ast.CallValueExpr); !ok {
		t.Fatalf("ожидался CallValueExpr в значении, получено %T", body[0].(*ast.VarDecl).Value)
	}
	if s, ok := body[1].(*ast.ExprStmt); !ok || s.CallValue == nil {
		t.Fatalf("ожидалась инструкция-вызов результата, получено %+v", body[1])
	}
}
