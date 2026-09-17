// "N ссылок" над каждым собственным объявлением документа (функция/
// сущность/метод/поле/константа сущности) - кликабельно (LSP CodeLens,
// не текст внутри hover, см. server.go/references.go): клик открывает
// стандартную панель "Найдено ссылок" редактора со всеми найденными
// местами использования, а не просто печатает число, которым нельзя
// воспользоваться (прежнее поведение, "редизайн 'число
// ссылок' в hover"). Плюс "запустить тест" над "функция главная" тестового
// файла (testrunner.TestFileSuffix) - единственный лишний CodeLens этого
// пакета, у которого команда клиента - НЕ встроенная
// "editor.action.showReferences" (см. Command/Args ниже).
package lsp

import (
	"strings"

	"github.com/IlyaFeoktistov/Rech/internal/lexer"
	"github.com/IlyaFeoktistov/Rech/internal/testrunner"
)

// CodeLens - один пункт CodeLens на строке Line (1-индексная, как
// lexer.Token.Line). Command == "" (обычный случай, "N ссылок") -
// server.go сам строит клиентскую команду "editor.action.showReferences"
// из Refs (позиции ссылок по всему дереву проекта, см. references.go/
// crossFileRefs); Command != "" (только "запустить тест" ниже) - готовая
// клиентская команда с аргументами Args, Refs не используется вовсе.
type CodeLens struct {
	Line    int
	Title   string
	Refs    []refLocation
	Command string
	Args    []any
}

// runTestCommand - имя клиентской команды VS Code (rech-vscode/
// extension.js, registerCommand), которую запускает клик по "запустить
// тест" - в отличие от "editor.action.showReferences" (штатная команда
// редактора), это команда САМОГО расширения, ей нужен путь файла как
// единственный аргумент.
const runTestCommand = "rech.runTestFile"

// mainFuncName - "функция главная" (internal/parser.entryPointName,
// неэкспортировано) - точка входа тестового файла, ровно как в обычной
// программе (testrunner запускает её тем же путём, что и "rech run").
const mainFuncName = "главная"

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
	if lens, ok := d.runTestCodeLens(); ok {
		out = append(out, lens)
	}
	return out
}

// runTestCodeLens - "▶ запустить тест" над "функция главная" - только для
// тестового файла (testrunner.TestFileSuffix) с настоящим путём на диске
// (d.path == "" у тестовых "test://..." документов пакета - там некуда
// передавать команде путь для "rech test <файл>"). false, если файл - не
// тест, у него нет пути, либо в нём нет "функция главная" вовсе (частично
// написанный файл, лениентный разбор ещё не дошёл до неё/не нашёл).
func (d *doc) runTestCodeLens() (CodeLens, bool) {
	if d.path == "" || !strings.HasSuffix(d.path, testrunner.TestFileSuffix) {
		return CodeLens{}, false
	}
	for _, fn := range d.astProg.Functions {
		if fn.Name == mainFuncName {
			return CodeLens{Line: fn.Line, Title: "▶ запустить тест", Command: runTestCommand, Args: []any{d.path}}, true
		}
	}
	return CodeLens{}, false
}

func (d *doc) newCodeLens(line int, matcher func([]lexer.Token, string) []lexer.Token, name string) CodeLens {
	refs := d.crossFileRefs(matcher, name)
	return CodeLens{Line: line, Title: refCountLabel(len(refs)), Refs: refs}
}
