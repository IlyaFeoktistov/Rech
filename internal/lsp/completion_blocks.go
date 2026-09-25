package lsp

import (
	"strings"

	"github.com/IlyaFeoktistov/Rech/internal/lexer"
)

// Структура блоков вокруг курсора - по токенам ТЕКУЩЕГО текста (не
// последнего удачного разбора): какие "{ ... }" его окружают и что это за
// блоки. Нужна автодополнению, чтобы знать место: тело сущности вне
// методов, тело функции или лямбды, верхний уровень файла. Разобранные
// функции (d.scopes) знают только строку объявления, не конец тела.

type blockKind int

const (
	blockTop       blockKind = iota // верхний уровень файла (внешнего блока нет)
	blockOther                      // если/пока/для/попытаться/... - тело инструкции
	blockEntity                     // сущность/класс/структура
	blockInterface                  // интерфейс
	blockEnum                       // перечисление
	blockFunc                       // тело функции, метода или конструктора
	blockLambda                     // тело "функция(...) { ... }" внутри выражения
	blockSwitch                     // "выбрать ... { когда ... }"
)

// textBlock - один открытый блок: вид, строка начала заголовка, токены
// заголовка (всё от начала заголовка до "{").
type textBlock struct {
	kind       blockKind
	headerLine int
	header     []lexer.Token
	// body - токены тела от "{" (не включая) до курсора.
	body []lexer.Token
}

// declModifiers - слова, которые могут стоять перед "функция"/именем метода
// в заголовке объявления.
var declModifiers = map[lexer.TokenType]bool{
	lexer.KwPublic: true, lexer.KwPrivate: true, lexer.KwProtected: true, lexer.KwInternal: true,
	lexer.KwStatic: true, lexer.KwAbstract: true, lexer.KwAsync: true, lexer.KwExport: true,
}

// enclosingBlocks - блоки, окружающие позицию (line, col) (1-индексные),
// от внешнего к внутреннему. Текст строки курсора после курсора
// отбрасывается (он ещё не дописан); nil - текст не разбирается лексером.
func enclosingBlocks(text string, line, col int) []textBlock {
	toks, ok := tokensBeforeCursor(text, line, col)
	if !ok {
		return nil
	}
	var stack []textBlock
	var opens []int
	for i, t := range toks {
		switch t.Type {
		case lexer.LBRACE:
			opens = append(opens, i)
			header := blockHeader(toks, i)
			parentKind := blockTop
			if len(stack) > 0 {
				parentKind = stack[len(stack)-1].kind
			}
			b := textBlock{kind: classifyBlock(header, parentKind), header: header}
			if len(header) > 0 {
				b.headerLine = header[0].Line
			}
			stack = append(stack, b)
		case lexer.RBRACE:
			if len(stack) > 0 {
				stack = stack[:len(stack)-1]
				opens = opens[:len(opens)-1]
			}
		}
	}
	for i := range stack {
		stack[i].body = toks[opens[i]+1:]
	}
	return stack
}

// inFieldSection - курсор в теле сущности b в секции "имеет:": после метки
// "имеет:" на уровне самой сущности не начался ни метод, ни конструктор,
// ни константа.
func inFieldSection(b textBlock) bool {
	in, depth := false, 0
	lineFirst := true
	for i, t := range b.body {
		if t.Type == lexer.NEWLINE {
			lineFirst = true
			continue
		}
		if depth == 0 {
			switch {
			case t.Type == lexer.KwHas:
				in = true
			case t.Type == lexer.KwConstructor, t.Type == lexer.KwConst, t.Type == lexer.KwOperator:
				in = false
			case lineFirst && t.Type == lexer.IDENT && i+1 < len(b.body) && b.body[i+1].Type == lexer.LPAREN:
				in = false
			}
		}
		switch t.Type {
		case lexer.LBRACE:
			depth++
			in = false
		case lexer.RBRACE:
			depth--
		}
		if !declModifiers[t.Type] {
			lineFirst = false
		}
	}
	return in
}

// tokensBeforeCursor - токены текста до курсора (строка курсора обрезана по
// курсору). Если обрезанная строка не лексится (незакрытая строка и т. п.),
// она выбрасывается целиком.
func tokensBeforeCursor(text string, line, col int) ([]lexer.Token, bool) {
	lines := strings.Split(text, "\n")
	if line < 1 || line > len(lines) {
		return nil, false
	}
	before := lines[:line]
	runes := []rune(before[line-1])
	if col-1 < len(runes) {
		before[line-1] = string(runes[:max(col-1, 0)])
	}
	toks, err := lexer.New(strings.Join(before, "\n")).Tokenize()
	if err != nil {
		before[line-1] = ""
		if toks, err = lexer.New(strings.Join(before, "\n")).Tokenize(); err != nil {
			return nil, false
		}
	}
	return toks, true
}

// blockHeader - токены заголовка блока, открытого "{" с индексом open: от
// начала строки заголовка (перенос строки перед "{" пропускается - стиль
// "{ на новой строке") до "{". Строка, начинающаяся с ")" (многострочная
// сигнатура), расширяется назад до строки с парной "(".
func blockHeader(toks []lexer.Token, open int) []lexer.Token {
	end := open - 1
	for end >= 0 && toks[end].Type == lexer.NEWLINE {
		end--
	}
	if end < 0 {
		return nil
	}
	start := lineStart(toks, end)
	if toks[start].Type == lexer.RPAREN {
		depth := 0
	back:
		for j := end; j >= 0; j-- {
			switch toks[j].Type {
			case lexer.RPAREN:
				depth++
			case lexer.LPAREN:
				depth--
				if depth == 0 {
					start = lineStart(toks, j)
					break back
				}
			}
		}
	}
	return toks[start : end+1]
}

// lineStart - индекс первого токена строки, в которой токен j.
func lineStart(toks []lexer.Token, j int) int {
	for j > 0 && toks[j-1].Type != lexer.NEWLINE && toks[j-1].Line == toks[j].Line {
		j--
	}
	return j
}

// classifyBlock - вид блока по заголовку и виду внешнего блока.
func classifyBlock(header []lexer.Token, parent blockKind) blockKind {
	first := 0
	for first < len(header) && declModifiers[header[first].Type] {
		first++
	}
	for _, t := range header {
		switch t.Type {
		case lexer.KwEntity, lexer.KwClass, lexer.KwStruct:
			return blockEntity
		case lexer.KwInterface:
			return blockInterface
		case lexer.KwEnum:
			return blockEnum
		case lexer.KwSwitch:
			return blockSwitch
		}
	}
	// Объявление функции - только на верхнем уровне или в теле сущности;
	// "функция(" где-то ещё - лямбда.
	declPlace := parent == blockTop || parent == blockEntity
	if declPlace && first < len(header) && header[first].Type == lexer.KwFunction {
		return blockFunc
	}
	for _, t := range header {
		if t.Type == lexer.KwFunction {
			return blockLambda
		}
	}
	if parent == blockEntity && first < len(header) &&
		(header[first].Type == lexer.KwConstructor || header[first].Type == lexer.KwOperator ||
			(header[first].Type == lexer.IDENT && first+1 < len(header) && header[first+1].Type == lexer.LPAREN)) {
		return blockFunc
	}
	return blockOther
}

// lambdaParams - параметры "функция(имя: Тип, ...)" из заголовка лямбды:
// имя и тип как написаны в исходнике.
func lambdaParams(header []lexer.Token) [][2]string {
	i := 0
	for i < len(header) && header[i].Type != lexer.KwFunction {
		i++
	}
	i++
	if i >= len(header) || header[i].Type != lexer.LPAREN {
		return nil
	}
	i++
	var out [][2]string
	for i < len(header) && header[i].Type != lexer.RPAREN {
		if header[i].Type == lexer.IDENT && i+1 < len(header) && header[i+1].Type == lexer.COLON {
			name := header[i].Value
			j := i + 2
			depth := 0
			var typ strings.Builder
			for j < len(header) {
				t := header[j]
				if depth == 0 && (t.Type == lexer.COMMA || t.Type == lexer.RPAREN) {
					break
				}
				switch t.Type {
				case lexer.LT, lexer.LPAREN:
					depth++
				case lexer.GT, lexer.RPAREN:
					depth--
				}
				typ.WriteString(t.Value)
				if t.Type == lexer.COMMA {
					typ.WriteString(" ")
				}
				j++
			}
			out = append(out, [2]string{name, typ.String()})
			i = j
			continue
		}
		i++
	}
	return out
}
