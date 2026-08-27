package parser

import "testing"

func TestParseMethodSelfReturnType(t *testing.T) {
	prog, err := parseSource(t, `
сущность Строитель
{
    задать(): этот
    {
        вернуть этот
    }
}
функция главная { вывести "ок" }
`)
	if err != nil {
		t.Fatalf("неожиданная ошибка: %v", err)
	}
	m := prog.Entities[0].Methods[0]
	if !m.SelfReturn || m.ReturnType != "" {
		t.Fatalf("ожидался SelfReturn=true, ReturnType=\"\", получено SelfReturn=%v ReturnType=%q", m.SelfReturn, m.ReturnType)
	}
}

func TestParseInterfaceMethodSelfReturnType(t *testing.T) {
	prog, err := parseSource(t, `
интерфейс Именуемый
{
    задать_имя(значение: Строка): этот
}
функция главная { вывести "ок" }
`)
	if err != nil {
		t.Fatalf("неожиданная ошибка: %v", err)
	}
	m := prog.Interfaces[0].Methods[0]
	if !m.SelfReturn || m.ReturnType != "" {
		t.Fatalf("ожидался SelfReturn=true, ReturnType=\"\", получено SelfReturn=%v ReturnType=%q", m.SelfReturn, m.ReturnType)
	}
}
