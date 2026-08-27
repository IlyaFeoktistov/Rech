// Разрешение "что находится под курсором" - hover.go/completion.go задают
// вопрос, resolve.go на него отвечает, работая над d.tokens (последний
// удачно разобранный текст, см. document.go) - не над текущим (возможно
// кривым) буфером напрямую.
package lsp

import (
	"github.com/IlyaFeoktistov/Rech/internal/ir"
	"github.com/IlyaFeoktistov/Rech/internal/lexer"
)

// tokenIndexAt возвращает индекс токена в d.tokens, чья позиция включает
// (line, col) (оба 1-индексные, как lexer.Token.Line/Col) - -1, если такого
// нет (курсор на пробеле/переносе строки/за пределами последнего токена).
func (d *doc) tokenIndexAt(line, col int) int {
	for i, tok := range d.tokens {
		if tok.Line != line {
			continue
		}
		width := len([]rune(tok.Value))
		if width == 0 {
			width = 1
		}
		if col >= tok.Col && col < tok.Col+width {
			return i
		}
	}
	return -1
}

// chainBefore строит цепочку имён "receiver.поле.метод" (без токена DOT
// между ними), заканчивая на токене с индексом end (включительно), идя
// назад по d.tokens, пока встречается чередование IDENT/KwThis, DOT -
// первое имя цепочки - её "receiver" (локальная переменная/параметр/
// "этот"/имя функции/сущности верхнего уровня, разрешается отдельно, см.
// resolveChain). Возвращает имена в порядке слева направо (base сначала).
func (d *doc) chainBefore(end int) []string {
	if end < 0 || end >= len(d.tokens) {
		return nil
	}
	var revNames []string
	i := end
	for i >= 0 {
		tok := d.tokens[i]
		if tok.Type != lexer.IDENT && tok.Type != lexer.KwThis {
			break
		}
		name := tok.Value
		if tok.Type == lexer.KwThis {
			name = "этот"
		}
		revNames = append(revNames, name)
		i--
		if i < 0 || d.tokens[i].Type != lexer.DOT {
			break
		}
		i-- // токен перед DOT - следующее (идя назад) имя цепочки
	}
	// revNames собран от конца цепочки к началу - развернуть.
	out := make([]string, len(revNames))
	for k, v := range revNames {
		out[len(revNames)-1-k] = v
	}
	return out
}

// resolveChain разрешает цепочку имён (см. chainBefore) в тип её
// ПОСЛЕДНЕГО элемента, в контексте функции/метода, содержащего строку line -
// base (chain[0]) ищется среди локальных переменных/параметров текущей
// функции, "этот" (если внутри метода), затем как имя функции/сущности
// верхнего уровня (тип - сама сущность, для "новый Тип"/имени типа под
// курсором); каждый следующий элемент - поле ТЕКУЩЕЙ сущности (методы в
// середине цепочки не поддержаны - вызов метода "a.метод().поле" не
// разрешается, cursor должен стоять на самом методе/поле, не после вызова).
func (d *doc) resolveChain(chain []string, line int) (ir.Type, bool) {
	if len(chain) == 0 {
		return ir.Type{}, false
	}
	scope := d.scopeAt(line)
	head := chain[0]
	var cur ir.Type
	switch {
	case head == "этот" && scope != nil && scope.entityName != "":
		cur = ir.EntityType(scope.entityName)
	default:
		if t, ok := localsAt(scope, line)[head]; ok {
			cur = t
		} else if _, ok := d.entities[head]; ok {
			cur = ir.EntityType(head)
		} else {
			return ir.Type{}, false
		}
	}
	for _, member := range chain[1:] {
		if cur.Kind != ir.KindEntity {
			return ir.Type{}, false
		}
		ent := d.entities[cur.Entity]
		if ent == nil {
			return ir.Type{}, false
		}
		found := false
		for _, f := range ent.Fields {
			if f.Name == member {
				cur = f.Type
				found = true
				break
			}
		}
		if !found {
			return ir.Type{}, false
		}
	}
	return cur, true
}
