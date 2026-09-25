package morph

import (
	"strings"
	"unicode"
)

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
	// "стая"/"свая" - редкие настоящие существительные на голое "-ая", не
	// субстантивированные прилагательные (в отличие от подавляющего
	// большинства слов на "-ая"/"-яя" - "гостиная"/"передняя"/"прямая" и
	// т. п. - которые ДЕЙСТВИТЕЛЬНО склоняются как прилагательное, см.
	// ClassAdjHardFeminine/ClassAdjSoftFeminine, noun.go): без этой записи
	// ClassifyByEnding классифицировал бы их как прилагательное и вывел бы
	// неверный винительный ("стую"/"свую" вместо настоящих "стаю"/"сваю").
	// Проверено, только пока для нижнего регистра (обычные слова/
	// переменные, "переменная стая: Список<Птица>") - lookup в
	// Dictionary.RegisterNoun регистрозависимый, поэтому ту же самую пару
	// слов, использованную как ИМЯ СУЩНОСТИ ("Стая наследует ..."), эта
	// запись не спасает; такое имя сущности - осознанно принятый узкий
	// пробел (тот же класс компромисса, что и беглые гласные у
	// ClassSecondHard/ClassSecondSoftPlain выше), а не тихая ошибка -
	// практических имён типов вида "Стая"/"Свая" в программах ожидать не
	// приходится, в отличие от повсеместных прилагательных-состояний
	// ("Активная"/"Открытая"/"Завершённая"), ради которых заведён сам класс.
	"стая": {ClassSecondSoftPlain, Feminine},
	"свая": {ClassSecondSoftPlain, Feminine},
}

// nounFullExceptions - существительные с чередованием основы, не сводимым
// ни к одному generic-классу склонения (BuildNounParadigm просто отбрасывает
// суффикс именительного падежа и добавляет окончания - для этого нужна
// готовая, вручную выписанная парадигма). Проверяется в Dictionary.RegisterNoun
// раньше nounGenderExceptions и ClassifyByEnding - расширение, заранее
// предусмотренное в docs/MORPHOLOGY_DESIGN.md ("Полностью готовая парадигма без
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
	// docs/MORPHOLOGY_DESIGN.md).
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

	// "остаток" - беглая "о" (остаток/остатка, а не "остатока"), тот же
	// случай, что и "список" выше.
	"остаток": {
		Gender: Masculine,
		Paradigm: Paradigm{
			{Nominative, Singular}:    "остаток",
			{Genitive, Singular}:      "остатка",
			{Dative, Singular}:        "остатку",
			{Instrumental, Singular}:  "остатком",
			{Prepositional, Singular}: "остатке",
			{Nominative, Plural}:      "остатки",
			{Genitive, Plural}:        "остатков",
			{Dative, Plural}:          "остаткам",
			{Instrumental, Plural}:    "остатками",
			{Prepositional, Plural}:   "остатках",
		},
	},

	// "слагаемое" - субстантивированное причастие (склоняется как
	// прилагательное на "-ое", не по одному из generic-классов
	// существительного - ClassifyByEnding его поэтому не берёт).
	"слагаемое": {
		Gender: Neuter,
		Paradigm: Paradigm{
			{Nominative, Singular}:    "слагаемое",
			{Genitive, Singular}:      "слагаемого",
			{Dative, Singular}:        "слагаемому",
			{Instrumental, Singular}:  "слагаемым",
			{Prepositional, Singular}: "слагаемом",
			{Nominative, Plural}:      "слагаемые",
			{Genitive, Plural}:        "слагаемых",
			{Dative, Plural}:          "слагаемым",
			{Instrumental, Plural}:    "слагаемыми",
			{Prepositional, Plural}:   "слагаемых",
		},
	},

	// "поле" - голое "-е" (ClassifyByEnding сознательно отказывается от
	// этого окончания вообще, см. её package doc: "кофе" мужского рода -
	// того же окончания без общей закономерности), а класса склонения для
	// этого паттерна ("окно"-подобная парадигма с мягкой, не твёрдой,
	// основой) в classTable ещё нет - полная парадигма проще нового класса
	// ради одного слова. lookupGenderNumberViaGomorphy (gomorphy_source.go)
	// сама по себе даёт для "поле" НЕВЕРНЫЙ род (masc) - её "лучший" разбор
	// трактует "поле" как предложный падеж имени "Поль", не именительный
	// самого слова (найдено на examples/проекты/змейка, где поле игры
	// называлось "поле").
	"поле": {
		Gender: Neuter,
		Paradigm: Paradigm{
			{Nominative, Singular}:    "поле",
			{Genitive, Singular}:      "поля",
			{Dative, Singular}:        "полю",
			{Instrumental, Singular}:  "полем",
			{Prepositional, Singular}: "поле",
			{Nominative, Plural}:      "поля",
			{Genitive, Plural}:        "полей",
			{Dative, Plural}:          "полям",
			{Instrumental, Plural}:    "полями",
			{Prepositional, Plural}:   "полях",
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

// AnimacyOf возвращает одушевлённость nomSg: сначала курируемый список
// исключений (nounAnimacy - для случаев, где нужно явно зафиксировать
// решение), затем словарь OpenCorpora через gomorphy
// (lookupAnimacyViaGomorphy - тот же узкий источник, что и у рода в
// NounAgreement), иначе Inanimate по умолчанию - безопасное умолчание:
// большинство идентификаторов программы называют значения, а не людей/
// животных. Приводит к нижнему регистру перед поиском - тем же принципом,
// что и NounAgreement ниже - иначе имя сущности ("Пользователь", заглавная
// буква) не находило бы запись, заведённую под словарной формой
// ("пользователь").
func AnimacyOf(nomSg string) Animacy {
	lower := strings.ToLower(nomSg)
	if a, ok := nounAnimacy[lower]; ok {
		return a
	}
	if a, ok := lookupAnimacyViaGomorphy(lower); ok {
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

// hasPluralEnding - true для слов, оканчивающихся на "-ы"/"-и" - обычные
// окончания именительного падежа МНОЖЕСТВЕННОГО числа (маршрут ->
// маршрутЫ, сессия -> сессиИ, пользователь -> пользователИ - твёрдая/
// мягкая основы соответственно, см. classTable в noun.go, поле nom[1]) -
// ни одно слово Фазы A не оканчивается на них в именительном падеже
// ЕДИНСТВЕННОГО числа (см. ClassifyByEnding). Не 100% надёжно - редкие
// несклоняемые заимствования на "-и" в единственном числе (например,
// "такси") ошибочно читались бы как множественное число - осознанный
// компромисс: в практике этого языка (русские идентификаторы) такое
// случается заметно реже, чем обратная ошибка (слово, реально
// множественное, требовало "новый"/"приватный" вместо "новые"/"приватные" -
// ).
func hasPluralEnding(lower string) bool {
	return strings.HasSuffix(lower, "ы") || strings.HasSuffix(lower, "и")
}

// NounAgreement возвращает род и число, нужные для согласования слов вроде
// "новый"/"новая"/"новое"/"новые" (docs/GRAMMAR_DRAFT.md, конструктор) с
// существительным nomSg. Число - Plural для существительных из
// pluraleTantum ИЛИ с окончанием hasPluralEnding - в обоих случаях род не
// определяется (согласование в множественном числе от рода не зависит).
// ok == false - определить не удалось (слово вне Фазы A - "-й"/голое "-е"
// без исключения - и не нашлось в словаре OpenCorpora через gomorphy) -
// вызывающий код не должен в этом случае требовать согласования вовсе, а
// не угадывать форму.
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
	if e, found := lexiconWord(lower, Inanimate); found {
		return e.gender, e.number, true
	}
	if hasPluralEnding(lower) {
		return 0, Plural, true
	}
	if class, found := ClassifyByEnding(lower); found {
		return GenderOf(class), Singular, true
	}
	if gender, number, found := lookupGenderNumberViaGomorphy(lower); found {
		return gender, number, true
	}
	return 0, 0, false
}

// splitPascalWords разбивает составное имя вида "ОпцииКуки"/"ОтветЛогина"
// (CamelCase - граница по заглавной букве) или "веса_скрытого"/
// "хранилище_задач" (snake_case - граница по "_", символ подчёркивания в
// результат не входит) на слова - имя, которое остаётся одним словом
// ("Маршрутизатор"), возвращает срез из одного элемента. Оба вида границы
// проверяются в одном проходе (не два отдельных разбиения подряд) - в
// именах этого языка они не смешиваются (сущности - CamelCase, поля -
// snake_case), но ничего не мешало бы им встретиться в одном имени.
func splitPascalWords(name string) []string {
	runes := []rune(name)
	var words []string
	start := 0
	for i := 1; i < len(runes); i++ {
		if runes[i] == '_' {
			words = append(words, string(runes[start:i]))
			start = i + 1
			continue
		}
		if unicode.IsUpper(runes[i]) && !unicode.IsUpper(runes[i-1]) {
			words = append(words, string(runes[start:i]))
			start = i
		}
	}
	words = append(words, string(runes[start:]))
	return words
}

// CompoundNounAgreement - как NounAgreement, но для составных имён вида
// "ОпцииКуки"/"ОтветЛогина" (CamelCase - сущности) и "веса_скрытого"/
// "хранилище_задач" (snake_case - поля) пробует определить род/число
// ПЕРВОГО слова (splitPascalWords) прежде, чем всё составное имя целиком.
// В русских составных именах такого вида ("Опции[чего?] Куки", "Ответ[чего?]
// Логина", "Веса[чего?] скрытого") согласование определяет ПЕРВОЕ слово
// (главное существительное) - второе в родительном падеже, окончание
// которого сбивает классификатор при разборе составного имени как одного
// слова (например, "ответлогина" ошибочно читался как оканчивающийся на
// "-а" - женский род; "веса_скрытого" - как оканчивающийся на "-о" -
// средний род единственное число, вместо верного мужского множественного
// у "веса"). Если первое слово не распознано как существительное (например,
// оно прилагательное - "УспешныйОтвет", где "успешный" ADJF, а не NOUN) -
// откат на разбор имени целиком, тем же permissive fallback, что и раньше.
func CompoundNounAgreement(name string) (gender Gender, number Number, ok bool) {
	words := splitPascalWords(name)
	if len(words) > 1 {
		if gender, number, ok := NounAgreement(words[0]); ok {
			return gender, number, ok
		}
	}
	return NounAgreement(name)
}

// NewKeywordFor возвращает форму конструктора "новый"/"новая"/"новое"/
// "новые", согласованную с (gender, number) - общая для internal/parser
// (checkNewKeywordAgreement - проверяет фактически написанную форму) и
// internal/lsp (hover/signature help - какую форму ПОКАЗАТЬ рядом с именем
// сущности, см. package doc definition этих мест) - тот же принцип "не
// дублировать логику между потребителями", что и у остальных таблиц этого
// пакета. Masculine (в т. ч. gender не распознан вызывающей стороной вовсе -
// нулевое значение Gender) - "новый" по умолчанию, тот же permissive
// fallback, что и у CompoundNounAgreement/NounAgreement.
func NewKeywordFor(gender Gender, number Number) string {
	if number == Plural {
		return "новые"
	}
	switch gender {
	case Feminine:
		return "новая"
	case Neuter:
		return "новое"
	default:
		return "новый"
	}
}

// SystemVerbs - курируемый словарь глаголов стандартной библиотеки. Формы
// заданы вручную (см. docs/MORPHOLOGY_DESIGN.md, "Глаголы: явно заданные формы") -
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
