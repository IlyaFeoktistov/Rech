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
	// usesIntDiv - нужен ли rech_int_div (см. generateIRArithmetic).
	usesIntDiv bool
	// usesShow - нужны ли функции показа (rech_show_...) - генерируются
	// только если "показать" реально встретилось в программе.
	usesShow bool
	// usesCast - нужны ли рантайм-хелперы явных кастов (castHelpers) - только
	// для приведений с участием "Строка" (разбор/форматирование); прямые
	// числовые приведения (Целое/Дробное/Действительное/Символ/Байт между
	// собой) - обычный C-каст, без хелперов.
	usesCast bool
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
	// externCNames - Rech-facing имя (Alias) внешней функции -> настоящий
	// C-символ (CName), из prog.ExternFuncs: CallExpr.Callee несёт Alias,
	// вызывается по CName, не по mangleName(Alias) (тот не имеет отношения
	// к настоящему символу).
	externCNames map[string]string
	thisEntity   string

	// interfaces - интерфейсы программы по имени (см.
	// internal/backend/cbackend/vtable.go).
	interfaces map[string]*ir.Interface
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

	// funcRefTrampolines/lambdaFuncsEmitted/callableCode - см.
	// internal/backend/cbackend/callable.go (ensureFuncRefTrampoline/
	// ensureLambdaFunc) - тот же принцип "накопить один раз, вставить в
	// файл в конце", что и taskCode выше.
	funcRefTrampolines map[string]bool
	lambdaFuncsEmitted map[string]bool
	callableCode       strings.Builder
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
// "структура" (по значению). "Не найдено в ctx.entities" - generic-
// инстанциация (Список<T> и т. п., они регистрируются отдельно, в
// prog.Generics, не в prog.Entities) - продолжает существующее представление
// "по значению" (обёртка-структура с полем-указателем на буфер), как и до
// появления "структура"/ссылочной сущности.
func (ctx *irGenCtx) entityIsRef(name string) bool {
	ent, ok := ctx.entities[name]
	return ok && !ent.IsStruct
}

// GenerateFromIR возвращает исходник на C, соответствующий программе prog -
// обычная программа, требует функцию "главная" (см. generateSource).
func GenerateFromIR(prog *ir.Program) (string, error) {
	return generateSource(prog, false)
}

// GenerateLibFromIR - как GenerateFromIR, но для разделяемой библиотеки
// (`rech build --lib`): "главная" не обязательна (библиотека не
// исполняется как программа), а вторым результатом возвращается текст
// C-заголовка (.h) с прототипами
// объявлений, помеченных "экспортировать" - см. header.go.
func GenerateLibFromIR(prog *ir.Program) (cSource string, header string, err error) {
	cSource, err = generateSource(prog, true)
	if err != nil {
		return "", "", err
	}
	header, err = generateHeader(prog)
	if err != nil {
		return "", "", err
	}
	return cSource, header, nil
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
	usesGC := false
	for _, ent := range prog.Entities {
		entities[ent.Name] = ent
		if !ent.IsStruct {
			usesGC = true
		}
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
	ctx := &irGenCtx{
		entities: entities, externCNames: externCNames, usesGC: usesGC, libMode: libMode,
		interfaces: interfaces, dispatchParticipants: participants, dispatchSlots: slots,
		// usesVtable - хотя бы одна сущность участвует в диспетчеризации
		// (participants), не "есть хотя бы один слот" (slots) - сущность
		// без единого метода (например, системная "Исключение" и её
		// маркерные наследники без переопределений) всё равно получает
		// поле vt в своей C-структуре (см. generateIREntityStruct), значит
		// rech_vtable/rech_vtable_header обязаны быть объявлены, даже если
		// slots в итоге пуст.
		usesVtable: len(participants) > 0,
		excTypeIDs: excTypeIDs, excTypeNames: excTypeNames, excIsaTable: excIsaTable,
		launchedFuncs: map[string]bool{},
	}
	ctx.irFuncs = map[string]*ir.Function{}
	for _, fn := range prog.Functions {
		ctx.irFuncs[fn.Name] = fn
	}

	var typedefs strings.Builder
	if ctx.usesVtable {
		typedefs.WriteString(vtableTypedefs(len(slots)))
	}
	for _, ent := range prog.Entities {
		typedefs.WriteString(generateIREntityStruct(ctx, ent))
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

	// genericsCode - typedef struct + функции для встроенных generic-
	// коллекций, использованных в программе - переиспользует
	// listCollectionCCode/stackCollectionCCode/queueCollectionCCode
	// (растущий массив) - IR не хранит собственных сигнатур методов для
	// них, MethodCallExpr уже несёт всё нужное для вызова, эта функция
	// только про C-текст самого типа.
	var genericsCode strings.Builder
	usesGenerics := len(prog.Generics) > 0
	for _, g := range prog.Generics {
		elemC := fromIRType(g.ElemType).cType(ctx)
		switch g.Template {
		case "Список":
			genericsCode.WriteString(listCollectionCCode(g.Name, elemC))
		case "Стопка":
			genericsCode.WriteString(stackCollectionCCode(g.Name, elemC))
		case "Очередь":
			genericsCode.WriteString(queueCollectionCCode(g.Name, elemC))
		default:
			return "", fmt.Errorf("неизвестный generic-шаблон %q", g.Template)
		}
	}

	// callableTypedefs - C-структуры {указатель на функцию, указатель на
	// окружение замыкания} для каждой сигнатуры "Вызываемый", встретившейся
	// в программе (prog.Callables - уже полный список, собранный
	// internal/semantic, без ленивого "по первому использованию", в
	// отличие от funcRefTrampolines/lambdaFuncsEmitted ниже) - см.
	// internal/backend/cbackend/callable.go.
	var callableTypedefs strings.Builder
	for _, sig := range prog.Callables {
		callableTypedefs.WriteString(callableStructTypedef(ctx, sig))
	}

	// showFuncs - функции показа ("показать") для каждой сущности и
	// generic-коллекции программы - заполняется НИЖЕ, после генерации тел
	// (body), а не здесь - см. package doc там.
	var showFuncs strings.Builder

	var protos strings.Builder
	for _, ext := range prog.ExternFuncs {
		protos.WriteString(externPrototypeOfIR(ctx, ext))
	}
	for _, ent := range prog.Entities {
		for _, m := range ent.Methods {
			fmt.Fprintf(&protos, "%s %s(%s);\n", cReturnTypeFor(ctx, m.HasReturn, m.ReturnType), mangleMethod(ent.Name, m.Name), cMethodParamListIR(ctx, ent.Name, m.Params))
		}
	}
	hasEntryPoint := false
	for _, fn := range prog.Functions {
		if fn.Name == entryPointName {
			hasEntryPoint = true
			continue
		}
		fmt.Fprintf(&protos, "%s%s %s(%s);\n", exportAttr(ctx, fn.Exported), cReturnTypeFor(ctx, fn.HasReturn, fn.ReturnType), mangleName(fn.Name), cParamListIR(ctx, fn.Params))
	}
	if !libMode && !hasEntryPoint {
		return "", fmt.Errorf("в программе не найдена функция %q - она служит точкой входа", entryPointName)
	}

	var body strings.Builder
	for _, ent := range prog.Entities {
		for _, m := range ent.Methods {
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
		for _, ent := range prog.Entities {
			showFuncs.WriteString(generateIREntityShowFunc(ctx, ent))
		}
		for _, g := range prog.Generics {
			showFuncs.WriteString(generateIRGenericShowFunc(ctx, g))
		}
	}

	var sb strings.Builder
	sb.WriteString("// Автоматически сгенерировано компилятором rech. Не редактировать руками.\n")
	sb.WriteString("#include <stdio.h>\n")
	sb.WriteString("#include <stdbool.h>\n")
	sb.WriteString("#include <stdint.h>\n")
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
	if ctx.usesThreads {
		sb.WriteString("#define GC_THREADS\n")
	}
	if ctx.usesGC || ctx.usesThreads {
		sb.WriteString("#include <gc.h>\n")
	}
	if ctx.usesConcat || ctx.usesStrCmp || ctx.usesConsoleInput || ctx.usesFileIO || ctx.usesSockets || ctx.usesCast || ctx.usesMemcpy {
		sb.WriteString("#include <string.h>\n")
	}
	if ctx.usesConcat || usesGenerics || ctx.usesPanic || ctx.usesConsoleInput || ctx.usesFileIO || ctx.usesCast || ctx.usesExceptions {
		sb.WriteString("#include <stdlib.h>\n")
	}
	if ctx.usesMath {
		sb.WriteString("#include <math.h>\n")
	}
	if ctx.usesExceptions {
		sb.WriteString("#include <setjmp.h>\n")
	}
	if ctx.usesSockets {
		sb.WriteString("#ifdef _WIN32\n#include <winsock2.h>\n#include <ws2tcpip.h>\n#else\n#include <sys/socket.h>\n#include <netinet/in.h>\n#include <arpa/inet.h>\n#include <netdb.h>\n#include <unistd.h>\n#endif\n")
	}
	// Потоки ("запустить"/"дождаться") - на Windows CreateThread/
	// WaitForSingleObject/CloseHandle уже объявлены через windows.h выше,
	// отдельного заголовка не нужно; на POSIX - pthread.h.
	if ctx.usesThreads {
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
	if ctx.usesConcat {
		sb.WriteString(strConcatHelper)
		sb.WriteString("\n")
	}
	// ctx.usesCast - castHelpers (ниже) - один общий блок текста на все
	// явные касты (int/float/char <-> Строка, см. package doc cast.go), не
	// отдельная функция на каждый - несколько из них (разбор строки в
	// число/символ) сами вызывают rech_panic при ошибке разбора, даже если
	// в программе встретился только каст, который панике не подвержен
	// (например, число -> Строка) - тот же принцип, что и у usesGenerics
	// здесь: panicHelper должен быть виден целиком, раз в файле есть хотя
	// бы один потенциальный вызывающий его блок текста.
	if usesGenerics || ctx.usesPanic || ctx.usesCast {
		sb.WriteString(panicHelper)
		sb.WriteString("\n")
	}
	if ctx.usesArgs {
		sb.WriteString(argFuncHelper)
		sb.WriteString("\n")
	}
	if ctx.usesCast {
		sb.WriteString(castHelpers)
		sb.WriteString("\n")
	}
	if ctx.usesIntDiv {
		sb.WriteString(intDivHelper)
		sb.WriteString("\n")
	}
	if ctx.usesConsoleInput {
		sb.WriteString(readHelpers)
		sb.WriteString("\n")
	}
	if ctx.usesFileIO {
		sb.WriteString(fileHelpers)
		sb.WriteString("\n")
	}
	if ctx.usesSockets {
		sb.WriteString(socketHelpers)
		sb.WriteString("\n")
	}
	if typedefs.Len() > 0 {
		sb.WriteString(typedefs.String())
	}
	if ctx.usesExceptions {
		// После typedefs - использует rech_vtable_header/rech_T_Исключение,
		// оба уже должны быть объявлены (см. exceptions.go).
		sb.WriteString(exceptionRuntimeCode(ctx))
		sb.WriteString("\n")
	}
	if genericsCode.Len() > 0 {
		sb.WriteString(genericsCode.String())
	}
	if callableTypedefs.Len() > 0 {
		sb.WriteString(callableTypedefs.String())
	}
	if ctx.usesShow && showFuncs.Len() > 0 {
		sb.WriteString(showFuncs.String())
	}
	if ctx.usesHTTP {
		// После typedefs - rech_http_request принимает/возвращает Запрос/
		// Ответ по значению, их typedef должен быть уже виден.
		sb.WriteString(httpHelpers)
		sb.WriteString("\n")
	}
	if protos.Len() > 0 {
		sb.WriteString(protos.String())
		sb.WriteString("\n")
	}
	// vtables (ссылается на &rech_M_... - нужны protos выше) и ctors
	// (ссылается на &rech_VT_... у участвующих в диспетчеризации сущностей
	// - нужны vtables) - строго в этом порядке, оба перед body (который их
	// использует).
	if vtables.Len() > 0 {
		sb.WriteString(vtables.String())
	}
	if ctors.Len() > 0 {
		sb.WriteString(ctors.String())
	}
	// taskCode - обёртки "запустить"/"дождаться" (internal/backend/cbackend/
	// concurrency.go) - ссылаются на &rech_<имя> обёрнутых функций (нужны
	// protos выше) и на rech_throw (нужен exceptionRuntimeCode выше), сама
	// заполняется как побочный эффект генерации body ниже, поэтому текст
	// уже готов к этому моменту, хотя размещается в файле раньше него.
	if ctx.taskCode.Len() > 0 {
		sb.WriteString(ctx.taskCode.String())
	}
	// callableCode - обёртки "&имя"/тела лямбд (internal/backend/cbackend/
	// callable.go) - тот же принцип, что и taskCode выше: заполняется как
	// побочный эффект генерации body, размещается в файле раньше него
	// (ссылается на &rech_<имя> обёрнутых функций - нужны protos выше).
	if ctx.callableCode.Len() > 0 {
		sb.WriteString(ctx.callableCode.String())
	}
	sb.WriteString(body.String())
	return sb.String(), nil
}

// generateIREntityStruct - C typedef struct для ent, как
// C typedef struct для ent, по уже собранным internal/ir.Field.
func generateIREntityStruct(ctx *irGenCtx, ent *ir.Entity) string {
	var sb strings.Builder
	sb.WriteString("typedef struct {\n")
	participates := ctx.dispatchParticipants[ent.Name]
	if participates {
		// "vt" - первым полем и всегда (не только у собственно
		// объявляющей сущности, но и у каждого её потомка) - общий
		// начальный кусок структуры делает приведение к vtableHeaderType
		// определённым для чтения (см. package doc vtable.go).
		fmt.Fprintf(&sb, "    const %s *vt;\n", vtableType)
	}
	if len(ent.Fields) == 0 && !participates {
		sb.WriteString("    char _rech_empty;\n")
	}
	for _, f := range ent.Fields {
		fmt.Fprintf(&sb, "    %s %s;\n", fromIRType(f.Type).cType(ctx), mangleField(f.Name))
	}
	fmt.Fprintf(&sb, "} %s;\n\n", mangleType(ent.Name))
	return sb.String()
}

// generateIREntityCtor - конструктор ссылочной сущности ("сущность"/
// "класс", ctx.entityIsRef(ent.Name) == true): выделяет память под структуру
// через GC_MALLOC (Boehm GC - решение пользователя, шаг 12, "Указатели/
// ссылки... сущность по умолчанию") и возвращает указатель. GC_MALLOC (не
// GC_MALLOC_ATOMIC) - поля сущности сами могут быть указателями на другие
// GC-объекты (вложенная сущность), нужно сканирование; она же зануляет
// выделенную память (документированная гарантия Boehm GC, в отличие от
// обычного malloc) - опора для explicit-конструктора ниже, чьё тело не
// обязано присвоить каждое поле явно.
//
// Без ent.Ctor - заполняет поля позиционно (Args уже позиционны по полю,
// см. package doc ir.NewExpr) прямо здесь, одной функцией. С ent.Ctor -
// выделение памяти отделено от самого тела конструктора
// (generateIREntityCtorInit) - см. её package doc про то, зачем именно.
//
// В отличие от "структура" (generateIRNewExpr - compound literal прямо в
// месте вызова, структуры не участвуют в явных конструкторах - см.
// ast.EntityDecl.Ctor), у ссылочной сущности ровно одна такая функция на
// тип, не код на каждый "новый" - тот же принцип, что и у show-функций.
func generateIREntityCtor(ctx *irGenCtx, ent *ir.Entity) (string, error) {
	if ent.Ctor != nil {
		initCode, err := generateIREntityCtorInit(ctx, ent)
		if err != nil {
			return "", err
		}
		return initCode + generateIREntityCtorNewWithInit(ctx, ent), nil
	}
	var sb strings.Builder
	cType := mangleType(ent.Name)
	params := make([]string, len(ent.Fields))
	for i, f := range ent.Fields {
		params[i] = fmt.Sprintf("%s %s", fromIRType(f.Type).cType(ctx), mangleVar(f.Name))
	}
	paramList := "void"
	if len(params) > 0 {
		paramList = strings.Join(params, ", ")
	}
	fmt.Fprintf(&sb, "static %s *%s(%s) {\n", cType, mangleCtor(ent.Name), paramList)
	fmt.Fprintf(&sb, "    %s *p = GC_MALLOC(sizeof(%s));\n", cType, cType)
	if ctx.dispatchParticipants[ent.Name] {
		fmt.Fprintf(&sb, "    p->vt = &%s;\n", vtableInstanceName(ent.Name))
	}
	for _, f := range ent.Fields {
		fmt.Fprintf(&sb, "    p->%s = %s;\n", mangleField(f.Name), mangleVar(f.Name))
	}
	sb.WriteString("    return p;\n")
	sb.WriteString("}\n\n")
	return sb.String(), nil
}

// generateIREntityCtorInit - тело явного конструктора ent.Ctor как отдельная
// C-функция, с тем же "Тип *this_" первым скрытым параметром, что и у
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
	// зануляет память, этого достаточно для полей без default, но не для
	// полей с ним. Тело конструктора (ниже) может переопределить любое из
	// них через "этот.поле = ...".
	for _, f := range ent.Fields {
		if f.Default == nil {
			continue
		}
		assign := &ir.FieldAssign{Receiver: &ir.ThisExpr{Entity: ent.Name}, Field: f.Name, Value: f.Default}
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
	var sb strings.Builder
	cType := mangleType(ent.Name)
	fmt.Fprintf(&sb, "static %s *%s(%s) {\n", cType, mangleCtor(ent.Name), cParamListIR(ctx, ent.Ctor.Params))
	fmt.Fprintf(&sb, "    %s *p = GC_MALLOC(sizeof(%s));\n", cType, cType)
	if ctx.dispatchParticipants[ent.Name] {
		fmt.Fprintf(&sb, "    p->vt = &%s;\n", vtableInstanceName(ent.Name))
	}
	initArgs := make([]string, len(ent.Ctor.Params)+1)
	initArgs[0] = "p"
	for i, p := range ent.Ctor.Params {
		initArgs[i+1] = mangleVar(p.Name)
	}
	fmt.Fprintf(&sb, "    %s(%s);\n", mangleCtorInit(ent.Name), strings.Join(initArgs, ", "))
	sb.WriteString("    return p;\n")
	sb.WriteString("}\n\n")
	return sb.String()
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
func externPrototypeOfIR(ctx *irGenCtx, ext *ir.ExternFuncDecl) string {
	cReturnType := "void"
	if ext.HasReturn {
		cReturnType = fromIRType(ext.ReturnType).cType(ctx)
	}
	types := make([]string, len(ext.Params))
	for i, p := range ext.Params {
		types[i] = fromIRType(p).cType(ctx)
	}
	return fmt.Sprintf("extern %s %s(%s);\n", cReturnType, ext.CName, strings.Join(types, ", "))
}

// cMethodParamListIR - как cParamListIR, но с обязательным скрытым первым
// параметром "Тип *this_" (получатель по указателю, как и в AST-пути) -
// у метода он есть всегда, поэтому в отличие от cParamListIR/
// combineParamList не нужен отдельный случай "нет параметров -> void".
func cMethodParamListIR(ctx *irGenCtx, entityName string, params []ir.Param) string {
	parts := make([]string, len(params)+1)
	parts[0] = fmt.Sprintf("%s *this_", mangleType(entityName))
	for i, p := range params {
		parts[i+1] = fmt.Sprintf("%s %s", fromIRType(p.Type).cType(ctx), mangleVar(p.Name))
	}
	return strings.Join(parts, ", ")
}

func generateIRFunction(ctx *irGenCtx, fn *ir.Function) (string, error) {
	var sb strings.Builder
	fmt.Fprintf(&sb, "// %s\n", fn.Name)
	cName := mangleName(fn.Name)
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
	fmt.Fprintf(&sb, "%s %s(%s) {\n", cReturnTypeFor(ctx, m.HasReturn, m.ReturnType), mangleMethod(entityName, m.Name), cMethodParamListIR(ctx, entityName, m.Params))
	fmt.Fprintf(&sb, "    %s", tracePush(entityName+"."+m.Name))
	ctx.thisEntity = entityName
	defer func() { ctx.thisEntity = "" }()
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

func generateIRStmt(sb *strings.Builder, ctx *irGenCtx, s ir.Stmt, depth int) error {
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
		code, typ, err := generateIRExpr(ctx, s.Value)
		if err != nil {
			return err
		}
		fmt.Fprintf(sb, "%s%s %s = %s;\n", ind, typ.cType(ctx), mangleVar(s.Name), code)
		return nil

	case *ir.Assign:
		code, _, err := generateIRExpr(ctx, s.Value)
		if err != nil {
			return err
		}
		fmt.Fprintf(sb, "%s%s = %s;\n", ind, varRef(ctx, s.Name), code)
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

	case *ir.WhileStmt:
		cond, _, err := generateIRExpr(ctx, s.Cond)
		if err != nil {
			return err
		}
		fmt.Fprintf(sb, "%swhile (%s) {\n", ind, cond)
		for _, st := range s.Body {
			if err := generateIRStmt(sb, ctx, st, depth+1); err != nil {
				return err
			}
		}
		fmt.Fprintf(sb, "%s}\n", ind)
		return nil

	case *ir.ReturnStmt:
		if s.Value == nil {
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
		op := "."
		if ctx.receiverIsPointer(s.Receiver) {
			op = "->"
		}
		fmt.Fprintf(sb, "%s(%s)%s%s = %s;\n", ind, receiverCode, op, mangleField(s.Field), code)
		return nil

	case *ir.BecomeStmt:
		// Шаг 4.1, упрощённая версия: состояние уже проверено семантическим
		// анализом как объявленное - typestate чисто компилятивный на этом
		// шаге, "становится" не эмитит C-код, как и в AST-пути.
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
		fmt.Fprintf(sb, "%sif (!(%s)) { rech_panic(\"%s\"); }\n", ind, cond, escapeCString(rtmsg.AssertFailed(s.Line)))
		return nil

	case *ir.ExprStmt:
		var call ir.Expr = s.Call
		switch {
		case s.MethodCall != nil:
			call = s.MethodCall
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
		for _, st := range s.Body {
			if err := generateIRStmt(sb, ctx, st, depth+1); err != nil {
				return err
			}
		}
		fmt.Fprintf(sb, "%s}\n", ind)
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
			for _, st := range s.Body {
				if err := generateIRStmt(sb, ctx, st, depth+1); err != nil {
					return err
				}
			}
			fmt.Fprintf(sb, "%s}\n", ind)
			return nil
		}
		varC := mangleVar(s.Var)
		idxC := "rech_fe_i_" + varC
		fmt.Fprintf(sb, "%sfor (long long %s = 0; %s < %s.len; %s++) {\n", ind, idxC, idxC, collCode, idxC)
		fmt.Fprintf(sb, "%s%s %s = %s.data[%s];\n", indent(depth+1), fromIRType(s.ElemType).cType(ctx), varC, collCode, idxC)
		for _, st := range s.Body {
			if err := generateIRStmt(sb, ctx, st, depth+1); err != nil {
				return err
			}
		}
		fmt.Fprintf(sb, "%s}\n", ind)
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

	case *ir.CallExpr:
		argCodes := make([]string, len(e.Args))
		for i, a := range e.Args {
			code, _, err := generateIRExpr(ctx, a)
			if err != nil {
				return "", typeUnknown, err
			}
			argCodes[i] = code
		}
		// Порядок проверки: builtin (inline C-выражение) -> внешняя функция
		// (настоящий C-символ, не mangleName) -> обычная функция программы.
		if code, ok := builtinCallCode(&ctx.builtinFlags, e.Callee, argCodes); ok {
			return code, fromIRType(e.Type), nil
		}
		callee := mangleName(e.Callee)
		if cName, ok := ctx.externCNames[e.Callee]; ok {
			callee = cName
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
		// общее исключение из копирования, см. ROADMAP.md, шаг 12). Но
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

	case *ir.CharLiteral:
		return fmt.Sprintf("%d", e.Value), typeChar, nil

	case *ir.CastExpr:
		return generateIRCastExpr(ctx, e)

	case *ir.IsExpr:
		return generateIRIsExpr(ctx, e)

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
		return fmt.Sprintf("(-%s)", code), typ, nil
	case "не":
		return fmt.Sprintf("(!%s)", code), typeBool, nil
	case "~":
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

	case "-", "*", "/":
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
		return fmt.Sprintf("(%s %s %s)", lc, e.Op, rc), typeInt, nil

	default:
		return "", typeUnknown, fmt.Errorf("генерация C из IR: неизвестная операция %q", e.Op)
	}
}

// generateIRArithmetic - "+" (нестроковый)/"-"/"*"/"/": приведение
// операнда к double при смешении Целое/Число - та же промоция, что и в
// generateIRArithmetic.
func generateIRArithmetic(ctx *irGenCtx, op string, lc string, lt ir.Type, rc string, rt ir.Type) (string, valueType, error) {
	if lt == ir.TypeFloat || rt == ir.TypeFloat {
		if lt == ir.TypeInt {
			lc = fmt.Sprintf("((double)%s)", lc)
		}
		if rt == ir.TypeInt {
			rc = fmt.Sprintf("((double)%s)", rc)
		}
		return fmt.Sprintf("(%s %s %s)", lc, op, rc), typeFloat, nil
	}
	if op == "/" {
		ctx.usesPanic = true
		ctx.usesIntDiv = true
		return fmt.Sprintf("rech_int_div(%s, %s)", lc, rc), typeInt, nil
	}
	return fmt.Sprintf("(%s %s %s)", lc, op, rc), typeInt, nil
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
		return fmt.Sprintf("%s(%s)", mangleCtor(e.Entity), strings.Join(argCodes, ", ")), entityType(e.Entity), nil
	}
	parts := make([]string, len(argCodes))
	for i, code := range argCodes {
		parts[i] = fmt.Sprintf(".%s = %s", mangleField(ent.Fields[i].Name), code)
	}
	initList := strings.Join(parts, ", ")
	if initList == "" {
		// Сущность без полей - зануляем техническую заглушку структуры
		// (generateIREntityStruct), а не полагаемся на "{}" (расширение
		// GNU, не входящее в стандартный C99) - как и в AST-пути.
		initList = "0"
	}
	return fmt.Sprintf("(%s){%s}", mangleType(e.Entity), initList), entityType(e.Entity), nil
}

// generateIRMethodCallExpr генерирует вызов метода - получатель по указателю
// (первый скрытый параметр this_): мутация поля через "этот.поле = ..."
// внутри метода видна вызывающему коду после возврата. Получатель ссылочной
// сущности - уже указатель сам по себе (передаётся как есть, receiverIsPointer);
// получатель "структура" (или получатель, порождённый ThisExpr) - значение,
// нужен "&", чтобы получить указатель для this_ - тот же принцип, что и у
// выбора "."/"->" в FieldAccess/FieldAssign.
func generateIRMethodCallExpr(ctx *irGenCtx, e *ir.MethodCallExpr) (string, valueType, error) {
	receiverCode, _, err := generateIRExpr(ctx, e.Receiver)
	if err != nil {
		return "", typeUnknown, err
	}
	argCodes := make([]string, len(e.Args)+1)
	if ctx.receiverIsPointer(e.Receiver) {
		argCodes[0] = fmt.Sprintf("(%s)", receiverCode)
	} else {
		argCodes[0] = fmt.Sprintf("&(%s)", receiverCode)
	}
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
	return fmt.Sprintf("%s(%s)", mangleMethod(e.Entity, e.Method), strings.Join(argCodes, ", ")), fromIRType(e.Type), nil
}

// fromIRType переводит ir.Type (backend-независимый, internal/ir) в
// valueType (C-специфичный, этот пакет) - единственное место, где IR-путь
// знает про valueType вообще, остальной generateIR* код работает через
// ir.Type/ir.TypeOf.
// irKindToValueType - ir.Kind -> valueType для всех примитивов (не
// KindEntity - у него есть переменная часть, имя сущности, см. fromIRType).
var irKindToValueType = map[ir.TypeKind]valueType{
	ir.KindByte:    typeByte,
	ir.KindInt:     typeInt,
	ir.KindFloat32: typeFloat32,
	ir.KindFloat:   typeFloat,
	ir.KindChar:    typeChar,
	ir.KindString:  typeString,
	ir.KindBool:    typeBool,
	ir.KindFile:    typeFile,
	ir.KindSocket:  typeSocket,
}

func fromIRType(t ir.Type) valueType {
	if t.Kind == ir.KindEntity {
		return entityType(t.Entity)
	}
	if t.Kind == ir.KindTask {
		return taskType(t.Entity)
	}
	if t.Kind == ir.KindCallable {
		return callableType(t.Entity)
	}
	if t.Kind == ir.KindExternEntity {
		return externEntityType(t.Entity)
	}
	if vt, ok := irKindToValueType[t.Kind]; ok {
		return vt
	}
	return typeUnknown
}
