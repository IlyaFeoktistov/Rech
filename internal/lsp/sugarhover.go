package lsp

import (
	"strings"

	"github.com/IlyaFeoktistov/Rech/internal/ir"
	"github.com/IlyaFeoktistov/Rech/internal/semantic"
)

// hoverSugarMethod - hover на методе встроенного типа-хендла
// ("бд.выполнить", "файл.закрыть", "сокет.отправить", "x.не_число()") -
// метод это сахар над свободной встроенной функцией (semantic.SugarMethods,
// получатель - её первый параметр): показывается её описание, а сигнатура -
// в виде метода, без параметра-получателя ("БД.выполнить(sql: Строка,
// ...): Целое").
func (d *doc) hoverSugarMethod(recv ir.Type, method string) (string, bool) {
	fn, ok := semantic.SugarMethods(recv)[method]
	if !ok {
		return "", false
	}
	sig, doc, ok := builtinFuncHoverParts(fn)
	if !ok {
		return "", false
	}
	block := codeBlock(d.dispType(recv) + "." + method + "(" + dropFirstParam(sig) + ")" + resultPart(sig))
	if doc != "" {
		block += "\n\n" + doc
	}
	return withThrows(block, describeBuiltinThrows(fn)), true
}

// sigParams/resultPart - содержимое круглых скобок сигнатуры
// "имя(параметры): Результат" и хвост после закрывающей скобки.
func sigParams(sig string) string {
	open := strings.IndexByte(sig, '(')
	end := matchingParen(sig, open)
	if open < 0 || end < 0 {
		return ""
	}
	return sig[open+1 : end]
}

func resultPart(sig string) string {
	end := matchingParen(sig, strings.IndexByte(sig, '('))
	if end < 0 {
		return ""
	}
	return sig[end+1:]
}

// dropFirstParam - параметры сигнатуры без первого (получателя метода).
func dropFirstParam(sig string) string {
	params := sigParams(sig)
	depth := 0
	for i, r := range params {
		switch r {
		case '(', '<', '[':
			depth++
		case ')', '>', ']':
			depth--
		case ',':
			if depth == 0 {
				return strings.TrimSpace(params[i+1:])
			}
		}
	}
	return ""
}

// matchingParen - индекс ")", закрывающей "(" на позиции open, -1 - нет.
func matchingParen(s string, open int) int {
	if open < 0 {
		return -1
	}
	depth := 0
	for i := open; i < len(s); i++ {
		switch s[i] {
		case '(':
			depth++
		case ')':
			depth--
			if depth == 0 {
				return i
			}
		}
	}
	return -1
}
