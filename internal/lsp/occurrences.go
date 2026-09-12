// Общая часть "textDocument/references" и "textDocument/documentHighlight"
// (оба - в server.go) - найти все вхождения имени под курсором в ТЕКУЩЕМ
// документе. Для ЛОКАЛЬНОЙ переменной/параметра вхождение - не только
// буквальное совпадение текста, но и любая её словоформа (dict.Analyze, тот
// же морфологический движок, что и падежное сопоставление аргументов
// естественного вызова, internal/parser/call.go) - "паролем" должно
// подсвечиваться/находиться как ссылка на переменную "пароль" (найдено по
// прямой просьбе пользователя: "переменная пароль и её форма паролем -
// выделяется только буквальное").
package lsp

import (
	"github.com/IlyaFeoktistov/Rech/internal/lexer"
	"github.com/IlyaFeoktistov/Rech/internal/morph"
)

// occPos - одно вхождение имени (строка/столбец начала токена, оба
// 1-индексные, и длина в рунах) - тот же формат, что definitionPos
// (definition.go), без URI: оба запроса, использующих occurrences,
// ограничены текущим документом (см. package doc).
type occPos struct {
	Line, Col, Len int
}

// occurrences находит все вхождения имени под курсором (line, col, оба
// 1-индексные) в текущем документе.
//
// Область действия (сознательно, не полная): только "голое" имя (chain
// длины 1, см. chainBefore) - локальная переменная/параметр функции/метода,
// либо функция/сущность/константа верхнего уровня. Член сущности
// ("receiver.поле", chain длины > 1) НЕ поддержан - "все вхождения"
// потребовало бы для КАЖДОГО "x.имя" во всём файле знать статический тип x,
// чтобы отличить "то же поле той же сущности" от одноимённого поля ДРУГОЙ
// сущности (по сути, заново вывести типы всех выражений документа) - явно
// не сделано сейчас, а не молча неверно: ok == false.
func (d *doc) occurrences(line, col int) ([]occPos, bool) {
	if d.irProg == nil || len(d.tokens) == 0 {
		return nil, false
	}
	i := d.tokenIndexAt(line, col)
	if i < 0 {
		return nil, false
	}
	if d.tokens[i].Type != lexer.IDENT {
		return nil, false
	}
	chain := d.chainBefore(i)
	if len(chain) != 1 {
		return nil, false
	}
	name := chain[0]

	if scope := d.scopeAt(line); scope != nil {
		if _, ok := localsAt(scope, line)[name]; ok {
			return d.localOccurrences(scope, name), true
		}
	}
	if _, ok := d.astFuncs[name]; ok {
		return d.topLevelOccurrences(name), true
	}
	if _, ok := d.astEntities[name]; ok {
		return d.topLevelOccurrences(name), true
	}
	if _, ok := d.astConsts[name]; ok {
		return d.topLevelOccurrences(name), true
	}
	return nil, false
}

// localOccurrences - все вхождения ЛОКАЛЬНОГО имени name (переменная/
// параметр) внутри scope, включая его словоформы - ограничено приблизительной
// границей тела (scopeLineRange - в этом пакете нигде нет точных границ
// конца тела, см. package doc scopeAt).
func (d *doc) localOccurrences(scope *funcScope, name string) []occPos {
	from, to := d.scopeLineRange(scope)
	return d.tokenOccurrences(from, to, func(word string) bool {
		return d.wordformMatchesLemma(word, name)
	})
}

// topLevelOccurrences - все вхождения имени верхнего уровня (функция/
// сущность/константа модуля) во всём документе - буквальным совпадением
// текста, без словоформ (глагол естественного вызова и имя метода/функции
// пишутся ровно как объявлены, без спряжения - см. docs/GRAMMAR_DRAFT.md,
// natural_method_call.go).
func (d *doc) topLevelOccurrences(name string) []occPos {
	from, to := 1, d.tokens[len(d.tokens)-1].Line
	return d.tokenOccurrences(from, to, func(word string) bool { return word == name })
}

// tokenOccurrences - общий проход по токенам документа в диапазоне строк
// [from, to] - IDENT-токен, для которого match(значение) верно, считается
// вхождением, КРОМЕ токена сразу после "."/"::" (receiver.имя/Тип::имя -
// член сущности, не голая ссылка на переменную/функцию/сущность/константу,
// даже если текст совпал).
func (d *doc) tokenOccurrences(from, to int, match func(word string) bool) []occPos {
	var out []occPos
	prevType := lexer.EOF
	for _, t := range d.tokens {
		if t.Line >= from && t.Line <= to && t.Type == lexer.IDENT &&
			prevType != lexer.DOT && prevType != lexer.DBLCOLON && match(t.Value) {
			out = append(out, occPos{Line: t.Line, Col: t.Col, Len: len([]rune(t.Value))})
		}
		prevType = t.Type
	}
	return out
}

// scopeLineRange - приблизительный диапазон строк тела scope: от declLine
// до строки перед ближайшим следующим объявлением функции/метода документа
// (или до конца файла, если scope - последний) - та же точность, что и у
// scopeAt (document.go).
func (d *doc) scopeLineRange(scope *funcScope) (from, to int) {
	from = scope.declLine
	to = d.tokens[len(d.tokens)-1].Line
	for i := range d.scopes {
		dl := d.scopes[i].declLine
		if dl > scope.declLine && dl-1 < to {
			to = dl - 1
		}
	}
	return from, to
}

// wordformMatchesLemma - true, если word - лемма lemma сама по себе (её
// именительный падеж - обычное, самое частое совпадение) либо любая её
// другая словоформа согласно d.dict (тем же движком - Dictionary.Analyze -
// что и падежное сопоставление аргументов естественного вызова).
func (d *doc) wordformMatchesLemma(word, lemma string) bool {
	if word == lemma {
		return true
	}
	if d.dict == nil {
		return false
	}
	for _, a := range d.dict.Analyze(word) {
		if a.POS == morph.Noun && a.Lemma == lemma {
			return true
		}
	}
	return false
}
