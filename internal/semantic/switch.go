package semantic

import (
	"slices"

	"github.com/IlyaFeoktistov/Rech/internal/ast"
	"github.com/IlyaFeoktistov/Rech/internal/diagnostics"
	"github.com/IlyaFeoktistov/Rech/internal/ir"
)

// switchSubjectVar - имя внутренней переменной, под которую desugaring
// "выбрать" (см. analyzeSwitchStmt) заводит однократно вычисленное
// значение Subject. Не может столкнуться с именем переменной пользователя
// на C-уровне (mangling.checkBlockMangling) - каждое "выбрать" эмитит
// собственный вложенный C-блок (ir.BlockStmt), а вложенные блоки проверяются
// на коллизии в СВОЕЙ, свежей области видимости (тот же принцип, что и у
// повторного использования "результат" в нескольких деепричастных вызовах,
// package doc ast.GerundStmt) - постоянное имя без счётчика достаточно.
const switchSubjectVar = "_подлежащее_выбора"

// analyzeSwitchStmt десахарирует "выбрать <выражение> { когда ... иначе
// ... }" в цепочку "если/иначе если" ещё на уровне AST (buildSwitchIfChain)
// и анализирует её уже существующим analyzeStmt(*ast.IfStmt) - весь
// typestate/nullable-established учёт, вложенные области видимости и
// сообщения об ошибках достаются бесплатно, ни C backend, ни интерпретатор
// не знают о "выбрать" вообще (см. ir.BlockStmt). Subject вычисляется РОВНО
// один раз - под него заводится временная переменная switchSubjectVar в
// собственной области видимости (sc.push/pop), которая закрывается вместе
// со всем "выбрать".
//
// Каждое значение ветки "когда" - голое имя типа сущности/интерфейса (не
// объявленное как переменная в текущей области видимости) сопоставляется
// через "относится к" (ast.IsExpr, полиморфная проверка), любое другое
// выражение - через равенство "==" (ast.BinaryExpr). Typestate-паттерны в
// "когда" (сопоставление не только по типу, но и по установленному
// состоянию сущности) сознательно не поддержаны: established сейчас чисто
// компилятивен (нет рантайм-представления состояния) - сопоставлять по
// недоказанному в компиляции состоянию было бы нечем.
func analyzeSwitchStmt(sc *ctx, s *ast.SwitchStmt) (ir.Stmt, error) {
	sc.push()
	defer sc.pop()

	subject, err := analyzeExpr(sc, s.Subject)
	if err != nil {
		return nil, err
	}
	subject = maybeUnwrapNarrowed(sc, subject)
	if typ := ir.TypeOf(subject); typ.Kind == ir.KindNullable {
		return nil, errAtLine(s.Line, CodeSemanticOther, "нельзя напрямую выбирать по значению nullable-типа %s - разверните его сначала (\"?? запасное\" или явный каст)", typ)
	}
	if err := sc.declare(switchSubjectVar, ir.TypeOf(subject)); err != nil {
		return nil, diagnostics.WithPos(err, diagnostics.Position{File: currentFile, Line: s.Line})
	}

	ifChain := buildSwitchIfChain(sc, s.Cases, s.Default)
	chainIR, err := analyzeStmt(sc, ifChain)
	if err != nil {
		return nil, err
	}

	return &ir.BlockStmt{
		Body: []ir.Stmt{
			&ir.VarDecl{Name: switchSubjectVar, Value: subject, Line: s.Line},
			chainIR,
		},
		Line: s.Line,
	}, nil
}

// buildSwitchIfChain строит AST-цепочку "если <когда[0]> {...} иначе {если
// <когда[1]> {...} иначе {...}}" из веток "выбрать", в порядке от последней
// к первой - последний элемент цепочки получает Default как "иначе"
// (nil, если "иначе" не было - ast.IfStmt трактует это как "ветки не было",
// ровно как и написанный вручную if без else).
func buildSwitchIfChain(sc *ctx, cases []ast.SwitchCase, defaultBody []ast.Stmt) *ast.IfStmt {
	var chain *ast.IfStmt
	elseBody := defaultBody
	for _, c := range slices.Backward(cases) {
		node := &ast.IfStmt{Cond: buildSwitchCaseCond(sc, c.Values), Then: c.Body, Line: c.Line}
		if chain != nil {
			node.Else = []ast.Stmt{chain}
		} else {
			node.Else = elseBody
		}
		chain = node
	}
	return chain
}

// buildSwitchCaseCond объединяет значения одной ветки "когда" через "или" -
// см. package doc analyzeSwitchStmt про выбор между "относится к" и "==".
func buildSwitchCaseCond(sc *ctx, values []ast.Expr) ast.Expr {
	subjectRef := &ast.Identifier{Name: switchSubjectVar}
	var cond ast.Expr
	for _, v := range values {
		var check ast.Expr
		if name, ok := switchTypePatternName(sc, v); ok {
			check = &ast.IsExpr{Value: subjectRef, Target: name, Line: exprLine(v)}
		} else {
			check = &ast.BinaryExpr{Op: "==", Left: subjectRef, Right: v, Line: exprLine(v)}
		}
		if cond == nil {
			cond = check
			continue
		}
		cond = &ast.BinaryExpr{Op: "или", Left: cond, Right: check, Line: exprLine(v)}
	}
	return cond
}

// switchTypePatternName - v является голым именем типа (не переменной,
// объявленной в текущей области видимости) - тогда его нужно сопоставлять
// через "относится к", а не через "==". Существование/связь самого имени с
// типом Subject не проверяется здесь - этим уже занимается analyzeIsExpr
// (resolveTypeName/relatedForIsCheck), вызываемый через обычный
// analyzeStmt(*ast.IfStmt) выше - здесь только решается, какую AST-форму
// построить.
func switchTypePatternName(sc *ctx, v ast.Expr) (string, bool) {
	ident, ok := v.(*ast.Identifier)
	if !ok {
		return "", false
	}
	if _, declared := sc.lookup(ident.Name); declared {
		return "", false
	}
	return ident.Name, true
}

// exprLine - строка исходного выражения e, для сообщений об ошибках вложенных
// синтетических узлов, которые desugaring строит без собственной позиции.
func exprLine(e ast.Expr) int {
	switch e := e.(type) {
	case *ast.Identifier:
		return e.Line
	case *ast.IntLiteral:
		return e.Line
	case *ast.FloatLiteral:
		return e.Line
	case *ast.StringLiteral:
		return e.Line
	case *ast.CharLiteral:
		return e.Line
	case *ast.BoolLiteral:
		return e.Line
	case *ast.BinaryExpr:
		return e.Line
	case *ast.TernaryExpr:
		return e.Line
	case *ast.UnaryExpr:
		return e.Line
	case *ast.FieldAccess:
		return e.Line
	default:
		return 0
	}
}
