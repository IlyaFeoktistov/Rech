package diagnostics

import (
	"fmt"
	"strings"
	"testing"
)

func TestRenderCaretOnCyrillic(t *testing.T) {
	// "переменная" - 10 рун, "х" - 11-я руна (колонка 12) - если бы Render
	// считал байтами, каретка ушла бы дальше вправо (кириллица - два байта
	// на руну в UTF-8).
	src := "переменная х = 5"
	d := New("RECH2001", Position{File: "f.rech", Line: 1, Col: 12}, "ожидалось имя")
	out := Render(d, src)

	lines := strings.Split(strings.TrimRight(out, "\n"), "\n")
	var snippetLine, caretLine string
	for i, l := range lines {
		if strings.Contains(l, src) {
			snippetLine = l
			caretLine = lines[i+1]
		}
	}
	if snippetLine == "" {
		t.Fatalf("сниппет не найден в выводе:\n%s", out)
	}
	snippetRunes := []rune(snippetLine)
	caretRunes := []rune(caretLine)
	caretIdx := -1
	for i, r := range caretRunes {
		if r == '^' {
			caretIdx = i
			break
		}
	}
	if caretIdx < 0 || caretIdx >= len(snippetRunes) {
		t.Fatalf("каретка не найдена или вне строки: %q / %q", snippetLine, caretLine)
	}
	if snippetRunes[caretIdx] != 'х' {
		t.Errorf("каретка указывает на %q, хочу 'х': снипет=%q каретка=%q", snippetRunes[caretIdx], snippetLine, caretLine)
	}
}

func TestRenderNoColumnNoCaret(t *testing.T) {
	d := New("RECH3001", Position{File: "f.rech", Line: 1}, "необъявленная переменная")
	out := Render(d, "х = 5")
	if strings.Contains(out, "^") {
		t.Errorf("Col == 0 не должен давать каретку: %s", out)
	}
}

func TestRenderMissingLineNoSnippet(t *testing.T) {
	d := New("RECH3001", Position{File: "f.rech", Line: 99}, "ошибка")
	out := Render(d, "одна строка")
	if strings.Contains(out, "|") {
		t.Errorf("несуществующая строка не должна давать сниппет: %s", out)
	}
}

func TestRenderHint(t *testing.T) {
	d := New("RECH3105", Position{}, "тип не совпадает").WithHint("возможно, требуется (Строка) ...")
	out := Render(d, "")
	if !strings.Contains(out, "подсказка: возможно, требуется (Строка) ...") {
		t.Errorf("подсказка не напечатана: %s", out)
	}
}

func TestBagSortsByPosition(t *testing.T) {
	b := NewBag()
	b.Add(New("RECH2001", Position{File: "b.rech", Line: 5}, "вторая"))
	b.Add(New("RECH2001", Position{File: "a.rech", Line: 9}, "третья"))
	b.Add(New("RECH2001", Position{File: "a.rech", Line: 1}, "первая"))

	got := b.Diagnostics()
	want := []string{"первая", "третья", "вторая"}
	if len(got) != len(want) {
		t.Fatalf("длина = %d, хочу %d", len(got), len(want))
	}
	for i, d := range got {
		if d.Message != want[i] {
			t.Errorf("позиция %d: %q, хочу %q", i, d.Message, want[i])
		}
	}
}

func TestAsDiagnosticUnwrapsChain(t *testing.T) {
	inner := New("RECH2001", Position{File: "f.rech", Line: 3, Col: 1}, "неожиданный токен")
	wrapped := fmt.Errorf("сущность %q: %w", "Кот", inner)

	got := AsDiagnostic(wrapped)
	if got != inner {
		t.Fatalf("AsDiagnostic не развернул цепочку до исходной диагностики: %+v", got)
	}
}

func TestAsDiagnosticPlainError(t *testing.T) {
	got := AsDiagnostic(fmt.Errorf("обычная ошибка"))
	if got.Code != "" || got.Pos.Line != 0 {
		t.Errorf("обычная ошибка не должна получить код/позицию: %+v", got)
	}
	if got.Message != "обычная ошибка" {
		t.Errorf("сообщение потеряно: %q", got.Message)
	}
}

func TestRenderAllMultipleHeader(t *testing.T) {
	diags := []*Diagnostic{
		New("RECH2001", Position{File: "f.rech", Line: 1}, "первая"),
		New("RECH2002", Position{File: "f.rech", Line: 2}, "вторая"),
	}
	out := RenderAll(diags, map[string]string{"f.rech": "а\nб"})
	if !strings.Contains(out, "2 ошибки") {
		t.Errorf("нет сводки количества: %s", out)
	}
}

func TestRegisterDuplicateSameDescriptionOK(t *testing.T) {
	Register("RECH0001", "тестовая категория")
	Register("RECH0001", "тестовая категория") // не паникует
}

func TestRegisterDuplicateDifferentDescriptionPanics(t *testing.T) {
	defer func() {
		if recover() == nil {
			t.Fatal("ожидалась паника при конфликтующей повторной регистрации")
		}
	}()
	Register("RECH0002", "первое описание")
	Register("RECH0002", "другое описание")
}
