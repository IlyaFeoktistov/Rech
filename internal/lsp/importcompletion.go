// Автодополнение строки-пути импорта - "использовать <имя>, ... из
// "<курсор>"" - список настоящих ".rech" файлов проекта (тот же обход, что
// у автоимпорта, workspace_scan.go), каждый как путь ОТНОСИТЕЛЬНО каталога
// текущего документа, без ".rech" - готовая форма для вставки, никогда
// "@"-путь (см. workspace_scan.go про то, почему).
package lsp

import (
	"path/filepath"

	"github.com/IlyaFeoktistov/Rech/internal/lexer"
)

// importPathCompletion - false, если курсор НЕ внутри строкового литерала
// пути импорта - иначе список файлов, каким бы ни был d.irProg (в отличие
// от остального completion.go, здесь МОЖЕТ не быть валидной программы
// вовсе - путь ещё печатается).
func (d *doc) importPathCompletion(line, col int) ([]CompletionItem, bool) {
	idx := d.stringTokenIndexAt(line, col)
	if idx <= 0 || d.tokens[idx-1].Type != lexer.KwOf {
		return nil, false
	}
	if d.path == "" {
		// Не "file://" URI (в частности, тестовые "test://..." по всему
		// пакету) - нет настоящего каталога на диске, откуда сканировать.
		return []CompletionItem{}, true
	}
	docDir := filepath.Dir(d.path)
	root := scanRoot(docDir)
	var out []CompletionItem
	for _, f := range listRechFiles(root, d.path) {
		rel, ok := relativeImportPath(docDir, f)
		if !ok {
			continue
		}
		out = append(out, CompletionItem{Label: rel, Kind: kindFile})
	}
	return out, true
}

// stringTokenIndexAt - индекс токена STRING в d.tokens, чьё содержимое
// (МЕЖДУ кавычками, кавычки сами не входят в tok.Value) включает позицию
// (line, col) - -1, если такого нет. В отличие от tokenIndexAt (resolve.go),
// у пустой строки ("", tok.Value == "") здесь по-прежнему ЕСТЬ валидная
// позиция курсора - сразу после открывающей кавычки (col == tok.Col+1) -
// именно этот момент важен для автодополнения ("использовать X из "|"" -
// самая частая точка вызова, сразу после того, как редактор автозакрыл
// вторую кавычку).
func (d *doc) stringTokenIndexAt(line, col int) int {
	for i, tok := range d.tokens {
		if tok.Type != lexer.STRING || tok.Line != line {
			continue
		}
		width := len([]rune(tok.Value))
		if col >= tok.Col+1 && col <= tok.Col+1+width {
			return i
		}
	}
	return -1
}
