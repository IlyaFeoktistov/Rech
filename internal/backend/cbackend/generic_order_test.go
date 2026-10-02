package cbackend

import (
	"os"
	"os/exec"
	"path/filepath"
	"testing"
)

// TestGenerateFromIRNestedCollectionOfStructCompiles - коллекция в коллекции
// с элементом-структурой ("Список<Список<Пара>>", "Словарь<Строка,
// Список<?Пара>>"): внешняя коллекция идёт после внутренней, её "равен"
// видит "равен" внутренней - C компилируется.
func TestGenerateFromIRNestedCollectionOfStructCompiles(t *testing.T) {
	out := generateFromSrc(t, `структура Пара
{
    имеет:
        Целое a
}

функция главная
{
    Список<Список<Список<Пара>>> т = [[[новая Пара(a: 1)]]]
    Словарь<Строка, Список<?Пара>> д = новый Словарь<Строка, Список<?Пара>>()
    Список<Список<Пара>> х = [[новая Пара(a: 1)]]
    утверждать х.равен([[новая Пара(a: 1)]])
    вывести т.длина() + д.длина()
}
`)
	ccPath := findTestCCompiler(t)
	cPath := filepath.Join(t.TempDir(), "nested.c")
	if err := os.WriteFile(cPath, []byte(out), 0o644); err != nil {
		t.Fatal(err)
	}
	// #nosec G204 -- ccPath найден локальным поиском (CC/cc/clang/gcc), cPath - файл, только что написанный этим же тестом.
	if msg, err := exec.Command(ccPath, "-fsyntax-only", "-Werror=implicit-function-declaration", cPath).CombinedOutput(); err != nil {
		t.Fatalf("сгенерированный C не компилируется: %v\n%s", err, msg)
	}
}
