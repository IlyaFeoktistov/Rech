package morph

import "fmt"

// participleEndings - окончания полной формы страдательного причастия по
// последней букве краткой формы (шаг 4.1, упрощённая версия - полное
// склонение прилагательных отложено на Фазу B/шаг 6.1, см.
// docs/MORPHOLOGY.md, "Фазы реализации" - здесь курируемое правило для
// конкретного узкого класса слов, а не общий анализатор). Ровно два
// курируемых класса, оба закрытые и регулярные для этого класса слов:
//
//   - краткая форма на "-н" (от "-овать"/"-ать"/"-ить": авторизован,
//     подключён) - конкатенация с окончанием сама даёт двойное "н"
//     ("авторизован" + "ный" = "авторизованный").
//   - краткая форма на "-т" (от "-ыть"/"-ить" с историческим "-т":
//     закрыт, открыт, взят) - окончание без дополнительной "н".
//
// Другие классы (общие краткие прилагательные вроде "мал", "рад") этим
// правилом не покрываются - см. LongFormAdjective.
var participleEndings = map[rune]struct {
	Masculine, Feminine, Neuter, Plural string
}{
	'н': {Masculine: "ный", Feminine: "ная", Neuter: "ное", Plural: "ные"},
	'т': {Masculine: "ый", Feminine: "ая", Neuter: "ое", Plural: "ые"},
}

// LongFormAdjective выводит полную форму страдательного причастия (для
// шага 4.1 - синтез имени производного типа состояния, "авторизован" ->
// "авторизованный" при муж. роде) из краткой формы short, согласуя её по
// роду/числу gender/number (см. NounAgreement - именно оттуда обычно
// приходит gender/number сущности, для которой синтезируется состояние).
// number == Plural - множественное число (существительное pluraleTantum),
// тогда gender не используется, как и в остальном движке.
//
// Ограничение (шаг 4.1, зафиксировано как упрощение - полное правило ждёт
// склонения прилагательных, Фаза B): выводит форму только для двух
// курируемых классов кратких причастий (см. participleEndings) - short,
// оканчивающийся иначе, даёт понятную ошибку, а не угаданную форму.
func LongFormAdjective(short string, gender Gender, number Number) (string, error) {
	runes := []rune(short)
	if len(runes) == 0 {
		return "", fmt.Errorf("пустое имя состояния")
	}
	endings, ok := participleEndings[runes[len(runes)-1]]
	if !ok {
		return "", fmt.Errorf("%q: краткая форма состояния должна оканчиваться на \"н\" или \"т\"", short)
	}
	if number == Plural {
		return short + endings.Plural, nil
	}
	switch gender {
	case Masculine:
		return short + endings.Masculine, nil
	case Feminine:
		return short + endings.Feminine, nil
	case Neuter:
		return short + endings.Neuter, nil
	default:
		return "", fmt.Errorf("неизвестный род %v", gender)
	}
}
