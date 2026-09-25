package morph

import (
	"fmt"
	"maps"
	"strings"
)

// Lemma - словарная запись: слово, его часть речи и то, как оно
// склоняется/спрягается.
type Lemma struct {
	Text string
	POS  PartOfSpeech

	Noun *NounInfo // POS == Noun
	Verb *VerbInfo // POS == Verb

	// Forms - парадигма леммы (POS == Noun) - та же самая, что уже
	// посчитана при регистрации и разошлась по обратному индексу (reverse) -
	// хранится ещё и здесь, чтобы можно было спросить "какая словоформа
	// нужна для ЭТОЙ леммы в ЭТОМ падеже" (FormOf) в прямом направлении, не
	// только в обратном (Analyze). nil у глаголов.
	Forms Paradigm
}

// NounInfo - грамматические признаки существительного, зафиксированные при
// регистрации (не хранятся отдельно от Paradigm - Paradigm уже содержит все
// словоформы, NounInfo - это то, что понадобится согласованию на шаге 6.1).
type NounInfo struct {
	Class   DeclensionClass
	Gender  Gender
	Animacy Animacy
}

// Analysis - один из возможных разборов словоформы, найденной в
// Dictionary.Analyze. Для существительных заполнен Tag, для глаголов -
// VerbForm. Срез из нескольких Analysis для одной словоформы - это
// синкретизм падежей (см. docs/MORPHOLOGY_DESIGN.md) или совпадение словоформ
// разных лемм, а не ошибка - разрешать неоднозначность (по ожидаемому
// падежу параметра и т.п.) должен вызывающий код (парсер, шаг 3).
type Analysis struct {
	Lemma    string
	POS      PartOfSpeech
	Tag      Tag
	VerbForm VerbFormKind
}

// Dictionary - словарь лемм, известных компилятору: идентификаторы,
// объявленные в программе, плюс курируемый системный словарь
// (exceptions.go). Паradigma каждой леммы считается один раз при
// регистрации; Analyze - обратный поиск по уже посчитанным словоформам, а
// не морфологический анализ "на лету".
type Dictionary struct {
	lemmas  map[string]*Lemma
	reverse map[string][]Analysis
}

func NewDictionary() *Dictionary {
	return &Dictionary{
		lemmas:  map[string]*Lemma{},
		reverse: map[string][]Analysis{},
	}
}

// addReverse добавляет словоформу в обратный индекс и, если применимо, её
// вариант со свёрнутым "ё"->"е" (foldYo) - чтобы Analyze находил лемму
// независимо от того, набрал ли автор исходника "ё" или "е".
func (d *Dictionary) addReverse(form string, a Analysis) {
	d.reverse[form] = append(d.reverse[form], a)
	if folded := foldYo(form); folded != form {
		d.reverse[folded] = append(d.reverse[folded], a)
	}
}

// RegisterNoun регистрирует существительное, выводя класс склонения и род
// в следующем порядке: словарь исключений (exceptions.go) -> классификатор
// по окончанию (ClassifyByEnding) -> для слов на "-ь" (там, где окончание
// само по себе не определяет род) - справочный словарь OpenCorpora через
// gomorphy (см. gomorphy_source.go и docs/MORPHOLOGY_DESIGN.md, "gomorphy: узкое
// использование") -> для слов на "-ы"/"-и" (переменная объявлена сразу во
// множественном числе, как обычно и называют коллекции - "сервисы",
// "элементы") - попытка восстановить единственное число и проверить его
// через тот же словарь (tryDepluralize), а если словарь помечает слово как
// pluralia tantum ("ножницы", "брюки") - парадигма из одного
// множественного числа (BuildPluraleTantumParadigm). Одушевлённость всегда
// передаётся явно - она не следует из формы слова вообще, ни один из
// источников её не определяет.
func (d *Dictionary) RegisterNoun(nomSg string, animacy Animacy) error {
	if exc, ok := nounFullExceptions[nomSg]; ok {
		return d.registerNounWithParadigm(nomSg, exc.Gender, animacy, exc.Paradigm)
	}
	if e, ok := lexiconLookup(nomSg, animacy); ok {
		return d.registerLexiconNoun(nomSg, e)
	}
	if exc, ok := nounGenderExceptions[nomSg]; ok {
		return d.RegisterNounExplicit(nomSg, exc.Class, exc.Gender, animacy)
	}
	if class, ok := ClassifyByEnding(nomSg); ok {
		return d.RegisterNounExplicit(nomSg, class, GenderOf(class), animacy)
	}
	if strings.HasSuffix(nomSg, "ь") {
		if isAgentNoun(nomSg) {
			return d.RegisterNounExplicit(nomSg, ClassFirstSoft, Masculine, animacy)
		}
		if gender, ok := lookupGenderViaGomorphy(nomSg); ok {
			if gender == Masculine {
				return d.RegisterNounExplicit(nomSg, ClassFirstSoft, Masculine, animacy)
			}
			return d.RegisterNounExplicit(nomSg, ClassThirdSoft, Feminine, animacy)
		}
	}
	if hasPluralEnding(nomSg) && isPluraleTantumViaGomorphy(nomSg) {
		if paradigm, ok := BuildPluraleTantumParadigm(nomSg, animacy); ok {
			return d.registerPluraleTantum(nomSg, animacy, paradigm)
		}
	}
	if hasPluralEnding(nomSg) {
		if singular, gender, ok := tryDepluralize(nomSg); ok {
			// ClassifyByEnding сама отказывается от "-ь" (окончание само по
			// себе не определяет род/класс) - tryDepluralize уже подтвердила
			// род gomorphy'ем, поэтому класс "-ь" выбирается по нему, тем же
			// принципом, что и в ветке "-ь" самого nomSg выше.
			if strings.HasSuffix(singular, "ь") {
				class := ClassFirstSoft
				if gender == Feminine {
					class = ClassThirdSoft
				}
				return d.registerNounFromSingular(nomSg, singular, class, gender, animacy)
			}
			if class, ok := ClassifyByEnding(singular); ok {
				return d.registerNounFromSingular(nomSg, singular, class, GenderOf(class), animacy)
			}
		}
		if singular, ok := guessLoanwordSingular(nomSg); ok {
			if class, ok := ClassifyByEnding(singular); ok {
				return d.registerNounFromSingular(nomSg, singular, class, GenderOf(class), animacy)
			}
		}
	}
	return fmt.Errorf("не удалось автоматически определить склонение %q (слова нет в словаре OpenCorpora, окончание неоднозначно) - зарегистрируйте через RegisterNounExplicit или добавьте в словарь исключений", nomSg)
}

// RegisterNounExplicit регистрирует существительное с явно заданным
// классом склонения и родом - минуя классификатор. Нужен для слов, которые
// ClassifyByEnding не берётся классифицировать, и для будущих разносклоняемых
// исключений.
// isAgentNoun - "-тель" ("оповещатель", "получатель"): существительное
// мужского рода, даже когда словарь его не знает.
func isAgentNoun(nomSg string) bool {
	return strings.HasSuffix(strings.ToLower(nomSg), "тель")
}

// guessLoanwordSingular - единственное число слова во множественном числе,
// которого нет в словаре (обычно заимствование: "токены", "коммиты",
// "биндинги"): "-ы" после твёрдой согласной и "-и" после г/к/х/ж/ш/ч/щ -
// основа мужского рода без окончания. У "-ки" это может быть и женский род
// ("вкладки" - "вкладка"), но дательный, творительный и предложный
// множественного числа ("вкладкам", "вкладками", "вкладках") от этого не
// меняются.
func guessLoanwordSingular(plural string) (string, bool) {
	runes := []rune(strings.ToLower(plural))
	if len(runes) < 3 {
		return "", false
	}
	last, prev := runes[len(runes)-1], runes[len(runes)-2]
	stem := string([]rune(plural)[:len(runes)-1])
	switch {
	case last == 'ы' && strings.ContainsRune("бвдзклмнпрстфх", prev):
		return stem, true
	case last == 'и' && strings.ContainsRune("гкхжшчщ", prev):
		return stem, true
	}
	return "", false
}

// registerLexiconNoun - имя с парадигмой из словаря OpenCorpora
// (lexicon.go): все формы и их варианты попадают в обратный индекс.
func (d *Dictionary) registerLexiconNoun(name string, e lexiconEntry) error {
	if _, exists := d.lemmas[name]; exists {
		return fmt.Errorf("лемма %q уже зарегистрирована", name)
	}
	d.lemmas[name] = &Lemma{
		Text:  name,
		POS:   Noun,
		Noun:  &NounInfo{Class: ClassIrregular, Gender: e.gender, Animacy: e.animacy},
		Forms: e.paradigm,
	}
	for tag, forms := range e.variants {
		for _, form := range forms {
			d.addReverse(form, Analysis{Lemma: name, POS: Noun, Tag: tag})
		}
	}
	return nil
}

func (d *Dictionary) RegisterNounExplicit(nomSg string, class DeclensionClass, gender Gender, animacy Animacy) error {
	return d.registerNounFromSingular(nomSg, nomSg, class, gender, animacy)
}

// registerNounFromSingular - общее тело RegisterNounExplicit: строит
// парадигму ИЗ singularForm (именительный падеж единственного числа -
// BuildNounParadigm умеет работать только с ним), но регистрирует лемму и
// обратный индекс под lemmaKey - отдельным параметром ради
// tryDepluralize (там lemmaKey - буквальное имя переменной,
// написанное программистом во множественном числе, "сервисы", а
// singularForm - восстановленная по нему форма "сервис", по которой только
// и можно построить парадигму).
func (d *Dictionary) registerNounFromSingular(lemmaKey, singularForm string, class DeclensionClass, gender Gender, animacy Animacy) error {
	if _, exists := d.lemmas[lemmaKey]; exists {
		return fmt.Errorf("лемма %q уже зарегистрирована", lemmaKey)
	}
	paradigm, err := BuildNounParadigm(singularForm, class, animacy)
	if err != nil {
		return err
	}
	d.lemmas[lemmaKey] = &Lemma{
		Text:  lemmaKey,
		POS:   Noun,
		Noun:  &NounInfo{Class: class, Gender: gender, Animacy: animacy},
		Forms: paradigm,
	}
	for tag, form := range paradigm {
		d.addReverse(form, Analysis{Lemma: lemmaKey, POS: Noun, Tag: tag})
	}
	return nil
}

// registerNounWithParadigm регистрирует существительное по готовой,
// вручную выписанной парадигме (nounFullExceptions) - винительный падеж, не
// входящий в неё, достраивается по одушевлённости тем же правилом, что и
// генератор (fillAccusative). Класс склонения - ClassIrregular: у слова нет
// generic-класса, парадигма не выводима правилом.
func (d *Dictionary) registerNounWithParadigm(nomSg string, gender Gender, animacy Animacy, paradigm Paradigm) error {
	if _, exists := d.lemmas[nomSg]; exists {
		return fmt.Errorf("лемма %q уже зарегистрирована", nomSg)
	}
	p := Paradigm{}
	maps.Copy(p, paradigm)
	fillAccusative(p, animacy, "")
	d.lemmas[nomSg] = &Lemma{
		Text:  nomSg,
		POS:   Noun,
		Noun:  &NounInfo{Class: ClassIrregular, Gender: gender, Animacy: animacy},
		Forms: p,
	}
	for tag, form := range p {
		d.addReverse(form, Analysis{Lemma: nomSg, POS: Noun, Tag: tag})
	}
	return nil
}

// RegisterVerb регистрирует глагол по явно заданным формам (см. VerbForms).
func (d *Dictionary) RegisterVerb(infinitive string, forms VerbForms) error {
	if _, exists := d.lemmas[infinitive]; exists {
		return fmt.Errorf("лемма %q уже зарегистрирована", infinitive)
	}
	if forms.Infinitive == "" {
		forms.Infinitive = infinitive
	}
	d.lemmas[infinitive] = &Lemma{Text: infinitive, POS: Verb, Verb: &VerbInfo{Forms: forms}}

	add := func(form string, kind VerbFormKind) {
		if form == "" {
			return
		}
		d.addReverse(form, Analysis{Lemma: infinitive, POS: Verb, VerbForm: kind})
	}
	add(forms.Infinitive, FormInfinitive)
	add(forms.PresentOrFuture3Sg, FormPresentOrFuture3Sg)
	add(forms.PastM, FormPastM)
	add(forms.PastF, FormPastF)
	add(forms.PastN, FormPastN)
	add(forms.PastPl, FormPastPl)
	add(forms.Gerund, FormGerund)
	add(forms.PastParticiple, FormPastParticiple)
	return nil
}

// Lookup возвращает словарную запись леммы, если она зарегистрирована.
func (d *Dictionary) Lookup(lemma string) (*Lemma, bool) {
	l, ok := d.lemmas[lemma]
	return l, ok
}

// FormOf возвращает словоформу lemma (существительного) в падеже c,
// единственного числа (у pluralia tantum - множественного) - нужен там, где известна лемма и нужна ИМЕННО эта
// форма (например, чтобы показать её в сообщении об ошибке "ожидался
// винительный падеж - %q"), в отличие от Analyze, который идёт в обратную
// сторону (от словоформы к лемме). false - lemma не существительное или не
// зарегистрирована вовсе.
func (d *Dictionary) FormOf(lemma string, c Case) (string, bool) {
	l, ok := d.lemmas[lemma]
	if !ok || l.Forms == nil {
		return "", false
	}
	if form, ok := l.Forms[Tag{Case: c, Number: Singular}]; ok {
		return form, true
	}
	// Pluralia tantum ("ножницы") - единственного числа нет вовсе.
	form, ok := l.Forms[Tag{Case: c, Number: Plural}]
	return form, ok
}

// Analyze возвращает все известные разборы словоформы. Пустой срез - слово
// не входит в словарь программы (обычный текст, а не идентификатор/системное
// слово). Разрешение неоднозначности - на вызывающей стороне (docs/MORPHOLOGY_DESIGN.md).
func (d *Dictionary) Analyze(wordform string) []Analysis {
	return d.reverse[wordform]
}
