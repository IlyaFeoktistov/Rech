package cbackend

import (
	"strings"
	"testing"

	"github.com/IlyaFeoktistov/Rech/internal/ir"
)

func datagramFieldsForTest() []ir.Field {
	return []ir.Field{
		{Name: "данные", Type: ir.TypeString}, {Name: "адрес", Type: ir.TypeString}, {Name: "порт", Type: ir.TypeInt},
	}
}

// TestGenerateFromIRUDPDatagramBuiltinCalls - "получить_от"/"отправить_в"
// (recvfrom/sendto над udp-узлом без connect, в отличие от "получить"/
// "отправить" на подключённом сокете) - см. package doc udp.go.
func TestGenerateFromIRUDPDatagramBuiltinCalls(t *testing.T) {
	prog := &ir.Program{
		Entities: []*ir.Entity{{Name: "Дейтаграмма", IsStruct: true, Fields: datagramFieldsForTest()}},
		Functions: []*ir.Function{{Name: "главная", Body: []ir.Stmt{
			&ir.VarDecl{Name: "узел", Value: &ir.CallExpr{
				Callee: "сетевой_узел",
				Args:   []ir.Expr{&ir.StringLiteral{Value: "udp"}, &ir.StringLiteral{Value: ""}, &ir.IntLiteral{Value: 9090}},
				Type:   ir.TypeSocket, HasReturn: true,
			}},
			&ir.VarDecl{Name: "д", Value: &ir.CallExpr{
				Callee: "получить_от",
				Args:   []ir.Expr{&ir.Identifier{Name: "узел", Type: ir.TypeSocket}, &ir.IntLiteral{Value: 1024}},
				Type:   ir.EntityType("Дейтаграмма"), HasReturn: true,
			}},
			&ir.ExprStmt{Call: &ir.CallExpr{
				Callee: "отправить_в",
				Args: []ir.Expr{
					&ir.Identifier{Name: "узел", Type: ir.TypeSocket},
					&ir.StringLiteral{Value: "эхо"},
					&ir.FieldAccess{Receiver: &ir.Identifier{Name: "д", Type: ir.EntityType("Дейтаграмма")}, Field: "адрес", Type: ir.TypeString},
					&ir.FieldAccess{Receiver: &ir.Identifier{Name: "д", Type: ir.EntityType("Дейтаграмма")}, Field: "порт", Type: ir.TypeInt},
				},
			}},
		}}},
	}
	out, err := GenerateFromIR(prog)
	if err != nil {
		t.Fatalf("неожиданная ошибка: %v", err)
	}
	for _, want := range []string{
		`rech_network_node("udp", "", 9090LL)`,
		"rech_recv_from(v_uzel, 1024LL)",
		"static void rech_send_to(rech_socket_t s, const char *data, const char *addr, long long port)",
		"rech_send_to(v_uzel, \"эхо\",",
	} {
		if !strings.Contains(out, want) {
			t.Fatalf("ожидалось %q в выводе, получено:\n%s", want, out)
		}
	}
}
