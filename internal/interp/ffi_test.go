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
	fnType := reflect.FuncOf([]reflect.Type{reflect.TypeOf(int64(0))}, []reflect.Type{reflect.TypeOf(int64(0))}, false)
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
