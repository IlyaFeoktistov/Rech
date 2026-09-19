package module

import (
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"sort"
	"strings"
)

// SourceExtensions - все допустимые расширения исходного файла Rech.
// ".rech" - основное (латиница), ".речь"/".р" - необязательные
// кириллические синонимы (полное слово и однобуквенное сокращение) -
// узнаются везде, где узнаётся ".rech" (компилятор, LSP, "rech fmt"), но
// сам компилятор всегда пишет/предлагает только ".rech". Не относится к
// ".rechproj"/".rechi" - у них синонимов нет.
var SourceExtensions = []string{".rech", ".речь", ".р"}

// HasSourceExtension - true, если у path одно из SourceExtensions.
func HasSourceExtension(path string) bool {
	return slices.Contains(SourceExtensions, filepath.Ext(path))
}

// TrimSourceExtension убирает расширение из path, если это одно из
// SourceExtensions - иначе возвращает path без изменений.
func TrimSourceExtension(path string) string {
	ext := filepath.Ext(path)
	for _, e := range SourceExtensions {
		if ext == e {
			return strings.TrimSuffix(path, e)
		}
	}
	return path
}

// ResolveSourceExtension - для base (путь БЕЗ расширения, уже посчитанный
// обычный резолвинг "использовать X из "путь"") ищет на диске, какое из
// SourceExtensions реально существует: ноль найдено - возвращает base с
// основным расширением ".rech" как есть (дальше по конвейеру это даст
// обычную ошибку "файл не найден"), одно найдено - его, больше одного -
// явная ошибка неоднозначности (см. package doc SourceExtensions - разные
// расширения для одного и того же пути импорта не должны молча
// приоритизироваться).
func ResolveSourceExtension(base string) (string, error) {
	var found []string
	for _, ext := range SourceExtensions {
		if _, err := os.Stat(base + ext); err == nil {
			found = append(found, ext)
		}
	}
	switch len(found) {
	case 0:
		return base + SourceExtensions[0], nil
	case 1:
		return base + found[0], nil
	default:
		return "", fmt.Errorf("%s: неоднозначность - одновременно существуют файлы с расширениями %s", base, strings.Join(found, ", "))
	}
}

// CheckNoAmbiguousExtensions группирует names (обычно - имена файлов,
// найденных обычным листингом каталога, а не по одному заранее известному
// пути, как у ResolveSourceExtension) по базовому имени без расширения и
// возвращает ошибку, если у какой-то базы больше одного файла с разными
// SourceExtensions одновременно ("главный.rech" и "главный.речь" в одном
// каталоге) - тем же принципом неоднозначности, что и при резолвинге
// "использовать", но по уже готовому списку имён, а не по stat() диска.
func CheckNoAmbiguousExtensions(names []string) error {
	byBase := map[string][]string{}
	for _, name := range names {
		if !HasSourceExtension(name) {
			continue
		}
		base := TrimSourceExtension(name)
		byBase[base] = append(byBase[base], name)
	}
	bases := make([]string, 0, len(byBase))
	for base := range byBase {
		bases = append(bases, base)
	}
	sort.Strings(bases)
	for _, base := range bases {
		if group := byBase[base]; len(group) > 1 {
			sort.Strings(group)
			return fmt.Errorf("%s: неоднозначность - одновременно существуют файлы %s", base, strings.Join(group, ", "))
		}
	}
	return nil
}
