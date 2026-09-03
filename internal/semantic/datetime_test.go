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

func TestAnalyzeDateTimeRejectsUserEntityNamedDateTime(t *testing.T) {
	_, err := AnalyzeSource(t, `
сущность ДатаВремя
{
    имеет:
        x: Целое
}
функция главная { вывести 1 }
`)
	if err == nil || !strings.Contains(err.Error(), "зарезервированное имя") {
		t.Fatalf("ожидалась ошибка про зарезервированное имя, получено: %v", err)
	}
}
