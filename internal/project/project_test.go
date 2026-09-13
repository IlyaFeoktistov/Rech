package project

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/IlyaFeoktistov/Rech/internal/ast"
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

// TestDBPoolTimeoutMsDefaultIsZero - "бд.таймаут_пула_мс" не указан (или
// .rechproj не несёт секции "бд" вовсе) - 0, без ошибки (в отличие от
// DBPoolSize, здесь 0 - содержательное умолчание "ждать бесконечно", не
// сентинел "не задано").
func TestDBPoolTimeoutMsDefaultIsZero(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "тест.rechproj")
	content := "проект:\n  имя: тест\n\nфайлы:\n  главный: исходники/главный.rech\n"
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatalf("запись файла проекта: %v", err)
	}
	pf, err := Load(path)
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	ms, err := pf.DBPoolTimeoutMs()
	if err != nil {
		t.Fatalf("DBPoolTimeoutMs: %v", err)
	}
	if ms != 0 {
		t.Fatalf("ожидалось 0 (ждать бесконечно), получено %d", ms)
	}
}

// TestDBPoolTimeoutMsParsesPositiveValue - значение указано явно.
func TestDBPoolTimeoutMsParsesPositiveValue(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "тест.rechproj")
	content := "проект:\n  имя: тест\n\nфайлы:\n  главный: исходники/главный.rech\n\nбд:\n  таймаут_пула_мс: 500\n"
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatalf("запись файла проекта: %v", err)
	}
	pf, err := Load(path)
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	ms, err := pf.DBPoolTimeoutMs()
	if err != nil {
		t.Fatalf("DBPoolTimeoutMs: %v", err)
	}
	if ms != 500 {
		t.Fatalf("ожидалось 500, получено %d", ms)
	}
}

// TestDBPoolTimeoutMsRejectsNegative - отрицательное значение бессмысленно
// как таймаут.
func TestDBPoolTimeoutMsRejectsNegative(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "тест.rechproj")
	content := "проект:\n  имя: тест\n\nфайлы:\n  главный: исходники/главный.rech\n\nбд:\n  таймаут_пула_мс: -1\n"
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatalf("запись файла проекта: %v", err)
	}
	pf, err := Load(path)
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if _, err := pf.DBPoolTimeoutMs(); err == nil {
		t.Fatalf("ожидалась ошибка на отрицательное значение таймаута")
	}
}

// TestDBSQLiteBusyTimeoutMsDefaultIsZero - не указан - 0 (internal/compiler
// в этом случае не переопределяет ir.Program.DBSQLiteBusyTimeoutMs,
// остаётся встроенное умолчание internal/semantic.
// DefaultDBSQLiteBusyTimeoutMs = 5000).
func TestDBSQLiteBusyTimeoutMsDefaultIsZero(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "тест.rechproj")
	content := "проект:\n  имя: тест\n\nфайлы:\n  главный: исходники/главный.rech\n"
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatalf("запись файла проекта: %v", err)
	}
	pf, err := Load(path)
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	ms, err := pf.DBSQLiteBusyTimeoutMs()
	if err != nil {
		t.Fatalf("DBSQLiteBusyTimeoutMs: %v", err)
	}
	if ms != 0 {
		t.Fatalf("ожидалось 0 (использовать встроенное умолчание), получено %d", ms)
	}
}

// TestDBSQLiteBusyTimeoutMsParsesPositiveValue - значение указано явно.
func TestDBSQLiteBusyTimeoutMsParsesPositiveValue(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "тест.rechproj")
	content := "проект:\n  имя: тест\n\nфайлы:\n  главный: исходники/главный.rech\n\nбд:\n  sqlite_таймаут_блокировки_мс: 2000\n"
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatalf("запись файла проекта: %v", err)
	}
	pf, err := Load(path)
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	ms, err := pf.DBSQLiteBusyTimeoutMs()
	if err != nil {
		t.Fatalf("DBSQLiteBusyTimeoutMs: %v", err)
	}
	if ms != 2000 {
		t.Fatalf("ожидалось 2000, получено %d", ms)
	}
}

// TestDBSQLiteBusyTimeoutMsRejectsNonPositive - 0/отрицательное не
// поддержаны как явное значение (см. package doc DBSQLiteBusyTimeoutMs).
func TestDBSQLiteBusyTimeoutMsRejectsNonPositive(t *testing.T) {
	for _, v := range []string{"0", "-1"} {
		dir := t.TempDir()
		path := filepath.Join(dir, "тест.rechproj")
		content := "проект:\n  имя: тест\n\nфайлы:\n  главный: исходники/главный.rech\n\nбд:\n  sqlite_таймаут_блокировки_мс: " + v + "\n"
		if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
			t.Fatalf("запись файла проекта: %v", err)
		}
		pf, err := Load(path)
		if err != nil {
			t.Fatalf("Load: %v", err)
		}
		if _, err := pf.DBSQLiteBusyTimeoutMs(); err == nil {
			t.Fatalf("ожидалась ошибка на значение %q", v)
		}
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

// TestLoadPlatformLibraries - "сборка.библиотеки_linux"/"_windows"/"_macos"
// (docs/SPEC.md, "#если ОС.<Член>") добавляются к безусловному списку
// ТОЛЬКО на своей платформе - тест написан под платформу, на которой
// реально исполняется (ast.CurrentPlatform()), тем же принципом, что и
// windowsWantOverrides в internal/interp/interp_test.go (сама фича
// платформозависима по определению).
func TestLoadPlatformLibraries(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "тест.rechproj")
	content := `проект:
  имя: тест

файлы:
  главный: исходники/главный.rech

сборка:
  библиотеки: curl
  библиотеки_linux: X11
  библиотеки_windows: user32, gdi32
  библиотеки_macos: Cocoa
`
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatalf("запись файла проекта: %v", err)
	}
	pf, err := Load(path)
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	got := pf.Библиотеки()
	want := append([]string{"curl"}, splitCommaList(map[string]string{
		"Linux": "X11", "Windows": "user32, gdi32", "MacOS": "Cocoa",
	}[ast.CurrentPlatform()])...)
	if len(got) != len(want) {
		t.Fatalf("Библиотеки() = %v, ожидалось %v (текущая платформа - %s)", got, want, ast.CurrentPlatform())
	}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("Библиотеки() = %v, ожидалось %v (текущая платформа - %s)", got, want, ast.CurrentPlatform())
		}
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

// TestLoadLibraryPathsResolvesRelativeToProjectDir - "сборка.путь_библиотек"
// разрешается относительно каталога САМОГО .rechproj, не текущего рабочего
// каталога процесса - тем же принципом, что "файлы.главный"/"файлы.выход".
func TestLoadLibraryPathsResolvesRelativeToProjectDir(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "тест.rechproj")
	// Абсолютный путь строится с ТЕМ ЖЕ диском, что и dir (пусто на Unix,
	// "C:" и т.п. на Windows) - буквальный "/абсолютный/путь" без буквы
	// диска на Windows НЕ абсолютный (filepath.IsAbs требует диск), тест
	// иначе проверял бы разное поведение на разных ОС.
	absPath := filepath.VolumeName(dir) + string(filepath.Separator) + "абсолютный" + string(filepath.Separator) + "путь"
	content := `проект:
  имя: тест

файлы:
  главный: исходники/главный.rech

сборка:
  путь_библиотек: ../экспорт_библиотеки/сборка, ` + absPath + `
`
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatalf("запись файла проекта: %v", err)
	}
	pf, err := Load(path)
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	got := pf.ПутиБиблиотек()
	want := []string{
		filepath.Join(dir, "../экспорт_библиотеки/сборка"),
		absPath,
	}
	if len(got) != 2 || got[0] != want[0] || got[1] != want[1] {
		t.Fatalf("ПутиБиблиотек() = %v, ожидалось %v", got, want)
	}
}

func TestLoadWithoutLibraryPathsReturnsNil(t *testing.T) {
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
	if got := pf.ПутиБиблиотек(); got != nil {
		t.Fatalf("ПутиБиблиотек() = %v, ожидался nil", got)
	}
}

func TestLoadDependencies(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "тест.rechproj")
	content := `проект:
  имя: тест

файлы:
  главный: а.rech

зависимости:
  клиент: https://github.com/user/repo v1.2.0
  утилиты: https://gitlab.com/user/utils main
`
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatalf("запись файла проекта: %v", err)
	}
	pf, err := Load(path)
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if len(pf.Зависимости) != 2 {
		t.Fatalf("ожидалось 2 зависимости, получено %+v", pf.Зависимости)
	}
	if got := pf.Зависимости["клиент"]; got.URL != "https://github.com/user/repo" || got.Версия != "v1.2.0" {
		t.Fatalf("зависимость \"клиент\" = %+v, не совпадает с ожидаемым", got)
	}
	if got := pf.Зависимости["утилиты"]; got.URL != "https://gitlab.com/user/utils" || got.Версия != "main" {
		t.Fatalf("зависимость \"утилиты\" = %+v, не совпадает с ожидаемым", got)
	}
}

func TestLoadDependencyDuplicateRejected(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "тест.rechproj")
	content := `проект:
  имя: тест
файлы:
  главный: а.rech
зависимости:
  клиент: https://github.com/user/repo v1.0.0
  клиент: https://github.com/user/repo v2.0.0
`
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatalf("запись файла проекта: %v", err)
	}
	if _, err := Load(path); err == nil {
		t.Fatalf("ожидалась ошибка на дубль зависимости \"клиент\"")
	}
}

func TestLoadEnvironment(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "тест.rechproj")
	content := `проект:
  имя: тест

файлы:
  главный: а.rech

окружение:
  RECH_HTTP_PORT: 9090
  ОТЛАДКА: истина
`
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatalf("запись файла проекта: %v", err)
	}
	pf, err := Load(path)
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if len(pf.Окружение) != 2 {
		t.Fatalf("ожидалось 2 переменные окружения, получено %+v", pf.Окружение)
	}
	if got := pf.Окружение["RECH_HTTP_PORT"]; got != "9090" {
		t.Fatalf("RECH_HTTP_PORT = %q, ожидалось \"9090\"", got)
	}
	if got := pf.Окружение["ОТЛАДКА"]; got != "истина" {
		t.Fatalf("ОТЛАДКА = %q, ожидалось \"истина\"", got)
	}
}

func TestLoadEnvironmentDuplicateRejected(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "тест.rechproj")
	content := `проект:
  имя: тест
файлы:
  главный: а.rech
окружение:
  ПОРТ: 8080
  ПОРТ: 9090
`
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatalf("запись файла проекта: %v", err)
	}
	if _, err := Load(path); err == nil {
		t.Fatalf("ожидалась ошибка на дубль переменной окружения \"ПОРТ\"")
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

// TestPoolShrinkEnabledDefaultIsTrue - "потоки.сжатие_пула" не указан (или
// .rechproj не несёт секции "потоки" вовсе, или .rechproj нет вовсе) -
// true, пул воркер-потоков сжимается по умолчанию (Шаг
// 81) - "ложь" нужно указать явно, чтобы вернуть поведение Шага 80 (растёт
// и никогда не сжимается).
func TestPoolShrinkEnabledDefaultIsTrue(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "тест.rechproj")
	content := "проект:\n  имя: тест\n\nфайлы:\n  главный: исходники/главный.rech\n"
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatalf("запись файла проекта: %v", err)
	}
	pf, err := Load(path)
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	enabled, err := pf.PoolShrinkEnabled()
	if err != nil {
		t.Fatalf("PoolShrinkEnabled: %v", err)
	}
	if !enabled {
		t.Fatalf("ожидалось true (пул сжимается по умолчанию), получено false")
	}
}

// TestPoolShrinkEnabledParsesTrueAndFalse - явные "истина"/"ложь".
func TestPoolShrinkEnabledParsesTrueAndFalse(t *testing.T) {
	for _, tc := range []struct {
		raw  string
		want bool
	}{
		{"истина", true},
		{"ложь", false},
	} {
		dir := t.TempDir()
		path := filepath.Join(dir, "тест.rechproj")
		content := "проект:\n  имя: тест\n\nфайлы:\n  главный: исходники/главный.rech\n\nпотоки:\n  сжатие_пула: " + tc.raw + "\n"
		if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
			t.Fatalf("запись файла проекта: %v", err)
		}
		pf, err := Load(path)
		if err != nil {
			t.Fatalf("Load: %v", err)
		}
		got, err := pf.PoolShrinkEnabled()
		if err != nil {
			t.Fatalf("PoolShrinkEnabled(%q): %v", tc.raw, err)
		}
		if got != tc.want {
			t.Fatalf("PoolShrinkEnabled(%q) = %v, ожидалось %v", tc.raw, got, tc.want)
		}
	}
}

// TestPoolShrinkEnabledRejectsGarbage - значение, отличное от
// "истина"/"ложь" - ошибка, не молчаливое "выключено".
func TestPoolShrinkEnabledRejectsGarbage(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "тест.rechproj")
	content := "проект:\n  имя: тест\n\nфайлы:\n  главный: исходники/главный.rech\n\nпотоки:\n  сжатие_пула: да\n"
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatalf("запись файла проекта: %v", err)
	}
	pf, err := Load(path)
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if _, err := pf.PoolShrinkEnabled(); err == nil {
		t.Fatalf("ожидалась ошибка на значение, отличное от \"истина\"/\"ложь\"")
	}
}

// TestPoolIdleTimeoutMsDefaultIsZero - не указан - 0 (internal/compiler в
// этом случае не переопределяет ir.Program.PoolIdleTimeoutMs, остаётся
// встроенное умолчание internal/semantic.DefaultPoolIdleTimeoutMs).
func TestPoolIdleTimeoutMsDefaultIsZero(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "тест.rechproj")
	content := "проект:\n  имя: тест\n\nфайлы:\n  главный: исходники/главный.rech\n"
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatalf("запись файла проекта: %v", err)
	}
	pf, err := Load(path)
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	ms, err := pf.PoolIdleTimeoutMs()
	if err != nil {
		t.Fatalf("PoolIdleTimeoutMs: %v", err)
	}
	if ms != 0 {
		t.Fatalf("ожидалось 0 (использовать встроенное умолчание), получено %d", ms)
	}
}

// TestPoolIdleTimeoutMsParsesPositiveValue - значение указано явно.
func TestPoolIdleTimeoutMsParsesPositiveValue(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "тест.rechproj")
	content := "проект:\n  имя: тест\n\nфайлы:\n  главный: исходники/главный.rech\n\nпотоки:\n  таймаут_простоя_мс: 5000\n"
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatalf("запись файла проекта: %v", err)
	}
	pf, err := Load(path)
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	ms, err := pf.PoolIdleTimeoutMs()
	if err != nil {
		t.Fatalf("PoolIdleTimeoutMs: %v", err)
	}
	if ms != 5000 {
		t.Fatalf("ожидалось 5000, получено %d", ms)
	}
}

// TestPoolIdleTimeoutMsRejectsNonPositive - ноль/отрицательное значение
// бессмысленны как таймаут простоя (в отличие от DBPoolTimeoutMs, здесь 0 -
// НЕ содержательное умолчание "ждать бесконечно").
func TestPoolIdleTimeoutMsRejectsNonPositive(t *testing.T) {
	for _, raw := range []string{"0", "-1"} {
		dir := t.TempDir()
		path := filepath.Join(dir, "тест.rechproj")
		content := "проект:\n  имя: тест\n\nфайлы:\n  главный: исходники/главный.rech\n\nпотоки:\n  таймаут_простоя_мс: " + raw + "\n"
		if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
			t.Fatalf("запись файла проекта: %v", err)
		}
		pf, err := Load(path)
		if err != nil {
			t.Fatalf("Load: %v", err)
		}
		if _, err := pf.PoolIdleTimeoutMs(); err == nil {
			t.Fatalf("ожидалась ошибка на значение %q", raw)
		}
	}
}

// TestHTTPMaxRequestBytesDefaultIsZero - поле не указано - 0, "используй
// встроенное умолчание" (тем же принципом, что DBPoolSize).
func TestHTTPMaxRequestBytesDefaultIsZero(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "тест.rechproj")
	content := "проект:\n  имя: тест\n\nфайлы:\n  главный: исходники/главный.rech\n"
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatalf("запись файла проекта: %v", err)
	}
	pf, err := Load(path)
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	n, err := pf.HTTPMaxRequestBytes()
	if err != nil {
		t.Fatalf("HTTPMaxRequestBytes: %v", err)
	}
	if n != 0 {
		t.Fatalf("ожидалось 0 (использовать встроенное умолчание), получено %d", n)
	}
}

// TestHTTPMaxRequestBytesParsesPositiveValue - значение указано явно.
func TestHTTPMaxRequestBytesParsesPositiveValue(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "тест.rechproj")
	content := "проект:\n  имя: тест\n\nфайлы:\n  главный: исходники/главный.rech\n\nhttp:\n  макс_размер_запроса_байт: 65536\n"
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatalf("запись файла проекта: %v", err)
	}
	pf, err := Load(path)
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	n, err := pf.HTTPMaxRequestBytes()
	if err != nil {
		t.Fatalf("HTTPMaxRequestBytes: %v", err)
	}
	if n != 65536 {
		t.Fatalf("ожидалось 65536, получено %d", n)
	}
}

// TestHTTPMaxRequestBytesRejectsNonPositive - ноль/отрицательное значение
// бессмысленны как предел размера ("0" неотличим от "поле не указано" - см.
// package doc HTTPMaxRequestBytes).
func TestHTTPMaxRequestBytesRejectsNonPositive(t *testing.T) {
	for _, raw := range []string{"0", "-1"} {
		dir := t.TempDir()
		path := filepath.Join(dir, "тест.rechproj")
		content := "проект:\n  имя: тест\n\nфайлы:\n  главный: исходники/главный.rech\n\nhttp:\n  макс_размер_запроса_байт: " + raw + "\n"
		if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
			t.Fatalf("запись файла проекта: %v", err)
		}
		pf, err := Load(path)
		if err != nil {
			t.Fatalf("Load: %v", err)
		}
		if _, err := pf.HTTPMaxRequestBytes(); err == nil {
			t.Fatalf("ожидалась ошибка на значение %q", raw)
		}
	}
}
