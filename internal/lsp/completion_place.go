package lsp

import (
	"strings"

	"github.com/IlyaFeoktistov/Rech/internal/builtins"
	"github.com/IlyaFeoktistov/Rech/internal/ir"
	"github.com/IlyaFeoktistov/Rech/internal/lexer"
)

// Место курсора для автодополнения без "получатель."/"Тип::"/"имя:" -
// от него зависит, что вообще уместно предлагать: при объявлении метода в
// теле сущности не нужны переменные, после "функция "/"Целое " набирается
// новое имя - предлагать нечего, внутри выражения не нужны "если"/"пока".

type completionPlace int

const (
	placeNone       completionPlace = iota // набирается новое имя
	placeTopLevel                          // начало строки на верхнем уровне файла
	placeMemberDecl                        // начало строки в теле сущности вне методов
	placeStatement                         // начало инструкции в теле функции/лямбды/блока
	placeExpression                        // внутри выражения
	placeNew                               // после "новый"/"новая"/"новое"/"новые"
	placeAfterBrace                        // "} <курсор>" - продолжение "если"/"попытаться"
)

// nameIntroducers - после этих слов набирается имя нового объявления.
var nameIntroducers = map[lexer.TokenType]bool{
	lexer.KwFunction: true, lexer.KwEntity: true, lexer.KwClass: true, lexer.KwStruct: true,
	lexer.KwInterface: true, lexer.KwEnum: true, lexer.KwConst: true, lexer.KwVariable: true,
	lexer.KwSynonym: true, lexer.KwFor: true, lexer.KwAs: true,
}

// placeAt - место курсора (line, col); prefix - значимые токены строки до
// набираемого слова, blocks - окружающие блоки (enclosingBlocks).
func (d *doc) placeAt(prefix []lexer.Token, blocks []textBlock) completionPlace {
	inner := blockTop
	if len(blocks) > 0 {
		inner = blocks[len(blocks)-1].kind
	}
	onlyModifiers := true
	for _, t := range prefix {
		if !declModifiers[t.Type] {
			onlyModifiers = false
		}
	}
	if onlyModifiers {
		switch inner {
		case blockTop:
			return placeTopLevel
		case blockEntity:
			return placeMemberDecl
		case blockInterface, blockEnum:
			return placeNone
		}
		if len(prefix) == 0 {
			return placeStatement
		}
	}
	last := prefix[len(prefix)-1]
	switch {
	case len(prefix) == 1 && last.Type == lexer.RBRACE:
		return placeAfterBrace
	case nameIntroducers[last.Type]:
		return placeNone
	case last.Type == lexer.KwNew:
		return placeNew
	case last.Type == lexer.LPAREN && len(prefix) >= 2 && prefix[len(prefix)-2].Type == lexer.KwCatch:
		return placeNone
	case d.isParamNamePosition(prefix):
		return placeNone
	case inner != blockTop && inner != blockEntity && d.isTypeOnlyPrefix(prefix):
		// "Целое <курсор>" - объявление "тип первым", дальше новое имя.
		return placeNone
	}
	return placeExpression
}

// isParamNamePosition - курсор на месте имени параметра в заголовке
// объявления или лямбды: "функция имя(", "функция(" или ", " внутри них.
func (d *doc) isParamNamePosition(prefix []lexer.Token) bool {
	last := prefix[len(prefix)-1].Type
	if last != lexer.LPAREN && last != lexer.COMMA {
		return false
	}
	depth := 0
	for i := len(prefix) - 1; i >= 0; i-- {
		switch prefix[i].Type {
		case lexer.RPAREN:
			depth++
		case lexer.LPAREN:
			if depth > 0 {
				depth--
				continue
			}
			// Открывающая скобка списка, в котором курсор: заголовок
			// функции/лямбды, если перед ней "функция [имя]" или имя метода
			// в начале строки.
			j := i - 1
			if j >= 0 && prefix[j].Type == lexer.IDENT {
				j--
			}
			if j >= 0 && (prefix[j].Type == lexer.KwFunction || prefix[j].Type == lexer.KwConstructor) {
				return true
			}
			// "имя(" без "функция" - вызов (заголовок метода без
			// модификатора от вызова по одной строке не отличить).
			return false
		}
	}
	return false
}

// isTypeOnlyPrefix - весь prefix - запись типа ("Целое", "?Точка",
// "Список<Точка>"): после неё в начале инструкции набирается имя.
func (d *doc) isTypeOnlyPrefix(prefix []lexer.Token) bool {
	i := 0
	if prefix[i].Type == lexer.QUESTION {
		i++
	}
	if i >= len(prefix) || prefix[i].Type != lexer.IDENT || !d.isTypeName(prefix[i].Value) {
		return false
	}
	i++
	if i == len(prefix) {
		return true
	}
	if prefix[i].Type != lexer.LT {
		return false
	}
	depth := 0
	for ; i < len(prefix); i++ {
		switch prefix[i].Type {
		case lexer.LT:
			depth++
		case lexer.GT:
			depth--
		case lexer.SHR:
			depth -= 2
		}
	}
	return depth == 0
}

// isTypeName - name - имя типа, видимое в документе.
func (d *doc) isTypeName(name string) bool {
	for _, n := range builtinTypeNames() {
		if n == name {
			return true
		}
	}
	_, ent := d.entities[name]
	_, iface := d.astInterfaces[name]
	_, enum := d.astEnums[name]
	_, syn := d.astSynonyms[name]
	return ent || iface || enum || syn
}

// Ключевые слова по местам.
var (
	topLevelKeywords = []string{
		"функция", "асинхронная", "сущность", "класс", "структура", "абстрактная", "абстрактный",
		"интерфейс", "перечисление", "константа", "синоним", "использовать", "внешняя", "экспортировать",
		"публичная", "публичный", "публичное", "приватная", "приватный", "приватное",
		"внутренняя", "внутренний", "внутреннее",
	}
	memberDeclKeywords = []string{
		"имеет", "конструктор", "константа", "состояние", "оператор", "экспортировать",
		"публичный", "публичная", "публичное", "публичные", "приватный", "приватная", "приватное", "приватные",
		"защищённый", "защищённая", "защищённое", "защищённые", "статический", "статическая", "статическое",
		"абстрактный", "асинхронный",
	}
	statementKeywords = []string{
		"если", "пока", "для", "вернуть", "вывести", "показать", "переменная", "константа", "статическая",
		"утверждать", "попытаться", "выбросить", "выбрать", "прервать", "продолжить", "дождаться",
		"поручить", "запустить",
	}
	expressionKeywords = []string{
		"истина", "ложь", "Ничто", "не", "и", "или", "новый", "новая", "новое", "новые", "функция",
		"дождаться", "запустить", "поручить", "разобрать", "сериализовать", "тип", "как", "в",
	}
	afterBraceKeywords = []string{"иначе", "поймать", "окончательно"}
)

func keywordList(words []string) []CompletionItem {
	out := make([]CompletionItem, len(words))
	for i, w := range words {
		out[i] = CompletionItem{Label: w, Kind: kindKeyword}
	}
	return out
}

// placeCompletion - подсказки для места place.
func (d *doc) placeCompletion(place completionPlace, line int, blocks []textBlock) []CompletionItem {
	c := newItemSet()
	switch place {
	case placeNone:
		return nil
	case placeAfterBrace:
		c.addAll(keywordList(afterBraceKeywords))
	case placeNew:
		d.addInstantiableTypes(c)
	case placeMemberDecl:
		c.addAll(keywordList(memberDeclKeywords))
		d.addOverridableMethods(c, blocks)
	case placeTopLevel:
		c.addAll(keywordList(topLevelKeywords))
		if !d.hasExplicitMain() {
			// Инструкции верхнего уровня - тело неявной "главная".
			d.addValues(c, line, blocks)
			d.addTypes(c)
			c.addAll(keywordList(statementKeywords))
		}
	case placeStatement:
		d.addValues(c, line, blocks)
		d.addTypes(c)
		c.addAll(keywordList(statementKeywords))
		if len(blocks) > 0 && blocks[len(blocks)-1].kind == blockSwitch {
			c.addAll(keywordList([]string{"когда", "иначе"}))
		}
		if d.inMethod(blocks) {
			c.addAll(keywordList([]string{"этот", "родитель"}))
		}
	case placeExpression:
		d.addValues(c, line, blocks)
		d.addTypes(c)
		c.addAll(keywordList(expressionKeywords))
		if d.inMethod(blocks) {
			c.addAll(keywordList([]string{"этот", "родитель"}))
		}
	}
	return c.items
}

// itemSet - элементы без повторов по Label (первый побеждает).
type itemSet struct {
	items []CompletionItem
	seen  map[string]bool
}

func newItemSet() *itemSet { return &itemSet{seen: map[string]bool{}} }

func (c *itemSet) add(it CompletionItem) {
	if !c.seen[it.Label] {
		c.seen[it.Label] = true
		c.items = append(c.items, it)
	}
}

func (c *itemSet) addAll(items []CompletionItem) {
	for _, it := range items {
		c.add(it)
	}
}

// funcBlock - ближайший окружающий блок функции/метода (не лямбды), nil -
// курсор не в теле функции.
func funcBlock(blocks []textBlock) *textBlock {
	for i := len(blocks) - 1; i >= 0; i-- {
		if blocks[i].kind == blockFunc {
			return &blocks[i]
		}
	}
	return nil
}

// scopeForBlocks - разобранная функция/метод, в теле которой курсор: по
// строке заголовка окружающего блока функции; nil - курсор не в функции.
func (d *doc) scopeForBlocks(blocks []textBlock, line int) *funcScope {
	fb := funcBlock(blocks)
	if fb == nil {
		if !d.hasExplicitMain() && len(blocks) == 0 {
			return d.implicitMainScope()
		}
		return nil
	}
	for i := range d.scopes {
		if d.scopes[i].declLine == fb.headerLine {
			return &d.scopes[i]
		}
	}
	return d.scopeAt(line)
}

// inMethod - курсор в теле метода или конструктора сущности.
func (d *doc) inMethod(blocks []textBlock) bool {
	for i := len(blocks) - 1; i >= 0; i-- {
		if blocks[i].kind == blockFunc {
			return i > 0 && blocks[i-1].kind == blockEntity
		}
	}
	return false
}

// hasExplicitMain - в документе объявлена "функция главная" (тогда
// инструкции на верхнем уровне недопустимы). У неявной "главная" из
// инструкций верхнего уровня строки объявления нет (Line == 0).
func (d *doc) hasExplicitMain() bool {
	fn, ok := d.astFuncs["главная"]
	return ok && fn.Line > 0
}

// implicitMainScope - неявная "главная" из инструкций верхнего уровня.
func (d *doc) implicitMainScope() *funcScope {
	for i := range d.scopes {
		if d.scopes[i].name == "главная" && d.scopes[i].entityName == "" {
			return &d.scopes[i]
		}
	}
	return nil
}

// addValues - всё, что можно написать голым именем как значение: локальные
// и параметры (в том числе параметры и переменные окружающих лямбд),
// функции, константы, перечисления, сущности (для "Тип::"), встроенные
// функции.
func (d *doc) addValues(c *itemSet, line int, blocks []textBlock) {
	for name, t := range d.visibleLocals(blocks, line) {
		detail := ""
		if t.Kind != ir.KindUnknown {
			detail = d.dispType(t)
		}
		c.add(CompletionItem{Label: name, Kind: kindVariable, Detail: detail})
	}
	for label, fn := range d.funcs {
		if d.localNames[label] && label != "главная" {
			c.add(CompletionItem{Label: label, Kind: kindFunction, Detail: d.functionSignature(label, fn.Params, fn.HasReturn, fn.ReturnType), Documentation: strings.TrimSpace(fn.Doc)})
		}
	}
	for label := range d.astConsts {
		c.add(CompletionItem{Label: label, Kind: kindConstant})
	}
	for label, v := range d.astVars {
		c.add(CompletionItem{Label: label, Kind: kindVariable, Documentation: strings.TrimSpace(v.Doc)})
	}
	for label := range d.astEnums {
		c.add(CompletionItem{Label: label, Kind: kindEnum})
	}
	for _, f := range builtins.All {
		c.add(CompletionItem{Label: f.Name, Kind: kindFunction, Detail: builtinFuncSignature(f), Documentation: builtinFuncShortDoc[f.Name]})
	}
	for name := range builtins.ReservedFuncNames {
		c.add(CompletionItem{Label: name, Kind: kindFunction, Detail: builtinFuncDetail(name)})
	}
}

// addTypes - типы: встроенные и объявленные в документе.
func (d *doc) addTypes(c *itemSet) {
	c.addAll(d.typeCompletion())
}

// addInstantiableTypes - после "новый": сущности (не абстрактные) и
// встроенные коллекции/зарезервированные сущности - то, что создаётся.
func (d *doc) addInstantiableTypes(c *itemSet) {
	for label, ent := range d.entities {
		if !d.localNames[label] || ent.IsAbstract {
			continue
		}
		kind := kindClass
		if ent.IsStruct {
			kind = kindStruct
		}
		c.add(CompletionItem{Label: label, Kind: kind, Documentation: strings.TrimSpace(ent.Doc)})
	}
	for _, name := range genericTemplateNames {
		c.add(CompletionItem{Label: name, Kind: kindClass, Documentation: builtinTypeDocs[name]})
	}
	for name := range builtins.ReservedEntityNames {
		c.add(CompletionItem{Label: name, Kind: kindClass, Documentation: builtinTypeDocs[name]})
	}
}

// addOverridableMethods - в теле сущности: методы базовых сущностей и
// реализуемых интерфейсов, которые эта сущность ещё не объявила.
func (d *doc) addOverridableMethods(c *itemSet, blocks []textBlock) {
	name := entityNameOf(blocks[len(blocks)-1].header)
	ent, ok := d.entities[name]
	if !ok {
		return
	}
	own := map[string]bool{}
	if decl, ok := d.astEntities[name]; ok {
		for _, m := range decl.Methods {
			own[m.Name] = true
		}
	}
	for baseName := ent.BaseName; baseName != ""; {
		base, ok := d.entities[baseName]
		if !ok {
			break
		}
		for _, m := range base.Methods {
			if !own[m.Name] {
				c.add(CompletionItem{Label: m.Name, Kind: kindMethod, Detail: d.functionSignature(m.Name, m.Params, m.HasReturn, m.ReturnType) + " (" + baseName + ")", Documentation: strings.TrimSpace(m.Doc)})
			}
		}
		baseName = base.BaseName
	}
	for _, ifaceName := range ent.Interfaces {
		d.addInterfaceMethodsToImplement(c, ifaceName, own, map[string]bool{})
	}
}

func (d *doc) addInterfaceMethodsToImplement(c *itemSet, ifaceName string, own, seen map[string]bool) {
	iface, ok := d.astInterfaces[ifaceName]
	if !ok || seen[ifaceName] {
		return
	}
	seen[ifaceName] = true
	for _, m := range iface.Methods {
		if !own[m.Name] {
			c.add(CompletionItem{Label: m.Name, Kind: kindMethod, Detail: interfaceMethodSignature(m) + " (" + ifaceName + ")", Documentation: strings.TrimSpace(m.Doc)})
		}
	}
	for _, parent := range iface.Extends {
		d.addInterfaceMethodsToImplement(c, parent, own, seen)
	}
}

// entityNameOf - имя сущности из заголовка "сущность Имя ...".
func entityNameOf(header []lexer.Token) string {
	for i, t := range header {
		if (t.Type == lexer.KwEntity || t.Type == lexer.KwClass || t.Type == lexer.KwStruct) && i+1 < len(header) {
			return header[i+1].Value
		}
	}
	return ""
}

// visibleLocals - локальные имена в точке курсора: параметры и переменные
// функции/метода (до строки line), параметры окружающих лямбд (из их
// заголовков) и переменные, объявленные в телах этих лямбд. Тип параметра
// лямбды, который не разрешился по имени, - KindUnknown.
func (d *doc) visibleLocals(blocks []textBlock, line int) map[string]ir.Type {
	scope := d.scopeForBlocks(blocks, line)
	out := localsAt(scope, line)
	if scope == nil {
		return out
	}
	lambdas := collectLambdas(scope.body)
	for _, b := range blocks {
		if b.kind != blockLambda {
			continue
		}
		params := lambdaParams(b.header)
		names := make([]string, len(params))
		for i, p := range params {
			names[i] = p[0]
			t, _ := d.typeByName(p[1])
			out[p[0]] = t
		}
		if lam := matchLambda(lambdas, names); lam != nil {
			walkLocals(lam.Body, line, func(name string, t ir.Type, _ *ir.VarDecl, _ int) { out[name] = t })
		}
	}
	return out
}

// collectLambdas - все лямбды тела (на любой глубине вложенности).
func collectLambdas(stmts []ir.Stmt) []*ir.LambdaExpr {
	var out []*ir.LambdaExpr
	var visitExpr func(e ir.Expr)
	var visitStmts func(ss []ir.Stmt)
	visitExpr = func(e ir.Expr) {
		if e == nil {
			return
		}
		if lam, ok := e.(*ir.LambdaExpr); ok {
			out = append(out, lam)
			visitStmts(lam.Body)
		}
		for _, child := range ir.ChildExprs(e) {
			visitExpr(child)
		}
	}
	visitStmts = func(ss []ir.Stmt) {
		for _, s := range ss {
			for _, e := range ir.ChildExprs(s) {
				visitExpr(e)
			}
			for _, body := range ir.ChildBlocks(s) {
				visitStmts(body)
			}
		}
	}
	visitStmts(stmts)
	return out
}

// matchLambda - лямбда с ровно такими именами параметров.
func matchLambda(lambdas []*ir.LambdaExpr, names []string) *ir.LambdaExpr {
	for _, lam := range lambdas {
		if len(lam.Params) != len(names) {
			continue
		}
		same := true
		for i, p := range lam.Params {
			if p.Name != names[i] {
				same = false
			}
		}
		if same {
			return lam
		}
	}
	return nil
}
