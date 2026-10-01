package cbackend

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

// TestGenerateFromIRModuleVars - переменная модуля - глобальная C-переменная
// под тем же именем, что и локальная (mangleVar), её значение
// присваивается в rech_init_globals(), которую main() зовёт до первой
// инструкции; сгенерированный C компилируется.
func TestGenerateFromIRModuleVars(t *testing.T) {
	out := generateFromSrc(t, `Целое счётчик = 0
?Строка имя = Ничто
Список<Целое> числа = [1, 2]

функция главная
{
    счётчик++
    числа.добавить(счётчик)
}
`)
	for _, want := range []string{
		"static long long " + mangleVar("счётчик") + ";",
		"static void rech_init_globals(void) {",
		"    rech_init_globals();\n",
	} {
		if !strings.Contains(out, want) {
			t.Fatalf("ожидалось %q в сгенерированном C:\n%s", want, out)
		}
	}
	if strings.Index(out, "static void rech_init_globals") > strings.Index(out, "int main(") {
		t.Fatalf("rech_init_globals должна быть определена до main")
	}
	ccPath := findTestCCompiler(t)
	cPath := filepath.Join(t.TempDir(), "globals.c")
	if err := os.WriteFile(cPath, []byte(out), 0o644); err != nil {
		t.Fatal(err)
	}
	// #nosec G204 -- ccPath найден локальным поиском (CC/cc/clang/gcc), cPath - файл, только что написанный этим же тестом.
	if msg, err := exec.Command(ccPath, "-fsyntax-only", cPath).CombinedOutput(); err != nil {
		t.Fatalf("сгенерированный C не компилируется: %v\n%s", err, msg)
	}
}

// TestGenerateFromIRTypeEquality - "Тип == Тип" - rech_type_eq (по имени
// дескриптора), не сравнение указателей.
func TestGenerateFromIRTypeEquality(t *testing.T) {
	out := generateFromSrc(t, "функция главная\n{\n    вывести тип Натуральное8 == тип Байт\n    вывести тип Целое != тип Байт\n}\n")
	if !strings.Contains(out, "rech_type_eq(") || !strings.Contains(out, "(!rech_type_eq(") || !strings.Contains(out, "static bool rech_type_eq(") {
		t.Fatalf("ожидалось сравнение через rech_type_eq:\n%s", out)
	}
}
