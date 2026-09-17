package datetime

import "strings"

// ParseHelpers - "разобрать_дату(строка): ?ДатаВремя" (internal/semantic/
// datetime.go) - строгий разбор формата "ГГГГ-ММ-ДД ЧЧ:ММ:СС", обратного к
// уже существующему rech_date_to_string (datetime.go). Не зависит от
// календарной арифметики (rech_days_from_civil/rech_civil_from_days) -
// поля читаются прямо из позиций строки, без пересчёта в секунды с эпохи и
// обратно.
//
// Плейсхолдеры (@@...@@), а не "%[N]s" fmt.Sprintf - у самого шаблона C
// достаточно настоящих '%' (нет ни одного здесь, но соседний tzdata.go уже
// требует Replacer по этой причине - тот же приём для единообразия).
func ParseHelpers(dtType, fYear, fMonth, fDay, fHour, fMin, fSec, nullableDT string) string {
	const tmpl = `static int rech_parse_date_digits(const char *s, int n, long long *out) {
    long long v = 0;
    for (int i = 0; i < n; i++) {
        if (s[i] < '0' || s[i] > '9') {
            return 0;
        }
        v = v * 10 + (s[i] - '0');
    }
    *out = v;
    return 1;
}

static @@NDT@@ rech_parse_date(const char *s) {
    if (strlen(s) != 19 || s[4] != '-' || s[7] != '-' || s[10] != ' ' || s[13] != ':' || s[16] != ':') {
        return @@NDT@@_empty();
    }
    long long y, mo, da, h, mi, se;
    if (!rech_parse_date_digits(s, 4, &y) || !rech_parse_date_digits(s + 5, 2, &mo) ||
        !rech_parse_date_digits(s + 8, 2, &da) || !rech_parse_date_digits(s + 11, 2, &h) ||
        !rech_parse_date_digits(s + 14, 2, &mi) || !rech_parse_date_digits(s + 17, 2, &se)) {
        return @@NDT@@_empty();
    }
    if (mo < 1 || mo > 12 || da < 1 || da > 31 || h > 23 || mi > 59 || se > 59) {
        return @@NDT@@_empty();
    }
    @@DT@@ d;
    d.@@FY@@ = y;
    d.@@FM@@ = mo;
    d.@@FD@@ = da;
    d.@@FH@@ = h;
    d.@@FMI@@ = mi;
    d.@@FS@@ = se;
    return @@NDT@@_wrap(d);
}

`
	r := strings.NewReplacer(
		"@@DT@@", dtType,
		"@@FY@@", fYear,
		"@@FM@@", fMonth,
		"@@FD@@", fDay,
		"@@FH@@", fHour,
		"@@FMI@@", fMin,
		"@@FS@@", fSec,
		"@@NDT@@", nullableDT,
	)
	return r.Replace(tmpl)
}
