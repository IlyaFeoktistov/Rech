package toolchain

import (
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	"github.com/IlyaFeoktistov/Rech/internal/compiler"
)

func TestLibExtensionMatchesCurrentPlatform(t *testing.T) {
	want := map[string]string{"windows": ".dll", "darwin": ".dylib"}[runtime.GOOS]
	if want == "" {
		want = ".so"
	}
	if got := (&CC{}).LibExtension(); got != want {
		t.Fatalf("LibExtension() = %q, ожидалось %q на %s", got, want, runtime.GOOS)
	}
}

func TestCompileLibPassesSharedFlag(t *testing.T) {
	script := fakeCC(t)
	t.Setenv("CC", script)

	dir := t.TempDir()
	src := filepath.Join(dir, "либа.c")
	if err := os.WriteFile(src, []byte("int rech_f(void){return 0;}\n"), 0o644); err != nil {
		t.Fatalf("запись исходника: %v", err)
	}

	cc := &CC{}
	libPath := filepath.Join(dir, "либа"+cc.LibExtension())
	if err := cc.CompileLib(src, libPath, compiler.CBuild{}); err != nil {
		t.Fatalf("CompileLib: %v", err)
	}
	args, err := os.ReadFile(script + ".args")
	if err != nil {
		t.Fatalf("чтение записанных аргументов: %v", err)
	}
	got := string(args)
	// fakeCC - POSIX shell-скрипт (см. package doc), runtime.GOOS в этом
	// тестовом процессе - всегда не-Windows, значит CompileLib здесь
	// проходит по ветке "linux и остальные POSIX" (darwin тоже возможен на
	// CI под macOS - оба варианта проверяются ниже).
	if !strings.Contains(got, "-shared") && !strings.Contains(got, "-dynamiclib") {
		t.Fatalf("ожидался -shared (Linux) или -dynamiclib (macOS), аргументы: %q", got)
	}
	if !strings.Contains(got, "-fvisibility=hidden") {
		t.Fatalf("ожидался -fvisibility=hidden на POSIX, аргументы: %q", got)
	}
	if !strings.Contains(got, "-lm") {
		t.Fatalf("-lm должен передаваться всегда, аргументы: %q", got)
	}
}

func TestCompileCAndCompileLibPassExtraLibs(t *testing.T) {
	script := fakeCC(t)
	t.Setenv("CC", script)

	dir := t.TempDir()
	src := filepath.Join(dir, "прог.c")
	if err := os.WriteFile(src, []byte("int main(void){return 0;}\n"), 0o644); err != nil {
		t.Fatalf("запись исходника: %v", err)
	}
	cc := &CC{}

	if err := cc.CompileC(src, filepath.Join(dir, "бин"), compiler.CBuild{Libs: []string{"curl", "ssl"}}); err != nil {
		t.Fatalf("CompileC: %v", err)
	}
	args, err := os.ReadFile(script + ".args")
	if err != nil {
		t.Fatalf("чтение записанных аргументов: %v", err)
	}
	if !strings.Contains(string(args), "-lcurl") || !strings.Contains(string(args), "-lssl") {
		t.Fatalf("ожидались -lcurl -lssl, аргументы: %q", args)
	}

	libPath := filepath.Join(dir, "либа"+cc.LibExtension())
	if err := cc.CompileLib(src, libPath, compiler.CBuild{Libs: []string{"curl"}}); err != nil {
		t.Fatalf("CompileLib: %v", err)
	}
	args, err = os.ReadFile(script + ".args")
	if err != nil {
		t.Fatalf("чтение записанных аргументов: %v", err)
	}
	if !strings.Contains(string(args), "-lcurl") {
		t.Fatalf("ожидался -lcurl, аргументы: %q", args)
	}
}
