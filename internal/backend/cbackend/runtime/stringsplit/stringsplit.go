package stringsplit

import "fmt"

// ResultType - см. internal/semantic/stringsplit.go, splitResultType - то
// же имя генерик-инстанциации, принудительно зарегистрированной для
// "разбить" (попадает в prog.Generics тем же путём, что и любая generic-
// коллекция, реально написанная пользователем).
const ResultType = "Список<Строка>"

// Helper - "разбить" (строка, разделитель): Список<Строка> - разбивает по
// буквальному вхождению разделителя (strstr, не regex), повторяя strstr с
// побайтовым сравнением - результат совпадений всегда приходится на
// границу code point'а (совпадение подстроки в валидном UTF-8 не может
// располовинить символ), поэтому, в отличие от strutil.Helpers, отдельная
// code-point-арифметика здесь не нужна. Пустой разделитель - ошибка
// (rech_panic), не бесконечный цикл и не молчаливый выбор одной из
// нескольких равно правдоподобных трактовок (по code point'ам? по байтам?).
//
// После typedefs/genericsCode (Список<Строка> - коллекция примитива,
// см. её package doc в internal/semantic/stringsplit.go) - rech_split_string
// использует typedef/метод "добавить" этой инстанциации, уже эмитированные
// там. mangledType/addMethod/emptySeparatorMsg - уже вычисленные вызывающей
// стороной (mangleType/mangleMethod - внутренняя, неэкспортированная
// инфраструктура cbackend, этот пакет от неё не зависит).
func Helper(mangledType, addMethod, emptySeparatorMsg string) string {
	return fmt.Sprintf(`static %[1]s *rech_split_string(const char *s, const char *sep) {
    if (!sep[0]) {
        rech_panic("%[3]s");
    }
    %[1]s *out = GC_MALLOC(sizeof(%[1]s));
    size_t sep_len = strlen(sep);
    const char *start = s;
    const char *found;
    while ((found = strstr(start, sep)) != NULL) {
        size_t len = (size_t)(found - start);
        char *piece = malloc(len + 1);
        memcpy(piece, start, len);
        piece[len] = '\0';
        %[2]s(out, piece);
        start = found + sep_len;
    }
    size_t len = strlen(start);
    char *piece = malloc(len + 1);
    memcpy(piece, start, len);
    piece[len] = '\0';
    %[2]s(out, piece);
    return out;
}
`, mangledType, addMethod, emptySeparatorMsg)
}
