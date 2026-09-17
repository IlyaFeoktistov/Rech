// "Перейти к определению" на встроенной функции (internal/builtins.All/
// ReservedFuncNames) - она реализована в Go, не в исходниках Rech, поэтому
// настоящего места объявления нет; вместо ничего (как раньше) - синтетический
// URI ("rech-builtin:///<имя>.rech") с текстом-заглушкой ("внешняя функция
// <имя>(...)" - та же конструкция синтаксиса, что и у настоящих внешних
// FFI-объявлений, здесь только для отображения, не для разбора) плюс её
// описание тем же текстом, что и hoverBuiltinFunc (builtindocs.go). Клиент
// (rech-vscode/extension.js) получает содержимое отдельным запросом
// "rech/builtinSource" (см. server.go, handleBuiltinSource) - обычный
// textDocument/definition несёт только Location (uri+range), не текст
// файла.
package lsp

import (
	"strings"

	"github.com/IlyaFeoktistov/Rech/internal/builtins"
)

const (
	builtinStubScheme = "rech-builtin"
	builtinStubSuffix = ".rech"
	builtinStubDecl   = "внешняя функция "
)

// builtinDefinitionPos - definitionPos на синтетический URI встроенной
// функции name - "", false, если name не встроенная функция (тот же охват,
// что и у hoverBuiltinFunc - internal/builtins.ByName/reservedFuncSignatures).
func builtinDefinitionPos(name string) (definitionPos, bool) {
	if _, _, ok := builtinStubParts(name); !ok {
		return definitionPos{}, false
	}
	return definitionPos{
		URI:  builtinStubURI(name),
		Line: 1,
		Col:  len([]rune(builtinStubDecl)) + 1,
		Len:  len([]rune(name)),
	}, true
}

// builtinStubURI/builtinStubName - взаимно обратные: URI синтетического
// документа встроенной функции <-> её имя.
func builtinStubURI(name string) string {
	return builtinStubScheme + ":///" + name + builtinStubSuffix
}

func builtinStubName(uri string) (string, bool) {
	prefix := builtinStubScheme + ":///"
	if !strings.HasPrefix(uri, prefix) || !strings.HasSuffix(uri, builtinStubSuffix) {
		return "", false
	}
	name := strings.TrimSuffix(strings.TrimPrefix(uri, prefix), builtinStubSuffix)
	if name == "" {
		return "", false
	}
	return name, true
}

// builtinStubSource - текст синтетического документа встроенной функции
// name - "", false, если name не встроенная функция. Объявление - ВСЕГДА
// строка 1 (имя начинается сразу после builtinStubDecl, столбец
// builtinDefinitionPos посчитан от той же константы) - описание идёт
// ПОСЛЕ (не перед, как у обычного doc-комментария "/** ... */" в реальных
// объявлениях Rech) специально, чтобы позиция имени не зависела от длины
// описания.
func builtinStubSource(name string) (string, bool) {
	sig, doc, ok := builtinStubParts(name)
	if !ok {
		return "", false
	}
	content := builtinStubDecl + sig + "\n"
	if doc != "" {
		content += "\n// " + doc + "\n"
	}
	return content, true
}

// builtinStubParts - сигнатура (без описания) и описание встроенной
// функции name, тем же охватом и тем же текстом, что и у hoverBuiltinFunc
// (builtindocs.go) - разложены на два отдельных поля, а не готовый
// markdown-блок, потому что синтетический документ - обычный текст Rech
// (для подсветки синтаксиса редактором), не markdown.
func builtinStubParts(name string) (sig, doc string, ok bool) {
	if f, ok := builtins.ByName(name); ok {
		return builtinFuncSignature(f), builtinFuncShortDoc[name], true
	}
	if full, ok := reservedFuncSignatures[name]; ok {
		if sig, doc, found := strings.Cut(full, " — "); found {
			return sig, doc, true
		}
		return full, "", true
	}
	return "", "", false
}
