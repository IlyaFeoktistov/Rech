package interp

import (
	"io"
	"strings"
	"testing"
	"unsafe"

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

// TestFFIStructFieldPointer - "Указатель<T>" ПОЛЕМ "внешняя структура"
// (например, самоссылающийся связный список в духе curl_slist) - маршалинг
// проверен напрямую (setFFIValue/readFFIValue), не через настоящий dlopen:
// под рукой нет готовой C-библиотеки с таким layout - сам механизм (сырой
// указатель в поле через unsafe.Pointer/reflect) уже используется
// настоящим dlopen'ом для ПАРАМЕТРА/РЕЗУЛЬТАТА внешней функции (см.
// TestRunAllExamplesMatchCompiledOutput/основы/внешняя_функция_указатель.rech,
// readRawPointerValue) - здесь проверяется, что то же самое верно и внутри
// поля структуры (семантически уже разрешено - isFFIType, docs/SPEC.md,
// "Указатель-поле... уже представимо"; "Вызываемый(...)" ПОЛЕМ - отдельный,
// более крупный отложенный вопрос - см. project_full_ffi_backlog, требует
// изменений и в cbackend, не только в interp, семантический анализ сам
// отклоняет такое поле сегодня для обоих backend'ов).
func TestFFIStructFieldPointer(t *testing.T) {
	in := buildInterp(t, `
внешняя структура Узел как УзелСписка
{
    имеет:
        значение: Целое
        следующий: Указатель<Целое>
}

функция главная { }
`)
	ent := in.entities["УзелСписка"]
	if ent == nil || !ent.IsExternStruct {
		t.Fatal("внешняя структура \"УзелСписка\" не найдена")
	}
	dataType := ent.Fields[1].Type

	structType := ir.Type{Kind: ir.KindEntity, Entity: "УзелСписка"}
	goType, err := in.ffiGoType(structType)
	if err != nil {
		t.Fatalf("ffiGoType: %v", err)
	}

	// "следующий" - сырой указатель на реальную Go-переменную (тестовая
	// "чужая" память - unsafe.Pointer от неё валиден на всё время теста,
	// x остаётся живой локальной переменной).
	var x int64 = 123
	dataVal := rawPointerValue(dataType, ir.TypeInt, unsafe.Pointer(&x), in)

	structVal := Value{
		Type: structType,
		Obj: &entityObj{typeName: "УзелСписка", fields: map[string]Value{
			"значение":  intValue(1),
			"следующий": dataVal,
		}},
	}

	rv := in.newFFIArgValue(structVal, goType)
	if got := *(*int64)(rv.Field(1).UnsafePointer()); got != 123 {
		t.Fatalf("поле \"следующий\": получено %d, ожидалось 123", got)
	}

	// readFFIValue - обратное направление: поле читается назад как сырой
	// указатель (единственный способ прочитать Указатель<T> ПОЛЕМ,
	// пришедшее из C, см. package doc ffi.go).
	back := in.readFFIValue(rv, structType)
	if got := back.Obj.getField("следующий").Ptr.get().I; got != 123 {
		t.Fatalf("readFFIValue, поле \"следующий\": получено %d, ожидалось 123", got)
	}
}
