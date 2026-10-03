package toolchain

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/IlyaFeoktistov/Rech/internal/compiler"
)

func TestCheckCCVersion(t *testing.T) {
	if err := checkCCVersion("cc", ccInfo{family: "gcc", major: 8}); err == nil || !strings.Contains(err.Error(), "gcc 9 или новее") {
		t.Fatalf("gcc 8 должен быть отвергнут, получено %v", err)
	}
	for _, ok := range []ccInfo{{"gcc", 9}, {"gcc", 16}, {"clang", 11}, {"", 3}, {"gcc", 0}} {
		if err := checkCCVersion("cc", ok); err != nil {
			t.Errorf("%+v: неожиданная ошибка %v", ok, err)
		}
	}
}

func TestCFlagsStrictOnlyForGCCClang(t *testing.T) {
	if got := cFlags(compiler.CBuild{}, ccInfo{family: "gcc", major: 13}); !strings.Contains(strings.Join(got, " "), "-std=gnu11") {
		t.Fatalf("gcc: ожидался -std=gnu11, получено %v", got)
	}
	if got := cFlags(compiler.CBuild{}, ccInfo{}); len(got) != 0 {
		t.Fatalf("неизвестный компилятор: флаги не добавляются, получено %v", got)
	}
	if got := cFlags(compiler.CBuild{CFlagsSet: true, CFlags: []string{"-O2"}}, ccInfo{family: "gcc"}); strings.Join(got, " ") != "-O2" {
		t.Fatalf("флаги проекта заменяют встроенные, получено %v", got)
	}
}

// TestStrictFlagsRejectFutureErrors - на установленном компиляторе то, что
// gcc 14+ считает ошибкой, - ошибка и со встроенными флагами.
func TestStrictFlagsRejectFutureErrors(t *testing.T) {
	path, err := findCCompiler("")
	if err != nil {
		t.Skip("нет компилятора C")
	}
	info := identifyCC(path)
	if info.family == "" {
		t.Skip("компилятор не gcc/clang")
	}
	src := filepath.Join(t.TempDir(), "a.c")
	if err := os.WriteFile(src, []byte("void f(char **p);\nint main(void) { const char *s; f(&s); return 0; }\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	args := append(cFlags(compiler.CBuild{}, info), "-c", src, "-o", src+".o")
	if out, err := exec.Command(path, args...).CombinedOutput(); err == nil {
		t.Fatalf("%s %d: несовместимые указатели должны быть ошибкой, вывод:\n%s", info.family, info.major, out)
	}
}
