package codegen

type ExprStmt struct {
	Expr Expr
}

func ExprStatement(expr Expr) Stmt {
	return ExprStmt{Expr: expr}
}

func (ExprStmt) stmt() {}

func (s ExprStmt) write(w *Writer) {
	w.writeIndent()
	s.Expr.write(w)
	w.Raw(";\n")
}

type ReturnStmt struct {
	Value Expr
}

func Return(value Expr) Stmt {
	return ReturnStmt{Value: value}
}

func (ReturnStmt) stmt() {}

func (s ReturnStmt) write(w *Writer) {
	w.writeIndent()

	if s.Value != nil {
		w.Raw("return ")
		s.Value.write(w)
	} else {
		w.Raw("return")
	}

	w.Raw(";\n")
}

type AssignStmt struct {
	Left  Expr
	Right Expr
}

func Assign(left, right Expr) Stmt {
	return AssignStmt{
		Left:  left,
		Right: right,
	}
}

func (AssignStmt) stmt() {}

func (s AssignStmt) write(w *Writer) {
	w.writeIndent()
	s.Left.write(w)
	w.Raw(" = ")
	s.Right.write(w)
	w.Raw(";\n")
}

type DeclStmt struct {
	Type string
	Name string
	Init Expr
}

func Decl(typ, name string, init Expr) Stmt {
	return DeclStmt{
		Type: typ,
		Name: name,
		Init: init,
	}
}

func (DeclStmt) stmt() {}

func (s DeclStmt) write(w *Writer) {
	w.writeIndent()
	w.Raw(s.Type)
	w.Raw(" ")
	w.Raw(s.Name)

	if s.Init != nil {
		w.Raw(" = ")
		s.Init.write(w)
	}

	w.Raw(";\n")
}

type IfStmt struct {
	Condition Expr
	Then      []Stmt
	ElseBody  []Stmt
}

func If(condition Expr, then ...Stmt) *IfStmt {
	return &IfStmt{
		Condition: condition,
		Then:      then,
	}
}

func (IfStmt) stmt() {}

func (s *IfStmt) write(w *Writer) {
	w.writeIndent()
	w.Raw("if (")
	s.Condition.write(w)
	w.Raw(") ")

	w.Raw("{\n")
	w.indent++

	for _, stmt := range s.Then {
		stmt.write(w)
	}

	w.indent--
	w.writeIndent()
	w.Raw("}")

	if len(s.ElseBody) > 0 {
		w.Raw(" else {\n")
		w.indent++

		for _, stmt := range s.ElseBody {
			stmt.write(w)
		}

		w.indent--
		w.writeIndent()
		w.Raw("}")
	}

	w.Raw("\n")
}

func (s *IfStmt) Else(stmts ...Stmt) *IfStmt {
	s.ElseBody = stmts
	return s
}
