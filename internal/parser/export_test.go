package parser

import (
	"testing"

	"github.com/IlyaFeoktistov/Rech/internal/ast"
)

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

// TestParseExportedMethodAndConstructor - "экспортировать" инлайн перед
// методом и перед конструктором внутри "экспортировать сущность" (docs/
// SPEC.md, "Закрытые (бинарные) пакеты").
func TestParseExportedMethodAndConstructor(t *testing.T) {
	prog, err := parseSource(t, `
экспортировать сущность Точка
{
    имеет:
        x: Целое

    экспортировать конструктор(x: Целое)
    {
        этот.x = x
    }

    экспортировать координата(): Целое
    {
        вернуть этот.x
    }

    приватный внутренний_метод(): Целое
    {
        вернуть этот.x
    }
}
функция главная { вывести "ок" }
`)
	if err != nil {
		t.Fatalf("неожиданная ошибка: %v", err)
	}
	ent := prog.Entities[0]
	if ent.Ctor == nil || !ent.Ctor.Exported {
		t.Fatalf("ожидался экспортированный конструктор, получено %+v", ent.Ctor)
	}
	var координата, внутренний *ast.MethodDecl
	for _, m := range ent.Methods {
		switch m.Name {
		case "координата":
			координата = m
		case "внутренний_метод":
			внутренний = m
		}
	}
	if координата == nil || !координата.Exported {
		t.Fatalf("ожидался экспортированный метод \"координата\", получено %+v", координата)
	}
	if внутренний == nil || внутренний.Exported {
		t.Fatalf("метод \"внутренний_метод\" без \"экспортировать\" не должен быть Exported, получено %+v", внутренний)
	}
}

func TestParseExportedStaticMethodRejected(t *testing.T) {
	_, err := parseSource(t, `
экспортировать сущность Точка
{
    экспортировать статический создать(): Целое
    {
        вернуть 1
    }
}
функция главная { вывести "ок" }
`)
	if err == nil {
		t.Fatal("ожидалась ошибка - экспортированный метод не может быть статическим (нет получателя)")
	}
}

// TestParseExportListBareRepeated - списковая форма (альтернатива инлайну,
// docs/SPEC.md), повторением "экспортировать <путь>" для каждого члена.
func TestParseExportListBareRepeated(t *testing.T) {
	prog, err := parseSource(t, `
сущность Точка
{
    имеет:
        x: Целое

    конструктор(x: Целое)
    {
        этот.x = x
    }

    координата(): Целое
    {
        вернуть этот.x
    }
}

экспортировать Точка
экспортировать Точка.конструктор
экспортировать Точка.координата
функция главная { вывести "ок" }
`)
	if err != nil {
		t.Fatalf("неожиданная ошибка: %v", err)
	}
	ent := prog.Entities[0]
	if !ent.Exported {
		t.Fatal("ожидалось Exported == true у сущности из списка")
	}
	if ent.Ctor == nil || !ent.Ctor.Exported {
		t.Fatal("ожидался экспортированный конструктор из списка")
	}
	if !ent.Methods[0].Exported {
		t.Fatal("ожидался экспортированный метод из списка")
	}
}

// TestParseExportListBlock - списковая форма одним блоком в скобках.
func TestParseExportListBlock(t *testing.T) {
	prog, err := parseSource(t, `
сущность Точка
{
    имеет:
        x: Целое

    координата(): Целое
    {
        вернуть этот.x
    }

    другая(): Целое
    {
        вернуть этот.x
    }
}

экспортировать(
    Точка
    Точка.координата, Точка.другая
)
функция главная { вывести "ок" }
`)
	if err != nil {
		t.Fatalf("неожиданная ошибка: %v", err)
	}
	ent := prog.Entities[0]
	if !ent.Exported || !ent.Methods[0].Exported || !ent.Methods[1].Exported {
		t.Fatalf("ожидались все три пути из блока применёнными, получено %+v", ent)
	}
}

func TestParseExportListForwardReferenceRejected(t *testing.T) {
	_, err := parseSource(t, `
экспортировать Точка
сущность Точка
{
    имеет:
        x: Целое
}
функция главная { вывести "ок" }
`)
	if err == nil {
		t.Fatal("ожидалась ошибка - список \"экспортировать\" видит только объявления ВЫШЕ по файлу")
	}
}

func TestParseExportListUnknownMemberRejected(t *testing.T) {
	_, err := parseSource(t, `
сущность Точка
{
    имеет:
        x: Целое
}
экспортировать Точка.нет_такого
функция главная { вывести "ок" }
`)
	if err == nil {
		t.Fatal("ожидалась ошибка - у сущности нет такого метода")
	}
}
