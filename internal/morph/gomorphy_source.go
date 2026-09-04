package morph

import "strings"

// gomorphyAnalyzer - минимальный интерфейс, который нам нужен от gomorphy -
// не импортируем сам пакет здесь напрямую, чтобы файл легко подменить
// заглушкой в тестах и не тащить внешнюю зависимость в остальной код.
type gomorphyAnalyzer interface {
	Tag(word string) string
	WordForms(word string) []string
}

// tagFields разбивает тег gomorphy ("NOUN,inan,masc sing,nomn") на отдельные
// грамматические признаки по общим для всего пакета разделителям (","/" ") -
// общий хелпер для lookupGenderViaGomorphy/lookupAnimacyViaGomorphy (один
// конкретный признак) и GerundOf/gerund.go (нужен порядок и количество
// токенов целиком, не один конкретный).
func tagFields(tag string) []string {
	return strings.FieldsFunc(tag, func(r rune) bool { return r == ',' || r == ' ' })
}

// sharedGomorphy - разделяемый экземпляр анализатора gomorphy, найден при
// первом использовании (см. initGomorphy в gomorphy_init.go). nil, если
// библиотека не смогла загрузить встроенный словарь - в этом случае
// lookupGenderViaGomorphy просто ничего не находит, деградация мягкая.
var sharedGomorphy gomorphyAnalyzer

// lookupGenderViaGomorphy - **узкое** использование gomorphy: только род
// существительного, ничего больше. Не используется как источник парадигмы
// или падежного разбора - у публичного API gomorphy нет надёжного способа
// получить все словоформы с привязкой к падежу/числу (WordForms не
// привязывает форму к тегу и дедуплицирует омонимичные формы, Tag отдаёт
// только одну "лучшую" трактовку, теряя синкретизм) - см. docs/MORPHOLOGY.md,
// "gomorphy: узкое использование". Парадигму всё равно строит наш
// BuildNounParadigm, gomorphy здесь - только замена/расширение словаря
// исключений для слов на "-ь", где окончание не определяет род однозначно.
func lookupGenderViaGomorphy(nomSg string) (Gender, bool) {
	if sharedGomorphy == nil {
		return 0, false
	}
	tag := sharedGomorphy.Tag(nomSg)
	if tag == "" || !strings.HasPrefix(tag, "NOUN") {
		return 0, false
	}
	for _, tok := range tagFields(tag) {
		switch tok {
		case "masc":
			return Masculine, true
		case "femn":
			return Feminine, true
		case "neut":
			return Neuter, true
		}
	}
	return 0, false
}

// lookupGenderNumberViaGomorphy - как lookupGenderViaGomorphy, но вместе с
// родом достаёт из тега и число ("sing"/"plur") - нужно NounAgreement,
// чтобы распознавать слова, реально НАПИСАННЫЕ во множественном числе
// (например, "маршруты" - тег "NOUN,inan,masc plur,nomn"), а не только
// курируемый список pluraleTantum. Та же оговорка про синкретизм, что и у
// lookupGenderViaGomorphy (package doc выше) - тег несёт ОДНУ "лучшую"
// трактовку gomorphy, омонимичная форма (например, "опции" - им. падеж
// мн.ч. "опция" ИЛИ род. падеж ед.ч. той же "опция") может выбрать не ту -
// осознанная деградация, не хуже сегодняшнего полного отсутствия
// определения числа вне pluraleTantum.
func lookupGenderNumberViaGomorphy(nomSg string) (Gender, Number, bool) {
	if sharedGomorphy == nil {
		return 0, 0, false
	}
	tag := sharedGomorphy.Tag(nomSg)
	if tag == "" || !strings.HasPrefix(tag, "NOUN") {
		return 0, 0, false
	}
	var gender Gender
	foundGender := false
	plural := false
	for _, tok := range tagFields(tag) {
		switch tok {
		case "masc":
			gender, foundGender = Masculine, true
		case "femn":
			gender, foundGender = Feminine, true
		case "neut":
			gender, foundGender = Neuter, true
		case "plur":
			plural = true
		}
	}
	if plural {
		return 0, Plural, true
	}
	if !foundGender {
		return 0, 0, false
	}
	return gender, Singular, true
}

// lookupAnimacyViaGomorphy - тот же узкий источник и то же ограничение, что
// и у lookupGenderViaGomorphy (только один маркер из тега gomorphy, не
// полный разбор) - для одушевлённости (anim/inan), не рода. Даёт AnimacyOf
// работать не только для курируемого списка nounAnimacy (сейчас там всего
// одно слово), а для любого существительного, которое знает словарь
// OpenCorpora - "сотрудник"/"админ"/"гость" и т.п. без необходимости
// перечислять каждое вручную.
// tryDepluralizeMasculine пытается восстановить именительный падеж
// единственного числа существительного мужского рода твёрдого/мягкого
// склонения по его форме на "-ы"/"-и" (переменные-коллекции обычно
// называют во множественном числе - "сервисы", "элементы" - а
// BuildNounParadigm умеет строить парадигму только из формы единственного
// числа, см. package doc RegisterNoun). "-ы"/"-и" - продуктивное окончание
// множественного числа ТОЛЬКО у существительных мужского рода (женский/
// средний род на "-а"/"-я"/"-о"/"-ие"/"-ия" уже перехвачен ClassifyByEnding
// раньше в RegisterNoun) - двух кандидатов достаточно: "-ы" -> голая
// твёрдая согласная основа ("серви+сы" -> "сервис"), "-и" -> либо мягкий
// знак после основы ("пользовател+и" -> "пользователь"), либо голая
// основа (шипящая/заднеязычная согласная перед "-и" по правилу русской
// орфографии, "нож+и" -> "нож"). Кандидат принимается, только если
// gomorphy РЕАЛЬНО подтверждает его как существительное мужского рода
// единственного числа - не угадывание вслепую, а проверка по словарю
// OpenCorpora (тот же принцип узкого использования gomorphy, что и у
// lookupGenderViaGomorphy выше - не источник парадигмы, только факт
// "такое слово есть и оно такое").
func tryDepluralizeMasculine(plural string) (string, bool) {
	if sharedGomorphy == nil {
		return "", false
	}
	var candidates []string
	switch {
	case strings.HasSuffix(plural, "ы"):
		candidates = []string{strings.TrimSuffix(plural, "ы")}
	case strings.HasSuffix(plural, "и"):
		stem := strings.TrimSuffix(plural, "и")
		candidates = []string{stem + "ь", stem}
	default:
		return "", false
	}
	for _, cand := range candidates {
		if cand == "" {
			continue
		}
		tag := sharedGomorphy.Tag(cand)
		if !strings.HasPrefix(tag, "NOUN") {
			continue
		}
		isMasc, isSing := false, false
		for _, tok := range tagFields(tag) {
			switch tok {
			case "masc":
				isMasc = true
			case "sing":
				isSing = true
			}
		}
		if isMasc && isSing {
			return cand, true
		}
	}
	return "", false
}

func lookupAnimacyViaGomorphy(nomSg string) (Animacy, bool) {
	if sharedGomorphy == nil {
		return 0, false
	}
	tag := sharedGomorphy.Tag(nomSg)
	if tag == "" || !strings.HasPrefix(tag, "NOUN") {
		return 0, false
	}
	for _, tok := range tagFields(tag) {
		switch tok {
		case "anim":
			return Animate, true
		case "inan":
			return Inanimate, true
		}
	}
	return 0, false
}
