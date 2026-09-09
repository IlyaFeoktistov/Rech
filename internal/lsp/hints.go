// Подсказки-упрощения ("три точки" - Hint-severity диагностика, см.
// deprecated.go про hintSeverity, тот же принцип, что "можно сделать
// static"/"можно сократить запись" в ReSharper/PhpStorm) - в отличие от
// deprecatedDiagnostics (там сигнал приходит из doc-комментария автора),
// эти диагностики компилятор находит сам, по форме AST:
//   - составное присваивание можно сократить (i = i + 1 -> i++, i = i + x
//     -> i += x и т. п.);
//   - метод можно сделать статическим (нигде в теле не ссылается на
//     "этот" - см. staticMethodSuggestion).
//
// Suggestion, в отличие от CodeAction (codeaction.go - только вставка новой
// строки, автоимпорт), несёт ДИАПАЗОН ЗАМЕНЫ произвольного существующего
// текста - обе формы quick fix существуют одновременно, server.go собирает
// ответ "textDocument/codeAction" из ОБОИХ списков.
//
// "i = i + 1" и "i += 1" разбираются ПАРСЕРОМ в ОДИНАКОВЫЙ AST (составное
// присваивание - чистый сахар, internal/parser/stmt.go,
// compoundAssignOps/parseAssign) - отличить уже сокращённую запись от
// длинной по одному только AST невозможно. Поэтому обе функции этого файла
// смотрят и в СЫРЫЕ ТОКЕНЫ строки (lineToks) - конкретно в токен ОПЕРАТОРА
// присваивания: bare "=" (lexer.ASSIGN) - длинная запись, есть что
// предложить; любой составной токен (PLUS_ASSIGN и т. п.) - уже
// сокращена, предлагать нечего.
package lsp

import (
	"fmt"
	"slices"
	"strings"

	"github.com/IlyaFeoktistov/Rech/internal/ast"
	"github.com/IlyaFeoktistov/Rech/internal/lexer"
)

// Suggestion - один хинт вместе с готовым quick fix'ом.
type Suggestion struct {
	// Line/Col/Len - диагностика (три точки под текстом), 1-индексные (Col
	// - руны, не байты, тот же принцип, что и у Diagnostic/lexer.Token.Col).
	Line, Col, Len int
	Message        string
	// Title - заголовок в списке "Быстрое исправление" (Ctrl+.).
	Title string
	// StartLine/StartCol - начало (включительно), EndLine/EndCol - конец
	// (исключительно) заменяемого диапазона текста, 1-индексные.
	StartLine, StartCol int
	EndLine, EndCol     int
	NewText             string
}

// suggestions - все хинты текущего документа (см. package doc). Пусто,
// если разбор не удался вовсе (d.astProg == nil) - те же условия, что и у
// codeActions/deprecatedDiagnostics.
func (d *doc) suggestions() []Suggestion {
	if d.astProg == nil {
		return nil
	}
	lineToks := d.linesOfTokens()
	lines := strings.Split(d.text, "\n")
	var out []Suggestion
	for _, fn := range d.astProg.Functions {
		out = append(out, compoundAssignSuggestions(fn.Body, lineToks, lines)...)
	}
	for _, ent := range d.astProg.Entities {
		if ent.Ctor != nil {
			out = append(out, compoundAssignSuggestions(ent.Ctor.Body, lineToks, lines)...)
		}
		for _, m := range ent.Methods {
			out = append(out, compoundAssignSuggestions(m.Body, lineToks, lines)...)
			if s, ok := staticMethodSuggestion(ent, m, lineToks); ok {
				out = append(out, s)
			}
		}
	}
	return out
}

// linesOfTokens - d.tokens, сгруппированные по физической строке (без
// NEWLINE/EOF) - тот же принцип, что buildFieldRow в internal/format/
// field_align.go, здесь нужен для доступа к СЫРЫМ токенам строки
// присваивания/объявления метода (см. package doc).
func (d *doc) linesOfTokens() map[int][]lexer.Token {
	out := map[int][]lexer.Token{}
	for _, t := range d.tokens {
		if t.Type == lexer.NEWLINE || t.Type == lexer.EOF {
			continue
		}
		out[t.Line] = append(out[t.Line], t)
	}
	return out
}

// arithOpTokenType - обратное отображение ast.BinaryExpr.Op (уже
// разобранный компилятором оператор, "+"/"-"/"*"/"/"/"%") в токен, которым
// он записан в исходнике - нужно найти именно ЭТОТ токен между двумя
// одноимёнными Identifier'ами (Name OP Name OP2 ...), не гадать по тексту.
var arithOpTokenType = map[string]lexer.TokenType{
	"+": lexer.PLUS, "-": lexer.MINUS, "*": lexer.STAR, "/": lexer.SLASH, "%": lexer.PERCENT,
}

// compoundAssignSuggestions ищет во ВСЕХ инструкциях stmts (рекурсивно,
// walkStmts) присваивание вида "имя = имя <оператор> <выражение>" - то же
// самое значение, что и составное присваивание/инкремент, просто записанное
// длинно (см. package doc). НЕ трогает ast.FieldAssign ("этот.поле = ...") -
// поле сущности не поддерживает "++"/"--" в этом языке вовсе (ast.
// IncDecExpr - только для простой переменной), а составное присваивание
// поля через this-цепочку токенов сложнее надёжно сопоставить с исходным
// текстом без риска ошибиться - осознанно вне этого среза.
func compoundAssignSuggestions(stmts []ast.Stmt, lineToks map[int][]lexer.Token, lines []string) []Suggestion {
	var out []Suggestion
	walkStmts(stmts, func(s ast.Stmt) {
		a, ok := s.(*ast.Assign)
		if !ok {
			return
		}
		bin, ok := a.Value.(*ast.BinaryExpr)
		if !ok {
			return
		}
		left, ok := bin.Left.(*ast.Identifier)
		if !ok || left.Name != a.Name {
			return
		}
		opTokType, ok := arithOpTokenType[bin.Op]
		if !ok {
			return
		}
		toks := lineToks[a.Line]
		// Ожидаемая раскладка токенов присваивания на строке: IDENT(имя)
		// ASSIGN IDENT(имя) <оператор> <...правая часть до конца строки>.
		if len(toks) < 4 {
			return
		}
		nameIdx := -1
		for i, t := range toks {
			if t.Type == lexer.IDENT && t.Value == a.Name && i+3 < len(toks) &&
				toks[i+1].Type == lexer.ASSIGN &&
				toks[i+2].Type == lexer.IDENT && toks[i+2].Value == a.Name &&
				toks[i+3].Type == opTokType {
				nameIdx = i
				break
			}
		}
		if nameIdx == -1 {
			return
		}
		rhsStart := nameIdx + 4
		if rhsStart >= len(toks) {
			return
		}
		if a.Line < 1 || a.Line > len(lines) {
			return
		}
		lineText := lines[a.Line-1]
		runes := []rune(lineText)
		endCol := len(runes) + 1
		rightText := strings.TrimRight(sliceRunes(runes, toks[rhsStart].Col, endCol), " \t\r")

		newText, title := compoundAssignRewrite(a.Name, bin.Op, bin.Right, rightText)
		out = append(out, Suggestion{
			Line: a.Line, Col: toks[nameIdx].Col, Len: (endCol - toks[nameIdx].Col),
			Message:   fmt.Sprintf("можно сократить запись до %q", newText),
			Title:     title,
			StartLine: a.Line, StartCol: toks[nameIdx].Col,
			EndLine: a.Line, EndCol: endCol,
			NewText: newText,
		})
	}, func(ast.Expr) {})
	return out
}

// compoundAssignRewrite - готовый текст замены и заголовок quick fix'а:
// "+ 1"/"- 1" (буквально IntLiteral(1) справа) - инкремент/декремент,
// иначе - составное присваивание тем же оператором, правая часть - СЫРОЙ
// исходный текст (rightText, уже вычленен по столбцам вызывающей стороной)
// - не пересобран заново из AST, чтобы не потерять авторское форматирование
// сложного выражения (пробелы вокруг вложенных операторов и т. п.).
func compoundAssignRewrite(name, op string, right ast.Expr, rightText string) (newText, title string) {
	if lit, ok := right.(*ast.IntLiteral); ok && lit.Value == 1 {
		switch op {
		case "+":
			return name + "++", fmt.Sprintf("Сократить до %q", name+"++")
		case "-":
			return name + "--", fmt.Sprintf("Сократить до %q", name+"--")
		}
	}
	compound := name + " " + op + "= " + rightText
	return compound, fmt.Sprintf("Сократить до составного присваивания (%q)", op+"=")
}

// sliceRunes - runes[fromCol-1:toCol-1] (Col - 1-индексный, руны) с
// защитой от выхода за границы (например, EOF-подобная toCol на конце
// строки без символа новой строки).
func sliceRunes(runes []rune, fromCol, toCol int) string {
	from, to := fromCol-1, toCol-1
	if from < 0 {
		from = 0
	}
	if to > len(runes) {
		to = len(runes)
	}
	if from >= to {
		return ""
	}
	return string(runes[from:to])
}

// walkStmts обходит stmts И ВСЕ вложенные блоки (тело если/иначе/пока/для/
// попытаться/поймать/окончательно/ветки "выбрать", А ТАКЖЕ тело любой
// лямбды, найденной где-либо среди выражений инструкции - callback-
// аргумент вызова, значение переменной и т. п., см. walkLambdaBodies)
// рекурсивно - onStmt вызывается на КАЖДОЙ инструкции (включая саму
// составную - "если"/"пока" и т. п. - раньше, чем на её вложенном теле),
// onExpr - на каждом НЕПОСРЕДСТВЕННОМ выражении этой инструкции (условие/
// значение/аргументы - НЕ то, что внутри вложенных блоков, те придут
// отдельно через собственные инструкции вложенного тела). Общий обход для
// обеих задач этого файла - поиска Assign (compoundAssignSuggestions, по
// onStmt) и поиска использования "этот" где угодно в теле
// (staticMethodSuggestion, по onExpr + exprContainsThis) - оба ОБЯЗАНЫ
// видеть код внутри лямбды-колбэка тоже (репортнуто пользователем -
// "j = j + 1" внутри колбэка не подсвечивалось, examples/бд/чанкование.rech).
func walkStmts(stmts []ast.Stmt, onStmt func(ast.Stmt), onExpr func(ast.Expr)) {
	for _, s := range stmts {
		onStmt(s)
		for _, e := range immediateExprs(s) {
			if e != nil {
				onExpr(e)
				walkLambdaBodies(e, func(body []ast.Stmt) { walkStmts(body, onStmt, onExpr) })
			}
		}
		switch st := s.(type) {
		case *ast.IfStmt:
			walkStmts(st.Then, onStmt, onExpr)
			walkStmts(st.Else, onStmt, onExpr)
		case *ast.WhileStmt:
			walkStmts(st.Body, onStmt, onExpr)
		case *ast.ForStmt:
			walkStmts(st.Body, onStmt, onExpr)
		case *ast.ForEachStmt:
			walkStmts(st.Body, onStmt, onExpr)
		case *ast.CStyleForStmt:
			if st.Init != nil {
				onStmt(st.Init)
				if st.Init.Value != nil {
					onExpr(st.Init.Value)
					walkLambdaBodies(st.Init.Value, func(body []ast.Stmt) { walkStmts(body, onStmt, onExpr) })
				}
			}
			if st.Post != nil {
				onStmt(st.Post)
				for _, e := range immediateExprs(st.Post) {
					if e != nil {
						onExpr(e)
						walkLambdaBodies(e, func(body []ast.Stmt) { walkStmts(body, onStmt, onExpr) })
					}
				}
			}
			walkStmts(st.Body, onStmt, onExpr)
		case *ast.TryStmt:
			walkStmts(st.Body, onStmt, onExpr)
			walkStmts(st.CatchBody, onStmt, onExpr)
			walkStmts(st.Finally, onStmt, onExpr)
		case *ast.SwitchStmt:
			for _, c := range st.Cases {
				for _, v := range c.Values {
					onExpr(v)
				}
				walkStmts(c.Body, onStmt, onExpr)
			}
			walkStmts(st.Default, onStmt, onExpr)
		case *ast.GerundStmt:
			walkStmts(st.Body, onStmt, onExpr)
		}
	}
}

// immediateExprs - непосредственные (не внутри вложенного блока)
// подвыражения одной инструкции - см. package doc walkStmts.
func immediateExprs(s ast.Stmt) []ast.Expr {
	switch st := s.(type) {
	case *ast.PrintStmt:
		return []ast.Expr{st.Value}
	case *ast.ShowStmt:
		return []ast.Expr{st.Value}
	case *ast.BecomeStmt:
		return []ast.Expr{st.Receiver}
	case *ast.AssertStmt:
		return []ast.Expr{st.Cond}
	case *ast.IfStmt:
		return []ast.Expr{st.Cond}
	case *ast.WhileStmt:
		return []ast.Expr{st.Cond}
	case *ast.ForStmt:
		return []ast.Expr{st.From, st.To}
	case *ast.ForEachStmt:
		return []ast.Expr{st.Collection}
	case *ast.CStyleForStmt:
		return []ast.Expr{st.Cond}
	case *ast.ReturnStmt:
		return []ast.Expr{st.Value}
	case *ast.ExprStmt:
		out := make([]ast.Expr, 0, 1)
		if st.Call != nil {
			out = append(out, st.Call)
		}
		if st.MethodCall != nil {
			out = append(out, st.MethodCall)
		}
		if st.IncDec != nil {
			out = append(out, st.IncDec)
		}
		if st.Await != nil {
			out = append(out, st.Await)
		}
		return out
	case *ast.GerundStmt:
		return st.Args
	case *ast.Assign:
		return []ast.Expr{st.Value}
	case *ast.FieldAssign:
		return []ast.Expr{st.Receiver, st.Value}
	case *ast.DerefAssignStmt:
		return []ast.Expr{st.Pointer, st.Value}
	case *ast.VarDecl:
		return []ast.Expr{st.Value}
	case *ast.ThrowStmt:
		return []ast.Expr{st.Value}
	case *ast.ParentCallStmt:
		return st.Args
	case *ast.ConstDecl:
		return []ast.Expr{st.Value}
	default:
		return nil
	}
}

// walkLambdaBodies находит КАЖДУЮ ast.LambdaExpr, достижимую из e
// (рекурсивно - callback-аргумент вызова, значение переменной/
// присваивания и т. п.), и вызывает onLambda с её телом - НЕ спускается
// рекурсивно ВНУТРЬ найденной лямбды сама (вложенные лямбды/присваивания
// внутри её тела обрабатывает уже сам вызывающий walkStmts, который зовёт
// эту функцию заново для КАЖДОЙ инструкции этого тела). Тот же список
// вариантов ast.Expr, что и у exprContainsThis (см. её package doc) -
// нужен отдельно от неё, потому что exprContainsThis ищет "этот" именно
// внутри ОДНОЙ лямбды через собственный ad-hoc вызов walkStmts, а этой
// функции нужно найти лямбду САМУ, не заглядывая внутрь.
func walkLambdaBodies(e ast.Expr, onLambda func([]ast.Stmt)) {
	if e == nil {
		return
	}
	switch v := e.(type) {
	case *ast.LambdaExpr:
		onLambda(v.Body)
	case *ast.BinaryExpr:
		walkLambdaBodies(v.Left, onLambda)
		walkLambdaBodies(v.Right, onLambda)
	case *ast.UnaryExpr:
		walkLambdaBodies(v.Operand, onLambda)
	case *ast.TernaryExpr:
		walkLambdaBodies(v.Cond, onLambda)
		walkLambdaBodies(v.Then, onLambda)
		walkLambdaBodies(v.Else, onLambda)
	case *ast.CallExpr:
		for _, a := range v.Args {
			walkLambdaBodies(a, onLambda)
		}
	case *ast.MethodCallExpr:
		walkLambdaBodies(v.Receiver, onLambda)
		for _, a := range v.Args {
			walkLambdaBodies(a, onLambda)
		}
	case *ast.NewExpr:
		for _, a := range v.Args {
			walkLambdaBodies(a, onLambda)
		}
	case *ast.FieldAccess:
		walkLambdaBodies(v.Receiver, onLambda)
	case *ast.IndexExpr:
		walkLambdaBodies(v.Receiver, onLambda)
		walkLambdaBodies(v.Index, onLambda)
	case *ast.ListLiteral:
		for _, e2 := range v.Elems {
			walkLambdaBodies(e2, onLambda)
		}
	case *ast.CastExpr:
		walkLambdaBodies(v.Value, onLambda)
	case *ast.AsExpr:
		walkLambdaBodies(v.Value, onLambda)
	case *ast.IsExpr:
		walkLambdaBodies(v.Value, onLambda)
	case *ast.ParseExpr:
		walkLambdaBodies(v.Source, onLambda)
	case *ast.SerializeExpr:
		walkLambdaBodies(v.Value, onLambda)
	case *ast.AwaitExpr:
		walkLambdaBodies(v.Task, onLambda)
	case *ast.LaunchExpr:
		if v.Call != nil {
			walkLambdaBodies(v.Call, onLambda)
		}
	case *ast.DerefExpr:
		walkLambdaBodies(v.Value, onLambda)
	case *ast.AddressOfExpr:
		walkLambdaBodies(v.Index, onLambda)
	}
}

// exprContainsThis - true, если где-то внутри e (рекурсивно) встречается
// "этот" (ast.ThisExpr) - используется staticMethodSuggestion, чтобы решить,
// можно ли предложить "статический" (нет получателя вообще - см. package
// doc SPEC.md "Статические члены"). Покрывает КАЖДЫЙ вариант ast.Expr - см.
// package doc этого файла, полный список типов найден grep'ом по "func
// (\*.*) expr() {}" в internal/ast.
func exprContainsThis(e ast.Expr) bool {
	if e == nil {
		return false
	}
	switch v := e.(type) {
	case *ast.ThisExpr:
		return true
	case *ast.Identifier, *ast.IntLiteral, *ast.FloatLiteral, *ast.BoolLiteral,
		*ast.CharLiteral, *ast.StringLiteral, *ast.NullLiteral, *ast.TypeOfExpr,
		*ast.FuncRefExpr:
		return false
	case *ast.BinaryExpr:
		return exprContainsThis(v.Left) || exprContainsThis(v.Right)
	case *ast.UnaryExpr:
		return exprContainsThis(v.Operand)
	case *ast.TernaryExpr:
		return exprContainsThis(v.Cond) || exprContainsThis(v.Then) || exprContainsThis(v.Else)
	case *ast.CallExpr:
		return anyExprContainsThis(v.Args)
	case *ast.MethodCallExpr:
		return exprContainsThis(v.Receiver) || anyExprContainsThis(v.Args)
	case *ast.NewExpr:
		return anyExprContainsThis(v.Args)
	case *ast.FieldAccess:
		return exprContainsThis(v.Receiver)
	case *ast.IndexExpr:
		return exprContainsThis(v.Receiver) || exprContainsThis(v.Index)
	case *ast.ListLiteral:
		return anyExprContainsThis(v.Elems)
	case *ast.CastExpr:
		return exprContainsThis(v.Value)
	case *ast.AsExpr:
		return exprContainsThis(v.Value)
	case *ast.IsExpr:
		return exprContainsThis(v.Value)
	case *ast.ParseExpr:
		return exprContainsThis(v.Source)
	case *ast.SerializeExpr:
		return exprContainsThis(v.Value)
	case *ast.AwaitExpr:
		return exprContainsThis(v.Task)
	case *ast.LaunchExpr:
		if v.Call == nil {
			return false
		}
		return exprContainsThis(v.Call)
	case *ast.DerefExpr:
		return exprContainsThis(v.Value)
	case *ast.AddressOfExpr:
		return v.Receiver == "этот" || exprContainsThis(v.Index)
	case *ast.IncDecExpr:
		return false // применимо только к простой переменной, не к "этот" (см. ast.IncDecExpr)
	case *ast.LambdaExpr:
		// Лямбда без собственного получателя захватывает "этот" ВНЕШНЕГО
		// метода лексически, как обычную переменную - тело нужно обойти тем
		// же walkStmts, не отдельным частным случаем.
		found := false
		walkStmts(v.Body, func(ast.Stmt) {}, func(e ast.Expr) {
			if !found && exprContainsThis(e) {
				found = true
			}
		})
		return found
	default:
		return false
	}
}

func anyExprContainsThis(exprs []ast.Expr) bool {
	return slices.ContainsFunc(exprs, exprContainsThis)
}

// staticMethodSuggestion - метод, который НИГДЕ в своём теле не ссылается
// на "этот" (exprContainsThis по всему телу), можно объявить статическим -
// он не нуждается в получателе вообще. Консервативно пропускает уже
// статические/абстрактные методы (нечего предлагать) и методы сущности,
// которая наследует или реализует интерфейс (переопределение/соответствие
// интерфейсу может полагаться на диспетчеризацию через экземпляр даже без
// использования "этот" в конкретно ЭТОЙ реализации - безопаснее не
// предлагать вовсе, чем предложить фикс, ломающий полиморфизм).
func staticMethodSuggestion(ent *ast.EntityDecl, m *ast.MethodDecl, lineToks map[int][]lexer.Token) (Suggestion, bool) {
	if m.IsStatic || m.IsAbstract || m.Body == nil {
		return Suggestion{}, false
	}
	if ent.BaseName != "" || len(ent.Interfaces) > 0 {
		return Suggestion{}, false
	}
	found := false
	walkStmts(m.Body, func(ast.Stmt) {}, func(e ast.Expr) {
		if !found && exprContainsThis(e) {
			found = true
		}
	})
	if found {
		return Suggestion{}, false
	}
	toks := lineToks[m.Line]
	if len(toks) == 0 {
		return Suggestion{}, false
	}
	// Модификатор "статический" в этом языке идёт ПЕРЕД видимостью
	// (internal/parser/entity.go, parseMethod) - значит перед САМЫМ первым
	// токеном строки объявления, будь то видимость или сразу имя метода.
	insertCol := toks[0].Col
	nameCol := insertCol
	for _, t := range toks {
		if t.Type == lexer.IDENT && t.Value == m.Name {
			nameCol = t.Col
			break
		}
	}
	return Suggestion{
		Line: m.Line, Col: nameCol, Len: len([]rune(m.Name)),
		Message:   fmt.Sprintf("метод %q не ссылается на \"этот\" - можно сделать статическим", m.Name),
		Title:     fmt.Sprintf("Сделать %q статическим", m.Name),
		StartLine: m.Line, StartCol: insertCol, EndLine: m.Line, EndCol: insertCol,
		NewText: "статический ",
	}, true
}
