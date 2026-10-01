package tzdata

import "strings"

// Helpers - "время_как_дата_в_поясе(метка, пояс): ?ДатаВремя"/
// "дата_в_поясе_как_время(дата, пояс): ?Целое" (internal/semantic/
// datetime.go) - собственный читатель формата TZif (RFC 8536) + компактный
// вычислитель POSIX TZ-хвоста (для дат за пределами последнего явного
// перехода в файле - обычная ситуация для будущих дат, вендоренная копия
// не обновляется сама по себе) - НЕ системный tzset/localtime_r (те
// по-разному доступны на POSIX/MinGW, тот же класс проблемы, что у
// gmtime_r/timegm, см. package doc internal/interp/datetime.go), читает
// базу, вшитую в саму программу (Blob), или каталог TZDIR, если он задан.
//
// Известное упрощение (документировано, не случайный пробел): секунды
// координации (leap seconds) полностью игнорируются (как и почти во всём
// прикладном ПО); обратное преобразование (rech_date_to_time_tz) у местного
// времени ровно в момент перехода на летнее/зимнее (когда одно и то же
// "настенное" время соответствует двум разным UTC-моментам или не
// соответствует ни одному) разрешается ОДНОЙ итерацией уточнения смещения,
// не полным перебором обоих вариантов - для подавляющего большинства
// дат (не в течение часа перехода) разницы нет вообще.
//
// Плейсхолдеры (@@...@@) через strings.Replacer, а не "%[N]s" fmt.Sprintf,
// как у соседних runtime/datetime/core.go - этот блок C неизбежно содержит
// МНОГО настоящих '%' (оператор "остаток от деления", "%s"/"%02d" в
// printf-вызовах) - экранировать каждый как "%%" в шаблоне такого объёма
// было бы источником трудноуловимой опечатки, Replacer не требует
// экранирования вовсе. mangledType/*Field/nullableDT/
// nullableInt - уже вычисленные вызывающей стороной (escapeCString/
// mangleType/mangleField - внутренняя, неэкспортированная инфраструктура
// cbackend, этот пакет от неё не зависит).
func Helpers(dtType, fYear, fMonth, fDay, fHour, fMin, fSec, nullableDT, nullableInt string) string {
	const tmpl = `typedef struct {
    long long offset;
    int isdst;
} rech_tz_ttinfo;

typedef struct {
    long long *times;
    int *type_idx;
    int n_times;
    rech_tz_ttinfo *types;
    int n_types;
    int default_type;
    char posix_tz[128];
} rech_tz_t;

/* rech_tz_be - большой-эндиан знаковое целое шириной width (4 или 8) байт -
   TZif хранит все целые в сетевом порядке байт. Трюк со стартовым -1/0 по
   старшему биту первого байта корректно расширяет знак на всю разрядность
   long long при последующих сдвигах. */
static long long rech_tz_be(const unsigned char *p, int width) {
    long long v = (p[0] & 0x80) ? -1 : 0;
    for (int i = 0; i < width; i++) {
        v = (v << 8) | p[i];
    }
    return v;
}

/* rech_tz_buf - курсор по содержимому одного файла TZif в памяти (вшитая
   база rech_tz_blob или файл из TZDIR, прочитанный целиком). */
typedef struct {
    const unsigned char *p;
    size_t n;
    size_t pos;
} rech_tz_buf;

static int rech_tz_read(rech_tz_buf *b, void *dst, size_t k) {
    if (b->n - b->pos < k) {
        return 0;
    }
    memcpy(dst, b->p + b->pos, k);
    b->pos += k;
    return 1;
}

static int rech_tz_skip(rech_tz_buf *b, long long k) {
    if (k < 0 || (unsigned long long)k > b->n - b->pos) {
        return 0;
    }
    b->pos += (size_t)k;
    return 1;
}

static int rech_tz_getc(rech_tz_buf *b) {
    return b->pos < b->n ? b->p[b->pos++] : -1;
}

static int rech_tz_read_i32(rech_tz_buf *f, long long *out) {
    unsigned char b[4];
    if (!rech_tz_read(f, b, 4)) {
        return 0;
    }
    *out = rech_tz_be(b, 4);
    return 1;
}

static int rech_tz_header(rech_tz_buf *f, char *version, long long *isutcnt, long long *isstdcnt,
                           long long *leapcnt, long long *timecnt, long long *typecnt, long long *charcnt) {
    unsigned char magic[4];
    if (!rech_tz_read(f, magic, 4) || memcmp(magic, "TZif", 4) != 0) {
        return 0;
    }
    unsigned char ver;
    if (!rech_tz_read(f, &ver, 1)) {
        return 0;
    }
    *version = (char)ver;
    unsigned char reserved[15];
    if (!rech_tz_read(f, reserved, 15)) {
        return 0;
    }
    if (!rech_tz_read_i32(f, isutcnt) || !rech_tz_read_i32(f, isstdcnt) || !rech_tz_read_i32(f, leapcnt) ||
        !rech_tz_read_i32(f, timecnt) || !rech_tz_read_i32(f, typecnt) || !rech_tz_read_i32(f, charcnt)) {
        return 0;
    }
    return 1;
}

/* rech_tz_data - один блок данных TZif (время_width - 4 у V1, 8 у V2+) -
   раскладка по RFC 8536 section 3.1: времена перехода, индексы типов,
   ttinfo-записи, строки обозначений (не нужны - пропускаются), leap-
   секунды (игнорируются - см. package doc Helpers), std/wall и ut/local
   индикаторы (не нужны для одного смещения). */
static int rech_tz_data(rech_tz_buf *f, int time_width, long long timecnt, long long typecnt, long long charcnt,
                         long long leapcnt, long long isstdcnt, long long isutcnt, rech_tz_t *out) {
    out->n_times = (int)timecnt;
    out->times = timecnt > 0 ? malloc(sizeof(long long) * (size_t)timecnt) : NULL;
    out->type_idx = timecnt > 0 ? malloc(sizeof(int) * (size_t)timecnt) : NULL;
    for (long long i = 0; i < timecnt; i++) {
        unsigned char buf[8];
        if (!rech_tz_read(f, buf, (size_t)time_width)) {
            return 0;
        }
        out->times[i] = rech_tz_be(buf, time_width);
    }
    for (long long i = 0; i < timecnt; i++) {
        unsigned char b;
        if (!rech_tz_read(f, &b, 1)) {
            return 0;
        }
        out->type_idx[i] = b;
    }
    out->n_types = (int)typecnt;
    out->types = typecnt > 0 ? malloc(sizeof(rech_tz_ttinfo) * (size_t)typecnt) : NULL;
    for (long long i = 0; i < typecnt; i++) {
        unsigned char buf[4];
        if (!rech_tz_read(f, buf, 4)) {
            return 0;
        }
        unsigned char isdst, desigidx;
        if (!rech_tz_read(f, &isdst, 1) || !rech_tz_read(f, &desigidx, 1)) {
            return 0;
        }
        out->types[i].offset = rech_tz_be(buf, 4);
        out->types[i].isdst = isdst;
    }
    /* Тип "по умолчанию" для меток раньше первого перехода (или при полном
       отсутствии переходов) - первый НЕ-летний тип, тем же соглашением,
       что glibc/musl, иначе - тип 0. */
    out->default_type = 0;
    for (long long i = 0; i < typecnt; i++) {
        if (!out->types[i].isdst) {
            out->default_type = (int)i;
            break;
        }
    }
    if (!rech_tz_skip(f, charcnt)) {
        return 0;
    }
    if (!rech_tz_skip(f, leapcnt * (time_width + 4))) {
        return 0;
    }
    if (!rech_tz_skip(f, isstdcnt + isutcnt)) {
        return 0;
    }
    return 1;
}

/* rech_tz_parse - разбор одного файла TZif в памяти: для версии 2+
   (единственной, которую реально порождает современный tzdata) целиком
   пропускает 32-битный V1-блок (его длина точно вычисляется из уже
   прочитанных counts; во вшитой базе он пуст - см. tzdata.slimTZif) и
   разбирает ВТОРОЙ, 64-битный блок - те же самые данные, но без
   ограничения 2038 годом (V1 хранит время как int32_t). После него - хвост
   "\n<POSIX TZ-строка>\n" (rech_tz_posix_offset ниже), если он есть. */
static int rech_tz_parse(rech_tz_buf *f, rech_tz_t *out) {
    char version;
    long long isutcnt, isstdcnt, leapcnt, timecnt, typecnt, charcnt;
    if (!rech_tz_header(f, &version, &isutcnt, &isstdcnt, &leapcnt, &timecnt, &typecnt, &charcnt)) {
        return 0;
    }
    if (version == 0) {
        return rech_tz_data(f, 4, timecnt, typecnt, charcnt, leapcnt, isstdcnt, isutcnt, out);
    }

    long long v1_data_len = timecnt * 4 + timecnt + typecnt * 6 + charcnt + leapcnt * 8 + isstdcnt + isutcnt;
    if (!rech_tz_skip(f, v1_data_len)) {
        return 0;
    }
    char version2;
    long long isutcnt2, isstdcnt2, leapcnt2, timecnt2, typecnt2, charcnt2;
    if (!rech_tz_header(f, &version2, &isutcnt2, &isstdcnt2, &leapcnt2, &timecnt2, &typecnt2, &charcnt2)) {
        return 0;
    }
    if (!rech_tz_data(f, 8, timecnt2, typecnt2, charcnt2, leapcnt2, isstdcnt2, isutcnt2, out)) {
        return 0;
    }
    int c = rech_tz_getc(f);
    if (c == '\n') {
        int i = 0;
        while ((c = rech_tz_getc(f)) != -1 && c != '\n' && i < (int)sizeof(out->posix_tz) - 1) {
            out->posix_tz[i++] = (char)c;
        }
        out->posix_tz[i] = '\0';
    }
    return 1;
}

/* rech_tz_load_dir - "<dir>/<zone>" целиком в память и rech_tz_parse. */
static int rech_tz_load_dir(const char *dir, const char *zone, rech_tz_t *out) {
    char path[1024];
    snprintf(path, sizeof(path), "%s/%s", dir, zone);
    FILE *f = fopen(path, "rb");
    if (!f) {
        return 0;
    }
    size_t cap = 4096, n = 0;
    unsigned char *data = malloc(cap);
    size_t got;
    while (data && (got = fread(data + n, 1, cap - n, f)) > 0) {
        n += got;
        if (n == cap) {
            unsigned char *grown = realloc(data, cap * 2);
            if (!grown) {
                free(data);
                data = NULL;
                break;
            }
            data = grown;
            cap *= 2;
        }
    }
    fclose(f);
    if (!data) {
        return 0;
    }
    rech_tz_buf b = {data, n, 0};
    int ok = rech_tz_parse(&b, out);
    free(data);
    return ok;
}

/* rech_tz_load - пояс zone ("Europe/Moscow"): из каталога TZDIR, если
   переменная окружения задана (стандартное имя, которым glibc/musl решают
   тот же вопрос - позволяет подменить базу без пересборки), иначе из
   вшитой в программу копии (двоичный поиск по rech_tz_index). */
static int rech_tz_load(const char *zone, rech_tz_t *out) {
    memset(out, 0, sizeof(*out));
    if (!zone || !zone[0]) {
        return 0;
    }
    for (const char *p = zone; *p; p++) {
        if (*p == '.' && p[1] == '.') {
            return 0;
        }
    }
    const char *dir = getenv("TZDIR");
    if (dir && dir[0]) {
        return rech_tz_load_dir(dir, zone, out);
    }
    int lo = 0, hi = rech_tz_index_len - 1;
    while (lo <= hi) {
        int mid = lo + (hi - lo) / 2;
        int cmp = strcmp(zone, rech_tz_index[mid].name);
        if (cmp == 0) {
            rech_tz_buf b = {rech_tz_blob + rech_tz_index[mid].off, rech_tz_index[mid].len, 0};
            return rech_tz_parse(&b, out);
        }
        if (cmp < 0) {
            hi = mid - 1;
        } else {
            lo = mid + 1;
        }
    }
    return 0;
}

static void rech_tz_free(rech_tz_t *tz) {
    free(tz->times);
    free(tz->type_idx);
    free(tz->types);
}

static long long rech_tz_type_offset(const rech_tz_t *tz, int type_i) {
    if (type_i < 0 || type_i >= tz->n_types) {
        return 0;
    }
    return tz->types[type_i].offset;
}

/* rech_tz_parse_offset - "[+-]hh[:mm[:ss]]" (сырое, POSIX-знаковое число
   часов от Гринвича - см. инверсию знака в rech_tz_posix_offset). */
static long long rech_tz_parse_offset(const char **p) {
    int sign = 1;
    if (**p == '-') {
        sign = -1;
        (*p)++;
    } else if (**p == '+') {
        (*p)++;
    }
    long h = strtol(*p, (char **)p, 10);
    long m = 0, s = 0;
    if (**p == ':') {
        (*p)++;
        m = strtol(*p, (char **)p, 10);
        if (**p == ':') {
            (*p)++;
            s = strtol(*p, (char **)p, 10);
        }
    }
    return (long long)sign * (h * 3600 + m * 60 + s);
}

/* rech_tz_skip_name - имя пояса в POSIX TZ-строке ("MSK", "EST") либо
   заключено в "<...>" (нужно для имён с цифрами/знаками, например "<+03>") -
   само имя нигде дальше не используется, важно только пропустить его. */
static void rech_tz_skip_name(const char **p) {
    if (**p == '<') {
        (*p)++;
        while (**p && **p != '>') {
            (*p)++;
        }
        if (**p == '>') {
            (*p)++;
        }
    } else {
        while ((**p >= 'A' && **p <= 'Z') || (**p >= 'a' && **p <= 'z')) {
            (*p)++;
        }
    }
}

typedef struct {
    int kind; /* 0 = Jn, 1 = n, 2 = Mm.w.d */
    long a, b, c;
} rech_tz_rule_t;

static int rech_tz_parse_rule(const char **p, rech_tz_rule_t *r) {
    if (**p == 'J') {
        (*p)++;
        r->kind = 0;
        r->a = strtol(*p, (char **)p, 10);
        return 1;
    }
    if (**p == 'M') {
        (*p)++;
        r->kind = 2;
        r->a = strtol(*p, (char **)p, 10);
        if (**p != '.') {
            return 0;
        }
        (*p)++;
        r->b = strtol(*p, (char **)p, 10);
        if (**p != '.') {
            return 0;
        }
        (*p)++;
        r->c = strtol(*p, (char **)p, 10);
        return 1;
    }
    r->kind = 1;
    r->a = strtol(*p, (char **)p, 10);
    return 1;
}

/* rech_tz_weekday - день недели (0 = воскресенье) для дня, отстоящего на
   days_since_epoch дней от 1970-01-01 - тот сам был четвергом (4). */
static long rech_tz_weekday(long long days_since_epoch) {
    long long w = (days_since_epoch + 4) % 7;
    if (w < 0) {
        w += 7;
    }
    return (long)w;
}

static int rech_tz_is_leap(long long y) {
    return (y % 4 == 0 && (y % 100 != 0 || y % 400 == 0));
}

/* rech_tz_rule_instant - момент (UTC, секунды с эпохи) правила r в году
   year - ref_offset - смещение, в котором выражено "настенное" time_of_day
   (по умолчанию 02:00:00 - обычное локальное стандартное/летнее время
   непосредственно ПЕРЕД переходом, тот же принцип, что у glibc/musl). */
static long long rech_tz_rule_instant(rech_tz_rule_t *r, long long year, long long ref_offset, long time_of_day) {
    long long month, day;
    if (r->kind == 0) {
        static const int mdays[12] = {31, 28, 31, 30, 31, 30, 31, 31, 30, 31, 30, 31};
        long long remaining = r->a;
        month = 1;
        while (month <= 12 && remaining > mdays[month - 1]) {
            remaining -= mdays[month - 1];
            month++;
        }
        day = remaining;
    } else if (r->kind == 1) {
        int leap = rech_tz_is_leap(year);
        int mdays[12] = {31, leap ? 29 : 28, 31, 30, 31, 30, 31, 31, 30, 31, 30, 31};
        long long remaining = r->a;
        month = 1;
        while (month <= 12 && remaining >= mdays[month - 1]) {
            remaining -= mdays[month - 1];
            month++;
        }
        day = remaining + 1;
    } else {
        month = r->a;
        long long first_day_epoch_days = rech_days_from_civil(year, month, 1);
        long long first_weekday = rech_tz_weekday(first_day_epoch_days);
        long long diff = (r->c - first_weekday + 7) % 7;
        day = 1 + diff + (r->b - 1) * 7;
        if (r->b == 5) {
            int leap = rech_tz_is_leap(year);
            static const int mdays2[12] = {31, 28, 31, 30, 31, 30, 31, 31, 30, 31, 30, 31};
            int dim = mdays2[month - 1];
            if (month == 2 && leap) {
                dim = 29;
            }
            while (day > dim) {
                day -= 7;
            }
        }
    }
    long long days = rech_days_from_civil(year, month, day);
    return days * 86400 + time_of_day - ref_offset;
}

/* rech_tz_posix_offset - хвостовая POSIX TZ-строка (RFC 8536 section 3.3,
   тот же формат, что переменная окружения TZ в POSIX/glibc) - используется
   ТОЛЬКО за пределами последнего явного перехода в файле (rech_tz_offset_at
   ниже), даёт правильный DST для ЛЮБОГО будущего года без переиздания
   вендоренной копии. */
static int rech_tz_posix_offset(const char *tzstr, long long ts, long long *out_offset) {
    if (!tzstr || !tzstr[0]) {
        return 0;
    }
    const char *p = tzstr;
    rech_tz_skip_name(&p);
    if (*p == '\0') {
        return 0;
    }
    /* POSIX инвертирует знак смещения (к западу от Гринвича - "+"). */
    long long std_offset = -rech_tz_parse_offset(&p);
    if (*p == '\0') {
        *out_offset = std_offset;
        return 1;
    }
    rech_tz_skip_name(&p);
    long long dst_offset;
    if (*p == '-' || *p == '+' || (*p >= '0' && *p <= '9')) {
        dst_offset = -rech_tz_parse_offset(&p);
    } else {
        dst_offset = std_offset + 3600;
    }
    if (*p != ',') {
        *out_offset = std_offset;
        return 1;
    }
    p++;
    rech_tz_rule_t start, end;
    if (!rech_tz_parse_rule(&p, &start)) {
        return 0;
    }
    long start_time = 7200;
    if (*p == '/') {
        p++;
        start_time = (long)rech_tz_parse_offset(&p);
    }
    if (*p != ',') {
        return 0;
    }
    p++;
    if (!rech_tz_parse_rule(&p, &end)) {
        return 0;
    }
    long end_time = 7200;
    if (*p == '/') {
        p++;
        end_time = (long)rech_tz_parse_offset(&p);
    }

    long long est_days = (ts + std_offset) / 86400;
    long long est_rem = (ts + std_offset) % 86400;
    if (est_rem < 0) {
        est_rem += 86400;
        est_days--;
    }
    long long y, m, d;
    rech_civil_from_days(est_days, &y, &m, &d);

    long long start_ts = rech_tz_rule_instant(&start, y, std_offset, start_time);
    long long end_ts = rech_tz_rule_instant(&end, y, dst_offset, end_time);

    int in_dst;
    if (start_ts <= end_ts) {
        in_dst = (ts >= start_ts && ts < end_ts);
    } else {
        in_dst = (ts >= start_ts || ts < end_ts);
    }
    *out_offset = in_dst ? dst_offset : std_offset;
    return 1;
}

/* rech_tz_offset_at - смещение от UTC (секунды), действующее в момент ts:
   бинарный поиск последнего перехода <= ts, POSIX-хвост за пределами
   последнего явного перехода, тип по умолчанию раньше первого. */
static long long rech_tz_offset_at(const rech_tz_t *tz, long long ts) {
    if (tz->n_times == 0) {
        long long off;
        if (rech_tz_posix_offset(tz->posix_tz, ts, &off)) {
            return off;
        }
        return rech_tz_type_offset(tz, tz->default_type);
    }
    if (ts < tz->times[0]) {
        return rech_tz_type_offset(tz, tz->default_type);
    }
    if (ts >= tz->times[tz->n_times - 1]) {
        long long off;
        if (rech_tz_posix_offset(tz->posix_tz, ts, &off)) {
            return off;
        }
        return rech_tz_type_offset(tz, tz->type_idx[tz->n_times - 1]);
    }
    int lo = 0, hi = tz->n_times - 1;
    while (lo < hi) {
        int mid = (lo + hi + 1) / 2;
        if (tz->times[mid] <= ts) {
            lo = mid;
        } else {
            hi = mid - 1;
        }
    }
    return rech_tz_type_offset(tz, tz->type_idx[lo]);
}

static @@NDT@@ rech_time_to_date_tz(long long ts, const char *zone) {
    rech_tz_t tz;
    if (!rech_tz_load(zone, &tz)) {
        return @@NDT@@_empty();
    }
    long long offset = rech_tz_offset_at(&tz, ts);
    rech_tz_free(&tz);
    @@DT@@ d = rech_time_to_date(ts + offset);
    return @@NDT@@_wrap(d);
}

/* rech_date_to_time_tz - обратное: гражданские поля d ИНТЕРПРЕТИРУЮТСЯ как
   настенное время в поясе zone. Одна итерация уточнения смещения (см.
   package doc Helpers выше про переход летнее/зимнее) - первая догадка
   (поля как если бы UTC) почти всегда совпадает со смещением В МОМЕНТ уже
   уточнённого результата, кроме самого часа перехода. */
static @@NI@@ rech_date_to_time_tz(@@DT@@ d, const char *zone) {
    rech_tz_t tz;
    if (!rech_tz_load(zone, &tz)) {
        return @@NI@@_empty();
    }
    long long ts0 = rech_date_to_time(d);
    long long offset1 = rech_tz_offset_at(&tz, ts0);
    long long ts1 = ts0 - offset1;
    long long offset2 = rech_tz_offset_at(&tz, ts1);
    rech_tz_free(&tz);
    return @@NI@@_wrap(ts0 - offset2);
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
		"@@NI@@", nullableInt,
	)
	return Blob() + r.Replace(tmpl)
}
