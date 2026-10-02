package numfmt

import "testing"

func TestFloat(t *testing.T) {
	a, b := 0.1, 0.2
	cases := []struct {
		f    float64
		bits int
		want string
	}{
		{a + b, 64, "0.30000000000000004"},
		{0.3, 64, "0.3"},
		{float64(float32(0.1)), 32, "0.1"},
		{100, 64, "100"},
		{0, 64, "0"},
		{-2.5, 64, "-2.5"},
		{0.0001, 64, "0.0001"},
		{0.00001, 64, "1e-05"},
		{1e15, 64, "1000000000000000"},
		{1e16, 64, "1e+16"},
		{1.8446744073709552e19, 64, "1.8446744073709552e+19"},
		{3e-15, 64, "3e-15"},
		{1e-300, 64, "1e-300"},
	}
	for _, c := range cases {
		if got := Float(c.f, c.bits); got != c.want {
			t.Errorf("Float(%v, %d) = %q, ожидалось %q", c.f, c.bits, got, c.want)
		}
	}
}
