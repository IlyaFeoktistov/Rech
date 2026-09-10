package module

import (
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"github.com/IlyaFeoktistov/Rech/internal/ast"
	"github.com/IlyaFeoktistov/Rech/internal/lexer"
	"github.com/IlyaFeoktistov/Rech/internal/parser"
)

// testFrontend - минимальный Frontend для тестов этого пакета (лексер +
// парсер, как internal/compiler.defaultFrontend) - не переиспользует
// internal/compiler, чтобы не завести цикл импортов (compiler зависит от
// module, не наоборот).
type testFrontend struct{}

func (testFrontend) Parse(src string) (*ast.Program, error) {
	toks, err := lexer.New(src).Tokenize()
	if err != nil {
		return nil, err
	}
	return parser.New(toks).ParseProgram()
}

func writeFile(t *testing.T, dir, name, content string) string {
	t.Helper()
	path := filepath.Join(dir, name)
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatalf("write: %v", err)
	}
	return path
}

func TestQualifier(t *testing.T) {
	cases := map[string]string{
		"коды_ответов": "КодыОтветов",
		"утилиты":      "Утилиты",
		"http":         "Http",
		"а_б_в":        "АБВ",
	}
	for in, want := range cases {
		if got := Qualifier(in); got != want {
			t.Errorf("Qualifier(%q) = %q, хотели %q", in, got, want)
		}
	}
}

func TestValidSegmentName(t *testing.T) {
	if err := ValidSegmentName("коды_ответов"); err != nil {
		t.Errorf("неожиданная ошибка: %v", err)
	}
	for _, bad := range []string{"", "КодыОтветов", "коды-ответов", "..", "коды.ответов"} {
		if err := ValidSegmentName(bad); err == nil {
			t.Errorf("ValidSegmentName(%q) должна была вернуть ошибку", bad)
		}
	}
}

func TestLoadResolvesNestedPath(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, dir, "утилиты/http/коды_ответов.rech", "функция взять(): Целое\n{\n    вернуть 200\n}\n")
	entry := writeFile(t, dir, "главный.rech", "использовать взять из \"утилиты/http/коды_ответов\"\n\nфункция главная\n{\n    вывести взять()\n}\n")

	prog, err := Load(entry, testFrontend{})
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if len(prog.Modules) != 2 {
		t.Fatalf("ожидалось 2 модуля, получено %d", len(prog.Modules))
	}
	if prog.Entry.AST.Functions[0].Name != "главная" {
		t.Fatalf("неверный входной модуль")
	}
}

func TestLoadMissingImportedName(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, dir, "утилиты.rech", "функция сложить(a: Целое, b: Целое): Целое\n{\n    вернуть a + b\n}\n")
	entry := writeFile(t, dir, "главный.rech", "использовать вычесть из \"утилиты\"\n\nфункция главная\n{\n    вывести 1\n}\n")

	if _, err := Load(entry, testFrontend{}); err == nil {
		t.Fatal("ожидалась ошибка - \"вычесть\" не объявлено в \"утилиты\"")
	}
}

// TestLoadRejectsImportOfPrivateFunction - "приватная функция" не видна для
// "использовать" из другого файла (docs/SPEC.md, "Импорты") - отдельная,
// более точная ошибка, чем "не объявлено" (имя объявлено, но приватно).
func TestLoadRejectsImportOfPrivateFunction(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, dir, "утилиты.rech", "приватная функция помощник(): Целое\n{\n    вернуть 1\n}\n")
	entry := writeFile(t, dir, "главный.rech", "использовать помощник из \"утилиты\"\n\nфункция главная\n{\n    вывести помощник()\n}\n")

	_, err := Load(entry, testFrontend{})
	if err == nil {
		t.Fatal("ожидалась ошибка - \"помощник\" приватная")
	}
	if !strings.Contains(err.Error(), "приватное") {
		t.Fatalf("неожиданный текст ошибки: %v", err)
	}
}

// TestLoadRejectsImportOfPrivateEntity - то же самое для "приватная
// сущность"/"приватный класс"/"приватная структура".
func TestLoadRejectsImportOfPrivateEntity(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, dir, "утилиты.rech", "приватная сущность Внутренний\n{\n    имеет:\n        x: Целое\n}\n")
	entry := writeFile(t, dir, "главный.rech", "использовать Внутренний из \"утилиты\"\n\nфункция главная\n{\n    вывести 1\n}\n")

	_, err := Load(entry, testFrontend{})
	if err == nil {
		t.Fatal("ожидалась ошибка - \"Внутренний\" приватная")
	}
	if !strings.Contains(err.Error(), "приватное") {
		t.Fatalf("неожиданный текст ошибки: %v", err)
	}
}

// TestLoadAllowsLocalUseOfPrivateFunction - приватность действует только
// на "использовать" ИЗ ДРУГОГО файла - вызов внутри собственного файла не
// ограничен вообще.
func TestLoadAllowsLocalUseOfPrivateFunction(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, dir, "утилиты.rech", "приватная функция помощник(): Целое\n{\n    вернуть 100\n}\nфункция удвоить(x: Целое): Целое\n{\n    вернуть x * 2 + помощник() - помощник()\n}\n")
	entry := writeFile(t, dir, "главный.rech", "использовать удвоить из \"утилиты\"\n\nфункция главная\n{\n    вывести удвоить(21)\n}\n")

	prog, err := Load(entry, testFrontend{})
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if _, err := prog.Link(); err != nil {
		t.Fatalf("Link: %v", err)
	}
}

func TestLoadDetectsCycle(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, dir, "а.rech", "использовать б из \"б\"\nфункция а() { вывести 1 }\n")
	entry := writeFile(t, dir, "б.rech", "использовать а из \"а\"\nфункция б() { вывести 2 }\n")

	if _, err := Load(entry, testFrontend{}); err == nil {
		t.Fatal("ожидалась ошибка циклического импорта")
	} else if !strings.Contains(err.Error(), "циклический импорт") {
		t.Fatalf("неожиданный текст ошибки: %v", err)
	}
}

func TestLinkRejectsUnimportedReference(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, dir, "утилиты.rech", "функция сложить(a: Целое, b: Целое): Целое\n{\n    вернуть a + b\n}\nфункция вычесть(a: Целое, b: Целое): Целое\n{\n    вернуть a - b\n}\n")
	entry := writeFile(t, dir, "главный.rech", "использовать сложить из \"утилиты\"\n\nфункция главная\n{\n    вывести вычесть(5, 2)\n}\n")

	prog, err := Load(entry, testFrontend{})
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if _, err := prog.Link(); err == nil {
		t.Fatal("ожидалась ошибка видимости - \"вычесть\" не импортирована")
	}
}

func TestLinkManglesNonEntryDeclarations(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, dir, "утилиты.rech", "функция взять(): Целое\n{\n    вернуть 42\n}\n")
	entry := writeFile(t, dir, "главный.rech", "использовать взять из \"утилиты\"\n\nфункция главная\n{\n    вывести взять()\n}\n")

	prog, err := Load(entry, testFrontend{})
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	merged, err := prog.Link()
	if err != nil {
		t.Fatalf("Link: %v", err)
	}
	if len(merged.Functions) != 2 {
		t.Fatalf("ожидалось 2 функции в слинкованной программе, получено %d", len(merged.Functions))
	}
	var names []string
	for _, fn := range merged.Functions {
		names = append(names, fn.Name)
	}
	if !contains(names, "главная") {
		t.Errorf("входной модуль не должен переименовываться, получено %v", names)
	}
	if !contains(names, "Утилиты_взять") {
		t.Errorf("ожидалось переименованное \"Утилиты_взять\", получено %v", names)
	}
	// Вызов внутри "главная" должен ссылаться на переименованное имя, иначе
	// cbackend/semantic не найдут функцию в слинкованной программе.
	var main *ast.FunctionDecl
	for _, fn := range merged.Functions {
		if fn.Name == "главная" {
			main = fn
		}
	}
	if main == nil {
		t.Fatalf("не нашлось \"главная\" среди %v", names)
	}
	call := main.Body[0].(*ast.PrintStmt).Value.(*ast.CallExpr)
	if call.Callee != "Утилиты_взять" {
		t.Errorf("вызов не переписан: %q", call.Callee)
	}
}

func contains(ss []string, s string) bool {
	return slices.Contains(ss, s)
}

// TestLinkPreservesEnums - регрессионный тест: Program.Link строит merged
// *ast.Program явным перечислением полей исходного (Functions/Entities/
// ExternFuncs/Enums) - "перечисление" (шаг 12) было добавлено в
// ast.Program позже остальных и изначально не попало в этот список, из-за
// чего единственный файл с "перечисление" (без единого "использовать")
// всё равно проходил через module.Load+Link (рекурсивный парсинг делает
// это независимо от того, есть ли реальные импорты) и терял свои
// перечисления молча - см. docs/ROADMAP.md, Шаг 12.
func TestLinkPreservesEnums(t *testing.T) {
	dir := t.TempDir()
	entry := writeFile(t, dir, "главный.rech", "перечисление Цвет { Красный, Зелёный }\nфункция главная { вывести Цвет.Зелёный }\n")

	prog, err := Load(entry, testFrontend{})
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	merged, err := prog.Link()
	if err != nil {
		t.Fatalf("Link: %v", err)
	}
	if len(merged.Enums) != 1 || merged.Enums[0].Name != "Цвет" {
		t.Fatalf("ожидалось перечисление %q в слинкованной программе, получено: %+v", "Цвет", merged.Enums)
	}
}

// TestLinkResolvesInterfaceTypeInNonEntryModule - найденный баг: имя
// интерфейса, объявленного в НЕ-entry файле, раньше резолвилось внутри
// самого этого файла (ссылка на него как тип параметра/поля/результата) в
// СМАНГЛЕННОЕ имя ("Файл_Интерфейс" - localTypeNames включает интерфейсы
// наравне с функциями/сущностями, buildNames раньше манглила их все
// одинаково), а слияние (Link) переносило сам интерфейс БЕЗ переименования
// (тем же принципом, что и Enums) - несовпадение ломало любую ссылку на
// интерфейс как тип внутри НЕ-entry файла ("неизвестный тип" у cbackend),
// хотя тот же код компилировался бы без единого "использовать" вообще.
func TestLinkResolvesInterfaceTypeInNonEntryModule(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, dir, "звук.rech", `интерфейс Звучащий
{
    издать_звук(): Строка
}

сущность Кошка реализует Звучащий
{
    издать_звук(): Строка
    {
        вернуть "мяу"
    }
}

функция озвучить(х: Звучащий): Строка
{
    вернуть х.издать_звук()
}
`)
	entry := writeFile(t, dir, "главный.rech", "использовать Кошка, озвучить из \"звук\"\n\nфункция главная\n{\n    вывести озвучить(новая Кошка())\n}\n")

	prog, err := Load(entry, testFrontend{})
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	merged, err := prog.Link()
	if err != nil {
		t.Fatalf("Link: %v", err)
	}
	if len(merged.Interfaces) != 1 || merged.Interfaces[0].Name != "Звучащий" {
		t.Fatalf("ожидался интерфейс %q в слинкованной программе, получено: %+v", "Звучащий", merged.Interfaces)
	}
	var found bool
	for _, fn := range merged.Functions {
		if fn.Name == "Звук_озвучить" {
			found = true
			if len(fn.Params) != 1 || fn.Params[0].Type != "Звучащий" {
				t.Fatalf("ожидался параметр типа %q, получено: %+v", "Звучащий", fn.Params)
			}
		}
	}
	if !found {
		t.Fatalf("не найдена смангленная функция \"Звук_озвучить\", получено: %+v", merged.Functions)
	}
}

// TestLinkResolvesInterfaceMethodEntityParamInNonEntryModule - НАЙДЕННЫЙ
// БАГ (исправлен здесь, docs/ROADMAP.md - "examples/проекты - показать
// полноценное ООП на Rech"): методы m.AST.Interfaces никогда не проходили
// через rewriteParams/rewriteReturnType вообще - в отличие от
// TestLinkResolvesInterfaceTypeInNonEntryModule выше (тот про ИМЯ самого
// интерфейса, не про типы ЕГО методов), ссылка на сущность из НЕ-entry
// модуля внутри сигнатуры метода интерфейса оставалась написанной
// исходным, ещё не смангленным именем - "неизвестный тип" у semantic, даже
// если сущность объявлена и видна через "использовать" в том же файле, что
// и сам интерфейс.
func TestLinkResolvesInterfaceMethodEntityParamInNonEntryModule(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, dir, "модель.rech", `интерфейс Наблюдатель
{
    на_событие(событие: Событие): Пусто
}

структура Событие
{
    имеет:
        сообщение: Строка
}
`)
	entry := writeFile(t, dir, "главный.rech", "использовать Наблюдатель, Событие из \"модель\"\n\nфункция главная\n{\n    вывести \"ок\"\n}\n")

	prog, err := Load(entry, testFrontend{})
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	merged, err := prog.Link()
	if err != nil {
		t.Fatalf("Link: %v", err)
	}
	if len(merged.Interfaces) != 1 || len(merged.Interfaces[0].Methods) != 1 {
		t.Fatalf("ожидался один интерфейс с одним методом, получено: %+v", merged.Interfaces)
	}
	m := merged.Interfaces[0].Methods[0]
	wantType := "Модель_Событие"
	if len(m.Params) != 1 || m.Params[0].Type != wantType {
		t.Fatalf("ожидался параметр типа %q (смангленное имя сущности из НЕ-entry модуля), получено: %+v", wantType, m.Params)
	}
}

// TestLinkImportsInterfaceByName - найденная находка (docs/ROADMAP.md, Шаг
// 41, http_сервер): "использовать" не умело импортировать интерфейс по
// имени вовсе (declaredNames/moduleVisibility не знали про
// ast.Program.Interfaces) - ссылка на интерфейс из ДРУГОГО файла как тип
// параметра падала с "не объявлено в этом файле и не импортировано", даже
// с явным "использовать ИмяИнтерфейса из ...", которое само по себе
// отвергалось как "файл не объявляет" это имя. В отличие от
// TestLinkResolvesInterfaceTypeInNonEntryModule выше (там интерфейс
// используется КАК ТИП в ТОМ ЖЕ файле, где объявлен, импорт не нужен) -
// здесь именно entry-файл ссылается на интерфейс, объявленный в другом
// файле, и должен его импортировать явно, как обычную сущность/функцию.
func TestLinkImportsInterfaceByName(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, dir, "звук.rech", `интерфейс Звучащий
{
    издать_звук(): Строка
}

сущность Кошка реализует Звучащий
{
    издать_звук(): Строка
    {
        вернуть "мяу"
    }
}
`)
	entry := writeFile(t, dir, "главный.rech", `использовать Кошка, Звучащий из "звук"

функция озвучить(х: Звучащий): Строка
{
    вернуть х.издать_звук()
}

функция главная
{
    вывести озвучить(новая Кошка())
}
`)

	prog, err := Load(entry, testFrontend{})
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	merged, err := prog.Link()
	if err != nil {
		t.Fatalf("Link: %v", err)
	}
	if len(merged.Interfaces) != 1 || merged.Interfaces[0].Name != "Звучащий" {
		t.Fatalf("ожидался интерфейс %q в слинкованной программе, получено: %+v", "Звучащий", merged.Interfaces)
	}
	var found bool
	for _, fn := range merged.Functions {
		if fn.Name == "озвучить" {
			found = true
			if len(fn.Params) != 1 || fn.Params[0].Type != "Звучащий" {
				t.Fatalf("ожидался параметр типа %q (bare, без мангла - интерфейсы никогда не манглятся), получено: %+v", "Звучащий", fn.Params)
			}
		}
	}
	if !found {
		t.Fatalf("не найдена функция \"озвучить\", получено: %+v", merged.Functions)
	}
}

// TestLinkResolvesExplicitConstructorParamType - найденный баг: явный
// конструктор сущности (EntityDecl.Ctor) не входил в цикл Link() вообще -
// в отличие от обычных методов, его параметры/тело никогда не проходили
// rewriteParams/rewriteStmts, поэтому тип параметра конструктора,
// ссылающийся на сущность из ДРУГОГО файла, оставался несмангленным
// ("неизвестный тип" у cbackend).
func TestLinkResolvesExplicitConstructorParamType(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, dir, "внутренняя.rech", "сущность Внутренняя\n{\n    имеет:\n        значение: Целое\n}\n")
	entry := writeFile(t, dir, "главный.rech", `использовать Внутренняя из "внутренняя"

сущность Внешняя
{
    имеет:
        приватный внутр: Внутренняя

    конструктор(внутр: Внутренняя)
    {
        этот.внутр = внутр
    }
}

функция главная
{
    вывести 1
}
`)

	prog, err := Load(entry, testFrontend{})
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	merged, err := prog.Link()
	if err != nil {
		t.Fatalf("Link: %v", err)
	}
	var found bool
	for _, ent := range merged.Entities {
		if ent.Name != "Внешняя" {
			continue
		}
		found = true
		// "Внутренняя_Внутренняя" - смангленное имя ("внутренняя.rech" -
		// НЕ-entry модуль, qualifier "Внутренняя" + "_" + имя сущности) -
		// именно ЭТО раньше не происходило вообще (см. package doc теста).
		if ent.Ctor == nil || len(ent.Ctor.Params) != 1 || ent.Ctor.Params[0].Type != "Внутренняя_Внутренняя" {
			t.Fatalf("ожидался параметр конструктора типа %q, получено: %+v", "Внутренняя_Внутренняя", ent.Ctor)
		}
	}
	if !found {
		t.Fatalf("не найдена сущность \"Внешняя\", получено: %+v", merged.Entities)
	}
}

func TestLinkNoCollisionBetweenUnrelatedFiles(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, dir, "один.rech", "функция взять(): Целое\n{\n    вернуть 1\n}\n")
	writeFile(t, dir, "два.rech", "функция взять(): Целое\n{\n    вернуть 2\n}\n")
	entry := writeFile(t, dir, "главный.rech", "использовать взять из \"один\"\n\nфункция главная\n{\n    вывести взять()\n}\n")

	prog, err := Load(entry, testFrontend{})
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if _, err := prog.Link(); err != nil {
		t.Fatalf("неожиданная ошибка (два несвязанных файла не должны сталкиваться по имени): %v", err)
	}
}

func TestLoadRejectsDuplicateImportWithoutAlias(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, dir, "а.rech", "функция взять(): Целое\n{\n    вернуть 1\n}\n")
	writeFile(t, dir, "б.rech", "функция взять(): Целое\n{\n    вернуть 2\n}\n")
	entry := writeFile(t, dir, "главный.rech",
		"использовать взять из \"а\"\nиспользовать взять из \"б\"\n\nфункция главная\n{\n    вывести взять()\n}\n")

	_, err := Load(entry, testFrontend{})
	if err == nil {
		t.Fatal("ожидалась ошибка - одно и то же имя импортировано дважды без переименования")
	}
	if !strings.Contains(err.Error(), "как") {
		t.Fatalf("сообщение должно подсказывать \"как\": %v", err)
	}
}

func TestLinkRenamesOnImport(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, dir, "а.rech", "функция взять(): Целое\n{\n    вернуть 1\n}\n")
	writeFile(t, dir, "б.rech", "функция взять(): Целое\n{\n    вернуть 2\n}\n")
	entry := writeFile(t, dir, "главный.rech",
		"использовать взять как взятьА из \"а\"\nиспользовать взять как взятьБ из \"б\"\n\nфункция главная\n{\n    вывести взятьА()\n    вывести взятьБ()\n}\n")

	prog, err := Load(entry, testFrontend{})
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if _, err := prog.Link(); err != nil {
		t.Fatalf("неожиданная ошибка (разные псевдонимы не должны сталкиваться): %v", err)
	}
}

func TestLinkImportsExternFuncPreservingCNameButRenamingAlias(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, dir, "математика.rech", "внешняя функция sqrt как корень(x: Действительное): Действительное\n")
	entry := writeFile(t, dir, "главный.rech",
		"использовать корень как корень2 из \"математика\"\n\nфункция главная\n{\n    вывести корень2(9.0)\n}\n")

	loaded, err := Load(entry, testFrontend{})
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	merged, err := loaded.Link()
	if err != nil {
		t.Fatalf("Link: %v", err)
	}
	if len(merged.ExternFuncs) != 1 {
		t.Fatalf("ожидалась одна внешняя функция в слитой программе, получено: %d", len(merged.ExternFuncs))
	}
	ext := merged.ExternFuncs[0]
	if ext.CName != "sqrt" {
		t.Fatalf("настоящий C-символ не должен переименовываться при линковке, получено: %q", ext.CName)
	}
	if ext.Alias != "Математика_корень" {
		t.Fatalf("Alias импортируемого модуля переименовывается по mangledName (Qualifier_имя), получено: %q", ext.Alias)
	}
	call := merged.Functions[0].Body[0].(*ast.PrintStmt).Value.(*ast.CallExpr)
	if call.Callee != ext.Alias {
		t.Fatalf("вызов в главном файле должен резолвиться на мангл-имя внешней функции %q, получено: %q", ext.Alias, call.Callee)
	}
}

func TestLinkImportsExternEntityPreservingCNameButRenamingAlias(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, dir, "внешние.rech", "внешняя сущность CURL как Дескриптор\n")
	entry := writeFile(t, dir, "главный.rech",
		"использовать Дескриптор из \"внешние\"\n\nвнешняя функция use_h как применить(д: Дескриптор): Целое\n\nфункция главная\n{\n    вывести 1\n}\n")

	loaded, err := Load(entry, testFrontend{})
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	merged, err := loaded.Link()
	if err != nil {
		t.Fatalf("Link: %v", err)
	}
	if len(merged.ExternEntities) != 1 {
		t.Fatalf("ожидалась одна внешняя сущность в слитой программе, получено: %d", len(merged.ExternEntities))
	}
	extEnt := merged.ExternEntities[0]
	if extEnt.CName != "CURL" {
		t.Fatalf("настоящий C-символ не должен переименовываться при линковке, получено: %q", extEnt.CName)
	}
	if extEnt.Alias != "Внешние_Дескриптор" {
		t.Fatalf("Alias импортируемого модуля переименовывается по mangledName (Qualifier_имя), получено: %q", extEnt.Alias)
	}
	if merged.ExternFuncs[0].Params[0].Type != extEnt.Alias {
		t.Fatalf("тип параметра внешней функции в главном файле должен резолвиться на мангл-имя внешней сущности %q, получено: %q", extEnt.Alias, merged.ExternFuncs[0].Params[0].Type)
	}
}

// TestLinkResolvesEntityInsideGenericTypeArgument - регрессионный тест:
// resolveTypeRef раньше пропускал ЛЮБОЕ имя типа с "<" целиком нерезолвленным
// (аргумент внутри "Список<T>" мог быть только примитивом, пока
// internal/semantic не научился принимать сущность элементом коллекции,
// см. docs/ROADMAP.md, "Шаг 39") - у сущности-элемента ИЗ НЕ-entry модуля
// (mangledName переименовывает её) ссылка "Список<Товар>" оставалась бы
// "Список<Товар>" (старое, ДО переименования имя), а не "Список<Утилиты_Товар>" -
// cbackend/semantic не нашли бы такой тип в слинкованной программе.
// TestLinkResolvesEntityInsideCallableTypeArgument - регрессионный тест
// (docs/ROADMAP.md, Шаг 41/43): resolveTypeRef раньше возвращал ЛЮБУЮ
// каноническую строку типа "Вызываемый<...>Результат" НЕИЗМЕНЁННОЙ - форма
// не оканчивается на ">" целиком (Результат идёт сразу за закрывающей ">"
// без разделителя), значит не подходила под общий случай "Шаблон<...>".
// Сущность-элемент из НЕ-entry модуля (переименовывается по mangledName)
// оставалась под старым, нерезолвленным именем и внутри параметра, и внутри
// результата - "неизвестный тип" у cbackend.
func TestLinkResolvesEntityInsideCallableTypeArgument(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, dir, "утилиты.rech", `приватная сущность Товар
{
    имеет:
        имя: Строка
}

функция применить(колбэк: Вызываемый(Товар): Товар, товар: Товар): Товар
{
    вернуть колбэк(товар)
}
`)
	entry := writeFile(t, dir, "главный.rech", "использовать применить из \"утилиты\"\n\nфункция главная\n{\n}\n")

	prog, err := Load(entry, testFrontend{})
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	merged, err := prog.Link()
	if err != nil {
		t.Fatalf("Link: %v", err)
	}
	var fn *ast.FunctionDecl
	for _, f := range merged.Functions {
		if f.Name == "Утилиты_применить" {
			fn = f
		}
	}
	if fn == nil {
		t.Fatalf("не нашлось \"Утилиты_применить\" среди слинкованных функций")
	}
	want := "Вызываемый<Утилиты_Товар>Утилиты_Товар"
	if fn.Params[0].Type != want {
		t.Fatalf("ожидался тип параметра %q (сущность-элемент переименована и в параметре, и в результате), получено %q", want, fn.Params[0].Type)
	}
	if fn.ReturnType != "Утилиты_Товар" {
		t.Fatalf("ожидался тип результата \"Утилиты_Товар\", получено %q", fn.ReturnType)
	}
}

// TestLinkResolvesGenericFunctionOwnTypeParamInsideContainer -
// НАЙДЕННЫЙ РЕАЛЬНЫЙ БАГ (обнаружен по ходу работы над Шагом 43, не связан
// с "Вызываемый(...)"): "функция первый<T>(список: Список<T>): T" -
// собственный параметр типа T generic-функции, использованный ВНУТРИ
// контейнера ("Список<T>", не сам по себе) - resolveTypeRef рекурсивно
// резолвил Аргумент "T" НАПРЯМУЮ (в обход rewriter.resolveType, единственное
// место, которое сверялось с typeParams), считая его ссылкой на
// необъявленный тип - падало ДАЖЕ В ОДНОМ ФАЙЛЕ БЕЗ ЕДИНОГО
// "использовать" (module.Link() вызывается всегда, compiler.Compile),
// включая существующий example examples/обобщённые/коробка.rech - не
// поймано раньше, потому что internal/interp.
// TestRunAllExamplesMatchCompiledOutput гоняет parser+semantic напрямую,
// минуя internal/module.
func TestLinkResolvesGenericFunctionOwnTypeParamInsideContainer(t *testing.T) {
	dir := t.TempDir()
	entry := writeFile(t, dir, "главный.rech", `функция первый<T>(список: Список<T>): T
{
    вернуть список.получить(0)
}

функция главная
{
    переменная сп = новый Список<Целое>()
    сп.добавить(5)
    вывести первый<Целое>(сп)
}
`)

	prog, err := Load(entry, testFrontend{})
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	merged, err := prog.Link()
	if err != nil {
		t.Fatalf("Link: %v", err)
	}
	var fn *ast.FunctionDecl
	for _, f := range merged.Functions {
		if f.Name == "первый" {
			fn = f
		}
	}
	if fn == nil {
		t.Fatalf("не нашлось \"первый\" среди слинкованных функций")
	}
	if fn.Params[0].Type != "Список<T>" {
		t.Fatalf("ожидался тип параметра \"Список<T>\" (T - собственный параметр типа, не ссылка), получено %q", fn.Params[0].Type)
	}
	if fn.ReturnType != "T" {
		t.Fatalf("ожидался тип результата \"T\", получено %q", fn.ReturnType)
	}
}

func TestLinkResolvesEntityInsideGenericTypeArgument(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, dir, "утилиты.rech", `приватная сущность Товар
{
    имеет:
        имя: Строка
}

функция получить_список(): Список<Товар>
{
    вернуть новый Список<Товар>()
}
`)
	entry := writeFile(t, dir, "главный.rech", "использовать получить_список из \"утилиты\"\n\nфункция главная\n{\n    переменная сп = получить_список()\n}\n")

	prog, err := Load(entry, testFrontend{})
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	merged, err := prog.Link()
	if err != nil {
		t.Fatalf("Link: %v", err)
	}
	var fn *ast.FunctionDecl
	for _, f := range merged.Functions {
		if f.Name == "Утилиты_получить_список" {
			fn = f
		}
	}
	if fn == nil {
		t.Fatalf("не нашлось \"Утилиты_получить_список\" среди слинкованных функций")
	}
	if fn.ReturnType != "Список<Утилиты_Товар>" {
		t.Fatalf("ожидался тип результата \"Список<Утилиты_Товар>\" (сущность-элемент переименована), получено %q", fn.ReturnType)
	}
	newExpr, ok := fn.Body[0].(*ast.ReturnStmt).Value.(*ast.NewExpr)
	if !ok || newExpr.Type != "Список<Утилиты_Товар>" {
		t.Fatalf("ожидался \"новый Список<Утилиты_Товар>()\" в теле, получено %#v", fn.Body[0])
	}
}

// TestLinkResolvesEntityInsideDictValueArgument - то же самое, что
// TestLinkResolvesEntityInsideGenericTypeArgument выше, но для "Словарь<К,
// В>" (два аргумента типа, splitTopLevelTypeArgs должна верно разделить их
// по запятой верхнего уровня, не спутав с запятой внутри вложенного "<...>").
func TestLinkResolvesEntityInsideDictValueArgument(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, dir, "утилиты.rech", `приватная сущность Товар
{
    имеет:
        имя: Строка
}

функция получить_словарь(): Словарь<Строка, Товар>
{
    вернуть новый Словарь<Строка, Товар>()
}
`)
	entry := writeFile(t, dir, "главный.rech", "использовать получить_словарь из \"утилиты\"\n\nфункция главная\n{\n    переменная сл = получить_словарь()\n}\n")

	prog, err := Load(entry, testFrontend{})
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	merged, err := prog.Link()
	if err != nil {
		t.Fatalf("Link: %v", err)
	}
	var fn *ast.FunctionDecl
	for _, f := range merged.Functions {
		if f.Name == "Утилиты_получить_словарь" {
			fn = f
		}
	}
	if fn == nil {
		t.Fatalf("не нашлось \"Утилиты_получить_словарь\" среди слинкованных функций")
	}
	if fn.ReturnType != "Словарь<Строка, Утилиты_Товар>" {
		t.Fatalf("ожидался тип результата \"Словарь<Строка, Утилиты_Товар>\", получено %q", fn.ReturnType)
	}
}

// TestLinkResolvesParentCallArgs - найденный баг (docs/ROADMAP.md, попутно
// с паритетом Enum/Const): "родитель(<арг>, ...)" (ast.ParentCallStmt) не
// входил ни в один case rewriteStmt - падал "внутренняя ошибка: неизвестная
// инструкция" при ЛЮБОМ прогоне через настоящую сборку (compiler.Compile
// всегда вызывает Link()), даже без единого "использовать" - не поймано
// раньше, потому что internal/interp.TestRunAllExamplesMatchCompiledOutput
// гоняет parser+semantic напрямую, минуя internal/module. Здесь аргумент
// "родитель(...)" - вызов функции из ДРУГОГО файла, проверяет, что
// резолюция реально происходит, не только что нет паники.
func TestLinkResolvesParentCallArgs(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, dir, "утилиты.rech", "функция значение_по_умолчанию(): Строка\n{\n    вернуть \"базовое\"\n}\n")
	entry := writeFile(t, dir, "главный.rech", `использовать значение_по_умолчанию из "утилиты"

сущность Базовый
{
    имеет:
        имя: Строка

    конструктор(имя: Строка)
    {
        этот.имя = имя
    }
}

сущность Производный наследует Базовый
{
    конструктор()
    {
        родитель(значение_по_умолчанию())
    }
}

функция главная
{
    переменная о = новый Производный()
    вывести о.имя
}
`)

	prog, err := Load(entry, testFrontend{})
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	merged, err := prog.Link()
	if err != nil {
		t.Fatalf("Link: %v", err)
	}
	var derived *ast.EntityDecl
	for _, e := range merged.Entities {
		if e.Name == "Производный" {
			derived = e
		}
	}
	if derived == nil || derived.Ctor == nil || len(derived.Ctor.Body) == 0 {
		t.Fatalf("не нашлось тело конструктора \"Производный\"")
	}
	call, ok := derived.Ctor.Body[0].(*ast.ParentCallStmt)
	if !ok {
		t.Fatalf("ожидался ParentCallStmt первой инструкцией, получено %T", derived.Ctor.Body[0])
	}
	callee, ok := call.Args[0].(*ast.CallExpr)
	if !ok || callee.Callee != "Утилиты_значение_по_умолчанию" {
		t.Fatalf("ожидался резолвленный вызов \"Утилиты_значение_по_умолчанию\" в аргументе \"родитель(...)\", получено %+v", call.Args)
	}
}

// TestLinkImportsEnumByName - паритет с уже существующей приватностью
// функций/сущностей/интерфейсов (docs/ROADMAP.md): "использовать
// ИмяПеречисления из ..." теперь разрешён, доступ к члену через "." из
// файла-импортёра резолвится (buildNames - bare passthrough, тем же
// принципом, что и у интерфейсов).
func TestLinkImportsEnumByName(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, dir, "цвета.rech", "перечисление Цвет\n{\n    Красный\n    Зелёный\n}\n")
	entry := writeFile(t, dir, "главный.rech", "использовать Цвет из \"цвета\"\n\nфункция главная\n{\n    вывести Цвет.Красный\n}\n")

	prog, err := Load(entry, testFrontend{})
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if _, err := prog.Link(); err != nil {
		t.Fatalf("Link: %v", err)
	}
}

// TestLinkRejectsUnimportedEnum - обратная сторона паритета: ссылка на
// перечисление, объявленное в другом файле программы, но НЕ
// импортированное в файле-ссылке - ошибка видимости (раньше молча
// резолвилось - единая плоская область видимости без проверки).
func TestLinkRejectsUnimportedEnum(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, dir, "цвета.rech", "перечисление Цвет\n{\n    Красный\n}\n\nфункция маркер(): Целое\n{\n    вернуть 1\n}\n")
	entry := writeFile(t, dir, "главный.rech", "использовать маркер из \"цвета\"\n\nфункция главная\n{\n    маркер()\n    вывести Цвет.Красный\n}\n")

	prog, err := Load(entry, testFrontend{})
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if _, err := prog.Link(); err == nil {
		t.Fatal("ожидалась ошибка видимости - \"Цвет\" не импортирован")
	}
}

// TestLinkRejectsPrivateEnumImport - "приватное перечисление" отклоняется
// проверкой видимости "использовать", тем же путём, что и у приватной
// функции/сущности (module.go, moduleVisibility).
func TestLinkRejectsPrivateEnumImport(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, dir, "цвета.rech", "приватное перечисление Скрытый\n{\n    А\n}\n")
	entry := writeFile(t, dir, "главный.rech", "использовать Скрытый из \"цвета\"\n\nфункция главная\n{\n}\n")

	if _, err := Load(entry, testFrontend{}); err == nil {
		t.Fatal("ожидалась ошибка - \"Скрытый\" приватное")
	} else if !strings.Contains(err.Error(), "приватное") {
		t.Fatalf("неожиданный текст ошибки: %v", err)
	}
}

// TestLinkImportsConstByName/TestLinkRejectsUnimportedConst - те же две
// проверки, что и у Enum выше, для константы уровня модуля.
func TestLinkImportsConstByName(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, dir, "настройки.rech", "константа Порог = 10\n")
	entry := writeFile(t, dir, "главный.rech", "использовать Порог из \"настройки\"\n\nфункция главная\n{\n    вывести Порог\n}\n")

	prog, err := Load(entry, testFrontend{})
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if _, err := prog.Link(); err != nil {
		t.Fatalf("Link: %v", err)
	}
}

func TestLinkRejectsUnimportedConst(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, dir, "настройки.rech", "константа Порог = 10\n\nфункция маркер(): Целое\n{\n    вернуть 1\n}\n")
	entry := writeFile(t, dir, "главный.rech", "использовать маркер из \"настройки\"\n\nфункция главная\n{\n    маркер()\n    вывести Порог\n}\n")

	prog, err := Load(entry, testFrontend{})
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if _, err := prog.Link(); err == nil {
		t.Fatal("ожидалась ошибка видимости - \"Порог\" не импортирован")
	}
}
