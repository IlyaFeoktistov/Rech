package project

import (
	"os"
	"path/filepath"
	"slices"
	"strings"
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
	if pf.Режим != "отладка" || pf.Платформа != "текущая" {
		t.Fatalf("неожиданные значения по умолчанию: %+v", pf)
	}
	if got, want := pf.MainPath(), filepath.Join(dir, "исходники", "главный.rech"); got != want {
		t.Fatalf("MainPath() = %q, ожидалось %q", got, want)
	}
	if got, want := pf.OutputDir(), filepath.Join(dir, "сборка"); got != want {
		t.Fatalf("OutputDir() = %q, ожидалось %q", got, want)
	}
	if pf.IsRelease() {
		t.Fatalf("режим по умолчанию - отладка, не релиз")
	}
	if pf.Тип != "консольное" || pf.IsLibrary() || pf.IsWindowed() {
		t.Fatalf("ожидался тип по умолчанию \"консольное\", получено %+v", pf)
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

// TestLoadBuildTypes - все значения "сборка.тип" и отказ от неизвестного
// (в том числе прежнего "программа").
func TestLoadBuildTypes(t *testing.T) {
	cases := []struct {
		value             string
		library, windowed bool
		wantErr           bool
	}{
		{value: "консольное"},
		{value: "оконное", windowed: true},
		{value: "библиотека", library: true},
		{value: "служба"},
		{value: "программа", wantErr: true},
	}
	for _, c := range cases {
		t.Run(c.value, func(t *testing.T) {
			path := filepath.Join(t.TempDir(), "тест.rechproj")
			content := "проект:\n  имя: тест\n\nфайлы:\n  главный: исходники/главный.rech\n\nсборка:\n  тип: " + c.value + "\n"
			if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
				t.Fatalf("запись файла проекта: %v", err)
			}
			pf, err := Load(path)
			if c.wantErr {
				if err == nil || !strings.Contains(err.Error(), "оконное") {
					t.Fatalf("ожидалась ошибка со списком допустимых значений, получено %v", err)
				}
				return
			}
			if err != nil {
				t.Fatalf("Load: %v", err)
			}
			if pf.IsLibrary() != c.library || pf.IsWindowed() != c.windowed {
				t.Fatalf("IsLibrary=%v IsWindowed=%v, ожидалось %v/%v", pf.IsLibrary(), pf.IsWindowed(), c.library, c.windowed)
			}
		})
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

// TestLoadHeaders - "сборка.заголовки" (docs/SPEC.md, раздел 12 "Внешние
// функции (FFI)") - тот же формат, что и "сборка.библиотеки" выше, но для
// C-заголовков ("#include <...>"), не библиотек для линковки.
func TestLoadHeaders(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "тест.rechproj")
	content := `проект:
  имя: тест

файлы:
  главный: исходники/главный.rech

сборка:
  заголовки: windows.h, shellapi.h
`
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatalf("запись файла проекта: %v", err)
	}
	pf, err := Load(path)
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	got := pf.Заголовки()
	if len(got) != 2 || got[0] != "windows.h" || got[1] != "shellapi.h" {
		t.Fatalf("Заголовки() = %v, ожидалось [windows.h shellapi.h]", got)
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

func TestLoadPlatformRejectsUnknownValue(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "тест.rechproj")
	content := `проект:
  имя: тест

файлы:
  главный: главный.rech

сборка:
  платформа: android
`
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatalf("запись файла проекта: %v", err)
	}
	if _, err := Load(path); err == nil {
		t.Fatal("ожидалась ошибка при неизвестном сборка.платформа")
	}
}

// TestUnixPlatformMeansLinuxOrMacOSNotWindows - "unix" в сборка.платформа -
// класс из двух конкретных платформ (не отдельный член перечисления "ОС"),
// проверяется напрямую по таблице (platformOSNames), не зависит от хоста,
// на котором реально выполняется тест.
func TestUnixPlatformMeansLinuxOrMacOSNotWindows(t *testing.T) {
	names := platformOSNames["unix"]
	if !slices.Contains(names, "Linux") || !slices.Contains(names, "MacOS") || slices.Contains(names, "Windows") || len(names) != 2 {
		t.Fatalf("платформа \"unix\" должна допускать ровно Linux и MacOS, получено %v", names)
	}
}

// TestMatchesHostPlatformForEveryKnownValue - для каждого допустимого
// значения "сборка.платформа" (ключи platformOSNames) MatchesHostPlatform/
// CheckHostPlatform согласуются с самой таблицей - host-независимый тест
// (ожидание считается из platformOSNames + ast.CurrentPlatform() текущего
// прогона, тем же принципом, что и TestLoadPlatformLibraries).
func TestMatchesHostPlatformForEveryKnownValue(t *testing.T) {
	host := ast.CurrentPlatform()
	dir := t.TempDir()
	for value, names := range platformOSNames {
		path := filepath.Join(dir, value+".rechproj")
		content := "проект:\n  имя: тест\n\nфайлы:\n  главный: главный.rech\n\nсборка:\n  платформа: " + value + "\n"
		if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
			t.Fatalf("запись файла проекта: %v", err)
		}
		pf, err := Load(path)
		if err != nil {
			t.Fatalf("Load(платформа=%q): %v", value, err)
		}
		want := names == nil || slices.Contains(names, host)
		if got := pf.MatchesHostPlatform(); got != want {
			t.Fatalf("платформа %q: MatchesHostPlatform() = %v, ожидалось %v (хост %s)", value, got, want, host)
		}
		if gotErr := pf.CheckHostPlatform(); (gotErr == nil) != want {
			t.Fatalf("платформа %q: CheckHostPlatform() = %v, ожидалось nil == %v", value, gotErr, want)
		}
	}
}

// TestSourcesRootWithoutFieldDefaultsToMainFileDir - без "файлы.исходники"
// SourcesRoot ведёт себя как раньше (каталог главного файла) - обратная
// совместимость для проектов, которые это поле не указывают.
func TestSourcesRootWithoutFieldDefaultsToMainFileDir(t *testing.T) {
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
	if got, want := pf.SourcesRoot(), filepath.Join(dir, "исходники"); got != want {
		t.Fatalf("SourcesRoot() = %q, ожидалось %q", got, want)
	}
}

// TestSourcesRootFromGlobIgnoresMainFileDepth - "файлы.исходники" -
// настоящий источник корня "@путь", даже когда главный файл лежит ГЛУБЖЕ
// объявленного корня (раньше "@" резолвился бы от каталога главного файла
// - на пример, до этой фичи, ошибочно "исходники/подпапка").
func TestSourcesRootFromGlobIgnoresMainFileDepth(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "тест.rechproj")
	content := "проект:\n  имя: тест\n\nфайлы:\n  исходники: исходники/**/*.rech\n  главный: исходники/подпапка/главный.rech\n"
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatalf("запись файла проекта: %v", err)
	}
	pf, err := Load(path)
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if got, want := pf.SourcesRoot(), filepath.Join(dir, "исходники"); got != want {
		t.Fatalf("SourcesRoot() = %q, ожидалось %q", got, want)
	}
}

// TestSourcesRootFromBareWildcardGlobIsProjectDir - "*.rech" (без
// подкаталога) - корень исходников совпадает с каталогом .rechproj.
func TestSourcesRootFromBareWildcardGlobIsProjectDir(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "тест.rechproj")
	content := "проект:\n  имя: тест\n\nфайлы:\n  исходники: *.rech\n  главный: главный.rech\n"
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatalf("запись файла проекта: %v", err)
	}
	pf, err := Load(path)
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if got, want := pf.SourcesRoot(), dir; got != want {
		t.Fatalf("SourcesRoot() = %q, ожидалось %q", got, want)
	}
}

// TestRootForUsesSourcesRootNotMainFileDir - project.RootFor (точка отсчёта
// "@путь" для "rech build"/"rech run"/LSP/"rech test", package doc RootFor)
// реально использует SourcesRoot, не просто каталог главного файла.
func TestRootForUsesSourcesRootNotMainFileDir(t *testing.T) {
	dir := t.TempDir()
	if err := os.MkdirAll(filepath.Join(dir, "исходники", "подпапка"), 0o755); err != nil {
		t.Fatalf("MkdirAll: %v", err)
	}
	mainPath := filepath.Join(dir, "исходники", "подпапка", "главный.rech")
	if err := os.WriteFile(mainPath, []byte("функция главная() {}\n"), 0o644); err != nil {
		t.Fatalf("запись главного файла: %v", err)
	}
	projPath := filepath.Join(dir, "тест.rechproj")
	content := "проект:\n  имя: тест\n\nфайлы:\n  исходники: исходники/**/*.rech\n  главный: исходники/подпапка/главный.rech\n"
	if err := os.WriteFile(projPath, []byte(content), 0o644); err != nil {
		t.Fatalf("запись файла проекта: %v", err)
	}
	root, err := RootFor(mainPath)
	if err != nil {
		t.Fatalf("RootFor: %v", err)
	}
	if want := filepath.Join(dir, "исходники"); root != want {
		t.Fatalf("RootFor() = %q, ожидалось %q", root, want)
	}
}

// TestLoadOptionsSectionCapturesNestedYAMLVerbatim - "опции:" не ограничена одним
// уровнем отступа/плоским "ключ: значение", в отличие от остальных секций
// .rechproj (package doc extractOptionsSection) - список ("- ...") и вложенность
// глубже одного уровня должны дойти до Опции как есть, без искажений.
func TestLoadOptionsSectionCapturesNestedYAMLVerbatim(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "тест.rechproj")
	content := "проект:\n  имя: тест\n\nфайлы:\n  главный: исходники/главный.rech\n\n" +
		"опции:\n  мой_заголовок:\n    мои_параметры:\n      - -123\n      - -345\n    ещё_параметр: значение\n\n" +
		"сборка:\n  режим: отладка\n"
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatalf("запись файла проекта: %v", err)
	}
	pf, err := Load(path)
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	want := "мой_заголовок:\n  мои_параметры:\n    - -123\n    - -345\n  ещё_параметр: значение"
	if pf.Опции != want {
		t.Fatalf("Опции = %q, ожидалось %q", pf.Опции, want)
	}
	if pf.Режим != "отладка" {
		t.Fatalf("сборка.режим не разобран после секции \"опции:\" - Режим = %q", pf.Режим)
	}
}

// TestLoadWithoutOptionsSectionIsEmptyString - секция необязательна, её отсутствие не
// ошибка, Опции - пустая строка (не nil-паника при последующем "разобрать
// YAML" - там будет обычная ОшибкаРазбора на пустом тексте, не паника
// компилятора).
func TestLoadWithoutOptionsSectionIsEmptyString(t *testing.T) {
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
	if pf.Опции != "" {
		t.Fatalf("Опции = %q, ожидалась пустая строка", pf.Опции)
	}
}

// TestLoadServiceName - "сборка.имя_службы" принимается латиницей и
// отклоняется с кириллицей/пробелом.
func TestLoadServiceName(t *testing.T) {
	for value, ok := range map[string]bool{"journal-logger": true, "my_svc.1": true, "служба": false, "два слова": false} {
		path := filepath.Join(t.TempDir(), "тест.rechproj")
		content := "проект:\n  имя: тест\n\nфайлы:\n  главный: главный.rech\n\nсборка:\n  тип: служба\n  имя_службы: " + value + "\n"
		if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
			t.Fatal(err)
		}
		pf, err := Load(path)
		if ok && (err != nil || pf.ИмяСлужбы != value) {
			t.Errorf("%q: ожидалось принять, получено %v", value, err)
		}
		if !ok && err == nil {
			t.Errorf("%q: ожидалась ошибка", value)
		}
	}
}
