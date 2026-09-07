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

// TestParseFunctionExplicitPustoReturnTypeNormalizesToEmpty - "Пусто" как
// тип результата - чисто синтаксическая форма "нет типа результата",
// эквивалентная полностью пропущенной аннотации ": Тип" -
// парсер схлопывает её в "" (см. normalizeReturnType), а не оставляет
// строку "Пусто" для семантики/backend'ов.
func TestParseFunctionExplicitPustoReturnTypeNormalizesToEmpty(t *testing.T) {
	prog, err := parseSource(t, `
функция поприветствовать(имя: Строка): Пусто { вывести имя }
функция главная { вывести "ок" }
`)
	if err != nil {
		t.Fatalf("неожиданная ошибка: %v", err)
	}
	if prog.Functions[0].ReturnType != "" {
		t.Fatalf("ожидался пустой тип результата (Пусто == нет аннотации), получено %q", prog.Functions[0].ReturnType)
	}
}

func TestParseFunctionArbitraryParamNameWithoutCaseRole(t *testing.T) {
	// Параметр без падежной роли (одно слово, не входящее в список из
	// шести) - обычный, только позиционный параметр. Раньше это было
	// ошибкой компиляции - решение отменено по итогам обсуждения с
	// пользователем: жёсткий потолок в 6 параметров на функцию снят,
	// осмысленные имена параметров разрешены.
	prog, err := parseSource(t, `
функция сложить(х: Целое): Целое { вернуть х }
функция главная { вывести "ок" }
`)
	if err != nil {
		t.Fatalf("неожиданная ошибка: %v", err)
	}
	param := prog.Functions[0].Params[0]
	if param.Name != "х" || param.CaseWord != "" {
		t.Fatalf("ожидался параметр без падежной роли с именем \"х\", получено %+v", param)
	}
}

func TestParseFunctionTwoWordParamCaseAndName(t *testing.T) {
	// "падеж имя: Тип" - падежная роль отдельно от отображаемого имени.
	prog, err := parseSource(t, `
функция оформить(кого пользователь: Строка, права: Строка) { вывести пользователь }
функция главная { вывести "ок" }
`)
	if err != nil {
		t.Fatalf("неожиданная ошибка: %v", err)
	}
	params := prog.Functions[0].Params
	if params[0].Name != "пользователь" || params[0].CaseWord != "кого" {
		t.Fatalf("ожидался параметр \"пользователь\" с ролью \"кого\", получено %+v", params[0])
	}
	if params[1].Name != "права" || params[1].CaseWord != "" {
		t.Fatalf("ожидался параметр \"права\" без падежной роли, получено %+v", params[1])
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

func TestParseExternFuncDecl(t *testing.T) {
	prog, err := parseSource(t, `
внешняя функция sqrt как корень2(x: Число): Число
функция главная { вывести корень2(4.0) }
`)
	if err != nil {
		t.Fatalf("неожиданная ошибка: %v", err)
	}
	if len(prog.ExternFuncs) != 1 {
		t.Fatalf("ожидалась одна внешняя функция, получено: %d", len(prog.ExternFuncs))
	}
	ext := prog.ExternFuncs[0]
	if ext.CName != "sqrt" || ext.Alias != "корень2" {
		t.Fatalf("CName/Alias не совпадают с ожидаемыми: %+v", ext)
	}
	if len(ext.Params) != 1 || ext.Params[0].Type != "Число" || ext.ReturnType != "Число" {
		t.Fatalf("параметры/тип результата разобраны неверно: %+v", ext)
	}
}

func TestParseExternFuncDeclWithoutAliasUsesCName(t *testing.T) {
	prog, err := parseSource(t, `
внешняя функция strlen(текст: Строка): Целое
функция главная { вывести strlen("привет") }
`)
	if err != nil {
		t.Fatalf("неожиданная ошибка: %v", err)
	}
	ext := prog.ExternFuncs[0]
	if ext.CName != "strlen" || ext.Alias != "strlen" {
		t.Fatalf("без \"как\" Alias должен совпадать с CName: %+v", ext)
	}
}

func TestParseExternFuncDeclWithoutReturnType(t *testing.T) {
	prog, err := parseSource(t, `
внешняя функция srand(семя: Целое)
функция главная { srand(1) }
`)
	if err != nil {
		t.Fatalf("неожиданная ошибка: %v", err)
	}
	if prog.ExternFuncs[0].ReturnType != "" {
		t.Fatalf("ожидался пустой тип результата, получено: %q", prog.ExternFuncs[0].ReturnType)
	}
}

func TestParseExternFuncDeclCannotBeEntryPoint(t *testing.T) {
	_, err := parseSource(t, `внешняя функция exit как главная(код: Целое)`)
	if err == nil || !strings.Contains(err.Error(), "точка входа") {
		t.Fatalf("ожидалась ошибка про точку входа, получено: %v", err)
	}
}

func TestParseEnumDeclAutoAndExplicitValues(t *testing.T) {
	prog, err := parseSource(t, `
перечисление Приоритет
{
    Низкий,
    Средний = 5,
    Высокий
}
функция главная { вывести Приоритет.Высокий }
`)
	if err != nil {
		t.Fatalf("неожиданная ошибка: %v", err)
	}
	if len(prog.Enums) != 1 {
		t.Fatalf("ожидалось одно перечисление, получено: %d", len(prog.Enums))
	}
	enum := prog.Enums[0]
	if enum.Name != "Приоритет" || len(enum.Members) != 3 {
		t.Fatalf("неожиданное перечисление: %+v", enum)
	}
	want := map[string]int64{"Низкий": 0, "Средний": 5, "Высокий": 6}
	for _, m := range enum.Members {
		if want[m.Name] != m.Value {
			t.Fatalf("член %q: ожидалось значение %d, получено %d", m.Name, want[m.Name], m.Value)
		}
	}
}

func TestParseEnumDeclRejectsDuplicateMember(t *testing.T) {
	_, err := parseSource(t, `
перечисление Цвет { Красный, Красный }
функция главная { вывести 1 }
`)
	if err == nil || !strings.Contains(err.Error(), "объявлен более одного раза") {
		t.Fatalf("ожидалась ошибка про дублирующийся член, получено: %v", err)
	}
}

func TestParseEnumDeclRejectsEmpty(t *testing.T) {
	_, err := parseSource(t, `
перечисление Цвет { }
функция главная { вывести 1 }
`)
	if err == nil || !strings.Contains(err.Error(), "хотя бы один член") {
		t.Fatalf("ожидалась ошибка про пустое перечисление, получено: %v", err)
	}
}

func TestParseConstDecl(t *testing.T) {
	prog, err := parseSource(t, `
функция главная
{
    константа порог = 10
    вывести порог
}
`)
	if err != nil {
		t.Fatalf("неожиданная ошибка: %v", err)
	}
	decl, ok := prog.Functions[0].Body[0].(*ast.ConstDecl)
	if !ok {
		t.Fatalf("ожидался *ast.ConstDecl, получено %T", prog.Functions[0].Body[0])
	}
	if decl.Name != "порог" {
		t.Fatalf("неожиданное имя константы: %q", decl.Name)
	}
}

func TestParseExternFuncDeclRequiresFunctionKeyword(t *testing.T) {
	_, err := parseSource(t, `внешняя sqrt(x: Число): Число`)
	if err == nil {
		t.Fatalf("ожидалась ошибка отсутствия \"функция\"")
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
	if err == nil || !strings.Contains(err.Error(), "не удалось подобрать по падежу") {
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
	if err == nil || !strings.Contains(err.Error(), "не удалось подобрать по падежу") {
		t.Fatalf("ожидалась ошибка про несовпадение по падежу/одушевлённости, получено: %v", err)
	}
}

// TestParseNaturalCallParamAsArgument - параметр функции регистрируется как
// существительное (registerNoun, decl.go/parseParams) так же, как и обычная
// переменная - раньше параметры вообще не регистрировались, поэтому их
// нельзя было передать словоформой в естественный вызов без предварительного
// присваивания локальной переменной (docs/ROADMAP.md, Шаг 77).
func TestParseNaturalCallParamAsArgument(t *testing.T) {
	prog, err := parseSource(t, `
функция сложить(что: Целое, кем: Целое): Целое { вернуть что + кем }
функция обёртка(число: Целое): Целое { вернуть сложить число числом }
`)
	if err != nil {
		t.Fatalf("неожиданная ошибка: %v", err)
	}
	ret := prog.Functions[1].Body[0].(*ast.ReturnStmt)
	call, ok := ret.Value.(*ast.CallExpr)
	if !ok || call.Callee != "сложить" || len(call.Args) != 2 {
		t.Fatalf("неожиданный вызов: %+v", ret.Value)
	}
	for i, want := range []string{"число", "число"} {
		if id, ok := call.Args[i].(*ast.Identifier); !ok || id.Name != want {
			t.Fatalf("аргумент %d: ожидалось %q, получено %+v", i, want, call.Args[i])
		}
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

func TestParseEntityStatesDerivesTypeName(t *testing.T) {
	prog, err := parseSource(t, `
сущность Пользователь
{
    состояние: неавторизован авторизован

    имеет:
        имя: Строка
}
функция главная { вывести "ок" }
`)
	if err != nil {
		t.Fatalf("неожиданная ошибка: %v", err)
	}
	ent := prog.Entities[0]
	if len(ent.States) != 2 {
		t.Fatalf("ожидалось 2 состояния, получено %+v", ent.States)
	}
	if ent.States[0].Name != "неавторизован" || ent.States[0].TypeName != "НеавторизованныйПользователь" {
		t.Fatalf("неожиданное первое состояние: %+v", ent.States[0])
	}
	if ent.States[1].Name != "авторизован" || ent.States[1].TypeName != "АвторизованныйПользователь" {
		t.Fatalf("неожиданное второе состояние: %+v", ent.States[1])
	}
	if len(ent.Fields) != 1 || ent.Fields[0].Name != "имя" {
		t.Fatalf("поля после \"состояние:\" не разобрались: %+v", ent.Fields)
	}
}

func TestParseEntityStatesFromActiveParticiple(t *testing.T) {
	prog, err := parseSource(t, `
сущность Пользователь
{
    состояние: вошедший

    имеет:
        имя: Строка
}
функция главная { вывести "ок" }
`)
	if err != nil {
		t.Fatalf("неожиданная ошибка: %v", err)
	}
	ent := prog.Entities[0]
	if len(ent.States) != 1 {
		t.Fatalf("ожидалось 1 состояние, получено %+v", ent.States)
	}
	if ent.States[0].Name != "вошедший" || ent.States[0].TypeName != "ВошедшийПользователь" {
		t.Fatalf("неожиданное состояние: %+v", ent.States[0])
	}
}

func TestParseEntityStatesFromActiveParticipleAgreesWithGender(t *testing.T) {
	prog, err := parseSource(t, `
сущность Сотрудница
{
    состояние: вошедший

    имеет:
        имя: Строка
}
функция главная { вывести "ок" }
`)
	if err != nil {
		t.Fatalf("неожиданная ошибка: %v", err)
	}
	ent := prog.Entities[0]
	if ent.States[0].TypeName != "ВошедшаяСотрудница" {
		t.Fatalf("ожидалось согласование по женскому роду, получено: %+v", ent.States[0])
	}
}

func TestParseEntityStatesUnknownGenderFails(t *testing.T) {
	_, err := parseSource(t, `
сущность Мнофе
{
    состояние: авторизован
}
функция главная { вывести "ок" }
`)
	if err == nil || !strings.Contains(err.Error(), "не удалось определить род сущности") {
		t.Fatalf("ожидалась ошибка про неопределимый род, получено: %v", err)
	}
}

func TestParseBecomeStmt(t *testing.T) {
	prog, err := parseSource(t, `
сущность Пользователь
{
    состояние: неавторизован авторизован
}
функция главная {
    переменная пользователь = новый Пользователь()
    пользователь становится авторизован
}
`)
	if err != nil {
		t.Fatalf("неожиданная ошибка: %v", err)
	}
	become, ok := prog.Functions[0].Body[1].(*ast.BecomeStmt)
	if !ok {
		t.Fatalf("ожидался ast.BecomeStmt, получено %+v", prog.Functions[0].Body[1])
	}
	if id, ok := become.Receiver.(*ast.Identifier); !ok || id.Name != "пользователь" {
		t.Fatalf("неожиданный получатель: %+v", become.Receiver)
	}
	if become.State != "авторизован" {
		t.Fatalf("неожиданное состояние: %q", become.State)
	}
}

func TestParseBecomeStmtWithThisInMethod(t *testing.T) {
	prog, err := parseSource(t, `
сущность Пользователь
{
    состояние: неавторизован авторизован

    авторизовать()
    {
        этот становится авторизован
    }
}
функция главная { вывести "ок" }
`)
	if err != nil {
		t.Fatalf("неожиданная ошибка: %v", err)
	}
	become, ok := prog.Entities[0].Methods[0].Body[0].(*ast.BecomeStmt)
	if !ok {
		t.Fatalf("ожидался ast.BecomeStmt, получено %+v", prog.Entities[0].Methods[0].Body[0])
	}
	if _, ok := become.Receiver.(*ast.ThisExpr); !ok {
		t.Fatalf("ожидался получатель \"этот\", получено %+v", become.Receiver)
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

func TestParseNewExprAndFieldAccess(t *testing.T) {
	prog, err := parseSource(t, `
сущность Пользователь
{
    имеет:
        имя: Строка
}
функция главная {
    переменная пользователь = новый Пользователь("Илья")
    вывести пользователь.имя
}
`)
	if err != nil {
		t.Fatalf("неожиданная ошибка: %v", err)
	}
	varDecl := prog.Functions[0].Body[0].(*ast.VarDecl)
	newExpr, ok := varDecl.Value.(*ast.NewExpr)
	if !ok || newExpr.Type != "Пользователь" || len(newExpr.Args) != 1 {
		t.Fatalf("ожидался NewExpr для \"Пользователь\" с одним аргументом, получено %+v", varDecl.Value)
	}
	printStmt := prog.Functions[0].Body[1].(*ast.PrintStmt)
	access, ok := printStmt.Value.(*ast.FieldAccess)
	if !ok || access.Field != "имя" {
		t.Fatalf("ожидался FieldAccess по полю \"имя\", получено %+v", printStmt.Value)
	}
	if id, ok := access.Receiver.(*ast.Identifier); !ok || id.Name != "пользователь" {
		t.Fatalf("ожидался получатель \"пользователь\", получено %+v", access.Receiver)
	}
}

func TestParseFieldAccessChain(t *testing.T) {
	prog, err := parseSource(t, `
функция главная {
    переменная х = 1
    вывести х.а.б
}
`)
	if err != nil {
		t.Fatalf("неожиданная ошибка: %v", err)
	}
	printStmt := prog.Functions[0].Body[1].(*ast.PrintStmt)
	outer, ok := printStmt.Value.(*ast.FieldAccess)
	if !ok || outer.Field != "б" {
		t.Fatalf("ожидался внешний FieldAccess по полю \"б\", получено %+v", printStmt.Value)
	}
	if _, ok := outer.Receiver.(*ast.FieldAccess); !ok {
		t.Fatalf("ожидался вложенный FieldAccess как получатель, получено %+v", outer.Receiver)
	}
}

func TestParseFieldAssign(t *testing.T) {
	prog, err := parseSource(t, `
функция главная {
    переменная пользователь = 0
    пользователь.возраст = 31
}
`)
	if err != nil {
		t.Fatalf("неожиданная ошибка: %v", err)
	}
	stmt, ok := prog.Functions[0].Body[1].(*ast.FieldAssign)
	if !ok {
		t.Fatalf("ожидался FieldAssign, получено %T", prog.Functions[0].Body[1])
	}
	if stmt.Field != "возраст" {
		t.Fatalf("ожидалось поле \"возраст\", получено %q", stmt.Field)
	}
	if id, ok := stmt.Receiver.(*ast.Identifier); !ok || id.Name != "пользователь" {
		t.Fatalf("ожидался получатель \"пользователь\", получено %+v", stmt.Receiver)
	}
	if _, ok := stmt.Value.(*ast.IntLiteral); !ok {
		t.Fatalf("ожидалось значение-литерал, получено %+v", stmt.Value)
	}
}

// TestParseHexOctalBinaryIntLiteralValues - значение (не только форма
// токена, см. lexer_test.go) - "0xFF"/"0o17"/"0b1010" дают те же int64, что
// написал бы человек десятичным способом; "007" (без префикса) остаётся
// десятичным (7, не восьмеричным).
func TestParseHexOctalBinaryIntLiteralValues(t *testing.T) {
	for _, tc := range []struct {
		src  string
		want int64
	}{
		{"0xFF", 255},
		{"0Xa", 10},
		{"0o17", 15},
		{"0b1010", 10},
		{"007", 7},
	} {
		prog, err := parseSource(t, `
функция главная {
    переменная х = `+tc.src+`
}
`)
		if err != nil {
			t.Fatalf("%s: неожиданная ошибка: %v", tc.src, err)
		}
		decl, ok := prog.Functions[0].Body[0].(*ast.VarDecl)
		if !ok {
			t.Fatalf("%s: ожидался VarDecl, получено %T", tc.src, prog.Functions[0].Body[0])
		}
		lit, ok := decl.Value.(*ast.IntLiteral)
		if !ok {
			t.Fatalf("%s: ожидался IntLiteral, получено %T", tc.src, decl.Value)
		}
		if lit.Value != tc.want {
			t.Fatalf("%s: ожидалось значение %d, получено %d", tc.src, tc.want, lit.Value)
		}
	}
}

func TestParseCompoundAssign(t *testing.T) {
	prog, err := parseSource(t, `
функция главная {
    переменная а = 10
    а += 5
    а -= 3
    а *= 2
    а /= 4
}
`)
	if err != nil {
		t.Fatalf("неожиданная ошибка: %v", err)
	}
	wantOps := []string{"+", "-", "*", "/"}
	for i, wantOp := range wantOps {
		stmt, ok := prog.Functions[0].Body[i+1].(*ast.Assign)
		if !ok {
			t.Fatalf("инструкция %d: ожидался Assign, получено %T", i, prog.Functions[0].Body[i+1])
		}
		if stmt.Name != "а" {
			t.Fatalf("инструкция %d: ожидалось имя \"а\", получено %q", i, stmt.Name)
		}
		bin, ok := stmt.Value.(*ast.BinaryExpr)
		if !ok || bin.Op != wantOp {
			t.Fatalf("инструкция %d: ожидался BinaryExpr с оператором %q, получено %+v", i, wantOp, stmt.Value)
		}
		if id, ok := bin.Left.(*ast.Identifier); !ok || id.Name != "а" {
			t.Fatalf("инструкция %d: левый операнд должен быть \"а\", получено %+v", i, bin.Left)
		}
	}
}

func TestParseFieldCompoundAssign(t *testing.T) {
	prog, err := parseSource(t, `
функция главная {
    переменная пользователь = 0
    пользователь.возраст += 1
}
`)
	if err != nil {
		t.Fatalf("неожиданная ошибка: %v", err)
	}
	stmt, ok := prog.Functions[0].Body[1].(*ast.FieldAssign)
	if !ok {
		t.Fatalf("ожидался FieldAssign, получено %T", prog.Functions[0].Body[1])
	}
	bin, ok := stmt.Value.(*ast.BinaryExpr)
	if !ok || bin.Op != "+" {
		t.Fatalf("ожидался BinaryExpr с оператором \"+\", получено %+v", stmt.Value)
	}
	fa, ok := bin.Left.(*ast.FieldAccess)
	if !ok || fa.Field != "возраст" {
		t.Fatalf("левый операнд должен быть доступом к полю \"возраст\", получено %+v", bin.Left)
	}
}

func TestParseNestedFieldAssign(t *testing.T) {
	// "x.a.b = значение" - получатель присваивания ("x.a") сам вложенный
	// доступ к полю, не просто IDENT (см. finishFieldAssign).
	prog, err := parseSource(t, `
функция главная {
    переменная пользователь = 0
    пользователь.адрес.город = "Казань"
}
`)
	if err != nil {
		t.Fatalf("неожиданная ошибка: %v", err)
	}
	stmt, ok := prog.Functions[0].Body[1].(*ast.FieldAssign)
	if !ok {
		t.Fatalf("ожидался FieldAssign, получено %T", prog.Functions[0].Body[1])
	}
	if stmt.Field != "город" {
		t.Fatalf("ожидалось поле \"город\", получено %q", stmt.Field)
	}
	recv, ok := stmt.Receiver.(*ast.FieldAccess)
	if !ok || recv.Field != "адрес" {
		t.Fatalf("ожидался получатель - доступ к полю \"адрес\", получено %+v", stmt.Receiver)
	}
	if id, ok := recv.Receiver.(*ast.Identifier); !ok || id.Name != "пользователь" {
		t.Fatalf("ожидался получатель \"пользователь\" на предыдущем уровне, получено %+v", recv.Receiver)
	}
}

func TestParseCallChainStmt(t *testing.T) {
	// "имя(...)" как самостоятельная инструкция - результат тоже годится
	// базой для продолжения цепочки "." (метод как инструкция, присваивание
	// полю), не только результат вызова МЕТОДА (parseDotStatement) - см.
	// finishCallChainStmt.
	prog, err := parseSource(t, `
функция главная {
    создать().ошибка("текст")
    создать().а.б = 1
}
`)
	if err != nil {
		t.Fatalf("неожиданная ошибка: %v", err)
	}
	exprStmt, ok := prog.Functions[0].Body[0].(*ast.ExprStmt)
	if !ok || exprStmt.MethodCall == nil {
		t.Fatalf("инструкция 0: ожидался ExprStmt с MethodCall, получено %T", prog.Functions[0].Body[0])
	}
	call, ok := exprStmt.MethodCall.Receiver.(*ast.CallExpr)
	if !ok || call.Callee != "создать" {
		t.Fatalf("инструкция 0: получатель метода должен быть вызовом \"создать\", получено %+v", exprStmt.MethodCall.Receiver)
	}
	if exprStmt.MethodCall.Method != "ошибка" {
		t.Fatalf("инструкция 0: ожидался метод \"ошибка\", получено %q", exprStmt.MethodCall.Method)
	}
	fieldAssign, ok := prog.Functions[0].Body[1].(*ast.FieldAssign)
	if !ok || fieldAssign.Field != "б" {
		t.Fatalf("инструкция 1: ожидался FieldAssign поля \"б\", получено %+v", prog.Functions[0].Body[1])
	}
	recv, ok := fieldAssign.Receiver.(*ast.FieldAccess)
	if !ok || recv.Field != "а" {
		t.Fatalf("инструкция 1: ожидался получатель - доступ к полю \"а\", получено %+v", fieldAssign.Receiver)
	}
	if _, ok := recv.Receiver.(*ast.CallExpr); !ok {
		t.Fatalf("инструкция 1: получатель на предыдущем уровне должен быть вызовом \"создать\", получено %+v", recv.Receiver)
	}
}

func TestParseMethodAndThis(t *testing.T) {
	prog, err := parseSource(t, `
сущность Пользователь
{
    имеет:
        возраст: Целое

    состариться()
    {
        этот.возраст = этот.возраст + 1
    }
}
функция главная { вывести "ок" }
`)
	if err != nil {
		t.Fatalf("неожиданная ошибка: %v", err)
	}
	ent := prog.Entities[0]
	if len(ent.Methods) != 1 || ent.Methods[0].Name != "состариться" {
		t.Fatalf("ожидался один метод \"состариться\", получено: %+v", ent.Methods)
	}
	assign := ent.Methods[0].Body[0].(*ast.FieldAssign)
	if _, ok := assign.Receiver.(*ast.ThisExpr); !ok {
		t.Fatalf("ожидался получатель ThisExpr, получено %+v", assign.Receiver)
	}
	bin := assign.Value.(*ast.BinaryExpr)
	access, ok := bin.Left.(*ast.FieldAccess)
	if !ok {
		t.Fatalf("ожидался FieldAccess слева, получено %+v", bin.Left)
	}
	if _, ok := access.Receiver.(*ast.ThisExpr); !ok {
		t.Fatalf("ожидался получатель ThisExpr у FieldAccess, получено %+v", access.Receiver)
	}
}

func TestParseThisOutsideMethodFails(t *testing.T) {
	_, err := parseSource(t, `функция главная { вывести этот.поле }`)
	if err == nil || !strings.Contains(err.Error(), "валиден только внутри тела метода") {
		t.Fatalf("ожидалась ошибка про \"этот\" вне метода, получено: %v", err)
	}
}

func TestParseMethodCallAsExpressionAndStatement(t *testing.T) {
	prog, err := parseSource(t, `
сущность П
{
    имеет:
        х: Целое
}
функция главная {
    переменная p = новый П(1)
    p.метод()
    переменная y = p.другой(1, 2)
}
`)
	if err != nil {
		t.Fatalf("неожиданная ошибка: %v", err)
	}
	stmt, ok := prog.Functions[0].Body[1].(*ast.ExprStmt)
	if !ok || stmt.MethodCall == nil || stmt.MethodCall.Method != "метод" {
		t.Fatalf("ожидался ExprStmt с вызовом метода \"метод\", получено %+v", prog.Functions[0].Body[1])
	}
	varDecl := prog.Functions[0].Body[2].(*ast.VarDecl)
	call, ok := varDecl.Value.(*ast.MethodCallExpr)
	if !ok || call.Method != "другой" || len(call.Args) != 2 {
		t.Fatalf("ожидался MethodCallExpr \"другой\" с двумя аргументами, получено %+v", varDecl.Value)
	}
}

func TestParseMethodRequiresParens(t *testing.T) {
	_, err := parseSource(t, `
сущность П
{
    имеет:
        х: Целое
    метод: Целое
}
функция главная { вывести "ок" }
`)
	if err == nil || !strings.Contains(err.Error(), "ожидалось \"(\"") {
		t.Fatalf("ожидалась ошибка про отсутствие скобок у метода, получено: %v", err)
	}
}

func TestParseGenericTypeNameInNewExpr(t *testing.T) {
	prog, err := parseSource(t, `
функция главная {
    переменная числа = новый Список<Целое>()
}
`)
	if err != nil {
		t.Fatalf("неожиданная ошибка: %v", err)
	}
	varDecl := prog.Functions[0].Body[0].(*ast.VarDecl)
	newExpr, ok := varDecl.Value.(*ast.NewExpr)
	if !ok || newExpr.Type != "Список<Целое>" {
		t.Fatalf("ожидался NewExpr с типом \"Список<Целое>\", получено %+v", varDecl.Value)
	}
}

func TestParseGenericTypeNameInParamAndReturnType(t *testing.T) {
	prog, err := parseSource(t, `
функция обработать(кого: Список<Строка>): Список<Целое> { вернуть кого }
функция главная { вывести "ок" }
`)
	if err != nil {
		t.Fatalf("неожиданная ошибка: %v", err)
	}
	fn := prog.Functions[0]
	if fn.Params[0].Type != "Список<Строка>" {
		t.Fatalf("ожидался тип параметра \"Список<Строка>\", получено %q", fn.Params[0].Type)
	}
	if fn.ReturnType != "Список<Целое>" {
		t.Fatalf("ожидался тип результата \"Список<Целое>\", получено %q", fn.ReturnType)
	}
}

func TestParseForEachOverListResolvesPrepositionalCase(t *testing.T) {
	prog, err := parseSource(t, `
функция главная {
    переменная список = новый Список<Целое>()
    для x в списке {
        вывести x
    }
}
`)
	if err != nil {
		t.Fatalf("неожиданная ошибка: %v", err)
	}
	forEach, ok := prog.Functions[0].Body[1].(*ast.ForEachStmt)
	if !ok {
		t.Fatalf("ожидался ast.ForEachStmt, получено %+v", prog.Functions[0].Body[1])
	}
	coll, ok := forEach.Collection.(*ast.Identifier)
	if !ok || coll.Name != "список" {
		t.Fatalf("ожидалась ссылка на переменную \"список\" (лемма из \"списке\"), получено %+v", forEach.Collection)
	}
	if forEach.Var != "x" {
		t.Fatalf("ожидалась переменная цикла %q, получено %q", "x", forEach.Var)
	}
}

// TestParseForEachUnrecognizedWordformFallsBackToLiteral - словарь (даже
// вместе с tryDepluralizeMasculine, dictionary.go) не знает АБСОЛЮТНО
// каждое существительное - без словоформы вообще проверить падеж
// невозможно, поэтому парсер принимает голое имя КАК ЕСТЬ вместо ошибки
// (docs/ROADMAP.md, найденная находка - падежный словарь не распознавал
// "сервис"/"элемент"/"пользователь" и т. п. в этой позиции) - настоящая
// проверка ("такая переменная вообще объявлена?") откладывается на
// семантический анализ, где ей и место.
func TestParseForEachUnrecognizedWordformFallsBackToLiteral(t *testing.T) {
	prog, err := parseSource(t, `
функция главная {
    переменная список = новый Список<Целое>()
    для x в чтототам {
        вывести x
    }
}
`)
	if err != nil {
		t.Fatalf("неожиданная ошибка: %v", err)
	}
	loop := prog.Functions[0].Body[1].(*ast.ForEachStmt)
	coll, ok := loop.Collection.(*ast.Identifier)
	if !ok || coll.Name != "чтототам" {
		t.Fatalf("ожидался Collection == Identifier(\"чтототам\"), получено %+v", loop.Collection)
	}
}

func TestParseNaturalCallMixedTaggedAndPositional(t *testing.T) {
	// Пример из обсуждения: "кого пользователь: Пользователь" (падежная
	// роль) + "права: Права" (без роли, только позиционно). "пользователя"
	// связывается по падежу с "пользователь"; "админ" не подходит по
	// падежу ни одному ещё не занятому помеченному параметру (свободных
	// таких уже нет), поэтому достаётся оставшемуся непомеченному "права"
	// по остатку/позиции.
	prog, err := parseSource(t, `
сущность Пользователь
{
    имеет:
        имя: Строка
}
сущность Права
{
    имеет:
        уровень: Целое
}
функция оформить(кого пользователь: Пользователь, права: Права) { вывести пользователь.имя }
функция главная {
    переменная пользователь = новый Пользователь("Илья")
    переменная админ = новые Права(1)
    оформить пользователя админ
}
`)
	if err != nil {
		t.Fatalf("неожиданная ошибка: %v", err)
	}
	stmt := prog.Functions[1].Body[2].(*ast.ExprStmt)
	call := stmt.Call
	if call == nil || len(call.Args) != 2 {
		t.Fatalf("ожидался вызов \"оформить\" с двумя аргументами, получено %+v", stmt)
	}
	if id, ok := call.Args[0].(*ast.Identifier); !ok || id.Name != "пользователь" {
		t.Fatalf("ожидался первым аргументом \"пользователь\" (падежная роль \"кого\"), получено %+v", call.Args[0])
	}
	if id, ok := call.Args[1].(*ast.Identifier); !ok || id.Name != "админ" {
		t.Fatalf("ожидался вторым аргументом \"админ\" (по остатку/позиции), получено %+v", call.Args[1])
	}
}

func TestParseNaturalCallNotEnoughArgumentsAfterBothPasses(t *testing.T) {
	_, err := parseSource(t, `
функция оформить(кого: Строка, права: Строка) { вывести кого }
функция главная {
    переменная пользователь = "Илья"
    оформить пользователя
}
`)
	if err == nil {
		t.Fatalf("ожидалась ошибка про нехватку аргументов")
	}
}

// TestParseBareIdentifierNotConfusedWithSameNameFunction - параметр
// конструктора, названный так же, как отдельная функция файла ("первый"),
// используется как обычное значение ("этот.первый = первый") - не должен
// ошибочно приниматься за начало естественного вызова (тому неоткуда взять
// аргумент - следом сразу конец строки).
func TestParseBareIdentifierNotConfusedWithSameNameFunction(t *testing.T) {
	prog, err := parseSource(t, `
функция первый(список: Строка): Строка { вернуть список }
сущность Коробка
{
    имеет:
        первый: Строка

    конструктор(первый: Строка)
    {
        этот.первый = первый
    }
}
функция главная { переменная коробка = новая Коробка("а") }
`)
	if err != nil {
		t.Fatalf("неожиданная ошибка: %v", err)
	}
	fa := prog.Entities[0].Ctor.Body[0].(*ast.FieldAssign)
	id, ok := fa.Value.(*ast.Identifier)
	if !ok || id.Name != "первый" {
		t.Fatalf("ожидалась ссылка на параметр \"первый\" как обычный идентификатор, получено %+v", fa.Value)
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

func TestParseNewExprGenderAgreement(t *testing.T) {
	// "Сова" (женский род, окончание "-а"), "Окно" (средний род, окончание
	// "-о"), "Пользователь" (мужской род, курируемое исключение) - по одному
	// представителю каждого рода через ClassifyByEnding/исключения (без
	// обращения к gomorphy, чьи теги для нарицательных слов не всегда
	// однозначны - см. docs/MORPHOLOGY.md), плюс "Права" (plurale tantum,
	// число вместо рода).
	src := `
сущность Сова
{
    имеет:
        х: Целое
}
сущность Окно
{
    имеет:
        х: Целое
}
сущность Пользователь
{
    имеет:
        х: Целое
}
сущность Права
{
    имеет:
        х: Целое
}
функция главная {
    переменная а = новая Сова(1)
    переменная б = новое Окно(1)
    переменная д = новый Пользователь(1)
    переменная г = новые Права(1)
}
`
	if _, err := parseSource(t, src); err != nil {
		t.Fatalf("неожиданная ошибка: %v", err)
	}
}

func TestParseNewExprGenderMismatchFails(t *testing.T) {
	_, err := parseSource(t, `
сущность Сова
{
    имеет:
        х: Целое
}
функция главная {
    переменная а = новый Сова(1)
}
`)
	if err == nil || !strings.Contains(err.Error(), "требует форму") {
		t.Fatalf("ожидалась ошибка согласования рода, получено: %v", err)
	}
}

func TestParseNewExprUnclassifiableWordAcceptsAnyForm(t *testing.T) {
	// "Мнрдчь" оканчивается на "-ь" (Фаза A не может определить род без
	// курируемого исключения/словаря gomorphy) - обе формы должны
	// приниматься без ошибки (permissive fallback).
	for _, kw := range []string{"новая", "новое"} {
		src := `
сущность Мнрдчь
{
    имеет:
        х: Целое
}
функция главная {
    переменная а = ` + kw + ` Мнрдчь(1)
}
`
		if _, err := parseSource(t, src); err != nil {
			t.Fatalf("форма %q: неожиданная ошибка: %v", kw, err)
		}
	}
}

func TestParseNewExprLabeledArgs(t *testing.T) {
	prog, err := parseSource(t, `
сущность Пользователь
{
    имеет:
        имя: Строка
        возраст: Целое
}
функция главная {
    переменная п = новый Пользователь(возраст: 30, имя: "Илья")
}
`)
	if err != nil {
		t.Fatalf("неожиданная ошибка: %v", err)
	}
	varDecl := prog.Functions[0].Body[0].(*ast.VarDecl)
	newExpr, ok := varDecl.Value.(*ast.NewExpr)
	if !ok || len(newExpr.FieldNames) != 2 {
		t.Fatalf("ожидался NewExpr с именованными аргументами, получено %+v", varDecl.Value)
	}
	if newExpr.FieldNames[0] != "возраст" || newExpr.FieldNames[1] != "имя" {
		t.Fatalf("ожидался порядок меток [возраст имя], получено %v", newExpr.FieldNames)
	}
}

func TestParseNewExprMixedLabeledAndPositionalFails(t *testing.T) {
	_, err := parseSource(t, `
сущность Пользователь
{
    имеет:
        имя: Строка
        возраст: Целое
}
функция главная {
    переменная п = новый Пользователь("Илья", возраст: 30)
}
`)
	if err == nil || !strings.Contains(err.Error(), "нельзя смешивать именованные и позиционные аргументы") {
		t.Fatalf("ожидалась ошибка смешивания форм аргументов, получено: %v", err)
	}
}

func TestParseFlatTopLevelStatementsBecomeMain(t *testing.T) {
	prog, err := parseSource(t, `
переменная а = 5
вывести а
`)
	if err != nil {
		t.Fatalf("неожиданная ошибка: %v", err)
	}
	if len(prog.Functions) != 1 || prog.Functions[0].Name != "главная" {
		t.Fatalf("ожидалась ровно одна синтезированная \"главная\", получено %+v", prog.Functions)
	}
	if len(prog.Functions[0].Body) != 2 {
		t.Fatalf("ожидалось 2 инструкции в теле, получено %d", len(prog.Functions[0].Body))
	}
}

func TestParseFlatTopLevelStatementsInterleavedWithFunction(t *testing.T) {
	prog, err := parseSource(t, `
переменная а = 10
функция удвоить(x: Целое): Целое
{
    вернуть x * 2
}
вывести удвоить(а)
`)
	if err != nil {
		t.Fatalf("неожиданная ошибка: %v", err)
	}
	var main *ast.FunctionDecl
	for _, fn := range prog.Functions {
		if fn.Name == "главная" {
			main = fn
		}
	}
	if main == nil || len(main.Body) != 2 {
		t.Fatalf("ожидалась синтезированная \"главная\" из 2 инструкций, получено %+v", prog.Functions)
	}
}

func TestParseFlatTopLevelStatementsRequireSeparator(t *testing.T) {
	_, err := parseSource(t, `переменная а = 5 вывести а`)
	if err == nil || !strings.Contains(err.Error(), "должны быть разделены") {
		t.Fatalf("ожидалась ошибка про разделитель инструкций, получено: %v", err)
	}
}

func TestParseFlatTopLevelStatementsConflictWithExplicitMainFails(t *testing.T) {
	_, err := parseSource(t, `
функция главная
{
    вывести "явная"
}
вывести "лишнее"
`)
	if err == nil || !strings.Contains(err.Error(), "объявляет функцию") {
		t.Fatalf("ожидалась ошибка про конфликт явной \"главная\" с верхнеуровневыми инструкциями, получено: %v", err)
	}
}

func TestParseBitwiseOperatorsPrecedence(t *testing.T) {
	prog, err := parseSource(t, `
функция главная {
    переменная а = 1 | 2 ^ 3 & 4 << 5
}
`)
	if err != nil {
		t.Fatalf("неожиданная ошибка: %v", err)
	}
	decl := prog.Functions[0].Body[0].(*ast.VarDecl)
	bin, ok := decl.Value.(*ast.BinaryExpr)
	if !ok || bin.Op != "|" {
		t.Fatalf("ожидался верхний оператор \"|\" (самый слабый из побитовых), получено %+v", decl.Value)
	}
}

func TestParseIncDecPrefixAndPostfixAsExpression(t *testing.T) {
	prog, err := parseSource(t, `
функция главная {
    переменная а = 5
    вывести а++
    вывести ++а
}
`)
	if err != nil {
		t.Fatalf("неожиданная ошибка: %v", err)
	}
	post := prog.Functions[0].Body[1].(*ast.PrintStmt).Value.(*ast.IncDecExpr)
	if post.Prefix || post.Op != "++" || post.Name != "а" {
		t.Fatalf("ожидался постфикс а++, получено %+v", post)
	}
	pre := prog.Functions[0].Body[2].(*ast.PrintStmt).Value.(*ast.IncDecExpr)
	if !pre.Prefix || pre.Op != "++" || pre.Name != "а" {
		t.Fatalf("ожидался префикс ++а, получено %+v", pre)
	}
}

func TestParseIncDecAsStatement(t *testing.T) {
	prog, err := parseSource(t, `
функция главная {
    переменная а = 5
    а++
    --а
}
`)
	if err != nil {
		t.Fatalf("неожиданная ошибка: %v", err)
	}
	post := prog.Functions[0].Body[1].(*ast.ExprStmt).IncDec
	if post == nil || post.Prefix || post.Op != "++" {
		t.Fatalf("ожидался а++ как инструкция, получено %+v", prog.Functions[0].Body[1])
	}
	pre := prog.Functions[0].Body[2].(*ast.ExprStmt).IncDec
	if pre == nil || !pre.Prefix || pre.Op != "--" {
		t.Fatalf("ожидался --а как инструкция, получено %+v", prog.Functions[0].Body[2])
	}
}

func TestParseEntityConstantPlainIsPublic(t *testing.T) {
	prog, err := parseSource(t, `
сущность Настройки
{
	константа ВЕРСИЯ = 1
}
`)
	if err != nil {
		t.Fatalf("неожиданная ошибка: %v", err)
	}
	c := prog.Entities[0].Constants[0]
	if c.Name != "ВЕРСИЯ" || c.Visibility != ast.Public {
		t.Fatalf("ожидалась публичная константа ВЕРСИЯ, получено %+v", c)
	}
}

func TestParseEntityConstantExplicitVisibility(t *testing.T) {
	prog, err := parseSource(t, `
сущность Секрет
{
	приватная константа КОД = 1
	публичная константа ВЕРСИЯ = 2
}
`)
	if err != nil {
		t.Fatalf("неожиданная ошибка: %v", err)
	}
	if len(prog.Entities[0].Constants) != 2 {
		t.Fatalf("ожидалось 2 константы, получено %d", len(prog.Entities[0].Constants))
	}
	code := prog.Entities[0].Constants[0]
	if code.Name != "КОД" || code.Visibility != ast.Private {
		t.Fatalf("ожидалась приватная константа КОД, получено %+v", code)
	}
	version := prog.Entities[0].Constants[1]
	if version.Name != "ВЕРСИЯ" || version.Visibility != ast.Public {
		t.Fatalf("ожидалась публичная константа ВЕРСИЯ, получено %+v", version)
	}
}

func TestParseEntityConstantVisibilityMustAgreeWithConstWordNotName(t *testing.T) {
	_, err := parseSource(t, `
сущность Секрет
{
	приватный КОД = 1
}
`)
	if err == nil {
		t.Fatal("ожидалась ошибка - без слова \"константа\" форма \"приватный ИМЯ = ...\" больше не поддерживается")
	}
}

func TestParseEntityConstantVisibilityWrongFormRejected(t *testing.T) {
	_, err := parseSource(t, `
сущность Секрет
{
	приватный константа КОД = 1
}
`)
	if err == nil || !strings.Contains(err.Error(), "приватная") {
		t.Fatalf("ожидалась ошибка о требуемой форме \"приватная\" (согласование со словом \"константа\"), получено: %v", err)
	}
}

func TestParseCharLiteral(t *testing.T) {
	prog, err := parseSource(t, `
функция главная {
    переменная а = 'я'
}
`)
	if err != nil {
		t.Fatalf("неожиданная ошибка: %v", err)
	}
	decl := prog.Functions[0].Body[0].(*ast.VarDecl)
	lit, ok := decl.Value.(*ast.CharLiteral)
	if !ok || lit.Value != 'я' {
		t.Fatalf("ожидался CharLiteral('я'), получено %+v", decl.Value)
	}
}

func TestParseCastExpr(t *testing.T) {
	prog, err := parseSource(t, `
функция главная {
    переменная а = (Целое) "42"
}
`)
	if err != nil {
		t.Fatalf("неожиданная ошибка: %v", err)
	}
	decl := prog.Functions[0].Body[0].(*ast.VarDecl)
	cast, ok := decl.Value.(*ast.CastExpr)
	if !ok || cast.Type != "Целое" {
		t.Fatalf("ожидался CastExpr(Целое), получено %+v", decl.Value)
	}
	if _, ok := cast.Value.(*ast.StringLiteral); !ok {
		t.Fatalf("ожидалось строковое значение внутри каста, получено %+v", cast.Value)
	}
}

func TestParseCastNegativeOperandBindsAsUnary(t *testing.T) {
	prog, err := parseSource(t, `
функция главная {
    переменная а = (Целое) -3.5
}
`)
	if err != nil {
		t.Fatalf("неожиданная ошибка: %v", err)
	}
	decl := prog.Functions[0].Body[0].(*ast.VarDecl)
	cast, ok := decl.Value.(*ast.CastExpr)
	if !ok {
		t.Fatalf("ожидался CastExpr, получено %+v", decl.Value)
	}
	if _, ok := cast.Value.(*ast.UnaryExpr); !ok {
		t.Fatalf("ожидался унарный \"-\" внутри каста, получено %+v", cast.Value)
	}
}

func TestParseParenthesizedExprIsNotConfusedWithCast(t *testing.T) {
	prog, err := parseSource(t, `
функция главная {
    переменная а = (1 + 2) * 3
}
`)
	if err != nil {
		t.Fatalf("неожиданная ошибка: %v", err)
	}
	decl := prog.Functions[0].Body[0].(*ast.VarDecl)
	bin, ok := decl.Value.(*ast.BinaryExpr)
	if !ok || bin.Op != "*" {
		t.Fatalf("ожидалось обычное выражение в скобках, не каст, получено %+v", decl.Value)
	}
}

func TestParseStructDeclSetsIsStruct(t *testing.T) {
	prog, err := parseSource(t, `
структура Точка
{
    имеет:
        x: Целое
        y: Целое
}
функция главная { вывести "ок" }
`)
	if err != nil {
		t.Fatalf("неожиданная ошибка: %v", err)
	}
	if len(prog.Entities) != 1 || !prog.Entities[0].IsStruct {
		t.Fatalf("ожидалась одна сущность с IsStruct == true, получено %+v", prog.Entities)
	}
	if prog.Entities[0].Name != "Точка" || len(prog.Entities[0].Fields) != 2 {
		t.Fatalf("ожидалась структура \"Точка\" с двумя полями, получено %+v", prog.Entities[0])
	}
}

func TestParseEntityAndClassDeclLeaveIsStructFalse(t *testing.T) {
	prog, err := parseSource(t, `
сущность А
{
    имеет:
        x: Целое
}
класс Б
{
    имеет:
        x: Целое
}
функция главная { вывести "ок" }
`)
	if err != nil {
		t.Fatalf("неожиданная ошибка: %v", err)
	}
	if len(prog.Entities) != 2 || prog.Entities[0].IsStruct || prog.Entities[1].IsStruct {
		t.Fatalf("ожидалось IsStruct == false у \"сущность\"/\"класс\", получено %+v", prog.Entities)
	}
}

func TestParseStructErrorMessageUsesStructLabel(t *testing.T) {
	_, err := parseSource(t, `
структура Точка
{
    имеет:
        x: Целое
    если
}
функция главная { вывести "ок" }
`)
	if err == nil || !strings.Contains(err.Error(), "структура \"Точка\"") {
		t.Fatalf("ожидалась ошибка со словом \"структура\" (не \"сущность\"), получено: %v", err)
	}
}

func TestParseInterfaceDecl(t *testing.T) {
	prog, err := parseSource(t, `
интерфейс Звучащий
{
    издать_звук(): Строка
    замолчать()
}
функция главная { вывести "ок" }
`)
	if err != nil {
		t.Fatalf("неожиданная ошибка: %v", err)
	}
	if len(prog.Interfaces) != 1 {
		t.Fatalf("ожидался один интерфейс, получено %+v", prog.Interfaces)
	}
	iface := prog.Interfaces[0]
	if iface.Name != "Звучащий" || len(iface.Methods) != 2 {
		t.Fatalf("ожидался интерфейс \"Звучащий\" с двумя методами, получено %+v", iface)
	}
	if iface.Methods[0].Name != "издать_звук" || iface.Methods[0].ReturnType != "Строка" {
		t.Fatalf("ожидался метод издать_звук(): Строка, получено %+v", iface.Methods[0])
	}
	if iface.Methods[1].Name != "замолчать" || iface.Methods[1].ReturnType != "" {
		t.Fatalf("ожидался метод замолчать() без результата, получено %+v", iface.Methods[1])
	}
}

func TestParseEntityInheritsAndImplements(t *testing.T) {
	prog, err := parseSource(t, `
сущность Животное
{
    имеет:
        имя: Строка
}
сущность Собака наследует Животное реализует Звучащий, Бегущий
{
    имеет:
        порода: Строка
}
функция главная { вывести "ок" }
`)
	if err != nil {
		t.Fatalf("неожиданная ошибка: %v", err)
	}
	if len(prog.Entities) != 2 {
		t.Fatalf("ожидались две сущности, получено %+v", prog.Entities)
	}
	dog := prog.Entities[1]
	if dog.Name != "Собака" || dog.BaseName != "Животное" {
		t.Fatalf("ожидалось Собака наследует Животное, получено %+v", dog)
	}
	if len(dog.Interfaces) != 2 || dog.Interfaces[0] != "Звучащий" || dog.Interfaces[1] != "Бегущий" {
		t.Fatalf("ожидалось реализует Звучащий, Бегущий, получено %+v", dog.Interfaces)
	}
}

func TestParseStructRejectsInheritsAndImplements(t *testing.T) {
	_, err := parseSource(t, `
структура Точка наследует Что_то
{
    имеет:
        x: Целое
}
функция главная { вывести "ок" }
`)
	if err == nil || !strings.Contains(err.Error(), "структура") {
		t.Fatalf("ожидалась ошибка - структура не может наследовать, получено: %v", err)
	}

	_, err = parseSource(t, `
структура Точка реализует Что_то
{
    имеет:
        x: Целое
}
функция главная { вывести "ок" }
`)
	if err == nil || !strings.Contains(err.Error(), "структура") {
		t.Fatalf("ожидалась ошибка - структура не может реализовывать интерфейс, получено: %v", err)
	}
}

func TestParseAbstractEntity(t *testing.T) {
	prog, err := parseSource(t, `
абстрактная сущность Животное
{
    голос(): Строка

    представиться(): Строка
    {
        вернуть "аноним"
    }
}
функция главная { вывести "ок" }
`)
	if err != nil {
		t.Fatalf("неожиданная ошибка: %v", err)
	}
	ent := prog.Entities[0]
	if !ent.IsAbstract {
		t.Fatalf("ожидалась абстрактная сущность, получено %+v", ent)
	}
	if len(ent.Methods) != 2 {
		t.Fatalf("ожидались два метода, получено %+v", ent.Methods)
	}
	voice := ent.Methods[0]
	if voice.Name != "голос" || !voice.IsAbstract || voice.Body != nil {
		t.Fatalf("ожидался абстрактный метод \"голос\" без тела, получено %+v", voice)
	}
	greet := ent.Methods[1]
	if greet.Name != "представиться" || greet.IsAbstract || greet.Body == nil {
		t.Fatalf("ожидался обычный метод \"представиться\" с телом, получено %+v", greet)
	}
}

func TestParseAbstractClassAcceptsMasculineForm(t *testing.T) {
	prog, err := parseSource(t, `
абстрактный класс Фигура
{
    площадь(): Действительное
}
функция главная { вывести "ок" }
`)
	if err != nil {
		t.Fatalf("неожиданная ошибка: %v", err)
	}
	if !prog.Entities[0].IsAbstract {
		t.Fatalf("ожидался абстрактный класс, получено %+v", prog.Entities[0])
	}
}

func TestParseAbstractEntityWrongGenderFormRejected(t *testing.T) {
	_, err := parseSource(t, `
абстрактный сущность Животное
{
    голос(): Строка
}
функция главная { вывести "ок" }
`)
	if err == nil || !strings.Contains(err.Error(), "согласуется по роду") {
		t.Fatalf("ожидалась ошибка согласования по роду, получено: %v", err)
	}

	_, err = parseSource(t, `
абстрактная класс Фигура
{
    площадь(): Действительное
}
функция главная { вывести "ок" }
`)
	if err == nil || !strings.Contains(err.Error(), "согласуется по роду") {
		t.Fatalf("ожидалась ошибка согласования по роду, получено: %v", err)
	}
}

func TestParseAbstractStructRejected(t *testing.T) {
	_, err := parseSource(t, `
абстрактная структура Точка
{
    имеет:
        x: Целое
}
функция главная { вывести "ок" }
`)
	if err == nil || !strings.Contains(err.Error(), "структура") {
		t.Fatalf("ожидалась ошибка - структура не может быть абстрактной, получено: %v", err)
	}
}

func TestParseGenericEntityCannotBeAbstract(t *testing.T) {
	_, err := parseSource(t, `
абстрактная сущность Коробка<T>
{
    получить(): T
}
функция главная { вывести "ок" }
`)
	if err == nil || !strings.Contains(err.Error(), "generic") {
		t.Fatalf("ожидалась ошибка - generic-сущность не может быть абстрактной, получено: %v", err)
	}
}

func TestParseMethodWithoutBodyOutsideAbstractEntityRejected(t *testing.T) {
	_, err := parseSource(t, `
сущность Животное
{
    голос(): Строка
}
функция главная { вывести "ок" }
`)
	if err == nil || !strings.Contains(err.Error(), "не указано тело") {
		t.Fatalf("ожидалась ошибка - у метода не указано тело, получено: %v", err)
	}
}
