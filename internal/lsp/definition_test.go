package lsp

import (
	"net/url"
	"testing"
)

func TestDefinitionOnFunctionCallJumpsToDecl(t *testing.T) {
	d := newDoc("test://a.rech", hoverTestSrc)
	line := lineOf(t, hoverTestSrc, "вывести сложить(1, 2)")
	col := runeCol(t, "    вывести сложить(1, 2)", "сложить")
	pos, ok := d.definition(line, col)
	if !ok {
		t.Fatalf("ожидалось определение функции \"сложить\"")
	}
	wantLine := lineOf(t, hoverTestSrc, "функция сложить")
	if pos.Line != wantLine {
		t.Fatalf("ожидалась строка %d, получено %d", wantLine, pos.Line)
	}
}

func TestDefinitionOnEntityConstructionJumpsToDecl(t *testing.T) {
	d := newDoc("test://a.rech", hoverTestSrc)
	line := lineOf(t, hoverTestSrc, "новая Точка(1, 2)")
	col := runeCol(t, "    переменная т = новая Точка(1, 2)", "Точка")
	pos, ok := d.definition(line, col)
	if !ok {
		t.Fatalf("ожидалось определение сущности \"Точка\"")
	}
	wantLine := lineOf(t, hoverTestSrc, "сущность Точка")
	if pos.Line != wantLine {
		t.Fatalf("ожидалась строка %d, получено %d", wantLine, pos.Line)
	}
}

func TestDefinitionOnFieldAccessJumpsToFieldDecl(t *testing.T) {
	d := newDoc("test://a.rech", hoverTestSrc)
	line := lineOf(t, hoverTestSrc, "вывести т.x")
	col := runeCol(t, "    вывести т.x", "x")
	pos, ok := d.definition(line, col)
	if !ok {
		t.Fatalf("ожидалось определение поля \"x\"")
	}
	wantLine := lineOf(t, hoverTestSrc, "Целое x")
	if pos.Line != wantLine {
		t.Fatalf("ожидалась строка %d, получено %d", wantLine, pos.Line)
	}
}

func TestDefinitionOnMethodCallJumpsToMethodDecl(t *testing.T) {
	d := newDoc("test://a.rech", hoverTestSrc)
	line := lineOf(t, hoverTestSrc, "т.длина()")
	col := runeCol(t, "    вывести т.длина()", "длина")
	pos, ok := d.definition(line, col)
	if !ok {
		t.Fatalf("ожидалось определение метода \"длина\"")
	}
	wantLine := lineOf(t, hoverTestSrc, "длина(): Целое")
	if pos.Line != wantLine {
		t.Fatalf("ожидалась строка %d, получено %d", wantLine, pos.Line)
	}
}

func TestDefinitionOnConstantAccessJumpsToConstDecl(t *testing.T) {
	d := newDoc("test://b.rech", constHoverSrc)
	line := lineOf(t, constHoverSrc, "вывести т.ПРЕДЕЛ")
	col := runeCol(t, "    вывести т.ПРЕДЕЛ", "ПРЕДЕЛ")
	pos, ok := d.definition(line, col)
	if !ok {
		t.Fatalf("ожидалось определение константы \"ПРЕДЕЛ\"")
	}
	wantLine := lineOf(t, constHoverSrc, "константа ПРЕДЕЛ")
	if pos.Line != wantLine {
		t.Fatalf("ожидалась строка %d, получено %d", wantLine, pos.Line)
	}
}

// TestDefinitionOnLocalVariableJumpsToVarDecl - "итог" в "вернуть итог"
// (функция "сложить", hoverTestSrc) должно вести к "переменная итог = ...",
// не к функции/сущности с тем же именем (которых здесь и нет) - локальная
// переменная/параметр раньше не поддерживались вовсе (см. localDeclPos).
func TestDefinitionOnLocalVariableJumpsToVarDecl(t *testing.T) {
	d := newDoc("test://a.rech", hoverTestSrc)
	line := lineOf(t, hoverTestSrc, "вернуть итог")
	col := runeCol(t, "    вернуть итог", "итог")
	pos, ok := d.definition(line, col)
	if !ok {
		t.Fatalf("ожидалось определение локальной переменной \"итог\"")
	}
	wantLine := lineOf(t, hoverTestSrc, "переменная итог")
	if pos.Line != wantLine {
		t.Fatalf("ожидалась строка %d, получено %d", wantLine, pos.Line)
	}
}

// TestDefinitionOnParameterJumpsToFunctionDecl - "кого" внутри тела
// "сложить" должно вести к строке САМОГО объявления функции (там же
// написаны параметры) - тот же принцип, что и у declPos везде в пакете.
func TestDefinitionOnParameterJumpsToFunctionDecl(t *testing.T) {
	d := newDoc("test://a.rech", hoverTestSrc)
	line := lineOf(t, hoverTestSrc, "переменная итог = кого + кому")
	col := runeCol(t, "    переменная итог = кого + кому", "кого")
	pos, ok := d.definition(line, col)
	if !ok {
		t.Fatalf("ожидалось определение параметра \"кого\"")
	}
	wantLine := lineOf(t, hoverTestSrc, "функция сложить")
	if pos.Line != wantLine {
		t.Fatalf("ожидалась строка %d, получено %d", wantLine, pos.Line)
	}
}

// TestDefinitionOnThisJumpsToOwningEntity - "этот" внутри метода "длина()"
// должно вести к объявлению самой сущности "Точка".
func TestDefinitionOnThisJumpsToOwningEntity(t *testing.T) {
	d := newDoc("test://a.rech", hoverTestSrc)
	line := lineOf(t, hoverTestSrc, "вернуть этот.x")
	col := runeCol(t, "        вернуть этот.x", "этот")
	pos, ok := d.definition(line, col)
	if !ok {
		t.Fatalf("ожидалось определение \"этот\"")
	}
	wantLine := lineOf(t, hoverTestSrc, "сущность Точка")
	if pos.Line != wantLine {
		t.Fatalf("ожидалась строка %d, получено %d", wantLine, pos.Line)
	}
}

// TestDefinitionOnBuiltinFuncJumpsToSyntheticStub - "перейти к определению"
// на встроенной функции ("корень" - internal/builtins.All) раньше не давал
// вообще ничего (нет ast.FunctionDecl - она реализована в Go) - теперь ведёт
// на синтетический URI-заглушку (builtindefinition.go), а не просто молчит.
func TestDefinitionOnBuiltinFuncJumpsToSyntheticStub(t *testing.T) {
	src := `
функция главная
{
    вывести корень(4.0)
}
`
	d := newDoc("test://a.rech", src)
	line := lineOf(t, src, "корень(4.0)")
	col := runeCol(t, "    вывести корень(4.0)", "корень")
	pos, ok := d.definition(line, col)
	if !ok {
		t.Fatalf("ожидалось определение встроенной функции \"корень\"")
	}
	if pos.URI != builtinStubURI("корень") {
		t.Fatalf("ожидался синтетический URI %q, получено %q", builtinStubURI("корень"), pos.URI)
	}
	content, ok := builtinStubSource("корень")
	if !ok {
		t.Fatalf("ожидался синтезированный текст-заглушка для \"корень\"")
	}
	if pos.Line != 1 || pos.Col != runeCol(t, content, "корень") {
		t.Fatalf("позиция имени в заглушке не совпадает с самим текстом: %+v, текст %q", pos, content)
	}
}

// TestDefinitionOnReservedEntityWithFieldsJumpsToSyntheticStub - "перейти к
// определению" на ГОЛОМ имени зарезервированной сущности с известными
// полями (internal/lsp/builtindocs.go, reservedEntityFields) - "Процесс" -
// НАЙДЕННЫЙ БАГ (репортнуто пользователем - "не для всех сущностей переход
// работает"): у неё нет ast.EntityDecl вовсе (регистрируется компилятором
// синтетически, internal/semantic/analyzer.go), в отличие от обычных
// сущностей "перейти к определению" на ней не делал ничего.
func TestDefinitionOnReservedEntityWithFieldsJumpsToSyntheticStub(t *testing.T) {
	src := "функция главная\n{\n    Процесс x = запустить_процесс(\"ls\", новый Список<Строка>())\n}\n"
	d := newDoc("test://a.rech", src)
	line := lineOf(t, src, "Процесс x")
	col := runeCol(t, "    Процесс x = запустить_процесс(\"ls\", новый Список<Строка>())", "Процесс")
	pos, ok := d.definition(line, col)
	if !ok {
		t.Fatalf("ожидалось определение зарезервированной сущности \"Процесс\"")
	}
	if pos.URI != builtinStubURI("Процесс") {
		t.Fatalf("ожидался синтетический URI %q, получено %q", builtinStubURI("Процесс"), pos.URI)
	}
	content, ok := builtinTypeStubSource("Процесс")
	if !ok {
		t.Fatalf("ожидался синтезированный текст-заглушка для \"Процесс\"")
	}
	if pos.Line != 1 || pos.Col != runeCol(t, content, "Процесс") {
		t.Fatalf("позиция имени в заглушке не совпадает с самим текстом: %+v, текст %q", pos, content)
	}
}

// TestDefinitionOnBareBuiltinTypeJumpsToSyntheticStub - та же проверка, что
// и выше, для типа БЕЗ известных полей (builtinTypeDocs, но не
// reservedEntityFields) - "Файл" - заглушка тогда несёт только голое имя,
// без блока "{ имеет: ... }".
func TestDefinitionOnBareBuiltinTypeJumpsToSyntheticStub(t *testing.T) {
	src := "функция главная\n{\n    Файл x = стандартный_ввод()\n}\n"
	d := newDoc("test://a.rech", src)
	line := lineOf(t, src, "Файл x")
	col := runeCol(t, "    Файл x = стандартный_ввод()", "Файл")
	pos, ok := d.definition(line, col)
	if !ok {
		t.Fatalf("ожидалось определение встроенного типа \"Файл\"")
	}
	if pos.URI != builtinStubURI("Файл") {
		t.Fatalf("ожидался синтетический URI %q, получено %q", builtinStubURI("Файл"), pos.URI)
	}
	content, ok := builtinTypeStubSource("Файл")
	if !ok {
		t.Fatalf("ожидался синтезированный текст-заглушка для \"Файл\"")
	}
	if pos.Line != 1 || pos.Col != runeCol(t, content, "Файл") {
		t.Fatalf("позиция имени в заглушке не совпадает с самим текстом: %+v, текст %q", pos, content)
	}
}

// TestBuiltinStubNamePercentDecodesURI - vscode.Uri.toString() percent-
// encode'ит кириллицу в пути ("rech-builtin:///%D0%BA...rech"), тем же
// принципом, что и настоящий "file://" (см. package doc multifile.go,
// uriToPath) - НАЙДЕННЫЙ БАГ: builtinStubName раньше не декодировал вовсе
// (наивный TrimPrefix/TrimSuffix), клиент открывал заглушку встроенной
// функции/типа ПУСТОЙ, хотя и Go unit-тесты (прямой вызов с кириллицей "в
// обход" URI-кодирования), и hand-crafted JSON-RPC smoke-test (ASCII-only
// URI) сервер считали полностью корректным - разрыв был именно в этом
// декодировании.
func TestBuiltinStubNamePercentDecodesURI(t *testing.T) {
	uri := builtinStubScheme + ":///" + url.PathEscape("корень") + builtinStubSuffix
	name, ok := builtinStubName(uri)
	if !ok {
		t.Fatalf("ожидалось успешное извлечение имени из %q", uri)
	}
	if name != "корень" {
		t.Fatalf("ожидалось имя \"корень\", получено %q", name)
	}
}

// declinedEntityDefinitionSrc - "Пользователя" (винительный падеж после
// "наследует", как и в internal/parser/declension_test.go) написан ИНАЧЕ,
// чем объявлена сама сущность ("Пользователь") - "перейти к определению" на
// нём раньше промахивался мимо d.astEntities (ключ - именительная лемма).
const declinedEntityDefinitionSrc = `
сущность Пользователь
{
}
сущность Сотрудник наследует Пользователя
{
}
функция главная { вывести "ок" }
`

func TestDefinitionOnDeclinedEntityNameJumpsToNominativeDecl(t *testing.T) {
	d := newDoc("test://a.rech", declinedEntityDefinitionSrc)
	line := lineOf(t, declinedEntityDefinitionSrc, "наследует Пользователя")
	col := runeCol(t, "сущность Сотрудник наследует Пользователя", "Пользователя")
	pos, ok := d.definition(line, col)
	if !ok {
		t.Fatalf("ожидалось определение сущности через словоформу \"Пользователя\"")
	}
	wantLine := lineOf(t, declinedEntityDefinitionSrc, "сущность Пользователь")
	if pos.Line != wantLine {
		t.Fatalf("ожидалась строка %d, получено %d", wantLine, pos.Line)
	}
}

// gerundCallDefinitionSrc - тот же фикстур, что и у internal/parser/
// gerund_test.go: "сложив" (деепричастная форма "сложить") написана ИНАЧЕ,
// чем объявлена сама функция.
const gerundCallDefinitionSrc = `
функция сложить(а: Целое, б: Целое): Целое
{
    вернуть а + б
}
функция главная
{
    сложив(2, 3)
        вывести результат
}
`

func TestDefinitionOnGerundCallJumpsToFunctionDecl(t *testing.T) {
	d := newDoc("test://a.rech", gerundCallDefinitionSrc)
	line := lineOf(t, gerundCallDefinitionSrc, "сложив(2, 3)")
	col := runeCol(t, "    сложив(2, 3)", "сложив")
	pos, ok := d.definition(line, col)
	if !ok {
		t.Fatalf("ожидалось определение функции через деепричастие \"сложив\"")
	}
	wantLine := lineOf(t, gerundCallDefinitionSrc, "функция сложить")
	if pos.Line != wantLine {
		t.Fatalf("ожидалась строка %d, получено %d", wantLine, pos.Line)
	}
}

// TestDefinitionOnLocalVariableWordformJumpsToDecl - "паролем" (творительный
// падеж "пароль", падежное сопоставление аргумента естественного вызова,
// occurrencesTestSrc - тот же фикстур, что и у occurrences_test.go) написано
// ИНАЧЕ, чем объявлена сама переменная ("пароль").
func TestDefinitionOnLocalVariableWordformJumpsToDecl(t *testing.T) {
	d := newDoc("test://a.rech", occurrencesTestSrc)
	line := lineOf(t, occurrencesTestSrc, "авторизовать паролем")
	col := runeCol(t, "    пользователь авторизовать паролем", "паролем")
	pos, ok := d.definition(line, col)
	if !ok {
		t.Fatalf("ожидалось определение переменной через словоформу \"паролем\"")
	}
	wantLine := lineOf(t, occurrencesTestSrc, `переменная пароль = "секрет"`)
	if pos.Line != wantLine {
		t.Fatalf("ожидалась строка %d, получено %d", wantLine, pos.Line)
	}
}
