package lsp

import (
	"github.com/IlyaFeoktistov/Rech/internal/lexer"
	"github.com/IlyaFeoktistov/Rech/internal/morph"
)

// Ссылки на ТИП - сущность/структура, интерфейс, перечисление, синоним:
// любое слово, которое является именем типа или его словоформой
// ("наследует Точку", "реализует Фигуру"), кроме самого объявления и
// члена через "." (поле/метод с тем же текстом). Имена типов пишутся с
// заглавной буквы и не пересекаются с переменными/функциями (snake_case),
// поэтому совпадение слова - это использование типа: в аннотации,
// "новый", "как", "реализует"/"наследует", аргументе generic, "Тип::член".

// typeDeclKeywords - ключевые слова, сразу после которых имя - объявление
// типа, а не ссылка на него.
var typeDeclKeywords = map[lexer.TokenType]bool{
	lexer.KwEntity:    true,
	lexer.KwClass:     true,
	lexer.KwStruct:    true,
	lexer.KwInterface: true,
	lexer.KwEnum:      true,
	lexer.KwSynonym:   true,
}

// typeRefMatcher - matcher для crossFileRefs/codelens: ссылки на тип name
// в toks - слово, совпадающее с одной из словоформ имени (typeNameForms).
func (d *doc) typeRefMatcher() func([]lexer.Token, string) []lexer.Token {
	return func(toks []lexer.Token, name string) []lexer.Token {
		forms := d.typeNameForms(name)
		var out []lexer.Token
		for i, tok := range toks {
			if tok.Type != lexer.IDENT || !forms[tok.Value] {
				continue
			}
			if i > 0 && (typeDeclKeywords[toks[i-1].Type] || toks[i-1].Type == lexer.DOT) {
				continue
			}
			out = append(out, tok)
		}
		return out
	}
}

// typeNameForms - все словоформы имени типа name (от объявления к формам,
// не разбором каждого слова): парадигма из словаря документа, если имя
// там зарегистрировано, иначе - построенная заново (тип из другого файла),
// для одушевлённого и неодушевлённого прочтения сразу (они различаются
// только винительным падежом). Само name - всегда форма.
func (d *doc) typeNameForms(name string) map[string]bool {
	forms := map[string]bool{name: true}
	add := func(dict *morph.Dictionary) {
		if l, ok := dict.Lookup(name); ok {
			for _, f := range l.Forms {
				forms[f] = true
			}
		}
	}
	if d.dict != nil {
		if _, ok := d.dict.Lookup(name); ok {
			add(d.dict)
			return forms
		}
	}
	for _, animacy := range []morph.Animacy{morph.Inanimate, morph.Animate} {
		dict := morph.NewDictionary()
		if dict.RegisterNoun(name, animacy) == nil {
			add(dict)
		}
	}
	return forms
}

// typeWordform - имя типа, одной из словоформ которого является word.
func (d *doc) typeWordform(word string) (string, bool) {
	for _, names := range []func() []string{d.entityNames, d.interfaceNames, d.enumNames, d.synonymNames} {
		for _, n := range names() {
			if d.typeNameForms(n)[word] {
				return n, true
			}
		}
	}
	return "", false
}

func (d *doc) entityNames() []string {
	out := make([]string, 0, len(d.astEntities))
	for n := range d.astEntities {
		out = append(out, n)
	}
	return out
}

func (d *doc) interfaceNames() []string {
	out := make([]string, 0, len(d.astInterfaces))
	for n := range d.astInterfaces {
		out = append(out, n)
	}
	return out
}

func (d *doc) enumNames() []string {
	out := make([]string, 0, len(d.astEnums))
	for n := range d.astEnums {
		out = append(out, n)
	}
	return out
}

func (d *doc) synonymNames() []string {
	out := make([]string, 0, len(d.astSynonyms))
	for n := range d.astSynonyms {
		out = append(out, n)
	}
	return out
}
