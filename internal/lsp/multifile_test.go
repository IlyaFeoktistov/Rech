package lsp

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func writeMultiFileSource(t *testing.T, dir, name, src string) string {
	t.Helper()
	path := filepath.Join(dir, name)
	if err := os.WriteFile(path, []byte(src), 0o644); err != nil {
		t.Fatalf("не удалось записать исходник: %v", err)
	}
	return path
}

const multiFileLoggerSrc = `
интерфейс Логгер
{
    сообщить(текст: Строка)
}

сущность КонсольныйЛоггер реализует Логгер
{
    сообщить(текст: Строка)
    {
        вывести текст
    }
}
`

const multiFileMainSrc = `
использовать Логгер, КонсольныйЛоггер из "логгер"

функция главная
{
    переменная л = новый КонсольныйЛоггер()
    л.сообщить("привет")
}
`

// TestMultiFileDefinitionJumpsIntoOtherFile - "go to definition" на имени
// "КонсольныйЛоггер" (написанном в главном файле, объявленном в
// "логгер.rech") - без многофайлового обогащения (multifile.go) semantic.
// Analyze одного главного файла падает целиком ("неизвестный тип
// Логгер"), d.irProg остаётся nil, definition() не находит вообще ничего -
// именно этот случай найден на реальном examples/проекты/http_сервер.
func TestMultiFileDefinitionJumpsIntoOtherFile(t *testing.T) {
	dir := t.TempDir()
	writeMultiFileSource(t, dir, "логгер.rech", multiFileLoggerSrc)
	mainPath := writeMultiFileSource(t, dir, "главный.rech", multiFileMainSrc)

	d := newDoc(fileURI(mainPath), multiFileMainSrc)
	if d.irProg == nil {
		t.Fatalf("ожидался успешный многофайловый разбор (irProg == nil)")
	}

	line := lineOf(t, multiFileMainSrc, "новый КонсольныйЛоггер()")
	col := runeCol(t, "    переменная л = новый КонсольныйЛоггер()", "КонсольныйЛоггер")
	pos, ok := d.definition(line, col)
	if !ok {
		t.Fatalf("ожидалось определение \"КонсольныйЛоггер\" в другом файле")
	}
	wantURI := fileURI(filepath.Join(dir, "логгер.rech"))
	if pos.URI != wantURI {
		t.Fatalf("ожидался URI %q, получено %q", wantURI, pos.URI)
	}
	wantLine := lineOf(t, multiFileLoggerSrc, "сущность КонсольныйЛоггер")
	if pos.Line != wantLine {
		t.Fatalf("ожидалась строка %d в логгер.rech, получено %d", wantLine, pos.Line)
	}
}

// TestMultiFileImplementationJumpsIntoOtherFile - "go to implementation" на
// имени интерфейса "Логгер" (импортированного, объявленного в другом
// файле) - тот же принцип, что и у definition выше.
func TestMultiFileImplementationJumpsIntoOtherFile(t *testing.T) {
	dir := t.TempDir()
	writeMultiFileSource(t, dir, "логгер.rech", multiFileLoggerSrc)
	mainPath := writeMultiFileSource(t, dir, "главный.rech", multiFileMainSrc)

	d := newDoc(fileURI(mainPath), multiFileMainSrc)
	if d.irProg == nil {
		t.Fatalf("ожидался успешный многофайловый разбор (irProg == nil)")
	}

	line := lineOf(t, multiFileMainSrc, "использовать Логгер")
	col := runeCol(t, "использовать Логгер, КонсольныйЛоггер из \"логгер\"", "Логгер")
	positions, ok := d.implementations(line, col)
	if !ok || len(positions) != 1 {
		t.Fatalf("ожидалась ровно 1 реализация интерфейса \"Логгер\", получено %+v (ok=%v)", positions, ok)
	}
	wantURI := fileURI(filepath.Join(dir, "логгер.rech"))
	if positions[0].URI != wantURI {
		t.Fatalf("ожидался URI %q, получено %q", wantURI, positions[0].URI)
	}
}

// TestMultiFileHoverShowsDisplayNameNotMangled - тип переменной "л"
// ("КонсольныйЛоггер", объявленной в ДРУГОМ файле) должен показываться в
// hover ТЕМ ЖЕ именем, каким его написал главный файл в своём "использовать"
// - не внутренним смангленным именем слитой программы (module.Program.Link,
// см. display.go) - живая проверка на examples/проекты/http_сервер нашла
// именно это: "контейнер: Контейнер_Контейнер" вместо "контейнер: Контейнер".
func TestMultiFileHoverShowsDisplayNameNotMangled(t *testing.T) {
	dir := t.TempDir()
	writeMultiFileSource(t, dir, "логгер.rech", multiFileLoggerSrc)
	mainPath := writeMultiFileSource(t, dir, "главный.rech", multiFileMainSrc)

	d := newDoc(fileURI(mainPath), multiFileMainSrc)
	if d.irProg == nil {
		t.Fatalf("ожидался успешный многофайловый разбор (irProg == nil)")
	}

	line := lineOf(t, multiFileMainSrc, "л.сообщить(\"привет\")")
	col := runeCol(t, "    л.сообщить(\"привет\")", "л")
	text, ok := d.hover(line, col)
	if !ok {
		t.Fatalf("ожидался hover переменной \"л\"")
	}
	if !strings.Contains(text, "л: КонсольныйЛоггер") {
		t.Fatalf("ожидалось \"л: КонсольныйЛоггер\" (без манглинга), получено:\n%s", text)
	}
	if strings.Contains(text, "_") {
		t.Fatalf("hover не должен содержать внутреннее смангленное имя, получено:\n%s", text)
	}
}

// TestMultiFileCompletionExcludesManglesNames - общее автодополнение не
// должно предлагать внутренние смангленные имена сущностей/функций из
// ДРУГИХ файлов (см. package doc document.go, localNames) - только
// собственные объявления и алиасы, под которыми они реально написаны в
// главном файле.
func TestMultiFileCompletionExcludesManglesNames(t *testing.T) {
	dir := t.TempDir()
	writeMultiFileSource(t, dir, "логгер.rech", multiFileLoggerSrc)
	mainPath := writeMultiFileSource(t, dir, "главный.rech", multiFileMainSrc)

	d := newDoc(fileURI(mainPath), multiFileMainSrc)
	if d.irProg == nil {
		t.Fatalf("ожидался успешный многофайловый разбор (irProg == nil)")
	}

	line := lineOf(t, multiFileMainSrc, "переменная л")
	items := d.completion(line, 4)
	foundAlias := false
	for _, it := range items {
		if strings.Contains(it.Label, "_") {
			t.Fatalf("автодополнение не должно содержать смангленное имя, получено %q", it.Label)
		}
		if it.Label == "КонсольныйЛоггер" {
			foundAlias = true
		}
	}
	if !foundAlias {
		t.Fatalf("ожидалось \"КонсольныйЛоггер\" в автодополнении, получено %+v", items)
	}
}
