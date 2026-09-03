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

// TestAnalyzeDBQueryParamsOptional - третий аргумент "запросить"/"выполнить"
// (параметры запроса) можно опустить - dbNoParamsDefault (db.go), тем же
// механизмом, что и обычные значения по умолчанию параметров функций (см.
// internal/semantic/param_default_test.go). Отдельный тест от
// TestAnalyzeDBQueryParamsOmitted в param_default_test.go - проверяет ещё
// и что при этом не ломается остальной конвейер БД (UsesDB/UsesReflection).
func TestAnalyzeDBQueryParamsOptional(t *testing.T) {
	prog, err := AnalyzeSource(t, `
функция главная
{
    переменная бд = подключиться_к_бд("sqlite::memory:")
    выполнить(бд, "CREATE TABLE t (x INTEGER)")
    переменная строки = запросить(бд, "SELECT * FROM t")
    вывести строки.длина()
    закрыть_бд(бд)
}
`)
	if err != nil {
		t.Fatalf("неожиданная ошибка: %v", err)
	}
	if !prog.UsesDB {
		t.Fatal("UsesDB должен быть true")
	}
}

// TestAnalyzeDBRowsAsListEntity - "запросить(...) как Список<Тип>"
// (db_rows.go) - маппинг строк результата в список сущностей по имени поля.
func TestAnalyzeDBRowsAsListEntity(t *testing.T) {
	prog, err := AnalyzeSource(t, `
сущность Пользователь
{
    имеет:
        username: Строка
        password: Строка
}
функция главная
{
    переменная бд = подключиться_к_бд("sqlite::memory:")
    переменная пользователи = запросить(бд, "SELECT * FROM users") как Список<Пользователь>
    вывести пользователи.длина()
    закрыть_бд(бд)
}
`)
	if err != nil {
		t.Fatalf("неожиданная ошибка: %v", err)
	}
	found := false
	for _, t := range prog.DBRowTargets {
		if t.Entity == "Пользователь" {
			found = true
		}
	}
	if !found {
		t.Fatal("ожидался \"Пользователь\" в prog.DBRowTargets")
	}
}

// TestAnalyzeDBRowsAsRejectsCtorEntity - цель "запросить(...) как
// Список<Тип>" не может иметь собственный явный конструктор (маппинг
// заполняет поля напрямую, тем же принципом, что и JSON-разбор).
func TestAnalyzeDBRowsAsRejectsCtorEntity(t *testing.T) {
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
    переменная бд = подключиться_к_бд("sqlite::memory:")
    переменная пользователи = запросить(бд, "SELECT * FROM users") как Список<Пользователь>
    закрыть_бд(бд)
}
`)
	if err == nil || !strings.Contains(err.Error(), "конструктор") {
		t.Fatalf("ожидалась ошибка про собственный конструктор, получено: %v", err)
	}
}

// TestAnalyzeDBRowsAsRejectsNestedEntityField - поле цели "запросить(...)
// как Список<Тип>" должно быть примитивом/nullable-примитивом - вложенная
// сущность недопустима (строка SQL-результата плоская).
func TestAnalyzeDBRowsAsRejectsNestedEntityField(t *testing.T) {
	_, err := AnalyzeSource(t, `
сущность Адрес
{
    имеет:
        город: Строка
}
сущность Пользователь
{
    имеет:
        адрес: Адрес
}
функция главная
{
    переменная бд = подключиться_к_бд("sqlite::memory:")
    переменная пользователи = запросить(бд, "SELECT * FROM users") как Список<Пользователь>
    закрыть_бд(бд)
}
`)
	if err == nil || !strings.Contains(err.Error(), "поле \"адрес\"") {
		t.Fatalf("ожидалась ошибка про недопустимый тип поля, получено: %v", err)
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
