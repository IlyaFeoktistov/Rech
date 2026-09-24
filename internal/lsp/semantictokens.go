// textDocument/semanticTokens/full - классифицирует КАЖДЫЙ идентификатор
// документа по РЕАЛЬНОМУ виду его объявления (функция/тип/переменная/
// параметр/поле/член перечисления), а не по регистру первой буквы имени,
// как делает статическая подсветка синтаксиса (TextMate, syntaxes/
// rech.tmLanguage.json - см. там же комментарии "голый идентификатор перед
// '(' "/"голое имя ЗАГЛАВНЫМИ_БУКВАМИ" и т. п.) - те правила ошибаются,
// как только имя написано не по соглашению (пользователь спросил напрямую:
// "а если я захочу функции в CamelCase?"). VS Code сам объединяет два
// источника: там, где семантический токен ЕСТЬ, он побеждает TextMate; там,
// где семантических токенов нет вовсе (эта функция ничего не вернула для
// документа с ошибкой разбора, либо конкретное имя не удалось
// классифицировать - обобщённый тип generic-параметра и т. п.), подсветка
// остаётся прежней, статической.
//
// Использует тот же способ разбора цепочки "receiver.имя", что и hover/
// definition (chainBefore/resolveChain, resolve.go) - применяется К КАЖДОМУ
// IDENT-токену документа по отдельности (не только под курсором), поэтому
// результат классификации ("какого вида это имя") вынесен в отдельные,
// более простые функции (classifyBareName/classifyMember), а не переиспользует
// напрямую hoverBareName/hoverMember (те строят готовый MARKDOWN-текст,
// здесь нужен только вид).
package lsp

import (
	"github.com/IlyaFeoktistov/Rech/internal/ir"
	"github.com/IlyaFeoktistov/Rech/internal/lexer"
)

// semanticTokenTypes/semanticTokenModifiers - легенда, отправляемая клиенту
// один раз при "initialize" (server.go) - индекс имени в этом списке и есть
// числовое значение поля "type"/"modifiers" у каждого токена ниже.
var semanticTokenTypes = []string{"function", "type", "variable", "parameter", "property", "enumMember"}
var semanticTokenModifiers = []string{"readonly"}

const (
	stFunction = iota
	stType
	stVariable
	stParameter
	stProperty
	stEnumMember
)

const modReadonly = 1 << 0

// semToken - один классифицированный идентификатор (строка/столбец начала,
// оба 1-индексные, как lexer.Token, и длина в рунах - тот же формат, что и
// definitionPos/occPos пакета, до LSP-кодирования encodeSemanticTokens
// переводит их в относительные смещения).
type semToken struct {
	Line, Col, Len int
	Type           int
	Mods           int
}

// semanticTokens - один проход по ВСЕМ токенам документа - nil, если
// последний разбор не удался вовсе (d.irProg/d.astProg == nil, тот же
// охват, что у hover/definition), а ТАКЖЕ если последний update() провалился
// на семантике/линковке (d.text != d.goodText) - в этом случае document.go,
// update() пересобирает d.tokens/d.astProg на НОВЫЙ (кривой) текст, но
// намеренно оставляет d.scopes/d.irProg/d.entities/d.funcs от ПРЕЖНЕГО
// удачного разбора (ради hover/definition - см. package doc update). Проход
// ниже классифицирует КАЖДЫЙ токен документа по scope/entities/funcs, а не
// только тот, что под курсором, поэтому такой рассинхрон (свежие позиции
// токенов против чужих старых scope-границ) даёт не "прежнюю" подсветку, а
// ПОРЧЕНУЮ - конкретно переменные (сильнее привязанные к scope/строке, чем
// функции/типы) внезапно перестают классифицироваться и остаются белыми,
// пока файл снова не станет разбираться полностью успешно. Отступать в nil
// в этом случае безопаснее: VS Code сам откатится на статическую
// TextMate-подсветку для ВСЕГО документа, а не на смесь верного и битого.
func (d *doc) semanticTokens() []semToken {
	if d.irProg == nil || d.astProg == nil || d.text != d.goodText {
		return nil
	}
	var out []semToken
	for i, tok := range d.tokens {
		if tok.Type != lexer.IDENT {
			continue
		}
		chain := d.chainBefore(i)
		if len(chain) == 0 {
			continue
		}
		name := chain[len(chain)-1]
		if len(chain) == 1 {
			if t, mods, ok := d.classifyBareName(name, tok.Line); ok {
				out = append(out, semToken{tok.Line, tok.Col, len([]rune(name)), t, mods})
			}
			continue
		}
		if container, ok := d.resolveChain(chain[:len(chain)-1], tok.Line); ok {
			if t, mods, ok := d.classifyMember(container, name); ok {
				out = append(out, semToken{tok.Line, tok.Col, len([]rune(name)), t, mods})
			}
			continue
		}
		// resolveChain не знает о перечислениях (chain[0] - имя типа, не
		// локальная переменная/поле/сущность) - "Имя.Член" только
		// ДВУХзвенное (тот же случай, что и у hover.go, hoverEnumMember).
		if len(chain) == 2 {
			if _, ok := d.hoverEnumMember(chain[0], name); ok {
				out = append(out, semToken{tok.Line, tok.Col, len([]rune(name)), stEnumMember, 0})
			}
		}
	}
	return out
}

// classifyBareName - вид имени name (не после "."), под курсором в строке
// line - тот же приоритет, что и у hoverBareName (hover.go): локальная
// переменная/параметр -> функция (обычная/внешняя) -> тип (сущность/
// внешняя сущность/встроенный тип/перечисление) -> константа модуля;
// падежная/деепричастная словоформа (wordform_definition.go) - запасной
// путь В САМОМ КОНЦЕ, рекурсивно на найденную лемму (та совпадёт с одной из
// веток выше буквально, дальше рекурсия не идёт - см. bareNameDeclPos,
// definition.go, тот же приём). false - name не опознано вообще (например,
// однобуквенный параметр типа generic-шаблона - тип он есть, но собственного
// объявления/записи о нём в документе нет).
func (d *doc) classifyBareName(name string, line int) (tokenType, mods int, ok bool) {
	if scope := d.scopeAt(line); scope != nil {
		for _, p := range scope.params {
			if p.Name == name {
				return stParameter, 0, true
			}
		}
		if _, ok := localsAt(scope, line)[name]; ok {
			return stVariable, 0, true
		}
	}
	if _, ok := d.funcs[name]; ok {
		return stFunction, 0, true
	}
	if _, ok := d.hoverExternFunc(name); ok {
		return stFunction, 0, true
	}
	if _, ok := d.hoverExternEntity(name); ok {
		return stType, 0, true
	}
	if _, ok := d.entities[name]; ok {
		return stType, 0, true
	}
	if _, ok := builtinTypeDocs[name]; ok {
		return stType, 0, true
	}
	if _, _, ok := builtinStubParts(name); ok {
		return stFunction, 0, true
	}
	if _, ok := d.hoverEnumType(name); ok {
		return stType, 0, true
	}
	if _, ok := d.astSynonyms[name]; ok {
		return stType, 0, true
	}
	if _, ok := d.hoverModuleConst(name); ok {
		return stVariable, modReadonly, true
	}
	if scope := d.scopeAt(line); scope != nil {
		if lemma, ok := d.localWordform(scope, line, name); ok {
			return d.classifyBareName(lemma, line)
		}
	}
	if lemma, ok := d.funcGerund(name); ok {
		return d.classifyBareName(lemma, line)
	}
	if lemma, ok := d.entityWordform(name); ok {
		return d.classifyBareName(lemma, line)
	}
	return 0, 0, false
}

// classifyMember - вид имени name после "receiver." (container - тип
// receiver, уже разрешённый resolveChain) - тот же перебор, что и у
// hoverMember (hover.go), но результат - вид, не готовый текст.
func (d *doc) classifyMember(container ir.Type, name string) (tokenType, mods int, ok bool) {
	if container.Kind != ir.KindEntity {
		return 0, 0, false
	}
	ent := d.entities[container.Entity]
	if ent == nil {
		if _, ok := hoverGenericCollectionMember(container.Entity, name); ok {
			return stFunction, 0, true
		}
		return 0, 0, false
	}
	if ent.IsSemaphore && name == "захватить" {
		return stFunction, 0, true
	}
	if ent.IsMutex || ent.IsSemaphore {
		if _, ok := mutexMethodDocs[name]; ok {
			return stFunction, 0, true
		}
	}
	if ent.IsTransaction {
		if _, ok := transactionMethodDocs[name]; ok {
			return stFunction, 0, true
		}
	}
	for _, f := range ent.Fields {
		if f.Name == name {
			return stProperty, 0, true
		}
	}
	for _, c := range ent.Constants {
		if c.Name == name {
			return stProperty, modReadonly, true
		}
	}
	for _, m := range ent.Methods {
		if m.Name == name {
			return stFunction, 0, true
		}
	}
	return 0, 0, false
}

// encodeSemanticTokens - []semToken (документный порядок - см.
// semanticTokens, d.tokens уже отсортирован лексером по строке/столбцу) в
// плоский массив LSP (пятёрки int, каждая относительно ПРЕДЫДУЩЕГО токена -
// deltaLine/deltaStartChar/length/tokenType/tokenModifiers, сам формат -
// LSP 3.16, "SemanticTokens.data") - 0-индексные строка/столбец (в отличие
// от 1-индексных semToken.Line/Col, как и весь остальной пакет).
func encodeSemanticTokens(tokens []semToken) []int {
	data := make([]int, 0, len(tokens)*5)
	prevLine, prevCol := 0, 0
	for _, t := range tokens {
		line, col := t.Line-1, t.Col-1
		deltaLine := line - prevLine
		deltaCol := col
		if deltaLine == 0 {
			deltaCol = col - prevCol
		}
		data = append(data, deltaLine, deltaCol, t.Len, t.Type, t.Mods)
		prevLine, prevCol = line, col
	}
	return data
}
