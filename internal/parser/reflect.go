package parser

import (
	"fmt"

	"github.com/IlyaFeoktistov/Rech/internal/ast"
	"github.com/IlyaFeoktistov/Rech/internal/lexer"
)

// parseTypeOfExpr разбирает "тип <Имя>" - дескриптор типа Имя, известного
// уже во время компиляции (аналог typeof(T)). "тип" не согласуется по
// роду/числу (в отличие от "новый"/"новая"/... у конструктора) - слово
// "тип" среднего рода и неизменно в этой позиции, дополнительная проверка
// не нужна.
func (p *Parser) parseTypeOfExpr() (ast.Expr, error) {
	kw, err := p.expect(lexer.KwType)
	if err != nil {
		return nil, err
	}
	typeName, err := p.parseTypeName()
	if err != nil {
		return nil, fmt.Errorf("после %q ожидалось имя типа: %w", kw.Value, err)
	}
	return &ast.TypeOfExpr{TypeName: typeName, Line: kw.Line}, nil
}

// parseAsExpr разбирает хвост "как <Имя>" уже после того, как left
// (боксированное значение) разобран - та же не цепочная позиция, что и у
// "относится к" (parseIsExpr), тем же принципом.
func (p *Parser) parseAsExpr(left ast.Expr) (ast.Expr, error) {
	kw := p.advance() // "как"
	typeName, err := p.parseTypeName()
	if err != nil {
		return nil, fmt.Errorf("после \"как\" ожидалось имя типа: %w", err)
	}
	return &ast.AsExpr{Value: left, TypeName: typeName, Line: kw.Line}, nil
}
