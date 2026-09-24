package parser

import (
	"testing"

	"github.com/IlyaFeoktistov/Rech/internal/lexer"
)

func lenientParse(t *testing.T, src string) (int, []error) {
	t.Helper()
	toks, err := lexer.New(src).Tokenize()
	if err != nil {
		t.Fatalf("лексер: %v", err)
	}
	prog, errs := New(toks).ParseProgramLenient()
	return len(prog.Functions), errs
}

func TestParseSynonymDecl(t *testing.T) {
	src := "приватный синоним ?Может<T, У> = ?Словарь<T, У>\nсиноним Ф = Вызываемый(Целое): Строка\n"
	toks, err := lexer.New(src).Tokenize()
	if err != nil {
		t.Fatalf("лексер: %v", err)
	}
	prog, err := New(toks).ParseProgram()
	if err != nil {
		t.Fatalf("парсер: %v", err)
	}
	if len(prog.Synonyms) != 2 {
		t.Fatalf("ожидалось 2 синонима, получено %d", len(prog.Synonyms))
	}
	s := prog.Synonyms[0]
	if s.Name != "Может" || !s.Nullable || len(s.TypeParams) != 2 || s.Target != "?Словарь<T, У>" {
		t.Fatalf("неверный разбор generic nullable-синонима: %+v", s)
	}
	if prog.Synonyms[1].Target != "Вызываемый<Целое>Строка" {
		t.Fatalf("цель-Вызываемый: %q", prog.Synonyms[1].Target)
	}
}

func TestParseSynonymVisibilityAgreesMasculine(t *testing.T) {
	if _, errs := lenientParse(t, "приватная синоним А = Целое\n"); len(errs) != 1 {
		t.Fatalf("\"приватная синоним\" - ожидалась ошибка согласования, получено %v", errs)
	}
}

// Ошибка в объявлении с модификатором ("публичная функция") - одна
// диагностика, а не по одной на каждое слово заголовка.
func TestLenientRecoveryAfterModifierReportsOnce(t *testing.T) {
	for _, head := range []string{"публичная функция", "/** Док. */\nфункция", "/** Док. */\nпубличная функция"} {
		n, errs := lenientParse(t, head+" сломанная(\n{\n}\n\nпубличная функция хорошая\n{\n}\n")
		if len(errs) != 1 {
			t.Fatalf("%q: ожидалась одна ошибка, получено %d: %v", head, len(errs), errs)
		}
		if n != 1 {
			t.Fatalf("%q: исправная функция после сломанной должна разобраться, получено %d функций", head, n)
		}
	}
}

// Переменная цикла соседней функции не делает словоформу коллекции
// неоднозначной: "числах" - параметр "числа" этой функции.
func TestForEachPrepositionalIgnoresOtherFunctionsNouns(t *testing.T) {
	loop := "(числа: Список<Целое>): Целое\n{\n    переменная сумма = 0\n    для число в числах {\n        сумма += число\n    }\n    вернуть сумма\n}\n"
	if _, errs := lenientParse(t, "функция первая"+loop+"\nфункция вторая"+loop); len(errs) != 0 {
		t.Fatalf("неожиданные ошибки: %v", errs)
	}
}
