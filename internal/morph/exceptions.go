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
// Не дублирует Dictionary.RegisterNoun - здесь нужен только род/число, не
// полная парадигма словоформ, поэтому источники те же (курируемые
// исключения -> ClassifyByEnding -> gomorphy), но без построения Paradigm.
func NounAgreement(nomSg string) (gender Gender, number Number, ok bool) {
	lower := strings.ToLower(nomSg)
	if pluraleTantum[lower] {
		return 0, Plural, true
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
