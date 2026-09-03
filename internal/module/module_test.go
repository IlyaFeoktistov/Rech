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
