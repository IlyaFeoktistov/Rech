package collections

import (
	"fmt"
	"strings"
)

// ListCollectionCode строит C-текст (typedef struct + функции) для
// конкретной инстанциации "Список<T>" по одному только C-типу элемента и
// уже манглированным C-именам - без зависимости от кодогенерации, чтобы
// одна и та же реализация (growable array) обслуживала любой путь
// кодогенерации, у которого сигнатуры методов уже целиком в самих
// IR-узлах (MethodCallExpr), а этому коду для кодогенерации нужен только
// сам C-текст. structC/addMethod/getMethod/setMethod/removeMethod/
// lengthMethod/emptyMethod/firstMethod/lastMethod/outOfRangeMsg - уже
// вычислены вызывающей стороной (mangleType/mangleMethod/escapeCString -
// внутренняя, неэкспортированная инфраструктура cbackend, этот пакет от
// неё не зависит).
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
func ListCollectionCode(elemC, structC, addMethod, getMethod, setMethod, removeMethod,
	lengthMethod, emptyMethod, firstMethod, lastMethod, outOfRangeMsg string) string {
	addr := structC + "_addr"

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

// %[10]s - сдвигает хвост (индексы после index) на одну позицию влево
// поэлементно (обычным присваиванием, не memmove - тот же приём, что и у
// "извлечь" очереди ниже, QueueCollectionCode: не добавляет новую
// зависимость от <string.h> одним только использованием Список) и
// возвращает удалённый элемент - та же семантика, что list.pop(i) в
// Python/ArrayList.remove(int) в Java. O(n) от длины хвоста, как и любое
// удаление из растущего массива - документированная плата за O(1) %[9]s/
// %[4]s остальных элементов по индексу.
static %[1]s %[10]s(%[2]s *this_, long long index) {
    if (index < 0 || index >= this_->len) {
        rech_panic("%[7]s");
    }
    %[1]s removed = this_->data[index];
    for (long long i = index; i < this_->len - 1; i++) {
        this_->data[i] = this_->data[i + 1];
    }
    this_->len--;
    return removed;
}

static long long %[6]s(%[2]s *this_) {
    return this_->len;
}

static bool %[8]s(%[2]s *this_) {
    return this_->len == 0;
}

// %[11]s/%[12]s - "первый()"/"последний()" - тот же элемент, что и %[4]s(0)/
// %[4]s(длина-1), переиспользует её же проверку границ/паническое
// сообщение (пустой список - тот же "выход за границы", не отдельная
// панику).
static %[1]s %[11]s(%[2]s *this_) {
    return %[4]s(this_, 0);
}

static %[1]s %[12]s(%[2]s *this_) {
    return %[4]s(this_, this_->len - 1);
}

`, elemC, structC, addMethod, getMethod, setMethod, lengthMethod, outOfRangeMsg, emptyMethod, addr, removeMethod, firstMethod, lastMethod)
}

// StackCollectionCode - как ListCollectionCode, для "Стопка<T>" (LIFO).
func StackCollectionCode(elemC, structC, pushMethod, popMethod, topMethod, lengthMethod, emptyMethod, onEmptyMsg string) string {
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

`, elemC, structC, pushMethod, popMethod, topMethod, lengthMethod, onEmptyMsg, emptyMethod)
}

// QueueCollectionCode - как ListCollectionCode, для "Очередь<T>" (FIFO).
func QueueCollectionCode(elemC, structC, addMethod, removeMethod, firstMethod, lengthMethod, emptyMethod, onEmptyMsg string) string {
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

`, elemC, structC, addMethod, removeMethod, firstMethod, lengthMethod, onEmptyMsg, emptyMethod)
}

// DictCollectionCode строит C-текст для "Словарь<К, В>" - компактная
// хеш-таблица: записи (slots) лежат плотным массивом в порядке вставки
// (state: 1 - занято, 2 - удалено), отдельная таблица index (открытая
// адресация с линейным пробированием) хранит номер записи +1 (0 - пустая
// ячейка, -1 - надгробие). Порядок вставки нужен циклу "для к, з в
// словаре" (generateIRForEachStmt) - тот же, что и у interp (dictPairs).
// Тот же принцип "растущий буфер через GC_MALLOC" (см. package doc
// ListCollectionCode - риск преждевременной сборки мусора у
// entity-значения, причина не использовать голый calloc/free), что и у
// списочных коллекций выше. Перестройка (resize) сразу и уплотняет
// записи (выбрасывает удалённые), и перехеширует index - при заполнении
// записями 70% index; размер удваивается, только если живых записей
// больше половины этого порога, иначе перестройка на том же размере
// просто убирает удалённые. isStringKey переключает hash/eq на strcmp/
// FNV-1a над байтами C-строки вместо прямого сравнения/умножения значения
// - единственное отличие между строковым и целочисленным (Целое/Байт/
// Символ/Логическое) ключом, оба уже приведены к общему C-типу keyC
// вызывающей стороной (ir_generator.go). structC/setMethod/getMethod/
// hasMethod/delMethod/lengthMethod/emptyMethod/notFoundMsg - уже
// вычислены вызывающей стороной (mangleType/mangleMethod/escapeCString).
// "<structC>_find" возвращает номер записи в slots (-1 - ключа нет) -
// его используют DictIndexSafeGetCode и "равен" (collection_eq.go).
func DictCollectionCode(keyC, valC, structC, setMethod, getMethod, hasMethod, delMethod,
	lengthMethod, emptyMethod, notFoundMsg string, isStringKey bool) string {
	slotType := structC + "_slot"
	hashFn := structC + "_hash"
	eqFn := structC + "_eq"
	resizeFn := structC + "_resize"
	findFn := structC + "_find"
	bucketFn := structC + "_bucket"

	var sb strings.Builder
	fmt.Fprintf(&sb, `typedef struct {
    int state;
    %[1]s key;
    %[2]s value;
} %[3]s;

struct %[4]s {
    %[3]s *slots;
    long long len;
    long long count;
    long long *index;
    long long cap;
};

`, keyC, valC, slotType, structC)

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
    long long oldlen = this_->len;
    this_->slots = GC_MALLOC((size_t)newcap * sizeof(%[3]s));
    this_->index = GC_MALLOC((size_t)newcap * sizeof(long long));
    this_->cap = newcap;
    this_->len = 0;
    for (long long i = 0; i < oldlen; i++) {
        if (old[i].state != 1) {
            continue;
        }
        long long e = this_->len++;
        this_->slots[e] = old[i];
        long long idx = (long long)(%[4]s(old[i].key) %% (unsigned long long)newcap);
        while (this_->index[idx] != 0) {
            idx = (idx + 1) %% newcap;
        }
        this_->index[idx] = e + 1;
    }
}

static long long %[16]s(%[2]s *this_, %[6]s key) {
    if (this_->cap == 0) {
        return -1;
    }
    long long idx = (long long)(%[4]s(key) %% (unsigned long long)this_->cap);
    for (long long probes = 0; probes < this_->cap; probes++) {
        long long e = this_->index[idx];
        if (e == 0) {
            return -1;
        }
        if (e > 0 && %[7]s(this_->slots[e - 1].key, key)) {
            return idx;
        }
        idx = (idx + 1) %% this_->cap;
    }
    return -1;
}

static long long %[5]s(%[2]s *this_, %[6]s key) {
    long long b = %[16]s(this_, key);
    return b < 0 ? -1 : this_->index[b] - 1;
}

static void %[8]s(%[2]s *this_, %[6]s key, %[9]s value) {
    long long e = %[5]s(this_, key);
    if (e >= 0) {
        this_->slots[e].value = value;
        return;
    }
    if (this_->cap == 0 || (this_->len + 1) * 10 > this_->cap * 7) {
        long long newcap = this_->cap == 0 ? 8 : this_->cap;
        if ((this_->count + 1) * 20 > newcap * 7) {
            newcap *= 2;
        }
        %[1]s(this_, newcap);
    }
    e = this_->len++;
    this_->slots[e].state = 1;
    this_->slots[e].key = key;
    this_->slots[e].value = value;
    this_->count++;
    long long idx = (long long)(%[4]s(key) %% (unsigned long long)this_->cap);
    while (this_->index[idx] > 0) {
        idx = (idx + 1) %% this_->cap;
    }
    this_->index[idx] = e + 1;
}

static %[9]s %[10]s(%[2]s *this_, %[6]s key) {
    long long e = %[5]s(this_, key);
    if (e < 0) {
        rech_panic("%[11]s");
    }
    return this_->slots[e].value;
}

static bool %[12]s(%[2]s *this_, %[6]s key) {
    return %[5]s(this_, key) >= 0;
}

static void %[13]s(%[2]s *this_, %[6]s key) {
    long long b = %[16]s(this_, key);
    if (b >= 0) {
        long long e = this_->index[b] - 1;
        memset(&this_->slots[e], 0, sizeof(%[3]s));
        this_->slots[e].state = 2;
        this_->index[b] = -1;
        this_->count--;
    }
}

static long long %[14]s(%[2]s *this_) {
    return this_->count;
}

static bool %[15]s(%[2]s *this_) {
    return this_->count == 0;
}

`, resizeFn, structC, slotType, hashFn, findFn, keyC, eqFn, setMethod, valC, getMethod, notFoundMsg, hasMethod, delMethod, lengthMethod, emptyMethod, bucketFn)

	return sb.String()
}

// GenericCtorCode строит C-текст конструктора "новый Список<T>()"/"новый
// Словарь<К, В>()" и т. п. - GC_MALLOC зануляет память сам (см.
// gcAllocStmts, тот же принцип, что и у обычных ссылочных сущностей) -
// zero-значение (cap/count/used/slots-указатель на NULL) уже и есть
// пустая коллекция, отдельно заполнять поля не нужно. Generic-инстанциации
// стали ссылочными (ctx.entityIsRef) вместе с этим конструктором - раньше
// "новый Список<T>()" был C-литералом структуры по значению
// ("(Тип){0}"), теперь - настоящий указатель, тем же представлением, что
// и у обычных сущностей, и тем же, что использует interp (Go-карта/срез
// внутри значения - объект, не копия). structC/ctorName - уже вычислены
// вызывающей стороной (mangleType/mangleCtor).
func GenericCtorCode(structC, ctorName string) string {
	return fmt.Sprintf(`static %[1]s *%[2]s(void) {
    return GC_MALLOC(sizeof(%[1]s));
}

`, structC, ctorName)
}

// toListLoops - заголовок цикла и выражение элемента для ToListCode по
// виду метода: порядок тот же, что у "для x в ..." (generateIRForEachStmt).
var toListLoops = map[string][2]string{
	"stack":      {"for (long long i = this_->len - 1; i >= 0; i--) {", "this_->data[i]"},
	"queue":      {"for (long long i = 0; i < this_->len; i++) {", "this_->data[i]"},
	"dictKeys":   {"for (long long i = 0; i < this_->len; i++) {\n        if (this_->slots[i].state != 1) { continue; }", "this_->slots[i].key"},
	"dictValues": {"for (long long i = 0; i < this_->len; i++) {\n        if (this_->slots[i].state != 1) { continue; }", "this_->slots[i].value"},
}

// ToListCode строит C-текст метода "ключи()"/"значения()" (fn) коллекции
// structC - новый Список (listStructC, его конструктор listCtor и
// "добавить" listAdd) с элементами в порядке обхода. kind - ключ
// toListLoops. Эмитится после всех generic-коллекций (нужен готовый
// Список<T> результата, который может идти в тексте позже самой
// коллекции).
func ToListCode(kind, structC, fn, listStructC, listCtor, listAdd string) string {
	loop := toListLoops[kind]
	return fmt.Sprintf(`static %[3]s *%[2]s(%[1]s *this_) {
    %[3]s *r = %[4]s();
    %[6]s
        %[5]s(r, %[7]s);
    }
    return r;
}

`, structC, fn, listStructC, listCtor, listAdd, loop[0], loop[1])
}
