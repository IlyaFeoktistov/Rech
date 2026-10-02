package lsp

import (
	"sort"
	"strings"

	"github.com/IlyaFeoktistov/Rech/internal/ast"
	"github.com/IlyaFeoktistov/Rech/internal/builtins"
	"github.com/IlyaFeoktistov/Rech/internal/ir"
	"github.com/IlyaFeoktistov/Rech/internal/lexer"
	"github.com/IlyaFeoktistov/Rech/internal/semantic"
)

// CompletionItem - один элемент списка автодополнения - Kind использует
// числовые значения LSP CompletionItemKind напрямую, не заводит
// собственный enum поверх них.
type CompletionItem struct {
	Label         string `json:"label"`
	Kind          int    `json:"kind"`
	Detail        string `json:"detail,omitempty"`
	Documentation string `json:"documentation,omitempty"`
}

const (
	kindMethod     = 2
	kindFunction   = 3
	kindField      = 5
	kindVariable   = 6
	kindClass      = 7
	kindInterface  = 8
	kindEnum       = 13
	kindKeyword    = 14
	kindFile       = 17
	kindEnumMember = 20
	kindConstant   = 21
	kindStruct     = 22
)

// genericTemplateNames - встроенные generic-шаблоны типов: их имена пишут
// с аргументами ("Список<Целое>"), самих инстанциаций в списке нет.
var genericTemplateNames = []string{"Список", "Стопка", "Очередь", "Словарь", "Множество", "Указатель"}

// completion строит список автодополнения для позиции (line, col, оба
// 1-индексные): после "получатель." - его члены, после "Тип::" -
// статические члены и константы типа, иначе - всё видимое в этом месте
// (локальные, свои и встроенные функции/типы, ключевые слова). Контекст и
// области видимости - см. completion_context.go.
func (d *doc) completion(line, col int) []CompletionItem {
	// Путь импорта - до/независимо от анализа: "использовать ... из
	// "<курсор>"" по определению ещё не резолвится, пока путь набирается
	// (см. importcompletion.go).
	if items, ok := d.importPathCompletion(line, col); ok {
		return items
	}
	ctx := d.completionContextAt(line, col)
	ad := d.completionDoc(line)
	if ad.irProg == nil {
		if ctx.kind == completionGeneral {
			return keywordItems()
		}
		return nil
	}
	blocks := enclosingBlocks(d.text, line, col)
	switch ctx.kind {
	case completionMember:
		return ad.memberCompletion(ctx.chain, line, blocks)
	case completionStatic:
		return ad.staticCompletion(ctx.chain[0].name)
	case completionType:
		return ad.typeCompletion()
	}
	return ad.placeCompletion(ad.placeAt(ctx.prefix, blocks), line, blocks)
}

// memberCompletion - члены того, во что разрешилась цепочка получателя.
func (d *doc) memberCompletion(chain []chainElem, line int, blocks []textBlock) []CompletionItem {
	target, ok := d.resolveCompletionChain(chain, line, blocks)
	if !ok {
		return nil
	}
	switch {
	case target.isEnum:
		var out []CompletionItem
		for _, m := range d.astEnums[target.typeName].Members {
			out = append(out, CompletionItem{Label: m.Name, Kind: kindEnumMember, Detail: target.typeName})
		}
		return out
	case target.typeName != "":
		// "Тип.ИМЯ" - константы сущности.
		var out []CompletionItem
		if ent := d.entities[target.typeName]; ent != nil {
			for _, c := range ent.Constants {
				out = append(out, d.constantItem(c))
			}
		}
		return out
	}
	viewer := ""
	if scope := d.scopeForBlocks(blocks, line); scope != nil {
		viewer = scope.entityName
	}
	return d.valueMembers(target.typ, viewer)
}

// valueMembers - члены значения типа t, видимые из кода сущности viewer
// ("" - из свободной функции): поля, константы и методы вместе с
// унаследованными (ближайшее объявление побеждает), методы встроенных
// сущностей, коллекций, интерфейсов и встроенных типов-хендлов.
func (d *doc) valueMembers(t ir.Type, viewer string) []CompletionItem {
	if t.Kind == ir.KindInterface {
		return d.interfaceMembers(t.Entity)
	}
	if t.Kind != ir.KindEntity {
		return d.sugarMembers(t)
	}
	ent := d.entities[t.Entity]
	if ent == nil {
		if items := d.interfaceMembers(t.Entity); items != nil {
			return items
		}
		items, _ := genericCollectionCompletion(t.Entity)
		return items
	}
	var out []CompletionItem
	seen := map[string]bool{}
	add := func(it CompletionItem) {
		if !seen[it.Label] {
			seen[it.Label] = true
			out = append(out, it)
		}
	}
	for cur := ent; cur != nil; cur = d.entities[cur.BaseName] {
		for _, f := range cur.Fields {
			if owner, vis := d.memberDecl(cur.Name, f.Name, fieldVisibility); d.memberVisible(vis, owner, viewer) {
				add(CompletionItem{Label: f.Name, Kind: kindField, Detail: d.dispType(f.Type), Documentation: strings.TrimSpace(f.Doc)})
			}
		}
		for _, c := range cur.Constants {
			if owner, vis := d.memberDecl(cur.Name, c.Name, constVisibility); d.memberVisible(vis, owner, viewer) {
				add(d.constantItem(c))
			}
		}
		for _, m := range cur.Methods {
			if m.IsStatic {
				continue
			}
			if owner, vis := d.memberDecl(cur.Name, m.Name, methodVisibility); d.memberVisible(vis, owner, viewer) {
				add(CompletionItem{Label: m.Name, Kind: kindMethod, Detail: d.functionSignature(m.Name, m.Params, m.HasReturn, m.ReturnType), Documentation: strings.TrimSpace(m.Doc)})
			}
		}
	}
	switch {
	case ent.IsSemaphore:
		add(CompletionItem{Label: "захватить", Kind: kindMethod, Detail: semaphoreAcquireDoc})
		add(CompletionItem{Label: "освободить", Kind: kindMethod, Detail: mutexMethodDocs["освободить"]})
	case ent.IsMutex:
		for name, doc := range mutexMethodDocs {
			add(CompletionItem{Label: name, Kind: kindMethod, Detail: doc})
		}
	case ent.IsTransaction:
		for name, doc := range transactionMethodDocs {
			add(CompletionItem{Label: name, Kind: kindMethod, Detail: doc})
		}
	case ent.Name == "ДатаВремя":
		for _, name := range semantic.DateTimeMethodNames() {
			add(CompletionItem{Label: name, Kind: kindMethod})
		}
	}
	return out
}

// memberVisible - член с видимостью vis, объявленный в owner, доступен из
// кода сущности viewer: приватный - только в самой owner, защищённый - ещё
// и в наследниках.
func (d *doc) memberVisible(vis ast.Visibility, owner, viewer string) bool {
	switch vis {
	case ast.Private:
		return viewer == owner
	case ast.Protected:
		for cur := d.entities[viewer]; cur != nil; cur = d.entities[cur.BaseName] {
			if cur.Name == owner {
				return true
			}
		}
		return false
	}
	return true
}

// memberDecl - сущность, объявившая член name (сама entity или ближайший
// предок, у которого он есть в исходнике), и его видимость. IR наследника
// несёт унаследованные поля как свои - видимость берётся у объявления.
// Нет объявления в исходнике (встроенная сущность) - публичный, владелец
// entity.
func (d *doc) memberDecl(entity, name string, lookup func(*ast.EntityDecl, string) (ast.Visibility, bool)) (string, ast.Visibility) {
	for cur := d.entities[entity]; cur != nil; cur = d.entities[cur.BaseName] {
		if vis, ok := lookup(d.astEntities[cur.Name], name); ok {
			return cur.Name, vis
		}
	}
	return entity, ast.Public
}

func fieldVisibility(decl *ast.EntityDecl, name string) (ast.Visibility, bool) {
	if decl != nil {
		for _, f := range decl.Fields {
			if f.Name == name {
				return f.Visibility, true
			}
		}
	}
	return ast.Public, false
}

func constVisibility(decl *ast.EntityDecl, name string) (ast.Visibility, bool) {
	if decl != nil {
		for _, c := range decl.Constants {
			if c.Name == name {
				return c.Visibility, true
			}
		}
	}
	return ast.Public, false
}

func methodVisibility(decl *ast.EntityDecl, name string) (ast.Visibility, bool) {
	if decl != nil {
		for _, m := range decl.Methods {
			if m.Name == name {
				return m.Visibility, true
			}
		}
	}
	return ast.Public, false
}

func (d *doc) constantItem(c ir.Constant) CompletionItem {
	return CompletionItem{Label: c.Name, Kind: kindConstant, Detail: d.dispType(c.Type) + " (константа)", Documentation: strings.TrimSpace(c.Doc)}
}

// interfaceMembers - методы интерфейса name; nil - такого интерфейса нет.
func (d *doc) interfaceMembers(name string) []CompletionItem {
	for _, iface := range d.irProg.Interfaces {
		if iface.Name != name {
			continue
		}
		out := []CompletionItem{}
		for _, m := range iface.Methods {
			out = append(out, CompletionItem{Label: m.Name, Kind: kindMethod, Detail: d.functionSignature(m.Name, m.Params, m.HasReturn, m.ReturnType)})
		}
		return out
	}
	return nil
}

// sugarMembers - методы встроенного типа-хендла (Файл/Сокет/БД/...,
// числа): каждый сводится к свободной встроенной функции, её сигнатура -
// в Detail.
func (d *doc) sugarMembers(t ir.Type) []CompletionItem {
	var out []CompletionItem
	for method, fn := range semantic.SugarMethods(t) {
		out = append(out, CompletionItem{Label: method, Kind: kindMethod, Detail: builtinFuncDetail(fn)})
	}
	sortItems(out)
	return out
}

// staticCompletion - "Тип::": константы числового примитива, статические
// поля/методы и константы сущности.
func (d *doc) staticCompletion(typeName string) []CompletionItem {
	var out []CompletionItem
	for _, c := range semantic.PrimitiveTypeConstantNames(typeName) {
		out = append(out, CompletionItem{Label: c, Kind: kindConstant, Detail: typeName})
	}
	ent := d.entities[typeName]
	if ent == nil {
		return out
	}
	for _, f := range ent.StaticFields {
		out = append(out, CompletionItem{Label: f.Name, Kind: kindField, Detail: d.dispType(f.Type), Documentation: strings.TrimSpace(f.Doc)})
	}
	for _, m := range ent.Methods {
		if m.IsStatic {
			out = append(out, CompletionItem{Label: m.Name, Kind: kindMethod, Detail: d.functionSignature(m.Name, m.Params, m.HasReturn, m.ReturnType), Documentation: strings.TrimSpace(m.Doc)})
		}
	}
	for _, c := range ent.Constants {
		out = append(out, d.constantItem(c))
	}
	return out
}

// typeCompletion - после "имя:" (параметр, поле, тип результата): только
// типы - встроенные и объявленные в документе.
func (d *doc) typeCompletion() []CompletionItem {
	var out []CompletionItem
	for _, name := range builtinTypeNames() {
		out = append(out, CompletionItem{Label: name, Kind: kindClass, Documentation: builtinTypeDocs[name]})
	}
	for label, ent := range d.entities {
		if !d.localNames[label] {
			continue
		}
		kind := kindClass
		if ent.IsStruct {
			kind = kindStruct
		}
		out = append(out, CompletionItem{Label: label, Kind: kind, Documentation: strings.TrimSpace(ent.Doc)})
	}
	for label := range d.astInterfaces {
		if d.localNames[label] {
			out = append(out, CompletionItem{Label: label, Kind: kindInterface})
		}
	}
	for label := range d.astEnums {
		out = append(out, CompletionItem{Label: label, Kind: kindEnum})
	}
	for label := range d.astSynonyms {
		out = append(out, CompletionItem{Label: label, Kind: kindClass, Detail: "синоним"})
	}
	return out
}

// builtinTypeNames - встроенные типы: примитивы, generic-шаблоны,
// "Исключение" и зарезервированные сущности.
func builtinTypeNames() []string {
	names := append(semantic.PrimitiveTypeNames(), genericTemplateNames...)
	names = append(names, "Исключение")
	for name := range builtins.ReservedEntityNames {
		names = append(names, name)
	}
	return names
}

// builtinFuncDetail - сигнатура встроенной функции name для Detail.
func builtinFuncDetail(name string) string {
	if f, ok := builtins.ByName(name); ok {
		return builtinFuncSignature(f)
	}
	sig, _, _ := strings.Cut(reservedFuncSignatures[name], " — ")
	return sig
}

func keywordItems() []CompletionItem {
	words := lexer.KeywordWords()
	out := make([]CompletionItem, len(words))
	for i, w := range words {
		out[i] = CompletionItem{Label: w, Kind: kindKeyword}
	}
	return out
}

func sortItems(items []CompletionItem) {
	sort.Slice(items, func(i, j int) bool { return items[i].Label < items[j].Label })
}
