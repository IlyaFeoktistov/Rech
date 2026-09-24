package toolchain

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/IlyaFeoktistov/Rech/internal/compiler"
	"github.com/IlyaFeoktistov/Rech/internal/project"
)

// Флаги из .rechproj заменяют встроенные целиком: остаётся только
// "<файл.c> -o <выход>" - ни -lm, ни библиотек проекта, ни -shared.
func TestProjectFlagsReplaceBuiltinFlags(t *testing.T) {
	script := fakeCC(t)
	t.Setenv("CC", "нет-такого-компилятора")

	dir := t.TempDir()
	src := filepath.Join(dir, "прог.c")
	if err := os.WriteFile(src, []byte("#include <gc.h>\nint main(void){return 0;}\n"), 0o644); err != nil {
		t.Fatalf("запись исходника: %v", err)
	}
	b := compiler.CBuild{
		CCompiler: script,
		CFlags:    []string{"-O2", "-DРЕЖИМ=1"}, CFlagsSet: true,
		LinkFlags: []string{"-lgc", "-lfoo"}, LinkFlagsSet: true,
		Libs: []string{"curl"},
	}
	cc := &CC{}
	builds := []struct {
		name string
		out  string
		run  func(out string) error
	}{
		{"программа", filepath.Join(dir, "бин"), func(out string) error { return cc.CompileC(src, out, b) }},
		{"библиотека", filepath.Join(dir, "либа.so"), func(out string) error { return cc.CompileLib(src, out, b) }},
	}
	for _, bl := range builds {
		if err := bl.run(bl.out); err != nil {
			t.Fatalf("%s: %v", bl.name, err)
		}
		args, err := os.ReadFile(script + ".args")
		if err != nil {
			t.Fatalf("чтение записанных аргументов: %v", err)
		}
		want := strings.Join([]string{"-O2", "-DРЕЖИМ=1", src, "-o", bl.out, "-lgc", "-lfoo"}, " ")
		if got := strings.TrimSpace(string(args)); got != want {
			t.Fatalf("%s: ожидалось %q (без встроенных флагов), получено %q", bl.name, want, got)
		}
	}
}

// Шаблон .rechproj (rech init) перечисляет все встроенные флаги - при
// новом флаге в AutoLinkLibs/LibModeFlags шаблон обязан его упомянуть.
func TestTemplateListsAllBuiltinFlags(t *testing.T) {
	for _, l := range AutoLinkLibs {
		line := "#   " + strings.Join(l.Flags, " ")
		if !strings.Contains(project.Template, line) {
			t.Errorf("шаблон .rechproj не упоминает %q", line)
		}
	}
	for _, goos := range []string{"linux", "darwin", "windows"} {
		if flags := strings.Join(LibModeFlags(goos), " "); !strings.Contains(project.Template, flags) {
			t.Errorf("шаблон .rechproj не упоминает флаги библиотеки %q (%s)", flags, goos)
		}
	}
}
