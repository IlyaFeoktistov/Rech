package parser

import (
	"strings"
	"testing"
)

// TestParseInheritsRequiresAccusativeCase - "наследует"/"реализует" -
// переходные глаголы, управляют винительным падежом (docs, package doc
// resolveDeclinedTypeRef) - "Пользователь" (одушевлённое, мужской род) имеет
// собственную винительную форму "Пользователя", отличную от именительной.
func TestParseInheritsRequiresAccusativeCase(t *testing.T) {
	prog, err := parseSource(t, `
сущность Пользователь
{
    имеет:
        Строка имя
}
сущность Сотрудник наследует Пользователя
{
}
функция главная { вывести "ок" }
`)
	if err != nil {
		t.Fatalf("неожиданная ошибка: %v", err)
	}
	if prog.Entities[1].BaseName != "Пользователь" {
		t.Fatalf("ожидалось разрешение винительной формы в имя сущности %q, получено %q", "Пользователь", prog.Entities[1].BaseName)
	}
}

func TestParseInheritsRejectsNominativeWhenAccusativeDiffers(t *testing.T) {
	_, err := parseSource(t, `
сущность Пользователь
{
    имеет:
        Строка имя
}
сущность Сотрудник наследует Пользователь
{
}
функция главная { вывести "ок" }
`)
	if err == nil || !strings.Contains(err.Error(), "ожидается винительный падеж") || !strings.Contains(err.Error(), "Пользователя") {
		t.Fatalf("ожидалась ошибка про требуемую винительную форму \"Пользователя\", получено: %v", err)
	}
}

// TestParseImplementsRequiresAccusativeCaseForEachInList - список через
// запятую ("реализует И1, И2, ...") - склонение проверяется для КАЖДОГО
// элемента списка независимо.
func TestParseImplementsRequiresAccusativeCaseForEachInList(t *testing.T) {
	prog, err := parseSource(t, `
интерфейс Звучащий
{
    издать_звук(): Строка
}
сущность Пользователь реализует Звучащий
{
    имеет:
        Строка имя

    издать_звук(): Строка
    {
        вернуть "..."
    }
}
сущность Сотрудник наследует Пользователя реализует Звучащий
{
}
функция главная { вывести "ок" }
`)
	if err != nil {
		t.Fatalf("неожиданная ошибка: %v", err)
	}
	derived := prog.Entities[1]
	if derived.BaseName != "Пользователь" {
		t.Fatalf("ожидалось разрешение винительной формы базовой сущности в %q, получено %q", "Пользователь", derived.BaseName)
	}
	if len(derived.Interfaces) != 1 || derived.Interfaces[0] != "Звучащий" {
		t.Fatalf("ожидался интерфейс %q (неодушевлённое/причастие - именительный==винительный), получено %+v", "Звучащий", derived.Interfaces)
	}
}

// TestParseInheritsResolvesAdjectiveShapedEntityName - НАЙДЕННЫЙ БАГ
// (репортнуто пользователем): сущность с именем, выглядящим как
// прилагательное ("Промежуточная" - "-ая", склоняется "Промежуточную" в
// винительном, а не "Промежуточню" как обычное существительное на голое
// "-я" вроде "земля" - см. package doc ClassAdjHardFeminine/
// ClassAdjSoftFeminine, internal/morph/noun.go) не резолвилась базовой
// сущностью вовсе - до фикса классификатор путал "-ая"/"-яя" с этим
// классом существительных и вычислял неверную винительную форму.
func TestParseInheritsResolvesAdjectiveShapedEntityName(t *testing.T) {
	prog, err := parseSource(t, `
сущность Промежуточная
{
}
сущность Круг наследует Промежуточную
{
}
функция главная { вывести "ок" }
`)
	if err != nil {
		t.Fatalf("неожиданная ошибка: %v", err)
	}
	if prog.Entities[1].BaseName != "Промежуточная" {
		t.Fatalf("ожидалось разрешение винительной формы прилагательного-подобного имени в %q, получено %q", "Промежуточная", prog.Entities[1].BaseName)
	}
}

// TestParseInheritsPermissiveFallbackForUnclassifiableName - имя, склонение
// которого неопределимо (не зарегистрировано или вне покрытия Фазы A) -
// принимается как есть, без ошибки (permissive fallback, тот же принцип,
// что и у checkNewKeywordAgreement).
func TestParseInheritsPermissiveFallbackForUnclassifiableName(t *testing.T) {
	prog, err := parseSource(t, `
сущность Ы
{
}
сущность Наследник наследует Ы
{
}
функция главная { вывести "ок" }
`)
	if err != nil {
		t.Fatalf("неожиданная ошибка: %v", err)
	}
	if prog.Entities[1].BaseName != "Ы" {
		t.Fatalf("ожидался permissive fallback (имя как есть), получено %q", prog.Entities[1].BaseName)
	}
}
