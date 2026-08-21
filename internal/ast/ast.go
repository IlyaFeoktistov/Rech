// Package ast описывает синтаксическое дерево Rech.
//
// Дерево не знает ничего о C, LLVM или интерпретаторе - оно описывает
// только структуру программы на русском языке. Backend'ы (cbackend и
// будущие interpreter/VM) читают это дерево и решают, во что его превратить.
// См. docs/ROADMAP.md, этап "IR": позже между AST и backend'ами появится
// отдельный слой IR, но интерфейс Stmt/Expr рассчитан так, чтобы это не
// потребовало переписывать парсер.
package ast

// Node - общий маркерный интерфейс всех узлов дерева.
type Node interface {
	node()
}

// Program - корень дерева: список объявленных функций.
type Program struct {
	Functions []*FunctionDecl
}

func (*Program) node() {}

// FunctionDecl соответствует конструкции:
//
//	функция <имя>
//	{
//	    <тело>
//	}
type FunctionDecl struct {
	Name string
	Body []Stmt
	Line int
}

func (*FunctionDecl) node() {}

// Stmt - инструкция внутри тела функции.
type Stmt interface {
	Node
	stmt()
}

// PrintStmt соответствует инструкции:
//
//	вывести <выражение>
type PrintStmt struct {
	Value Expr
	Line  int
}

func (*PrintStmt) node() {}
func (*PrintStmt) stmt() {}

// Expr - выражение, вычисляемое в значение.
type Expr interface {
	Node
	expr()
}

// StringLiteral - строковый литерал в двойных кавычках.
type StringLiteral struct {
	Value string
	Line  int
}

func (*StringLiteral) node() {}
func (*StringLiteral) expr() {}
