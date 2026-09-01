package codegen

type Node interface {
	write(*Writer)
}

type Expr interface {
	Node
	expr()
}

type Stmt interface {
	Node
	stmt()
}
