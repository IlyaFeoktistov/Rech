package lsp

import (
	"sort"
	"strings"

	"github.com/IlyaFeoktistov/Rech/internal/builtins"
	"github.com/IlyaFeoktistov/Rech/internal/ir"
	"github.com/IlyaFeoktistov/Rech/internal/rtmsg"
)

// Что бросает функция/метод/конструктор - для строки "Исключения: ..."/
// "Паники: ..." в hover. Считается по IR документа: "выбросить" в теле,
// вызовы своих функций/методов/конструкторов (транзитивно, до неподвижной
// точки - рекурсия не зацикливает), встроенные функции и методы коллекций
// (builtins.FuncThrows/CollectionMethodThrows), "разобрать", каст строки,
// "как". Пойманное в "попытаться" вычитается с учётом наследования. Тела
// лямбд и задач ("запустить") не считаются - они исполняются не в момент
// вызова. Операторы ("/", "[i]") не считаются - их паники есть почти в
// любом коде и только зашумили бы подсказку.

type throwSet map[string]bool

// throwsIndex - множества бросаемых типов по ключу вызываемого
// (throwsFuncKey/throwsMethodKey/throwsCtorKey).
type throwsIndex struct {
	entities map[string]*ir.Entity
	funcs    map[string]*ir.Function
	sets     map[string]throwSet
}

func throwsFuncKey(name string) string             { return "ф:" + name }
func throwsMethodKey(entity, method string) string { return "м:" + entity + "." + method }
func throwsCtorKey(entity string) string           { return "к:" + entity }

// throwsIndex документа - строится при первом обращении и заново после
// каждого нового IR.
func (d *doc) throwsIdx() *throwsIndex {
	if d.throws == nil || d.throwsProg != d.irProg {
		d.throws, d.throwsProg = buildThrowsIndex(d.irProg), d.irProg
	}
	return d.throws
}

// throwsOf - строки hover "Исключения/Паники" для ключа key.
func (d *doc) throwsOf(key string) string {
	idx := d.throwsIdx()
	if idx == nil {
		return ""
	}
	return idx.describe(idx.sets[key])
}

// buildThrowsIndex - см. package doc выше. nil у документа без IR.
func buildThrowsIndex(prog *ir.Program) *throwsIndex {
	if prog == nil {
		return nil
	}
	idx := &throwsIndex{
		entities: map[string]*ir.Entity{},
		funcs:    map[string]*ir.Function{},
		sets:     map[string]throwSet{},
	}
	bodies := map[string][]ir.Stmt{}
	for _, fn := range prog.Functions {
		idx.funcs[fn.Name] = fn
		bodies[throwsFuncKey(fn.Name)] = fn.Body
	}
	for _, ent := range prog.Entities {
		idx.entities[ent.Name] = ent
		for _, m := range ent.Methods {
			bodies[throwsMethodKey(ent.Name, m.Name)] = m.Body
		}
		if ent.Ctor != nil {
			bodies[throwsCtorKey(ent.Name)] = ent.Ctor.Body
		}
	}
	for changed := true; changed; {
		changed = false
		for key, body := range bodies {
			got := idx.block(body)
			set := idx.sets[key]
			if set == nil {
				set = throwSet{}
				idx.sets[key] = set
			}
			for t := range got {
				if !set[t] {
					set[t] = true
					changed = true
				}
			}
		}
	}
	return idx
}

func (idx *throwsIndex) block(stmts []ir.Stmt) throwSet {
	out := throwSet{}
	for _, s := range stmts {
		idx.stmt(s, out)
	}
	return out
}

func (idx *throwsIndex) stmt(s ir.Stmt, out throwSet) {
	switch s := s.(type) {
	case *ir.TryStmt:
		for t := range idx.block(s.Body) {
			if s.NoCatch || !idx.caughtBy(t, s.CatchTypes) {
				out[t] = true
			}
		}
		addAll(out, idx.block(s.CatchBody))
		addAll(out, idx.block(s.Finally))
		return
	case *ir.ThrowStmt:
		if t := ir.TypeOf(s.Value); t.Kind == ir.KindEntity {
			out[t.Entity] = true
		}
	case *ir.ParentCallStmt:
		addAll(out, idx.sets[throwsCtorKey(s.BaseEntity)])
	}
	for _, e := range ir.ChildExprs(s) {
		idx.expr(e, out)
	}
	for _, b := range ir.ChildBlocks(s) {
		addAll(out, idx.block(b))
	}
}

func (idx *throwsIndex) expr(e ir.Expr, out throwSet) {
	switch e := e.(type) {
	case *ir.LambdaExpr:
		return
	case *ir.CallExpr:
		if _, ok := idx.funcs[e.Callee]; ok {
			addAll(out, idx.sets[throwsFuncKey(e.Callee)])
		} else {
			addNames(out, builtins.FuncThrows[e.Callee])
		}
	case *ir.MethodCallExpr:
		if base, _, generic := strings.Cut(e.Entity, "<"); generic {
			addNames(out, builtins.CollectionMethodThrows[base][e.Method])
		} else if owner, ok := idx.methodOwner(e.Entity, e.Method); ok {
			addAll(out, idx.sets[throwsMethodKey(owner, e.Method)])
		}
	case *ir.StaticMethodCallExpr:
		addAll(out, idx.sets[throwsMethodKey(e.Entity, e.Method)])
	case *ir.NewExpr:
		addAll(out, idx.sets[throwsCtorKey(e.Entity)])
	case *ir.ParseExpr:
		out[rtmsg.ErrorParse] = true
	case *ir.CastExpr:
		if ir.TypeOf(e.Value) == ir.TypeString && e.To != ir.TypeString {
			out[rtmsg.ErrorParse] = true
		}
	case *ir.AsExpr:
		out[rtmsg.PanicBadCast] = true
	}
	for _, c := range ir.ChildExprs(e) {
		idx.expr(c, out)
	}
}

// methodOwner - сущность, в которой объявлен метод name, видимый у entity
// (сама entity или ближайший предок).
func (idx *throwsIndex) methodOwner(entity, name string) (string, bool) {
	for ent := idx.entities[entity]; ent != nil; ent = idx.entities[ent.BaseName] {
		for _, m := range ent.Methods {
			if m.Name == name {
				return ent.Name, true
			}
		}
	}
	return "", false
}

// isA - t сама target или её наследник.
func (idx *throwsIndex) isA(t, target string) bool {
	for name := t; name != ""; {
		if name == target {
			return true
		}
		ent := idx.entities[name]
		if ent == nil {
			return false
		}
		name = ent.BaseName
	}
	return false
}

func (idx *throwsIndex) caughtBy(t string, catchTypes []string) bool {
	for _, c := range catchTypes {
		if idx.isA(t, c) {
			return true
		}
	}
	return false
}

// describe - строки "Исключения: ..."/"Паники: ..." для set, "" - пусто.
func (idx *throwsIndex) describe(set throwSet) string {
	var excs, panics []string
	for t := range set {
		if idx.isA(t, rtmsg.PanicEntity) {
			panics = append(panics, "`"+t+"`")
		} else {
			excs = append(excs, "`"+t+"`")
		}
	}
	return describeThrows(excs, panics)
}

// describeBuiltinThrows - то же для встроенной функции name - таблица
// builtins.FuncThrows, без IR документа.
func describeBuiltinThrows(name string) string {
	return describeNames(builtins.FuncThrows[name])
}

// describeNames - строки hover для готового списка встроенных типов.
func describeNames(names []string) string {
	var excs, panics []string
	for _, t := range names {
		if isBuiltinPanic(t) {
			panics = append(panics, "`"+t+"`")
		} else {
			excs = append(excs, "`"+t+"`")
		}
	}
	return describeThrows(excs, panics)
}

func isBuiltinPanic(name string) bool {
	for _, p := range rtmsg.PanicKinds {
		if p == name {
			return true
		}
	}
	return false
}

func describeThrows(excs, panics []string) string {
	sort.Strings(excs)
	sort.Strings(panics)
	var lines []string
	if len(excs) > 0 {
		lines = append(lines, "Исключения: "+strings.Join(excs, ", "))
	}
	if len(panics) > 0 {
		lines = append(lines, "Паники: "+strings.Join(panics, ", "))
	}
	return strings.Join(lines, "\n\n")
}

// withThrows добавляет строки throws под блоком hover (пусто - без изменений).
func withThrows(block, throws string) string {
	if throws == "" {
		return block
	}
	return block + "\n\n" + throws
}

func addAll(dst, src throwSet) {
	for t := range src {
		dst[t] = true
	}
}

func addNames(dst throwSet, names []string) {
	for _, t := range names {
		dst[t] = true
	}
}
