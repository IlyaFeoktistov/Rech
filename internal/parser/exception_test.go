package parser

import (
	"strings"
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

func TestParseIsExprRequiresDativeCase(t *testing.T) {
	prog, err := parseSource(t, `
сущность Пользователь
{
    имеет:
        имя: Строка
}
функция главная
{
    переменная кто = новый Пользователь("Аня")
    если кто относится к Пользователю {
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
		t.Fatalf("ожидалось разрешение дательной формы в имя типа %q, получено %q", "Пользователь", isExpr.Target)
	}
}

func TestParseIsExprRejectsNominativeWhenDativeDiffers(t *testing.T) {
	_, err := parseSource(t, `
сущность Пользователь
{
    имеет:
        имя: Строка
}
функция главная
{
    переменная кто = новый Пользователь("Аня")
    если кто относится к Пользователь {
        вывести "да"
    }
}
`)
	if err == nil || !strings.Contains(err.Error(), "ожидается дательный падеж") || !strings.Contains(err.Error(), "Пользователю") {
		t.Fatalf("ожидалась ошибка про требуемую дательную форму \"Пользователю\", получено: %v", err)
	}
}

func TestParseIsExprAgainstSystemExceptionEntity(t *testing.T) {
	prog, err := parseSource(t, `
функция главная
{
    попытаться {
        выбросить новое Исключение("что-то пошло не так")
    } поймать(иск: Исключение) {
        если иск относится к Исключению {
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
		t.Fatalf("ожидалось разрешение дательной формы \"Исключению\" в лемму %q, получено %q", "Исключение", isExpr.Target)
	}
}
