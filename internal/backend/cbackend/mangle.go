// Package cbackend транслирует internal/ir.Program (см. package doc
// internal/ir) в исходник на C.
//
// Это первый и на сегодня единственный backend Rech. По архитектуре из
// он не единственный на будущее: IR специально не содержит
// ничего C-специфичного, чтобы позже рядом можно было добавить другой
// backend без изменения lexer/parser/ast/semantic/ir. internal/interp -
// уже пример второго потребителя того же IR, не C backend'а.
package cbackend

import (
	"strings"
	"unicode"

	"github.com/IlyaFeoktistov/Rech/internal/backend/cbackend/runtime/process"
	"github.com/IlyaFeoktistov/Rech/internal/ir"
)

// entryPointName - функция с этим именем становится точкой входа программы
// (C main). Остальные имена транслитерируются в валидные C-идентификаторы.
const entryPointName = "главная"

// translit - таблица транслитерации кириллицы для имён функций и переменных.
// Не претендует на лингвистическую точность (это не морфология, см.
// ), задача - дать читаемый и уникальный C-идентификатор.
var translit = map[rune]string{
	'а': "a", 'б': "b", 'в': "v", 'г': "g", 'д': "d", 'е': "e", 'ё': "yo",
	'ж': "zh", 'з': "z", 'и': "i", 'й': "j", 'к': "k", 'л': "l", 'м': "m",
	'н': "n", 'о': "o", 'п': "p", 'р': "r", 'с': "s", 'т': "t", 'у': "u",
	'ф': "f", 'х': "h", 'ц': "c", 'ч': "ch", 'ш': "sh", 'щ': "sch", 'ъ': "",
	'ы': "y", 'ь': "", 'э': "e", 'ю': "yu", 'я': "ya",

	// Символы generic-инстанциации ("Список<Целое>", "Пара<Целое, Строка>") -
	// имя типа с параметром кодируется как строка в AST (docs, "Синтаксис
	// типа с параметром") и мангируется тем же transliterate, что и обычные
	// имена; без этих записей "<"/">"/","/" " попали бы в C-идентификатор
	// как есть, что невалидно. Пробел - только из разделителя между
	// несколькими аргументами типа (", "), сам по себе не встречается ни в
	// одном обычном имени - можно просто вырезать, не заменяя ни на что.
	'<': "_of_", '>': "", ',': "_", ' ': "",

	// "[N]" - массив фиксированной длины ("Целое[3]") - "_X3_": заглавная
	// буква не встречается в остальном мангл-имени (оно в нижнем регистре).
	'[': "_X", ']': "_",

	// "?" - префикс nullable-типа ("?Целое", ir.NullableType) - тем же
	// принципом, что "<"/">" выше.
	'?': "n_",

	// "(", ")" - мангл-имя конкретной перегрузки ("сложить(Целое,Целое)",
	// см. overloadMangledName, internal/semantic/overload.go) - тем же
	// принципом, что "<"/">" выше, разделитель между именем и списком типов
	// параметров ("__") отличен от разделителя generic-аргументов ("_of_"),
	// чтобы перегрузка функции с именем, оканчивающимся на "_of_..." (уже
	// смангленный generic-вызов), не могла случайно дать такую же строку,
	// как перегрузка обычной функции.
	'(': "__", ')': "",

	// Символы перегрузки оператора ("оператор+"/"оператор=="/... -
	// ast.OperatorMethodName, internal/semantic/operator.go) - метод с
	// таким именем мангируется тем же mangleMethod, что и обычный
	// статический метод, эти символы иначе попали бы в C-идентификатор как
	// есть (невалидно). "<"/">" не дублируются здесь - уже покрыты записями
	// выше (generic-скобки), символ там же используется и для операторов
	// "<"/">"/"<="/">=" - коллизии с настоящим generic-именем нет, метод
	// (в отличие от типа) никогда не мангируется вместе с generic-
	// аргументами.
	'+': "_plus_", '-': "_minus_", '*': "_mul_", '/': "_div_", '%': "_mod_",
	'=': "_eq_", '!': "_not_",
}

// ASCIIName - имя из латиницы, цифр, "_" и "-" для мест вне C-кода, где
// кириллица недопустима (имя юнита systemd, службы Windows): кириллица -
// по таблице translit, остальные символы - "_".
func ASCIIName(name string) string {
	var sb strings.Builder
	for _, r := range strings.ToLower(name) {
		switch {
		case isCyrillic(r):
			sb.WriteString(translit[r])
		case r < unicode.MaxASCII && (unicode.IsLetter(r) || unicode.IsDigit(r) || r == '_' || r == '-'):
			sb.WriteRune(r)
		default:
			sb.WriteByte('_')
		}
	}
	return sb.String()
}

// transliterate - C-идентификатор для Rech-имени. Имя режется на сегменты
// по служебным символам (generic-скобки, "?", скобки перегрузки,
// операторы - всё, что в translit, кроме кириллицы): сегмент с кириллицей
// транслитерируется в нижний регистр, сегмент БЕЗ кириллицы, но с
// латинской буквой - остаётся как написан, с маркером latinMarker впереди.
// Транслитерация кириллицы никогда не даёт заглавных букв, поэтому
// латинское "f" (EN_f) не совпадёт с кириллическим "ф" (f).
func transliterate(name string) string {
	var sb strings.Builder
	start := 0
	flush := func(end int) {
		seg := name[start:end]
		if seg == "" {
			return
		}
		if isLatinSegment(seg) {
			sb.WriteString(latinMarker)
			sb.WriteString(seg)
			return
		}
		for _, r := range strings.ToLower(seg) {
			if lat, ok := translit[r]; ok {
				sb.WriteString(lat)
				continue
			}
			sb.WriteRune(r)
		}
	}
	for i, r := range name {
		sep, ok := translit[r]
		if !ok || isCyrillic(r) {
			continue
		}
		flush(i)
		sb.WriteString(sep)
		start = i + len(string(r))
	}
	flush(len(name))
	return sb.String()
}

// latinMarker - префикс сегмента имени без кириллицы (см. transliterate).
const latinMarker = "EN_"

func isCyrillic(r rune) bool {
	return unicode.Is(unicode.Cyrillic, r)
}

// isLatinSegment - в сегменте нет кириллицы, но есть латинская буква
// (сегмент только из цифр/"_" маркера не получает).
func isLatinSegment(seg string) bool {
	hasLatin := false
	for _, r := range seg {
		if isCyrillic(r) {
			return false
		}
		if r < 128 && unicode.IsLetter(r) {
			hasLatin = true
		}
	}
	return hasLatin
}

func mangleName(name string) string {
	if name == entryPointName {
		return "main"
	}
	return "rech_" + transliterate(name)
}

// funcCName - C-имя функции: настоящее имя как есть (никакой
// транслитерации/префикса "rech_"), если функция помечена
// "экспортировать" (docs/SPEC.md, "Экспорт в разделяемую библиотеку") -
// потребитель разделяемой библиотеки обязан видеть тот C-символ, который
// сам автор написал в Rech (`применить`, не `rech_primenit`), чтобы
// подключить его буквально тем же именем ("внешняя функция применить"),
// без переименования через "как". GCC/Clang принимают произвольный UTF-8
// в идентификаторах без специальных флагов (проверено живьём), ELF/PE
// динамические таблицы символов - тоже (обычная последовательность байт,
// не только ASCII) - технических препятствий для этого нет ни на одной из
// платформ, которые уже поддерживает internal/toolchain. Неэкспортированная
// функция - как и раньше, mangleName (внутренняя деталь реализации, никогда
// не видна снаружи, коллизии внутренних имён это не касается).
func funcCName(ctx *irGenCtx, name string) string {
	if ctx.exportedFuncNames[name] {
		return name
	}
	if name == entryPointName && ctx.service {
		return process.ServiceBodyName
	}
	return mangleName(name)
}

// entityCName - C-имя (typedef) сущности, тем же принципом, что и
// funcCName выше, - настоящее имя для сущности, помеченной
// "экспортировать", иначе mangleType.
func entityCName(ctx *irGenCtx, name string) string {
	if ctx.exportedEntityNames[name] {
		return name
	}
	return mangleType(name)
}

func mangleVar(name string) string {
	return "v_" + transliterate(name)
}

// mangleType мангирует имя сущности в C typedef-имя. Отдельный префикс
// (rech_T_, не rech_ у функций) - в C typedef-имена и имена функций делят
// один неймспейс, разные префиксы структурно исключают коллизию сущности
// и функции с одним и тем же именем без отдельной проверки.
func mangleType(name string) string {
	return "rech_T_" + transliterate(name)
}

// mangleField мангирует имя поля сущности в имя члена C-структуры.
// Отдельный префикс от mangleVar (f_, не v_) - чисто для читаемости
// сгенерированного C, коллизии с переменными в C всё равно невозможны
// (члены структуры - собственный неймспейс).
func mangleField(name string) string {
	return "f_" + transliterate(name)
}

// exportedCtorCSymbol - настоящее C-имя экспортированного конструктора
// (без транслитерации) - тем же принципом, что exportedMethodCSymbol выше
// (общая формула для ctorCName и для проверки коллизий, mangling.go).
func exportedCtorCSymbol(entity string) string {
	return entity + "_новый"
}

// ctorCName - C-имя функции, выделяющей объект ("rech_new_X" по умолчанию,
// mangleCtor) - настоящее exportedCtorCSymbol, если конструктор сущности
// помечен "экспортировать" (docs/SPEC.md, "Экспорт в разделяемую
// библиотеку") - тем же принципом, что и methodCName выше. Автор явно
// решает экспорт конструктора отдельно от экспорта самой сущности - в
// отличие от метода, конструктор нельзя вызвать снаружи по умолчанию вовсе.
func ctorCName(ctx *irGenCtx, entity string) string {
	if ctx.exportedCtors[entity] {
		return exportedCtorCSymbol(entity)
	}
	return mangleCtor(entity)
}

// mangleCtor мангирует имя ссылочной сущности в имя её C-конструктора
// (GC_MALLOC + заполнение полей, см. generateIREntityCtor) - отдельный
// префикс от mangleType/mangleName по тому же принципу, что и у остальных
// mangle*: разные пространства имён внутри одного C-неймспейса структурно
// исключают коллизию.
func mangleCtor(name string) string {
	return "rech_new_" + transliterate(name)
}

// mangleCtorInit мангирует имя ссылочной сущности с явным конструктором
// ("конструктор(...) { ... }") в имя C-функции, которая исполняет ЕГО ТЕЛО
// на уже существующем указателе (не выделяет память сама) - отдельная от
// mangleCtor (та выделяет память через GC_MALLOC и вызывает эту функцию),
// чтобы "родитель(...)" производного конструктора мог вызвать ровно тело
// базового на том же объекте, не создавая новый (см.
// generateIREntityCtorInit).
func mangleCtorInit(name string) string {
	return "rech_init_" + transliterate(name)
}

// mangleMethod мангирует пару (сущность, метод) в имя C-функции. Отдельный
// префикс (rech_M_, не rech_ у функций и не rech_T_ у типов) - в C
// typedef-имена и имена функций делят один неймспейс, разные префиксы
// структурно исключают коллизию метода с функцией/типом того же имени.
func mangleMethod(entity, method string) string {
	return "rech_M_" + transliterate(entity) + "_" + transliterate(method)
}

// exportedMethodCSymbol - настоящее C-имя экспортированного метода (без
// транслитерации) - общая формула для methodCName ниже и для проверки
// коллизий (mangling.go, checkManglingCollisions), чтобы обе стороны не
// могли разойтись.
func exportedMethodCSymbol(entity, method string) string {
	return entity + "_" + method
}

// buildExportedMethods собирает "сущность -> метод -> экспортирован" из
// prog.Entities - общий код для обеих точек построения irGenCtx
// (generateHeader/generateSource), чтобы формула не могла разойтись.
func buildExportedMethods(prog *ir.Program) map[string]map[string]bool {
	out := map[string]map[string]bool{}
	for _, ent := range prog.Entities {
		for _, m := range ent.Methods {
			if !m.Exported {
				continue
			}
			if out[ent.Name] == nil {
				out[ent.Name] = map[string]bool{}
			}
			out[ent.Name][m.Name] = true
		}
	}
	return out
}

// buildExportedCtors собирает "сущность -> конструктор экспортирован" из
// prog.Entities - тем же принципом, что и buildExportedMethods выше.
func buildExportedCtors(prog *ir.Program) map[string]bool {
	out := map[string]bool{}
	for _, ent := range prog.Entities {
		if ent.Ctor != nil && ent.Ctor.Exported {
			out[ent.Name] = true
		}
	}
	return out
}

// methodCName - C-имя метода: exportedMethodCSymbol, если метод помечён
// "экспортировать" (docs/SPEC.md, "Экспорт в разделяемую библиотеку") -
// тем же принципом, что и funcCName/entityCName выше. Неэкспортированный
// метод - как и раньше, mangleMethod.
func methodCName(ctx *irGenCtx, entity, method string) string {
	if ctx.exportedMethods[entity][method] {
		return exportedMethodCSymbol(entity, method)
	}
	return mangleMethod(entity, method)
}

// mangleStaticField мангирует пару (сущность, статическое поле) в имя
// C-глобальной переменной (см. package doc ir.Entity.StaticFields,
// internal/backend/cbackend/statics.go) - отдельный префикс (rech_S_) тем же
// принципом, что и у mangleMethod/mangleField.
func mangleStaticField(entity, field string) string {
	return "rech_S_" + transliterate(entity) + "_" + transliterate(field)
}
