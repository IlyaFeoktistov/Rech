package cbackend

import (
	"strings"
	"testing"

	"github.com/IlyaFeoktistov/Rech/internal/ir"
)

func TestGenerateFromIRAESBuiltinCalls(t *testing.T) {
	prog := &ir.Program{
		Functions: []*ir.Function{{Name: "главная", Body: []ir.Stmt{
			&ir.PrintStmt{Value: &ir.CallExpr{
				Callee: "зашифровать",
				Args:   []ir.Expr{&ir.StringLiteral{Value: "data"}, &ir.StringLiteral{Value: "key"}},
				Type:   ir.TypeString, HasReturn: true,
			}},
			&ir.PrintStmt{Value: &ir.CallExpr{
				Callee: "расшифровать",
				Args:   []ir.Expr{&ir.StringLiteral{Value: "hex"}, &ir.StringLiteral{Value: "key"}},
				Type:   ir.TypeString, HasReturn: true,
			}},
		}}},
	}
	out, err := GenerateFromIR(prog)
	if err != nil {
		t.Fatalf("неожиданная ошибка: %v", err)
	}
	for _, want := range []string{
		"rech_aes_encrypt_hex(",
		"rech_aes_decrypt_hex(",
		"static char *rech_aes_encrypt_hex(const char *plaintext, const char *passphrase)",
		"static char *rech_aes_decrypt_hex(const char *ciphertext_hex, const char *passphrase)",
		"#include <openssl/evp.h>",
		"#include <openssl/rand.h>",
		// AES использует SHA-256 для получения ключа - cryptoHelpers должен
		// быть эмитирован тоже (usesAES тянет usesCrypto=true).
		"static char *rech_sha256_hex(const char *s)",
	} {
		if !strings.Contains(out, want) {
			t.Fatalf("ожидалось вхождение %q, получено:\n%s", want, out)
		}
	}
}

func TestGenerateFromIRWithoutAESOmitsHelpers(t *testing.T) {
	prog := &ir.Program{
		Functions: []*ir.Function{{Name: "главная", Body: []ir.Stmt{
			&ir.PrintStmt{Value: &ir.IntLiteral{Value: 1}},
		}}},
	}
	out, err := GenerateFromIR(prog)
	if err != nil {
		t.Fatalf("неожиданная ошибка: %v", err)
	}
	if strings.Contains(out, "rech_aes_encrypt_hex") || strings.Contains(out, "rech_aes_decrypt_hex") ||
		strings.Contains(out, "#include <openssl/evp.h>") {
		t.Fatalf("не ожидались AES-хелперы без использования \"зашифровать\"/\"расшифровать\", получено:\n%s", out)
	}
}
