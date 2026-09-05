package semantic

import (
	"strings"
	"testing"
)

func TestAnalyzeExternEntityInSignature(t *testing.T) {
	_, err := AnalyzeSource(t, `
внешняя сущность void как Дескриптор
внешняя функция make_handle как создать(v: Целое): Дескриптор
внешняя функция use_handle как применить(д: Дескриптор): Целое
функция главная { вывести применить(создать(1)) }
`)
	if err != nil {
		t.Fatalf("неожиданная ошибка: %v", err)
	}
}

func TestAnalyzeExternEntityNewRejected(t *testing.T) {
	_, err := AnalyzeSource(t, `
внешняя сущность void как Дескриптор
функция главная { переменная д = новый Дескриптор() }
`)
	if err == nil || !strings.Contains(err.Error(), "непрозрачный") {
		t.Fatalf("ожидалась ошибка про непрозрачный тип, получено: %v", err)
	}
}

func TestAnalyzeExternStructPassedByValue(t *testing.T) {
	_, err := AnalyzeSource(t, `
внешняя структура point как Точка
{
    имеет:
        x: Целое
        y: Целое
}
внешняя функция point_sum как сумма_точки(p: Точка): Целое
функция главная { вывести сумма_точки(новая Точка(x: 1, y: 2)) }
`)
	if err != nil {
		t.Fatalf("неожиданная ошибка: %v", err)
	}
}

func TestAnalyzeExternStructRejectsUnsafeFieldType(t *testing.T) {
	_, err := AnalyzeSource(t, `
внешняя структура p как Точка
{
    имеет:
        список: Список<Целое>
}
функция главная { вывести 1 }
`)
	if err == nil || !strings.Contains(err.Error(), "внешняя структура") {
		t.Fatalf("ожидалась ошибка про недопустимый тип поля, получено: %v", err)
	}
}

// TestAnalyzeExternFuncAcceptsPointerParam - "Указатель<T>" над примитивом
// допустима в сигнатуре внешней функции (docs/ROADMAP.md, "FFI: указатели
// в внешняя структура/внешняя функция") - типичный out-параметр C API.
func TestAnalyzeExternFuncAcceptsPointerParam(t *testing.T) {
	_, err := AnalyzeSource(t, `
внешняя функция modf как разбить_на_части(x: Действительное, целая: Указатель<Действительное>): Действительное
функция главная { переменная ц = 0.0; вывести разбить_на_части(3.5, &ц) }
`)
	if err != nil {
		t.Fatalf("неожиданная ошибка: %v", err)
	}
}

// TestAnalyzeExternFuncRejectsPointerToUnsafeEntity - "Указатель<T>" сама
// FFI-безопасна, только если T тоже FFI-безопасен - обычная (не внешняя)
// сущность внутри "Указатель<...>" по-прежнему отклоняется, её C-
// представление (GC-структура с vtable) не то, что ожидает сторонний C API.
func TestAnalyzeExternFuncRejectsPointerToUnsafeEntity(t *testing.T) {
	_, err := AnalyzeSource(t, `
сущность Штука
{
    имеет:
        поле: Целое
}
внешняя функция что_то(п: Указатель<Штука>)
функция главная { вывести 1 }
`)
	if err == nil || !strings.Contains(err.Error(), "внешней функции") {
		t.Fatalf("ожидалась ошибка про недопустимый тип параметра, получено: %v", err)
	}
}

func TestAnalyzeExternEntityNameCollisionWithEntityRejected(t *testing.T) {
	_, err := AnalyzeSource(t, `
внешняя сущность void как Точка
сущность Точка
{
    имеет:
        x: Целое
}
функция главная { вывести 1 }
`)
	if err == nil {
		t.Fatal("ожидалась ошибка про коллизию имён")
	}
}
