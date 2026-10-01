package semantic

import (
	"github.com/IlyaFeoktistov/Rech/internal/ast"
	"github.com/IlyaFeoktistov/Rech/internal/diagnostics"
	"github.com/IlyaFeoktistov/Rech/internal/ir"
)

// Переменная, объявленная без значения ("Целое х", "переменная х: Целое"),
// читается только после присваивания на КАЖДОМ пути до точки чтения -
// иначе ошибка компиляции. "Присвоена" - доказанный факт того же
// потокового механизма established, что и состояния typestate: после
// "если/иначе" он остаётся, только если доказан в обеих ветках (ветка,
// которая выходит "вернуть"/"выбросить", не в счёт), после цикла - только
// если доказан до цикла (тело может не выполниться ни разу).

// assignedState - факт "присвоена" в наборе established переменной (не
// может совпасть с именем состояния: такого идентификатора в языке нет).
const assignedState = "\x00присвоена"

// markDeclaredBare - name только что объявлена (declare) без значения в
// самой внутренней области видимости.
func (c *ctx) markDeclaredBare(name string) {
	c.declaredBare[len(c.declaredBare)-1][name] = true
}

// withAssigned - states плюс факт "присвоена" у name, если name объявлена
// без значения; иначе states как есть.
func (c *ctx) withAssigned(name string, states map[string]bool) map[string]bool {
	if !c.isDeclaredBare(name) {
		return states
	}
	out := cloneStates(states)
	out[assignedState] = true
	return out
}

// isDeclaredBare - name (в той области видимости, куда оно резолвится)
// объявлена без значения.
func (c *ctx) isDeclaredBare(name string) bool {
	for i := len(c.scopes) - 1; i >= 0; i-- {
		if _, ok := c.scopes[i][name]; ok {
			return c.declaredBare[i][name]
		}
	}
	return false
}

// analyzeBareVarDecl - "Тип имя"/"переменная имя: Тип" без значения (тип
// declared уже разрешён): переменная объявлена, но не присвоена.
func analyzeBareVarDecl(sc *ctx, s *ast.VarDecl, declared ir.Type) (ir.Stmt, error) {
	if err := sc.declare(s.Name, declared); err != nil {
		return nil, diagnostics.WithPos(err, diagnostics.Position{File: currentFile, Line: s.Line})
	}
	sc.markDeclaredBare(s.Name)
	return &ir.VarDecl{Name: s.Name, DeclType: declared, Line: s.Line, Doc: s.Doc}, nil
}

// errReadBeforeAssign - чтение name до присваивания (readBeforeAssign).
func errReadBeforeAssign(name string, line int) error {
	return errAtLine(line, CodeStmtCheck, "переменная %q читается до присваивания - присвойте ей значение на каждом пути до этого места", name)
}

// readBeforeAssign - name объявлена без значения и в этой точке не
// доказано, что она присвоена. В теле лямбды имя внешней функции
// проверяется в её ctx: лямбда видит то, что присвоено до неё.
func (c *ctx) readBeforeAssign(name string) bool {
	for i := len(c.scopes) - 1; i >= 0; i-- {
		if _, ok := c.scopes[i][name]; ok {
			return c.declaredBare[i][name] && !c.established[i][name][assignedState]
		}
	}
	if c.parent != nil {
		return c.parent.readBeforeAssign(name)
	}
	return false
}
