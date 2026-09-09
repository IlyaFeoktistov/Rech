// Package semantic: PNG/JPEG-кодек - третий срез работы с изображениями
// (docs/ROADMAP.md, "Работа с изображениями") - в отличие от первых двух
// (BMP + примитивы, examples/изображения/*.rech - библиотеки НА САМОМ
// Rech), настоящее сжатие требует внешней библиотеки (libpng/libjpeg,
// пользователь выбрал их вместо однофайлового stb_image - тот же класс
// решения, что уже принят для TLS/HTTP2 - см. package doc tls.go/http2.go)
// - значит эта часть встроена в компилятор (hand-written C в
// internal/backend/cbackend/{png,jpeg,imagecodec}.go), а не выразима в
// пользовательском Rech-коде через `внешняя функция` (полный libpng/
// libjpeg API - callback-based I/O, setjmp/longjmp, error manager - не
// представим декларативным FFI-подмножеством раздела 12 docs/SPEC.md).
//
// Один builtin на оба формата - "декодировать_изображение(путь,
// ФорматИзображения)"/"закодировать_изображение(путь, ширина, высота,
// пиксели, ФорматИзображения, качество)" - тот же принцип, что и
// "хеш(АлгоритмХеша, данные)" (types.go, StandardEnums) - формат выбирается
// через builtin-перечисление, не через отдельное имя функции на каждый
// формат. "качество" (JPEG-специфичный параметр, 0-100) игнорируется при
// формате PNG (без сжатия с потерями) - тот же класс компромисса, что и у
// ParamDefaults в других builtin-функциях с параметром, нужным не всем
// веткам сразу.
//
// Встроенные функции работают с ПУТЁМ к файлу напрямую (не с "Список<Байт>"
// в памяти, в отличие от bytesio.go) - сам кодек и файловый ввод-вывод не
// разделены здесь умышленно (упрощает C-реализацию: png_image_begin_read_
// from_file/jpeg_stdio_src читают прямо из файла, без буфера-читателя в
// памяти) - конверсия в/из пользовательской сущности "Изображение"
// (examples/изображения/растровое_изображение.rech) - тонкая обёртка на
// самом Rech (examples/изображения/png_jpeg.rech), тем же принципом, что и
// у "сохранить_bmp"/"загрузить_bmp" в первом срезе.
package semantic

import "github.com/IlyaFeoktistov/Rech/internal/ir"

// imageCodecEntityName - "ДекодированноеИзображение", результат
// "декодировать_изображение" - ширина/высота плюс пиксели построчно как
// RGBA (Список<Байт>, тот же формат, что и у поля "пиксели"
// пользовательской "Изображение") - НЕ САМА пользовательская "Изображение"
// (компилятор ничего не знает про сущности, объявленные в examples/*.rech -
// точно тот же довод, что и у "Запрос"/"Ответ"/"RGBЦвет" не пытающихся
// притвориться пользовательским типом).
const imageCodecEntityName = "ДекодированноеИзображение"

var ImageCodecFields = []ir.Field{
	{Name: "ширина", Type: ir.TypeInt},
	{Name: "высота", Type: ir.TypeInt},
	{Name: "пиксели", Type: ir.EntityType(byteListType)},
}

// imageCodecFuncNames - имена функций этого файла - используется
// analyzeCallExpr (expr.go) для решения, нужно ли звать
// registerImageCodecEntityOnCall при разборе конкретного вызова - тот же
// принцип, что и bytesIOFuncNames (bytesio.go).
var imageCodecFuncNames = map[string]bool{
	"декодировать_изображение": true,
	"закодировать_изображение": true,
}

// registerImageCodecFuncs регистрирует сигнатуры обеих функций этого
// файла - вызывается безусловно (как registerBytesIOFuncs) при сборе
// сигнатур программы; ReturnType/Params ссылаются на imageCodecEntityName/
// byteListType по имени - сами типы резолвятся лениво, при первом реальном
// вызове (registerImageCodecEntityOnCall), не здесь - тот же принцип, что и
// у "начать_транзакцию"/"Транзакция" (dbtx.go). "формат" - обычный Целое
// (член встроенного перечисления "ФорматИзображения", types.go) - тем же
// принципом, что и "алгоритм" у "хеш" (builtins.All).
func registerImageCodecFuncs(funcs map[string]funcSig) {
	funcs["декодировать_изображение"] = funcSig{
		Params:     []ir.Type{ir.TypeString, ir.TypeInt},
		ReturnType: ir.EntityType(imageCodecEntityName),
		HasReturn:  true,
	}
	funcs["закодировать_изображение"] = funcSig{
		Params: []ir.Type{ir.TypeString, ir.TypeInt, ir.TypeInt, ir.EntityType(byteListType), ir.TypeInt, ir.TypeInt},
	}
}

// registerImageCodecEntityOnCall - лениво регистрирует "Список<Байт>"
// (registerByteListTypeOnCall, bytesio.go - "пиксели" ссылается на него) и
// саму "ДекодированноеИзображение" - вызывается из analyzeCallExpr
// (expr.go) при каждом вызове одной из imageCodecFuncNames, идемпотентно
// (registerHTTPEntity/registerByteListTypeOnCall сами проверяют, не
// зарегистрировано ли уже).
func registerImageCodecEntityOnCall(entities map[string]entitySig, methods map[string]map[string]funcSig, aliases map[string]string, interfaces map[string]interfaceSig) error {
	if err := registerByteListTypeOnCall(entities, methods, aliases, interfaces); err != nil {
		return err
	}
	registerHTTPEntity(entities, imageCodecEntityName, ImageCodecFields)
	return nil
}
