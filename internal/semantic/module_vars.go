package semantic

import (
	"github.com/IlyaFeoktistov/Rech/internal/ast"
	"github.com/IlyaFeoktistov/Rech/internal/diagnostics"
	"github.com/IlyaFeoktistov/Rech/internal/ir"
)

// analyzeModuleVars разбирает переменные уровня модуля (prog.Vars) по
// порядку объявления - тем же разбором, что и локальное объявление
// переменной (analyzeStmt), в контексте без локальных имён. Значение видит
// только переменные модуля, объявленные раньше (их значения к этому
// моменту уже вычислены); после разбора переменная видна всем функциям и
// методам (ctx.lookup). Ошибка одной переменной не останавливает разбор
// остальных - её имя всё равно регистрируется, если тип известен из
// аннотации, чтобы использования не дали лишних ошибок "не объявлена".
func (a *Analyzer) analyzeModuleVars(prog *ast.Program, bag *diagnostics.Bag) []*ir.GlobalVar {
	a.moduleVars = map[string]ir.Type{}
	a.laterModuleVars = map[string]bool{}
	for _, v := range prog.Vars {
		a.laterModuleVars[v.Name] = true
	}
	defer func() { a.laterModuleVars = nil }()
	var out []*ir.GlobalVar
	for _, v := range prog.Vars {
		delete(a.laterModuleVars, v.Name)
		currentFile = v.File
		if err := a.checkModuleVarName(v); err != nil {
			bag.AddErr(err)
			continue
		}
		sc := newCtx(a, a.funcs, a.entities, a.methods, a.aliases, a.interfaces, a.enums, "", ir.Type{}, false, a.resolvedConsts, a.resolvingConsts)
		sc.inferScope = v.Name
		stmt, err := analyzeStmt(sc, v)
		if err != nil {
			bag.AddErr(diagnostics.WrapAt(err, diagnostics.Position{File: currentFile, Line: v.Line}, "переменная модуля %q", v.Name))
			if v.Type != "" {
				if t, err := resolveTypeName(a.entities, a.methods, a.aliases, a.interfaces, a.enums, v.Type); err == nil {
					a.moduleVars[v.Name] = t
				}
			}
			continue
		}
		decl := stmt.(*ir.VarDecl)
		typ := ir.TypeOf(decl.Value)
		if decl.DeclType.Kind != ir.KindUnknown {
			typ = decl.DeclType
		}
		a.moduleVars[v.Name] = typ
		out = append(out, &ir.GlobalVar{Name: v.Name, Type: typ, Value: decl.Value, Line: v.Line, File: v.File, Doc: v.Doc})
	}
	return out
}

// errModuleVarDeclaredLater - значение переменной модуля обращается к
// переменной модуля name, объявленной ниже.
func errModuleVarDeclaredLater(name string, line int) error {
	return errAtLine(line, CodeNameResolution, "переменная модуля %q объявлена ниже - значение переменной модуля может использовать только переменные, объявленные выше", name)
}

// checkModuleVarName - имя переменной модуля не совпадает с другой
// переменной модуля и с объявлениями верхнего уровня.
func (a *Analyzer) checkModuleVarName(v *ast.VarDecl) error {
	clash := func(what string) error {
		return errAtLine(v.Line, CodeDuplicateDecl, "переменная модуля %q: имя совпадает с уже объявленн%s", v.Name, what)
	}
	if _, exists := a.moduleVars[v.Name]; exists {
		return errAtLine(v.Line, CodeDuplicateDecl, "переменная модуля %q объявлена более одного раза", v.Name)
	}
	if _, exists := a.moduleConstExprs[v.Name]; exists {
		return clash("ой константой")
	}
	if _, exists := a.funcs[v.Name]; exists {
		return clash("ой функцией")
	}
	if _, exists := a.entities[v.Name]; exists {
		return clash("ой сущностью")
	}
	if _, exists := a.enums[v.Name]; exists {
		return clash("ым перечислением")
	}
	if _, exists := a.interfaces[v.Name]; exists {
		return clash("ым интерфейсом")
	}
	return nil
}
