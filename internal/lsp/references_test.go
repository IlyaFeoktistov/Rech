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

// TestCodeLensCountsGenericFunctionCall - "имя<Тип>(...)" (явный аргумент
// типа generic-вызова) - тоже вызов, должен считаться "N ссылок" - раньше
// matchFunctionRefs требовал "(" СРАЗУ за именем, а после generic-имени
// сразу идёт "<", не "(" - вызов вообще не находился, "N ссылок" всегда
// показывало 0 (репортнуто пользователем, examples/бд/чанкование.rech -
// "чанковать_по_полю<T>", вызывается дважды в том же файле).
func TestCodeLensCountsGenericFunctionCall(t *testing.T) {
	src := `функция обернуть<T>(значение: T): T
{
    вернуть значение
}

функция главная
{
    вывести обернуть<Целое>(1)
    вывести обернуть<Целое>(2)
}
`
	d := newDoc("test://a.rech", src)
	lenses := d.codeLenses()
	line := lineOf(t, src, "функция обернуть")
	cl := findLens(t, lenses, line)
	if cl.Title != "2 ссылки" {
		t.Fatalf("ожидалось \"2 ссылки\" на generic-вызов, получено %q (Refs=%+v)", cl.Title, cl.Refs)
	}
}

// TestCodeLensCountsModuleConstReference - модульная константа тоже
// получает "N ссылок" (раньше codelens.go для неё вообще не строил
// CodeLens - репортнуто пользователем).
func TestCodeLensCountsModuleConstReference(t *testing.T) {
	src := `константа МАКС = 3

функция главная
{
    вывести МАКС
    вывести МАКС
}
`
	d := newDoc("test://a.rech", src)
	lenses := d.codeLenses()
	line := lineOf(t, src, "константа МАКС")
	cl := findLens(t, lenses, line)
	if cl.Title != "2 ссылки" {
		t.Fatalf("ожидалось \"2 ссылки\" на модульную константу, получено %q (Refs=%+v)", cl.Title, cl.Refs)
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
	const authSrc = `использовать создать_токен из "../токен/токен"

функция авторизовать(имя: Строка): Строка
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

// TestCodeLensDoesNotCrossUnrelatedNeighboringProjects - репортнуто
// пользователем: если рядом на диске лежат ДВА НЕЗАВИСИМЫХ проекта (ничего
// друг у друга не импортируют), под ОДНИМ общим .rechproj-деревом (типичная
// раскладка - `examples/` основного репозитория Rech: множество
// подкаталогов-примеров без СОБСТВЕННОГО .rechproj каждый, scanRoot
// (workspace_scan.go) поднимается до ближайшего .rechproj ВЫШЕ них обоих),
// и оба объявляют функцию с ОДНИМ И ТЕМ ЖЕ именем - "N ссылок" в проекте A
// не должно учитывать текстовое совпадение имени в проекте Б (см. package
// doc multifile.go/fileImportsTarget - раньше считалось ЛЮБОЕ совпадение
// имени под общим каталогом-предком, независимо от реального
// "использовать").
func TestCodeLensDoesNotCrossUnrelatedNeighboringProjects(t *testing.T) {
	root := t.TempDir()
	projA := filepath.Join(root, "проект_а")
	projB := filepath.Join(root, "проект_б")
	if err := os.MkdirAll(projA, 0o755); err != nil {
		t.Fatalf("не удалось создать каталог: %v", err)
	}
	if err := os.MkdirAll(projB, 0o755); err != nil {
		t.Fatalf("не удалось создать каталог: %v", err)
	}
	// Один .rechproj в ОБЩЕМ корне (НЕ в каждом подпроекте по отдельности) -
	// та же раскладка, что и у настоящего examples/ основного репозитория.
	if err := os.WriteFile(filepath.Join(root, "тест.rechproj"), []byte("проект:\n  имя: тест\nфайлы:\n  главный: проект_а/главный.rech\n"), 0o644); err != nil {
		t.Fatalf("не удалось записать .rechproj: %v", err)
	}
	const srcA = `функция обработать(имя: Строка): Строка
{
    вернуть имя
}
`
	// Проект Б - СВОЁ, НИЧЕМ не связанное с А объявление "обработать" (то же
	// имя, чистое совпадение) плюс настоящий вызов ЕГО СОБСТВЕННОЙ функции -
	// чтобы отличить "0 ссылок в самом проекте Б" (сломанный тест) от "не
	// подхватило чужой проект А" (то, что здесь проверяется).
	const srcB = `функция обработать(имя: Строка): Строка
{
    вернуть имя
}

функция главная
{
    вывести обработать("б")
}
`
	pathA := writeMultiFileSource(t, projA, "главный.rech", srcA)
	writeMultiFileSource(t, projB, "главный.rech", srcB)

	d := newDoc(fileURI(pathA), srcA)
	lenses := d.codeLenses()
	line := lineOf(t, srcA, "функция обработать")
	cl := findLens(t, lenses, line)
	if cl.Title != "0 ссылок" {
		t.Fatalf("ожидалось \"0 ссылок\" (проект Б не связан с А) - получено %q, Refs=%+v", cl.Title, cl.Refs)
	}
}
