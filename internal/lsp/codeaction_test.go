package lsp

import (
	"os"
	"path/filepath"
	"testing"
)

// TestCodeActionOffersAutoImportForMissingEntity - главный файл ссылается
// на "КонсольныйЛоггер" (multiFileLoggerSrc, объявлена в "логгер.rech"),
// но не пишет "использовать" вовсе - линковка (multifile.go) должна
// провалиться с "не объявлено... не импортировано", а codeActions -
// предложить вставить нужную строку.
func TestCodeActionOffersAutoImportForMissingEntity(t *testing.T) {
	dir := t.TempDir()
	writeMultiFileSource(t, dir, "логгер.rech", multiFileLoggerSrc)
	src := `
функция главная
{
    переменная л = новый КонсольныйЛоггер()
    л.сообщить("привет")
}
`
	mainPath := writeMultiFileSource(t, dir, "главный.rech", src)

	d := newDoc(fileURI(mainPath), src)
	if len(d.diagnostics) == 0 {
		t.Fatalf("ожидалась диагностика \"не объявлено... не импортировано\"")
	}

	actions := d.codeActions()
	if len(actions) != 1 {
		t.Fatalf("ожидался ровно 1 quick fix, получено %+v", actions)
	}
	if actions[0].NewText != "использовать КонсольныйЛоггер из \"логгер\"\n" {
		t.Fatalf("неожиданный текст вставки: %q", actions[0].NewText)
	}
	if actions[0].NewLine != 1 {
		t.Fatalf("ожидалась вставка на строке 1 (импортов ещё не было), получено %d", actions[0].NewLine)
	}
}

// TestCodeActionInsertsAfterExistingImports - если "использовать" уже
// есть, новая строка должна вставляться СРАЗУ ПОСЛЕ последнего из них, не
// в начало файла.
func TestCodeActionInsertsAfterExistingImports(t *testing.T) {
	dir := t.TempDir()
	writeMultiFileSource(t, dir, "логгер.rech", multiFileLoggerSrc)
	src := `использовать Логгер из "логгер"

функция применить(логгер: Логгер)
{
    переменная л = новый КонсольныйЛоггер()
    л.сообщить("привет")
}
`
	mainPath := writeMultiFileSource(t, dir, "главный.rech", src)

	d := newDoc(fileURI(mainPath), src)
	if len(d.diagnostics) == 0 {
		t.Fatalf("ожидалась диагностика \"не объявлено... не импортировано\"")
	}

	actions := d.codeActions()
	if len(actions) != 1 {
		t.Fatalf("ожидался ровно 1 quick fix, получено %+v", actions)
	}
	wantLine := lineOf(t, src, "использовать Логгер из \"логгер\"") + 1
	if actions[0].NewLine != wantLine {
		t.Fatalf("ожидалась вставка на строке %d (сразу после существующего импорта), получено %d", wantLine, actions[0].NewLine)
	}
}

func TestScanRootFindsRechprojAncestor(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "проект.rechproj"), []byte("сервер:\n  порт: 8080\n"), 0o644); err != nil {
		t.Fatalf("не удалось записать .rechproj: %v", err)
	}
	sub := filepath.Join(dir, "исходники", "вложенный")
	if err := os.MkdirAll(sub, 0o755); err != nil {
		t.Fatalf("не удалось создать каталог: %v", err)
	}
	got := scanRoot(sub)
	if got != dir {
		t.Fatalf("ожидался корень %q, получено %q", dir, got)
	}
}
