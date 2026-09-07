package cbackend

import (
	"fmt"

	"github.com/IlyaFeoktistov/Rech/internal/rtmsg"
)

// pngHelpers - "png_декодировать_файл(Строка): ДекодированноеИзображение"/
// "png_кодировать_файл(Строка, Целое, Целое, Список<Байт>)" - биндинг к
// libpng через её "упрощённый" API (<png.h>, png_image_* - часть
// официального libpng с версии 1.6, разработана специально для точно
// этого случая: весь файл в память/из памяти одним вызовом, без
// setjmp/callback-машинерии низкоуровневого API) - -lpng добавляется по
// маркеру "#include <png.h>" (internal/toolchain/cc.go).
//
// Формат пикселей - PNG_FORMAT_RGBA (4 байта на пиксель, построчно) - тот
// же формат, что у поля "пиксели" пользовательской "Изображение"
// (examples/изображения/растровое_изображение.rech) - конверсия между
// ними на стороне Rech (examples/изображения/png_jpeg.rech) - чистое
// копирование, без перестановки байт.
var pngHelpers = fmt.Sprintf(`static %[1]s rech_png_decode_file(const char *path) {
    png_image image;
    memset(&image, 0, sizeof(image));
    image.version = PNG_IMAGE_VERSION;
    if (!png_image_begin_read_from_file(&image, path)) {
        char errbuf[512];
        snprintf(errbuf, sizeof(errbuf), "%[6]s: %%s", image.message);
        rech_panic(errbuf);
    }
    image.format = PNG_FORMAT_RGBA;
    size_t size = PNG_IMAGE_SIZE(image);
    unsigned char *buffer = malloc(size > 0 ? size : 1);
    if (!png_image_finish_read(&image, NULL, buffer, 0, NULL)) {
        char errbuf[512];
        snprintf(errbuf, sizeof(errbuf), "%[6]s: %%s", image.message);
        free(buffer);
        png_image_free(&image);
        rech_panic(errbuf);
    }
    %[1]s out;
    out.%[2]s = (long long)image.width;
    out.%[3]s = (long long)image.height;
    out.%[4]s = rech_bytes_wrap(buffer, (long long)size);
    free(buffer);
    png_image_free(&image);
    return out;
}

static void rech_png_encode_file(const char *path, long long width, long long height, %[5]s *pixels) {
    png_image image;
    memset(&image, 0, sizeof(image));
    image.version = PNG_IMAGE_VERSION;
    image.width = (png_uint_32)width;
    image.height = (png_uint_32)height;
    image.format = PNG_FORMAT_RGBA;
    if (!png_image_write_to_file(&image, path, 0, pixels->data, 0, NULL)) {
        char errbuf[512];
        snprintf(errbuf, sizeof(errbuf), "%[7]s: %%s", image.message);
        png_image_free(&image);
        rech_panic(errbuf);
    }
    png_image_free(&image);
}
`, mangleType(imageCodecEntityName), mangleField("ширина"), mangleField("высота"), mangleField("пиксели"), mangleType(byteListType), escapeCString(rtmsg.PNGDecodeFailed), escapeCString(rtmsg.PNGEncodeFailed))
