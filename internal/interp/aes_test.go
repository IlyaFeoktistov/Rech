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

// TestRunAESEncryptDecryptRoundTrip - "зашифровать"/"расшифровать" - nonce
// случаен (RAND/crypto.rand), поэтому сам шифротекст непредсказуем -
// проверяется round trip (расшифровать(зашифровать(x)) == x), не
// конкретное значение.
func TestRunAESEncryptDecryptRoundTrip(t *testing.T) {
	got := runSrc(t, `
функция главная
{
    переменная шифротекст = зашифровать("секретное сообщение", "пароль123")
    вывести расшифровать(шифротекст, "пароль123")
}
`)
	if got != "секретное сообщение\n" {
		t.Fatalf("ожидалось %q, получено %q", "секретное сообщение\n", got)
	}
}

// TestRunAESDecryptCbackendCiphertext - шифротекст, полученный реальной
// cbackend-сборкой ("зашифровать(\"секретное сообщение\", \"пароль123\")"),
// расшифровывается интерпретатором - подтверждает побайтовое совпадение
// раскладки HEX(nonce || шифротекст || тег) между backend'ами (см. package
// doc internal/backend/cbackend/aes.go).
func TestRunAESDecryptCbackendCiphertext(t *testing.T) {
	got := runSrc(t, `
функция главная
{
    вывести расшифровать("1e09b7de22f491fc59b37762f6244f9abfdb4a7fbb985dff29ef21feec28bfa97266b07d34da48d5d135f65c97895900230a6e798871a2033dbadb1e895f4004e4", "пароль123")
}
`)
	if got != "секретное сообщение\n" {
		t.Fatalf("ожидалось %q, получено %q", "секретное сообщение\n", got)
	}
}

func TestRunAESDecryptWrongKeyPanics(t *testing.T) {
	toks, err := lexer.New(`
функция главная
{
    переменная шифротекст = зашифровать("секрет", "правильный-ключ")
    вывести расшифровать(шифротекст, "неверный-ключ")
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
		t.Fatal("ожидалась ошибка времени выполнения (неверный ключ)")
	}
	if !strings.Contains(err.Error(), rtmsg.AESAuthenticationFailed) {
		t.Fatalf("ожидалось сообщение %q, получено: %v", rtmsg.AESAuthenticationFailed, err)
	}
}

func TestRunAESDecryptInvalidCiphertextPanics(t *testing.T) {
	toks, err := lexer.New(`
функция главная
{
    вывести расшифровать("не-hex-совсем", "ключ")
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
		t.Fatal("ожидалась ошибка времени выполнения (некорректный шифротекст)")
	}
	if !strings.Contains(err.Error(), rtmsg.AESCiphertextInvalid) {
		t.Fatalf("ожидалось сообщение %q, получено: %v", rtmsg.AESCiphertextInvalid, err)
	}
}
