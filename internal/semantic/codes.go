package semantic

import "github.com/IlyaFeoktistov/Rech/internal/diagnostics"

// Коды ошибок семантического анализа (RECH3xxx) - по категориям, не 1:1 на
// сообщение (см. package doc internal/diagnostics/codes.go). В отличие от
// парсера, категории здесь смешивают "вид ошибки" (необъявленное имя,
// повторное объявление - сквозные, встречаются в любом файле пакета) и
// "подсистему" (интерфейсы/исключения/экспорт/... - у остальных сообщений
// пакета естественная группировка - файл, в котором они уже физически
// разделены по теме).
var (
	// Сквозные - проверяются раньше, чем принадлежность к файлу.
	CodeNameResolution = diagnostics.Register("RECH3001", "необъявленное имя (переменная/функция/тип/метод/поле)")
	CodeDuplicateDecl  = diagnostics.Register("RECH3002", "повторное объявление там, где допустимо только одно")

	// По подсистеме (файлу).
	CodeTypeMismatch   = diagnostics.Register("RECH3101", "несовпадение типов / нужно явное приведение")
	CodeTypeResolution = diagnostics.Register("RECH3110", "разрешение имени типа/generic-коллекции")
	CodeStmtCheck      = diagnostics.Register("RECH3130", "проверка на уровне инструкции")
	CodeExprCheck      = diagnostics.Register("RECH3140", "проверка на уровне выражения/встроенной функции/оператора")
	CodeEntityDecl     = diagnostics.Register("RECH3210", "объявление сущности/структуры/функции верхнего уровня")
	CodeInterface      = diagnostics.Register("RECH3220", "объявление интерфейса")
	CodeInheritance    = diagnostics.Register("RECH3230", "наследование/переопределение")
	CodeNullable       = diagnostics.Register("RECH3320", "nullable-тип (\"?Тип\")")
	CodePointer        = diagnostics.Register("RECH3330", "указатель (\"Указатель<T>\")")
	CodeStateVerb      = diagnostics.Register("RECH3340", "состояния/противоположный глагол")
	CodeFlow           = diagnostics.Register("RECH3700", "анализ потока управления (недостижимый код/вернуть)")
	CodeGenerics       = diagnostics.Register("RECH3401", "generics")
	CodeCallable       = diagnostics.Register("RECH3430", "тип \"Вызываемый(...)\"")
	CodeException      = diagnostics.Register("RECH3510", "исключения (попытаться/поймать/выбросить)")
	CodeExportSemantic = diagnostics.Register("RECH3610", "семантика \"экспортировать\"/внешних объявлений")
	CodeBuiltinTarget  = diagnostics.Register("RECH3801", "цель JSON/XML/YAML/PROTOBUF/строки БД (разобрать/сериализовать/сканировать)")
	CodeConstEval      = diagnostics.Register("RECH3910", "вычисление константы/значения по умолчанию")
	CodeSemanticOther  = diagnostics.Register("RECH3990", "прочая семантическая проверка")
)

// currentFile - путь исходного файла, чьё верхнеуровневое объявление
// (сущность/функция) сейчас разбирает Analyzer - проставляется перед
// каждой итерацией циклов по prog.Entities/prog.Functions в
// collectEntities/collectFuncSigs/buildIR (analyzer.go, единственные места
// присваивания). Читается errAtLine/errNoPos при создании диагностики -
// альтернатива протаскиванию File аргументом через все ~400 мест создания
// диагностик пакета. Безопасно как простая (не потокобезопасная)
// глобальная переменная: semantic.Analyze - последовательный проход по
// ОДНОЙ программе, без параллелизма, и каждая итерация цикла полностью
// завершается (включая любое создание диагностики внутри неё) до начала
// следующей.
//
// Не у каждого верхнеуровневого объявления есть File (ast.InterfaceDecl/
// EnumDecl/ConstDecl/ExternFuncDecl/ExternEntityDecl его не несут, в
// отличие от FunctionDecl/EntityDecl, см. ast.StampFile) - диагностики
// внутри их разбора наследуют currentFile от последнего объявления, для
// которого он был известен: для однофайловых программ (подавляющее
// большинство) это всегда верный файл; для программы с "использовать" -
// приемлемое приближение (точнее было бы разрешено расширением File на
// эти типы ast, что здесь сознательно не сделано - самостоятельная задача,
// не часть переноса существующих сообщений на диагностику).
var currentFile string

// errAtLine строит диагностику семантики с позицией (только строка -
// семантика не сохраняет столбец у большинства узлов AST/IR, в отличие от
// лексера/парсера) и currentFile (см. package doc выше). Возвращает
// *diagnostics.Diagnostic, не error - удовлетворяет error в обычном
// "return nil, errAtLine(...)" (интерфейс реализован указателем) и
// одновременно позволяет вызывающей стороне дописать подсказку цепочкой
// errAtLine(...).WithHint(...) там, где она есть (см. package doc
// internal/semantic/suggest.go) - без этого пришлось бы заводить отдельную
// errAtLineHint-функцию только ради двух-трёх мест с подсказкой.
func errAtLine(line int, code, format string, args ...any) *diagnostics.Diagnostic {
	return diagnostics.New(code, diagnostics.Position{File: currentFile, Line: line}, format, args...)
}

// errNoPos - errAtLine без номера строки (её сейчас неоткуда взять - см.
// package doc internal/diagnostics.Position.Line, Render просто не
// печатает сниппет), но с тем же currentFile - позволяет хотя бы назвать
// файл в заголовке диагностики.
func errNoPos(code, format string, args ...any) *diagnostics.Diagnostic {
	return diagnostics.New(code, diagnostics.Position{File: currentFile}, format, args...)
}
