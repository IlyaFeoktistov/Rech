// Package decimal - тип "Десятичное": точная десятичная арифметика, как
// decimal в C#. Значение - целая мантисса до 2^96 - 1, знак и масштаб от
// 0 до 28: число = мантисса / 10^масштаб. 0.1 + 0.2 = 0.3 ровно.
//
// Округление - однократное и банковское (половина - к чётному: 2.5 -> 2,
// 3.5 -> 4): результат, не помещающийся в 96 бит или с масштабом больше
// 28, теряет младшие цифры ровно настолько, насколько нужно. Не
// помещается и с масштабом 0 - переполнение.
//
// cbackend повторяет тот же алгоритм на C (decimal_runtime.go) - любое
// изменение правила делается в обоих местах.
package decimal

import (
	"errors"
	"math/big"
	"strings"
)

// MaxScale - наибольший масштаб (знаков после запятой).
const MaxScale = 28

// MaxParseDigits - наибольшее число значащих цифр в записи числа (Parse):
// C-рантайм разбирает запись в 192-битное целое.
const MaxParseDigits = 57

// Decimal - значение "Десятичное". Нулевое значение - 0.
type Decimal struct {
	Lo    uint64 // младшие 64 бита мантиссы
	Hi    uint32 // старшие 32 бита мантиссы
	Scale uint8
	Neg   bool
}

var (
	// ErrOverflow - результат не помещается в Десятичное.
	ErrOverflow = errors.New("переполнение Десятичное: значение не помещается в 96 бит мантиссы")
	// ErrDivByZero - деление или остаток на ноль.
	ErrDivByZero = errors.New("деление на ноль")
	// ErrSyntax - строка не является десятичным числом.
	ErrSyntax = errors.New("не является десятичным числом")
)

var (
	ten    = big.NewInt(10)
	maxMan = new(big.Int).Sub(new(big.Int).Lsh(big.NewInt(1), 96), big.NewInt(1))
)

func (d Decimal) mant() *big.Int {
	m := new(big.Int).SetUint64(uint64(d.Hi))
	m.Lsh(m, 64)
	return m.Or(m, new(big.Int).SetUint64(d.Lo))
}

// IsZero - d == 0 (при любом масштабе).
func (d Decimal) IsZero() bool { return d.Lo == 0 && d.Hi == 0 }

func pow10(k int) *big.Int { return new(big.Int).Exp(ten, big.NewInt(int64(k)), nil) }

// roundDivPow10 - n / 10^k, округлённое банковски (n >= 0).
func roundDivPow10(n *big.Int, k int) *big.Int {
	if k == 0 {
		return new(big.Int).Set(n)
	}
	p := pow10(k)
	q, r := new(big.Int).QuoRem(n, p, new(big.Int))
	half := new(big.Int).Rsh(p, 1)
	switch c := r.Cmp(half); {
	case c > 0, c == 0 && q.Bit(0) == 1:
		q.Add(q, big.NewInt(1))
	}
	return q
}

// normalize - точная величина n / 10^scale со знаком neg, приведённая к
// Десятичное: масштаб не больше MaxScale, мантисса не больше 96 бит.
func normalize(n *big.Int, scale int, neg bool) (Decimal, error) {
	k := 0
	if scale > MaxScale {
		k = scale - MaxScale
	}
	for {
		q := roundDivPow10(n, k)
		if q.Cmp(maxMan) <= 0 {
			return fromMant(q, scale-k, neg), nil
		}
		if scale-k <= 0 {
			return Decimal{}, ErrOverflow
		}
		k++
	}
}

func fromMant(m *big.Int, scale int, neg bool) Decimal {
	lo := new(big.Int).And(m, new(big.Int).SetUint64(^uint64(0))).Uint64()
	hi := uint32(new(big.Int).Rsh(m, 64).Uint64())
	if m.Sign() == 0 {
		neg = false
	}
	return Decimal{Lo: lo, Hi: hi, Scale: uint8(scale), Neg: neg}
}

// FromInt - целое v ровно.
func FromInt(v int64) Decimal {
	if v < 0 {
		return fromMant(new(big.Int).Neg(big.NewInt(v)), 0, true)
	}
	return fromMant(big.NewInt(v), 0, false)
}

// FromUint - беззнаковое v ровно.
func FromUint(v uint64) Decimal { return fromMant(new(big.Int).SetUint64(v), 0, false) }

// Parse - "-123.4500", "1e-5", "+7": точно (лишние знаки после 28-го -
// банковское округление).
func Parse(s string) (Decimal, error) {
	t := s
	neg := false
	if strings.HasPrefix(t, "-") || strings.HasPrefix(t, "+") {
		neg = t[0] == '-'
		t = t[1:]
	}
	exp := 0
	if i := strings.IndexAny(t, "eE"); i >= 0 {
		e, ok := parseExp(t[i+1:])
		if !ok {
			return Decimal{}, ErrSyntax
		}
		exp, t = e, t[:i]
	}
	intPart, frac, hasPoint := strings.Cut(t, ".")
	if intPart == "" && frac == "" || hasPoint && frac == "" && intPart == "" {
		return Decimal{}, ErrSyntax
	}
	digits := intPart + frac
	for _, c := range digits {
		if c < '0' || c > '9' {
			return Decimal{}, ErrSyntax
		}
	}
	if len(strings.TrimLeft(digits, "0")) > MaxParseDigits {
		return Decimal{}, ErrSyntax
	}
	n, _ := new(big.Int).SetString(digits, 10)
	scale := len(frac) - exp
	if scale < 0 {
		n.Mul(n, pow10(-scale))
		scale = 0
	}
	return normalize(n, scale, neg)
}

func parseExp(s string) (int, bool) {
	neg := false
	if strings.HasPrefix(s, "-") || strings.HasPrefix(s, "+") {
		neg = s[0] == '-'
		s = s[1:]
	}
	if s == "" || len(s) > 4 {
		return 0, false
	}
	v := 0
	for _, c := range s {
		if c < '0' || c > '9' {
			return 0, false
		}
		v = v*10 + int(c-'0')
	}
	if neg {
		v = -v
	}
	return v, true
}

// String - запись с масштабом: 1.10 остаётся "1.10", 0 с масштабом 2 -
// "0.00".
func (d Decimal) String() string {
	digits := d.mant().String()
	s := int(d.Scale)
	if s > 0 {
		if len(digits) <= s {
			digits = strings.Repeat("0", s-len(digits)+1) + digits
		}
		digits = digits[:len(digits)-s] + "." + digits[len(digits)-s:]
	}
	if d.Neg && !d.IsZero() {
		return "-" + digits
	}
	return digits
}

// aligned - мантиссы a и b со знаком, приведённые к общему масштабу.
func aligned(a, b Decimal) (*big.Int, *big.Int, int) {
	s := max(int(a.Scale), int(b.Scale))
	am := a.mant()
	am.Mul(am, pow10(s-int(a.Scale)))
	bm := b.mant()
	bm.Mul(bm, pow10(s-int(b.Scale)))
	if a.Neg {
		am.Neg(am)
	}
	if b.Neg {
		bm.Neg(bm)
	}
	return am, bm, s
}

func signed(n *big.Int, scale int) (Decimal, error) {
	return normalize(new(big.Int).Abs(n), scale, n.Sign() < 0)
}

// Add - a + b.
func Add(a, b Decimal) (Decimal, error) {
	am, bm, s := aligned(a, b)
	return signed(am.Add(am, bm), s)
}

// Sub - a - b.
func Sub(a, b Decimal) (Decimal, error) {
	am, bm, s := aligned(a, b)
	return signed(am.Sub(am, bm), s)
}

// Mul - a * b.
func Mul(a, b Decimal) (Decimal, error) {
	m := a.mant()
	m.Mul(m, b.mant())
	return normalize(m, int(a.Scale)+int(b.Scale), a.Neg != b.Neg)
}

// Div - a / b: точный результат, если он помещается, иначе столько
// знаков, сколько помещается (не больше 28 после запятой), последний -
// банковское округление. 1/3 = 0.3333333333333333333333333333, 4.00/2 =
// 2.00.
func Div(a, b Decimal) (Decimal, error) {
	if b.IsZero() {
		return Decimal{}, ErrDivByZero
	}
	bm := b.mant()
	q, r := new(big.Int).QuoRem(a.mant(), bm, new(big.Int))
	scale := int(a.Scale) - int(b.Scale)
	step := func() {
		r.Mul(r, ten)
		d, rr := new(big.Int).QuoRem(r, bm, new(big.Int))
		q.Mul(q, ten)
		q.Add(q, d)
		r = rr
		scale++
	}
	for scale < 0 {
		step()
		if q.Cmp(maxMan) > 0 {
			return Decimal{}, ErrOverflow
		}
	}
	for r.Sign() != 0 && scale < MaxScale {
		next := new(big.Int).Mul(r, ten)
		d := new(big.Int).Quo(next, bm)
		if new(big.Int).Add(new(big.Int).Mul(q, ten), d).Cmp(maxMan) > 0 {
			break
		}
		step()
	}
	if r.Sign() != 0 {
		twice := new(big.Int).Lsh(r, 1)
		if c := twice.Cmp(bm); c > 0 || c == 0 && q.Bit(0) == 1 {
			q.Add(q, big.NewInt(1))
		}
	}
	return normalize(q, scale, a.Neg != b.Neg)
}

// Rem - остаток a % b, знак - у делимого (как у "%" целых).
func Rem(a, b Decimal) (Decimal, error) {
	if b.IsZero() {
		return Decimal{}, ErrDivByZero
	}
	am, bm, s := aligned(a, b)
	r := new(big.Int).Rem(am.Abs(am), bm.Abs(bm))
	return normalize(r, s, a.Neg)
}

// Neg - -d.
func Neg(d Decimal) Decimal {
	if d.IsZero() {
		return d
	}
	d.Neg = !d.Neg
	return d
}

// Cmp - -1/0/1: a < b, a == b, a > b (1.10 == 1.1).
func Cmp(a, b Decimal) int {
	am, bm, _ := aligned(a, b)
	return am.Cmp(bm)
}

// Round - d с не больше чем places знаков после запятой (банковское
// округление); places вне 0..28 приводится к границе.
func Round(d Decimal, places int) Decimal {
	places = min(max(places, 0), MaxScale)
	if int(d.Scale) <= places {
		return d
	}
	return fromMant(roundDivPow10(d.mant(), int(d.Scale)-places), places, d.Neg)
}

// ToInt - целая часть (отбрасывание к нулю); ok=false - не помещается в
// Целое.
func ToInt(d Decimal) (int64, bool) {
	q := new(big.Int).Quo(d.mant(), pow10(int(d.Scale)))
	if d.Neg {
		q.Neg(q)
	}
	if !q.IsInt64() {
		return 0, false
	}
	return q.Int64(), true
}
