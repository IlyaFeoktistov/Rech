package parser

import (
	"github.com/IlyaFeoktistov/Rech/internal/ast"
	"github.com/IlyaFeoktistov/Rech/internal/lexer"
	"github.com/IlyaFeoktistov/Rech/internal/morph"
)

// Переменные уровня модуля - "переменная <имя> = ..." и "<Тип> <имя> = ..."
// на верхнем уровне файла. Разбираются той же веткой, что и инструкции
// неявной "главная" (parseTopLevelItem), и только в конце файла
// (resolveModuleVars) становятся объявлениями: в файле-скрипте (инструкции
// на верхнем уровне без явной "главная") они остаются инструкциями по
// порядку выполнения - иначе "переменная у = х" после "х = х + 1"
// вычислялась бы до неё.

// moduleVarCandidate - объявление переменной на верхнем уровне файла.
type moduleVarCandidate struct {
	decl *ast.VarDecl
	// visTok - модификатор видимости перед объявлением, nil - его не было.
	visTok *lexer.Token
}

// moduleVisibility - модификатор видимости последнего объявления верхнего
// уровня (parseModuleVisibility): tok == nil - модификатора не было;
// deferred - модификатор стоит перед "<Тип> <имя>", согласуется с типом
// переменной, проверяется после разбора объявления (noteTopLevelStmt).
type moduleVisibility struct {
	tok      *lexer.Token
	vis      ast.Visibility
	deferred bool
}

// noteTopLevelStmt - stmt разобран веткой инструкции верхнего уровня:
// объявление переменной запоминается кандидатом в переменные модуля;
// отложенное согласование модификатора видимости - проверяется по типу.
func (p *Parser) noteTopLevelStmt(stmt ast.Stmt) error {
	mv := p.topVisibility
	decl, isVar := stmt.(*ast.VarDecl)
	if !isVar || decl.IsStatic {
		if mv.tok != nil {
			return errAt(mv.tok.Line, mv.tok.Col, CodeModifierAgree, "модификатор видимости здесь неуместен - ожидалась \"функция\", \"сущность\", \"класс\", \"структура\", \"перечисление\", \"константа\", \"синоним\" или объявление переменной")
		}
		return nil
	}
	if mv.deferred {
		if err := checkVarVisibilityAgreement(*mv.tok, mv.vis, decl.Type); err != nil {
			return err
		}
	}
	decl.Visibility = mv.vis
	p.moduleVarCandidates = append(p.moduleVarCandidates, moduleVarCandidate{decl: decl, visTok: mv.tok})
	return nil
}

// checkVarVisibilityAgreement - модификатор перед "<Тип> <имя>" согласуется
// по роду/числу с типом переменной (тем же правилом, что у поля сущности);
// если род типа неизвестен, подходит любая форма.
func checkVarVisibilityAgreement(kw lexer.Token, vis ast.Visibility, typeName string) error {
	gender, number, ok := morph.TypeAgreement(typeName)
	if !ok {
		return nil
	}
	if want := visibilityKeywordFor(vis, gender, number); kw.Value != want {
		return errAt(kw.Line, kw.Col, CodeModifierAgree, "модификатор видимости переменной не согласован по роду/числу с её типом").
			WithHint("требует форму %q, получено %q", want, kw.Value)
	}
	return nil
}

// resolveModuleVars переносит кандидатов в prog.Vars (и убирает их из
// инструкций верхнего уровня), если файл не скрипт; у скрипта они
// остаются инструкциями. Ошибки - по одной на объявление.
func (p *Parser) resolveModuleVars(prog *ast.Program, topLevelStmts *[]ast.Stmt, hasExplicitMain bool) []error {
	cands := p.moduleVarCandidates
	p.moduleVarCandidates = nil
	if len(cands) == 0 {
		return nil
	}
	isCand := make(map[ast.Stmt]bool, len(cands))
	for _, c := range cands {
		isCand[c.decl] = true
	}
	script := false
	for _, s := range *topLevelStmts {
		if !isCand[s] {
			script = true
			break
		}
	}
	var errs []error
	if script && !hasExplicitMain {
		for _, c := range cands {
			if c.visTok != nil {
				errs = append(errs, errAt(c.visTok.Line, c.visTok.Col, CodeModifierAgree, "модификатор видимости у переменной %q: в файле-скрипте (инструкции на верхнем уровне без \"функция главная\") переменные верхнего уровня - инструкции, а не объявления модуля", c.decl.Name))
			}
		}
		return errs
	}
	rest := (*topLevelStmts)[:0:0]
	for _, s := range *topLevelStmts {
		if !isCand[s] {
			rest = append(rest, s)
		}
	}
	*topLevelStmts = rest
	for _, c := range cands {
		if c.decl.Value == nil {
			errs = append(errs, errAtLine(c.decl.Line, CodeStmtRestrict, "у переменной модуля %q нужно начальное значение (\"= <выражение>\")", c.decl.Name))
			continue
		}
		prog.Vars = append(prog.Vars, c.decl)
	}
	return errs
}
