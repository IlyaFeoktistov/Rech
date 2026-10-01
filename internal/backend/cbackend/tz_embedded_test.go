package cbackend

import (
	"fmt"
	"io/fs"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

// zoneinfoSourceDir - исходные файлы вшитой базы поясов (относительно
// каталога этого пакета).
const zoneinfoSourceDir = "runtime/tzdata/zoneinfo"

// TestEmbeddedTZMatchesSourceFiles - собранная программа без TZDIR читает
// пояса из вшитой базы и даёт ровно тот же результат, что при TZDIR на
// исходные файлы, - для каждого пояса, в прямую и обратную сторону, до и
// после последнего явного перехода (POSIX-хвост) и после 2038 года.
func TestEmbeddedTZMatchesSourceFiles(t *testing.T) {
	var zones []string
	err := filepath.WalkDir(zoneinfoSourceDir, func(path string, d fs.DirEntry, err error) error {
		if err != nil || d.IsDir() {
			return err
		}
		rel, err := filepath.Rel(zoneinfoSourceDir, path)
		zones = append(zones, filepath.ToSlash(rel))
		return err
	})
	if err != nil {
		t.Fatal(err)
	}
	stamps := []int64{-2000000000, 0, 1000000000, 1721044800, 2200000000, 4102444800}

	var src strings.Builder
	src.WriteString("функция главная\n{\n    переменная запасная = новая ДатаВремя(1, 1, 1, 0, 0, 0)\n")
	for _, z := range zones {
		for _, ts := range stamps {
			fmt.Fprintf(&src, "    вывести `%s %d ${дату_в_строку(время_как_дата_в_поясе(%d, \"%s\") ?? запасная)}\\n`\n", z, ts, ts, z)
		}
		fmt.Fprintf(&src, "    вывести `%s обратно ${дата_в_поясе_как_время(новая ДатаВремя(2030, 3, 31, 2, 30, 0), \"%s\") ?? -1}\\n`\n", z, z)
	}
	src.WriteString("    вывести `${время_как_дата_в_поясе(0, \"Не/Существует\") == Ничто}\\n`\n}\n")

	out := generateFromSrc(t, src.String())
	ccPath := findTestCCompiler(t)
	dir := t.TempDir()
	cPath := filepath.Join(dir, "tz.c")
	if err := os.WriteFile(cPath, []byte(out), 0o644); err != nil {
		t.Fatal(err)
	}
	binPath := filepath.Join(dir, "tz")
	// #nosec G204 -- ccPath найден локальным поиском (CC/cc/clang/gcc), cPath - файл, только что написанный этим же тестом.
	if msg, err := exec.Command(ccPath, cPath, "-o", binPath, "-lgc", "-lm").CombinedOutput(); err != nil {
		t.Fatalf("сборка: %v\n%s", err, msg)
	}
	run := func(tzdir string) string {
		t.Helper()
		// #nosec G204 -- binPath - бинарник, только что собранный этим же тестом.
		cmd := exec.Command(binPath)
		cmd.Env = append(os.Environ(), "TZDIR="+tzdir)
		got, err := cmd.Output()
		if err != nil {
			t.Fatalf("запуск (TZDIR=%q): %v", tzdir, err)
		}
		return strings.ReplaceAll(string(got), "\r\n", "\n")
	}
	embedded := run("")
	abs, err := filepath.Abs(zoneinfoSourceDir)
	if err != nil {
		t.Fatal(err)
	}
	files := run(abs)

	if strings.Contains(embedded, "0001-01-01") || strings.Contains(embedded, "обратно -1\n") {
		t.Fatalf("часть поясов не нашлась во вшитой базе:\n%s", embedded)
	}
	if !strings.HasSuffix(embedded, "true\n") && !strings.HasSuffix(embedded, "истина\n") {
		t.Fatalf("неизвестный пояс должен давать Ничто, хвост вывода: %q", embedded[max(0, len(embedded)-40):])
	}
	if embedded != files {
		wantLines, gotLines := strings.Split(files, "\n"), strings.Split(embedded, "\n")
		for i := range min(len(wantLines), len(gotLines)) {
			if wantLines[i] != gotLines[i] {
				t.Fatalf("вшитая база расходится с файлами: %q, ожидалось %q", gotLines[i], wantLines[i])
			}
		}
		t.Fatalf("вшитая база расходится с файлами по числу строк: %d против %d", len(gotLines), len(wantLines))
	}
}
