package cbackend

import (
	"fmt"

	"github.com/IlyaFeoktistov/Rech/internal/rtmsg"
)

// jpegHelpers - "jpeg_декодировать_файл(Строка): ДекодированноеИзображение"/
// "jpeg_кодировать_файл(Строка, Целое, Целое, Список<Байт>, Целое)" -
// биндинг к libjpeg (<jpeglib.h>) через её стандартный файловый API
// (jpeg_stdio_src/jpeg_stdio_dest, из example.c самой библиотеки) - -ljpeg
// добавляется по маркеру "#include <jpeglib.h>" (internal/toolchain/cc.go).
//
// В отличие от pngHelpers (упрощённый API libpng сам решает setjmp/error-
// handling) - у libjpeg НЕТ упрощённого API, ошибка по умолчанию
// (jpeg_std_error) сама вызывает exit() - здесь заменена на СОБСТВЕННЫЙ
// error_exit (rech_jpeg_error_mgr, тот же приём, что показан в example.c
// самой библиотеки) с setjmp/longjmp обратно в rech_panic - вместо тихого
// exit() программа получает то же самое сообщение об ошибке, что и у
// PNG/BMP.
//
// out_color_space/in_color_space принудительно JCS_RGB на декодировании
// (3 канала, серые JPEG сами апсемплятся libjpeg) - альфа-канал в RGBA
// выхода всегда 255 (в JPEG её физически не бывает); тот же выбор при
// кодировании - RGBA-вход теряет альфу (drop), тем же принципом, что и
// сохранение BMP (examples/изображения/растровое_изображение.rech).
var jpegHelpers = fmt.Sprintf(`struct rech_jpeg_error_mgr {
    struct jpeg_error_mgr pub;
    jmp_buf setjmp_buffer;
};

static void rech_jpeg_error_exit(j_common_ptr cinfo) {
    struct rech_jpeg_error_mgr *err = (struct rech_jpeg_error_mgr *)cinfo->err;
    longjmp(err->setjmp_buffer, 1);
}

static %[1]s rech_jpeg_decode_file(const char *path) {
    FILE *f = fopen(path, "rb");
    if (f == NULL) {
        rech_panic("%[6]s");
    }

    struct jpeg_decompress_struct cinfo;
    struct rech_jpeg_error_mgr jerr;
    cinfo.err = jpeg_std_error(&jerr.pub);
    jerr.pub.error_exit = rech_jpeg_error_exit;
    if (setjmp(jerr.setjmp_buffer)) {
        jpeg_destroy_decompress(&cinfo);
        fclose(f);
        rech_panic("%[7]s");
    }

    jpeg_create_decompress(&cinfo);
    jpeg_stdio_src(&cinfo, f);
    jpeg_read_header(&cinfo, TRUE);
    cinfo.out_color_space = JCS_RGB;
    jpeg_start_decompress(&cinfo);

    long long width = (long long)cinfo.output_width;
    long long height = (long long)cinfo.output_height;
    unsigned char *row = malloc((size_t)width * 3);
    unsigned char *rgba = malloc((size_t)(width * height * 4));
    long long y = 0;
    while (cinfo.output_scanline < cinfo.output_height) {
        unsigned char *rowptr[1];
        rowptr[0] = row;
        jpeg_read_scanlines(&cinfo, rowptr, 1);
        for (long long x = 0; x < width; x++) {
            unsigned char *src = row + x * 3;
            unsigned char *dst = rgba + (y * width + x) * 4;
            dst[0] = src[0];
            dst[1] = src[1];
            dst[2] = src[2];
            dst[3] = 255;
        }
        y++;
    }
    free(row);
    jpeg_finish_decompress(&cinfo);
    jpeg_destroy_decompress(&cinfo);
    fclose(f);

    %[1]s out;
    out.%[2]s = width;
    out.%[3]s = height;
    out.%[4]s = rech_bytes_wrap(rgba, width * height * 4);
    free(rgba);
    return out;
}

static void rech_jpeg_encode_file(const char *path, long long width, long long height, %[5]s *pixels, long long quality) {
    FILE *f = fopen(path, "wb");
    if (f == NULL) {
        rech_panic("%[8]s");
    }

    struct jpeg_compress_struct cinfo;
    struct rech_jpeg_error_mgr jerr;
    cinfo.err = jpeg_std_error(&jerr.pub);
    jerr.pub.error_exit = rech_jpeg_error_exit;
    if (setjmp(jerr.setjmp_buffer)) {
        jpeg_destroy_compress(&cinfo);
        fclose(f);
        rech_panic("%[9]s");
    }

    jpeg_create_compress(&cinfo);
    jpeg_stdio_dest(&cinfo, f);
    cinfo.image_width = (JDIMENSION)width;
    cinfo.image_height = (JDIMENSION)height;
    cinfo.input_components = 3;
    cinfo.in_color_space = JCS_RGB;
    jpeg_set_defaults(&cinfo);
    jpeg_set_quality(&cinfo, (int)quality, TRUE);
    jpeg_start_compress(&cinfo, TRUE);

    unsigned char *row = malloc((size_t)width * 3);
    long long y = 0;
    while (cinfo.next_scanline < cinfo.image_height) {
        for (long long x = 0; x < width; x++) {
            unsigned char *src = pixels->data + (y * width + x) * 4;
            unsigned char *dst = row + x * 3;
            dst[0] = src[0];
            dst[1] = src[1];
            dst[2] = src[2];
        }
        unsigned char *rowptr[1];
        rowptr[0] = row;
        jpeg_write_scanlines(&cinfo, rowptr, 1);
        y++;
    }
    free(row);
    jpeg_finish_compress(&cinfo);
    fclose(f);
    jpeg_destroy_compress(&cinfo);
}
`, mangleType(imageCodecEntityName), mangleField("ширина"), mangleField("высота"), mangleField("пиксели"), mangleType(byteListType),
	escapeCString(rtmsg.JPEGOpenFailed), escapeCString(rtmsg.JPEGDecodeFailed), escapeCString(rtmsg.JPEGEncodeOpenFailed), escapeCString(rtmsg.JPEGEncodeFailed))
