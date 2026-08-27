// Переход к определению ("go to definition") - функция/метод/сущность/
// поле/константа верхнего уровня документа; локальные переменные/
// параметры не поддержаны (тот же класс ограничения "однофайловый
// документ", что и у остального пакета, см. package doc document.go).
package lsp

import (
	"github.com/IlyaFeoktistov/Rech/internal/ast"
	"github.com/IlyaFeoktistov/Rech/internal/ir"
	"github.com/IlyaFeoktistov/Rech/internal/lexer"
)

// definitionPos - место объявления найденного под курсором имени: строка/
// столбец начала самого идентификатора (оба 1-индексные, как lexer.Token)
// и его длина в рунах, для построения LSP Range без повторного разбора
// строки на стороне вызывающего кода (server.go).
type definitionPos struct {
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
		for _, fd := range d.astProg.Functions {
			if fd.Name == name {
				return d.declPos(fd.Line, name)
			}
		}
		for _, ed := range d.astProg.Entities {
			if ed.Name == name {
				return d.declPos(ed.Line, name)
			}
		}
		return definitionPos{}, false
	}

	container, ok := d.resolveChain(chain[:len(chain)-1], line)
	if !ok || container.Kind != ir.KindEntity {
		return definitionPos{}, false
	}
	ed := d.astEntity(container.Entity)
	if ed == nil {
		return definitionPos{}, false
	}
	for _, f := range ed.Fields {
		if f.Name == name {
			return d.declPos(f.Line, name)
		}
	}
	for _, c := range ed.Constants {
		if c.Name == name {
			return d.declPos(c.Line, name)
		}
	}
	for _, m := range ed.Methods {
		if m.Name == name {
			return d.declPos(m.Line, name)
		}
	}
	return definitionPos{}, false
}

// astEntity ищет ast.EntityDecl по имени сущности в d.astProg.
func (d *doc) astEntity(name string) *ast.EntityDecl {
	for _, e := range d.astProg.Entities {
		if e.Name == name {
			return e
		}
	}
	return nil
}

// declPos - позиция первого IDENT-токена строки line со значением name -
// объявления (ast.FunctionDecl/EntityDecl/Field/ConstDecl/MethodDecl) несут
// Line, но не Col, поэтому столбец имени восстанавливается поиском по уже
// разобранным токенам (тот же приём, что и в остальном пакете).
func (d *doc) declPos(line int, name string) (definitionPos, bool) {
	for _, tok := range d.tokens {
		if tok.Line == line && tok.Type == lexer.IDENT && tok.Value == name {
			return definitionPos{Line: line, Col: tok.Col, Len: len([]rune(name))}, true
		}
	}
	return definitionPos{}, false
}
