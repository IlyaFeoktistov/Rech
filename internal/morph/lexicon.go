package morph

import (
	"slices"
	"strings"
	"unicode"
	"unicode/utf8"

	"github.com/IlyaFeoktistov/Rech/internal/thirdparty/gomorphy"
)

// Парадигма имени по словарю OpenCorpora (gomorphy, internal/thirdparty):
// для любого слова, которое словарь знает как существительное,
// местоимение-существительное или прилагательное в роли существительного
// ("животные", "все", "выбранное"), берутся его настоящие формы, а не
// выведенные правилами по окончанию (noun.go) - так склоняются слова с
// беглой гласной ("питомец" - "питомцы"), супплетивные ("человек" -
// "люди"), pluralia tantum ("ножницы"), разносклоняемые ("имя", "дочь").
// Составное имя ("счётчик_слов", "ОтветЛогина", "главное_окно")
// склоняется по главному существительному, стоящие перед ним
// прилагательные согласуются с ним, остальные слова не меняются. Слово,
// которого нет в словаре, остаётся правилам (RegisterNoun).

// lexiconEntry - результат разбора имени по словарю.
type lexiconEntry struct {
	paradigm Paradigm
	// variants - все формы каждого тега, в том числе вторые ("дочерьми" и
	// "дочерями", "в лесе"/"в лесу"); первая совпадает с paradigm[tag].
	variants map[Tag][]string
	gender   Gender
	number   Number
	animacy  Animacy
}

// lexiconParses - разборы слова словарём; nil без словаря (тесты с
// заглушкой, неудачная загрузка).
var lexiconParses = func(word string) []gomorphy.Parse {
	if sharedLexicon == nil {
		return nil
	}
	return sharedLexicon.Parses(word)
}

// sharedLexicon - тот же анализатор, что и sharedGomorphy (gomorphy_init.go).
var sharedLexicon *gomorphy.Analyzer

// tagCase - падеж из тега OpenCorpora; false - звательный или нет падежа.
var tagCase = map[string]Case{
	"nomn": Nominative, "gent": Genitive, "gen1": Genitive, "gen2": Genitive,
	"datv": Dative, "accs": Accusative, "acc2": Accusative, "ablt": Instrumental,
	"loct": Prepositional, "loc1": Prepositional, "loc2": Prepositional,
}

// Вторые падежи ("gen2"/"loc2"/"acc2") - дополнительные варианты формы,
// основная - у обычного падежа.
var secondaryCases = map[string]bool{"gen2": true, "loc2": true, "acc2": true}

var tagGender = map[string]Gender{"masc": Masculine, "femn": Feminine, "neut": Neuter}

// parsedTag - признаки тега OpenCorpora.
type parsedTag struct {
	pos       string
	cas       Case
	hasCase   bool
	secondary bool
	number    Number
	hasNumber bool
	gender    Gender
	hasGender bool
	anim      int // 0 - не указана, 1 - anim, -1 - inan
	pltm      bool
	fields    []string
}

func parseTag(tag string) parsedTag {
	fields := tagFields(tag)
	t := parsedTag{fields: fields}
	if len(fields) > 0 {
		t.pos = fields[0]
	}
	for _, f := range fields {
		if c, ok := tagCase[f]; ok {
			t.cas, t.hasCase, t.secondary = c, true, secondaryCases[f]
		}
		switch f {
		case "sing":
			t.number, t.hasNumber = Singular, true
		case "plur":
			t.number, t.hasNumber = Plural, true
		case "anim":
			t.anim = 1
		case "inan":
			t.anim = -1
		case "Pltm":
			t.pltm = true
		}
		if g, ok := tagGender[f]; ok {
			t.gender, t.hasGender = g, true
		}
	}
	return t
}

// nounLikePOS - части речи, которые могут быть именем переменной/сущности:
// существительное, местоимение-существительное, прилагательное и причастие
// в роли существительного.
var nounLikePOS = map[string]int{"NOUN": 1, "NPRO": 2, "ADJF": 3, "PRTF": 4}

// adjectivePOS - определение в составном имени: прилагательное или причастие.
var adjectivePOS = map[string]int{"ADJF": 1, "PRTF": 2}

// lexiconLookup - парадигма имени name по словарю (см. package doc);
// animacy - одушевлённость, если её нужно навязать прилагательному (у
// существительного её знает сам словарь).
func lexiconLookup(name string, animacy Animacy) (lexiconEntry, bool) {
	words := splitPascalWords(name)
	if len(words) == 1 {
		return lexiconWord(name, animacy)
	}
	return lexiconCompound(name, words, animacy)
}

// lexiconWord - одно слово в начальной форме (им. падеж, ед. ч., или мн.
// ч. у слова во множественном числе).
func lexiconWord(word string, animacy Animacy) (lexiconEntry, bool) {
	parse, tag, ok := bestNominativeParse(word)
	if !ok {
		return lexiconEntry{}, false
	}
	e := lexiconEntry{paradigm: Paradigm{}, variants: map[Tag][]string{}, gender: tag.gender, number: tag.number, animacy: animacy}
	if tag.pos == "NOUN" || tag.pos == "NPRO" {
		switch tag.anim {
		case 1:
			e.animacy = Animate
		case -1:
			e.animacy = Inanimate
		}
		if tag.pltm || (tag.pos == "NPRO" && tag.number == Plural) {
			e.number = Plural
		}
	}
	for _, f := range parse.Lexeme {
		ft := parseTag(f.Tag)
		if !ft.hasCase || !ft.hasNumber {
			continue
		}
		if (tag.pos == "ADJF" || tag.pos == "PRTF") && !adjFormAgrees(ft, tag, e.animacy) {
			continue
		}
		e.addForm(Tag{Case: ft.cas, Number: ft.number}, matchCase(word, f.Word), ft.secondary)
	}
	if _, ok := e.paradigm[Tag{Case: Nominative, Number: e.number}]; !ok {
		return lexiconEntry{}, false
	}
	return e, true
}

// adjFormAgrees - форма прилагательного-имени ft из той же лексемы
// согласована с его начальной формой nom: то же число, во ед. ч. - тот же
// род; у винительного, где одушевлённость различает форму, - нужная.
func adjFormAgrees(ft, nom parsedTag, animacy Animacy) bool {
	if ft.number != nom.number {
		return false
	}
	if ft.number == Singular && ft.hasGender && nom.hasGender && ft.gender != nom.gender {
		return false
	}
	if ft.cas == Accusative && ft.anim != 0 {
		return (ft.anim == 1) == (animacy == Animate)
	}
	return true
}

// bestNominativeParse - разбор word, в котором word - именительный падеж
// части речи из nounLikePOS; у существительного - предпочтительнее всего.
func bestNominativeParse(word string) (gomorphy.Parse, parsedTag, bool) {
	return bestNominativeParseOf(word, nounLikePOS)
}

// bestNominativeParseOf - то же, среди частей речи ranks (ключ - часть
// речи, значение - приоритет, меньше - лучше).
func bestNominativeParseOf(word string, ranks map[string]int) (gomorphy.Parse, parsedTag, bool) {
	if p, t, ok := bestNominativeParseExact(word, ranks); ok {
		return p, t, true
	}
	// "е" вместо "ё" ("стекла" - "стёкла"): словарь хранит "ё".
	for _, v := range yoVariants(word) {
		if p, t, ok := bestNominativeParseExact(v, ranks); ok {
			return p, t, true
		}
	}
	return gomorphy.Parse{}, parsedTag{}, false
}

// yoVariants - word с одной из букв "е" (по очереди), заменённой на "ё".
func yoVariants(word string) []string {
	runes := []rune(word)
	var out []string
	for i, r := range runes {
		if r == 'е' || r == 'Е' {
			v := append([]rune{}, runes...)
			v[i] = map[rune]rune{'е': 'ё', 'Е': 'Ё'}[r]
			out = append(out, string(v))
		}
	}
	return out
}

func bestNominativeParseExact(word string, ranks map[string]int) (gomorphy.Parse, parsedTag, bool) {
	// Однобуквенное имя ("п", "А") - не слово, а метка: словарь знает такие
	// только как аббревиатуры/инициалы.
	if utf8.RuneCountInString(word) < 2 {
		return gomorphy.Parse{}, parsedTag{}, false
	}
	var best gomorphy.Parse
	var bestTag parsedTag
	bestRank := 0
	for _, p := range lexiconParses(word) {
		t := parseTag(p.Tag)
		rank, ok := ranks[t.pos]
		if !ok || !t.hasCase || t.cas != Nominative || t.secondary {
			continue
		}
		if slices.Contains(t.fields, "Abbr") || slices.Contains(t.fields, "Init") {
			continue
		}
		if slices.Contains(t.fields, "Surn") || slices.Contains(t.fields, "Patr") {
			rank += 10
		}
		if bestRank == 0 || rank < bestRank {
			best, bestTag, bestRank = p, t, rank
		}
	}
	return best, bestTag, bestRank != 0
}

// hasNominativeParse - у word есть разбор именительным падежом одной из
// частей речи pos.
func hasNominativeParse(word string, pos ...string) bool {
	if utf8.RuneCountInString(word) < 2 {
		return false
	}
	for _, p := range lexiconParses(word) {
		t := parseTag(p.Tag)
		if t.hasCase && t.cas == Nominative && !t.secondary && slices.Contains(pos, t.pos) {
			return true
		}
	}
	return false
}

func (e *lexiconEntry) addForm(t Tag, form string, secondary bool) {
	if slices.Contains(e.variants[t], form) {
		return
	}
	if _, ok := e.paradigm[t]; !ok && !secondary {
		e.paradigm[t] = form
		e.variants[t] = append([]string{form}, e.variants[t]...)
		return
	}
	e.variants[t] = append(e.variants[t], form)
}

// lexiconCompound - составное имя: склоняется первое существительное
// (главное слово), прилагательные перед ним согласуются с ним, слова после
// него (зависимые, "слов" в "счётчик_слов") не меняются. Разделители
// ("_" и границы CamelCase) сохраняются.
func lexiconCompound(name string, words []string, animacy Animacy) (lexiconEntry, bool) {
	// Главное слово - первое существительное, перед которым только
	// определения: слово, которое может быть и прилагательным, и
	// существительным ("главные"), считается определением, если за ним
	// идёт существительное.
	head := -1
	for i, w := range words {
		if !hasNominativeParse(w, "NOUN") {
			continue
		}
		if hasNominativeParse(w, "ADJF", "PRTF") && i+1 < len(words) && hasNominativeParse(words[i+1], "NOUN") {
			continue
		}
		head = i
		break
	}
	if head < 0 {
		return lexiconEntry{}, false
	}
	headEntry, ok := lexiconWord(words[head], animacy)
	if !ok {
		return lexiconEntry{}, false
	}
	parts := make([]lexiconEntry, head)
	for i := 0; i < head; i++ {
		adj, ok := lexiconAgreeingAdjective(words[i], headEntry)
		if !ok {
			return lexiconEntry{}, false
		}
		parts[i] = adj
	}
	seps := compoundSeparators(name, words)
	e := lexiconEntry{paradigm: Paradigm{}, variants: map[Tag][]string{}, gender: headEntry.gender, number: headEntry.number, animacy: headEntry.animacy}
	for t, forms := range headEntry.variants {
		for vi, form := range forms {
			var b strings.Builder
			for i, w := range words {
				if i > 0 {
					b.WriteString(seps[i-1])
				}
				switch {
				case i < head:
					b.WriteString(parts[i].paradigm[t])
				case i == head:
					b.WriteString(form)
				default:
					b.WriteString(w)
				}
			}
			e.addForm(t, b.String(), vi > 0)
		}
	}
	return e, true
}

// lexiconAgreeingAdjective - формы прилагательного word, согласованные по
// роду/числу/одушевлённости с главным существительным head.
func lexiconAgreeingAdjective(word string, head lexiconEntry) (lexiconEntry, bool) {
	parse, _, ok := bestNominativeParseOf(word, adjectivePOS)
	if !ok {
		return lexiconEntry{}, false
	}
	e := lexiconEntry{paradigm: Paradigm{}, variants: map[Tag][]string{}}
	for _, f := range parse.Lexeme {
		ft := parseTag(f.Tag)
		if !ft.hasCase || !ft.hasNumber || slices.Contains(ft.fields, "Supr") || slices.Contains(ft.fields, "Cmp2") {
			continue
		}
		if ft.number == Singular && ft.hasGender && ft.gender != head.gender {
			continue
		}
		if ft.cas == Accusative && ft.anim != 0 && (ft.anim == 1) != (head.animacy == Animate) {
			continue
		}
		e.addForm(Tag{Case: ft.cas, Number: ft.number}, matchCase(word, f.Word), ft.secondary)
	}
	return e, len(e.paradigm) > 0
}

// compoundSeparators - что стоит между словами имени ("_" или ничего у
// CamelCase), по порядку.
func compoundSeparators(name string, words []string) []string {
	var seps []string
	rest := name
	for i, w := range words {
		idx := strings.Index(rest, w)
		if idx < 0 {
			return make([]string, len(words)-1)
		}
		if i > 0 {
			seps = append(seps, rest[:idx])
		}
		rest = rest[idx+len(w):]
	}
	return seps
}

// matchCase - form с тем же регистром первой буквы, что у original
// ("Точка" -> "Точками"): словарь хранит слова строчными.
func matchCase(original, form string) string {
	r, _ := utf8.DecodeRuneInString(original)
	if !unicode.IsUpper(r) {
		return form
	}
	f, size := utf8.DecodeRuneInString(form)
	return string(unicode.ToUpper(f)) + form[size:]
}
