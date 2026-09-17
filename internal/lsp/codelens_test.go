package lsp

import "testing"

func TestRunTestCodeLensOnTestFile(t *testing.T) {
	dir := t.TempDir()
	src := `
функция главная
{
    утверждать 1 + 1 == 2
}
`
	path := writeMultiFileSource(t, dir, "сложение_тест.rech", src)
	d := newDoc(fileURI(path), src)

	lenses := d.codeLenses()
	var found *CodeLens
	for i := range lenses {
		if lenses[i].Command == runTestCommand {
			found = &lenses[i]
		}
	}
	if found == nil {
		t.Fatalf("ожидался CodeLens \"запустить тест\", получено %+v", lenses)
	}
	if found.Title != "▶ запустить тест" {
		t.Fatalf("неожиданный заголовок: %q", found.Title)
	}
	if len(found.Args) != 1 || found.Args[0] != path {
		t.Fatalf("ожидался единственный аргумент - путь файла %q, получено %+v", path, found.Args)
	}
	if found.Line != lineOf(t, src, "функция главная") {
		t.Fatalf("ожидалась строка объявления \"функция главная\", получено %d", found.Line)
	}
}

func TestRunTestCodeLensAbsentOnRegularFile(t *testing.T) {
	dir := t.TempDir()
	src := `
функция главная
{
    вывести "привет"
}
`
	path := writeMultiFileSource(t, dir, "главный.rech", src)
	d := newDoc(fileURI(path), src)

	for _, cl := range d.codeLenses() {
		if cl.Command == runTestCommand {
			t.Fatalf("не ожидался CodeLens \"запустить тест\" на обычном файле: %+v", cl)
		}
	}
}

func TestRunTestCodeLensAbsentWithoutMainFunc(t *testing.T) {
	dir := t.TempDir()
	src := `
функция помощник(): Целое
{
    вернуть 1
}
`
	path := writeMultiFileSource(t, dir, "без_главной_тест.rech", src)
	d := newDoc(fileURI(path), src)

	for _, cl := range d.codeLenses() {
		if cl.Command == runTestCommand {
			t.Fatalf("не ожидался CodeLens \"запустить тест\" без \"функция главная\": %+v", cl)
		}
	}
}
