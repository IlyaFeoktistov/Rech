// "N ссылок" над каждым собственным объявлением документа (функция/
// сущность/интерфейс/перечисление/синоним/метод/поле/константа/
// переменная модуля) - кликабельно (LSP CodeLens,
// не текст внутри hover, см. server.go/references.go): клик открывает
// стандартную панель "Найдено ссылок" редактора со всеми найденными
// местами использования, а не просто печатает число, которым нельзя
// воспользоваться (прежнее поведение, "редизайн 'число
// ссылок' в hover"). Плюс "N переопределений" (overrides.go) - ВТОРЫМ
// CodeLens на той же строке метода (VS Code сам сводит несколько
// CodeLens одной строки в один визуальный ряд через " | ") - только у
// метода, реально переопределённого хотя бы одним потомком по цепочке
// "наследует" (0 переопределений не показывается вовсе, в отличие от "0
// ссылок" - иначе КАЖДЫЙ метод любой сущности без наследников получал бы
// лишнюю, всегда нулевую надпись). "Запустить тест" над "функция главная"
// тестового файла раньше был отдельным CodeLens здесь же - переведён на
// нативный vscode.tests API (rech-vscode/extension.js, TestController)
// ради настоящей зелёной стрелки-гаттера рядом с функцией и узла в панели
// Testing, которых обычный текстовый CodeLens дать не может (тот всегда
// рисуется НАД строкой).
package lsp

import (
	"github.com/IlyaFeoktistov/Rech/internal/lexer"
)

// CodeLens - один пункт CodeLens на строке Line (1-индексная, как
// lexer.Token.Line) - server.go сам строит клиентскую команду
// "editor.action.showReferences" из Refs (позиции ссылок по всему дереву
// проекта, см. references.go/crossFileRefs).
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
	typeRefs := d.typeRefMatcher()
	for _, ent := range d.astProg.Entities {
		out = append(out, d.newCodeLens(ent.Line, typeRefs, ent.Name))
		for _, f := range ent.Fields {
			out = append(out, d.newCodeLens(f.Line, matchMemberRefs, f.Name))
		}
		for _, c := range ent.Constants {
			out = append(out, d.newCodeLens(c.Line, matchMemberRefs, c.Name))
		}
		for _, m := range ent.Methods {
			out = append(out, d.newCodeLens(m.Line, matchMethodRefs, m.Name))
			if overrides := d.overrideRefs(ent.Name, m); len(overrides) > 0 {
				out = append(out, CodeLens{Line: m.Line, Title: overrideCountLabel(len(overrides)), Refs: overrides})
			}
		}
	}
	for _, c := range d.astProg.Consts {
		out = append(out, d.newCodeLens(c.Line, matchModuleConstRefs, c.Name))
	}
	for _, iface := range d.astProg.Interfaces {
		out = append(out, d.newCodeLens(iface.Line, typeRefs, iface.Name))
	}
	for _, e := range d.astProg.Enums {
		out = append(out, d.newCodeLens(e.Line, typeRefs, e.Name))
	}
	for _, s := range d.astProg.Synonyms {
		out = append(out, d.newCodeLens(s.Line, typeRefs, s.Name))
	}
	for _, v := range d.astProg.Vars {
		out = append(out, d.newCodeLens(v.Line, matchModuleConstRefs, v.Name))
	}
	return out
}

func (d *doc) newCodeLens(line int, matcher func([]lexer.Token, string) []lexer.Token, name string) CodeLens {
	refs := d.crossFileRefs(matcher, name)
	return CodeLens{Line: line, Title: refCountLabel(len(refs)), Refs: refs}
}
