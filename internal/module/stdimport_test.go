package module

import (
	"strings"
	"testing"
)

func linkSource(t *testing.T, src string) error {
	t.Helper()
	entry := writeFile(t, t.TempDir(), "главный.rech", src)
	prog, err := Load(entry, testFrontend{})
	if err != nil {
		return err
	}
	_, err = prog.Link()
	return err
}

// Без импорта встроенное имя свободно: своя функция - обычное объявление,
// рядом - встроенная под псевдонимом.
func TestStdImportOwnNameAndAlias(t *testing.T) {
	err := linkSource(t, `использовать знак как знак_числа из стандарт/математика

функция знак(значение: Целое): Целое
{
    вернуть 42
}

функция главная
{
    вывести знак(1)
    вывести знак_числа(-1.5)
}
`)
	if err != nil {
		t.Fatalf("Link: %v", err)
	}
}

func TestStdImportErrors(t *testing.T) {
	cases := map[string]string{
		"функция главная { вывести корень(4.0) }\n":                                           `добавьте "использовать корень из стандарт/математика"`,
		"функция главная { вывести HTTPКод.ОК }\n":                                            `добавьте "использовать HTTPКод из стандарт/http"`,
		"использовать корень из стандарт/матем\nфункция главная { вывести 1 }\n":              `стандартного модуля "матем" нет`,
		"использовать время из стандарт/математика\nфункция главная { вывести 1 }\n":          `"время" - не из стандарт/математика, а из стандарт/время`,
		"использовать корень, корень из стандарт/математика\nфункция главная { вывести 1 }\n": `уже импортировано`,
		"использовать прочитать_строку из стандарт/строки\nфункция главная { вывести 1 }\n":   `в стандарт/строки нет "прочитать_строку"`,
	}
	for src, want := range cases {
		err := linkSource(t, src)
		if err == nil || !strings.Contains(err.Error(), want) {
			t.Errorf("%q: ожидалась ошибка с %q, получено %v", src, want, err)
		}
	}
	// Ядро - без импорта.
	if err := linkSource(t, "функция главная { выйти(0) }\n"); err != nil {
		t.Fatalf("ядро: %v", err)
	}
}

// Автоимпорт видит код всех платформ, дописывает в строку модуля и не
// трогает уже импортированное.
func TestFixStdImports(t *testing.T) {
	src := `использовать корень из стандарт/математика

#если ОС.Windows {
    функция платформенная(): Действительное
    {
        вернуть синус(1.0)
    }
} #иначе {
    функция платформенная(): Действительное
    {
        вернуть косинус(1.0)
    }
}

функция главная
{
    вывести корень(платформенная())
    вывести время()
}
`
	path := writeFile(t, t.TempDir(), "главный.rech", src)
	got, err := FixStdImports(path, src, testFrontend{}, "")
	if err != nil {
		t.Fatalf("FixStdImports: %v", err)
	}
	for _, want := range []string{"использовать корень, ", "синус", "косинус", "использовать время из стандарт/время\n"} {
		if !strings.Contains(got, want) {
			t.Errorf("ожидалось %q в:\n%s", want, got)
		}
	}
	again, _ := FixStdImports(path, got, testFrontend{}, "")
	if again != got {
		t.Errorf("повторный проход изменил текст:\n%s", again)
	}
}

func TestInsertStdImportPlacement(t *testing.T) {
	header := "// Шапка.\n\nфункция главная { }\n"
	if got := insertStdImport(header, "время"); got != "// Шапка.\n\nиспользовать время из стандарт/время\n\nфункция главная { }\n" {
		t.Errorf("после шапки: %q", got)
	}
	attached := "/** Документация функции. */\nфункция главная { }\n"
	if got := insertStdImport(attached, "время"); !strings.HasPrefix(got, "использовать время из стандарт/время\n\n/**") {
		t.Errorf("doc-комментарий не отрывается от объявления: %q", got)
	}
}
