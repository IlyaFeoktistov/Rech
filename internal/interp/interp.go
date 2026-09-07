// Package interp - Interpreter backend поверх internal/ir, второй backend
// рядом с cbackend.GenerateFromIR (см. package doc internal/ir). Исполняет
// *ir.Program напрямую, без
// генерации C и без вызова системного компилятора - быстрый цикл разработки
// и тестирования языка (в частности, unit-тесты этого пакета не зависят от
// настоящего cc, в отличие от сквозной проверки cbackend через реальный
// компилятор).
//
// Покрывает то же подмножество языка, что и internal/semantic.Analyze на
// момент написания - шаги 0-4, 4.1 (состояния), 4.2 (встроенные generic-
// коллекции), плюс "утверждать" (docs/ROADMAP.md, "rech test") - полное
// покрытие текущего языка, реализовано целиком в один проход (а не
// вертикальными срезами, как IR и cbackend.GenerateFromIR): в отличие от
// них, у Interpreter нет риска "спроектировать IR неправильно по ходу" -
// IR уже полностью зафиксирован и проверен обоими другими путями, поэтому
// здесь достаточно верификации по всем существующим примерам разом, без
// пошагового наращивания.
//
// Семантика значений повторяет ту, что уже выбрана в cbackend: "структура" -
// значение (копируется при переменная/присваивание/передаче аргумента/
// возврате, см. cloneValue), "сущность"/"класс" - ссылка (все имена делят
// один и тот же объект - естественно получается тем, что entityObj и так Go-
// указатель, и cloneValue его не трогает). Получатель метода ("этот") - по
// ссылке для ОБОИХ видов, даже для "структура" (мутация видна после
// возврата - реализовано прямой передачей указателя на *entityObj, как
// cbackend передаёт "Тип *this_"). Встроенные generic-коллекции (Список/
// Стопка/Очередь) - тем же принципом, что и "сущность": они физически
// представлены растущим буфером (как growable array в сгенерированном C,
// structs с полем-указателем), копирование значения такого типа копирует
// сам объект-обёртку по ссылке (та же алиасинг-семантика, что даёт C при
// копировании структуры "{T* data; long len; long cap}" - копируется
// указатель, не буфер).
package interp

import (
	"bufio"
	"database/sql"
	"fmt"
	"io"
	"net"
	"os"
	"sync"

	"github.com/IlyaFeoktistov/Rech/internal/ir"
	"github.com/IlyaFeoktistov/Rech/internal/rtmsg"
)

// entryPointName - точка входа программы (как entryPointName в cbackend,
// не переиспользуем оттуда - разным backend'ам разрешено знать об этом
// константу отдельно, они не зависят друг от друга).
const entryPointName = "главная"

// entityObj - представление значения сущности во время исполнения. Поля -
// ровно одно из трёх заполнено: обычные (fields, у пользовательских
// сущностей), встроенная generic-коллекция (coll) или системная
// "Блокировка" (mutex, см. package doc mutex.go) - естественный аналог
// нативного pthread_mutex_t/CRITICAL_SECTION у cbackend, "Блокировка" в
// интерпретаторе не нуждается ни в каком собственном C-подобном коде.
type entityObj struct {
	typeName string
	fields   map[string]Value
	coll     *collection
	mutex    *sync.Mutex
	// semaphore - непусто только у "Семафор" (см. package doc
	// internal/interp/semaphore.go) - тот же принцип, что mutex выше,
	// отдельное поле (не переиспользует mutex) - у семафора есть счётчик и
	// condvar, не голый sync.Mutex.
	semaphore *semaphoreState
	// dbTx - непусто только у "Транзакция" (internal/interp/dbtx.go) -
	// параметр колбэка "начать_транзакцию", никогда не создаётся через
	// "новый" (internal/semantic/expr.go, analyzeNewExpr, IsTransaction).
	dbTx *dbTxState
	// fieldsMu - защищает fields от гонки самого Go-рантайма (не Rech-
	// уровня "программист не защитил поле"): один и тот же ссылочный
	// объект может быть виден из нескольких потоков ОС одновременно
	// ("запустить"/"дождаться") - без этой блокировки конкурентное
	// чтение ОДНОГО поля и запись ДРУГОГО поля того же объекта уже роняет
	// сам процесс ("fatal error: concurrent map read and map write" -
	// свойство встроенной map в Go, не поведение отдельных полей C-
	// структуры, где это было бы безопасно на уровне памяти) - обнаружено
	// эмпирически на примере с "Блокировка", защищающей ровно ОДНО поле
	// объекта, пока другой поток тем же тактом читает ДРУГОЕ его поле
	// (саму "Блокировку", чтобы её захватить). Осознанная гонка ЗНАЧЕНИЙ
	// (два потока пишут одно и то же поле без Rech-уровневой блокировки)
	// по-прежнему не проверяется и не гарантируется - это остаётся на
	// программисте (docs/ROADMAP.md, "Синхронизация между потоками") - речь
	// только о том, чтобы интерпретатор сам не падал жёстче, чем упал бы
	// эквивалентный C.
	fieldsMu sync.RWMutex

	// states - установленные состояния typestate (BecomeStmt) - краткая
	// форма имени (та же, что established на этапе компиляции) -> true.
	// Рантайм-эквивалент "state_flags" в cbackend (там - битовая маска ради
	// компактного C-представления, здесь - обычная map, компактность не
	// нужна): читается StateCheckExpr (когда established не смог доказать
	// переход на этапе компиляции), пишется BecomeStmt. Отсутствие записи -
	// то же самое, что false (состояние не установлено) - обычный zero
	// value карты, отдельной инициализации не требует. statesMu - та же
	// защита от гонки Go-рантайма, что и у fields (fieldsMu выше), отдельный
	// мьютекс - состояния и поля меняются независимо друг от друга.
	states   map[string]bool
	statesMu sync.RWMutex
}

// getField/setField/hasField - единственные места, где entityObj.fields
// читается/пишется - см. package doc fieldsMu выше.
func (o *entityObj) getField(name string) Value {
	o.fieldsMu.RLock()
	defer o.fieldsMu.RUnlock()
	return o.fields[name]
}

func (o *entityObj) getFieldOK(name string) (Value, bool) {
	o.fieldsMu.RLock()
	defer o.fieldsMu.RUnlock()
	v, ok := o.fields[name]
	return v, ok
}

func (o *entityObj) setField(name string, v Value) {
	o.fieldsMu.Lock()
	defer o.fieldsMu.Unlock()
	o.fields[name] = v
}

// hasState/setState - единственные места, где entityObj.states
// читается/пишется - см. package doc statesMu выше.
func (o *entityObj) hasState(name string) bool {
	o.statesMu.RLock()
	defer o.statesMu.RUnlock()
	return o.states[name]
}

func (o *entityObj) setState(name string) {
	o.statesMu.Lock()
	defer o.statesMu.Unlock()
	if o.states == nil {
		o.states = map[string]bool{}
	}
	o.states[name] = true
}

func (o *entityObj) clearState(name string) {
	o.statesMu.Lock()
	defer o.statesMu.Unlock()
	delete(o.states, name)
}

// collection - буфер встроенной generic-коллекции (Список/Стопка/Очередь/
// Словарь). template определяет, какое из двух полей ниже используется -
// "Словарь" (pairs) или любой из трёх списочных шаблонов (elems); нужен и
// для сообщений об ошибках (у списочных - методы уже определяют поведение
// сами по своему имени, у Словаря отдельного сообщения не требуется -
// "получить" паникует по отсутствующему ключу безусловно).
type collection struct {
	template string
	elems    []Value
	// pairs - хранилище "Словарь<К, В>" - непусто (инициализировано
	// evalNewExpr) только при template == "Словарь". Value как ключ Go-map
	// сравним напрямую (==) для примитивных типов ключа Словаря (Целое/
	// Строка/Байт/Символ/Логическое, см. internal/semantic.dictKeyKinds) -
	// указательные поля Value (Obj/File/Listener/Conn/Task/Callable) у них
	// всегда nil, значит сравнение всей структуры корректно отражает
	// равенство самого значения ключа.
	pairs map[Value]Value
}

// Value - значение во время исполнения. Структура (не interface{}) по тому
// же принципу, что и ir.Type/valueType в cbackend - явное поле Type несёт
// форму значения, не нужно приведение типа на каждом шаге.
type Value struct {
	Type ir.Type
	I    int64
	F    float64
	S    string
	B    bool
	Obj  *entityObj // непусто только при Type.Kind == ir.KindEntity
	File *os.File   // непусто только при Type.Kind == ir.KindFile

	// Listener/Conn/PacketConn - непрозрачный дескриптор сетевого сокета
	// (сетевые примитивы), Type.Kind == ir.KindSocket - ровно одно из трёх
	// непусто: Listener - результат "сетевой_узел("tcp", "", порт)" (только
	// "принять" имеет смысл), Conn - результат "принять"/"сетевой_узел"
	// с непустым хостом (клиентский tcp/udp, только "отправить"/
	// "получить"/"закрыть_сокет"), PacketConn - результат
	// "сетевой_узел("udp", "", порт)" (udp-сервер без connect, только
	// "получить_от"/"отправить_в"/"закрыть_сокет"). На уровне Rech-типа
	// все три - один и тот же "Сокет" (компилятор не различает вид сокета
	// статически, как и cbackend не различает их на уровне rech_socket_t) -
	// вызов не той операции здесь, в отличие от cbackend (где это ловит
	// сама ОС кодом ошибки), ловится явной проверкой (см. evalBuiltinCall,
	// rtmsg.SocketWrongKind).
	Listener   net.Listener
	Conn       net.Conn
	PacketConn net.PacketConn

	// RawListener - непусто только для TLS-слушателя (слушать_tcp_защищённо
	// - Listener тогда обёрнут в tls.NewListener, чей встроенный net.Listener
	// - интерфейсное поле, не даёт продвинуть SetDeadline от конкретного
	// *net.TCPListener наружу, даже если сам он его умеет) - "сырой", ДО
	// обёртки в TLS слушатель, тот же fd - установить_таймаут/"принять" с
	// таймаутом (network_timeout.go) ставят дедлайн именно на него, а
	// Accept() всё равно зовут на обёрнутом Listener (для рукопожатия).
	RawListener net.Listener

	// Task - непусто только при Type.Kind == ir.KindTask (шаг 15,
	// "запустить"/"дождаться") - см. internal/interp/concurrency.go.
	Task *taskValue

	// Callable - непусто только при Type.Kind == ir.KindCallable
	// ("Вызываемый", "&имя"/лямбда) - см. internal/interp/callable.go.
	Callable *callableValue

	// Nullable - валидно только при Type.Kind == ir.KindNullable ("?Тип") -
	// nil означает "Ничто", ненулевой указатель несёт обёрнутое значение
	// (со своим настоящим внутренним Type) - указатель, а не значение по
	// значению, тем же принципом, что и Boxed ниже: сама занятость (has/
	// Ничто) неотличима от нулевого значения примитива (0/""/false), в
	// отличие от них указатель однозначно различает "нет значения" и "есть
	// значение, которое случайно нулевое".
	Nullable *Value

	// Boxed - непусто только при Type.Kind == ir.KindAny ("Значение",
	// боксирование для рефлексии) - само боксированное значение целиком
	// (со своим настоящим Type) под указателем, см.
	// internal/interp/reflect.go. Указатель, а не значение по значению -
	// Value и так копируется целиком при переменная/присваивании, Boxed
	// не нуждается в собственном клонировании сверху (см. cloneValue).
	// Type.Kind == ir.KindTypeInfo ("Тип") использует S (имя типа), не
	// Boxed - дескриптору типа нечего боксировать, он сам и есть значение.
	Boxed *Value

	// DB - непрозрачный дескриптор соединения с БД, Type.Kind == ir.KindDB -
	// обёртка над database/sql поверх pure-Go драйверов (см. package doc
	// internal/interp/db.go); один и тот же Go-тип для
	// SQLite/PostgreSQL/MySQL, конкретный драйвер решён в рантайме по
	// схеме строки подключения при "подключиться_к_бд", здесь это уже не
	// видно.
	DB *sql.DB

	// HTTP2 - непрозрачный дескриптор активной HTTP/2-сессии, Type.Kind ==
	// ir.KindHTTP2Session - см. package doc internal/interp/http2.go.
	HTTP2 *http2Session

	// Ptr - непрозрачный "адрес" ("Указатель<T>", Type.Kind ==
	// ir.KindPointer) - см. package doc internal/interp/pointer.go.
	Ptr *pointerTarget
}

func intValue(v int64) Value     { return Value{Type: ir.TypeInt, I: v} }
func floatValue(v float64) Value { return Value{Type: ir.TypeFloat, F: v} }
func stringValue(v string) Value { return Value{Type: ir.TypeString, S: v} }
func boolValue(v bool) Value     { return Value{Type: ir.TypeBool, B: v} }
func fileValue(f *os.File) Value { return Value{Type: ir.TypeFile, File: f} }

// charValue - "Символ", кодовая точка Unicode, хранится в I (как и Байт) -
// значение отличает только Type.
func charValue(v rune) Value { return Value{Type: ir.TypeChar, I: int64(v)} }

// byteValue - "Байт", 8 бит без знака - маска на 0xFF воспроизводит
// усечение C unsigned char (то же самое, что происходит в cbackend при
// касте в Байт).
func byteValue(v int64) Value { return Value{Type: ir.TypeByte, I: v & 0xFF} }

// float32Value - "Дробное", одинарная точность - округление до float32 при
// каждом создании воспроизводит фактическую потерю точности C float,
// вместо того чтобы хранить полную точность float64 под именем "Дробное".
func float32Value(v float64) Value { return Value{Type: ir.TypeFloat32, F: float64(float32(v))} }

func listenerValue(l net.Listener) Value      { return Value{Type: ir.TypeSocket, Listener: l} }
func connValue(c net.Conn) Value              { return Value{Type: ir.TypeSocket, Conn: c} }
func packetConnValue(pc net.PacketConn) Value { return Value{Type: ir.TypeSocket, PacketConn: pc} }

// zeroValue - нулевое значение типа t, нужно только явному конструктору
// сущности (evalNewExprWithCtor) - объект заводится с нулевыми полями ДО
// исполнения тела конструктора (тело может не присвоить все поля явно, как
// и GC_MALLOC в cbackend, который зануляет всю память под структуру перед
// запуском тела конструктора - см. package doc evalNewExprWithCtor). Поле
// типа сущности/интерфейса - Value{Obj: nil}: язык пока не различает
// "ещё не присвоено" и "пусто" (docs/ROADMAP.md, будущее Ничто/nullable) -
// не эта фича должна это решать.
func zeroValue(t ir.Type) Value {
	switch t.Kind {
	case ir.KindInt:
		return intValue(0)
	case ir.KindFloat32:
		return float32Value(0)
	case ir.KindFloat:
		return floatValue(0)
	case ir.KindChar:
		return charValue(0)
	case ir.KindByte:
		return byteValue(0)
	case ir.KindString:
		return stringValue("")
	case ir.KindBool:
		return boolValue(false)
	default:
		return Value{Type: t}
	}
}

// cloneValue - применяет значение-копирующую семантику "структура" (докс.
// SPEC.md, "Структуры - значения") в момент "коммита" в новое хранилище
// (переменная/параметр/поле/возврат) - навигация по цепочке чтения
// (FieldAccess.Receiver, receiver вызова метода) саму по себе не клонирует,
// иначе получатель метода не мог бы мутировать оригинал через "этот".
// "сущность"/"класс" (не структура) - ссылочная семантика по умолчанию:
// тот же *entityObj возвращается как есть, оба имени делят один объект -
// естественно получается ничего не делая, потому что entityObj и так Go-
// указатель, а сборка мусора самого Go уже управляет его временем жизни.
// Generic-коллекции - тот же принцип (обёртка не найдётся в in.entities,
// т.к. они регистрируются отдельно, в in.generics, см. package doc) - без
// глубокого клонирования буфера, как и раньше.
func (in *Interp) cloneValue(v Value) Value {
	if v.Type.Kind == ir.KindNullable {
		if v.Nullable == nil {
			return v
		}
		cloned := in.cloneValue(*v.Nullable)
		return Value{Type: v.Type, Nullable: &cloned}
	}
	if v.Type.Kind != ir.KindEntity || v.Obj == nil {
		return v
	}
	if ent, ok := in.entities[v.Obj.typeName]; !ok || !ent.IsStruct {
		return v
	}
	v.Obj.fieldsMu.RLock()
	newFields := make(map[string]Value, len(v.Obj.fields))
	for k, fv := range v.Obj.fields {
		newFields[k] = in.cloneValue(fv)
	}
	v.Obj.fieldsMu.RUnlock()
	return Value{Type: v.Type, Obj: &entityObj{typeName: v.Obj.typeName, fields: newFields}}
}

// Env - область видимости переменных (стек, как ctx.scopes в cbackend и
// scopeStack в internal/semantic) - блоки если/пока/для заводят вложенный
// Env, а не переиспользуют функцию целиком.
type Env struct {
	vars   map[string]Value
	parent *Env

	// staticIDs - имя "статической переменной" (ir.VarDecl.IsStatic) в
	// этой области видимости -> ir.VarDecl.StaticID. Параллельная vars
	// структура (та же цепочка родителей, то же затенение) - значение
	// самой переменной по-прежнему живёт в vars (обычный env, как у любой
	// другой переменной, копия на каждый вызов), но её АКТУАЛЬНОЕ значение
	// обязано пережить возврат из функции - staticIDs говорит записывающей
	// стороне (Assign/IncDec/NullCoalesceAssign), что после обновления vars
	// нужно ЕЩЁ И зеркально обновить Interp.staticLocals[id] - см. package
	// doc Interp.staticLocals.
	staticIDs map[string]int
}

func newEnv(parent *Env) *Env {
	return &Env{vars: map[string]Value{}, parent: parent}
}

// declareStatic регистрирует name как статическую переменную с этим id - в
// самой внутренней области видимости, тем же принципом, что и declare (см.
// package doc staticIDs).
func (e *Env) declareStatic(name string, id int) {
	if e.staticIDs == nil {
		e.staticIDs = map[string]int{}
	}
	e.staticIDs[name] = id
}

// lookupStaticID ищет name в staticIDs через цепочку родителей (тот же
// порядок, что и get/set) - true и StaticID, если name - статическая
// переменная, объявленная где-то в видимой области.
func (e *Env) lookupStaticID(name string) (int, bool) {
	for cur := e; cur != nil; cur = cur.parent {
		if id, ok := cur.staticIDs[name]; ok {
			return id, true
		}
		if _, ok := cur.vars[name]; ok {
			// Дошли до обычной (не статической) переменной с этим именем в
			// более внутренней области, чем возможная статическая с тем же
			// именем снаружи - она её затеняет, дальше искать не нужно.
			return 0, false
		}
	}
	return 0, false
}

func (e *Env) get(name string) (Value, bool) {
	for cur := e; cur != nil; cur = cur.parent {
		if v, ok := cur.vars[name]; ok {
			return v, true
		}
	}
	return Value{}, false
}

// set - присваивание уже объявленной переменной: находит область, где имя
// объявлено, и обновляет его там (а не заводит новое в самой внутренней,
// как declare) - соответствует "имя = выражение" (не "переменная имя = ...").
func (e *Env) set(name string, v Value) {
	for cur := e; cur != nil; cur = cur.parent {
		if _, ok := cur.vars[name]; ok {
			cur.vars[name] = v
			return
		}
	}
	e.vars[name] = v
}

func (e *Env) declare(name string, v Value) {
	e.vars[name] = v
}

// callCtx - контекст вызова функции/метода: this - объект-получатель метода
// (nil у свободной функции) - передаётся отдельно от Env, а не через него,
// потому что не меняется внутри вложенных блоков одного вызова и не должен
// наследоваться через границу вызова (новый вызов - новый callCtx).
//
// curLine - строка исполняемой сейчас инструкции ЭТОЙ активации вызова (см.
// package doc internal/interp/trace.go) - указатель, а не int, потому что
// execStmt получает cc по значению интерфейса callCtx на каждом уровне
// вложенных if/пока/для ОДНОГО И ТОГО ЖЕ вызова (execBlock передаёт тот же
// cc дальше, не создаёт новый), но должен писать в одну и ту же ячейку,
// которую в конце читает callFunction/callMethod этой активации.
// dbg - THREAD этой активации, если программа исполняется под отладчиком
// (internal/interp/debugger.go) - nil в обычном Run()/rech test, в отличие
// от curLine НЕ пересоздаётся заново на каждый вызов, а передаётся от
// вызывающей стороны как есть (тот же поток исполнения продолжает нести
// один и тот же стек кадров отладчика) - см. callFunction/callMethod.
type callCtx struct {
	this    *entityObj
	curLine *int
	dbg     *dbgThread
}

// runtimeError - ошибка времени выполнения программы Rech (не компилятора) -
// выход за границы/операция на пустой встроенной коллекции, ложное условие
// "утверждать". Соответствует тому же классу ошибок, что и rech_panic в
// сгенерированном C (docs/SPEC.md, раздел 8) - аварийное завершение
// выполнения программы, а не паника самого интерпретатора. Run превращает
// её в обычную error (см. panicRuntime и recover в Run) - решение о том,
// завершать ли процесс целиком (`rech run`-подобный сценарий) или считать
// одним неудачным тестом среди многих и продолжить (`rech test`) - на
// стороне вызывающего кода, не этого пакета.
// trace - стек вызовов (internal/interp/trace.go), пуст сразу после
// panicRuntime и заполняется по мере того, как паника проходит через
// callFunction/callMethod вверх до Run.
type runtimeError struct {
	msg   string
	trace []traceFrame
}

func (e *runtimeError) Error() string { return e.msg + traceSuffix(e.trace) }

// panicRuntime останавливает исполнение текущей программы Rech как раз в
// той точке, где встретилась ошибка (используется panic/recover Go как
// внутренний механизм нелокального выхода) - невосстановимая ошибка, в
// отличие от "выбросить" (thrownException, exception.go), которую
// пользовательский код может перехватить через "попытаться"/"поймать" -
// см. runtimeError и Run. rtmsg.PanicPrefix - тот же префикс, что печатает
// rech_panic в сгенерированном C.
func panicRuntime(format string, args ...any) {
	panic(&runtimeError{msg: rtmsg.PanicPrefix + fmt.Sprintf(format, args...)})
}

// Interp - состояние интерпретатора одной программы. debugger - непусто
// только при исполнении через NewDebugger (internal/interp/debugger.go,
// "rech debug") - обычный Run/rech test его не заводит вовсе, поэтому не
// платит ни за какие проверки/блокировки отладчика.
type Interp struct {
	out        io.Writer
	stdin      *bufio.Reader
	funcs      map[string]*ir.Function
	entities   map[string]*ir.Entity
	interfaces map[string]*ir.Interface
	generics   map[string]ir.GenericInstantiation
	debugger   *Debugger

	// interfaceImplementors - см. package doc ir.Program.InterfaceImplementors
	// (internal/semantic уже вычислил его транзитивно, entitySatisfies) -
	// нужен evalAsExpr для "значение как ИмяИнтерфейса".
	interfaceImplementors map[string][]string
	// entityInterfaces - см. package doc ir.Program.EntityInterfaces - нужен
	// evalTypeInfoMethod для "число_интерфейсов"/"получить_интерфейс".
	entityInterfaces map[string][]string

	// staticFields - значение статического поля сущности ("Тип::поле", см.
	// ir.Entity.StaticFields) по (имя сущности, имя поля) - одно общее
	// хранилище на всю программу, заполняется значениями по умолчанию один
	// раз в newInterp (см. initStaticFields), до вызова "главная".
	staticFields map[string]map[string]Value

	// staticLocals - значение "статической переменной" (ir.VarDecl.
	// IsStatic) по StaticID - хранилище живёт дольше одного вызова функции/
	// метода (в отличие от обычного Env, который создаётся заново на
	// каждый вызов), см. package doc Env.staticIDs.
	staticLocals map[int]Value

	// dbPoolSize - ir.Program.DBPoolSize (внешне заданное значение
	// "бд.размер_пула" из .rechproj, если оно было - internal/compiler
	// подставляет его до semantic.Analyze; иначе - internal/semantic.
	// DefaultDBPoolSize) - dbConnect передаёт его в database/sql
	// (SetMaxOpenConns), см. package doc dbConnect в db.go.
	dbPoolSize int
	// dbPoolTimeoutMs - ir.Program.DBPoolTimeoutMs ("бд.таймаут_пула_мс" из
	// .rechproj, 0 - ждать бесконечно, тем же принципом, что и до появления
	// этой настройки) - dbQuery/dbExec читают его, см. их package doc в
	// db.go про известную асимметрию с cbackend.
	dbPoolTimeoutMs int
	// dbSQLiteBusyTimeoutMs - ir.Program.DBSQLiteBusyTimeoutMs ("бд.
	// sqlite_таймаут_блокировки_мс" из .rechproj, по умолчанию internal/
	// semantic.DefaultDBSQLiteBusyTimeoutMs) - dbConnect передаёт его в DSN
	// SQLite ("?_busy_timeout=") - см. её package doc в db.go.
	dbSQLiteBusyTimeoutMs int
	// httpMaxRequestBytes - ir.Program.HTTPMaxRequestBytes ("http.
	// макс_размер_запроса_байт" из .rechproj, по умолчанию internal/
	// semantic.DefaultHTTPMaxRequestBytes) - recvHTTPRequest сверяет с ним
	// накопленный буфер запроса, см. её package doc в httpserver.go.
	httpMaxRequestBytes int
}

// Run исполняет prog: находит "главная" и выполняет её тело, печатая
// "вывести" в out и читая прочитать_строку/прочитать_целое/прочитать_число
// (шаг 8, internal/builtins) из stdin. Возвращает error и в случае ошибки
// компиляции (в prog нет точки входа - не должно происходить, если prog
// прошёл internal/semantic.Analyze, но Run не полагается на это слепо), и
// в случае ошибки
// времени выполнения самой программы Rech (runtimeError/panicRuntime -
// перехватывается здесь через recover, превращается в обычный error, а не
// падает наружу как паника Go или завершает процесс сам - решение о
// дальнейшем (аварийно завершить процесс или считать одним неудачным
// тестом и продолжить) принимает вызывающий код, см. runtimeError).
//
// Recover здесь ловит вообще любую панику, не только runtimeError - вызов
// Run не должен ронять процесс целиком ни при какой ошибке во время
// исполнения программы Rech, включая необработанную ошибку самого Go
// (например, если runtimeError по какой-то причине не покрыл случай) -
// иначе один упавший файл в "rech test" завершал бы весь прогон вместо
// того, чтобы считаться одним неудачным тестом.
func Run(prog *ir.Program, out io.Writer, stdin io.Reader) (err error) {
	in, main, err := newInterp(prog, out, stdin)
	if err != nil {
		return err
	}
	defer func() {
		if r := recover(); r != nil {
			if re, ok := r.(*runtimeError); ok {
				err = re
				return
			}
			err = fmt.Errorf("внутренняя ошибка интерпретатора: %v", r)
		}
	}()
	_, err = in.callFunction(main, nil, nil)
	return err
}

// newInterp - общая подготовка Run и NewDebugger (internal/interp/
// debugger.go): проверка внешних функций (шаг 7, FFI - интерпретатор,
// чистый Go, не умеет вызвать произвольный C-символ без настоящей
// динамической линковки, которой пока нет, поэтому отказывается сам, а не
// молча игнорирует/падает глубже), таблицы функций/сущностей/generic-
// инстанциаций программы по имени, точка входа "главная".
func newInterp(prog *ir.Program, out io.Writer, stdin io.Reader) (*Interp, *ir.Function, error) {
	if len(prog.ExternFuncs) > 0 {
		return nil, nil, fmt.Errorf("внешняя функция %q: внешние функции пока не поддерживаются интерпретатором/`rech test` - соберите и запустите программу через `rech build`/`rech run`",
			prog.ExternFuncs[0].Alias)
	}
	in := &Interp{
		out:                   out,
		stdin:                 bufio.NewReader(stdin),
		funcs:                 map[string]*ir.Function{},
		entities:              map[string]*ir.Entity{},
		interfaces:            map[string]*ir.Interface{},
		generics:              map[string]ir.GenericInstantiation{},
		staticFields:          map[string]map[string]Value{},
		staticLocals:          map[int]Value{},
		interfaceImplementors: prog.InterfaceImplementors,
		entityInterfaces:      prog.EntityInterfaces,
		dbPoolSize:            prog.DBPoolSize,
		dbPoolTimeoutMs:       prog.DBPoolTimeoutMs,
		dbSQLiteBusyTimeoutMs: prog.DBSQLiteBusyTimeoutMs,
		httpMaxRequestBytes:   prog.HTTPMaxRequestBytes,
	}
	for _, fn := range prog.Functions {
		in.funcs[fn.Name] = fn
	}
	for _, ent := range prog.Entities {
		in.entities[ent.Name] = ent
	}
	for _, iface := range prog.Interfaces {
		in.interfaces[iface.Name] = iface
	}
	for _, g := range prog.Generics {
		in.generics[g.Name] = g
	}
	if err := in.initStaticFields(prog); err != nil {
		return nil, nil, err
	}
	main, ok := in.funcs[entryPointName]
	if !ok {
		return nil, nil, fmt.Errorf("в программе не найдена функция %q - она служит точкой входа", entryPointName)
	}
	return in, main, nil
}

// initStaticFields заполняет in.staticFields значениями по умолчанию всех
// статических полей программы (см. package doc Entity.StaticFields в
// internal/ir/decl.go) - РОВНО один раз, до вызова "главная", тем же
// принципом, что и rech_init_statics() в cbackend (internal/backend/
// cbackend/statics.go). Поле без Default получает нулевое значение своего
// типа (zeroValue) - та же гарантия, что и у C-глобальной переменной без
// явного инициализатора.
func (in *Interp) initStaticFields(prog *ir.Program) error {
	cc := &callCtx{curLine: new(int)}
	for _, ent := range prog.Entities {
		if len(ent.StaticFields) == 0 {
			continue
		}
		fields := map[string]Value{}
		for _, f := range ent.StaticFields {
			if f.Default == nil {
				fields[f.Name] = in.zeroFieldValue(f.Type)
				continue
			}
			v, err := in.evalExpr(f.Default, newEnv(nil), cc)
			if err != nil {
				return fmt.Errorf("сущность %q: статическое поле %q: %w", ent.Name, f.Name, err)
			}
			fields[f.Name] = in.cloneValue(v)
		}
		in.staticFields[ent.Name] = fields
	}
	return nil
}

// callFunction/callMethod - см. package doc internal/interp/trace.go про то,
// как именно кадр этого вызова (name/*cc.curLine) попадает в trace паники/
// исключения, проходящей через эту активацию: defer/recover для
// panicRuntime (Go panic, разматывается снизу вверх сама, без явного стека -
// см. runtimeError.trace) и обычная проверка err для thrownException
// (пробрасывается как ЗНАЧЕНИЕ error, см. package doc exception.go).
//
// dbg - поток отладчика (internal/interp/debugger.go), nil у обычного Run/
// rech test - тогда push/pop кадра дальше ничего не делают (см.
// dbgThread.push/pop, nil-получатель безопасен). В отличие от cc,
// пересоздаваемого заново, dbg передаётся ОТ ВЫЗЫВАЮЩЕЙ СТОРОНЫ как есть -
// один и тот же поток исполнения несёт один и тот же стек кадров отладчика
// через границы вызовов (новый dbg заводится только "запустить" -
// concurrency.go, новая горутина - новый поток отладчика).

func (in *Interp) callFunction(fn *ir.Function, args []Value, dbg *dbgThread) (result Value, err error) {
	env := newEnv(nil)
	for i, p := range fn.Params {
		env.declare(p.Name, in.cloneValue(args[i]))
	}
	cc := &callCtx{curLine: new(int), dbg: dbg}
	dbg.push(fn.Name, fn.File)
	defer dbg.pop()
	defer func() {
		if r := recover(); r != nil {
			re, ok := r.(*runtimeError)
			if !ok {
				panic(r)
			}
			re.trace = append(re.trace, traceFrame{name: fn.Name, line: *cc.curLine})
			panic(re)
		}
	}()
	ctrl, execErr := in.execBlock(fn.Body, env, cc)
	if execErr != nil {
		if te, ok := execErr.(*thrownException); ok {
			te.trace = append(te.trace, traceFrame{name: fn.Name, line: *cc.curLine})
		}
		return Value{}, execErr
	}
	if ctrl.kind == ctrlReturn {
		return ctrl.value, nil
	}
	return Value{}, nil
}

func (in *Interp) callMethod(m *ir.Method, this *entityObj, args []Value, dbg *dbgThread) (result Value, err error) {
	env := newEnv(nil)
	for i, p := range m.Params {
		env.declare(p.Name, in.cloneValue(args[i]))
	}
	name := m.Name
	if name == "" {
		// ent.Ctor (явный конструктор сущности) не несёт своего имени -
		// callMethod вызывается для него и напрямую (evalNewExprWithCtor),
		// и через "родитель(...)" (ParentCallStmt) - тот же текст, что и
		// cbackend.generateIREntityCtorInit (trace.go, tracePush).
		name = "конструктор"
	}
	displayName := this.typeName + "." + name
	cc := &callCtx{this: this, curLine: new(int), dbg: dbg}
	dbg.pushMethod(displayName, m.File, this)
	defer dbg.pop()
	defer func() {
		if r := recover(); r != nil {
			re, ok := r.(*runtimeError)
			if !ok {
				panic(r)
			}
			re.trace = append(re.trace, traceFrame{name: displayName, line: *cc.curLine})
			panic(re)
		}
	}()
	ctrl, execErr := in.execBlock(m.Body, env, cc)
	if execErr != nil {
		if te, ok := execErr.(*thrownException); ok {
			te.trace = append(te.trace, traceFrame{name: displayName, line: *cc.curLine})
		}
		return Value{}, execErr
	}
	if ctrl.kind == ctrlReturn {
		return ctrl.value, nil
	}
	return Value{}, nil
}

// callStaticMethod - как callMethod, но для статического метода
// (ir.Method.IsStatic): нет получателя ("this" всегда nil в cc и в кадре
// отладчика - "этот" внутри тела грамматически невозможен, парсер уже это
// гарантирует), displayName строится из имени сущности-владельца
// напрямую, а не через this.typeName (this.typeName обратился бы к полю
// нулевого указателя).
func (in *Interp) callStaticMethod(m *ir.Method, entityName string, args []Value, dbg *dbgThread) (result Value, err error) {
	env := newEnv(nil)
	for i, p := range m.Params {
		env.declare(p.Name, in.cloneValue(args[i]))
	}
	displayName := entityName + "." + m.Name
	cc := &callCtx{this: nil, curLine: new(int), dbg: dbg}
	dbg.pushMethod(displayName, m.File, nil)
	defer dbg.pop()
	defer func() {
		if r := recover(); r != nil {
			re, ok := r.(*runtimeError)
			if !ok {
				panic(r)
			}
			re.trace = append(re.trace, traceFrame{name: displayName, line: *cc.curLine})
			panic(re)
		}
	}()
	ctrl, execErr := in.execBlock(m.Body, env, cc)
	if execErr != nil {
		if te, ok := execErr.(*thrownException); ok {
			te.trace = append(te.trace, traceFrame{name: displayName, line: *cc.curLine})
		}
		return Value{}, execErr
	}
	if ctrl.kind == ctrlReturn {
		return ctrl.value, nil
	}
	return Value{}, nil
}

type ctrlKind int

const (
	ctrlNone ctrlKind = iota
	ctrlReturn
	ctrlBreak
	ctrlContinue
)

type ctrl struct {
	kind  ctrlKind
	value Value
}

// execBlock исполняет stmts по порядку; при встрече "вернуть"/"прервать"/
// "продолжить" останавливает выполнение этого блока и пробрасывает
// полученный ctrl ВВЕРХ немедленно (а не только текущий блок) - вызывающий
// код на каждом уровне (если/попытаться/... - execStmt) сам решает,
// пробросить ли ctrl ещё выше или (только цикл - WhileStmt/ForStmt/
// ForEachStmt/CStyleForStmt в exec.go) перехватить ctrlBreak/ctrlContinue
// как собственно предназначенные ему.
func (in *Interp) execBlock(stmts []ir.Stmt, env *Env, cc *callCtx) (ctrl, error) {
	for _, s := range stmts {
		c, err := in.execStmt(s, env, cc)
		if err != nil {
			return ctrl{}, err
		}
		if c.kind != ctrlNone {
			return c, nil
		}
	}
	return ctrl{}, nil
}
