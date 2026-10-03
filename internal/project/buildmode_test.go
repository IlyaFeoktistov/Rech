package project

import (
	"os"
	"path/filepath"
	"testing"
)

func loadBuildSection(t *testing.T, build string) (*File, error) {
	t.Helper()
	path := filepath.Join(t.TempDir(), "тест.rechproj")
	content := "проект:\n  имя: тест\n\nфайлы:\n  главный: исходники/главный.rech\n\nсборка:\n" + build
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatalf("запись файла проекта: %v", err)
	}
	return Load(path)
}

// "сборка.режим" - отладка/релиз, "сборка.сохранять_си" - отдельно от
// режима.
func TestBuildModeAndKeepC(t *testing.T) {
	cases := []struct {
		build          string
		release, keepC bool
	}{
		{"  режим: релиз\n", true, false},
		{"  режим: отладка\n", false, false},
		{"  режим: релиз\n  сохранять_си: истина\n", true, true},
		{"  сохранять_си: ложь\n", false, false},
	}
	for _, c := range cases {
		pf, err := loadBuildSection(t, c.build)
		if err != nil {
			t.Fatalf("%q: %v", c.build, err)
		}
		if pf.IsRelease() != c.release || pf.KeepC() != c.keepC {
			t.Errorf("%q: IsRelease=%v KeepC=%v", c.build, pf.IsRelease(), pf.KeepC())
		}
	}
	if _, err := loadBuildSection(t, "  сохранять_си: да\n"); err == nil {
		t.Error("сохранять_си: да - ожидалась ошибка")
	}
}
