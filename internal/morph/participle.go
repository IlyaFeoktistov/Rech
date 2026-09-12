package morph

import (
	"fmt"
	"strings"
)

// participleEndings - окончания полной формы страдательного причастия по
// последней букве краткой формы (шаг 4.1, упрощённая версия - полное
// склонение прилагательных отложено на Фазу B/шаг 6.1, см.
// docs/MORPHOLOGY_DESIGN.md, "Фазы реализации" - здесь курируемое правило для
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

// ActiveParticipleAgreement выводит форму действительного причастия
// прошедшего времени (шаг 6.4 - причастие глагола как второй, наравне с
// LongFormAdjective выше, источник слова для "состояние:" - "вошедший" из
// глагола "войти", а не только краткая форма страдательного причастия вроде
// "авторизован"), согласуя masculine (форма муж. рода им. п. ед. ч. -
// curated в VerbForms.PastParticiple, см. SystemVerbs) по gender/number.
//
// В отличие от LongFormAdjective, здесь только один курируемый класс
// окончания: действительное причастие прошедшего времени всегда
// оканчивается на "-ший" независимо от глагола ("вошедший", "вышедший",
// "создавший" - гласная/согласная перед "ший" зависит от основы глагола, но
// сам суффикс "-ший" - нет), поэтому одно универсальное правило покрывает
// любой глагол, а не требует словарного различения по нему.
func ActiveParticipleAgreement(masculine string, gender Gender, number Number) (string, error) {
	if !strings.HasSuffix(masculine, "ший") {
		return "", fmt.Errorf("%q: действительное причастие прошедшего времени должно оканчиваться на \"-ший\"", masculine)
	}
	stem := strings.TrimSuffix(masculine, "ий")
	if number == Plural {
		return stem + "ие", nil
	}
	switch gender {
	case Masculine:
		return masculine, nil
	case Feminine:
		return stem + "ая", nil
	case Neuter:
		return stem + "ее", nil
	default:
		return "", fmt.Errorf("неизвестный род %v", gender)
	}
}
