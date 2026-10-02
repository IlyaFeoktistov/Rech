package parser

import (
	"strings"

	"github.com/IlyaFeoktistov/Rech/internal/lexer"
	"github.com/IlyaFeoktistov/Rech/internal/morph"
)

// ModifierFormFor - форма согласуемого модификатора mod перед типом typeName:
// видимость и "статический" ("приватная Строка имя") - по morph.
// TypeAgreement, "новый" ("новая Точка(...)") - по имени сущности
// (newKeywordFor). Те же правила, что проверяет сам парсер, - для быстрого
// исправления в LSP. ok == false - mod не согласуется с типом или род типа
// не определить (подходит любая форма).
func ModifierFormFor(mod lexer.Token, typeName string) (string, bool) {
	if mod.Type == lexer.KwNew {
		return newKeywordFor(typeName)
	}
	gender, number, ok := morph.TypeAgreement(typeName)
	if !ok {
		return "", false
	}
	switch {
	case mod.Type == lexer.KwStatic:
		return staticKeywordFor(gender, number), true
	case isVisibilityKeyword(mod.Type), mod.Type == lexer.KwInternal:
		return visibilityKeywordFor(visibilityFromToken(mod), gender, number), true
	}
	return "", false
}

// newKeywordFor - форма "новый"/"новая"/"новое"/"новые" перед typeName
// (generic-аргументы не учитываются: "новый Список<Строка>()").
func newKeywordFor(typeName string) (string, bool) {
	base, _, _ := strings.Cut(typeName, "<")
	gender, number, ok := morph.CompoundNounAgreement(base)
	if !ok {
		return "", false
	}
	return morph.NewKeywordFor(gender, number), true
}
