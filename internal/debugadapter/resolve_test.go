package debugadapter

import (
	"os"
	"path/filepath"
	"testing"
)

func TestResolveProgramPassesThroughRechFile(t *testing.T) {
	got, err := resolveProgram("программа.rech")
	if err != nil {
		t.Fatalf("resolveProgram: %v", err)
	}
	if got != "программа.rech" {
		t.Fatalf("resolveProgram(%q) = %q, ожидалось без изменений", "программа.rech", got)
	}
}

func TestResolveProgramResolvesRechproj(t *testing.T) {
	dir := t.TempDir()
	projPath := filepath.Join(dir, "проект.rechproj")
	content := "проект:\n  имя: проект\nфайлы:\n  главный: исходники/главный.rech\n"
	if err := os.WriteFile(projPath, []byte(content), 0o644); err != nil {
		t.Fatalf("запись файла проекта: %v", err)
	}
	got, err := resolveProgram(projPath)
	if err != nil {
		t.Fatalf("resolveProgram: %v", err)
	}
	want := filepath.Join(dir, "исходники", "главный.rech")
	if got != want {
		t.Fatalf("resolveProgram(%q) = %q, ожидалось %q", projPath, got, want)
	}
}
