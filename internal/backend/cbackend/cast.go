package cbackend

import (
	"fmt"

	"github.com/IlyaFeoktistov/Rech/internal/ir"
)

// generateIRCastExpr - "(Тип) значение" (internal/semantic уже проверил
// допустимость пары исходный/целевой тип, см. internal/semantic/cast.go,
// allowedCasts). Приведения с участием "Строка" (разбор/форматирование) -
// через рантайм-хелперы (castHelpers, ctx.usesCast); всё остальное (числа и
// Символ/Байт между собой) - обычный C-каст, C сам умеет округлять/
// усекать/расширять между long long/float/double/int32_t/unsigned char.
func generateIRCastExpr(ctx *irGenCtx, e *ir.CastExpr) (string, valueType, error) {
	code, from, err := generateIRExpr(ctx, e.Value)
	if err != nil {
		return "", typeUnknown, err
	}
	to := fromIRType(e.To)

	if to == typeAny {
		// Боксирование - см. internal/backend/cbackend/reflect.go,
		// reflectBoxExprCode. Структура GC-копируется в отдельный блок
		// памяти (rech_reflect_box_<Ent>) - боксированное значение живёт
		// независимо от исходного выражения дальше, тот же принцип, что и
		// internal/interp.cloneValue при боксировании там.
		return reflectBoxExprCode(ctx, ir.TypeOf(e.Value), code), typeAny, nil
	}

	if from == typeString && to != typeString {
		ctx.usesCast = true
		ctx.usesPanic = true // разбор строки может провалиться
		switch {
		case to == typeChar:
			return fmt.Sprintf("rech_cast_str_to_char(%s)", code), typeChar, nil
		case to == typeFloat || to == typeFloat32:
			return fmt.Sprintf("((%s)rech_cast_str_to_float(%s, \"%s\"))", to.cType(ctx), code, escapeCString(e.To.String())), to, nil
		case isUnsignedFixedWidth(to):
			return fmt.Sprintf("((%s)rech_cast_str_to_uint(%s, \"%s\"))", to.cType(ctx), code, escapeCString(e.To.String())), to, nil
		default: // typeInt, typeByte, знаковые Целое8/16/32/64
			return fmt.Sprintf("((%s)rech_cast_str_to_int(%s, \"%s\"))", to.cType(ctx), code, escapeCString(e.To.String())), to, nil
		}
	}
	if to == typeString && from != typeString {
		ctx.usesCast = true
		if from == typeChar {
			return fmt.Sprintf("rech_cast_char_to_str(%s)", code), typeString, nil
		}
		if from == typeFloat || from == typeFloat32 {
			return fmt.Sprintf("rech_cast_float_to_str((double)%s)", code), typeString, nil
		}
		if isUnsignedFixedWidth(from) {
			// Не (long long) - Натуральное64 может нести значение выше
			// INT64_MAX, которое signed-каст отобразил бы как отрицательное.
			return fmt.Sprintf("rech_cast_uint_to_str((unsigned long long)%s)", code), typeString, nil
		}
		return fmt.Sprintf("rech_cast_int_to_str((long long)%s)", code), typeString, nil
	}

	// Прямое числовое приведение (в т. ч. Символ<->Целое/Байт) - обычный
	// C-каст, без рантайм-хелпера.
	return fmt.Sprintf("((%s)(%s))", to.cType(ctx), code), to, nil
}

// isUnsignedFixedWidth - vt один из Натуральное/Натуральное8/16/32/64.
// Отдельная беззнаковая ветка при касте со/в "Строка" нужна, потому что
// Натуральное64 может нести значение выше INT64_MAX - через long long/
// strtoll оно ушло бы отрицательным или обрезалось бы неверно.
func isUnsignedFixedWidth(vt valueType) bool {
	switch vt {
	case typeUint, typeUint8, typeUint16, typeUint32, typeUint64:
		return true
	default:
		return false
	}
}

// castHelpers - рантайм-хелперы для явных кастов, вовлекающих "Строка"
// (разбор строки в число/символ, форматирование числа/символа в строку) -
// эмитятся только если ctx.usesCast (см. GenerateFromIR). Текст сообщений
// об ошибке разбора должен совпадать байт-в-байт с internal/rtmsg
// (CastStrToIntFailedFormat/CastStrToFloatFailedFormat/
// CastStrToCharFailedFormat), которые использует internal/interp для тех
// же кастов. rech_cast_alloc - выделение результирующей строки (аналог
// rech_dup из httpHelpers, но не завязан на usesHTTP - кастам он нужен
// независимо от того, используется ли HTTP-клиент).
const castHelpers = `static char *rech_cast_alloc(const char *s, size_t len) {
    char *r = malloc(len + 1);
    memcpy(r, s, len);
    r[len] = '\0';
    return r;
}

static long long rech_cast_str_to_int(const char *s, const char *type_name) {
    char *end;
    long long v = strtoll(s, &end, 10);
    if (end == s || *end != '\0') {
        char msg[256];
        snprintf(msg, sizeof(msg), "каст (%s): \"%s\" не является целым числом", type_name, s);
        rech_panic(msg);
    }
    return v;
}

static unsigned long long rech_cast_str_to_uint(const char *s, const char *type_name) {
    char *end;
    unsigned long long v = strtoull(s, &end, 10);
    if (end == s || *end != '\0') {
        char msg[256];
        snprintf(msg, sizeof(msg), "каст (%s): \"%s\" не является целым числом", type_name, s);
        rech_panic(msg);
    }
    return v;
}

static double rech_cast_str_to_float(const char *s, const char *type_name) {
    char *end;
    double v = strtod(s, &end);
    if (end == s || *end != '\0') {
        char msg[256];
        snprintf(msg, sizeof(msg), "каст (%s): \"%s\" не является числом", type_name, s);
        rech_panic(msg);
    }
    return v;
}

static char *rech_cast_uint_to_str(unsigned long long v) {
    char buf[32];
    snprintf(buf, sizeof(buf), "%llu", v);
    return rech_cast_alloc(buf, strlen(buf));
}

static char *rech_cast_int_to_str(long long v) {
    char buf[32];
    snprintf(buf, sizeof(buf), "%lld", v);
    return rech_cast_alloc(buf, strlen(buf));
}

static char *rech_cast_float_to_str(double v) {
    char buf[64];
    snprintf(buf, sizeof(buf), "%g", v);
    return rech_cast_alloc(buf, strlen(buf));
}

/* rech_cast_char_to_str - Символ (кодовая точка Unicode) -> Строка (UTF-8) */
static char *rech_cast_char_to_str(int32_t c) {
    unsigned char buf[4];
    size_t len;
    if (c < 0x80) {
        buf[0] = (unsigned char)c;
        len = 1;
    } else if (c < 0x800) {
        buf[0] = (unsigned char)(0xC0 | (c >> 6));
        buf[1] = (unsigned char)(0x80 | (c & 0x3F));
        len = 2;
    } else if (c < 0x10000) {
        buf[0] = (unsigned char)(0xE0 | (c >> 12));
        buf[1] = (unsigned char)(0x80 | ((c >> 6) & 0x3F));
        buf[2] = (unsigned char)(0x80 | (c & 0x3F));
        len = 3;
    } else {
        buf[0] = (unsigned char)(0xF0 | (c >> 18));
        buf[1] = (unsigned char)(0x80 | ((c >> 12) & 0x3F));
        buf[2] = (unsigned char)(0x80 | ((c >> 6) & 0x3F));
        buf[3] = (unsigned char)(0x80 | (c & 0x3F));
        len = 4;
    }
    return rech_cast_alloc((const char *)buf, len);
}

/* rech_cast_str_to_char - Строка -> Символ: декодирует ровно одну кодовую
   точку UTF-8, паника, если байтов больше или меньше одного символа. */
static int32_t rech_cast_str_to_char(const char *s) {
    unsigned char c0 = (unsigned char)s[0];
    int32_t cp;
    int len;
    if (c0 == 0) {
        goto fail;
    } else if ((c0 & 0x80) == 0) {
        cp = c0;
        len = 1;
    } else if ((c0 & 0xE0) == 0xC0) {
        cp = c0 & 0x1F;
        len = 2;
    } else if ((c0 & 0xF0) == 0xE0) {
        cp = c0 & 0x0F;
        len = 3;
    } else if ((c0 & 0xF8) == 0xF0) {
        cp = c0 & 0x07;
        len = 4;
    } else {
        goto fail;
    }
    for (int i = 1; i < len; i++) {
        unsigned char ci = (unsigned char)s[i];
        if ((ci & 0xC0) != 0x80) {
            goto fail;
        }
        cp = (cp << 6) | (ci & 0x3F);
    }
    if (s[len] != '\0') {
        goto fail;
    }
    return cp;
fail:
    {
        char msg[256];
        snprintf(msg, sizeof(msg), "каст (Символ): строка \"%s\" не состоит ровно из одного символа", s);
        rech_panic(msg);
    }
    return 0;
}
`
