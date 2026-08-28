package parser

import "testing"

func TestParseExportedFunction(t *testing.T) {
	prog, err := parseSource(t, `
экспортировать функция сумма(а: Целое, б: Целое): Целое
{
    вернуть а + б
}
функция главная { вывести "ок" }
`)
	if err != nil {
		t.Fatalf("неожиданная ошибка: %v", err)
	}
	found := false
	for _, f := range prog.Functions {
		if f.Name == "сумма" {
			found = true
			if !f.Exported {
				t.Fatal("ожидалось Exported == true у экспортируемой функции")
			}
		}
	}
	if !found {
		t.Fatal("не найдена функция \"сумма\"")
	}
}

func TestParseExportedEntity(t *testing.T) {
	prog, err := parseSource(t, `
экспортировать сущность Точка
{
    имеет:
        x: Целое
}
функция главная { вывести "ок" }
`)
	if err != nil {
		t.Fatalf("неожиданная ошибка: %v", err)
	}
	if len(prog.Entities) != 1 || !prog.Entities[0].Exported {
		t.Fatalf("ожидалась одна экспортируемая сущность, получено %+v", prog.Entities)
	}
}

func TestParseExportedEntityNotExportedByDefault(t *testing.T) {
	prog, err := parseSource(t, `
сущность Точка
{
    имеет:
        x: Целое
}
функция главная { вывести "ок" }
`)
	if err != nil {
		t.Fatalf("неожиданная ошибка: %v", err)
	}
	if prog.Entities[0].Exported {
		t.Fatal("сущность без \"экспортировать\" не должна быть Exported")
	}
}

func TestParseExportedMainRejected(t *testing.T) {
	_, err := parseSource(t, `
экспортировать функция главная
{
    вывести "ок"
}
`)
	if err == nil {
		t.Fatal("ожидалась ошибка - точку входа программы нельзя экспортировать")
	}
}

func TestParseExportWithoutDeclRejected(t *testing.T) {
	_, err := parseSource(t, `
экспортировать переменная а = 1
функция главная { вывести "ок" }
`)
	if err == nil {
		t.Fatal("ожидалась ошибка - после \"экспортировать\" ожидается функция/сущность/класс/структура")
	}
}
