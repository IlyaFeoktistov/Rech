package semantic

import (
	"fmt"
	"sort"
	"strings"

	"github.com/IlyaFeoktistov/Rech/internal/diagnostics"
	"github.com/IlyaFeoktistov/Rech/internal/ir"
	"github.com/IlyaFeoktistov/Rech/internal/ir/cfg"
)

// Проверка адресов элементов списка ("&список[i]"). Буфер Список<T> растёт
// перевыделением (GC_REALLOC в cbackend), поэтому адрес элемента теряет силу
// после "добавить" в тот же список. Правила:
//
//   - адрес элемента списка ("элементный указатель") живёт только в
//     локальной переменной, при разыменовании, в сравнении, при выводе и
//     аргументом обычного вызова; вернуть его, сохранить в поле/коллекции/
//     статической переменной, передать в конструктор или "запустить",
//     захватить лямбдой - ошибка компиляции;
//   - использование элементного указателя на пути, где после взятия адреса
//     список мог вырасти, - ошибка компиляции (поток данных по CFG тела,
//     internal/ir/cfg).
//
// Список может вырасти: "добавить" у самого списка; у "общего" списка
// (параметр, захват лямбды, переменная не из "новый Список"/литерала, или
// сам список уже передан куда-то дальше) - ещё и любой вызов
// пользовательского кода и "добавить" у любого другого списка того же типа
// (он может оказаться тем же объектом под другим именем).

// ptrStale - почему элементный указатель мог потерять силу.
type ptrStale struct {
	line   int
	reason string
}

// ptrState - факты в точке программы: borrows - на элементы каких списков
// может указывать переменная-указатель, stale - какие из них могли потерять
// силу, shared - какие списки общие (см. package doc выше).
type ptrState struct {
	borrows map[string]map[string]bool
	stale   map[string]ptrStale
	shared  map[string]bool
}

func newPtrState() *ptrState {
	return &ptrState{borrows: map[string]map[string]bool{}, stale: map[string]ptrStale{}, shared: map[string]bool{}}
}

func (s *ptrState) clone() *ptrState {
	c := newPtrState()
	for p, ls := range s.borrows {
		m := make(map[string]bool, len(ls))
		for l := range ls {
			m[l] = true
		}
		c.borrows[p] = m
	}
	for p, st := range s.stale {
		c.stale[p] = st
	}
	for l := range s.shared {
		c.shared[l] = true
	}
	return c
}

// merge объединяет other в s ("может быть" на любом из входящих путей),
// true - если s изменилось.
func (s *ptrState) merge(other *ptrState) bool {
	changed := false
	for p, ls := range other.borrows {
		m, ok := s.borrows[p]
		if !ok {
			m = map[string]bool{}
			s.borrows[p] = m
		}
		for l := range ls {
			if !m[l] {
				m[l] = true
				changed = true
			}
		}
	}
	for p, st := range other.stale {
		if cur, ok := s.stale[p]; !ok || st.line < cur.line {
			s.stale[p] = st
			changed = true
		}
	}
	for l := range other.shared {
		if !s.shared[l] {
			s.shared[l] = true
			changed = true
		}
	}
	return changed
}

// listElemChecker - один проход передаточной функции по инструкциям блока;
// report == false во время поиска неподвижной точки (ошибки собираются
// только финальным проходом по уже устоявшимся входным состояниям).
type listElemChecker struct {
	st *ptrState
	// lists - тип каждого списка, у элемента которого брали адрес
	// ("Список<Целое>"); owned - см. ownedLists.
	lists  map[string]string
	owned  map[string]bool
	report bool
	line   int
	errs   []error
}

func (c *listElemChecker) fail(format string, args ...any) {
	if c.report {
		c.errs = append(c.errs, errAtLine(c.line, CodePointer, format, args...))
	}
}

// checkListElementPointers - точка входа (вызывается из analyzeFlow для
// каждого тела функции/метода/лямбды).
func checkListElementPointers(g *cfg.Graph, body []ir.Stmt) error {
	if !usesListElementAddress(body) {
		return nil
	}
	owned := ownedLists(body)

	in := make([]*ptrState, len(g.Blocks))
	in[g.Entry] = newPtrState()
	lists := map[string]string{}
	transfer := func(id int, st *ptrState, report bool) (*ptrState, []error) {
		c := &listElemChecker{st: st, lists: lists, owned: owned, report: report}
		c.block(g.Blocks[id])
		return c.st, c.errs
	}

	queue := []int{g.Entry}
	queued := map[int]bool{g.Entry: true}
	for len(queue) > 0 {
		id := queue[0]
		queue = queue[1:]
		queued[id] = false
		out, _ := transfer(id, in[id].clone(), false)
		for _, succ := range g.Blocks[id].Succs {
			if in[succ] == nil {
				in[succ] = out.clone()
			} else if !in[succ].merge(out) {
				continue
			}
			if !queued[succ] {
				queued[succ] = true
				queue = append(queue, succ)
			}
		}
	}

	var errs []error
	for id := range g.Blocks {
		if in[id] == nil {
			continue
		}
		_, e := transfer(id, in[id].clone(), true)
		errs = append(errs, e...)
	}
	if len(errs) == 0 {
		return nil
	}
	sort.SliceStable(errs, func(i, j int) bool { return errLine(errs[i]) < errLine(errs[j]) })
	return errs[0]
}

func errLine(err error) int {
	if d, ok := err.(*diagnostics.Diagnostic); ok {
		return d.Pos.Line
	}
	return 0
}

// usesListElementAddress - есть ли в теле вообще "&список[i]" (дешёвый
// фильтр: подавляющее большинство тел без него).
func usesListElementAddress(body []ir.Stmt) bool {
	found := false
	var visitExpr func(e ir.Expr)
	visitExpr = func(e ir.Expr) {
		if found || e == nil {
			return
		}
		if a, ok := e.(*ir.AddressOfExpr); ok {
			if _, ok := a.Receiver.(*ir.ListElementRef); ok {
				found = true
				return
			}
		}
		for _, ch := range ir.ChildExprs(e) {
			visitExpr(ch)
		}
	}
	var visitStmts func(ss []ir.Stmt)
	visitStmts = func(ss []ir.Stmt) {
		for _, s := range ss {
			for _, e := range ir.ChildExprs(s) {
				visitExpr(e)
			}
			if f, ok := s.(*ir.CStyleForStmt); ok {
				visitStmts([]ir.Stmt{f.Init, f.Post})
			}
			for _, b := range childBodies(s) {
				visitStmts(b)
			}
		}
	}
	visitStmts(body)
	return found
}

// childBodies - вложенные тела инструкции (тот же набор, что строит cfg).
func childBodies(s ir.Stmt) [][]ir.Stmt {
	switch s := s.(type) {
	case *ir.IfStmt:
		return [][]ir.Stmt{s.Then, s.Else}
	case *ir.WhileStmt:
		return [][]ir.Stmt{s.Body}
	case *ir.ForStmt:
		return [][]ir.Stmt{s.Body}
	case *ir.ForEachStmt:
		return [][]ir.Stmt{s.Body}
	case *ir.CStyleForStmt:
		return [][]ir.Stmt{s.Body}
	case *ir.TryStmt:
		return [][]ir.Stmt{s.Body, s.CatchBody, s.Finally}
	case *ir.GerundStmt:
		return [][]ir.Stmt{s.Body}
	case *ir.BlockStmt:
		return [][]ir.Stmt{s.Body}
	}
	return nil
}

// ownedLists - переменные-списки, КАЖДОЕ объявление которых в теле -
// "новый Список<T>()" или литерал "[...]": только у таких списков нет
// других имён. Остальные (параметры, захваты, списки из полей/вызовов) -
// общие с самого начала.
func ownedLists(body []ir.Stmt) map[string]bool {
	owned := map[string]bool{}
	notOwned := map[string]bool{}
	var visit func(ss []ir.Stmt)
	visit = func(ss []ir.Stmt) {
		for _, s := range ss {
			if d, ok := s.(*ir.VarDecl); ok && isListType(ir.TypeOf(d.Value)) {
				switch d.Value.(type) {
				case *ir.NewExpr, *ir.ListLiteralExpr:
					if d.IsStatic {
						notOwned[d.Name] = true
					} else {
						owned[d.Name] = true
					}
				default:
					notOwned[d.Name] = true
				}
			}
			if f, ok := s.(*ir.CStyleForStmt); ok && f.Init != nil {
				visit([]ir.Stmt{f.Init})
			}
			for _, b := range childBodies(s) {
				visit(b)
			}
		}
	}
	visit(body)
	for n := range notOwned {
		delete(owned, n)
	}
	return owned
}

func isListType(t ir.Type) bool {
	return t.Kind == ir.KindEntity && strings.HasPrefix(t.Entity, "Список<")
}

// isBuiltinCollection - получатель метода - встроенная коллекция (её методы
// не исполняют пользовательский код и не сохраняют аргументы где-то, кроме
// самой коллекции).
func isBuiltinCollection(entity string) bool {
	for _, p := range []string{"Список<", "Словарь<", "Стопка<", "Очередь<"} {
		if strings.HasPrefix(entity, p) {
			return true
		}
	}
	return false
}

func (c *listElemChecker) block(b *cfg.Block) {
	if b.Loop != nil {
		c.loopHeader(b.Loop)
	}
	for _, s := range b.Stmts {
		if l := ir.LineOf(s); l != 0 {
			c.line = l
		}
		c.stmt(s)
	}
}

// loopHeader - то, что цикл делает на каждой итерации в своём заголовке.
func (c *listElemChecker) loopHeader(s ir.Stmt) {
	if l := ir.LineOf(s); l != 0 {
		c.line = l
	}
	switch s := s.(type) {
	case *ir.WhileStmt:
		c.expr(s.Cond, "")
	case *ir.CStyleForStmt:
		if s.Cond != nil {
			c.expr(s.Cond, "")
		}
	case *ir.ForEachStmt:
		c.kill(s.Var)
		if s.KeyVar != "" {
			c.kill(s.KeyVar)
		}
	case *ir.ForStmt:
		c.kill(s.Var)
	}
}

// Контексты, в которых элементный указатель запрещён (текст - продолжение
// фразы "адрес элемента списка нельзя ...").
const (
	ctxHere      = "использовать в этом выражении"
	ctxReturn    = "вернуть"
	ctxField     = "сохранить в поле"
	ctxStatic    = "сохранить в статической переменной"
	ctxNew       = "передать в конструктор"
	ctxLaunch    = "передать в \"запустить\"/\"поручить\""
	ctxLambda    = "захватить лямбдой"
	ctxContainer = "сохранить в коллекции"
)

func (c *listElemChecker) stmt(s ir.Stmt) {
	switch s := s.(type) {
	case *ir.IfStmt:
		c.expr(s.Cond, ctxHere)
	case *ir.WhileStmt, *ir.ForStmt, *ir.CStyleForStmt, *ir.TryStmt:
		// Условие/границы - в заголовке (loopHeader), Init/Post - отдельные
		// инструкции блоков (cfg.emitCStyleFor).
		if f, ok := s.(*ir.ForStmt); ok {
			for _, e := range ir.ChildExprs(f) {
				c.expr(e, ctxHere)
			}
		}
	case *ir.ForEachStmt:
		c.listUse(s.Collection)
	case *ir.VarDecl:
		ctx := ""
		if s.IsStatic {
			ctx = ctxStatic
		}
		c.assign(s.Name, s.Value, ctx)
	case *ir.Assign:
		c.assign(s.Name, s.Value, "")
	case *ir.DerefAssignStmt:
		c.expr(s.Pointer, "")
		c.expr(s.Value, ctxHere)
	case *ir.PrintStmt:
		c.printable(s.Value)
	case *ir.ShowStmt:
		c.printable(s.Value)
	case *ir.ReturnStmt:
		if s.Value != nil {
			c.expr(s.Value, ctxReturn)
		}
	case *ir.FieldAssign:
		c.expr(s.Receiver, ctxHere)
		c.expr(s.Value, ctxField)
	case *ir.StaticFieldAssign:
		for _, e := range ir.ChildExprs(s) {
			c.expr(e, ctxField)
		}
	default:
		for _, e := range ir.ChildExprs(s) {
			c.expr(e, ctxHere)
		}
	}
}

func (c *listElemChecker) printable(e ir.Expr) {
	if id, ok := e.(*ir.Identifier); ok && isListType(id.Type) {
		return
	}
	c.expr(e, "")
}

// assign - "имя = значение": запоминает, на элементы каких списков теперь
// указывает имя (пусто - ни на какие).
func (c *listElemChecker) assign(name string, value ir.Expr, ctx string) {
	c.expr(value, ctx)
	borrows := c.borrowsOf(value)
	c.kill(name)
	if len(borrows) > 0 && ctx == "" {
		c.st.borrows[name] = borrows
	}
}

func (c *listElemChecker) isShared(list string) bool {
	return !c.owned[list] || c.st.shared[list]
}

func (c *listElemChecker) kill(name string) {
	delete(c.st.borrows, name)
	delete(c.st.stale, name)
}

// borrowsOf - на элементы каких списков может указывать значение e.
func (c *listElemChecker) borrowsOf(e ir.Expr) map[string]bool {
	switch e := e.(type) {
	case *ir.AddressOfExpr:
		if ref, ok := e.Receiver.(*ir.ListElementRef); ok {
			if id, ok := ref.Receiver.(*ir.Identifier); ok {
				c.lists[id.Name] = ref.CollectionName
				return map[string]bool{id.Name: true}
			}
		}
	case *ir.Identifier:
		if ls, ok := c.st.borrows[e.Name]; ok {
			out := make(map[string]bool, len(ls))
			for l := range ls {
				out[l] = true
			}
			return out
		}
	case *ir.PointerOffsetExpr:
		return c.borrowsOf(e.Pointer)
	case *ir.TernaryExpr:
		out := c.borrowsOf(e.Then)
		for l := range c.borrowsOf(e.Else) {
			if out == nil {
				out = map[string]bool{}
			}
			out[l] = true
		}
		return out
	}
	return nil
}

func (c *listElemChecker) isElemPointer(e ir.Expr) bool {
	return len(c.borrowsOf(e)) > 0
}

// expr обходит выражение; ctx != "" - элементный указатель в этой позиции
// запрещён (текст причины).
func (c *listElemChecker) expr(e ir.Expr, ctx string) {
	if e == nil {
		return
	}
	if ctx != "" && c.isElemPointer(e) {
		c.fail("адрес элемента списка нельзя %s: он теряет силу, когда список растёт (\"добавить\"); держите его в локальной переменной и берите заново после изменения списка", ctx)
	}
	switch e := e.(type) {
	case *ir.Identifier:
		c.use(e)
		if isListType(e.Type) {
			c.escapeList(e.Name)
		}
	case *ir.AddressOfExpr:
		if ref, ok := e.Receiver.(*ir.ListElementRef); ok {
			c.expr(ref.Index, ctxHere)
			return
		}
		for _, ch := range ir.ChildExprs(e) {
			c.expr(ch, ctxHere)
		}
	case *ir.DerefExpr:
		c.expr(e.Value, "")
	case *ir.PointerOffsetExpr:
		c.expr(e.Pointer, "")
		c.expr(e.Index, ctxHere)
	case *ir.TernaryExpr:
		c.expr(e.Cond, ctxHere)
		c.expr(e.Then, ctx)
		c.expr(e.Else, ctx)
	case *ir.BinaryExpr:
		if e.Op == "==" || e.Op == "!=" {
			c.compared(e.Left)
			c.compared(e.Right)
			return
		}
		c.expr(e.Left, ctxHere)
		c.expr(e.Right, ctxHere)
	case *ir.NullCheckExpr:
		for _, ch := range ir.ChildExprs(e) {
			c.compared(ch)
		}
	case *ir.CastExpr:
		if e.To == ir.TypeString {
			c.printable(e.Value)
			return
		}
		c.expr(e.Value, ctxHere)
	case *ir.ListIndexExpr:
		c.listUse(e.Receiver)
		c.expr(e.Index, ctxHere)
	case *ir.MethodCallExpr:
		c.methodCall(e)
	case *ir.CallExpr:
		c.args(e.Args, "")
		c.userCall()
	case *ir.CallValueExpr, *ir.StaticMethodCallExpr, *ir.AwaitExpr:
		c.args(ir.ChildExprs(e), "")
		c.userCall()
	case *ir.NewExpr:
		c.args(e.Args, ctxNew)
		if !isBuiltinCollection(e.Entity) {
			c.userCall()
		}
	case *ir.LaunchExpr:
		c.args(ir.ChildExprs(e), ctxLaunch)
		c.userCall()
	case *ir.ListLiteralExpr:
		c.args(ir.ChildExprs(e), ctxContainer)
	case *ir.LambdaExpr:
		c.lambda(e)
	default:
		for _, ch := range ir.ChildExprs(e) {
			c.expr(ch, ctxHere)
		}
	}
}

// compared - операнд "=="/"!=" (сравнение адресов/тождества, ничего не
// сохраняет).
func (c *listElemChecker) compared(e ir.Expr) {
	if id, ok := e.(*ir.Identifier); ok {
		c.use(id)
		return
	}
	c.expr(e, "")
}

// listUse - список в позиции, которая его не сохраняет и не меняет
// (получатель индекса, коллекция "для ... в").
func (c *listElemChecker) listUse(e ir.Expr) {
	if id, ok := e.(*ir.Identifier); ok && isListType(id.Type) {
		return
	}
	c.expr(e, ctxHere)
}

func (c *listElemChecker) args(args []ir.Expr, ctx string) {
	for _, a := range args {
		c.expr(a, ctx)
	}
}

// use - чтение переменной: элементный указатель, который мог потерять
// силу, - ошибка.
func (c *listElemChecker) use(id *ir.Identifier) {
	st, ok := c.st.stale[id.Name]
	if !ok {
		return
	}
	lists := make([]string, 0, len(c.st.borrows[id.Name]))
	for l := range c.st.borrows[id.Name] {
		lists = append(lists, l)
	}
	sort.Strings(lists)
	c.fail("указатель %q на элемент списка %q мог потерять силу: на строке %d %s; возьмите адрес заново после изменения списка", id.Name, strings.Join(lists, "\", \""), st.line, st.reason)
	// Одна ошибка на одно устаревание, а не на каждое следующее чтение.
	delete(c.st.stale, id.Name)
}

func (c *listElemChecker) methodCall(e *ir.MethodCallExpr) {
	recvID, recvIsIdent := e.Receiver.(*ir.Identifier)
	if recvIsIdent {
		c.use(recvID)
	} else {
		c.expr(e.Receiver, ctxHere)
	}

	if !isBuiltinCollection(e.Entity) {
		c.args(e.Args, "")
		c.userCall()
		return
	}
	c.args(e.Args, ctxContainer)
	if e.Method != "добавить" || !strings.HasPrefix(e.Entity, "Список<") {
		return
	}
	if recvIsIdent {
		c.invalidate(recvID.Name, fmt.Sprintf("в список %q добавлен элемент", recvID.Name))
	}
	// Другой список того же типа может быть тем же объектом, что и любой
	// общий список.
	for l, typ := range c.lists {
		if (!recvIsIdent || l != recvID.Name) && typ == e.Entity && c.isShared(l) {
			c.invalidate(l, fmt.Sprintf("добавлен элемент в список того же типа %s, который может быть тем же объектом, что и %q", e.Entity, l))
		}
	}
}

// userCall - вызов пользовательского кода: может нарастить любой общий
// список.
func (c *listElemChecker) userCall() {
	for l := range c.lists {
		if c.isShared(l) {
			c.invalidate(l, fmt.Sprintf("вызов, который может изменить общий список %q", l))
		}
	}
}

// escapeList - список ушёл дальше (передан в вызов, присвоен, захвачен) -
// с этого момента он общий, и уже взятые адреса его элементов теряют силу.
func (c *listElemChecker) escapeList(name string) {
	c.st.shared[name] = true
	c.invalidate(name, fmt.Sprintf("список %q передан дальше и может быть изменён", name))
}

func (c *listElemChecker) invalidate(list, reason string) {
	for p, ls := range c.st.borrows {
		if !ls[list] {
			continue
		}
		if _, already := c.st.stale[p]; !already {
			c.st.stale[p] = ptrStale{line: c.line, reason: reason}
		}
	}
}

func (c *listElemChecker) lambda(e *ir.LambdaExpr) {
	for _, cv := range e.Captures {
		if cv.Init != nil {
			c.expr(cv.Init, ctxLambda)
			continue
		}
		if _, ok := c.st.borrows[cv.Name]; ok {
			c.fail("адрес элемента списка нельзя %s: он теряет силу, когда список растёт (\"добавить\"); держите его в локальной переменной и берите заново после изменения списка", ctxLambda)
		}
		if isListType(cv.Type) {
			c.escapeList(cv.Name)
		}
	}
}
