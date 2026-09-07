// PNG/JPEG-кодек - см. package doc internal/semantic/imagecodec.go для
// полного описания решения (единый builtin на оба формата,
// "ФорматИзображения" как enum-параметр). В ОТЛИЧИЕ от cbackend
// (internal/backend/cbackend/png.go/jpeg.go - hand-written C поверх
// libpng/libjpeg) - здесь используется готовый чистый Go image/png/
// image/jpeg стандартной библиотеки, внешняя библиотека не нужна вовсе -
// та же асимметрия между backend'ами, что и у cryptoHelpers/rech.SHA256 (Go
// crypto/sha256 против собственной реализации в cbackend, crypto.go) -
// сам кодек в двух backend'ах принципиально разный код, не текст
// сообщений/константы (rtmsg остаётся общим).
package interp

import (
	"image"
	"image/draw"
	"image/jpeg"
	"image/png"
	"os"

	"github.com/IlyaFeoktistov/Rech/internal/ir"
	"github.com/IlyaFeoktistov/Rech/internal/rtmsg"
)

// imageCodecEntityName - см. internal/semantic/imagecodec.go, тот же
// принцип дублирования имени константой в нескольких пакетах, что и у
// byteListType (bytesio.go).
const imageCodecEntityName = "ДекодированноеИзображение"

// imageFormatPNG/imageFormatJPEG - ИНДЕКСЫ ДОЛЖНЫ БУКВАЛЬНО СОВПАДАТЬ с
// internal/semantic/types.go, StandardEnums["ФорматИзображения"].
const (
	imageFormatPNG  = 0
	imageFormatJPEG = 1
)

// imageToRGBABytes - любой decoded image.Image (PNG - *image.RGBA/*image.
// NRGBA/*image.Paletted/..., JPEG - обычно *image.YCbCr, без альфы) в
// плоский RGBA-буфер построчно, тем же форматом, что и PNG_FORMAT_RGBA в
// cbackend (png.go) - image/draw сам делает то, что там вручную (либо
// прямая копия Pix, либо построчный png_image_finish_read): рисует
// исходное изображение НА СВЕЖЕЕ *image.NRGBA (не premultiplied - "N" -
// straight alpha, ровно наш формат) через draw.Src, JPEG (нет альфы) при
// этом получает alpha=255 автоматически (YCbCr.At всегда непрозрачен).
func imageToRGBABytes(img image.Image) (width, height int, data []byte) {
	bounds := img.Bounds()
	width = bounds.Dx()
	height = bounds.Dy()
	nrgba := image.NewNRGBA(image.Rect(0, 0, width, height))
	draw.Draw(nrgba, nrgba.Bounds(), img, bounds.Min, draw.Src)
	return width, height, nrgba.Pix
}

// decodeImageFile - "декодировать_изображение(путь, формат)".
func decodeImageFile(path string, format int64) Value {
	f, err := os.Open(path)
	if err != nil {
		if format == imageFormatJPEG {
			panicRuntime("%s", rtmsg.JPEGOpenFailed)
		}
		panicRuntime("%s", rtmsg.PNGDecodeFailed)
	}
	defer f.Close()

	var img image.Image
	switch format {
	case imageFormatPNG:
		img, err = png.Decode(f)
		if err != nil {
			panicRuntime("%s: %s", rtmsg.PNGDecodeFailed, err)
		}
	case imageFormatJPEG:
		img, err = jpeg.Decode(f)
		if err != nil {
			panicRuntime("%s: %s", rtmsg.JPEGDecodeFailed, err)
		}
	default:
		panicRuntime("%s", rtmsg.ImageCodecUnknownFormat)
	}

	width, height, data := imageToRGBABytes(img)
	fields := map[string]Value{
		"ширина":  intValue(int64(width)),
		"высота":  intValue(int64(height)),
		"пиксели": bytesValue(data),
	}
	return Value{Type: ir.EntityType(imageCodecEntityName), Obj: &entityObj{typeName: imageCodecEntityName, fields: fields}}
}

// encodeImageFile - "закодировать_изображение(путь, ширина, высота,
// пиксели, формат, качество)" - "пиксели" уже RGBA построчно (Список<
// Байт>, bytesFromValue) - тот же layout, что и image.NRGBA.Pix для
// свежесозданного изображения (Stride == 4*ширина), значит просто copy(),
// без ручного построения по пикселю.
func encodeImageFile(path string, width, height int64, pixelsRGBA []byte, format int64, quality int64) {
	img := image.NewNRGBA(image.Rect(0, 0, int(width), int(height)))
	copy(img.Pix, pixelsRGBA)

	f, err := os.Create(path)
	if err != nil {
		if format == imageFormatJPEG {
			panicRuntime("%s", rtmsg.JPEGEncodeOpenFailed)
		}
		panicRuntime("%s", rtmsg.PNGEncodeFailed)
	}
	defer f.Close()

	switch format {
	case imageFormatPNG:
		if err := png.Encode(f, img); err != nil {
			panicRuntime("%s: %s", rtmsg.PNGEncodeFailed, err)
		}
	case imageFormatJPEG:
		if err := jpeg.Encode(f, img, &jpeg.Options{Quality: int(quality)}); err != nil {
			panicRuntime("%s: %s", rtmsg.JPEGEncodeFailed, err)
		}
	default:
		panicRuntime("%s", rtmsg.ImageCodecUnknownFormat)
	}
}
