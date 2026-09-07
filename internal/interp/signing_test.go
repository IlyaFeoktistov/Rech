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

// TestRunSigningRoundTripRSAAndEd25519 - "сгенерировать_приватный_ключ"/
// "получить_публичный_ключ"/"подписать"/"проверить_подпись" для обоих
// алгоритмов - ключ/подпись непредсказуемы (случайная генерация,
// RSA-подпись тоже недетерминирована из-за PKCS#1 v1.5 padding в OpenSSL/
// Go), поэтому проверяется round trip, не конкретное значение.
func TestRunSigningRoundTripRSAAndEd25519(t *testing.T) {
	got := runSrc(t, `
функция главная
{
    переменная приватный_ключ_rsa = сгенерировать_приватный_ключ(АлгоритмПодписи.RSA)
    переменная публичный_ключ_rsa = получить_публичный_ключ(приватный_ключ_rsa)
    переменная подпись_rsa = подписать("привет", приватный_ключ_rsa)
    вывести проверить_подпись("привет", подпись_rsa, публичный_ключ_rsa)
    вывести проверить_подпись("подделка", подпись_rsa, публичный_ключ_rsa)

    переменная приватный_ключ_ed = сгенерировать_приватный_ключ(АлгоритмПодписи.Ed25519)
    переменная публичный_ключ_ed = получить_публичный_ключ(приватный_ключ_ed)
    переменная подпись_ed = подписать("привет", приватный_ключ_ed)
    вывести проверить_подпись("привет", подпись_ed, публичный_ключ_ed)
    вывести проверить_подпись("подделка", подпись_ed, публичный_ключ_ed)
}
`)
	want := "истина\nложь\nистина\nложь\n"
	if got != want {
		t.Fatalf("ожидалось %q, получено %q", want, got)
	}
}

func TestRunSigningUnknownAlgorithmPanics(t *testing.T) {
	toks, err := lexer.New(`
функция главная
{
    вывести сгенерировать_приватный_ключ(99)
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
		t.Fatal("ожидалась ошибка времени выполнения (неизвестный алгоритм)")
	}
	if !strings.Contains(err.Error(), rtmsg.UnknownSignatureAlgorithm) {
		t.Fatalf("ожидалось сообщение %q, получено: %v", rtmsg.UnknownSignatureAlgorithm, err)
	}
}

func TestRunSigningInvalidPrivateKeyPanics(t *testing.T) {
	toks, err := lexer.New(`
функция главная
{
    вывести подписать("данные", "не PEM вовсе")
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
		t.Fatal("ожидалась ошибка времени выполнения (некорректный приватный ключ)")
	}
	if !strings.Contains(err.Error(), rtmsg.SigningInvalidPrivateKey) {
		t.Fatalf("ожидалось сообщение %q, получено: %v", rtmsg.SigningInvalidPrivateKey, err)
	}
}

func TestRunSigningInvalidSignatureHexPanics(t *testing.T) {
	toks, err := lexer.New(`
функция главная
{
    переменная приватный_ключ = сгенерировать_приватный_ключ(АлгоритмПодписи.Ed25519)
    переменная публичный_ключ = получить_публичный_ключ(приватный_ключ)
    вывести проверить_подпись("данные", "не-hex-совсем", публичный_ключ)
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
		t.Fatal("ожидалась ошибка времени выполнения (некорректный HEX подписи)")
	}
	if !strings.Contains(err.Error(), rtmsg.SigningInvalidSignatureHex) {
		t.Fatalf("ожидалось сообщение %q, получено: %v", rtmsg.SigningInvalidSignatureHex, err)
	}
}
