package decimal

import "testing"

func mustParse(t *testing.T, s string) Decimal {
	t.Helper()
	d, err := Parse(s)
	if err != nil {
		t.Fatalf("Parse(%q): %v", s, err)
	}
	return d
}

func TestArithmetic(t *testing.T) {
	cases := []struct {
		op   func(a, b Decimal) (Decimal, error)
		a, b string
		want string
	}{
		{Add, "0.1", "0.2", "0.3"},
		{Add, "1.10", "2", "3.10"},
		{Sub, "1", "0.01", "0.99"},
		{Sub, "0.5", "0.5", "0.0"},
		{Mul, "1.5", "-2", "-3.0"},
		{Mul, "0.1", "0.1", "0.01"},
		{Div, "1", "3", "0.3333333333333333333333333333"},
		{Div, "2", "3", "0.6666666666666666666666666667"},
		{Div, "1", "4", "0.25"},
		{Div, "4.00", "2", "2.00"},
		{Div, "10", "0.5", "20"},
		{Div, "-7", "2", "-3.5"},
		{Rem, "7.5", "2", "1.5"},
		{Rem, "-7", "3", "-1"},
		{Add, "79228162514264337593543950335", "0.4", "79228162514264337593543950335"},
		{Mul, "0.0000000000000001", "0.0000000000000001", "0.0000000000000000000000000000"},
	}
	for _, c := range cases {
		got, err := c.op(mustParse(t, c.a), mustParse(t, c.b))
		if err != nil {
			t.Errorf("%s ? %s: %v", c.a, c.b, err)
			continue
		}
		if got.String() != c.want {
			t.Errorf("%s ? %s = %s, ожидалось %s", c.a, c.b, got, c.want)
		}
	}
	if _, err := Add(mustParse(t, "79228162514264337593543950335"), mustParse(t, "1")); err != ErrOverflow {
		t.Errorf("ожидалось переполнение, получено %v", err)
	}
	if _, err := Div(mustParse(t, "1"), mustParse(t, "0")); err != ErrDivByZero {
		t.Errorf("ожидалось деление на ноль, получено %v", err)
	}
}

func TestBankersRounding(t *testing.T) {
	cases := map[string]string{"2.5": "2", "3.5": "4", "-2.5": "-2", "2.51": "3", "1.451": "1", "0.125": "0.12"}
	for in, want := range cases {
		places := 0
		if want == "0.12" {
			places = 2
		}
		if got := Round(mustParse(t, in), places).String(); got != want {
			t.Errorf("Round(%s, %d) = %s, ожидалось %s", in, places, got, want)
		}
	}
	// 29 знаков после запятой - округление к 28.
	if got := mustParse(t, "0.00000000000000000000000000025").String(); got != "0.0000000000000000000000000002" {
		t.Errorf("округление при разборе: %s", got)
	}
}

func TestCmpAndConvert(t *testing.T) {
	if Cmp(mustParse(t, "1.10"), mustParse(t, "1.1")) != 0 || Cmp(mustParse(t, "-1"), mustParse(t, "0.5")) != -1 {
		t.Error("Cmp")
	}
	if v, ok := ToInt(mustParse(t, "-7.9")); !ok || v != -7 {
		t.Errorf("ToInt = %d, %v", v, ok)
	}
	if mustParse(t, "1e-5").String() != "0.00001" || mustParse(t, "1.5e3").String() != "1500" {
		t.Error("экспонента")
	}
	for _, bad := range []string{"", "-", "1.2.3", "abc", ".", "1e"} {
		if _, err := Parse(bad); err == nil {
			t.Errorf("Parse(%q) должен быть ошибкой", bad)
		}
	}
}
