// Package diagnostics - единая структура ошибок компиляции: код (RECHxxxx),
// позиция в исходнике, сообщение, необязательная подсказка. Заменяет
// разрозненные fmt.Errorf с "строка %d, столбец %d: ..." внутри самого
// текста сообщения (internal/lexer/internal/parser/internal/semantic) на
// один структурный тип, который cmd/rech умеет печатать со сниппетом
// исходной строки и кареткой-указателем (см. Render/RenderAll), а LSP
// (internal/lsp) - превращать в protocol.Diagnostic без парсинга текста
// обратно.
package diagnostics

import "fmt"

// Severity - серьёзность диагностики. Сейчас у компилятора нет
// предупреждений (любая диагностика останавливает сборку) - Warning
// зарезервирован на будущее, ни один текущий вызывающий код его не
// использует.
type Severity int

const (
	Error Severity = iota
	Warning
)

func (s Severity) String() string {
	if s == Warning {
		return "предупреждение"
	}
	return "ошибка"
}

// Position - место диагностики в исходнике. File - путь, как его видит
// пользователь (то, что передано в rech build/использовать), "" - нет
// привязки к конкретному файлу (код без файла-источника - "rech build -c",
// либо диагностика уровня тулчейна, см. RECH9000 в codes.go). Line/Col -
// 1-индексные, в рунах, не в байтах (тем же соглашением, что и
// internal/lexer.Lexer - Col нельзя вычислить как byte-offset строки,
// кириллица многобайтовая). Col == 0 - колонка не известна (сниппет тогда
// без каретки, только сама строка).
type Position struct {
	File string
	Line int
	Col  int
}

// Diagnostic - одна диагностика компиляции. Реализует error - существующий
// код, ожидающий обычный error (errors.Is/errors.As/%w), продолжает
// работать без изменений; новый код различает *Diagnostic через errors.As,
// чтобы достать Code/Pos/Hint для Render.
type Diagnostic struct {
	// Code - "RECH2007" и т.п. (см. codes.go) - "" у диагностик, которые
	// сознательно не мигрированы на конкретный код (RECH9000 - обёртка
	// над внутренними ошибками cbackend/toolchain, см. package doc
	// codes.go).
	Code     string
	Severity Severity
	Pos      Position
	Message  string
	// Hint - "" - нет подсказки. Печатается отдельной строкой под
	// сниппетом (см. Render) - только для диагностик, где подсказка
	// однозначна и не требует догадок (см. package doc hint.go).
	Hint string
}

// Error - текстовое summary без сниппета (для %w/логов/тестов, которые
// матчат текст ошибки подстрокой - см. Message, а не Error(), если нужен
// сам текст без кода/позиции/подсказки). Hint, если есть, дописывается в
// скобках в конце - без этого текст диагностики с подсказкой (например,
// рассогласование рода модификатора - ожидалась форма/фактическая форма
// теперь в Hint, не в Message, см. package doc Diagnostic.Hint) терял бы
// при печати через plain error ровно ту часть, ради которой он раньше был
// в основном сообщении.
func (d *Diagnostic) Error() string {
	msg := d.Message
	if d.Hint != "" {
		msg = fmt.Sprintf("%s (подсказка: %s)", msg, d.Hint)
	}
	if d.Pos.File == "" && d.Pos.Line == 0 {
		if d.Code == "" {
			return msg
		}
		return fmt.Sprintf("[%s] %s", d.Code, msg)
	}
	loc := fmt.Sprintf("строка %d", d.Pos.Line)
	if d.Pos.Col > 0 {
		loc = fmt.Sprintf("%s, столбец %d", loc, d.Pos.Col)
	}
	if d.Pos.File != "" {
		loc = fmt.Sprintf("%s:%s", d.Pos.File, loc)
	}
	if d.Code == "" {
		return fmt.Sprintf("%s: %s", loc, msg)
	}
	return fmt.Sprintf("[%s] %s: %s", d.Code, loc, msg)
}

// New строит диагностику с позицией. code - "" допустим (см. package doc
// Diagnostic.Code) для мест, ещё не мигрированных на конкретный код.
func New(code string, pos Position, format string, args ...any) *Diagnostic {
	return &Diagnostic{Code: code, Pos: pos, Message: fmt.Sprintf(format, args...)}
}

// WithHint возвращает d с проставленным Hint - удобно в цепочке с New:
// diagnostics.New(...).WithHint("...").
func (d *Diagnostic) WithHint(format string, args ...any) *Diagnostic {
	d.Hint = fmt.Sprintf(format, args...)
	return d
}
