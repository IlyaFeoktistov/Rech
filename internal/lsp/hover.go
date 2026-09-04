package lsp

import (
	"fmt"
	"strconv"
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
	if doc, ok := keywordDocs[tok.Type]; ok {
		return doc, true
	}
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
			return codeBlock(fmt.Sprintf("%s: %s", name, d.dispType(t))), true
		}
	}
	if fn, ok := d.funcs[name]; ok {
		block := withDoc(codeBlock(d.functionSignature(d.disp(fn.Name), fn.Params, fn.HasReturn, fn.ReturnType)), fn.Doc)
		return withRefCount(block, d.countFunctionRefs(name)), true
	}
	if ent, ok := d.entities[name]; ok {
		kind := "сущность"
		if ent.IsStruct {
			kind = "структура"
		}
		if ent.IsAbstract {
			kind = "абстрактная " + kind
		}
		block := withDoc(codeBlock(fmt.Sprintf("%s %s", kind, d.disp(ent.Name))), ent.Doc)
		return withRefCount(block, d.countEntityRefs(name)), true
	}
	if doc, ok := builtinTypeDocs[name]; ok {
		return codeBlock(name) + "\n\n" + doc, true
	}
	if block, ok := hoverBuiltinFunc(name); ok {
		return block, true
	}
	return "", false
}

// hoverMember - name - поле/метод/константа ent (последний элемент цепочки
// "receiver.поле"/"receiver.метод").
func (d *doc) hoverMember(ent *ir.Entity, name string) (string, bool) {
	entName := d.disp(ent.Name)
	for _, f := range ent.Fields {
		if f.Name == name {
			block := withDoc(codeBlock(fmt.Sprintf("%s.%s: %s", entName, f.Name, d.dispType(f.Type))), f.Doc)
			return withRefCount(block, d.countMemberRefs(name)), true
		}
	}
	for _, c := range ent.Constants {
		if c.Name == name {
			block := codeBlock(fmt.Sprintf("%s.%s: %s (константа)", entName, c.Name, d.dispType(c.Type)))
			if lit, ok := literalText(c.Value); ok {
				block += fmt.Sprintf("\n\nЗначение: `%s`", lit)
			}
			block = withDoc(block, c.Doc)
			return withRefCount(block, d.countMemberRefs(name)), true
		}
	}
	for _, m := range ent.Methods {
		if m.Name == name {
			block := withDoc(codeBlock(d.functionSignature(entName+"."+m.Name, m.Params, m.HasReturn, m.ReturnType)), m.Doc)
			return withRefCount(block, d.countMethodRefs(name)), true
		}
	}
	return "", false
}

// literalText - текстовое представление значения константы, если оно уже
// разрешилось до простого литерала (типичный случай - "константа ИМЯ =
// 100") - "", false для любого более сложного выражения (арифметика,
// обращение к другой константе, вызов функции - resolveEntityConstant
// разрешает "любое выражение", но hover показывает значение только там,
// где это один литерал, не пытаясь заново вычислить/напечатать
// произвольное выражение).
func literalText(v ir.Expr) (string, bool) {
	switch v := v.(type) {
	case *ir.IntLiteral:
		return strconv.FormatInt(v.Value, 10), true
	case *ir.FloatLiteral:
		return strconv.FormatFloat(v.Value, 'g', -1, 64), true
	case *ir.StringLiteral:
		return strconv.Quote(v.Value), true
	case *ir.BoolLiteral:
		if v.Value {
			return "истина", true
		}
		return "ложь", true
	case *ir.CharLiteral:
		return "'" + string(v.Value) + "'", true
	default:
		return "", false
	}
}

// functionSignature - "функция имя(параметр: Тип, ...): ТипРезультата" -
// без "функция" для методов (entityMethodName уже несёт "Тип.метод"). name
// уже должно быть пропущено через d.disp вызывающей стороной (сама
// функция/метод могла быть найдена по алиасу, но название параметров ей не
// принадлежит явно - только типы параметров/результата нуждаются в
// dispType здесь).
func (d *doc) functionSignature(name string, params []ir.Param, hasReturn bool, returnType ir.Type) string {
	parts := make([]string, len(params))
	for i, p := range params {
		parts[i] = fmt.Sprintf("%s: %s", p.Name, d.dispType(p.Type))
	}
	sig := fmt.Sprintf("%s(%s)", name, strings.Join(parts, ", "))
	if hasReturn {
		sig += ": " + d.dispType(returnType)
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
