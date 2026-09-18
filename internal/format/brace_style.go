package format

import (
	"fmt"
	"strings"

	"github.com/IlyaFeoktistov/Rech/internal/lexer"
	"github.com/IlyaFeoktistov/Rech/internal/parser"
)

// normalizeBraceStyle - вторая (после computeLineDepths) обязанность
// форматтера: расположение "{"/"}" по вертикали, отдельная забота от
// отступов (те пересчитываются заново уже ПОСЛЕ этого прохода, поэтому
// собственный отступ строк, которые эта функция создаёт/переставляет, не
// важен вообще).
//
// Правило (feature_brace_style): управляющие конструкции
// (если/иначе/для/пока/попытаться/поймать/окончательно/выбрать) - K&R,
// "{" всегда на строке заголовка, а "}" предыдущей ветки - на одной
// строке со следующим продолжением ("} иначе {", "} поймать (...) {",
// "} окончательно {"). Объявления с телом (функция/метод/конструктор/
// сущность/структура/интерфейс/перечисление) - Allman, "{" на
// собственной строке, ЕДИНСТВЕННОЕ исключение - многострочный список
// параметров (висячая запятая, PSR-12): тогда "{" остаётся на строке
// "): Тип" - переносить её отдельно только ради Allman бессмысленно,
// сама граница списка параметров уже несёт перенос строки.
//
// Трогает ТОЛЬКО те "{"/"}", что уже разнесены автором по разным
// строкам - блок, целиком уместившийся в одну физическую строку
// (заголовок+тело+"}"), этим проходом не разбирается и не сжимается: v1
// форматтера принципиально не меняет число строк исходника походя (см.
// package doc format.go) - здесь оно тоже меняется только там, где сам
// автор уже перенёс строку, никогда не ради переноса как такового.
func normalizeBraceStyle(src string) (string, error) {
	toks, err := lexer.New(src).Tokenize()
	if err != nil {
		return "", fmt.Errorf("лексер: %w", err)
	}
	if _, err := parser.New(toks).ParseProgram(); err != nil {
		return "", fmt.Errorf("парсер: %w", err)
	}

	significant := make([]lexer.Token, 0, len(toks))
	for _, t := range toks {
		if t.Type == lexer.NEWLINE || t.Type == lexer.EOF {
			continue
		}
		significant = append(significant, t)
	}
	match := matchAllBrackets(significant)

	normalized := strings.ReplaceAll(src, "\r\n", "\n")
	lines := strings.Split(normalized, "\n")

	lu := newLineUnion()
	splitBefore := map[int]bool{} // номер строки, у которой "{" в конце нужно перенести на свою строку

	// "}" сразу перед "иначе"/"поймать"/"окончательно" - всегда одна
	// строка с продолжением, независимо от классификации следующей "{"
	// (та обрабатывается уже отдельно, ниже).
	for i, t := range significant {
		if t.Type != lexer.RBRACE {
			continue
		}
		if i+1 < len(significant) && continuationBraceKeywords[significant[i+1].Type] && significant[i+1].Line != t.Line {
			lu.union(t.Line, significant[i+1].Line)
		}
	}

	for i, t := range significant {
		if t.Type != lexer.LBRACE {
			continue
		}
		closeIdx := match[i]
		if closeIdx < 0 || significant[closeIdx].Line == t.Line {
			continue // блок целиком на одной строке - не трогаем
		}
		if i == 0 {
			continue
		}
		headerLine := significant[i-1].Line
		class, isLambda, multilineParams, keywordLine := classifyBrace(significant, match, i)
		switch {
		case class == braceControl && keywordLine != headerLine:
			// Условие/заголовок управляющей конструкции сам перенесён
			// автором на несколько строк ("если x\n    или y {") - "{"
			// переносится на собственную строку (той же табуляции, что и
			// ведущее ключевое слово, после отдельного прохода
			// computeLineDepths) вместо обычного K&R этой ветки ниже:
			// иначе она рисовалась бы на одной плоскости с последней
			// строкой уже перенесённого условия, а не с самим "если" -
			// тот же принцип, что и у multilineParams ниже, только в
			// обратную сторону (K&R -> Allman, а не Allman -> K&R,
			// потому что у управляющих конструкций умолчание - K&R).
			if headerLine == t.Line {
				splitBefore[t.Line] = true
			}
		case isLambda:
			// Лямбда (анонимная "функция(...)", без имени - колбэк) - K&R,
			// как управляющая конструкция, а не Allman как у ИМЕНОВАННОГО
			// объявления - пользователь попросил явно (иначе колбэк с
			// однострочным телом занимал бы неоправданно много вертикального
			// места посреди списка аргументов вызова). Без исключения на
			// многострочный список параметров (в отличие от case выше) -
			// пользователь не просил его для лямбд.
			if headerLine != t.Line {
				lu.union(headerLine, t.Line)
			}
		case class == braceControl:
			if headerLine != t.Line {
				lu.union(headerLine, t.Line)
			}
		case multilineParams:
			if headerLine != t.Line {
				lu.union(headerLine, t.Line)
			}
		case headerLine == t.Line:
			splitBefore[t.Line] = true
		}
	}

	return renderLineUnion(lines, lu, splitBefore), nil
}

// braceClass - K&R (управляющая конструкция) или Allman (объявление).
type braceClass int

const (
	braceControl braceClass = iota
	braceDecl
)

// controlBraceKeywords - ведущее ключевое слово управляющей конструкции -
// её "{" всегда K&R, без исключений (в отличие от объявлений, см.
// declBraceKeywords).
var controlBraceKeywords = map[lexer.TokenType]bool{
	lexer.KwIf: true, lexer.KwElse: true, lexer.KwWhile: true, lexer.KwFor: true,
	lexer.KwTry: true, lexer.KwCatch: true, lexer.KwFinally: true, lexer.KwSwitch: true,
}

// declBraceKeywords - ведущее ключевое слово объявления с телом - Allman,
// кроме многострочного списка параметров (KwFunction/KwConstructor) -
// сущности/структуры/интерфейсы/перечисления вообще не имеют своего
// списка параметров в заголовке, поэтому исключение для них никогда не
// срабатывает (classifyBrace ниже не находит "(" вообще).
var declBraceKeywords = map[lexer.TokenType]bool{
	lexer.KwFunction: true, lexer.KwConstructor: true, lexer.KwEntity: true,
	lexer.KwClass: true, lexer.KwStruct: true, lexer.KwInterface: true, lexer.KwEnum: true,
}

// continuationBraceKeywords - продолжение цепочки предыдущего блока,
// сразу после его закрывающей "}" (док см. normalizeBraceStyle).
var continuationBraceKeywords = map[lexer.TokenType]bool{
	lexer.KwElse: true, lexer.KwCatch: true, lexer.KwFinally: true,
}

// classifyBrace определяет класс "{" на позиции lbraceIdx в significant,
// сканируя НАЗАД от неё до первого узнанного ключевого слова (или до
// границы "{"/"}" предыдущего члена - это метод сущности, у которого
// собственного ведущего ключевого слова нет вовсе, internal/parser/
// entity.go, parseMethod - тоже объявление, Allman). "()"/"[]" по пути
// перепрыгиваются через match (их содержимое не может само оборвать
// сканирование управляющим/декларативным ключевым словом - оно уже
// проверено парсером на корректность). multilineParams - true, если
// САМЫЙ БЛИЖНИЙ к "{" список параметров (первая ")" по пути назад)
// открывается на другой физической строке, чем закрывается. isLambda -
// true, если найденное ключевое слово - KwFunction БЕЗ имени сразу за ним
// (анонимная "функция(...)", колбэк) - единственное исключение внутри
// declBraceKeywords, которое всё равно K&R, не Allman (см. вызывающую
// сторону, normalizeBraceStyle). keywordLine - строка найденного ведущего
// ключевого слова (значима только при class == braceControl - сравнивается
// с headerLine вызывающей стороной, чтобы отличить однострочное условие/
// заголовок от перенесённого автором на несколько строк).
func classifyBrace(sig []lexer.Token, match []int, lbraceIdx int) (class braceClass, isLambda bool, multilineParams bool, keywordLine int) {
	class = braceDecl // по умолчанию - метод без ведущего ключевого слова
	firstRParen := -1
	k := lbraceIdx - 1
	for k >= 0 {
		t := sig[k]
		if t.Type == lexer.RPAREN || t.Type == lexer.RBRACKET {
			if t.Type == lexer.RPAREN && firstRParen == -1 {
				firstRParen = k
			}
			if match[k] < 0 {
				break
			}
			k = match[k] - 1
			continue
		}
		if t.Type == lexer.LBRACE || t.Type == lexer.RBRACE {
			break // граница предыдущего члена - дальше не наше дело
		}
		if controlBraceKeywords[t.Type] {
			class = braceControl
			keywordLine = t.Line
			break
		}
		if declBraceKeywords[t.Type] {
			class = braceDecl
			if t.Type == lexer.KwFunction && k+1 < len(sig) && sig[k+1].Type == lexer.LPAREN {
				isLambda = true
			}
			break
		}
		k--
	}
	if firstRParen != -1 && match[firstRParen] >= 0 {
		multilineParams = sig[match[firstRParen]].Line != sig[firstRParen].Line
	}
	return class, isLambda, multilineParams, keywordLine
}

// matchAllBrackets сопоставляет КАЖДУЮ "()"/"[]"/"{}" её парой одним
// общим стеком (программа уже проверена парсером - несопоставленных
// скобок быть не может, см. тот же принцип в computeBracketWeights) -
// в отличие от computeBracketWeights (тот считает вес для отступа и
// сознательно игнорирует "<>"), здесь важна именно пара индексов, чтобы
// classifyBrace могла перепрыгивать через "()"/"[]" при сканировании
// назад.
func matchAllBrackets(sig []lexer.Token) []int {
	match := make([]int, len(sig))
	for i := range match {
		match[i] = -1
	}
	var stack []int
	for i, t := range sig {
		switch t.Type {
		case lexer.LPAREN, lexer.LBRACE, lexer.LBRACKET:
			stack = append(stack, i)
		case lexer.RPAREN, lexer.RBRACE, lexer.RBRACKET:
			if len(stack) == 0 {
				break
			}
			open := stack[len(stack)-1]
			stack = stack[:len(stack)-1]
			match[open] = i
			match[i] = open
		}
	}
	return match
}

// lineUnion - union-find по номерам физических строк (1-based) - строки,
// объединённые в одну группу, printLineUnion склеивает в одну ВЫХОДНУЮ
// строку (их содержимое, обрезанное от пробелов по краям, через пробел,
// в порядке возрастания номера) - порядок объединения не важен.
type lineUnion struct {
	parent map[int]int
}

func newLineUnion() *lineUnion { return &lineUnion{parent: map[int]int{}} }

func (u *lineUnion) find(x int) int {
	p, ok := u.parent[x]
	if !ok {
		u.parent[x] = x
		return x
	}
	if p != x {
		p = u.find(p)
		u.parent[x] = p
	}
	return p
}

func (u *lineUnion) union(a, b int) {
	ra, rb := u.find(a), u.find(b)
	if ra == rb {
		return
	}
	if ra > rb {
		ra, rb = rb, ra
	}
	u.parent[rb] = ra
}

// renderLineUnion строит итоговый текст: строки одной группы (lu) -
// склеены в одну (порядок - по возрастанию номера строки), строки без
// группы - как есть, кроме отмеченных в splitBefore (у них последняя "{"
// вырезается и переносится на отдельную новую строку сразу следом -
// собственный отступ обеих не важен, computeLineDepths пересчитывает его
// с нуля по СВЕЖЕ пересчитанным скобкам).
func renderLineUnion(lines []string, lu *lineUnion, splitBefore map[int]bool) string {
	total := len(lines)
	membersByRoot := make(map[int][]int, total)
	for line := 1; line <= total; line++ {
		r := lu.find(line)
		membersByRoot[r] = append(membersByRoot[r], line)
	}

	var out []string
	processed := make(map[int]bool, total)
	for line := 1; line <= total; line++ {
		if processed[line] {
			continue
		}
		members := membersByRoot[lu.find(line)]
		if len(members) > 1 {
			var parts []string
			for _, m := range members {
				processed[m] = true
				if t := strings.TrimSpace(lines[m-1]); t != "" {
					parts = append(parts, t)
				}
			}
			out = append(out, strings.Join(parts, " "))
			continue
		}
		processed[line] = true
		text := lines[line-1]
		if splitBefore[line] {
			idx := strings.LastIndex(text, "{")
			before := strings.TrimRight(text[:idx], " \t")
			after := text[idx+1:]
			out = append(out, before)
			out = append(out, "{"+after)
			continue
		}
		out = append(out, text)
	}
	return strings.Join(out, "\n") + "\n"
}
