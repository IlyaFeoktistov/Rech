package cbackend

import (
	"fmt"

	"github.com/IlyaFeoktistov/Rech/internal/rtmsg"
)

// regexGroupsType/regexResultType - см. internal/semantic/regex.go - те же
// имена generic-инстанциаций, принудительно зарегистрированных для
// "регулярное".
const (
	regexGroupsType = "Список<Строка>"
	regexResultType = "Список<Совпадение>"
)

// regexHelpers - "регулярное(строка, шаблон): Список<Совпадение>" - движок
// POSIX regex.h/regcomp/regexec (glibc, без внешней линковки - см. пакетную
// документацию internal/interp/regex.go для Go regexp/RE2, используемого
// интерпретатором). Синтаксис POSIX ERE (REG_EXTENDED) - НЕ идентичен RE2
// один в один:
//   - "\d"/"\D" - GNU-расширение glibc НЕ включает их (только "\w"/"\W"/
//     "\s"/"\S"/"\b"/"\B") - rech_regex_translate_pattern переводит их в
//     "[0-9]"/"[^0-9]" САМ, до regcomp, чтобы типичный шаблон с "\d" вёл
//     себя одинаково в обоих backend'ах.
//   - "\w"/"\W" - у glibc это GNU-расширение, locale-осознанное (под
//     setlocale(LC_ALL, "C.UTF-8") видит кириллицу как "словесный символ"),
//     а у Go regexp/RE2 (интерпретатор) "\w" ВСЕГДА ASCII-only ([0-9A-Za-z_],
//     без кириллицы вовсе, независимо от локали ОС - RE2 не завязан на неё).
//     Чтобы "\w" вёл себя ОДИНАКОВО в обоих backend'ах для кириллического
//     текста (а не только латиницы), rech_regex_translate_pattern переводит
//     "\w"/"\W" в "[[:alnum:]_]"/"[^[:alnum:]_]" (locale-осознанные POSIX-
//     классы) - Go-сторона (internal/interp/regex.go,
//     regexTranslatePattern) переводит их же в "[\p{L}\p{N}_]"/
//     "[^\p{L}\p{N}_]" (Unicode-классы RE2) - оба перевода семантически
//     "буква/цифра/подчёркивание с учётом Unicode", не два случайно похожих
//     решения.
//   - Не поддерживаются (и не эмулируются): non-capturing группы "(?:...)",
//     lookahead/lookbehind - это уже настоящий PCRE/RE2-синтаксис, которого
//     в POSIX ERE нет вовсе (regcomp вернёт ошибку компиляции шаблона,
//     rtmsg.RegexInvalidPatternFormat) - см. docs/SPEC.md, раздел
//     "регулярное", список известных ограничений.
//
// rech_regex_find_all вызывает setlocale(LC_ALL, "C.UTF-8") при КАЖДОМ
// вызове (дёшево, идемпотентно) - без явной локали glibc regex по умолчанию
// живёт в "C" (чистый ASCII): "\w"/классы вида "[а-я]" не распознают
// кириллицу вообще (проверено вручную - "прив\w+" не находит "привет" без
// этого вызова) - "C.UTF-8" гарантированно доступна на любом glibc >= 2.35
// БЕЗ установленных locale-данных (в отличие от "ru_RU.UTF-8"), см. её
// package doc в internal/interp/regex.go для того же принципа на стороне
// Go (там просто не нужен - regexp/RE2 не завязан на локаль ОС вовсе).
var regexHelpers = fmt.Sprintf(`static char *rech_regex_translate_pattern(const char *pattern) {
    size_t len = strlen(pattern);
    char *out = malloc(len * 7 + 1);
    size_t oi = 0;
    for (size_t i = 0; i < len; i++) {
        if (pattern[i] == '\\' && i + 1 < len && pattern[i + 1] == 'd') {
            memcpy(out + oi, "[0-9]", 5);
            oi += 5;
            i++;
        } else if (pattern[i] == '\\' && i + 1 < len && pattern[i + 1] == 'D') {
            memcpy(out + oi, "[^0-9]", 6);
            oi += 6;
            i++;
        } else if (pattern[i] == '\\' && i + 1 < len && pattern[i + 1] == 'w') {
            memcpy(out + oi, "[[:alnum:]_]", 12);
            oi += 12;
            i++;
        } else if (pattern[i] == '\\' && i + 1 < len && pattern[i + 1] == 'W') {
            memcpy(out + oi, "[^[:alnum:]_]", 13);
            oi += 13;
            i++;
        } else if (pattern[i] == '\\' && i + 1 < len) {
            out[oi++] = pattern[i];
            out[oi++] = pattern[i + 1];
            i++;
        } else {
            out[oi++] = pattern[i];
        }
    }
    out[oi] = '\0';
    return out;
}

static long long rech_regex_codepoint_index(const char *s, long long byte_offset) {
    long long idx = 0;
    for (long long i = 0; i < byte_offset; i++) {
        if ((((const unsigned char *)s)[i] & 0xC0) != 0x80) {
            idx++;
        }
    }
    return idx;
}

static char *rech_regex_substr(const char *s, regoff_t so, regoff_t eo) {
    size_t len = (size_t)(eo - so);
    char *out = malloc(len + 1);
    memcpy(out, s + so, len);
    out[len] = '\0';
    return out;
}

static %[1]s *rech_regex_find_all(const char *s, const char *pattern) {
    setlocale(LC_ALL, "C.UTF-8");
    char *translated = rech_regex_translate_pattern(pattern);
    regex_t re;
    int rc = regcomp(&re, translated, REG_EXTENDED);
    if (rc != 0) {
        char errbuf[256];
        regerror(rc, &re, errbuf, sizeof(errbuf));
        char msg[512];
        snprintf(msg, sizeof(msg), "%[7]s", pattern, errbuf);
        rech_panic(msg);
    }
    %[1]s *result = GC_MALLOC(sizeof(%[1]s));
    size_t nmatch = re.re_nsub + 1;
    regmatch_t *pmatch = malloc(sizeof(regmatch_t) * nmatch);
    const char *cursor = s;
    int notbol = 0;
    for (;;) {
        int mrc = regexec(&re, cursor, nmatch, pmatch, notbol ? REG_NOTBOL : 0);
        if (mrc != 0) {
            break;
        }
        %[2]s *groups = GC_MALLOC(sizeof(%[2]s));
        for (size_t g = 1; g < nmatch; g++) {
            char *piece;
            if (pmatch[g].rm_so < 0) {
                piece = "";
            } else {
                piece = rech_regex_substr(cursor, pmatch[g].rm_so, pmatch[g].rm_eo);
            }
            %[3]s(groups, piece);
        }

        %[4]s item;
        item.%[5]s = rech_regex_substr(cursor, pmatch[0].rm_so, pmatch[0].rm_eo);
        item.%[6]s = rech_regex_codepoint_index(s, (long long)(cursor - s) + pmatch[0].rm_so);
        item.%[8]s = groups;
        %[9]s(result, item);

        if (pmatch[0].rm_eo == pmatch[0].rm_so) {
            if (cursor[pmatch[0].rm_eo] == '\0') {
                break;
            }
            cursor += pmatch[0].rm_eo + 1;
        } else {
            cursor += pmatch[0].rm_eo;
        }
        notbol = 1;
    }
    free(pmatch);
    regfree(&re);
    return result;
}
`, mangleType(regexResultType), mangleType(regexGroupsType), mangleMethod(regexGroupsType, "добавить"),
	mangleType("Совпадение"), mangleField("текст"), mangleField("начало"),
	escapeCString(rtmsg.RegexInvalidPatternTemplate), mangleField("группы"), mangleMethod(regexResultType, "добавить"))
