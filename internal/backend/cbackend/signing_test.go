package cbackend

import (
	"strings"
	"testing"

	"github.com/IlyaFeoktistov/Rech/internal/ir"
)

func TestGenerateFromIRSigningBuiltinCalls(t *testing.T) {
	prog := &ir.Program{
		Functions: []*ir.Function{{Name: "главная", Body: []ir.Stmt{
			&ir.PrintStmt{Value: &ir.CallExpr{
				Callee: "сгенерировать_приватный_ключ", Args: []ir.Expr{&ir.IntLiteral{Value: 1}},
				Type: ir.TypeString, HasReturn: true,
			}},
			&ir.PrintStmt{Value: &ir.CallExpr{
				Callee: "получить_публичный_ключ", Args: []ir.Expr{&ir.StringLiteral{Value: "pem"}},
				Type: ir.TypeString, HasReturn: true,
			}},
			&ir.PrintStmt{Value: &ir.CallExpr{
				Callee: "подписать",
				Args:   []ir.Expr{&ir.StringLiteral{Value: "data"}, &ir.StringLiteral{Value: "pem"}},
				Type:   ir.TypeString, HasReturn: true,
			}},
			&ir.PrintStmt{Value: &ir.CallExpr{
				Callee: "проверить_подпись",
				Args:   []ir.Expr{&ir.StringLiteral{Value: "data"}, &ir.StringLiteral{Value: "sig"}, &ir.StringLiteral{Value: "pem"}},
				Type:   ir.TypeBool, HasReturn: true,
			}},
		}}},
	}
	out, err := GenerateFromIR(prog)
	if err != nil {
		t.Fatalf("неожиданная ошибка: %v", err)
	}
	for _, want := range []string{
		"rech_generate_private_key_pem(",
		"rech_get_public_key_pem(",
		"rech_sign_hex(",
		"rech_verify_signature_hex(",
		"static char *rech_generate_private_key_pem(int algorithm)",
		"static char *rech_get_public_key_pem(const char *private_key_pem)",
		"static char *rech_sign_hex(const char *data, const char *private_key_pem)",
		"static bool rech_verify_signature_hex(const char *data, const char *signature_hex, const char *public_key_pem)",
		"#include <openssl/evp.h>",
		"#include <openssl/pem.h>",
		"#include <openssl/bio.h>",
	} {
		if !strings.Contains(out, want) {
			t.Fatalf("ожидалось вхождение %q, получено:\n%s", want, out)
		}
	}
}

func TestGenerateFromIRWithoutSigningOmitsHelpers(t *testing.T) {
	prog := &ir.Program{
		Functions: []*ir.Function{{Name: "главная", Body: []ir.Stmt{
			&ir.PrintStmt{Value: &ir.IntLiteral{Value: 1}},
		}}},
	}
	out, err := GenerateFromIR(prog)
	if err != nil {
		t.Fatalf("неожиданная ошибка: %v", err)
	}
	if strings.Contains(out, "rech_generate_private_key_pem") || strings.Contains(out, "rech_sign_hex") ||
		strings.Contains(out, "#include <openssl/pem.h>") {
		t.Fatalf("не ожидались signing-хелперы без использования соответствующих builtin'ов, получено:\n%s", out)
	}
}

// TestGenerateFromIRSigningWithoutAESOmitsDuplicateEVPInclude - usesSigning
// БЕЗ usesAES всё равно подключает <openssl/evp.h> ровно один раз (сам
// добавляет его, раз usesAES этого не сделал) - см. package doc ветку
// "if !ctx.usesAES" в ir_generator.go.
func TestGenerateFromIRSigningWithoutAESOmitsDuplicateEVPInclude(t *testing.T) {
	prog := &ir.Program{
		Functions: []*ir.Function{{Name: "главная", Body: []ir.Stmt{
			&ir.PrintStmt{Value: &ir.CallExpr{
				Callee: "получить_публичный_ключ", Args: []ir.Expr{&ir.StringLiteral{Value: "pem"}},
				Type: ir.TypeString, HasReturn: true,
			}},
		}}},
	}
	out, err := GenerateFromIR(prog)
	if err != nil {
		t.Fatalf("неожиданная ошибка: %v", err)
	}
	if strings.Count(out, "#include <openssl/evp.h>") != 1 {
		t.Fatalf("ожидалось ровно одно вхождение \"#include <openssl/evp.h>\", получено:\n%s", out)
	}
}
