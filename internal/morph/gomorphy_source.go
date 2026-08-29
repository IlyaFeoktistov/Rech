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

// lookupAnimacyViaGomorphy - тот же узкий источник и то же ограничение, что
// и у lookupGenderViaGomorphy (только один маркер из тега gomorphy, не
// полный разбор) - для одушевлённости (anim/inan), не рода. Даёт AnimacyOf
// работать не только для курируемого списка nounAnimacy (сейчас там всего
// одно слово), а для любого существительного, которое знает словарь
// OpenCorpora - "сотрудник"/"админ"/"гость" и т.п. без необходимости
// перечислять каждое вручную.
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
