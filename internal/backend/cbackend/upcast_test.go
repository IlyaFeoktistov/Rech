package cbackend

import (
	"strings"
	"testing"
)

// Потомок там, где ожидается предок (аргумент, переменная, результат,
// родитель(...)) - явное приведение указателя: GCC 14+ считает неявное
// приведение между разными структурами ошибкой.
func TestGenerateUpcastsChildToParentExplicitly(t *testing.T) {
	out := generateFromSrc(t, `
сущность Животное
{
    имеет:
        Строка имя

    конструктор(имя: Строка)
    {
        этот.имя = имя
    }
}

сущность Кошка наследует Животное
{
    конструктор(имя: Строка)
    {
        родитель(имя)
    }
}

функция назвать(ж: Животное): Строка
{
    вернуть ж.имя
}

функция главная
{
    переменная кошка = новая Кошка("Мурка")
    Животное ж = кошка
    вывести назвать(кошка)
}
`)
	for _, want := range []string{
		"rech_init_zhivotnoe((rech_T_zhivotnoe *)this_, v_imya);",
		"rech_nazvat(((rech_T_zhivotnoe *)(v_koshka)))",
		"= ((rech_T_zhivotnoe *)(v_koshka));",
	} {
		if !strings.Contains(out, want) {
			t.Fatalf("ожидался фрагмент %q, получено:\n%s", want, out)
		}
	}
}
