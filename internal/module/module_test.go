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
	writeFile(t, dir, "математика.rech", "внешняя функция sqrt как корень(x: Число): Число\n")
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
