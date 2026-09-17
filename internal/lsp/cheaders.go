// Резервный текст hover для "внешняя функция"/"внешняя сущность" без
// собственного doc-комментария - ближайший комментарий, стоящий прямо
// перед объявлением того же C-имени в одном из заголовков, перечисленных в
// "сборка.заголовки"/"_windows"/"_linux"/"_macos" текущего .rechproj (см.
// internal/project.File.Заголовки). НЕ настоящий C-парсер (нет
// препроцессора, макросы/условная компиляция/несколько деклараций на одной
// строке не разбираются) - эвристика поверх сырого текста заголовка,
// которая справляется с типичным случаем "один комментарий прямо над одной
// декларацией на своей строке" (подавляющее большинство системных
// заголовков) и молча ничего не находит в остальных - never крашится,
// never блокирует остальной hover. Типы в самом hover ВСЕГДА берутся из
// Rech-объявления (externhover.go), заголовок используется ТОЛЬКО как
// источник текста описания, когда у пользователя нет своего.
package lsp

import (
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strings"
	"sync"

	"github.com/IlyaFeoktistov/Rech/internal/project"
)

var (
	includeDirsOnce  sync.Once
	includeDirsCache []string
)

// cIncludeDirs - каталоги поиска системных заголовков (`#include <...>`),
// которыми реально пользуется C-компилятор этой машины (internal/toolchain
// вызывает тот же `cc`/`gcc`/`clang` для настоящей сборки) - "cc -E
// -Wp,-v -xc -" и разбор блока между "#include <...> search starts here:"
// и "End of search list." (формат GCC/Clang, оба поддерживают этот флаг).
// Вычисляется ОДИН раз за жизнь процесса `rech lsp` - вызов компилятора не
// бесплатный, hover не должен ждать его на каждое наведение; заголовки
// подключённых системных библиотек не меняются, пока жив LSP-сервер.
func cIncludeDirs() []string {
	includeDirsOnce.Do(func() {
		includeDirsCache = discoverIncludeDirs()
	})
	return includeDirsCache
}

func discoverIncludeDirs() []string {
	for _, cc := range []string{"cc", "gcc", "clang"} {
		path, err := exec.LookPath(cc)
		if err != nil {
			continue
		}
		cmd := exec.Command(path, "-E", "-Wp,-v", "-xc", "-")
		cmd.Stdin = strings.NewReader("")
		out, _ := cmd.CombinedOutput()
		if dirs := parseIncludeDirs(string(out)); len(dirs) > 0 {
			return dirs
		}
	}
	return nil
}

// parseIncludeDirs разбирает вывод "-Wp,-v" - строки МЕЖДУ "#include <...>
// search starts here:" и "End of search list." (каждая с одним ведущим
// пробелом - сам путь каталога).
func parseIncludeDirs(output string) []string {
	var dirs []string
	inList := false
	for line := range strings.SplitSeq(output, "\n") {
		switch {
		case strings.Contains(line, "#include <...> search starts here:"):
			inList = true
		case strings.HasPrefix(line, "End of search list"):
			inList = false
		case inList:
			if dir := strings.TrimSpace(line); dir != "" {
				dirs = append(dirs, dir)
			}
		}
	}
	return dirs
}

// findHeaderFile ищет name (как написано в "сборка.заголовки" -
// "stdio.h", "curl/curl.h" и т. п.) среди cIncludeDirs() - "", false, если
// не найден ни в одном (нет подходящего компилятора в PATH, заголовок
// реально отсутствует на этой машине).
func findHeaderFile(name string) (string, bool) {
	for _, dir := range cIncludeDirs() {
		path := filepath.Join(dir, name)
		if info, err := os.Stat(path); err == nil && !info.IsDir() {
			return path, true
		}
	}
	return "", false
}

var headerCommentCache = struct {
	mu    sync.Mutex
	byKey map[string]string
}{byKey: map[string]string{}}

// headerDeclComment - комментарий перед первым найденным в headerPath
// объявлением cSymbol - requireCall true для функций (после имени
// ожидается "(" - см. extractDeclComment), false для непрозрачных типов
// (typedef/struct - имя само по себе, без вызова). Кешируется по (путь,
// имя) - заголовок не меняется, пока жив процесс, повторное чтение и
// разбор одного и того же файла на каждое наведение ничего не даёт.
func headerDeclComment(headerPath, cSymbol string, requireCall bool) (string, bool) {
	key := headerPath + "\x00" + cSymbol
	if requireCall {
		key += "\x00call"
	}
	headerCommentCache.mu.Lock()
	if v, ok := headerCommentCache.byKey[key]; ok {
		headerCommentCache.mu.Unlock()
		return v, v != ""
	}
	headerCommentCache.mu.Unlock()

	comment := ""
	if text, err := os.ReadFile(headerPath); err == nil {
		comment = extractDeclComment(string(text), cSymbol, requireCall)
	}
	headerCommentCache.mu.Lock()
	headerCommentCache.byKey[key] = comment
	headerCommentCache.mu.Unlock()
	return comment, comment != ""
}

var commentBlockRe = regexp.MustCompile(`(?s)/\*.*?\*/|(?:[ \t]*//[^\n]*\n?)+`)

// extractDeclComment - комментарий прямо перед ПЕРВЫМ вхождением cSymbol
// как отдельного слова в text (после "(", если requireCall), у которого
// такой комментарий вообще есть - "", если вхождений нет вовсе, либо ни
// перед одним из них нет комментария. Проверяет КАЖДОЕ вхождение по
// очереди, не только самое первое - настоящее объявление в заголовке редко
// оказывается первым текстовым совпадением: то же имя обычно раньше
// встречается внутри чужого макроса (см. `strlen` в glibc `string.h` -
// сначала внутри тела `strdupa`, без комментария, и только потом в
// собственном объявлении с комментарием прямо над ним) или как параметр в
// сигнатуре другой функции. "Между комментарием и cSymbol есть что-то
// похожее на конец другой декларации (';'/'{'/'}')" - комментарий
// относится не к этому вхождению, а к чему-то более раннему, пропустить.
func extractDeclComment(text, cSymbol string, requireCall bool) string {
	pattern := `\b` + regexp.QuoteMeta(cSymbol) + `\b`
	if requireCall {
		pattern += `\s*\(`
	}
	declRe, err := regexp.Compile(pattern)
	if err != nil {
		return ""
	}
	for _, loc := range declRe.FindAllStringIndex(text, -1) {
		searchStart := 0
		if idx := strings.LastIndexAny(text[:loc[0]], ";{}"); idx >= 0 {
			searchStart = idx + 1
		}
		prefix := text[searchStart:loc[0]]

		matches := commentBlockRe.FindAllStringIndex(prefix, -1)
		if len(matches) == 0 {
			continue
		}
		last := matches[len(matches)-1]
		afterComment := prefix[last[1]:]
		if strings.ContainsAny(afterComment, ";{}") {
			continue
		}
		if comment := cleanCComment(prefix[last[0]:last[1]]); comment != "" {
			return comment
		}
	}
	return ""
}

// cleanCComment убирает разметку "/* */"/"//" построчно и склеивает
// многострочный комментарий в одну строку через пробел - достаточно для
// короткого текста в hover, не сохраняет оригинальные переносы строк.
func cleanCComment(raw string) string {
	raw = strings.TrimSpace(raw)
	raw = strings.TrimSuffix(raw, "*/")
	raw = strings.TrimPrefix(raw, "/*")
	var out []string
	for line := range strings.SplitSeq(raw, "\n") {
		line = strings.TrimSpace(line)
		line = strings.TrimPrefix(line, "//")
		line = strings.TrimPrefix(line, "*")
		line = strings.TrimSpace(line)
		if line != "" {
			out = append(out, line)
		}
	}
	return strings.Join(out, " ")
}

// headerDocFor - см. package doc - "" если у документа нет настоящего
// файлового пути (не "file://" URI), нет .rechproj над ним, в нём не
// заполнено "сборка.заголовки" ни одной из вариаций, ни в одном из них не
// нашлось cSymbol с комментарием перед ним.
func (d *doc) headerDocFor(cSymbol string, requireCall bool) string {
	if d.path == "" {
		return ""
	}
	pf, err := project.LoadFor(d.path)
	if err != nil || pf == nil {
		return ""
	}
	for _, h := range pf.Заголовки() {
		path, ok := findHeaderFile(h)
		if !ok {
			continue
		}
		if comment, ok := headerDeclComment(path, cSymbol, requireCall); ok {
			return comment
		}
	}
	return ""
}
