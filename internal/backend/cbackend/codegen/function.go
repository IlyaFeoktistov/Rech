package codegen

type Param struct {
	Type string
	Name string
}

func Parameter(typ, name string) Param {
	return Param{
		Type: typ,
		Name: name,
	}
}

type Function struct {
	Modifiers  []string
	Return     string
	Name       string
	ParamsBody []Param
	Body       []Stmt
}

func Func(name string) *Function {
	return &Function{
		Return: "void",
		Name:   name,
	}
}

func (f *Function) Static() *Function {
	f.Modifiers = append(f.Modifiers, "static")
	return f
}

func (f *Function) ReturnType(typ string) *Function {
	f.Return = typ
	return f
}

func (f *Function) Params(params ...Param) *Function {
	f.ParamsBody = params
	return f
}

func (f *Function) Statements(stmts ...Stmt) *Function {
	f.Body = append(f.Body, stmts...)
	return f
}

func (f *Function) Write(w *Writer) {
	w.writeIndent()

	for _, modifier := range f.Modifiers {
		w.Raw(modifier)
		w.Raw(" ")
	}

	w.Raw(f.Return)
	w.Raw(" ")
	w.Raw(f.Name)
	w.Raw("(")

	for i, param := range f.ParamsBody {
		if i > 0 {
			w.Raw(", ")
		}

		w.Raw(param.Type)
		w.Raw(" ")
		w.Raw(param.Name)
	}

	w.Raw(") {\n")

	w.indent++

	for _, stmt := range f.Body {
		stmt.write(w)
	}

	w.indent--
	w.Line("}")
}
