package cbackend

import (
	"fmt"
	"strings"

	"github.com/IlyaFeoktistov/Rech/internal/rtmsg"
)

// listCollectionCCode строит C-текст (typedef struct + функции) для
// конкретной инстанциации "Список<T>" по одному только имени и C-типу
// элемента - без зависимости от кодогенерации, чтобы одна и та же
// реализация (growable array) обслуживала любой путь кодогенерации, у
// которого сигнатуры методов уже целиком в самих IR-узлах (MethodCallExpr),
// а этому коду для кодогенерации нужен только сам C-текст.
//
// this_->data растёт через GC_REALLOC (Boehm GC), не голый realloc - T
// может быть указателем на GC_MALLOC'енную сущность (T элемент - сущность/
// класс, см. internal/semantic.resolveGenericTypeName), а обычный malloc/
// realloc-буфер Boehm GC не сканирует на предмет исходящих указателей -
// единственная живая ссылка на элемент только из такого буфера означала бы
// его преждевременную сборку мусора. GC_REALLOC - honest drop-in замена
// (тот же контракт, что у realloc: может переместить блок, копирует
// содержимое, старый блок сам станет мусором) - применяется КО ВСЕМ
// элементам без разбора по T (не только сущностям) ради одного простого
// правила вместо двух путей кодогенерации.
func listCollectionCCode(name, elemC string) string {
	structName := mangleType(name)
	add := mangleMethod(name, "добавить")
	get := mangleMethod(name, "получить")
	set := mangleMethod(name, "установить")
	length := mangleMethod(name, "длина")
	empty := mangleMethod(name, "пусто")
	outOfRange := escapeCString(rtmsg.IndexOutOfRange(name))
	addr := structName + "_addr"

	return fmt.Sprintf(`struct %[2]s {
    %[1]s *data;
    long long len;
    long long cap;
};

static void %[3]s(%[2]s *this_, %[1]s value) {
    if (this_->len == this_->cap) {
        this_->cap = this_->cap == 0 ? 4 : this_->cap * 2;
        this_->data = GC_REALLOC(this_->data, sizeof(%[1]s) * this_->cap);
    }
    this_->data[this_->len] = value;
    this_->len++;
}

static %[1]s %[4]s(%[2]s *this_, long long index) {
    if (index < 0 || index >= this_->len) {
        rech_panic("%[7]s");
    }
    return this_->data[index];
}

// %[9]s - адрес РЕАЛЬНОЙ ячейки хранения элемента (не копия значения) -
// для "&список[i]" (docs/SPEC.md, "Указатели") - паникует при выходе за
// границу, тем же принципом, что %[4]s выше. Теряет силу при следующем
// %[3]s в тот же список, реаллоцирующем this_->data - ответственность
// программиста, задокументированное ограничение, не проверяется рантаймом.
static %[1]s *%[9]s(%[2]s *this_, long long index) {
    if (index < 0 || index >= this_->len) {
        rech_panic("%[7]s");
    }
    return &this_->data[index];
}

static void %[5]s(%[2]s *this_, long long index, %[1]s value) {
    if (index < 0 || index >= this_->len) {
        rech_panic("%[7]s");
    }
    this_->data[index] = value;
}

static long long %[6]s(%[2]s *this_) {
    return this_->len;
}

static bool %[8]s(%[2]s *this_) {
    return this_->len == 0;
}

`, elemC, structName, add, get, set, length, outOfRange, empty, addr)
}

// stackCollectionCCode - как listCollectionCCode, для "Стопка<T>" (LIFO).
func stackCollectionCCode(name, elemC string) string {
	structName := mangleType(name)
	push := mangleMethod(name, "положить")
	pop := mangleMethod(name, "снять")
	top := mangleMethod(name, "верх")
	length := mangleMethod(name, "длина")
	empty := mangleMethod(name, "пусто")
	onEmpty := escapeCString(rtmsg.EmptyStackOp(name))

	return fmt.Sprintf(`struct %[2]s {
    %[1]s *data;
    long long len;
    long long cap;
};

static void %[3]s(%[2]s *this_, %[1]s value) {
    if (this_->len == this_->cap) {
        this_->cap = this_->cap == 0 ? 4 : this_->cap * 2;
        this_->data = GC_REALLOC(this_->data, sizeof(%[1]s) * this_->cap);
    }
    this_->data[this_->len] = value;
    this_->len++;
}

static %[1]s %[4]s(%[2]s *this_) {
    if (this_->len == 0) {
        rech_panic("%[7]s");
    }
    this_->len--;
    return this_->data[this_->len];
}

static %[1]s %[5]s(%[2]s *this_) {
    if (this_->len == 0) {
        rech_panic("%[7]s");
    }
    return this_->data[this_->len - 1];
}

static long long %[6]s(%[2]s *this_) {
    return this_->len;
}

static bool %[8]s(%[2]s *this_) {
    return this_->len == 0;
}

`, elemC, structName, push, pop, top, length, onEmpty, empty)
}

// queueCollectionCCode - как listCollectionCCode, для "Очередь<T>" (FIFO).
func queueCollectionCCode(name, elemC string) string {
	structName := mangleType(name)
	add := mangleMethod(name, "добавить")
	remove := mangleMethod(name, "извлечь")
	first := mangleMethod(name, "первый")
	length := mangleMethod(name, "длина")
	empty := mangleMethod(name, "пусто")
	onEmpty := escapeCString(rtmsg.EmptyQueueOp(name))

	return fmt.Sprintf(`struct %[2]s {
    %[1]s *data;
    long long len;
    long long cap;
};

static void %[3]s(%[2]s *this_, %[1]s value) {
    if (this_->len == this_->cap) {
        this_->cap = this_->cap == 0 ? 4 : this_->cap * 2;
        this_->data = GC_REALLOC(this_->data, sizeof(%[1]s) * this_->cap);
    }
    this_->data[this_->len] = value;
    this_->len++;
}

static %[1]s %[4]s(%[2]s *this_) {
    if (this_->len == 0) {
        rech_panic("%[7]s");
    }
    %[1]s value = this_->data[0];
    for (long long i = 1; i < this_->len; i++) {
        this_->data[i - 1] = this_->data[i];
    }
    this_->len--;
    return value;
}

static %[1]s %[5]s(%[2]s *this_) {
    if (this_->len == 0) {
        rech_panic("%[7]s");
    }
    return this_->data[0];
}

static long long %[6]s(%[2]s *this_) {
    return this_->len;
}

static bool %[8]s(%[2]s *this_) {
    return this_->len == 0;
}

`, elemC, structName, add, remove, first, length, onEmpty, empty)
}

// dictCollectionCCode строит C-текст для "Словарь<К, В>" - открытая
// адресация с линейным пробированием и надгробиями (state: 0 - пусто, 1 -
// занято, 2 - удалено) - тот же принцип "растущий буфер через GC_MALLOC"
// (см. package doc listCollectionCCode - тот же риск преждевременной
// сборки мусора у entity-значения, та же причина не использовать голый
// calloc/free), что и у списочных коллекций выше, но с рехешированием
// вместо простого удвоения (перестройка слотов при росте - индекс зависит
// от cap; старый блок слотов сам станет мусором, не free() вручную).
// isStringKey переключает hash/eq на strcmp/FNV-1a над байтами C-строки
// вместо прямого сравнения/умножения значения - единственное отличие
// между строковым и целочисленным (Целое/Байт/Символ/Логическое) ключом,
// оба уже приведены к общему для printf-независимого кода C-типу keyC
// вызывающей стороной (ir_generator.go).
func dictCollectionCCode(name, keyC, valC string, isStringKey bool) string {
	structName := mangleType(name)
	slotType := structName + "_slot"
	hashFn := structName + "_hash"
	eqFn := structName + "_eq"
	resizeFn := structName + "_resize"
	findFn := structName + "_find"
	set := mangleMethod(name, "установить")
	get := mangleMethod(name, "получить")
	has := mangleMethod(name, "содержит")
	del := mangleMethod(name, "удалить")
	length := mangleMethod(name, "длина")
	empty := mangleMethod(name, "пусто")
	notFound := escapeCString(rtmsg.DictKeyNotFound(name))

	var sb strings.Builder
	fmt.Fprintf(&sb, `typedef struct {
    int state;
    %[1]s key;
    %[2]s value;
} %[3]s;

struct %[4]s {
    %[3]s *slots;
    long long cap;
    long long count;
    long long used;
};

`, keyC, valC, slotType, structName)

	if isStringKey {
		fmt.Fprintf(&sb, `static unsigned long long %[1]s(%[2]s k) {
    unsigned long long h = 1469598103934665603ULL;
    for (const unsigned char *p = (const unsigned char *)k; *p; p++) {
        h ^= (unsigned long long)*p;
        h *= 1099511628211ULL;
    }
    return h;
}

static bool %[3]s(%[2]s a, %[2]s b) {
    return strcmp(a, b) == 0;
}

`, hashFn, keyC, eqFn)
	} else {
		fmt.Fprintf(&sb, `static unsigned long long %[1]s(%[2]s k) {
    return (unsigned long long)k * 11400714819323198485ULL;
}

static bool %[3]s(%[2]s a, %[2]s b) {
    return a == b;
}

`, hashFn, keyC, eqFn)
	}

	fmt.Fprintf(&sb, `static void %[1]s(%[2]s *this_, long long newcap) {
    %[3]s *old = this_->slots;
    long long oldcap = this_->cap;
    this_->slots = GC_MALLOC((size_t)newcap * sizeof(%[3]s));
    this_->cap = newcap;
    this_->count = 0;
    this_->used = 0;
    for (long long i = 0; i < oldcap; i++) {
        if (old[i].state == 1) {
            long long idx = (long long)(%[4]s(old[i].key) %% (unsigned long long)newcap);
            while (this_->slots[idx].state == 1) {
                idx = (idx + 1) %% newcap;
            }
            this_->slots[idx].state = 1;
            this_->slots[idx].key = old[i].key;
            this_->slots[idx].value = old[i].value;
            this_->count++;
            this_->used++;
        }
    }
}

static long long %[5]s(%[2]s *this_, %[6]s key) {
    if (this_->cap == 0) {
        return -1;
    }
    long long idx = (long long)(%[4]s(key) %% (unsigned long long)this_->cap);
    for (long long probes = 0; probes < this_->cap; probes++) {
        if (this_->slots[idx].state == 0) {
            return -1;
        }
        if (this_->slots[idx].state == 1 && %[7]s(this_->slots[idx].key, key)) {
            return idx;
        }
        idx = (idx + 1) %% this_->cap;
    }
    return -1;
}

static void %[8]s(%[2]s *this_, %[6]s key, %[9]s value) {
    if (this_->cap == 0 || this_->used * 10 >= this_->cap * 7) {
        %[1]s(this_, this_->cap == 0 ? 8 : this_->cap * 2);
    }
    long long idx = (long long)(%[4]s(key) %% (unsigned long long)this_->cap);
    long long firstDeleted = -1;
    for (;;) {
        if (this_->slots[idx].state == 0) {
            long long target = firstDeleted != -1 ? firstDeleted : idx;
            if (firstDeleted == -1) {
                this_->used++;
            }
            this_->slots[target].state = 1;
            this_->slots[target].key = key;
            this_->slots[target].value = value;
            this_->count++;
            return;
        }
        if (this_->slots[idx].state == 1 && %[7]s(this_->slots[idx].key, key)) {
            this_->slots[idx].value = value;
            return;
        }
        if (this_->slots[idx].state == 2 && firstDeleted == -1) {
            firstDeleted = idx;
        }
        idx = (idx + 1) %% this_->cap;
    }
}

static %[9]s %[10]s(%[2]s *this_, %[6]s key) {
    long long idx = %[5]s(this_, key);
    if (idx < 0) {
        rech_panic("%[11]s");
    }
    return this_->slots[idx].value;
}

static bool %[12]s(%[2]s *this_, %[6]s key) {
    return %[5]s(this_, key) >= 0;
}

static void %[13]s(%[2]s *this_, %[6]s key) {
    long long idx = %[5]s(this_, key);
    if (idx >= 0) {
        this_->slots[idx].state = 2;
        this_->count--;
    }
}

static long long %[14]s(%[2]s *this_) {
    return this_->count;
}

static bool %[15]s(%[2]s *this_) {
    return this_->count == 0;
}

`, resizeFn, structName, slotType, hashFn, findFn, keyC, eqFn, set, valC, get, notFound, has, del, length, empty)

	return sb.String()
}

// genericCtorCCode строит C-текст конструктора "новый Список<T>()"/
// "новый Словарь<К, В>()" и т. п. - GC_MALLOC зануляет память сам
// (см. gcAllocStmts, тот же принцип, что и у обычных ссылочных сущностей) -
// zero-значение (cap/count/used/slots-указатель на NULL) уже и есть
// пустая коллекция, отдельно заполнять поля не нужно. Generic-инстанциации
// стали ссылочными (ctx.entityIsRef) вместе с этим конструктором - раньше
// "новый Список<T>()" был C-литералом структуры по значению
// ("(Тип){0}"), теперь - настоящий указатель, тем же представлением, что
// и у обычных сущностей, и тем же, что использует interp (Go-карта/срез
// внутри значения - объект, не копия).
func genericCtorCCode(name string) string {
	structName := mangleType(name)
	ctor := mangleCtor(name)
	return fmt.Sprintf(`static %[1]s *%[2]s(void) {
    return GC_MALLOC(sizeof(%[1]s));
}

`, structName, ctor)
}
