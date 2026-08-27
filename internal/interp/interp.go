// Package interp - Interpreter backend поверх internal/ir (шаг 5, второй
// backend рядом с cbackend.GenerateFromIR, см. package doc internal/ir и
// docs/ROADMAP.md, раздел "Шаг 5"). Исполняет *ir.Program напрямую, без
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
	"fmt"
	"io"
	"net"
	"os"

	"github.com/IlyaFeoktistov/Rech/internal/ir"
	"github.com/IlyaFeoktistov/Rech/internal/rtmsg"
)

// entryPointName - точка входа программы (как entryPointName в cbackend,
// не переиспользуем оттуда - разным backend'ам разрешено знать об этом
// константу отдельно, они не зависят друг от друга).
const entryPointName = "главная"

// entityObj - представление значения сущности во время исполнения. Поля -
// либо обычные (fields, у пользовательских сущностей), либо встроенная
// generic-коллекция (coll) - ровно одно из двух заполнено.
type entityObj struct {
	typeName string
	fields   map[string]Value
	coll     *collection
}

// collection - буфер встроенной generic-коллекции (Список/Стопка/Очередь).
// template - "Список"/"Стопка"/"Очередь", нужен только для сообщений об
// ошибках (методы уже определяют поведение сами по своему имени).
type collection struct {
	template string
	elems    []Value
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

	// Listener/Conn - непрозрачный дескриптор сетевого сокета (шаг 8,
	// сетевые примитивы), Type.Kind == ir.KindSocket - ровно одно из двух
	// непусто: Listener - результат "слушать_tcp" (только "принять" имеет
	// смысл), Conn - результат "принять"/"подключиться_tcp"/
	// "подключиться_udp" (только "отправить"/"получить"/"закрыть_сокет").
	// На уровне Rech-типа оба - один и тот же "Сокет" (компилятор не
	// различает слушающий/подключённый сокет статически, как и cbackend не
	// различает их на уровне rech_socket_t) - вызов не той операции здесь,
	// в отличие от cbackend (где это ловит сама ОС кодом ошибки), ловится
	// явной проверкой (см. evalBuiltinCall, rtmsg.SocketWrongKind).
	Listener net.Listener
	Conn     net.Conn

	// Task - непусто только при Type.Kind == ir.KindTask (шаг 15,
	// "запустить"/"дождаться") - см. internal/interp/concurrency.go.
	Task *taskValue

	// Callable - непусто только при Type.Kind == ir.KindCallable
	// ("Вызываемый", "&имя"/лямбда) - см. internal/interp/callable.go.
	Callable *callableValue
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

func listenerValue(l net.Listener) Value { return Value{Type: ir.TypeSocket, Listener: l} }
func connValue(c net.Conn) Value         { return Value{Type: ir.TypeSocket, Conn: c} }

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
	if v.Type.Kind != ir.KindEntity || v.Obj == nil {
		return v
	}
	if ent, ok := in.entities[v.Obj.typeName]; !ok || !ent.IsStruct {
		return v
	}
	newFields := make(map[string]Value, len(v.Obj.fields))
	for k, fv := range v.Obj.fields {
		newFields[k] = in.cloneValue(fv)
	}
	return Value{Type: v.Type, Obj: &entityObj{typeName: v.Obj.typeName, fields: newFields}}
}

// Env - область видимости переменных (стек, как ctx.scopes в cbackend и
// scopeStack в internal/semantic) - блоки если/пока/для заводят вложенный
// Env, а не переиспользуют функцию целиком.
type Env struct {
	vars   map[string]Value
	parent *Env
}

func newEnv(parent *Env) *Env {
	return &Env{vars: map[string]Value{}, parent: parent}
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
type callCtx struct {
	this *entityObj
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
type runtimeError struct{ msg string }

func (e *runtimeError) Error() string { return e.msg }

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

// Interp - состояние интерпретатора одной программы.
type Interp struct {
	out      io.Writer
	stdin    *bufio.Reader
	funcs    map[string]*ir.Function
	entities map[string]*ir.Entity
	generics map[string]ir.GenericInstantiation
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
	// Внешние функции (шаг 7, FFI) - явный, документированный пробел
	// именно интерпретатора (docs/ROADMAP.md), не самого IR:
	// semantic.Analyze производит ir.Program.ExternFuncs как обычно (у
	// cbackend.GenerateFromIR это реально работает) - интерпретатор
	// (чистый Go) не умеет вызвать произвольный C-символ без настоящей
	// динамической линковки (dlopen/dlsym), которой пока нет, поэтому
	// отказывается сам, здесь, а не молча игнорирует/падает глубже.
	if len(prog.ExternFuncs) > 0 {
		return fmt.Errorf("внешняя функция %q: внешние функции пока не поддерживаются интерпретатором/`rech test` - соберите и запустите программу через `rech build`/`rech run`",
			prog.ExternFuncs[0].Alias)
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

	in := &Interp{
		out:      out,
		stdin:    bufio.NewReader(stdin),
		funcs:    map[string]*ir.Function{},
		entities: map[string]*ir.Entity{},
		generics: map[string]ir.GenericInstantiation{},
	}
	for _, fn := range prog.Functions {
		in.funcs[fn.Name] = fn
	}
	for _, ent := range prog.Entities {
		in.entities[ent.Name] = ent
	}
	for _, g := range prog.Generics {
		in.generics[g.Name] = g
	}
	main, ok := in.funcs[entryPointName]
	if !ok {
		return fmt.Errorf("в программе не найдена функция %q - она служит точкой входа", entryPointName)
	}
	_, err = in.callFunction(main, nil)
	return err
}

func (in *Interp) callFunction(fn *ir.Function, args []Value) (Value, error) {
	env := newEnv(nil)
	for i, p := range fn.Params {
		env.declare(p.Name, in.cloneValue(args[i]))
	}
	ctrl, err := in.execBlock(fn.Body, env, &callCtx{})
	if err != nil {
		return Value{}, err
	}
	if ctrl.kind == ctrlReturn {
		return ctrl.value, nil
	}
	return Value{}, nil
}

func (in *Interp) callMethod(m *ir.Method, this *entityObj, args []Value) (Value, error) {
	env := newEnv(nil)
	for i, p := range m.Params {
		env.declare(p.Name, in.cloneValue(args[i]))
	}
	ctrl, err := in.execBlock(m.Body, env, &callCtx{this: this})
	if err != nil {
		return Value{}, err
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
)

type ctrl struct {
	kind  ctrlKind
	value Value
}

// execBlock исполняет stmts по порядку; при встрече "вернуть" останавливает
// выполнение и возвращает ctrlReturn, чтобы вызывающие (если/пока/для, а
// затем сам вызов функции/метода) прервали исполнение немедленно, а не
// только текущую итерацию/блок.
func (in *Interp) execBlock(stmts []ir.Stmt, env *Env, cc *callCtx) (ctrl, error) {
	for _, s := range stmts {
		c, err := in.execStmt(s, env, cc)
		if err != nil {
			return ctrl{}, err
		}
		if c.kind == ctrlReturn {
			return c, nil
		}
	}
	return ctrl{}, nil
}
