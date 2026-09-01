package codegen

import "fmt"

type RawExpr struct {
	Code string
}

func Raw(code string) Expr {
	return RawExpr{Code: code}
}

func (RawExpr) expr() {}
func (e RawExpr) write(w *Writer) {
	w.Raw(e.Code)
}

type VarExpr struct {
	Name string
}

func Var(name string) Expr {
	return VarExpr{Name: name}
}

func (VarExpr) expr() {}
func (e VarExpr) write(w *Writer) {
	w.Raw(e.Name)
}

type IntExpr struct {
	Value int
}

func Int(value int) Expr {
	return IntExpr{Value: value}
}

func (IntExpr) expr() {}
func (e IntExpr) write(w *Writer) {
	fmt.Fprintf(&w.sb, "%d", e.Value)
}

type StringExpr struct {
	Value string
}

func String(value string) Expr {
	return StringExpr{Value: value}
}

func (StringExpr) expr() {}
func (e StringExpr) write(w *Writer) {
	fmt.Fprintf(&w.sb, "%q", e.Value)
}

type NullExpr struct{}

func Null() Expr {
	return NullExpr{}
}

func (NullExpr) expr() {}
func (NullExpr) write(w *Writer) {
	w.Raw("NULL")
}

type CallExpr struct {
	Name string
	Args []Expr
}

func Call(name string, args ...Expr) Expr {
	return CallExpr{
		Name: name,
		Args: args,
	}
}

func (CallExpr) expr() {}

func (e CallExpr) write(w *Writer) {
	w.Raw(e.Name)
	w.Raw("(")

	for i, arg := range e.Args {
		if i > 0 {
			w.Raw(", ")
		}

		arg.write(w)
	}

	w.Raw(")")
}

type BinaryExpr struct {
	Left  Expr
	Op    string
	Right Expr
}

func Binary(left Expr, op string, right Expr) Expr {
	return BinaryExpr{
		Left:  left,
		Op:    op,
		Right: right,
	}
}

func (BinaryExpr) expr() {}

func (e BinaryExpr) write(w *Writer) {
	w.Raw("(")
	e.Left.write(w)
	w.Raw(" ")
	w.Raw(e.Op)
	w.Raw(" ")
	e.Right.write(w)
	w.Raw(")")
}

func Eq(left, right Expr) Expr {
	return Binary(left, "==", right)
}

func Not(left, right Expr) Expr {
	return Binary(left, "!=", right)
}

func And(left, right Expr) Expr {
	return Binary(left, "&&", right)
}

func Or(left, right Expr) Expr {
	return Binary(left, "||", right)
}

type FieldExpr struct {
	Object Expr
	Field  string
}

func Field(object Expr, field string) Expr {
	return FieldExpr{
		Object: object,
		Field:  field,
	}
}

func (FieldExpr) expr() {}

func (e FieldExpr) write(w *Writer) {
	e.Object.write(w)
	w.Raw(".")
	w.Raw(e.Field)
}

type PointerFieldExpr struct {
	Object Expr
	Field  string
}

func PointerField(object Expr, field string) Expr {
	return PointerFieldExpr{
		Object: object,
		Field:  field,
	}
}

func (PointerFieldExpr) expr() {}

func (e PointerFieldExpr) write(w *Writer) {
	e.Object.write(w)
	w.Raw("->")
	w.Raw(e.Field)
}

type CastExpr struct {
	Type  Expr
	Value Expr
}

func Cast(typ Expr, value Expr) Expr {
	return CastExpr{
		Type:  typ,
		Value: value,
	}
}

func (CastExpr) expr() {}

func (e CastExpr) write(w *Writer) {
	w.Raw("(")
	e.Type.write(w)
	w.Raw(")")
	e.Value.write(w)
}

type AddressExpr struct {
	Value Expr
}

func Address(value Expr) Expr {
	return AddressExpr{Value: value}
}

func (AddressExpr) expr() {}

func (e AddressExpr) write(w *Writer) {
	w.Raw("&")
	e.Value.write(w)
}

type DerefExpr struct {
	Value Expr
}

func Deref(value Expr) Expr {
	return DerefExpr{Value: value}
}

func (DerefExpr) expr() {}

func (e DerefExpr) write(w *Writer) {
	w.Raw("*")
	e.Value.write(w)
}
