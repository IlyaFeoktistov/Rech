package lsp

import (
	"fmt"

	"github.com/IlyaFeoktistov/Rech/internal/ast"
)

// moduleVarsByName - переменные уровня модуля prog по имени.
func moduleVarsByName(prog *ast.Program) map[string]*ast.VarDecl {
	out := make(map[string]*ast.VarDecl, len(prog.Vars))
	for _, v := range prog.Vars {
		out[v.Name] = v
	}
	return out
}

// hoverModuleVar - hover на имени переменной уровня модуля: тип (из IR,
// если анализ его дал) и doc-комментарий объявления.
func (d *doc) hoverModuleVar(name string) (string, bool) {
	v, ok := d.astVars[name]
	if !ok {
		return "", false
	}
	header := "переменная модуля " + name
	if d.irProg != nil {
		for _, g := range d.irProg.Globals {
			if g.Name == name {
				header = fmt.Sprintf("%s %s (переменная модуля)", d.dispType(g.Type), name)
				break
			}
		}
	}
	return withDoc(codeBlock(header), v.Doc), true
}
