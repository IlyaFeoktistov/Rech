package toolchain

import (
	"os"
	"path/filepath"
	"runtime"
	"slices"
	"strings"
	"testing"

	"github.com/IlyaFeoktistov/Rech/internal/compiler"
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
	if err := cc.CompileC(withGC, binWith, compiler.CBuild{}); err != nil {
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
	if err := cc.CompileC(withoutGC, binWithout, compiler.CBuild{}); err != nil {
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

// TestCompileCPassesLibraryPathFlags - "сборка.путь_библиотек" (internal/
// project.File.ПутиБиблиотек) должен дойти до cc как "-L<путь>" (сама
// линковка ищет "-l<имя>" не только в системных путях) и, на не-Windows,
// дополнительно "-Wl,-rpath,<путь>" (чтобы собранный бинарник САМ нашёл
// разделяемую библиотеку в момент запуска, без ручного LD_LIBRARY_PATH).
func TestCompileCPassesLibraryPathFlags(t *testing.T) {
	script := fakeCC(t)
	t.Setenv("CC", script)

	dir := t.TempDir()
	src := filepath.Join(dir, "прог.c")
	if err := os.WriteFile(src, []byte("int main(void){return 0;}\n"), 0o644); err != nil {
		t.Fatalf("запись исходника: %v", err)
	}

	cc := &CC{}
	bin := filepath.Join(dir, "бин")
	if err := cc.CompileC(src, bin, compiler.CBuild{Libs: []string{"myrechlib"}, LibPaths: []string{"/некий/путь"}}); err != nil {
		t.Fatalf("CompileC: %v", err)
	}
	args, err := os.ReadFile(script + ".args")
	if err != nil {
		t.Fatalf("чтение записанных аргументов: %v", err)
	}
	got := string(args)
	if !strings.Contains(got, "-L/некий/путь") {
		t.Fatalf("ожидался флаг -L/некий/путь, аргументы: %q", got)
	}
	if runtime.GOOS != "windows" && !strings.Contains(got, "-Wl,-rpath,/некий/путь") {
		t.Fatalf("ожидался флаг -Wl,-rpath,/некий/путь, аргументы: %q", got)
	}
	if !strings.Contains(got, "-lmyrechlib") {
		t.Fatalf("ожидался флаг -lmyrechlib, аргументы: %q", got)
	}
}

// TestCompileCLinksSSLOnlyWhenSourceUsesTLS - -lssl/-lcrypto (TLS для
// "слушать_tcp_защищённо", internal/backend/cbackend/tls.go) - тот же
// принцип, что и у -lgc выше: линкуются только по маркеру "#include
// <openssl/ssl.h>" в сгенерированном файле, не безусловно (сломало бы
// сборку любой программы без libssl-dev, даже без единого TLS-вызова).
func TestCompileCLinksSSLOnlyWhenSourceUsesTLS(t *testing.T) {
	script := fakeCC(t)
	t.Setenv("CC", script)

	dir := t.TempDir()
	withTLS := filepath.Join(dir, "with_tls.c")
	if err := os.WriteFile(withTLS, []byte("#include <openssl/ssl.h>\nint main(void){return 0;}\n"), 0o644); err != nil {
		t.Fatalf("запись исходника: %v", err)
	}
	withoutTLS := filepath.Join(dir, "without_tls.c")
	if err := os.WriteFile(withoutTLS, []byte("int main(void){return 0;}\n"), 0o644); err != nil {
		t.Fatalf("запись исходника: %v", err)
	}

	cc := &CC{}
	binWith := filepath.Join(dir, "bin_with")
	if err := cc.CompileC(withTLS, binWith, compiler.CBuild{}); err != nil {
		t.Fatalf("CompileC: %v", err)
	}
	argsWith, err := os.ReadFile(script + ".args")
	if err != nil {
		t.Fatalf("чтение записанных аргументов: %v", err)
	}
	if !strings.Contains(string(argsWith), "-lssl") || !strings.Contains(string(argsWith), "-lcrypto") {
		t.Fatalf("ожидались флаги -lssl -lcrypto для файла с #include <openssl/ssl.h>, аргументы: %q", argsWith)
	}

	binWithout := filepath.Join(dir, "bin_without")
	if err := cc.CompileC(withoutTLS, binWithout, compiler.CBuild{}); err != nil {
		t.Fatalf("CompileC: %v", err)
	}
	argsWithout, err := os.ReadFile(script + ".args")
	if err != nil {
		t.Fatalf("чтение записанных аргументов: %v", err)
	}
	if strings.Contains(string(argsWithout), "-lssl") {
		t.Fatalf("не ожидался флаг -lssl для файла без TLS (сломает сборку без libssl-dev), аргументы: %q", argsWithout)
	}
}

// TestCompileCLinksDBLibrariesOnlyWhenSourceUsesThem - internal/dbscheme.Set
// ("toolchain линкует -lsqlite3 -lpq -lmysqlclient
// безусловно") - каждая из трёх библиотек линкуется НЕЗАВИСИМО от двух
// других, по маркеру функции подключения именно ЭТОЙ библиотеки
// ("sqlite3_open("/"PQconnectdb("/"mysql_init(" - см. commonLinkFlags), не
// по общему "rech_db_connect(" (та эмитируется для любой программы с БД,
// независимо от того, какая конкретно СУБД внутри неё скомпилирована,
// internal/backend/cbackend/db.go, dbRuntimeCode).
// TestMissingLibHint - missingLibHint (cc.go) разбирает вывод линковщика
// трёх разных тулчейнов (GNU ld на Linux, ld64 на macOS, link.exe/MSVC на
// Windows - см. package doc) на предмет "не хватает клиентской библиотеки
// БД" и даёт подсказку по установке для соответствующей платформы -
// проверяется тем же runtime.GOOS, на котором реально гоняется тест (тот
// же принцип, что и installHintFor).
func TestMissingLibHint(t *testing.T) {
	cases := []struct {
		name   string
		output string
		want   string
	}{
		{"gnu-ld-sqlite", "/usr/bin/ld: cannot find -lsqlite3: No such file or directory", "sqlite3"},
		{"macos-ld64-pq", "ld: library not found for -lpq", "pq"},
		{"msvc-mysql", "LINK : fatal error LNK1181: cannot open input file 'mysqlclient.lib'", "mysqlclient"},
		{"gnu-ld-nghttp2", "/usr/bin/ld: cannot find -lnghttp2: No such file or directory", "nghttp2"},
		{"unrelated-error", "undefined reference to `main'", ""},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			hint := missingLibHint(tc.output)
			if tc.want == "" {
				if hint != "" {
					t.Fatalf("не ожидалась подсказка для %q, получено: %q", tc.output, hint)
				}
				return
			}
			if !strings.Contains(hint, tc.want) {
				t.Fatalf("ожидалось упоминание %q в подсказке, получено: %q", tc.want, hint)
			}
			if !strings.Contains(hint, installHintFor(tc.want)) {
				t.Fatalf("ожидалась команда установки %q в подсказке, получено: %q", installHintFor(tc.want), hint)
			}
		})
	}
}

func TestCompileCLinksDBLibrariesOnlyWhenSourceUsesThem(t *testing.T) {
	script := fakeCC(t)
	t.Setenv("CC", script)

	dir := t.TempDir()
	cc := &CC{}
	compileAndReadArgs := func(name, src string) string {
		path := filepath.Join(dir, name+".c")
		if err := os.WriteFile(path, []byte(src), 0o644); err != nil {
			t.Fatalf("запись исходника: %v", err)
		}
		if err := cc.CompileC(path, filepath.Join(dir, "bin_"+name), compiler.CBuild{}); err != nil {
			t.Fatalf("CompileC: %v", err)
		}
		args, err := os.ReadFile(script + ".args")
		if err != nil {
			t.Fatalf("чтение записанных аргументов: %v", err)
		}
		return string(args)
	}

	cases := []struct {
		name string
		src  string
		want []string
		dont []string
	}{
		{
			name: "sqlite_only",
			src:  "static void *rech_db_connect(const char *c) { sqlite3_open(c, 0); return 0; }\nint main(void){return 0;}\n",
			want: []string{"-lsqlite3"},
			dont: []string{"-lpq", "-lmysqlclient"},
		},
		{
			name: "all_three",
			src:  "static void *rech_db_connect(const char *c) { sqlite3_open(c, 0); PQconnectdb(c); mysql_init(0); return 0; }\nint main(void){return 0;}\n",
			want: []string{"-lsqlite3", "-lpq", "-lmysqlclient"},
		},
		{
			name: "without_db",
			src:  "int main(void){return 0;}\n",
			dont: []string{"-lsqlite3", "-lpq", "-lmysqlclient"},
		},
	}
	for _, tc := range cases {
		args := compileAndReadArgs(tc.name, tc.src)
		for _, lib := range tc.want {
			if !strings.Contains(args, lib) {
				t.Fatalf("%s: ожидался флаг %s, аргументы: %q", tc.name, lib, args)
			}
		}
		for _, lib := range tc.dont {
			if strings.Contains(args, lib) {
				t.Fatalf("%s: не ожидался флаг %s, аргументы: %q", tc.name, lib, args)
			}
		}
	}
}

// TestCCArgsWindowedAddsGUIFlagOnlyOnWindows - "сборка.тип: оконное" даёт
// -mwindows только для Windows и даже при флагах линковки проекта.
func TestCCArgsWindowedAddsGUIFlagOnlyOnWindows(t *testing.T) {
	b := compiler.CBuild{Windowed: true, LinkFlags: []string{"-lgc"}, LinkFlagsSet: true}
	if got := ccArgs(b, "a.c", "a.exe", nil, "windows"); !slices.Contains(got, windowsGUIFlag) {
		t.Fatalf("на Windows ожидался %s, получено %q", windowsGUIFlag, got)
	}
	if got := ccArgs(b, "a.c", "a", nil, "linux"); slices.Contains(got, windowsGUIFlag) {
		t.Fatalf("на Linux %s не нужен, получено %q", windowsGUIFlag, got)
	}
	b.Windowed = false
	if got := ccArgs(b, "a.c", "a.exe", nil, "windows"); slices.Contains(got, windowsGUIFlag) {
		t.Fatalf("консольное приложение не должно получать %s: %q", windowsGUIFlag, got)
	}
}
