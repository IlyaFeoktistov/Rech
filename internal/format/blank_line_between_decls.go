package format

import (
	"strings"

	"github.com/IlyaFeoktistov/Rech/internal/lexer"
	"github.com/IlyaFeoktistov/Rech/internal/parser"
)

// insertBlankLinesBetweenDecls гарантирует РОВНО одну пустую строку прямо
// перед каждым объявлением функции/метода/конструктора (docs/ROADMAP.md,
// пользователь попросил явно - "чтобы они слитно друг с другом не
// стояли") - НЕ трогает уже разделённые (идемпотентность) и не заставляет
// пустую строку появляться там, где объявление и так первое в своём
// контейнере (файл/тело сущности, сразу после открывающей "{").
//
// Считает "объявлением" не саму строку с "функция"/именем метода, а его
// doc-комментарий (если он есть непосредственно перед, без уже своей
// пустой строки) - блок "документация + объявление" разделяется от
// предыдущего кода как единое целое, не порознь.
//
// Решение "нужна ли пустая строка" - по ТЕКСТУ строки НАД объявлением
// (простой и надёжный признак, без анализа вложенности): пусто ИЛИ
// заканчивается на "{" (это открывающая скобка объемлющего контейнера -
// первый член сразу после неё не нуждается в отступе-разделителе) -
// ничего не делать; иначе - вставить одну пустую строку.
func insertBlankLinesBetweenDecls(src string) (string, error) {
	toks, err := lexer.New(src).Tokenize()
	if err != nil {
		return "", err
	}
	prog, err := parser.New(toks).ParseProgram()
	if err != nil {
		return "", err
	}

	docEndLine := map[int]int{} // строка ПОСЛЕДНЕЙ строки doc-комментария -> строка его начала
	for _, t := range toks {
		if t.Type != lexer.DOC_COMMENT {
			continue
		}
		last := t.Line + strings.Count(t.Value, "\n")
		docEndLine[last] = t.Line
	}

	normalized := strings.ReplaceAll(src, "\r\n", "\n")
	lines := strings.Split(normalized, "\n")

	// isPlainCommentLine - "//"-строка, которую лексер съедает целиком (ни
	// одного токена не остаётся, см. package doc format.go) - без этой
	// проверки declStart не видел бы такую строку вообще и вставил бы
	// пустую строку МЕЖДУ повисшим "//"-комментарием и его объявлением,
	// разлучив их (обычный doc-комментарий "/** ... */" - настоящий токен,
	// уже покрыт docEndLine выше).
	isPlainCommentLine := func(lineNo int) bool {
		if lineNo < 1 || lineNo > len(lines) {
			return false
		}
		return strings.HasPrefix(strings.TrimSpace(lines[lineNo-1]), "//")
	}

	// declStart - самая верхняя строка "блока" объявления (само
	// объявление + непрерывная цепочка doc-/"//"-комментариев прямо над
	// ним, без единой пустой строки-разрыва) - именно ПЕРЕД ней вставляется
	// пустая строка-разделитель, не между объявлением и его же
	// документацией.
	declStart := func(line int) int {
		for {
			if start, ok := docEndLine[line-1]; ok {
				line = start
				continue
			}
			if isPlainCommentLine(line - 1) {
				line--
				continue
			}
			break
		}
		return line
	}

	starts := map[int]bool{}
	for _, fn := range prog.Functions {
		starts[declStart(fn.Line)] = true
	}
	for _, ent := range prog.Entities {
		if ent.Ctor != nil {
			starts[declStart(ent.Ctor.Line)] = true
		}
		for _, m := range ent.Methods {
			starts[declStart(m.Line)] = true
		}
	}

	var out []string
	for i, line := range lines {
		lineNo := i + 1
		if starts[lineNo] && lineNo > 1 {
			above := strings.TrimRight(lines[lineNo-2], " \t\r")
			if above != "" && !strings.HasSuffix(above, "{") {
				out = append(out, "")
			}
		}
		out = append(out, line)
	}
	return strings.Join(out, "\n") + "\n", nil
}
