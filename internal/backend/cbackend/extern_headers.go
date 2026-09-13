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
		return ctx.includesStringH
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
