package gomorphy

import "strings"

// Этот файл добавлен в Rech поверх gomorphy v0.2.3 (github.com/jus1d/
// gomorphy, MIT): публичный API библиотеки отдаёт только лучший тег слова и
// список форм без тегов, а морфологии языка нужны ВСЕ разборы словоформы и
// каждая форма лексемы со своим тегом.

// Form - словоформа и её тег OpenCorpora ("NOUN,anim,masc plur,loct").
type Form struct {
	Word string
	Tag  string
}

// Parse - один разбор словоформы: её тег и вся лексема (все формы
// парадигмы, первой - начальная форма).
type Parse struct {
	Tag    string
	Lexeme []Form
}

// Parses - все разборы word (в нижнем регистре, "ё" значима); nil - слова
// нет в словаре.
func (a *Analyzer) Parses(word string) []Parse {
	lower := strings.ToLower(strings.TrimSpace(word))
	if lower == "" {
		return nil
	}
	var out []Parse
	for _, e := range a.words.get(lower) {
		para := a.paradigms[e.paradigmID]
		n := len(para) / 3
		if int(e.formIdx) >= n {
			continue
		}
		stem, ok := a.extractStem(lower, para, n, int(e.formIdx))
		if !ok {
			continue
		}
		p := Parse{Tag: a.tagAt(para, n, int(e.formIdx))}
		for i := 0; i < n; i++ {
			p.Lexeme = append(p.Lexeme, Form{
				Word: paradigmPrefixes[para[2*n+i]] + stem + a.suffixes[para[i]],
				Tag:  a.tagAt(para, n, i),
			})
		}
		out = append(out, p)
	}
	return out
}

func (a *Analyzer) tagAt(para []uint16, n, i int) string {
	id := int(para[n+i])
	if id >= len(a.gramtab) {
		return ""
	}
	return a.gramtab[id]
}
