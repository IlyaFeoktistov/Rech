package parser

import "github.com/IlyaFeoktistov/Rech/internal/diagnostics"

// Коды ошибок парсера (RECH2xxx) - по категориям грамматики, не 1:1 на
// сообщение (см. package doc internal/diagnostics/codes.go).
var (
	CodeUnexpectedToken  = diagnostics.Register("RECH2001", "неожиданный токен / несовпадение ожидаемой грамматики")
	CodeMissingAfterKw   = diagnostics.Register("RECH2002", "не хватает обязательного токена после ключевого слова/имени")
	CodeUnclosedBracket  = diagnostics.Register("RECH2003", "незакрытая скобка/висячая запятая")
	CodeModifierAgree    = diagnostics.Register("RECH2004", "рассогласование рода/числа модификатора")
	CodeDuplicateDecl    = diagnostics.Register("RECH2005", "повторное объявление там, где допустимо только одно")
	CodeEntryPoint       = diagnostics.Register("RECH2006", "нарушение ограничений точки входа (\"главная\")")
	CodeNaturalCall      = diagnostics.Register("RECH2007", "падежное сопоставление аргументов естественного вызова")
	CodeExport           = diagnostics.Register("RECH2008", "нарушение правил \"экспортировать\"")
	CodeEntityInvariant  = diagnostics.Register("RECH2009", "нарушение инварианта сущности/структуры/перечисления/оператора")
	CodeContextRestrict  = diagnostics.Register("RECH2010", "\"этот\"/\"родитель\" вне допустимого контекста")
	CodeStmtRestrict     = diagnostics.Register("RECH2011", "нарушение ограничения на уровне инструкции")
	CodeInvalidLiteral   = diagnostics.Register("RECH2012", "некорректный литерал")
	CodeConditionalBuild = diagnostics.Register("RECH2013", "ошибка в блоке условной компиляции (\"#если\")")
	CodeSwitch           = diagnostics.Register("RECH2014", "ошибка в \"выбрать\"/\"когда\"")
	CodeTemplate         = diagnostics.Register("RECH2015", "ошибка в шаблонном литерале")
)

// errAt строит диагностику парсера с позицией (в рунах - см. package doc
// internal/lexer.Lexer про Line/Col). Возвращает *diagnostics.Diagnostic, не
// error - удовлетворяет error в обычном "return nil, errAt(...)"
// (интерфейс реализован указателем) и одновременно позволяет вызывающей
// стороне дописать подсказку цепочкой errAt(...).WithHint(...) там, где она
// есть (например, модификаторы согласования рода - см. entity.go/
// modvisibility.go).
func errAt(line, col int, code, format string, args ...any) *diagnostics.Diagnostic {
	return diagnostics.New(code, diagnostics.Position{Line: line, Col: col}, format, args...)
}

// errAtLine - errAt без известной колонки.
func errAtLine(line int, code, format string, args ...any) *diagnostics.Diagnostic {
	return diagnostics.New(code, diagnostics.Position{Line: line}, format, args...)
}
