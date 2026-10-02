package lsp

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// findLenses - ВСЕ CodeLens (codelens.go) на строке line, в порядке
// codeLenses() - в отличие от findLens (references_test.go, первое
// совпадение), метод с переопределениями теперь несёт ДВА CodeLens на
// одной строке ("N ссылок" + "N переопределений").
func findLenses(lenses []CodeLens, line int) []CodeLens {
	var out []CodeLens
	for _, cl := range lenses {
		if cl.Line == line {
			out = append(out, cl)
		}
	}
	return out
}

func lensTitles(lenses []CodeLens) []string {
	titles := make([]string, len(lenses))
	for i, cl := range lenses {
		titles[i] = cl.Title
	}
	return titles
}

func containsTitle(lenses []CodeLens, title string) bool {
	for _, cl := range lenses {
		if cl.Title == title {
			return true
		}
	}
	return false
}

// Комментарии "// объявление"/"// переопределение" - ЧИСТО для теста
// (lineOf, hover_test.go, ищет строку по подстроке - у абстрактного и
// переопределяющего метода иначе БУКВАЛЬНО одинаковая строка "площадь():
// Действительное", различить их по тексту было бы нечем).
const overrideTestSrc = `
абстрактная сущность Фигура
{
    площадь(): Действительное // объявление
}

сущность Круг наследует Фигуру
{
    имеет:
        Действительное радиус

    площадь(): Действительное // переопределение
    {
        вернуть этот.радиус
    }
}

функция главная { вывести 1 }
`

// TestCodeLensShowsOverrideCountForAbstractMethod - "площадь" у "Фигура"
// переопределена ровно один раз, в "Круг".
func TestCodeLensShowsOverrideCountForAbstractMethod(t *testing.T) {
	d := newDoc("test://a.rech", overrideTestSrc)
	lenses := d.codeLenses()
	line := lineOf(t, overrideTestSrc, "// объявление")
	at := findLenses(lenses, line)
	if !containsTitle(at, "1 переопределение") {
		t.Fatalf("ожидалось \"1 переопределение\" на \"площадь\" Фигуры, получено: %v", lensTitles(at))
	}
}

// TestCodeLensOmitsOverrideCountWhenNoOverrides - метод "Круг.площадь" сам
// никем не переопределён (у "Круг" нет наследников) - лишней надписи "0
// переопределений" быть не должно, в отличие от "N ссылок" (та
// показывается и при нуле).
func TestCodeLensOmitsOverrideCountWhenNoOverrides(t *testing.T) {
	d := newDoc("test://a.rech", overrideTestSrc)
	lenses := d.codeLenses()
	line := lineOf(t, overrideTestSrc, "// переопределение")
	at := findLenses(lenses, line)
	for _, cl := range at {
		if cl.Title != "0 ссылок" && cl.Title != "1 ссылка" {
			t.Fatalf("не ожидался CodeLens переопределений на методе без наследников: %v", lensTitles(at))
		}
	}
}

// TestCodeLensShowsOverrideCountTransitively - Фигура <- Основа
// (сама абстрактная, НЕ переопределяет) <- Круг (переопределяет) -
// переопределение засчитывается ТРАНЗИТИВНО, через всю цепочку "наследует",
// не только у прямого наследника.
func TestCodeLensShowsOverrideCountTransitively(t *testing.T) {
	src := `
абстрактная сущность Фигура
{
    площадь(): Действительное // объявление
}

абстрактная сущность Основа наследует Фигуру
{
}

сущность Круг наследует Основу
{
    имеет:
        Действительное радиус

    площадь(): Действительное
    {
        вернуть этот.радиус
    }
}

функция главная { вывести 1 }
`
	d := newDoc("test://a.rech", src)
	lenses := d.codeLenses()
	line := lineOf(t, src, "// объявление")
	at := findLenses(lenses, line)
	if !containsTitle(at, "1 переопределение") {
		t.Fatalf("ожидалось транзитивное \"1 переопределение\" через Основу, получено: %v", lensTitles(at))
	}
}

// TestCodeLensShowsOverrideCountForMultipleDirectOverriders - два прямых
// наследника "Фигура", каждый переопределяет "площадь" - счётчик и
// склонение "2 переопределения".
func TestCodeLensShowsOverrideCountForMultipleDirectOverriders(t *testing.T) {
	src := `
абстрактная сущность Фигура
{
    площадь(): Действительное // объявление
}

сущность Круг наследует Фигуру
{
    площадь(): Действительное
    {
        вернуть 1.0
    }
}

сущность Квадрат наследует Фигуру
{
    площадь(): Действительное
    {
        вернуть 2.0
    }
}

функция главная { вывести 1 }
`
	d := newDoc("test://a.rech", src)
	lenses := d.codeLenses()
	line := lineOf(t, src, "// объявление")
	at := findLenses(lenses, line)
	if !containsTitle(at, "2 переопределения") {
		t.Fatalf("ожидалось \"2 переопределения\" от двух прямых наследников, получено: %v", lensTitles(at))
	}
}

// TestCodeLensCountsOverridesFromOtherFiles - переопределение в ДРУГОМ
// файле дерева (обычное дело - базовая сущность и её наследники нередко
// живут в разных файлах) - тот же принцип, что и у
// TestCodeLensCountsReferencesFromOtherFiles (references_test.go).
func TestCodeLensCountsOverridesFromOtherFiles(t *testing.T) {
	dir := t.TempDir()
	baseDir := filepath.Join(dir, "исходники", "фигуры")
	circleDir := filepath.Join(dir, "исходники", "круг")
	if err := os.MkdirAll(baseDir, 0o755); err != nil {
		t.Fatalf("не удалось создать каталог: %v", err)
	}
	if err := os.MkdirAll(circleDir, 0o755); err != nil {
		t.Fatalf("не удалось создать каталог: %v", err)
	}
	if err := os.WriteFile(filepath.Join(dir, "тест.rechproj"), []byte("проект:\n  имя: тест\nфайлы:\n  главный: исходники/фигуры/фигура.rech\n"), 0o644); err != nil {
		t.Fatalf("не удалось записать .rechproj: %v", err)
	}
	const baseSrc = `абстрактная сущность Фигура
{
    площадь(): Действительное
}
`
	const circleSrc = `использовать Фигура из "../фигуры/фигура"

сущность Круг наследует Фигуру
{
    площадь(): Действительное
    {
        вернуть 1.0
    }
}

функция главная { вывести 1 }
`
	basePath := writeMultiFileSource(t, baseDir, "фигура.rech", baseSrc)
	writeMultiFileSource(t, circleDir, "круг.rech", circleSrc)

	d := newDoc(fileURI(basePath), baseSrc)
	lenses := d.codeLenses()
	line := lineOf(t, baseSrc, "площадь(): Действительное")
	at := findLenses(lenses, line)
	if !containsTitle(at, "1 переопределение") {
		t.Fatalf("ожидалось \"1 переопределение\" из другого файла, получено: %v", lensTitles(at))
	}
	for _, cl := range at {
		if cl.Title == "1 переопределение" {
			if len(cl.Refs) != 1 || cl.Refs[0].File != filepath.Join(circleDir, "круг.rech") {
				t.Fatalf("ожидалась 1 позиция переопределения в круг.rech, получено: %+v", cl.Refs)
			}
		}
	}
}

// TestCodeLensOverrideCountMatchesParamTypes - у потомка одноимённый метод
// с другим списком параметров - перегрузка, не переопределение: каждому
// базовому варианту засчитывается только вариант с тем же списком.
func TestCodeLensOverrideCountMatchesParamTypes(t *testing.T) {
	src := `
сущность Фигура
{
    публичный описать(): Строка // без параметров
    {
        вернуть "фигура"
    }

    публичный описать(имя: Строка): Строка // со строкой
    {
        вернуть имя
    }
}

сущность Круг наследует Фигуру
{
    публичный описать(): Строка
    {
        вернуть "круг"
    }

    публичный описать(подробно: Логическое): Строка
    {
        вернуть "круг"
    }
}

функция главная { вывести 1 }
`
	d := newDoc("test://a.rech", src)
	lenses := d.codeLenses()
	if at := findLenses(lenses, lineOf(t, src, "// без параметров")); !containsTitle(at, "1 переопределение") {
		t.Fatalf("ожидалось \"1 переопределение\" у описать(), получено: %v", lensTitles(at))
	}
	for _, cl := range findLenses(lenses, lineOf(t, src, "// со строкой")) {
		if strings.Contains(cl.Title, "переопредел") {
			t.Fatalf("у описать(Строка) нет переопределений, получено: %v", cl.Title)
		}
	}
}
