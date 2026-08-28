package parser

import "testing"

func TestParseExternEntityDecl(t *testing.T) {
	prog, err := parseSource(t, `
внешняя сущность CURL как Дескриптор
функция главная { вывести 1 }
`)
	if err != nil {
		t.Fatalf("неожиданная ошибка: %v", err)
	}
	if len(prog.ExternEntities) != 1 {
		t.Fatalf("ожидалась одна внешняя сущность, получено: %d", len(prog.ExternEntities))
	}
	ext := prog.ExternEntities[0]
	if ext.CName != "CURL" || ext.Alias != "Дескриптор" {
		t.Fatalf("CName/Alias не совпадают с ожидаемыми: %+v", ext)
	}
}

func TestParseExternEntityDeclWithoutAliasUsesCName(t *testing.T) {
	prog, err := parseSource(t, `
внешняя сущность CURL
функция главная { вывести 1 }
`)
	if err != nil {
		t.Fatalf("неожиданная ошибка: %v", err)
	}
	ext := prog.ExternEntities[0]
	if ext.CName != "CURL" || ext.Alias != "CURL" {
		t.Fatalf("без \"как\" Alias должен совпадать с CName: %+v", ext)
	}
}

func TestParseExternStructDecl(t *testing.T) {
	prog, err := parseSource(t, `
внешняя структура timeval как Время
{
    имеет:
        секунды: Целое
        микросекунды: Целое
}
функция главная { вывести 1 }
`)
	if err != nil {
		t.Fatalf("неожиданная ошибка: %v", err)
	}
	if len(prog.Entities) != 1 {
		t.Fatalf("ожидалась одна сущность, получено: %d", len(prog.Entities))
	}
	ent := prog.Entities[0]
	if ent.Name != "Время" || !ent.IsStruct || !ent.IsExternStruct || ent.ExternCName != "timeval" {
		t.Fatalf("внешняя структура разобрана неверно: %+v", ent)
	}
	if len(ent.Fields) != 2 {
		t.Fatalf("ожидалось 2 поля, получено: %d", len(ent.Fields))
	}
}

func TestParseExternStructDeclRequiresAtLeastOneField(t *testing.T) {
	_, err := parseSource(t, `
внешняя структура timeval как Время
{
    имеет:
}
функция главная { вывести 1 }
`)
	if err == nil {
		t.Fatal("ожидалась ошибка - внешняя структура без полей")
	}
}
