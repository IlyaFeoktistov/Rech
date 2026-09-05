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

// matchFunctionRefs - IDENT-токен name в toks, за которым сразу "(" (вызов)
// или перед которым "&" (FuncRefExpr) - НЕ считая само объявление ("функция
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
		if i+1 < len(toks) && toks[i+1].Type == lexer.LPAREN {
			out = append(out, tok)
		}
	}
	return out
}

// matchMethodRefs - "получатель.имя(...)" в toks - объявление метода само
// не подходит (методы объявляются без ведущей "." перед именем).
func matchMethodRefs(toks []lexer.Token, name string) []lexer.Token {
	var out []lexer.Token
	for i, tok := range toks {
		if tok.Type == lexer.IDENT && tok.Value == name && i > 0 && toks[i-1].Type == lexer.DOT && i+1 < len(toks) && toks[i+1].Type == lexer.LPAREN {
			out = append(out, tok)
		}
	}
	return out
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
// ДРУГОЙ файл - читается и лексится с диска заново (приближённо, как и
// весь пакет - см. package doc выше). d.path == "" (нет реального файла на
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
	mod100 := n % 100
	if mod100 >= 11 && mod100 <= 14 {
		return fmt.Sprintf("%d ссылок", n)
	}
	switch n % 10 {
	case 1:
		return fmt.Sprintf("%d ссылка", n)
	case 2, 3, 4:
		return fmt.Sprintf("%d ссылки", n)
	default:
		return fmt.Sprintf("%d ссылок", n)
	}
}
