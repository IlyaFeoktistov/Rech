package textutf8

import "testing"

func TestFix(t *testing.T) {
	cases := map[string]string{
		"привет":           "привет",
		"a\x00b":           "a�b",
		"\xd0":             "�",
		"\xd0\x41":         "�A",
		"\xc0\xaf":         "��",  // overlong
		"\xed\xa0\x80":     "���", // суррогат
		"\xf4\x90\x80\x80": "����",
		"ok\xe2\x82":       "ok��",
		"�":                "�",
	}
	for in, want := range cases {
		if got := Fix(in); got != want {
			t.Errorf("Fix(%q) = %q, ожидалось %q", in, got, want)
		}
	}
}

func TestIncompleteTail(t *testing.T) {
	cases := map[string]int{
		"abc": 0, "ab\xd0": 1, "ab\xe2\x82": 2, "\xf0\x9f\x98": 3,
		"ab\xd0\x96": 0, "ab\xc0": 0, "ab\xed\xa0": 0, "\x82": 0,
	}
	for in, want := range cases {
		if got := IncompleteTail([]byte(in)); got != want {
			t.Errorf("IncompleteTail(%q) = %d, ожидалось %d", in, got, want)
		}
	}
}
