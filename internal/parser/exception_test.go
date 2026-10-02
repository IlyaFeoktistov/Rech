package parser

import (
	"testing"

	"github.com/IlyaFeoktistov/Rech/internal/ast"
)

func TestParseTryCatchThrow(t *testing.T) {
	prog, err := parseSource(t, `
сущность ОшибкаДеления наследует Исключение
{
}
функция главная
{
    попытаться {
        выбросить новая ОшибкаДеления("деление на ноль")
    } поймать(иск: Исключение|ОшибкаДеления) {
        вывести иск.сообщение
    }
}
`)
	if err != nil {
		t.Fatalf("неожиданная ошибка: %v", err)
	}
	fn := prog.Functions[0]
	if len(fn.Body) != 1 {
		t.Fatalf("ожидалась одна инструкция в теле главная, получено %d", len(fn.Body))
	}
	try, ok := fn.Body[0].(*ast.TryStmt)
	if !ok {
		t.Fatalf("ожидался TryStmt, получено %T", fn.Body[0])
	}
	if try.CatchVar != "иск" {
		t.Fatalf("ожидалось имя переменной \"иск\", получено %q", try.CatchVar)
	}
	if len(try.CatchTypes) != 2 || try.CatchTypes[0] != "Исключение" || try.CatchTypes[1] != "ОшибкаДеления" {
		t.Fatalf("ожидались типы [Исключение ОшибкаДеления], получено %+v", try.CatchTypes)
	}
	if len(try.Body) != 1 {
		t.Fatalf("ожидалась одна инструкция в теле попытаться, получено %d", len(try.Body))
	}
	throw, ok := try.Body[0].(*ast.ThrowStmt)
	if !ok {
		t.Fatalf("ожидался ThrowStmt, получено %T", try.Body[0])
	}
	if _, ok := throw.Value.(*ast.NewExpr); !ok {
		t.Fatalf("ожидалось \"новый ...\" в качестве выражения выбросить, получено %T", throw.Value)
	}
}

// TestParseIsExprTakesTargetLiterallyWithoutDeclension - "относится к"
// грамматически требует дательного падежа, но Target берётся словарной
// формой как написано, без проверки/резолюции склонения (в отличие от
// "наследует"/"реализует") - решение, что подключать здесь реальное
// склонение стоит только вместе с настоящим морфологическим движком,
// который умеет разбирать составные имена (см. package doc parseIsExpr).
func TestParseIsExprTakesTargetLiterallyWithoutDeclension(t *testing.T) {
	prog, err := parseSource(t, `
сущность Пользователь
{
    имеет:
        Строка имя
}
функция главная
{
    переменная кто = новый Пользователь("Аня")
    если кто относится к Пользователь {
        вывести "да"
    }
}
`)
	if err != nil {
		t.Fatalf("неожиданная ошибка: %v", err)
	}
	fn := prog.Functions[0]
	ifStmt, ok := fn.Body[1].(*ast.IfStmt)
	if !ok {
		t.Fatalf("ожидался IfStmt, получено %T", fn.Body[1])
	}
	isExpr, ok := ifStmt.Cond.(*ast.IsExpr)
	if !ok {
		t.Fatalf("ожидался IsExpr, получено %T", ifStmt.Cond)
	}
	if isExpr.Target != "Пользователь" {
		t.Fatalf("ожидался Target %q без склонения, получено %q", "Пользователь", isExpr.Target)
	}
}

// TestParseTryCatchPanicIsOrdinaryType - "Паника" (и её наследники) в
// "поймать(...)" - обычное имя типа, в том числе вперемешку с исключениями.
func TestParseTryCatchPanicIsOrdinaryType(t *testing.T) {
	prog, err := parseSource(t, `
функция главная
{
    попытаться {
        вывести "тело"
    } поймать(сообщение: Паника|Исключение) {
        вывести "поймано"
    }
}
`)
	if err != nil {
		t.Fatalf("неожиданная ошибка: %v", err)
	}
	try := prog.Functions[0].Body[0].(*ast.TryStmt)
	if len(try.CatchTypes) != 2 || try.CatchTypes[0] != "Паника" || try.CatchTypes[1] != "Исключение" {
		t.Fatalf("ожидался CatchTypes [Паника Исключение], получено %+v", try.CatchTypes)
	}
}

func TestParseIsExprAgainstSystemExceptionEntity(t *testing.T) {
	prog, err := parseSource(t, `
функция главная
{
    попытаться {
        выбросить новое Исключение("что-то пошло не так")
    } поймать(иск: Исключение) {
        если иск относится к Исключение {
            вывести "да"
        }
    }
}
`)
	if err != nil {
		t.Fatalf("неожиданная ошибка: %v", err)
	}
	try := prog.Functions[0].Body[0].(*ast.TryStmt)
	ifStmt := try.CatchBody[0].(*ast.IfStmt)
	isExpr := ifStmt.Cond.(*ast.IsExpr)
	if isExpr.Target != "Исключение" {
		t.Fatalf("ожидался Target %q, получено %q", "Исключение", isExpr.Target)
	}
}
