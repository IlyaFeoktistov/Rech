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

// ChildBlocks - непосредственно вложенные блоки инструкции s (тела
// если/иначе, циклов, ветвей выбрать, попытаться/поймать/окончательно) -
// поля []Stmt и Stmt, в том числе внутри вложенных структур без
// собственного узла (ветви "выбрать"). Одиночное поле-Stmt возвращается
// блоком из одной инструкции. В тела лямбд (выражения) не спускается.
func ChildBlocks(s Stmt) [][]Stmt {
	var out [][]Stmt
	v := reflect.ValueOf(s)
	if v.Kind() == reflect.Pointer {
		if v.IsNil() {
			return nil
		}
		v = v.Elem()
	}
	if v.Kind() == reflect.Struct {
		collectStructBlocks(v, &out)
	}
	return out
}

func collectStructBlocks(v reflect.Value, out *[][]Stmt) {
	for i := 0; i < v.NumField(); i++ {
		collectValueBlocks(v.Field(i), out)
	}
}

func collectValueBlocks(f reflect.Value, out *[][]Stmt) {
	t := f.Type()
	switch {
	case t == typeType:
		return
	case t.Implements(exprIface):
		return
	case t == stmtIface || (t.Kind() == reflect.Pointer && t.Implements(stmtIface)):
		if !f.IsNil() {
			*out = append(*out, []Stmt{f.Interface().(Stmt)})
		}
	case t.Kind() == reflect.Slice && t.Elem() == stmtIface:
		*out = append(*out, f.Interface().([]Stmt))
	case t.Kind() == reflect.Slice && t.Elem().Kind() != reflect.Interface:
		for j := 0; j < f.Len(); j++ {
			collectValueBlocks(f.Index(j), out)
		}
	case t.Kind() == reflect.Struct:
		collectStructBlocks(f, out)
	case t.Kind() == reflect.Pointer && t.Elem().Kind() == reflect.Struct && !t.Implements(stmtIface):
		if !f.IsNil() {
			collectStructBlocks(f.Elem(), out)
		}
	}
}
