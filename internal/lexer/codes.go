package lexer

import "github.com/IlyaFeoktistov/Rech/internal/diagnostics"

// Коды ошибок лексера (RECH1xxx) - см. package doc
// internal/diagnostics/codes.go про категории, не 1:1 на сообщение:
// незакрытая строка/символьный литерал/литерал в обратных кавычках/
// перенос строки внутри них - всё "литерал не закрыт или некорректен",
// не отдельный код на каждую комбинацию.
var (
	CodeUnexpectedChar    = diagnostics.Register("RECH1001", "неожиданный символ")
	CodeUnclosedComment   = diagnostics.Register("RECH1002", "комментарий не закрыт")
	CodeBadLiteral        = diagnostics.Register("RECH1003", "литерал не закрыт или некорректен")
	CodeBadEscape         = diagnostics.Register("RECH1004", "неизвестная последовательность экранирования")
	CodeBadEncoding       = diagnostics.Register("RECH1005", "исходный файл не в допустимой кодировке/нормализации Unicode")
	CodeCommentInTemplate = diagnostics.Register("RECH1006", "комментарий внутри вставки шаблонной строки")
)

// errAt строит диагностику лексера с позицией (line, col) - в отличие от
// внутреннего l.line/l.col (текущая позиция лексера, могла уйти дальше
// начала токена/литерала за время его разбора), вызывающая сторона сама
// передаёт нужную позицию явно - для большинства мест это начало
// разбираемого литерала (line, col, переданные в readString/readChar/...),
// для эскейп-последовательностей - позиция самого "\" (l.line, l.col в
// момент вызова, см. readChar/readTag).
func errAt(code string, line, col int, format string, args ...any) error {
	return diagnostics.New(code, diagnostics.Position{Line: line, Col: col}, format, args...)
}
