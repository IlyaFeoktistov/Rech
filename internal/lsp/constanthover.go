// Hover для константы МОДУЛЬНОГО уровня (d.astConsts, см. document.go/
// multifile.go) - раньше не поддержан вовсе, тем же классом пробела, что и
// у перечислений до enumhover.go: имя константы - не сущность/функция
// (hoverBareName о нём не знало ничего), поэтому doc-комментарий над
// "константа ИМЯ = значение" не показывался вообще - найдено по прямой
// просьбе пользователя ("не работает докблок на константах которые на
// уровне модуля").
package lsp

import (
	"fmt"

	"github.com/IlyaFeoktistov/Rech/internal/ast"
)

// hoverModuleConst - "константа ИМЯ = значение" - курсор на самом имени
// константы модульного уровня (не на константе сущности - та резолвится
// через ir.Entity.Constants, см. hoverMember/literalText, hover.go: модульная
// константа в IR не сохраняется, значение читается прямо из AST).
func (d *doc) hoverModuleConst(name string) (string, bool) {
	c, ok := d.astConsts[name]
	if !ok {
		return "", false
	}
	block := codeBlock(fmt.Sprintf("константа %s", c.Name))
	if lit, ok := literalTextAST(c.Value); ok {
		block = codeBlock(fmt.Sprintf("константа %s = %s", c.Name, lit))
	}
	return withDoc(block, c.Doc), true
}

// literalTextAST - как literalText (hover.go), но для ast.Expr, не
// ir.Expr - модульная константа не резолвится семантикой ни во что
// сохраняемое (в отличие от константы сущности, ir.Entity.Constants),
// поэтому у hoverModuleConst нет типизированного ir.Expr, только исходное
// AST-выражение сразу после парсера. Тот же принцип - "", false для
// любого не-литерала (арифметика, обращение к другой константе, вызов
// функции), не пытаясь заново вычислить произвольное выражение.
func literalTextAST(v ast.Expr) (string, bool) {
	switch v := v.(type) {
	case *ast.IntLiteral:
		return fmt.Sprintf("%d", v.Value), true
	case *ast.FloatLiteral:
		return fmt.Sprintf("%g", v.Value), true
	case *ast.StringLiteral:
		return fmt.Sprintf("%q", v.Value), true
	case *ast.BoolLiteral:
		if v.Value {
			return "истина", true
		}
		return "ложь", true
	case *ast.CharLiteral:
		return "'" + string(v.Value) + "'", true
	default:
		return "", false
	}
}
