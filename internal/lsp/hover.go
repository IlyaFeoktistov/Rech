package lsp

import (
	"fmt"
	"strings"

	"github.com/IlyaFeoktistov/Rech/internal/ir"
	"github.com/IlyaFeoktistov/Rech/internal/lexer"
)

// hover строит текст всплывающей подсказки для позиции (line, col, оба
// 1-индексные) - "", false, если под курсором нечего показать (пробел,
// ключевое слово, нераспознанный идентификатор).
func (d *doc) hover(line, col int) (string, bool) {
	if d.irProg == nil {
		return "", false
	}
	i := d.tokenIndexAt(line, col)
	if i < 0 {
		return "", false
	}
	tok := d.tokens[i]
	if tok.Type != lexer.IDENT && tok.Type != lexer.KwThis {
		return "", false
	}
	chain := d.chainBefore(i)
	if len(chain) == 0 {
		return "", false
	}
	name := chain[len(chain)-1]

	if len(chain) == 1 {
		return d.hoverBareName(name, line)
	}

	container, ok := d.resolveChain(chain[:len(chain)-1], line)
	if !ok || container.Kind != ir.KindEntity {
		return "", false
	}
	ent := d.entities[container.Entity]
	if ent == nil {
		return "", false
	}
	return d.hoverMember(ent, name)
}

// hoverBareName - курсор на самостоятельном имени (не после точки):
// локальная переменная/параметр, "этот", функция или сущность верхнего
// уровня, в этом порядке (первое совпадение побеждает).
func (d *doc) hoverBareName(name string, line int) (string, bool) {
	scope := d.scopeAt(line)
	if name == "этот" && scope != nil && scope.entityName != "" {
		return codeBlock(fmt.Sprintf("этот: %s", scope.entityName)), true
	}
	if scope != nil {
		if t, ok := localsAt(scope, line)[name]; ok {
			return codeBlock(fmt.Sprintf("%s: %s", name, t)), true
		}
	}
	if fn, ok := d.funcs[name]; ok {
		return withDoc(codeBlock(functionSignature(fn.Name, fn.Params, fn.HasReturn, fn.ReturnType)), fn.Doc), true
	}
	if ent, ok := d.entities[name]; ok {
		kind := "сущность"
		if ent.IsStruct {
			kind = "структура"
		}
		return withDoc(codeBlock(fmt.Sprintf("%s %s", kind, ent.Name)), ent.Doc), true
	}
	return "", false
}

// hoverMember - name - поле/метод/константа ent (последний элемент цепочки
// "receiver.поле"/"receiver.метод").
func (d *doc) hoverMember(ent *ir.Entity, name string) (string, bool) {
	for _, f := range ent.Fields {
		if f.Name == name {
			return withDoc(codeBlock(fmt.Sprintf("%s.%s: %s", ent.Name, f.Name, f.Type)), f.Doc), true
		}
	}
	for _, c := range ent.Constants {
		if c.Name == name {
			return withDoc(codeBlock(fmt.Sprintf("%s.%s: %s (константа)", ent.Name, c.Name, c.Type)), c.Doc), true
		}
	}
	for _, m := range ent.Methods {
		if m.Name == name {
			return withDoc(codeBlock(functionSignature(ent.Name+"."+m.Name, m.Params, m.HasReturn, m.ReturnType)), m.Doc), true
		}
	}
	return "", false
}

// functionSignature - "функция имя(параметр: Тип, ...): ТипРезультата" -
// без "функция" для методов (entityMethodName уже несёт "Тип.метод").
func functionSignature(name string, params []ir.Param, hasReturn bool, returnType ir.Type) string {
	parts := make([]string, len(params))
	for i, p := range params {
		parts[i] = fmt.Sprintf("%s: %s", p.Name, p.Type)
	}
	sig := fmt.Sprintf("%s(%s)", name, strings.Join(parts, ", "))
	if hasReturn {
		sig += ": " + returnType.String()
	}
	return sig
}

func codeBlock(code string) string {
	return "```rech\n" + code + "\n```"
}

// withDoc добавляет текст doc-комментария (обрезанный по краям пробел)
// под блоком кода - пусто, если Doc == "" (у объявления не было
// doc-комментария).
func withDoc(block, doc string) string {
	doc = strings.TrimSpace(doc)
	if doc == "" {
		return block
	}
	return block + "\n\n" + doc
}
