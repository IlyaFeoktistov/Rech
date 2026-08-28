package cbackend

import (
	"fmt"

	"github.com/IlyaFeoktistov/Rech/internal/rtmsg"
)

// nullableCollectionCCode - typedef struct (тег занятости + значение) и три
// вспомогательные функции (обернуть/пусто/развернуть) для одной конкретной
// инстанциации "?Тип" (name - каноническое имя с "?", тем же принципом, что
// listCollectionCCode/stackCollectionCCode для встроенных generic-
// коллекций). Тег занятости - единообразное представление для ЛЮБОГО
// внутреннего типа (в т.ч. Строка/сущность, у которых технически хватило бы
// NULL-указателя) - один путь кодогенерации на все случаи, см. package doc
// ir.KindNullable.
func nullableCollectionCCode(name, elemC string) string {
	structName := mangleType(name)
	wrap := structName + "_wrap"
	empty := structName + "_empty"
	unwrap := structName + "_unwrap"
	coalesce := structName + "_coalesce"
	coalesceN := structName + "_coalesce_n"
	nullMsg := escapeCString(rtmsg.NullAccess)

	return fmt.Sprintf(`typedef struct {
    int has;
    %[1]s value;
} %[2]s;

static %[2]s %[3]s(%[1]s value) {
    %[2]s r;
    r.has = 1;
    r.value = value;
    return r;
}

static %[2]s %[4]s(void) {
    %[2]s r = {0};
    return r;
}

static %[1]s %[5]s(%[2]s v) {
    if (!v.has) {
        rech_panic("%[6]s");
    }
    return v.value;
}

// %[7]s/%[8]s - "??" (null-coalescing): аргументы-функции, а не макрос/
// тернарник над самим выражением, чтобы v/fallback вычислялись РОВНО один
// раз (v.has и v.value иначе читались бы из ДВУХ разных вычислений
// произвольного выражения слева от "??" - лишний побочный эффект/вызов при
// каждом употреблении).
static %[1]s %[7]s(%[2]s v, %[1]s fallback) {
    return v.has ? v.value : fallback;
}

static %[2]s %[8]s(%[2]s v, %[2]s fallback) {
    return v.has ? v : fallback;
}

`, elemC, structName, wrap, empty, unwrap, nullMsg, coalesce, coalesceN)
}
