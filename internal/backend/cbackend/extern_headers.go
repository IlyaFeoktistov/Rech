// externSymbolHeader - курируемая (НЕ исчерпывающая) таблица "CName
// известной стандартной C-функции -> какой из #include, эмитируемых
// cbackend по СОБСТВЕННЫМ, не связанным с FFI причинам (см. generateSource,
// ir_generator.go), уже объявляет ЭТУ ЖЕ функцию".
//
// Нужна, чтобы "внешняя функция" не писала СВОЙ extern-прототип поверх
// уже включённого заголовка - иначе, если оба вида включения совпали в
// одной программе, компилятор C видит ДВЕ РАЗНЫЕ, возможно несовместимые
// по типу декларации ОДНОГО имени (например, glibc's strlen -
// size_t(const char*), наша собственная догадка по объявлению
// пользователя - long long(const char*)) - настоящая ошибка компиляции,
// найдено живьём (examples/основы/внешние_функции.rech: strlen + <string.h>,
// включённый совсем по другой причине - memcpy захваченной лямбды).
//
// Курируемый список - не претендует на полноту всей стандартной библиотеки
// C, только на самые частые кандидаты по трём заголовкам, которые cbackend
// САМ условно подключает (string.h/stdlib.h/math.h, см. ctx.includesStringH/
// includesStdlibH/includesMathH) - расширять по мере нахождения новых
// коллизий, тем же принципом, что и другие курируемые таблицы проекта
// (internal/lsp/builtindocs.go и т. п.).
package cbackend

import (
	"fmt"
	"runtime"
	"strings"

	"github.com/IlyaFeoktistov/Rech/internal/ir"
)

var externSymbolHeader = map[string]string{
	// windows.h - подключается БЕЗУСЛОВНО на любой сборке под Windows (см.
	// generateSource, "#ifdef _WIN32 ... #include <windows.h>"), не по
	// флагу, как string.h/stdlib.h/math.h ниже - значит для КАЖДОЙ из
	// перечисленных WinAPI-функций заголовок точно уже виден, вне
	// зависимости от остального кода программы. DWORD (беззнаковый 32-бита) -
	// частый реальный тип результата этих функций, конфликтующий с
	// собственной догадкой Rech ("Целое" -> "long long") - найдено живьём
	// (examples/основы/условная_компиляция.rech: GetCurrentProcessId,
	// examples/проекты/условная_компиляция: GetTickCount). Список - те же
	// функции, что уже встречаются в examples/ - не претендует на полноту
	// всего WinAPI (см. package doc), расширять по мере нахождения новых
	// коллизий.
	"GetCurrentProcessId": "windows.h", "GetTickCount": "windows.h",
	"GetLastError": "windows.h", "GetCurrentThreadId": "windows.h",
	"Sleep": "windows.h",
	// user32.h/winuser.h (окна/сообщения) - та же коллизия, найденная живьём
	// на WinAPI-примере окна (GetModuleHandleA возвращает HMODULE, не
	// unsigned char*, RegisterClassA принимает CONST WNDCLASSA*, ShowWindow
	// принимает WINBOOL/int, не bool, и т. д. - собственная догадка Rech по
	// типам параметров/результата почти никогда textуально не совпадает с
	// настоящим прототипом WinAPI, конфликт неизбежен для любой такой
	// функции, раз windows.h виден безусловно).
	"GetModuleHandleA": "windows.h", "RegisterClassA": "windows.h",
	"CreateWindowExA": "windows.h", "ShowWindow": "windows.h",
	"UpdateWindow": "windows.h", "GetMessageA": "windows.h",
	"TranslateMessage": "windows.h", "DispatchMessageA": "windows.h",
	"BeginPaint": "windows.h", "EndPaint": "windows.h",
	"TextOutA": "windows.h", "PostQuitMessage": "windows.h",
	"DefWindowProcA": "windows.h", "DestroyWindow": "windows.h",
	// string.h
	"strlen": "string.h", "strcpy": "string.h", "strncpy": "string.h",
	"strcat": "string.h", "strncat": "string.h", "strcmp": "string.h",
	"strncmp": "string.h", "strchr": "string.h", "strrchr": "string.h",
	"strstr": "string.h", "strtok": "string.h", "strdup": "string.h",
	"memcpy": "string.h", "memmove": "string.h", "memset": "string.h",
	"memcmp": "string.h", "memchr": "string.h", "strerror": "string.h",

	// stdlib.h
	"malloc": "stdlib.h", "calloc": "stdlib.h", "realloc": "stdlib.h",
	"free": "stdlib.h", "atoi": "stdlib.h", "atol": "stdlib.h",
	"atof": "stdlib.h", "strtol": "stdlib.h", "strtod": "stdlib.h",
	"rand": "stdlib.h", "srand": "stdlib.h", "exit": "stdlib.h",
	"abort": "stdlib.h", "abs": "stdlib.h", "labs": "stdlib.h",
	"qsort": "stdlib.h", "getenv": "stdlib.h", "system": "stdlib.h",
	"div": "stdlib.h", "atexit": "stdlib.h",

	// math.h
	"sqrt": "math.h", "pow": "math.h", "sin": "math.h", "cos": "math.h",
	"tan": "math.h", "asin": "math.h", "acos": "math.h", "atan": "math.h",
	"atan2": "math.h", "exp": "math.h", "log": "math.h", "log10": "math.h",
	"floor": "math.h", "ceil": "math.h", "fabs": "math.h", "fmod": "math.h",
	"round": "math.h", "trunc": "math.h",
}

// externPrototypeAlreadyProvided - true, если cName - известный (см.
// externSymbolHeader) стандартный символ, а заголовок, объявляющий его, В
// ЭТОЙ ПРОГРАММЕ и так уже включён по другой причине (ctx.includesStringH/
// includesStdlibH/includesMathH, взведены после генерации ВСЕХ тел -
// см. generateSource) - externPrototypeOfIR в этом случае не пишет
// собственный extern-прототип, полагается на прототип из заголовка.
func externPrototypeAlreadyProvided(ctx *irGenCtx, cName string) bool {
	switch externSymbolHeader[cName] {
	case "string.h":
		// На Windows <windows.h> (включённый безусловно, см. generateSource,
		// "#ifdef _WIN32 ... #include <windows.h>") сам транзитивно тянет
		// <string.h> целиком (MinGW: windows.h -> windef.h -> minwindef.h ->
		// winnt.h -> guiddef.h -> string.h, ради memcmp у IsEqualGUID) - живой
		// баг: "extern long long strlen(...)" конфликтовал с настоящим
		// "size_t strlen(...)" из транзитивно включённого string.h, даже
		// когда ctx.includesStringH сам по себе false (программа не
		// использует ничего из string.h по СОБСТВЕННЫМ причинам cbackend).
		// <stdlib.h>/<math.h> той же цепочкой НЕ затягиваются (atoi/srand/
		// rand/sqrt/div/malloc/free в том же файле не конфликтуют) - поэтому
		// тот же приём НЕ повторён для case "stdlib.h"/"math.h" ниже.
		return ctx.includesStringH || runtime.GOOS == "windows"
	case "stdlib.h":
		return ctx.includesStdlibH
	case "math.h":
		return ctx.includesMathH
	case "windows.h":
		// Целевая платформа сборки - хост самого компилятора rech (нет
		// настоящей кросс-компиляции, см. internal/project/project.go,
		// "сборка.платформа" принимает только "текущая") - тот же принцип,
		// что и ast.CurrentPlatform().
		return runtime.GOOS == "windows"
	default:
		return false
	}
}

// bridgeExternHeaderArgCode - оборачивает code (уже сгенерированное
// Rech-выражение аргумента вызова) в приведение к "(void *)", если C-тип
// этого аргумента - указатель, а сама функция вызывается через прототип из
// НАСТОЯЩЕГО заголовка (externPrototypeAlreadyProvided), не через
// собственную догадку cbackend (externPrototypeOfIR). Нужно потому что
// настоящий заголовок почти всегда объявляет параметр НЕПРОЗРАЧНЫМ
// структурным указателем своего собственного имени (например, WinAPI:
// HWND/HDC/LPMSG - каждый "typedef struct HWND__ *HWND;" и т. п., см.
// windef.h "DECLARE_HANDLE"), а не тем же указательным C-типом, что
// придумал бы для параметра сам Rech (unsigned char*/rech_T_имя* - живой
// пример, найденный на WinAPI-примере окна: RegisterClassA ожидает "const
// WNDCLASSA *", Rech передаёт "rech_T_okonnyjklass *") - ДВА РАЗНЫХ,
// но ABI-совместимых (оба - просто адрес) типа указателя, компилятор C
// теперь отказывается передавать один вместо другого без приведения
// ("incompatible pointer type" - error по умолчанию у современных gcc/
// mingw, раньше было только предупреждение). "void *" - единственный
// указательный тип, неявно совместимый С ЛЮБЫМ другим указательным типом
// в обе стороны без явного приведения (ISO C, 6.3.2.3) - оборачивание
// аргумента в "(void *)" снимает проверку типа на СТОРОНЕ ПЕРЕДАЧИ,
// компилятор молча преобразует его к тому, что реально ожидает настоящий
// прототип из заголовка.
func bridgeExternHeaderArgCode(ctx *irGenCtx, argType ir.Type, code string) string {
	if !strings.HasSuffix(fromIRType(argType).cType(ctx), "*") {
		return code
	}
	return fmt.Sprintf("(void *)(%s)", code)
}

// bridgeExternHeaderReturnCode - симметричный приём для РЕЗУЛЬТАТА вызова:
// настоящий заголовок обычно объявляет результат непрозрачным структурным
// указателем (например, GetModuleHandleA -> HMODULE), а Rech-сторона
// (переменная/присваивание, использующие cType(ctx) результата дальше) -
// собственным указательным типом. В отличие от аргумента - здесь нельзя
// положиться на неявное преобразование через "void *" (это была бы
// ИНИЦИАЛИЗАЦИЯ/присваивание из настоящего типа заголовка НАПРЯМУЮ в
// Rech-тип, тот же класс ошибки, что и у аргумента, только с другой
// стороны) - явное приведение ВСЕГО вызова к ожидаемому Rech-типу
// разрешено для любой пары указательных типов в C без посредника.
func bridgeExternHeaderReturnCode(ctx *irGenCtx, returnType ir.Type, code string) string {
	ctype := fromIRType(returnType).cType(ctx)
	if !strings.HasSuffix(ctype, "*") {
		return code
	}
	return fmt.Sprintf("(%s)(%s)", ctype, code)
}

// bridgeExternStructCallCode - "внешняя структура X как Y" (Y.IsExternStruct,
// см. package doc ir.Entity.IsExternStruct) с "внешняя функция ... : Y" -
// РЕАЛЬНЫЙ возврат такого вызова МОЖЕТ оказаться настоящим C-типом X
// (известный header-символ, см. externSymbolHeader - externPrototypeOfIR в
// этом случае НЕ пишет свой прототип, полагается на заголовок), а не
// собственным rech_T_... структурным типом cType - хоть оба совпадают по
// layout (тот же контракт "внешняя структура"), это ДВА РАЗНЫХ C-типа
// номинально - "cType v = div(...);" не компилируется ("invalid
// initializer"). Мост - union-компаунд-литерал с доступом к нужному полю
// сразу (type punning, GCC/Clang оба поддерживают тем же путём, что и
// __typeof__ - ОБЕ конструкции GNU-расширения, не голый ISO C, тот же
// компилятор, что уже используется проектом) - НЕ нужно знать настоящее
// имя X как строку, только совпадение размера/layout через __typeof__(code).
//
// Вызывается ДО того, как ctx.includesStdlibH/includesStringH/includesMathH
// вообще посчитаны (тела функций генерируются раньше, см. generateSource) -
// значит НЕЛЬЗЯ здесь проверить externPrototypeAlreadyProvided, чтобы
// понять, действительно ли в ЭТОМ конкретном случае был использован
// настоящий заголовок вместо своего прототипа. Решение - не проверять
// вовсе: если свой прототип В ИТОГЕ всё-таки был написан (заголовок не
// понадобился), настоящий C-тип code УЖЕ равен cType (сам же прототип его
// таким и объявил) - обёртка через union в этом случае оборачивает cType
// сам через себя же, без всякого эффекта, лишний, но безопасный no-op.
func bridgeExternStructCallCode(ctx *irGenCtx, value ir.Expr, entityName, cType, code string) (string, bool) {
	call, ok := value.(*ir.CallExpr)
	if !ok {
		return "", false
	}
	if _, isExtern := ctx.externCNames[call.Callee]; !isExtern {
		return "", false
	}
	ent := ctx.entities[entityName]
	if ent == nil || !ent.IsExternStruct {
		return "", false
	}
	return fmt.Sprintf("(union { __typeof__(%s) rech_ffi_src; %s rech_ffi_dst; }){ .rech_ffi_src = (%s) }.rech_ffi_dst", code, cType, code), true
}
