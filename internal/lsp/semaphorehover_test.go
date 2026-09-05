package lsp

import (
	"strings"
	"testing"
)

const semaphoreHoverTestSrc = `
функция главная
{
    переменная сем = новый Семафор(2)
    сем.захватить()
    сем.освободить()
}
`

// TestHoverOnSemaphoreCtorShowsInitialValueParam - "Семафор", в отличие от
// "Блокировки", берёт ОДИН обязательный параметр конструктора - hover на
// имени типа должен показать его, а не общий путь "Тип()" (см. package doc
// entityCtorSignature, internal/lsp/hover.go).
func TestHoverOnSemaphoreCtorShowsInitialValueParam(t *testing.T) {
	d := newDoc("test://a.rech", semaphoreHoverTestSrc)
	if d.irProg == nil {
		t.Fatalf("документ не разобрался")
	}
	line := lineOf(t, semaphoreHoverTestSrc, "новый Семафор(2)")
	col := runeCol(t, "    переменная сем = новый Семафор(2)", "Семафор")
	text, ok := d.hover(line, col)
	if !ok {
		t.Fatalf("ожидался hover на имени типа Семафор")
	}
	if !strings.Contains(text, "новый Семафор(начальное_значение: Целое)") {
		t.Fatalf("неожиданный текст hover: %q", text)
	}
}

// TestHoverOnSemaphoreAcquireReleaseShowsDoc - "захватить"/"освободить" на
// значении "Семафор" синтезированы backend'ом (нет ent.Methods) - hover
// обязан их всё равно распознать (mutexMethodDocs, hoverMember).
func TestHoverOnSemaphoreAcquireReleaseShowsDoc(t *testing.T) {
	d := newDoc("test://a.rech", semaphoreHoverTestSrc)
	line := lineOf(t, semaphoreHoverTestSrc, "сем.захватить()")
	col := runeCol(t, "    сем.захватить()", "захватить")
	text, ok := d.hover(line, col)
	if !ok {
		t.Fatalf("ожидался hover на методе захватить")
	}
	if !strings.Contains(text, "Семафор.захватить()") {
		t.Fatalf("неожиданный текст hover: %q", text)
	}

	line2 := lineOf(t, semaphoreHoverTestSrc, "сем.освободить()")
	col2 := runeCol(t, "    сем.освободить()", "освободить")
	text2, ok := d.hover(line2, col2)
	if !ok {
		t.Fatalf("ожидался hover на методе освободить")
	}
	if !strings.Contains(text2, "Семафор.освободить()") {
		t.Fatalf("неожиданный текст hover: %q", text2)
	}
}

const mutexHoverTestSrc = `
функция главная
{
    переменная м = новая Блокировка()
    м.захватить()
    м.освободить()
}
`

// TestHoverOnMutexAcquireReleaseShowsDoc - тот же пробел, что и у "Семафора"
// выше, но у "Блокировки" - существовал ДО этой сессии незамеченным (нет
// ast.EntityDecl/ent.Methods ни у одной из двух системных сущностей).
func TestHoverOnMutexAcquireReleaseShowsDoc(t *testing.T) {
	d := newDoc("test://a.rech", mutexHoverTestSrc)
	if d.irProg == nil {
		t.Fatalf("документ не разобрался")
	}
	line := lineOf(t, mutexHoverTestSrc, "м.захватить()")
	col := runeCol(t, "    м.захватить()", "захватить")
	text, ok := d.hover(line, col)
	if !ok {
		t.Fatalf("ожидался hover на методе захватить")
	}
	if !strings.Contains(text, "Блокировка.захватить()") {
		t.Fatalf("неожиданный текст hover: %q", text)
	}
}
