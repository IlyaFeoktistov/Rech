package parser

import (
	"strings"
	"testing"

	"github.com/IlyaFeoktistov/Rech/internal/ast"
)

func TestNaturalMethodCallReceiverTypedVariable(t *testing.T) {
	prog, err := parseSource(t, `
сущность Пользователь {
    авторизовать(кем пароль: Строка): Логическое { вернуть истина }
}
функция главная {
    переменная пароль = "секрет"
    переменная пользователь: Пользователь = новый Пользователь()
    пользователь авторизовать паролем
}
`)
	if err != nil {
		t.Fatalf("неожиданная ошибка: %v", err)
	}
	stmt := prog.Functions[0].Body[2].(*ast.ExprStmt)
	call := stmt.MethodCall
	if call == nil {
		t.Fatalf("ожидался MethodCallExpr, получено %+v", stmt)
	}
	receiver, ok := call.Receiver.(*ast.Identifier)
	if !ok || receiver.Name != "пользователь" {
		t.Fatalf("ожидался получатель \"пользователь\", получено %+v", call.Receiver)
	}
	if call.Method != "авторизовать" || len(call.Args) != 1 {
		t.Fatalf("неожиданный вызов: %+v", call)
	}
	if id, ok := call.Args[0].(*ast.Identifier); !ok || id.Name != "пароль" {
		t.Fatalf("ожидался аргумент \"пароль\", получено %+v", call.Args[0])
	}
}

func TestNaturalMethodCallReceiverThis(t *testing.T) {
	prog, err := parseSource(t, `
сущность Пользователь {
    авторизовать(кем пароль: Строка): Логическое { вернуть истина }
    проверить(): Логическое {
        переменная пароль = "секрет"
        вернуть этот авторизовать паролем
    }
}
`)
	if err != nil {
		t.Fatalf("неожиданная ошибка: %v", err)
	}
	ret := prog.Entities[0].Methods[1].Body[1].(*ast.ReturnStmt)
	call, ok := ret.Value.(*ast.MethodCallExpr)
	if !ok {
		t.Fatalf("ожидался MethodCallExpr, получено %+v", ret.Value)
	}
	if _, ok := call.Receiver.(*ast.ThisExpr); !ok {
		t.Fatalf("ожидался получатель \"этот\", получено %+v", call.Receiver)
	}
	if call.Method != "авторизовать" {
		t.Fatalf("неожиданное имя метода: %q", call.Method)
	}
}

func TestNaturalMethodCallReceiverParamOfEnclosingMethod(t *testing.T) {
	prog, err := parseSource(t, `
сущность Книга {
    выдать() { }
}
сущность Библиотекарь {
    обработать(что книга: Книга) {
        книга выдать
    }
}
`)
	if err != nil {
		t.Fatalf("неожиданная ошибка: %v", err)
	}
	// prog.Entities[1] - "Библиотекарь", его единственный метод "обработать".
	stmt := prog.Entities[1].Methods[0].Body[0].(*ast.ExprStmt)
	call := stmt.MethodCall
	if call == nil {
		t.Fatalf("ожидался MethodCallExpr, получено %+v", stmt)
	}
	receiver, ok := call.Receiver.(*ast.Identifier)
	if !ok || receiver.Name != "книга" {
		t.Fatalf("ожидался получатель \"книга\" (параметр метода), получено %+v", call.Receiver)
	}
	if call.Method != "выдать" || len(call.Args) != 0 {
		t.Fatalf("неожиданный вызов: %+v", call)
	}
}

func TestNaturalMethodCallWithoutExplicitTypeDoesNotTrigger(t *testing.T) {
	// "пользователь" без явной аннотации типа - тип не выведен парсером
	// (создан через конструктор, не через "переменная x: Тип") - естественный
	// вызов метода недоступен в этом срезе (см. beginVarTypeScope), поэтому
	// "пользователь авторизовать" остаётся ОБЫЧНОЙ синтаксической ошибкой,
	// как "IDENT IDENT" и до этой фичи.
	_, err := parseSource(t, `
сущность Пользователь {
    авторизовать(): Логическое { вернуть истина }
}
функция главная {
    переменная пользователь = новый Пользователь()
    пользователь авторизовать
}
`)
	if err == nil || !strings.Contains(err.Error(), "неожиданный токен") {
		t.Fatalf("ожидалась ошибка \"неожиданный токен\" (тип получателя не выведен), получено: %v", err)
	}
}

func TestNaturalMethodCallContinuationPrint(t *testing.T) {
	prog, err := parseSource(t, `
сущность Счётчик {
    значение(): Целое { вернуть 1 }
}
функция главная {
    переменная счётчик: Счётчик = новый Счётчик()
    счётчик значение и вывести
}
`)
	if err != nil {
		t.Fatalf("неожиданная ошибка: %v", err)
	}
	print, ok := prog.Functions[0].Body[1].(*ast.PrintStmt)
	if !ok {
		t.Fatalf("ожидался PrintStmt (\"и вывести\"), получено %+v", prog.Functions[0].Body[1])
	}
	if _, ok := print.Value.(*ast.MethodCallExpr); !ok {
		t.Fatalf("ожидался MethodCallExpr внутри PrintStmt.Value, получено %+v", print.Value)
	}
}

func TestNaturalMethodCallStrictDotStillWorks(t *testing.T) {
	// Оба синтаксиса всегда сосуществуют - строгий "receiver.метод(...)" не
	// должен сломаться появлением естественного вызова метода.
	prog, err := parseSource(t, `
сущность Пользователь {
    авторизовать(кем пароль: Строка): Логическое { вернуть истина }
}
функция главная {
    переменная пароль = "секрет"
    переменная пользователь: Пользователь = новый Пользователь()
    вывести пользователь.авторизовать(пароль)
}
`)
	if err != nil {
		t.Fatalf("неожиданная ошибка: %v", err)
	}
	print := prog.Functions[0].Body[2].(*ast.PrintStmt)
	call, ok := print.Value.(*ast.MethodCallExpr)
	if !ok || call.Method != "авторизовать" || len(call.Args) != 1 {
		t.Fatalf("неожиданный строгий вызов метода: %+v", print.Value)
	}
}

func TestNaturalMethodCallOrderIndependent(t *testing.T) {
	prog, err := parseSource(t, `
сущность Банк {
    начислить(кому клиент: Строка, что сумма: Целое): Целое { вернуть 0 }
}
функция главная {
    переменная банк: Банк = новый Банк()
    переменная клиент = "Мария"
    переменная сумма = 500
    банк начислить сумму клиенту
}
`)
	if err != nil {
		t.Fatalf("неожиданная ошибка: %v", err)
	}
	stmt := prog.Functions[0].Body[3].(*ast.ExprStmt)
	call := stmt.MethodCall
	if call == nil || len(call.Args) != 2 {
		t.Fatalf("неожиданный вызов: %+v", stmt)
	}
	if id, ok := call.Args[0].(*ast.Identifier); !ok || id.Name != "клиент" {
		t.Fatalf("ожидался первым аргументом \"клиент\" (параметр \"кому\"), получено %+v", call.Args[0])
	}
	if id, ok := call.Args[1].(*ast.Identifier); !ok || id.Name != "сумма" {
		t.Fatalf("ожидался вторым аргументом \"сумма\" (параметр \"что\"), получено %+v", call.Args[1])
	}
}

func TestNaturalMethodCallWrongEntityMethodNotConfused(t *testing.T) {
	// Два разных типа с методом того же имени, но разными параметрами -
	// получатель однозначно определяет, какой список параметров
	// используется для падежного сопоставления (entityMethods по имени
	// сущности, не общий для всех типов сразу).
	prog, err := parseSource(t, `
сущность Приёмная {
    обработать(кого пользователь: Строка) { }
}
сущность Канцелярия {
    обработать(что документ: Строка) { }
}
функция главная {
    переменная пользователь = "Гость"
    переменная документ = "Договор"
    переменная приёмная: Приёмная = новая Приёмная()
    переменная канцелярия: Канцелярия = новая Канцелярия()
    приёмная обработать пользователя
    канцелярия обработать документ
}
`)
	if err != nil {
		t.Fatalf("неожиданная ошибка: %v", err)
	}
	stmtA := prog.Functions[0].Body[4].(*ast.ExprStmt).MethodCall
	stmtB := prog.Functions[0].Body[5].(*ast.ExprStmt).MethodCall
	if id, ok := stmtA.Args[0].(*ast.Identifier); !ok || id.Name != "пользователь" {
		t.Fatalf("сущность Приёмная: ожидался аргумент \"пользователь\", получено %+v", stmtA.Args[0])
	}
	if id, ok := stmtB.Args[0].(*ast.Identifier); !ok || id.Name != "документ" {
		t.Fatalf("сущность Канцелярия: ожидался аргумент \"документ\", получено %+v", stmtB.Args[0])
	}
}
