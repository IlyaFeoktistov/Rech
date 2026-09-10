package semantic

import (
	"strings"
	"testing"
)

// TestAnalyzeEntityEqualityComparesByReference - НАЙДЕННЫЙ по ходу
// examples/проекты/чат_сервер недочёт (docs/ROADMAP.md): "=="/"!=" не
// поддерживался ни для одной сущности вовсе. Сущность/класс (ссылочная
// семантика) теперь сравнивается по ссылке - тем же принципом, что
// entityIsRefType (cast.go).
func TestAnalyzeEntityEqualityComparesByReference(t *testing.T) {
	_, err := AnalyzeSource(t, `
сущность Точка
{
    имеет:
        x: Целое

    конструктор(x: Целое)
    {
        этот.x = x
    }
}
функция главная
{
    переменная а = новая Точка(1)
    переменная б = а
    утверждать а == б
    утверждать не (а == новая Точка(1))
    утверждать а != новая Точка(1)
}
`)
	if err != nil {
		t.Fatalf("неожиданная ошибка: %v", err)
	}
}

// TestAnalyzeStructEqualityStillRejected - "структура" (значение, не
// ссылка) сознательно НЕ включена - потребовалось бы поле-в-поле
// сравнение, отдельная задача.
func TestAnalyzeStructEqualityStillRejected(t *testing.T) {
	_, err := AnalyzeSource(t, `
структура Точка
{
    имеет:
        x: Целое
}
функция главная
{
    переменная а = новая Точка(1)
    переменная б = новая Точка(1)
    вывести а == б
}
`)
	if err == nil || !strings.Contains(err.Error(), "не поддерживается для типов") {
		t.Fatalf("ожидалась ошибка про неподдерживаемое сравнение структур, получено: %v", err)
	}
}

// TestAnalyzeGenericCollectionEqualityComparesByReference - Список<T> и
// подобные - тоже ссылочные (ctx.entityIsRef, cbackend), значит тот же
// принцип: "==" сравнивает, тот ли это САМЫЙ список, не совпадение
// содержимого.
func TestAnalyzeGenericCollectionEqualityComparesByReference(t *testing.T) {
	_, err := AnalyzeSource(t, `
функция главная
{
    переменная а = новый Список<Целое>()
    переменная б = а
    переменная третий = новый Список<Целое>()
    утверждать а == б
    утверждать а != третий
}
`)
	if err != nil {
		t.Fatalf("неожиданная ошибка: %v", err)
	}
}

// TestAnalyzeEntityEqualityRequiresSameType - разные типы сущностей - всё
// ещё ошибка компиляции (та же ветка "lt != rt", что и раньше).
func TestAnalyzeEntityEqualityRequiresSameType(t *testing.T) {
	_, err := AnalyzeSource(t, `
сущность А {}
сущность Б {}
функция главная
{
    переменная а = новая А()
    переменная б = новый Б()
    вывести а == б
}
`)
	if err == nil || !strings.Contains(err.Error(), "не поддерживается для типов") {
		t.Fatalf("ожидалась ошибка про несовпадающие типы, получено: %v", err)
	}
}
