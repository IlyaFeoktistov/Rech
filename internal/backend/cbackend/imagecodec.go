package cbackend

import (
	"fmt"

	"github.com/IlyaFeoktistov/Rech/internal/rtmsg"
)

// imageCodecEntityName - см. internal/semantic/imagecodec.go, тот же
// принцип дублирования имени константой в обоих пакетах, что и у
// byteListType (bytesio.go, package doc) - не импорта internal/semantic
// отсюда. Общая для png.go/jpeg.go - обе возвращают одну и ту же
// "ДекодированноеИзображение".
const imageCodecEntityName = "ДекодированноеИзображение"

// imageCodecDispatchHelpers - rech_decode_image_file/rech_encode_image_file
// - диспетчер builtin'ов "декодировать_изображение(путь, формат)"/
// "закодировать_изображение(путь, ширина, высота, пиксели, формат,
// качество)" по значению члена перечисления "ФорматИзображения" - ИНДЕКСЫ
// ДОЛЖНЫ БУКВАЛЬНО СОВПАДАТЬ с internal/semantic/types.go,
// StandardEnums["ФорматИзображения"] (0=PNG, 1=JPEG) - тот же контракт, что
// у rech_hash_hex (crypto.go) для "АлгоритмХеша". "качество" игнорируется
// веткой PNG (см. package doc internal/semantic/imagecodec.go).
var imageCodecDispatchHelpers = fmt.Sprintf(`static %[1]s rech_decode_image_file(const char *path, int format) {
    switch (format) {
    case 0:
        return rech_png_decode_file(path);
    case 1:
        return rech_jpeg_decode_file(path);
    default: {
        rech_panic("%[2]s");
        %[1]s out;
        memset(&out, 0, sizeof(out));
        return out;
    }
    }
}

static void rech_encode_image_file(const char *path, long long width, long long height, %[3]s *pixels, int format, long long quality) {
    switch (format) {
    case 0:
        rech_png_encode_file(path, width, height, pixels);
        return;
    case 1:
        rech_jpeg_encode_file(path, width, height, pixels, quality);
        return;
    default:
        rech_panic("%[2]s");
    }
}
`, mangleType(imageCodecEntityName), escapeCString(rtmsg.ImageCodecUnknownFormat), mangleType(byteListType))
