// Бинарно-безопасный файловый ввод-вывод - см. package doc internal/
// semantic/bytesio.go для полного описания решения (почему "Список<Байт>",
// не новый тип). Эмитится ПОСЛЕ genericsCode (ir_generator.go), как и
// queryParamsHelper (httpserver.go) - её C-структуре (mangleType(
// byteListType) - "data"/"len"/"cap") нужно уже существовать, поэтому не
// обычный package-level var (в отличие от fileHelpers, builtins_shared.go,
// не зависящей ни от одной generic-инстанциации) - функция, вызванная в
// нужном месте generateSource.
package cbackend

import (
	"fmt"

	"github.com/IlyaFeoktistov/Rech/internal/rtmsg"
)

// byteListType - см. internal/semantic/bytesio.go, package doc
// queryParamsDictType (httpserver.go) для того же принципа дублирования
// имени константой в обоих пакетах, не импорта internal/semantic отсюда.
const byteListType = "Список<Байт>"

// bytesWrapHelper - rech_bytes_wrap: копия len байт из data в СВЕЖИЙ
// GC_MALLOC'нутый Список<Байт> (не разделяет память с data - вызывающий
// код может свободно освободить/переиспользовать data сразу после
// вызова). len == 0 - data может быть NULL (пустой список, data-поле само
// NULL - тот же случай, что и у "новый Список<Байт>()" без единого
// "добавить"). Отдельно от bytesIOHelpers ниже (не только файловый ввод-
// вывод нуждается в ней - "принять_http_байты"/"отправить_http_ответ_
// байты", httpbytes.go, строят тело запроса из сырых байт сокета тем же
// способом, без единого fopen/fread) - гейтится СОБСТВЕННЫМ флагом
// (ctx.usesByteListWrap), не usesBytesIO/usesFileIO.
func bytesWrapHelper() string {
	cType := mangleType(byteListType)
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
`, cType)
}

// byteStringConvHelpers - "строка_из_байт"/"байты_из_строки" (мост между
// "Список<Байт>" и "Строка" - см. package doc registerBytesIOFuncs,
// internal/semantic/bytesio.go, зачем он нужен). "rech_string_from_bytes"
// физически копирует ВСЕ байты в новый буфер (даже если среди них есть
// нулевые) - потеря происходит не здесь, а позже, у любой операции,
// читающей результат через strlen (врождённое ограничение null-terminated
// "Строка" в cbackend, не новое). "rech_bytes_from_string" не теряет
// ничего - strlen(s) уже и есть весь реальный размер содержимого Строки.
func byteStringConvHelpers() string {
	cType := mangleType(byteListType)
	return fmt.Sprintf(`static char *rech_string_from_bytes(%[1]s *b) {
    char *out = malloc((size_t)b->len + 1);
    if (b->len > 0) {
        memcpy(out, b->data, (size_t)b->len);
    }
    out[b->len] = '\0';
    return out;
}

static %[1]s *rech_bytes_from_string(const char *s) {
    return rech_bytes_wrap((const unsigned char *)s, (long long)strlen(s));
}
`, cType)
}

func bytesIOHelpers() string {
	cType := mangleType(byteListType)
	return fmt.Sprintf(`static %[1]s *rech_read_bytes(rech_file_t f, long long max) {
    if (max <= 0) {
        rech_panic("%[2]s");
    }
    unsigned char *buf = malloc((size_t)max);
    size_t n = fread(buf, 1, (size_t)max, f);
    if (n == 0 && ferror(f)) {
        free(buf);
        rech_panic("%[3]s");
    }
    %[1]s *out = rech_bytes_wrap(buf, (long long)n);
    free(buf);
    return out;
}

static void rech_write_bytes(rech_file_t f, %[1]s *b) {
    if (b->len > 0 && fwrite(b->data, 1, (size_t)b->len, f) != (size_t)b->len) {
        rech_panic("%[4]s");
    }
}

static %[1]s *rech_read_whole_file_bytes(const char *path) {
    rech_file_t f = rech_open_mode(path, "rb", L"rb", "прочитать_файл_байты");
    if (fseek(f, 0, SEEK_END) != 0 || ftell(f) < 0) {
        rech_panic("%[3]s");
    }
    long size = ftell(f);
    rewind(f);
    unsigned char *buf = malloc((size_t)size > 0 ? (size_t)size : 1);
    size_t n = fread(buf, 1, (size_t)size, f);
    if (n != (size_t)size && ferror(f)) {
        free(buf);
        fclose(f);
        rech_panic("%[3]s");
    }
    fclose(f);
    %[1]s *out = rech_bytes_wrap(buf, (long long)n);
    free(buf);
    return out;
}

static void rech_write_file_bytes(const char *path, %[1]s *b) {
    rech_file_t f = rech_open_mode(path, "wb", L"wb", "записать_в_файл_байты");
    rech_write_bytes(f, b);
    fclose(f);
}
`, cType, rtmsg.ReadBytesInvalidMax, rtmsg.ReadBytesIOFailed, rtmsg.WriteBytesIOFailed)
}
