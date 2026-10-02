package parser

import (
	"strings"
	"testing"

	"github.com/IlyaFeoktistov/Rech/internal/ast"
)

func TestParseProtectedFieldAgreesWithFieldGender(t *testing.T) {
	prog, err := parseSource(t, `
сущность Животное
{
    имеет:
        защищённая Строка имя
        защищённая Строка порода
}
функция главная { вывести "ок" }
`)
	if err != nil {
		t.Fatalf("неожиданная ошибка: %v", err)
	}
	fields := prog.Entities[0].Fields
	if len(fields) != 2 || fields[0].Visibility != ast.Protected || fields[1].Visibility != ast.Protected {
		t.Fatalf("ожидались два защищённых поля, получено %+v", fields)
	}
}

func TestParseProtectedFieldWrongGenderRejected(t *testing.T) {
	_, err := parseSource(t, `
сущность Животное
{
    имеет:
        защищённый Строка порода
}
функция главная { вывести "ок" }
`)
	if err == nil || !strings.Contains(err.Error(), "защищённая") {
		t.Fatalf("ожидалась ошибка о согласовании рода (\"защищённая Строка\"), получено: %v", err)
	}
}

func TestParseProtectedMethodOnlyMasculineForm(t *testing.T) {
	prog, err := parseSource(t, `
сущность Животное
{
    защищённый издать_звук(): Строка
    {
        вернуть "..."
    }
}
функция главная { вывести "ок" }
`)
	if err != nil {
		t.Fatalf("неожиданная ошибка: %v", err)
	}
	if len(prog.Entities[0].Methods) != 1 || prog.Entities[0].Methods[0].Visibility != ast.Protected {
		t.Fatalf("ожидался один защищённый метод, получено %+v", prog.Entities[0].Methods)
	}
}

func TestParseProtectedConstantAgreesWithConstWord(t *testing.T) {
	prog, err := parseSource(t, `
сущность Настройки
{
	защищённая константа ВЕРСИЯ = 1
}
`)
	if err != nil {
		t.Fatalf("неожиданная ошибка: %v", err)
	}
	c := prog.Entities[0].Constants[0]
	if c.Name != "ВЕРСИЯ" || c.Visibility != ast.Protected {
		t.Fatalf("ожидалась защищённая константа ВЕРСИЯ, получено %+v", c)
	}
}
