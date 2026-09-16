package cbackend

import (
	"runtime"
	"strings"
	"testing"

	"github.com/IlyaFeoktistov/Rech/internal/lexer"
	"github.com/IlyaFeoktistov/Rech/internal/parser"
	"github.com/IlyaFeoktistov/Rech/internal/semantic"
)

// TestExternPrototypeSkippedWhenHeaderAlreadyIncluded - найденный живьём
// баг ("FFI: конфликт extern-прототипов со стандартными
// заголовками", examples/основы/внешние_функции.rech): "strlen" объявлена
// вручную ("внешняя функция strlen как длина_байт"), а <string.h>
// включается программой ПО СОВСЕМ ДРУГОЙ причине (memcpy захваченной
// лямбды, ctx.usesMemcpy) - собственный extern-прототип с угаданным типом
// (long long, не size_t настоящего glibc) конфликтовал бы с прототипом из
// заголовка. externPrototypeAlreadyProvided (extern_headers.go) должен
// пропустить собственную декларацию.
func TestExternPrototypeSkippedWhenHeaderAlreadyIncluded(t *testing.T) {
	out := generateFromSrc(t, `
внешняя функция strlen как длина_байт(текст: Строка): Целое
внешняя функция atexit(f: Вызываемый())

функция главная
{
    вывести длина_байт("привет")
    переменная счётчик = 0
    atexit(функция() {
        счётчик = счётчик + 1
        вывести счётчик
    })
}
`)
	if !strings.Contains(out, "#include <string.h>") {
		t.Fatalf("ожидалось \"#include <string.h>\" (memcpy захваченной лямбды), получено:\n%s", out)
	}
	if strings.Contains(out, "extern long long strlen(") {
		t.Fatalf("не ожидался собственный extern-прототип strlen - <string.h> уже объявляет его, получено:\n%s", out)
	}
	if !strings.Contains(out, "strlen(rech_v_") && !strings.Contains(out, "strlen(\"") {
		t.Fatalf("вызов strlen должен остаться в теле (просто без своего прототипа), получено:\n%s", out)
	}
}

// TestExternPrototypeKeptWhenHeaderNotIncluded - тот же strlen, но БЕЗ
// ничего, что включило бы <string.h> по другой причине - собственный
// extern-прототип остаётся как есть (единственный источник декларации).
// На Windows - НАОБОРОТ: <windows.h> (включённый безусловно на этой
// платформе) сам транзитивно тянет <string.h> (MinGW: windows.h -> ... ->
// guiddef.h -> string.h), так что собственный прототип НЕ пишется, даже
// когда ctx.includesStringH сам по себе false (см. externSymbolHeader,
// extern_headers.go, живой баг - "conflicting types for 'strlen'" на
// настоящей Windows).
func TestExternPrototypeKeptWhenHeaderNotIncluded(t *testing.T) {
	out := generateFromSrc(t, `
внешняя функция strlen как длина_байт(текст: Строка): Целое
функция главная { вывести длина_байт("привет") }
`)
	if runtime.GOOS == "windows" {
		if strings.Contains(out, "extern long long strlen(") {
			t.Fatalf("не ожидался собственный extern-прототип strlen на Windows (<windows.h> транзитивно тянет <string.h>), получено:\n%s", out)
		}
		return
	}
	if !strings.Contains(out, "extern long long strlen(") {
		t.Fatalf("ожидался собственный extern-прототип strlen (string.h не включён), получено:\n%s", out)
	}
}

// TestExternPrototypeSkippedForAnySymbolWhenProjectHeadersConfigured -
// "сборка.заголовки" (internal/project.File.Заголовки, ir.Program.
// ExternHeaders) - в ОТЛИЧИЕ от TestExternPrototypeSkippedWhenHeaderAlreadyIncluded
// выше (курируемый externSymbolHeader, только несколько известных имён),
// проект, явно объявивший хотя бы один заголовок, доверяется ЦЕЛИКОМ -
// "extern функция" на ЛЮБОЕ (не курируемое) имя не получает собственного
// прототипа, и сам заголовок эмитится "#include <...>" - см. package doc
// externPrototypeAlreadyProvided.
func TestExternPrototypeSkippedForAnySymbolWhenProjectHeadersConfigured(t *testing.T) {
	toks, err := lexer.New(`
внешняя функция НечтоНеКурируемое как позвать(): Целое
функция главная { вывести позвать() }
`).Tokenize()
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
	irProg.ExternHeaders = []string{"придуманный_заголовок.h"}
	out, err := GenerateFromIR(irProg)
	if err != nil {
		t.Fatalf("GenerateFromIR: %v", err)
	}
	if !strings.Contains(out, "#include <придуманный_заголовок.h>") {
		t.Fatalf("ожидался \"#include <придуманный_заголовок.h>\", получено:\n%s", out)
	}
	if strings.Contains(out, "extern long long НечтоНеКурируемое(") {
		t.Fatalf("не ожидался собственный extern-прототип - проект сам объявил заголовок, получено:\n%s", out)
	}
}
