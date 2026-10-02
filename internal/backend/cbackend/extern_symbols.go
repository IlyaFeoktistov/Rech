// Прототипы "внешняя функция" - собственные, под собственным C-именем
// (rech_ext_<псевдоним>), привязанным к настоящему символу через asm-метку
// GCC/Clang:
//
//	extern long long rech_ext_dlina(const char *) __asm__(RECH_SYM("strlen"));
//
// Прототип никогда не конфликтует с заголовком, который объявляет тот же
// символ со своими типами (windows.h - всегда на Windows, string.h/
// stdlib.h/math.h - когда их подключает сам cbackend, "сборка.заголовки" -
// по просьбе проекта): это другое C-имя. Компилятор C видит в вызове ровно
// те типы, что объявлены в Rech (Указатель<T>, внешняя структура и т. п.),
// никаких приведений к типам заголовка не нужно. RECH_SYM добавляет
// платформенный префикс символа (__USER_LABEL_PREFIX__: "_" на macOS, ""
// на Linux/Windows x64).
package cbackend

import (
	"fmt"
	"strings"

	"github.com/IlyaFeoktistov/Rech/internal/ir"
)

// externSymbolMacros - RECH_SYM(имя): строка настоящего имени символа с
// платформенным префиксом, для __asm__-метки прототипа.
const externSymbolMacros = `#define RECH_STR_(x) #x
#define RECH_STR(x) RECH_STR_(x)
#define RECH_SYM(name) RECH_STR(__USER_LABEL_PREFIX__) name
`

// mangleExternFunc - C-имя прототипа внешней функции с псевдонимом alias
// (по псевдониму, не по C-символу: два объявления одного символа с разными
// Rech-типами - два разных прототипа).
func mangleExternFunc(alias string) string { return "rech_ext_" + transliterate(alias) }

// externPrototypeOfIR - прототип внешней функции ext (см. package doc).
// "Вызываемый(...)" параметром - голый указатель на функцию (колбэк,
// ffi_callback.go), "Вызываемый"/"?Вызываемый" результатом - тоже.
func externPrototypeOfIR(ctx *irGenCtx, ext *ir.ExternFuncDecl) (string, error) {
	cReturnType := "void"
	if ext.HasReturn {
		cReturnType = fromIRType(ext.ReturnType).cType(ctx)
		if sigName, _, ok := ctx.ffiFuncPointerSig(ext.ReturnType); ok {
			cReturnType = mangleRawFnType(sigName)
		}
	}
	types := make([]string, len(ext.Params))
	for i, p := range ext.Params {
		t, err := ffiParamCType(ctx, p)
		if err != nil {
			return "", err
		}
		types[i] = t
	}
	if ext.Variadic {
		types = append(types, "...")
	}
	params := "void"
	if len(types) > 0 {
		params = strings.Join(types, ", ")
	}
	return fmt.Sprintf("extern %s %s(%s) __asm__(RECH_SYM(\"%s\"));\n", cReturnType, mangleExternFunc(ext.Alias), params, ext.CName), nil
}
