package cbackend

import "fmt"

// datetimeHelpers - "время"/"время_как_дата"/"дата_как_время"/
// "дата_в_строку" - см. package doc internal/interp/datetime.go для
// полного обоснования алгоритма (Ховард Хиннант, чистая целочисленная
// арифметика вместо gmtime_r/timegm/gmtime_s/_mkgmtime - их доступность и
// точное поведение расходится между POSIX/MinGW/musl, собственная
// реализация переносима без единого #ifdef и байт-в-байт совпадает с
// портом того же алгоритма на Go в internal/interp). rech_time_now -
// единственная функция здесь, которой действительно нужен системный
// источник времени (текущий момент нельзя вычислить арифметикой) -
// обычный ANSI C time() из <time.h>, доступен без единого #ifdef на любой
// платформе, которую собирает internal/toolchain.
var datetimeHelpers = fmt.Sprintf(`static long long rech_time_now(void) {
    return (long long)time(NULL);
}

static long long rech_days_from_civil(long long y, long long m, long long d) {
    if (m <= 2) {
        y--;
    }
    long long era = (y >= 0 ? y : y - 399) / 400;
    long long yoe = y - era * 400;
    long long madj = m > 2 ? -3 : 9;
    long long doy = (153 * (m + madj) + 2) / 5 + d - 1;
    long long doe = yoe * 365 + yoe / 4 - yoe / 100 + doy;
    return era * 146097 + doe - 719468;
}

static void rech_civil_from_days(long long z, long long *y, long long *m, long long *d) {
    z += 719468;
    long long era = (z >= 0 ? z : z - 146096) / 146097;
    long long doe = z - era * 146097;
    long long yoe = (doe - doe / 1460 + doe / 36524 - doe / 146096) / 365;
    long long y_ = yoe + era * 400;
    long long doy = doe - (365 * yoe + yoe / 4 - yoe / 100);
    long long mp = (5 * doy + 2) / 153;
    *d = doy - (153 * mp + 2) / 5 + 1;
    *m = mp < 10 ? mp + 3 : mp - 9;
    *y = y_ + (*m <= 2 ? 1 : 0);
}

static %[1]s rech_time_to_date(long long ts) {
    long long days = ts / 86400;
    long long rem = ts %% 86400;
    if (rem < 0) {
        rem += 86400;
        days--;
    }
    long long y, m, d;
    rech_civil_from_days(days, &y, &m, &d);
    %[1]s out;
    out.%[2]s = y;
    out.%[3]s = m;
    out.%[4]s = d;
    out.%[5]s = rem / 3600;
    out.%[6]s = (rem / 60) %% 60;
    out.%[7]s = rem %% 60;
    return out;
}

static long long rech_date_to_time(%[1]s d) {
    long long days = rech_days_from_civil(d.%[2]s, d.%[3]s, d.%[4]s);
    return days * 86400 + d.%[5]s * 3600 + d.%[6]s * 60 + d.%[7]s;
}

static char *rech_date_to_string(%[1]s d) {
    char *out = malloc(32);
    snprintf(out, 32, "%%04lld-%%02lld-%%02lld %%02lld:%%02lld:%%02lld",
        d.%[2]s, d.%[3]s, d.%[4]s, d.%[5]s, d.%[6]s, d.%[7]s);
    return out;
}
`, mangleType("ДатаВремя"), mangleField("год"), mangleField("месяц"), mangleField("день"),
	mangleField("час"), mangleField("минута"), mangleField("секунда"))
