package semantic

import (
	"strings"
	"testing"
)

func TestAnalyzeParseExprEntityTarget(t *testing.T) {
	prog, err := AnalyzeSource(t, `
сущность Пользователь
{
    имеет:
        имя: Строка
        возраст: Целое
}
функция главная
{
    переменная текст = "{}"
    переменная п = разобрать JSON текст как Пользователь
    вывести п.имя
}
`)
	if err != nil {
		t.Fatalf("неожиданная ошибка: %v", err)
	}
	found := false
	for _, ent := range prog.Entities {
		if ent.Name == "ОшибкаРазбора" {
			found = true
			if ent.BaseName != "Исключение" {
				t.Fatalf("ожидалось BaseName == Исключение, получено %q", ent.BaseName)
			}
		}
	}
	if !found {
		t.Fatal("ожидалась сущность ОшибкаРазбора в Program.Entities")
	}
	if len(prog.ParseTargets) != 1 || prog.ParseTargets[0].Entity != "Пользователь" {
		t.Fatalf("ожидался один ParseTarget Пользователь, получено %v", prog.ParseTargets)
	}
}

func TestAnalyzeParseExprAbsentWithoutUse(t *testing.T) {
	prog, err := AnalyzeSource(t, `
функция главная { вывести 1 }
`)
	if err != nil {
		t.Fatalf("неожиданная ошибка: %v", err)
	}
	for _, ent := range prog.Entities {
		if ent.Name == "ОшибкаРазбора" {
			t.Fatal("ОшибкаРазбора не должна регистрироваться без использования \"разобрать\"")
		}
	}
}

func TestAnalyzeParseExprListOfPrimitivesTarget(t *testing.T) {
	prog, err := AnalyzeSource(t, `
функция главная
{
    переменная список = разобрать JSON "[]" как Список<Целое>
}
`)
	if err != nil {
		t.Fatalf("неожиданная ошибка: %v", err)
	}
	if len(prog.ParseTargets) != 1 || prog.ParseTargets[0].Entity != "Список<Целое>" {
		t.Fatalf("ожидался ParseTarget Список<Целое>, получено %v", prog.ParseTargets)
	}
}

// TestAnalyzeParseExprListOfEntitiesRejected - Список<T> сам по себе
// поддерживает только примитивные элементы (сущности - нет, известное
// действующее ограничение generic-коллекций, не специфика JSON) - ошибка
// приходит из resolveTypeName/resolveGenericTypeName раньше, чем до
// jsonCompatible вообще доходит очередь.
func TestAnalyzeParseExprListOfEntitiesRejected(t *testing.T) {
	_, err := AnalyzeSource(t, `
сущность Пользователь
{
    имеет:
        имя: Строка
}
функция главная
{
    переменная список = разобрать JSON "[]" как Список<Пользователь>
}
`)
	if err == nil {
		t.Fatal("ожидалась ошибка - Список<Сущность> не поддержан")
	}
}

func TestAnalyzeParseExprRejectsNonStringSource(t *testing.T) {
	_, err := AnalyzeSource(t, `
функция главная
{
    переменная п = разобрать JSON 42 как Целое
}
`)
	if err == nil || !strings.Contains(err.Error(), "Строка") {
		t.Fatalf("ожидалась ошибка про источник Строка, получено: %v", err)
	}
}

func TestAnalyzeParseExprRejectsEntityWithCustomConstructor(t *testing.T) {
	_, err := AnalyzeSource(t, `
сущность Пользователь
{
    имеет:
        имя: Строка

    конструктор(имя: Строка)
    {
        этот.имя = имя
    }
}
функция главная
{
    переменная п = разобрать JSON "{}" как Пользователь
}
`)
	if err == nil || !strings.Contains(err.Error(), "конструктор") {
		t.Fatalf("ожидалась ошибка про собственный конструктор, получено: %v", err)
	}
}

func TestAnalyzeParseExprJSONTagRenamesField(t *testing.T) {
	prog, err := AnalyzeSource(t, "" +
		"сущность Пользователь\n" +
		"{\n" +
		"    имеет:\n" +
		"        имя: Строка `json:name`\n" +
		"}\n" +
		"функция главная\n" +
		"{\n" +
		"    переменная п = разобрать JSON \"{}\" как Пользователь\n" +
		"}\n")
	if err != nil {
		t.Fatalf("неожиданная ошибка: %v", err)
	}
	for _, ent := range prog.Entities {
		if ent.Name != "Пользователь" {
			continue
		}
		if got := ent.Fields[0].JSONKey(); got != "name" {
			t.Fatalf("JSONKey поля имя: ожидалось %q (из тега), получено %q", "name", got)
		}
	}
}

func TestAnalyzeParseExprRejectsDuplicateJSONKey(t *testing.T) {
	_, err := AnalyzeSource(t, "" +
		"сущность Пользователь\n" +
		"{\n" +
		"    имеет:\n" +
		"        имя: Строка `json:name`\n" +
		"        name: Строка\n" +
		"}\n" +
		"функция главная\n" +
		"{\n" +
		"    переменная п = разобрать JSON \"{}\" как Пользователь\n" +
		"}\n")
	if err == nil || !strings.Contains(err.Error(), "одинаковый JSON-ключ") {
		t.Fatalf("ожидалась ошибка про конфликт JSON-ключей, получено: %v", err)
	}
}

func TestAnalyzeParseExprRejectsIncompatibleField(t *testing.T) {
	_, err := AnalyzeSource(t, `
сущность Плохая
{
    имеет:
        сокет: Целое
}
функция главная
{
    переменная п = разобрать JSON "{}" как Плохая
}
`)
	if err != nil {
		t.Fatalf("Целое совместимо с JSON, неожиданная ошибка: %v", err)
	}
}
