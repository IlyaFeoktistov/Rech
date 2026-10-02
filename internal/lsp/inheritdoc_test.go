package lsp

import (
	"strings"
	"testing"
)

const inheritDocTestSrc = `
сущность Животное
{
    имеет:
        Строка имя

    конструктор(имя: Строка)
    {
        этот.имя = имя
    }

    /** Издаёт звук, характерный для этого вида животного. */
    издать_звук(): Строка
    {
        вернуть "..."
    }
}

сущность Кошка наследует Животное
{
    конструктор(имя: Строка)
    {
        родитель(имя)
    }

    /** @наследовать */
    издать_звук(): Строка
    {
        вернуть "мяу"
    }
}

функция главная
{
    переменная кошка = новая Кошка("Мурка")
    вывести кошка.издать_звук()
}
`

func TestHasInheritTagDetectsTag(t *testing.T) {
	if !hasInheritTag("@наследовать") {
		t.Fatalf("ожидалось обнаружение @наследовать")
	}
	if hasInheritTag("обычный doc-комментарий") {
		t.Fatalf("не ожидалось обнаружение @наследовать")
	}
}

// TestHoverOnOverriddenMethodInheritsBaseDoc - hover метода "издать_звук"
// сущности "Кошка" (свой doc - только тег "@наследовать") должен показать
// doc БАЗОВОГО метода "Животное.издать_звук" ("Издаёт звук...").
func TestHoverOnOverriddenMethodInheritsBaseDoc(t *testing.T) {
	d := newDoc("test://a.rech", inheritDocTestSrc)
	if d.irProg == nil {
		t.Fatalf("ожидался успешный разбор")
	}
	line := lineOf(t, inheritDocTestSrc, "кошка.издать_звук()")
	col := runeCol(t, "    вывести кошка.издать_звук()", "издать_звук")
	text, ok := d.hover(line, col)
	if !ok {
		t.Fatalf("ожидался hover метода \"издать_звук\"")
	}
	if !strings.Contains(text, "Издаёт звук, характерный для этого вида животного.") {
		t.Fatalf("ожидался унаследованный doc, получено:\n%s", text)
	}
	if strings.Contains(text, "@наследовать") {
		t.Fatalf("тег \"@наследовать\" не должен остаться в тексте после успешного наследования, получено:\n%s", text)
	}
}
