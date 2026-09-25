// Hover для перечислений (объявленных в документе - d.astEnums, см.
// document.go/multifile.go - и встроенных: HTTPКод/ДеньНедели/Месяц,
// semantic.StandardEnums) - раньше не поддержан вовсе: имя перечисления -
// не сущность (d.entities/resolveChain о нём не знают), поэтому и
// "HTTPКод" само по себе, и "HTTPКод.ОК" (чтение члена) не показывали
// hover ничего - найдено по прямой просьбе пользователя ("http коды ответа
// тоже не задокументированы").
package lsp

import (
	"fmt"
	"sort"

	"github.com/IlyaFeoktistov/Rech/internal/semantic"
)

// enumMembers - члены перечисления name (имя как оно написано В ЭТОМ
// документе - свои и импортированные никогда не манглятся при линковке,
// см. multifile.go/applyLink, поэтому d.astEnums уже содержит их под
// бare-именем без отдельных алиасов, в отличие от astEntities/astFuncs) -
// nil, false, если name не перечисление вовсе (ни своё, ни встроенное).
func (d *doc) enumMembers(name string) (map[string]int64, bool) {
	if ed, ok := d.astEnums[name]; ok {
		out := make(map[string]int64, len(ed.Members))
		for _, m := range ed.Members {
			out[m.Name] = m.Value
		}
		return out, true
	}
	if members, ok := semantic.StandardEnums[name]; ok {
		return members, true
	}
	return nil, false
}

// hoverEnumType - "перечисление ИмяЕё" - курсор на самом имени
// перечисления (не на члене).
func (d *doc) hoverEnumType(name string) (string, bool) {
	members, ok := d.enumMembers(name)
	if !ok {
		return "", false
	}
	names := make([]string, 0, len(members))
	for m := range members {
		names = append(names, m)
	}
	sort.Slice(names, func(i, j int) bool { return members[names[i]] < members[names[j]] })
	lines := make([]string, len(names))
	for i, m := range names {
		lines[i] = fmt.Sprintf("%s = %d", m, members[m])
	}
	block := codeBlock(fmt.Sprintf("перечисление %s\n{\n    %s\n}", name, joinWithComma(lines)))
	if ed, ok := d.astEnums[name]; ok {
		block = withDoc(block, ed.Doc)
	}
	return block, true
}

// hoverEnumMember - "ИмяЕё.Член: Целое = значение" - курсор на самом члене
// (последнее звено цепочки "ИмяЕё.Член").
func (d *doc) hoverEnumMember(enumName, memberName string) (string, bool) {
	members, ok := d.enumMembers(enumName)
	if !ok {
		return "", false
	}
	value, ok := members[memberName]
	if !ok {
		return "", false
	}
	return codeBlock(fmt.Sprintf("%s.%s: Целое = %d", enumName, memberName, value)), true
}

func joinWithComma(lines []string) string {
	out := ""
	for i, l := range lines {
		if i > 0 {
			out += ",\n    "
		}
		out += l
	}
	return out
}
