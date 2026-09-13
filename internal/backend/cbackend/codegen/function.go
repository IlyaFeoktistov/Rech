package codegen

type Param struct {
	Type string
	Name string
	// Raw - Type уже несёт ЗАВЕРШЁННЫЙ C-декларатор (тип и имя вместе,
	// например "int (*compar)(int)" - указатель на функцию, чьё имя ДОЛЖНО
	// быть внутри скобок после "*", не может быть просто дописано после
	// всего типа обычным разделителем) - Name в этом случае не пишется,
	// см. StructDecl.write.
	Raw bool
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

// writeSignature пишет модификаторы + тип результата + имя + список
// параметров (с "void" при пустом списке) - общая часть Write/Prototype,
// чтобы объявление в прототипе не могло разойтись с настоящим определением.
func (f *Function) writeSignature(w *Writer) {
	for _, modifier := range f.Modifiers {
		w.Raw(modifier)
		w.Raw(" ")
	}

	w.writeTypedName(f.Return, f.Name)
	w.Raw("(")

	if len(f.ParamsBody) == 0 {
		// Пустой список параметров - всегда "(void)", как и везде в
		// cbackend (см. cParamListIR) - неявного "()" (означающего в C
		// "неизвестное число параметров", а не "ноль") здесь быть не должно.
		w.Raw("void")
	}

	for i, param := range f.ParamsBody {
		if i > 0 {
			w.Raw(", ")
		}

		w.writeTypedName(param.Type, param.Name)
	}

	w.Raw(")")
}

func (f *Function) Write(w *Writer) {
	w.writeIndent()
	f.writeSignature(w)
	w.Raw(" {\n")

	w.indent++

	for _, stmt := range f.Body {
		stmt.write(w)
	}

	w.indent--
	w.Line("}")
}

// Prototype пишет только сигнатуру, как объявление ("...;\n"), без тела -
// та же writeSignature, что и Write, гарантирует, что прототип не может
// разойтись с настоящим определением этой же *Function (см. package doc
// generateHeader про этот риск).
func (f *Function) Prototype(w *Writer) {
	w.writeIndent()
	f.writeSignature(w)
	w.Raw(";\n")
}
