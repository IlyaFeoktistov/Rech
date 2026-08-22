package morph

// Закрытый список падежных вопросительных слов, которыми называются
// параметры функций (docs/GRAMMAR.md, "Естественный вызов и падежное
// сопоставление"). Это одновременно и допустимое имя параметра, и его
// падежная роль - других имён параметров в шаге 3 нет.
const (
	ParamWho    = "кто"  // именительный
	ParamWhom   = "кого" // винительный, одушевлённый
	ParamToWhom = "кому" // дательный
	ParamByWhom = "кем"  // творительный
	ParamWhere  = "где"  // предложный
	ParamWhat   = "что"  // винительный, неодушевлённый
)

// paramCases связывает каждое падежное имя параметра с его падежом.
// ParamWhom и ParamWhat оба дают Accusative - это одна и та же
// поверхностная форма винительного падежа, они различаются только
// одушевлённостью существительного-аргумента (см. Dictionary.Analyze и
// NounInfo.Animacy), а не самим падежом.
var paramCases = map[string]Case{
	ParamWho:    Nominative,
	ParamWhom:   Accusative,
	ParamToWhom: Dative,
	ParamByWhom: Instrumental,
	ParamWhere:  Prepositional,
	ParamWhat:   Accusative,
}

// ParamNames - все допустимые падежные имена параметров в порядке
// объявления констант выше. Нужен вызывающему коду (парсеру) для текста
// сообщения об ошибке - чтобы список не дублировался как отдельный
// литерал ещё и там.
var ParamNames = []string{ParamWho, ParamWhom, ParamToWhom, ParamByWhom, ParamWhere, ParamWhat}

// CaseOfParamName возвращает падеж, закреплённый за падежным именем
// параметра, если name входит в закрытый список (см. константы Param*).
func CaseOfParamName(name string) (Case, bool) {
	c, ok := paramCases[name]
	return c, ok
}

// IsAnimateParamName - true для ParamWhom (винительный, одушевлённый), false
// для ParamWhat (винительный, неодушевлённый). Вызывающая сторона должна
// сама убедиться, что name - один из этих двух (через CaseOfParamName),
// иначе результат не имеет смысла.
func IsAnimateParamName(name string) bool {
	return name == ParamWhom
}
