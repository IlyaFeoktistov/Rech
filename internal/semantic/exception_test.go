package semantic

import (
	"strings"
	"testing"

	"github.com/IlyaFeoktistov/Rech/internal/ir"
)

func TestAnalyzeThrowCatchSingleTypeBindsExactCatchVarType(t *testing.T) {
	prog := parseProgram(t, `
сущность ОшибкаДеления наследует Исключение
{
    конструктор(сообщение: Строка)
    {
        родитель(сообщение)
    }
}
функция главная
{
    попытаться {
        выбросить новая ОшибкаДеления("ноль")
    } поймать(иск: ОшибкаДеления) {
        вывести иск.сообщение
    }
}
`)
	fn := findFunc(prog, "главная")
	try := fn.Body[0].(*ir.TryStmt)
	if try.CatchVarType != ir.EntityType("ОшибкаДеления") {
		t.Fatalf("ожидался тип ОшибкаДеления у переменной поймать, получено %s", try.CatchVarType)
	}
}

func TestAnalyzeThrowCatchUnionTypeBindsSystemExceptionEntity(t *testing.T) {
	prog := parseProgram(t, `
сущность ОшибкаВвода наследует Исключение
{
    конструктор(сообщение: Строка)
    {
        родитель(сообщение)
    }
}
сущность ОшибкаВывода наследует Исключение
{
    конструктор(сообщение: Строка)
    {
        родитель(сообщение)
    }
}
функция главная
{
    попытаться {
        выбросить новая ОшибкаВвода("а")
    } поймать(иск: ОшибкаВвода|ОшибкаВывода) {
        вывести иск.сообщение
    }
}
`)
	fn := findFunc(prog, "главная")
	try := fn.Body[0].(*ir.TryStmt)
	if try.CatchVarType != ir.EntityType("Исключение") {
		t.Fatalf("ожидался общий тип Исключение у переменной поймать (несколько типов в объединении), получено %s", try.CatchVarType)
	}
}

func TestAnalyzeThrowRejectsNonExceptionType(t *testing.T) {
	_, err := AnalyzeSource(t, `
сущность Точка
{
    имеет:
        x: Целое = 0
}
функция главная
{
    выбросить новая Точка()
}
`)
	if err == nil || !strings.Contains(err.Error(), "выбросить") {
		t.Fatalf("ожидалась ошибка - Точка не наследует Исключение, получено: %v", err)
	}
}

func TestAnalyzeCatchRejectsNonExceptionType(t *testing.T) {
	_, err := AnalyzeSource(t, `
сущность Точка
{
    имеет:
        x: Целое = 0
}
функция главная
{
    попытаться {
        вывести "тело"
    } поймать(иск: Точка) {
        вывести "поймано"
    }
}
`)
	if err == nil || !strings.Contains(err.Error(), "поймать") {
		t.Fatalf("ожидалась ошибка - Точка не наследует Исключение, получено: %v", err)
	}
}

func TestAnalyzeCatchRejectsDuplicateTypeInUnion(t *testing.T) {
	_, err := AnalyzeSource(t, `
функция главная
{
    попытаться {
        вывести "тело"
    } поймать(иск: Исключение|Исключение) {
        вывести "поймано"
    }
}
`)
	if err == nil || !strings.Contains(err.Error(), "перечислен более одного раза") {
		t.Fatalf("ожидалась ошибка о повторе типа в объединении, получено: %v", err)
	}
}

// TestAnalyzeCatchVarTypeAcrossHierarchies - тип переменной "поймать":
// единственный тип - он сам; несколько из одной иерархии - её корень;
// обе иерархии вперемешку - Строка (сообщение).
func TestAnalyzeCatchVarTypeAcrossHierarchies(t *testing.T) {
	cases := map[string]ir.Type{
		"Паника": ir.EntityType("Паника"),
		"ВыходЗаГраницы | ДелениеНаНоль": ir.EntityType("Паника"),
		"Паника | Исключение":            ir.TypeString,
		"ОшибкаРазбора | Исключение":     ir.EntityType("Исключение"),
	}
	for types, want := range cases {
		t.Run(types, func(t *testing.T) {
			prog := parseProgram(t, `
функция главная
{
    попытаться {
        вывести "тело"
    } поймать(о: `+types+`) {
        вывести о
    }
}
`)
			try := findFunc(prog, "главная").Body[0].(*ir.TryStmt)
			if try.CatchVarType != want {
				t.Fatalf("ожидался тип %s, получено %s", want, try.CatchVarType)
			}
		})
	}
}

func TestAnalyzeCatchRejectsDuplicateAndNonThrowable(t *testing.T) {
	for src, want := range map[string]string{
		"Паника | Паника": "перечислен более одного раза",
		"Целое":           "ожидает сущность",
	} {
		_, err := AnalyzeSource(t, `
функция главная
{
    попытаться {
        вывести "тело"
    } поймать(о: `+src+`) {
        вывести "поймано"
    }
}
`)
		if err == nil || !strings.Contains(err.Error(), want) {
			t.Fatalf("%s: ожидалась ошибка с %q, получено: %v", src, want, err)
		}
	}
}

func TestAnalyzeIsExprBetweenRelatedEntities(t *testing.T) {
	prog := parseProgram(t, `
сущность ОшибкаДеления наследует Исключение
{
    конструктор(сообщение: Строка)
    {
        родитель(сообщение)
    }
}
функция главная
{
    попытаться {
        выбросить новая ОшибкаДеления("ноль")
    } поймать(иск: Исключение) {
        если иск относится к ОшибкаДеления {
            вывести "деление"
        }
    }
}
`)
	fn := findFunc(prog, "главная")
	try := fn.Body[0].(*ir.TryStmt)
	ifStmt := try.CatchBody[0].(*ir.IfStmt)
	isExpr, ok := ifStmt.Cond.(*ir.IsExpr)
	if !ok {
		t.Fatalf("ожидался IsExpr, получено %T", ifStmt.Cond)
	}
	if isExpr.Target != "ОшибкаДеления" {
		t.Fatalf("ожидался Target ОшибкаДеления, получено %q", isExpr.Target)
	}
}

func TestAnalyzeIsExprRejectsUnrelatedTypes(t *testing.T) {
	_, err := AnalyzeSource(t, `
сущность Точка
{
    имеет:
        x: Целое = 0
}
функция главная
{
    переменная т = новая Точка()
    если т относится к Исключение {
        вывести "да"
    }
}
`)
	if err == nil || !strings.Contains(err.Error(), "не связаны наследованием/интерфейсом") {
		t.Fatalf("ожидалась ошибка о несвязанных типах, получено: %v", err)
	}
}

func TestAnalyzeBareSystemExceptionConstructsAndCatches(t *testing.T) {
	prog := parseProgram(t, `
функция главная
{
    попытаться {
        выбросить новое Исключение("что-то пошло не так")
    } поймать(иск: Исключение) {
        вывести иск.сообщение
    }
}
`)
	found := false
	for _, ent := range prog.Entities {
		if ent.Name == "Исключение" {
			found = true
			if len(ent.Fields) != 2 || ent.Fields[0].Name != "сообщение" || ent.Fields[0].Type != ir.TypeString ||
				ent.Fields[1].Name != "код" || ent.Fields[1].Type != ir.NullableType(ir.TypeInt) {
				t.Fatalf("ожидались поля сообщение:Строка и код:?Целое у системной сущности Исключение, получено %+v", ent.Fields)
			}
		}
	}
	if !found {
		t.Fatalf("системная сущность Исключение не найдена в ir.Program.Entities")
	}
}

func findFunc(prog *ir.Program, name string) *ir.Function {
	for _, fn := range prog.Functions {
		if fn.Name == name {
			return fn
		}
	}
	return nil
}
