// Package structtag разбирает тег поля сущности/структуры - текст между
// обратными кавычками после объявления поля ("имя: Строка `json:name`",
// см. internal/lexer, токен TAG) - набор пар "ключ:значение", разделённых
// запятой ("json:name, sql:user_name"). В отличие от тега Go значение не
// берётся в кавычки (в самом теге и так нет пробелов внутри значения,
// экранировать нечего) - запятая как разделитель пар выбрана вместо
// пробела именно поэтому: без кавычек пробел не отличил бы конец одного
// значения от начала следующего ключа так же надёжно.
package structtag

import "strings"

// Tag - разобранный тег: ключ -> значение. Пустой (не nil, но len == 0) у
// поля без тега - Get на нём просто ничего не находит.
type Tag map[string]string

// Parse разбирает сырой текст тега (raw - как лексер вернул его в
// Token.Value, без окружающих обратных кавычек). Незнакомые ключи не
// отбрасываются - какие ключи имеют смысл, решает вызывающий код (JSON -
// "json", БД - "sql"), сам тег - нейтральное хранилище пар.
func Parse(raw string) Tag {
	if raw == "" {
		return nil
	}
	out := Tag{}
	for part := range strings.SplitSeq(raw, ",") {
		key, value, ok := strings.Cut(strings.TrimSpace(part), ":")
		if !ok {
			continue
		}
		key = strings.TrimSpace(key)
		value = strings.TrimSpace(value)
		if key == "" || value == "" {
			continue
		}
		out[key] = value
	}
	return out
}

// Get возвращает значение key, "" - тега нет или ключ в нём не задан.
func (t Tag) Get(key string) string {
	return t[key]
}

// XMLName - внешнее имя поля fieldName в XML по ключу "xml" тега raw и
// признак атрибута: "xml:имя" - дочерний элемент с этим именем,
// "xml:@имя" - атрибут элемента сущности с этим именем, "xml:@" - атрибут
// с именем самого поля; без ключа "xml" - дочерний элемент с именем поля.
func XMLName(raw, fieldName string) (name string, attr bool) {
	name = Parse(raw).Get("xml")
	if rest, ok := strings.CutPrefix(name, "@"); ok {
		name, attr = rest, true
	}
	if name == "" {
		name = fieldName
	}
	return name, attr
}
