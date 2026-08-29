// Творительный падеж полной формы прилагательного/причастия - нужен
// "получатель становится <прилагательное>" (полная форма согласования по
// роду/числу И падежу - "стать X" в русском управляет творительным
// падежом, "пользователь становится авторизованным", "система становится
// авторизованной"). В отличие от LongFormAdjective/ActiveParticipleAgreement
// (именительный падеж - нужен для производного ИМЕНИ ТИПА состояния,
// "АвторизованныйПользователь") - это отдельная, самостоятельная форма
// того же слова, не переиспользующая именительную.
//
// Средний род сливается с мужским в творительном падеже прилагательных
// ("авторизованным" - и для "устройство", и для "пользователь") - это не
// упрощение движка, а факт русской грамматики (родовое различие сохраняется
// только в именительном/винительном), поэтому у обеих таблиц ниже нет
// отдельного поля Neuter вообще, только Masculine (используется для обоих
// родов) - лишнее поле-дубликат было бы избыточным состоянием, которое
// пришлось бы поддерживать синхронизированным вручную.
package morph

import (
	"fmt"
	"strings"
)

// instrumentalParticipleEndings - окончания творительного падежа полной
// формы страдательного причастия по последней букве краткой формы - тот же
// принцип и те же два курируемых класса, что и participleEndings
// (participle.go, LongFormAdjective).
var instrumentalParticipleEndings = map[rune]struct {
	Masculine, Feminine, Plural string
}{
	'н': {Masculine: "ным", Feminine: "ной", Plural: "ными"},
	'т': {Masculine: "ым", Feminine: "ой", Plural: "ыми"},
}

// InstrumentalLongFormAdjective - творительный падеж полной формы
// страдательного причастия, выведенной из краткой short (тот же курируемый
// охват, что и у LongFormAdjective - только "-н"/"-т", любое другое
// окончание - явная ошибка, не угаданная форма).
func InstrumentalLongFormAdjective(short string, gender Gender, number Number) (string, error) {
	runes := []rune(short)
	if len(runes) == 0 {
		return "", fmt.Errorf("пустое имя состояния")
	}
	endings, ok := instrumentalParticipleEndings[runes[len(runes)-1]]
	if !ok {
		return "", fmt.Errorf("%q: краткая форма состояния должна оканчиваться на \"н\" или \"т\"", short)
	}
	if number == Plural {
		return short + endings.Plural, nil
	}
	switch gender {
	case Masculine, Neuter:
		return short + endings.Masculine, nil
	case Feminine:
		return short + endings.Feminine, nil
	default:
		return "", fmt.Errorf("неизвестный род %v", gender)
	}
}

// InstrumentalActiveParticiple - творительный падеж действительного
// причастия прошедшего времени (masculine - форма муж. рода им. п. ед. ч.,
// curated в VerbForms.PastParticiple, см. SystemVerbs) - тот же курируемый
// охват, что и у ActiveParticipleAgreement (только окончание "-ший").
func InstrumentalActiveParticiple(masculine string, gender Gender, number Number) (string, error) {
	if !strings.HasSuffix(masculine, "ший") {
		return "", fmt.Errorf("%q: действительное причастие прошедшего времени должно оканчиваться на \"-ший\"", masculine)
	}
	stem := strings.TrimSuffix(masculine, "ий")
	if number == Plural {
		return stem + "ими", nil
	}
	switch gender {
	case Masculine, Neuter:
		return stem + "им", nil
	case Feminine:
		return stem + "ей", nil
	default:
		return "", fmt.Errorf("неизвестный род %v", gender)
	}
}
