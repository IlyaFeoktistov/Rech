package cbackend

import (
	"strings"
	"testing"

	"github.com/IlyaFeoktistov/Rech/internal/ir"
)

func TestGenerateFromIRCryptoBuiltinCalls(t *testing.T) {
	prog := &ir.Program{
		Functions: []*ir.Function{{Name: "главная", Body: []ir.Stmt{
			&ir.PrintStmt{Value: &ir.CallExpr{
				Callee: "хеш_sha256", Args: []ir.Expr{&ir.StringLiteral{Value: "abc"}},
				Type: ir.TypeString, HasReturn: true,
			}},
			&ir.PrintStmt{Value: &ir.CallExpr{
				Callee: "подпись_hmac_sha256",
				Args:   []ir.Expr{&ir.StringLiteral{Value: "data"}, &ir.StringLiteral{Value: "key"}},
				Type:   ir.TypeString, HasReturn: true,
			}},
			&ir.PrintStmt{Value: &ir.CallExpr{
				Callee: "закодировать_base64url", Args: []ir.Expr{&ir.StringLiteral{Value: "text"}},
				Type: ir.TypeString, HasReturn: true,
			}},
			&ir.PrintStmt{Value: &ir.CallExpr{
				Callee: "декодировать_base64url", Args: []ir.Expr{&ir.StringLiteral{Value: "dGV4dA"}},
				Type: ir.TypeString, HasReturn: true,
			}},
		}}},
	}
	out, err := GenerateFromIR(prog)
	if err != nil {
		t.Fatalf("неожиданная ошибка: %v", err)
	}
	for _, want := range []string{
		"rech_sha256_hex(",
		"rech_hmac_sha256_hex(",
		"rech_base64url_encode(",
		"rech_base64url_decode(",
		"static char *rech_sha256_hex(const char *s)",
		"static char *rech_hmac_sha256_hex(const char *data, const char *key)",
		"static char *rech_base64url_encode(const char *s)",
		"static char *rech_base64url_decode(const char *s)",
	} {
		if !strings.Contains(out, want) {
			t.Fatalf("ожидалось вхождение %q, получено:\n%s", want, out)
		}
	}
}

func TestGenerateFromIRWithoutCryptoOmitsHelpers(t *testing.T) {
	prog := &ir.Program{
		Functions: []*ir.Function{{Name: "главная", Body: []ir.Stmt{
			&ir.PrintStmt{Value: &ir.IntLiteral{Value: 1}},
		}}},
	}
	out, err := GenerateFromIR(prog)
	if err != nil {
		t.Fatalf("неожиданная ошибка: %v", err)
	}
	if strings.Contains(out, "rech_sha256_hex") || strings.Contains(out, "rech_hmac_sha256_hex") ||
		strings.Contains(out, "rech_base64url_encode") || strings.Contains(out, "rech_base64url_decode") {
		t.Fatalf("не ожидались crypto-хелперы без использования соответствующих builtin'ов, получено:\n%s", out)
	}
}
