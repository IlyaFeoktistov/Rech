// "N ссылок" над каждым собственным объявлением документа (функция/
// сущность/метод/поле/константа сущности) - кликабельно (LSP CodeLens,
// не текст внутри hover, см. server.go/references.go): клик открывает
// стандартную панель "Найдено ссылок" редактора со всеми найденными
// местами использования, а не просто печатает число, которым нельзя
// воспользоваться (прежнее поведение, "редизайн 'число
// ссылок' в hover").
package lsp

import "github.com/IlyaFeoktistov/Rech/internal/lexer"

// CodeLens - один "N ссылок" над объявлением на строке Line (1-индексная,
// как lexer.Token.Line) - Refs несёт позиции всех найденных ссылок (см.
// references.go/crossFileRefs - по всему дереву проекта, не только этому
// документу), на которые редактор перейдёт при клике.
type CodeLens struct {
	Line  int
	Title string
	Refs  []refLocation
}

// codeLenses строит по одному CodeLens на каждое СОБСТВЕННОЕ объявление
// текущего документа (d.astProg - всегда разбор именно этого буфера, см.
// package doc document.go, в отличие от d.entities/d.funcs, которые после
// многофайлового обогащения несут ещё и алиасы импортов ИЗ ДРУГИХ файлов -
// для них здесь показывать нечего, у них нет своей строки в ЭТОМ
// документе) - функции, сущности, их методы и поля/константы, в этом
// порядке.
func (d *doc) codeLenses() []CodeLens {
	if d.astProg == nil {
		return nil
	}
	var out []CodeLens
	for _, fn := range d.astProg.Functions {
		out = append(out, d.newCodeLens(fn.Line, matchFunctionRefs, fn.Name))
	}
	for _, ent := range d.astProg.Entities {
		out = append(out, d.newCodeLens(ent.Line, matchEntityRefs, ent.Name))
		for _, f := range ent.Fields {
			out = append(out, d.newCodeLens(f.Line, matchMemberRefs, f.Name))
		}
		for _, c := range ent.Constants {
			out = append(out, d.newCodeLens(c.Line, matchMemberRefs, c.Name))
		}
		for _, m := range ent.Methods {
			out = append(out, d.newCodeLens(m.Line, matchMethodRefs, m.Name))
		}
	}
	for _, c := range d.astProg.Consts {
		out = append(out, d.newCodeLens(c.Line, matchModuleConstRefs, c.Name))
	}
	return out
}

func (d *doc) newCodeLens(line int, matcher func([]lexer.Token, string) []lexer.Token, name string) CodeLens {
	refs := d.crossFileRefs(matcher, name)
	return CodeLens{Line: line, Title: refCountLabel(len(refs)), Refs: refs}
}
