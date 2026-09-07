package interp

import (
	"bytes"
	"strings"
	"testing"

	"github.com/IlyaFeoktistov/Rech/internal/lexer"
	"github.com/IlyaFeoktistov/Rech/internal/parser"
	"github.com/IlyaFeoktistov/Rech/internal/rtmsg"
	"github.com/IlyaFeoktistov/Rech/internal/semantic"
)

// TestRunCryptoBuiltins - "хеш_sha256"/"подпись_hmac_sha256"/
// "закодировать_base64url"/"декодировать_base64url" - значения сверены с
// общеизвестными тестовыми векторами (SHA-256("abc"), HMAC-SHA256 из
// FIPS/RFC-примеров) - то же самое, что и cbackend.
// TestGenerateFromIRCryptoBuiltinCalls там же должен посчитать побайтово
// одинаково (см. package doc crypto.go в cbackend).
func TestRunCryptoBuiltins(t *testing.T) {
	got := runSrc(t, `
функция главная
{
    вывести хеш_sha256("")
    вывести хеш_sha256("abc")
    вывести подпись_hmac_sha256("The quick brown fox jumps over the lazy dog", "key")
    переменная закодировано = закодировать_base64url("hello world?")
    вывести закодировано
    вывести декодировать_base64url(закодировано)
}
`)
	want := "e3b0c44298fc1c149afbf4c8996fb92427ae41e4649b934ca495991b7852b855\n" +
		"ba7816bf8f01cfea414140de5dae2223b00361a396177a9cb410ff61f20015ad\n" +
		"f7bc83f430538424b13298e6aa6fb143ef4d59a14946175997479dbc2d1a3cd8\n" +
		"aGVsbG8gd29ybGQ_\n" +
		"hello world?\n"
	if got != want {
		t.Fatalf("ожидалось %q, получено %q", want, got)
	}
}

// TestRunSHA1AndMD5Builtins - "хеш_sha1"/"хеш_md5" - значения сверены с
// общеизвестными тестовыми векторами (пустая строка, "abc") - то же самое,
// что и cbackend.TestGenerateFromIRSHA1AndMD5BuiltinCalls (там - только
// присутствие вызова в сгенерированном C, не значение - см. package doc
// crypto.go в cbackend про побайтовое совпадение).
func TestRunSHA1AndMD5Builtins(t *testing.T) {
	got := runSrc(t, `
функция главная
{
    вывести хеш_sha1("")
    вывести хеш_sha1("abc")
    вывести хеш_md5("")
    вывести хеш_md5("abc")
}
`)
	want := "da39a3ee5e6b4b0d3255bfef95601890afd80709\n" +
		"a9993e364706816aba3e25717850c26c9cd0d89d\n" +
		"d41d8cd98f00b204e9800998ecf8427e\n" +
		"900150983cd24fb0d6963f7d28e17f72\n"
	if got != want {
		t.Fatalf("ожидалось %q, получено %q", want, got)
	}
}

func TestRunBase64URLDecodeInvalidCharPanics(t *testing.T) {
	toks, err := lexer.New(`
функция главная
{
    вывести декодировать_base64url("abc@def")
}
`).Tokenize()
	if err != nil {
		t.Fatalf("лексер: %v", err)
	}
	prog, err := parser.New(toks).ParseProgram()
	if err != nil {
		t.Fatalf("парсер: %v", err)
	}
	irProg, err := semantic.Analyze(prog)
	if err != nil {
		t.Fatalf("семантический анализ: %v", err)
	}
	var buf bytes.Buffer
	err = Run(irProg, &buf, strings.NewReader(""))
	if err == nil {
		t.Fatal("ожидалась ошибка времени выполнения (некорректный base64url)")
	}
	if !strings.Contains(err.Error(), rtmsg.Base64URLInvalidInput) {
		t.Fatalf("ожидалось сообщение %q, получено: %v", rtmsg.Base64URLInvalidInput, err)
	}
}
