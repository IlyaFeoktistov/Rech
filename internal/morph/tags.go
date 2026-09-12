// Package morph - морфологический движок Rech (шаг 1.5 в
// дизайн - docs/MORPHOLOGY_DESIGN.md). Работает только со словами из конечного,
// известного на этапе компиляции словаря: идентификаторами, объявленными в
// программе, и курируемым системным словарём (exceptions.go). Это не общий
// морфологический анализатор русского языка - анализ ищет совпадение среди
// словоформ уже зарегистрированных лемм, а не разбирает произвольное слово
// с нуля.
package morph

// Case - падеж существительного/прилагательного.
type Case int

const (
	Nominative    Case = iota // именительный
	Genitive                  // родительный
	Dative                    // дательный
	Accusative                // винительный
	Instrumental              // творительный
	Prepositional             // предложный
)

func (c Case) String() string {
	switch c {
	case Nominative:
		return "именительный"
	case Genitive:
		return "родительный"
	case Dative:
		return "дательный"
	case Accusative:
		return "винительный"
	case Instrumental:
		return "творительный"
	case Prepositional:
		return "предложный"
	default:
		return "неизвестный падеж"
	}
}

// Number - число словоформы.
type Number int

const (
	Singular Number = iota
	Plural
)

func (n Number) String() string {
	if n == Plural {
		return "множественное"
	}
	return "единственное"
}

// Gender - грамматический род существительного.
type Gender int

const (
	Masculine Gender = iota
	Feminine
	Neuter
)

func (g Gender) String() string {
	switch g {
	case Masculine:
		return "мужской"
	case Feminine:
		return "женский"
	case Neuter:
		return "средний"
	default:
		return "неизвестный род"
	}
}

// Animacy - одушевлённость. Отдельно от Gender, потому что именно она (а не
// род) решает, совпадает винительный падеж с родительным или с именительным.
type Animacy int

const (
	Inanimate Animacy = iota
	Animate
)

// PartOfSpeech - часть речи леммы.
type PartOfSpeech int

const (
	Noun PartOfSpeech = iota
	Adjective
	Verb
)

// Tag - грамматические признаки одной словоформы существительного или
// прилагательного (падеж x число).
type Tag struct {
	Case   Case
	Number Number
}

// Paradigm - все словоформы леммы, посчитанные один раз при регистрации в
// Dictionary. Ключ - Tag, значение - словоформа.
type Paradigm map[Tag]string
