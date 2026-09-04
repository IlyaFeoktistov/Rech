// Сканирование каталога проекта для автоимпорта (codeaction.go) -
// НЕ путать с multifile.go (тот разрешает граф "использовать" от уже
// открытого документа, здесь наоборот - ищем файл, который стоило бы
// импортировать, среди файлов, о которых открытый документ пока не знает
// вовсе). Разбор каждого найденного .rech - только лексер+парсер (не
// semantic.Analyze - достаточно списка объявленных имён, типы/тела не
// нужны для самого автоимпорта).
package lsp

import (
	"os"
	"path/filepath"
	"strings"

	"github.com/IlyaFeoktistov/Rech/internal/lexer"
	"github.com/IlyaFeoktistov/Rech/internal/parser"
	"github.com/IlyaFeoktistov/Rech/internal/project"
)

// workspaceCandidate - один файл, объявляющий имя, которое ищет автоимпорт.
type workspaceCandidate struct {
	// ImportPath - готовая строка для "использовать <имя> из "<ImportPath>""
	// - относительно КАТАЛОГА текущего документа (module.resolveImportPath,
	// internal/module/module.go - тот же алфавит "../", что и там), БЕЗ
	// ".rech" - никогда "@"-путь (см. package doc про то, почему).
	ImportPath string
}

// scanRoot - каталог, откуда начинается обход вниз в поисках .rech-файлов -
// ближайший каталог-предок с единственным *.rechproj (project.Find,
// internal/project - тот же поиск, что и у "rech build" без явного пути),
// не дальше maxUp уровней вверх от каталога документа; не найден - сам
// каталог документа (тогда виден только его собственный поддерево, не
// каталоги выше - разумное по умолчанию для файла без .rechproj вообще).
func scanRoot(docDir string) string {
	const maxUp = 10
	dir := docDir
	for range maxUp {
		if path, err := project.Find(dir); err == nil && path != "" {
			return dir
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			break
		}
		dir = parent
	}
	return docDir
}

// findWorkspaceDecl ищет name (сущность/функция/интерфейс верхнего уровня)
// среди .rech-файлов ПОД scanRoot(docDir), кроме самого skipPath (текущий
// документ - искать импорт для имени, уже присутствующего в нём самом,
// бессмысленно). Несколько файлов объявляют одно и то же имя - возвращает
// ВСЕ (вызывающая сторона предложит по одному варианту quick fix на
// каждый - см. codeaction.go), не гадает, какой из них имелся в виду.
func findWorkspaceDecl(docDir, docPath, name string) []workspaceCandidate {
	root := scanRoot(docDir)
	var out []workspaceCandidate
	filepath.WalkDir(root, func(path string, d os.DirEntry, err error) error {
		if err != nil {
			return nil
		}
		if d.IsDir() {
			if strings.HasPrefix(d.Name(), ".") && path != root {
				return filepath.SkipDir
			}
			return nil
		}
		if filepath.Ext(path) != ".rech" || path == docPath {
			return nil
		}
		if declaresName(path, name) {
			if rel, ok := relativeImportPath(docDir, path); ok {
				out = append(out, workspaceCandidate{ImportPath: rel})
			}
		}
		return nil
	})
	return out
}

// declaresName - true, если path (уже известно - валидный .rech, читается
// с диска заново, не из открытых документов LSP - те же файлы, что могли
// бы быть открыты в других вкладках, здесь не нужны отдельно, обычное
// приближение, как и у остального пакета) верхнеуровнево объявляет name
// как функцию/сущность/интерфейс - лексер+парсер, без семантики (см.
// package doc).
func declaresName(path, name string) bool {
	src, err := os.ReadFile(path)
	if err != nil {
		return false
	}
	toks, err := lexer.New(string(src)).Tokenize()
	if err != nil {
		return false
	}
	prog, err := parser.New(toks).ParseProgram()
	if err != nil {
		return false
	}
	for _, fn := range prog.Functions {
		if fn.Name == name {
			return true
		}
	}
	for _, ent := range prog.Entities {
		if ent.Name == name {
			return true
		}
	}
	for _, iface := range prog.Interfaces {
		if iface.Name == name {
			return true
		}
	}
	return false
}

// relativeImportPath - "использовать ... из "<результат>"" для файла target,
// написанного в файле, лежащем в каталоге fromDir - относительный путь без
// ".rech", сегменты через "/" (module.resolveImportPath ожидает именно
// такую форму - "../" сегменты как обычно у относительных путей ОС,
// строчные буквы/цифры/"_" на остальных - гарантированы самим тем, что имя
// файла/каталога уже существует и прошло проверку при создании).
func relativeImportPath(fromDir, target string) (string, bool) {
	rel, err := filepath.Rel(fromDir, target)
	if err != nil {
		return "", false
	}
	rel = strings.TrimSuffix(rel, ".rech")
	rel = filepath.ToSlash(rel)
	return rel, true
}
