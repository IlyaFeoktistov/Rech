package morph

import "testing"

// TestGerundOfMatchesCuratedSystemVerbs - GerundOf, найденное автоматически
// через gomorphy, должно совпасть с уже вручную курируемыми значениями
// VerbForms.Gerund у тех же глаголов (см. exceptions.go, SystemVerbs) -
// не гарантия само по себе (совпадение проверено вручную при разработке
// именно на этих словах), но регрессионный тест на то, что автоматический
// путь не тихо разойдётся с уже проверенными значениями при обновлении
// встроенного словаря gomorphy.
func TestGerundOfMatchesCuratedSystemVerbs(t *testing.T) {
	if sharedGomorphy == nil {
		t.Skip("gomorphy не инициализировался (нет встроенного словаря) - пропускаем")
	}
	for infinitive, want := range map[string]string{
		"войти":            SystemVerbs["войти"].Gerund,
		"выйти":            SystemVerbs["выйти"].Gerund,
		"создать":          SystemVerbs["создать"].Gerund,
		"зарегистрировать": SystemVerbs["зарегистрировать"].Gerund,
		"отправить":        SystemVerbs["отправить"].Gerund,
	} {
		got, ok := GerundOf(infinitive)
		if !ok {
			t.Errorf("GerundOf(%q): не найдено", infinitive)
			continue
		}
		if got != want {
			t.Errorf("GerundOf(%q) = %q, ожидалось %q (как в SystemVerbs)", infinitive, got, want)
		}
	}
}

// TestGerundOfImperfectiveVerbs - несовершенный вид даёт деепричастие
// настоящего времени, не прошедшего (у "делать"/"читать" оба слота
// существуют в gomorphy, "чистый" прошедшего времени - редкая/устаревшая
// форма - см. package doc GerundOf).
func TestGerundOfImperfectiveVerbs(t *testing.T) {
	if sharedGomorphy == nil {
		t.Skip("gomorphy не инициализировался (нет встроенного словаря) - пропускаем")
	}
	for infinitive, want := range map[string]string{
		"делать":   "делая",
		"читать":   "читая",
		"говорить": "говоря",
	} {
		got, ok := GerundOf(infinitive)
		if !ok || got != want {
			t.Errorf("GerundOf(%q) = (%q, %v), ожидалось (%q, true)", infinitive, got, ok, want)
		}
	}
}

// TestGerundOfUnknownWordFails - слово, которого нет в словаре OpenCorpora
// (или не глагол вовсе), не должно давать угаданный результат.
func TestGerundOfUnknownWordFails(t *testing.T) {
	if sharedGomorphy == nil {
		t.Skip("gomorphy не инициализировался (нет встроенного словаря) - пропускаем")
	}
	if _, ok := GerundOf("аллилуйяскокотурма"); ok {
		t.Fatal("ожидалось, что несуществующее слово не даст деепричастие")
	}
}
