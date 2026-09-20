// Package frontend превращает исходный текст на Rech в AST (лексер +
// парсер) - вынесен из internal/compiler отдельным пакетом специально
// затем, чтобы потребителям, которым нужен только разбор (LSP, internal/
// testrunner), не приходилось импортировать internal/compiler целиком и
// тем самым транзитивно тянуть internal/backend/cbackend (кодогенерация в
// C, реально нужна только "rech build/run") - тип Frontend/NewFrontend
// раньше жили в одном файле и одном пакете с internal/backend/cbackend-
// зависимым Backend, так что Go всё равно компилировал cbackend в состав
// любого бинарника, который хотя бы упоминал internal/compiler, даже если
// звал только NewFrontend. internal/compiler переэкспортирует Frontend/
// NewFrontend отсюда (type-alias) ради обратной совместимости остальных
// вызывающих сторон (cmd/rech, internal/debugadapter).
package frontend

import (
	"fmt"

	"github.com/IlyaFeoktistov/Rech/internal/ast"
	"github.com/IlyaFeoktistov/Rech/internal/diagnostics"
	"github.com/IlyaFeoktistov/Rech/internal/lexer"
	"github.com/IlyaFeoktistov/Rech/internal/parser"
)

// Frontend превращает исходный текст на Rech в AST (лексер + парсер).
type Frontend interface {
	Parse(src string) (*ast.Program, error)
}

type defaultFrontend struct{}

// NewFrontend создаёт стандартный Frontend v0.1 (lexer.New + parser.New).
func NewFrontend() Frontend { return defaultFrontend{} }

// Parse разбирает src в AST. Синтаксическая ошибка внутри ОДНОГО
// верхнеуровневого объявления не обрывает весь разбор (parser.
// ParseProgramLenient, тот же механизм восстановления, что раньше
// использовался только LSP - см. internal/parser/recovery.go): все
// независимые ошибки собираются в internal/diagnostics.Bag за один
// прогон, а не только первая. Частичный *ast.Program при ЛЮБОЙ ошибке
// (одной или нескольких) намеренно отбрасывается (возвращается nil) -
// сборка/кодогенерация не должны увидеть программу со сломанными
// объявлениями внутри.
func (defaultFrontend) Parse(src string) (*ast.Program, error) {
	if err := lexer.ValidateUTF8(src); err != nil {
		return nil, err
	}
	if err := lexer.ValidateNFC(src); err != nil {
		return nil, err
	}
	tokens, err := lexer.New(src).Tokenize()
	if err != nil {
		return nil, fmt.Errorf("лексер: %w", err)
	}
	prog, errs := parser.New(tokens).ParseProgramLenient()
	if len(errs) > 0 {
		bag := diagnostics.NewBag()
		for _, e := range errs {
			bag.AddErr(e)
		}
		return nil, bag.AsError()
	}
	return prog, nil
}
