package morph

import "testing"

// TestLexiconDeclinesHardWords - имена, которые правила по окончанию
// склоняли неверно или не склоняли вовсе: беглая гласная, супплетивы,
// субстантивированные прилагательные, местоимение, pluralia tantum,
// разносклоняемые, составные имена.
func TestLexiconDeclinesHardWords(t *testing.T) {
	cases := []struct {
		name string
		form string
		tag  Tag
	}{
		{"питомцы", "питомцах", Tag{Prepositional, Plural}},
		{"питомец", "питомцем", Tag{Instrumental, Singular}},
		{"животные", "животных", Tag{Prepositional, Plural}},
		{"все", "всех", Tag{Prepositional, Plural}},
		{"люди", "людях", Tag{Prepositional, Plural}},
		{"дети", "детьми", Tag{Instrumental, Plural}},
		{"ножницы", "ножницах", Tag{Prepositional, Plural}},
		{"дочери", "дочерях", Tag{Prepositional, Plural}},
		{"имена", "именах", Tag{Prepositional, Plural}},
		{"время", "времени", Tag{Genitive, Singular}},
		{"мыши", "мышах", Tag{Prepositional, Plural}},
		{"папа", "папой", Tag{Instrumental, Singular}},
		{"кофе", "кофе", Tag{Prepositional, Singular}},
		{"выбранные", "выбранных", Tag{Prepositional, Plural}},
		{"счётчики_слов", "счётчиках_слов", Tag{Prepositional, Plural}},
		{"главные_окна", "главных_окнах", Tag{Prepositional, Plural}},
		{"СписокДел", "СпискомДел", Tag{Instrumental, Singular}},
		{"Животное", "Животное", Tag{Accusative, Singular}},
		{"Строитель", "Строителя", Tag{Accusative, Singular}},
		{"стекла", "стеклах", Tag{Prepositional, Plural}},
		{"ежи", "ежах", Tag{Prepositional, Plural}},
		{"токены", "токенах", Tag{Prepositional, Plural}},
		{"коммиты", "коммитами", Tag{Instrumental, Plural}},
		{"биндинги", "биндингах", Tag{Prepositional, Plural}},
		{"вкладки", "вкладкам", Tag{Dative, Plural}},
		{"Оповещатель", "Оповещателем", Tag{Instrumental, Singular}},
	}
	d := NewDictionary()
	for _, c := range cases {
		if err := d.RegisterNoun(c.name, AnimacyOf(c.name)); err != nil {
			t.Errorf("%s: %v", c.name, err)
			continue
		}
		found := false
		for _, a := range d.Analyze(c.form) {
			if a.Lemma == c.name && a.Tag == c.tag {
				found = true
			}
		}
		if !found {
			l, _ := d.Lookup(c.name)
			t.Errorf("%s: форма %q (%v %v) не распознана; парадигма: %v", c.name, c.form, c.tag.Case, c.tag.Number, l.Forms)
		}
	}
}

// TestLexiconAgreement - род и число имени для согласования
// ("новый"/"новая"/"новое", модификатор видимости поля).
func TestLexiconAgreement(t *testing.T) {
	cases := []struct {
		name   string
		gender Gender
		number Number
	}{
		{"папа", Masculine, Singular},
		{"кофе", Masculine, Singular},
		{"ИгровоеПоле", Neuter, Singular},
		{"веса_скрытого", Masculine, Plural},
		{"адреса", Masculine, Plural},
		{"ножницы", 0, Plural},
	}
	for _, c := range cases {
		g, n, ok := CompoundNounAgreement(c.name)
		if !ok || n != c.number || (n == Singular && g != c.gender) {
			t.Errorf("%s: получено %v %v %v", c.name, g, n, ok)
		}
	}
}
