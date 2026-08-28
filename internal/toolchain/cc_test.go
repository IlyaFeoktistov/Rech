package toolchain

import (
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

// fakeCC - записывающий скрипт-заглушка вместо настоящего cc: сохраняет
// свои аргументы в файл рядом с "выходным" файлом (последний по позиции
// после "-o") и создаёт сам этот файл (пустым) - CompileC не проверяет
// содержимое бинарника, только успешность вызова. POSIX-only (shell-
// скрипт) - на Windows тест пропускается.
func fakeCC(t *testing.T) string {
	t.Helper()
	if runtime.GOOS == "windows" {
		t.Skip("fakeCC - POSIX shell-скрипт, на Windows нет /bin/sh")
	}
	dir := t.TempDir()
	script := filepath.Join(dir, "cc")
	body := `#!/bin/sh
echo "$@" > "$0.args"
out=""
prev=""
for a in "$@"; do
  if [ "$prev" = "-o" ]; then
    out="$a"
  fi
  prev="$a"
done
[ -n "$out" ] && touch "$out"
`
	if err := os.WriteFile(script, []byte(body), 0o755); err != nil {
		t.Fatalf("запись фейкового cc: %v", err)
	}
	return script
}

func TestCompileCLinksGCOnlyWhenSourceUsesIt(t *testing.T) {
	script := fakeCC(t)
	t.Setenv("CC", script)

	dir := t.TempDir()
	withGC := filepath.Join(dir, "with_gc.c")
	if err := os.WriteFile(withGC, []byte("#include <gc.h>\nint main(void){return 0;}\n"), 0o644); err != nil {
		t.Fatalf("запись исходника: %v", err)
	}
	withoutGC := filepath.Join(dir, "without_gc.c")
	if err := os.WriteFile(withoutGC, []byte("int main(void){return 0;}\n"), 0o644); err != nil {
		t.Fatalf("запись исходника: %v", err)
	}

	cc := &CC{}
	binWith := filepath.Join(dir, "bin_with")
	if err := cc.CompileC(withGC, binWith, nil); err != nil {
		t.Fatalf("CompileC: %v", err)
	}
	argsWith, err := os.ReadFile(script + ".args")
	if err != nil {
		t.Fatalf("чтение записанных аргументов: %v", err)
	}
	if !strings.Contains(string(argsWith), "-lgc") {
		t.Fatalf("ожидался флаг -lgc для файла с #include <gc.h>, аргументы: %q", argsWith)
	}

	binWithout := filepath.Join(dir, "bin_without")
	if err := cc.CompileC(withoutGC, binWithout, nil); err != nil {
		t.Fatalf("CompileC: %v", err)
	}
	argsWithout, err := os.ReadFile(script + ".args")
	if err != nil {
		t.Fatalf("чтение записанных аргументов: %v", err)
	}
	if strings.Contains(string(argsWithout), "-lgc") {
		t.Fatalf("не ожидался флаг -lgc для файла без #include <gc.h> (сломает сборку без установленного libgc), аргументы: %q", argsWithout)
	}
	if !strings.Contains(string(argsWithout), "-lm") {
		t.Fatalf("-lm должен передаваться всегда, аргументы: %q", argsWithout)
	}
}

func TestCompileCLinksDBLibrariesOnlyWhenSourceUsesThem(t *testing.T) {
	script := fakeCC(t)
	t.Setenv("CC", script)

	dir := t.TempDir()
	withDB := filepath.Join(dir, "with_db.c")
	if err := os.WriteFile(withDB, []byte("static void *rech_db_connect(const char *c) { return 0; }\nint main(void){return 0;}\n"), 0o644); err != nil {
		t.Fatalf("запись исходника: %v", err)
	}
	withoutDB := filepath.Join(dir, "without_db.c")
	if err := os.WriteFile(withoutDB, []byte("int main(void){return 0;}\n"), 0o644); err != nil {
		t.Fatalf("запись исходника: %v", err)
	}

	cc := &CC{}
	binWith := filepath.Join(dir, "bin_with")
	if err := cc.CompileC(withDB, binWith, nil); err != nil {
		t.Fatalf("CompileC: %v", err)
	}
	argsWith, err := os.ReadFile(script + ".args")
	if err != nil {
		t.Fatalf("чтение записанных аргументов: %v", err)
	}
	for _, lib := range []string{"-lsqlite3", "-lpq", "-lmysqlclient"} {
		if !strings.Contains(string(argsWith), lib) {
			t.Fatalf("ожидался флаг %s для файла с rech_db_connect(, аргументы: %q", lib, argsWith)
		}
	}

	binWithout := filepath.Join(dir, "bin_without")
	if err := cc.CompileC(withoutDB, binWithout, nil); err != nil {
		t.Fatalf("CompileC: %v", err)
	}
	argsWithout, err := os.ReadFile(script + ".args")
	if err != nil {
		t.Fatalf("чтение записанных аргументов: %v", err)
	}
	for _, lib := range []string{"-lsqlite3", "-lpq", "-lmysqlclient"} {
		if strings.Contains(string(argsWithout), lib) {
			t.Fatalf("не ожидался флаг %s без rech_db_connect( (сломает сборку без установленных клиентских библиотек БД), аргументы: %q", lib, argsWithout)
		}
	}
}
