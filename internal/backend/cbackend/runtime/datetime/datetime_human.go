package datetime

import "strings"

// DayOfWeekHelper - "день_недели(д): Целое" (0 = воскресенье .. 6 = суббота) -
// и, косвенно, "д.является_выходным()" (internal/semantic/datetime_methods.go
// разворачивает его в "день_недели(д) == 0 || день_недели(д) == 6"). Только
// календарная дата важна, не время суток - использует уже существующий
// rech_days_from_civil (datetime.go, тот же алгоритм Ховарда Хиннанта),
// день недели 1970-01-01 (эпоха) - четверг (индекс 4).
func DayOfWeekHelper(dtType, fYear, fMonth, fDay string) string {
	const tmpl = `static long long rech_day_of_week(@@DT@@ d) {
    long long days = rech_days_from_civil(d.@@FY@@, d.@@FM@@, d.@@FD@@);
    long long w = (days + 4) % 7;
    if (w < 0) {
        w += 7;
    }
    return w;
}

`
	r := strings.NewReplacer("@@DT@@", dtType, "@@FY@@", fYear, "@@FM@@", fMonth, "@@FD@@", fDay)
	return r.Replace(tmpl)
}

// HumanDateHelper - "дату_в_читаемом_виде(д): Строка" (и "д.в_читаемом_виде()") -
// относительное время в стиле Carbon'овского diffForHumans(), сравнивает d с
// ТЕКУЩИМ моментом (rech_time_now) - "5 минут назад"/"через 2 часа". Только
// секунды/минуты/часы/дни (без месяцев/лет - тот же осознанно уменьшенный
// охват, что и у арифметических методов, package doc datetime_methods.go) -
// русское склонение числительных (1/2-4/5+, с исключением 11-14) через
// rech_ru_plural, отдельную от dateToString'а функцию (там склонения нет
// вовсе).
func HumanDateHelper(dtType string) string {
	const tmpl = `static const char *rech_ru_plural(long long n, const char *one, const char *few, const char *many) {
    long long n100 = n % 100;
    if (n100 >= 11 && n100 <= 14) {
        return many;
    }
    switch (n % 10) {
        case 1:
            return one;
        case 2:
        case 3:
        case 4:
            return few;
        default:
            return many;
    }
}

static char *rech_date_to_human(@@DT@@ d) {
    long long ts = rech_date_to_time(d);
    long long now = rech_time_now();
    long long diff = now - ts;
    int future = diff < 0;
    if (diff < 0) {
        diff = -diff;
    }
    char *out = malloc(64);
    if (diff < 5) {
        snprintf(out, 64, "только что");
        return out;
    }
    long long n;
    const char *unit;
    if (diff < 60) {
        n = diff;
        unit = rech_ru_plural(n, "секунда", "секунды", "секунд");
    } else if (diff < 3600) {
        n = diff / 60;
        unit = rech_ru_plural(n, "минута", "минуты", "минут");
    } else if (diff < 86400) {
        n = diff / 3600;
        unit = rech_ru_plural(n, "час", "часа", "часов");
    } else {
        n = diff / 86400;
        unit = rech_ru_plural(n, "день", "дня", "дней");
    }
    if (future) {
        snprintf(out, 64, "через %lld %s", n, unit);
    } else {
        snprintf(out, 64, "%lld %s назад", n, unit);
    }
    return out;
}

`
	return strings.ReplaceAll(tmpl, "@@DT@@", dtType)
}
