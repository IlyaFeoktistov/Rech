package morph

import "testing"

func TestInstrumentalLongFormAdjective(t *testing.T) {
	cases := []struct {
		short  string
		gender Gender
		number Number
		want   string
	}{
		{"авторизован", Masculine, Singular, "авторизованным"},
		{"авторизован", Feminine, Singular, "авторизованной"},
		{"авторизован", Neuter, Singular, "авторизованным"},
		{"авторизован", 0, Plural, "авторизованными"},
		{"закрыт", Masculine, Singular, "закрытым"},
		{"закрыт", Feminine, Singular, "закрытой"},
	}
	for _, c := range cases {
		got, err := InstrumentalLongFormAdjective(c.short, c.gender, c.number)
		if err != nil {
			t.Errorf("InstrumentalLongFormAdjective(%q, %v, %v): %v", c.short, c.gender, c.number, err)
			continue
		}
		if got != c.want {
			t.Errorf("InstrumentalLongFormAdjective(%q, %v, %v) = %q, ожидалось %q", c.short, c.gender, c.number, got, c.want)
		}
	}
}

func TestInstrumentalLongFormAdjectiveRejectsUnknownEnding(t *testing.T) {
	if _, err := InstrumentalLongFormAdjective("рад", Masculine, Singular); err == nil {
		t.Fatal("ожидалась ошибка - \"рад\" не оканчивается на \"н\"/\"т\"")
	}
}

func TestInstrumentalActiveParticiple(t *testing.T) {
	cases := []struct {
		masculine string
		gender    Gender
		number    Number
		want      string
	}{
		{"вошедший", Masculine, Singular, "вошедшим"},
		{"вошедший", Feminine, Singular, "вошедшей"},
		{"вошедший", Neuter, Singular, "вошедшим"},
		{"вошедший", 0, Plural, "вошедшими"},
		{"создавший", Feminine, Singular, "создавшей"},
	}
	for _, c := range cases {
		got, err := InstrumentalActiveParticiple(c.masculine, c.gender, c.number)
		if err != nil {
			t.Errorf("InstrumentalActiveParticiple(%q, %v, %v): %v", c.masculine, c.gender, c.number, err)
			continue
		}
		if got != c.want {
			t.Errorf("InstrumentalActiveParticiple(%q, %v, %v) = %q, ожидалось %q", c.masculine, c.gender, c.number, got, c.want)
		}
	}
}

func TestInstrumentalActiveParticipleRejectsWrongSuffix(t *testing.T) {
	if _, err := InstrumentalActiveParticiple("сделал", Masculine, Singular); err == nil {
		t.Fatal("ожидалась ошибка - \"сделал\" не оканчивается на \"-ший\"")
	}
}
