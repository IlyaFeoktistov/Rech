// Приблизительное число ссылок на объявление (для отображения в hover,
// "N ссылок") - по токенам текущего документа (см. package doc resolve.go
// про то, почему это приемлемо: подсчёт не восстанавливает области
// видимости, значит переменная/поле другой сущности с тем же именем даёт
// ложное совпадение - тот же класс упрощения, что и у остального пакета,
// не претендует на точность, нужную для настоящего переименования/
// рефакторинга.
package lsp

import (
	"fmt"

	"github.com/IlyaFeoktistov/Rech/internal/ir"
	"github.com/IlyaFeoktistov/Rech/internal/lexer"
)

// countFunctionRefs - IDENT-токен name, за которым сразу "(" (вызов) или
// перед которым "&" (FuncRefExpr) - НЕ считая само объявление ("функция
// имя(...)", отличается предшествующим KwFunction) и не считая "получатель.
// имя(...)" (это вызов МЕТОДА с тем же именем, см. countMethodRefs).
func (d *doc) countFunctionRefs(name string) int {
	count := 0
	for i, tok := range d.tokens {
		if tok.Type != lexer.IDENT || tok.Value != name {
			continue
		}
		if i > 0 && d.tokens[i-1].Type == lexer.KwFunction {
			continue
		}
		if i > 0 && d.tokens[i-1].Type == lexer.BITAND {
			count++
			continue
		}
		if i > 0 && d.tokens[i-1].Type == lexer.DOT {
			continue
		}
		if i+1 < len(d.tokens) && d.tokens[i+1].Type == lexer.LPAREN {
			count++
		}
	}
	return count
}

// countMethodRefs - "получатель.имя(...)" - объявление метода само не
// подходит (методы объявляются без ведущей "." перед именем).
func (d *doc) countMethodRefs(name string) int {
	count := 0
	for i, tok := range d.tokens {
		if tok.Type != lexer.IDENT || tok.Value != name {
			continue
		}
		if i > 0 && d.tokens[i-1].Type == lexer.DOT && i+1 < len(d.tokens) && d.tokens[i+1].Type == lexer.LPAREN {
			count++
		}
	}
	return count
}

// countMemberRefs - "получатель.имя" (поле или константа) - без требования
// "(" следом (в отличие от countMethodRefs).
func (d *doc) countMemberRefs(name string) int {
	count := 0
	for i, tok := range d.tokens {
		if tok.Type == lexer.IDENT && tok.Value == name && i > 0 && d.tokens[i-1].Type == lexer.DOT {
			count++
		}
	}
	return count
}

// countEntityRefs - "новый/новая/новое/новые Имя(...)" (создание
// экземпляра, по токенам) плюс каждое использование Имя как типа поля/
// параметра/результата (по уже разрешённому IR - точнее токенов, раз эти
// данные уже под рукой).
func (d *doc) countEntityRefs(name string) int {
	count := 0
	for i, tok := range d.tokens {
		if tok.Type == lexer.IDENT && tok.Value == name && i > 0 && d.tokens[i-1].Type == lexer.KwNew {
			count++
		}
	}
	if d.irProg == nil {
		return count
	}
	target := ir.EntityType(name)
	countType := func(t ir.Type) {
		if t == target {
			count++
		}
	}
	for _, fn := range d.irProg.Functions {
		for _, p := range fn.Params {
			countType(p.Type)
		}
		if fn.HasReturn {
			countType(fn.ReturnType)
		}
	}
	for _, e := range d.irProg.Entities {
		for _, f := range e.Fields {
			countType(f.Type)
		}
		for _, m := range e.Methods {
			for _, p := range m.Params {
				countType(p.Type)
			}
			if m.HasReturn {
				countType(m.ReturnType)
			}
		}
	}
	return count
}

// refCountLabel - число ссылок, согласованное со словом "ссылка" (1
// ссылка, 2 ссылки, 5 ссылок, 11 ссылок, 21 ссылка, ...) - обычное правило
// русских числительных (11-14 всегда "ссылок", иначе по последней цифре).
func refCountLabel(n int) string {
	mod100 := n % 100
	if mod100 >= 11 && mod100 <= 14 {
		return fmt.Sprintf("%d ссылок", n)
	}
	switch n % 10 {
	case 1:
		return fmt.Sprintf("%d ссылка", n)
	case 2, 3, 4:
		return fmt.Sprintf("%d ссылки", n)
	default:
		return fmt.Sprintf("%d ссылок", n)
	}
}

// withRefCount добавляет строку числа ссылок под текстом hover.
func withRefCount(block string, count int) string {
	return block + "\n\n" + refCountLabel(count)
}
