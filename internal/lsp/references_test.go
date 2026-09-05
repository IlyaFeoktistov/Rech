package lsp

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// findLens - CodeLens (codelens.go) на строке line - t.Fatalf, если её нет
// (каждое собственное объявление документа обязано получить ровно один
// CodeLens, см. package doc codelens.go).
func findLens(t *testing.T, lenses []CodeLens, line int) CodeLens {
	t.Helper()
	for _, cl := range lenses {
		if cl.Line == line {
			return cl
		}
	}
	t.Fatalf("не найден CodeLens на строке %d, получено: %+v", line, lenses)
	return CodeLens{}
}

// TestCodeLensShowsReferenceCountForFunction - "сложить" вызывается ровно
// один раз в hoverTestSrc ("вывести сложить(1, 2)"), не считая
// собственного объявления.
func TestCodeLensShowsReferenceCountForFunction(t *testing.T) {
	d := newDoc("test://a.rech", hoverTestSrc)
	lenses := d.codeLenses()
	line := lineOf(t, hoverTestSrc, "функция сложить")
	cl := findLens(t, lenses, line)
	if cl.Title != "1 ссылка" {
		t.Fatalf("ожидалась \"1 ссылка\" на \"сложить\", получено: %q", cl.Title)
	}
	if len(cl.Refs) != 1 {
		t.Fatalf("ожидалась ровно 1 позиция ссылки, получено: %+v", cl.Refs)
	}
}

// TestCodeLensShowsReferenceCountForMethod - "длина" вызывается один раз
// ("т.длина()").
func TestCodeLensShowsReferenceCountForMethod(t *testing.T) {
	d := newDoc("test://a.rech", hoverTestSrc)
	lenses := d.codeLenses()
	line := lineOf(t, hoverTestSrc, "длина(): Целое")
	cl := findLens(t, lenses, line)
	if cl.Title != "1 ссылка" {
		t.Fatalf("ожидалась \"1 ссылка\" на \"длина\", получено: %q", cl.Title)
	}
}

// TestCodeLensShowsReferenceCountForField - поле "x" встречается дважды:
// "т.x" в главная и "этот.x" внутри метода длина.
func TestCodeLensShowsReferenceCountForField(t *testing.T) {
	d := newDoc("test://a.rech", hoverTestSrc)
	lenses := d.codeLenses()
	line := lineOf(t, hoverTestSrc, "x: Целое")
	cl := findLens(t, lenses, line)
	if cl.Title != "3 ссылки" {
		t.Fatalf("ожидались \"3 ссылки\" на \"x\" (включая присваивание в конструкторе), получено: %q", cl.Title)
	}
}

// TestCodeLensShowsReferenceCountForEntity - "Точка" создаётся один раз
// ("новая Точка(1, 2)").
func TestCodeLensShowsReferenceCountForEntity(t *testing.T) {
	d := newDoc("test://a.rech", hoverTestSrc)
	lenses := d.codeLenses()
	line := lineOf(t, hoverTestSrc, "сущность Точка")
	cl := findLens(t, lenses, line)
	if cl.Title != "1 ссылка" {
		t.Fatalf("ожидалась \"1 ссылка\" на \"Точка\", получено: %q", cl.Title)
	}
}

const constHoverSrc = `
сущность Точка
{
    константа ПРЕДЕЛ = 100

    имеет:
        x: Целое

    конструктор(x: Целое)
    {
        этот.x = x
    }
}
функция главная
{
    переменная т = новая Точка(1)
    вывести т.ПРЕДЕЛ
    вывести т.ПРЕДЕЛ
}
`

// TestHoverOnConstantShowsResolvedValue - "т.ПРЕДЕЛ" - hover должен
// показать разрешённое значение константы (100), не только её тип.
func TestHoverOnConstantShowsResolvedValue(t *testing.T) {
	d := newDoc("test://b.rech", constHoverSrc)
	if d.irProg == nil {
		t.Fatalf("документ не разобрался")
	}
	line := lineOf(t, constHoverSrc, "вывести т.ПРЕДЕЛ")
	col := runeCol(t, "    вывести т.ПРЕДЕЛ", "ПРЕДЕЛ")
	text, ok := d.hover(line, col)
	if !ok {
		t.Fatalf("ожидался hover на константе ПРЕДЕЛ")
	}
	if !strings.Contains(text, "Значение: `100`") {
		t.Fatalf("ожидалось значение константы в hover, получено: %q", text)
	}
}

// TestCodeLensShowsReferenceCountForConstant - "ПРЕДЕЛ" (константа
// сущности) встречается дважды.
func TestCodeLensShowsReferenceCountForConstant(t *testing.T) {
	d := newDoc("test://b.rech", constHoverSrc)
	lenses := d.codeLenses()
	line := lineOf(t, constHoverSrc, "константа ПРЕДЕЛ")
	cl := findLens(t, lenses, line)
	if cl.Title != "2 ссылки" {
		t.Fatalf("ожидались \"2 ссылки\" на \"ПРЕДЕЛ\", получено: %q", cl.Title)
	}
}

// TestCodeLensCountsReferencesFromOtherFiles - функция, объявленная в
// ОДНОМ файле дерева и вызываемая ТОЛЬКО из ДРУГОГО (обычное дело для
// многофайлового проекта, см. package doc references.go/crossFileRefs) -
// живая проверка нашла именно это на examples/проекты/http_сервер:
// "создать_токен" (объявлена в исходники/токен/токен.rech) показывала "0
// ссылок", хотя вызывается из исходники/авторизация/авторизация.rech -
// CodeLens должен считать ссылки по ВСЕМУ дереву проекта, не только по
// открытому документу.
func TestCodeLensCountsReferencesFromOtherFiles(t *testing.T) {
	dir := t.TempDir()
	tokenDir := filepath.Join(dir, "исходники", "токен")
	authDir := filepath.Join(dir, "исходники", "авторизация")
	if err := os.MkdirAll(tokenDir, 0o755); err != nil {
		t.Fatalf("не удалось создать каталог: %v", err)
	}
	if err := os.MkdirAll(authDir, 0o755); err != nil {
		t.Fatalf("не удалось создать каталог: %v", err)
	}
	// .rechproj в корне - без него scanRoot (workspace_scan.go) не поднимается
	// выше каталога САМОГО открытого документа (исходники/токен/), и
	// авторизация.rech в СОСЕДНЕМ каталоге просто не будет найден - тот же
	// принцип, что и у автоимпорта (findWorkspaceDecl).
	if err := os.WriteFile(filepath.Join(dir, "тест.rechproj"), []byte("проект:\n  имя: тест\nфайлы:\n  главный: исходники/токен/токен.rech\n"), 0o644); err != nil {
		t.Fatalf("не удалось записать .rechproj: %v", err)
	}
	const tokenSrc = `функция создать_токен(имя: Строка): Строка
{
    вернуть имя
}
`
	const authSrc = `функция авторизовать(имя: Строка): Строка
{
    вернуть создать_токен(имя)
}
`
	tokenPath := writeMultiFileSource(t, tokenDir, "токен.rech", tokenSrc)
	writeMultiFileSource(t, authDir, "авторизация.rech", authSrc)

	d := newDoc(fileURI(tokenPath), tokenSrc)
	lenses := d.codeLenses()
	line := lineOf(t, tokenSrc, "функция создать_токен")
	cl := findLens(t, lenses, line)
	if cl.Title != "1 ссылка" {
		t.Fatalf("ожидалась \"1 ссылка\" на \"создать_токен\" из другого файла, получено: %q (Refs=%+v)", cl.Title, cl.Refs)
	}
	if len(cl.Refs) != 1 || cl.Refs[0].File != filepath.Join(authDir, "авторизация.rech") {
		t.Fatalf("ожидалась 1 позиция ссылки в авторизация.rech, получено: %+v", cl.Refs)
	}
}
