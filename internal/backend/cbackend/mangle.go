// Package cbackend транслирует internal/ir.Program (см. package doc
// internal/ir) в исходник на C.
//
// Это первый и на сегодня единственный backend Rech. По архитектуре из
// docs/ROADMAP.md он не единственный на будущее: IR специально не содержит
// ничего C-специфичного, чтобы позже рядом можно было добавить другой
// backend без изменения lexer/parser/ast/semantic/ir. internal/interp -
// уже пример второго потребителя того же IR, не C backend'а.
package cbackend

import "strings"

// entryPointName - функция с этим именем становится точкой входа программы
// (C main). Остальные имена транслитерируются в валидные C-идентификаторы.
const entryPointName = "главная"

// translit - таблица транслитерации кириллицы для имён функций и переменных.
// Не претендует на лингвистическую точность (это не морфология, см.
// docs/ROADMAP.md), задача - дать читаемый и уникальный C-идентификатор.
var translit = map[rune]string{
	'а': "a", 'б': "b", 'в': "v", 'г': "g", 'д': "d", 'е': "e", 'ё': "e",
	'ж': "zh", 'з': "z", 'и': "i", 'й': "j", 'к': "k", 'л': "l", 'м': "m",
	'н': "n", 'о': "o", 'п': "p", 'р': "r", 'с': "s", 'т': "t", 'у': "u",
	'ф': "f", 'х': "h", 'ц': "c", 'ч': "ch", 'ш': "sh", 'щ': "sch", 'ъ': "",
	'ы': "y", 'ь': "", 'э': "e", 'ю': "yu", 'я': "ya",

	// Символы generic-инстанциации ("Список<Целое>", шаг 4.2) - имя типа с
	// параметром кодируется как строка в AST (docs, "Синтаксис типа с
	// параметром") и мангируется тем же transliterate, что и обычные имена;
	// без этих записей "<"/">"/"," попали бы в C-идентификатор как есть,
	// что невалидно.
	'<': "_of_", '>': "", ',': "_",
}

func transliterate(name string) string {
	var sb strings.Builder
	for _, r := range strings.ToLower(name) {
		if lat, ok := translit[r]; ok {
			sb.WriteString(lat)
			continue
		}
		sb.WriteRune(r)
	}
	return sb.String()
}

func mangleName(name string) string {
	if name == entryPointName {
		return "main"
	}
	return "rech_" + transliterate(name)
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
