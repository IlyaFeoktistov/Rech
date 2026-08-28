package semantic

import (
	"strings"
	"testing"
)

func TestAnalyzeDBRoundTrip(t *testing.T) {
	prog, err := AnalyzeSource(t, `
функция главная
{
    переменная бд = подключиться_к_бд("sqlite::memory:")
    переменная параметры = новый Список<Значение>()
    параметры.добавить((Значение) 1)
    переменная строки = запросить(бд, "SELECT ?", параметры)
    вывести строки.длина()
    переменная затронуто = выполнить(бд, "SELECT 1", параметры)
    вывести затронуто
    закрыть_бд(бд)
}
`)
	if err != nil {
		t.Fatalf("неожиданная ошибка: %v", err)
	}
	if !prog.UsesDB {
		t.Fatal("UsesDB должен быть true")
	}
	if !prog.UsesReflection {
		t.Fatal("UsesReflection должен быть true - результат несёт Значение")
	}
	wantGenerics := map[string]bool{
		"Список<Значение>":                  false,
		"Словарь<Строка, Значение>":         false,
		"Список<Словарь<Строка, Значение>>": false,
	}
	for _, g := range prog.Generics {
		if _, ok := wantGenerics[g.Name]; ok {
			wantGenerics[g.Name] = true
		}
	}
	for name, found := range wantGenerics {
		if !found {
			t.Fatalf("ожидалась generic-инстанциация %q в prog.Generics", name)
		}
	}
}

func TestAnalyzeDBAbsentWithoutUse(t *testing.T) {
	prog, err := AnalyzeSource(t, `
функция главная { вывести 1 }
`)
	if err != nil {
		t.Fatalf("неожиданная ошибка: %v", err)
	}
	if prog.UsesDB {
		t.Fatal("UsesDB не должен взводиться без единого вызова функций БД")
	}
	for _, g := range prog.Generics {
		if g.Name == "Список<Словарь<Строка, Значение>>" {
			t.Fatal("Список<Словарь<Строка, Значение>> не должен регистрироваться без вызова \"запросить\"/\"выполнить\"")
		}
	}
}

func TestAnalyzeDBRejectsUserFunctionNamedПодключитьсяКБД(t *testing.T) {
	_, err := AnalyzeSource(t, `
функция подключиться_к_бд(строка: Строка): Целое { вернуть 1 }
функция главная { вывести 1 }
`)
	if err == nil || !strings.Contains(err.Error(), "зарезервированное имя") {
		t.Fatalf("ожидалась ошибка про зарезервированное имя, получено: %v", err)
	}
}

func TestAnalyzeDBRejectsUserFunctionNamedЗапросить(t *testing.T) {
	_, err := AnalyzeSource(t, `
функция запросить(): Целое { вернуть 1 }
функция главная { вывести 1 }
`)
	if err == nil || !strings.Contains(err.Error(), "зарезервированное имя") {
		t.Fatalf("ожидалась ошибка про зарезервированное имя, получено: %v", err)
	}
}
