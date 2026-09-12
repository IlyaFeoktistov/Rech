// Мини-отладчик - доступ к памяти уже запущенного процесса по pid.
// Переиспользует уже существующую сущность "Процесс" (registerSubprocessFuncs,
// subprocess.go) - тот же непрозрачный дескриптор "только pid", что и у
// "запустить_процесс": с точки зрения этих функций неважно, был процесс
// запущен САМОЙ программой или получен по чужому pid - платформенное
// представление решает каждый backend сам (см. package doc internal/
// backend/cbackend/procmem.go).
//
// "прочитать"/"записать" (метод-сахар handle_methods.go) читают/пишут
// произвольное количество байт по абсолютному адресу в адресном
// пространстве процесса - результат/аргумент "Список<Байт>", тем же
// типом, что и бинарно-безопасный файловый ввод-вывод (internal/semantic/
// bytesio.go) - тут нет ни малейшего знания о том, что лежит по адресу
// (структура, строка, число), это дело вызывающей программы.
//
// Платформы - Linux (ptrace(2) + process_vm_readv/writev) и Windows
// (OpenProcess + ReadProcessMemory/WriteProcessMemory) - остальные (macOS
// и т. д.) собираются, но паникуют с понятным сообщением при попытке
// реально подключиться (см. internal/interp/procmem_other.go) - тот же
// принцип, что и "отправить_сигнал" с сигналом, которого нет на текущей
// ОС (internal/interp/signals.go, sigUnsupported).
package semantic

import "github.com/IlyaFeoktistov/Rech/internal/ir"

// procMemFuncNames - имена функций этого файла, работающих с
// "Список<Байт>" - используется analyzeCallExpr (expr.go) для решения,
// нужно ли звать registerByteListTypeOnCall при разборе конкретного
// вызова (тем же принципом, что bytesIOFuncNames, internal/semantic/
// bytesio.go). "найти_байты" тоже сюда входит - параметр "шаблон" того же
// типа, хотя результат - другой (intListType, ниже).
var procMemFuncNames = map[string]bool{
	"прочитать_память": true,
	"записать_память":  true,
	"найти_байты":      true,
}

// intListType - "Список<Целое>", результат "найти_байты" (адреса
// совпадений) - тем же принципом ленивой регистрации, что и byteListType
// (bytesio.go).
const intListType = "Список<Целое>"

// registerIntListTypeOnCall - вызывается из analyzeCallExpr при вызове
// "найти_байты" - тем же принципом, что registerByteListTypeOnCall.
func registerIntListTypeOnCall(entities map[string]entitySig, methods map[string]map[string]funcSig, aliases map[string]string, interfaces map[string]interfaceSig, enums map[string]map[string]int64) error {
	_, err := resolveTypeName(entities, methods, aliases, interfaces, enums, intListType)
	return err
}

// registerProcMemFuncs регистрирует четыре встроенные функции работы с
// памятью чужого процесса (docs/SPEC.md) - на уже зарегистрированной
// (registerSubprocessFuncs) сущности "Процесс":
//   - "подключиться_к_процессу(pid: Целое): Процесс" - подключается к уже
//     запущенному процессу по pid (Linux - ptrace(PTRACE_ATTACH), процесс
//     при этом останавливается, тем же способом, каким это делает
//     настоящий отладчик; Windows - только проверка, что процесс с таким
//     pid существует и доступен, без реальной остановки - см. package doc
//     internal/backend/cbackend/procmem.go, там же честно про эту
//     асимметрию платформ).
//   - "отключиться_от_процесса(процесс: Процесс)" - отключается (Linux -
//     PTRACE_DETACH, процесс возобновляется).
//   - "прочитать_память(процесс: Процесс, адрес: Целое, размер: Целое):
//     Список<Байт>" - читает "размер" байт начиная с "адреса" в адресном
//     пространстве процесса.
//   - "записать_память(процесс: Процесс, адрес: Целое, байты:
//     Список<Байт>)" - пишет байты по "адресу".
func registerProcMemFuncs(funcs map[string]funcSig) {
	funcs["подключиться_к_процессу"] = funcSig{
		Params:     []ir.Type{ir.TypeInt},
		ReturnType: ir.EntityType("Процесс"),
		HasReturn:  true,
	}
	funcs["отключиться_от_процесса"] = funcSig{
		Params: []ir.Type{ir.EntityType("Процесс")},
	}
	funcs["прочитать_память"] = funcSig{
		Params:     []ir.Type{ir.EntityType("Процесс"), ir.TypeInt, ir.TypeInt},
		ReturnType: ir.EntityType(byteListType),
		HasReturn:  true,
	}
	funcs["записать_память"] = funcSig{
		Params: []ir.Type{ir.EntityType("Процесс"), ir.TypeInt, ir.EntityType(byteListType)},
	}
	// "найти_байты(процесс: Процесс, шаблон: Список<Байт>): Список<Целое>" -
	// сканирует ВСЕ читаемые страницы памяти процесса, возвращает адреса
	// всех вхождений "шаблона" (см. package doc internal/backend/cbackend/
	// procmem.go, procMemFindHelper - там же про ограничение на размер
	// одного сканируемого региона).
	funcs["найти_байты"] = funcSig{
		Params:     []ir.Type{ir.EntityType("Процесс"), ir.EntityType(byteListType)},
		ReturnType: ir.EntityType(intListType),
		HasReturn:  true,
	}
}
