package semantic

import (
	"strings"
	"testing"
)

func TestBareCallableDeclTakesValueSignature(t *testing.T) {
	_, err := AnalyzeSource(t, `
функция удвоить(x: Целое): Целое
{
    вернуть x * 2
}

функция главная
{
    Вызываемый привет = функция () {
        вывести "привет"
    }
    привет()
    Вызываемый сложить = функция (а: Целое, б: Целое): Целое {
        вернуть а + б
    }
    Целое сумма = сложить(2, 3)
    Вызываемый по_ссылке = &удвоить
    сумма = по_ссылке(сумма)
}
`)
	if err != nil {
		t.Fatalf("неожиданная ошибка: %v", err)
	}
}

func TestBareCallableDeclRejects(t *testing.T) {
	cases := map[string]struct{ src, want string }{
		"не функция": {`
функция главная
{
    Вызываемый фн = 5
}
`, "инициализирована значением типа Целое"},
		"другая сигнатура при присваивании": {`
функция главная
{
    Вызываемый фн = функция (x: Целое): Целое {
        вернуть x
    }
    фн = функция () {
    }
}
`, "нельзя присвоить"},
	}
	for name, tc := range cases {
		t.Run(name, func(t *testing.T) {
			_, err := AnalyzeSource(t, tc.src)
			if err == nil || !strings.Contains(err.Error(), tc.want) {
				t.Fatalf("ожидалась ошибка с %q, получено: %v", tc.want, err)
			}
		})
	}
}
