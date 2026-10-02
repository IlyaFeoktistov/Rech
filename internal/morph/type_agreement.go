package morph

import (
	"strings"
	"unicode"
)

// Модификатор перед "Тип имя" (видимость, "статический") согласуется с
// ТИПОМ, как прилагательное с главным словом: "публичная Строка имя"
// ("широкая река Волга"), тем же правилом, что "новая Точка(...)".

// TypeAgreementWord - слово типа, с которым согласуется модификатор:
// внешний тип без "?"/"*", generic-аргументов, сигнатуры и ширины
// ("?Строка" -> "Строка", "Список<Строка>" -> "Список", "Целое32" ->
// "Целое"). "" - имя не кириллическое (параметр типа "T" и т.п.), рода нет.
func TypeAgreementWord(typeName string) string {
	w := strings.TrimLeft(typeName, "?*")
	if i := strings.IndexAny(w, "<( "); i >= 0 {
		w = w[:i]
	}
	w = strings.TrimRightFunc(w, unicode.IsDigit)
	for _, r := range w {
		if unicode.Is(unicode.Cyrillic, r) {
			return w
		}
	}
	return ""
}

// TypeAgreement - род/число типа typeName; ok == false - не определить,
// подходит любая форма модификатора.
func TypeAgreement(typeName string) (Gender, Number, bool) {
	w := TypeAgreementWord(typeName)
	if w == "" {
		return 0, 0, false
	}
	return CompoundNounAgreement(w)
}

// HardAdjectiveForm - форма прилагательного твёрдого склонения по основе
// ("публичн" -> "публичный"/"публичная"/"публичное"/"публичные").
func HardAdjectiveForm(stem string, gender Gender, number Number) string {
	if number == Plural {
		return stem + "ые"
	}
	switch gender {
	case Feminine:
		return stem + "ая"
	case Neuter:
		return stem + "ое"
	default:
		return stem + "ый"
	}
}
