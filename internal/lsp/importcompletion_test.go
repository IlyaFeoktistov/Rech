package lsp

import (
	"sort"
	"testing"
)

func TestImportPathCompletionListsProjectFiles(t *testing.T) {
	dir := t.TempDir()
	writeMultiFileSource(t, dir, "логгер.rech", multiFileLoggerSrc)
	writeMultiFileSource(t, dir, "строки.rech", `функция главная
{
}
`)
	src := `использовать Логгер из ""

функция главная
{
}
`
	mainPath := writeMultiFileSource(t, dir, "главный.rech", src)

	d := newDoc(fileURI(mainPath), src)
	line := lineOf(t, src, "использовать Логгер из")
	col := runeCol(t, `использовать Логгер из ""`, `""`) + 1 // сразу после открывающей кавычки

	items := d.completion(line, col)
	if len(items) == 0 {
		t.Fatalf("ожидался непустой список файлов проекта")
	}
	var labels []string
	for _, it := range items {
		if it.Kind != kindFile {
			t.Fatalf("ожидался Kind == kindFile у всех элементов, получено %+v", it)
		}
		labels = append(labels, it.Label)
	}
	sort.Strings(labels)
	want := []string{"логгер", "строки"}
	sort.Strings(want)
	if len(labels) != len(want) {
		t.Fatalf("ожидались файлы %v, получено %v", want, labels)
	}
	for i := range want {
		if labels[i] != want[i] {
			t.Fatalf("ожидались файлы %v, получено %v", want, labels)
		}
	}
}

func TestImportPathCompletionOnlyInsideImportString(t *testing.T) {
	dir := t.TempDir()
	writeMultiFileSource(t, dir, "логгер.rech", multiFileLoggerSrc)
	src := `функция главная
{
    переменная с = "привет"
}
`
	mainPath := writeMultiFileSource(t, dir, "главный.rech", src)
	d := newDoc(fileURI(mainPath), src)

	line := lineOf(t, src, `"привет"`)
	col := runeCol(t, `    переменная с = "привет"`, `"привет"`) + 1

	items := d.completion(line, col)
	for _, it := range items {
		if it.Kind == kindFile {
			t.Fatalf("обычный строковый литерал не должен предлагать файлы проекта: %+v", items)
		}
	}
}
