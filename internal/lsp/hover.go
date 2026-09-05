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
	if !ok {
		// resolveChain не знает о перечислениях (chain[0] - не локальная
		// переменная/поле/сущность) - "Имя.Член" только ДВУХзвенное (в
		// самом языке нет вложенных перечислений), проверяем именно этот
		// случай отдельным путём (enumhover.go), прежде чем сдаться.
		if len(chain) == 2 {
			if block, ok := d.hoverEnumMember(chain[0], name); ok {
				return block, true
			}
		}
		return "", false
	}
	if container.Kind != ir.KindEntity {
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
		return block, true
	}
	if ent, ok := d.entities[name]; ok {
		kind := "сущность"
		if ent.IsStruct {
			kind = "структура"
		}
		if ent.IsAbstract {
			kind = "абстрактная " + kind
		}
		header := fmt.Sprintf("%s %s", kind, d.disp(ent.Name))
		if ctor, ok := d.entityCtorSignature(ent); ok {
			header += "\n" + ctor
		}
		block := withDoc(codeBlock(header), ent.Doc)
		return block, true
	}
	if doc, ok := builtinTypeDocs[name]; ok {
		return codeBlock(name) + "\n\n" + doc, true
	}
	if block, ok := hoverBuiltinFunc(name); ok {
		return block, true
	}
	if block, ok := d.hoverEnumType(name); ok {
		return block, true
	}
	return "", false
}

// entityCtorSignature - "новый Тип(параметры)" для ent, показываемое рядом
// с самим объявлением при наведении на имя сущности - явный конструктор
// (ent.Ctor), если он есть; иначе, у структуры без ctor - её поля
// позиционно (единственная форма, которую неявный конструктор структуры
// принимает, см. internal/semantic/expr.go, analyzeNewExpr, ветка
// !sig.HasCtor && sig.IsStruct); иначе (сущность/класс без ctor) - "новый
// Тип()" совсем без аргументов (та же функция, ветка !sig.IsStruct - у
// класса без явного "конструктор" поля не задаются позиционно вообще, HasCtor
// НИКОГДА не наследуется от базовой сущности - internal/semantic/
// inheritance.go, applyInheritance - ent.Ctor == nil здесь означает ровно
// это, а не "проверить базовую"). "", false - абстрактная сущность (нельзя
// создать напрямую, показывать нечего).
func (d *doc) entityCtorSignature(ent *ir.Entity) (string, bool) {
	if ent.IsAbstract {
		return "", false
	}
	name := "новый " + d.disp(ent.Name)
	if ent.IsSemaphore {
		// "Семафор" - единственная системная сущность (наравне с
		// "Блокировкой" чуть ниже) без ast.EntityDecl/ent.Ctor, но, в
		// отличие от "Блокировки", с ОДНИМ обязательным параметром
		// конструктора (internal/semantic/semaphore.go) - общий путь ниже
		// (!ent.IsStruct -> "Тип()") дал бы неверную сигнатуру без
		// аргумента.
		return name + "(начальное_значение: Целое)", true
	}
	if ent.Ctor != nil {
		return d.functionSignature(name, ent.Ctor.Params, false, ir.Type{}), true
	}
	if !ent.IsStruct {
		return name + "()", true
	}
	params := make([]ir.Param, len(ent.Fields))
	for i, f := range ent.Fields {
		params[i] = ir.Param{Name: f.Name, Type: f.Type}
	}
	return d.functionSignature(name, params, false, ir.Type{}), true
}

// hoverMember - name - поле/метод/константа ent (последний элемент цепочки
// "receiver.поле"/"receiver.метод").
func (d *doc) hoverMember(ent *ir.Entity, name string) (string, bool) {
	entName := d.disp(ent.Name)
	if ent.IsMutex || ent.IsSemaphore {
		// "Блокировка"/"Семафор" - без ast.EntityDecl, ent.Methods у них
		// пуст (оба метода синтезированы backend'ом напрямую по
		// IsMutex/IsSemaphore, см. package doc internal/backend/cbackend/
		// mutex.go/semaphore.go) - обычный цикл по ent.Methods ниже никогда
		// не находит "захватить"/"освободить", поэтому наведение на них
		// нужно перехватить здесь же, канонической строкой.
		if doc, ok := mutexMethodDocs[name]; ok {
			return withDoc(codeBlock(fmt.Sprintf("%s.%s()", entName, name)), doc), true
		}
	}
	for _, f := range ent.Fields {
		if f.Name == name {
			block := withDoc(codeBlock(fmt.Sprintf("%s.%s: %s", entName, f.Name, d.dispType(f.Type))), f.Doc)
			return block, true
		}
	}
	for _, c := range ent.Constants {
		if c.Name == name {
			block := codeBlock(fmt.Sprintf("%s.%s: %s (константа)", entName, c.Name, d.dispType(c.Type)))
			if lit, ok := literalText(c.Value); ok {
				block += fmt.Sprintf("\n\nЗначение: `%s`", lit)
			}
			block = withDoc(block, c.Doc)
			return block, true
		}
	}
	for _, m := range ent.Methods {
		if m.Name == name {
			doc := d.resolvedMethodDoc(ent, name, m.Doc)
			block := withDoc(codeBlock(d.functionSignature(entName+"."+m.Name, m.Params, m.HasReturn, m.ReturnType)), doc)
			return block, true
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
