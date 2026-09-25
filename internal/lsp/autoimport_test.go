package lsp

import (
	"testing"
)

// TestAutoImportOnSave - при сохранении добавляются импорты всех имён,
// объявленных ровно в одном файле проекта (даже если их несколько - анализ
// останавливается на первой ошибке, импорты собираются по одному), а имя,
// объявленное в двух файлах, не импортируется.
func TestAutoImportOnSave(t *testing.T) {
	dir := t.TempDir()
	writeMultiFileSource(t, dir, "точка.rech", "публичная сущность Точка\n{\n    имеет:\n        публичный x: Целое\n}\n")
	writeMultiFileSource(t, dir, "сумма.rech", "публичная функция сумма(а: Целое, б: Целое): Целое\n{\n    вернуть а + б\n}\n")
	writeMultiFileSource(t, dir, "один.rech", "публичная функция двойная(): Целое\n{\n    вернуть 1\n}\n")
	writeMultiFileSource(t, dir, "два.rech", "публичная функция двойная(): Целое\n{\n    вернуть 2\n}\n")
	src := `функция главная
{
    Точка т = новая Точка()
    вывести сумма(т.x, 1)
}
`
	mainPath := writeMultiFileSource(t, dir, "главный.rech", src)
	d := newDoc(fileURI(mainPath), src)
	line, text := d.autoImportEdit()
	want := "использовать Точка из \"точка\"\nиспользовать сумма из \"сумма\"\n"
	if line != 1 || text != want {
		t.Fatalf("ожидалась вставка %q на строке 1, получено %q на строке %d", want, text, line)
	}

	ambiguous := "функция главная\n{\n    вывести двойная()\n}\n"
	mainPath = writeMultiFileSource(t, dir, "главный.rech", ambiguous)
	if _, text := newDoc(fileURI(mainPath), ambiguous).autoImportEdit(); text != "" {
		t.Fatalf("имя из двух файлов не должно импортироваться само, получено %q", text)
	}
}
