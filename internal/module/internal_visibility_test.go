package module

import (
	"strings"
	"testing"
)

// TestLoadAllowsSameProjectImportOfInternalFunction - "внутренний" виден
// любому файлу ТОГО ЖЕ проекта (обычный относительный импорт, не через
// границу зависимости) - см. package doc ast.Internal.
func TestLoadAllowsSameProjectImportOfInternalFunction(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, dir, "утилиты.rech", "внутренняя функция помощник(): Целое\n{\n    вернуть 1\n}\n")
	entry := writeFile(t, dir, "главный.rech", "использовать помощник из \"утилиты\"\n\nфункция главная\n{\n    вывести помощник()\n}\n")

	if _, err := Load(entry, testFrontend{}); err != nil {
		t.Fatalf("Load: %v", err)
	}
}

// TestLoadRejectsImportOfInternalFunctionAcrossDependencyBoundary -
// "внутренний" НЕ виден файлу, дошедшему до объявления ЧЕРЕЗ ГРАНИЦУ
// зависимости ("использовать ... из "имя_зависимости/путь"") - тот же
// сценарий, что TestLoadWithDepsResolvesFirstSegmentAsDependency
// (deps_test.go), но целевая функция помечена "внутренний".
func TestLoadRejectsImportOfInternalFunctionAcrossDependencyBoundary(t *testing.T) {
	depDir := t.TempDir()
	writeFile(t, depDir, "утилиты.rech", "внутренняя функция помощник(): Целое\n{\n    вернуть 1\n}\n")

	projDir := t.TempDir()
	entry := writeFile(t, projDir, "главный.rech", "использовать помощник из \"клиент/утилиты\"\n\nфункция главная\n{\n    вывести помощник()\n}\n")

	deps := map[string]string{"клиент": depDir}
	_, err := LoadWithDeps(entry, testFrontend{}, deps)
	if err == nil {
		t.Fatal("ожидалась ошибка - \"помощник\" внутреннее, дошли через границу зависимости")
	}
	if !strings.Contains(err.Error(), "внутреннее") {
		t.Fatalf("неожиданный текст ошибки: %v", err)
	}
}

// TestLoadAllowsInternalFunctionWithinSameDependency - "внутренний"
// по-прежнему виден файлам ВНУТРИ ТОЙ ЖЕ зависимости, не только корневой
// программе - граница пересекается только один раз, при переходе В саму
// зависимость, а не на каждом дальнейшем "использовать" внутри неё.
func TestLoadAllowsInternalFunctionWithinSameDependency(t *testing.T) {
	depDir := t.TempDir()
	writeFile(t, depDir, "утилиты.rech", "внутренняя функция помощник(): Целое\n{\n    вернуть 1\n}\n")
	writeFile(t, depDir, "главный.rech", "использовать помощник из \"утилиты\"\n\nфункция наружное(): Целое\n{\n    вернуть помощник()\n}\n")

	projDir := t.TempDir()
	entry := writeFile(t, projDir, "главный.rech", "использовать наружное из \"клиент/главный\"\n\nфункция главная\n{\n    вывести наружное()\n}\n")

	deps := map[string]string{"клиент": depDir}
	if _, err := LoadWithDeps(entry, testFrontend{}, deps); err != nil {
		t.Fatalf("LoadWithDeps: %v", err)
	}
}
