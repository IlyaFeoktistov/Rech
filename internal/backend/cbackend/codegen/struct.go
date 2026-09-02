package codegen

// StructDecl - "typedef struct { ... } Name;" - список типизированных полей
// (Param переиспользуется, поле структуры - та же пара "тип, имя", что и
// параметр функции).
type StructDecl struct {
	Name   string
	Fields []Param
}

func Struct(name string) *StructDecl {
	return &StructDecl{Name: name}
}

func (s *StructDecl) Field(typ, name string) *StructDecl {
	s.Fields = append(s.Fields, Parameter(typ, name))
	return s
}

func (s *StructDecl) write(w *Writer) {
	w.Line("typedef struct {")
	w.indent++
	for _, f := range s.Fields {
		w.writeIndent()
		w.writeTypedName(f.Type, f.Name)
		w.Raw(";\n")
	}
	w.indent--
	w.Line("} %s;", s.Name)
}

func (s *StructDecl) Write(w *Writer) {
	s.write(w)
}
