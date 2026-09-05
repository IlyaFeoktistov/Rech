package parser

import (
	"fmt"
	"slices"
	"strings"

	"github.com/IlyaFeoktistov/Rech/internal/ast"
	"github.com/IlyaFeoktistov/Rech/internal/lexer"
	"github.com/IlyaFeoktistov/Rech/internal/morph"
)

func (p *Parser) parseCallExpr() (*ast.CallExpr, error) {
	name := p.advance() // IDENT
	args, err := p.parseArgList()
	if err != nil {
		return nil, fmt.Errorf("в аргументах вызова %q: %w", name.Value, err)
	}
	return &ast.CallExpr{Callee: name.Value, Args: args, Line: name.Line}, nil
}

// parseArgList разбирает "(" [ expr { "," expr } ] ")" - список аргументов,
// общий для строгого вызова функции, конструктора и вызова метода. Позиция
// парсера на входе - на "(".
func (p *Parser) parseArgList() ([]ast.Expr, error) {
	p.advance() // "("
	var args []ast.Expr
	if p.cur().Type != lexer.RPAREN {
		for {
			arg, err := p.parseExpr()
			if err != nil {
				return nil, err
			}
			args = append(args, arg)
			if p.cur().Type != lexer.COMMA {
				break
			}
			p.advance() // ","
		}
	}
	if _, err := p.expect(lexer.RPAREN); err != nil {
		return nil, err
	}
	return args, nil
}

// parseNewExpr разбирает "создать" IDENT "(" [ expr {"," expr} ] ")" -
// конструктор сущности (шаг 4). Только строгий синтаксис, аргументы
// позиционные по порядку полей - падежное согласование имени типа как
// прямого дополнения отложено (docs/GRAMMAR.md).
// parseNewExpr разбирает конструктор: "новый"/"новая"/"новое"/"новые"
// (согласуется по роду/числу с именем сущности - см. checkNewKeywordAgreement)
// type_name "(" args ")". Аргументы - либо позиционные (по порядку полей,
// как раньше), либо именованные (`имя_поля: значение`, в любом порядке) -
// смешивать в одном вызове нельзя.
func (p *Parser) parseNewExpr() (ast.Expr, error) {
	kw, err := p.expect(lexer.KwNew)
	if err != nil {
		return nil, err
	}
	typeName, err := p.parseTypeName()
	if err != nil {
		return nil, fmt.Errorf("после %q ожидалось имя сущности: %w", kw.Value, err)
	}
	if err := p.checkNewKeywordAgreement(kw, typeName); err != nil {
		return nil, err
	}
	if p.cur().Type != lexer.LPAREN {
		return nil, fmt.Errorf("строка %d, столбец %d: после имени сущности %q ожидалось \"(\"", p.cur().Line, p.cur().Col, typeName)
	}
	p.advance() // "("

	var args []ast.Expr
	var fieldNames []string
	if p.cur().Type != lexer.RPAREN {
		labeled := p.cur().Type == lexer.IDENT && p.peekAt(1).Type == lexer.COLON
		for {
			nowLabeled := p.cur().Type == lexer.IDENT && p.peekAt(1).Type == lexer.COLON
			if nowLabeled != labeled {
				return nil, fmt.Errorf("строка %d, столбец %d: в конструкторе %q нельзя смешивать именованные и позиционные аргументы",
					p.cur().Line, p.cur().Col, typeName)
			}
			if labeled {
				label, err := p.expect(lexer.IDENT)
				if err != nil {
					return nil, err
				}
				p.advance() // ":"
				val, err := p.parseExpr()
				if err != nil {
					return nil, fmt.Errorf("в значении поля %q конструктора %q: %w", label.Value, typeName, err)
				}
				fieldNames = append(fieldNames, label.Value)
				args = append(args, val)
			} else {
				val, err := p.parseExpr()
				if err != nil {
					return nil, fmt.Errorf("в аргументах конструктора %q: %w", typeName, err)
				}
				args = append(args, val)
			}
			if p.cur().Type != lexer.COMMA {
				break
			}
			p.advance() // ","
		}
	}
	if _, err := p.expect(lexer.RPAREN); err != nil {
		return nil, err
	}
	return &ast.NewExpr{Type: typeName, Args: args, FieldNames: fieldNames, Line: kw.Line}, nil
}

// newKeywordFor возвращает форму конструктора, согласованную с (gender,
// number) - "новый"/"новая"/"новое"/"новые".
func newKeywordFor(gender morph.Gender, number morph.Number) string {
	if number == morph.Plural {
		return "новые"
	}
	switch gender {
	case morph.Feminine:
		return "новая"
	case morph.Neuter:
		return "новое"
	default:
		return "новый"
	}
}

// checkNewKeywordAgreement проверяет, что фактически написанная форма
// конструктора (kw.Value) согласуется по роду/числу с typeName -
// morph.CompoundNounAgreement (составные имена вида "ОпцииКуки" -
// согласование по первому слову) - только если она вообще смогла
// определить род/число (permissive fallback: слово, которое Фаза A и
// словарь OpenCorpora не classify, не блокирует компиляцию - подходит
// любая форма). typeName для generic-инстанциации ("Список<Целое>") -
// согласование по имени шаблона (до "<"), у самого аргумента типа рода нет
// значения здесь.
func (p *Parser) checkNewKeywordAgreement(kw lexer.Token, typeName string) error {
	base, _, _ := strings.Cut(typeName, "<")
	gender, number, ok := morph.CompoundNounAgreement(base)
	if !ok {
		return nil
	}
	want := newKeywordFor(gender, number)
	if kw.Value != want {
		return fmt.Errorf("строка %d, столбец %d: сущность %q требует форму %q, получено %q",
			kw.Line, kw.Col, typeName, want, kw.Value)
	}
	return nil
}

// naturalCallPrepositions - предлоги, разрешённые перед аргументом
// естественного вызова. Чисто декоративные (падеж определяет роль
// аргумента, не предлог, docs/GRAMMAR.md) - парсер их просто пропускает,
// не проверяя согласование с падежом следующего слова. Небольшой курируемый
// список (как и всё в этом движке, а не попытка перечислить все предлоги
// русского языка) - ровно то, что нужно каноническому примеру из
// docs/GRAMMAR.md; расширяется по мере надобности.
var naturalCallPrepositions = map[lexer.TokenType]bool{
	lexer.KwWith: true,
	lexer.KwIn:   true,
	lexer.KwFor:  true,
	lexer.KwFrom: true,
	lexer.KwOf:   true,
}

// startsNaturalCallArg - true, если tok может быть первым токеном аргумента
// естественного вызова (сам аргумент - lexer.IDENT, возможно после предлога
// из naturalCallPrepositions, см. parseNaturalCall) - используется
// parsePrimary (expr.go), чтобы отличить голый идентификатор-значение,
// случайно совпавший именем с какой-то функцией файла, от настоящего начала
// естественного вызова.
func startsNaturalCallArg(tok lexer.Token) bool {
	return tok.Type == lexer.IDENT || naturalCallPrepositions[tok.Type]
}

// parseNaturalCall разбирает естественный вызов - вызов функции без скобок,
// с аргументами, которые связываются с параметрами по падежу словоформы
// (docs/GRAMMAR.md, "Естественный вызов и падежное сопоставление"), а не по
// порядку слов в предложении. Позиция парсера на входе - на имени функции
// (IDENT), params - её параметры в порядке объявления (для построения
// CallExpr.Args по позиции, а не по порядку слов).
//
// Ограничения этой реализации (docs/SPEC.md): аргумент - всегда IDENT,
// ссылающийся на уже объявленную переменную (лемма из p.dict), не литерал;
// вся фраза - в одной строке (без продолжения по отступу).
// parseNaturalCall разбирает естественный вызов - вызов функции без скобок.
// Каждый параметр получает ровно один аргумент-словоформу; связывание -
// в два прохода (docs/SPEC.md, "Вызов функции (естественный синтаксис)"):
//
//  1. Падежное сопоставление - только для параметров с падежной ролью
//     (Param.CaseWord != ""), как и раньше: словоформа должна грамматически
//     подходить по падежу (и по одушевлённости для кого/что) ещё не
//     занятому помеченному параметру.
//  2. Аргументы, не занявшие место на проходе 1 (в порядке, в котором они
//     написаны), заполняют по порядку объявления оставшиеся параметры БЕЗ
//     падежной роли - падеж для них не важен, берётся буквальный текст
//     словоформы как имя переменной (без обращения к словарю/лемме).
//
// Параметр с падежной ролью, не получивший подходящий по падежу аргумент
// на проходе 1, не подбирается "на что осталось" на проходе 2 - это
// осталось бы ошибкой пользователя (неверная словоформа), а не поводом
// угадывать.
func (p *Parser) parseNaturalCall(params []ast.Param) (*ast.CallExpr, error) {
	name := p.advance() // имя функции
	args, err := p.matchNaturalCallArgs(name, params)
	if err != nil {
		return nil, err
	}
	return &ast.CallExpr{Callee: name.Value, Args: args, Line: name.Line}, nil
}

// matchNaturalCallArgs - падежное сопоставление аргументов естественного
// вызова, общее для свободной функции (parseNaturalCall) и метода сущности
// (parseNaturalMethodCall, natural_method_call.go) - subject уже
// потреблённый токен-"глагол" вызова (имя функции или имя метода, у обоих
// вызовов ровно одна словоформа), используется только для текста ошибок.
// Позиция парсера на входе - на первом токене-аргументе (сразу после
// subject).
func (p *Parser) matchNaturalCallArgs(subject lexer.Token, params []ast.Param) ([]ast.Expr, error) {
	name := subject

	tokens := make([]lexer.Token, len(params))
	// tokenPrep[i] - предлог, реально стоявший перед tokens[i] в исходном
	// тексте ("" - не было). Для большинства падежных имён декоративно и не
	// проверяется, но для параметров с morph.MandatoryPrepositions (ParamWith*/
	// ParamFor*/ParamWhereTo/ParamWhereFrom) обязателен и должен входить в
	// её список допустимых предлогов - см. проверку ниже, после того как
	// токен сопоставлен с параметром (какой параметр - неизвестно, пока не
	// пройден проход 1).
	tokenPrep := make([]string, len(params))
	for i := range tokens {
		if naturalCallPrepositions[p.cur().Type] {
			tokenPrep[i] = p.cur().Value
			p.advance()
		}
		argTok, err := p.expect(lexer.IDENT)
		if err != nil {
			return nil, fmt.Errorf("в естественном вызове %q: не хватает аргументов (не заполнено ещё %d): %w", name.Value, len(params)-i, err)
		}
		tokens[i] = argTok
	}

	args := make([]ast.Expr, len(params))
	filled := make([]bool, len(params))
	tokenUsed := make([]bool, len(tokens))

	// Проход 1: падежное сопоставление для параметров с падежной ролью.
	for ti, argTok := range tokens {
		analyses := p.dict.Analyze(argTok.Value)
		if len(analyses) == 0 {
			continue // не опознано словарём - может ещё подойти на проходе 2
		}
		matchIdx := -1
		matchLemma := ""
		for i, param := range params {
			if filled[i] || param.CaseWord == "" {
				continue
			}
			paramCase, _ := morph.CaseOfParamName(param.CaseWord)
			for _, a := range analyses {
				if a.POS != morph.Noun || a.Tag.Case != paramCase {
					continue
				}
				if wantAnimacy, ok := morph.AnimacyRequiredByParamName(param.CaseWord); ok {
					lemma, lok := p.dict.Lookup(a.Lemma)
					if !lok || lemma.Noun == nil || lemma.Noun.Animacy != wantAnimacy {
						continue
					}
				}
				if matchIdx != -1 && (matchIdx != i || matchLemma != a.Lemma) {
					return nil, fmt.Errorf("строка %d, столбец %d: словоформа %q неоднозначна - подходит сразу нескольким незаполненным параметрам вызова %q",
						argTok.Line, argTok.Col, argTok.Value, name.Value)
				}
				matchIdx, matchLemma = i, a.Lemma
			}
		}
		if matchIdx != -1 {
			if allowed, needsPrep := morph.MandatoryPrepositions(params[matchIdx].CaseWord); needsPrep && !slices.Contains(allowed, tokenPrep[ti]) {
				return nil, fmt.Errorf("строка %d, столбец %d: параметру %q нужен предлог (%s) перед аргументом",
					argTok.Line, argTok.Col, params[matchIdx].Name, strings.Join(allowed, "/"))
			}
			args[matchIdx] = &ast.Identifier{Name: matchLemma, Line: argTok.Line}
			filled[matchIdx] = true
			tokenUsed[ti] = true
		}
	}

	// Проход 2: оставшиеся токены по порядку - в оставшиеся параметры без
	// падежной роли, по порядку объявления.
	ti := 0
	for i, param := range params {
		if filled[i] || param.CaseWord != "" {
			continue
		}
		for ti < len(tokenUsed) && tokenUsed[ti] {
			ti++
		}
		if ti >= len(tokens) {
			break // не хватило аргументов - разберётся общая проверка ниже
		}
		args[i] = &ast.Identifier{Name: tokens[ti].Value, Line: tokens[ti].Line}
		filled[i] = true
		tokenUsed[ti] = true
		ti++
	}

	for i, param := range params {
		if !filled[i] {
			if param.CaseWord != "" {
				return nil, fmt.Errorf("строка %d, столбец %d: не удалось подобрать по падежу аргумент для параметра %q вызова %q",
					name.Line, name.Col, param.Name, name.Value)
			}
			return nil, fmt.Errorf("строка %d, столбец %d: не хватает аргументов для параметра %q вызова %q",
				name.Line, name.Col, param.Name, name.Value)
		}
	}
	for i, used := range tokenUsed {
		if !used {
			return nil, fmt.Errorf("строка %d, столбец %d: словоформа %q лишняя в вызове %q",
				tokens[i].Line, tokens[i].Col, tokens[i].Value, name.Value)
		}
	}

	return args, nil
}

// naturalCallPutWord - контекстное слово "положить" в продолжении "...
// и положить в <переменная>" (parseNaturalCallContinuation) - НЕ заведено
// отдельным токеном лексера, в отличие от "и"/"вывести"/"в": "положить"
// уже занято как имя метода Стопка<T> (docs/MANUAL.md,
// "положить(значение: T)"), обычный IDENT после точки
// ("стопка.положить(...)") - зарезервировать его как ключевое слово
// лексера сломало бы этот вызов. Здесь просто сравнивается буквальное
// значение текущего IDENT-токена, в позиции сразу после "и", где имя
// метода появиться не может.
const naturalCallPutWord = "положить"

// parseNaturalCallContinuation разбирает необязательное продолжение
// естественного вызова call - "и <действие>" (docs/GRAMMAR.md,
// "Естественный вызов как значение"):
//
//	<вызов> и вывести               - печатает результат немедленно
//	<вызов> и положить в <имя>      - сохраняет результат в имя (обычное
//	                                   присваивание уже существующей
//	                                   переменной либо, если имя -
//	                                   отложенное объявление того же блока
//	                                   ("переменная имя" без "=",
//	                                   pendingvar.go) - её объявление)
//
// Без "и" - как раньше: результат отбрасывается, самостоятельная
// инструкция. Оба продолжения не требуют новых токенов лексера сверх уже
// существующих (KwAnd/KwPrint/KwIn) - только сам факт "и" сразу после
// фразы естественного вызова снимает неоднозначность (docs/GRAMMAR.md,
// закрытый этим открытый вопрос): граница фразы уже известна (ровно
// len(params) словоформ, см. parseNaturalCall), "и" - явный маркер
// продолжения, не часть выражения.
func (p *Parser) parseNaturalCallContinuation(call *ast.CallExpr) (ast.Stmt, error) {
	stmt, has, err := p.parseNaturalCallContinuationCore(call, call.Line)
	if err != nil {
		return nil, err
	}
	if has {
		return stmt, nil
	}
	return &ast.ExprStmt{Call: call, Line: call.Line}, nil
}

// parseNaturalCallContinuationCore - общая часть parseNaturalCallContinuation
// (вызов свободной функции) и parseNaturalMethodCallContinuation
// (natural_method_call.go, вызов метода) - разбирает необязательное "и
// <действие>" после уже разобранного вызова call (оба вида вызова
// естественного синтаксиса продолжаются одинаково, docs/GRAMMAR.md,
// "Естественный вызов как значение"). hasContinuation == false ("и" не
// найдено) - вызывающая сторона сама строит ExprStmt для СВОЕГО конкретного
// вида вызова (ast.ExprStmt.Call vs .MethodCall - разные поля, не общий
// интерфейс), это единственное, что не обобщено здесь.
func (p *Parser) parseNaturalCallContinuationCore(call ast.Expr, line int) (stmt ast.Stmt, hasContinuation bool, err error) {
	if p.cur().Type != lexer.KwAnd {
		return nil, false, nil
	}
	p.advance() // "и"
	if p.cur().Type == lexer.KwPrint {
		p.advance()
		return &ast.PrintStmt{Value: call, Line: line}, true, nil
	}
	if p.cur().Type == lexer.IDENT && p.cur().Value == naturalCallPutWord {
		p.advance() // "положить"
		if _, err := p.expect(lexer.KwIn); err != nil {
			return nil, true, fmt.Errorf("после %q ожидалось \"в\" и имя переменной: %w", naturalCallPutWord, err)
		}
		target, err := p.expect(lexer.IDENT)
		if err != nil {
			return nil, true, fmt.Errorf("после \"в\" ожидалось имя переменной: %w", err)
		}
		return p.resolveAssignTarget(target, call), true, nil
	}
	return nil, true, fmt.Errorf("строка %d, столбец %d: после \"и\" ожидалось \"вывести\" или %q в <переменная>, получено %q",
		p.cur().Line, p.cur().Col, naturalCallPutWord, p.cur().Value)
}
