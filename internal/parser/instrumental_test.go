package parser

import "testing"

// TestParseEntityStatesComputesBecomeFormFromShortAdjective - "становится"
// требует полную форму, согласованную по роду сущности И творительным
// падежом - см. package doc ast.StateDecl.BecomeForm.
func TestParseEntityStatesComputesBecomeFormFromShortAdjective(t *testing.T) {
	prog, err := parseSource(t, `
сущность Пользователь
{
    состояние: неавторизован авторизован
}
функция главная { вывести "ок" }
`)
	if err != nil {
		t.Fatalf("неожиданная ошибка: %v", err)
	}
	ent := prog.Entities[0]
	if ent.States[0].BecomeForm != "неавторизованным" {
		t.Fatalf("ожидался BecomeForm \"неавторизованным\", получено %+v", ent.States[0])
	}
	if ent.States[1].BecomeForm != "авторизованным" {
		t.Fatalf("ожидался BecomeForm \"авторизованным\", получено %+v", ent.States[1])
	}
}

// TestParseEntityStatesComputesBecomeFormForFeminineEntity - тот же
// источник слова, но сущность женского рода - другое окончание.
func TestParseEntityStatesComputesBecomeFormForFeminineEntity(t *testing.T) {
	prog, err := parseSource(t, `
сущность Система
{
    состояние: авторизован
}
функция главная { вывести "ок" }
`)
	if err != nil {
		t.Fatalf("неожиданная ошибка: %v", err)
	}
	if got := prog.Entities[0].States[0].BecomeForm; got != "авторизованной" {
		t.Fatalf("ожидался BecomeForm \"авторизованной\" (жен. род), получено %q", got)
	}
}

// TestParseEntityStatesComputesBecomeFormFromActiveParticiple - второй
// источник слова (действительное причастие) тоже даёт BecomeForm.
func TestParseEntityStatesComputesBecomeFormFromActiveParticiple(t *testing.T) {
	prog, err := parseSource(t, `
сущность Пользователь
{
    состояние: вошедший
}
функция главная { вывести "ок" }
`)
	if err != nil {
		t.Fatalf("неожиданная ошибка: %v", err)
	}
	if got := prog.Entities[0].States[0].BecomeForm; got != "вошедшим" {
		t.Fatalf("ожидался BecomeForm \"вошедшим\", получено %q", got)
	}
}
