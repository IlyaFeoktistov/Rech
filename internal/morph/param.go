package morph

// Закрытый список падежных вопросительных слов, которыми называются
// параметры функций (docs/GRAMMAR.md, "Естественный вызов и падежное
// сопоставление"). Это одновременно и допустимое имя параметра, и его
// падежная роль.
//
// Для трёх падежей, где различие только в одушевлённости аргумента, слова
// идут парами "одушевлённое/неодушевлённое" - как ParamWhom/ParamWhat
// исходно: ParamByWhom/ParamByWhomInan (творительный), ParamToWhom/
// ParamToWhomInan (дательный). У каждой пары один и тот же падеж
// (paramCases) - значит у одной функции не может быть обоих сразу, как и
// любых двух параметров с одинаковой падежной ролью (см. parseParams).
//
// ParamWith*/ParamFor*/ParamWhereTo/ParamWhereFrom дополнительно требуют
// ОБЯЗАТЕЛЬНОГО (не декоративного) предлога перед аргументом в вызове -
// см. MandatoryPrepositions.
const (
	ParamWho    = "кто"  // именительный
	ParamWhom   = "кого" // винительный, одушевлённый
	ParamToWhom = "кому" // дательный, одушевлённость не проверяется
	ParamByWhom = "кем"  // творительный, одушевлённость не проверяется
	ParamWhere  = "где"  // предложный
	ParamWhat   = "что"  // винительный, неодушевлённый

	ParamToWhomInan = "чему" // дательный, неодушевлённый
	ParamByWhomInan = "чем"  // творительный, неодушевлённый

	ParamWithWhom = "с_кем" // творительный, одушевлённый, обязательное "с"
	ParamWithWhat = "с_чем" // творительный, неодушевлённый, обязательное "с"

	ParamForWhom = "для_кого" // родительный, одушевлённый, обязательное "для"
	ParamForWhat = "для_чего" // родительный, неодушевлённый, обязательное "для"

	ParamWhereTo   = "куда"   // винительный, обязательное "в"
	ParamWhereFrom = "откуда" // родительный, обязательное "от"/"из"
)

// paramCases связывает каждое падежное имя параметра с его падежом.
var paramCases = map[string]Case{
	ParamWho:        Nominative,
	ParamWhom:       Accusative,
	ParamToWhom:     Dative,
	ParamByWhom:     Instrumental,
	ParamWhere:      Prepositional,
	ParamWhat:       Accusative,
	ParamToWhomInan: Dative,
	ParamByWhomInan: Instrumental,
	ParamWithWhom:   Instrumental,
	ParamWithWhat:   Instrumental,
	ParamForWhom:    Genitive,
	ParamForWhat:    Genitive,
	ParamWhereTo:    Accusative,
	ParamWhereFrom:  Genitive,
}

// ParamNames - все допустимые падежные имена параметров. Нужен вызывающему
// коду (парсеру) для текста сообщения об ошибке - чтобы список не
// дублировался как отдельный литерал ещё и там.
var ParamNames = []string{
	ParamWho, ParamWhom, ParamToWhom, ParamByWhom, ParamWhere, ParamWhat,
	ParamToWhomInan, ParamByWhomInan,
	ParamWithWhom, ParamWithWhat,
	ParamForWhom, ParamForWhat,
	ParamWhereTo, ParamWhereFrom,
}

// CaseOfParamName возвращает падеж, закреплённый за падежным именем
// параметра, если name входит в закрытый список (см. константы Param*).
func CaseOfParamName(name string) (Case, bool) {
	c, ok := paramCases[name]
	return c, ok
}

// paramAnimacyRequirement - падежные имена параметров, требующие конкретной
// одушевлённости аргумента (в противовес большинству падежных имён, где
// одушевлённость не проверяется вовсе).
var paramAnimacyRequirement = map[string]Animacy{
	ParamWhom:       Animate,
	ParamWhat:       Inanimate,
	ParamToWhomInan: Inanimate,
	ParamByWhomInan: Inanimate,
	ParamWithWhom:   Animate,
	ParamWithWhat:   Inanimate,
	ParamForWhom:    Animate,
	ParamForWhat:    Inanimate,
}

// AnimacyRequiredByParamName возвращает одушевлённость, обязательную для
// аргумента name (см. paramAnimacyRequirement), и false, если для этого
// падежного имени одушевлённость не проверяется.
func AnimacyRequiredByParamName(name string) (Animacy, bool) {
	a, ok := paramAnimacyRequirement[name]
	return a, ok
}

// paramMandatoryPreposition - падежные имена параметров, где предлог перед
// аргументом в естественном вызове не декоративный (naturalCallPrepositions
// в парсере пропускает его молча для любого падежного параметра), а
// обязательная часть роли - список допустимых предлогов (не одно слово:
// ParamWhereFrom принимает и "от", и "из").
var paramMandatoryPreposition = map[string][]string{
	ParamWithWhom:  {"с"},
	ParamWithWhat:  {"с"},
	ParamForWhom:   {"для"},
	ParamForWhat:   {"для"},
	ParamWhereTo:   {"в"},
	ParamWhereFrom: {"от", "из"},
}

// MandatoryPrepositions возвращает допустимые предлоги для name (см.
// paramMandatoryPreposition) и false, если для этого падежного имени
// предлог не обязателен (декоративен либо не нужен вовсе).
func MandatoryPrepositions(name string) ([]string, bool) {
	p, ok := paramMandatoryPreposition[name]
	return p, ok
}
