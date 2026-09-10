// Package format реализует "rech fmt" - канонический форматтер отступов
// Rech-исходников, тем же принципом, что gofmt у Go: печатает форматированный
// текст на stdout по умолчанию, `--write` переписывает файл на месте.
//
// v1 нормализует ТОЛЬКО отступы (4 пробела на уровень вложенности,
// вычисленный по скобкам "()"/"[]"/"{}") и сжимает подряд идущие пустые
// строки до одной - НЕ переставляет "{" между строками (Allman/K&R, разные
// правила для объявлений/управляющих конструкций/многострочных сигнатур,
// docs/ROADMAP.md, Шаг 88 - слишком много уточнений стиля внутри одной этой
// сессии, чтобы кодировать их как единственно верные прямо сейчас) и не
// трогает межтокенное расположение пробелов внутри строки вовсе - только
// ведущий отступ каждой строки заменяется на канонический, остаток строки
// (включая переменное число пробелов вокруг операторов, если оно там было)
// переносится как есть. Работает НАД ИСХОДНЫМ ТЕКСТОМ, не путём
// перегенерации из AST - строковые/тег-литералы (обратные кавычки,
// поддерживающие настоящий перенос строки внутри себя, docs/SPEC.md) и
// doc-комментарии ("/** ... */", тоже многострочные) переносятся байт-в-байт
// без единого изменения - канонический отступ применяется только к строкам,
// целиком лежащим МЕЖДУ такими токенами, не внутри них (иначе форматтер
// испортил бы содержимое литерала/комментария лишним отступом).
package format

import (
	"fmt"
	"strings"

	"github.com/IlyaFeoktistov/Rech/internal/ast"
	"github.com/IlyaFeoktistov/Rech/internal/lexer"
	"github.com/IlyaFeoktistov/Rech/internal/parser"
)

// openers/closers - токены, меняющие глубину вложенности для отступа.
// Generic-угловые скобки ("<...>") сознательно не учтены - "<"/">" делят
// токен с операторами сравнения/сдвига (GT/SHR, contextual - см.
// internal/parser, expectCloseAngle/pendingGT), и надёжно отличить один
// generic-список от выражения "a < b" без того же контекста, что есть у
// парсера, здесь не стоит того риска - многострочный generic-список
// (Шаг 88) отступ получает от окружающих "()"/"{}", не от самого "<>".
var openers = map[lexer.TokenType]bool{
	lexer.LPAREN: true, lexer.LBRACE: true, lexer.LBRACKET: true,
}
var closers = map[lexer.TokenType]bool{
	lexer.RPAREN: true, lexer.RBRACE: true, lexer.RBRACKET: true,
}

const indentUnit = "    "

// Format переформатирует исходник src (валиден, только если лексер И
// парсер принимают его без ошибок - тот же принцип, что и у gofmt/go-
// parser: форматирование кода с синтаксической ошибкой не определено,
// поэтому просто отказ, а не попытка угадать).
func Format(src string) (string, error) {
	crlf := strings.Contains(src, "\r\n")

	src, err := canonicalizeDocComments(src)
	if err != nil {
		return "", err
	}
	src, err = explodeInlineExitStatements(src)
	if err != nil {
		return "", err
	}
	src, err = insertBlankLineBeforeExitStmt(src)
	if err != nil {
		return "", err
	}
	src, err = normalizeBraceStyle(src)
	if err != nil {
		return "", err
	}
	src, err = insertLambdaSpace(src)
	if err != nil {
		return "", err
	}
	src, err = alignEntityFields(src)
	if err != nil {
		return "", err
	}
	src, err = insertBlankLinesBetweenDecls(src)
	if err != nil {
		return "", err
	}
	src, err = enforceTrailingCommas(src)
	if err != nil {
		return "", err
	}
	src, err = alignVarDecls(src)
	if err != nil {
		return "", err
	}

	toks, err := lexer.New(src).Tokenize()
	if err != nil {
		return "", fmt.Errorf("лексер: %w", err)
	}
	prog, err := parser.New(toks).ParseProgram()
	if err != nil {
		return "", fmt.Errorf("парсер: %w", err)
	}

	normalized := strings.ReplaceAll(src, "\r\n", "\n")
	lines := strings.Split(normalized, "\n")

	depth, verbatim := computeLineDepths(toks, lines, prog)

	outLines := make([]string, 0, len(lines))
	blankRun := 0
	for i, line := range lines {
		lineNo := i + 1
		if verbatim[lineNo] {
			outLines = append(outLines, line)
			blankRun = 0
			continue
		}
		trimmed := strings.TrimRight(strings.TrimLeft(line, " \t"), " \t\r")
		if trimmed == "" {
			blankRun++
			if blankRun > 1 {
				// Схлопнуть подряд идущие пустые строки до одной - эта
				// строка просто не попадает в вывод вовсе.
				continue
			}
			outLines = append(outLines, "")
			continue
		}
		blankRun = 0
		outLines = append(outLines, strings.Repeat(indentUnit, depth[lineNo])+trimmed)
	}

	result := strings.TrimRight(strings.Join(outLines, "\n"), "\n") + "\n"
	if crlf {
		result = strings.ReplaceAll(result, "\n", "\r\n")
	}
	return result, nil
}

// computeLineDepths возвращает: depth[номер строки] - глубина вложенности
// (число уровней "()"/"[]"/"{}"), применяемая как отступ для этой строки;
// verbatim[номер строки] - true для строк, лежащих ВНУТРИ многострочного
// TAG (обратные кавычки)/DOC_COMMENT токена - кроме самой ПЕРВОЙ его строки,
// которая индексируется как обычно (отступ перед открывающей "`"/"/**" всё
// равно нужен) - такие строки форматтер обязан оставить байт-в-байт, не
// трогая даже ведущие пробелы (это часть содержимого литерала/выравнивание
// комментария, выбранное автором, не код).
func computeLineDepths(toks []lexer.Token, lines []string, prog *ast.Program) (depth map[int]int, verbatim map[int]bool) {
	totalLines := len(lines)
	depth = make(map[int]int, totalLines)
	verbatim = make(map[int]bool)

	// significant - toks без NEWLINE/EOF, только то, что реально видно на
	// строке - нужен отдельно от toks, чтобы "предыдущий токен" (weightOf
	// ниже) значил "предыдущий РЕАЛЬНЫЙ токен", а не случайный NEWLINE.
	significant := make([]lexer.Token, 0, len(toks))
	for _, tok := range toks {
		if tok.Type == lexer.NEWLINE || tok.Type == lexer.EOF {
			continue
		}
		significant = append(significant, tok)
	}

	weight := computeBracketWeights(significant)

	cur := 0
	lastSeenLine := 0
	for i, tok := range significant {
		if tok.Line != lastSeenLine {
			// Строки СТРОГО МЕЖДУ предыдущим токеном и этим - без единого
			// токена вовсе (пустые строки, строки с одним лишь
			// "//"-комментарием - лексер съедает такой комментарий целиком
			// как пробел) - получают ТЕКУЩУЮ "cur" как есть: раз токенов
			// нет, глубина физически не могла измениться со времени
			// последнего обработанного токена. Использовать здесь "cur" в
			// момент МЕЖДУ строками, а не итоговый depth[] предыдущей
			// строки - решающе: строка, где открылась "{" (глубина ДО
			// открытия), сама - на глубину меньше, чем СОДЕРЖИМОЕ внутри
			// неё - комментарий сразу после "{" обязан получить уже
			// увеличенную "cur", не "depth" строки с "{".
			for l := lastSeenLine + 1; l < tok.Line; l++ {
				depth[l] = cur
			}
			d := cur
			if closers[tok.Type] {
				d = cur - weight[i]
			}
			depth[tok.Line] = d
			lastSeenLine = tok.Line
		}
		switch {
		case openers[tok.Type]:
			cur += weight[i]
		case closers[tok.Type]:
			cur -= weight[i]
		}
		// Считать переносы строк В ЗНАЧЕНИИ токена как настоящие ФИЗИЧЕСКИЕ
		// строки исходника можно только у TAG (шаблонная строка в обратных
		// кавычках, docs/SPEC.md - настоящий перенос без экранирования) и
		// DOC_COMMENT ("/** ... */", перенос как есть, без обработки
		// escape-последовательностей вовсе) - у STRING (двойные кавычки)
		// перенос НЕ поддерживается физически (SPEC.md, "Многострочные
		// строковые литералы не поддерживаются"), но Value уже несёт
		// РАСКОДИРОВАННЫЕ escape-последовательности ("\n" - два символа
		// исходника - decode'ится в один настоящий '\n' - Value), значит
		// "\n" внутри Value обычной строки - это данные, а не перенос
		// строки исходника - подсчёт по ней вслепую сбил бы дальнейшее
		// отслеживание строк на весь остаток файла.
		if tok.Type == lexer.TAG || tok.Type == lexer.DOC_COMMENT {
			if n := strings.Count(tok.Value, "\n"); n > 0 {
				for l := tok.Line + 1; l <= tok.Line+n; l++ {
					verbatim[l] = true
				}
				lastSeenLine = tok.Line + n
			}
		}
	}
	// Хвост файла после последнего токена (пустые строки в самом конце) -
	// той же "cur".
	for l := lastSeenLine + 1; l <= totalLines; l++ {
		depth[l] = cur
	}
	applyHasSectionIndent(significant, lines, depth, totalLines)
	applyWhenCaseIndent(significant, depth, totalLines)
	applyGerundIndent(prog, significant, lines, depth, totalLines)
	return depth, verbatim
}

// collectGerundStmtLines возвращает Line всех ast.GerundStmt, достижимых из
// stmts, включая ВЛОЖЕННЫЕ (деепричастный вызов внутри тела другого,
// внутри "если"/"пока"/"для"/"попытаться"/"выбрать" и т. п.) - НЕ спускается
// внутрь лямбда-литералов (значение переменной/аргумент вызова) - деепричастие
// внутри лямбды, вложенной в выражение, не поддержано этим v1 (редкий
// случай, ни одного примера в репозитории).
func collectGerundStmtLines(stmts []ast.Stmt) []int {
	var lines []int
	var walk func([]ast.Stmt)
	walk = func(stmts []ast.Stmt) {
		for _, s := range stmts {
			switch st := s.(type) {
			case *ast.GerundStmt:
				lines = append(lines, st.Line)
				walk(st.Body)
			case *ast.IfStmt:
				walk(st.Then)
				walk(st.Else)
			case *ast.WhileStmt:
				walk(st.Body)
			case *ast.ForStmt:
				walk(st.Body)
			case *ast.ForEachStmt:
				walk(st.Body)
			case *ast.CStyleForStmt:
				walk(st.Body)
			case *ast.TryStmt:
				walk(st.Body)
				walk(st.CatchBody)
				walk(st.Finally)
			case *ast.SwitchStmt:
				for _, c := range st.Cases {
					walk(c.Body)
				}
				walk(st.Default)
			}
		}
	}
	walk(stmts)
	return lines
}

// applyGerundIndent - тот же принцип, что applyHasSectionIndent/
// applyWhenCaseIndent, для тела деепричастного вызова
// ("<деепричастие>(<арг>, ...)" без "{}", значимый отступ по столбцу - см.
// internal/parser/gerund.go, parseGerundStmt) - границы GerundStmt.Body уже
// известны АСТ (собраны parser'ом), значит не нужно самому переоткрывать
// морфологию/резолюцию деепричастной формы - только сопоставить Line/Col
// обратно с токеном.
func applyGerundIndent(prog *ast.Program, significant []lexer.Token, lines []string, depth map[int]int, totalLines int) {
	if prog == nil {
		return
	}
	var gerundLines []int
	for _, fn := range prog.Functions {
		gerundLines = append(gerundLines, collectGerundStmtLines(fn.Body)...)
	}
	for _, ent := range prog.Entities {
		if ent.Ctor != nil {
			gerundLines = append(gerundLines, collectGerundStmtLines(ent.Ctor.Body)...)
		}
		for _, m := range ent.Methods {
			gerundLines = append(gerundLines, collectGerundStmtLines(m.Body)...)
		}
	}
	if len(gerundLines) == 0 {
		return
	}

	firstTokCol := make(map[int]int, len(significant))
	for _, t := range significant {
		if _, ok := firstTokCol[t.Line]; !ok {
			firstTokCol[t.Line] = t.Col
		}
	}

	for _, labelLine := range gerundLines {
		labelCol, ok := firstTokCol[labelLine]
		if !ok {
			continue
		}
		// Деепричастие НЕ пропускает doc-комментарий при поиске границы
		// (internal/parser/gerund.go, parseGerundStmt - peekPastNewlines,
		// не peekPastNewlinesAndDoc, в отличие от "имеет:") -
		// skipDocComment=false.
		endLine := findSignificantIndentSectionEnd(significant, lines, labelLine, labelCol, false, totalLines)
		for l := labelLine + 1; l < endLine && l <= totalLines; l++ {
			depth[l]++
		}
	}
}

// applyWhenCaseIndent добавляет +1 к отступу строк одной ветки
// "когда .../иначе:" внутри "выбрать { ... }" (internal/parser/switch.go,
// parseSwitchCaseBody) - ветка сама, как и "имеет:", не заводит СОБСТВЕННЫХ
// "{}" (одна пара скобок на весь "выбрать"), а инструкции внутри нее
// традиционно печатаются на уровень глубже метки. В отличие от "имеет:",
// граница здесь НЕ по отступу - сам парсер завершает ветку строго по
// токену ("когда"/"иначе:"/закрывающая "}" - см. package doc
// parseSwitchCaseBody), значимого отступа тут нет вовсе, значит и
// путаницы, аналогичной "имеет:", здесь не может возникнуть - переносить
// код с ЛЮБЫМ отступом внутри одной ветки безопасно.
// "иначе" здесь отличается от обычного "если .../иначе {" ИМЕННО
// следующим токеном - непосредственно за "иначе" веткой "выбрать" всегда
// идёт ":", а не "{" (или "если" - цепочка "иначе { если ... }") -
// надёжный признак без анализа фигурных скобок вообще.
func applyWhenCaseIndent(significant []lexer.Token, depth map[int]int, totalLines int) {
	isCaseLabel := func(k int) bool {
		if k >= len(significant) {
			return false
		}
		t := significant[k]
		if t.Type == lexer.KwWhen {
			return true
		}
		return t.Type == lexer.KwElse && k+1 < len(significant) && significant[k+1].Type == lexer.COLON
	}
	for i := range significant {
		if !isCaseLabel(i) {
			continue
		}
		labelLine := significant[i].Line
		relDepth := 0
		endLine := totalLines + 1
	scan:
		for k := i + 1; k < len(significant); k++ {
			t := significant[k]
			switch {
			case openers[t.Type]:
				relDepth++
			case closers[t.Type]:
				if relDepth == 0 {
					endLine = t.Line
					break scan
				}
				relDepth--
			case relDepth == 0 && isCaseLabel(k):
				endLine = t.Line
				break scan
			}
		}
		for l := labelLine + 1; l < endLine && l <= totalLines; l++ {
			depth[l]++
		}
	}
}

// applyHasSectionIndent добавляет +1 к отступу строк секции "имеет:" -
// ЕДИНСТВЕННОЕ место грамматики Rech со значимым отступом, не скобками
// (docs/SPEC.md, "имеет:" - поле принадлежит секции, пока Col его первого
// токена строго больше Col метки "имеет", internal/parser/entity.go,
// parseFieldsSection) - без этой поправки поля легли бы на тот же отступ,
// что и сама метка "имеет:", и распарсились бы уже не как поля вовсе
// (секция считалась бы пустой) - реальная порча кода, не просто другой
// стиль, ЕДИНСТВЕННОЕ место, где v1 обязан знать больше, чем скобки.
// Повторяет ТОЧНО то же правило конца секции, что и сам парсер (peek
// NEWLINE/DOC_COMMENT, сравнить Col с меткой) - на ОРИГИНАЛЬНЫХ Col из
// significant, других данных для этого решения нет.
func applyHasSectionIndent(significant []lexer.Token, lines []string, depth map[int]int, totalLines int) {
	for _, tok := range significant {
		if tok.Type != lexer.KwHas {
			continue
		}
		labelLine, labelCol := tok.Line, tok.Col
		// "имеет:" пропускает doc-комментарий при поиске границы
		// (internal/parser/entity.go, parseFieldsSection -
		// peekPastNewlinesAndDoc) - skipDocComment=true.
		endLine := findSignificantIndentSectionEnd(significant, lines, labelLine, labelCol, true, totalLines)
		for l := labelLine + 1; l < endLine && l <= totalLines; l++ {
			depth[l]++
		}
	}
}

// findSignificantIndentSectionEnd находит первую строку ПОСЛЕ labelLine, на
// которой значимо-отступленная секция (имеет:/деепричастие) заканчивается -
// первая строка с содержимым (не пустая), чья "колонка" не больше labelCol:
// у строки с обычным токеном - Col ПЕРВОГО токена; у строки без единого
// токена ("//"-комментарий - лексер съедает его целиком, ни одного токена
// не остаётся) - колонка ЕГО СОБСТВЕННОГО текста в исходнике, ОРИГИНАЛЬНАЯ,
// не пересчитанная - автор уже показал этим отступом, какой секции
// комментарий принадлежит, форматтер обязан прочитать этот сигнал, а не
// проигнорировать строку целиком. Пустые строки ничего не решают и
// пропускаются молча (как разделитель инструкций в самом парсере).
// skipDocComment - true у "имеет:" (парсер пропускает DOC_COMMENT при
// поиске границы, peekPastNewlinesAndDoc - висячий в конце секции
// doc-комментарий не продлевает её), false у деепричастия (peekPastNewlines,
// DOC_COMMENT - обычный токен наравне с остальными).
func findSignificantIndentSectionEnd(significant []lexer.Token, lines []string, labelLine, labelCol int, skipDocComment bool, totalLines int) int {
	firstTokOnLine := make(map[int]lexer.Token, len(significant))
	for _, t := range significant {
		if _, ok := firstTokOnLine[t.Line]; !ok {
			firstTokOnLine[t.Line] = t
		}
	}
	pendingDocLine := -1
	for l := labelLine + 1; l <= totalLines; l++ {
		text := strings.TrimSpace(lines[l-1])
		if text == "" {
			continue
		}
		tok, hasTok := firstTokOnLine[l]
		if !hasTok {
			col := len(lines[l-1]) - len(strings.TrimLeft(lines[l-1], " \t")) + 1
			if col <= labelCol {
				if pendingDocLine != -1 {
					return pendingDocLine
				}
				return l
			}
			continue
		}
		if tok.Type == lexer.DOC_COMMENT && skipDocComment {
			if pendingDocLine == -1 {
				pendingDocLine = l
			}
			continue
		}
		if tok.Type == lexer.RBRACE || tok.Col <= labelCol {
			if pendingDocLine != -1 {
				return pendingDocLine
			}
			return l
		}
		pendingDocLine = -1
	}
	return totalLines + 1
}

// bracketFrame - один уровень стека сопоставления скобок в
// computeBracketWeights.
type bracketFrame struct {
	openIdx       int
	ownLevelBreak bool // на ЭТОМ уровне вложенности была запятая, сразу за которой (следующий значимый токен) шла другая строка
}

// computeBracketWeights сопоставляет каждую открывающую/закрывающую скобку
// в significant (без NEWLINE/EOF) её парой (обычный стек, программа уже
// проверена парсером - несопоставленных скобок быть не может) и назначает
// вес - 1 (обычная скобка, меняет отступ на вложенных строках) или 0 (скобка
// вызова/списка, чьё "многострочие" целиком объясняется ВЛОЖЕННЫМ
// содержимым, не её СОБСТВЕННЫМИ элементами через запятую - типичный случай:
// "f(x, y, функция()\n{...})" или тот же вызов с ещё аргументами ПОСЛЕ
// колбэка на одной строке с "}" - ни одна из "своих" запятых вызова "f" не
// сопровождается переносом строки, весь перенос идёт от вложенного тела
// колбэка). Отличается от настоящего PSR-12 многострочного списка
// (Шаг 88) именно этим: там хотя бы одна "своя" запятая скобки сама
// сопровождается переносом строки (ownLevelBreak) - тогда скобка получает
// вес 1 как обычно, каждый элемент - на свой уровень отступа. "{" сама -
// всегда вес 1, исключение касается только "("/"[".
func computeBracketWeights(significant []lexer.Token) []int {
	weight := make([]int, len(significant))
	var stack []bracketFrame
	for i, tok := range significant {
		switch {
		case openers[tok.Type]:
			stack = append(stack, bracketFrame{openIdx: i})
		case tok.Type == lexer.COMMA:
			if len(stack) > 0 {
				top := &stack[len(stack)-1]
				if i+1 < len(significant) && significant[i+1].Line != tok.Line {
					top.ownLevelBreak = true
				}
			}
		case closers[tok.Type]:
			if len(stack) == 0 {
				break // не должно случиться у уже проверенной парсером программы
			}
			f := stack[len(stack)-1]
			stack = stack[:len(stack)-1]
			oi := f.openIdx
			w := 1
			if significant[oi].Type != lexer.LBRACE && significant[oi].Line != tok.Line && !f.ownLevelBreak {
				w = 0
			}
			weight[oi] = w
			weight[i] = w
		}
	}
	return weight
}
