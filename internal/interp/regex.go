// "регулярное(строка, шаблон): Список<Совпадение>" - поиск ВСЕХ
// непересекающихся вхождений regexp-шаблона (движок Go regexp, RE2 -
// линейное время, без backtracking) - см. package doc
// internal/backend/cbackend/regex.go для эквивалентной реализации на C
// (POSIX regex.h) и её ограничений относительно RE2.
package interp

import (
	"regexp"
	"strings"
	"unicode/utf8"

	"github.com/IlyaFeoktistov/Rech/internal/ir"
	"github.com/IlyaFeoktistov/Rech/internal/rtmsg"
)

// regexGroupsType/regexResultType - см. internal/semantic/regex.go -
// имена generic-инстанциаций, зарегистрированных для "регулярное"
// принудительно (registerRegexTypesOnCall), тем же принципом, что
// splitResultType у stringsplit.go.
const (
	regexGroupsType = "Список<Строка>"
	regexResultType = "Список<Совпадение>"
)

// regexFindAll - "регулярное" - компилирует шаблон и ищет все
// непересекающиеся совпадения в s (regexp.FindAllStringSubmatchIndex -
// байтовые индексы каждого совпадения и каждой скобочной группы за один
// проход, без повторной компиляции/поиска на группу).
func regexFindAll(s, pattern string) Value {
	re, err := regexp.Compile(regexTranslatePattern(pattern))
	if err != nil {
		panicRuntime("%s", rtmsg.RegexInvalidPatternFormat(pattern, err.Error()))
	}
	matches := re.FindAllStringSubmatchIndex(s, -1)
	elems := make([]Value, len(matches))
	for i, m := range matches {
		elems[i] = regexMatchValue(s, m)
	}
	return Value{Type: ir.EntityType(regexResultType), Obj: &entityObj{typeName: regexResultType, coll: &collection{template: "Список", elems: elems}}}
}

// regexTranslatePattern переводит "\w"/"\W" в Unicode-осознанные классы
// ("[\p{L}\p{N}_]"/"[^\p{L}\p{N}_]") ДО regexp.Compile - у Go regexp/RE2
// "\w" по умолчанию ASCII-only ([0-9A-Za-z_]), не видит кириллицу вовсе,
// независимо от локали ОС (RE2 не завязан на неё). См. package doc
// internal/backend/cbackend/regex.go для того же перевода на стороне POSIX
// ("[[:alnum:]_]", locale-осознанный под "C.UTF-8" - там "\w" и без
// перевода locale-осознан, но остаётся согласованность, раз это ОДНО и то
// же понятие "буква/цифра/подчёркивание с учётом Unicode" в обоих местах).
// "\d"/"\D" НЕ переводятся здесь - Go regexp "\d" уже ASCII-only ([0-9]),
// тем же множеством, в которое POSIX-сторона переводит "\d" сама
// (rech_regex_translate_pattern) - совпадение уже есть без вмешательства.
func regexTranslatePattern(pattern string) string {
	var b strings.Builder
	for i := 0; i < len(pattern); i++ {
		if pattern[i] == '\\' && i+1 < len(pattern) {
			switch pattern[i+1] {
			case 'w':
				b.WriteString(`[\p{L}\p{N}_]`)
			case 'W':
				b.WriteString(`[^\p{L}\p{N}_]`)
			default:
				b.WriteByte(pattern[i])
				b.WriteByte(pattern[i+1])
			}
			i++
			continue
		}
		b.WriteByte(pattern[i])
	}
	return b.String()
}

// regexMatchValue строит одну сущность "Совпадение" из одной записи
// FindAllStringSubmatchIndex - m[0]/m[1] - границы всего совпадения,
// m[2*i]/m[2*i+1] (i >= 1) - границы i-й скобочной группы (-1/-1, если
// группа не участвовала в совпадении, например необязательная часть
// шаблона - тогда "группы[i-1]" пустая строка, не null, см. package doc
// matchFields).
func regexMatchValue(s string, m []int) Value {
	start := int64(utf8.RuneCountInString(s[:m[0]]))
	groupCount := len(m)/2 - 1
	groupElems := make([]Value, groupCount)
	for g := range groupCount {
		lo, hi := m[2*(g+1)], m[2*(g+1)+1]
		if lo < 0 {
			groupElems[g] = stringValue("")
			continue
		}
		groupElems[g] = stringValue(s[lo:hi])
	}
	groups := Value{Type: ir.EntityType(regexGroupsType), Obj: &entityObj{typeName: regexGroupsType, coll: &collection{template: "Список", elems: groupElems}}}
	fields := map[string]Value{
		"текст":  stringValue(s[m[0]:m[1]]),
		"начало": intValue(start),
		"группы": groups,
	}
	return Value{Type: ir.EntityType("Совпадение"), Obj: &entityObj{typeName: "Совпадение", fields: fields}}
}
