package main

import (
	"path/filepath"
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
	}{
		{flag: "--console", wantName: "проба"},
		{flag: "--gui", wantName: "проба", windowed: true},
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
			if pf.IsLibrary() != c.library || pf.IsStaticLibrary() != c.static || pf.IsWindowed() != c.windowed {
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
