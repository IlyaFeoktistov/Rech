package cbackend

import (
	"fmt"

	"github.com/IlyaFeoktistov/Rech/internal/rtmsg"
)

// listCollectionCCode строит C-текст (typedef struct + функции) для
// конкретной инстанциации "Список<T>" по одному только имени и C-типу
// элемента - без зависимости от кодогенерации, чтобы одна и та же
// реализация (growable array) обслуживала любой путь кодогенерации, у
// которого сигнатуры методов уже целиком в самих IR-узлах (MethodCallExpr),
// а этому коду для кодогенерации нужен только сам C-текст.
func listCollectionCCode(name, elemC string) string {
	structName := mangleType(name)
	add := mangleMethod(name, "добавить")
	get := mangleMethod(name, "получить")
	set := mangleMethod(name, "установить")
	length := mangleMethod(name, "длина")
	empty := mangleMethod(name, "пусто")
	outOfRange := escapeCString(rtmsg.IndexOutOfRange(name))

	return fmt.Sprintf(`typedef struct {
    %[1]s *data;
    long long len;
    long long cap;
} %[2]s;

static void %[3]s(%[2]s *this_, %[1]s value) {
    if (this_->len == this_->cap) {
        this_->cap = this_->cap == 0 ? 4 : this_->cap * 2;
        this_->data = realloc(this_->data, sizeof(%[1]s) * this_->cap);
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

`, elemC, structName, add, get, set, length, outOfRange, empty)
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

	return fmt.Sprintf(`typedef struct {
    %[1]s *data;
    long long len;
    long long cap;
} %[2]s;

static void %[3]s(%[2]s *this_, %[1]s value) {
    if (this_->len == this_->cap) {
        this_->cap = this_->cap == 0 ? 4 : this_->cap * 2;
        this_->data = realloc(this_->data, sizeof(%[1]s) * this_->cap);
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

	return fmt.Sprintf(`typedef struct {
    %[1]s *data;
    long long len;
    long long cap;
} %[2]s;

static void %[3]s(%[2]s *this_, %[1]s value) {
    if (this_->len == this_->cap) {
        this_->cap = this_->cap == 0 ? 4 : this_->cap * 2;
        this_->data = realloc(this_->data, sizeof(%[1]s) * this_->cap);
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
