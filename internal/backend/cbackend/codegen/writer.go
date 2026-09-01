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
