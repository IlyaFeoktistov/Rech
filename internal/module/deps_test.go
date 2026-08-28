package module

import (
	"path/filepath"
	"testing"
)

// TestLoadWithDepsResolvesFirstSegmentAsDependency - "использовать X из
// "клиент/файл"" резолвится ВНУТРИ каталога зависимости "клиент" (не
// относительно каталога импортирующего файла), если "клиент" объявлена в
// deps - см. package doc LoadWithDeps.
func TestLoadWithDepsResolvesFirstSegmentAsDependency(t *testing.T) {
	depDir := t.TempDir()
	writeFile(t, depDir, "файл.rech", "функция взять(): Целое\n{\n    вернуть 7\n}\n")

	projDir := t.TempDir()
	entry := writeFile(t, projDir, "главный.rech", "использовать взять из \"клиент/файл\"\n\nфункция главная\n{\n    вывести взять()\n}\n")

	deps := map[string]string{"клиент": depDir}
	prog, err := LoadWithDeps(entry, testFrontend{}, deps)
	if err != nil {
		t.Fatalf("LoadWithDeps: %v", err)
	}
	if len(prog.Modules) != 2 {
		t.Fatalf("ожидалось 2 модуля, получено %d", len(prog.Modules))
	}
}

// TestLoadWithoutDepsDoesNotMatchDependencyName - без deps (обычный Load)
// первый сегмент - обычный локальный каталог, не имя зависимости - тот же
// путь молча не найдётся как локальный файл, если такого каталога нет
// рядом с импортирующим файлом.
func TestLoadWithoutDepsDoesNotMatchDependencyName(t *testing.T) {
	dir := t.TempDir()
	entry := writeFile(t, dir, "главный.rech", "использовать взять из \"клиент/файл\"\n\nфункция главная\n{\n    вывести взять()\n}\n")
	if _, err := Load(entry, testFrontend{}); err == nil {
		t.Fatalf("ожидалась ошибка - \"клиент/файл\" не существует локально и deps не заданы")
	}
}

// TestResolveImportPathPrefersDependencyOverLocalDir - если "клиент" ОБЕ
// является объявленной зависимостью И существует как локальный подкаталог
// рядом с импортирующим файлом, побеждает зависимость (deps проверяются
// раньше baseDir) - однозначное правило, не угадывание по факту
// существования файла на диске.
func TestResolveImportPathPrefersDependencyOverLocalDir(t *testing.T) {
	depDir := t.TempDir()
	got, err := resolveImportPath("/базовый/каталог", "клиент/файл", map[string]string{"клиент": depDir})
	if err != nil {
		t.Fatalf("неожиданная ошибка: %v", err)
	}
	want := filepath.Join(depDir, "файл") + ".rech"
	if got != want {
		t.Fatalf("resolveImportPath = %q, ожидалось %q", got, want)
	}
}
