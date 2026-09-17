package lsp

import (
	"strings"

	"github.com/IlyaFeoktistov/Rech/internal/ir"
	"github.com/IlyaFeoktistov/Rech/internal/lexer"
)

// CompletionItem - один элемент списка автодополнения - Kind использует
// числовые значения LSP CompletionItemKind напрямую (2 - Method, 3 -
// Function, 5 - Field, 6 - Variable, 7 - Class, 14 - Keyword), не заводит
// собственный enum поверх них.
type CompletionItem struct {
	Label         string `json:"label"`
	Kind          int    `json:"kind"`
	Detail        string `json:"detail,omitempty"`
	Documentation string `json:"documentation,omitempty"`
}

const (
	kindMethod   = 2
	kindFunction = 3
	kindField    = 5
	kindVariable = 6
	kindClass    = 7
	kindKeyword  = 14
	kindFile     = 17
)

// keywords - фиксированный список ключевых слов для общего автодополнения
// (не после точки) - ровно то, что уже значимо для лексера/парсера
// (internal/lexer/token.go, keywords), без падежных синонимов модификаторов
// видимости/"новый" (каждый вариант отдельно не несёт смысла для
// автодополнения - достаточно словарной формы).
var keywords = []string{
	"функция", "вывести", "показать", "переменная", "и", "или", "не",
	"истина", "ложь", "если", "иначе", "пока", "для", "от", "до", "вернуть",
	"сущность", "класс", "структура", "имеет", "новый", "этот", "состояние",
	"становится", "утверждать", "приватный", "публичный", "защищённый",
	"использовать", "из", "как", "внешняя", "константа", "перечисление",
	"интерфейс", "наследует", "реализует", "конструктор", "родитель",
	"попытаться", "поймать", "выбросить", "относится", "запустить", "дождаться",
}

// completion строит список автодополнения для позиции (line, col, оба
// 1-индексные) - member-завершение (после точки), если непосредственно
// перед col стоит токен DOT, иначе общее (локальные переменные + функции/
// сущности верхнего уровня + ключевые слова).
func (d *doc) completion(line, col int) []CompletionItem {
	// Путь импорта - до/независимо от d.irProg: "использовать ... из
	// "<курсор>"" по определению ещё не резолвится (путь не набран/не
	// существует), пока пользователь печатает его - ждать успешного
	// семантического анализа здесь было бы неверно (см. importcompletion.go).
	if items, ok := d.importPathCompletion(line, col); ok {
		return items
	}
	if d.irProg == nil {
		return nil
	}
	if dotIdx := d.dotBefore(line, col); dotIdx >= 0 {
		return d.memberCompletion(dotIdx, line)
	}
	return d.generalCompletion(line)
}

// dotBefore - индекс токена DOT, непосредственно предшествующего позиции
// (line, col) в d.tokens, -1 - если его нет (общее автодополнение).
func (d *doc) dotBefore(line, col int) int {
	for i, tok := range d.tokens {
		if tok.Type == lexer.DOT && tok.Line == line && tok.Col+1 == col {
			return i
		}
	}
	return -1
}

// memberCompletion - поля/константы/методы сущности, на которую резолвится
// цепочка ДО токена DOT с индексом dotIdx (см. dotBefore).
func (d *doc) memberCompletion(dotIdx, line int) []CompletionItem {
	chain := d.chainBefore(dotIdx - 1)
	if len(chain) == 0 {
		return nil
	}
	typ, ok := d.resolveChain(chain, line)
	if !ok || typ.Kind != ir.KindEntity {
		return nil
	}
	ent := d.entities[typ.Entity]
	if ent == nil {
		return nil
	}
	var out []CompletionItem
	for _, f := range ent.Fields {
		out = append(out, CompletionItem{Label: f.Name, Kind: kindField, Detail: d.dispType(f.Type), Documentation: strings.TrimSpace(f.Doc)})
	}
	for _, c := range ent.Constants {
		out = append(out, CompletionItem{Label: c.Name, Kind: kindField, Detail: d.dispType(c.Type) + " (константа)", Documentation: strings.TrimSpace(c.Doc)})
	}
	for _, m := range ent.Methods {
		out = append(out, CompletionItem{Label: m.Name, Kind: kindMethod, Detail: d.functionSignature(m.Name, m.Params, m.HasReturn, m.ReturnType), Documentation: strings.TrimSpace(m.Doc)})
	}
	return out
}

// generalCompletion - локальные переменные/параметры текущей функции +
// функции/сущности верхнего уровня + ключевые слова. Функции/сущности - из
// d.funcs/d.entities (не d.irProg.Functions/.Entities напрямую), отфильтрованы
// по d.localNames - при многофайловом обогащении (multifile.go) эти карты
// несут ЕЩЁ и внутренние смангленные ключи сущностей/функций из ДРУГИХ
// файлов (нужны resolveChain/implementation.go) - без фильтра они бы тоже
// попали в список автодополнения под именем, которое пользователь в
// принципе не может написать сам (см. package doc display.go).
func (d *doc) generalCompletion(line int) []CompletionItem {
	var out []CompletionItem
	scope := d.scopeAt(line)
	if scope != nil {
		if scope.entityName != "" {
			out = append(out, CompletionItem{Label: "этот", Kind: kindVariable, Detail: scope.entityName})
		}
		for name, t := range localsAt(scope, line) {
			out = append(out, CompletionItem{Label: name, Kind: kindVariable, Detail: d.dispType(t)})
		}
	}
	for label, fn := range d.funcs {
		if !d.localNames[label] {
			continue
		}
		out = append(out, CompletionItem{Label: label, Kind: kindFunction, Detail: d.functionSignature(label, fn.Params, fn.HasReturn, fn.ReturnType), Documentation: strings.TrimSpace(fn.Doc)})
	}
	for label, ent := range d.entities {
		if !d.localNames[label] {
			continue
		}
		out = append(out, CompletionItem{Label: label, Kind: kindClass, Documentation: strings.TrimSpace(ent.Doc)})
	}
	for _, kw := range keywords {
		out = append(out, CompletionItem{Label: kw, Kind: kindKeyword})
	}
	return out
}
