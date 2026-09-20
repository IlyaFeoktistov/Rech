package main

import (
	"os"
	"path/filepath"
	"testing"
)

// captureCheckOutput перехватывает stdout checkOneFile/runFmt --check - оба
// пишут путь непроверенного файла через fmt.Println(path), не возвращают
// его как значение.
func captureCheckOutput(t *testing.T, fn func() bool) (string, bool) {
	t.Helper()
	r, w, err := os.Pipe()
	if err != nil {
		t.Fatal(err)
	}
	old := os.Stdout
	os.Stdout = w
	ok := fn()
	w.Close()
	os.Stdout = old
	buf := make([]byte, 4096)
	n, _ := r.Read(buf)
	return string(buf[:n]), ok
}

func TestCheckOneFileFormatted(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "a.rech")
	if err := os.WriteFile(path, []byte("функция главная()\n{\n}\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	out, ok := captureCheckOutput(t, func() bool { return checkOneFile(path) })
	if !ok {
		t.Fatalf("уже отформатированный файл: checkOneFile вернул false, вывод %q", out)
	}
	if out != "" {
		t.Fatalf("уже отформатированный файл: ничего не должно печататься, получено %q", out)
	}
}

func TestCheckOneFileUnformatted(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "b.rech")
	const unformatted = "функция главная()\n{\n  переменная х = 1\n}\n"
	if err := os.WriteFile(path, []byte(unformatted), 0o644); err != nil {
		t.Fatal(err)
	}
	out, ok := captureCheckOutput(t, func() bool { return checkOneFile(path) })
	if ok {
		t.Fatalf("неотформатированный файл: checkOneFile вернул true")
	}
	if out != path+"\n" {
		t.Fatalf("ожидался путь файла в stdout, получено %q", out)
	}
	// --check не должен ничего переписывать на диске.
	src, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if string(src) != unformatted {
		t.Fatalf("--check изменил файл на диске: %q", src)
	}
}

func TestRunFmtWriteAndCheckIncompatible(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "c.rech")
	if err := os.WriteFile(path, []byte("функция главная()\n{\n}\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if runFmt([]string{"--write", "--check", path}) {
		t.Fatal("--write вместе с --check обязаны быть ошибкой")
	}
}

func TestRunFmtCheckDirectory(t *testing.T) {
	dir := t.TempDir()
	good := filepath.Join(dir, "good.rech")
	bad := filepath.Join(dir, "bad.rech")
	if err := os.WriteFile(good, []byte("функция главная()\n{\n}\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(bad, []byte("функция главная()\n{\n  переменная х = 1\n}\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	out, ok := captureCheckOutput(t, func() bool { return runFmt([]string{"--check", dir}) })
	if ok {
		t.Fatal("каталог с неотформатированным файлом: runFmt --check обязан вернуть false")
	}
	if out != bad+"\n" {
		t.Fatalf("ожидался только путь неотформатированного файла, получено %q", out)
	}
}
