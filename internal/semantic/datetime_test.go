package semantic

import (
	"strings"
	"testing"
)

func TestAnalyzeDateTimeRoundTrip(t *testing.T) {
	_, err := AnalyzeSource(t, `
функция главная
{
    переменная метка = время()
    переменная дата = время_как_дата(метка)
    вывести дата.год
    вывести дата.месяц
    вывести дата.день
    вывести дата.час
    вывести дата.минута
    вывести дата.секунда
    вывести дату_в_строку(дата)
    утверждать дата_как_время(дата) == метка
}
`)
	if err != nil {
		t.Fatalf("неожиданная ошибка: %v", err)
	}
}

func TestAnalyzeDateTimeRejectsUserFunctionNamedTime(t *testing.T) {
	_, err := AnalyzeSource(t, `
функция время(): Целое { вернуть 1 }
функция главная { вывести 1 }
`)
	if err == nil || !strings.Contains(err.Error(), "зарезервированное имя") {
		t.Fatalf("ожидалась ошибка про зарезервированное имя, получено: %v", err)
	}
}

// "ДатаВремя" - internal/builtins.ShadowableEntityNames: пользователь может
// объявить свою сущность с этим именем (затенение, [[project_entity_shadowing]]) -
// резолвится в СВОЮ сущность (одно поле "x"), не в builtin (год/месяц/...).
func TestAnalyzeDateTimeUserEntityShadowsBuiltin(t *testing.T) {
	prog, err := AnalyzeSource(t, `
сущность ДатаВремя
{
    имеет:
        Целое x
}
функция главная { вывести 1 }
`)
	if err != nil {
		t.Fatalf("неожиданная ошибка: %v", err)
	}
	found := false
	for _, ent := range prog.Entities {
		if ent.Name != "ДатаВремя" {
			continue
		}
		found = true
		if len(ent.Fields) != 1 || ent.Fields[0].Name != "x" {
			t.Fatalf("ожидалась пользовательская сущность ДатаВремя с единственным полем x, получено: %+v", ent.Fields)
		}
	}
	if !found {
		t.Fatalf("сущность ДатаВремя не найдена в ir.Program.Entities")
	}
}
