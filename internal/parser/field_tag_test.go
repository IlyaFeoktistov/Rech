package parser

import "testing"

func TestParseFieldTag(t *testing.T) {
	src := "" +
		"сущность Пользователь\n" +
		"{\n" +
		"    имеет:\n" +
		"        имя: Строка `json:name, sql:user_name`\n" +
		"        возраст: Целое\n" +
		"}\n" +
		"функция главная { вывести \"ок\" }\n"
	prog, err := parseSource(t, src)
	if err != nil {
		t.Fatalf("неожиданная ошибка: %v", err)
	}
	ent := prog.Entities[0]
	if want := "json:name, sql:user_name"; ent.Fields[0].Tag != want {
		t.Fatalf("тег поля имя: ожидалось %q, получено %q", want, ent.Fields[0].Tag)
	}
	if ent.Fields[1].Tag != "" {
		t.Fatalf("у поля возраст без тега Tag должен быть \"\", получено %q", ent.Fields[1].Tag)
	}
}

func TestParseFieldTagWithDefaultValue(t *testing.T) {
	src := "" +
		"сущность Пользователь\n" +
		"{\n" +
		"    имеет:\n" +
		"        возраст: Целое = 18 `json:age`\n" +
		"}\n" +
		"функция главная { вывести \"ок\" }\n"
	prog, err := parseSource(t, src)
	if err != nil {
		t.Fatalf("неожиданная ошибка: %v", err)
	}
	f := prog.Entities[0].Fields[0]
	if f.Default == nil {
		t.Fatal("значение по умолчанию не разобралось")
	}
	if want := "json:age"; f.Tag != want {
		t.Fatalf("тег: ожидалось %q, получено %q", want, f.Tag)
	}
}
