package project

import (
	"os"
	"path/filepath"
	"testing"
)

func TestLoadDefaultsAndPaths(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "тест.rechproj")
	content := `проект:
  имя: тест

файлы:
  главный: исходники/главный.rech
`
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatalf("запись файла проекта: %v", err)
	}
	pf, err := Load(path)
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if pf.Выход != "сборка" {
		t.Fatalf("ожидался каталог выхода по умолчанию \"сборка\", получено %q", pf.Выход)
	}
	if pf.Режим != "релиз" || pf.Платформа != "текущая" {
		t.Fatalf("неожиданные значения по умолчанию: %+v", pf)
	}
	if got, want := pf.MainPath(), filepath.Join(dir, "исходники", "главный.rech"); got != want {
		t.Fatalf("MainPath() = %q, ожидалось %q", got, want)
	}
	if got, want := pf.OutputDir(), filepath.Join(dir, "сборка"); got != want {
		t.Fatalf("OutputDir() = %q, ожидалось %q", got, want)
	}
	if pf.KeepC() {
		t.Fatalf("режим релиз не должен сохранять .c")
	}
	if pf.Тип != "программа" || pf.IsLibrary() {
		t.Fatalf("ожидался тип по умолчанию \"программа\", получено %+v", pf)
	}
}

func TestLoadLibraryType(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "тест.rechproj")
	content := `проект:
  имя: тест

файлы:
  главный: исходники/главный.rech

сборка:
  тип: библиотека
`
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatalf("запись файла проекта: %v", err)
	}
	pf, err := Load(path)
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if !pf.IsLibrary() {
		t.Fatal("ожидалось IsLibrary() == true при сборка.тип: библиотека")
	}
}

func TestLoadLibraries(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "тест.rechproj")
	content := `проект:
  имя: тест

файлы:
  главный: исходники/главный.rech

сборка:
  библиотеки: curl, ssl
`
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatalf("запись файла проекта: %v", err)
	}
	pf, err := Load(path)
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	got := pf.Библиотеки()
	if len(got) != 2 || got[0] != "curl" || got[1] != "ssl" {
		t.Fatalf("Библиотеки() = %v, ожидалось [curl ssl]", got)
	}
}

func TestLoadWithoutLibrariesReturnsNil(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "тест.rechproj")
	content := "проект:\n  имя: тест\nфайлы:\n  главный: а.rech\n"
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatalf("запись файла проекта: %v", err)
	}
	pf, err := Load(path)
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if got := pf.Библиотеки(); got != nil {
		t.Fatalf("Библиотеки() = %v, ожидался nil", got)
	}
}

func TestLoadUnknownTypeRejected(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "тест.rechproj")
	content := `проект:
  имя: тест

файлы:
  главный: исходники/главный.rech

сборка:
  тип: непонятно
`
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatalf("запись файла проекта: %v", err)
	}
	if _, err := Load(path); err == nil {
		t.Fatal("ожидалась ошибка при неизвестном сборка.тип")
	}
}

func TestFindSingleAndAmbiguous(t *testing.T) {
	dir := t.TempDir()
	if p, err := Find(dir); err != nil || p != "" {
		t.Fatalf("Find на пустом каталоге: %q, %v", p, err)
	}

	path := filepath.Join(dir, "один.rechproj")
	if err := os.WriteFile(path, []byte("проект:\n  имя: один\nфайлы:\n  главный: а.rech\n"), 0o644); err != nil {
		t.Fatalf("запись: %v", err)
	}
	got, err := Find(dir)
	if err != nil {
		t.Fatalf("Find: %v", err)
	}
	if got != path {
		t.Fatalf("Find() = %q, ожидалось %q", got, path)
	}

	if err := os.WriteFile(filepath.Join(dir, "два.rechproj"), []byte("проект:\n  имя: два\nфайлы:\n  главный: б.rech\n"), 0o644); err != nil {
		t.Fatalf("запись: %v", err)
	}
	if _, err := Find(dir); err == nil {
		t.Fatalf("ожидалась ошибка при нескольких файлах проекта")
	}
}
