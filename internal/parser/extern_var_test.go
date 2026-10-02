package parser

import "testing"

// TestParseExternAndExportedModuleVars - "внешняя Тип ИмяC [как Имя]" -
// внешняя переменная модуля; "экспортировать Тип имя = ..." - экспортируемая
// переменная модуля, не списковая форма "экспортировать Имя".
func TestParseExternAndExportedModuleVars(t *testing.T) {
	prog, err := parseSource(t, `внешняя Целое32 opterr как флаг
экспортировать Целое счёт = 0

функция главная
{
}
`)
	if err != nil {
		t.Fatalf("неожиданная ошибка: %v", err)
	}
	if len(prog.Vars) != 2 {
		t.Fatalf("ожидались 2 переменные модуля, получено %+v", prog.Vars)
	}
	ext, exp := prog.Vars[0], prog.Vars[1]
	if ext.Name != "флаг" || ext.ExternCName != "opterr" || ext.Type != "Целое32" {
		t.Errorf("внешняя переменная: %+v", ext)
	}
	if exp.Name != "счёт" || !exp.Exported || exp.ExternCName != "" {
		t.Errorf("экспортируемая переменная: %+v", exp)
	}
}
