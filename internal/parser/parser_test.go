package parser

import (
	"strings"
	"testing"

	"github.com/IlyaFeoktistov/Rech/internal/ast"
	"github.com/IlyaFeoktistov/Rech/internal/lexer"
	"github.com/IlyaFeoktistov/Rech/internal/morph"
)

func parseSource(t *testing.T, src string) (*ast.Program, error) {
	t.Helper()
	_, prog, err := parseSourceWithParser(t, src)
	return prog, err
}

func parseSourceWithParser(t *testing.T, src string) (*Parser, *ast.Program, error) {
	t.Helper()
	toks, err := lexer.New(src).Tokenize()
	if err != nil {
		t.Fatalf("лексер: %v", err)
	}
	p := New(toks)
	prog, err := p.ParseProgram()
	return p, prog, err
}

func TestParseFunctionParamsAndReturnType(t *testing.T) {
	src := `
функция сложить(кого: Целое, кому: Целое): Целое
{
    вернуть кого + кому
}
функция главная { вывести "ок" }
`
	prog, err := parseSource(t, src)
	if err != nil {
		t.Fatalf("неожиданная ошибка: %v", err)
	}
	fn := prog.Functions[0]
	if fn.Name != "сложить" {
		t.Fatalf("ожидалось имя \"сложить\", получено %q", fn.Name)
	}
	if len(fn.Params) != 2 {
		t.Fatalf("ожидалось 2 параметра, получено %d", len(fn.Params))
	}
	if fn.Params[0].Name != "кого" || fn.Params[0].Type != "Целое" {
		t.Fatalf("неожиданный первый параметр: %+v", fn.Params[0])
	}
	if fn.Params[1].Name != "кому" || fn.Params[1].Type != "Целое" {
		t.Fatalf("неожиданный второй параметр: %+v", fn.Params[1])
	}
	if fn.ReturnType != "Целое" {
		t.Fatalf("ожидался тип результата \"Целое\", получено %q", fn.ReturnType)
	}
	ret, ok := fn.Body[0].(*ast.ReturnStmt)
	if !ok || ret.Value == nil {
		t.Fatalf("ожидался \"вернуть <значение>\", получено %+v", fn.Body[0])
	}
}

func TestParseFunctionInvalidParamName(t *testing.T) {
	_, err := parseSource(t, `
функция сложить(х: Целое): Целое { вернуть х }
функция главная { вывести "ок" }
`)
	if err == nil || !strings.Contains(err.Error(), "падежных вопросительных слов") {
		t.Fatalf("ожидалась ошибка про падежные имена параметров, получено: %v", err)
	}
}

func TestParseFunctionDuplicateCase(t *testing.T) {
	_, err := parseSource(t, `
функция сложить(кого: Целое, что: Целое): Целое { вернуть кого + что }
функция главная { вывести "ок" }
`)
	if err == nil || !strings.Contains(err.Error(), "дублирует падежную роль") {
		t.Fatalf("ожидалась ошибка про дублирование падежа, получено: %v", err)
	}
}

func TestParseEntryPointCannotHaveParams(t *testing.T) {
	_, err := parseSource(t, `функция главная(кого: Целое) { вывести кого }`)
	if err == nil || !strings.Contains(err.Error(), "точка входа") {
		t.Fatalf("ожидалась ошибка про точку входа, получено: %v", err)
	}
}

func TestParseFunctionWithoutParamsStillWorks(t *testing.T) {
	prog, err := parseSource(t, `функция главная { вывести "ок" }`)
	if err != nil {
		t.Fatalf("неожиданная ошибка: %v", err)
	}
	fn := prog.Functions[0]
	if len(fn.Params) != 0 || fn.ReturnType != "" {
		t.Fatalf("функция без параметров/типа результата должна остаться пустой: %+v", fn)
	}
}

func TestParseCallExprAsExpression(t *testing.T) {
	prog, err := parseSource(t, `
функция главная {
    переменная х = сложить(1, 2)
}
`)
	if err != nil {
		t.Fatalf("неожиданная ошибка: %v", err)
	}
	varDecl := prog.Functions[0].Body[0].(*ast.VarDecl)
	call, ok := varDecl.Value.(*ast.CallExpr)
	if !ok {
		t.Fatalf("ожидался CallExpr, получено %T", varDecl.Value)
	}
	if call.Callee != "сложить" || len(call.Args) != 2 {
		t.Fatalf("неожиданный вызов: %+v", call)
	}
}

func TestParseCallAsStatement(t *testing.T) {
	prog, err := parseSource(t, `функция главная { поздороваться() }`)
	if err != nil {
		t.Fatalf("неожиданная ошибка: %v", err)
	}
	stmt, ok := prog.Functions[0].Body[0].(*ast.ExprStmt)
	if !ok || stmt.Call.Callee != "поздороваться" {
		t.Fatalf("ожидался ExprStmt с вызовом \"поздороваться\", получено %+v", prog.Functions[0].Body[0])
	}
}

func TestVariableRegisteredAsNounInDictionary(t *testing.T) {
	p, _, err := parseSourceWithParser(t, `
функция главная {
    переменная пользователь = "Илья"
    вывести пользователь
}
`)
	if err != nil {
		t.Fatalf("неожиданная ошибка: %v", err)
	}
	analyses := p.dict.Analyze("пользователя")
	found := false
	for _, a := range analyses {
		if a.Lemma == "пользователь" && a.POS == morph.Noun && a.Tag.Case == morph.Accusative {
			found = true
		}
	}
	if !found {
		t.Fatalf("ожидался разбор \"пользователя\" как винительного падежа леммы \"пользователь\", получено: %+v", analyses)
	}
}

func TestVariableWithUnclassifiableNameIsNotRegistered(t *testing.T) {
	// "чай" оканчивается на "-й" - в Фазе A движка (internal/morph) это
	// неоднозначное окончание (ClassifyByEnding возвращает ok=false), слова
	// нет ни в курируемых исключениях, ни в словаре OpenCorpora для "-й"
	// (тот справочник подключён только для слов на "-ь") - поэтому
	// RegisterNoun не может его классифицировать. Это ожидаемо: такая
	// переменная просто недоступна для естественного вызова.
	p, _, err := parseSourceWithParser(t, `
функция главная {
    переменная чай = "зелёный"
    вывести чай
}
`)
	if err != nil {
		t.Fatalf("неожиданная ошибка: %v", err)
	}
	if _, ok := p.dict.Lookup("чай"); ok {
		t.Fatalf("слово с неоднозначным для Фазы A окончанием не должно регистрироваться как существительное")
	}
}

func TestParseNaturalCallOrderIndependent(t *testing.T) {
	src := `
функция поприветствовать(кого: Строка, где: Строка) { вывести кого }
функция главная {
    переменная пользователь = "Илья"
    переменная система = "чат"
    поприветствовать %s
}
`
	for _, phrase := range []string{"пользователя в системе", "в системе пользователя", "пользователя системе"} {
		prog, err := parseSource(t, strings.Replace(src, "%s", phrase, 1))
		if err != nil {
			t.Fatalf("фраза %q: неожиданная ошибка: %v", phrase, err)
		}
		stmt := prog.Functions[1].Body[2].(*ast.ExprStmt)
		call := stmt.Call
		if call.Callee != "поприветствовать" || len(call.Args) != 2 {
			t.Fatalf("фраза %q: неожиданный вызов: %+v", phrase, call)
		}
		if id, ok := call.Args[0].(*ast.Identifier); !ok || id.Name != "пользователь" {
			t.Fatalf("фраза %q: ожидался первым аргументом \"пользователь\" (параметр \"кого\"), получено %+v", phrase, call.Args[0])
		}
		if id, ok := call.Args[1].(*ast.Identifier); !ok || id.Name != "система" {
			t.Fatalf("фраза %q: ожидался вторым аргументом \"система\" (параметр \"где\"), получено %+v", phrase, call.Args[1])
		}
	}
}

func TestParseNaturalCallUnrecognizedWordform(t *testing.T) {
	_, err := parseSource(t, `
функция поприветствовать(кого: Строка) { вывести кого }
функция главная { поприветствовать чтототам }
`)
	if err == nil || !strings.Contains(err.Error(), "не опознана") {
		t.Fatalf("ожидалась ошибка про неопознанную словоформу, получено: %v", err)
	}
}

func TestParseNaturalCallAnimacyDisambiguatesWhomVsWhat(t *testing.T) {
	_, err := parseSource(t, `
функция обработать(кого: Строка) { вывести кого }
функция главная {
    переменная система = "чат"
    обработать систему
}
`)
	if err == nil || !strings.Contains(err.Error(), "не подходит по падежу") {
		t.Fatalf("ожидалась ошибка про несовпадение по падежу/одушевлённости, получено: %v", err)
	}
}

func TestParseEntityWithFields(t *testing.T) {
	prog, err := parseSource(t, `
сущность Пользователь
{
    имеет:
        имя: Строка
        возраст: Целое
}
функция главная { вывести "ок" }
`)
	if err != nil {
		t.Fatalf("неожиданная ошибка: %v", err)
	}
	if len(prog.Entities) != 1 {
		t.Fatalf("ожидалась одна сущность, получено %d", len(prog.Entities))
	}
	ent := prog.Entities[0]
	if ent.Name != "Пользователь" || len(ent.Fields) != 2 {
		t.Fatalf("неожиданная сущность: %+v", ent)
	}
	if ent.Fields[0].Name != "имя" || ent.Fields[0].Type != "Строка" {
		t.Fatalf("неожиданное первое поле: %+v", ent.Fields[0])
	}
	if ent.Fields[1].Name != "возраст" || ent.Fields[1].Type != "Целое" {
		t.Fatalf("неожиданное второе поле: %+v", ent.Fields[1])
	}
}

func TestParseClassIsSynonymForEntity(t *testing.T) {
	prog, err := parseSource(t, `
класс Пользователь
{
    имеет:
        имя: Строка
}
функция главная { вывести "ок" }
`)
	if err != nil {
		t.Fatalf("неожиданная ошибка: %v", err)
	}
	if len(prog.Entities) != 1 || prog.Entities[0].Name != "Пользователь" {
		t.Fatalf("\"класс\" должен разбираться как синоним \"сущность\": %+v", prog)
	}
}

func TestParseFieldAtSameIndentAsHasEndsSection(t *testing.T) {
	// Поле на той же глубине отступа, что и "имеет", уже не принадлежит
	// секции (docs/GRAMMAR.md, "Секции внутри объявления сущности") -
	// после пустой секции тут нет ничего валидного (методов ещё нет), так
	// что ожидаем ошибку разбора, а не поле в EntityDecl.Fields.
	_, err := parseSource(t, `
сущность Пользователь
{
    имеет:
    имя: Строка
}
функция главная { вывести "ок" }
`)
	if err == nil {
		t.Fatalf("ожидалась ошибка - поле на уровне \"имеет\" не должно попасть в секцию")
	}
}

func TestParseEmptyEntity(t *testing.T) {
	prog, err := parseSource(t, `
сущность Пустая {}
функция главная { вывести "ок" }
`)
	if err != nil {
		t.Fatalf("неожиданная ошибка: %v", err)
	}
	if len(prog.Entities) != 1 || len(prog.Entities[0].Fields) != 0 {
		t.Fatalf("неожиданный результат: %+v", prog.Entities)
	}
}

func TestParseReturnWithoutValueStillWorks(t *testing.T) {
	prog, err := parseSource(t, `
функция главная {
    если истина {
        вернуть
    }
    вывести "ок"
}
`)
	if err != nil {
		t.Fatalf("неожиданная ошибка: %v", err)
	}
	ifStmt := prog.Functions[0].Body[0].(*ast.IfStmt)
	ret := ifStmt.Then[0].(*ast.ReturnStmt)
	if ret.Value != nil {
		t.Fatalf("ожидался \"вернуть\" без значения, получено значение: %+v", ret.Value)
	}
}
