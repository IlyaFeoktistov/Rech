package morph

import "strings"

// nounGenderExceptions - слова, для которых ClassifyByEnding не может
// определить класс склонения однозначно (в Фазе A - в первую очередь слова
// на "-ь": возможен и мужской, и женский род). Проверяется в
// Dictionary.RegisterNoun раньше классификатора. Одушевлённость сюда не
// входит - она передаётся в RegisterNoun явно всегда, независимо от того,
// найдено слово здесь или классифицировано автоматически.
var nounGenderExceptions = map[string]struct {
	Class  DeclensionClass
	Gender Gender
}{
	"пользователь": {ClassFirstSoft, Masculine},
	"пароль":       {ClassFirstSoft, Masculine},
}

// nounFullExceptions - существительные с чередованием основы, не сводимым
// ни к одному generic-классу склонения (BuildNounParadigm просто отбрасывает
// суффикс именительного падежа и добавляет окончания - для этого нужна
// готовая, вручную выписанная парадигма). Проверяется в Dictionary.RegisterNoun
// раньше nounGenderExceptions и ClassifyByEnding - расширение, заранее
// предусмотренное в docs/MORPHOLOGY.md ("Полностью готовая парадигма без
// генератора"). Винительный падеж сюда не входит - его достраивает
// fillAccusative по одушевлённости, как и для генератора.
//
// "список" - первое слово словаря примеров, которому это понадобилось (шаг
// 4.2.b, цикл "для x в списке"): беглая "о" (список/списка, а не
// "списока") и правило написания после заднеязычной "к" (им.п. мн.ч.
// "списки", а не "спискы").
var nounFullExceptions = map[string]struct {
	Gender   Gender
	Paradigm Paradigm
}{
	"список": {
		Gender: Masculine,
		Paradigm: Paradigm{
			{Nominative, Singular}:    "список",
			{Genitive, Singular}:      "списка",
			{Dative, Singular}:        "списку",
			{Instrumental, Singular}:  "списком",
			{Prepositional, Singular}: "списке",
			{Nominative, Plural}:      "списки",
			{Genitive, Plural}:        "списков",
			{Dative, Plural}:          "спискам",
			{Instrumental, Plural}:    "списками",
			{Prepositional, Plural}:   "списках",
		},
	},

	// Десять "разносклоняемых" существительных на "-мя" (стандартный,
	// закрытый список русской грамматики - средний род, вставка "-ен-" во
	// всех косвенных падежах ед.ч. и во всей парадигме мн.ч., которую
	// ClassifyByEnding (голое "-я" -> ClassSecondSoftPlain, женский род) не
	// покрывает и не должен пытаться покрыть - это не общий случай слов на
	// "-я", а полностью нерегулярный, фиксированный набор слов; см.
	// docs/MORPHOLOGY.md).
	//
	// Множественное число дано только там, где оно реально употребимо
	// ("имя"/"время"/"знамя"/"племя"/"семя"/"стремя") - для "пламя",
	// "бремя", "вымя", "темя" оставлено только единственное число
	// (в обычном употреблении это по сути singularia tantum: "пламя"
	// неисчисляемо, "вымя"/"темя" анатомически по одному на особь) -
	// обращение к множественному числу этих четырёх слов, если такое
	// когда-нибудь понадобится программе, даст явную ошибку "форма не
	// найдена", а не угаданную (и, возможно, неверную) форму.
	"имя": {
		Gender: Neuter,
		Paradigm: Paradigm{
			{Nominative, Singular}:    "имя",
			{Genitive, Singular}:      "имени",
			{Dative, Singular}:        "имени",
			{Instrumental, Singular}:  "именем",
			{Prepositional, Singular}: "имени",
			{Nominative, Plural}:      "имена",
			{Genitive, Plural}:        "имён",
			{Dative, Plural}:          "именам",
			{Instrumental, Plural}:    "именами",
			{Prepositional, Plural}:   "именах",
		},
	},
	"время": {
		Gender: Neuter,
		Paradigm: Paradigm{
			{Nominative, Singular}:    "время",
			{Genitive, Singular}:      "времени",
			{Dative, Singular}:        "времени",
			{Instrumental, Singular}:  "временем",
			{Prepositional, Singular}: "времени",
			{Nominative, Plural}:      "времена",
			{Genitive, Plural}:        "времён",
			{Dative, Plural}:          "временам",
			{Instrumental, Plural}:    "временами",
			{Prepositional, Plural}:   "временах",
		},
	},
	"знамя": {
		Gender: Neuter,
		Paradigm: Paradigm{
			{Nominative, Singular}:    "знамя",
			{Genitive, Singular}:      "знамени",
			{Dative, Singular}:        "знамени",
			{Instrumental, Singular}:  "знаменем",
			{Prepositional, Singular}: "знамени",
			{Nominative, Plural}:      "знамёна",
			{Genitive, Plural}:        "знамён",
			{Dative, Plural}:          "знамёнам",
			{Instrumental, Plural}:    "знамёнами",
			{Prepositional, Plural}:   "знамёнах",
		},
	},
	"племя": {
		Gender: Neuter,
		Paradigm: Paradigm{
			{Nominative, Singular}:    "племя",
			{Genitive, Singular}:      "племени",
			{Dative, Singular}:        "племени",
			{Instrumental, Singular}:  "племенем",
			{Prepositional, Singular}: "племени",
			{Nominative, Plural}:      "племена",
			{Genitive, Plural}:        "племён",
			{Dative, Plural}:          "племенам",
			{Instrumental, Plural}:    "племенами",
			{Prepositional, Plural}:   "племенах",
		},
	},
	"семя": {
		Gender: Neuter,
		Paradigm: Paradigm{
			{Nominative, Singular}:    "семя",
			{Genitive, Singular}:      "семени",
			{Dative, Singular}:        "семени",
			{Instrumental, Singular}:  "семенем",
			{Prepositional, Singular}: "семени",
			{Nominative, Plural}:      "семена",
			{Genitive, Plural}:        "семян",
			{Dative, Plural}:          "семенам",
			{Instrumental, Plural}:    "семенами",
			{Prepositional, Plural}:   "семенах",
		},
	},
	"стремя": {
		Gender: Neuter,
		Paradigm: Paradigm{
			{Nominative, Singular}:    "стремя",
			{Genitive, Singular}:      "стремени",
			{Dative, Singular}:        "стремени",
			{Instrumental, Singular}:  "стременем",
			{Prepositional, Singular}: "стремени",
			{Nominative, Plural}:      "стремена",
			{Genitive, Plural}:        "стремян",
			{Dative, Plural}:          "стременам",
			{Instrumental, Plural}:    "стременами",
			{Prepositional, Plural}:   "стременах",
		},
	},
	"пламя": {
		Gender: Neuter,
		Paradigm: Paradigm{
			{Nominative, Singular}:    "пламя",
			{Genitive, Singular}:      "пламени",
			{Dative, Singular}:        "пламени",
			{Instrumental, Singular}:  "пламенем",
			{Prepositional, Singular}: "пламени",
		},
	},
	"бремя": {
		Gender: Neuter,
		Paradigm: Paradigm{
			{Nominative, Singular}:    "бремя",
			{Genitive, Singular}:      "бремени",
			{Dative, Singular}:        "бремени",
			{Instrumental, Singular}:  "бременем",
			{Prepositional, Singular}: "бремени",
		},
	},
	"вымя": {
		Gender: Neuter,
		Paradigm: Paradigm{
			{Nominative, Singular}:    "вымя",
			{Genitive, Singular}:      "вымени",
			{Dative, Singular}:        "вымени",
			{Instrumental, Singular}:  "выменем",
			{Prepositional, Singular}: "вымени",
		},
	},
	"темя": {
		Gender: Neuter,
		Paradigm: Paradigm{
			{Nominative, Singular}:    "темя",
			{Genitive, Singular}:      "темени",
			{Dative, Singular}:        "темени",
			{Instrumental, Singular}:  "теменем",
			{Prepositional, Singular}: "темени",
		},
	},
}

// nounAnimacy - курируемый словарь одушевлённости для идентификаторов
// программы и системных существительных. Одушевлённость не выводится ни из
// окончания слова, ни из словаря OpenCorpora (см. Dictionary.RegisterNoun -
// "она не следует из формы слова вообще, ни один из источников её не
// определяет") - без явного источника отсутствие слова в этой таблице
// означает "неодушевлённое" (AnimacyOf) - безопасное умолчание: большинство
// идентификаторов программы называют значения, а не людей.
var nounAnimacy = map[string]Animacy{
	"пользователь": Animate,
}

// AnimacyOf возвращает одушевлённость, курируемую для nomSg, или Inanimate,
// если слова нет в таблице (см. nounAnimacy).
func AnimacyOf(nomSg string) Animacy {
	if a, ok := nounAnimacy[nomSg]; ok {
		return a
	}
	return Inanimate
}

// pluraleTantum - курируемый список существительных, у которых нет формы
// единственного числа в том смысле, в котором слово используется в
// программе ("права" - как в "права доступа", не "право" в смысле "закон").
// Как и nounAnimacy, не выводится ни из чего - только явный список.
var pluraleTantum = map[string]bool{
	"права": true,
}

// NounAgreement возвращает род и число, нужные для согласования слов вроде
// "новый"/"новая"/"новое"/"новые" (docs/GRAMMAR.md, конструктор) с
// существительным nomSg. Число - Plural только для существительных из
// pluraleTantum - для них род не определяется (согласование в
// множественном числе от рода не зависит). ok == false - определить не
// удалось (слово вне Фазы A - "-й"/голое "-е" без исключения - и не
// нашлось в словаре OpenCorpora через gomorphy) - вызывающий код не должен
// в этом случае требовать согласования вовсе, а не угадывать форму.
//
// Источники рода - те же и в том же порядке, что и у Dictionary.RegisterNoun
// (nounFullExceptions -> nounGenderExceptions -> ClassifyByEnding ->
// gomorphy), не дублирует сами данные - для слов вроде "имя" род берётся
// прямо из nounFullExceptions (полная парадигма нужна RegisterNoun, но её
// Gender ровно тот же, что нужен и здесь), а не заводится второй раз в
// nounGenderExceptions.
func NounAgreement(nomSg string) (gender Gender, number Number, ok bool) {
	lower := strings.ToLower(nomSg)
	if pluraleTantum[lower] {
		return 0, Plural, true
	}
	if exc, found := nounFullExceptions[lower]; found {
		return exc.Gender, Singular, true
	}
	if exc, found := nounGenderExceptions[lower]; found {
		return exc.Gender, Singular, true
	}
	if class, found := ClassifyByEnding(lower); found {
		return GenderOf(class), Singular, true
	}
	if gender, found := lookupGenderViaGomorphy(lower); found {
		return gender, Singular, true
	}
	return 0, 0, false
}

// SystemVerbs - курируемый словарь глаголов стандартной библиотеки. Формы
// заданы вручную (см. docs/MORPHOLOGY.md, "Глаголы: явно заданные формы") -
// в частности "войти"/"выйти" специально показывают, что семья "-йти"
// спрягается нерегулярно (чередование основы "-й-"/"-д-") и не может быть
// выведена никаким классификатором.
var SystemVerbs = map[string]VerbForms{
	"войти": {
		PresentOrFuture3Sg: "войдёт",
		PastM:              "вошёл",
		PastF:              "вошла",
		PastN:              "вошло",
		PastPl:             "вошли",
		Gerund:             "войдя",
		PastParticiple:     "вошедший",
	},
	"выйти": {
		PresentOrFuture3Sg: "выйдет",
		PastM:              "вышел",
		PastF:              "вышла",
		PastN:              "вышло",
		PastPl:             "вышли",
		Gerund:             "выйдя",
		PastParticiple:     "вышедший",
	},
	"создать": {
		PresentOrFuture3Sg: "создаст",
		PastM:              "создал",
		PastF:              "создала",
		PastN:              "создало",
		PastPl:             "создали",
		Gerund:             "создав",
		PastParticiple:     "создавший",
	},
	"зарегистрировать": {
		PresentOrFuture3Sg: "зарегистрирует",
		PastM:              "зарегистрировал",
		PastF:              "зарегистрировала",
		PastN:              "зарегистрировало",
		PastPl:             "зарегистрировали",
		Gerund:             "зарегистрировав",
		PastParticiple:     "зарегистрировавший",
	},
	"отправить": {
		PresentOrFuture3Sg: "отправит",
		PastM:              "отправил",
		PastF:              "отправила",
		PastN:              "отправило",
		PastPl:             "отправили",
		Gerund:             "отправив",
		PastParticiple:     "отправивший",
	},
	"дать": {
		PresentOrFuture3Sg: "даст",
		PastM:              "дал",
		PastF:              "дала",
		PastN:              "дало",
		PastPl:             "дали",
		Gerund:             "дав",
		PastParticiple:     "давший",
	},
}

// RegisterSystemVerbs регистрирует весь словарь SystemVerbs в d. Вызывается
// при инициализации словаря компилятором (когда движок будет подключён к
// парсеру на шаге 3) - здесь как готовая функция, чтобы тесты и будущий
// вызывающий код не дублировали список имён.
func RegisterSystemVerbs(d *Dictionary) error {
	for infinitive, forms := range SystemVerbs {
		if err := d.RegisterVerb(infinitive, forms); err != nil {
			return err
		}
	}
	return nil
}
