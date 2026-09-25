package morph

import (
	"slices"
	"strings"
)

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

// lookupGenderViaGomorphy - род слова на "-ь" для правил по окончанию
// (RegisterNoun): до них слово уже искалось в словаре целиком
// (lexicon.go) и не нашлось как имя - здесь только его род.
func lookupGenderViaGomorphy(nomSg string) (Gender, bool) {
	if sharedGomorphy == nil {
		return 0, false
	}
	tag := sharedGomorphy.Tag(nomSg)
	if tag == "" || !strings.HasPrefix(tag, "NOUN") {
		return 0, false
	}
	tokens := tagFields(tag)
	if !slices.Contains(tokens, "nomn") && !slices.Contains(tokens, "plur") {
		// nomSg по определению вызывающей стороны - именительный падеж
		// (см. package doc выше и RegisterNoun/NounAgreement, которые сюда
		// зовут). Если "лучший" разбор gomorphy - НЕ именительный падеж
		// (и не форма множественного числа, у которой тоже свой "nomn" -
		// "маршруты" тегируется "NOUN,...,plur,nomn" - см. lookupGenderNumberViaGomorphy),
		// значит совпадение по строке нашлось у СЛОВОФОРМЫ ДРУГОЙ леммы/
		// падежа ("поле" - предложный падеж имени "Поль", тег "NOUN,anim,
		// masc,Name sing,loct" - живой найденный случай), не у именительного
		// самого запрошенного слова - доверять роду из такого разбора
		// нельзя.
		return 0, false
	}
	for _, tok := range tokens {
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
	tokens := tagFields(tag)
	if !slices.Contains(tokens, "nomn") {
		// Тот же принцип, что и в lookupGenderViaGomorphy - разбор, у
		// которого "лучший" тег НЕ именительный падеж, относится к
		// словоформе другой леммы/падежа, не к запрошенному слову.
		return 0, 0, false
	}
	var gender Gender
	foundGender := false
	plural := false
	for _, tok := range tokens {
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
// tryDepluralize пытается восстановить именительный падеж единственного
// числа существительного по его форме на "-ы"/"-и" (переменные-коллекции
// обычно называют во множественном числе - "сервисы", "задачи" - а
// BuildNounParadigm умеет строить парадигму только из формы единственного
// числа, см. package doc RegisterNoun). Кандидаты по окончанию:
//   - "-ы" -> голая твёрдая основа ("серви+сы" -> "сервис", мужской род) или
//     основа + "-а" ("комнат+ы" -> "комната", женский род);
//   - "-и" -> основа + "-ь" ("пользовател+и" -> "пользователь", мужской;
//     "тетрад+и" -> "тетрадь", женский), голая основа (шипящая/заднеязычная
//     перед "-и" по правилу орфографии, "нож+и" -> "нож"), основа + "-я"
//     ("сесси+и" -> "сессия", "недел+и" -> "неделя") или основа + "-а"
//     после шипящей/заднеязычной ("задач+и" -> "задача", "книг+и" -> "книга").
//
// Средний род сюда не попадает: его множественное число оканчивается на
// "-а"/"-я" и уже перехвачено ClassifyByEnding как единственное число
// женского рода раньше в RegisterNoun. Кандидат принимается, только если
// gomorphy РЕАЛЬНО подтверждает его как существительное единственного числа
// мужского или женского рода в именительном падеже - не угадывание вслепую, а проверка по словарю
// OpenCorpora (тот же принцип узкого использования gomorphy, что и у
// lookupGenderViaGomorphy выше - не источник парадигмы, только факт "такое
// слово есть и оно такое"). Первый подтверждённый кандидат в порядке выше
// побеждает.
func tryDepluralize(plural string) (string, Gender, bool) {
	if sharedGomorphy == nil {
		return "", 0, false
	}
	var candidates []string
	switch {
	case strings.HasSuffix(plural, "ы"):
		stem := strings.TrimSuffix(plural, "ы")
		candidates = []string{stem, stem + "а"}
	case strings.HasSuffix(plural, "и"):
		stem := strings.TrimSuffix(plural, "и")
		candidates = []string{stem + "ь", stem, stem + "я", stem + "а"}
	default:
		return "", 0, false
	}
	for _, cand := range candidates {
		if cand == "" || cand == "а" || cand == "я" || cand == "ь" {
			continue
		}
		tag := sharedGomorphy.Tag(cand)
		if !strings.HasPrefix(tag, "NOUN") {
			continue
		}
		var gender Gender
		hasGender, isSing, isNom := false, false, false
		for _, tok := range tagFields(tag) {
			switch tok {
			case "masc":
				gender, hasGender = Masculine, true
			case "femn":
				gender, hasGender = Feminine, true
			case "sing":
				isSing = true
			case "nomn":
				isNom = true
			}
		}
		if hasGender && isSing && isNom {
			return cand, gender, true
		}
	}
	return "", 0, false
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
