package cbackend

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/IlyaFeoktistov/Rech/internal/lexer"
	"github.com/IlyaFeoktistov/Rech/internal/parser"
	"github.com/IlyaFeoktistov/Rech/internal/semantic"
)

// TestGenerateLibFromIRInitializesStaticsAndGlobals - у библиотеки нет
// main(), статические поля и переменные модуля инициализирует
// rech_init_library (constructor) до main() программы-потребителя.
func TestGenerateLibFromIRInitializesStaticsAndGlobals(t *testing.T) {
	toks, err := lexer.New(`приватная Целое основа = 41

сущность Счётчик
{
    имеет:
        статическое начало: Целое = 100
}

экспортировать функция значение(): Целое
{
    вернуть основа + 1
}

экспортировать функция начало_счётчика(): Целое
{
    вернуть Счётчик::начало
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
	cSource, _, _, err := GenerateLibFromIR(irProg, "")
	if err != nil {
		t.Fatalf("генерация: %v", err)
	}
	if !strings.Contains(cSource, "__attribute__((constructor)) static void rech_init_library(void) {") {
		t.Fatalf("ожидалась rech_init_library в библиотеке:\n%s", cSource)
	}

	ccPath := findTestCCompiler(t)
	dir := t.TempDir()
	libPath := filepath.Join(dir, "lib.c")
	mainPath := filepath.Join(dir, "main.c")
	binPath := filepath.Join(dir, "prog")
	if err := os.WriteFile(libPath, []byte(cSource), 0o644); err != nil {
		t.Fatal(err)
	}
	mainSrc := "#include <stdio.h>\nlong long значение(void);\nlong long начало_счётчика(void);\n" +
		"int main(void) { printf(\"%lld %lld\\n\", значение(), начало_счётчика()); return 0; }\n"
	if err := os.WriteFile(mainPath, []byte(mainSrc), 0o644); err != nil {
		t.Fatal(err)
	}
	// #nosec G204 -- ccPath найден локальным поиском (CC/cc/clang/gcc), пути - файлы этого теста.
	if msg, err := exec.Command(ccPath, "-o", binPath, libPath, mainPath, "-lgc", "-lm").CombinedOutput(); err != nil {
		t.Fatalf("не собирается: %v\n%s", err, msg)
	}
	// #nosec G204 -- binPath только что собран этим тестом.
	out, err := exec.Command(binPath).CombinedOutput()
	if err != nil {
		t.Fatalf("запуск: %v\n%s", err, out)
	}
	if got := strings.TrimSpace(string(out)); got != "42 100" {
		t.Fatalf("ожидалось \"42 100\", получено %q", got)
	}
}
