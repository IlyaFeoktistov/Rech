// GenerateFromIR - единственный C backend Rech, транслирует
// internal/ir.Program (уже полностью проверенный internal/semantic) в
// исходник на C. Вызывается напрямую из internal/compiler.
//
// Использует общую для пакета инфраструктуру, не зависящую от IR как
// такового (mangleName/mangleVar/mangleType/mangleField/mangleMethod,
// valueType, printCall, escapeCString, formatFloatLiteral, indent,
// builtinCallCode, generic-коллекции - см. соседние файлы пакета).
package cbackend

import (
	"fmt"
	"strings"

	"github.com/IlyaFeoktistov/Rech/internal/backend/cbackend/codegen"
	"github.com/IlyaFeoktistov/Rech/internal/ir"
	"github.com/IlyaFeoktistov/Rech/internal/rtmsg"
)

// irGenCtx - состояние генерации из IR: флаги использованных
// runtime-хелперов (как ctx.usesConcat/usesStrCmp),
// сущности программы по имени (нужно NewExpr - её Args позиционны, без
// имён полей, см. package doc ir.NewExpr - имена нужны только здесь, для
// designated initializer в C) и имя сущности-владельца метода, который
// сейчас генерируется ("" - вне метода, thisEntity делает ThisExpr
// типизируемым без внешнего контекста, как ctx.thisEntity в AST-пути).
// Символьной таблицы переменных здесь, в отличие от AST-пути, нет вообще -
// IR-узлы (Identifier.Type, BinaryExpr.Type) уже несут тип, разрешённый
// internal/semantic, генератору достаточно его прочитать, а не искать по
// имени - в этом и смысл шага 5 (тип решается один раз, не в каждом
// backend'е заново).
type irGenCtx struct {
	usesConcat bool
	// builtinFlags - единая таблица диспетчеризации builtinCallCode
	// (usesPanic из неё же покрывает "утверждать"/деление на ноль -
	// generic-коллекции учтены отдельно через usesGenerics/prog.Generics в
	// GenerateFromIR, не через этот флаг).
	builtinFlags
	// intDivMod/uintDivMod - какая именно комбинация (ширина × "/" или "%")
	// целочисленного деления реально понадобилась программе (см.
	// generateIRArithmetic) - по одному биту на комбинацию, не общий флаг на
	// всё семейство: rech_int_divN/rech_int_modN - НАСТОЯЩИЙ C-тип своей
	// ширины (int8_t/int16_t/int32_t/long long), не общий "widen до 64 бит и
	// усечь обратно" - неиспользуемая static-функция другой ширины/операции
	// была бы предупреждением (потенциально ошибкой при -Werror) у
	// настоящего cc, значит эмитировать нужно ТОЧНО использованные
	// комбинации. intDivMod - для Целое/Целое8/16/32/64 (Целое и Целое64
	// делят один и тот же 64-битный хелпер, div64/mod64 - оба на "long long",
	// это один и тот же C-тип по значению); uintDivMod - для Натуральное/
	// Натуральное8/16/32/64 (Натуральное и Натуральное64 аналогично делят
	// uint64-хелпер).
	intDivMod  intDivModUsage
	uintDivMod intDivModUsage
	// usesShow - нужны ли функции показа (rech_show_...) - генерируются
	// только если "показать" реально встретилось в программе.
	usesShow bool
	// structEqEntities - какие именно "структуры" программы нуждаются в
	// rech_struct_eq_... (struct_eq.go - "==" для
	// структур) - В ОТЛИЧИЕ от usesShow/showFuncs выше (генерируются для
	// ВСЕХ сущностей программы разом, раз показ умеет напечатать ЛЮБОЕ
	// поле каким-нибудь способом, generateIRShowExpr), здесь так нельзя:
	// semantic (structFieldsEquatable, internal/semantic/cast.go) проверяет
	// поля ТОЛЬКО той структуры, что реально сравнивается "==" (и,
	// рекурсивно, вложенные структуры-поля) - непроверенная сторонняя
	// структура программы (например, внутренний builtin-тип с nullable-
	// полем) не переживёт слепую попытку сравнить её поля через "==" -
	// значит генерировать можно ТОЛЬКО для того множества, что
	// registerStructEqEntity реально собрал (struct_eq.go), не для всех
	// prog.Entities. nil - "==" структуры ни разу не встретился.
	structEqEntities map[string]bool
	// usesDictString - хотя бы один "Словарь<Строка, ...>" использован в
	// программе - хеш/сравнение ключа-строки (dictCollectionCCode)
	// используют strcmp, значит нужен "#include <string.h>" - отдельный от
	// usesGenerics флаг (не каждый generic-тип - строковый Словарь), тем же
	// принципом осторожности, что и у остальных #include-флагов: включать
	// string.h без нужды означает риск конфликта с "внешняя функция",
	// объявляющей настоящие strlen/strcmp/... с несовпадающими типами.
	usesDictString bool
	// usesCast - нужны ли рантайм-хелперы явных кастов (castHelpers) - только
	// для приведений с участием "Строка" (разбор/форматирование); прямые
	// числовые приведения (Целое/Дробное/Действительное/Символ/Байт между
	// собой) - обычный C-каст, без хелперов.
	usesCast bool
	// includesStringH/includesStdlibH/includesMathH - взведены (один раз,
	// после генерации ВСЕХ тел функций/методов - см. generateSource) в
	// точности когда сама программа реально включит "#include <string.h>"/
	// "<stdlib.h>"/"<math.h>" по СОБСТВЕННЫМ причинам (usesMemcpy/usesCast/
	// usesMath/... - см. их же сборку ниже). externPrototypeOfIR
	// (extern_headers.go, externPrototypeAlreadyProvided) сверяется с ними,
	// чтобы не писать собственный extern-прототип для стандартного символа
	// (strlen/malloc/sqrt/...), который заголовок и так уже объявляет -
	// иначе два расходящихся по типу прототипа ОДНОГО имени - ошибка
	// компиляции C (найдено живьём, "FFI: конфликт
	// extern-прототипов со стандартными заголовками").
	includesStringH bool
	includesStdlibH bool
	includesMathH   bool
	// usesGC - нужен ли Boehm GC (#include <gc.h>, GC_INIT(), см.
	// generateIREntityCtor) - только если в программе есть хотя бы одна
	// ссылочная сущность ("сущность"/"класс", не "структура"). internal/
	// toolchain.CC решает, добавлять ли "-lgc" при линковке, по тому же
	// признаку (наличию "#include <gc.h>" в сгенерированном файле), не по
	// отдельному флагу - libgc не входит в стандартный C-тулчейн ни на
	// одной платформе (в отличие от -lm), линковать её для программы без
	// единой сущности означало бы новое обязательное требование к
	// окружению сборки без всякой на то нужды.
	usesGC bool
	// libMode - генерация для разделяемой библиотеки (GenerateLibFromIR),
	// не для обычной программы - см. package doc generateSource. Включает
	// атрибут видимости у C-функций, соответствующих экспортированным
	// Rech-функциям (exportAttr).
	libMode  bool
	entities map[string]*ir.Entity
	// generics - имена generic-инстанциаций (Список<T>/Стопка<T>/
	// Очередь<T>/Словарь<К, В>) из prog.Generics - entityIsRef обращается
	// сюда для ЛЮБОГО имени, не найденного в entities (значит это либо
	// generic-инстанциация, либо опечатка/баг компилятора выше по
	// конвейеру, которая уже должна была быть отловлена семантикой) - см.
	// package doc entityIsRef.
	generics map[string]bool
	// nullableInner - каноническое имя "?Тип" (ir.Type.Entity) -> его
	// внутренний Тип, из prog.Nullables - нужен там, где по значению уже
	// известного KindNullable ir.Type требуется добраться до внутреннего
	// типа без повторного разбора строки (см. generateIRShowExpr).
	nullableInner map[string]ir.Type
	// exportedFuncNames/exportedEntityNames - имена функций/сущностей,
	// помеченных "экспортировать" (prog.Functions[i].Exported/
	// prog.Entities[i].Exported) - funcCName/entityCName (mangle.go)
	// сверяются с ними, чтобы дать C-символу настоящее (не транслитерированное)
	// имя именно у публичного ABI разделяемой библиотеки, оставив внутренние
	// (неэкспортированные) объявления как есть, за mangleName/mangleType.
	exportedFuncNames   map[string]bool
	exportedEntityNames map[string]bool
	// exportedMethods - сущность -> имя метода -> помечен "экспортировать"
	// (prog.Entities[i].Methods[j].Exported) - methodCName (mangle.go)
	// сверяется с ним тем же принципом, что exportedFuncNames/
	// exportedEntityNames выше, но отдельным полем (метод не однозначно
	// определён одним только именем без сущности-владельца).
	exportedMethods map[string]map[string]bool
	// exportedCtors - сущность -> конструктор помечен "экспортировать"
	// (prog.Entities[i].Ctor.Exported) - ctorCName (mangle.go) сверяется с
	// ним тем же принципом, что exportedMethods выше.
	exportedCtors map[string]bool
	// pointerInner - каноническое имя "Указатель<T>" (ir.Type.Entity) -> T,
	// из prog.Pointers - тот же принцип, что и nullableInner выше, нужен
	// valueType.cType, чтобы построить C-тип "T *" по одному только имени.
	pointerInner map[string]ir.Type
	// externCNames - Rech-facing имя (Alias) внешней функции -> настоящий
	// C-символ (CName), из prog.ExternFuncs: CallExpr.Callee несёт Alias,
	// вызывается по CName, не по mangleName(Alias) (тот не имеет отношения
	// к настоящему символу).
	externCNames map[string]string
	thisEntity   string

	// interfaces - интерфейсы программы по имени (см.
	// internal/backend/cbackend/vtable.go).
	interfaces map[string]*ir.Interface
	// interfaceImplementors - см. package doc ir.Program.InterfaceImplementors -
	// нужен reflectCheckedUnboxExprCode для "значение как ИмяИнтерфейса"
	// (число реализующих сущностей - в rech_unbox_check_iface(...) на месте
	// вызова, сам массив - interfaceImplementorsArrayName).
	interfaceImplementors map[string][]string
	// entityInterfaces - см. package doc ir.Program.EntityInterfaces - нужен
	// reflectGen.ensure/typeDescriptorCode для дескриптора сущности (поле
	// "interfaces"/"число_интерфейсов"/"получить_интерфейс"/"реализует_ли").
	entityInterfaces map[string][]string
	// dispatchSlots/dispatchParticipants/usesVtable - см. package doc
	// vtable.go, computeDispatch.
	dispatchSlots        map[string]int
	dispatchParticipants map[string]bool
	usesVtable           bool

	// excTypeIDs/excIsaTable - см. internal/backend/cbackend/exceptions.go,
	// buildIsaTable. usesExceptions - "выбросить"/"попытаться"/"относится
	// к" (с реальной проверкой типа - см. needsDispatch в generateIRIsExpr)
	// реально встретились в программе (в отличие от usesVtable, участие в
	// наследовании/интерфейсах само по себе не требует рантайма исключений) -
	// гейтит #include <setjmp.h> и весь блок рантайм-хелперов разом
	// (jmp_buf-кадры и rech_isa - один блок, не два отдельных гейта: "только
	// относится к, без единого попытаться" всё равно тянет за собой
	// неиспользуемые jmp_buf-хелперы - принято ради простоты одного гейта).
	// tryCounter - счётчик для уникальных имён C-переменных вложенных
	// "попытаться" (см. generateIRTryStmt).
	excTypeIDs     map[string]int
	excTypeNames   []string
	excIsaTable    [][]bool
	usesExceptions bool
	tryCounter     int

	// usesPanicCatch - хотя бы один "поймать (<имя>: Паника)" реально
	// встретился в программе (изоляция паники, см.
	// ir.TryStmt.CatchPanic/generateIRTryCatchPanicStmt) - переключает
	// rech_panic с безусловного fprintf+exit (panicHelper) на версию,
	// которая ищет ближайший такой кадр вверх по rech_catch_top
	// (panicCatchHelperCode) - подавляющее большинство программ этот флаг
	// не поднимают, panicHelper для них не меняется вовсе.
	usesPanicCatch bool

	// irFuncs - функции программы по имени (нужно generateIRLaunchExpr,
	// чтобы найти параметры/тип результата обёрнутой функции - см.
	// internal/backend/cbackend/concurrency.go, ensureTaskWrapper).
	irFuncs map[string]*ir.Function
	// launchedFuncs/taskCode - какие функции уже когда-либо "запускались"
	// в этой программе (ensureTaskWrapper эмитит C-обёртку один раз на
	// функцию, при первой встрече) и сам накопленный текст этих обёрток -
	// тот же принцип, что и showFuncs/vtables (отдельный builder,
	// заполняемый по ходу генерации тела, вставляется в файл в нужном
	// текстовом месте в конце). usesThreads - "запустить" реально
	// встретилось в программе - гейтит #include <pthread.h>/поточные
	// заголовки Windows и сам этот блок целиком.
	launchedFuncs map[string]bool
	taskCode      strings.Builder
	usesThreads   bool

	// coroutineFuncs/coroTaskCode/usesCoroutines - тот же принцип, что и
	// launchedFuncs/taskCode/usesThreads выше, но для "дождаться
	// <асинхронная функция>(...)" (ir.LaunchExpr.Coroutine == true, см.
	// internal/backend/cbackend/coroutine_task.go, ensureCoroTaskWrapper) -
	// отдельная дедупликация/накопление: одна и та же функция может быть
	// ОДНОВРЕМЕННО и явно "запущена" (usesThreads/launchedFuncs), и
	// вызвана через сахар (usesCoroutines/coroutineFuncs) в разных местах
	// программы - это два независимых пути кодогенерации на одну и ту же
	// Rech-функцию, не взаимоисключающие. usesCoroutines (ИЛИ usesSockets -
	// Шаг 132, plainSocketOpsCode теперь тоже опирается на планировщик
	// корутин) гейтит coroutineRuntimeCode (coroutine.go) целиком -
	// программа без обоих вовсе не платит за ucontext/epoll/kqueue
	// рантайм.
	coroutineFuncs map[string]bool
	coroTaskCode   strings.Builder
	usesCoroutines bool

	// usesMutex - хотя бы одна "Блокировка" реально встретилась в
	// программе (см. internal/backend/cbackend/mutex.go) - гейтит
	// дополнительно (наравне с usesThreads) #include <pthread.h> на POSIX и
	// сам блок mutexRuntimeCode.
	usesMutex bool

	// usesSemaphore - хотя бы один "Семафор" реально встретился в
	// программе (см. internal/backend/cbackend/semaphore.go) - тот же
	// принцип, что usesMutex выше, отдельный флаг (программа с ОДНОЙ
	// "Блокировкой" не должна тянуть за собой semaphoreRuntimeCode и
	// наоборот).
	usesSemaphore bool

	// usesDBTransaction - хотя бы один "Транзакция" реально встретился в
	// программе (см. internal/backend/cbackend/dbtx.go) - "начать_транзакцию"
	// уже подразумевает prog.UsesDB (dbFuncNames, internal/semantic/db.go),
	// но dbtx.go эмитируется отдельно, только когда реально нужен - программа
	// с обычной "БД" без единой транзакции не должна тянуть за собой этот
	// рантайм.
	usesDBTransaction bool

	// usesStateFlags - хотя бы одна сущность программы объявляет
	// "состояние:" (ir.Entity.HasStateFlags - уже включает потомков без
	// собственного "состояние:", см. package doc) - гейтит "#include
	// <stddef.h>" (offsetof) и rech_require_state (internal/backend/
	// cbackend/typestate.go). Вычисляется сразу из prog.Entities
	// (generateSource), а не по факту встреченного StateCheckExpr во время
	// генерации тела - структурное поле "state_flags" в C-структуре нужно
	// уже ради самого BecomeStmt (пишет в него), даже если в итоге ни один
	// вызов не потребовал рантайм-проверки.
	usesStateFlags bool

	// usesStaticInit - хотя бы одно статическое поле сущности/структуры
	// программы объявлено со значением по умолчанию ("=") - гейтит вызов
	// rech_init_statics() из main() (см. internal/backend/cbackend/
	// statics.go, staticFieldsInitCode).
	usesStaticInit bool

	// funcRefTrampolines/lambdaFuncsEmitted/callableCode - см.
	// internal/backend/cbackend/callable.go (ensureFuncRefTrampoline/
	// ensureLambdaFunc) - тот же принцип "накопить один раз, вставить в
	// файл в конце", что и taskCode выше.
	funcRefTrampolines map[string]bool
	lambdaFuncsEmitted map[string]bool
	callableCode       strings.Builder

	// callableSigs - каноническое имя сигнатуры "Вызываемый" (ir.Type.Entity)
	// -> её разобранные Params/Result, из prog.Callables - тот же принцип,
	// что и nullableInner/pointerInner выше, нужен ffi_callback.go
	// (ensureCallbackTrampoline), чтобы построить голую C-сигнатуру
	// колбэка по одному только имени типа аргумента/поля.
	// callbackTrampolinesEmitted - дедупликация (одна сигнатура - один
	// глобальный слот + один trampoline на всю программу), тот же принцип,
	// что funcRefTrampolines/lambdaFuncsEmitted выше.
	callableSigs               map[string]ir.CallableSig
	callbackTrampolinesEmitted map[string]bool
	// usesMemcpy - лямбда с хотя бы одним захватом реально встретилась в
	// программе (окружение замыкания строится через memcpy, см.
	// generateIRLambdaExpr) - гейтит "#include <string.h>", как и
	// usesConcat/usesCast и т. п.
	usesMemcpy bool

	// capturedRedirect - активно только во время генерации ТЕЛА лямбды
	// (ensureLambdaFunc) с хотя бы одним захватом: имя захваченной
	// переменной -> C-выражение, читающее/пишущее её через окружение
	// (rech_captured->...), а не через обычную локальную C-переменную (см.
	// varRef) - без этого запись внутри тела лямбды мутировала бы только
	// временную копию, живущую до конца вызова, а не общее для ВСЕХ
	// вызовов ОДНОГО И ТОГО ЖЕ значения лямбды хранилище (счётчик,
	// накапливающий между вызовами - типичный сценарий замыкания).
	// Сохраняется/восстанавливается вокруг вложенной лямбды (лямбда внутри
	// лямбды может сама что-то захватывать транзитивно из внешней -
	// её собственный capturedRedirect не должен добавляться к уже
	// активному, а полностью его временно заменять на время своего тела).
	capturedRedirect map[string]string

	// cleanupStack - действия, которые обязан выполнить "вернуть" ПЕРЕД
	// настоящим C `return`, если он встретился лексически внутри
	// охватывающего "попытаться" (внутри его Body/CatchBody - см.
	// exceptions.go, generateIRReturnStmt/emitCleanupUnwind) - каждый узел
	// "попытаться" добавляет сюда одно действие (восстановление
	// rech_catch_top до значения ДО себя, плюс запуск "окончательно", если
	// оно есть у узла) на время генерации своего Body/CatchBody и снимает
	// его после (append/срез, обычный стек). При возврате действия
	// прогоняются в ОБРАТНОМ порядке (LIFO - самый внутренний "попытаться"
	// первым), потому что raw C `return`, в отличие от "выбросить"/паники
	// (setjmp/longjmp, восстанавливается самим рантаймом), никак не
	// взаимодействует с rech_catch_top сам по себе. Сохраняется/
	// восстанавливается вокруг тела лямбды тем же принципом, что и
	// capturedRedirect выше - "вернуть" внутри лямбды принадлежит ЕЁ
	// собственной C-функции, не может видеть кадры внешнего попытаться.
	cleanupStack []cleanupAction

	// chainTmpCounter/chainTmpPreamble/curIndent - материализация получателя
	// вызова метода/присваивания полю, когда получатель ("структура",
	// значение) - не адресуемое выражение (результат вызова, а не
	// переменная/поле/"этот") - см. materializeIfNeeded. curIndent - отступ
	// ТЕКУЩЕЙ инструкции (обновляется в generateIRStmt перед разбором её
	// тела) - туда же, тем же отступом, попадает и объявление временной
	// переменной. chainTmpPreamble копится, пока разбирается ОДНА
	// инструкция (сколько угодно уровней цепочки внутри одного выражения -
	// каждый добавляет свою строку) - generateIRStmt сбрасывает и сбрасывает
	// в sb перед собственной строкой инструкции, см. package doc там же.
	chainTmpCounter  int
	chainTmpPreamble []string
	curIndent        string

	// loopStack - один кадр на каждый ОХВАТЫВАЮЩИЙ в точке генерации цикл
	// (WhileStmt/ForStmt/ForEachStmt/CStyleForStmt), от САМОГО ВНЕШНЕГО
	// (индекс 0) до самого внутреннего (последний) - читается только
	// ir.BreakStmt/ir.ContinueStmt (emitLoopJump, break_continue.go).
	// "прервать N"/"продолжить N" индексирует его с конца:
	// loopStack[len(loopStack)-N] - N-й по счёту цикл, считая текущий (N=1 -
	// как раньше, ближайший). cleanupBase - len(cleanupStack) В МОМЕНТ входа
	// в тело ЭТОГО цикла: "прервать"/"продолжить" обязаны прогнать
	// (restoreCatchTop/"окончательно") только действия, добавленные ВНУТРИ
	// цикла (вложенным "попытаться" МЕЖДУ началом цикла и самой инструкцией,
	// в т. ч. и вложенными циклами МЕЖДУ целевым циклом и инструкцией при
	// N>1) - НЕ действия внешних "попытаться", охватывающих цикл СНАРУЖИ
	// (тот кадр перехвата остаётся активным - цикл прерывается,
	// "попытаться" вокруг него ещё не закончилось). continueTarget/
	// breakTarget - всегда непустые уникальные метки (loopLabelCounter) -
	// раньше (до "прервать N") While/For/ForEach обходились голым C
	// `break`/`continue` (корректны сами по себе для БЛИЖАЙШЕГО цикла) и
	// только CStyleForStmt заводил метку continueTarget (Post -
	// произвольная Rech-инструкция, НЕ помещается в заголовок C `for` как
	// выражение - "продолжить" обязан сначала исполнить её, что raw C
	// `continue` там сделать не может, см. package doc generateIRCStyleFor
	// в break_continue.go) - с "прервать N"/"продолжить N" цель может быть
	// ЛЮБЫМ охватывающим циклом, не только ближайшим, а raw `break`/
	// `continue` способны адресовать только ближайший - единый label-based
	// `goto` для ВСЕХ четырёх видов цикла проще и надёжнее, чем два разных
	// кодовых пути в зависимости от N. continueTarget - метка сразу ПЕРЕД
	// закрывающей "}" тела цикла (у CStyleForStmt - перед Post, как и
	// раньше); breakTarget - метка СРАЗУ ПОСЛЕ всего цикла целиком (после
	// Post у CStyleForStmt - "прервать" не должен его исполнять). Кадр
	// сохраняется/восстанавливается вокруг тела лямбды тем же принципом,
	// что и cleanupStack/capturedRedirect выше - "прервать"/"продолжить"
	// внутри лямбды не могут видеть цикл, в котором эта лямбда лексически
	// объявлена (её тело - СОБСТВЕННАЯ C-функция).
	loopStack []loopFrame
	// loopLabelCounter - счётчик уникальных меток break/continue у ЛЮБОГО
	// вида цикла (rech_loop_break_N/rech_loop_continue_N) - как tryCounter у
	// попытаться, растёт на каждый узел, никогда не переиспользуется в
	// пределах одной программы.
	loopLabelCounter int
}

// loopFrame - см. package doc irGenCtx.loopStack.
type loopFrame struct {
	cleanupBase    int
	continueTarget string
	breakTarget    string
}

// materializeIfNeeded - получатель вызова метода/присваивания полю не
// адресуем НАПРЯМУЮ (не переменная/поле/"этот", результат вызова
// функции/метода/конструктора) - у "структура" (значение) в C нет
// постоянного адреса, "&(code)"/"(code).поле = ..." невалидны для rvalue.
// Заводит временную C-переменную (объявление копится в
// ctx.chainTmpPreamble, generateIRStmt вставляет её ПЕРЕД строкой текущей
// инструкции) и возвращает её имя вместо code - у переменной уже есть
// адрес, дальше generateIRAddressableExpr/FieldAssign работают с ней как с
// любым обычным адресуемым получателем. Для получателя-ссылки
// (сущность/интерфейс) вызывать не нужно - у неё "&"/материализация не
// нужны вообще (см. receiverIsPointer), поэтому здесь их не различает -
// вызывается только из веток, уже отфильтровавших "не указатель".
func (ctx *irGenCtx) materializeIfNeeded(receiver ir.Expr, code string) string {
	if isAddressableIR(ctx, receiver) {
		return code
	}
	ctx.chainTmpCounter++
	name := fmt.Sprintf("rech_chain_tmp%d", ctx.chainTmpCounter)
	cType := fromIRType(ir.TypeOf(receiver)).cType(ctx)
	ctx.chainTmpPreamble = append(ctx.chainTmpPreamble, fmt.Sprintf("%s%s %s = %s;\n", ctx.curIndent, cType, name, code))
	return name
}

// isAddressableIR - IR-эквивалент internal/semantic.receiverAddressable
// (переменная/"этот"/доступ к полю), но для доступа к полю - рекурсивно:
// `x.a.b` адресуем, только если адресуем сам `x` (пока цепочка не упрётся
// в получатель-указатель, для которого "->" валиден на ЛЮБОМ rvalue-коде
// слева, см. receiverIsPointer) - `создать().a.b` НЕ адресуем целиком, хотя
// сам узел верхнего уровня - FieldAccess, как и в адресуемом случае.
func isAddressableIR(ctx *irGenCtx, e ir.Expr) bool {
	switch v := e.(type) {
	case *ir.Identifier, *ir.ThisExpr:
		return true
	case *ir.FieldAccess:
		if ctx.receiverIsPointer(v.Receiver) {
			return true
		}
		return isAddressableIR(ctx, v.Receiver)
	default:
		return false
	}
}

// varRef - C-выражение, читающее/пишущее переменную name в текущей точке
// кодогенерации - обычная локальная C-переменная (mangleVar), кроме случая
// "name - переменная, захваченная лямбдой, тело которой сейчас генерируется"
// (см. ctx.capturedRedirect) - единственная точка, которую генерация
// Identifier/Assign/IncDecExpr спрашивает про имя переменной, чтобы не
// дублировать эту проверку в каждом из них по отдельности.
func varRef(ctx *irGenCtx, name string) string {
	if expr, ok := ctx.capturedRedirect[name]; ok {
		return expr
	}
	return mangleVar(name)
}

// exceptionsUseDispatch - true, если системная сущность "Исключение"
// реально участвует в диспетчеризации (есть хотя бы один пользовательский
// наследник где-то в программе) - единственный случай, когда объект
// исключения гарантированно несёт vt/type_id (embedding одинаков для ВСЕХ
// объектов типа "Исключение" во всей программе, свой у каждого backend'а
// один typedef на тип - см. generateIREntityStruct). Если false - в
// программе в принципе не может существовать никакого исключения, кроме
// самого "Исключение" (наследников нет ни у кого) - чтение vt->type_id
// было бы неопределённым поведением на объекте без этого поля, поэтому
// катч/isExpr в этом случае разрешаются на этапе компиляции без
// обращения к рантайму вовсе (см. generateIRTryStmt/generateIRIsExpr).
func (ctx *irGenCtx) exceptionsUseDispatch() bool {
	return ctx.dispatchParticipants[systemExceptionEntity]
}

// receiverIsPointer - true, если C-выражение, которое сгенерирует
// generateIRExpr(ctx, receiver), уже само по себе указатель ("Тип *"), а не
// значение ("Тип"). Три места используют один и тот же ответ на один и тот
// же вопрос: FieldAccess/FieldAssign (выбор "." против "->") и передача
// получателя в вызов метода (нужен ли "&", чтобы получить указатель для
// скрытого "Тип *this_"). ThisExpr не требует особого случая здесь - её
// собственная кодогенерация (case *ir.ThisExpr) уже возвращает строку в
// ТОЙ ЖЕ форме, что и любое другое выражение того же статического типа
// (указатель "this_" для ссылочной сущности, разыменованное "(*this_)" для
// структуры) - единообразие с TypeOf(receiver) держится сразу на обеих
// сторонах.
//
// KindInterface - всегда указатель (представлен как "void *", как и общий
// vtable-заголовок - см. package doc vtable.go), безусловно, без отдельного
// случая "интерфейс по значению": в языке такого нет вовсе, у интерфейса
// нет полей, значит нет и compound-literal представления. FieldAccess/
// FieldAssign сюда с интерфейсным получателем никогда не попадают
// (семантический анализ отклоняет доступ к полю через интерфейсное
// значение раньше) - это ветвление имеет значение только для получателя
// вызова метода.
func (ctx *irGenCtx) receiverIsPointer(receiver ir.Expr) bool {
	t := ir.TypeOf(receiver)
	if t.Kind == ir.KindInterface {
		return true
	}
	return t.Kind == ir.KindEntity && ctx.entityIsRef(t.Entity)
}

// entityIsRef - true, если name - объявленная "сущность"/"класс" (ссылочная
// семантика по умолчанию - сущность как указатель, GC_MALLOC), а не
// "структура" (по значению), ИЛИ generic-инстанциация (Список<T>/
// Стопка<T>/Очередь<T>/Словарь<К, В>, ctx.generics - они регистрируются
// отдельно, в prog.Generics, не в prog.Entities). Generic-коллекции - тоже
// ссылочные (тем же принципом, что и обычные сущности) - раньше были
// представлены "по значению" (сама обёртка-структура копировалась при
// передаче аргументом/присваивании, хотя её ВНУТРЕННИЙ буфер-указатель
// оставался общим) - это расходилось с interp (Go-карта/срез внутри
// значения - тот же объект при копировании) и приводило к молчаливой
// потере мутаций после того, как копия проходила через resize
// (генерация нового буфера видна только в ТОЙ копии, где он произошёл) -
// найдено и задокументировано в (Шаг 28) при написании
// examples/проекты/http_сервер, исправлено здесь.
func (ctx *irGenCtx) entityIsRef(name string) bool {
	if ent, ok := ctx.entities[name]; ok {
		return !ent.IsStruct
	}
	return ctx.generics[name]
}

// GenerateFromIR возвращает исходник на C, соответствующий программе prog -
// обычная программа, требует функцию "главная" (см. generateSource).
func GenerateFromIR(prog *ir.Program) (string, error) {
	return generateSource(prog, false)
}

// GenerateLibFromIR - как GenerateFromIR, но для разделяемой библиотеки
// (`rech build --lib`): "главная" не обязательна (библиотека не
// исполняется как программа), а вторым результатом возвращается текст
// C-заголовка (.h) с прототипами объявлений, помеченных "экспортировать" -
// см. header.go. Третий результат - публичный интерфейс в синтаксисе Rech
// (.rechi), РАЗБИТЫЙ по исходным файлам библиотеки (см. package doc
// generateInterfaceStubsByFile) - ключ карты - путь файла-источника
// относительно libRoot без расширения, "" у libRoot == "" (без файла
// проекта - один плоский результат, как раньше).
func GenerateLibFromIR(prog *ir.Program, libRoot string) (cSource, header string, interfaceStubs map[string]string, err error) {
	cSource, err = generateSource(prog, true)
	if err != nil {
		return "", "", nil, err
	}
	header, err = generateHeader(prog)
	if err != nil {
		return "", "", nil, err
	}
	interfaceStubs, err = generateInterfaceStubsByFile(prog, libRoot)
	if err != nil {
		return "", "", nil, err
	}
	return cSource, header, interfaceStubs, nil
}

// generateSource - общее тело GenerateFromIR/GenerateLibFromIR. libMode
// снимает требование функции "главная" (см. hasEntryPoint ниже) и включает
// атрибут видимости у экспортированных функций (exportAttr) - линковка в
// разделяемую библиотеку без этого сделала бы видимым снаружи КАЖДЫЙ символ
// программы, не только помеченные "экспортировать" (internal/toolchain.CC
// добавляет "-fvisibility=hidden" при сборке библиотеки на Linux/macOS
// именно ради этого атрибута - см. package doc там).
func generateSource(prog *ir.Program, libMode bool) (string, error) {
	if err := checkManglingCollisions(prog); err != nil {
		return "", err
	}

	entities := map[string]*ir.Entity{}
	nullableInner := map[string]ir.Type{}
	for _, n := range prog.Nullables {
		nullableInner[n.Name] = n.Inner
	}
	pointerInner := map[string]ir.Type{}
	for _, p := range prog.Pointers {
		pointerInner[p.Name] = p.Inner
	}
	callableSigs := map[string]ir.CallableSig{}
	for _, sig := range prog.Callables {
		callableSigs[sig.Name] = sig
	}
	exportedFuncNames := map[string]bool{}
	for _, fn := range prog.Functions {
		if fn.Exported {
			exportedFuncNames[fn.Name] = true
		}
	}
	exportedEntityNames := map[string]bool{}
	for _, ent := range prog.Entities {
		if ent.Exported {
			exportedEntityNames[ent.Name] = true
		}
	}
	exportedMethods := buildExportedMethods(prog)
	exportedCtors := buildExportedCtors(prog)
	usesGC := false
	usesStateFlags := false
	for _, ent := range prog.Entities {
		entities[ent.Name] = ent
		if !ent.IsStruct {
			usesGC = true
		}
		if ent.HasStateFlags {
			usesStateFlags = true
		}
		if ent.Name == http2StreamTypeC {
			// "HTTP2Поток" сам - структура (IsStruct, не попадает под
			// проверку выше), но её РАНТАЙМ (http2.go - сессия/поток
			// nghttp2, заголовки, растущие буферы) выделяет память через
			// GC_MALLOC независимо от того, есть ли в программе хоть одна
			// ссылочная сущность - тем же принципом, что prog.UsesDB/
			// len(prog.Generics)>0 ниже.
			usesGC = true
		}
	}
	for _, t := range prog.ReflectTargets {
		// Боксирование структуры ("(Значение) x") GC_MALLOC'ит независимую
		// копию (см. reflectBoxStructFuncName, reflect.go) - программа без
		// единой ссылочной сущности, но с рефлексией над структурой, всё
		// равно нуждается в Boehm GC.
		if ent, ok := entities[t.Entity]; ok && ent.IsStruct {
			usesGC = true
		}
	}
	if prog.UsesDB {
		// БД (internal/backend/cbackend/db.go) выделяет rech_db_t и копии
		// текстовых значений через GC_MALLOC - независимо от того, есть ли в
		// программе хоть одна ссылочная сущность.
		usesGC = true
	}
	if len(prog.Generics) > 0 {
		// Список/Стопка/Очередь/Словарь - ссылочные (см. entityIsRef),
		// конструктор ("новый Список<T>()") выделяет память через
		// GC_MALLOC (genericCtorCCode) - независимо от того, есть ли в
		// программе хоть одна пользовательская ссылочная сущность.
		usesGC = true
	}
	externCNames := map[string]string{}
	for _, ext := range prog.ExternFuncs {
		externCNames[ext.Alias] = ext.CName
	}
	interfaces := map[string]*ir.Interface{}
	for _, iface := range prog.Interfaces {
		interfaces[iface.Name] = iface
	}
	participants, slots := computeDispatch(prog)
	excTypeIDs, excTypeNames, excIsaTable := buildIsaTable(prog, participants)
	generics := make(map[string]bool, len(prog.Generics))
	for _, g := range prog.Generics {
		generics[g.Name] = true
	}
	ctx := &irGenCtx{
		entities:              entities,
		generics:              generics,
		nullableInner:         nullableInner,
		pointerInner:          pointerInner,
		callableSigs:          callableSigs,
		exportedFuncNames:     exportedFuncNames,
		exportedEntityNames:   exportedEntityNames,
		exportedMethods:       exportedMethods,
		exportedCtors:         exportedCtors,
		externCNames:          externCNames,
		usesGC:                usesGC,
		usesStateFlags:        usesStateFlags,
		libMode:               libMode,
		interfaces:            interfaces,
		interfaceImplementors: prog.InterfaceImplementors,
		entityInterfaces:      prog.EntityInterfaces,
		dispatchParticipants:  participants,
		dispatchSlots:         slots,
		// usesVtable - хотя бы одна сущность участвует в диспетчеризации
		// (participants), не "есть хотя бы один слот" (slots) - сущность
		// без единого метода (например, системная "Исключение" и её
		// маркерные наследники без переопределений) всё равно получает
		// поле vt в своей C-структуре (см. generateIREntityStruct), значит
		// rech_vtable/rech_vtable_header обязаны быть объявлены, даже если
		// slots в итоге пуст.
		usesVtable:     len(participants) > 0,
		excTypeIDs:     excTypeIDs,
		excTypeNames:   excTypeNames,
		excIsaTable:    excIsaTable,
		launchedFuncs:  map[string]bool{},
		coroutineFuncs: map[string]bool{},
	}
	ctx.irFuncs = map[string]*ir.Function{}
	for _, fn := range prog.Functions {
		ctx.irFuncs[fn.Name] = fn
	}
	if ent, ok := entities[mutexEntityName]; ok && ent.IsMutex {
		ctx.usesMutex = true
	}
	if ent, ok := entities[semaphoreEntityName]; ok && ent.IsSemaphore {
		ctx.usesSemaphore = true
	}
	if ent, ok := entities[transactionEntityName]; ok && ent.IsTransaction {
		ctx.usesDBTransaction = true
		// rech_db_run_transaction (dbtx.go) ловит и "Паника" (catch_panics-
		// кадр - нужен panicCatchHelperCode, полное тело rech_panic,
		// ищущее такой кадр), и "Исключение" (обычный кадр - нужен
		// rech_throw/catchFrameType, exceptionRuntimeCode) - тем же набором
		// флагов, что и обычное "поймать (x: Паника)" в Rech-коде
		// (generateIRTryCatchPanicStmt).
		ctx.usesExceptions = true
		ctx.usesPanicCatch = true
	}
	if ctx.usesStateFlags {
		// rech_require_state (typestate.go) бросает через rech_throw -
		// сущность со "состояние:" почти всегда участвует и в
		// попытаться/поймать где-то рядом, но даже если нет - рантайм
		// исключений всё равно должен быть объявлен ДО первого возможного
		// вызова rech_require_state, а не обнаруживаться по факту (как
		// usesExceptions обычно обнаруживается) - решаем безусловно здесь,
		// а не пытаемся отследить каждое место, где реально понадобится
		// StateCheckExpr.
		ctx.usesExceptions = true
	}

	// callableCollections - имена generic-инстанциаций (Список<T>/Словарь<К,
	// В>), чей элемент/ключ - "Вызываемый" по значению - те же, что попадают
	// в genericsCodeAfterCallables ниже. Нужны уже здесь, чтобы решить, в
	// какую из двух групп typedefs/typedefsAfterCallables попадёт сущность,
	// несущая такую коллекцию полем по значению (см. их package doc).
	callableCollections := map[string]bool{}
	// entityCollections - имена generic-инстанциаций, чей ЭЛЕМЕНТ -
	// настоящая пользовательская сущность (тот же признак, что решает
	// genericsCodeAfterEntities ниже) - сигнатура "Вызываемый", несущая
	// такую коллекцию ПАРАМЕТРОМ/РЕЗУЛЬТАТОМ (не полем сущности - для этого
	// уже есть typedefsAfterCallables/genericsCodeAfterCallables), нуждается
	// в её typedef заранее (`fn` - указатель на функцию, принимающую
	// `rech_T_...коллекция *` - C проверяет полноту типа под указателем
	// внутри структуры-члена, forward declaration недостаточно без
	// отдельного `typedef struct ... rech_T_...;` до тела) - собрана здесь
	// же, до callableTypedefs ниже, чтобы решить, в какую из ДВУХ групп
	// (callableTypedefs/callableTypedefsAfterEntities) попадёт каждая
	// сигнатура.
	entityCollections := map[string]bool{}
	for _, g := range prog.Generics {
		if g.ElemType.Kind == ir.KindCallable || g.KeyType.Kind == ir.KindCallable {
			callableCollections[g.Name] = true
		}
		if g.ElemType.Kind == ir.KindEntity && entities[g.ElemType.Entity] != nil {
			entityCollections[g.Name] = true
		}
	}

	// typedefs/typedefsAfterCallables - C-структуры сущностей программы.
	// Разделены по тому же принципу, что и genericsCode/
	// genericsCodeAfterCallables ниже: сущность, несущая ПОЛЕМ ПО ЗНАЧЕНИЮ
	// коллекцию с "Вызываемый"-элементом (callableCollections выше) или сам
	// "Вызываемый" напрямую, нуждается в уже объявленных callableTypedefs/
	// genericsCodeAfterCallables - обычная сущность (подавляющее
	// большинство) по-прежнему объявляется РАНЬШЕ них, как и раньше.
	var typedefs, typedefsAfterCallables strings.Builder
	if ctx.usesVtable {
		typedefs.WriteString(vtableTypedefs(len(slots)))
	}
	for _, ent := range prog.Entities {
		dest := &typedefs
		if !ent.IsExternStruct {
			// "внешняя структура" - поле "Вызываемый(...)" (если оно
			// есть) эмитится ниже голым указателем на функцию
			// (ffiParamCType), а не {fn, env}-структурой (callableTypedefs) -
			// в отличие от ОБЫЧНОЙ сущности с таким полем, не нуждается в
			// том, чтобы typedef колбэка был объявлен раньше.
			for _, f := range ent.Fields {
				if f.Type.Kind == ir.KindCallable || callableCollections[f.Type.Entity] {
					dest = &typedefsAfterCallables
					break
				}
			}
		}
		s, err := generateIREntityStruct(ctx, ent)
		if err != nil {
			return "", err
		}
		dest.WriteString(s)
	}

	// vtables - статические таблицы диспетчеризации участвующих сущностей
	// (см. vtable.go) - ссылаются на C-функции методов по адресу, поэтому
	// должны идти в файле ПОСЛЕ protos (иначе &rech_M_... указывал бы на
	// ещё не объявленную функцию - неявного объявления функций в C99 нет).
	var vtables strings.Builder
	for _, ent := range prog.Entities {
		if ctx.dispatchParticipants[ent.Name] {
			vtables.WriteString(generateVtableInstance(ctx, ent))
		}
	}

	// ctors - GC_MALLOC-конструкторы для ссылочных сущностей ("сущность"/
	// "класс", не "структура") - см. generateIREntityCtor. "структура"
	// продолжает конструироваться как compound literal прямо в месте
	// вызова (generateIRNewExpr), без отдельной функции.
	var ctors strings.Builder
	for _, ent := range prog.Entities {
		if ctx.entityIsRef(ent.Name) {
			code, err := generateIREntityCtor(ctx, ent)
			if err != nil {
				return "", fmt.Errorf("сущность %q: конструктор: %w", ent.Name, err)
			}
			ctors.WriteString(code)
		}
	}

	staticDecl := staticFieldsDecl(ctx, prog)
	staticInit, err := staticFieldsInitCode(ctx, prog)
	if err != nil {
		return "", err
	}
	ctx.usesStaticInit = staticInit != ""

	// genericsCode - typedef struct + функции для встроенных generic-
	// коллекций, использованных в программе - переиспользует
	// listCollectionCCode/stackCollectionCCode/queueCollectionCCode/
	// dictCollectionCCode (растущий массив/хеш-таблица) - IR не хранит
	// собственных сигнатур методов для них, MethodCallExpr уже несёт всё
	// нужное для вызова, эта функция только про C-текст самого типа.
	//
	// genericsCodeAfterCallables - ОТДЕЛЬНАЯ группа для коллекций, чей
	// элемент/ключ - "Вызываемый" (например, "Список<Вызываемый(Запрос):
	// Ответ>" - таблица маршрутов): C-структура сигнатуры (rech_C_...)
	// несёт функцию/параметры ПО ЗНАЧЕНИЮ, значит должна быть уже объявлена
	// раньше typedef такой коллекции - обратный порядок относительно
	// обычных genericsCode выше (примитив как элемент не заставляет
	// коллекцию ждать ничего, кроме себя самой) - эмитится позже,
	// после callableTypedefs (см. её появление в sb ниже).
	//
	// genericsCodeAfterEntities - ЕЩЁ ОДНА отдельная группа: коллекция, чей
	// элемент - настоящая ПОЛЬЗОВАТЕЛЬСКАЯ сущность (entities[...] != nil -
	// тот же самый признак "настоящая сущность, не generic-псевдо-сущность
	// вроде Список<T>", которым уже пользуется nullablesAfterTypedefs ниже
	// для того же класса проблемы - "?ИмяСущности") - анонимный `typedef
	// struct {...} rech_Имя;` сущности не forward-declarable в C, значит
	// коллекция с сущностью-элементом обязана идти СТРОГО ПОСЛЕ typedef
	// этой сущности. internal/semantic.collectEntities взамен запрещает
	// такой коллекции быть полем другой сущности/структуры (иначе
	// понадобился бы полный топологический порядок typedef'ов вперемешку с
	// сущностями, а не фиксированный порядок бакетов) - только локальная
	// переменная/параметр/результат функции, поэтому единственное, что эта
	// группа обязана видеть заранее - ВСЕ typedefs/typedefsAfterCallables
	// (обе группы сущностей) - см. её появление в sb ниже, сразу после
	// typedefsAfterCallables.
	var genericsCode, genericsCodeAfterCallables, genericsCodeAfterEntities, indexSafeGetCode strings.Builder
	usesGenerics := len(prog.Generics) > 0
	// hasStructTargets - вычислена здесь (не там, где раньше использовалась
	// единственный раз, ниже), потому что теперь нужна РАНЬШЕ - см. package
	// doc ctx.includesStringH.
	hasStructTargets := len(prog.ParseTargets) > 0 || len(prog.SerializeTargets) > 0 || len(prog.XMLParseTargets) > 0 || len(prog.XMLSerializeTargets) > 0 || len(prog.YAMLParseTargets) > 0 || len(prog.YAMLSerializeTargets) > 0 || len(prog.ProtobufParseTargets) > 0 || len(prog.ProtobufSerializeTargets) > 0
	for _, g := range prog.Generics {
		dest := &genericsCode
		switch {
		case g.ElemType.Kind == ir.KindCallable || g.KeyType.Kind == ir.KindCallable:
			dest = &genericsCodeAfterCallables
		case g.ElemType.Kind == ir.KindEntity && entities[g.ElemType.Entity] != nil:
			dest = &genericsCodeAfterEntities
		}
		elemC := fromIRType(g.ElemType).cType(ctx)
		// rech_new_<...> - конструктор "новый Список<T>()" и т. п.
		// (generateIRNewExpr, ctx.entityIsRef теперь true и для
		// generic-инстанциаций - см. её package doc) - СРАЗУ после
		// typedef/методов самой коллекции (тот же dest, что и у
		// genericsCode/genericsCodeAfterCallables вообще - конструктору
		// нужен уже готовый typedef для sizeof()), но ДО NeedsListLiteral
		// ниже - у "Список" listLiteralCCode сама вызывает этот
		// конструктор внутри своего тела (см. её package doc), значит
		// должна идти строго после него в тексте (C - без forward-
		// declaration в пределах одного dest, функция должна быть уже
		// объявлена/определена к месту вызова).
		switch g.Template {
		case "Список":
			dest.WriteString(listCollectionCCode(g.Name, elemC))
			dest.WriteString(genericCtorCCode(g.Name))
			if g.Equatable {
				dest.WriteString(listEqualCCode(ctx, g))
			}
			if g.NeedsSafeIndex {
				indexSafeGetCode.WriteString(listIndexSafeGetCCode(g.Name, mangleType("?"+g.ElemType.String())))
			}
			if g.NeedsListLiteral {
				dest.WriteString(listLiteralCCode(g.Name, elemC))
			}
		case "Стопка":
			dest.WriteString(stackCollectionCCode(g.Name, elemC))
			dest.WriteString(genericCtorCCode(g.Name))
		case "Очередь":
			dest.WriteString(queueCollectionCCode(g.Name, elemC))
			dest.WriteString(genericCtorCCode(g.Name))
		case "Словарь":
			keyC := fromIRType(g.KeyType).cType(ctx)
			isStringKey := g.KeyType.Kind == ir.KindString
			if isStringKey {
				ctx.usesDictString = true
			}
			dest.WriteString(dictCollectionCCode(g.Name, keyC, elemC, isStringKey))
			dest.WriteString(genericCtorCCode(g.Name))
			if g.Equatable {
				dest.WriteString(dictEqualCCode(ctx, g))
			}
			if g.NeedsSafeIndex {
				indexSafeGetCode.WriteString(dictIndexSafeGetCCode(g.Name, keyC, mangleType("?"+g.ElemType.String())))
			}
		default:
			return "", fmt.Errorf("неизвестный generic-шаблон %q", g.Template)
		}
	}

	// nullablesCode/nullablesAfterTypedefs - typedef struct + функции
	// (обернуть/пусто/развернуть/??) для каждого конкретного "?Тип",
	// встретившегося в программе - тем же принципом, что genericsCode выше,
	// но по prog.Nullables. Разбито на ДВЕ группы по порядку эмита:
	// "?Целое"/"?Список<T>" и т.п. (внутренний тип - примитив/generic-
	// коллекция, entities[...] здесь пусто - Generics живут в отдельном
	// списке) нужны РАНЬШЕ typedefs сущностей (сущность может нести
	// nullable-поле по значению - тот же класс проблемы, что у genericsCode/
	// typedefs; "?Список<T>" при этом и так уже позже genericsCode, где
	// определён сам Список<T>), а "?ИмяСущности" (настоящая, объявленная
	// сущность) - НАОБОРОТ, ПОЗЖЕ (обёртка ссылается на C-typedef самой
	// сущности, mangleType(entity), который ещё не объявлен раньше
	// typedefs). Использует rech_panic (usesNullables ниже участвует в
	// условии panicHelper).
	var nullablesCode, nullablesAfterTypedefs strings.Builder
	usesNullables := len(prog.Nullables) > 0
	for _, n := range prog.Nullables {
		elemC := fromIRType(n.Inner).cType(ctx)
		code := nullableCollectionCCode(n.Name, elemC)
		if n.Inner.Kind == ir.KindEntity && entities[n.Inner.Entity] != nil {
			nullablesAfterTypedefs.WriteString(code)
		} else {
			nullablesCode.WriteString(code)
		}
	}

	// callableTypedefs - C-структуры {указатель на функцию, указатель на
	// окружение замыкания} для каждой сигнатуры "Вызываемый", встретившейся
	// в программе (prog.Callables - уже полный список, собранный
	// internal/semantic, без ленивого "по первому использованию", в
	// отличие от funcRefTrampolines/lambdaFuncsEmitted ниже) - см.
	// internal/backend/cbackend/callable.go.
	//
	// callableTypedefsAfterEntities - ОТДЕЛЬНАЯ группа для сигнатуры,
	// параметр/результат которой сам - generic-коллекция с сущностью-
	// элементом (entityCollections выше, например "Вызываемый(Список<T>)" у
	// generic-функции чанкования БД, examples/бд/чанкование.rech): `fn`
	// принимает указатель на её C-структуру, значит typedef коллекции
	// обязан существовать РАНЬШЕ - обратный порядок относительно обычных
	// callableTypedefs (примитив/сама сущность параметром не заставляет
	// сигнатуру ждать ничего, кроме себя самой) - эмитится позже, после
	// genericsCodeAfterEntities (см. её появление в sb ниже).
	var callableTypedefs, callableTypedefsAfterEntities strings.Builder
	for _, sig := range prog.Callables {
		dest := &callableTypedefs
		for _, p := range sig.Params {
			if entityCollections[p.Entity] {
				dest = &callableTypedefsAfterEntities
				break
			}
		}
		if dest == &callableTypedefs && sig.Result != nil && entityCollections[sig.Result.Entity] {
			dest = &callableTypedefsAfterEntities
		}
		dest.WriteString(callableStructTypedef(ctx, sig))
	}

	// showFuncs - функции показа ("показать") для каждой сущности и
	// generic-коллекции программы - заполняется НИЖЕ, после генерации тел
	// (body), а не здесь - см. package doc там. showProtos - их прототипы
	// (та же роль, что и у protos ниже для методов/конструкторов, но
	// эмитируется в СВОЙ, более ранний блок sb - см. её появление там: protos
	// сам заполняется/сбрасывается позже, ПОСЛЕ showFuncs).
	var showFuncs, showProtos strings.Builder
	// structEqFuncs/structEqProtos - тот же принцип, что и showFuncs/
	// showProtos выше (заполняются НИЖЕ, после генерации тел - ctx.
	// usesStructEq взводится "==" структуры внутри них, см. struct_eq.go).
	var structEqFuncs, structEqProtos strings.Builder

	var protos strings.Builder
	if ctx.usesMutex {
		// Прототипы - не только определения (mutexRuntimeCode, ниже, в
		// builder "ctors") - "Блокировка" эмитится в out.Entities поздно
		// (internal/semantic/analyzer.go, buildIR - после тел всех
		// функций/методов программы), поэтому код, использующий её РАНЬШЕ
		// по файлу, не может полагаться на текстовый порядок определений,
		// как обычные сущности - только на эти прототипы.
		protos.WriteString(mutexPrototypes())
	}
	if ctx.usesSemaphore {
		// Та же причина, что у mutexPrototypes выше - "Семафор" тоже может
		// добавиться в out.Entities поздно относительно кода, который её
		// использует.
		protos.WriteString(semaphorePrototypes())
	}
	if ctx.usesDBTransaction {
		// Та же причина, что у mutexPrototypes/semaphorePrototypes выше.
		protos.WriteString(dbtxPrototypes())
	}
	for _, ent := range prog.Entities {
		for _, m := range ent.Methods {
			if m.IsAbstract {
				// Абстрактный метод (нет тела) - вызвать его напрямую по
				// мангл-имени нельзя, C-функции для него не существует -
				// см. package doc ir.Method.IsAbstract.
				continue
			}
			if m.IsStatic {
				fmt.Fprintf(&protos, "%s %s(%s);\n", cReturnTypeFor(ctx, m.HasReturn, m.ReturnType), methodCName(ctx, ent.Name, m.Name), cParamListIR(ctx, m.Params))
				continue
			}
			fmt.Fprintf(&protos, "%s%s %s(%s);\n", exportAttr(ctx, m.Exported), cReturnTypeFor(ctx, m.HasReturn, m.ReturnType), methodCName(ctx, ent.Name, m.Name), cMethodParamListIR(ctx, ent.Name, m.Params))
		}
		// Прототипы rech_init_X/rech_new_X - тем же принципом, что и у
		// методов выше: сущность, эмитированная в prog.Entities поздно
		// (например, системная "Исключение" - см. package doc buildIR,
		// internal/semantic/analyzer.go), не может полагаться на текстовый
		// порядок определений, если её конструктор вызван РАНЬШЕ по файлу -
		// как напрямую ("новый X(...)"), так и через "родитель(...)" в
		// конструкторе пользовательского наследника. mutex/семафор/транзакция -
		// отдельными mutexPrototypes()/semaphorePrototypes()/dbtxPrototypes()
		// выше, здесь не участвуют (у "Транзакция" вдобавок нет вообще
		// никакого конструктора - см. internal/semantic/expr.go,
		// analyzeNewExpr, IsTransaction).
		if !ent.IsStruct && !ent.IsMutex && !ent.IsSemaphore && !ent.IsTransaction {
			ctorParams := []ir.Param(nil)
			if ent.Ctor != nil {
				ctorParams = ent.Ctor.Params
			}
			fmt.Fprintf(&protos, "static void %s(%s);\n", mangleCtorInit(ent.Name), cMethodParamListIR(ctx, ent.Name, ctorParams))
			ctorExported := ctx.exportedCtors[ent.Name]
			ctorStatic := "static "
			if ctorExported {
				ctorStatic = ""
			}
			fmt.Fprintf(&protos, "%s%s%s *%s(%s);\n", ctorStatic, exportAttr(ctx, ctorExported), entityCName(ctx, ent.Name), ctorCName(ctx, ent.Name), cParamListIR(ctx, ctorParams))
		}
	}
	hasEntryPoint := false
	for _, fn := range prog.Functions {
		if fn.Name == entryPointName {
			hasEntryPoint = true
			continue
		}
		fmt.Fprintf(&protos, "%s%s %s(%s);\n", exportAttr(ctx, fn.Exported), cReturnTypeFor(ctx, fn.HasReturn, fn.ReturnType), funcCName(ctx, fn.Name), cParamListIR(ctx, fn.Params))
	}
	if !libMode && !hasEntryPoint {
		return "", fmt.Errorf("в программе не найдена функция %q - она служит точкой входа", entryPointName)
	}

	var body strings.Builder
	for _, ent := range prog.Entities {
		for _, m := range ent.Methods {
			if m.IsAbstract {
				continue
			}
			mCode, err := generateIRMethod(ctx, ent.Name, m)
			if err != nil {
				return "", fmt.Errorf("сущность %q: метод %q: %w", ent.Name, m.Name, err)
			}
			body.WriteString(mCode)
		}
	}
	for _, fn := range prog.Functions {
		fnCode, err := generateIRFunction(ctx, fn)
		if err != nil {
			return "", fmt.Errorf("функция %q: %w", fn.Name, err)
		}
		body.WriteString(fnCode)
	}

	// ctx.usesHTTP2 => ctx.usesSockets - ЖИВОЙ НАЙДЕННЫЙ БАГ: http2SessionOpsCode/
	// http2CallbacksCode (http2.go) эмитируются ОДНИМ блоком целиком при
	// usesHTTP2 - внутри БЕЗУСЛОВНО есть rech_http2_accept_session/rech_
	// http2_io_thread/rech_http2_start_thread, которые зовут accept()/recv()/
	// setsockopt() напрямую (нужны <sys/socket.h>/<errno.h>/... - emitted
	// ниже, только если ctx.usesSockets) - но usesSockets выставляют ТОЛЬКО
	// диспетчеризации http2_принять_сессию/новая_http2_сессия/http2_закрыть_
	// сессию (builtins_shared.go) - ни одна из grpc_*/http2_открыть_поток/
	// http2_отправить_данные/... (работают уже НАД готовой сессией, сама
	// сессия установлена в ДРУГОМ месте программы, а то и в другом файле,
	// пришедшем через "использовать") этого не делает. Программа/библиотека,
	// вызывающая ТОЛЬКО такие функции (например, сгенерированная "rech
	// proto" клиентская обёртка Сервис_Метод(сессия: HTTP2Сессия, ...) -
	// сессию открывает ВЫЗЫВАЮЩАЯ программа, не сама обёртка) падала на
	// "unknown type name"/implicit-declaration у cc - найдено на "rech
	// build --lib" файла со сгенерированными gRPC-стабами (Шаг 96), без
	// единого вызова http2_принять_сессию/новая_http2_сессия в НЕЙ САМОЙ
	// (маскировалось в обычной сборке до сих пор тем, что проверенные
	// программы попутно ещё и открывали сессию в той же единице трансляции).
	if ctx.usesHTTP2 {
		ctx.usesSockets = true
	}

	// ctx.includesStringH/includesStdlibH/includesMathH - вычислены только
	// теперь, тела выше (ctx.usesMemcpy/usesCast/usesRandom/... - и то, что
	// ниже собирает "#include <...>", ir_generator.go, sb) - единственное
	// место, где эти флаги взводятся, поэтому раньше их не узнать. externPrototypeOfIR
	// ниже (перенесённый сюда специально, НЕ рядом с mutexPrototypes/
	// semaphorePrototypes выше, где не было бы ещё известно ни одного из
	// них) заранее знает, войдёт ли string.h/stdlib.h/math.h в файл ПО
	// ДРУГОЙ причине - и, если да, для известного (externSymbolHeader,
	// extern_headers.go) стандартного символа не пишет собственный,
	// потенциально расходящийся по типу extern-прототип поверх уже
	// объявленного заголовком (найдено живьём - examples/основы/
	// внешние_функции.rech, strlen + <string.h>, включённый совсем по
	// другой причине - memcpy захваченной лямбды).
	ctx.includesStringH = ctx.usesConcat || ctx.usesStrCmp || ctx.usesConsoleInput || ctx.usesFileIO || ctx.usesSockets || ctx.usesCast || ctx.usesMemcpy || ctx.usesDictString || ctx.usesQueryParams || ctx.usesCookies || ctx.usesStringOps || ctx.usesCrypto || ctx.usesEnv || ctx.usesPanicCatch || ctx.usesStringSplit || ctx.usesRegex || ctx.usesHTTPHeaders || ctx.usesColor || ctx.usesSubprocess || hasStructTargets || prog.UsesReflection || ctx.usesTZ || ctx.usesDateParse
	// usesNullables/prog.UsesReflection - те же два условия, что у panicGate
	// (ниже) сверх её собственных: panicGate решает, эмитить ли panicHelper
	// (безусловный exit(1)), includesStdlibH - подключать ли <stdlib.h>, где
	// exit() объявлен - расхождение между двумя списками значило "exit()
	// вызывается, а <stdlib.h> не подключён" (implicit-function-declaration
	// - GCC на Linux это лишь предупреждение, но Apple Clang - жёсткая
	// ошибка компиляции). usesNullables фактически истинен почти всегда -
	// встроенное "Исключение" безусловно регистрирует "?Целое" для поля
	// "код" (см. package doc systemExceptionFields), значит panicHelper (а
	// с ним и <stdlib.h>) эмитится в подавляющем большинстве программ, не
	// только там, где пользователь сам пишет "?Тип".
	ctx.includesStdlibH = ctx.usesConcat || usesGenerics || usesNullables || ctx.usesPanic || ctx.usesConsoleInput || ctx.usesFileIO || ctx.usesCast || ctx.usesExceptions || ctx.usesDateTime || ctx.usesColor || ctx.usesQueryParams || ctx.usesCookies || ctx.usesRandom || ctx.usesStringOps || ctx.usesCrypto || ctx.usesEnv || ctx.usesRegex || ctx.usesHTTPHeaders || hasStructTargets || ctx.usesExit || ctx.usesTZ || ctx.usesHumanDate || prog.UsesReflection
	ctx.includesMathH = ctx.usesMath
	for _, ext := range prog.ExternFuncs {
		protoStr, err := externPrototypeOfIR(ctx, ext)
		if err != nil {
			return "", err
		}
		protos.WriteString(protoStr)
	}

	// showFuncs строится только теперь, когда ctx.usesShow уже окончательно
	// известен (тела выше - единственное место, где "показать" реально
	// встречается и взводит его) - не раньше: у generateIRShowExpr есть
	// побочный эффект (ctx.usesCast = true для поля типа "Символ", см. её
	// package doc) - построй showFuncs заранее "на всякий случай" для
	// КАЖДОЙ сущности программы, этот эффект просочился бы в решение
	// "нужен ли #include <string.h>/<stdlib.h>" (оно принимается раньше,
	// см. ниже) даже для программы, которая "показать" не использует вовсе
	// - именно так и обнаружилось на практике (внешняя структура с полем
	// "Символ" молча тянула за собой чужой конфликтующий системный
	// заголовок в любой программе с "внешняя функция").
	if ctx.usesShow {
		// showProtos - "static void rech_show_...(...);" для КАЖДОЙ функции
		// показа программы, ДО единой их определений ниже - сущность может
		// нести generic-коллекцию ПОЛЕМ ("сотрудники: Список<Пользователь>",
		// см. TestAnalyzeCollectionEntityFieldAllowed), чья функция показа
		// зовёт rech_show_<Сущность>, а функция показа самой "Пользователь"
		// тоже может (рекурсивно, через ДРУГОЕ своё поле) звать
		// rech_show_<та же или другая коллекция> - настоящий цикл между
		// определениями, ЛЮБОЙ фиксированный порядок проходов (как раньше -
		// сначала generics-без-сущности, потом сущности, потом остальные
		// generics) его не решает. Прототипы снимают вопрос порядка
		// полностью - как и у protos ниже для методов/конструкторов.
		for _, ent := range prog.Entities {
			paramType := entityCName(ctx, ent.Name)
			if ctx.entityIsRef(ent.Name) {
				paramType += " *"
			}
			fmt.Fprintf(&showProtos, "static void %s(%s);\n", showFuncName(ent.Name), paramType)
		}
		for _, g := range prog.Generics {
			fmt.Fprintf(&showProtos, "static void %s(%s *);\n", showFuncName(g.Name), mangleType(g.Name))
		}
		for _, g := range prog.Generics {
			showFuncs.WriteString(generateIRGenericShowFunc(ctx, g))
		}
		for _, ent := range prog.Entities {
			showFuncs.WriteString(generateIREntityShowFunc(ctx, ent))
		}
	}

	if len(ctx.structEqEntities) > 0 {
		// structEqProtos - прототип rech_struct_eq_... для КАЖДОЙ структуры
		// из ctx.structEqEntities (не всех prog.Entities - см. её package
		// doc), ДО единых определений ниже - та же причина, что и у
		// showProtos выше (вложенная "структура" полем - настоящий цикл
		// между определениями возможен). Перебор prog.Entities (не самого
		// ctx.structEqEntities) - только ради детерминированного порядка
		// вывода (порядок итерации по map в Go не гарантирован).
		for _, ent := range prog.Entities {
			if !ctx.structEqEntities[ent.Name] {
				continue
			}
			cName := entityCName(ctx, ent.Name)
			fmt.Fprintf(&structEqProtos, "static bool %s(%s, %s);\n", structEqFuncName(ent.Name), cName, cName)
		}
		for _, ent := range prog.Entities {
			if !ctx.structEqEntities[ent.Name] {
				continue
			}
			structEqFuncs.WriteString(generateIRStructEqFunc(ctx, ent))
		}
	}

	var sb strings.Builder
	sb.WriteString("// Автоматически сгенерировано компилятором rech. Не редактировать руками.\n")
	if ctx.usesProcMem {
		// _GNU_SOURCE - ДО любого системного заголовка (glibc проверяет
		// её при первом же include) - process_vm_readv/process_vm_writev
		// (procmem.go) объявлены в <sys/uio.h> только при этом
		// определении (man process_vm_readv(2), "Feature Test Macro
		// Requirements").
		sb.WriteString("#ifdef __linux__\n#define _GNU_SOURCE\n#endif\n")
	}
	sb.WriteString("#include <stdio.h>\n")
	sb.WriteString("#include <stdbool.h>\n")
	sb.WriteString("#include <stdint.h>\n")
	if ctx.usesStateFlags {
		sb.WriteString("#include <stddef.h>\n")
	}
	// windows.h/shellapi.h - rech_init_console/rech_init_args нужны
	// безусловно, файловый ввод-вывод - за MultiByteToWideChar.
	// WIN32_LEAN_AND_MEAN/winsock2.h - нужны при usesSockets. ДО "#include
	// <gc.h>" ниже - на Windows редирект CreateThread -> GC_CreateThread
	// внутри <gc.h> (потоки, см. ensureTaskWrapper) активен только если сам
	// <gc.h> уже видит, что windows.h включён.
	sb.WriteString("#ifdef _WIN32\n#define WIN32_LEAN_AND_MEAN\n#include <windows.h>\n#include <shellapi.h>\n#endif\n")
	// gc.h - Boehm GC (сущность/класс - ссылочная семантика, выделяются
	// GC_MALLOC, см. generateIREntityCtor) - только если в
	// программе реально есть хотя бы одна ссылочная сущность (ctx.usesGC) -
	// в отличие от -lm/остальных всегда-подключаемых заголовков, libgc не
	// входит в стандартный C-тулчейн ни на одной платформе (проверено
	// эмпирически - ни на Ubuntu, ни на MinGW-w64 её нет по умолчанию),
	// программа без единой сущности не должна требовать её установки.
	// internal/toolchain.CC ищет ровно эту строку в файле, чтобы решить,
	// нужен ли "-lgc" при линковке.
	// GC_THREADS - открывает интерцепцию pthread_create/CreateThread
	// (gc_pthread_redirects.h/win32-редирект внутри <gc.h> сам подменяет их
	// на GC_pthread_create/GC_CreateThread, регистрирующие поток в GC
	// автоматически, см. ensureTaskWrapper) - должен идти ДО "#include
	// <gc.h>" (заголовочный guard <gc.h> учитывает его только на первое
	// включение).
	if ctx.usesThreads || ctx.usesHTTP2 {
		sb.WriteString("#define GC_THREADS\n")
	}
	if ctx.usesGC || ctx.usesThreads || ctx.usesHTTP2 || ctx.usesCoroutines || ctx.usesSockets {
		sb.WriteString("#include <gc.h>\n")
	}
	if ctx.includesStringH {
		sb.WriteString("#include <string.h>\n")
	}
	if ctx.usesRandom {
		// _CRT_RAND_S должен быть определён ДО <stdlib.h> - объявляет
		// rand_s (см. randomHelpers, random.go) в MSVCRT/UCRT.
		sb.WriteString("#ifdef _WIN32\n#define _CRT_RAND_S\n#endif\n")
	}
	if ctx.includesStdlibH {
		sb.WriteString("#include <stdlib.h>\n")
	}
	// <regex.h>/<locale.h> - "регулярное" (rech_regex_find_all, regex.go) -
	// POSIX regcomp/regexec/regfree + setlocale (кириллица в "\w"/классах
	// символов, см. package doc regexHelpers). Windows/MinGW не гарантирует
	// POSIX regex.h из коробки (в отличие от glibc) - тот же класс
	// ограничения переносимости, что уже принят у "Блокировка"/потоков
	// (ctx.usesThreads, POSIX-only ветка ниже) - не решён здесь, зафиксирован
	// как известное ограничение.
	if ctx.usesRegex {
		sb.WriteString("#include <regex.h>\n#include <locale.h>\n")
	}
	if ctx.includesMathH {
		sb.WriteString("#include <math.h>\n")
	}
	if ctx.usesDateTime || ctx.usesSleep {
		sb.WriteString("#include <time.h>\n")
	}
	if ctx.usesExceptions {
		sb.WriteString("#include <setjmp.h>\n")
	}
	if ctx.usesConsole {
		// termios.h/sys/ioctl.h/unistd.h - "установить_сырой_режим"/
		// "ширина_терминала"/"высота_терминала" (console.go) - на Windows не
		// нужны вовсе (windows.h уже подключён безусловно выше, консольный
		// API - оттуда же).
		sb.WriteString("#ifndef _WIN32\n#include <termios.h>\n#include <sys/ioctl.h>\n#include <unistd.h>\n#endif\n")
	}
	if ctx.usesShutdownFlag || ctx.usesSockets || ctx.usesSignals {
		// usesSockets - "signal(SIGPIPE, SIG_IGN)" (см. package doc
		// socketTypeAndConnectCode) - без него запись в уже закрытый
		// пиром сокет убивает ВЕСЬ процесс (POSIX-умолчание для SIGPIPE -
		// завершение, не просто ошибка `write`/`send`).
		sb.WriteString("#include <signal.h>\n")
	}
	if ctx.usesSignals {
		// <unistd.h> - только POSIX-ветка "rech_send_signal" (signals.go,
		// "отправить_сигнал"): kill()/pid_t объявлены там же, что и у
		// остальных POSIX-специфичных вызовов процесса в этом компиляторе -
		// на Windows не нужен, там используются GenerateConsoleCtrlEvent/
		// TerminateProcess из уже подключённого <windows.h>.
		sb.WriteString("#ifndef _WIN32\n#include <unistd.h>\n#endif\n")
	}
	if ctx.usesNonBlockingRead {
		// <poll.h> - POSIX poll() (nonblocking_read.go, "можно_прочитать_байт").
		// <io.h> - Windows _fileno/_get_osfhandle (windows.h уже подключён
		// безусловно выше, WaitForSingleObject - оттуда же).
		sb.WriteString("#ifndef _WIN32\n#include <poll.h>\n#else\n#include <io.h>\n#endif\n")
	}
	if ctx.usesSubprocess {
		// <unistd.h> - fork/execvp/pipe/dup2/chdir/_exit. <sys/wait.h> -
		// waitpid/WIFEXITED/WEXITSTATUS/WIFSIGNALED/WTERMSIG. <fcntl.h> -
		// F_SETFD/FD_CLOEXEC (самопайп, см. package doc subprocess.go).
		// <errno.h> - errno в потомке после неудачного execvp. <poll.h> -
		// как и usesNonBlockingRead, но независимо от него (программа может
		// вызвать "выполнить_команду" без единого "можно_прочитать_байт").
		// На Windows ничего из этого не нужно - CreateProcessA/CreatePipe/
		// CreateThread уже объявлены через windows.h, подключённый
		// безусловно выше.
		sb.WriteString("#ifndef _WIN32\n#include <unistd.h>\n#include <sys/wait.h>\n#include <fcntl.h>\n#include <errno.h>\n#include <poll.h>\n#endif\n")
	}
	if ctx.usesProcMem {
		// <sys/ptrace.h> - ptrace(PTRACE_ATTACH/PTRACE_DETACH). <sys/uio.h> -
		// process_vm_readv/process_vm_writev + struct iovec. <sys/wait.h> -
		// waitpid, сразу после PTRACE_ATTACH (см. package doc procmem.go).
		// Именно "__linux__", не "#ifndef _WIN32" - обе функции
		// process_vm_readv/writev - расширения Linux (см. package doc
		// procmem.go про уже найденный баг того же класса у SIGSTKFLT/
		// SIGPWR). На Windows ничего из этого не нужно - OpenProcess/
		// ReadProcessMemory/WriteProcessMemory уже объявлены через
		// windows.h, подключённый безусловно выше.
		sb.WriteString("#ifdef __linux__\n#include <sys/ptrace.h>\n#include <sys/uio.h>\n#include <sys/wait.h>\n#include <sys/types.h>\n#include <unistd.h>\n#endif\n")
	}
	if ctx.usesProcRegs {
		// <sys/user.h> - struct user_regs_struct (PTRACE_GETREGS/SETREGS,
		// procregs.go) - Linux-специфичный заголовок (тот же принцип, что
		// и usesProcMem выше). На Windows не нужен - CONTEXT уже объявлена
		// через windows.h.
		sb.WriteString("#ifdef __linux__\n#include <sys/user.h>\n#endif\n")
	}
	if ctx.usesSockets {
		// <errno.h> - только POSIX-ветка: "принять"/"установить_таймаут"
		// различают "истёк таймаут" (EAGAIN/EWOULDBLOCK) от настоящей
		// ошибки accept() - на Windows та же проверка идёт через
		// WSAGetLastError()/WSAETIMEDOUT (winsock2.h), errno там не нужен.
		// <fcntl.h> - "принять" на корутине (Шаг 132, plainSocketOpsCode)
		// временно переводит слушающий сокет в неблокирующий режим
		// (fcntl/F_GETFL/F_SETFL/O_NONBLOCK) перед accept() - дубль с
		// ctx.usesSubprocess выше безвреден (обычный include guard).
		sb.WriteString("#ifdef _WIN32\n#include <winsock2.h>\n#include <ws2tcpip.h>\n#else\n#include <sys/socket.h>\n#include <netinet/in.h>\n#include <arpa/inet.h>\n#include <netdb.h>\n#include <unistd.h>\n#include <errno.h>\n#include <fcntl.h>\n#endif\n")
	}
	// <openssl/ssl.h>/<openssl/err.h> - TLS ("слушать_tcp_защищённо", docs/
	// tls.go) - единственная внешняя (не входящая в стандартный
	// C-тулчейн) библиотека этого блока, кроме libgc/клиентов БД выше -
	// НАСТОЯЩИЙ TLS-стек, не переиспользованная collection/панический
	// хелпер, поэтому не эмитится безусловно вместе с usesSockets - только
	// когда программа реально вызвала "слушать_tcp_защищённо" (-lssl
	// -lcrypto - internal/toolchain/cc.go, по маркеру этого #include).
	if ctx.usesTLS {
		sb.WriteString("#include <openssl/ssl.h>\n#include <openssl/err.h>\n")
	}
	// <openssl/evp.h>/<openssl/rand.h> - AES-256-GCM ("зашифровать"/
	// "расшифровать", aes.go) - тот же OpenSSL, что и у TLS выше, но
	// отдельный флаг/отдельный маркер для -lcrypto (internal/toolchain/
	// cc.go): "зашифровать"/"расшифровать" не тянут libssl (нет TLS-
	// рукопожатия), только libcrypto (EVP/RAND) - не одно и то же с usesTLS,
	// программа может использовать одно без другого.
	if ctx.usesAES {
		sb.WriteString("#include <openssl/evp.h>\n#include <openssl/rand.h>\n")
	}
	// <openssl/pem.h>/<openssl/bio.h> - асимметричная подпись
	// ("сгенерировать_приватный_ключ"/"получить_публичный_ключ"/
	// "подписать"/"проверить_подпись", signing.go) - <openssl/evp.h> тут же
	// (EVP_PKEY и т. п.), но не дублируется, если usesAES уже его подключил
	// - "-lcrypto" всё равно достаточно (проверка cc.go по любому из двух
	// маркеров - "#include <openssl/evp.h>" присутствует в обоих случаях).
	if ctx.usesSigning {
		if !ctx.usesAES {
			sb.WriteString("#include <openssl/evp.h>\n")
		}
		sb.WriteString("#include <openssl/pem.h>\n#include <openssl/bio.h>\n")
	}
	// <nghttp2/nghttp2.h> - HTTP/2-транспорт (http2.go) - единственная
	// внешняя библиотека этого блока, кроме OpenSSL выше (-lnghttp2,
	// internal/toolchain/cc.go, по маркеру этого #include) - POSIX-only
	// (см. package doc http2.go, "#error" на _WIN32 внутри самого рантайма),
	// поэтому <sys/time.h> (struct timeval, тайм-аут recv() у IO-потока)
	// подключается тут же, безусловно вместе с ним.
	if ctx.usesHTTP2 {
		sb.WriteString("#include <nghttp2/nghttp2.h>\n#include <sys/time.h>\n")
	}
	// <png.h>/<jpeglib.h>/<setjmp.h> - PNG/JPEG-кодек (png.go/jpeg.go/
	// imagecodec.go, "декодировать_изображение"/"закодировать_изображение")
	// - libpng+libjpeg, обе разом (-lpng -ljpeg - internal/toolchain/cc.go,
	// по маркерам этих #include) - третий срез работы с изображениями
	// ("Работа с изображениями"); <stdio.h> (FILE) уже
	// подключён безусловно выше - <jpeglib.h> сам его требует, но не
	// включает. setjmp/longjmp - собственный обработчик ошибок libjpeg
	// (package doc jpeg.go), НЕ через ctx.usesExceptions (тот подключается
	// только при "попытаться"/"поймать" в самой Rech-программе, независимая
	// причина).
	if ctx.usesImageCodec {
		sb.WriteString("#include <png.h>\n#include <jpeglib.h>\n#include <setjmp.h>\n")
	}
	// Потоки ("запустить"/"дождаться") и "Блокировка" - на Windows
	// CreateThread/CRITICAL_SECTION уже объявлены через windows.h выше,
	// отдельного заголовка не нужно; на POSIX - pthread.h (pthread_mutex_t
	// оттуда же, что и pthread_create - один и тот же заголовок для обоих
	// usesThreads/usesMutex, поэтому один общий #include, не два одинаковых).
	// usesTLS/prog.UsesDB - тоже pthread_mutex_t (реестр TLS-соединений,
	// tls.go, rech_tls_lock; общий мьютекс rech_db_t, db.go) - их
	// собственная блокировка нужна независимо от того, использует ли
	// ПРОГРАММА "запустить"/"Блокировка" - "БД" может быть полем сущности
	// или параметром функции, разделяемым между потоками, запущенными где
	// угодно в программе - её собственная блокировка защищает ЭТО
	// соединение, независимо от того, как именно программа организует
	// свою многопоточность.
	if ctx.usesThreads || ctx.usesMutex || ctx.usesSemaphore || ctx.usesTLS || prog.UsesDB || ctx.usesActiveTaskWait {
		sb.WriteString("#ifndef _WIN32\n#include <pthread.h>\n#endif\n")
	}
	sb.WriteString("\n")
	// RECH_THREAD_LOCAL - определён здесь, безусловно, ОДИН раз (не C11
	// _Thread_local, см. exceptions.go) - используется и traceHelper ниже
	// (стек вызовов, эмитится всегда), и exceptionRuntimeCode (только при
	// ctx.usesExceptions, см. exceptions.go) - раньше определялся только
	// там, теперь общий для обоих потребителей.
	sb.WriteString("#if defined(_WIN32)\n#define RECH_THREAD_LOCAL __declspec(thread)\n#else\n#define RECH_THREAD_LOCAL __thread\n#endif\n")
	sb.WriteString(traceHelper)
	sb.WriteString("\n")
	sb.WriteString(consoleInitHelper)
	sb.WriteString("\n")
	sb.WriteString(argsGlobals)
	sb.WriteString("\n")

	// usesCookies - rech_cookie_string (cookies.go, ниже) сама
	// собирается через rech_str_concat.
	appendCodeIf(&sb, ctx.usesConcat || ctx.usesCookies, strConcatHelper)
	// ctx.usesCast - castHelpers (ниже) - один общий блок текста на все
	// явные касты (int/float/char <-> Строка, см. package doc cast.go), не
	// отдельная функция на каждый - несколько из них (разбор строки в
	// число/символ) сами вызывают rech_panic при ошибке разбора, даже если
	// в программе встретился только каст, который панике не подвержен
	// (например, число -> Строка) - тот же принцип, что и у usesGenerics
	// здесь: panicHelper должен быть виден целиком, раз в файле есть хотя
	// бы один потенциальный вызывающий его блок текста.
	//
	// ctx.usesPanicCatch - полное тело rech_panic эмитится не здесь, а
	// НИЖЕ, после exceptionRuntimeCode (panicCatchHelperCode читает
	// rech_catch_frame/rech_catch_top оттуда) - здесь в этом случае только
	// прототип, чтобы все вызовы rech_panic() выше (readHelpers/
	// socketHelpers/castHelpers и т. п., ниже этой строки, но раньше
	// exceptionRuntimeCode) остались валидным C (объявление обязано
	// предшествовать первому вызову).
	panicGate := usesGenerics || usesNullables || ctx.usesPanic || ctx.usesCast || prog.UsesReflection || ctx.usesPanicCatch
	if ctx.usesPanicCatch {
		appendCodeIf(&sb, panicGate, "static void rech_panic(const char *msg);\n")
	} else {
		appendCodeIf(&sb, panicGate, panicHelper)
	}
	// coroutineRuntimeCode (coroutine.go) - ЗДЕСЬ, не рядом с taskCode/
	// coroTaskCode ниже (как было раньше, до Шага 132): rech_accept/
	// rech_send_raw/rech_socket_read_raw (plainSocketOpsCode,
	// builtins_shared.go, эмитируется дальше по файлу как часть
	// socketHelpers) теперь тоже ссылаются на rech_coro_current/
	// rech_coro_wait_readable/writable текстом - объявление обязано
	// предшествовать первому использованию, а socketHelpers эмитируется
	// значительно раньше, чем прежнее место (около taskCode). Зависит
	// только от rech_panic (сразу выше - panicGate безусловно true для
	// любой программы, см. её package doc про usesNullables/"?Сокет") и
	// "#include <gc.h>" (эмитирован ещё раньше, в начале файла) - никаких
	// других зависимостей у самого рантайма корутин нет. Гейтится
	// ctx.usesCoroutines (есть хотя бы одна "дождаться <асинхронная
	// функция>") ИЛИ ctx.usesSockets (сетевые builtin'ы теперь ВСЕГДА
	// корутино-осведомлены во время исполнения - см. package doc
	// plainSocketOpsCode, Шаг 132) - программа без обоих вовсе не платит
	// за ucontext/epoll/kqueue рантайм.
	appendCodeIf(&sb, ctx.usesCoroutines || ctx.usesSockets, coroutineRuntimeCode)
	appendCodeIf(&sb, ctx.usesArgs, argFuncHelper)
	appendCodeIf(&sb, ctx.usesCurrentOS, currentOSHelper)
	appendCodeIf(&sb, ctx.usesCast, castHelpers)

	if ctx.intDivMod.any() || ctx.uintDivMod.any() {
		// Прототип - ДО текста хелперов ниже (они уже вызывают
		// rech_divzero_throw): определение (divZeroThrowHelperCode) эмитится
		// сильно позже, вместе с structParseThrowHelperCode (нужны и
		// rech_throw, и прототип rech_new_<ОшибкаДеленияНаНоль> из
		// protos, оба готовы только там) - без прототипа здесь был бы
		// неявный вызов необъявленной функции (ошибка в C99+).
		sb.WriteString("static void rech_divzero_throw(const char *msg);\n")
	}
	sb.WriteString(intDivModHelperCode(ctx.intDivMod, true))
	sb.WriteString(intDivModHelperCode(ctx.uintDivMod, false))
	appendCodeIf(&sb, ctx.usesConsoleInput, readHelpers)
	appendCodeIf(&sb, ctx.usesFileIO, fileHelpers)
	// rechSocketTypedef - см. её package doc (builtins_shared.go) - БЕЗУСЛОВНО
	// (не ctx.usesSockets - "?Сокет", обёртка "принять", попадает в
	// prog.Nullables для ЛЮБОЙ программы независимо от реального
	// использования сетей, значит и rech_socket_t, на который эта обёртка
	// ссылается, обязан существовать всегда). Остальной рантайм сокетов
	// (socketHelpers/socketHelpersTLS, включая rech_accept) по-прежнему
	// условный и эмитируется НАМНОГО позже, после nullablesCode ниже.
	sb.WriteString(rechSocketTypedef)
	if prog.UsesReflection {
		// Перед genericsCode - Список<Значение> ссылается на rech_any_t как
		// на свой C-тип элемента (см. package doc reflectTypedefs) - тот же
		// класс проблемы, что и у genericsCode/typedefs ниже.
		sb.WriteString(reflectTypedefs)
		sb.WriteString(primitiveReflectDescriptors())
		sb.WriteString(nullableReflectDescriptors(prog))
		sb.WriteString(reflectUnboxCheckHelper)
		sb.WriteString("\n")
		sb.WriteString(reflectUnboxCheckIfaceHelper)
		sb.WriteString("\n")
		sb.WriteString(reflectLookupHelpers)
		sb.WriteString("\n")
		sb.WriteString(reflectTagLookupHelper)
		sb.WriteString("\n")
	}

	// Перед genericsCode/typedefs - сущность может нести поле типа "БД"
	// (см. package doc dbHandleTypedefCode, db.go) - её C-typedef
	// ("rech_db_t") обязан быть виден раньше typedef самой сущности,
	// раньше, чем полный dbRuntimeCode (тот эмитируется намного позже -
	// зависит от genericsCode, которого здесь ещё нет).
	appendCodeIf(&sb, prog.UsesDB, dbHandleTypedefCode(prog.DBPoolSize, prog.DBPoolTimeoutMs, prog.DBSQLiteBusyTimeoutMs))

	// Тот же принцип, что и dbHandleTypedefCode выше - сущность "HTTP2Поток"
	// несёт поле типа "HTTP2Сессия" (rech_http2_session_t *), её typedef
	// обязан быть виден раньше typedef самой "HTTP2Поток". http2CallbacksCode
	// (nghttp2-колбэки) не зависит ни от genericsCode/typedefs, ни от
	// socketHelpers/bytesWrapHelper (те нужны только http2SessionOpsCode -
	// см. её отдельную, более позднюю точку эмиссии) - можно эмитить сразу
	// вместе с typedef'ом.
	if ctx.usesHTTP2 {
		sb.WriteString(http2SessionTypedefCode)
		sb.WriteString(http2CallbacksCode(ctx.usesTLS))
	}

	// genericForwardDecl - "typedef struct rech_T_... rech_T_...;" для
	// КАЖДОЙ generic-инстанциации программы (Список/Стопка/Очередь/Словарь) -
	// опережает ЛЮБОЙ typedef ниже (genericsCode/typedefs/
	// genericsCodeAfterEntities/...), тем же приёмом, что и у header.go
	// (generateHeader, opaque forward declaration ссылочных сущностей для
	// потребителя библиотеки): generic-коллекция - ВСЕГДА ссылочный тип
	// (ctx.entityIsRef, generics-ветка), значит поле сущности/структуры,
	// которое несёт коллекцию, - это на C-уровне всего лишь указатель
	// (rech_T_...  *) и ему не нужно ничего, кроме имени типа, объявленного
	// ХОТЯ БЫ неполным (opaque) - настоящее тело (listCollectionCCode/
	// stackCollectionCCode/queueCollectionCCode/dictCollectionCCode, теперь
	// "struct rech_T_... {...};", а не анонимный "typedef struct {...}
	// rech_T_...;" - анонимную структуру нельзя было бы объявить неполной
	// заранее, у неё попросту нет тега) по-прежнему эмитится в СВОЙ обычный
	// бакет (genericsCode/genericsCodeAfterCallables/genericsCodeAfterEntities,
	// ниже) - порядок между самими бакетами не меняется, коллекция с
	// сущностью/структурой-элементом всё так же ждёт typedef этой
	// сущности/структуры для СВОЕГО тела (см. package doc
	// genericsCodeAfterEntities). Снимает единственную причину, по которой
	// internal/semantic раньше запрещал такой коллекции быть полем сущности/
	// структуры (collectionRealEntityElem, теперь не вызывается).
	var genericForwardDecl strings.Builder
	for _, g := range prog.Generics {
		structName := mangleType(g.Name)
		fmt.Fprintf(&genericForwardDecl, "typedef struct %[1]s %[1]s;\n", structName)
	}
	appendCodeIf(&sb, genericForwardDecl.Len() > 0, genericForwardDecl.String())

	appendCodeIf(&sb, genericsCode.Len() > 0, genericsCode.String())

	// После genericsCode (внутренний тип "?Тип" может сам быть Список/
	// Словарь) и перед typedefs (сущность может нести nullable-поле по
	// значению) - тот же класс проблемы упорядочивания, что и у
	// genericsCode/typedefs ниже.
	appendCodeIf(&sb, nullablesCode.Len() > 0, nullablesCode.String())
	if prog.UsesReflection {
		hasReflectAnyList := false
		for _, g := range prog.Generics {
			if g.Name == reflectAnyListType {
				hasReflectAnyList = true
				break
			}
		}
		if hasReflectAnyList {
			// После genericsCode выше - reflectDispatchHelpersCode ссылается
			// на C-тип Список<Значение> (mangleType) по значению - эмитится,
			// только если пользователь его реально где-то сконструировал
			// ("новый Список<Значение>()") - без этого у "значение.вызвать"/
			// "тип.создать" не может быть ни одного валидного вызова вообще
			// (их сигнатура требует такой список аргументом, см.
			// internal/semantic/reflect.go), значит typedef уже
			// гарантированно существует в genericsCode выше.
			sb.WriteString(reflectDispatchHelpersCode())
			sb.WriteString("\n")
		}
		// reflectFieldIndexByTagHelperCode/reflectGetFieldByTagHelperCode -
		// см. их package doc (reflect.go) - тем же принципом "после
		// nullablesCode", что и hasReflectAnyList выше.
		sb.WriteString(reflectFieldIndexByTagHelperCode())
		hasReflectAnyField := false
		for _, n := range prog.Nullables {
			if n.Name == ir.NullableType(ir.TypeAny).Entity {
				hasReflectAnyField = true
				break
			}
		}
		if hasReflectAnyField {
			sb.WriteString(reflectGetFieldByTagHelperCode())
		}
	}
	appendCodeIf(&sb, typedefs.Len() > 0, typedefs.String())
	appendCodeIf(&sb, staticDecl != "", staticDecl)
	appendCodeIf(&sb, nullablesAfterTypedefs.Len() > 0, nullablesAfterTypedefs.String())

	// После ОБЕИХ групп nullablesCode/nullablesAfterTypedefs - rech_accept
	// (socketHelpers/socketHelpersTLS) возвращает "?Сокет" (обёртку
	// rech_T_n_soket_wrap/_empty, nullable.go), которая уже должна
	// существовать (см. package doc rechSocketTypedef, builtins_shared.go, -
	// сам typedef rech_socket_t эмитирован намного раньше, только эта,
	// остальная часть рантайма сокетов - здесь).
	if ctx.usesTLS {
		// socketHelpersTLS (tls.go) ВМЕСТО socketHelpers - тот же
		// socketTypeAndConnectCode (слушать/подключиться, TLS не касается),
		// но TLS-осведомлённые rech_accept/rech_send/rech_recv/
		// rech_close_socket - см. package doc tls.go.
		sb.WriteString(socketHelpersTLS)
	} else {
		appendCodeIf(&sb, ctx.usesSockets, socketHelpers)
	}

	// После genericsCode (нужен C-typedef/find-функция самой коллекции)
	// И после ОБЕИХ групп nullablesCode/nullablesAfterTypedefs (нужна
	// обёртка "?T" элемента, независимо от того, в какую из двух групп
	// она попала) - см. package doc internal/backend/cbackend/index.go.
	appendCodeIf(&sb, indexSafeGetCode.Len() > 0, indexSafeGetCode.String())

	// После typedefs - использует rech_vtable_header/rech_T_Исключение,
	// оба уже должны быть объявлены (см. exceptions.go).
	appendCodeIf(&sb, ctx.usesExceptions, exceptionRuntimeCode(ctx))
	// panicCatchHelperCode - полное тело rech_panic, когда программа
	// содержит "поймать (x: Паника)" (см. прототип вместо panicHelper
	// выше) - после exceptionRuntimeCode: читает rech_catch_frame/
	// rech_catch_top, определённые там.
	appendCodeIf(&sb, ctx.usesPanicCatch, panicCatchHelperCode())
	appendCodeIf(&sb, callableTypedefs.Len() > 0, callableTypedefs.String())
	// После callableTypedefs - см. её package doc выше (genericsCode).
	appendCodeIf(&sb, genericsCodeAfterCallables.Len() > 0, genericsCodeAfterCallables.String())
	// После ОБЕИХ групп typedefs/typedefsAfterCallables (rech_struct_eq_...
	// сравнивает только уже объявленные поля структуры - typedef структуры
	// достаточно, ничего из generic-коллекций ей не нужно) и ДО
	// genericsCodeAfterEntities ниже - "Список<Структура>.равен" (её dest -
	// именно genericsCodeAfterEntities, см. package doc entitySig.Equatable)
	// зовёт rech_struct_eq_... элемента, прототип/тело обязаны быть уже
	// видны C здесь, а не после (как structEqProtos/structEqFuncs стояли
	// раньше, ДО того, как "равен" начал ссылаться на них извне struct_eq.go).
	appendCodeIf(&sb, typedefsAfterCallables.Len() > 0, typedefsAfterCallables.String())
	appendCodeIf(&sb, structEqProtos.Len() > 0, structEqProtos.String())
	appendCodeIf(&sb, structEqFuncs.Len() > 0, structEqFuncs.String())
	// После ОБЕИХ групп typedefs/typedefsAfterCallables (коллекция с
	// сущностью-элементом ждёт typedef именно этой сущности, а та могла
	// попасть в любую из двух групп) - см. package doc genericsCodeAfterEntities
	// выше. До showFuncs ниже - её третий проход зовёт rech_show_<такая
	// коллекция>, функция должна быть уже объявлена.
	appendCodeIf(&sb, genericsCodeAfterEntities.Len() > 0, genericsCodeAfterEntities.String())
	// После genericsCodeAfterEntities - см. package doc callableTypedefsAfterEntities выше.
	appendCodeIf(&sb, callableTypedefsAfterEntities.Len() > 0, callableTypedefsAfterEntities.String())
	appendCodeIf(&sb, showProtos.Len() > 0, showProtos.String())
	appendCodeIf(&sb, ctx.usesShow && showFuncs.Len() > 0, showFuncs.String())
	// rech_dup - общий для HTTP-клиента/сервера и получить_от (адрес
	// отправителя UDP-датаграммы копируется через rech_dup) - эмитится
	// один раз перед всеми тремя (см. rechDupHelper).
	appendCodeIf(&sb, ctx.usesHTTP || ctx.usesHTTPServer || ctx.usesUDPDatagram || ctx.usesMultipart, rechDupHelper)
	// chunkedHelpers - разбор chunked transfer encoding, нужен и клиенту
	// (httpHelpers), и серверу
	// (httpServerHelpers) - эмитится один раз перед обоими, тем же
	// принципом, что и rechDupHelper выше.
	appendCodeIf(&sb, ctx.usesHTTP || ctx.usesHTTPServer, chunkedHelpers)

	// После typedefs - rech_http_request принимает/возвращает Запрос/
	// Ответ по значению, их typedef должен быть уже виден.
	appendCodeIf(&sb, ctx.usesHTTP, httpHelpers)
	// После typedefs, тем же принципом, что и usesHTTP выше.
	appendCodeIf(&sb, ctx.usesHTTPServer, httpServerHelpersCode(prog.HTTPMaxRequestBytes))
	// После typedefs и rechDupHelper выше - rech_recv_from возвращает
	// "Дейтаграмма" по значению (её typedef уже должен быть виден) и
	// зовёт rech_dup для адреса отправителя.
	appendCodeIf(&sb, ctx.usesUDPDatagram, udpDatagramHelpers())
	// После genericsCode выше - rech_query_params использует typedef/
	// метод "установить" Словарь<Строка, Строка>, уже эмитированные там
	// (queryParamsDictType зарегистрирован в prog.Generics принудительно,
	// см. internal/semantic/httpserver.go).
	appendCodeIf(&sb, ctx.usesQueryParams, queryParamsHelper)
	// После genericsCode выше - rech_bytes_wrap (bytesio.go, bytesWrapHelper)
	// использует typedef Список<Байт> (зарегистрирован в prog.Generics
	// принудительно, internal/semantic/bytesio.go). Нужна и bytesIOHelpers
	// ниже (файловый ввод-вывод), и httpBytesHelpers (сетевой) - общий
	// флаг, эмитится РОВНО один раз для программы, использующей и то, и
	// другое сразу.
	// ctx.usesProcMem - живой найденный баг: rech_proc_read (procmem.go,
	// эмитируется ОДНИМ куском со всеми функциями мини-отладчика при
	// подключиться_к_процессу/отключиться_от_процесса/регистры_процесса/
	// точке останова/возобновить - не только настоящем чтении памяти)
	// безусловно зовёт rech_bytes_wrap - гейтить отдельным флагом на
	// КАЖДОМ вызывающем месте (internal/backend/cbackend/builtins_
	// shared.go) ненадёжно, легко забыть при следующей функции того же
	// семейства - надёжнее здесь, в точке эмиссии самого rech_proc_read.
	appendCodeIf(&sb, ctx.usesByteListWrap || ctx.usesProcMem, bytesWrapHelper())
	// http2SessionOpsCode - после socketHelpers/socketHelpersTLS
	// (rech_connect_tcp) И bytesWrapHelper выше (rech_http2_read_data зовёт
	// rech_bytes_wrap) И typedefs (её typedef "HTTP2Поток" уже должен быть
	// виден) - typedef/колбэки самой сессии эмитированы РАНЬШЕ (см. их
	// отдельную точку выше, рядом с dbHandleTypedefCode).
	if ctx.usesHTTP2 {
		sb.WriteString(http2SessionOpsCode(ctx.usesTLS))
	}
	// grpcSessionOpsCode - ПОСЛЕ http2SessionOpsCode выше (зовёт rech_http2_*
	// по имени, см. package doc grpc.go) И после nullablesCode/
	// nullablesAfterTypedefs (обе группы уже эмитированы раньше по коду
	// функции - см. их точки выше; "?Список<Байт>"_wrap/_empty оттуда).
	if ctx.usesGRPC {
		sb.WriteString(grpcSessionOpsCode())
	}
	// grpcsListenAndConnectCode - ПОСЛЕ http2SessionOpsCode выше (зовёт
	// rech_http2_new_callbacks/rech_http2_start_thread по имени) И после
	// socketHelpersTLS (rech_listen_tcp/rech_tls_register/SSL_CTX_new -
	// usesGRPCS форсирует usesTLS, см. её дизайн-flag в builtins_shared.go)
	// - см. package doc grpcs.go.
	if ctx.usesGRPCS {
		sb.WriteString(grpcsListenAndConnectCode())
	}
	// После bytesWrapHelper выше - rech_bytes_from_string зовёт
	// rech_bytes_wrap напрямую.
	appendCodeIf(&sb, ctx.usesByteStringConv, byteStringConvHelpers())
	// После bytesWrapHelper (rech_bytes_wrap) И rechDupHelper (rech_dup)
	// выше - rech_parse_multipart (multipart.go) использует обе.
	appendCodeIf(&sb, ctx.usesMultipart, multipartHelpers())
	// После genericsCode выше И после fileHelpers (usesFileIO, эмитирована
	// намного раньше) - rech_read_bytes и соседи (internal/backend/
	// cbackend/bytesio.go) используют typedef/методы Список<Байт>
	// (зарегистрирован в prog.Generics принудительно, internal/semantic/
	// bytesio.go) и rech_file_t/rech_open_mode из fileHelpers.
	appendCodeIf(&sb, ctx.usesBytesIO, bytesIOHelpers())
	// После bytesWrapHelper выше (rech_bytes_wrap) - rech_png_decode_file
	// (png.go)/rech_jpeg_decode_file (jpeg.go) строят через неё поле
	// "пиксели"; typedef "ДекодированноеИзображение" эмитирован раньше
	// вместе с остальными entity-typedef'ами (обычная сущность, тот же
	// путь, что и у RGBЦвет). rech_decode_image_file/rech_encode_image_file
	// (imagecodec.go) - диспетчер поверх обеих - должен идти ПОСЛЕ них.
	appendCodeIf(&sb, ctx.usesImageCodec, pngHelpers)
	appendCodeIf(&sb, ctx.usesImageCodec, jpegHelpers)
	appendCodeIf(&sb, ctx.usesImageCodec, imageCodecDispatchHelpers)
	// После genericsCode/bytesWrapHelper выше И после httpServerHelpersCode
	// (usesHTTPServer, эмитирована раньше - rech_recv_http_request/
	// rech_http_parse_head) - rech_http_accept_request_bytes/rech_http_
	// send_response_bytes (httpbytes.go) используют и то, и другое.
	appendCodeIf(&sb, ctx.usesHTTPBytes, httpBytesHelpers())
	// После genericsCode выше - rech_split_string использует typedef/метод
	// "добавить" Список<Строка>, уже эмитированные там (splitResultType
	// зарегистрирован в prog.Generics принудительно, см. internal/semantic/
	// stringsplit.go).
	appendCodeIf(&sb, ctx.usesStringSplit, stringSplitHelper)
	// После genericsCodeAfterEntities выше (не только genericsCode) -
	// rech_regex_find_all использует typedef/метод "добавить" Список<
	// Совпадение> (элемент - сущность "Совпадение", см. её package doc
	// genericsCodeAfterEntities) И typedefs (само поле "текст"/"начало"/
	// "группы" сущности "Совпадение", regexResultType/regexGroupsType
	// зарегистрированы в prog.Generics принудительно, см. internal/semantic/
	// regex.go).
	appendCodeIf(&sb, ctx.usesRegex, regexHelpers)
	// После typedefs - rech_time_to_date/rech_date_to_time/
	// rech_date_to_string принимают/возвращают ДатаВремя по значению.
	appendCodeIf(&sb, ctx.usesDateTime, datetimeHelpers)
	// После typedefs (rech_T_n_data_vremya - "?ДатаВремя" - уже объявлен,
	// если usesDateParse) - rech_parse_date не зависит от datetimeHelpers
	// выше (чистый разбор строки, без календарной арифметики).
	appendCodeIf(&sb, ctx.usesDateParse, dateParseHelpers(mangleType("ДатаВремя"),
		mangleField("год"), mangleField("месяц"), mangleField("день"),
		mangleField("час"), mangleField("минута"), mangleField("секунда"),
		mangleType("?ДатаВремя")))
	// После datetimeHelpers выше (rech_time_to_date_tz/rech_date_to_time_tz
	// зовут rech_time_to_date/rech_date_to_time/rech_days_from_civil/
	// rech_civil_from_days) и после typedefs ("?ДатаВремя"/"?Целое").
	appendCodeIf(&sb, ctx.usesTZ, tzHelpers(tzdataDir(), mangleType("ДатаВремя"),
		mangleField("год"), mangleField("месяц"), mangleField("день"),
		mangleField("час"), mangleField("минута"), mangleField("секунда"),
		mangleType("?ДатаВремя"), mangleType("?Целое")))
	// После datetimeHelpers выше (rech_day_of_week зовёт rech_days_from_civil).
	appendCodeIf(&sb, ctx.usesDayOfWeek, dayOfWeekHelper(mangleType("ДатаВремя"),
		mangleField("год"), mangleField("месяц"), mangleField("день")))
	// После datetimeHelpers выше (rech_date_to_human зовёт rech_date_to_time/
	// rech_time_now).
	appendCodeIf(&sb, ctx.usesHumanDate, humanDateHelper(mangleType("ДатаВремя")))
	appendCodeIf(&sb, ctx.usesColor, colorHelpers)
	// numbase.go - "целое_в_*"/"целое_из_*" - не зависит от typedefs, только
	// примитивы (Целое/Строка) - "из_*" зовёт rech_panic (panicHelper выше).
	appendCodeIf(&sb, ctx.usesNumBase, numBaseToStringHelpers)
	appendCodeIf(&sb, ctx.usesNumBase, numBaseFromStringHelpers())
	// process.go - "спать_мс" - не зависит от typedefs, только примитив
	// (Целое).
	appendCodeIf(&sb, ctx.usesSleep, processHelpers)
	// Не зависит от typedefs вообще (только примитивы) - здесь же просто
	// для единообразия с остальными builtin-хелперами шага 8.
	appendCodeIf(&sb, ctx.usesRandom, randomHelpers)
	// Не зависит от typedefs - только примитивы (const char *) - после
	// panicHelper выше (rech_substring зовёт rech_panic).
	appendCodeIf(&sb, ctx.usesStringOps, stringOpsHelpers)
	// Не зависит от typedefs - только примитивы (const char *) - после
	// panicHelper выше (rech_base64url_decode зовёт rech_panic).
	appendCodeIf(&sb, ctx.usesCrypto, cryptoHelpers)
	// aes.go - "зашифровать"/"расшифровать" - ПОСЛЕ cryptoHelpers выше
	// (rech_aes_key_from_passphrase зовёт rech_sha256_*, rech_aes_encrypt_
	// hex зовёт rech_bytes_to_hex) - usesAES всегда идёт вместе с
	// usesCrypto=true (builtins_shared.go), так что порядок гарантирован.
	appendCodeIf(&sb, ctx.usesAES, aesHelpers)
	// signing.go - "сгенерировать_приватный_ключ"/"получить_публичный_
	// ключ"/"подписать"/"проверить_подпись" - ПОСЛЕ cryptoHelpers (rech_
	// sign_hex зовёт rech_bytes_to_hex, rech_verify_signature_hex зовёт
	// rech_hex_decode) и ПОСЛЕ panicHelper (обе тянут rech_panic) - оба уже
	// гарантированы: usesSigning всегда идёт вместе с usesCrypto=true.
	appendCodeIf(&sb, ctx.usesSigning, signingHelpers)
	// После typedefs (rech_T_opcii_kuki/rech_T_n_stroka уже объявлены) и
	// после strConcatHelper выше (rech_cookie_string зовёт rech_str_concat).
	appendCodeIf(&sb, ctx.usesCookies, cookieHelpers())
	// После typedefs (rech_T_n_stroka - "?Строка" - уже объявлен, тем же
	// принципом, что и usesCookies выше).
	appendCodeIf(&sb, ctx.usesEnv, envHelpers(mangleType("?Строка")))
	// После typedefs (rech_T_n_stroka уже объявлен) - тем же принципом,
	// что и usesEnv/usesCookies выше.
	appendCodeIf(&sb, ctx.usesHTTPHeaders, headerValueHelper(mangleType("?Строка")))
	// Не зависит от typedefs (только sig_atomic_t/int) - можно эмитировать
	// в любом месте после panicHelper, здесь - просто рядом с остальными
	// небольшими самодостаточными хелперами.
	appendCodeIf(&sb, ctx.usesShutdownFlag, shutdownFlagCode)
	appendCodeIf(&sb, ctx.usesSignals, signalRuntimeCode())
	appendCodeIf(&sb, ctx.usesSubprocess, subprocessRuntimeCode)
	appendCodeIf(&sb, ctx.usesProcMem, procMemRuntimeCode)
	appendCodeIf(&sb, ctx.usesProcMemFind, procMemFindHelper())
	appendCodeIf(&sb, ctx.usesProcRegs, procRegsRuntimeCode())
	appendCodeIf(&sb, ctx.usesConsole, consoleRuntimeCode())
	appendCodeIf(&sb, ctx.usesNonBlockingRead, nonBlockingReadRuntimeCode())
	// "rech_db_t" сам уже виден (dbHandleTypedefCode, эмитирован раньше
	// typedefs/genericsCode выше) - здесь только остальной рантайм БД
	// (подключение/запросы/подстановка параметров), который ссылается на
	// generic-типы (Список<Значение> и т. п.) - До protos ниже, прототипы
	// функций, принимающих/возвращающих БД, ссылаются на функции отсюда.
	// Зависит от genericsCode и primitiveReflectDescriptors выше - оба уже
	// эмитированы к этому месту - см. package doc dbRuntimeCode/
	// dbHandleTypedefCode (internal/backend/cbackend/db.go).
	appendCodeIf(&sb, prog.UsesDB, dbRuntimeCode(prog.DBBackends))
	// Транзакции БД (internal/backend/cbackend/dbtx.go) - только если
	// реально использованы (ctx.usesDBTransaction) - после dbRuntimeCode
	// выше (rech_db_checkout/rech_db_query/rech_db_exec, на которые
	// dbtxRuntimeCode ссылается, уже должны существовать).
	if ctx.usesDBTransaction {
		appendCodeIf(&sb, true, dbtxRunSimpleCode(prog.DBBackends))
		appendCodeIf(&sb, true, dbtxRuntimeCode(ctx))
	}
	appendCodeIf(&sb, protos.Len() > 0, protos.String())

	// vtables (ссылается на &rech_M_... - нужны protos выше) и ctors
	// (ссылается на &rech_VT_... у участвующих в диспетчеризации сущностей
	// - нужны vtables) - строго в этом порядке, оба перед body (который их
	// использует).
	appendCodeIf(&sb, vtables.Len() > 0, vtables.String())
	appendCodeIf(&sb, ctors.Len() > 0, ctors.String())
	appendCodeIf(&sb, staticInit != "", staticInit)
	// После ctors - функции извлечения ("разобрать JSON"/"разобрать
	// XML") конструируют целевые сущности через уже сгенерированный
	// rech_new_<Тип> (mangleCtor) и выбрасывают "ОшибкаРазбора" через
	// rech_struct_throw/rech_throw (exceptionRuntimeCode выше) - общий
	// для обоих форматов, эмитится один раз.
	appendCodeIf(&sb, len(prog.ParseTargets) > 0 || len(prog.XMLParseTargets) > 0 || len(prog.YAMLParseTargets) > 0 || len(prog.ProtobufParseTargets) > 0, structParseThrowHelperCode())
	appendCodeIf(&sb, ctx.intDivMod.any() || ctx.uintDivMod.any(), divZeroThrowHelperCode())
	appendCodeIf(&sb, ctx.usesStateFlags, requireStateHelperCode())
	appendCodeIf(&sb, ctx.usesStateFlags, revokeStatesHelperCode())

	err = resolveRuntimeCode(
		&sb,
		ctx,
		prog,
		prog.ParseTargets,
		jsonRuntimeCode,
	)
	if err != nil {
		return "", err
	}

	appendCodeIf(&sb, len(prog.XMLParseTargets) > 0 || len(prog.XMLSerializeTargets) > 0, xmlSbCode)

	err = resolveRuntimeCode(&sb, ctx, prog, prog.XMLParseTargets, xmlRuntimeCode)
	if err != nil {
		return "", err
	}

	err = resolveRuntimeCode(&sb, ctx, prog, prog.YAMLParseTargets, yamlRuntimeCode)
	if err != nil {
		return "", err
	}

	err = resolveRuntimeCode(&sb, ctx, prog, prog.ProtobufParseTargets, protobufParseRuntimeCode)
	if err != nil {
		return "", err
	}

	// dbRowsRuntimeCode - функции "запросить(...) как Список<Тип>"
	// (ir.DBRowsAsExpr, db_rows.go) - тем же принципом, что jsonRuntimeCode
	// выше: нужны уже готовый rech_new_<Тип> (ctors выше) и Список<Тип>
	// (genericsCodeAfterEntities, эмитирован ещё раньше typedefs). Зовёт
	// rech_panic через reflectCheckedUnboxExprCode - panicHelper уже
	// безусловно эмитирован раньше (prog.UsesReflection уже true всегда,
	// когда достижим этот путь - registerDBTypesOnCall, db.go), отдельный
	// ctx.usesPanic здесь не нужен.
	err = resolveRuntimeCode(&sb, ctx, prog, prog.DBRowTargets, dbRowsRuntimeCode)
	if err != nil {
		return "", err
	}

	// Обратное к разбору выше - не зависит от ctors/exceptionRuntimeCode
	// (сериализация не создаёт новых сущностей и не может провалиться),
	// только от typedefs.
	err = resolveRuntimeCode(&sb, ctx, prog, prog.SerializeTargets, jsonSerializeRuntimeCode)
	if err != nil {
		return "", err
	}

	err = resolveRuntimeCode(&sb, ctx, prog, prog.XMLSerializeTargets, xmlSerializeRuntimeCode)
	if err != nil {
		return "", err
	}

	err = resolveRuntimeCode(&sb, ctx, prog, prog.YAMLSerializeTargets, yamlSerializeRuntimeCode)
	if err != nil {
		return "", err
	}

	err = resolveRuntimeCode(&sb, ctx, prog, prog.ProtobufSerializeTargets, protobufSerializeRuntimeCode)
	if err != nil {
		return "", err
	}

	if len(prog.ReflectTargets) > 0 {
		// Прототипы ДО reflectRuntimeCode - её method-таблицы зовут эти
		// хелперы по имени раньше места, где эмитятся их тела (круговая
		// зависимость с самими телами - см. package doc
		// nullableReflectBoxHelperProtos в reflect.go). После typedefs/
		// nullablesCode/nullablesAfterTypedefs выше - нужен уже видимый
		// C-тип самой обёртки "?Тип" как тип параметра.
		sb.WriteString(nullableReflectBoxHelperProtos(prog))
		// После protos/ctors - геттеры/сеттеры/инвокеры полей и методов
		// вызывают уже сгенерированные C-функции полей/методов/rech_new_<Тип>
		// (см. package doc reflect.go).
		reflectCode, err := reflectRuntimeCode(ctx, prog.ReflectTargets, prog.Generics, prog.InterfaceImplementors)
		if err != nil {
			return "", err
		}
		sb.WriteString(reflectCode)
		// Тела хелперов - ПОСЛЕ reflectRuntimeCode: тело для "?Задача"
		// ссылается на дескриптор сущности "Задача" (&rech_type_meta_zadacha),
		// чьё опережающее объявление пишет именно reflectRuntimeCode (см.
		// reflectGen.ensure) - см. package doc nullableReflectBoxHelpers.
		sb.WriteString(nullableReflectBoxHelpers(ctx, prog))
	}

	// threadPoolCode (threadpool.go) - ДО activeTasksCode/taskCode ниже:
	// rech_launch_... (concurrency.go, ensureTaskWrapper) зовёт
	// rech_pool_submit текстом, объявление должно предшествовать
	// использованию - тот же принцип, что и у activeTasksCode ниже.
	appendCodeIf(&sb, ctx.usesThreads, threadPoolCode(prog.PoolShrinkEnabled, prog.PoolIdleTimeoutMs))
	// activeTasksCode (active_tasks.go) - ДО taskCode ниже: точка входа
	// потока (concurrency.go, ensureTaskWrapper) зовёт rech_active_tasks_dec
	// текстом, объявление должно предшествовать использованию.
	appendCodeIf(&sb, ctx.usesThreads || ctx.usesActiveTaskWait, activeTasksCode)
	// taskCode - обёртки "запустить"/"дождаться" (internal/backend/cbackend/
	// concurrency.go) - ссылаются на &rech_<имя> обёрнутых функций (нужны
	// protos выше) и на rech_throw (нужен exceptionRuntimeCode выше), сама
	// заполняется как побочный эффект генерации body ниже, поэтому текст
	// уже готов к этому моменту, хотя размещается в файле раньше него.
	appendCodeIf(&sb, ctx.taskCode.Len() > 0, ctx.taskCode.String())

	// coroutineRuntimeCode (coroutine.go) - эмитится СИЛЬНО РАНЬШЕ этого
	// места (сразу после rech_panic, до socketHelpers - Шаг 132, см. её
	// собственный appendCodeIf выше по файлу) - rech_accept/rech_send_raw/
	// rech_socket_read_raw (plainSocketOpsCode) теперь тоже ссылаются на
	// rech_coro_current/rech_coro_wait_readable/writable текстом, а
	// socketHelpers эмитируется задолго до этой точки (protos выше).
	// coroTaskCode остаётся здесь - обёртки "дождаться <асинхронная
	// функция>(...)" (тот же принцип, что и taskCode выше: заполняется как
	// побочный эффект генерации тела ниже, текст уже готов к этому
	// моменту) ссылаются на &rech_<имя> обёрнутых функций программы (нужны
	// protos выше), не на socketHelpers - раньше им сюда не нужно.
	appendCodeIf(&sb, ctx.coroTaskCode.Len() > 0, ctx.coroTaskCode.String())

	// callableCode - обёртки "&имя"/тела лямбд (internal/backend/cbackend/
	// callable.go) - тот же принцип, что и taskCode выше: заполняется как
	// побочный эффект генерации body, размещается в файле раньше него
	// (ссылается на &rech_<имя> обёрнутых функций - нужны protos выше).
	appendCodeIf(&sb, ctx.callableCode.Len() > 0, ctx.callableCode.String())

	sb.WriteString(body.String())
	return sb.String(), nil
}

type ResolveFn = func(ctx *irGenCtx, prog *ir.Program, targets []ir.Type) (string, error)

func resolveRuntimeCode(
	sb *strings.Builder,
	ctx *irGenCtx,
	prog *ir.Program,
	irType []ir.Type,
	resolveFn ResolveFn,
) error {
	if len(irType) > 0 {
		runtimeCode, err := resolveFn(ctx, prog, irType)
		if err != nil {
			return err
		}
		sb.WriteString(runtimeCode)
	}

	return nil
}

func appendCodeIf(sb *strings.Builder, appendIf bool, code string) {
	if appendIf {
		sb.WriteString(code)
		sb.WriteString("\n")
	}
}

// generateIREntityStruct - C typedef struct для ent, как
// C typedef struct для ent, по уже собранным internal/ir.Field.
func generateIREntityStruct(ctx *irGenCtx, ent *ir.Entity) (string, error) {
	if ent.IsMutex {
		return mutexStructTypedef(), nil
	}
	if ent.IsSemaphore {
		return semaphoreStructTypedef(), nil
	}
	if ent.IsTransaction {
		return dbtxTypedefCode(), nil
	}
	s := codegen.Struct(entityCName(ctx, ent.Name))
	participates := ctx.dispatchParticipants[ent.Name]
	if participates {
		// "vt" - первым полем и всегда (не только у собственно
		// объявляющей сущности, но и у каждого её потомка) - общий
		// начальный кусок структуры делает приведение к vtableHeaderType
		// определённым для чтения (см. package doc vtable.go).
		s.Field("const "+vtableType+" *", "vt")
	}
	if ent.HasStateFlags {
		// "state_flags" - позиционно инвариантна той же логике, что и "vt"
		// выше: потомок сущности, объявляющей "состояние:", несёт это поле
		// даже без своего "состояние:" (см. package doc ir.Entity.
		// HasStateFlags) - указатель на потомка используется как указатель
		// на предка в рантайм-проверке (internal/backend/cbackend/
		// typestate.go, offsetof(<предок>, state_flags)).
		s.Field("uint64_t", "state_flags")
	}
	if len(ent.Fields) == 0 && !participates && !ent.HasStateFlags {
		s.Field("char", "_rech_empty")
	}
	for _, f := range ent.Fields {
		if ent.IsExternStruct && f.Type.Kind == ir.KindCallable {
			// "Вызываемый(...)" полем "внешняя структура" (WNDCLASS.
			// lpfnWndProc-стиль) - реальный C API читает/пишет ГОЛЫЙ
			// указатель на функцию в самой typedef-структуре, не {fn, env}
			// по значению (fromIRType(...).cType - callableType, см.
			// callable.go) - декларатор с именем ВНУТРИ скобок
			// (ffiFieldDeclarator, ffi_callback.go), FieldDeclarator (не
			// обычный Field - см. её package doc, codegen/struct.go).
			decl, err := ffiFieldDeclarator(ctx, f.Type, mangleField(f.Name))
			if err != nil {
				return "", err
			}
			s.FieldDeclarator(decl)
			continue
		}
		s.Field(fromIRType(f.Type).cType(ctx), mangleField(f.Name))
	}
	w := codegen.NewWriter()
	s.Write(w)
	return w.String() + "\n", nil
}

// generateIREntityCtor - конструктор ссылочной сущности ("сущность"/
// "класс", ctx.entityIsRef(ent.Name) == true): выделяет память под структуру
// через GC_MALLOC (Boehm GC - решение пользователя, шаг 12, "Указатели/
// ссылки... сущность по умолчанию") и возвращает указатель. GC_MALLOC (не
// GC_MALLOC_ATOMIC) - поля сущности сами могут быть указателями на другие
// GC-объекты (вложенная сущность), нужно сканирование; она же зануляет
// выделенную память (документированная гарантия Boehm GC, в отличие от
// обычного malloc) - опора для тела конструктора ниже, чьё тело не обязано
// присвоить каждое поле явно.
//
// ent.Ctor никогда не nil здесь (ctx.entityIsRef(ent.Name) == true) - у
// сущности без своего "конструктор(...)" в исходнике internal/semantic
// подставляет синтетический пустой (0 параметров, пустое тело, см. package
// doc analyzer.go) - "как в нормальных языках" неявный конструктор не
// зависит от полей вообще, а "новый Тип()" без аргументов - единственная
// форма, доступная без явного "конструктор(...)" (docs/SPEC.md, "Явные
// конструкторы"). Выделение памяти отделено от самого тела конструктора
// (generateIREntityCtorInit) - см. её package doc про то, зачем именно.
//
// В отличие от "структура" (generateIRNewExpr - compound literal прямо в
// месте вызова, структуры не участвуют в конструкторах вообще - см.
// ast.EntityDecl.Ctor), у ссылочной сущности ровно одна такая функция на
// тип, не код на каждый "новый" - тот же принцип, что и у show-функций.
func generateIREntityCtor(ctx *irGenCtx, ent *ir.Entity) (string, error) {
	if ent.IsMutex {
		return mutexRuntimeCode(), nil
	}
	if ent.IsSemaphore {
		return semaphoreRuntimeCode(), nil
	}
	if ent.IsTransaction {
		// "Транзакция" не имеет ни конструктора, ни обычных полей вообще
		// (см. internal/semantic/expr.go, analyzeNewExpr, IsTransaction -
		// "новый" запрещён) - её C-структура (dbtxTypedefCode,
		// generateIREntityStruct выше) устроена совсем иначе, чем обычная
		// сущность (нет "vt", свои поля mini/real_db/real_idx/resolved) -
		// обычный путь ниже (generateIREntityCtorInit и т. п.) писал бы код
		// для несуществующих полей. Сам рантайм (rech_db_run_transaction +
		// три метода) эмитируется отдельно, из GenerateFromIR, рядом с
		// dbRuntimeCode (там доступен prog.DBBackends, здесь - нет).
		return "", nil
	}
	if ent.Ctor == nil {
		// internal/semantic держит этот инвариант (каждая ссылочная
		// сущность получает Ctor - явный или синтетический пустой, см.
		// analyzeEntity) - запасной вариант здесь только на случай
		// системной сущности, заведённой напрямую литералом ir.Entity{...}
		// в обход analyzeEntity (как когда-то было с "Исключение").
		ent.Ctor = &ir.Method{}
	}
	initCode, err := generateIREntityCtorInit(ctx, ent)
	if err != nil {
		return "", err
	}
	return initCode + generateIREntityCtorNewWithInit(ctx, ent), nil
}

// gcAllocStmts - "Тип *p = GC_MALLOC(sizeof(Тип));" + (если ent участвует в
// диспетчеризации) "p->vt = &rech_VT_...;" - общее начало тела и у
// позиционного конструктора (generateIREntityCtor), и у обёртки над явным
// (generateIREntityCtorNewWithInit) - единственная разница между ними дальше
// (позиционное заполнение полей против вызова rech_init_...).
func gcAllocStmts(ctx *irGenCtx, ent *ir.Entity, cType string) []codegen.Stmt {
	stmts := []codegen.Stmt{
		codegen.Decl(cType+" *", "p", codegen.Call("GC_MALLOC", codegen.Sizeof(codegen.Raw(cType)))),
	}
	if ctx.dispatchParticipants[ent.Name] {
		stmts = append(stmts, codegen.Assign(codegen.PointerField(codegen.Var("p"), "vt"), codegen.Address(codegen.Raw(vtableInstanceName(ent.Name)))))
	}
	return stmts
}

// zeroFieldValueExpr - выражение для явного зануления поля БЕЗ "= выражение"
// (см. package doc generateIREntityCtorInit) - nil, если голых нулевых
// байт GC_MALLOC'а уже достаточно (Целое/Дробное/Действительное/
// Логическое/Символ/Байт - всё это просто числовой 0, а также ссылка на
// другую сущность/интерфейс, которой намеренно разрешено быть nil "ещё не
// присвоено" - тем же принципом, что и internal/interp/eval.go,
// zeroFieldValue). Строка - нулевые байты дают NULL char*, не то же самое,
// что "" (segfault при первом же обращении к полю - живая проверка нашла
// именно это); generic-коллекция (Список<T>/Стопка<T>/Очередь<T>/
// Словарь<К,В>, ctx.generics) - нулевые байты дают несогласованную C-
// структуру буфера/карты (например, len/cap ненулевые при указателе-NULL,
// в зависимости от представления) - первый же вызов метода тоже падает.
func zeroFieldValueExpr(ctx *irGenCtx, t ir.Type) ir.Expr {
	switch {
	case t.Kind == ir.KindString:
		return &ir.StringLiteral{Value: ""}
	case t.Kind == ir.KindEntity && ctx.generics[t.Entity]:
		return &ir.NewExpr{Entity: t.Entity}
	default:
		return nil
	}
}

// generateIREntityCtorInit - тело конструктора ent.Ctor (явного или
// синтетического пустого, см. package doc generateIREntityCtor) как
// отдельная C-функция, с тем же "Тип *this_" первым скрытым параметром, что
// и у
// метода (cMethodParamListIR/ctx.thisEntity - буквально тот же путь
// кодогенерации тела, что и generateIRMethod, включая "этот"). Отделена от
// выделения памяти (generateIREntityCtorNewWithInit) ровно для того, чтобы
// "родитель(...)" производного конструктора (ir.ParentCallStmt) мог вызвать
// её напрямую на уже существующем объекте, не создавая новый - единый
// объект строится последовательными вызовами тел конструкторов вверх по
// цепочке BaseName, а не вложенными "новый".
func generateIREntityCtorInit(ctx *irGenCtx, ent *ir.Entity) (string, error) {
	var sb strings.Builder
	fmt.Fprintf(&sb, "// %s: конструктор\n", ent.Name)
	fmt.Fprintf(&sb, "static void %s(%s) {\n", mangleCtorInit(ent.Name), cMethodParamListIR(ctx, ent.Name, ent.Ctor.Params))
	fmt.Fprintf(&sb, "    %s", tracePush(ent.Name+".конструктор"))
	ctx.thisEntity = ent.Name
	defer func() { ctx.thisEntity = "" }()
	// Поле с "имеет: имя: Тип = выражение" начинается со своего значения по
	// умолчанию ещё до тела явного конструктора (docs/SPEC.md) - GC_MALLOC
	// зануляет память, этого достаточно для полей без default ТОЛЬКО когда
	// нулевые байты - валидное нулевое значение типа (Целое/Дробное/
	// Действительное/Логическое/Символ/Байт, а также ссылка на другую
	// сущность/интерфейс - см. zeroFieldValueExpr) - Строка/generic-
	// коллекция нуждаются в явном занулении (zeroFieldValueExpr, nil для
	// всех остальных типов - для них ничего не генерируется, как и раньше).
	// Тело конструктора (ниже) может переопределить любое из полей через
	// "этот.поле = ...", включая эти явно занулённые.
	for _, f := range ent.Fields {
		value := f.Default
		if value == nil {
			value = zeroFieldValueExpr(ctx, f.Type)
		}
		if value == nil {
			continue
		}
		assign := &ir.FieldAssign{Receiver: &ir.ThisExpr{Entity: ent.Name}, Field: f.Name, Value: value}
		if err := generateIRStmt(&sb, ctx, assign, 1); err != nil {
			return "", err
		}
	}
	for _, s := range ent.Ctor.Body {
		if err := generateIRStmt(&sb, ctx, s, 1); err != nil {
			return "", err
		}
	}
	fmt.Fprintf(&sb, "    %s", tracePop)
	sb.WriteString("}\n\n")
	return sb.String(), nil
}

// generateIREntityCtorNewWithInit - "rech_new_X" когда есть явный
// конструктор: GC_MALLOC (зануляет память) + вызов тела конструктора
// (generateIREntityCtorInit) на свежевыделенном указателе + возврат его.
// Определена ПОСЛЕ generateIREntityCtorInit в общем тексте (см.
// generateIREntityCtor) - обращение к mangleCtorInit(ent.Name) здесь уже
// после её определения, C не нуждается в отдельном прототипе. Порядок
// сущностей в prog.Entities - порядок объявления в файле (buildIR сохраняет
// его как есть), а базовая сущность обязана быть объявлена раньше
// производной (semantic.applyInheritance) - поэтому и "родитель(...)"
// производного конструктора всегда ссылается на уже определённую
// rech_init_БАЗА к моменту, когда до неё доходит очередь в этом же цикле
// (GenerateFromIR), без отдельного прототипа и для этого случая тоже.
func generateIREntityCtorNewWithInit(ctx *irGenCtx, ent *ir.Entity) string {
	cType := entityCName(ctx, ent.Name)
	exported := ctx.exportedCtors[ent.Name]
	fn := codegen.Func(ctorCName(ctx, ent.Name)).ReturnType(exportAttr(ctx, exported) + cType + " *")
	if !exported {
		fn.Static()
	}
	params := make([]codegen.Param, len(ent.Ctor.Params))
	for i, p := range ent.Ctor.Params {
		params[i] = codegen.Parameter(fromIRType(p.Type).cType(ctx), mangleVar(p.Name))
	}
	fn.Params(params...)
	initArgs := make([]codegen.Expr, len(ent.Ctor.Params)+1)
	initArgs[0] = codegen.Var("p")
	for i, p := range ent.Ctor.Params {
		initArgs[i+1] = codegen.Var(mangleVar(p.Name))
	}
	stmts := gcAllocStmts(ctx, ent, cType)
	stmts = append(stmts,
		codegen.ExprStatement(codegen.Call(mangleCtorInit(ent.Name), initArgs...)),
		codegen.Return(codegen.Var("p")),
	)
	fn.Statements(stmts...)
	w := codegen.NewWriter()
	fn.Write(w)
	return w.String() + "\n"
}

// cReturnTypeFor - C-тип результата функции/метода: сама Type, если он
// объявлен (hasReturn), иначе "int" - технический 0 в конце (не значение
// "результата"), чтобы int-функция не осталась без return (для строгого
// C - undefined behavior), как и в AST-пути.
func cReturnTypeFor(ctx *irGenCtx, hasReturn bool, returnType ir.Type) string {
	if hasReturn {
		return fromIRType(returnType).cType(ctx)
	}
	return "int"
}

func cParamListIR(ctx *irGenCtx, params []ir.Param) string {
	if len(params) == 0 {
		return "void"
	}
	parts := make([]string, len(params))
	for i, p := range params {
		parts[i] = fmt.Sprintf("%s %s", fromIRType(p.Type).cType(ctx), mangleVar(p.Name))
	}
	return strings.Join(parts, ", ")
}

// externPrototypeOfIR - "extern <тип> <CName>(<типы>);" для внешней
// функции - без имён параметров (в отличие от cbackend.externPrototypeOf,
// C допускает анонимные параметры в прототипе, а сами имена
// параметров Rech после семантического анализа уже не нужны ни для чего,
// кроме сообщений об ошибках, которые уже показаны на этапе Analyze.
func externPrototypeOfIR(ctx *irGenCtx, ext *ir.ExternFuncDecl) (string, error) {
	if externPrototypeAlreadyProvided(ctx, ext.CName) {
		// Стандартный символ (strlen/malloc/sqrt/...), а заголовок,
		// объявляющий его, программа и так включает по другой причине -
		// см. package doc extern_headers.go - НЕ дублируем прототип,
		// полагаемся на уже видимый из заголовка.
		return "", nil
	}
	cReturnType := "void"
	if ext.HasReturn {
		// ext.ReturnType никогда KindCallable (semantic отклоняет - см.
		// collectExternFuncSigs) - ffiParamCType здесь ведёт себя как
		// обычный fromIRType(...).cType(ctx).
		cReturnType = fromIRType(ext.ReturnType).cType(ctx)
	}
	types := make([]string, len(ext.Params))
	for i, p := range ext.Params {
		t, err := ffiParamCType(ctx, p)
		if err != nil {
			return "", err
		}
		types[i] = t
	}
	return fmt.Sprintf("extern %s %s(%s);\n", cReturnType, ext.CName, strings.Join(types, ", ")), nil
}

// cMethodParamListIR - как cParamListIR, но с обязательным скрытым первым
// параметром "Тип *this_" (получатель по указателю, как и в AST-пути) -
// у метода он есть всегда, поэтому в отличие от cParamListIR/
// combineParamList не нужен отдельный случай "нет параметров -> void".
func cMethodParamListIR(ctx *irGenCtx, entityName string, params []ir.Param) string {
	parts := make([]string, len(params)+1)
	parts[0] = fmt.Sprintf("%s *this_", entityCName(ctx, entityName))
	for i, p := range params {
		parts[i+1] = fmt.Sprintf("%s %s", fromIRType(p.Type).cType(ctx), mangleVar(p.Name))
	}
	return strings.Join(parts, ", ")
}

func generateIRFunction(ctx *irGenCtx, fn *ir.Function) (string, error) {
	var sb strings.Builder
	fmt.Fprintf(&sb, "// %s\n", fn.Name)
	cName := funcCName(ctx, fn.Name)
	paramList := cParamListIR(ctx, fn.Params)
	if cName == "main" {
		// "главная" всегда получает настоящую C-сигнатуру main(int argc,
		// char **argv) - как и в AST-пути (generateCallable) - количество_
		// аргументов/аргумент читают их из глобальных rech_argc/rech_argv
		// (argsGlobals), вызываемых из любой функции программы, не только
		// из главная.
		paramList = "int argc, char **argv"
	}
	fmt.Fprintf(&sb, "%s%s %s(%s) {\n", exportAttr(ctx, fn.Exported), cReturnTypeFor(ctx, fn.HasReturn, fn.ReturnType), cName, paramList)
	fmt.Fprintf(&sb, "    %s", tracePush(fn.Name))
	if cName == "main" {
		if ctx.usesGC {
			sb.WriteString("    GC_INIT();\n")
		}
		sb.WriteString("    rech_init_console();\n")
		sb.WriteString("    rech_init_args(argc, argv);\n")
		if ctx.usesStaticInit {
			sb.WriteString("    rech_init_statics();\n")
		}
	}
	for _, s := range fn.Body {
		if err := generateIRStmt(&sb, ctx, s, 1); err != nil {
			return "", err
		}
	}
	if !fn.HasReturn {
		fmt.Fprintf(&sb, "    %s", tracePop)
		sb.WriteString("    return 0;\n")
	}
	sb.WriteString("}\n\n")
	return sb.String(), nil
}

// generateIRMethod - как generateIRFunction, но со скрытым параметром
// "Тип *this_" и ctx.thisEntity, включённым на время генерации тела - то,
// что делает ThisExpr типизируемым (generateIRExpr, case *ir.ThisExpr),
// как и ctx.thisEntity в AST-пути (generateMethod).
func generateIRMethod(ctx *irGenCtx, entityName string, m *ir.Method) (string, error) {
	var sb strings.Builder
	fmt.Fprintf(&sb, "// %s.%s\n", entityName, m.Name)
	paramList := cMethodParamListIR(ctx, entityName, m.Params)
	if m.IsStatic {
		// Статический метод - обычная C-функция, без скрытого "this_" (см.
		// package doc ir.Method.IsStatic) - "этот" внутри тела грамматически
		// невозможен (парсер), поэтому ctx.thisEntity не выставляется.
		paramList = cParamListIR(ctx, m.Params)
	}
	fmt.Fprintf(&sb, "%s%s %s(%s) {\n", exportAttr(ctx, m.Exported), cReturnTypeFor(ctx, m.HasReturn, m.ReturnType), methodCName(ctx, entityName, m.Name), paramList)
	fmt.Fprintf(&sb, "    %s", tracePush(entityName+"."+m.Name))
	if !m.IsStatic {
		ctx.thisEntity = entityName
		defer func() { ctx.thisEntity = "" }()
	}
	for _, s := range m.Body {
		if err := generateIRStmt(&sb, ctx, s, 1); err != nil {
			return "", err
		}
	}
	if !m.HasReturn {
		fmt.Fprintf(&sb, "    %s", tracePop)
		sb.WriteString("    return 0;\n")
	}
	sb.WriteString("}\n\n")
	return sb.String(), nil
}

// generateIRStmt - тонкая обёртка над generateIRStmtBody: заводит для
// ЭТОЙ инструкции свежий ctx.chainTmpPreamble (см. package doc
// materializeIfNeeded) и локальный буфер, генерирует тело инструкции туда
// (сама инструкция, а если она составная - IfStmt/WhileStmt/... - и всё
// вложенное тело рекурсивными вызовами ЭТОЙ ЖЕ обёртки, каждый со своим
// собственным chainTmpPreamble), затем сбрасывает накопленные объявления
// временных переменных В sb ПЕРЕД строкой самой инструкции - порядок
// важен (переменная должна быть объявлена раньше своего использования).
// Вложенные инструкции (тела if/while и т. п.) пишут не в исходный sb
// напрямую, а в тот же локальный буфер этой функции (см. вызовы
// generateIRStmt(sb, ...) внутри switch generateIRStmtBody - там sb уже
// это локальный буфер, а не аргумент верхнего вызова) - поэтому временная
// переменная вложенной инструкции окажется на своём месте, прямо перед
// НЕЙ, а не выше всего блока.
func generateIRStmt(sb *strings.Builder, ctx *irGenCtx, s ir.Stmt, depth int) error {
	ctx.curIndent = indent(depth)
	ctx.chainTmpPreamble = nil
	var body strings.Builder
	err := generateIRStmtBody(&body, ctx, s, depth)
	for _, p := range ctx.chainTmpPreamble {
		sb.WriteString(p)
	}
	ctx.chainTmpPreamble = nil
	if err != nil {
		return err
	}
	sb.WriteString(body.String())
	return nil
}

func generateIRStmtBody(sb *strings.Builder, ctx *irGenCtx, s ir.Stmt, depth int) error {
	ind := indent(depth)
	// Строка ТЕКУЩЕЙ инструкции обновляет верхний кадр стека вызовов
	// (rech_trace_line) ДО генерации собственно инструкции - если внутри
	// неё (вложенным вызовом или напрямую) случится паника, кадр уже
	// покажет правильную строку. См. stmtLine/trace.go.
	if line := ir.LineOf(s); line > 0 {
		fmt.Fprintf(sb, "%srech_trace_line(%d);\n", ind, line)
	}
	switch s := s.(type) {
	case *ir.PrintStmt:
		code, typ, err := generateIRExpr(ctx, s.Value)
		if err != nil {
			return err
		}
		fmt.Fprintf(sb, "%s%s\n", ind, printCall(ctx, typ, code))
		return nil

	case *ir.ShowStmt:
		ctx.usesShow = true
		code, _, err := generateIRExpr(ctx, s.Value)
		if err != nil {
			return err
		}
		fmt.Fprintf(sb, "%s%s;\n", ind, generateIRShowExpr(ctx, ir.TypeOf(s.Value), code))
		fmt.Fprintf(sb, "%sprintf(\"\\n\");\n", ind)
		return nil

	case *ir.VarDecl:
		if s.IsStatic {
			return generateIRStaticVarDecl(sb, ctx, s, ind)
		}
		code, typ, err := generateIRExpr(ctx, s.Value)
		if err != nil {
			return err
		}
		// Через временную переменную, не напрямую "Тип v_x = <code>;" - в C
		// имя объявляемой переменной входит в область видимости уже в точке
		// её собственного объявления, ДО инициализатора справа от "=" -
		// самоссылающийся инициализатор при затенении внешней переменной с
		// тем же именем ("если истина { переменная х = х + 1 }", где
		// снаружи уже есть своя "х") иначе читал бы ещё не
		// проинициализированную (мусорную) новую "х", а не старую. "_INIT" -
		// заглавными буквами специально: mangleVar/transliterate всегда
		// приводят весь мангл-идентификатор к нижнему регистру, поэтому
		// такое имя не может совпасть ни с одной настоящей переменной Rech.
		cType := typ.cType(ctx)
		mangled := mangleVar(s.Name)
		tmp := mangled + "_INIT"
		if bridged, ok := bridgeExternStructCallCode(ctx, s.Value, typ.entity, cType, code); ok {
			fmt.Fprintf(sb, "%s%s %s = %s;\n", ind, cType, tmp, bridged)
		} else {
			fmt.Fprintf(sb, "%s%s %s = %s;\n", ind, cType, tmp, code)
		}
		fmt.Fprintf(sb, "%s%s %s = %s;\n", ind, cType, mangled, tmp)
		return nil

	case *ir.Assign:
		code, _, err := generateIRExpr(ctx, s.Value)
		if err != nil {
			return err
		}
		fmt.Fprintf(sb, "%s%s = %s;\n", ind, varRef(ctx, s.Name), code)
		return nil

	case *ir.NullCoalesceAssignStmt:
		code, _, err := generateIRExpr(ctx, s.Value)
		if err != nil {
			return err
		}
		name := varRef(ctx, s.Name)
		fmt.Fprintf(sb, "%sif (!%s.has) { %s = %s; }\n", ind, name, name, code)
		return nil

	case *ir.IfStmt:
		cond, _, err := generateIRExpr(ctx, s.Cond)
		if err != nil {
			return err
		}
		fmt.Fprintf(sb, "%sif (%s) {\n", ind, cond)
		for _, st := range s.Then {
			if err := generateIRStmt(sb, ctx, st, depth+1); err != nil {
				return err
			}
		}
		if s.Else != nil {
			fmt.Fprintf(sb, "%s} else {\n", ind)
			for _, st := range s.Else {
				if err := generateIRStmt(sb, ctx, st, depth+1); err != nil {
					return err
				}
			}
		}
		fmt.Fprintf(sb, "%s}\n", ind)
		return nil

	case *ir.GerundStmt:
		// "{"/"}" без условия - только чтобы у "результат" (первый элемент
		// Body, см. package doc ir.GerundStmt) была собственная область
		// видимости в C, не любая специальная кодогенерация.
		fmt.Fprintf(sb, "%s{\n", ind)
		for _, st := range s.Body {
			if err := generateIRStmt(sb, ctx, st, depth+1); err != nil {
				return err
			}
		}
		fmt.Fprintf(sb, "%s}\n", ind)
		return nil

	case *ir.BlockStmt:
		// Тот же приём, что и у GerundStmt выше - "{"/"}" только ради
		// собственной C-области видимости.
		fmt.Fprintf(sb, "%s{\n", ind)
		for _, st := range s.Body {
			if err := generateIRStmt(sb, ctx, st, depth+1); err != nil {
				return err
			}
		}
		fmt.Fprintf(sb, "%s}\n", ind)
		return nil

	case *ir.WhileStmt:
		cond, _, err := generateIRExpr(ctx, s.Cond)
		if err != nil {
			return err
		}
		fmt.Fprintf(sb, "%swhile (%s) {\n", ind, cond)
		lf := ctx.pushLoop()
		for _, st := range s.Body {
			if err := generateIRStmt(sb, ctx, st, depth+1); err != nil {
				ctx.popLoop()
				return err
			}
		}
		ctx.popLoop()
		fmt.Fprintf(sb, "%s%s: ;\n", indent(depth+1), lf.continueTarget)
		fmt.Fprintf(sb, "%s}\n", ind)
		fmt.Fprintf(sb, "%s%s: ;\n", ind, lf.breakTarget)
		return nil

	case *ir.BreakStmt:
		return generateIRBreakStmt(sb, ctx, depth, s.Depth)

	case *ir.ContinueStmt:
		return generateIRContinueStmt(sb, ctx, depth, s.Depth)

	case *ir.CStyleForStmt:
		return generateIRCStyleFor(sb, ctx, s, depth)

	case *ir.ReturnStmt:
		if s.Value == nil {
			if len(ctx.cleanupStack) > 0 {
				fmt.Fprintf(sb, "%s{\n", ind)
				if err := emitCleanupUnwind(sb, ctx, depth+1); err != nil {
					return err
				}
				fmt.Fprintf(sb, "%s    %sreturn 0;\n", ind, tracePop)
				fmt.Fprintf(sb, "%s}\n", ind)
				return nil
			}
			fmt.Fprintf(sb, "%s%s%sreturn 0;\n", ind, tracePop, ind)
			return nil
		}
		code, typ, err := generateIRExpr(ctx, s.Value)
		if err != nil {
			return err
		}
		// Значение вычисляется во временную переменную ДО rech_trace_pop() -
		// если сам code (вложенный вызов) паникует, кадр этой функции ещё
		// обязан быть на стеке (см. package doc trace.go). "return code;"
		// сразу после pop() вычислил бы code уже без него. Обёрнуто в
		// собственный "{ }" - имя переменной одно и то же на каждом "вернуть"
		// функции, отдельная область видимости на каждый исключает
		// повторное объявление, даже если "вернуть" встречается более
		// одного раза (разные ветки "если").
		fmt.Fprintf(sb, "%s{\n", ind)
		fmt.Fprintf(sb, "%s    %s %s = %s;\n", ind, typ.cType(ctx), returnTmpVar, code)
		if err := emitCleanupUnwind(sb, ctx, depth+1); err != nil {
			return err
		}
		fmt.Fprintf(sb, "%s    %s%s    return %s;\n", ind, tracePop, ind, returnTmpVar)
		fmt.Fprintf(sb, "%s}\n", ind)
		return nil

	case *ir.FieldAssign:
		receiverCode, _, err := generateIRExpr(ctx, s.Receiver)
		if err != nil {
			return err
		}
		code, _, err := generateIRExpr(ctx, s.Value)
		if err != nil {
			return err
		}
		if valType := ir.TypeOf(s.Value); valType.Kind == ir.KindCallable {
			// "объект.поле = замыкание" - то же самое, что и поле у "новый
			// Тип(...)" (generateIRNewExpr) - если поле принадлежит
			// "внешняя структура", {fn, env} по значению не то, что
			// ожидает C (голый указатель на функцию).
			if recvEnt, ok := ctx.entities[ir.TypeOf(s.Receiver).Entity]; ok && recvEnt.IsExternStruct {
				marshaled, err := ctx.marshalCallbackArg(valType, code)
				if err != nil {
					return err
				}
				code = marshaled
			}
		}
		op := "."
		if ctx.receiverIsPointer(s.Receiver) {
			op = "->"
		} else {
			// Получатель-"структура" не адресуем напрямую (результат вызова,
			// не переменная/поле/"этот") - "(rvalue).поле = ..." невалиден в
			// C так же, как и "&(rvalue)" у вызова метода. materializeIfNeeded
			// заводит временную переменную (уже lvalue, "." работает).
			receiverCode = ctx.materializeIfNeeded(s.Receiver, receiverCode)
		}
		fmt.Fprintf(sb, "%s(%s)%s%s = %s;\n", ind, receiverCode, op, mangleField(s.Field), code)
		return nil

	case *ir.StaticFieldAssign:
		code, _, err := generateIRExpr(ctx, s.Value)
		if err != nil {
			return err
		}
		fmt.Fprintf(sb, "%s%s = %s;\n", ind, mangleStaticField(s.Entity, s.Field), code)
		return nil

	case *ir.DerefAssignStmt:
		ptrCode, _, err := generateIRExpr(ctx, s.Pointer)
		if err != nil {
			return err
		}
		valCode, _, err := generateIRExpr(ctx, s.Value)
		if err != nil {
			return err
		}
		fmt.Fprintf(sb, "%s*%s = %s;\n", ind, ptrCode, valCode)
		return nil

	case *ir.BecomeStmt:
		// Состояние уже проверено семантическим анализом как объявленное у
		// сущности получателя - здесь только устанавливает бит в
		// рантайм-хранилище ("state_flags", см. package doc
		// ir.Entity.HasStateFlags) - читает его StateCheckExpr там, где
		// established не смог доказать переход на этапе компиляции.
		recvCode, _, err := generateIRExpr(ctx, s.Receiver)
		if err != nil {
			return err
		}
		entName := ir.TypeOf(s.Receiver).Entity
		ent, ok := ctx.entities[entName]
		if !ok {
			return fmt.Errorf("генерация C из IR: сущность %q (\"становится\") не найдена", entName)
		}
		bit, ok := ent.StateBit(s.State)
		if !ok {
			return fmt.Errorf("генерация C из IR: у сущности %q нет состояния %q", entName, s.State)
		}
		fmt.Fprintf(sb, "%s%s->state_flags |= 1ULL << %d;\n", ind, recvCode, bit)
		return nil

	case *ir.ParentCallStmt:
		argCodes := make([]string, len(s.Args)+1)
		argCodes[0] = "this_"
		for i, a := range s.Args {
			code, _, err := generateIRExpr(ctx, a)
			if err != nil {
				return err
			}
			argCodes[i+1] = code
		}
		fmt.Fprintf(sb, "%s%s(%s);\n", ind, mangleCtorInit(s.BaseEntity), strings.Join(argCodes, ", "))
		return nil

	case *ir.AssertStmt:
		cond, _, err := generateIRExpr(ctx, s.Cond)
		if err != nil {
			return err
		}
		ctx.usesPanic = true
		if s.Message == nil {
			fmt.Fprintf(sb, "%sif (!(%s)) { rech_panic(\"%s\"); }\n", ind, cond, escapeCString(rtmsg.AssertFailed(s.Line)))
			return nil
		}
		// "утверждать <условие>, <сообщение>" (внятные
		// сообщения о несовпадении) - сообщение конкатенируется с обычным
		// текстом ("утверждение не выполнено (строка N): <сообщение>"),
		// вычисляется в тексте самого "if" (C сам не вычисляет его, если
		// условие истинно - обычное короткое замыкание условного блока, тот
		// же принцип, что и у ветки тернарного оператора).
		msgCode, _, err := generateIRExpr(ctx, s.Message)
		if err != nil {
			return err
		}
		ctx.usesConcat = true
		prefix := escapeCString(rtmsg.AssertFailed(s.Line) + ": ")
		fmt.Fprintf(sb, "%sif (!(%s)) { rech_panic(rech_str_concat(\"%s\", %s)); }\n", ind, cond, prefix, msgCode)
		return nil

	case *ir.ExprStmt:
		var call ir.Expr = s.Call
		switch {
		case s.MethodCall != nil:
			call = s.MethodCall
		case s.StaticMethodCall != nil:
			call = s.StaticMethodCall
		case s.IncDec != nil:
			call = s.IncDec
		case s.Await != nil:
			call = s.Await
		case s.CallValue != nil:
			call = s.CallValue
		}
		code, _, err := generateIRExpr(ctx, call)
		if err != nil {
			return err
		}
		fmt.Fprintf(sb, "%s%s;\n", ind, code)
		return nil

	case *ir.ForStmt:
		fromCode, _, err := generateIRExpr(ctx, s.From)
		if err != nil {
			return err
		}
		toCode, _, err := generateIRExpr(ctx, s.To)
		if err != nil {
			return err
		}
		varC := mangleVar(s.Var)
		fmt.Fprintf(sb, "%sfor (long long %s = %s; %s <= %s; %s++) {\n", ind, varC, fromCode, varC, toCode, varC)
		lf := ctx.pushLoop()
		for _, st := range s.Body {
			if err := generateIRStmt(sb, ctx, st, depth+1); err != nil {
				ctx.popLoop()
				return err
			}
		}
		ctx.popLoop()
		fmt.Fprintf(sb, "%s%s: ;\n", indent(depth+1), lf.continueTarget)
		fmt.Fprintf(sb, "%s}\n", ind)
		fmt.Fprintf(sb, "%s%s: ;\n", ind, lf.breakTarget)
		return nil

	case *ir.ThrowStmt:
		return generateIRThrowStmt(sb, ctx, s, depth)

	case *ir.TryStmt:
		return generateIRTryStmt(sb, ctx, s, depth)

	case *ir.ForEachStmt:
		collCode, collType, err := generateIRExpr(ctx, s.Collection)
		if err != nil {
			return err
		}
		// "для <строка> в <Файл>" - потоковое построчное чтение, отдельная
		// форма от индексированного Список<T> ниже - файл не известной
		// заранее длины (тот же принцип, что и в cbackend.generateStmt,
		// AST-путь).
		if collType == typeFile {
			ctx.usesFileIO = true
			ctx.usesPanic = true
			varC := mangleVar(s.Var)
			okC := "rech_fe_ok_" + varC
			fmt.Fprintf(sb, "%sfor (;;) {\n", ind)
			fmt.Fprintf(sb, "%sbool %s = false;\n", indent(depth+1), okC)
			fmt.Fprintf(sb, "%sconst char *%s = rech_read_line_from_file(%s, &%s);\n", indent(depth+1), varC, collCode, okC)
			fmt.Fprintf(sb, "%sif (!%s) { break; }\n", indent(depth+1), okC)
			lf := ctx.pushLoop()
			for _, st := range s.Body {
				if err := generateIRStmt(sb, ctx, st, depth+1); err != nil {
					ctx.popLoop()
					return err
				}
			}
			ctx.popLoop()
			fmt.Fprintf(sb, "%s%s: ;\n", indent(depth+1), lf.continueTarget)
			fmt.Fprintf(sb, "%s}\n", ind)
			fmt.Fprintf(sb, "%s%s: ;\n", ind, lf.breakTarget)
			return nil
		}
		varC := mangleVar(s.Var)
		idxC := "rech_fe_i_" + varC
		fmt.Fprintf(sb, "%sfor (long long %s = 0; %s < %s->len; %s++) {\n", ind, idxC, idxC, collCode, idxC)
		fmt.Fprintf(sb, "%s%s %s = %s->data[%s];\n", indent(depth+1), fromIRType(s.ElemType).cType(ctx), varC, collCode, idxC)
		lf := ctx.pushLoop()
		for _, st := range s.Body {
			if err := generateIRStmt(sb, ctx, st, depth+1); err != nil {
				ctx.popLoop()
				return err
			}
		}
		ctx.popLoop()
		fmt.Fprintf(sb, "%s%s: ;\n", indent(depth+1), lf.continueTarget)
		fmt.Fprintf(sb, "%s}\n", ind)
		fmt.Fprintf(sb, "%s%s: ;\n", ind, lf.breakTarget)
		return nil

	default:
		return fmt.Errorf("генерация C из IR: инструкция %T пока не поддерживается", s)
	}
}

func generateIRExpr(ctx *irGenCtx, e ir.Expr) (string, valueType, error) {
	switch e := e.(type) {
	case *ir.StringLiteral:
		return fmt.Sprintf(`"%s"`, escapeCString(e.Value)), typeString, nil

	case *ir.IntLiteral:
		return fmt.Sprintf("%dLL", e.Value), typeInt, nil

	case *ir.FloatLiteral:
		return formatFloatLiteral(e.Value), typeFloat, nil

	case *ir.BoolLiteral:
		if e.Value {
			return "true", typeBool, nil
		}
		return "false", typeBool, nil

	case *ir.Identifier:
		return varRef(ctx, e.Name), fromIRType(e.Type), nil

	case *ir.UnaryExpr:
		return generateIRUnaryExpr(ctx, e)

	case *ir.IncDecExpr:
		name := varRef(ctx, e.Name)
		if e.Prefix {
			return fmt.Sprintf("(%s%s)", e.Op, name), fromIRType(e.Type), nil
		}
		return fmt.Sprintf("(%s%s)", name, e.Op), fromIRType(e.Type), nil

	case *ir.BinaryExpr:
		return generateIRBinaryExpr(ctx, e)

	case *ir.TernaryExpr:
		// Родной тернарный оператор C - уже короткое замыкание (ровно одна
		// из веток вычисляется), Then/Else семантически проверены
		// одинакового типа (analyzeTernaryExpr) - никакого хелпера не
		// нужно, в отличие от "??" ниже (там Left - nullable-обёртка,
		// нужно её развернуть).
		condCode, _, err := generateIRExpr(ctx, e.Cond)
		if err != nil {
			return "", typeUnknown, err
		}
		thenCode, _, err := generateIRExpr(ctx, e.Then)
		if err != nil {
			return "", typeUnknown, err
		}
		elseCode, _, err := generateIRExpr(ctx, e.Else)
		if err != nil {
			return "", typeUnknown, err
		}
		return fmt.Sprintf("((%s) ? (%s) : (%s))", condCode, thenCode, elseCode), fromIRType(e.Type), nil

	case *ir.CallExpr:
		cName, isExtern := ctx.externCNames[e.Callee]
		argCodes := make([]string, len(e.Args))
		for i, a := range e.Args {
			code, _, err := generateIRExpr(ctx, a)
			if err != nil {
				return "", typeUnknown, err
			}
			// isExtern && KindCallable - "Вызываемый(...)" аргумент внешней
			// функции ("FFI: маршалинг колбэков") - {fn, env}
			// по значению не то, что ожидает C API (голый указатель на
			// функцию, без слота под env) - marshalCallbackArg подменяет code
			// на C-имя сгенерированного trampoline (см. ffi_callback.go).
			if isExtern && ir.TypeOf(a).Kind == ir.KindCallable {
				marshaled, err := ctx.marshalCallbackArg(ir.TypeOf(a), code)
				if err != nil {
					return "", typeUnknown, err
				}
				code = marshaled
			}
			argCodes[i] = code
		}
		// Порядок проверки: builtin (inline C-выражение) -> внешняя функция
		// (настоящий C-символ, не mangleName) -> обычная функция программы.
		if code, ok := builtinCallCode(&ctx.builtinFlags, e.Callee, argCodes); ok {
			return code, fromIRType(e.Type), nil
		}
		callee := funcCName(ctx, e.Callee)
		if isExtern {
			callee = cName
		}
		if e.Callee == entryPointName {
			// "главная" манглится в настоящий C "main" (mangleName) - у НЕГО,
			// в отличие от любой другой функции программы, сигнатура ВСЕГДА
			// "(int argc, char **argv)" (см. package doc про paramList "int
			// argc, char **argv" в generateIRFunction), а не та, что вывелась
			// бы из ir.Function.Params самой "главная" (у неё их нет вовсе,
			// docs/SPEC.md - "главная" не принимает аргументы командной
			// строки, они читаются отдельными builtin'ами). Аргументы самого
			// Rech-вызова ("главная()") поэтому всегда пусты - C-вызов обязан
			// передать (rech_argc, rech_argv), не "()" - иначе "main()" не
			// совпадает с собственным прототипом "main(int, char**)" и cc
			// отказывается собирать (найдено живьём при проверке, можно ли
			// вообще вызвать "главная" из самого же Rech-кода).
			return fmt.Sprintf("%s(rech_argc, rech_argv)", callee), fromIRType(e.Type), nil
		}
		return fmt.Sprintf("%s(%s)", callee, strings.Join(argCodes, ", ")), fromIRType(e.Type), nil

	case *ir.NewExpr:
		return generateIRNewExpr(ctx, e)

	case *ir.FieldAccess:
		code, _, err := generateIRExpr(ctx, e.Receiver)
		if err != nil {
			return "", typeUnknown, err
		}
		op := "."
		if ctx.receiverIsPointer(e.Receiver) {
			op = "->"
		}
		return fmt.Sprintf("(%s)%s%s", code, op, mangleField(e.Field)), fromIRType(e.Type), nil

	case *ir.ThisExpr:
		// this_ - параметр метода, указатель всегда, вне зависимости от
		// того, "сущность" её тип или "структура" (получатель метода -
		// общее исключение из копирования, шаг 12). Но
		// "видимая" форма ЗНАЧЕНИЯ этого типа - то, что ожидают FieldAccess/
		// присваивание/возврат/передача параметром - должна совпадать с
		// формой, в которой этот тип представлен everywhere: указатель для
		// ссылочной сущности ("this_" как есть - тот же указатель, что и
		// любая другая переменная этого типа), разыменованное значение для
		// структуры ("(*this_)" - this_ здесь просто временный адрес для
		// мутации через метод, не "истинное" представление значения).
		if ctx.entityIsRef(e.Entity) {
			return "this_", entityType(e.Entity), nil
		}
		return "(*this_)", entityType(e.Entity), nil

	case *ir.MethodCallExpr:
		return generateIRMethodCallExpr(ctx, e)

	case *ir.StaticFieldExpr:
		return mangleStaticField(e.Entity, e.Field), fromIRType(e.Type), nil

	case *ir.StaticMethodCallExpr:
		return generateIRStaticMethodCallExpr(ctx, e)

	case *ir.CharLiteral:
		return fmt.Sprintf("%d", e.Value), typeChar, nil

	case *ir.CastExpr:
		return generateIRCastExpr(ctx, e)

	case *ir.ParseExpr:
		return generateIRParseExpr(ctx, e)

	case *ir.SerializeExpr:
		return generateIRSerializeExpr(ctx, e)

	case *ir.IsExpr:
		return generateIRIsExpr(ctx, e)

	case *ir.StateCheckExpr:
		return generateIRStateCheckExpr(ctx, e)

	case *ir.HasStateExpr:
		return generateIRHasStateExpr(ctx, e)

	case *ir.LaunchExpr:
		return generateIRLaunchExpr(ctx, e)

	case *ir.AwaitExpr:
		return generateIRAwaitExpr(ctx, e)

	case *ir.FuncRefExpr:
		return generateIRFuncRefExpr(ctx, e)

	case *ir.LambdaExpr:
		return generateIRLambdaExpr(ctx, e)

	case *ir.CallValueExpr:
		return generateIRCallValueExpr(ctx, e)

	case *ir.TypeOfExpr:
		return fmt.Sprintf("(&%s)", reflectMetaVarFor(e.Type)), typeTypeInfo, nil

	case *ir.AsExpr:
		return generateIRAsExpr(ctx, e)

	case *ir.DBRowsAsExpr:
		return generateIRDBRowsAsExpr(ctx, e)

	case *ir.NullLiteral:
		t := fromIRType(e.Type)
		return fmt.Sprintf("%s_empty()", mangleType(t.entity)), t, nil

	case *ir.NullWrapExpr:
		code, _, err := generateIRExpr(ctx, e.Value)
		if err != nil {
			return "", typeUnknown, err
		}
		t := fromIRType(e.Type)
		return fmt.Sprintf("%s_wrap(%s)", mangleType(t.entity), code), t, nil

	case *ir.NullUnwrapExpr:
		code, _, err := generateIRExpr(ctx, e.Value)
		if err != nil {
			return "", typeUnknown, err
		}
		src := fromIRType(ir.TypeOf(e.Value))
		return fmt.Sprintf("%s_unwrap(%s)", mangleType(src.entity), code), fromIRType(e.Type), nil

	case *ir.NullCheckExpr:
		code, _, err := generateIRExpr(ctx, e.Value)
		if err != nil {
			return "", typeUnknown, err
		}
		if e.IsNot {
			return fmt.Sprintf("((%s).has)", code), typeBool, nil
		}
		return fmt.Sprintf("(!(%s).has)", code), typeBool, nil

	case *ir.ListIndexExpr:
		_, recvPtr, err := generateIRAddressableExpr(ctx, e.Receiver)
		if err != nil {
			return "", typeUnknown, err
		}
		idxCode, _, err := generateIRExpr(ctx, e.Index)
		if err != nil {
			return "", typeUnknown, err
		}
		fn := mangleType(e.CollectionName) + "_get_safe"
		return fmt.Sprintf("%s(%s, %s)", fn, recvPtr, idxCode), fromIRType(e.Type), nil

	case *ir.DictIndexExpr:
		_, recvPtr, err := generateIRAddressableExpr(ctx, e.Receiver)
		if err != nil {
			return "", typeUnknown, err
		}
		keyCode, _, err := generateIRExpr(ctx, e.Key)
		if err != nil {
			return "", typeUnknown, err
		}
		fn := mangleType(e.CollectionName) + "_get_safe"
		return fmt.Sprintf("%s(%s, %s)", fn, recvPtr, keyCode), fromIRType(e.Type), nil

	case *ir.AddressOfExpr:
		switch recv := e.Receiver.(type) {
		case *ir.Identifier:
			return fmt.Sprintf("(&%s)", varRef(ctx, recv.Name)), fromIRType(e.Type), nil
		case *ir.FieldAccess:
			code, _, err := generateIRExpr(ctx, recv)
			if err != nil {
				return "", typeUnknown, err
			}
			return fmt.Sprintf("(&%s)", code), fromIRType(e.Type), nil
		case *ir.ListElementRef:
			_, listPtr, err := generateIRAddressableExpr(ctx, recv.Receiver)
			if err != nil {
				return "", typeUnknown, err
			}
			idxCode, _, err := generateIRExpr(ctx, recv.Index)
			if err != nil {
				return "", typeUnknown, err
			}
			fn := mangleType(recv.CollectionName) + "_addr"
			return fmt.Sprintf("%s(%s, %s)", fn, listPtr, idxCode), fromIRType(e.Type), nil
		default:
			return "", typeUnknown, fmt.Errorf("генерация C из IR: неподдержанный получатель \"&\" %T", e.Receiver)
		}

	case *ir.DerefExpr:
		code, _, err := generateIRExpr(ctx, e.Value)
		if err != nil {
			return "", typeUnknown, err
		}
		return fmt.Sprintf("(*%s)", code), fromIRType(e.Type), nil

	case *ir.ListLiteralExpr:
		fn := mangleType(e.CollectionName) + "_from_literal"
		if len(e.Elems) == 0 {
			return fmt.Sprintf("%s(0, NULL)", fn), fromIRType(e.Type), nil
		}
		elemC := fromIRType(e.ElemType).cType(ctx)
		codes := make([]string, len(e.Elems))
		for i, el := range e.Elems {
			code, _, err := generateIRExpr(ctx, el)
			if err != nil {
				return "", typeUnknown, err
			}
			codes[i] = code
		}
		arr := fmt.Sprintf("(%s[]){%s}", elemC, strings.Join(codes, ", "))
		return fmt.Sprintf("%s(%dLL, %s)", fn, len(e.Elems), arr), fromIRType(e.Type), nil

	case *ir.NullCoalesceExpr:
		leftCode, _, err := generateIRExpr(ctx, e.Left)
		if err != nil {
			return "", typeUnknown, err
		}
		rightCode, _, err := generateIRExpr(ctx, e.Right)
		if err != nil {
			return "", typeUnknown, err
		}
		src := fromIRType(ir.TypeOf(e.Left))
		structName := mangleType(src.entity)
		result := fromIRType(e.Type)
		if result.kind == kindNullable {
			return fmt.Sprintf("%s_coalesce_n(%s, %s)", structName, leftCode, rightCode), result, nil
		}
		return fmt.Sprintf("%s_coalesce(%s, %s)", structName, leftCode, rightCode), result, nil

	default:
		return "", typeUnknown, fmt.Errorf("генерация C из IR: выражение %T пока не поддерживается", e)
	}
}

func generateIRUnaryExpr(ctx *irGenCtx, e *ir.UnaryExpr) (string, valueType, error) {
	code, _, err := generateIRExpr(ctx, e.Operand)
	if err != nil {
		return "", typeUnknown, err
	}
	typ := fromIRType(e.Type)
	switch e.Op {
	case "-":
		// Целое8/16/32/64, Натуральное/8/16/32/64 - явный каст обратно
		// к своему C-типу: обычные арифметические преобразования C иначе
		// оставили бы результат в более широком типе (int), не усекая/
		// оборачивая его на переполнении - тот же принцип, что и в
		// generateIRArithmetic.
		if _, _, ok := e.Type.Kind.IntWidthSigned(); ok && e.Type.Kind != ir.KindInt {
			return fmt.Sprintf("((%s)(-%s))", typ.cType(ctx), code), typ, nil
		}
		return fmt.Sprintf("(-%s)", code), typ, nil
	case "не":
		return fmt.Sprintf("(!%s)", code), typeBool, nil
	case "~":
		// Целое8/16/32/64, Натуральное/8/16/32/64 и Байт - C продвигает их
		// до int для "~" (обычные арифметические преобразования) - явный
		// каст обратно к своему C-типу, тот же приём, что и у унарного "-"
		// выше.
		if _, _, ok := e.Type.Kind.IntWidthSigned(); ok && e.Type.Kind != ir.KindInt {
			return fmt.Sprintf("((%s)(~%s))", typ.cType(ctx), code), typ, nil
		}
		if e.Type == ir.TypeByte {
			return fmt.Sprintf("((unsigned char)(~%s))", code), typeByte, nil
		}
		return fmt.Sprintf("(~%s)", code), typeInt, nil
	default:
		return "", typeUnknown, fmt.Errorf("генерация C из IR: неизвестная унарная операция %q", e.Op)
	}
}

// generateIRBinaryExpr доверяет типам, уже разрешённым internal/semantic
// (e.Type, ir.TypeOf(e.Left/e.Right)) - в отличие от cbackend.genBinaryExpr
// сам не перепроверяет допустимость операции для этих типов,
// это уже сделано семантическим анализом один раз (шаг 5.b), а не заново в
// каждом backend'е.
func generateIRBinaryExpr(ctx *irGenCtx, e *ir.BinaryExpr) (string, valueType, error) {
	lc, _, err := generateIRExpr(ctx, e.Left)
	if err != nil {
		return "", typeUnknown, err
	}
	rc, _, err := generateIRExpr(ctx, e.Right)
	if err != nil {
		return "", typeUnknown, err
	}
	lt, rt := ir.TypeOf(e.Left), ir.TypeOf(e.Right)

	switch e.Op {
	case "+":
		if lt == ir.TypeString && rt == ir.TypeString {
			ctx.usesConcat = true
			return fmt.Sprintf("rech_str_concat(%s, %s)", lc, rc), typeString, nil
		}
		return generateIRArithmetic(ctx, e.Op, lc, lt, rc, rt)

	case "-", "*", "/", "%":
		return generateIRArithmetic(ctx, e.Op, lc, lt, rc, rt)

	case "==", "!=":
		if lt == ir.TypeString && rt == ir.TypeString {
			ctx.usesStrCmp = true
			cmp := fmt.Sprintf("strcmp(%s, %s)", lc, rc)
			if e.Op == "==" {
				return fmt.Sprintf("(%s == 0)", cmp), typeBool, nil
			}
			return fmt.Sprintf("(%s != 0)", cmp), typeBool, nil
		}
		if lt.Kind == ir.KindEntity && lt == rt && !ctx.entityIsRef(lt.Entity) {
			// "структура" (значение) - поле-в-поле через сгенерированную
			// rech_struct_eq_... (struct_eq.go) - semantic
			// (structFieldsEquatable) уже проверил, что это разрешимо.
			// Ссылочная сущность/класс идёт ниже, обычным пойнтерным "==".
			registerStructEqEntity(ctx, lt.Entity)
			cmp := fmt.Sprintf("%s(%s, %s)", structEqFuncName(lt.Entity), lc, rc)
			if e.Op == "==" {
				return cmp, typeBool, nil
			}
			return fmt.Sprintf("(!%s)", cmp), typeBool, nil
		}
		return fmt.Sprintf("(%s %s %s)", lc, e.Op, rc), typeBool, nil

	case "<", ">", "<=", ">=":
		return fmt.Sprintf("(%s %s %s)", lc, e.Op, rc), typeBool, nil

	case "и", "или":
		cOp := "&&"
		if e.Op == "или" {
			cOp = "||"
		}
		return fmt.Sprintf("(%s %s %s)", lc, cOp, rc), typeBool, nil

	case "&", "|", "^", "<<", ">>":
		// Целое8/16/32/64, Натуральное/8/16/32/64 и Байт продвигаются до int
		// обычными арифметическими преобразованиями C - явный каст назад к
		// своему C-типу, тот же приём, что и у арифметики фикс-ширинных
		// типов (generateIRArithmetic). Обычное Целое (int64_t) уже
		// достаточно широкое - каста не требует.
		if _, _, ok := lt.Kind.IntWidthSigned(); ok && lt.Kind != ir.KindInt {
			vt := fromIRType(lt)
			return fmt.Sprintf("((%s)(%s %s %s))", vt.cType(ctx), lc, e.Op, rc), vt, nil
		}
		if lt == ir.TypeByte {
			return fmt.Sprintf("((unsigned char)(%s %s %s))", lc, e.Op, rc), typeByte, nil
		}
		return fmt.Sprintf("(%s %s %s)", lc, e.Op, rc), typeInt, nil

	default:
		return "", typeUnknown, fmt.Errorf("генерация C из IR: неизвестная операция %q", e.Op)
	}
}

// generateIRArithmetic - "+" (нестроковый)/"-"/"*"/"/"/"%": приведение
// операнда к double при смешении Целое/Число - та же промоция, что и в
// generateIRArithmetic.
func generateIRArithmetic(ctx *irGenCtx, op string, lc string, lt ir.Type, rc string, rt ir.Type) (string, valueType, error) {
	// Целое8/16/32/64, Натуральное/8/16/32/64 - строго между операндами
	// одного и того же Kind (internal/semantic уже это гарантировал,
	// analyzeArithmetic) - результат явно приводится к
	// C-типу самого Kind (cType), чтобы усечение/оборачивание при
	// переполнении произошло НА УРОВНЕ ВЫРАЖЕНИЯ (не только при
	// присваивании) - иначе результат промежуточного `a + b` без
	// присваивания остался бы в более широком типе после обычных
	// арифметических преобразований C. "/" - отдельный рантайм-хелпер с
	// панике на ноль (как и у Целое, rech_int_div) - rech_uint_div для
	// беззнаковых, чтобы не интерпретировать Натуральное64 > INT64_MAX как
	// отрицательное значение через rech_int_div.
	if lt == rt {
		if bits, signed, ok := lt.Kind.IntWidthSigned(); ok && lt.Kind != ir.KindInt {
			vt := fromIRType(lt)
			cType := vt.cType(ctx)
			if op == "/" || op == "%" {
				ctx.usesExceptions = true
				isMod := op == "%"
				fn := intDivModFuncName(signed, isMod, bits)
				if signed {
					ctx.intDivMod.mark(isMod, bits)
				} else {
					ctx.uintDivMod.mark(isMod, bits)
				}
				// Хелпер сам возвращает НАСТОЯЩИЙ C-тип своей ширины
				// (int8_t/.../uint64_t) - никакого внешнего "((cType)...)"
				// не нужно, в отличие от +/-/* ниже, где C сам продвигает
				// операнды до int и результат приходится усекать вручную.
				return fmt.Sprintf("%s(%s, %s)", fn, lc, rc), vt, nil
			}
			return fmt.Sprintf("((%s)(%s %s %s))", cType, lc, op, rc), vt, nil
		}
	}
	if lt == ir.TypeFloat || rt == ir.TypeFloat {
		if lt == ir.TypeInt {
			lc = fmt.Sprintf("((double)%s)", lc)
		}
		if rt == ir.TypeInt {
			rc = fmt.Sprintf("((double)%s)", rc)
		}
		// Дробное (float) - слабее Действительное (double) в numericRanks -
		// C сам продвигает float до double при смешении, явный каст не
		// нужен (в отличие от Целое выше, которому явный "(double)" нужен
		// всегда - у него нет "родного" плавающего представления).
		if op == "%" {
			ctx.usesMath = true
			return fmt.Sprintf("fmod(%s, %s)", lc, rc), typeFloat, nil
		}
		return fmt.Sprintf("(%s %s %s)", lc, op, rc), typeFloat, nil
	}
	if lt == ir.TypeFloat32 || rt == ir.TypeFloat32 {
		// Целое смешанное с Дробное - C сам продвигает int до float при
		// обычной арифметике; "%" на float/double в C не определён вовсе -
		// нужен явный fmodf, не оператор.
		if op == "%" {
			ctx.usesMath = true
			return fmt.Sprintf("fmodf(%s, %s)", lc, rc), typeFloat32, nil
		}
		return fmt.Sprintf("(%s %s %s)", lc, op, rc), typeFloat32, nil
	}
	if op == "/" || op == "%" {
		ctx.usesExceptions = true
		isMod := op == "%"
		// Обычное 64-битное "Целое" делит хелпер с "Целое64" (см. package
		// doc irGenCtx.intDivMod) - оба реально 64 бита в C, разница только
		// в спеллинге cType ("long long" против "int64_t").
		ctx.intDivMod.mark(isMod, 64)
		return fmt.Sprintf("%s(%s, %s)", intDivModFuncName(true, isMod, 64), lc, rc), typeInt, nil
	}
	return fmt.Sprintf("(%s %s %s)", lc, op, rc), typeInt, nil
}

// intDivModUsage - какие ширины/операции целочисленного деления/остатка
// реально понадобились программе (generateIRArithmetic) - по одному биту на
// комбинацию (ширина × "/" или "%"), не общий флаг на всё семейство: каждый
// rech_int_div<N>/rech_int_mod<N> - НАСТОЯЩИЙ C-тип своей ширины
// (int8_t/int16_t/int32_t/int64_t), не общий "продвинуть до 64 бит и усечь
// обратно" (в отличие от более раннего среза) - неиспользуемая static-
// функция другой ширины/операции была бы предупреждением (потенциально
// ошибкой при -Werror) у настоящего cc, значит эмитировать нужно только то,
// что реально вызвано. Обычное 64-битное "Целое" и "Целое64" (аналогично
// "Натуральное"/"Натуральное64") делят один и тот же 64-битный хелпер - оба
// реально 64 бита в C.
type intDivModUsage struct {
	div8, div16, div32, div64 bool
	mod8, mod16, mod32, mod64 bool
}

func (u *intDivModUsage) mark(isMod bool, bits int) {
	switch {
	case !isMod && bits == 8:
		u.div8 = true
	case !isMod && bits == 16:
		u.div16 = true
	case !isMod && bits == 32:
		u.div32 = true
	case !isMod && bits == 64:
		u.div64 = true
	case isMod && bits == 8:
		u.mod8 = true
	case isMod && bits == 16:
		u.mod16 = true
	case isMod && bits == 32:
		u.mod32 = true
	case isMod && bits == 64:
		u.mod64 = true
	}
}

func (u intDivModUsage) wants(isMod bool, bits int) bool {
	switch {
	case !isMod && bits == 8:
		return u.div8
	case !isMod && bits == 16:
		return u.div16
	case !isMod && bits == 32:
		return u.div32
	case !isMod && bits == 64:
		return u.div64
	case isMod && bits == 8:
		return u.mod8
	case isMod && bits == 16:
		return u.mod16
	case isMod && bits == 32:
		return u.mod32
	case isMod && bits == 64:
		return u.mod64
	}
	return false
}

func (u intDivModUsage) any() bool {
	return u.div8 || u.div16 || u.div32 || u.div64 || u.mod8 || u.mod16 || u.mod32 || u.mod64
}

// intDivModFuncName - единая точка сборки имени rech_int_div<N>/
// rech_int_mod<N>/rech_uint_div<N>/rech_uint_mod<N>, чтобы вызывающий код
// (generateIRArithmetic) и генератор текста хелпера (builtins_shared.go,
// intDivModHelperCode) не могли разойтись в написании.
func intDivModFuncName(signed bool, isMod bool, bits int) string {
	sign := "int"
	if !signed {
		sign = "uint"
	}
	kind := "div"
	if isMod {
		kind = "mod"
	}
	return fmt.Sprintf("rech_%s_%s%d", sign, kind, bits)
}

// generateIRNewExpr генерирует "новый Тип(...)" - e.Args уже позиционны по
// полю (см. package doc ir.NewExpr), имена берутся из ctx.entities[e.Entity]
// .Fields, собранных раньше в GenerateFromIR. "структура" - C99 compound
// literal прямо в месте вызова (значение, копируется дальше как обычно
// присваиванием/передачей параметра); ссылочная "сущность"/"класс" - вызов
// сгенерированного конструктора (generateIREntityCtor, GC_MALLOC), который
// возвращает указатель - тот же аргументы, но позиционные параметры функции,
// не designated initializer.
func generateIRNewExpr(ctx *irGenCtx, e *ir.NewExpr) (string, valueType, error) {
	ent := ctx.entities[e.Entity]
	argCodes := make([]string, len(e.Args))
	for i, a := range e.Args {
		code, _, err := generateIRExpr(ctx, a)
		if err != nil {
			return "", typeUnknown, err
		}
		argCodes[i] = code
	}
	if ctx.entityIsRef(e.Entity) {
		return fmt.Sprintf("%s(%s)", ctorCName(ctx, e.Entity), strings.Join(argCodes, ", ")), entityType(e.Entity), nil
	}
	parts := make([]string, len(argCodes))
	for i, code := range argCodes {
		if ent.IsExternStruct && ent.Fields[i].Type.Kind == ir.KindCallable {
			// "Вызываемый(...)" полем "внешняя структура" (WNDCLASS.
			// lpfnWndProc-стиль) - тот же приём, что и у аргумента внешней
			// функции (case *ir.CallExpr выше) - {fn, env} по значению не
			// то, что ожидает C (голый указатель на функцию, тип поля уже
			// объявлен так generateIREntityStruct'ом) - marshalCallbackArg
			// подменяет code на C-имя trampoline.
			marshaled, err := ctx.marshalCallbackArg(ent.Fields[i].Type, code)
			if err != nil {
				return "", typeUnknown, err
			}
			code = marshaled
		}
		parts[i] = fmt.Sprintf(".%s = %s", mangleField(ent.Fields[i].Name), code)
	}
	initList := strings.Join(parts, ", ")
	if initList == "" {
		// Сущность без полей - зануляем техническую заглушку структуры
		// (generateIREntityStruct), а не полагаемся на "{}" (расширение
		// GNU, не входящее в стандартный C99) - как и в AST-пути.
		initList = "0"
	}
	return fmt.Sprintf("(%s){%s}", entityCName(ctx, e.Entity), initList), entityType(e.Entity), nil
}

// generateIRMethodCallExpr генерирует вызов метода - получатель по указателю
// (первый скрытый параметр this_): мутация поля через "этот.поле = ..."
// внутри метода видна вызывающему коду после возврата. Получатель ссылочной
// сущности - уже указатель сам по себе (передаётся как есть, receiverIsPointer);
// получатель "структура" (или получатель, порождённый ThisExpr) - значение,
// нужен "&", чтобы получить указатель для this_ - тот же принцип, что и у
// выбора "."/"->" в FieldAccess/FieldAssign.
// generateIRAddressableExpr генерирует receiver ОДИН раз и возвращает и
// обычный C-код значения (code), и код указателя на него (ptr) - для
// вызова метода/безопасной индексации, которым нужен указатель на
// получателя (this_/this_ у *_get_safe). Для ссылочной сущности/
// интерфейса/generic-коллекции (receiverIsPointer - истина) получателем
// годится ЛЮБОЕ выражение, включая результат другого вызова
// (`x.метод1().метод2()`) - "&" не берётся вовсе, code уже указатель. Для
// "структура" (значение) - materializeIfNeeded: если receiver уже
// адресуем (переменная/поле/"этот") - "&(code)" синтаксически валиден
// напрямую; иначе (результат вызова - rvalue, "&" от него невалиден в C) -
// материализуется временная переменная (см. package doc там же), и "&"
// берётся уже от неё. Не пересчитывает code дважды, в отличие от вызова
// generateIRExpr отдельно для каждой формы.
func generateIRAddressableExpr(ctx *irGenCtx, receiver ir.Expr) (code, ptr string, err error) {
	code, _, err = generateIRExpr(ctx, receiver)
	if err != nil {
		return "", "", err
	}
	if ctx.receiverIsPointer(receiver) {
		return code, fmt.Sprintf("(%s)", code), nil
	}
	code = ctx.materializeIfNeeded(receiver, code)
	return code, fmt.Sprintf("&(%s)", code), nil
}

func generateIRMethodCallExpr(ctx *irGenCtx, e *ir.MethodCallExpr) (string, valueType, error) {
	switch ir.TypeOf(e.Receiver).Kind {
	case ir.KindAny:
		return generateIRAnyMethodCall(ctx, e)
	case ir.KindTypeInfo:
		return generateIRTypeInfoMethodCall(ctx, e)
	}
	receiverCode, receiverPtr, err := generateIRAddressableExpr(ctx, e.Receiver)
	if err != nil {
		return "", typeUnknown, err
	}
	if len(e.RevokesStates) > 0 {
		wrapped, err := wrapRevokeStates(ctx, e.Entity, e.RevokesStates, receiverPtr)
		if err != nil {
			return "", typeUnknown, err
		}
		receiverPtr = wrapped
	}
	argCodes := make([]string, len(e.Args)+1)
	argCodes[0] = receiverPtr
	for i, a := range e.Args {
		code, _, err := generateIRExpr(ctx, a)
		if err != nil {
			return "", typeUnknown, err
		}
		argCodes[i+1] = code
	}
	if ctx.needsDispatch(e.Entity) {
		return generateDispatchCall(ctx, e, receiverCode, argCodes)
	}
	return fmt.Sprintf("%s(%s)", methodCName(ctx, e.Entity, e.Method), strings.Join(argCodes, ", ")), fromIRType(e.Type), nil
}

// generateIRStaticMethodCallExpr - "Тип::метод(...)" - как
// generateIRMethodCallExpr, но без получателя (см. package doc
// ir.StaticMethodCallExpr) - статический метод не участвует в
// диспетчеризации (needsDispatch), вызывается напрямую по мангл-имени.
func generateIRStaticMethodCallExpr(ctx *irGenCtx, e *ir.StaticMethodCallExpr) (string, valueType, error) {
	argCodes := make([]string, len(e.Args))
	for i, a := range e.Args {
		code, _, err := generateIRExpr(ctx, a)
		if err != nil {
			return "", typeUnknown, err
		}
		argCodes[i] = code
	}
	return fmt.Sprintf("%s(%s)", mangleMethod(e.Entity, e.Method), strings.Join(argCodes, ", ")), fromIRType(e.Type), nil
}

// fromIRType переводит ir.Type (backend-независимый, internal/ir) в
// valueType (C-специфичный, этот пакет) - единственное место, где IR-путь
// знает про valueType вообще, остальной generateIR* код работает через
// ir.Type/ir.TypeOf.
// irKindToValueType - ir.Kind -> valueType для всех примитивов (не
// KindEntity - у него есть переменная часть, имя сущности, см. fromIRType).
var irKindToValueType = map[ir.TypeKind]valueType{
	ir.KindByte:         typeByte,
	ir.KindInt:          typeInt,
	ir.KindFloat32:      typeFloat32,
	ir.KindFloat:        typeFloat,
	ir.KindChar:         typeChar,
	ir.KindString:       typeString,
	ir.KindBool:         typeBool,
	ir.KindFile:         typeFile,
	ir.KindSocket:       typeSocket,
	ir.KindDB:           typeDB,
	ir.KindHTTP2Session: typeHTTP2Session,
	ir.KindInt8:         typeInt8,
	ir.KindInt16:        typeInt16,
	ir.KindInt32:        typeInt32,
	ir.KindInt64:        typeInt64,
	ir.KindUint:         typeUint,
	ir.KindUint8:        typeUint8,
	ir.KindUint16:       typeUint16,
	ir.KindUint32:       typeUint32,
	ir.KindUint64:       typeUint64,
	ir.KindAny:          typeAny,
	ir.KindTypeInfo:     typeTypeInfo,
}

func fromIRType(t ir.Type) valueType {
	if t.Kind == ir.KindEntity {
		return entityType(t.Entity)
	}
	if t.Kind == ir.KindTask {
		return taskType(t.Entity)
	}
	if t.Kind == ir.KindCoroTask {
		return coroTaskType(t.Entity)
	}
	if t.Kind == ir.KindCallable {
		return callableType(t.Entity)
	}
	if t.Kind == ir.KindExternEntity {
		return externEntityType(t.Entity)
	}
	if t.Kind == ir.KindNullable {
		return nullableType(t.Entity)
	}
	if t.Kind == ir.KindPointer {
		return pointerType(t.Entity)
	}
	if vt, ok := irKindToValueType[t.Kind]; ok {
		return vt
	}
	return typeUnknown
}
