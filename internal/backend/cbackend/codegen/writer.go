package codegen

import (
	"fmt"
	"strings"
)

type Writer struct {
	sb     strings.Builder
	indent int
}

func NewWriter() *Writer {
	return &Writer{}
}

func (w *Writer) String() string {
	return w.sb.String()
}

func (w *Writer) writeIndent() {
	for i := 0; i < w.indent; i++ {
		w.sb.WriteString("    ")
	}
}

func (w *Writer) Line(format string, args ...any) {
	w.writeIndent()
	fmt.Fprintf(&w.sb, format, args...)
	w.sb.WriteByte('\n')
}

func (w *Writer) Raw(s string) {
	w.sb.WriteString(s)
}

func (w *Writer) Block(fn func()) {
	w.Line("{")
	w.indent++
	fn()
	w.indent--
	w.Line("}")
}

// typeSep - разделитель между типом и последующим именем (переменной,
// параметра, функции): без пробела, если typ уже оканчивается на "*" - в
// этой кодовой базе указатель всегда пишется слитно с именем ("T *name",
// не "T* name" и не "T * name" - см. cParamListIR/cMethodParamListIR и
// весь остальной cbackend), иначе один пробел.
func typeSep(typ string) string {
	if strings.HasSuffix(typ, "*") {
		return ""
	}
	return " "
}

// writeTypedName пишет "typ" + разделитель (typeSep) + "name" - общая точка
// для Decl/Param/Function, чтобы указатели везде форматировались одинаково.
func (w *Writer) writeTypedName(typ, name string) {
	w.Raw(typ)
	w.Raw(typeSep(typ))
	w.Raw(name)
}
