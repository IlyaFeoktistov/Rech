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

// FieldDeclarator - поле, чей ПОЛНЫЙ C-декларатор (тип+имя вместе) уже
// готов - см. Param.Raw. Единственный сегодняшний потребитель - поле
// "Вызываемый(...)" "внешняя структура" (указатель на функцию, имя внутри
// скобок - "int (*fieldName)(int)", не "int (*)(int) fieldName").
func (s *StructDecl) FieldDeclarator(declarator string) *StructDecl {
	s.Fields = append(s.Fields, Param{Type: declarator, Raw: true})
	return s
}

func (s *StructDecl) write(w *Writer) {
	w.Line("typedef struct {")
	w.indent++
	for _, f := range s.Fields {
		w.writeIndent()
		if f.Raw {
			w.Raw(f.Type)
		} else {
			w.writeTypedName(f.Type, f.Name)
		}
		w.Raw(";\n")
	}
	w.indent--
	w.Line("} %s;", s.Name)
}

func (s *StructDecl) Write(w *Writer) {
	s.write(w)
}
