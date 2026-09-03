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

// TestLoadWithGraphResolvesTransitiveDependencyScope - файл верхнего
// уровня импортирует из зависимости "клиент", файл ВНУТРИ "клиент"
// импортирует из ЕЁ СОБСТВЕННОЙ зависимости "внутр" - резолюция второго
// импорта идёт по карте имён "клиент" (scopes[depADir]), не по карте
// программы верхнего уровня, где "внутр" вообще не объявлено.
func TestLoadWithGraphResolvesTransitiveDependencyScope(t *testing.T) {
	depBDir := t.TempDir()
	writeFile(t, depBDir, "файл.rech", "функция изB(): Целое\n{\n    вернуть 7\n}\n")

	depADir := t.TempDir()
	writeFile(t, depADir, "главный.rech", "использовать изB из \"внутр/файл\"\n\nфункция изA(): Целое\n{\n    вернуть изB()\n}\n")

	projDir := t.TempDir()
	entry := writeFile(t, projDir, "главный.rech", "использовать изA из \"клиент/главный\"\n\nфункция главная\n{\n    вывести изA()\n}\n")

	scopes := map[string]map[string]string{
		depADir: {"внутр": depBDir},
	}
	prog, err := LoadWithGraph(entry, testFrontend{}, map[string]string{"клиент": depADir}, scopes)
	if err != nil {
		t.Fatalf("LoadWithGraph: %v", err)
	}
	if len(prog.Modules) != 3 {
		t.Fatalf("ожидалось 3 модуля (главный/depA/depB), получено %d", len(prog.Modules))
	}
}

// TestResolveImportPathPrefersDependencyOverLocalDir - если "клиент" ОБЕ
// является объявленной зависимостью И существует как локальный подкаталог
// рядом с импортирующим файлом, побеждает зависимость (deps проверяются
// раньше baseDir) - однозначное правило, не угадывание по факту
// существования файла на диске.
func TestResolveImportPathPrefersDependencyOverLocalDir(t *testing.T) {
	depDir := t.TempDir()
	got, crossedInto, err := resolveImportPath("/базовый/каталог", "/корень/программы", "клиент/файл", map[string]string{"клиент": depDir})
	if err != nil {
		t.Fatalf("неожиданная ошибка: %v", err)
	}
	want := filepath.Join(depDir, "файл") + ".rech"
	if got != want {
		t.Fatalf("resolveImportPath = %q, ожидалось %q", got, want)
	}
	if crossedInto != depDir {
		t.Fatalf("crossedInto = %q, ожидалось %q", crossedInto, depDir)
	}
}
