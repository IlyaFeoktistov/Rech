package morph

import (
	"fmt"
	"strings"
)

// velarConsonants - заднеязычные, после которых по правилу орфографии пишется
// "-и", а не "-ы", при твёрдой основе ("брюк+и", "очк+и").
const velarConsonants = "гкх"

// isPluraleTantumViaGomorphy - true, если словарь OpenCorpora помечает слово
// как pluralia tantum (тег "Pltm") в именительном падеже - "ножницы",
// "брюки", "сани", "часы". Тот же узкий принцип использования gomorphy, что
// и у lookupGenderViaGomorphy: только факт, не источник парадигмы.
func isPluraleTantumViaGomorphy(word string) bool {
	if sharedGomorphy == nil {
		return false
	}
	fields := tagFields(sharedGomorphy.Tag(word))
	return hasTagField(fields, "NOUN") && hasTagField(fields, "Pltm") && hasTagField(fields, "nomn")
}

// isPluraleTantumGenitiveViaGomorphy - true, если form подтверждена
// словарём как родительный падеж pluralia tantum.
func isPluraleTantumGenitiveViaGomorphy(form string) bool {
	if sharedGomorphy == nil {
		return false
	}
	fields := tagFields(sharedGomorphy.Tag(form))
	return hasTagField(fields, "Pltm") && hasTagField(fields, "gent")
}

func hasTagField(fields []string, want string) bool {
	for _, f := range fields {
		if f == want {
			return true
		}
	}
	return false
}

// BuildPluraleTantumParadigm строит парадигму существительного, у которого
// есть только множественное число, по его именительному падежу на "-ы"/"-и".
// Дательный/творительный/предложный выводятся правилом: "-ам"/"-ами"/"-ах"
// при твёрдой основе (окончание "-ы", либо "-и" после заднеязычной или
// шипящей - "ножницах", "брюках", "часах"), "-ям"/"-ями"/"-ях" при мягкой
// ("санях"). Родительный падеж правилом не выводится ("ножниц", "часов",
// "саней") - берётся первый из кандидатов (голая основа, "-ов", "-ей"),
// который словарь подтверждает как родительный падеж pluralia tantum; не
// подтвердился ни один - родительного (и винительного у одушевлённых) в
// парадигме нет. Форм единственного числа нет вовсе.
func BuildPluraleTantumParadigm(nomPl string, animacy Animacy) (Paradigm, bool) {
	var stem string
	soft := false
	switch {
	case strings.HasSuffix(nomPl, "ы"):
		stem = strings.TrimSuffix(nomPl, "ы")
	case strings.HasSuffix(nomPl, "и"):
		stem = strings.TrimSuffix(nomPl, "и")
		soft = !endsWithAny(stem, velarConsonants+hushingConsonants)
	default:
		return nil, false
	}
	if stem == "" {
		return nil, false
	}
	vowel := "а"
	if soft {
		vowel = "я"
	}
	p := Paradigm{
		{Nominative, Plural}:    nomPl,
		{Dative, Plural}:        stem + vowel + "м",
		{Instrumental, Plural}:  stem + vowel + "ми",
		{Prepositional, Plural}: stem + vowel + "х",
	}
	for _, gen := range []string{stem, stem + "ов", stem + "ей"} {
		if isPluraleTantumGenitiveViaGomorphy(gen) {
			p[Tag{Genitive, Plural}] = gen
			break
		}
	}
	if animacy == Animate {
		if gen, ok := p[Tag{Genitive, Plural}]; ok {
			p[Tag{Accusative, Plural}] = gen
		}
	} else {
		p[Tag{Accusative, Plural}] = nomPl
	}
	return p, true
}

// registerPluraleTantum регистрирует готовую парадигму pluralia tantum как
// есть - без fillAccusative (винительный уже достроен
// BuildPluraleTantumParadigm, а единственного числа, от которого fillAccusative
// строит вин.п. ед.ч., нет). Род не определён (тег "GNdr" в OpenCorpora) -
// во множественном числе согласование от него не зависит (NounAgreement).
func (d *Dictionary) registerPluraleTantum(nomPl string, animacy Animacy, paradigm Paradigm) error {
	if _, exists := d.lemmas[nomPl]; exists {
		return fmt.Errorf("лемма %q уже зарегистрирована", nomPl)
	}
	d.lemmas[nomPl] = &Lemma{
		Text:  nomPl,
		POS:   Noun,
		Noun:  &NounInfo{Class: ClassIrregular, Gender: Masculine, Animacy: animacy},
		Forms: paradigm,
	}
	for tag, form := range paradigm {
		d.addReverse(form, Analysis{Lemma: nomPl, POS: Noun, Tag: tag})
	}
	return nil
}
