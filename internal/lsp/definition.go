// Переход к определению ("go to definition") - функция/метод/сущность/
// поле/константа верхнего уровня документа, в том числе объявленных в
// ДРУГОМ файле, найденном через "использовать" (см. multifile.go); локальная
// переменная/параметр (в пределах текущей функции/метода) и "этот" (к
// объявлению владеющей сущности) - тоже.
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
		scope := d.scopeAt(line)
		// Порядок приоритета - тот же, что у hoverBareName (resolve.go
		// такого приоритета не задаёт сам по себе, но конфликт имени
		// локальной переменной/параметра с функцией/сущностью верхнего
		// уровня того же имени - обычное затенение, локальная переменная
		// должна выигрывать): "этот" -> локальная переменная/параметр ->
		// функция -> сущность.
		if name == "этот" && scope != nil && scope.entityName != "" {
			if ed, ok := d.astEntities[scope.entityName]; ok {
				return d.declPosInFile(ed.File, ed.Line, scope.entityName)
			}
			return definitionPos{}, false
		}
		if pos, ok := d.bareNameDeclPos(name, line, scope); ok {
			return pos, true
		}
		if pos, ok := builtinDefinitionPos(name); ok {
			return pos, true
		}
		if pos, ok := builtinTypeDefinitionPos(name); ok {
			return pos, true
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

// bareNameDeclPos - "этот"-независимая часть definition() для chain длины
// 1: локальная переменная/параметр -> функция -> сущность, буквальным
// совпадением, а если оно не нашлось - падежной/деепричастной словоформой
// (wordform_definition.go): написанное под курсором имя один раз резолвится
// к лемме объявления и bareNameDeclPos вызывает себя же с ней - лемма
// гарантированно совпадёт буквально на этом втором вызове (иначе она не
// была бы объявленной леммой), дальше рекурсия не идёт.
func (d *doc) bareNameDeclPos(name string, line int, scope *funcScope) (definitionPos, bool) {
	if scope != nil {
		if pos, ok := d.localDeclPos(scope, line, name); ok {
			return pos, true
		}
	}
	if fd, ok := d.astFuncs[name]; ok {
		return d.declPosInFile(fd.File, fd.Line, name)
	}
	if ed, ok := d.astEntities[name]; ok {
		return d.declPosInFile(ed.File, ed.Line, name)
	}
	if sd, ok := d.astSynonyms[name]; ok {
		return d.declPosInFile(sd.File, sd.Line, name)
	}
	if scope != nil {
		if lemma, ok := d.localWordform(scope, line, name); ok {
			return d.bareNameDeclPos(lemma, line, scope)
		}
	}
	if lemma, ok := d.funcGerund(name); ok {
		return d.bareNameDeclPos(lemma, line, scope)
	}
	if lemma, ok := d.entityWordform(name); ok {
		return d.bareNameDeclPos(lemma, line, scope)
	}
	return definitionPos{}, false
}

// localDeclPos - место объявления локальной переменной/параметра name,
// ДЕЙСТВУЮЩЕЕ в строке line внутри scope - то же правило "последнее
// объявление до этой строки побеждает" (не моделирует область видимости
// вложенных блоков по-настоящему), что и у localsAt/collectVarDecls
// (document.go) - только здесь нужно ме́сто объявления, не тип. Переменная
// (`переменная x = ...`) - её собственная строка; параметр - строка
// объявления самой функции/метода (весь список параметров предполагается
// на этой же строке - тот же приём, что и у declPos везде в пакете).
func (d *doc) localDeclPos(scope *funcScope, line int, name string) (definitionPos, bool) {
	lines := map[string]int{}
	collectVarDeclLines(scope.body, line, lines)
	if declLine, ok := lines[name]; ok {
		return d.declPos(declLine, name)
	}
	for _, p := range scope.params {
		if p.Name == name {
			return d.declPos(scope.declLine, name)
		}
	}
	return definitionPos{}, false
}

// collectVarDeclLines - как collectVarDecls (document.go), но копит СТРОКУ
// объявления вместо типа - структурно идентичный обход (тот же порядок
// рекурсии в те же вложенные тела), чтобы результат совпадал с тем, что
// показывает hover для того же имени в той же точке.
func collectVarDeclLines(stmts []ir.Stmt, maxLine int, out map[string]int) {
	for _, s := range stmts {
		switch s := s.(type) {
		case *ir.VarDecl:
			if s.Line <= maxLine {
				out[s.Name] = s.Line
			}
		case *ir.IfStmt:
			collectVarDeclLines(s.Then, maxLine, out)
			collectVarDeclLines(s.Else, maxLine, out)
		case *ir.WhileStmt:
			collectVarDeclLines(s.Body, maxLine, out)
		case *ir.ForStmt:
			if s.Line <= maxLine {
				out[s.Var] = s.Line
			}
			collectVarDeclLines(s.Body, maxLine, out)
		case *ir.ForEachStmt:
			if s.Line <= maxLine {
				if s.KeyVar != "" {
					out[s.KeyVar] = s.Line
				}
				out[s.Var] = s.Line
			}
			collectVarDeclLines(s.Body, maxLine, out)
		case *ir.CStyleForStmt:
			if s.Init != nil && s.Line <= maxLine {
				out[s.Init.Name] = s.Line
			}
			collectVarDeclLines(s.Body, maxLine, out)
		case *ir.TryStmt:
			collectVarDeclLines(s.Body, maxLine, out)
			collectVarDeclLines(s.CatchBody, maxLine, out)
		case *ir.GerundStmt:
			collectVarDeclLines(s.Body, maxLine, out)
		case *ir.BlockStmt:
			collectVarDeclLines(s.Body, maxLine, out)
		}
	}
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
	tok, ok := tokenAt(toks, line, name)
	if !ok {
		return definitionPos{}, false
	}
	return definitionPos{Line: line, Col: tok.Col, Len: len([]rune(name))}, true
}

// tokenAt - первый IDENT-токен строки line со значением name в toks - общий
// поиск, используемый declPosIn выше и overrideRefs (overrides.go), у
// которого позиция нужна не готовой definitionPos, а самим lexer.Token
// (refLocation, того же вида, что и у "N ссылок").
func tokenAt(toks []lexer.Token, line int, name string) (lexer.Token, bool) {
	for _, tok := range toks {
		if tok.Line == line && tok.Type == lexer.IDENT && tok.Value == name {
			return tok, true
		}
	}
	return lexer.Token{}, false
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
