package module

import (
	"errors"
	"os"
	"path/filepath"
	"regexp"
	"strings"

	"github.com/IlyaFeoktistov/Rech/internal/ast"
	"github.com/IlyaFeoktistov/Rech/internal/stdlib"
)

// Автоимпорт встроенных имён ("rech fmt", форматирование в редакторе):
// каждому встроенному имени, которое файл использует без импорта,
// дописывается импорт из его стандартного модуля - как goimports.

// maxStdFixes - предел имён за один вызов: связывание останавливается на
// первой ошибке, поэтому каждое имя - отдельный проход.
const maxStdFixes = 128

// FixStdImports - src (текст файла path) с недостающими импортами из
// стандартных модулей. Недостающие импорты файлов, которые path
// импортирует, дописываются только в памяти (чтобы дойти до ошибок самого
// path) - на диск попадает лишь результат для path. Ошибка - только при
// невозможности разобрать граф импортов; любая другая ошибка связывания
// просто останавливает дописывание.
//
// Код под "#если" других платформ тоже проверяется: импорт, нужный только
// на Windows, дописывается и при форматировании на Linux.
func FixStdImports(path, src string, frontend Frontend, projectRoot string) (string, error) {
	clean := filepath.Clean(path)
	overlay := map[string]string{clean: src}
	for _, platform := range ast.PlatformNames {
		if err := fixStdImportsFor(clean, overlay, frontend, projectRoot, platform); err != nil {
			return overlay[clean], err
		}
	}
	return overlay[clean], nil
}

// fixStdImportsFor - FixStdImports для одной платформы; overlay
// дополняется исправленными текстами.
func fixStdImportsFor(clean string, overlay map[string]string, frontend Frontend, projectRoot, platform string) error {
	for range maxStdFixes {
		prog, err := loadForPlatform(clean, frontend, nil, nil, projectRoot, overlay, platform)
		if err != nil {
			return err
		}
		_, err = prog.Link()
		name, ok := IsNotImportedBuiltin(err)
		if !ok {
			return nil
		}
		file := moduleOfError(prog, err)
		if file == "" {
			return nil
		}
		text, loaded := overlay[file]
		if !loaded {
			data, rerr := os.ReadFile(file)
			if rerr != nil {
				return nil
			}
			text = string(data)
		}
		fixed := insertStdImport(text, name)
		if fixed == text {
			return nil
		}
		overlay[file] = fixed
	}
	return errors.New("слишком много недостающих импортов")
}

// moduleOfError - путь модуля prog, к которому относится ошибка
// связывания err (Link начинает сообщение с пути модуля); "" - не найден.
func moduleOfError(prog *Program, err error) string {
	msg := err.Error()
	for _, m := range prog.Modules {
		if strings.HasPrefix(msg, m.Path+": ") {
			return m.Path
		}
	}
	return ""
}

// stdUseLineRe - однострочный импорт из стандартного модуля.
var stdUseLineRe = regexp.MustCompile(`^использовать (.+) из ` + regexp.QuoteMeta(stdlib.Prefix) + `(\S+)\s*$`)

// StdImportEdit - правка src, импортирующая встроенное name: name
// дописывается в строку импорта своего модуля, если она есть (replace -
// строка line заменяется на text), иначе text - новые строки, вставляемые
// перед строкой line, в конец блока импортов (или в начало файла после
// шапки-комментария, отделённой пустой строкой). line - с 1; ok=false -
// name не из стандартного модуля.
func StdImportEdit(src, name string) (line int, text string, replace, ok bool) {
	module, ok := stdlib.ModuleOf(name)
	if !ok {
		return 0, "", false, false
	}
	lines := strings.Split(src, "\n")
	for i, l := range lines {
		if m := stdUseLineRe.FindStringSubmatch(l); m != nil && m[2] == module {
			return i + 1, "использовать " + m[1] + ", " + name + " из " + stdlib.Prefix + module + "\n", true, true
		}
	}
	at, newBlock := importInsertIndex(lines)
	text = "использовать " + name + " из " + stdlib.Prefix + module + "\n"
	if newBlock && at < len(lines) && strings.TrimSpace(lines[at]) != "" {
		text += "\n"
	}
	return at + 1, text, false, true
}

// insertStdImport - src с применённой StdImportEdit.
func insertStdImport(src, name string) string {
	line, text, replace, ok := StdImportEdit(src, name)
	if !ok {
		return src
	}
	lines := strings.SplitAfter(src, "\n")
	at := line - 1
	tail := lines[at:]
	if replace {
		tail = lines[at+1:]
	}
	return strings.Join(lines[:at], "") + text + strings.Join(tail, "")
}

// importInsertIndex - индекс строки, перед которой вставляется новый
// импорт; newBlock - импортов в файле ещё нет.
func importInsertIndex(lines []string) (at int, newBlock bool) {
	last := -1
	for i, l := range lines {
		if strings.HasPrefix(l, "использовать ") {
			last = i
		}
	}
	if last >= 0 {
		// Импорт может продолжаться на следующих строках до "из".
		for last < len(lines)-1 && !strings.Contains(lines[last], " из ") {
			last++
		}
		return last + 1, false
	}
	// Шапка-комментарий, отделённая пустой строкой, остаётся сверху.
	i := 0
	switch {
	case strings.HasPrefix(lines[0], "/*"):
		for i < len(lines) && !strings.Contains(lines[i], "*/") {
			i++
		}
		i++
	case strings.HasPrefix(lines[0], "//"):
		for i < len(lines) && strings.HasPrefix(lines[i], "//") {
			i++
		}
	default:
		return 0, true
	}
	if i < len(lines) && strings.TrimSpace(lines[i]) == "" {
		return i + 1, true
	}
	return 0, true
}
