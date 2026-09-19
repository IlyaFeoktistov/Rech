package interp

import (
	"testing"
	"time"
)

func TestRunDateTimeConversionsMatchKnownEpoch(t *testing.T) {
	got := runSrc(t, `
функция главная
{
    переменная дата = время_как_дата(1700000000)
    вывести `+"`${"+`дату_в_строку(дата)}\n`+"`"+`
    вывести `+"`${"+`дата_как_время(дата)}\n`+"`"+`
}
`)
	want := "2023-11-14 22:13:20\n1700000000\n"
	if got != want {
		t.Fatalf("получили %q, хотели %q", got, want)
	}
}

func TestRunDateTimeConversionsMatchNegativeEpoch(t *testing.T) {
	got := runSrc(t, `
функция главная
{
    переменная дата = время_как_дата(-100000)
    вывести `+"`${"+`дату_в_строку(дата)}\n`+"`"+`
    вывести `+"`${"+`дата_как_время(дата)}\n`+"`"+`
}
`)
	want := "1969-12-30 20:13:20\n-100000\n"
	if got != want {
		t.Fatalf("получили %q, хотели %q", got, want)
	}
}

func TestCivilCalendarRoundTripsAgainstStdlib(t *testing.T) {
	ref := time.Date(1970, 1, 1, 0, 0, 0, 0, time.UTC)
	for _, days := range []int64{0, 1, 365, -1, -365, 18000, -18000, 730000, -730000} {
		wantT := ref.AddDate(0, 0, int(days))
		y, m, d := civilFromDays(days)
		if int(y) != wantT.Year() || int(m) != int(wantT.Month()) || int(d) != wantT.Day() {
			t.Fatalf("civilFromDays(%d) = %d-%d-%d, ожидалось %d-%d-%d", days, y, m, d, wantT.Year(), wantT.Month(), wantT.Day())
		}
		gotDays := daysFromCivil(y, m, d)
		if gotDays != days {
			t.Fatalf("daysFromCivil(%d, %d, %d) = %d, ожидалось %d", y, m, d, gotDays, days)
		}
	}
}
