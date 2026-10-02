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
	if ref, ok := launch.Value.(*ir.FuncRefExpr); !ok || ref.FuncName != "посчитать" {
		t.Fatalf("ожидался запуск &посчитать, получено %#v", launch.Value)
	}
	if typ := ir.TypeOf(launch); typ != ir.TaskType("Вызываемый<>Целое") {
		t.Fatalf("ожидался тип задачи сигнатуры \"Вызываемый<>Целое\", получено %s", typ)
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
    переменная задачи = новый Список<Задача<Вызываемый(): Целое>>()
    задачи.добавить(запустить посчитать())

    для задача в задачи {
        переменная результат = дождаться задача
        вывести результат
    }
}
`)
	if err != nil {
		t.Fatalf("ожидалось, что тип Задача<Вызываемый(): Целое> распознаётся в коллекции и дожидается, получено: %v", err)
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
	if err == nil || !strings.Contains(err.Error(), "у задачи нет результата") {
		t.Fatalf("ожидалась ошибка про задачу без результата при использовании дождаться как значения, получено: %v", err)
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

// Тип задачи - сигнатура запущенного, поэтому перегруженная функция
// аннотируется так же, как любая другая: нужная перегрузка выбирается по
// аргументам, её сигнатура совпадает с аннотацией.
func TestAnalyzeTaskAnnotationOfOverloadedFunction(t *testing.T) {
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
    переменная задачи: Список<Задача<Вызываемый(Целое, Целое): Целое>> = []
    задачи.добавить(запустить посчитать(1, 2))
    переменная другая: Задача<Вызываемый(Строка): Целое> = запустить посчитать("х")
}
`)
	if err != nil {
		t.Fatalf("неожиданная ошибка: %v", err)
	}
}

func TestAnalyzeTaskAnnotationRejectsFunctionName(t *testing.T) {
	_, err := AnalyzeSource(t, `
функция посчитать(): Целое
{
    вернуть 1
}
функция главная
{
    переменная з: Задача<посчитать> = запустить посчитать()
}
`)
	if err == nil || !strings.Contains(err.Error(), "пишется сигнатура запущенного") {
		t.Fatalf("ожидалась ошибка про старую форму Задача<имя_функции>, получено: %v", err)
	}
}

// Задачи функции, лямбды из переменной, метода и корутины одной
// сигнатуры - один тип, одна коллекция.
func TestAnalyzeTasksOfOneSignatureShareType(t *testing.T) {
	_, err := AnalyzeSource(t, `
синоним Работа = Вызываемый(Целое): Целое

сущность Счётчик
{
    имеет:
        Целое шаг

    прибавить(число: Целое): Целое
    {
        вернуть число + этот.шаг
    }
}

функция удвоить(х: Целое): Целое
{
    вернуть х * 2
}

асинхронная функция утроить(х: Целое): Целое
{
    вернуть х * 3
}

функция главная
{
    переменная квадрат: Работа = функция(х: Целое): Целое { вернуть х * х }
    переменная счётчик = новый Счётчик()
    переменная задачи: Список<Задача<Работа>> = []
    задачи.добавить(запустить удвоить(1))
    задачи.добавить(запустить квадрат(2))
    задачи.добавить(запустить счётчик.прибавить(3))
    задачи.добавить(поручить утроить(4))
    для задача в задачах {
        переменная р: Целое = дождаться задача
    }
}
`)
	if err != nil {
		t.Fatalf("неожиданная ошибка: %v", err)
	}
}

func TestAnalyzeLaunchMethodCapturesReceiver(t *testing.T) {
	prog := parseProgram(t, `
сущность Журнал
{
    имеет:
        Целое записи

    записать()
    {
        этот.записи++
    }
}

функция главная
{
    переменная журнал = новый Журнал()
    переменная з = запустить журнал.записать()
    дождаться з
}
`)
	fn := findFunc(prog, "главная")
	launch := fn.Body[1].(*ir.VarDecl).Value.(*ir.LaunchExpr)
	lambda, ok := launch.Value.(*ir.LambdaExpr)
	if !ok || len(lambda.Captures) != 1 || lambda.Captures[0].Init == nil {
		t.Fatalf("метод должен запускаться обёрткой с инициализирующим захватом получателя, получено %#v", launch.Value)
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
	if ref, ok := launch.Value.(*ir.FuncRefExpr); !ok || ref.FuncName != "получить_данные" {
		t.Fatalf("ожидался запуск &получить_данные, получено %#v", launch.Value)
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

// TestAnalyzeCoroLaunchGivesSameTaskType - "поручить <асинхронная
// функция>()" даёт ir.LaunchExpr{Coroutine: true} с тем же типом задачи,
// что и "запустить" той же функции.
func TestAnalyzeCoroLaunchGivesSameTaskType(t *testing.T) {
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
	if typ := ir.TypeOf(launch); typ != ir.TaskType("Вызываемый<>Целое") {
		t.Fatalf("ожидался тип задачи сигнатуры \"Вызываемый<>Целое\", получено %s", typ)
	}
	// "дождаться задача" - Task здесь ПЕРЕМЕННАЯ (не прямой вызов) - общий
	// путь, не сахар analyzeAwaitTask.
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
