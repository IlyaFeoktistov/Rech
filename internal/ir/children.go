package ir

import "reflect"

var (
	exprIface = reflect.TypeFor[Expr]()
	stmtIface = reflect.TypeFor[Stmt]()
	typeType  = reflect.TypeFor[Type]()
)

// ChildExprs - непосредственные дочерние выражения узла n (Expr или Stmt) в
// порядке объявления полей: поля типа Expr/[]Expr и Expr-поля вложенных
// структур без собственного узла (CaptureVar.Init и т. п.). Во вложенные
// инструкции (тела если/циклов/лямбд, []Stmt, поле-Stmt) не спускается -
// их обходит вызывающая сторона сама, если они ей нужны. Построен на
// reflect, чтобы новый вид узла IR не требовал правки обходчика.
func ChildExprs(n any) []Expr {
	var out []Expr
	v := reflect.ValueOf(n)
	if v.Kind() == reflect.Pointer {
		if v.IsNil() {
			return nil
		}
		v = v.Elem()
	}
	if v.Kind() == reflect.Struct {
		collectStructExprs(v, &out)
	}
	return out
}

func collectStructExprs(v reflect.Value, out *[]Expr) {
	for i := 0; i < v.NumField(); i++ {
		collectValueExprs(v.Field(i), out)
	}
}

func collectValueExprs(f reflect.Value, out *[]Expr) {
	t := f.Type()
	switch {
	case t == typeType:
		return
	case t.Implements(stmtIface) && !t.Implements(exprIface):
		return
	case t.Kind() == reflect.Interface && t == exprIface:
		if !f.IsNil() {
			*out = append(*out, f.Interface().(Expr))
		}
	case t.Kind() == reflect.Pointer && t.Implements(exprIface):
		if !f.IsNil() {
			*out = append(*out, f.Interface().(Expr))
		}
	case t.Kind() == reflect.Slice:
		if t.Elem() == stmtIface {
			return
		}
		for j := 0; j < f.Len(); j++ {
			collectValueExprs(f.Index(j), out)
		}
	case t.Kind() == reflect.Struct:
		collectStructExprs(f, out)
	case t.Kind() == reflect.Pointer && t.Elem().Kind() == reflect.Struct && !t.Implements(stmtIface):
		if !f.IsNil() {
			collectStructExprs(f.Elem(), out)
		}
	}
}
