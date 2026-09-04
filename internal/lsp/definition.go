// Переход к определению ("go to definition") - функция/метод/сущность/
// поле/константа верхнего уровня документа, в том числе объявленных в
// ДРУГОМ файле, найденном через "использовать" (см. multifile.go); локальные
// переменные/параметры не поддержаны.
package lsp

import (
	"github.com/IlyaFeoktistov/Rech/internal/ir"
	"github.com/IlyaFeoktistov/Rech/internal/lexer"
)

// definitionPos - место объявления найденного под курсором имени: строка/
// столбец начала самого идентификатора (оба 1-индексные, как lexer.Token)
// и его длина в рунах, для построения LSP Range без повторного разбора
// строки на стороне вызывающего кода (server.go). URI - "" (позиция в ТЕКУЩЕМ
// открытом документе, вызывающая сторона в server.go подставляет URI самого
// запроса) или адрес ДРУГОГО файла (см. declPosInFile/multifile.go) -
// объявление, до которого дотянулись через "использовать", лежит не в том
// файле, где стоит курсор.
type definitionPos struct {
	URI            string
	Line, Col, Len int
}

// definition находит место объявления имени под курсором (line, col, оба
// 1-индексные) - тем же разбором цепочки "receiver.поле", что и hover (см.
// chainBefore/resolveChain, resolve.go).
func (d *doc) definition(line, col int) (definitionPos, bool) {
	if d.irProg == nil || d.astProg == nil {
		return definitionPos{}, false
	}
	i := d.tokenIndexAt(line, col)
	if i < 0 {
		return definitionPos{}, false
	}
	tok := d.tokens[i]
	if tok.Type != lexer.IDENT && tok.Type != lexer.KwThis {
		return definitionPos{}, false
	}
	chain := d.chainBefore(i)
	if len(chain) == 0 {
		return definitionPos{}, false
	}
	name := chain[len(chain)-1]

	if len(chain) == 1 {
		if fd, ok := d.astFuncs[name]; ok {
			return d.declPosInFile(fd.File, fd.Line, name)
		}
		if ed, ok := d.astEntities[name]; ok {
			return d.declPosInFile(ed.File, ed.Line, name)
		}
		return definitionPos{}, false
	}

	container, ok := d.resolveChain(chain[:len(chain)-1], line)
	if !ok || container.Kind != ir.KindEntity {
		return definitionPos{}, false
	}
	ed, ok := d.astEntities[container.Entity]
	if !ok {
		return definitionPos{}, false
	}
	for _, f := range ed.Fields {
		if f.Name == name {
			return d.declPosInFile(ed.File, f.Line, name)
		}
	}
	for _, c := range ed.Constants {
		if c.Name == name {
			return d.declPosInFile(ed.File, c.Line, name)
		}
	}
	for _, m := range ed.Methods {
		if m.Name == name {
			return d.declPosInFile(ed.File, m.Line, name)
		}
	}
	return definitionPos{}, false
}

// declPos - позиция первого IDENT-токена строки line со значением name В
// ТЕКУЩЕМ документе - объявления (ast.FunctionDecl/EntityDecl/Field/
// ConstDecl/MethodDecl) несут Line, но не Col, поэтому столбец имени
// восстанавливается поиском по уже разобранным токенам (тот же приём, что
// и в остальном пакете).
func (d *doc) declPos(line int, name string) (definitionPos, bool) {
	return declPosIn(d.tokens, line, name)
}

// declPosIn - как declPos, но по ПРОИЗВОЛЬНОМУ списку токенов - общий
// поиск, используемый и для текущего документа (declPos), и для чужого
// файла (declPosInFile).
func declPosIn(toks []lexer.Token, line int, name string) (definitionPos, bool) {
	for _, tok := range toks {
		if tok.Line == line && tok.Type == lexer.IDENT && tok.Value == name {
			return definitionPos{Line: line, Col: tok.Col, Len: len([]rune(name))}, true
		}
	}
	return definitionPos{}, false
}

// declPosInFile - как declPos, но объявление может лежать в ЛЮБОМ файле
// графа импортов, не только в текущем открытом документе - file берётся из
// ast.EntityDecl.File/ast.FunctionDecl.File (см. package doc, ast.StampFile).
// Тот же файл, что и текущий документ (isCurrentFile) - тот же результат,
// что и у declPos, URI не заполняется (сервер сам подставит URI запроса).
// Другой файл - читает и лексит его с диска (foreignTokens, кешируется на
// доке) для точного столбца; если это не удалось (файл переименован/удалён
// с последней линковки) или сам токен не нашёлся - приблизительная позиция
// (начало строки, нулевая длина) вместо отказа показать хоть что-то.
func (d *doc) declPosInFile(file string, line int, name string) (definitionPos, bool) {
	if d.isCurrentFile(file) {
		return d.declPos(line, name)
	}
	if toks, ok := d.foreignTokens(file); ok {
		if pos, ok := declPosIn(toks, line, name); ok {
			pos.URI = fileURI(file)
			return pos, true
		}
	}
	return definitionPos{URI: fileURI(file), Line: line, Col: 1, Len: 0}, true
}
