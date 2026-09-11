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

// TestAnalyzeCatchPanicBindsCatchVarAsString - "поймать (x: Паника)" (docs/
// изоляция паники) - CatchVar всегда Строка, независимо от
// того, объявлена ли в программе своя сущность "Исключение" вообще.
func TestAnalyzeCatchPanicBindsCatchVarAsString(t *testing.T) {
	prog := parseProgram(t, `
функция главная
{
    попытаться {
        вывести "тело"
    } поймать(сообщение: Паника) {
        вывести сообщение
    }
}
`)
	fn := findFunc(prog, "главная")
	try := fn.Body[0].(*ir.TryStmt)
	if !try.CatchPanic {
		t.Fatalf("ожидался CatchPanic == true")
	}
	if try.CatchVarType != ir.TypeString {
		t.Fatalf("ожидался тип Строка у переменной поймать(x: Паника), получено %s", try.CatchVarType)
	}
	if len(try.CatchTypes) != 0 {
		t.Fatalf("ожидался пустой CatchTypes, получено %+v", try.CatchTypes)
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
