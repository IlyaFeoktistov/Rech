package morph

import "slices"

// GerundOf - широкое использование gomorphy (в отличие от узкого
// lookupGenderViaGomorphy/lookupAnimacyViaGomorphy - только один грамматический
// признак): для инфинитива infinitive, зарегистрированного в OpenCorpora,
// ищет среди gomorphy.WordForms(infinitive) деепричастие (тег "GRND,...") без
// вариантной пометки в конце (V-sh - разговорная форма на "-вши"/"-ши", Fimp -
// застывшая идиоматическая форма вроде "сложа" в "сложа руки") - выбор между
// двумя "чистыми" слотами деепричастия (настоящего и прошедшего времени)
// делается по виду самого infinitive: несовершенный вид даёт деепричастие
// настоящего времени ("делая"), совершенный - прошедшего ("сложив", "создав",
// "войдя" - совпадает с уже курируемыми значениями SystemVerbs, проверено
// вручную при разработке).
//
// false - infinitive не найден в словаре OpenCorpora вообще, вид не удалось
// определить, или ни одна форма не даёт однозначного "чистого" деепричастия -
// в любом из этих случаев вызывающая сторона (парсер, тег `деепричастие:...`
// у объявления функции) требует явно заданное слово, не пытается угадать
// дальше сама.
func GerundOf(infinitive string) (string, bool) {
	if sharedGomorphy == nil {
		return "", false
	}
	wantTense, ok := gerundTenseForAspect(sharedGomorphy.Tag(infinitive))
	if !ok {
		return "", false
	}
	for _, form := range sharedGomorphy.WordForms(infinitive) {
		if tense, ok := plainGerundTense(sharedGomorphy.Tag(form)); ok && tense == wantTense {
			return form, true
		}
	}
	return "", false
}

// gerundTenseForAspect - несовершенный вид (тег содержит "impf") даёт
// деепричастие настоящего времени, совершенный ("perf") - прошедшего.
func gerundTenseForAspect(infinitiveTag string) (tense string, ok bool) {
	switch {
	case tagHasGrammeme(infinitiveTag, "impf"):
		return "pres", true
	case tagHasGrammeme(infinitiveTag, "perf"):
		return "past", true
	default:
		return "", false
	}
}

// plainGerundTense - tag имеет вид "GRND,<вид>,<переходность> <время>"
// (ровно четыре грамматических признака: часть речи, вид, переходность,
// время) - форма с дополнительной вариантной пометкой пятым признаком
// (V-sh/Fimp/...) не подходит, отклоняется здесь же, не самой GerundOf.
func plainGerundTense(tag string) (tense string, ok bool) {
	tokens := tagFields(tag)
	if len(tokens) != 4 || tokens[0] != "GRND" {
		return "", false
	}
	return tokens[3], true
}

// tagHasGrammeme - true, если want встречается среди токенов tag (см.
// tagFields, gomorphy_source.go).
func tagHasGrammeme(tag, want string) bool {
	return slices.Contains(tagFields(tag), want)
}
