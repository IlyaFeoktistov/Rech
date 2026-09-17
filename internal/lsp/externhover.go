// Hover для "внешняя функция"/"внешняя сущность" (FFI, docs/SPEC.md, раздел
// 12) - до этой фичи наведение на имя, под которым внешнее объявление
// видно остальному коду ("как <Псевдоним>", если он написан, иначе само
// CName), не показывало НИЧЕГО: ExternFuncDecl - отдельный от обычных
// функций список (ast.Program.ExternFuncs/ir.Program.ExternFuncs, не
// попадает в d.funcs), а ExternEntityDecl (непрозрачный указатель) вообще
// не получает ir.Entity (см. internal/semantic/analyzer.go, цикл по
// prog.ExternEntities - только entitySig{IsExternOpaque: true} для
// проверки типов, без собственного представления в выходном IR), значит и
// не попадает в d.entities. Типы параметров/результата берутся ПРЯМО из AST (ast.Param.
// Type - уже готовая строка, как её написал пользователь), не через ir.Type -
// сигнатура должна показывать РОВНО то, что написал пользователь в Rech-коде,
// а не что-либо, вычисленное заново.
//
// "внешняя структура" (в отличие от обеих веток выше) - НЕ отдельный AST
// тип, а обычный *ast.EntityDecl с IsExternStruct == true (parser.go,
// parseExternStructDecl) - проходит весь обычный путь entity- hover без
// доработок, отдельная ветка ей не нужна.
package lsp

import (
	"fmt"
	"strings"

	"github.com/IlyaFeoktistov/Rech/internal/ast"
)

// hoverExternFunc - name уже разрешено как ОБЫЧНОЕ голое имя (см.
// hoverBareName) - ищет среди ExternFuncDecl текущего документа тот, чей
// Alias (или CName, если "как" не было - парсер уже проставляет Alias =
// CName в этом случае, см. parseExternFuncDecl) совпадает.
func (d *doc) hoverExternFunc(name string) (string, bool) {
	if d.astProg == nil {
		return "", false
	}
	for _, ef := range d.astProg.ExternFuncs {
		if ef.Alias != name {
			continue
		}
		header := "внешняя функция " + externFuncSignature(ef.Alias, ef.Params, ef.ReturnType)
		if ef.Alias != ef.CName {
			header += fmt.Sprintf("\n\nC-имя: `%s`", ef.CName)
		}
		return withDoc(codeBlock(header), ef.Doc), true
	}
	return "", false
}

// hoverExternEntity - как hoverExternFunc, но для непрозрачного внешнего
// типа ("внешняя сущность <ИмяC> [как <Псевдоним>]") - у него нет ни
// параметров, ни полей, ни методов (см. package doc).
func (d *doc) hoverExternEntity(name string) (string, bool) {
	if d.astProg == nil {
		return "", false
	}
	for _, ee := range d.astProg.ExternEntities {
		if ee.Alias != name {
			continue
		}
		header := "внешняя сущность " + ee.Alias
		if ee.Alias != ee.CName {
			header += fmt.Sprintf("\n\nC-имя: `%s`", ee.CName)
		}
		return withDoc(codeBlock(header), ee.Doc), true
	}
	return "", false
}

// externFuncSignature - "имя(параметр: Тип, ...): ТипВозврата" - параметры/
// тип результата ПРЯМО из AST (уже строки, как написал пользователь), в
// отличие от d.functionSignature (hover.go), которая форматирует уже
// разрешённые ir.Type обычных функций.
func externFuncSignature(name string, params []ast.Param, returnType string) string {
	parts := make([]string, len(params))
	for i, p := range params {
		parts[i] = p.Name + ": " + p.Type
	}
	sig := fmt.Sprintf("%s(%s)", name, strings.Join(parts, ", "))
	if returnType != "" {
		sig += ": " + returnType
	}
	return sig
}
