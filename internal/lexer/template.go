package lexer

// TemplatePart - кусок строки в обратных кавычках: буквальный текст (Expr
// == false, экранирования уже раскрыты) либо исходный текст одной
// "${...}"-вставки без "${"/"}" (Expr == true, как написан - разбирается
// парсером отдельно). Line/Col - позиция начала куска в исходнике.
type TemplatePart struct {
	Text string
	Expr bool
	Line int
	Col  int
}

// readTag читает строку в обратных кавычках (открывающая "`" - текущий
// символ). Один и тот же токен TAG обслуживает две роли, различаемые
// парсером по позиции: тег поля/метода (`json:"name"`, internal/structtag)
// - берётся Value как есть; шаблонная строка (`Привет, ${имя}!`) в позиции
// выражения - Parts. Настоящий перенос строки в исходнике - перенос и в
// значении. Текст между вставками разэкранируется ("\n", "\`", "\$" и т.д.);
// "\$" - буквальный "$", не начало вставки. Вставка "${...}" копируется как
// есть до парной "}" - с учётом вложенных "{...}", строк "...", символов
// '...' и вложенных строк в обратных кавычках (со своими вставками), так
// что ни "}", ни "`" внутри них не закрывают внешнюю вставку/строку.
// Value - текст целиком (вставки - в виде "${...}"), Parts - по кускам.
func (l *Lexer) readTag(line, col int) (Token, error) {
	l.advance() // "`"
	var value, text []rune
	var parts []TemplatePart
	textLine, textCol := l.line, l.col
	flushText := func() {
		if len(text) > 0 {
			parts = append(parts, TemplatePart{Text: string(text), Line: textLine, Col: textCol})
			text = nil
		}
	}
	for {
		if l.pos >= len(l.src) {
			return Token{}, errAt(CodeBadLiteral, line, col, "строка в обратных кавычках не закрыта (нет \"`\")")
		}
		r := l.peek()
		if r == '`' {
			l.advance()
			break
		}
		if r == '$' && l.peekAt(1) == '{' {
			flushText()
			l.advance()
			l.advance()
			exprLine, exprCol := l.line, l.col
			expr, err := l.readTemplateExpr(exprLine, exprCol-2)
			if err != nil {
				return Token{}, err
			}
			parts = append(parts, TemplatePart{Text: expr, Expr: true, Line: exprLine, Col: exprCol})
			value = append(value, []rune("${"+expr+"}")...)
			textLine, textCol = l.line, l.col
			continue
		}
		if len(text) == 0 {
			textLine, textCol = l.line, l.col
		}
		if r == '\\' {
			l.advance()
			esc, ok := tagEscapes[l.peek()]
			if !ok {
				return Token{}, errAt(CodeBadEscape, l.line, l.col, "неизвестная последовательность экранирования \\%c", l.peek())
			}
			l.advance()
			text = append(text, esc)
			value = append(value, esc)
			continue
		}
		text = append(text, r)
		value = append(value, r)
		l.advance()
	}
	flushText()
	return Token{Type: TAG, Value: string(value), Line: line, Col: col, EndLine: l.line, Parts: parts}, nil
}

// tagEscapes - экранирования строки в обратных кавычках.
var tagEscapes = map[rune]rune{
	'n': '\n', 't': '\t', 'r': '\r', '`': '`', '\\': '\\', '$': '$',
}

// readTemplateExpr - исходный текст вставки после "${" до парной "}"
// (сама "}" поглощается). line/col - позиция "${", для ошибки незакрытой
// вставки (в т.ч. когда до конца файла не закрылась вложенная строка в
// обратных кавычках - "`" внутри вставки открывает именно её).
func (l *Lexer) readTemplateExpr(line, col int) (string, error) {
	start := l.pos
	depth := 1
	for l.pos < len(l.src) {
		switch r := l.peek(); r {
		case '{':
			depth++
		case '}':
			depth--
			if depth == 0 {
				expr := string(l.src[start:l.pos])
				l.advance()
				return expr, nil
			}
		case '"':
			l.advance()
			for l.pos < len(l.src) && l.peek() != '"' {
				l.advance()
			}
		case '\'':
			l.advance()
			for l.pos < len(l.src) && l.peek() != '\'' {
				if l.peek() == '\\' {
					l.advance()
				}
				l.advance()
			}
		case '`':
			if _, err := l.readTag(l.line, l.col); err != nil {
				return "", errAt(CodeBadLiteral, line, col, "вставка \"${\" не закрыта символом \"}\"")
			}
			continue
		}
		if l.pos < len(l.src) {
			l.advance()
		}
	}
	return "", errAt(CodeBadLiteral, line, col, "вставка \"${\" не закрыта символом \"}\"")
}
