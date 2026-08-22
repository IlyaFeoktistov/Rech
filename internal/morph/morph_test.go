package morph

import "testing"

// golden-таблица словоформ, сверенная вручную с реальным русским
// словоизменением - см. docs/MORPHOLOGY.md, "Стратегия тестирования".
// Ошибка здесь означает ошибку в данных (окончаниях), а не в логике.
func TestNounParadigms(t *testing.T) {
	cases := []struct {
		name    string
		nomSg   string
		class   DeclensionClass
		animacy Animacy
		want    map[Tag]string
	}{
		{
			name:  "пользователь (муж., мягкая, одуш.)",
			nomSg: "пользователь", class: ClassFirstSoft, animacy: Animate,
			want: map[Tag]string{
				{Nominative, Singular}:    "пользователь",
				{Genitive, Singular}:      "пользователя",
				{Dative, Singular}:        "пользователю",
				{Accusative, Singular}:    "пользователя",
				{Instrumental, Singular}:  "пользователем",
				{Prepositional, Singular}: "пользователе",
				{Nominative, Plural}:      "пользователи",
				{Genitive, Plural}:        "пользователей",
				{Dative, Plural}:          "пользователям",
				{Accusative, Plural}:      "пользователей",
				{Instrumental, Plural}:    "пользователями",
				{Prepositional, Plural}:   "пользователях",
			},
		},
		{
			name:  "пароль (муж., мягкая, неодуш.)",
			nomSg: "пароль", class: ClassFirstSoft, animacy: Inanimate,
			want: map[Tag]string{
				{Nominative, Singular}:    "пароль",
				{Genitive, Singular}:      "пароля",
				{Dative, Singular}:        "паролю",
				{Accusative, Singular}:    "пароль",
				{Instrumental, Singular}:  "паролем",
				{Prepositional, Singular}: "пароле",
				{Nominative, Plural}:      "пароли",
				{Accusative, Plural}:      "пароли",
			},
		},
		{
			name:  "документ (муж., твёрдая, неодуш.)",
			nomSg: "документ", class: ClassFirstHard, animacy: Inanimate,
			want: map[Tag]string{
				{Nominative, Singular}:    "документ",
				{Genitive, Singular}:      "документа",
				{Dative, Singular}:        "документу",
				{Accusative, Singular}:    "документ",
				{Instrumental, Singular}:  "документом",
				{Prepositional, Singular}: "документе",
				{Nominative, Plural}:      "документы",
				{Genitive, Plural}:        "документов",
			},
		},
		{
			name:  "система (жен., твёрдая, неодуш.)",
			nomSg: "система", class: ClassSecondHard, animacy: Inanimate,
			want: map[Tag]string{
				{Nominative, Singular}:    "система",
				{Genitive, Singular}:      "системы",
				{Dative, Singular}:        "системе",
				{Accusative, Singular}:    "систему",
				{Instrumental, Singular}:  "системой",
				{Prepositional, Singular}: "системе",
				{Nominative, Plural}:      "системы",
				{Genitive, Plural}:        "систем",
			},
		},
		{
			name:  "компания (жен., мягкая, неодуш.)",
			nomSg: "компания", class: ClassSecondSoft, animacy: Inanimate,
			want: map[Tag]string{
				{Nominative, Singular}:    "компания",
				{Genitive, Singular}:      "компании",
				{Dative, Singular}:        "компании",
				{Accusative, Singular}:    "компанию",
				{Instrumental, Singular}:  "компанией",
				{Prepositional, Singular}: "компании",
			},
		},
		{
			name:  "сообщение (сред., мягкая, неодуш.)",
			nomSg: "сообщение", class: ClassNeuterSoft, animacy: Inanimate,
			want: map[Tag]string{
				{Nominative, Singular}:    "сообщение",
				{Genitive, Singular}:      "сообщения",
				{Dative, Singular}:        "сообщению",
				{Accusative, Singular}:    "сообщение",
				{Instrumental, Singular}:  "сообщением",
				{Prepositional, Singular}: "сообщении",
				{Nominative, Plural}:      "сообщения",
				{Genitive, Plural}:        "сообщений",
				{Prepositional, Plural}:   "сообщениях",
			},
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got, err := BuildNounParadigm(tc.nomSg, tc.class, tc.animacy)
			if err != nil {
				t.Fatalf("BuildNounParadigm(%q): %v", tc.nomSg, err)
			}
			for tag, want := range tc.want {
				if got[tag] != want {
					t.Errorf("%s %s: получено %q, ожидалось %q", tag.Case, tag.Number, got[tag], want)
				}
			}
		})
	}
}

func TestClassifyByEnding(t *testing.T) {
	cases := []struct {
		word      string
		wantClass DeclensionClass
		wantOK    bool
	}{
		{"документ", ClassFirstHard, true},
		{"система", ClassSecondHard, true},
		{"компания", ClassSecondSoft, true},
		{"сообщение", ClassNeuterSoft, true},
		{"окно", ClassNeuterHard, true},
		{"пользователь", 0, false}, // "-ь" неоднозначно - только через exceptions/Explicit
		{"пароль", 0, false},
		{"случай", 0, false}, // "-й" - вне Фазы A
	}
	for _, tc := range cases {
		class, ok := ClassifyByEnding(tc.word)
		if ok != tc.wantOK {
			t.Errorf("ClassifyByEnding(%q) ok = %v, want %v", tc.word, ok, tc.wantOK)
			continue
		}
		if ok && class != tc.wantClass {
			t.Errorf("ClassifyByEnding(%q) class = %v, want %v", tc.word, class, tc.wantClass)
		}
	}
}

func TestDictionaryRegisterNoun(t *testing.T) {
	d := NewDictionary()
	if err := d.RegisterNoun("пользователь", Animate); err != nil {
		t.Fatalf("RegisterNoun(пользователь): %v", err)
	}
	if err := d.RegisterNoun("документ", Inanimate); err != nil {
		t.Fatalf("RegisterNoun(документ): %v", err)
	}

	// Синкретизм: у одушевлённого пользователь винительный ед.ч. совпадает
	// с родительным - обе трактовки должны найтись при обратном поиске.
	got := d.Analyze("пользователя")
	if len(got) != 2 {
		t.Fatalf("Analyze(пользователя) = %v, ожидалось 2 варианта (родительный и винительный)", got)
	}
	seen := map[Case]bool{}
	for _, a := range got {
		seen[a.Tag.Case] = true
	}
	if !seen[Genitive] || !seen[Accusative] {
		t.Errorf("Analyze(пользователя) = %v, ожидались родительный и винительный падежи", got)
	}

	// Синкретизм у неодушевлённого документ: винительный = именительный.
	got = d.Analyze("документ")
	if len(got) != 2 {
		t.Fatalf("Analyze(документ) = %v, ожидалось 2 варианта (именительный и винительный)", got)
	}

	// Слово, которого нет в словаре, не должно давать ложных срабатываний.
	if got := d.Analyze("непонятноеслово"); len(got) != 0 {
		t.Errorf("Analyze(непонятноеслово) = %v, ожидался пустой срез", got)
	}
}

// TestDictionaryRegisterNounFullException проверяет "список" -
// nounFullExceptions (беглая гласная + написание "и" после заднеязычной во
// мн.ч.), первое слово, потребовавшее готовой парадигмы вместо генератора
// (docs/MORPHOLOGY.md, "Полностью готовая парадигма без генератора").
func TestDictionaryRegisterNounFullException(t *testing.T) {
	d := NewDictionary()
	if err := d.RegisterNoun("список", Inanimate); err != nil {
		t.Fatalf("RegisterNoun(список): %v", err)
	}

	got := d.Analyze("списке")
	if len(got) != 1 || got[0].Lemma != "список" || got[0].Tag != (Tag{Prepositional, Singular}) {
		t.Errorf("Analyze(списке) = %v, ожидался предложный падеж ед.ч. леммы \"список\"", got)
	}

	// Беглая гласная: "списока" (генератор по умолчанию) не форма слова -
	// а "списка" - да.
	if got := d.Analyze("списока"); len(got) != 0 {
		t.Errorf("Analyze(списока) = %v, ожидался пустой срез - это не форма \"список\"", got)
	}
	got = d.Analyze("списка")
	if len(got) != 1 || got[0].Tag.Case != Genitive {
		t.Errorf("Analyze(списка) = %v, ожидался родительный падеж", got)
	}

	// Неодушевлённое: винительный ед.ч. совпадает с именительным (достроен
	// fillAccusative, не входит в готовую парадигму).
	got = d.Analyze("список")
	seen := map[Case]bool{}
	for _, a := range got {
		seen[a.Tag.Case] = true
	}
	if !seen[Nominative] || !seen[Accusative] {
		t.Errorf("Analyze(список) = %v, ожидались именительный и винительный падежи", got)
	}
}

func TestDictionaryRegisterVerb(t *testing.T) {
	d := NewDictionary()
	if err := RegisterSystemVerbs(d); err != nil {
		t.Fatalf("RegisterSystemVerbs: %v", err)
	}

	got := d.Analyze("вошёл")
	if len(got) != 1 || got[0].Lemma != "войти" || got[0].VerbForm != FormPastM {
		t.Errorf("Analyze(вошёл) = %v, ожидалась форма прош. вр. муж. рода леммы \"войти\"", got)
	}

	got = d.Analyze("зарегистрировав")
	if len(got) != 1 || got[0].Lemma != "зарегистрировать" || got[0].VerbForm != FormGerund {
		t.Errorf("Analyze(зарегистрировав) = %v, ожидалось деепричастие леммы \"зарегистрировать\"", got)
	}
}

func TestRegisterNounAmbiguousRequiresExplicit(t *testing.T) {
	d := NewDictionary()
	if err := d.RegisterNoun("дверь", Inanimate); err == nil {
		t.Fatal("RegisterNoun(дверь) без словаря исключений должен вернуть ошибку - класс на \"-ь\" женского рода (ClassThirdSoft) не реализован (Фаза B), даже если gomorphy верно определил род")
	}
}

// TestRegisterNounViaGomorphy проверяет саму интеграцию с gomorphy: слово на
// "-ь", которого нет в нашем ручном nounGenderExceptions, должно всё равно
// зарегистрироваться - род взят из словаря OpenCorpora, а не угадан.
func TestRegisterNounViaGomorphy(t *testing.T) {
	if sharedGomorphy == nil {
		t.Skip("gomorphy не инициализировался (нет встроенного словаря) - пропускаем")
	}
	if _, ok := nounGenderExceptions["руководитель"]; ok {
		t.Fatal("тест предполагает, что \"руководитель\" НЕ в ручном словаре исключений - иначе он не проверяет gomorphy")
	}

	d := NewDictionary()
	if err := d.RegisterNoun("руководитель", Animate); err != nil {
		t.Fatalf("RegisterNoun(руководитель) через gomorphy: %v", err)
	}
	got := d.Analyze("руководителя")
	if len(got) == 0 {
		t.Fatal("Analyze(руководителя) пусто - регистрация через gomorphy не дала парадигмы")
	}
	found := false
	for _, a := range got {
		if a.Lemma == "руководитель" && a.Tag.Case == Genitive && a.Tag.Number == Singular {
			found = true
		}
	}
	if !found {
		t.Errorf("Analyze(руководителя) = %v, ожидался родительный падеж леммы \"руководитель\"", got)
	}
}

// TestFoldYo проверяет, что словоформа с "ё" находится и по варианту с "е".
func TestFoldYo(t *testing.T) {
	d := NewDictionary()
	if err := RegisterSystemVerbs(d); err != nil {
		t.Fatalf("RegisterSystemVerbs: %v", err)
	}
	withYo := d.Analyze("вошёл")
	withE := d.Analyze("вошел")
	if len(withYo) == 0 || len(withE) == 0 {
		t.Fatalf("Analyze(вошёл)=%v, Analyze(вошел)=%v - оба варианта должны находиться", withYo, withE)
	}
	if withYo[0].Lemma != withE[0].Lemma || withYo[0].VerbForm != withE[0].VerbForm {
		t.Errorf("вошёл и вошел разобрались по-разному: %v vs %v", withYo[0], withE[0])
	}
}

func TestLongFormAdjective(t *testing.T) {
	cases := []struct {
		short  string
		gender Gender
		number Number
		want   string
	}{
		{"авторизован", Masculine, Singular, "авторизованный"},
		{"авторизован", Feminine, Singular, "авторизованная"},
		{"авторизован", Neuter, Singular, "авторизованное"},
		{"закрыт", Masculine, Singular, "закрытый"},
		{"закрыт", Feminine, Singular, "закрытая"},
	}
	for _, c := range cases {
		got, err := LongFormAdjective(c.short, c.gender, c.number)
		if err != nil {
			t.Errorf("LongFormAdjective(%q, %v, %v): %v", c.short, c.gender, c.number, err)
			continue
		}
		if got != c.want {
			t.Errorf("LongFormAdjective(%q, %v, %v) = %q, ожидалось %q", c.short, c.gender, c.number, got, c.want)
		}
	}

	if _, err := LongFormAdjective("мал", Masculine, Singular); err == nil {
		t.Error("LongFormAdjective(мал, ...) - ожидалась ошибка (не причастие \"-н\"/\"-т\"), получено nil")
	}
}

func TestActiveParticipleAgreement(t *testing.T) {
	cases := []struct {
		masculine string
		gender    Gender
		number    Number
		want      string
	}{
		{"вошедший", Masculine, Singular, "вошедший"},
		{"вошедший", Feminine, Singular, "вошедшая"},
		{"вошедший", Neuter, Singular, "вошедшее"},
		{"вошедший", 0, Plural, "вошедшие"},
		{"создавший", Feminine, Singular, "создавшая"},
	}
	for _, c := range cases {
		got, err := ActiveParticipleAgreement(c.masculine, c.gender, c.number)
		if err != nil {
			t.Errorf("ActiveParticipleAgreement(%q, %v, %v): %v", c.masculine, c.gender, c.number, err)
			continue
		}
		if got != c.want {
			t.Errorf("ActiveParticipleAgreement(%q, %v, %v) = %q, ожидалось %q", c.masculine, c.gender, c.number, got, c.want)
		}
	}

	if _, err := ActiveParticipleAgreement("авторизован", Masculine, Singular); err == nil {
		t.Error("ActiveParticipleAgreement(авторизован, ...) - ожидалась ошибка (не оканчивается на \"-ший\"), получено nil")
	}
}
