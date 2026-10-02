package module

import (
	"strings"
	"testing"

	"github.com/IlyaFeoktistov/Rech/internal/ast"
)

func TestLinkImportsSynonymAndRewritesItsTarget(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, dir, "типы.rech", "сущность Точка\n{\n    имеет:\n        Целое x\n}\n\nпубличный синоним Точки = Список<Точка>\n")
	entry := writeFile(t, dir, "главный.rech", "использовать Точки как Набор из \"типы\"\n\nфункция главная\n{\n    переменная т: Набор = []\n}\n")

	prog, err := Load(entry, testFrontend{})
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	merged, err := prog.Link()
	if err != nil {
		t.Fatalf("Link: %v", err)
	}
	if len(merged.Synonyms) != 1 {
		t.Fatalf("ожидался 1 синоним, получено %d", len(merged.Synonyms))
	}
	syn := merged.Synonyms[0]
	if syn.Name != "Точки" || syn.Target != "Список<Типы_Точка>" {
		t.Fatalf("синоним - под своим именем, цель - со смангленной сущностью: %q = %q", syn.Name, syn.Target)
	}
	for _, fn := range merged.Functions {
		if fn.Name != "главная" {
			continue
		}
		decl := fn.Body[0].(*ast.VarDecl)
		if decl.Type != "Точки" {
			t.Fatalf("алиас импорта должен разрешиться в имя синонима, получено %q", decl.Type)
		}
	}
}

func TestLinkRejectsPrivateSynonymImport(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, dir, "типы.rech", "приватный синоним Числа = Список<Целое>\n")
	entry := writeFile(t, dir, "главный.rech", "использовать Числа из \"типы\"\n\nфункция главная\n{\n}\n")

	_, err := Load(entry, testFrontend{})
	if err == nil || !strings.Contains(err.Error(), "Числа") {
		t.Fatalf("ожидалась ошибка импорта приватного синонима, получено %v", err)
	}
}
