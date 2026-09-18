package crypto

import (
	"fmt"

	"github.com/IlyaFeoktistov/Rech/internal/rtmsg"
)

// RandomHelpers - "случайное"/"случайное_целое" (internal/builtins.All) -
// криптографически стойкий источник (arc4random/arc4random_uniform из
// <stdlib.h> на POSIX/BSD/macOS - glibc >= 2.36 и musl уже несут их сами,
// без ручного srand(); rand_s из MSVCRT/UCRT на Windows, требует
// "#define _CRT_RAND_S" ДО ЛЮБОГО системного заголовка, не только
// "#include <stdlib.h>" - см. ir_generator.go, generateSource), не
// rand()/math.rand-аналог - нужен в первую очередь для
// непредсказуемых токенов сессий (см. examples/проекты/http_сервер).
//
// rech_random_int для диапазона шире 32 бит (Целое - 64-битный) объединяет
// два вызова генератора в одно 64-битное значение и берёт остаток от
// деления на размер диапазона - небольшое смещение распределения у самых
// широких диапазонов (обычная плата за простоту взамен честного rejection
// sampling), не проблема для типичного использования (токены, кости,
// выбор индекса).
var RandomHelpers = fmt.Sprintf(`static double rech_random(void) {
#ifdef _WIN32
    unsigned int r;
    rand_s(&r);
    return (double)r / 4294967296.0;
#else
    return (double)arc4random() / 4294967296.0;
#endif
}

static long long rech_random_int(long long mn, long long mx) {
    if (mx < mn) {
        rech_panic("%[1]s");
    }
    unsigned long long range = (unsigned long long)(mx - mn) + 1ULL;
#ifdef _WIN32
    unsigned int lo, hi;
    rand_s(&lo);
    rand_s(&hi);
    unsigned long long r = ((unsigned long long)hi << 32) | lo;
#else
    unsigned long long r = ((unsigned long long)arc4random() << 32) | arc4random();
#endif
    return mn + (long long)(r %% range);
}
`, rtmsg.RandomIntInvalidRange)
