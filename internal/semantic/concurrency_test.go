package semantic

import (
	"strings"
	"testing"

	"github.com/IlyaFeoktistov/Rech/internal/ast"
	"github.com/IlyaFeoktistov/Rech/internal/ir"
)

func TestAnalyzeLaunchAwaitRoundTripsReturnType(t *testing.T) {
	prog := parseProgram(t, `
функция посчитать(): Целое
{
    вернуть 42
}
функция главная
{
    переменная задача = запустить посчитать()
    переменная результат = дождаться задача
    вывести результат
}
`)
	fn := findFunc(prog, "главная")
	decl, ok := fn.Body[0].(*ir.VarDecl)
	if !ok {
		t.Fatalf("ожидался VarDecl, получено %T", fn.Body[0])
	}
	launch, ok := decl.Value.(*ir.LaunchExpr)
	if !ok {
		t.Fatalf("ожидался LaunchExpr, получено %T", decl.Value)
	}
	if launch.Callee != "посчитать" {
		t.Fatalf("ожидался callee \"посчитать\", получено %q", launch.Callee)
	}
	if typ := ir.TypeOf(launch); typ != ir.TaskType("посчитать") {
		t.Fatalf("ожидался тип Задача<посчитать>, получено %s", typ)
	}
	result, ok := fn.Body[1].(*ir.VarDecl)
	if !ok {
		t.Fatalf("ожидался VarDecl, получено %T", fn.Body[1])
	}
	await, ok := result.Value.(*ir.AwaitExpr)
	if !ok {
		t.Fatalf("ожидался AwaitExpr, получено %T", result.Value)
	}
	if await.Type != ir.TypeInt || !await.HasReturn {
		t.Fatalf("ожидался тип Целое с результатом, получено %s (HasReturn=%v)", await.Type, await.HasReturn)
	}
}

func TestAnalyzeBareTaskTypeIsRecognizedInCollections(t *testing.T) {
	_, err := AnalyzeSource(t, `
функция посчитать(): Целое
{
    вернуть 42
}
функция главная
{
    переменная задачи = новый Список<Задача>()
    задачи.добавить(запустить посчитать())
}
`)
	if err != nil {
		t.Fatalf("ожидалось, что тип Задача распознаётся в коллекции, получено: %v", err)
	}
}

func TestAnalyzeConcreteTaskTypeIsRecognizedInCollectionsAndAwaited(t *testing.T) {
	_, err := AnalyzeSource(t, `
функция посчитать(): Целое
{
    вернуть 42
}
функция главная
{
    переменная задачи = новый Список<Задача<посчитать>>()
    задачи.добавить(запустить посчитать())

    для задача в задачи {
        переменная результат = дождаться задача
        вывести результат
    }
}
`)
	if err != nil {
		t.Fatalf("ожидалось, что тип Задача<посчитать> распознаётся в коллекции и дожидается, получено: %v", err)
	}
}

func TestAnalyzeAwaitWithoutReturnUsableOnlyAsStatement(t *testing.T) {
	prog := parseProgram(t, `
функция вывести_привет()
{
    вывести "привет"
}
функция главная
{
    переменная задача = запустить вывести_привет()
    дождаться задача
}
`)
	fn := findFunc(prog, "главная")
	stmt, ok := fn.Body[1].(*ir.ExprStmt)
	if !ok || stmt.Await == nil {
		t.Fatalf("ожидался ExprStmt{Await}, получено %T", fn.Body[1])
	}

	_, err := AnalyzeSource(t, `
функция вывести_привет()
{
    вывести "привет"
}
функция главная
{
    переменная задача = запустить вывести_привет()
    переменная х = дождаться задача
}
`)
	if err == nil || !strings.Contains(err.Error(), "абстрактный тип \"Задача\"") {
		t.Fatalf("ожидалась ошибка про абстрактный тип Задача при использовании дождаться как значения, получено: %v", err)
	}
}

func TestAnalyzeLaunchRejectsBuiltinFunction(t *testing.T) {
	_, err := AnalyzeSource(t, `
функция главная
{
    переменная задача = запустить корень(4.0)
}
`)
	if err == nil || !strings.Contains(err.Error(), "встроенные функции") {
		t.Fatalf("ожидалась ошибка о встроенных функциях, получено: %v", err)
	}
}

func TestAnalyzeAwaitRejectsAbstractTaskHandle(t *testing.T) {
	sc := newCtx(nil, map[string]funcSig{}, map[string]entitySig{}, map[string]map[string]funcSig{}, map[string]string{}, map[string]interfaceSig{}, map[string]map[string]int64{}, "", ir.TypeUnknown, false, map[string]ir.Expr{}, map[string]bool{})
	sc.scopes[0]["задача"] = ir.TaskType("")

	_, err := analyzeAwaitExpr(sc, &ast.AwaitExpr{Task: &ast.Identifier{Name: "задача"}, Line: 1})
	if err == nil || !strings.Contains(err.Error(), "абстрактный тип \"Задача\"") {
		t.Fatalf("ожидалась ошибка про абстрактный тип Задача, получено: %v", err)
	}
}

func TestAnalyzeAwaitRejectsNonTaskValue(t *testing.T) {
	_, err := AnalyzeSource(t, `
функция главная
{
    переменная х = 5
    переменная y = дождаться х
}
`)
	if err == nil || !strings.Contains(err.Error(), "ожидает значение задачи") {
		t.Fatalf("ожидалась ошибка о том, что дождаться ожидает задачу, получено: %v", err)
	}
}

func TestAnalyzeAwaitRejectsBareTaskTypeFromCollection(t *testing.T) {
	_, err := AnalyzeSource(t, `
функция посчитать(): Целое
{
    вернуть 42
}
функция главная
{
    переменная задачи = новый Список<Задача>()
    задачи.добавить(запустить посчитать())
    переменная r = дождаться задачи[0]
}
`)
	if err == nil || !strings.Contains(err.Error(), "ожидает значение задачи") {
		t.Fatalf("ожидалась ошибка про абстрактный тип Задача, получено: %v", err)
	}
}

// TestAnalyzeExplicitTaskAnnotationRejectsOverloadedCallee - РЕГРЕССИЯ
// найденной при живом прогоне путаницы: "Задача<T>" в явной аннотации
// несёт ГОЛОЕ имя T, а настоящий "запустить T(...)" перегруженной функции
// сам резолвит нужную перегрузку по аргументам вызова и несёт УЖЕ
// манглированное имя ("T(Целое,Целое)") - до этой проверки итог был не
// "неизвестное имя", а путающее "объявлена типом Задача<T>, но
// инициализирована значением типа Задача<T(Целое,Целое)>". Спеллинга для
// манглированного имени как типа не существует вовсе (скобки внутри
// "<...>" не входят в грамматику type_name) - явная аннотация для
// перегруженной функции сейчас в принципе невозможна, ошибка должна
// сказать это прямо, а не изображать несовпадение типов.
func TestAnalyzeExplicitTaskAnnotationRejectsOverloadedCallee(t *testing.T) {
	_, err := AnalyzeSource(t, `
функция посчитать(a: Целое, b: Целое): Целое
{
    вернуть a + b
}
функция посчитать(a: Строка): Целое
{
    вернуть 0
}
функция главная
{
    переменная x: Задача<посчитать> = запустить посчитать(1, 2)
}
`)
	if err == nil || !strings.Contains(err.Error(), "перегружена") {
		t.Fatalf("ожидалась ошибка о запрете явной аннотации для перегруженной функции, получено: %v", err)
	}
}

// TestAnalyzeExplicitTaskAnnotationRejectsOverloadedCalleeInsideList -
// тот же запрет, что и TestAnalyzeExplicitTaskAnnotationRejectsOverloadedCallee
// выше, но T встречается ВЛОЖЕННО ("Список<Задача<T>>", не голая "Задача<T>"
// напрямую) - проверяет, что overloadedTaskAnnotationCallee ищет "Задача<T>"
// ГДЕ УГОДНО в строке аннотации, не только как annotation целиком.
func TestAnalyzeExplicitTaskAnnotationRejectsOverloadedCalleeInsideList(t *testing.T) {
	_, err := AnalyzeSource(t, `
функция посчитать(a: Целое, b: Целое): Целое
{
    вернуть a + b
}
функция посчитать(a: Строка): Целое
{
    вернуть 0
}
функция главная
{
    переменная задачи: Список<Задача<посчитать>> = новый Список<Задача<посчитать>>()
}
`)
	if err == nil || !strings.Contains(err.Error(), "перегружена") {
		t.Fatalf("ожидалась ошибка о запрете явной аннотации для перегруженной функции, получено: %v", err)
	}
}

func TestAnalyzeAwaitOfAsyncFunctionDesugarsToLaunch(t *testing.T) {
	prog := parseProgram(t, `
асинхронная функция получить_данные(): Целое
{
    вернуть 42
}
функция главная
{
    переменная результат = дождаться получить_данные()
    вывести результат
}
`)
	fn := findFunc(prog, "главная")
	decl, ok := fn.Body[0].(*ir.VarDecl)
	if !ok {
		t.Fatalf("ожидался VarDecl, получено %T", fn.Body[0])
	}
	await, ok := decl.Value.(*ir.AwaitExpr)
	if !ok {
		t.Fatalf("ожидался AwaitExpr, получено %T", decl.Value)
	}
	launch, ok := await.Task.(*ir.LaunchExpr)
	if !ok {
		t.Fatalf("\"дождаться <асинхронный вызов>()\" должен десугариться в LaunchExpr, получено %T", await.Task)
	}
	if launch.Callee != "получить_данные" {
		t.Fatalf("ожидался callee \"получить_данные\", получено %q", launch.Callee)
	}
	if !launch.Coroutine {
		t.Fatalf("сахар \"дождаться <асинхронная функция>()\" должен давать LaunchExpr.Coroutine == true")
	}
	if await.Type != ir.TypeInt || !await.HasReturn {
		t.Fatalf("ожидался тип Целое с результатом, получено %s (HasReturn=%v)", await.Type, await.HasReturn)
	}
}

func TestAnalyzeExplicitLaunchOfAsyncFunctionStaysOnThread(t *testing.T) {
	prog := parseProgram(t, `
асинхронная функция получить_данные(): Целое
{
    вернуть 42
}
функция главная
{
    переменная задача = запустить получить_данные()
    переменная результат = дождаться задача
    вывести результат
}
`)
	fn := findFunc(prog, "главная")
	decl, ok := fn.Body[0].(*ir.VarDecl)
	if !ok {
		t.Fatalf("ожидался VarDecl, получено %T", fn.Body[0])
	}
	launch, ok := decl.Value.(*ir.LaunchExpr)
	if !ok {
		t.Fatalf("ожидался LaunchExpr, получено %T", decl.Value)
	}
	if launch.Coroutine {
		t.Fatalf("явный \"запустить\" на асинхронной функции обязан оставаться на потоке ОС (Coroutine == false) - модель под ним не меняется никогда")
	}
}

func TestDirectCallOfAsyncFunctionStaysSynchronous(t *testing.T) {
	prog := parseProgram(t, `
асинхронная функция получить_данные(): Целое
{
    вернуть 42
}
функция главная
{
    переменная результат = получить_данные()
    вывести результат
}
`)
	fn := findFunc(prog, "главная")
	decl, ok := fn.Body[0].(*ir.VarDecl)
	if !ok {
		t.Fatalf("ожидался VarDecl, получено %T", fn.Body[0])
	}
	if _, ok := decl.Value.(*ir.CallExpr); !ok {
		t.Fatalf("прямой вызов \"асинхронная функция\" без \"дождаться\" должен остаться обычным CallExpr, получено %T", decl.Value)
	}
}

func TestAnalyzeAsyncFunctionOverloadRejected(t *testing.T) {
	_, err := AnalyzeSource(t, `
асинхронная функция получить_данные(): Целое
{
    вернуть 1
}
асинхронная функция получить_данные(параметр: Строка): Целое
{
    вернуть 2
}
функция главная { вывести "ок" }
`)
	if err == nil || !strings.Contains(err.Error(), "перегрузка не поддерживается для \"асинхронная\"") {
		t.Fatalf("ожидалась ошибка о запрете перегрузки для \"асинхронная\", получено: %v", err)
	}
}

// TestAnalyzeAsyncFunctionMayAwaitAnotherAsyncFunctionWhenCalledDirectly -
// "асинхронная функция" вправе свободно звать "дождаться <другая
// асинхронная функция>()" в своём теле - НЕ отклоняется статически (см.
// package doc analyzeAwaitTask про то, почему лексическая проверка здесь
// была бы неверной: вложенность корутин - динамическое условие, зависящее
// от того, КАК вызвана объемлющая функция в конкретный раз, не от того,
// объявлена ли она "асинхронная"). Прямой вызов (без "дождаться") -
// синхронный, не на корутине вовсе - её собственное "дождаться" внутри
// абсолютно безопасно.
func TestAnalyzeAsyncFunctionMayAwaitAnotherAsyncFunctionWhenCalledDirectly(t *testing.T) {
	prog := parseProgram(t, `
асинхронная функция б(): Целое
{
    вернуть 2
}
асинхронная функция а(): Целое
{
    вернуть дождаться б()
}
функция главная
{
    переменная результат = а()
    вывести результат
}
`)
	if prog == nil {
		t.Fatalf("ожидалась успешная компиляция")
	}
}

func TestAnalyzeLaunchArgTypeMismatchFails(t *testing.T) {
	_, err := AnalyzeSource(t, `
функция сложить(а: Целое, б: Целое): Целое
{
    вернуть а + б
}
функция главная
{
    переменная задача = запустить сложить("не число", 2)
}
`)
	if err == nil || !strings.Contains(err.Error(), "должен быть типа") {
		t.Fatalf("ожидалась ошибка типа аргумента, получено: %v", err)
	}
}

// TestAnalyzeCoroLaunchRejectsNonAsyncFunction - "поручить" (Шаг 132,
// в отличие от "запустить") поддерживает только "асинхронная функция".
func TestAnalyzeCoroLaunchRejectsNonAsyncFunction(t *testing.T) {
	_, err := AnalyzeSource(t, `
функция обычная(): Целое
{
    вернуть 1
}
функция главная
{
    переменная задача = поручить обычная()
}
`)
	if err == nil || !strings.Contains(err.Error(), "поддерживает только \"асинхронная функция\"") {
		t.Fatalf("ожидалась ошибка о том, что \"поручить\" поддерживает только асинхронные функции, получено: %v", err)
	}
}

// TestAnalyzeCoroLaunchGivesCoroTaskType - "поручить <асинхронная
// функция>()" даёт ir.LaunchExpr{Coroutine: true} (тот же узел, что и
// сахар "дождаться", см. ir.KindCoroTask) - тип результата отличим от
// обычного ir.KindTask (потока ОС), хотя строковое имя ("Задача<...>")
// то же самое (невидимо для Rech-кода).
func TestAnalyzeCoroLaunchGivesCoroTaskType(t *testing.T) {
	prog := parseProgram(t, `
асинхронная функция получить_данные(): Целое
{
    вернуть 42
}
функция главная
{
    переменная задача = поручить получить_данные()
    переменная результат = дождаться задача
    вывести результат
}
`)
	fn := findFunc(prog, "главная")
	decl, ok := fn.Body[0].(*ir.VarDecl)
	if !ok {
		t.Fatalf("ожидался VarDecl, получено %T", fn.Body[0])
	}
	launch, ok := decl.Value.(*ir.LaunchExpr)
	if !ok {
		t.Fatalf("ожидался LaunchExpr, получено %T", decl.Value)
	}
	if !launch.Coroutine {
		t.Fatalf("\"поручить\" должен давать LaunchExpr.Coroutine == true")
	}
	if typ := ir.TypeOf(launch); typ.Kind != ir.KindCoroTask {
		t.Fatalf("ожидался ir.KindCoroTask, получено %s (Kind=%v)", typ, typ.Kind)
	}
	// "дождаться задача" - Task здесь ПЕРЕМЕННАЯ (не прямой вызов), должен
	// разрешиться через тот же общий путь (KindCoroTask), не через сахар
	// analyzeAwaitTask - см. её package doc.
	result, ok := fn.Body[1].(*ir.VarDecl)
	if !ok {
		t.Fatalf("ожидался VarDecl, получено %T", fn.Body[1])
	}
	await, ok := result.Value.(*ir.AwaitExpr)
	if !ok {
		t.Fatalf("ожидался AwaitExpr, получено %T", result.Value)
	}
	if await.Type != ir.TypeInt || !await.HasReturn {
		t.Fatalf("ожидался тип Целое с результатом, получено %s (HasReturn=%v)", await.Type, await.HasReturn)
	}
	if _, ok := await.Task.(*ir.Identifier); !ok {
		t.Fatalf("ожидался Task == *ir.Identifier (переменная), получено %T", await.Task)
	}
}

// TestAnalyzeCoroLaunchRejectsBuiltinFunction - та же граница, что и у
// "запустить" (analyzeLaunchExpr переиспользуется целиком).
func TestAnalyzeCoroLaunchRejectsBuiltinFunction(t *testing.T) {
	_, err := AnalyzeSource(t, `
функция главная
{
    переменная задача = поручить корень(4.0)
}
`)
	if err == nil || !strings.Contains(err.Error(), "встроенные функции") {
		t.Fatalf("ожидалась ошибка о встроенных функциях, получено: %v", err)
	}
}
