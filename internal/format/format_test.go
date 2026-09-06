package format

import "testing"

func TestFormatReindentsNestedBlocks(t *testing.T) {
	src := "функция главная\n{\nпеременная i = 0\nпока i < 3\n{\nвывести i\ni = i + 1\n}\n}\n"
	want := "функция главная\n{\n    переменная i = 0\n    пока i < 3\n    {\n        вывести i\n        i = i + 1\n    }\n}\n"
	got, err := Format(src)
	if err != nil {
		t.Fatalf("неожиданная ошибка: %v", err)
	}
	if got != want {
		t.Fatalf("отступы не совпадают:\nполучено:\n%q\nожидалось:\n%q", got, want)
	}
}

// TestFormatPreservesAlreadyCanonical - уже отформатированный код не
// меняется (идемпотентность форматирования).
func TestFormatPreservesAlreadyCanonical(t *testing.T) {
	src := "функция главная\n{\n    вывести \"ок\"\n}\n"
	got, err := Format(src)
	if err != nil {
		t.Fatalf("неожиданная ошибка: %v", err)
	}
	if got != src {
		t.Fatalf("ожидалась идемпотентность:\nполучено:\n%q\nисходно:\n%q", got, src)
	}
}

// TestFormatSquashesExtraBlankLines - три пустые строки подряд схлопываются
// в одну.
func TestFormatSquashesExtraBlankLines(t *testing.T) {
	src := "функция главная\n{\n    вывести 1\n\n\n\n    вывести 2\n}\n"
	want := "функция главная\n{\n    вывести 1\n\n    вывести 2\n}\n"
	got, err := Format(src)
	if err != nil {
		t.Fatalf("неожиданная ошибка: %v", err)
	}
	if got != want {
		t.Fatalf("получено:\n%q\nожидалось:\n%q", got, want)
	}
}

// TestFormatLeavesMultilineTemplateStringVerbatim - перенос строки внутри
// шаблонной строки (обратные кавычки, docs/SPEC.md) - НЕ код, отступ строк
// внутри нее трогать нельзя.
func TestFormatLeavesMultilineTemplateStringVerbatim(t *testing.T) {
	src := "функция главная\n{\n    вывести `первая строка\nвторая строка, с    непримечательными пробелами`\n}\n"
	got, err := Format(src)
	if err != nil {
		t.Fatalf("неожиданная ошибка: %v", err)
	}
	want := "функция главная\n{\n    вывести `первая строка\nвторая строка, с    непримечательными пробелами`\n}\n"
	if got != want {
		t.Fatalf("многострочный литерал искажён:\nполучено:\n%q\nожидалось:\n%q", got, want)
	}
}

// TestFormatLeavesDocCommentVerbatim - doc-комментарий (docs/SPEC.md) со
// своим собственным ручным выравниванием строк ("* ...") не переотступается.
func TestFormatLeavesDocCommentVerbatim(t *testing.T) {
	src := "/** Первая строка\n  * Вторая строка, отступ автора, не 4 пробела\n */\nфункция главная\n{\n    вывести 1\n}\n"
	got, err := Format(src)
	if err != nil {
		t.Fatalf("неожиданная ошибка: %v", err)
	}
	if got != src {
		t.Fatalf("doc-комментарий искажён:\nполучено:\n%q\nисходно:\n%q", got, src)
	}
}

// TestFormatRejectsSyntaxError - синтаксически некорректный код не
// форматируется молча - явная ошибка, тот же принцип, что и у gofmt/
// go/parser.
func TestFormatRejectsSyntaxError(t *testing.T) {
	_, err := Format("функция главная {\n")
	if err == nil {
		t.Fatal("ожидалась ошибка парсера у незакрытого блока")
	}
}

// TestFormatCommentOnlyLineRightAfterOpenBraceGetsInnerDepth - "//"-строка
// сразу после "{" - лексер съедает такой комментарий целиком, ни одного
// токена на этой строке не остаётся - обязана получить глубину СОДЕРЖИМОГО
// блока (как и следующая за ней настоящая строка кода), а не глубину строки
// с самой "{" (глубина ДО открытия).
func TestFormatCommentOnlyLineRightAfterOpenBraceGetsInnerDepth(t *testing.T) {
	src := "функция главная\n{\n// комментарий\nвывести 1\n}\n"
	want := "функция главная\n{\n    // комментарий\n    вывести 1\n}\n"
	got, err := Format(src)
	if err != nil {
		t.Fatalf("неожиданная ошибка: %v", err)
	}
	if got != want {
		t.Fatalf("получено:\n%q\nожидалось:\n%q", got, want)
	}
}

// TestFormatGerundNestedBodyDanglingCommentUsesOwnColumn - деепричастный
// вызов (значимый отступ, internal/parser/gerund.go, без "{}") вложенный в
// другой такой же - висячий "//"-комментарий МЕЖДУ концом внутреннего тела
// и следующей инструкцией внешнего, БЕЗ единой пустой строки-разделителя -
// единственный сигнал, какому уровню он принадлежит, это СОБСТВЕННЫЙ отступ
// комментария в исходнике (лексер не даёт ему токена вовсе).
func TestFormatGerundNestedBodyDanglingCommentUsesOwnColumn(t *testing.T) {
	src := "функция сложить(а: Целое, б: Целое): Целое\n{\n    вернуть а + б\n}\nфункция умножить(а: Целое, б: Целое): Целое\n{\n    вернуть а * б\n}\nфункция главная\n{\n    сложив(10, 20)\n        вывести результат\n        умножив(результат, 2)\n            вывести результат\n        // комментарий на уровне внешнего тела, без пустой строки перед ним\n        вывести результат\n}\n"
	got, err := Format(src)
	if err != nil {
		t.Fatalf("неожиданная ошибка: %v", err)
	}
	if got != src {
		t.Fatalf("получено:\n%q\nожидалось (без изменений):\n%q", got, src)
	}
}

// TestFormatEscapedNewlineInStringDoesNotDesyncLineTracking - "\n"/"\r\n"
// ВНУТРИ обычной строки в двойных кавычках - экранированная
// последовательность, раскодированная в настоящий '\n'/'\r' символ ЗНАЧЕНИЯ
// токена, но НЕ настоящий перенос строки исходника (строка физически на
// одной строке) - подсчёт "\n" в Value такого STRING (в отличие от TAG/
// DOC_COMMENT, которые ДЕЙСТВИТЕЛЬНО можно перенести) не должен путать
// дальнейшее отслеживание строк.
func TestFormatEscapedNewlineInStringDoesNotDesyncLineTracking(t *testing.T) {
	src := "функция главная\n{\n    вывести \"a\\r\\nb\"\n    вывести 1\n}\n"
	got, err := Format(src)
	if err != nil {
		t.Fatalf("неожиданная ошибка: %v", err)
	}
	if got != src {
		t.Fatalf("получено:\n%q\nожидалось (без изменений):\n%q", got, src)
	}
}

// TestFormatHasSectionFieldsIndentedDeeperThanLabel - "имеет:" (docs/SPEC.md)
// - единственная секция со значимым отступом, не скобками: поля обязаны
// получить отступ СТРОГО глубже самой метки "имеет:", иначе распарсились бы
// уже не как поля вовсе (internal/parser/entity.go, parseFieldsSection).
func TestFormatHasSectionFieldsIndentedDeeperThanLabel(t *testing.T) {
	src := "структура Пользователь\n{\nимеет:\n id: Целое\n имя: Строка\n}\n"
	want := "структура Пользователь\n{\n    имеет:\n        id: Целое\n        имя: Строка\n}\n"
	got, err := Format(src)
	if err != nil {
		t.Fatalf("неожиданная ошибка: %v", err)
	}
	if got != want {
		t.Fatalf("получено:\n%q\nожидалось:\n%q", got, want)
	}
}

// TestFormatDanglingDocCommentDoesNotExtendHasSection - doc-комментарий,
// висящий МЕЖДУ последним полем "имеет:" и СЛЕДУЮЩИМ членом сущности
// (не полем) - документация этого следующего члена, не части секции полей
// (internal/parser/entity.go, parseFieldsSection, package doc) - остаётся
// на внешнем уровне отступа, не получает +1 секции.
func TestFormatDanglingDocCommentDoesNotExtendHasSection(t *testing.T) {
	src := "абстрактная сущность Фигура\n{\n    имеет:\n        имя: Строка\n\n    /** Висит перед методом, не полем. */\n    площадь(): Действительное\n}\n"
	got, err := Format(src)
	if err != nil {
		t.Fatalf("неожиданная ошибка: %v", err)
	}
	if got != src {
		t.Fatalf("получено:\n%q\nожидалось (без изменений):\n%q", got, src)
	}
}

// TestFormatDanglingPlainCommentDoesNotExtendHasSection - тот же случай,
// что TestFormatDanglingDocCommentDoesNotExtendHasSection, но обычным
// "//"-комментарием, который лексер съедает целиком (ни одного токена на
// этой строке нет вовсе) - секцию "имеет:" от него отделяет пустая строка,
// признак висячего хвоста здесь другой (см. package doc
// applyHasSectionIndent).
func TestFormatDanglingPlainCommentDoesNotExtendHasSection(t *testing.T) {
	src := "сущность Счётчик\n{\n    имеет:\n        значение: Целое\n\n    // Комментарий висит перед конструктором, не полем.\n    конструктор(начало: Целое)\n    {\n        этот.значение = начало\n    }\n}\n"
	got, err := Format(src)
	if err != nil {
		t.Fatalf("неожиданная ошибка: %v", err)
	}
	if got != src {
		t.Fatalf("получено:\n%q\nожидалось (без изменений):\n%q", got, src)
	}
}

// TestFormatSwitchCaseBodyIndentedDeeperThanLabel - "выбрать { когда .../
// иначе: ... }" (docs/ROADMAP.md, feature_switch_vybrat_kogda) - ветка не
// заводит своих "{}", но её инструкции печатаются на уровень глубже метки,
// тем же принципом, что и "имеет:" (но без значимого отступа - границу
// решает сам токен "когда"/"иначе:"/закрывающая "}", не Col).
func TestFormatSwitchCaseBodyIndentedDeeperThanLabel(t *testing.T) {
	src := "функция главная\n{\n    выбрать x\n    {\n    когда 1:\n    вывести \"один\"\n    иначе:\n    вывести \"другое\"\n    }\n}\n"
	want := "функция главная\n{\n    выбрать x\n    {\n        когда 1:\n            вывести \"один\"\n        иначе:\n            вывести \"другое\"\n    }\n}\n"
	got, err := Format(src)
	if err != nil {
		t.Fatalf("неожиданная ошибка: %v", err)
	}
	if got != want {
		t.Fatalf("получено:\n%q\nожидалось:\n%q", got, want)
	}
}

// TestFormatCallbackNotLastArgumentStaysFlat - колбэк НЕ последним
// аргументом (с ещё аргументами после закрывающей "}" на той же строке) -
// тот же "плоский" стиль, что и колбэк последним аргументом, ни одна из
// СВОИХ (не вложенных) запятых внешнего вызова не сопровождается переносом
// строки - только вложенное тело колбэка.
func TestFormatCallbackNotLastArgumentStaysFlat(t *testing.T) {
	src := "функция чанковать(ф: Вызываемый(), доп: Строка)\n{\n    ф()\n}\nфункция главная\n{\n    чанковать(функция()\n    {\n    вывести 1\n}, \"доп\")\n}\n"
	want := "функция чанковать(ф: Вызываемый(), доп: Строка)\n{\n    ф()\n}\nфункция главная\n{\n    чанковать(функция()\n    {\n        вывести 1\n    }, \"доп\")\n}\n"
	got, err := Format(src)
	if err != nil {
		t.Fatalf("неожиданная ошибка: %v", err)
	}
	if got != want {
		t.Fatalf("получено:\n%q\nожидалось:\n%q", got, want)
	}
}

// TestFormatMultilineParamsWithTrailingCommaGetOwnDepth - в отличие от
// висящего колбэка последним аргументом (см. ниже), у PSR-12 многострочного
// списка параметров (висячая запятая перед СВОЕЙ отдельной строкой ")")
// каждый параметр получает СВОЙ уровень отступа - висячая запятая перед
// закрывающей скобкой отличает этот случай от колбэка без запятой.
func TestFormatMultilineParamsWithTrailingCommaGetOwnDepth(t *testing.T) {
	src := "функция сложить(\nа: Целое,\nб: Целое,\n): Целое {\nвернуть а + б\n}\n"
	want := "функция сложить(\n    а: Целое,\n    б: Целое,\n): Целое {\n    вернуть а + б\n}\n"
	got, err := Format(src)
	if err != nil {
		t.Fatalf("неожиданная ошибка: %v", err)
	}
	if got != want {
		t.Fatalf("получено:\n%q\nожидалось:\n%q", got, want)
	}
}

// TestFormatMultilineCallTrailingBraceParen - строка "})" (конец лямбды,
// сразу закрывающая внешний вызов) получает отступ ПЕРВОГО закрывающего
// токена строки.
func TestFormatMultilineCallTrailingBraceParen(t *testing.T) {
	src := "функция обработать(ф: Вызываемый())\n{\n    ф()\n}\nфункция главная\n{\n    обработать(функция()\n    {\n    вывести 1\n})\n}\n"
	want := "функция обработать(ф: Вызываемый())\n{\n    ф()\n}\nфункция главная\n{\n    обработать(функция()\n    {\n        вывести 1\n    })\n}\n"
	got, err := Format(src)
	if err != nil {
		t.Fatalf("неожиданная ошибка: %v", err)
	}
	if got != want {
		t.Fatalf("получено:\n%q\nожидалось:\n%q", got, want)
	}
}
