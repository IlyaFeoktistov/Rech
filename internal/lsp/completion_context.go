package lsp

import (
	"strings"

	"github.com/IlyaFeoktistov/Rech/internal/ir"
	"github.com/IlyaFeoktistov/Rech/internal/lexer"
	"github.com/IlyaFeoktistov/Rech/internal/semantic"
)

// Контекст автодополнения - что стоит перед курсором в ТЕКУЩЕМ тексте
// строки (не в последнем удачно разобранном): "получатель." или
// "Тип::" с уже набранным началом имени, либо просто начало имени. Пока
// строка недописана, файл не разбирается целиком, и функция с этой строкой
// выпадает из анализа вместе со своими локальными переменными - поэтому
// области видимости берутся из анализа текста, где строка курсора
// заменена пустой (completionDoc).

type completionKind int

const (
	completionGeneral completionKind = iota
	completionMember                 // "получатель.<курсор>"
	completionStatic                 // "Тип::<курсор>"
	completionType                   // "имя: <курсор>" - аннотация типа
)

// chainElem - звено цепочки получателя: имя и был ли это вызов ("метод()").
type chainElem struct {
	name string
	call bool
}

type completionCtx struct {
	kind  completionKind
	chain []chainElem
	// prefix - значимые токены строки до набираемого слова (для
	// completionGeneral - место курсора, completion_place.go).
	prefix []lexer.Token
}

// completionContextAt - см. package doc выше; (line, col) - 1-индексные.
func (d *doc) completionContextAt(line, col int) completionCtx {
	lines := strings.Split(d.text, "\n")
	if line < 1 || line > len(lines) {
		return completionCtx{}
	}
	runes := []rune(lines[line-1])
	if col-1 < len(runes) {
		runes = runes[:max(col-1, 0)]
	}
	prefix := templateInsertionTail(string(runes))
	toks, err := lexer.New(prefix).Tokenize()
	if err != nil {
		return completionCtx{}
	}
	var sig []lexer.Token
	for _, t := range toks {
		if t.Type != lexer.EOF && t.Type != lexer.NEWLINE {
			sig = append(sig, t)
		}
	}
	i := len(sig) - 1
	if i >= 0 && isWordToken(sig[i]) && sig[i].Col-1+len([]rune(sig[i].Value)) == len([]rune(prefix)) {
		i--
	}
	if i < 0 {
		return completionCtx{}
	}
	general := completionCtx{prefix: sig[:i+1]}
	switch sig[i].Type {
	case lexer.DOT:
		return completionCtx{kind: completionMember, chain: chainBack(sig, i-1)}
	case lexer.DBLCOLON:
		if i >= 1 && sig[i-1].Type == lexer.IDENT {
			return completionCtx{kind: completionStatic, chain: []chainElem{{name: sig[i-1].Value}}}
		}
	case lexer.COLON:
		// "а ? б : <курсор>" - ветка тернарного оператора, не тип.
		for _, t := range sig[:i] {
			if t.Type == lexer.QUESTION {
				return general
			}
		}
		return completionCtx{kind: completionType}
	}
	return general
}

// templateInsertionTail - у курсора внутри незакрытой вставки "${" шаблонной
// строки - текст после неё (выражение вставки), иначе s как есть.
func templateInsertionTail(s string) string {
	open := strings.LastIndex(s, "${")
	if open < 0 || strings.Contains(s[open:], "}") {
		return s
	}
	return s[open+2:]
}

// isWordToken - токен-слово (имя или ключевое слово): набранное начало
// имени у курсора может оказаться и ключевым словом целиком.
func isWordToken(t lexer.Token) bool {
	v := []rune(t.Value)
	return len(v) > 0 && (t.Type == lexer.IDENT || t.Type == lexer.KwThis || isLetter(v[0]))
}

func isLetter(r rune) bool {
	return r == '_' || (r >= 'a' && r <= 'z') || (r >= 'A' && r <= 'Z') || (r >= 'а' && r <= 'я') || (r >= 'А' && r <= 'Я') || r == 'ё' || r == 'Ё'
}

// chainBack - цепочка получателя, кончающаяся токеном j (включительно):
// имена, "этот" и вызовы "имя(...)" через точку, слева направо.
func chainBack(toks []lexer.Token, j int) []chainElem {
	var rev []chainElem
	for j >= 0 {
		call := false
		if toks[j].Type == lexer.RPAREN {
			depth := 0
			for ; j >= 0; j-- {
				if toks[j].Type == lexer.RPAREN {
					depth++
				} else if toks[j].Type == lexer.LPAREN {
					depth--
					if depth == 0 {
						break
					}
				}
			}
			j--
			call = true
			if j < 0 {
				return nil
			}
		}
		t := toks[j]
		if t.Type != lexer.IDENT && t.Type != lexer.KwThis {
			if call {
				return nil
			}
			break
		}
		name := t.Value
		if t.Type == lexer.KwThis {
			name = "этот"
		}
		rev = append(rev, chainElem{name: name, call: call})
		if j == 0 || toks[j-1].Type != lexer.DOT {
			break
		}
		j -= 2
	}
	out := make([]chainElem, len(rev))
	for k, e := range rev {
		out[len(rev)-1-k] = e
	}
	return out
}

// completionDoc - документ, по которому автодополнение берёт области
// видимости и типы: сам d, если в нём нет ошибок, иначе анализ текста, где
// строка курсора заменена пустой (см. package doc); если и он не
// разобрался - d.
func (d *doc) completionDoc(line int) *doc {
	if !d.hasErrors() {
		return d
	}
	lines := strings.Split(d.text, "\n")
	if line < 1 || line > len(lines) {
		return d
	}
	lines[line-1] = ""
	repaired := &doc{uri: d.uri, path: d.path, forCompletion: true}
	repaired.update(strings.Join(lines, "\n"))
	if repaired.irProg == nil {
		return d
	}
	return repaired
}

func (d *doc) hasErrors() bool {
	for _, diag := range d.diagnostics {
		if diag.Severity == errorSeverity {
			return true
		}
	}
	return false
}

// chainTarget - во что разрешилась цепочка получателя: тип значения, либо
// имя сущности/перечисления, если цепочка - само имя типа ("Точка.",
// "Цвет.").
type chainTarget struct {
	typ      ir.Type
	typeName string
	isEnum   bool
}

// resolveCompletionChain - тип значения цепочки в контексте строки line:
// голова - "этот", локальная переменная/параметр, вызов своей функции или
// имя типа/перечисления; дальше поля и вызовы методов (с наследованием),
// методы встроенных коллекций. Сужение "?Тип" после проверки на Ничто
// принимается без проверки - автодополнение показывает члены внутреннего
// типа.
func (d *doc) resolveCompletionChain(chain []chainElem, line int, blocks []textBlock) (chainTarget, bool) {
	if len(chain) == 0 {
		return chainTarget{}, false
	}
	scope := d.scopeForBlocks(blocks, line)
	head := chain[0]
	var cur ir.Type
	switch {
	case head.name == "этот" && scope != nil && scope.entityName != "":
		cur = ir.EntityType(scope.entityName)
	case head.call:
		fn, ok := d.funcs[head.name]
		if !ok || !fn.HasReturn {
			return chainTarget{}, false
		}
		cur = fn.ReturnType
	default:
		if t, ok := d.visibleLocals(blocks, line)[head.name]; ok && t.Kind != ir.KindUnknown {
			cur = t
		} else if len(chain) == 1 {
			if _, ok := d.astEnums[head.name]; ok {
				return chainTarget{typeName: head.name, isEnum: true}, true
			}
			if _, ok := d.entities[head.name]; ok {
				return chainTarget{typeName: head.name}, true
			}
			return chainTarget{}, false
		} else {
			return chainTarget{}, false
		}
	}
	for _, e := range chain[1:] {
		next, ok := d.memberType(d.unwrapNullable(cur), e)
		if !ok {
			return chainTarget{}, false
		}
		cur = next
	}
	return chainTarget{typ: d.unwrapNullable(cur)}, true
}

// memberType - тип поля/результата метода e у значения типа t.
func (d *doc) memberType(t ir.Type, e chainElem) (ir.Type, bool) {
	if t.Kind != ir.KindEntity {
		return ir.Type{}, false
	}
	ent := d.entities[t.Entity]
	if ent == nil {
		methods, ok := collectionMethodsOf(t.Entity)
		if !ok || !e.call {
			return ir.Type{}, false
		}
		for _, m := range methods {
			if m.Name == e.name && m.Return != "" {
				return d.typeByName(m.Return)
			}
		}
		return ir.Type{}, false
	}
	for cur := ent; cur != nil; cur = d.entities[cur.BaseName] {
		if !e.call {
			for _, f := range cur.Fields {
				if f.Name == e.name {
					return f.Type, true
				}
			}
			continue
		}
		for _, m := range cur.Methods {
			if m.Name == e.name && m.HasReturn {
				return m.ReturnType, true
			}
		}
	}
	return ir.Type{}, false
}

// unwrapNullable - внутренний тип "?Тип" (сам t, если он не nullable).
func (d *doc) unwrapNullable(t ir.Type) ir.Type {
	if t.Kind != ir.KindNullable {
		return t
	}
	if inner, ok := d.typeByName(strings.TrimPrefix(t.Entity, "?")); ok {
		return inner
	}
	return t
}

// typeByName - тип по его каноническому имени: примитив, иначе сущность
// или инстанциация коллекции ("Список<Целое>").
func (d *doc) typeByName(name string) (ir.Type, bool) {
	if t, ok := semantic.PrimitiveType(name); ok {
		return t, true
	}
	if name == "" {
		return ir.Type{}, false
	}
	return ir.EntityType(name), true
}
