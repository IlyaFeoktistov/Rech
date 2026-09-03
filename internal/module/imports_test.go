package module

import (
	"path/filepath"
	"strings"
	"testing"
)

// TestLoadResolvesAbsoluteImportFromSiblingSubfolder - НАЙДЕННАЯ И
// ИСПРАВЛЕННАЯ находка (docs/ROADMAP.md, Шаг 41/48): обычный относительный
// путь без "@" не может выйти из своей поддиректории вообще - файл в
// "бд/файл.rech" не мог дотянуться до "токен/файл.rech" (соседняя
// подпапка). "@путь" резолвится от projectRoot (каталог entry-файла), не
// от каталога импортирующего файла - работает из любой подпапки, на любой
// глубине.
func TestLoadResolvesAbsoluteImportFromSiblingSubfolder(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, dir, "токен/токен.rech", "функция создать_токен(): Строка\n{\n    вернуть \"жетон\"\n}\n")
	writeFile(t, dir, "бд/бд.rech", "использовать создать_токен из \"@токен/токен\"\n\nфункция подключиться(): Строка\n{\n    вернуть создать_токен()\n}\n")
	entry := writeFile(t, dir, "главный.rech", "использовать подключиться из \"бд/бд\"\n\nфункция главная\n{\n    вывести подключиться()\n}\n")

	prog, err := Load(entry, testFrontend{})
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if len(prog.Modules) != 3 {
		t.Fatalf("ожидалось 3 модуля, получено %d", len(prog.Modules))
	}
}

// TestLoadResolvesRelativeParentImport - "../" (в начале пути) - обычная
// навигация вверх по дереву каталогов относительно файла, где написано
// "использовать" - тот же смысл, что и в путях большинства языков.
func TestLoadResolvesRelativeParentImport(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, dir, "токен/токен.rech", "функция создать_токен(): Строка\n{\n    вернуть \"жетон\"\n}\n")
	writeFile(t, dir, "бд/бд.rech", "использовать создать_токен из \"../токен/токен\"\n\nфункция подключиться(): Строка\n{\n    вернуть создать_токен()\n}\n")
	entry := writeFile(t, dir, "главный.rech", "использовать подключиться из \"бд/бд\"\n\nфункция главная\n{\n    вывести подключиться()\n}\n")

	prog, err := Load(entry, testFrontend{})
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if len(prog.Modules) != 3 {
		t.Fatalf("ожидалось 3 модуля, получено %d", len(prog.Modules))
	}
}

func TestResolveImportPathAbsoluteFromProjectRoot(t *testing.T) {
	got, crossedInto, err := resolveImportPath("/проект/бд", "/проект", "@токен/токен", nil)
	if err != nil {
		t.Fatalf("неожиданная ошибка: %v", err)
	}
	if want := filepath.Join("/проект", "токен", "токен") + ".rech"; got != want {
		t.Fatalf("resolveImportPath = %q, ожидалось %q", got, want)
	}
	if crossedInto != "" {
		t.Fatalf("crossedInto = %q, ожидалось пусто - \"@\" не пересекает границу зависимости", crossedInto)
	}
}

func TestResolveImportPathParentNavigation(t *testing.T) {
	got, _, err := resolveImportPath("/проект/бд", "/проект", "../токен/токен", nil)
	if err != nil {
		t.Fatalf("неожиданная ошибка: %v", err)
	}
	if want := filepath.Join("/проект", "токен", "токен") + ".rech"; got != want {
		t.Fatalf("resolveImportPath = %q, ожидалось %q", got, want)
	}
}

func TestResolveImportPathDoubleParentNavigation(t *testing.T) {
	got, _, err := resolveImportPath("/проект/а/б", "/проект", "../../в/файл", nil)
	if err != nil {
		t.Fatalf("неожиданная ошибка: %v", err)
	}
	if want := filepath.Join("/проект", "в", "файл") + ".rech"; got != want {
		t.Fatalf("resolveImportPath = %q, ожидалось %q", got, want)
	}
}

// TestResolveImportPathRejectsDotDotInMiddle - ".." годится ТОЛЬКО ведущей
// последовательностью в начале пути - "токен/../токен/токен" (петляющий,
// не поднимающийся путь) по-прежнему отклонён самим алфавитом
// ValidSegmentName, как и раньше.
func TestResolveImportPathRejectsDotDotInMiddle(t *testing.T) {
	_, _, err := resolveImportPath("/проект/бд", "/проект", "токен/../токен/токен", nil)
	if err == nil || !strings.Contains(err.Error(), "строчных букв") {
		t.Fatalf("ожидалась ошибка алфавита сегмента, получено: %v", err)
	}
}

// TestResolveImportPathRejectsBareDotDot - ".." без какого-либо пути после
// неё - нечего резолвить.
func TestResolveImportPathRejectsBareDotDot(t *testing.T) {
	_, _, err := resolveImportPath("/проект/бд", "/проект", "..", nil)
	if err == nil || !strings.Contains(err.Error(), "ожидался путь к файлу") {
		t.Fatalf("ожидалась ошибка про отсутствие пути после \"..\", получено: %v", err)
	}
}

// TestLoadWithGraphAbsoluteImportInDependencyUsesOwnRoot - "@путь" внутри
// файла ЗАВИСИМОСТИ резолвится от КОРНЯ САМОЙ этой зависимости, не от
// корня программы, которая её подключила - тем же принципом
// "приватности", что и у deps/scopes (package doc LoadWithGraph).
func TestLoadWithGraphAbsoluteImportInDependencyUsesOwnRoot(t *testing.T) {
	depDir := t.TempDir()
	writeFile(t, depDir, "внутренности/утилита.rech", "функция взять(): Целое\n{\n    вернуть 7\n}\n")
	writeFile(t, depDir, "главный.rech", "использовать взять из \"@внутренности/утилита\"\n\nфункция изЗависимости(): Целое\n{\n    вернуть взять()\n}\n")

	projDir := t.TempDir()
	entry := writeFile(t, projDir, "главный.rech", "использовать изЗависимости из \"клиент/главный\"\n\nфункция главная\n{\n    вывести изЗависимости()\n}\n")

	prog, err := LoadWithDeps(entry, testFrontend{}, map[string]string{"клиент": depDir})
	if err != nil {
		t.Fatalf("LoadWithDeps: %v", err)
	}
	if len(prog.Modules) != 3 {
		t.Fatalf("ожидалось 3 модуля, получено %d", len(prog.Modules))
	}
}
