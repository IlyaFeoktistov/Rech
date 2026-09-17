// "N переопределений" над методом (абстрактным или обычным) сущности,
// которая (транзитивно, через "наследует") имеет хотя бы одного потомка,
// переопределяющего этот метод - CodeLens (codelens.go), тот же принцип,
// что и "N ссылок" (references.go), только связь другая: не "кто
// ВЫЗЫВАЕТ", а "кто ПЕРЕОПРЕДЕЛЯЕТ". В языке нет ключевого слова
// "переопределить" (docs/SPEC.md, "Абстрактные методы и сущности") -
// переопределение узнаётся ТОЛЬКО по совпадению имени метода у сущности,
// стоящей ниже по цепочке "наследует", тем же механизмом диспетчеризации
// (vtable-слот), что и у обычного наследования/интерфейсов.
//
// Потомок может быть объявлен в файле, который открытый документ НЕ
// импортирует вовсе (обычное дело - базовая абстрактная сущность и её
// конкретные реализации нередко живут в разных файлах, а импорт идёт в
// ОБРАТНУЮ сторону - "круг.rech" импортирует "фигура.rech", не наоборот) -
// тот же принцип, что и у crossFileRefs (references.go): listRechFiles+
// fileImportsTarget находят кандидатов по всему дереву проекта, каждый
// читается и РАЗБИРАЕТСЯ заново (лексер+парсер, без семантики - тот же
// приём, что и у declaresName, workspace_scan.go). "наследует <ИмяВ
// винительном падеже>" в СВЕЖЕРАЗОБРАННОМ файле ещё не лемматизировано
// семантическим анализом (в отличие от d.astEntities текущего документа,
// куда сущности попадают уже ПОСЛЕ успешной линковки/анализа) - сравнение
// идёт словоформой (d.wordformMatchesLemma), не буквальным именем.
package lsp

import (
	"os"
	"path/filepath"

	"github.com/IlyaFeoktistov/Rech/internal/ast"
	"github.com/IlyaFeoktistov/Rech/internal/lexer"
	"github.com/IlyaFeoktistov/Rech/internal/parser"
)

// entityCandidates - каждая сущность, которую overrideRefs вправе
// рассмотреть как возможного потомка: собственные объявления открытого
// документа И всех файлов, что forward-линковка уже подтянула
// (d.astEntities), плюс сущности любых ДРУГИХ файлов дерева проекта,
// реально импортирующих d.path (см. package doc выше) - те, у кого
// forward-линковка от d.path не дотягивается, потому что импорт идёт в
// обратную сторону.
func (d *doc) entityCandidates() []*ast.EntityDecl {
	out := make([]*ast.EntityDecl, 0, len(d.astEntities))
	seenFile := map[string]bool{}
	for _, ed := range d.astEntities {
		out = append(out, ed)
		if ed.File != "" {
			seenFile[ed.File] = true
		}
	}
	if d.path == "" {
		return out
	}
	for _, file := range listRechFiles(scanRoot(filepath.Dir(d.path)), d.path) {
		if seenFile[file] || !fileImportsTarget(file, d.path) {
			continue
		}
		prog, ok := parseForeignProgram(file)
		if !ok {
			continue
		}
		out = append(out, prog.Entities...)
	}
	return out
}

// parseForeignProgram читает и разбирает path заново (лексер+парсер, БЕЗ
// семантического анализа - только структура объявлений нужна, тем же
// принципом, что и declaresName, workspace_scan.go) - "", false при любой
// ошибке (файл удалён/содержит синтаксическую ошибку - тогда он просто не
// участвует в поиске потомков, не валит весь ответ).
func parseForeignProgram(path string) (*ast.Program, bool) {
	src, err := os.ReadFile(path)
	if err != nil {
		return nil, false
	}
	toks, err := lexer.New(string(src)).Tokenize()
	if err != nil {
		return nil, false
	}
	prog, err := parser.New(toks).ParseProgram()
	if err != nil {
		return nil, false
	}
	ast.StampFile(prog, path) // ed.File иначе остался бы "" - isCurrentFile/foreignTokens ниже не смогли бы отличить эту сущность от собственной документа
	return prog, true
}

// overrideRefs - позиция ПЕРЕОПРЕДЕЛЕНИЯ метода methodName у каждого
// потомка entityName (см. package doc выше) - того же вида (refLocation),
// что и у crossFileRefs (references.go), чтобы CodeLens мог показать её
// тем же "editor.action.showReferences" (server.go, handleCodeLens), не
// заводя вторую клиентскую команду.
func (d *doc) overrideRefs(entityName, methodName string) []refLocation {
	candidates := d.entityCandidates()
	seen := map[string]bool{}
	var out []refLocation
	var walk func(base string)
	walk = func(base string) {
		for _, ed := range candidates {
			if ed.BaseName == "" || seen[ed.Name] {
				continue
			}
			if !d.wordformMatchesLemma(ed.BaseName, base) {
				continue
			}
			seen[ed.Name] = true
			for _, m := range ed.Methods {
				if m.Name != methodName {
					continue
				}
				toks := d.tokens
				file := ""
				if !d.isCurrentFile(ed.File) {
					var ok bool
					toks, ok = d.foreignTokens(ed.File)
					if !ok {
						break
					}
					file = ed.File
				}
				if tok, ok := tokenAt(toks, m.Line, methodName); ok {
					out = append(out, refLocation{File: file, Tok: tok})
				}
				break
			}
			walk(ed.Name)
		}
	}
	walk(entityName)
	return out
}

// overrideCountLabel - число переопределений, согласованное со словом
// "переопределение" (1 переопределение, 2 переопределения, 5
// переопределений, ...) - то же правило русских числительных, что и у
// refCountLabel, через общий ruCountLabel (references.go).
func overrideCountLabel(n int) string {
	return ruCountLabel(n, "переопределение", "переопределения", "переопределений")
}
