// Падежные/деепричастные словоформы голых имён - объявление верхнего уровня
// (сущность/функция) или локальная переменная/параметр может быть НАПИСАНО
// в месте использования иначе, чем в самом объявлении: имя сущности после
// "наследует"/"реализует" - в винительном падеже (internal/parser/entity.go,
// resolveDeclinedTypeRef), имя функции в деепричастном вызове ("сложив(2,
// 3)", internal/parser/gerund.go) - в деепричастной форме, локальная
// переменная/параметр при падежном сопоставлении аргумента естественного
// вызова (internal/parser/call.go) - любой словоформой. Парсер уже резолвит
// все три случая до леммы для СЕМАНТИКИ (IR несёт только именительную/
// инфинитивную форму), но сырые токены документа (d.tokens, на них держится
// hover/definition) остаются написанными как в исходнике - точное
// совпадение по карте имени (d.astFuncs[name] и т. п.) промахивается по
// буквально несовпадающей строке. Функции здесь - тот же резолв в обратную
// сторону (от написанной словоформы к лемме объявления); используются
// вызывающей стороной (hover.go/definition.go) ТОЛЬКО как запасной путь
// ПОСЛЕ буквального совпадения, чтобы не менять уже работающий, более
// частый случай.
package lsp

import (
	"github.com/IlyaFeoktistov/Rech/internal/ast"
	"github.com/IlyaFeoktistov/Rech/internal/morph"
	"github.com/IlyaFeoktistov/Rech/internal/structtag"
)

// localWordform ищет среди локальных переменных/параметров scope,
// действующих в строке line, ту, чья словоформа (d.dict.Analyze, тот же
// движок, что и wordformMatchesLemma/occurrences.go) - word (сама лемма или
// любая её падежная форма) - "", false, если ни одна не совпала.
func (d *doc) localWordform(scope *funcScope, line int, word string) (name string, ok bool) {
	lines := map[string]int{}
	collectVarDeclLines(scope.body, line, lines)
	for n := range lines {
		if d.wordformMatchesLemma(word, n) {
			return n, true
		}
	}
	for _, p := range scope.params {
		if d.wordformMatchesLemma(word, p.Name) {
			return p.Name, true
		}
	}
	return "", false
}

// entityWordform ищет среди сущностей документа ту, чья словоформа - word -
// самый частый случай - винительный падеж после "наследует"/"реализует",
// но проверяется любая словоформа dict.Analyze, тем же принципом, что и у
// localWordform.
func (d *doc) entityWordform(word string) (name string, ok bool) {
	for n := range d.astEntities {
		if d.wordformMatchesLemma(word, n) {
			return n, true
		}
	}
	return "", false
}

// funcGerund ищет среди функций документа ту, для которой word - её
// деепричастная форма (явный тег `деепричастие:...` переопределяет
// автоматическое morph.GerundOf(имяФункции) - тот же порядок приоритета,
// что и у internal/parser/gerund.go, registerGerundWord).
func (d *doc) funcGerund(word string) (name string, ok bool) {
	for n, fd := range d.astFuncs {
		if gerundWordOf(fd) == word {
			return n, true
		}
	}
	return "", false
}

func gerundWordOf(fd *ast.FunctionDecl) string {
	if tag := structtag.Parse(fd.Tag).Get("деепричастие"); tag != "" {
		return tag
	}
	auto, ok := morph.GerundOf(fd.Name)
	if !ok {
		return ""
	}
	return auto
}
