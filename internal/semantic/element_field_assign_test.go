package semantic

import (
	"strings"
	"testing"
)

const elemHeader = `структура Пара
{
    имеет:
        Целое a
}

функция создать(): Список<Пара>
{
    вернуть [новая Пара(a: 1)]
}
`

func TestAnalyzeElementFieldAssign(t *testing.T) {
	if _, err := AnalyzeSource(t, elemHeader+`
функция главная
{
    Список<Пара> сп = [новая Пара(a: 1)]
    сп[0].a = 7
    сп.получить(0).a += 1
    сп[0].a++
    Словарь<Строка, Пара> д = новый Словарь<Строка, Пара>()
    д["к"] = новая Пара(a: 1)
    д["к"].a = 5
}
`); err != nil {
		t.Fatalf("неожиданная ошибка: %v", err)
	}
	_, err := AnalyzeSource(t, elemHeader+"\nфункция главная\n{\n    создать().получить(0).a = 1\n}\n")
	if err == nil || !strings.Contains(err.Error(), "коллекция должна быть переменной или полем") {
		t.Fatalf("ожидалась ошибка про коллекцию-результат вызова, получено %v", err)
	}
}
