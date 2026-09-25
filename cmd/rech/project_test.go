package main

import (
	"path/filepath"
	"runtime"
	"slices"
	"testing"

	"github.com/IlyaFeoktistov/Rech/internal/project"
	"github.com/IlyaFeoktistov/Rech/internal/semantic"
)

// TestInitProjectKinds - каждый тип "rech init" даёт .rechproj с нужным
// "сборка.тип" и главный файл, который проходит разбор и анализ на этой ОС.
func TestInitProjectKinds(t *testing.T) {
	cases := []struct {
		flag, wantName  string
		library, static bool
		windowed        bool
		service         bool
	}{
		{flag: "--console", wantName: "проба"},
		{flag: "--gui", wantName: "проба", windowed: true},
		{flag: "--service", wantName: "проба", service: true},
		{flag: "--library", wantName: "libпроба", library: true},
		{flag: "--static-library", wantName: "libпроба", library: true, static: true},
	}
	for _, c := range cases {
		t.Run(c.flag, func(t *testing.T) {
			t.Chdir(t.TempDir())
			if err := initProject([]string{c.flag, "проба"}); err != nil {
				t.Fatalf("initProject: %v", err)
			}
			pf, err := project.Load(c.wantName + ".rechproj")
			if err != nil {
				t.Fatalf("project.Load: %v", err)
			}
			if pf.IsLibrary() != c.library || pf.IsStaticLibrary() != c.static || pf.IsWindowed() != c.windowed || pf.IsService() != c.service {
				t.Fatalf("тип %q не соответствует флагу %s", pf.Тип, c.flag)
			}
			if filepath.Base(pf.MainPath()) != initKinds[c.flag].mainFile {
				t.Fatalf("главный файл %q, ожидался %q", pf.MainPath(), initKinds[c.flag].mainFile)
			}
			prog, _, err := InitializeCompiler().ParseSource(pf.MainPath())
			if err != nil {
				t.Fatalf("ParseSource: %v", err)
			}
			if _, err := semantic.Analyze(prog); err != nil {
				t.Fatalf("semantic.Analyze: %v", err)
			}
		})
	}
}

// TestParseInitArgs - флаг по умолчанию и отказ от неверных аргументов.
func TestParseInitArgs(t *testing.T) {
	if flag, name, err := parseInitArgs([]string{"имя"}); err != nil || flag != defaultInitFlag || name != "имя" {
		t.Fatalf("без флага: %q %q %v", flag, name, err)
	}
	for _, args := range [][]string{{"--неизвестный"}, {"--console", "--gui"}, {"а", "б"}} {
		if _, _, err := parseInitArgs(args); err == nil {
			t.Errorf("ожидалась ошибка для %q", args)
		}
	}
}

// TestWriteServiceFiles - файлы установки службы лежат рядом с программой,
// имя службы/юнита - латиницей.
func TestWriteServiceFiles(t *testing.T) {
	dir := t.TempDir()
	paths, err := writeServiceFiles(filepath.Join(dir, "служба"), "моя_служба", "")
	if err != nil {
		t.Fatalf("writeServiceFiles: %v", err)
	}
	var names []string
	for _, p := range paths {
		if filepath.Dir(p) != dir {
			t.Errorf("%s не рядом с программой", p)
		}
		names = append(names, filepath.Base(p))
	}
	want := map[string][]string{
		"linux":   {"moya_sluzhba.service"},
		"windows": {"moya_sluzhba_удалить.cmd", "moya_sluzhba_установить.cmd"},
	}[runtime.GOOS]
	if !slices.Equal(names, want) {
		t.Fatalf("файлы %q, ожидались %q", names, want)
	}
}

// TestWriteServiceFilesExplicitName - "сборка.имя_службы" вместо
// транслитерации.
func TestWriteServiceFilesExplicitName(t *testing.T) {
	if runtime.GOOS != "linux" {
		t.Skip("имя файла юнита - только на Linux")
	}
	paths, err := writeServiceFiles(filepath.Join(t.TempDir(), "служба"), "моя_служба", "journal-logger")
	if err != nil || len(paths) != 1 || filepath.Base(paths[0]) != "journal-logger.service" {
		t.Fatalf("ожидался journal-logger.service, получено %q, %v", paths, err)
	}
}
