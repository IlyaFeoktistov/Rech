// Переход к реализации ("go to implementation") - от имени интерфейса или
// от вызова его метода через получателя интерфейсного типа - к каждой
// сущности программы, реально реализующей этот интерфейс (докстрока
// definitionPos/tokenIndexAt/chainBefore/resolveChain, см. definition.go/
// resolve.go - тот же разбор цепочки, что и у "go to definition"). В
// отличие от "go to definition" (ровно одно место), результат здесь может
// содержать НЕСКОЛЬКО позиций - интерфейс обычно реализован не одной
// сущностью.
package lsp

import (
	"github.com/IlyaFeoktistov/Rech/internal/ast"
	"github.com/IlyaFeoktistov/Rech/internal/ir"
	"github.com/IlyaFeoktistov/Rech/internal/lexer"
)

// implementations находит позиции реализаций для имени под курсором (line,
// col, оба 1-индексные) - см. package doc.
func (d *doc) implementations(line, col int) ([]definitionPos, bool) {
	if d.irProg == nil || d.astProg == nil {
		return nil, false
	}
	i := d.tokenIndexAt(line, col)
	if i < 0 {
		return nil, false
	}
	tok := d.tokens[i]
	if tok.Type != lexer.IDENT {
		return nil, false
	}
	chain := d.chainBefore(i)
	if len(chain) == 0 {
		return nil, false
	}
	name := chain[len(chain)-1]

	if len(chain) == 1 {
		// Курсор на самом имени интерфейса ("интерфейс Имя" объявление,
		// "тип Имя" у рефлексии, аннотация типа поля/параметра) - каждая
		// сущность программы, реально его реализующая (транзитивно, через
		// "реализует"/"наследует" - см. ir.Program.InterfaceImplementors).
		if d.astInterface(name) != nil {
			return d.entityDeclPositions(d.irProg.InterfaceImplementors[name])
		}
		// Не имя интерфейса само по себе - возможно, метод, вызванный на
		// РЕЗУЛЬТАТЕ ДРУГОГО вызова (`контейнер.логгер().инфо(...)`) -
		// chainBefore не строит цепочку сквозь "(...)" получателя (см. её
		// package doc), поэтому цепочка тут - только "инфо" (длина 1), не
		// цепочка "получатель.метод.метод". Ищем в уже разрешённом IR
		// (implementation_call.go) - там тип получателя уже известен,
		// какой бы сложной ни была сама цепочка перед ним.
		return d.implementationsForCallReceiver(name, line)
	}

	// Курсор на методе, вызванном через получателя интерфейсного типа
	// (`получатель.метод(...)`, получатель - простое имя/поле, БЕЗ вызова
	// в цепочке - см. resolveChain) - у каждой реализующей сущности перейти
	// к ЕЁ СОБСТВЕННОЙ реализации именно этого метода (не к самому
	// интерфейсу - у контракта нет тела).
	container, ok := d.resolveChain(chain[:len(chain)-1], line)
	if !ok || container.Kind != ir.KindInterface {
		return nil, false
	}
	return d.methodImplementationPositions(container.Entity, name)
}

// implementationsForCallReceiver - см. вызов из implementations выше и
// package doc implementation_call.go: получатель вызова "name" - результат
// ДРУГОГО вызова, не простое имя - ищет сам этот MethodCallExpr в уже
// разобранном IR текущей функции/метода (по строке курсора и имени
// метода) и читает тип ЕГО получателя напрямую, а не через chainBefore.
func (d *doc) implementationsForCallReceiver(name string, line int) ([]definitionPos, bool) {
	scope := d.scopeAt(line)
	if scope == nil {
		return nil, false
	}
	call, ok := findMethodCallOnLine(scope.body, line, name)
	if !ok {
		return nil, false
	}
	recv := ir.TypeOf(call.Receiver)
	if recv.Kind != ir.KindInterface {
		return nil, false
	}
	return d.methodImplementationPositions(recv.Entity, name)
}

// methodImplementationPositions - позиция реализации метода methodName у
// КАЖДОЙ сущности, реализующей интерфейс interfaceName.
func (d *doc) methodImplementationPositions(interfaceName, methodName string) ([]definitionPos, bool) {
	var out []definitionPos
	for _, entName := range d.irProg.InterfaceImplementors[interfaceName] {
		ed := d.astEntity(entName)
		if ed == nil {
			continue
		}
		for _, m := range ed.Methods {
			if m.Name != methodName {
				continue
			}
			if pos, ok := d.declPos(m.Line, methodName); ok {
				out = append(out, pos)
			}
			break
		}
	}
	return out, len(out) > 0
}

// astInterface ищет ast.InterfaceDecl по имени в d.astProg.Interfaces.
func (d *doc) astInterface(name string) *ast.InterfaceDecl {
	for _, iface := range d.astProg.Interfaces {
		if iface.Name == name {
			return iface
		}
	}
	return nil
}

// entityDeclPositions - позиция ИМЕНИ каждой сущности из names (реализация
// интерфейса без указания конкретного метода - сама реализующая сущность,
// не один из её методов).
func (d *doc) entityDeclPositions(names []string) ([]definitionPos, bool) {
	var out []definitionPos
	for _, name := range names {
		ed := d.astEntity(name)
		if ed == nil {
			continue
		}
		if pos, ok := d.declPos(ed.Line, name); ok {
			out = append(out, pos)
		}
	}
	return out, len(out) > 0
}
