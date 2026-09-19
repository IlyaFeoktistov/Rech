package module

import "testing"

// TestLoadResolvesCyrillicExtensionSynonyms - "использовать X из "путь""
// находит "путь.речь" (полный кириллический синоним) или "путь.р"
// (короткий), когда "путь.rech" не существует - синонимы равноправны с
// ".rech" везде, где резолвится импорт (docs/SPEC.md, раздел 1).
func TestLoadResolvesCyrillicExtensionSynonyms(t *testing.T) {
	for _, ext := range []string{".речь", ".р"} {
		t.Run(ext, func(t *testing.T) {
			dir := t.TempDir()
			writeFile(t, dir, "утилиты"+ext, "публичная функция удвоить(х: Целое): Целое\n{\n    вернуть х * 2\n}\n")
			entry := writeFile(t, dir, "главный.rech", "использовать удвоить из \"утилиты\"\n\nфункция главная\n{\n    вывести удвоить(21)\n}\n")

			prog, err := Load(entry, testFrontend{})
			if err != nil {
				t.Fatalf("Load: %v", err)
			}
			if len(prog.Modules) != 2 {
				t.Fatalf("ожидалось 2 модуля, получено %d", len(prog.Modules))
			}
		})
	}
}

// TestLoadRejectsAmbiguousExtensionSynonyms - "путь.rech" и "путь.речь"
// одновременно рядом - явная ошибка компиляции (неоднозначность), а не
// молчаливый приоритет одного расширения над другим.
func TestLoadRejectsAmbiguousExtensionSynonyms(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, dir, "утилиты.rech", "публичная функция удвоить(х: Целое): Целое\n{\n    вернуть х * 2\n}\n")
	writeFile(t, dir, "утилиты.речь", "публичная функция удвоить(х: Целое): Целое\n{\n    вернуть х * 2\n}\n")
	entry := writeFile(t, dir, "главный.rech", "использовать удвоить из \"утилиты\"\n\nфункция главная\n{\n    вывести удвоить(21)\n}\n")

	_, err := Load(entry, testFrontend{})
	if err == nil {
		t.Fatal("ожидалась ошибка неоднозначности расширений")
	}
}

// TestValidateSourceFileNameAcceptsSynonyms - проверяет сам список
// SourceExtensions/HasSourceExtension/TrimSourceExtension согласованно,
// без зависимости от internal/compiler (у него отдельный тест на
// validateSourceFileName).
func TestHasAndTrimSourceExtension(t *testing.T) {
	cases := map[string]bool{
		"главный.rech":     true,
		"главный.речь":     true,
		"главный.р":        true,
		"главный.rechproj": false,
		"главный.rechi":    false,
		"главный":          false,
	}
	for name, want := range cases {
		if got := HasSourceExtension(name); got != want {
			t.Errorf("HasSourceExtension(%q) = %v, хотели %v", name, got, want)
		}
	}
	if got := TrimSourceExtension("главный.речь"); got != "главный" {
		t.Errorf("TrimSourceExtension(\"главный.речь\") = %q, хотели \"главный\"", got)
	}
	if got := TrimSourceExtension("главный.rechproj"); got != "главный.rechproj" {
		t.Errorf("TrimSourceExtension не должен трогать чужое расширение, получено %q", got)
	}
}
