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

	"github.com/IlyaFeoktistov/Rech/internal/backend/cbackend/runtime/bytesio"
	"github.com/IlyaFeoktistov/Rech/internal/backend/cbackend/runtime/collections"
	"github.com/IlyaFeoktistov/Rech/internal/backend/cbackend/runtime/color"
	"github.com/IlyaFeoktistov/Rech/internal/backend/cbackend/runtime/concurrency"
	"github.com/IlyaFeoktistov/Rech/internal/backend/cbackend/runtime/console"
	"github.com/IlyaFeoktistov/Rech/internal/backend/cbackend/runtime/cookies"
	"github.com/IlyaFeoktistov/Rech/internal/backend/cbackend/runtime/crypto"
	"github.com/IlyaFeoktistov/Rech/internal/backend/cbackend/runtime/datetime"
	"github.com/IlyaFeoktistov/Rech/internal/backend/cbackend/runtime/http2"
	"github.com/IlyaFeoktistov/Rech/internal/backend/cbackend/runtime/httpbytes"
	"github.com/IlyaFeoktistov/Rech/internal/backend/cbackend/runtime/httpserver"
	"github.com/IlyaFeoktistov/Rech/internal/backend/cbackend/runtime/httpwire"
	"github.com/IlyaFeoktistov/Rech/internal/backend/cbackend/runtime/image"
	"github.com/IlyaFeoktistov/Rech/internal/backend/cbackend/runtime/interfacestub"
	"github.com/IlyaFeoktistov/Rech/internal/backend/cbackend/runtime/keyboard"
	"github.com/IlyaFeoktistov/Rech/internal/backend/cbackend/runtime/multipart"
	"github.com/IlyaFeoktistov/Rech/internal/backend/cbackend/runtime/numbase"
	"github.com/IlyaFeoktistov/Rech/internal/backend/cbackend/runtime/options"
	"github.com/IlyaFeoktistov/Rech/internal/backend/cbackend/runtime/process"
	"github.com/IlyaFeoktistov/Rech/internal/backend/cbackend/runtime/procmem"
	"github.com/IlyaFeoktistov/Rech/internal/backend/cbackend/runtime/procregs"
	"github.com/IlyaFeoktistov/Rech/internal/backend/cbackend/runtime/regex"
	"github.com/IlyaFeoktistov/Rech/internal/backend/cbackend/runtime/signals"
	"github.com/IlyaFeoktistov/Rech/internal/backend/cbackend/runtime/stringsplit"
	"github.com/IlyaFeoktistov/Rech/internal/backend/cbackend/runtime/strutil"
	"github.com/IlyaFeoktistov/Rech/internal/backend/cbackend/runtime/subprocess"
	"github.com/IlyaFeoktistov/Rech/internal/backend/cbackend/runtime/syncprim"
	"github.com/IlyaFeoktistov/Rech/internal/backend/cbackend/runtime/tzdata"
	"github.com/IlyaFeoktistov/Rech/internal/backend/cbackend/runtime/udp"
	"github.com/IlyaFeoktistov/Rech/internal/ir"
	"github.com/IlyaFeoktistov/Rech/internal/rtmsg"
)

// byteListType - см. internal/semantic/bytesio.go, тот же принцип
// дублирования имени константой в обоих пакетах (не импорта internal/
// semantic отсюда), что и у image.EntityName (runtime/image) -
// runtime/bytesio получает мангленное имя параметром (см. её package doc),
// а не читает эту константу сама, поэтому дублирования между ней и этим
// файлом нет - только между этим файлом и internal/semantic.
const byteListType = "Список<Байт>"

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
	// usesMath/... - см. их же сборку ниже).
	includesStringH bool
	// usesText - хоть одна точка входа внешних байт в Строку (файлы,
	// сокеты, процессы, БД, FFI, строка_из_байт, декодеры) - эмитит
	// strutil.TextCode.
	usesText        bool
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
	libMode bool
	// service - "сборка.тип: служба" (ir.Program.Service): тело "главная"
	// - process.ServiceBodyName, main() - process.ServiceMainCode.
	service bool
	// mainArgsType - тип параметра "главная(имя: Список<Строка>)", "" - без
	// параметра: нужен ли rech_main_args (main_args.go).
	mainArgsType string
	entities     map[string]*ir.Entity
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
	// externHeaders - prog.ExternHeaders ("сборка.заголовки" файла проекта,
	// internal/project.File.Заголовки) - nil, если проект их не объявил.
	// Непусто - каждый эмитится "#include <...>" (generateSource); прототипы
	// "внешняя функция" с ним не конфликтуют (свои C-имена, extern_symbols.go).
	externHeaders []string
	thisEntity    string

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

	// irFuncs - функции программы по имени (нужно generateIRLaunchExpr,
	// чтобы найти параметры/тип результата обёрнутой функции - см.
	// internal/backend/cbackend/concurrency.go, ensureTaskWrapper).
	irFuncs map[string]*ir.Function
	// taskCode - обёртки задач (concurrency.go): структура задачи и
	// rech_await_... - один раз на сигнатуру (taskSigsEmitted), запуск на
	// потоке ОС (threadLaunchEmitted) и на корутине (coroLaunchEmitted) -
	// отдельно, по первой встрече каждого вида; taskSyncEmitted - общая для
	// всех сигнатур синхронизация потока. Заполняется по ходу генерации
	// тела, вставляется в файл в нужном месте в конце. usesThreads -
	// гейтит #include <pthread.h>/поточные заголовки Windows и пул
	// воркеров; usesCoroutines (ИЛИ usesSockets - сетевые builtin'ы
	// опираются на планировщик) - рантайм корутин (runtime/concurrency/
	// coroutine.go) целиком.
	taskCode            strings.Builder
	taskSigsEmitted     map[string]bool
	threadLaunchEmitted map[string]bool
	coroLaunchEmitted   map[string]bool
	taskSyncEmitted     bool
	usesThreads         bool
	usesCoroutines      bool

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
	// usesGlobalInit - в программе есть переменные уровня модуля - гейтит
	// вызов rech_init_globals() из main() (globals.go).
	usesGlobalInit bool

	// usesAnyText/usesAnyEq - "${з}"/"вывести з" и "==" у "Значение"
	// (any_value.go).
	usesAnyText bool
	usesAnyEq   bool

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
	// rawThunksEmitted/rawThunksOptEmitted/toCallbackEmitted/
	// toCallbackOptEmitted - дедупликация FFI-преобразований "указатель на
	// функцию <-> Вызываемый" (ffi_callback.go), по сигнатуре.
	rawThunksEmitted     map[string]bool
	rawThunksOptEmitted  map[string]bool
	toCallbackEmitted    map[string]bool
	toCallbackOptEmitted map[string]bool
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

	// body - соглашение C-функции, тело которой сейчас генерируется (см.
	// bodyKind) - читается ir.ReturnStmt.
	body bodyKind
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
// interfacestub.GenerateByFile) - ключ карты - путь файла-источника
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
	interfaceStubs, err = interfacestub.GenerateByFile(prog, libRoot)
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
		if ent.Name == http2.StreamTypeC {
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
		externHeaders:         prog.ExternHeaders,
		usesGC:                usesGC,
		usesStateFlags:        usesStateFlags,
		libMode:               libMode,
		service:               prog.Service,
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
		usesVtable:          len(participants) > 0,
		excTypeIDs:          excTypeIDs,
		excTypeNames:        excTypeNames,
		excIsaTable:         excIsaTable,
		taskSigsEmitted:     map[string]bool{},
		threadLaunchEmitted: map[string]bool{},
		coroLaunchEmitted:   map[string]bool{},
	}
	ctx.irFuncs = map[string]*ir.Function{}
	for _, fn := range prog.Functions {
		ctx.irFuncs[fn.Name] = fn
	}
	if ent, ok := entities[syncprim.MutexEntityName]; ok && ent.IsMutex {
		ctx.usesMutex = true
	}
	if ent, ok := entities[syncprim.SemaphoreEntityName]; ok && ent.IsSemaphore {
		ctx.usesSemaphore = true
	}
	if ent, ok := entities[transactionEntityName]; ok && ent.IsTransaction {
		ctx.usesDBTransaction = true
	}
	// Встроенные паники (rech_panic_as) бросаются тем же rech_throw, что и
	// исключения, а паника возможна в любой программе - рантайм
	// исключений нужен всегда.
	ctx.usesExceptions = true
	// Служба останавливается через тот же флаг, что и SIGINT/SIGTERM
	// (process.ServiceMainCode), даже если программа его не опрашивает.
	if ctx.service {
		ctx.usesShutdownFlag = true
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
				if f.Type.Kind == ir.KindCallable || callableCollections[f.Type.Entity] || (f.Type.Kind == ir.KindNullable && ctx.nullableInner[f.Type.Entity].Kind == ir.KindCallable) {
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
	globalDecl := globalVarsDecl(ctx, prog)
	globalInit, err := globalVarsInitCode(ctx, prog)
	if err != nil {
		return "", err
	}
	ctx.usesGlobalInit = globalInit != ""

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
	var genericsCode, genericsCodeAfterCallables, genericsCodeAfterEntities, indexSafeGetCode, toListCode strings.Builder
	usesGenerics := len(prog.Generics) > 0
	// hasStructTargets - вычислена здесь (не там, где раньше использовалась
	// единственный раз, ниже), потому что теперь нужна РАНЬШЕ - см. package
	// doc ctx.includesStringH.
	hasStructTargets := len(prog.ParseTargets) > 0 || len(prog.SerializeTargets) > 0 || len(prog.XMLParseTargets) > 0 || len(prog.XMLSerializeTargets) > 0 || len(prog.YAMLParseTargets) > 0 || len(prog.YAMLSerializeTargets) > 0 || len(prog.ProtobufParseTargets) > 0 || len(prog.ProtobufSerializeTargets) > 0
	genericByName := make(map[string]ir.GenericInstantiation, len(prog.Generics))
	for _, g := range prog.Generics {
		genericByName[g.Name] = g
	}
	for _, g := range prog.Generics {
		dest := &genericsCode
		// Элемент "?Тип" ждёт свою обёртку, а та - то же, что и сам Тип
		// (группы nullablesCode/nullablesAfterTypedefs/nullablesAfterCallables
		// ниже) - бакет выбирается по внутреннему типу.
		elemOrder := g.ElemType
		if elemOrder.Kind == ir.KindNullable {
			elemOrder = ctx.nullableInner[elemOrder.Entity]
		}
		switch {
		case elemOrder.Kind == ir.KindCallable || g.KeyType.Kind == ir.KindCallable:
			dest = &genericsCodeAfterCallables
		case genericNeedsEntities(ctx, entities, genericByName, g):
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
			structC := mangleType(g.Name)
			dest.WriteString(collections.ListCollectionCode(elemC, structC,
				mangleMethod(g.Name, "добавить"), mangleMethod(g.Name, "получить"), mangleMethod(g.Name, "установить"),
				mangleMethod(g.Name, "удалить"), mangleMethod(g.Name, "длина"), mangleMethod(g.Name, "пусто"),
				mangleMethod(g.Name, "первый"), mangleMethod(g.Name, "последний"), escapeCString(rtmsg.IndexOutOfRange(g.Name))))
			dest.WriteString(collections.GenericCtorCode(structC, mangleCtor(g.Name)))
			if g.Equatable {
				dest.WriteString(listEqualCCode(ctx, g))
			}
			if g.NeedsSafeIndex {
				indexSafeGetCode.WriteString(collections.ListIndexSafeGetCode(mangleType(g.Name), mangleType(ir.NullableOf(g.ElemType).String()), g.ElemType.Kind == ir.KindNullable))
			}
			if g.NeedsListLiteral {
				dest.WriteString(collections.ListLiteralCode(mangleType(g.Name), elemC, mangleCtor(g.Name)))
			}
		case "Стопка":
			structC := mangleType(g.Name)
			dest.WriteString(collections.StackCollectionCode(elemC, structC,
				mangleMethod(g.Name, "положить"), mangleMethod(g.Name, "снять"), mangleMethod(g.Name, "верх"),
				mangleMethod(g.Name, "длина"), mangleMethod(g.Name, "пусто"), escapeCString(rtmsg.EmptyStackOp(g.Name))))
			dest.WriteString(collections.GenericCtorCode(structC, mangleCtor(g.Name)))
			writeToListCode(&toListCode, generics, "stack", g, "значения", g.ElemType)
		case "Очередь":
			structC := mangleType(g.Name)
			dest.WriteString(collections.QueueCollectionCode(elemC, structC,
				mangleMethod(g.Name, "добавить"), mangleMethod(g.Name, "извлечь"), mangleMethod(g.Name, "первый"),
				mangleMethod(g.Name, "длина"), mangleMethod(g.Name, "пусто"), escapeCString(rtmsg.EmptyQueueOp(g.Name))))
			dest.WriteString(collections.GenericCtorCode(structC, mangleCtor(g.Name)))
			writeToListCode(&toListCode, generics, "queue", g, "значения", g.ElemType)
		case "Словарь":
			keyC := fromIRType(g.KeyType).cType(ctx)
			isStringKey := g.KeyType.Kind == ir.KindString
			if isStringKey {
				ctx.usesDictString = true
			}
			structC := mangleType(g.Name)
			dest.WriteString(collections.DictCollectionCode(keyC, elemC, structC,
				mangleMethod(g.Name, "установить"), mangleMethod(g.Name, "получить"), mangleMethod(g.Name, "содержит"),
				mangleMethod(g.Name, "удалить"), mangleMethod(g.Name, "длина"), mangleMethod(g.Name, "пусто"),
				escapeCString(rtmsg.DictKeyNotFound(g.Name)), isStringKey))
			dest.WriteString(collections.GenericCtorCode(structC, mangleCtor(g.Name)))
			writeToListCode(&toListCode, generics, "dictKeys", g, "ключи", g.KeyType)
			writeToListCode(&toListCode, generics, "dictValues", g, "значения", g.ElemType)
			if g.Equatable {
				dest.WriteString(dictEqualCCode(ctx, g))
			}
			if g.NeedsSafeIndex {
				indexSafeGetCode.WriteString(collections.DictIndexSafeGetCode(mangleType(g.Name), keyC, mangleType(ir.NullableOf(g.ElemType).String()), g.ElemType.Kind == ir.KindNullable))
			}
		case "Множество":
			ctx.usesDictString = ctx.usesDictString || g.KeyType.Kind == ir.KindString
			dest.WriteString(setCollectionCode(ctx, g))
			writeToListCode(&toListCode, generics, "dictKeys", g, "значения", g.KeyType)
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
	// условии panicPrototypes).
	//
	// nullablesAfterCallables - третья группа, "?Вызываемый(...)": обёртка
	// хранит {fn, env} по значению, typedef сигнатуры обязан быть раньше
	// (callableTypedefs).
	var nullablesCode, nullablesAfterTypedefs, nullablesAfterCallables strings.Builder
	usesNullables := len(prog.Nullables) > 0
	for _, n := range prog.Nullables {
		elemC := fromIRType(n.Inner).cType(ctx)
		code := collections.NullableCode(mangleType(n.Name), elemC, escapeCString(rtmsg.NullAccess))
		if n.Inner.Kind == ir.KindCallable {
			nullablesAfterCallables.WriteString(code)
		} else if n.Inner.Kind == ir.KindEntity && entities[n.Inner.Entity] != nil {
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
		protos.WriteString(syncprim.MutexPrototypes(mangleType(syncprim.MutexEntityName), mangleCtor(syncprim.MutexEntityName),
			mangleMethod(syncprim.MutexEntityName, "захватить"), mangleMethod(syncprim.MutexEntityName, "освободить")))
	}
	if ctx.usesSemaphore {
		// Та же причина, что у mutexPrototypes выше - "Семафор" тоже может
		// добавиться в out.Entities поздно относительно кода, который её
		// использует.
		protos.WriteString(syncprim.SemaphorePrototypes(mangleType(syncprim.SemaphoreEntityName), mangleCtor(syncprim.SemaphoreEntityName),
			mangleMethod(syncprim.SemaphoreEntityName, "захватить"), mangleMethod(syncprim.SemaphoreEntityName, "освободить")))
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
	// место, где эти флаги взводятся, поэтому раньше их не узнать.
	// Точки входа внешних байт в Строку - используют strutil.TextCode.
	ctx.usesText = ctx.usesText || ctx.usesFileIO || ctx.usesConsoleInput || ctx.usesByteStringConv || ctx.usesBytesIO || ctx.usesSockets || ctx.usesEnv || ctx.usesSubprocess || prog.UsesDB || ctx.usesHTTP || ctx.usesHTTPServer || ctx.usesHTTP2 || ctx.usesMultipart || ctx.usesUDPDatagram || ctx.usesCrypto || ctx.usesAES || hasStructTargets || ctx.usesQueryParams
	ctx.includesStringH = ctx.usesText || ctx.usesConcat || ctx.usesStrCmp || ctx.usesConsoleInput || ctx.usesFileIO || ctx.usesSockets || ctx.usesCast || ctx.usesMemcpy || ctx.usesDictString || ctx.usesQueryParams || ctx.usesCookies || ctx.usesStringOps || ctx.usesStringText || ctx.usesCrypto || ctx.usesEnv || ctx.usesExceptions || ctx.usesStringSplit || ctx.usesRegex || ctx.usesHTTPHeaders || ctx.usesHTTPHeaderValues || ctx.usesColor || ctx.usesSubprocess || ctx.usesCwd || hasStructTargets || prog.UsesReflection || ctx.usesTZ || ctx.usesDateParse
	// usesNullables/prog.UsesReflection - те же два условия, что у panicGate
	// (ниже) сверх её собственных: panicGate решает, эмитить ли panicPrototypes
	// (безусловный exit(1)), includesStdlibH - подключать ли <stdlib.h>, где
	// exit() объявлен - расхождение между двумя списками значило "exit()
	// вызывается, а <stdlib.h> не подключён" (implicit-function-declaration
	// - GCC на Linux это лишь предупреждение, но Apple Clang - жёсткая
	// ошибка компиляции). usesNullables фактически истинен почти всегда -
	// встроенное "Исключение" безусловно регистрирует "?Целое" для поля
	// "код" (см. package doc systemExceptionFields), значит panicPrototypes (а
	// с ним и <stdlib.h>) эмитится в подавляющем большинстве программ, не
	// только там, где пользователь сам пишет "?Тип".
	ctx.includesStdlibH = ctx.usesText || ctx.usesConcat || usesGenerics || usesNullables || ctx.usesPanic || ctx.usesConsoleInput || ctx.usesFileIO || ctx.usesCast || ctx.usesExceptions || ctx.usesDateTime || ctx.usesColor || ctx.usesQueryParams || ctx.usesCookies || ctx.usesRandom || ctx.usesStringOps || ctx.usesStringText || ctx.usesCrypto || ctx.usesEnv || ctx.usesRegex || ctx.usesHTTPHeaders || ctx.usesHTTPHeaderValues || hasStructTargets || ctx.usesExit || ctx.usesTZ || ctx.usesHumanDate || prog.UsesReflection
	ctx.includesMathH = ctx.usesMath
	if len(prog.ExternFuncs) > 0 {
		protos.WriteString(externSymbolMacros)
	}
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
	if ctx.usesRandom {
		// _CRT_RAND_S должен быть определён ДО ЛЮБОГО системного заголовка,
		// не только до собственного "#include <stdlib.h>" ниже - на
		// MinGW-w64 "#include <windows.h>" (эмитится безусловно на Windows,
		// см. ниже) транзитивно тянет <stdlib.h> сам, раньше, чем эта
		// программа успела бы дойти до своего явного include - объявление
		// rand_s (crypto.RandomHelpers) тогда пропадает молча (заголовочный
		// guard <stdlib.h> уже сработал БЕЗ макроса), и gcc падает с
		// "implicit declaration of function 'rand_s'" - найдено живым
		// прогоном на настоящем MinGW-w64 (не воспроизводится на MSVC,
		// который либо не имеет такого транзитивного пути, либо переживает
		// повторное включение иначе).
		sb.WriteString("#ifdef _WIN32\n#define _CRT_RAND_S\n#endif\n")
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
	// "сборка.заголовки" файла проекта (ctx.externHeaders) - подключены как
	// есть, доверием пользователя (см. package doc extern_headers.go) -
	// делает настоящий прототип видимым для ЛЮБОЙ "внешняя функция"/
	// "внешняя структура" программы, не только курируемого списка ниже.
	for _, h := range ctx.externHeaders {
		fmt.Fprintf(&sb, "#include <%s>\n", h)
	}
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
	if ctx.usesKeyboard {
		// <linux/input.h> - struct input_event/EV_KEY/KEY_*/EVIOCGBIT
		// (runtime/keyboard, "прочитать_код_клавиши"). <dirent.h>/<fcntl.h> -
		// перебор /dev/input/event* (opendir/readdir) и open() устройства.
		// <poll.h>/<time.h> - тот же неблокирующий опрос с дедлайном, что и
		// у usesNonBlockingRead выше, плюс clock_gettime(CLOCK_MONOTONIC)
		// для многошагового таймаута (несколько событий подряд могут не
		// быть клавишей). <string.h> - memset/strncmp/snprintf. На Windows
		// ничего из этого не нужно - ReadConsoleInputW уже объявлен через
		// windows.h, подключённый безусловно выше.
		sb.WriteString("#ifndef _WIN32\n#include <linux/input.h>\n#include <dirent.h>\n#include <fcntl.h>\n#include <poll.h>\n#include <time.h>\n#include <string.h>\n#include <unistd.h>\n#endif\n")
	}
	if ctx.usesCwd {
		// <unistd.h> - POSIX getcwd() (runtime/process/cwd.go, "текущий_
		// каталог"). На Windows не нужен - GetCurrentDirectoryW уже
		// объявлена через windows.h, подключённый безусловно выше.
		sb.WriteString("#ifndef _WIN32\n#include <unistd.h>\n#endif\n")
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
	// <zlib.h> - gzip-(де)компрессия gRPC-сообщений (grpc.go,
	// rech_gzip_compress/rech_gzip_decompress) - безусловно вместе с gRPC
	// (ctx.usesGRPC), не отдельным опт-ином: "grpc_прочитать_сообщение"
	// обязана уметь распаковать входящее сжатое сообщение от НАСТОЯЩЕГО
	// gRPC-собеседника независимо от того, использует ли САМА программа
	// "grpc_отправить_сообщение_сжато" (-lz, internal/toolchain/cc.go, по
	// маркеру этого #include).
	if ctx.usesGRPC {
		sb.WriteString("#include <zlib.h>\n")
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
	if ctx.usesThreads || ctx.usesMutex || ctx.usesSemaphore || ctx.usesTLS || prog.UsesDB || ctx.usesActiveTaskWait || ctx.usesOptions {
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
	// Прототипы rech_panic/rech_panic_as - раньше любого хелпера, который
	// их вызывает; определения (panicRuntimeCode) - после прототипов
	// конструкторов встроенных паник.
	// strutil.TextCode - починка внешних байт в валидный текст (правило
	// "Строка - валидный UTF-8 без NUL", internal/textutf8) - раньше
	// любого хелпера, читающего внешние данные.
	appendCodeIf(&sb, ctx.usesText, strutil.TextCode)
	sb.WriteString(panicPrototypes)
	// concurrency.CoroutineRuntimeCode (runtime/concurrency/coroutine.go) -
	// ЗДЕСЬ, не рядом с taskCode/
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
	appendCodeIf(&sb, ctx.usesCoroutines || ctx.usesSockets, concurrency.CoroutineRuntimeCode)
	appendCodeIf(&sb, ctx.usesCoroutines || ctx.usesSockets, concurrency.CoroutineRuntimeWindowsCode)
	appendCodeIf(&sb, ctx.usesCurrentOS, currentOSHelper)
	appendCodeIf(&sb, ctx.usesCast, castHelpers)
	appendCodeIf(&sb, ctx.usesIntSign, rechSignHelper)
	appendCodeIf(&sb, ctx.usesIntMinMax, rechIntMinMaxHelper)
	appendCodeIf(&sb, ctx.usesClamp, rechClampHelper)
	appendCodeIf(&sb, ctx.usesIntPow, rechIntPowHelper)

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
		sb.WriteString(anyValueProtos(ctx))
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
		sb.WriteString(http2.SessionTypedefCode)
		sb.WriteString(http2.CallbacksCode(ctx.usesTLS))
	}

	// taskForwardDecl - "typedef struct rech_TASK_... rech_TASK_..." для
	// всех задач, реально используемых в этой программе. Типы задач всегда
	// представлены указателем на C-структуру, поэтому достаточно opaque
	// forward declaration раньше, чем generic/nullable/field typedefs, где
	// они могут встречаться как элемент коллекции или nullable-обёртки.
	//
	// Полное определение само по себе остаётся в ctx.taskCode/ctx.coroTaskCode
	// позднее по файлу, но на момент первого использования C уже видит имя
	// типа и не падает на "unknown type name".
	var taskForwardDecl strings.Builder

	for _, sig := range prog.Callables {
		taskType := mangleTaskType(sig.Name)
		fmt.Fprintf(&taskForwardDecl, "typedef struct %s %s;\n", taskType, taskType)
	}

	appendCodeIf(&sb, taskForwardDecl.Len() > 0, taskForwardDecl.String())

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

	// entityForwardDecl - то же для ссылочных сущностей программы (тег
	// структуры - generateIREntityStruct): typedef сигнатуры Вызываемый/
	// задачи с указателем на сущность не зависит от того, в какой группе
	// typedefs (до или после Вызываемых) оказалась сама сущность.
	var entityForwardDecl strings.Builder
	for _, ent := range prog.Entities {
		if ent.IsStruct || ent.IsExternStruct || ent.IsMutex || ent.IsSemaphore || ent.IsTransaction {
			continue
		}
		fmt.Fprintf(&entityForwardDecl, "typedef struct %[1]s %[1]s;\n", entityCName(ctx, ent.Name))
	}
	appendCodeIf(&sb, entityForwardDecl.Len() > 0, entityForwardDecl.String())

	// До genericsCode (элемент "Список<?Целое>" хранится по значению -
	// обёртка нужна раньше тела коллекции; внутренний тип "?Список<T>" -
	// всего лишь указатель, ему хватает genericForwardDecl выше) и перед
	// typedefs (сущность может нести nullable-поле по значению).
	appendCodeIf(&sb, nullablesCode.Len() > 0, nullablesCode.String())
	appendCodeIf(&sb, genericsCode.Len() > 0, genericsCode.String())
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
	appendCodeIf(&sb, callableTypedefs.Len() > 0, callableTypedefs.String())
	appendCodeIf(&sb, nullablesAfterCallables.Len() > 0, nullablesAfterCallables.String())
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
	// После ВСЕХ групп generic-коллекций - "ключи()"/"значения()" собирают
	// Список<T>, который может стоять в любой из них (см. writeToListCode).
	appendCodeIf(&sb, toListCode.Len() > 0, toListCode.String())
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
	appendCodeIf(&sb, ctx.usesHTTP || ctx.usesHTTPServer, httpwire.ChunkedHelpers)
	// rech_header_line_matches - общая часть значение_заголовка/
	// значения_заголовка (httpheaders.go) И rech_http_send_response
	// (httpserver.go, проверка "уже задал ли пользователь Connection/
	// Content-Length сам") - РОВНО ОДИН раз для любого из трёх - до
	// typedefs, ей не нужны, но должна предшествовать httpserver.Helpers
	// ниже (та её зовёт).
	appendCodeIf(&sb, ctx.usesHTTPHeaders || ctx.usesHTTPHeaderValues || ctx.usesHTTPServer, httpwire.HeaderLineMatchesHelper())

	// После typedefs - rech_http_request принимает/возвращает Запрос/
	// Ответ по значению, их typedef должен быть уже виден.
	appendCodeIf(&sb, ctx.usesHTTP, httpHelpers)
	// После typedefs, тем же принципом, что и usesHTTP выше.
	appendCodeIf(&sb, ctx.usesHTTPServer, httpserver.Helpers(prog.HTTPMaxRequestBytes,
		mangleField("метод"), mangleField("путь"), mangleField("адрес"), mangleField("заголовки"),
		mangleField("порт"), mangleField("тело"), mangleType("Запрос"), mangleType("Ответ"),
		mangleField("код"), rtmsg.HTTPServerRecvFailed, rtmsg.HTTPServerRequestTooLarge))
	// После typedefs и rechDupHelper выше - rech_recv_from возвращает
	// "Дейтаграмма" по значению (её typedef уже должен быть виден) и
	// зовёт rech_dup для адреса отправителя.
	appendCodeIf(&sb, ctx.usesUDPDatagram, udp.DatagramHelpers(mangleType("Дейтаграмма"),
		mangleField("данные"), mangleField("адрес"), mangleField("порт")))
	// После genericsCode выше - rech_query_params использует typedef/
	// метод "установить" Словарь<Строка, Строка>, уже эмитированные там
	// (queryParamsDictType зарегистрирован в prog.Generics принудительно,
	// см. internal/semantic/httpserver.go).
	appendCodeIf(&sb, ctx.usesQueryParams, httpserver.QueryParamsHelper(
		mangleType(httpserver.QueryParamsDictType), mangleMethod(httpserver.QueryParamsDictType, "установить")))
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
	appendCodeIf(&sb, ctx.usesByteListWrap || ctx.usesProcMem, bytesio.WrapHelper(mangleType(byteListType)))
	// http2SessionOpsCode - после socketHelpers/socketHelpersTLS
	// (rech_connect_tcp) И bytesWrapHelper выше (rech_http2_read_data зовёт
	// rech_bytes_wrap) И typedefs (её typedef "HTTP2Поток" уже должен быть
	// виден) - typedef/колбэки самой сессии эмитированы РАНЬШЕ (см. их
	// отдельную точку выше, рядом с dbHandleTypedefCode).
	if ctx.usesHTTP2 {
		sb.WriteString(http2.SessionOpsCode(ctx.usesTLS,
			mangleType(http2.StreamTypeC), mangleType("?Строка"), mangleType(byteListType), mangleType("?Сокет"),
			escapeCString(rtmsg.HTTP2AcceptFailed), escapeCString(rtmsg.HTTP2HandshakeFailed), escapeCString(rtmsg.HTTP2DialFailed), escapeCString(rtmsg.HTTP2SessionClosed),
			mangleField(http2.StreamSessionFieldName), mangleField(http2.StreamIDFieldName),
			escapeCString(rtmsg.HTTP2OpenStreamFailed), escapeCString(rtmsg.HTTP2StreamNotFound),
			escapeCString(rtmsg.HTTP2RecvInvalidMax), escapeCString(rtmsg.HTTP2InvalidTrailerName)))
	}
	// http2.GRPCSessionOpsCode - ПОСЛЕ http2.SessionOpsCode выше (зовёт
	// rech_http2_* по имени, см. package doc runtime/http2/grpc.go) И после
	// nullablesCode/nullablesAfterTypedefs (обе группы уже эмитированы
	// раньше по коду функции - см. их точки выше; "?Список<Байт>"_wrap/
	// _empty оттуда).
	if ctx.usesGRPC {
		sb.WriteString(http2.GRPCSessionOpsCode(mangleType(http2.StreamTypeC), mangleType(byteListType), mangleType("?"+byteListType), mangleType("?Строка"),
			mangleField(http2.StreamSessionFieldName), mangleField(http2.StreamIDFieldName),
			escapeCString(rtmsg.HTTP2OpenStreamFailed), escapeCString(rtmsg.GRPCFramingError), escapeCString(rtmsg.GRPCCompressedUnsupported),
			escapeCString(rtmsg.GRPCStatusMissing), escapeCString(rtmsg.GRPCStatusInvalid), escapeCString(rtmsg.GRPCCompressFailed)))
	}
	// http2.GRPCSListenAndConnectCode - ПОСЛЕ http2.SessionOpsCode выше
	// (зовёт rech_http2_new_callbacks/rech_http2_start_thread по имени) И
	// после socketHelpersTLS (rech_listen_tcp/rech_tls_register/SSL_CTX_new
	// - usesGRPCS форсирует usesTLS, см. её дизайн-flag в builtins_
	// shared.go) - см. package doc runtime/http2/grpcs.go.
	if ctx.usesGRPCS {
		sb.WriteString(http2.GRPCSListenAndConnectCode(
			escapeCString(rtmsg.GRPCSCertLoadFailed), escapeCString(rtmsg.GRPCSClientHandshakeFailed), escapeCString(rtmsg.GRPCSALPNRejected)))
	}
	// После bytesWrapHelper выше - rech_bytes_from_string зовёт
	// rech_bytes_wrap напрямую.
	appendCodeIf(&sb, ctx.usesByteStringConv, bytesio.StringConvHelpers(mangleType(byteListType)))
	// После bytesWrapHelper (rech_bytes_wrap) И rechDupHelper (rech_dup)
	// выше - rech_parse_multipart (multipart.go) использует обе.
	appendCodeIf(&sb, ctx.usesMultipart, multipart.Helpers(mangleType(byteListType), mangleType(multipart.FormPartType),
		mangleType(multipart.ResultType), mangleField("имя"), mangleField("имя_файла"), mangleField("тип_содержимого"),
		mangleField("данные"), mangleMethod(multipart.ResultType, "добавить"), rtmsg.MultipartMissingBoundary))
	// После genericsCode выше И после fileHelpers (usesFileIO, эмитирована
	// намного раньше) - rech_read_bytes и соседи (internal/backend/
	// cbackend/bytesio.go) используют typedef/методы Список<Байт>
	// (зарегистрирован в prog.Generics принудительно, internal/semantic/
	// bytesio.go) и rech_file_t/rech_open_mode из fileHelpers.
	appendCodeIf(&sb, ctx.usesBytesIO, bytesio.IOHelpers(mangleType(byteListType)))
	// После bytesWrapHelper выше (rech_bytes_wrap) - rech_png_decode_file
	// (png.go)/rech_jpeg_decode_file (jpeg.go) строят через неё поле
	// "пиксели"; typedef "ДекодированноеИзображение" эмитирован раньше
	// вместе с остальными entity-typedef'ами (обычная сущность, тот же
	// путь, что и у RGBЦвет). rech_decode_image_file/rech_encode_image_file
	// (imagecodec.go) - диспетчер поверх обеих - должен идти ПОСЛЕ них.
	if ctx.usesImageCodec {
		imgType := mangleType(image.EntityName)
		widthField, heightField, pixelsField := mangleField("ширина"), mangleField("высота"), mangleField("пиксели")
		byteListC := mangleType(byteListType)
		sb.WriteString(image.PNGHelpers(imgType, widthField, heightField, pixelsField, byteListC,
			escapeCString(rtmsg.PNGDecodeFailed), escapeCString(rtmsg.PNGEncodeFailed)))
		sb.WriteString(image.JPEGHelpers(imgType, widthField, heightField, pixelsField, byteListC,
			escapeCString(rtmsg.JPEGOpenFailed), escapeCString(rtmsg.JPEGDecodeFailed),
			escapeCString(rtmsg.JPEGEncodeOpenFailed), escapeCString(rtmsg.JPEGEncodeFailed)))
		sb.WriteString(image.DispatchHelpers(imgType, escapeCString(rtmsg.ImageCodecUnknownFormat), byteListC))
	}
	// После genericsCode/bytesWrapHelper выше И после httpServerHelpersCode
	// (usesHTTPServer, эмитирована раньше - rech_recv_http_request/
	// rech_http_parse_head) - rech_http_accept_request_bytes/rech_http_
	// send_response_bytes (httpbytes.go) используют и то, и другое.
	appendCodeIf(&sb, ctx.usesHTTPBytes, httpbytes.Helpers(mangleType(httpbytes.RequestType), mangleType(httpbytes.ResponseType),
		mangleField("адрес"), mangleField("порт"), mangleField("метод"), mangleField("путь"),
		mangleField("тело"), mangleField("заголовки"), mangleField("код")))
	// После genericsCode выше - rech_split_string использует typedef/метод
	// "добавить" Список<Строка>, уже эмитированные там (splitResultType
	// зарегистрирован в prog.Generics принудительно, см. internal/semantic/
	// stringsplit.go).
	appendCodeIf(&sb, ctx.mainArgsType != "", mainArgsHelper(mangleType(ctx.mainArgsType), mangleMethod(ctx.mainArgsType, "добавить")))
	appendCodeIf(&sb, ctx.usesStringSplit, stringsplit.Helper(mangleType(stringsplit.ResultType),
		mangleMethod(stringsplit.ResultType, "добавить"), rtmsg.SplitEmptySeparator))
	// rech_header_values - "значения_заголовка" (httpheaders.go) - та же
	// generic-инстанциация "Список<Строка>", что и у "разбить"
	// (stringsplit.ResultType, идентичная строка - resolveTypeName
	// идемпотентен, см. package doc registerHeaderValuesTypeOnCall,
	// internal/semantic/httpheaders.go) - typedef/метод "добавить" уже
	// эмитированы выше, тем же genericsCode, что и для usesStringSplit.
	appendCodeIf(&sb, ctx.usesHTTPHeaderValues, httpwire.HeaderValuesHelper(mangleType(stringsplit.ResultType),
		mangleMethod(stringsplit.ResultType, "добавить")))
	// После genericsCodeAfterEntities выше (не только genericsCode) -
	// rech_regex_find_all использует typedef/метод "добавить" Список<
	// Совпадение> (элемент - сущность "Совпадение", см. её package doc
	// genericsCodeAfterEntities) И typedefs (само поле "текст"/"начало"/
	// "группы" сущности "Совпадение", regexResultType/regexGroupsType
	// зарегистрированы в prog.Generics принудительно, см. internal/semantic/
	// regex.go).
	appendCodeIf(&sb, ctx.usesRegex, regex.Helpers(mangleType(regex.ResultType), mangleType(regex.GroupsType), mangleMethod(regex.GroupsType, "добавить"),
		mangleType("Совпадение"), mangleField("текст"), mangleField("начало"),
		escapeCString(rtmsg.RegexInvalidPatternTemplate), mangleField("группы"), mangleMethod(regex.ResultType, "добавить")))
	// После typedefs - rech_time_to_date/rech_date_to_time/
	// rech_date_to_string принимают/возвращают ДатаВремя по значению.
	appendCodeIf(&sb, ctx.usesDateTime, datetime.Helpers(mangleType("ДатаВремя"), mangleField("год"), mangleField("месяц"),
		mangleField("день"), mangleField("час"), mangleField("минута"), mangleField("секунда")))
	// После typedefs (rech_T_n_data_vremya - "?ДатаВремя" - уже объявлен,
	// если usesDateParse) - rech_parse_date не зависит от datetimeHelpers
	// выше (чистый разбор строки, без календарной арифметики).
	appendCodeIf(&sb, ctx.usesDateParse, datetime.ParseHelpers(mangleType("ДатаВремя"),
		mangleField("год"), mangleField("месяц"), mangleField("день"),
		mangleField("час"), mangleField("минута"), mangleField("секунда"),
		mangleType("?ДатаВремя")))
	// После datetimeHelpers выше (rech_time_to_date_tz/rech_date_to_time_tz
	// зовут rech_time_to_date/rech_date_to_time/rech_days_from_civil/
	// rech_civil_from_days) и после typedefs ("?ДатаВремя"/"?Целое").
	appendCodeIf(&sb, ctx.usesTZ, tzdata.Helpers(mangleType("ДатаВремя"),
		mangleField("год"), mangleField("месяц"), mangleField("день"),
		mangleField("час"), mangleField("минута"), mangleField("секунда"),
		mangleType("?ДатаВремя"), mangleType("?Целое")))
	// После datetimeHelpers выше (rech_day_of_week зовёт rech_days_from_civil).
	appendCodeIf(&sb, ctx.usesDayOfWeek, datetime.DayOfWeekHelper(mangleType("ДатаВремя"),
		mangleField("год"), mangleField("месяц"), mangleField("день")))
	// После datetimeHelpers выше (rech_date_to_human зовёт rech_date_to_time/
	// rech_time_now).
	appendCodeIf(&sb, ctx.usesHumanDate, datetime.HumanDateHelper(mangleType("ДатаВремя")))
	appendCodeIf(&sb, ctx.usesColor, color.Helpers(mangleType("RGBЦвет"),
		mangleField("красный"), mangleField("зелёный"), mangleField("синий"), escapeCString(rtmsg.HexColorInvalidFormat)))
	// numbase.go - "целое_в_*"/"целое_из_*" - не зависит от typedefs, только
	// примитивы (Целое/Строка) - "из_*" зовёт rech_panic (panicPrototypes выше).
	appendCodeIf(&sb, ctx.usesNumBase, numbase.ToStringHelpers)
	appendCodeIf(&sb, ctx.usesNumBase, numbase.FromStringHelpers(
		escapeCString(rtmsg.NumBaseParseFailedFormat("целое_из_hex", "%s")),
		escapeCString(rtmsg.NumBaseParseFailedFormat("целое_из_oct", "%s")),
		escapeCString(rtmsg.NumBaseParseFailedFormat("целое_из_bin", "%s"))))
	// process.go - "спать_мс" - не зависит от typedefs, только примитив
	// (Целое).
	appendCodeIf(&sb, ctx.usesSleep, process.Helpers)
	// Не зависит от typedefs вообще (только примитивы) - здесь же просто
	// для единообразия с остальными builtin-хелперами шага 8.
	appendCodeIf(&sb, ctx.usesRandom, crypto.RandomHelpers)
	// Не зависит от typedefs - только примитивы (const char *) - после
	// panicPrototypes выше (rech_substring зовёт rech_panic).
	appendCodeIf(&sb, ctx.usesStringOps, strutil.Helpers)
	if ctx.usesStringText {
		sb.WriteString(strutil.TextHelpers())
	}
	// Не зависит от typedefs - только примитивы (const char *) - после
	// panicPrototypes выше (rech_base64url_decode зовёт rech_panic).
	appendCodeIf(&sb, ctx.usesCrypto, crypto.Helpers)
	// runtime/crypto/aes.go - "зашифровать"/"расшифровать" - ПОСЛЕ
	// crypto.Helpers выше (rech_aes_key_from_passphrase зовёт rech_sha256_*,
	// rech_aes_encrypt_hex зовёт rech_bytes_to_hex) - usesAES всегда идёт
	// вместе с usesCrypto=true (builtins_shared.go), так что порядок
	// гарантирован.
	appendCodeIf(&sb, ctx.usesAES, crypto.AESHelpers)
	// runtime/crypto/signing.go - "сгенерировать_приватный_ключ"/
	// "получить_публичный_ключ"/"подписать"/"проверить_подпись" - ПОСЛЕ
	// crypto.Helpers (rech_sign_hex зовёт rech_bytes_to_hex, rech_verify_
	// signature_hex зовёт rech_hex_decode) и ПОСЛЕ panicPrototypes (обе тянут
	// rech_panic) - оба уже гарантированы: usesSigning всегда идёт вместе с
	// usesCrypto=true.
	appendCodeIf(&sb, ctx.usesSigning, crypto.SigningHelpers)
	// После typedefs (rech_T_opcii_kuki/rech_T_n_stroka уже объявлены) и
	// после strConcatHelper выше (rech_cookie_string зовёт rech_str_concat).
	appendCodeIf(&sb, ctx.usesCookies, cookies.Helpers(mangleType("?Строка"), mangleType("ОпцииКуки"),
		mangleField("путь"), mangleField("домен"), mangleField("максимальный_возраст"),
		mangleField("только_http"), mangleField("безопасный"), mangleField("тот_же_сайт")))
	// После typedefs (rech_T_n_stroka - "?Строка" - уже объявлен, тем же
	// принципом, что и usesCookies выше).
	appendCodeIf(&sb, ctx.usesEnv, process.EnvHelpers(mangleType("?Строка")))
	// Не зависит от typedefs (обычная "Строка", не nullable) - можно
	// эмитировать в любом месте после panicPrototypes, тем же принципом, что и
	// usesHTTPHeaders ниже.
	appendCodeIf(&sb, ctx.usesOptions, options.Helpers(escapeCString(prog.OptionsYAML)))
	// Не зависит от typedefs (обычная "Строка", не nullable) - тем же
	// принципом, что и usesEnv/usesOptions выше; требует panicPrototypes (уже
	// эмитирован - usesCwd подразумевает usesPanic, см. package doc
	// builtinFlags.usesCwd).
	appendCodeIf(&sb, ctx.usesCwd, process.CwdHelper(escapeCString(rtmsg.CwdFailed)))
	// После typedefs (rech_T_n_stroka уже объявлен) - тем же принципом,
	// что и usesEnv/usesCookies выше.
	appendCodeIf(&sb, ctx.usesHTTPHeaders, httpwire.HeaderValueHelper(mangleType("?Строка")))
	// Не зависит от typedefs (только sig_atomic_t/int) - можно эмитировать
	// в любом месте после panicPrototypes, здесь - просто рядом с остальными
	// небольшими самодостаточными хелперами.
	appendCodeIf(&sb, ctx.usesShutdownFlag, process.ShutdownFlagCode)
	appendCodeIf(&sb, ctx.usesSignals, signals.RuntimeCode(escapeCString(rtmsg.SignalUncatchable), escapeCString(rtmsg.SignalSendFailed)))
	appendCodeIf(&sb, ctx.usesSubprocess, subprocess.RuntimeCode(mangleType("Процесс"), mangleType(stringsplit.ResultType), mangleField("pid"),
		mangleType("РезультатКоманды"), mangleField("вывод"), mangleField("ошибка"), mangleField("код_возврата"),
		escapeCString(rtmsg.ProcessSpawnFailed), escapeCString(rtmsg.SubprocessWaitFailed), escapeCString(rtmsg.CommandRunFailed)))
	appendCodeIf(&sb, ctx.usesProcMem, procmem.RuntimeCode(mangleType("Процесс"), mangleField("pid"), mangleType(byteListType),
		escapeCString(rtmsg.ProcAttachFailed), escapeCString(rtmsg.ProcDetachFailed),
		escapeCString(rtmsg.ProcMemReadFailed), escapeCString(rtmsg.ProcMemWriteFailed)))
	appendCodeIf(&sb, ctx.usesProcMemFind, procmem.FindHelper(mangleType(procmem.IntListType), mangleType("Процесс"), mangleType(byteListType),
		escapeCString(rtmsg.ProcMemPatternEmpty), mangleField("pid"), mangleMethod(procmem.IntListType, "добавить")))
	if ctx.usesProcRegs {
		mangledFields := make([]string, len(procregs.RegisterFieldNames))
		for i, name := range procregs.RegisterFieldNames {
			mangledFields[i] = mangleField(name)
		}
		sb.WriteString(procregs.RuntimeCode(mangleType("Регистры"), mangleType("Процесс"), mangleField("pid"), mangledFields,
			escapeCString(rtmsg.ProcRegsReadFailed), escapeCString(rtmsg.ProcRegsWriteFailed),
			escapeCString(rtmsg.ProcBreakpointSetFailed), escapeCString(rtmsg.ProcBreakpointRemoveFailed),
			escapeCString(rtmsg.ProcResumeFailed)))
	}
	appendCodeIf(&sb, ctx.usesConsole, console.RuntimeCode())
	appendCodeIf(&sb, ctx.usesKeyboard, keyboard.RuntimeCode())
	appendCodeIf(&sb, ctx.usesNonBlockingRead, console.NonBlockingReadRuntimeCode())
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
	// Переменные модуля - перед прототипами: к этому месту объявлены все
	// типы, которые могут быть у переменной (тем же требованием, что у
	// параметров в прототипах).
	appendCodeIf(&sb, globalDecl != "", globalDecl)
	appendCodeIf(&sb, protos.Len() > 0, protos.String())

	// vtables - ссылается на &rech_M_... (нужны protos выше).
	appendCodeIf(&sb, vtables.Len() > 0, vtables.String())
	appendCodeIf(&sb, staticInit != "", staticInit)
	appendCodeIf(&sb, globalInit != "", globalInit)
	sb.WriteString(libraryInitCode(ctx))
	// После ctors - функции извлечения ("разобрать JSON"/"разобрать
	// XML") конструируют целевые сущности через уже сгенерированный
	// rech_new_<Тип> (mangleCtor) и выбрасывают "ОшибкаРазбора" через
	// rech_struct_throw/rech_throw (exceptionRuntimeCode выше) - общий
	// для обоих форматов, эмитится один раз.
	appendCodeIf(&sb, len(prog.ParseTargets) > 0 || len(prog.XMLParseTargets) > 0 || len(prog.YAMLParseTargets) > 0 || len(prog.ProtobufParseTargets) > 0, structParseThrowHelperCode())
	appendCodeIf(&sb, ctx.usesStateFlags, requireStateHelperCode())
	appendCodeIf(&sb, ctx.usesStateFlags, revokeStatesHelperCode())
	sb.WriteString(panicRuntimeCode())

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
	// rech_panic через reflectCheckedUnboxExprCode - panicPrototypes уже
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
		sb.WriteString(nullableReflectBoxHelperProtos(prog, false))
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
		sb.WriteString(nullableReflectBoxHelpers(ctx, prog, false))
	} else if prog.UsesReflection {
		// "(Значение) ?Целое" без целей рефлексии - хелперы только для
		// "?примитив": дескрипторы сущностей тут не генерируются.
		sb.WriteString(nullableReflectBoxHelperProtos(prog, true))
		sb.WriteString(nullableReflectBoxHelpers(ctx, prog, true))
	}
	if prog.UsesReflection {
		sb.WriteString(anyValueHelpers(ctx))
	}

	// concurrency.ThreadPoolCode (runtime/concurrency/threadpool.go) - ДО
	// concurrency.ActiveTasksCode/taskCode ниже: rech_launch_... (concurrency.go,
	// ensureTaskWrapper) зовёт rech_pool_submit текстом, объявление должно
	// предшествовать использованию - тот же принцип, что и у
	// concurrency.ActiveTasksCode ниже.
	appendCodeIf(&sb, ctx.usesThreads, concurrency.ThreadPoolCode(prog.PoolShrinkEnabled, prog.PoolIdleTimeoutMs))
	// concurrency.ActiveTasksCode (runtime/concurrency/active_tasks.go) - ДО
	// taskCode ниже: точка входа потока (concurrency.go, ensureTaskWrapper)
	// зовёт rech_active_tasks_dec текстом, объявление должно предшествовать
	// использованию.
	appendCodeIf(&sb, ctx.usesThreads || ctx.usesActiveTaskWait, concurrency.ActiveTasksCode)
	// taskCode - обёртки "запустить"/"дождаться" (internal/backend/cbackend/
	// concurrency.go) - ссылаются на &rech_<имя> обёрнутых функций (нужны
	// protos выше) и на rech_throw (нужен exceptionRuntimeCode выше), сама
	// заполняется как побочный эффект генерации body ниже, поэтому текст
	// уже готов к этому моменту, хотя размещается в файле раньше него.
	appendCodeIf(&sb, ctx.taskCode.Len() > 0, ctx.taskCode.String())

	// callableCode - обёртки "&имя"/тела лямбд (internal/backend/cbackend/
	// callable.go) - тот же принцип, что и taskCode выше: заполняется как
	// побочный эффект генерации body, размещается в файле раньше него
	// (ссылается на &rech_<имя> обёрнутых функций - нужны protos выше).
	appendCodeIf(&sb, ctx.callableCode.Len() > 0, ctx.callableCode.String())

	// ctors - ПОСЛЕ reflectRuntimeCode: тело конструктора может ссылаться на
	// дескриптор рефлексии (&rech_type_meta_X, "тип X"/"значение как X"
	// внутри "конструктор(...)"), а reflectRuntimeCode форвард-объявляет
	// его только здесь, не раньше. rech_new_<Тип> (сам конструктор)
	// прототипирован уже в protos выше - другой код зовёт его независимо
	// от того, где эмитировано ЕГО ТЕЛО.
	appendCodeIf(&sb, ctors.Len() > 0, ctors.String())

	sb.WriteString(body.String())
	appendCodeIf(&sb, ctx.service, process.ServiceMainCode)
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
