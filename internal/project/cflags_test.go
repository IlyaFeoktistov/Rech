package project

import (
	"os"
	"path/filepath"
	"slices"
	"testing"
)

func loadProj(t *testing.T, build string) *File {
	t.Helper()
	path := filepath.Join(t.TempDir(), "тест.rechproj")
	content := "проект:\n  имя: тест\n\nфайлы:\n  главный: главный.rech\n\nсборка:\n" + build
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatalf("запись файла проекта: %v", err)
	}
	pf, err := Load(path)
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	return pf
}

func TestCFlagsAbsentMeansBuiltin(t *testing.T) {
	pf := loadProj(t, "  режим: отладка\n")
	if _, set, _ := pf.ФлагиC(); set {
		t.Fatalf("без поля флаги_c встроенные флаги не заменяются")
	}
	if _, set, _ := pf.ФлагиЛинковки(); set {
		t.Fatalf("без поля флаги_линковки встроенные флаги не заменяются")
	}
	if pf.КомпиляторC() != "" {
		t.Fatalf("без поля компилятор_c - \"\"")
	}
}

func TestCFlagsEmptyValueStillReplaces(t *testing.T) {
	pf := loadProj(t, "  флаги_c:\n")
	flags, set, err := pf.ФлагиC()
	if err != nil || !set || len(flags) != 0 {
		t.Fatalf("пустое флаги_c - замена на пустой список, получено %q, %v, %v", flags, set, err)
	}
}

func TestLinkFlagsPlatformVariantAppendsAndQuotes(t *testing.T) {
	suffix := platformSuffix()
	pf := loadProj(t, "  флаги_линковки: -lm \"-L/путь с пробелом\"\n  флаги_линковки"+suffix+": -lдоп\n  компилятор_c: gcc\n  компилятор_c"+suffix+": clang\n")
	flags, set, err := pf.ФлагиЛинковки()
	if err != nil || !set {
		t.Fatalf("ФлагиЛинковки: %v, %v", set, err)
	}
	if want := []string{"-lm", "-L/путь с пробелом", "-lдоп"}; !slices.Equal(flags, want) {
		t.Fatalf("ожидалось %q, получено %q", want, flags)
	}
	if pf.КомпиляторC() != "clang" {
		t.Fatalf("компилятор_c для платформы заменяет общий, получено %q", pf.КомпиляторC())
	}
}

func TestOnlyPlatformVariantAlsoReplaces(t *testing.T) {
	pf := loadProj(t, "  флаги_c"+platformSuffix()+": -O3\n")
	flags, set, _ := pf.ФлагиC()
	if !set || !slices.Equal(flags, []string{"-O3"}) {
		t.Fatalf("вариант для платформы без общего поля - тоже замена, получено %q, %v", flags, set)
	}
}

func TestCFlagsUnclosedQuoteFails(t *testing.T) {
	pf := loadProj(t, "  флаги_c: \"-DX\n")
	if _, _, err := pf.ФлагиC(); err == nil {
		t.Fatalf("ожидалась ошибка незакрытой кавычки")
	}
}
