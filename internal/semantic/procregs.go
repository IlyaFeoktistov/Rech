// Мини-отладчик, tier3 - регистры процессора и точки останова - см. package
// doc internal/semantic/procmem.go (tier1/tier2) для общего контекста
// (сущность "Процесс" - только pid, честная асимметрия платформ).
//
// "Регистры" - сущность с полями общего назначения x86-64 (rax..r15, rip,
// eflags) - тот же набор имён, что использует и Linux (struct user_regs_
// struct, glibc/ptrace(2)), и Windows (CONTEXT, WinNT.h) для amd64 - не
// выдуманное имя, стандартная терминология архитектуры. Только amd64 -
// arm64 (Apple Silicon macOS, Windows on ARM) не поддержан вовсе (macOS и
// так не поддержан для всего мини-отладчика - см. package doc procmem.go;
// Windows on ARM - самостоятельная тема, не согласована).
//
// "продолжить" - уже ключевое слово (break/continue цикла) - метод назван
// "возобновить" (то же самое, что уже сделано для "дождаться_завершения"
// вместо голого "дождаться", handle_methods.go).
//
// "возобновить" САМ разбирается с уже АРМИРОВАННОЙ точкой останова под
// текущим RIP (если он там есть) - восстанавливает исходный байт,
// пошагово выполняет ОДНУ настоящую инструкцию, заново пишет 0xCC - и
// только потом по-настоящему продолжает выполнение до следующей точки
// останова/сигнала/завершения процесса. Вызывающему коду не нужно знать
// про эту пляску - "поставил точку останова - вызывай возобновить в
// цикле, получай адрес остановки" - тот же высокоуровневый контракт, что
// у любого настоящего отладчика.
package semantic

import "github.com/IlyaFeoktistov/Rech/internal/ir"

// RegistersFields - поля встроенной сущности "Регистры" - общие регистры
// amd64 (см. package doc выше).
var RegistersFields = []ir.Field{
	{Name: "rax", Type: ir.TypeInt},
	{Name: "rbx", Type: ir.TypeInt},
	{Name: "rcx", Type: ir.TypeInt},
	{Name: "rdx", Type: ir.TypeInt},
	{Name: "rsi", Type: ir.TypeInt},
	{Name: "rdi", Type: ir.TypeInt},
	{Name: "rbp", Type: ir.TypeInt},
	{Name: "rsp", Type: ir.TypeInt},
	{Name: "rip", Type: ir.TypeInt},
	{Name: "r8", Type: ir.TypeInt},
	{Name: "r9", Type: ir.TypeInt},
	{Name: "r10", Type: ir.TypeInt},
	{Name: "r11", Type: ir.TypeInt},
	{Name: "r12", Type: ir.TypeInt},
	{Name: "r13", Type: ir.TypeInt},
	{Name: "r14", Type: ir.TypeInt},
	{Name: "r15", Type: ir.TypeInt},
	{Name: "eflags", Type: ir.TypeInt},
}

// registerProcRegsFuncs регистрирует сущность "Регистры" и пять встроенных
// функций (docs/SPEC.md):
//   - "регистры_процесса(процесс: Процесс): Регистры" - читает текущие
//     регистры остановленного процесса.
//   - "установить_регистры_процесса(процесс: Процесс, регистры:
//     Регистры)" - пишет регистры обратно (типичный мотив - поменять
//     rip, чтобы перенаправить выполнение).
//   - "установить_точку_останова(процесс: Процесс, адрес: Целое)" -
//     ставит программную точку останова (int3) по адресу.
//   - "снять_точку_останова(процесс: Процесс, адрес: Целое)" - снимает
//     (восстанавливает исходный байт).
//   - "возобновить_процесс(процесс: Процесс): Целое" - продолжает
//     выполнение до следующей остановки, возвращает адрес остановки
//     (-1 - процесс завершился).
func registerProcRegsFuncs(entities map[string]entitySig, funcs map[string]funcSig) {
	registerHTTPEntity(entities, "Регистры", RegistersFields)
	funcs["регистры_процесса"] = funcSig{
		Params:     []ir.Type{ir.EntityType("Процесс")},
		ReturnType: ir.EntityType("Регистры"),
		HasReturn:  true,
	}
	funcs["установить_регистры_процесса"] = funcSig{
		Params: []ir.Type{ir.EntityType("Процесс"), ir.EntityType("Регистры")},
	}
	funcs["установить_точку_останова"] = funcSig{
		Params: []ir.Type{ir.EntityType("Процесс"), ir.TypeInt},
	}
	funcs["снять_точку_останова"] = funcSig{
		Params: []ir.Type{ir.EntityType("Процесс"), ir.TypeInt},
	}
	funcs["возобновить_процесс"] = funcSig{
		Params:     []ir.Type{ir.EntityType("Процесс")},
		ReturnType: ir.TypeInt,
		HasReturn:  true,
	}
}
