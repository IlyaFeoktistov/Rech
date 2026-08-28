// Пользовательские generic-типы - мономорфизация без настоящих generic'ов
// во время выполнения, тот же принцип, что и у встроенных generic-
// коллекций (Список<T>/Стопка<T>/Очередь<T>): каждое конкретное использование
// "Коробка<Целое>"/"первый<Целое>(...)" синтезирует ОБЫЧНОЕ объявление
// сущности/функции (T заменён на Целое всюду - поля, параметры, тип
// результата, тело) ДО начала обычного semantic.Analyze - дальше весь
// конвейер (collectEntities/collectFuncSigs/buildIR, cbackend, interp)
// вообще не знает, что эта сущность/функция когда-то была generic,
// работает с ней как с любой обычной. instantiateGenerics - единственная
// точка входа, вызывается из Analyze первой.
package semantic

import (
	"fmt"
	"strings"

	"github.com/IlyaFeoktistov/Rech/internal/ast"
	"github.com/IlyaFeoktistov/Rech/internal/ir"
)

// splitTopLevelArgs разбивает список аргументов типа на верхнем уровне по
// запятой, не заходя внутрь вложенных "<...>" ("A, B<C, D>, E" -> ["A",
// "B<C, D>", "E"]) - те же байтовые границы, что и у остальных разборов
// generic-имени в пакете (только ASCII "<"/">"/"," значимы, кириллица
// многобайтовая, но ни один её байт не совпадает с ASCII).
func splitTopLevelArgs(s string) []string {
	var parts []string
	depth := 0
	start := 0
	for i := 0; i < len(s); i++ {
		switch s[i] {
		case '<':
			depth++
		case '>':
			depth--
		case ',':
			if depth == 0 {
				parts = append(parts, strings.TrimSpace(s[start:i]))
				start = i + 1
			}
		}
	}
	parts = append(parts, strings.TrimSpace(s[start:]))
	return parts
}

// parseGenericTypeNameMulti разбирает "Шаблон<A, B>" на ("Шаблон", ["A",
// "B"], true) - обобщение parseGenericTypeName (types.go) на несколько
// аргументов (встроенные коллекции - всегда один, пользовательские
// generic-типы, этот файл - могут быть любым числом).
func parseGenericTypeNameMulti(name string) (template string, args []string, ok bool) {
	i := strings.IndexByte(name, '<')
	if i < 0 || !strings.HasSuffix(name, ">") {
		return "", nil, false
	}
	return name[:i], splitTopLevelArgs(name[i+1 : len(name)-1]), true
}

// substituteTypeString заменяет имя параметра типа (целиком совпадающее с
// ключом subst) на аргумент - рекурсивно, чтобы вложенные generic-имена
// ("Список<T>") тоже подставились ("Список<Целое>"). Имя, не совпадающее
// ни с одним параметром и не являющееся generic-именем вовсе (примитив,
// обычная сущность) - возвращается как есть.
func substituteTypeString(s string, subst map[string]string) string {
	if repl, ok := subst[s]; ok {
		return repl
	}
	// "Вызываемый<...>..." - отдельная форма (parseGenericTypeNameMulti
	// требует, чтобы имя заканчивалось на ">", а тут после закрывающей
	// "<...>" может идти ещё тип результата без разделителя) - разбирается
	// собственным разбором (splitCallableTypeString, internal/semantic/
	// callable.go), не обычным generic-механизмом ниже.
	if params, result, ok := splitCallableTypeString(s); ok {
		newParams := make([]string, len(params))
		for i, p := range params {
			newParams[i] = substituteTypeString(p, subst)
		}
		newResult := result
		if result != "" {
			newResult = substituteTypeString(result, subst)
		}
		return fmt.Sprintf("%s<%s>%s", ir.CallableTypeName, strings.Join(newParams, ", "), newResult)
	}
	tpl, args, ok := parseGenericTypeNameMulti(s)
	if !ok {
		return s
	}
	newArgs := make([]string, len(args))
	for i, a := range args {
		newArgs[i] = substituteTypeString(a, subst)
	}
	return tpl + "<" + strings.Join(newArgs, ", ") + ">"
}

// substParams/substFields - копии с подстановкой T в Type - новые срезы,
// шаблон (используется для ДРУГИХ инстанциаций тоже) не мутируется.
func substParams(params []ast.Param, subst map[string]string) []ast.Param {
	if len(params) == 0 {
		return nil
	}
	out := make([]ast.Param, len(params))
	for i, p := range params {
		out[i] = p
		out[i].Type = substituteTypeString(p.Type, subst)
	}
	return out
}

func substFields(fields []ast.Field, subst map[string]string) []ast.Field {
	if len(fields) == 0 {
		return nil
	}
	out := make([]ast.Field, len(fields))
	for i, f := range fields {
		out[i] = f
		out[i].Type = substituteTypeString(f.Type, subst)
		if f.Default != nil {
			out[i].Default = substExpr(f.Default, subst)
		}
	}
	return out
}

// substStmts/substStmt/substExpr - глубокая копия тела с подстановкой T во
// ВСЕХ местах, где встречается имя типа (NewExpr.Type/CastExpr.Type/
// IsExpr.Target/TryStmt.CatchTypes/CallExpr.TypeArgs) - копируется целиком
// (не только затронутые узлы) ради простоты: шаблон instantiируется не
// каждую итерацию цикла, а один раз на уникальную комбинацию аргументов,
// накладные расходы копирования пренебрежимы по сравнению с этим.
func substStmts(stmts []ast.Stmt, subst map[string]string) []ast.Stmt {
	if len(stmts) == 0 {
		return nil
	}
	out := make([]ast.Stmt, len(stmts))
	for i, s := range stmts {
		out[i] = substStmt(s, subst)
	}
	return out
}

func substStmt(s ast.Stmt, subst map[string]string) ast.Stmt {
	switch s := s.(type) {
	case *ast.PrintStmt:
		return &ast.PrintStmt{Value: substExpr(s.Value, subst), Line: s.Line}
	case *ast.ShowStmt:
		return &ast.ShowStmt{Value: substExpr(s.Value, subst), Line: s.Line}
	case *ast.VarDecl:
		return &ast.VarDecl{Name: s.Name, Value: substExpr(s.Value, subst), Line: s.Line}
	case *ast.ConstDecl:
		return &ast.ConstDecl{Name: s.Name, Value: substExpr(s.Value, subst), Visibility: s.Visibility, Line: s.Line}
	case *ast.Assign:
		return &ast.Assign{Name: s.Name, Value: substExpr(s.Value, subst), Line: s.Line}
	case *ast.FieldAssign:
		return &ast.FieldAssign{Receiver: substExpr(s.Receiver, subst), Field: s.Field, Value: substExpr(s.Value, subst), Line: s.Line}
	case *ast.BecomeStmt:
		return &ast.BecomeStmt{Receiver: substExpr(s.Receiver, subst), State: s.State, Line: s.Line}
	case *ast.AssertStmt:
		return &ast.AssertStmt{Cond: substExpr(s.Cond, subst), Line: s.Line}
	case *ast.IfStmt:
		return &ast.IfStmt{Cond: substExpr(s.Cond, subst), Then: substStmts(s.Then, subst), Else: substStmts(s.Else, subst), Line: s.Line}
	case *ast.WhileStmt:
		return &ast.WhileStmt{Cond: substExpr(s.Cond, subst), Body: substStmts(s.Body, subst), Line: s.Line}
	case *ast.ForStmt:
		return &ast.ForStmt{Var: s.Var, From: substExpr(s.From, subst), To: substExpr(s.To, subst), Body: substStmts(s.Body, subst), Line: s.Line}
	case *ast.ForEachStmt:
		return &ast.ForEachStmt{Var: s.Var, Collection: substExpr(s.Collection, subst), Body: substStmts(s.Body, subst), Line: s.Line}
	case *ast.ReturnStmt:
		if s.Value == nil {
			return &ast.ReturnStmt{Line: s.Line}
		}
		return &ast.ReturnStmt{Value: substExpr(s.Value, subst), Line: s.Line}
	case *ast.ExprStmt:
		out := &ast.ExprStmt{Line: s.Line}
		switch {
		case s.MethodCall != nil:
			out.MethodCall = substExpr(s.MethodCall, subst).(*ast.MethodCallExpr)
		case s.IncDec != nil:
			out.IncDec = s.IncDec
		case s.Await != nil:
			out.Await = substExpr(s.Await, subst).(*ast.AwaitExpr)
		default:
			out.Call = substExpr(s.Call, subst).(*ast.CallExpr)
		}
		return out
	case *ast.ThrowStmt:
		return &ast.ThrowStmt{Value: substExpr(s.Value, subst), Line: s.Line}
	case *ast.TryStmt:
		catchTypes := make([]string, len(s.CatchTypes))
		for i, t := range s.CatchTypes {
			catchTypes[i] = substituteTypeString(t, subst)
		}
		return &ast.TryStmt{
			Body: substStmts(s.Body, subst), CatchVar: s.CatchVar, CatchTypes: catchTypes,
			CatchBody: substStmts(s.CatchBody, subst), Line: s.Line,
		}
	default:
		return s
	}
}

func substExprs(exprs []ast.Expr, subst map[string]string) []ast.Expr {
	if len(exprs) == 0 {
		return nil
	}
	out := make([]ast.Expr, len(exprs))
	for i, e := range exprs {
		out[i] = substExpr(e, subst)
	}
	return out
}

func substExpr(e ast.Expr, subst map[string]string) ast.Expr {
	switch e := e.(type) {
	case *ast.StringLiteral, *ast.IntLiteral, *ast.FloatLiteral, *ast.BoolLiteral, *ast.CharLiteral, *ast.Identifier, *ast.ThisExpr, *ast.IncDecExpr:
		return e
	case *ast.CallExpr:
		var typeArgs []string
		if len(e.TypeArgs) > 0 {
			typeArgs = make([]string, len(e.TypeArgs))
			for i, t := range e.TypeArgs {
				typeArgs[i] = substituteTypeString(t, subst)
			}
		}
		return &ast.CallExpr{Callee: e.Callee, Args: substExprs(e.Args, subst), TypeArgs: typeArgs, Line: e.Line}
	case *ast.NewExpr:
		return &ast.NewExpr{Type: substituteTypeString(e.Type, subst), Args: substExprs(e.Args, subst), FieldNames: e.FieldNames, Line: e.Line}
	case *ast.FieldAccess:
		return &ast.FieldAccess{Receiver: substExpr(e.Receiver, subst), Field: e.Field, Line: e.Line}
	case *ast.MethodCallExpr:
		return &ast.MethodCallExpr{Receiver: substExpr(e.Receiver, subst), Method: e.Method, Args: substExprs(e.Args, subst), Line: e.Line}
	case *ast.BinaryExpr:
		return &ast.BinaryExpr{Op: e.Op, Left: substExpr(e.Left, subst), Right: substExpr(e.Right, subst), Line: e.Line}
	case *ast.UnaryExpr:
		return &ast.UnaryExpr{Op: e.Op, Operand: substExpr(e.Operand, subst), Line: e.Line}
	case *ast.CastExpr:
		return &ast.CastExpr{Type: e.Type, Value: substExpr(e.Value, subst), Line: e.Line}
	case *ast.ParseExpr:
		return &ast.ParseExpr{Format: e.Format, Source: substExpr(e.Source, subst), TypeName: substituteTypeString(e.TypeName, subst), Line: e.Line}
	case *ast.SerializeExpr:
		return &ast.SerializeExpr{Format: e.Format, Value: substExpr(e.Value, subst), Line: e.Line}
	case *ast.LaunchExpr:
		return &ast.LaunchExpr{Call: substExpr(e.Call, subst).(*ast.CallExpr), Line: e.Line}
	case *ast.AwaitExpr:
		return &ast.AwaitExpr{Task: substExpr(e.Task, subst), Line: e.Line}
	case *ast.IsExpr:
		return &ast.IsExpr{Value: substExpr(e.Value, subst), Target: substituteTypeString(e.Target, subst), Line: e.Line}
	case *ast.FuncRefExpr:
		return e
	case *ast.TypeOfExpr:
		return &ast.TypeOfExpr{TypeName: substituteTypeString(e.TypeName, subst), Line: e.Line}
	case *ast.AsExpr:
		return &ast.AsExpr{Value: substExpr(e.Value, subst), TypeName: substituteTypeString(e.TypeName, subst), Line: e.Line}
	case *ast.LambdaExpr:
		return &ast.LambdaExpr{
			Params: substParams(e.Params, subst), ReturnType: substituteTypeString(e.ReturnType, subst),
			Body: substStmts(e.Body, subst), Line: e.Line,
		}
	default:
		return e
	}
}

func substConstants(consts []*ast.ConstDecl, subst map[string]string) []*ast.ConstDecl {
	if len(consts) == 0 {
		return nil
	}
	out := make([]*ast.ConstDecl, len(consts))
	for i, c := range consts {
		out[i] = &ast.ConstDecl{Name: c.Name, Value: substExpr(c.Value, subst), Visibility: c.Visibility, Line: c.Line, Doc: c.Doc}
	}
	return out
}

func substMethods(methods []*ast.MethodDecl, subst map[string]string) []*ast.MethodDecl {
	if len(methods) == 0 {
		return nil
	}
	out := make([]*ast.MethodDecl, len(methods))
	for i, m := range methods {
		out[i] = &ast.MethodDecl{
			Name: m.Name, Params: substParams(m.Params, subst),
			ReturnType: substituteTypeString(m.ReturnType, subst), SelfReturn: m.SelfReturn,
			Body: substStmts(m.Body, subst), Visibility: m.Visibility, Line: m.Line, Doc: m.Doc,
		}
	}
	return out
}

func substCtor(ctor *ast.ConstructorDecl, subst map[string]string) *ast.ConstructorDecl {
	if ctor == nil {
		return nil
	}
	return &ast.ConstructorDecl{Params: substParams(ctor.Params, subst), Body: substStmts(ctor.Body, subst), Line: ctor.Line}
}

// mangleGenericCallName - "первый" + ["Целое"] -> "первый<Целое>" - то же
// имя, под которым инстанциация регистрируется в entities/funcs (та же
// схема, что и у generic-имени типа - "Шаблон<Аргументы>").
func mangleGenericCallName(callee string, typeArgs []string) string {
	return callee + "<" + strings.Join(typeArgs, ", ") + ">"
}

// constraintCheck - одно ограничение параметра типа конкретной
// инстанциации, отложенное до момента, когда entities/interfaces уже
// известны (в самом начале Analyze, когда работает instantiateGenerics,
// их ещё нет - см. package doc) - проверяется в collectEntities/
// collectFuncSigs, когда до этой инстанциации доходит очередь.
type constraintCheck struct {
	ParamName   string
	ArgType     string
	Constraints []string
	Line        int
}

// instantiator - состояние мономорфизации одной программы - см. package
// doc. entities/funcs - готовые инстанциации по мангл-имени (регистрация
// СРАЗУ, до скана собственного тела - защита от бесконечной рекурсии при
// использовании инстанциации внутри её же тела, например рекурсивный
// generic-вызов с тем же самым набором аргументов типа).
// entityOrder/funcOrder - те же имена, но в порядке "зависимость раньше
// зависящего" (topological, добавляется ПОСЛЕ скана, см. ensureEntity/
// ensureFunc) - entities/funcs как map не несёт порядка, а
// collectEntities/collectFuncSigs (дальше по конвейеру) требуют, чтобы
// сущность, на которую ссылается поле/параметр, была объявлена РАНЬШЕ по
// списку.
type instantiator struct {
	entityTemplates map[string]*ast.EntityDecl
	funcTemplates   map[string]*ast.FunctionDecl
	entities        map[string]*ast.EntityDecl
	funcs           map[string]*ast.FunctionDecl
	entityOrder     []string
	funcOrder       []string
	constraints     map[string][]constraintCheck
}

func newInstantiator(prog *ast.Program) *instantiator {
	inst := &instantiator{
		entityTemplates: map[string]*ast.EntityDecl{},
		funcTemplates:   map[string]*ast.FunctionDecl{},
		entities:        map[string]*ast.EntityDecl{},
		funcs:           map[string]*ast.FunctionDecl{},
		constraints:     map[string][]constraintCheck{},
	}
	for _, ent := range prog.Entities {
		if len(ent.TypeParams) > 0 {
			inst.entityTemplates[ent.Name] = ent
		}
	}
	for _, fn := range prog.Functions {
		if len(fn.TypeParams) > 0 {
			inst.funcTemplates[fn.Name] = fn
		}
	}
	return inst
}

// ensureEntity гарантирует, что инстанциация name ("Коробка<Целое>")
// существует - синтезирует её при первой встрече (подстановка T,
// рекурсивный скан собственного тела на предмет ДАЛЬШЕ вложенных
// инстанциаций), иначе не делает ничего. name, не являющееся generic-именем
// вообще или относящееся к неизвестному шаблону (примитив, обычная
// сущность, встроенная коллекция) - не ошибка здесь: resolveTypeName дальше
// по конвейеру либо найдёт его как обычную сущность, либо выдаст свою
// собственную, более уместную ошибку "не объявлено".
func (inst *instantiator) ensureEntity(name string, line int) error {
	if _, ok := inst.entities[name]; ok {
		return nil
	}
	tplName, args, ok := parseGenericTypeNameMulti(name)
	if !ok {
		return nil
	}
	template, ok := inst.entityTemplates[tplName]
	if !ok {
		return nil
	}
	if len(args) != len(template.TypeParams) {
		return fmt.Errorf("строка %d: сущность %q ожидает параметров типа: %d, получено: %d", line, tplName, len(template.TypeParams), len(args))
	}
	subst := make(map[string]string, len(args))
	var checks []constraintCheck
	for i, tp := range template.TypeParams {
		subst[tp.Name] = args[i]
		if len(tp.Constraints) > 0 {
			checks = append(checks, constraintCheck{ParamName: tp.Name, ArgType: args[i], Constraints: tp.Constraints, Line: line})
		}
	}
	synthesized := &ast.EntityDecl{
		Name:      name,
		IsStruct:  template.IsStruct,
		Constants: substConstants(template.Constants, subst),
		States:    template.States,
		Fields:    substFields(template.Fields, subst),
		Methods:   substMethods(template.Methods, subst),
		Ctor:      substCtor(template.Ctor, subst),
		Line:      line,
		File:      template.File,
		Doc:       template.Doc,
	}
	inst.entities[name] = synthesized
	if len(checks) > 0 {
		inst.constraints[name] = checks
	}
	if err := inst.scanEntity(synthesized); err != nil {
		return err
	}
	inst.entityOrder = append(inst.entityOrder, name)
	return nil
}

// ensureFunc - то же самое для generic-функции ("первый<Целое>").
func (inst *instantiator) ensureFunc(name string, line int) error {
	if _, ok := inst.funcs[name]; ok {
		return nil
	}
	tplName, args, ok := parseGenericTypeNameMulti(name)
	if !ok {
		return nil
	}
	template, ok := inst.funcTemplates[tplName]
	if !ok {
		return nil
	}
	if len(args) != len(template.TypeParams) {
		return fmt.Errorf("строка %d: функция %q ожидает аргументов типа: %d, получено: %d", line, tplName, len(template.TypeParams), len(args))
	}
	subst := make(map[string]string, len(args))
	var checks []constraintCheck
	for i, tp := range template.TypeParams {
		subst[tp.Name] = args[i]
		if len(tp.Constraints) > 0 {
			checks = append(checks, constraintCheck{ParamName: tp.Name, ArgType: args[i], Constraints: tp.Constraints, Line: line})
		}
	}
	synthesized := &ast.FunctionDecl{
		Name:       name,
		Params:     substParams(template.Params, subst),
		ReturnType: substituteTypeString(template.ReturnType, subst),
		Body:       substStmts(template.Body, subst),
		Line:       line,
		File:       template.File,
		Doc:        template.Doc,
	}
	inst.funcs[name] = synthesized
	if len(checks) > 0 {
		inst.constraints[name] = checks
	}
	if err := inst.scanFunc(synthesized); err != nil {
		return err
	}
	inst.funcOrder = append(inst.funcOrder, name)
	return nil
}

func (inst *instantiator) scanEntity(ent *ast.EntityDecl) error {
	for _, f := range ent.Fields {
		if err := inst.ensureEntity(f.Type, f.Line); err != nil {
			return err
		}
	}
	for _, c := range ent.Constants {
		if err := inst.scanExpr(c.Value); err != nil {
			return err
		}
	}
	for _, m := range ent.Methods {
		for _, p := range m.Params {
			if err := inst.ensureEntity(p.Type, m.Line); err != nil {
				return err
			}
		}
		if m.ReturnType != "" {
			if err := inst.ensureEntity(m.ReturnType, m.Line); err != nil {
				return err
			}
		}
		if err := inst.scanStmts(m.Body); err != nil {
			return err
		}
	}
	if ent.Ctor != nil {
		for _, p := range ent.Ctor.Params {
			if err := inst.ensureEntity(p.Type, ent.Ctor.Line); err != nil {
				return err
			}
		}
		if err := inst.scanStmts(ent.Ctor.Body); err != nil {
			return err
		}
	}
	return nil
}

func (inst *instantiator) scanFunc(fn *ast.FunctionDecl) error {
	for _, p := range fn.Params {
		if err := inst.ensureEntity(p.Type, fn.Line); err != nil {
			return err
		}
	}
	if fn.ReturnType != "" {
		if err := inst.ensureEntity(fn.ReturnType, fn.Line); err != nil {
			return err
		}
	}
	return inst.scanStmts(fn.Body)
}

func (inst *instantiator) scanStmts(stmts []ast.Stmt) error {
	for _, s := range stmts {
		if err := inst.scanStmt(s); err != nil {
			return err
		}
	}
	return nil
}

func (inst *instantiator) scanStmt(s ast.Stmt) error {
	switch s := s.(type) {
	case *ast.PrintStmt:
		return inst.scanExpr(s.Value)
	case *ast.ShowStmt:
		return inst.scanExpr(s.Value)
	case *ast.VarDecl:
		return inst.scanExpr(s.Value)
	case *ast.ConstDecl:
		return inst.scanExpr(s.Value)
	case *ast.Assign:
		return inst.scanExpr(s.Value)
	case *ast.FieldAssign:
		if err := inst.scanExpr(s.Receiver); err != nil {
			return err
		}
		return inst.scanExpr(s.Value)
	case *ast.BecomeStmt:
		return inst.scanExpr(s.Receiver)
	case *ast.AssertStmt:
		return inst.scanExpr(s.Cond)
	case *ast.IfStmt:
		if err := inst.scanExpr(s.Cond); err != nil {
			return err
		}
		if err := inst.scanStmts(s.Then); err != nil {
			return err
		}
		return inst.scanStmts(s.Else)
	case *ast.WhileStmt:
		if err := inst.scanExpr(s.Cond); err != nil {
			return err
		}
		return inst.scanStmts(s.Body)
	case *ast.ForStmt:
		if err := inst.scanExpr(s.From); err != nil {
			return err
		}
		if err := inst.scanExpr(s.To); err != nil {
			return err
		}
		return inst.scanStmts(s.Body)
	case *ast.ForEachStmt:
		if err := inst.scanExpr(s.Collection); err != nil {
			return err
		}
		return inst.scanStmts(s.Body)
	case *ast.ReturnStmt:
		if s.Value == nil {
			return nil
		}
		return inst.scanExpr(s.Value)
	case *ast.ExprStmt:
		switch {
		case s.MethodCall != nil:
			return inst.scanExpr(s.MethodCall)
		case s.IncDec != nil:
			return nil
		case s.Await != nil:
			return inst.scanExpr(s.Await)
		default:
			return inst.scanExpr(s.Call)
		}
	case *ast.ThrowStmt:
		return inst.scanExpr(s.Value)
	case *ast.TryStmt:
		if err := inst.scanStmts(s.Body); err != nil {
			return err
		}
		for _, t := range s.CatchTypes {
			if err := inst.ensureEntity(t, s.Line); err != nil {
				return err
			}
		}
		return inst.scanStmts(s.CatchBody)
	default:
		return nil
	}
}

func (inst *instantiator) scanExpr(e ast.Expr) error {
	switch e := e.(type) {
	case *ast.CallExpr:
		for _, a := range e.Args {
			if err := inst.scanExpr(a); err != nil {
				return err
			}
		}
		for _, t := range e.TypeArgs {
			if err := inst.ensureEntity(t, e.Line); err != nil {
				return err
			}
		}
		if len(e.TypeArgs) > 0 {
			return inst.ensureFunc(mangleGenericCallName(e.Callee, e.TypeArgs), e.Line)
		}
		return nil
	case *ast.NewExpr:
		if err := inst.ensureEntity(e.Type, e.Line); err != nil {
			return err
		}
		for _, a := range e.Args {
			if err := inst.scanExpr(a); err != nil {
				return err
			}
		}
		return nil
	case *ast.FieldAccess:
		return inst.scanExpr(e.Receiver)
	case *ast.MethodCallExpr:
		if err := inst.scanExpr(e.Receiver); err != nil {
			return err
		}
		for _, a := range e.Args {
			if err := inst.scanExpr(a); err != nil {
				return err
			}
		}
		return nil
	case *ast.BinaryExpr:
		if err := inst.scanExpr(e.Left); err != nil {
			return err
		}
		return inst.scanExpr(e.Right)
	case *ast.UnaryExpr:
		return inst.scanExpr(e.Operand)
	case *ast.LaunchExpr:
		return inst.scanExpr(e.Call)
	case *ast.AwaitExpr:
		return inst.scanExpr(e.Task)
	case *ast.IsExpr:
		if err := inst.scanExpr(e.Value); err != nil {
			return err
		}
		return inst.ensureEntity(e.Target, e.Line)
	case *ast.TypeOfExpr:
		return inst.ensureEntity(e.TypeName, e.Line)
	case *ast.AsExpr:
		if err := inst.scanExpr(e.Value); err != nil {
			return err
		}
		return inst.ensureEntity(e.TypeName, e.Line)
	case *ast.LambdaExpr:
		for _, p := range e.Params {
			if err := inst.ensureEntity(p.Type, e.Line); err != nil {
				return err
			}
		}
		if e.ReturnType != "" {
			if err := inst.ensureEntity(e.ReturnType, e.Line); err != nil {
				return err
			}
		}
		return inst.scanStmts(e.Body)
	default:
		return nil
	}
}

// checkGenericConstraints - если name (сущность или функция) - инстанциация
// generic-типа с ограничениями (a.genericConstraints, собрано заранее -
// см. package doc), проверяет, что каждый аргумент типа удовлетворяет
// своему ограничению: равен одному из перечисленных типов или связан с
// одним из них наследованием/соответствием интерфейсу (isAssignable, та
// же проверка отношения, что и у присваивания/аргумента вызова). entities/
// methods/aliases - те же карты, что и в месте вызова (в collectEntities
// это ещё локальные, не a.entities - сущность обязана видеть только то,
// что накоплено до неё, см. package doc collectEntities).
func (a *Analyzer) checkGenericConstraints(entities map[string]entitySig, methods map[string]map[string]funcSig, aliases map[string]string, name string) error {
	checks, ok := a.genericConstraints[name]
	if !ok {
		return nil
	}
	for _, c := range checks {
		argType, err := resolveTypeName(entities, methods, aliases, a.interfaces, c.ArgType)
		if err != nil {
			return fmt.Errorf("строка %d: %q: параметр типа %q: %w", c.Line, name, c.ParamName, err)
		}
		satisfied := false
		for _, constraint := range c.Constraints {
			constraintType, err := resolveTypeName(entities, methods, aliases, a.interfaces, constraint)
			if err != nil {
				return fmt.Errorf("строка %d: %q: параметр типа %q: ограничение %q: %w", c.Line, name, c.ParamName, constraint, err)
			}
			if isAssignable(entities, argType, constraintType) {
				satisfied = true
				break
			}
		}
		if !satisfied {
			return fmt.Errorf("строка %d: %q: параметр типа %q = %s не удовлетворяет ограничению (%s)",
				c.Line, name, c.ParamName, argType, strings.Join(c.Constraints, "|"))
		}
	}
	return nil
}

// instantiateGenerics - точка входа: возвращает копию prog, где явные
// использования generic-сущностей/функций добавлены как обычные объявления
// (в порядке зависимостей - см. package doc instantiator), сами шаблоны
// (T - не настоящий тип, не должен доходить до обычного анализа) убраны из
// списка. constraints - ограничения параметров типа каждой инстанциации,
// для отложенной проверки (см. collectEntities/collectFuncSigs) - там, где
// entities/interfaces уже известны, а здесь - ещё нет. prog без единого
// generic-объявления возвращается как есть, без всякой работы.
func instantiateGenerics(prog *ast.Program) (*ast.Program, map[string][]constraintCheck, error) {
	inst := newInstantiator(prog)
	if len(inst.entityTemplates) == 0 && len(inst.funcTemplates) == 0 {
		return prog, nil, nil
	}

	for _, ent := range prog.Entities {
		if len(ent.TypeParams) > 0 {
			continue
		}
		if err := inst.scanEntity(ent); err != nil {
			return nil, nil, err
		}
	}
	for _, fn := range prog.Functions {
		if len(fn.TypeParams) > 0 {
			continue
		}
		if err := inst.scanFunc(fn); err != nil {
			return nil, nil, err
		}
	}

	out := &ast.Program{Uses: prog.Uses, ExternFuncs: prog.ExternFuncs, Enums: prog.Enums, Interfaces: prog.Interfaces}
	for _, ent := range prog.Entities {
		if len(ent.TypeParams) == 0 {
			out.Entities = append(out.Entities, ent)
		}
	}
	for _, fn := range prog.Functions {
		if len(fn.TypeParams) == 0 {
			out.Functions = append(out.Functions, fn)
		}
	}
	for _, name := range inst.entityOrder {
		out.Entities = append(out.Entities, inst.entities[name])
	}
	for _, name := range inst.funcOrder {
		out.Functions = append(out.Functions, inst.funcs[name])
	}
	return out, inst.constraints, nil
}
