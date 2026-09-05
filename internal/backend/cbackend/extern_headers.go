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

var externSymbolHeader = map[string]string{
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
	default:
		return false
	}
}
