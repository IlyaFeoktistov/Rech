package semantic

import (
	"errors"
	"strings"
	"testing"

	"github.com/IlyaFeoktistov/Rech/internal/diagnostics"
)

// diagMessages возвращает тексты всех диагностик err - саму *diagnostics.
// Diagnostic (одна диагностика) или все накопленные *diagnostics.Bag
// (несколько) - тем же путём, что и cmd/rech.compileErrorText.
func diagMessages(t *testing.T, err error) []string {
	t.Helper()
	if err == nil {
		t.Fatalf("ожидалась ошибка анализа, получено nil")
	}
	var bag *diagnostics.Bag
	if errors.As(err, &bag) {
		var out []string
		for _, d := range bag.Diagnostics() {
			out = append(out, d.Error())
		}
		return out
	}
	return []string{err.Error()}
}

func containsSubstring(msgs []string, substr string) bool {
	for _, m := range msgs {
		if strings.Contains(m, substr) {
			return true
		}
	}
	return false
}

// TestCollectEntitiesCollectsIndependentErrors проверяет, что ошибка ОДНОЙ
// сущности (collectOneEntity) не останавливает разбор prog.Entities целиком -
// см. package doc collectEntities про bag/poison-маркеры.
func TestCollectEntitiesCollectsIndependentErrors(t *testing.T) {
	src := `
сущность Первая
{
    имеет:
        поле: Целое
        поле: Строка
}

абстрактная сущность Вторая
{
    метод_один(): Целое
}

сущность Третья наследует Вторую
{
}

функция главная
{
    вывести "привет"
}
`
	_, err := AnalyzeSource(t, src)
	msgs := diagMessages(t, err)
	if len(msgs) != 2 {
		t.Fatalf("ожидалось 2 независимые диагностики, получено %d: %v", len(msgs), msgs)
	}
	if !containsSubstring(msgs, `поле "поле" объявлено более одного раза`) {
		t.Errorf("не найдена ошибка дублирующегося поля сущности \"Первая\": %v", msgs)
	}
	if !containsSubstring(msgs, `не реализован абстрактный метод "метод_один"`) {
		t.Errorf("не найдена ошибка неабстрактной сущности \"Третья\": %v", msgs)
	}
}

// TestCollectEntitiesPoisonedEntityDoesNotCascade проверяет, что ссылка на
// СЛОМАННУЮ сущность (поле другой сущности, "наследует") не порождает
// вторую, вводящую в заблуждение диагностику - см. package doc
// entitySig.Poisoned.
func TestCollectEntitiesPoisonedEntityDoesNotCascade(t *testing.T) {
	src := `
сущность Первая
{
    имеет:
        поле: Целое
        поле: Строка
}

сущность Использует
{
    имеет:
        ссылка: Первая
}

функция главная
{
    вывести "привет"
}
`
	_, err := AnalyzeSource(t, src)
	msgs := diagMessages(t, err)
	if len(msgs) != 1 {
		t.Fatalf("ожидалась ровно 1 диагностика (без каскада от \"Использует\"), получено %d: %v", len(msgs), msgs)
	}
	if !containsSubstring(msgs, `поле "поле" объявлено более одного раза`) {
		t.Errorf("неожиданная диагностика: %v", msgs)
	}
}

// TestCollectFuncSigsCollectsIndependentErrors проверяет, что ошибка ОДНОЙ
// свободной функции (например, коллизия перегрузок) не останавливает разбор
// остальных - см. package doc collectFuncSigs.
func TestCollectFuncSigsCollectsIndependentErrors(t *testing.T) {
	src := `
функция одна(x: Целое)
{
    вывести "привет"
}

функция одна(y: Целое)
{
    вывести "привет"
}

функция два(x: Строка)
{
    вывести "привет"
}

функция два(y: Строка)
{
    вывести "привет"
}

функция главная
{
    вывести "привет"
}
`
	_, err := AnalyzeSource(t, src)
	msgs := diagMessages(t, err)
	if len(msgs) != 2 {
		t.Fatalf("ожидалось 2 независимые диагностики, получено %d: %v", len(msgs), msgs)
	}
	if !containsSubstring(msgs, `функция "одна": две перегрузки`) {
		t.Errorf("не найдена ошибка перегрузки \"одна\": %v", msgs)
	}
	if !containsSubstring(msgs, `функция "два": две перегрузки`) {
		t.Errorf("не найдена ошибка перегрузки \"два\": %v", msgs)
	}
}
