package interp

import (
	"io"
	"reflect"
	"strings"
	"testing"
	"unsafe"

	"github.com/ebitengine/purego"

	"github.com/IlyaFeoktistov/Rech/internal/ir"
	"github.com/IlyaFeoktistov/Rech/internal/lexer"
	"github.com/IlyaFeoktistov/Rech/internal/parser"
	"github.com/IlyaFeoktistov/Rech/internal/semantic"
)

// buildInterp - тот же пайплайн, что и runSrc (interp_test.go), но
// возвращает сам *Interp вместо результата "главная" - для тестов ниже,
// проверяющих внутренний маршалинг FFI напрямую (setFFIValue/
// readFFIValue), не полный прогон программы.
func buildInterp(t *testing.T, src string) *Interp {
	t.Helper()
	toks, err := lexer.New(src).Tokenize()
	if err != nil {
		t.Fatalf("лексер: %v", err)
	}
	prog, err := parser.New(toks).ParseProgram()
	if err != nil {
		t.Fatalf("парсер: %v", err)
	}
	irProg, err := semantic.Analyze(prog)
	if err != nil {
		t.Fatalf("семантический анализ: %v", err)
	}
	in, _, err := newInterp(irProg, io.Discard, strings.NewReader(""))
	if err != nil {
		t.Fatalf("newInterp: %v", err)
	}
	return in
}

// TestFFIStructFieldPointerAndCallback - "Указатель<T>" И "Вызываемый(...)"
// ПОЛЕМ "внешняя структура" (самоссылающийся список в духе curl_slist, и
// WNDCLASS.lpfnWndProc-стиль) - маршалинг проверен напрямую (setFFIValue/
// readFFIValue), не через настоящий dlopen: под рукой нет готовой
// C-библиотеки с таким layout - тот же механизм (сырой указатель/трамплин
// колбэка в поле) уже используется настоящим dlopen'ом для ПАРАМЕТРА/
// РЕЗУЛЬТАТА внешней функции (TestRunAllExamplesMatchCompiledOutput/
// основы/внешняя_функция_указатель.rech, TestRunCallsExternFuncCallbackArg) -
// здесь проверяется, что то же самое верно и внутри поля структуры. У
// cbackend - зеркальная проверка живой компиляцией/запуском (см.
// internal/backend/cbackend, generateIREntityStruct/generateIRNewExpr).
func TestFFIStructFieldPointerAndCallback(t *testing.T) {
	in := buildInterp(t, `
внешняя структура ОкноКласс как КлассОкна
{
    имеет:
        стиль: Целое
        обработчик: Вызываемый(Целое): Целое
        данные: Указатель<Целое>
}

функция главная { }
`)
	ent := in.entities["КлассОкна"]
	if ent == nil || !ent.IsExternStruct {
		t.Fatal("внешняя структура \"КлассОкна\" не найдена")
	}
	handlerType, dataType := ent.Fields[1].Type, ent.Fields[2].Type

	structType := ir.Type{Kind: ir.KindEntity, Entity: "КлассОкна"}
	goType, err := in.ffiGoType(structType)
	if err != nil {
		t.Fatalf("ffiGoType: %v", err)
	}

	// "данные" - сырой указатель на реальную Go-переменную (тестовая
	// "чужая" память - unsafe.Pointer от неё валиден на всё время теста, x
	// остаётся живой локальной переменной).
	var x int64 = 123
	dataVal := rawPointerValue(dataType, ir.TypeInt, unsafe.Pointer(&x), in)

	// "обработчик" - обычное Rech-замыкание (удваивает аргумент).
	handlerVal := callableTypeValue(handlerType, func(args []Value, dbg *dbgThread) (Value, error) {
		return intValue(args[0].I * 2), nil
	})

	structVal := Value{
		Type: structType,
		Obj: &entityObj{typeName: "КлассОкна", fields: map[string]Value{
			"стиль":      intValue(1),
			"обработчик": handlerVal,
			"данные":     dataVal,
		}},
	}

	rv := in.newFFIArgValue(structVal, goType)

	if got := *(*int64)(rv.Field(2).UnsafePointer()); got != 123 {
		t.Fatalf("поле \"данные\": получено %d, ожидалось 123", got)
	}

	trampolineAddr := rv.Field(1).Uint()
	if trampolineAddr == 0 {
		t.Fatal("поле \"обработчик\": трамплин не зарегистрирован (адрес 0)")
	}
	fnType := reflect.FuncOf([]reflect.Type{reflect.TypeFor[int64]()}, []reflect.Type{reflect.TypeFor[int64]()}, false)
	fnPtr := reflect.New(fnType)
	purego.RegisterFunc(fnPtr.Interface(), uintptr(trampolineAddr))
	results := fnPtr.Elem().Call([]reflect.Value{reflect.ValueOf(int64(21))})
	if results[0].Int() != 42 {
		t.Fatalf("поле \"обработчик\": вызов трамплина дал %d, ожидалось 42", results[0].Int())
	}

	// readFFIValue - обратное направление: "данные" читается назад как
	// сырой указатель (единственный способ прочитать Указатель<T> ПОЛЕМ,
	// пришедшее из C, см. package doc ffi.go).
	back := in.readFFIValue(rv, structType)
	if got := back.Obj.getField("данные").Ptr.get().I; got != 123 {
		t.Fatalf("readFFIValue, поле \"данные\": получено %d, ожидалось 123", got)
	}
}

// TestFFICallbackPoolMultipleSimultaneousCallbacks - пул слотов
// (ffi_callback.go, ir.FFICallbackPoolSize) даёт НЕСКОЛЬКО ОДНОВРЕМЕННО
// живых замыканий ОДНОЙ сигнатуры, не одно (раньше вторая регистрация
// заменяла бы первую, и оба трамплина отвечали бы значением ВТОРОГО
// замыкания) - обе регистрации выполнены ДО первого вызова любого из двух
// трамплинов, оба трамплина вызваны реальным machine-level вызовом (тот же
// приём, что и TestFFIStructFieldPointerAndCallback - purego.RegisterFunc
// поверх адреса), проверено, что каждый отвечает СВОИМ значением.
func TestFFICallbackPoolMultipleSimultaneousCallbacks(t *testing.T) {
	in := buildInterp(t, `
внешняя структура ОкноКласс как КлассОкна
{
    имеет:
        обработчик: Вызываемый(Целое): Целое
}

функция главная { }
`)
	ent := in.entities["КлассОкна"]
	if ent == nil || !ent.IsExternStruct {
		t.Fatal("внешняя структура \"КлассОкна\" не найдена")
	}
	sigName := ent.Fields[0].Type.Entity

	first := Value{Callable: &callableValue{call: func(args []Value, dbg *dbgThread) (Value, error) {
		return intValue(args[0].I + 10), nil
	}}}
	second := Value{Callable: &callableValue{call: func(args []Value, dbg *dbgThread) (Value, error) {
		return intValue(args[0].I + 20), nil
	}}}

	firstAddr, err := in.registerCallbackArg(sigName, first)
	if err != nil {
		t.Fatalf("registerCallbackArg (первое замыкание): %v", err)
	}
	secondAddr, err := in.registerCallbackArg(sigName, second)
	if err != nil {
		t.Fatalf("registerCallbackArg (второе замыкание): %v", err)
	}
	if firstAddr == secondAddr {
		t.Fatalf("оба замыкания получили ОДИН И ТОТ ЖЕ адрес трамплина (%#x) - пул не разделяет слоты", firstAddr)
	}

	fnType := reflect.FuncOf([]reflect.Type{reflect.TypeFor[int64]()}, []reflect.Type{reflect.TypeFor[int64]()}, false)
	callVia := func(addr uintptr, arg int64) int64 {
		fnPtr := reflect.New(fnType)
		purego.RegisterFunc(fnPtr.Interface(), addr)
		return fnPtr.Elem().Call([]reflect.Value{reflect.ValueOf(arg)})[0].Int()
	}

	// Оба трамплина вызваны ПОСЛЕ того, как ОБА замыкания уже
	// зарегистрированы - если бы регистрация второго молча заменяла слот
	// первого (старая модель), вызов через firstAddr тоже вернул бы 21
	// (5+20), а не 15 (5+10).
	if got := callVia(firstAddr, 5); got != 15 {
		t.Fatalf("первый трамплин: получено %d, ожидалось 15 (своё замыкание - 5+10)", got)
	}
	if got := callVia(secondAddr, 5); got != 25 {
		t.Fatalf("второй трамплин: получено %d, ожидалось 25 (своё замыкание - 5+20)", got)
	}
}

// TestFFICallbackPoolExhausted - регистрация сверх ir.FFICallbackPoolSize
// одновременно живых слотов одной сигнатуры - явная ошибка, не тихая
// подмена/переполнение.
func TestFFICallbackPoolExhausted(t *testing.T) {
	in := buildInterp(t, `
внешняя структура ОкноКласс как КлассОкна
{
    имеет:
        обработчик: Вызываемый(Целое): Целое
}

функция главная { }
`)
	ent := in.entities["КлассОкна"]
	sigName := ent.Fields[0].Type.Entity

	for i := 0; i < ir.FFICallbackPoolSize; i++ {
		v := Value{Callable: &callableValue{call: func(args []Value, dbg *dbgThread) (Value, error) {
			return intValue(0), nil
		}}}
		if _, err := in.registerCallbackArg(sigName, v); err != nil {
			t.Fatalf("регистрация %d/%d: неожиданная ошибка: %v", i+1, ir.FFICallbackPoolSize, err)
		}
	}
	extra := Value{Callable: &callableValue{call: func(args []Value, dbg *dbgThread) (Value, error) {
		return intValue(0), nil
	}}}
	if _, err := in.registerCallbackArg(sigName, extra); err == nil {
		t.Fatal("регистрация сверх пула должна была вернуть ошибку, получено nil")
	}
}
