// disp/dispType - показ имени сущности/функции/типа В HOVER/COMPLETION ТАК,
// КАК ЕГО ПИШЕТ ЭТОТ ФАЙЛ - не как оно хранится внутри уже слинкованной
// программы (см. multifile.go, package doc про module.Program.Link):
// сущность/функция ИЗ ДРУГОГО файла получает при слиянии смангленное имя
// ("Логгер_ФайловыйЛоггер" вместо "ФайловыйЛоггер") - реального
// соответствия этому имени в исходнике Rech нет вообще, ни в одном файле,
// показывать его пользователю неверно (найдено при живой проверке hover на
// examples/проекты/http_сервер - параметр "контейнер: Контейнер" hover
// показывал как "контейнер: Контейнер_Контейнер").
package lsp

import "github.com/IlyaFeoktistov/Rech/internal/ir"

// disp возвращает name, каким его видит ЭТОТ файл, если name смангленное
// (d.displayName - заполняется только при успешной линковке, см.
// applyLink) - иначе name без изменений (свои объявления никогда не
// манглятся, d.displayName для них пуст - тождество работает само собой).
func (d *doc) disp(name string) string {
	if alias, ok := d.displayName[name]; ok {
		return alias
	}
	return name
}

// dispType - как ir.Type.String(), но с именем сущности/интерфейса,
// пропущенным через disp - остальные виды типа (примитивы, generic-
// коллекции и т. п.) не несут смангленного имени сущности из ДРУГОГО файла
// в этой же позиции, поэтому не нуждаются в подмене. Callable/Nullable/
// Pointer, чьё .Entity - уже готовая составная строка (не голое имя
// сущности - см. ir.Type.String()), сознательно не разбираются здесь: их
// возможная смангленность внутри - известное, более редкое ограничение
// этого среза, не забытый случай.
func (d *doc) dispType(t ir.Type) string {
	switch t.Kind {
	case ir.KindEntity, ir.KindInterface, ir.KindExternEntity:
		return d.disp(t.Entity)
	default:
		return t.String()
	}
}
