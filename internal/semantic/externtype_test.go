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
