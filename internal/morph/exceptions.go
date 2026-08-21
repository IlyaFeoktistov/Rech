package morph

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
