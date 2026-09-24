// Перегрузка поверх унаследованного метода - группа вариантов метода у
// потомка ОБЪЕДИНЯЕТСЯ с вариантами базовой сущности (как в Java/C#), не
// скрывает их. Вариант потомка с тем же списком типов параметров, что у
// базового варианта, - переопределение: получает тот же ключ (голое имя
// обычного метода или мангл-имя перегрузки базовой), а значит тот же слот
// виртуальной диспетчеризации; остальные - новые перегрузки со своим
// мангл-именем.
package semantic

import (
	"github.com/IlyaFeoktistov/Rech/internal/ast"
	"github.com/IlyaFeoktistov/Rech/internal/ir"
)

// baseMethodVariants - варианты метода name, унаследованные от base: группа
// перегрузок базовой сущности, либо её обычный (не статический) метод -
// единственный вариант с MangledName = голому имени (ключ, под которым он
// уже лежит в methods[base] и vtable).
func (a *Analyzer) baseMethodVariants(methods map[string]map[string]funcSig, base, name string) []funcSig {
	if group := a.methodOverloads[base][name]; group != nil {
		return group
	}
	sig, ok := methods[base][name]
	if !ok || sig.IsStatic {
		return nil
	}
	sig.MangledName = name
	return []funcSig{sig}
}

// alignOverloadsWithBase - до applyInheritance: для каждого имени метода
// потомка ent, у которого есть унаследованные варианты, раскладывает свои
// варианты по сигнатурам базовых (см. package doc) и переносит имя из
// ownMethods в ownOverloads. Обычное переопределение обычного метода (один
// свой, один базовый, тот же список параметров) не трогается - его
// проверяет applyInheritance как раньше.
func (a *Analyzer) alignOverloadsWithBase(methods map[string]map[string]funcSig, ent *ast.EntityDecl, byMethodName map[string][]*ast.MethodDecl, ownMethods map[string]funcSig, ownOverloads map[string][]funcSig) error {
	if ent.BaseName == "" {
		return nil
	}
	for name, decls := range byMethodName {
		base := a.baseMethodVariants(methods, ent.BaseName, name)
		if base == nil {
			continue
		}
		own := ownOverloads[name]
		if own == nil {
			sig, ok := ownMethods[name]
			if !ok {
				continue
			}
			if len(base) == 1 && base[0].MangledName == name && sameParamTypeList(base[0].Params, sig.Params) {
				continue
			}
			own = []funcSig{sig}
		}
		aligned := make([]funcSig, len(own))
		for i, v := range own {
			if decls[i].IsAbstract || decls[i].IsStatic || hasVariadicParam(decls[i].Params) {
				return errNoPos(CodeEntityDecl, "сущность %q: метод %q: базовая сущность %q уже объявляет метод с этим именем - перегрузка поверх него не поддерживается для абстрактных/статических/вариативных методов", ent.Name, name, ent.BaseName)
			}
			v.MangledName = overloadMangledName(name, v.Params)
			for _, b := range base {
				if !sameParamTypeList(b.Params, v.Params) {
					continue
				}
				if b.Variadic || !sigsMatch(b, v) {
					return errNoPos(CodeInheritance, "сущность %q: метод %q(%s) переопределяет вариант базовой сущности %q, но не совпадает с ним по типу результата", ent.Name, name, paramTypesString(v.Params), ent.BaseName)
				}
				v.MangledName = b.MangledName
			}
			a.methodDeclNames[decls[i]] = v.MangledName
			aligned[i] = v
		}
		delete(ownMethods, name)
		ownOverloads[name] = aligned
	}
	return nil
}

// mergeInheritedOverloads - группа name потомка: унаследованные варианты,
// кроме переопределённых (тот же MangledName у собственного), затем
// собственные.
func mergeInheritedOverloads(base, own []funcSig, self ir.Type) []funcSig {
	overridden := make(map[string]bool, len(own))
	for _, v := range own {
		overridden[v.MangledName] = true
	}
	merged := make([]funcSig, 0, len(base)+len(own))
	for _, b := range base {
		if overridden[b.MangledName] {
			continue
		}
		if b.SelfReturn {
			b.ReturnType = self
		}
		merged = append(merged, b)
	}
	return append(merged, own...)
}
