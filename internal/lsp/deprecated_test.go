package lsp

import "testing"

const deprecatedTestSrc = `
/** Старая версия. @устарело используйте свежую_версию() вместо этого. */
функция старая_версия(): Целое
{
    вернуть 1
}

функция свежая_версия(): Целое
{
    вернуть 2
}

/** @устарело */
сущность СтараяСущность
{
    имеет:
        x: Целое
}

сущность Контейнер
{
    /** @устарело используйте новыйМетод() */
    старыйМетод(): Целое
    {
        вернуть 0
    }

    новыйМетод(): Целое
    {
        вернуть 1
    }
}

функция главная
{
    вывести старая_версия()
    вывести свежая_версия()
    переменная старая_с = новая СтараяСущность()
    переменная контейнер = новый Контейнер()
    вывести контейнер.старыйМетод()
    вывести контейнер.новыйМетод()
}
`

func TestDeprecatedInfoExtractsMessage(t *testing.T) {
	msg, ok := deprecatedInfo("Старая версия. @устарело используйте свежую_версию() вместо этого.")
	if !ok {
		t.Fatalf("ожидалось обнаружение @устарело")
	}
	if msg != "устарело: используйте свежую_версию() вместо этого." {
		t.Fatalf("неожиданное сообщение: %q", msg)
	}
}

func TestDeprecatedInfoWithoutTextGivesGenericMessage(t *testing.T) {
	msg, ok := deprecatedInfo("@устарело")
	if !ok || msg != "устарело" {
		t.Fatalf("ожидалось общее сообщение \"устарело\", получено %q (ok=%v)", msg, ok)
	}
}

func TestDeprecatedInfoAbsentReturnsFalse(t *testing.T) {
	if _, ok := deprecatedInfo("обычный doc-комментарий без пометки"); ok {
		t.Fatalf("не ожидалось обнаружение @устарело")
	}
}

// TestDeprecatedDiagnosticsMarksFunctionCall - вызов "старая_версия()"
// должен получить Diagnostic с тегом "устарело" - объявление функции и
// вызов "свежая_версия()" - нет.
func TestDeprecatedDiagnosticsMarksFunctionCall(t *testing.T) {
	d := newDoc("test://a.rech", deprecatedTestSrc)
	if d.irProg == nil {
		t.Fatalf("ожидался успешный разбор")
	}
	diags := d.deprecatedDiagnostics()

	wantLine := lineOf(t, deprecatedTestSrc, "вывести старая_версия()")
	found := false
	for _, diag := range diags {
		if diag.Line == wantLine {
			found = true
			if diag.Severity != hintSeverity {
				t.Fatalf("ожидался severity=%d, получено %d", hintSeverity, diag.Severity)
			}
			if len(diag.Tags) != 1 || diag.Tags[0] != deprecatedTag {
				t.Fatalf("ожидался тег deprecated, получено %+v", diag.Tags)
			}
		}
	}
	if !found {
		t.Fatalf("ожидалась диагностика на строке %d (вызов устаревшей функции), получено %+v", wantLine, diags)
	}

	freshLine := lineOf(t, deprecatedTestSrc, "вывести свежая_версия()")
	for _, diag := range diags {
		if diag.Line == freshLine {
			t.Fatalf("не ожидалась диагностика на вызове НЕустаревшей функции, получено %+v", diag)
		}
	}
}

// TestDeprecatedDiagnosticsMarksEntityConstruction - "новая СтараяСущность()"
// должно получить диагностику ("устарело" без своего текста).
func TestDeprecatedDiagnosticsMarksEntityConstruction(t *testing.T) {
	d := newDoc("test://a.rech", deprecatedTestSrc)
	diags := d.deprecatedDiagnostics()

	wantLine := lineOf(t, deprecatedTestSrc, "новая СтараяСущность()")
	for _, diag := range diags {
		if diag.Line == wantLine && diag.Message == "устарело" {
			return
		}
	}
	t.Fatalf("ожидалась диагностика \"устарело\" на строке %d, получено %+v", wantLine, diags)
}

// TestDeprecatedDiagnosticsMarksMethodCall - "контейнер.старыйМетод()"
// должно получить диагностику, "контейнер.новыйМетод()" - нет.
func TestDeprecatedDiagnosticsMarksMethodCall(t *testing.T) {
	d := newDoc("test://a.rech", deprecatedTestSrc)
	diags := d.deprecatedDiagnostics()

	oldLine := lineOf(t, deprecatedTestSrc, "контейнер.старыйМетод()")
	newLine := lineOf(t, deprecatedTestSrc, "контейнер.новыйМетод()")
	foundOld := false
	for _, diag := range diags {
		if diag.Line == oldLine {
			foundOld = true
		}
		if diag.Line == newLine {
			t.Fatalf("не ожидалась диагностика на вызове НЕустаревшего метода, получено %+v", diag)
		}
	}
	if !foundOld {
		t.Fatalf("ожидалась диагностика на строке %d (вызов устаревшего метода), получено %+v", oldLine, diags)
	}
}
