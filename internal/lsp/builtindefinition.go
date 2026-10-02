// "Перейти к определению" на встроенной функции (internal/builtins.All/
// ReservedFuncNames) ИЛИ имени встроенного/зарезервированного ТИПА
// (примитив/непрозрачный дескриптор/generic-коллекция/сущность из
// internal/builtins.ReservedEntityNames, см. builtinTypeDefinitionPos ниже) -
// оба реализованы в Go, не в исходниках Rech, поэтому настоящего места
// объявления нет; вместо ничего (как раньше) - синтетический URI
// ("rech-builtin:///<имя>.rech") с текстом-заглушкой ("внешняя функция
// <имя>(...)"/"внешняя сущность <имя>"/"внешняя структура <имя> {...}" -
// та же конструкция синтаксиса, что и у настоящих внешних FFI-объявлений,
// здесь только для отображения, не для разбора) плюс её описание тем же
// текстом, что и hoverBuiltinFunc/hoverBareName (builtindocs.go). Клиент
// (rech-vscode/extension.js) получает содержимое отдельным запросом
// "rech/builtinSource" (см. server.go, handleBuiltinSource) - обычный
// textDocument/definition несёт только Location (uri+range), не текст
// файла.
package lsp

import (
	"net/url"
	"strings"

	"github.com/IlyaFeoktistov/Rech/internal/builtins"
)

const (
	builtinStubScheme     = "rech-builtin"
	builtinStubSuffix     = ".rech"
	builtinStubDecl       = "внешняя функция "
	builtinTypeEntityDecl = "внешняя сущность "
	builtinTypeStructDecl = "внешняя структура "
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

// builtinStubName percent-декодирует name из URI (net/url, тот же приём,
// что и uriToPath - multifile.go) - имена встроенных функций/типов Rech
// почти все кириллические, а vscode.Uri.toString() percent-encode'ит
// не-ASCII сегменты пути так же, как это делает у "file://" (см. package
// doc multifile.go) - НАЙДЕННЫЙ БАГ: наивный TrimPrefix/TrimSuffix
// оставлял "name" в percent-encoded виде ("%D0%BA%D0%BE%D1%80%D0%B5%D0%BD%D1%8C"
// вместо "корень"), builtinStubParts/builtinTypeStubDecl не находили такое
// имя ни в одной карте - клиент открывал синтетический документ пустым,
// хотя сервер (Go unit-тесты И hand-crafted JSON-RPC с ASCII-именем)
// выглядел полностью корректным - разрыв был именно в этом декодировании,
// не проверявшемся ни одним из двух способов.
func builtinStubName(uri string) (string, bool) {
	prefix := builtinStubScheme + ":///"
	if !strings.HasPrefix(uri, prefix) || !strings.HasSuffix(uri, builtinStubSuffix) {
		return "", false
	}
	name := strings.TrimSuffix(strings.TrimPrefix(uri, prefix), builtinStubSuffix)
	if name == "" {
		return "", false
	}
	if decoded, err := url.PathUnescape(name); err == nil {
		name = decoded
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

// builtinTypeDefinitionPos - как builtinDefinitionPos, но для ИМЕНИ ТИПА,
// не функции: примитив/непрозрачный дескриптор/generic-коллекция/
// зарезервированная сущность (internal/builtins.ReservedEntityNames) -
// охват builtinTypeStubDecl, тот же, что и у ветки builtinTypeDocs/
// reservedEntityCtorSignature в hoverBareName (hover.go/builtindocs.go).
// НАЙДЕННЫЙ БАГ (репортнуто пользователем - "не для всех сущностей переход
// работает"): у этой ветки вообще не было аналога builtinDefinitionPos -
// definition() проверял только d.astEntities (объявления из реального
// исходника Rech), а зарезервированные сущности регистрирует сам
// компилятор синтетически (internal/semantic/analyzer.go, лениво или
// сразу для фиксированного списка) - ast.EntityDecl для них не
// существует вовсе, ни при каких условиях, "перейти к определению" молча
// ничего не делал.
func builtinTypeDefinitionPos(name string) (definitionPos, bool) {
	decl, ok := builtinTypeStubDecl(name)
	if !ok {
		return definitionPos{}, false
	}
	return definitionPos{
		URI:  builtinStubURI(name),
		Line: 1,
		Col:  len([]rune(decl)) + 1,
		Len:  len([]rune(name)),
	}, true
}

// builtinTypeStubDecl - заголовок синтетического документа имени типа name:
// "внешняя структура " (известны поля - reservedEntityFields,
// builtindocs.go, та же форма, что и настоящая "внешняя структура <ИмяC>
// {имеет: ...}", internal/parser/externtype.go), "внешняя сущность "
// (непрозрачный дескриптор/примитив/generic-коллекция - только описание,
// без полей, builtinTypeDocs) - "", false, если name не тип вовсе.
func builtinTypeStubDecl(name string) (string, bool) {
	if _, ok := reservedEntityFields[name]; ok {
		return builtinTypeStructDecl, true
	}
	if _, ok := builtinTypeDocs[name]; ok {
		return builtinTypeEntityDecl, true
	}
	return "", false
}

// builtinTypeStubSource - текст синтетического документа имени типа name,
// тем же принципом позиции имени/описания-после, что и у builtinStubSource
// выше - у сущности с известными полями (reservedEntityFields) заголовок
// несёт настоящий блок "{ имеет: ... }", у остальных - голое имя типа.
func builtinTypeStubSource(name string) (string, bool) {
	decl, ok := builtinTypeStubDecl(name)
	if !ok {
		return "", false
	}
	doc := builtinTypeDocs[name]
	content := decl + name
	if fields, ok := reservedEntityFields[name]; ok {
		content += "\n{\n    имеет:\n"
		for _, f := range fields {
			content += "    " + f.Type.String() + " " + f.Name + "\n"
		}
		content += "}\n"
	} else {
		content += "\n"
	}
	if doc != "" {
		content += "\n// " + doc + "\n"
	}
	return content, true
}
