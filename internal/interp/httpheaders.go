package interp

import (
	"strings"

	"github.com/IlyaFeoktistov/Rech/internal/ir"
)

// splitHeaderLine - одна строка "заголовки" ("имя: значение", без
// пробела перед ":", тем же форматом, что пишет rech_http_send_response/
// sendHTTPResponse) -> (имя без изменений регистра, обрезанное по
// пробелам значение, ok). Общая часть headerValue/headerValues ниже.
func splitHeaderLine(line string) (name, value string, ok bool) {
	idx := strings.IndexByte(line, ':')
	if idx < 0 {
		return "", "", false
	}
	return line[:idx], strings.TrimSpace(line[idx+1:]), true
}

// headerValue - "значение_заголовка(заголовки, имя): ?Строка" - ищет
// строку вида "имя: значение" в заголовках ("\r\n"-разделённых, тем же
// форматом, что "заголовки" у Запрос/Ответ) - ПЕРВОЕ совпадение по
// порядку появления. Совпадение имени - БЕЗ УЧЁТА РЕГИСТРА (RFC 7230
// §3.2 - имя поля заголовка регистронезависимо) - до 2026-09-20 было
// точным (тот же принцип, что у cookieValue/"значение_куки", где имя
// COOKIE, а не HTTP-заголовка, регистрозависимо по RFC 6265 - это
// осталось верным) - живая находка при аудите поддержки заголовков:
// клиент, приславший "content-type" строчными, не находился при поиске
// "Content-Type".
func headerValue(headers, name string) Value {
	nullableType := ir.NullableType(ir.TypeString)
	for line := range strings.SplitSeq(headers, "\r\n") {
		lineName, value, ok := splitHeaderLine(line)
		if !ok || !strings.EqualFold(lineName, name) {
			continue
		}
		v := stringValue(value)
		return Value{Type: nullableType, Nullable: &v}
	}
	return Value{Type: nullableType}
}

// headerValues - "значения_заголовка(заголовки, имя): Список<Строка>" -
// ВСЕ совпадения по порядку появления (пустой список, не Ничто/паника,
// если ни одного) - настоящие HTTP-заголовки МОГУТ повторяться
// (`X-Forwarded-For`, несколько `Accept` и т. п.) - headerValue выше
// отдаёт только первое. headerValuesResultType - см. internal/semantic/
// httpheaders.go, headerValuesResultType - та же generic-инстанциация
// "Список<Строка>", что у "разбить" (splitResultType, stringsplit.go,
// идентичная строка - resolveTypeName идемпотентен).
func headerValues(headers, name string) Value {
	var elems []Value
	for line := range strings.SplitSeq(headers, "\r\n") {
		lineName, value, ok := splitHeaderLine(line)
		if !ok || !strings.EqualFold(lineName, name) {
			continue
		}
		elems = append(elems, stringValue(value))
	}
	return Value{Type: ir.EntityType(splitResultType), Obj: &entityObj{typeName: splitResultType, coll: &collection{template: "Список", elems: elems}}}
}
