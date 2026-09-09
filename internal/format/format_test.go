package format

import "testing"

func TestFormatReindentsNestedBlocks(t *testing.T) {
	src := "функция главная\n{\nпеременная i = 0\nпока i < 3\n{\nвывести i\ni = i + 1\n}\n}\n"
	want := "функция главная\n{\n    переменная i = 0\n    пока i < 3 {\n        вывести i\n        i = i + 1\n    }\n}\n"
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

// TestFormatCanonicalizesDocCommentAlignment - многострочный
// doc-комментарий с "кривым" авторским выравниванием (текст сразу за
// "/**", соседняя строка - под другим отступом) приводится к канону
// Javadoc/JSDoc - "/**" само по себе, каждая строка - " * <текст>" (см.
// package doc doc_comment.go - пользователь попросил явно, "докблоки
// должны тоже сами форматироваться").
func TestFormatCanonicalizesDocCommentAlignment(t *testing.T) {
	src := "/** Первая строка\n  * Вторая строка, отступ автора, не 4 пробела\n */\nфункция главная\n{\n    вывести 1\n}\n"
	want := "/**\n * Первая строка\n * Вторая строка, отступ автора, не 4 пробела\n */\nфункция главная\n{\n    вывести 1\n}\n"
	got, err := Format(src)
	if err != nil {
		t.Fatalf("неожиданная ошибка: %v", err)
	}
	if got != want {
		t.Fatalf("получено:\n%q\nожидалось:\n%q", got, want)
	}
	got2, err := Format(got)
	if err != nil {
		t.Fatalf("неожиданная ошибка при повторном форматировании: %v", err)
	}
	if got2 != got {
		t.Fatalf("не идемпотентно:\nпервый проход:\n%q\nвторой проход:\n%q", got, got2)
	}
}

// TestFormatSingleLineDocCommentUntouched - однострочный doc-комментарий
// ("/** ... */" без единого переноса строки, подавляющее большинство в
// репозитории) НЕ трогается вовсе.
func TestFormatSingleLineDocCommentUntouched(t *testing.T) {
	src := "/** Коротко. */\nфункция главная\n{\n    вывести 1\n}\n"
	got, err := Format(src)
	if err != nil {
		t.Fatalf("неожиданная ошибка: %v", err)
	}
	if got != src {
		t.Fatalf("получено:\n%q\nожидалось (без изменений):\n%q", got, src)
	}
}

// TestFormatDocCommentPreservesBlankParagraphLine - пустая строка СРЕДИ
// содержимого (разрыв абзаца) сохраняется как " *"; пустая строка сразу
// после "/**"/перед "*/" - нет (см. package doc doc_comment.go).
func TestFormatDocCommentPreservesBlankParagraphLine(t *testing.T) {
	src := "/**\n * Первый абзац.\n *\n * Второй абзац.\n */\nфункция главная\n{\n    вывести 1\n}\n"
	got, err := Format(src)
	if err != nil {
		t.Fatalf("неожиданная ошибка: %v", err)
	}
	if got != src {
		t.Fatalf("получено:\n%q\nожидалось (без изменений):\n%q", got, src)
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
	want := "функция сложить(а: Целое, б: Целое): Целое\n{\n    вернуть а + б\n}\n\nфункция умножить(а: Целое, б: Целое): Целое\n{\n    вернуть а * б\n}\n\nфункция главная\n{\n    сложив(10, 20)\n        вывести результат\n        умножив(результат, 2)\n            вывести результат\n        // комментарий на уровне внешнего тела, без пустой строки перед ним\n        вывести результат\n}\n"
	got, err := Format(src)
	if err != nil {
		t.Fatalf("неожиданная ошибка: %v", err)
	}
	if got != want {
		t.Fatalf("получено:\n%q\nожидалось:\n%q", got, want)
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
	want := "структура Пользователь\n{\n    имеет:\n        id:  Целое\n        имя: Строка\n}\n"
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
	want := "функция главная\n{\n    выбрать x {\n        когда 1:\n            вывести \"один\"\n        иначе:\n            вывести \"другое\"\n    }\n}\n"
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
	want := "функция чанковать(ф: Вызываемый(), доп: Строка)\n{\n    ф()\n}\n\nфункция главная\n{\n    чанковать(функция()\n    {\n        вывести 1\n    }, \"доп\")\n}\n"
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
// TestFormatIfElseChainMergesBraces - управляющие конструкции (если/
// иначе/для/пока/попытаться/поймать/окончательно/выбрать) - K&R: "{"
// заголовка и "}" предыдущей ветки/следующее продолжение - на одной
// строке, даже если автор развёл их по разным строкам (Allman).
func TestFormatIfElseChainMergesBraces(t *testing.T) {
	src := "функция главная\n{\n    если x > 5\n    {\n        вывести 1\n    }\n    иначе\n    {\n        если x > 2\n        {\n            вывести 2\n        }\n        иначе\n        {\n            вывести 3\n        }\n    }\n}\n"
	want := "функция главная\n{\n    если x > 5 {\n        вывести 1\n    } иначе {\n        если x > 2 {\n            вывести 2\n        } иначе {\n            вывести 3\n        }\n    }\n}\n"
	got, err := Format(src)
	if err != nil {
		t.Fatalf("неожиданная ошибка: %v", err)
	}
	if got != want {
		t.Fatalf("получено:\n%q\nожидалось:\n%q", got, want)
	}
	// Идемпотентность - повторное форматирование уже канонического текста
	// не меняет его.
	got2, err := Format(got)
	if err != nil {
		t.Fatalf("неожиданная ошибка при повторном форматировании: %v", err)
	}
	if got2 != got {
		t.Fatalf("не идемпотентно:\nпервый проход:\n%q\nвторой проход:\n%q", got, got2)
	}
}

// TestFormatTryCatchFinallyMergesBraces - "попытаться { ... } поймать
// (...) { ... } окончательно { ... }" - та же K&R-цепочка, что и у
// если/иначе, включая несколько "поймать" подряд.
func TestFormatTryCatchFinallyMergesBraces(t *testing.T) {
	src := "функция главная\n{\n    попытаться\n    {\n        вывести 1\n    }\n    поймать (е: ОшибкаА)\n    {\n        вывести 2\n    }\n    поймать (е: ОшибкаБ)\n    {\n        вывести 3\n    }\n    окончательно\n    {\n        вывести 4\n    }\n}\n"
	want := "функция главная\n{\n    попытаться {\n        вывести 1\n    } поймать (е: ОшибкаА) {\n        вывести 2\n    } поймать (е: ОшибкаБ) {\n        вывести 3\n    } окончательно {\n        вывести 4\n    }\n}\n"
	got, err := Format(src)
	if err != nil {
		t.Fatalf("неожиданная ошибка: %v", err)
	}
	if got != want {
		t.Fatalf("получено:\n%q\nожидалось:\n%q", got, want)
	}
}

// TestFormatFunctionKeepsAllmanBraceOnOwnLine - объявление функции с
// однострочным списком параметров - "{" остаётся (или переносится) на
// СОБСТВЕННУЮ строку (Allman), в отличие от управляющих конструкций.
func TestFormatFunctionKeepsAllmanBraceOnOwnLine(t *testing.T) {
	src := "функция сложить(а: Целое, б: Целое): Целое {\n    вернуть а + б\n}\n"
	want := "функция сложить(а: Целое, б: Целое): Целое\n{\n    вернуть а + б\n}\n"
	got, err := Format(src)
	if err != nil {
		t.Fatalf("неожиданная ошибка: %v", err)
	}
	if got != want {
		t.Fatalf("получено:\n%q\nожидалось:\n%q", got, want)
	}
	got2, err := Format(got)
	if err != nil {
		t.Fatalf("неожиданная ошибка при повторном форматировании: %v", err)
	}
	if got2 != got {
		t.Fatalf("не идемпотентно:\nпервый проход:\n%q\nвторой проход:\n%q", got, got2)
	}
}

// TestFormatEntityMethodKeepsAllmanBraceOnOwnLine - метод сущности (без
// ведущего "функция") - то же самое объявление с точки зрения стиля
// скобок, хотя у него нет собственного ключевого слова вовсе.
func TestFormatEntityMethodKeepsAllmanBraceOnOwnLine(t *testing.T) {
	src := "структура Точка\n{\n    длина(): Целое {\n        вернуть 1\n    }\n}\n"
	want := "структура Точка\n{\n    длина(): Целое\n    {\n        вернуть 1\n    }\n}\n"
	got, err := Format(src)
	if err != nil {
		t.Fatalf("неожиданная ошибка: %v", err)
	}
	if got != want {
		t.Fatalf("получено:\n%q\nожидалось:\n%q", got, want)
	}
}

// TestFormatMultilineParamsExceptionAttachesBrace - у функции с
// многострочным списком параметров (висячая запятая) "{" ОСТАЁТСЯ
// прикреплённой к строке "): Тип", даже если автор написал её отдельной
// строкой (Allman) - единственное исключение из правила выше.
func TestFormatMultilineParamsExceptionAttachesBrace(t *testing.T) {
	src := "функция сложить(\n    а: Целое,\n    б: Целое,\n): Целое\n{\n    вернуть а + б\n}\n"
	got, err := Format(src)
	if err != nil {
		t.Fatalf("неожиданная ошибка: %v", err)
	}
	want := "функция сложить(\n    а: Целое,\n    б: Целое,\n): Целое {\n    вернуть а + б\n}\n"
	if got != want {
		t.Fatalf("получено:\n%q\nожидалось:\n%q", got, want)
	}
}

// TestFormatAlignsFieldsLikeATable - "имеет:" секция без пустой строки
// между полями выравнивается в таблицу: по ":"/Тип - всегда, по "=" и
// тегу - только у тех полей, у которых они вообще есть (рваные строки,
// см. package doc alignEntityFields).
func TestFormatAlignsFieldsLikeATable(t *testing.T) {
	src := "структура Пользователь\n{\n    имеет:\n        id: Целое\n        имя: Строка = \"аноним\" `sql:name`\n        возраст: Целое = 0\n}\n"
	want := "структура Пользователь\n{\n    имеет:\n        id:      Целое\n        имя:     Строка = \"аноним\" `sql:name`\n        возраст: Целое  = 0\n}\n"
	got, err := Format(src)
	if err != nil {
		t.Fatalf("неожиданная ошибка: %v", err)
	}
	if got != want {
		t.Fatalf("получено:\n%q\nожидалось:\n%q", got, want)
	}
	got2, err := Format(got)
	if err != nil {
		t.Fatalf("неожиданная ошибка при повторном форматировании: %v", err)
	}
	if got2 != got {
		t.Fatalf("не идемпотентно:\nпервый проход:\n%q\nвторой проход:\n%q", got, got2)
	}
}

// TestFormatFieldAlignmentBreaksOnBlankLine - пустая строка между полями
// начинает НОВЫЙ пробег выравнивания, независимый от предыдущего.
func TestFormatFieldAlignmentBreaksOnBlankLine(t *testing.T) {
	src := "структура Точка\n{\n    имеет:\n        x: Целое\n        игрек: Целое\n\n        z: Целое\n}\n"
	want := "структура Точка\n{\n    имеет:\n        x:     Целое\n        игрек: Целое\n\n        z: Целое\n}\n"
	got, err := Format(src)
	if err != nil {
		t.Fatalf("неожиданная ошибка: %v", err)
	}
	if got != want {
		t.Fatalf("получено:\n%q\nожидалось:\n%q", got, want)
	}
}

// TestFormatExplodesInlineReturnInsideIf - "если x { вернуть 0 }" внутри
// НЕ функции (управляющая конструкция) - "вернуть" переносится на
// собственную строку, легко проглядеть иначе.
func TestFormatExplodesInlineReturnInsideIf(t *testing.T) {
	src := "функция ф(x: Целое): Целое\n{\n    если x < 0 { вернуть 0 }\n    вернуть x\n}\n"
	want := "функция ф(x: Целое): Целое\n{\n    если x < 0 {\n        вернуть 0\n    }\n    вернуть x\n}\n"
	got, err := Format(src)
	if err != nil {
		t.Fatalf("неожиданная ошибка: %v", err)
	}
	if got != want {
		t.Fatalf("получено:\n%q\nожидалось:\n%q", got, want)
	}
	got2, err := Format(got)
	if err != nil {
		t.Fatalf("неожиданная ошибка при повторном форматировании: %v", err)
	}
	if got2 != got {
		t.Fatalf("не идемпотентно:\nпервый проход:\n%q\nвторой проход:\n%q", got, got2)
	}
}

// TestFormatExplodesInlineBreakContinueInsideLoop - то же самое для
// "прервать"/"продолжить" внутри "пока".
func TestFormatExplodesInlineBreakContinueInsideLoop(t *testing.T) {
	src := "функция главная\n{\n    переменная i = 0\n    пока i < 10\n    {\n        если i == 5 { прервать }\n        если i == 2 { продолжить }\n        i++\n    }\n}\n"
	want := "функция главная\n{\n    переменная i = 0\n    пока i < 10 {\n        если i == 5 {\n            прервать\n        }\n        если i == 2 {\n            продолжить\n        }\n        i++\n    }\n}\n"
	got, err := Format(src)
	if err != nil {
		t.Fatalf("неожиданная ошибка: %v", err)
	}
	if got != want {
		t.Fatalf("получено:\n%q\nожидалось:\n%q", got, want)
	}
}

// TestFormatDoesNotExplodeInlineReturnInFunctionBody - однострочное тело
// ФУНКЦИИ ("функция ф(): Целое { вернуть 1 }") - исключение, не трогается
// (см. package doc explode_exit_stmt.go).
func TestFormatDoesNotExplodeInlineReturnInFunctionBody(t *testing.T) {
	src := "функция ф(): Целое { вернуть 1 }\n"
	got, err := Format(src)
	if err != nil {
		t.Fatalf("неожиданная ошибка: %v", err)
	}
	if got != src {
		t.Fatalf("однострочное тело функции не должно меняться:\nполучено:\n%q\nисходно:\n%q", got, src)
	}
}

// TestFormatAlreadyExplodedReturnStaysUnchanged - идемпотентность
// напрямую: уже правильно оформленный "если" не должен меняться вовсе.
func TestFormatAlreadyExplodedReturnStaysUnchanged(t *testing.T) {
	src := "функция ф(x: Целое): Целое\n{\n    если x < 0 {\n        вернуть 0\n    }\n    вернуть x\n}\n"
	got, err := Format(src)
	if err != nil {
		t.Fatalf("неожиданная ошибка: %v", err)
	}
	if got != src {
		t.Fatalf("получено:\n%q\nожидалось (без изменений):\n%q", got, src)
	}
}

// TestFormatInsertsBlankLineBetweenGluedFunctions - две функции без единой
// пустой строки между ними - вставляется ровно одна.
func TestFormatInsertsBlankLineBetweenGluedFunctions(t *testing.T) {
	src := "функция а(): Целое\n{\n    вернуть 1\n}\nфункция б(): Целое\n{\n    вернуть 2\n}\n"
	want := "функция а(): Целое\n{\n    вернуть 1\n}\n\nфункция б(): Целое\n{\n    вернуть 2\n}\n"
	got, err := Format(src)
	if err != nil {
		t.Fatalf("неожиданная ошибка: %v", err)
	}
	if got != want {
		t.Fatalf("получено:\n%q\nожидалось:\n%q", got, want)
	}
	got2, err := Format(got)
	if err != nil {
		t.Fatalf("неожиданная ошибка при повторном форматировании: %v", err)
	}
	if got2 != got {
		t.Fatalf("не идемпотентно:\nпервый проход:\n%q\nвторой проход:\n%q", got, got2)
	}
}

// TestFormatInsertsBlankLineBetweenGluedMethods - то же самое для методов
// одной сущности - первый метод (сразу после "{" сущности) НЕ получает
// лишнюю пустую строку, второй (слитно после первого) - получает.
func TestFormatInsertsBlankLineBetweenGluedMethods(t *testing.T) {
	src := "структура Точка\n{\n    x(): Целое\n    {\n        вернуть 1\n    }\n    y(): Целое\n    {\n        вернуть 2\n    }\n}\n"
	want := "структура Точка\n{\n    x(): Целое\n    {\n        вернуть 1\n    }\n\n    y(): Целое\n    {\n        вернуть 2\n    }\n}\n"
	got, err := Format(src)
	if err != nil {
		t.Fatalf("неожиданная ошибка: %v", err)
	}
	if got != want {
		t.Fatalf("получено:\n%q\nожидалось:\n%q", got, want)
	}
}

// TestFormatDoesNotSeparateDocCommentFromItsFunction - doc-комментарий,
// слитно предшествующий второй функции - пустая строка вставляется ПЕРЕД
// ним, не между ним и функцией.
func TestFormatDoesNotSeparateDocCommentFromItsFunction(t *testing.T) {
	src := "функция а(): Целое\n{\n    вернуть 1\n}\n/** Документация б. */\nфункция б(): Целое\n{\n    вернуть 2\n}\n"
	want := "функция а(): Целое\n{\n    вернуть 1\n}\n\n/** Документация б. */\nфункция б(): Целое\n{\n    вернуть 2\n}\n"
	got, err := Format(src)
	if err != nil {
		t.Fatalf("неожиданная ошибка: %v", err)
	}
	if got != want {
		t.Fatalf("получено:\n%q\nожидалось:\n%q", got, want)
	}
}

// TestFormatAddsTrailingCommaToMultilineParamList - список параметров,
// реально разложенный по одному на строку (уже есть перенос после первой
// запятой), но БЕЗ висячей запятой у последнего - она добавляется.
func TestFormatAddsTrailingCommaToMultilineParamList(t *testing.T) {
	src := "функция сложить(\n    а: Целое,\n    б: Целое\n): Целое {\n    вернуть а + б\n}\n"
	want := "функция сложить(\n    а: Целое,\n    б: Целое,\n): Целое {\n    вернуть а + б\n}\n"
	got, err := Format(src)
	if err != nil {
		t.Fatalf("неожиданная ошибка: %v", err)
	}
	if got != want {
		t.Fatalf("получено:\n%q\nожидалось:\n%q", got, want)
	}
	got2, err := Format(got)
	if err != nil {
		t.Fatalf("неожиданная ошибка при повторном форматировании: %v", err)
	}
	if got2 != got {
		t.Fatalf("не идемпотентно:\nпервый проход:\n%q\nвторой проход:\n%q", got, got2)
	}
}

// TestFormatDoesNotAddTrailingCommaOnSingleLine - однострочный список
// (даже если он длинный) НЕ получает висячую запятую - грамматика
// (internal/parser/parser.go, skipNewlinesAfterListComma) требует, чтобы
// висячая запятая сопровождалась переносом закрывающей скобки на свою
// строку, значит на одной строке такая запятая в принципе невозможна.
func TestFormatDoesNotAddTrailingCommaOnSingleLine(t *testing.T) {
	src := "функция сложить(а: Целое, б: Целое): Целое\n{\n    вернуть а + б\n}\n"
	got, err := Format(src)
	if err != nil {
		t.Fatalf("неожиданная ошибка: %v", err)
	}
	if got != src {
		t.Fatalf("получено:\n%q\nожидалось (без изменений):\n%q", got, src)
	}
}

// TestFormatAddsTrailingCommaToMultilineListLiteral - то же самое для
// литерала списка "[...]".
func TestFormatAddsTrailingCommaToMultilineListLiteral(t *testing.T) {
	src := "функция главная\n{\n    переменная x = [\n        1,\n        2\n    ]\n}\n"
	want := "функция главная\n{\n    переменная x = [\n        1,\n        2,\n    ]\n}\n"
	got, err := Format(src)
	if err != nil {
		t.Fatalf("неожиданная ошибка: %v", err)
	}
	if got != want {
		t.Fatalf("получено:\n%q\nожидалось:\n%q", got, want)
	}
}

// TestFormatAlignsConsecutiveVarDecls - соседние "переменная имя = ..."
// выравниваются по "=" (то же самое, что gofmt делает внутри "var (...)").
func TestFormatAlignsConsecutiveVarDecls(t *testing.T) {
	src := "функция главная\n{\n    переменная i = 0\n    переменная сумма = 0\n    вывести сумма\n}\n"
	want := "функция главная\n{\n    переменная i     = 0\n    переменная сумма = 0\n    вывести сумма\n}\n"
	got, err := Format(src)
	if err != nil {
		t.Fatalf("неожиданная ошибка: %v", err)
	}
	if got != want {
		t.Fatalf("получено:\n%q\nожидалось:\n%q", got, want)
	}
	got2, err := Format(got)
	if err != nil {
		t.Fatalf("неожиданная ошибка при повторном форматировании: %v", err)
	}
	if got2 != got {
		t.Fatalf("не идемпотентно:\nпервый проход:\n%q\nвторой проход:\n%q", got, got2)
	}
}

// TestFormatVarDeclAlignmentBreaksOnOtherStatement - инструкция МЕЖДУ
// двумя "переменная" обрывает пробег выравнивания - второй "переменная" не
// выравнивается с первым.
func TestFormatVarDeclAlignmentBreaksOnOtherStatement(t *testing.T) {
	src := "функция главная\n{\n    переменная i = 0\n    вывести i\n    переменная сумма = 0\n}\n"
	got, err := Format(src)
	if err != nil {
		t.Fatalf("неожиданная ошибка: %v", err)
	}
	if got != src {
		t.Fatalf("получено:\n%q\nожидалось (без изменений):\n%q", got, src)
	}
}

func TestFormatMultilineCallTrailingBraceParen(t *testing.T) {
	src := "функция обработать(ф: Вызываемый())\n{\n    ф()\n}\nфункция главная\n{\n    обработать(функция()\n    {\n    вывести 1\n})\n}\n"
	want := "функция обработать(ф: Вызываемый())\n{\n    ф()\n}\n\nфункция главная\n{\n    обработать(функция()\n    {\n        вывести 1\n    })\n}\n"
	got, err := Format(src)
	if err != nil {
		t.Fatalf("неожиданная ошибка: %v", err)
	}
	if got != want {
		t.Fatalf("получено:\n%q\nожидалось:\n%q", got, want)
	}
}
