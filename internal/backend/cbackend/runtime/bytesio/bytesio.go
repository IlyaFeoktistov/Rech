// Бинарно-безопасный файловый ввод-вывод - см. package doc internal/
// semantic/bytesio.go для полного описания решения (почему "Список<Байт>",
// не новый тип). Эмитится ПОСЛЕ genericsCode (ir_generator.go), как и
// queryParamsHelper (httpserver.go) - её C-структуре (mangledByteList -
// "data"/"len"/"cap") нужно уже существовать, поэтому функции здесь
// принимают уже вычисленный C-тип "Список<Байт>" параметром, а не читают
// его сами (mangleType - внутренняя, неэкспортированная инфраструктура
// cbackend, этот пакет от неё не зависит - тот же приём, что и у
// runtime/syncprim).
package bytesio

import (
	"fmt"

	"github.com/IlyaFeoktistov/Rech/internal/rtmsg"
)

// WrapHelper - rech_bytes_wrap: копия len байт из data в СВЕЖИЙ
// GC_MALLOC'нутый Список<Байт> (не разделяет память с data - вызывающий
// код может свободно освободить/переиспользовать data сразу после
// вызова). len == 0 - data может быть NULL (пустой список, data-поле само
// NULL - тот же случай, что и у "новый Список<Байт>()" без единого
// "добавить"). Отдельно от IOHelpers ниже (не только файловый ввод-
// вывод нуждается в ней - "принять_http_байты"/"отправить_http_ответ_
// байты", httpbytes.go, строят тело запроса из сырых байт сокета тем же
// способом, без единого fopen/fread) - гейтится СОБСТВЕННЫМ флагом
// (ctx.usesByteListWrap), не usesBytesIO/usesFileIO.
func WrapHelper(mangledByteList string) string {
	return fmt.Sprintf(`static %[1]s *rech_bytes_wrap(const unsigned char *data, long long len) {
    %[1]s *out = GC_MALLOC(sizeof(%[1]s));
    out->len = len;
    out->cap = len;
    out->data = len > 0 ? GC_MALLOC((size_t)len) : NULL;
    if (len > 0) {
        memcpy(out->data, data, (size_t)len);
    }
    return out;
}
`, mangledByteList)
}

// StringConvHelpers - "строка_из_байт"/"байты_из_строки" (мост между
// "Список<Байт>" и "Строка" - см. package doc registerBytesIOFuncs,
// internal/semantic/bytesio.go, зачем он нужен). "rech_string_from_bytes"
// - по правилу "Строка - валидный UTF-8 без NUL" (rech_text_take,
// strutil.TextCode): невалидные байты и NUL становятся U+FFFD.
// "rech_bytes_from_string" не теряет ничего - в Строке нет NUL, strlen(s)
// и есть весь её размер.
func StringConvHelpers(mangledByteList string) string {
	return fmt.Sprintf(`static char *rech_string_from_bytes(%[1]s *b) {
    char *out = malloc((size_t)b->len + 1);
    if (b->len > 0) {
        memcpy(out, b->data, (size_t)b->len);
    }
    return rech_text_take(out, (size_t)b->len);
}

static %[1]s *rech_bytes_from_string(const char *s) {
    return rech_bytes_wrap((const unsigned char *)s, (long long)strlen(s));
}
`, mangledByteList)
}

// IOHelpers - файловое чтение/запись "Список<Байт>" целиком (rech_read_
// bytes/rech_write_bytes/rech_read_whole_file_bytes/rech_write_file_bytes).
func IOHelpers(mangledByteList string) string {
	return fmt.Sprintf(`static %[1]s *rech_read_bytes(rech_file_t f, long long max) {
    if (max <= 0) {
        rech_panic("%[2]s");
    }
    unsigned char *buf = malloc((size_t)max);
    size_t n = 0;
    int c;
    while (n < (size_t)max && (c = rech_pend_next(f)) != -1) {
        buf[n++] = (unsigned char)c;
    }
    n += fread(buf + n, 1, (size_t)max - n, f);
    if (n == 0 && ferror(f)) {
        free(buf);
        rech_error_as(RECH_ERR_FILE, "%[3]s");
    }
    %[1]s *out = rech_bytes_wrap(buf, (long long)n);
    free(buf);
    return out;
}

static void rech_write_bytes(rech_file_t f, %[1]s *b) {
    if (b->len > 0 && fwrite(b->data, 1, (size_t)b->len, f) != (size_t)b->len) {
        rech_error_as(RECH_ERR_FILE, "%[4]s");
    }
}

static %[1]s *rech_read_whole_file_bytes(const char *path) {
    rech_file_t f = rech_open_mode(path, "rb", L"rb", "прочитать_файл_байты");
    // Растущий буфер вместо fseek(SEEK_END)+ftell (найденный баг - virtual-
    // файлы вроде "/proc/<pid>/maps" не поддерживают SEEK_END, ftell там
    // всегда 0, из-за чего файл читался как пустой) - тот же приём, что
    // уже использует rech_read_pipe_thread (subprocess.go) для пайпов,
    // размер которых заранее неизвестен ни при каких обстоятельствах.
    size_t cap = 4096, len = 0;
    unsigned char *buf = malloc(cap);
    for (;;) {
        if (len + 1 >= cap) {
            cap *= 2;
            buf = realloc(buf, cap);
        }
        size_t n = fread(buf + len, 1, cap - len - 1, f);
        len += n;
        if (n == 0) {
            break;
        }
    }
    if (ferror(f)) {
        free(buf);
        fclose(f);
        rech_error_as(RECH_ERR_FILE, "%[3]s");
    }
    fclose(f);
    %[1]s *out = rech_bytes_wrap(buf, (long long)len);
    free(buf);
    return out;
}

static void rech_write_file_bytes(const char *path, %[1]s *b) {
    rech_file_t f = rech_open_mode(path, "wb", L"wb", "записать_в_файл_байты");
    rech_write_bytes(f, b);
    fclose(f);
}
`, mangledByteList, rtmsg.ReadBytesInvalidMax, rtmsg.ReadBytesIOFailed, rtmsg.WriteBytesIOFailed)
}
