// Приблизительные позиции ссылок на объявление - по токенам, ЛИБО
// текущего документа (см. package doc resolve.go про то, почему это
// приемлемо: подсчёт не восстанавливает области видимости, значит
// переменная/поле другой сущности с тем же именем даёт ложное совпадение -
// тот же класс упрощения, что и у остального пакета, не претендует на
// точность, нужную для настоящего переименования/рефакторинга), ЛИБО (для
// codelens.go - объявление документа может использоваться СОВСЕМ в другом
// файле проекта, см. workspace_scan.go) - всего дерева .rech-файлов под
// корнем проекта. Используются в трёх местах: codelens.go (кликабельная
// "N ссылок" над объявлением, по всему проекту), deprecated.go
// (зачёркивание каждого вхождения устаревшего имени, только в текущем
// документе - там курсор всё равно только в нём).
package lsp

import (
	"fmt"
	"path/filepath"

	"github.com/IlyaFeoktistov/Rech/internal/lexer"
)

// matchFunctionRefs - IDENT-токен name в toks, за которым сразу "(" (вызов,
// в т. ч. generic - "имя<Тип, ...>(...)", см. isCallSite) или перед
// которым "&" (FuncRefExpr) - НЕ считая само объявление ("функция
// имя(...)", отличается предшествующим KwFunction) и не считая
// "получатель.имя(...)" (это вызов МЕТОДА с тем же именем, см.
// matchMethodRefs).
func matchFunctionRefs(toks []lexer.Token, name string) []lexer.Token {
	var out []lexer.Token
	for i, tok := range toks {
		if tok.Type != lexer.IDENT || tok.Value != name {
			continue
		}
		if i > 0 && toks[i-1].Type == lexer.KwFunction {
			continue
		}
		if i > 0 && toks[i-1].Type == lexer.DOT {
			continue
		}
		if i > 0 && toks[i-1].Type == lexer.BITAND {
			out = append(out, tok)
			continue
		}
		if isCallSite(toks, i) {
			out = append(out, tok)
		}
	}
	return out
}

// matchMethodRefs - "получатель.имя(...)" в toks (в т. ч. generic-вызов
// метода - "получатель.имя<Тип>(...)", см. isCallSite) - объявление
// метода само не подходит (методы объявляются без ведущей "." перед
// именем).
func matchMethodRefs(toks []lexer.Token, name string) []lexer.Token {
	var out []lexer.Token
	for i, tok := range toks {
		if tok.Type == lexer.IDENT && tok.Value == name && i > 0 && toks[i-1].Type == lexer.DOT && isCallSite(toks, i) {
			out = append(out, tok)
		}
	}
	return out
}

// isCallSite - true, если IDENT на позиции i в toks - место ВЫЗОВА (сразу
// "(", либо список аргументов типа generic-вызова "<Тип, ...>" перед "(" -
// "имя<Тип>(...)"/"получатель.имя<Тип>(...)") - без этого случая счётчик
// ссылок (codelens.go) не находил ни одного вызова generic-функции/метода
// вообще, показывая "0 ссылок" при реальном использовании (репортнуто
// пользователем, examples/бд/чанкование.rech - "чанковать_по_полю<T>",
// вызывается дважды в том же файле).
func isCallSite(toks []lexer.Token, i int) bool {
	next := i + 1
	if next < len(toks) && toks[next].Type == lexer.LT {
		if closeIdx, ok := skipGenericArgs(toks, next); ok {
			next = closeIdx + 1
		}
	}
	return next < len(toks) && toks[next].Type == lexer.LPAREN
}

// skipGenericArgs - от индекса ПЕРВОЙ "<" (ltIdx) пытается найти индекс
// закрывающей ">"/половины ">>" списка АРГУМЕНТОВ ТИПА generic-вызова -
// тем же классом эвристики, что и остальной пакет (не настоящий парсер,
// приближение, см. package doc выше): содержимое между "<" и ">" обязано
// выглядеть как список имён типов (IDENT/","/"?" nullable/вложенные
// "<...>") - при первом токене, не подходящем под этот класс, сдаётся
// (ok=false - это НЕ generic-список, а обычное "<" сравнения).
func skipGenericArgs(toks []lexer.Token, ltIdx int) (closeIdx int, ok bool) {
	depth := 0
	for i := ltIdx; i < len(toks); i++ {
		switch toks[i].Type {
		case lexer.LT:
			depth++
		case lexer.GT:
			depth--
			if depth == 0 {
				return i, true
			}
			if depth < 0 {
				return 0, false
			}
		case lexer.SHR:
			depth -= 2
			if depth == 0 {
				return i, true
			}
			if depth < 0 {
				return 0, false
			}
		case lexer.IDENT, lexer.COMMA, lexer.QUESTION:
			// часть списка имён типов - продолжаем.
		default:
			return 0, false
		}
	}
	return 0, false
}

// matchMemberRefs - "получатель.имя" (поле или константа) в toks - без
// требования "(" следом (в отличие от matchMethodRefs).
func matchMemberRefs(toks []lexer.Token, name string) []lexer.Token {
	var out []lexer.Token
	for i, tok := range toks {
		if tok.Type == lexer.IDENT && tok.Value == name && i > 0 && toks[i-1].Type == lexer.DOT {
			out = append(out, tok)
		}
	}
	return out
}

// matchModuleConstRefs - IDENT(name) как самостоятельная ссылка на
// константу МОДУЛЬНОГО уровня ("голым именем" в теле функции, docs/
// SPEC.md) - не считает саму декларацию ("константа ИМЯ = ...", отличается
// предшествующим KwConst) и не считает "получатель.ИМЯ" (чтение ПОЛЯ/
// константы сущности с тем же именем - не модульная константа, тот же
// принцип, что и у matchFunctionRefs про DOT). Имя модульной константы
// гарантированно уникально среди функций/сущностей/перечислений/констант
// этого файла (collectModuleConsts, internal/semantic/analyzer.go) -
// совпадение с именем чего-то ещё в ЭТОМ файле исключено. Раньше
// codelens.go вообще не строил "N ссылок" для модульных констант
// (репортнуто пользователем).
func matchModuleConstRefs(toks []lexer.Token, name string) []lexer.Token {
	var out []lexer.Token
	for i, tok := range toks {
		if tok.Type != lexer.IDENT || tok.Value != name {
			continue
		}
		if i > 0 && (toks[i-1].Type == lexer.KwConst || toks[i-1].Type == lexer.DOT) {
			continue
		}
		out = append(out, tok)
	}
	return out
}

// matchEntityRefs - "новый/новая/новое/новые Имя(...)" в toks (создание
// экземпляра) - НЕ учитывает использование Имя как типа поля/параметра/
// результата: у такого использования нет отдельного токена, отличимого от
// имени сущности где-либо ещё, значит для него не построить кликабельную
// позицию - оставлять его в счёте, но не в списке ссылок, значило бы
// показывать "N ссылок", которых меньше N при клике (см. codelens.go).
func matchEntityRefs(toks []lexer.Token, name string) []lexer.Token {
	var out []lexer.Token
	for i, tok := range toks {
		if tok.Type == lexer.IDENT && tok.Value == name && i > 0 && toks[i-1].Type == lexer.KwNew {
			out = append(out, tok)
		}
	}
	return out
}

// functionRefTokens/methodRefTokens/entityRefTokens - варианты match*Refs
// выше по токенам ЭТОГО документа - используются deprecated.go (курсор
// всегда в текущем документе, зачёркивать использования в других файлах
// незачем).
func (d *doc) functionRefTokens(name string) []lexer.Token { return matchFunctionRefs(d.tokens, name) }
func (d *doc) methodRefTokens(name string) []lexer.Token   { return matchMethodRefs(d.tokens, name) }
func (d *doc) entityRefTokens(name string) []lexer.Token   { return matchEntityRefs(d.tokens, name) }

// refLocation - одна найденная ссылка (crossFileRefs) - File == "" -
// текущий документ (вызывающая сторона подставляет URI запроса, тот же
// принцип, что и у definitionPos.URI, definition.go), иначе - путь файла
// на диске, найденного при обходе дерева проекта.
type refLocation struct {
	File string
	Tok  lexer.Token
}

// crossFileRefs ищет ВСЕ ссылки на name (matcher - matchFunctionRefs и т.
// п. выше) по ВСЕМУ дереву проекта, не только по текущему документу -
// иначе функция/сущность/метод/поле, объявленные в одном файле и
// используемые ТОЛЬКО из другого (обычное дело - см. package doc
// module.go про "использовать"), показывали бы "0 ссылок", хотя реально
// используются (живая проверка на examples/проекты/http_сервер,
// "создать_токен": объявлена в исходники/токен/токен.rech, вызывается из
// исходники/авторизация/авторизация.rech). Текущий документ ищется по
// d.tokens (живой буфер, не то, что на диске - в отличие от остальных
// файлов дерева, см. package doc listRechFiles/workspace_scan.go), каждый
// ДРУГОЙ файл-КАНДИДАТ - читается и лексится с диска заново (приближённо,
// как и весь пакет - см. package doc выше), но засчитывается ТОЛЬКО если
// fileImportsTarget подтверждает, что кандидат действительно (прямо или
// транзитивно) использует d.path - иначе текстовое совпадение имени между
// ДВУМЯ НЕСВЯЗАННЫМИ файлами (в т. ч. из разных .rechproj-проектов рядом
// на диске) засчиталось бы как ссылка, хотя один файл вообще не видит
// объявление другого (репортнуто пользователем, см. package doc
// multifile.go/fileImportsTarget). d.path == "" (нет реального файла на
// диске, см. multifile.go) - результат ограничен текущим документом, тем
// же поведением, что и раньше.
func (d *doc) crossFileRefs(matcher func([]lexer.Token, string) []lexer.Token, name string) []refLocation {
	var out []refLocation
	for _, tok := range matcher(d.tokens, name) {
		out = append(out, refLocation{Tok: tok})
	}
	if d.path == "" {
		return out
	}
	for _, file := range listRechFiles(scanRoot(filepath.Dir(d.path)), d.path) {
		if !fileImportsTarget(file, d.path) {
			continue
		}
		toks, ok := d.foreignTokens(file)
		if !ok {
			continue
		}
		for _, tok := range matcher(toks, name) {
			out = append(out, refLocation{File: file, Tok: tok})
		}
	}
	return out
}

// refCountLabel - число ссылок, согласованное со словом "ссылка" (1
// ссылка, 2 ссылки, 5 ссылок, 11 ссылок, 21 ссылка, ...) - обычное правило
// русских числительных (11-14 всегда "ссылок", иначе по последней цифре).
func refCountLabel(n int) string {
	return ruCountLabel(n, "ссылка", "ссылки", "ссылок")
}

// ruCountLabel - число n, согласованное с существительным (one - "1 файл",
// few - "2/3/4 файла", many - "5.../11-14.../0 файлов") - то же правило
// русских числительных, что и у refCountLabel, только не завязанное на
// конкретное слово "ссылка" - используется им же и overrideCountLabel
// (codelens.go), чтобы не копировать правило склонения дважды.
func ruCountLabel(n int, one, few, many string) string {
	mod100 := n % 100
	if mod100 >= 11 && mod100 <= 14 {
		return fmt.Sprintf("%d %s", n, many)
	}
	switch n % 10 {
	case 1:
		return fmt.Sprintf("%d %s", n, one)
	case 2, 3, 4:
		return fmt.Sprintf("%d %s", n, few)
	default:
		return fmt.Sprintf("%d %s", n, many)
	}
}
