package module

import (
	"fmt"
	"strings"

	"github.com/IlyaFeoktistov/Rech/internal/ast"
	"github.com/IlyaFeoktistov/Rech/internal/builtins"
)

// builtinTypeBases - имена типов, встроенных в язык (не требуют ни
// объявления, ни импорта): примитивы, "Файл"/"Сокет"/"БД" (непрозрачные
// дескрипторы), "Запрос"/"Ответ" (HTTP-клиент,
// internal/builtins.ReservedEntityNames), "Исключение" (база для
// "наследует" в любом файле без явного объявления, internal/semantic),
// "Блокировка" (синхронизация между потоками, internal/semantic/mutex.go) и
// три generic-коллекции. У
// generic-инстанциации ("Список<Целое>") аргумент внутри "<...>" сейчас
// может быть только примитивом (internal/semantic.resolveGenericTypeName -
// "элементы встроенных коллекций пока поддерживаются только для
// Строка/Целое/Действительное/Логическое, не для сущностей"), поэтому у
// любой generic-инстанциации, которую вообще можно написать, база уже
// входит сюда - resolveTypeRef ниже этим пользуется, чтобы не резолвить
// аргумент отдельно.
var builtinTypeBases = map[string]bool{
	"Целое":               true,
	"Действительное":      true,
	"Дробное":             true,
	"Символ":              true,
	"Байт":                true,
	"Строка":              true,
	"Логическое":          true,
	"Файл":                true,
	"Сокет":               true,
	"БД":                  true,
	"Запрос":              true,
	"Ответ":               true,
	"Исключение":          true,
	"Список":              true,
	"Стопка":              true,
	"Очередь":             true,
	"Словарь":             true,
	"ДатаВремя":           true,
	"ОшибкаРазбора":       true,
	"ОшибкаДеленияНаНоль": true,
	"Блокировка":          true,
	"ОпцииКуки":           true,
	"Целое8":              true,
	"Целое16":             true,
	"Целое32":             true,
	"Целое64":             true,
	"Натуральное":         true,
	"Натуральное8":        true,
	"Натуральное16":       true,
	"Натуральное32":       true,
	"Натуральное64":       true,
	"Тип":                 true,
	"Значение":            true,
}

// resolveBareName ищет name (имя функции или простого - не generic - типа)
// в names (карта "как написано в файле" -> "итоговое, возможно
// переименованное имя", см. link.go) - встроенные имена (типы из
// builtinTypeBases, функции шага 8 из internal/builtins - ввод консоли,
// математика) возвращаются как есть: они не объявлены ни в одном файле
// Rech-программы и потому не участвуют в переименовании при линковке.
// Не найденное имя - ошибка ("не объявлено и не импортировано").
func resolveBareName(name string, names map[string]string) (string, error) {
	if builtinTypeBases[name] {
		return name, nil
	}
	if builtins.IsReservedName(name) {
		return name, nil
	}
	resolved, ok := names[name]
	if !ok {
		return "", fmt.Errorf("%q не объявлено в этом файле и не импортировано", name)
	}
	return resolved, nil
}

// resolveTypeRef - то же самое для имени типа, включая generic-инстанциации
// ("Список<Целое>") - см. builtinTypeBases про то, почему аргумент внутри
// "<...>" не нужно резолвить отдельно. "?Тип" (nullable, ir.KindNullable) -
// отдельный случай ДО проверки "<": сама nullable-обёртка не объявляется и
// не импортируется (как и "Список"), но её внутренний Тип - обычное имя,
// которое обязано пройти резолюцию/переименование при импорте как любое
// другое (например, "?Пользователь" из another-файла с "как" должно стать
// "?Псевдоним", не остаться нерезолвленным).
func resolveTypeRef(typeName string, names map[string]string) (string, error) {
	if typeName == "" {
		return typeName, nil
	}
	if strings.HasPrefix(typeName, "?") {
		inner, err := resolveTypeRef(typeName[len("?"):], names)
		if err != nil {
			return "", err
		}
		return "?" + inner, nil
	}
	if strings.Contains(typeName, "<") {
		return typeName, nil
	}
	return resolveBareName(typeName, names)
}

// rewriter переписывает каждую ссылку на глобальное имя (вызов функции,
// имя типа) в дереве по names - см. link.go, buildNames. typeParams - имена
// параметров типа generic-сущности/функции, чьи поля/параметры/тип
// результата сейчас переписываются - локальные заглушки ("T"), не
// ссылки на объявленный где-то тип, поэтому исключены из резолюции;
// выставляется/сбрасывается вызывающим кодом (link.go) перед/после каждого
// generic-объявления, nil - вне такого объявления (обычная резолюция).
type rewriter struct {
	names      map[string]string
	typeParams map[string]bool
	// locals - имена, объявленные внутри тела функции/метода, которое
	// сейчас переписывается (параметры, "переменная"/"константа",
	// переменная цикла "для"/"для...в", "поймать(...)") - у локальной
	// переменной типа "Вызываемый", вызванной как "ф(...)" ("ф" не найдено
	// в names - это не файловая декларация), отсутствие записи в names не
	// ошибка видимости, в отличие от истинно необъявленного/
	// неимпортированного имени - см. CallExpr case в rewriteExpr.
	// Сбрасывается вызывающим кодом (link.go) перед каждым новым телом
	// функции/метода верхнего уровня - не настоящая вложенная область
	// видимости (переменная одной ветки "если" считается локальной для
	// всего тела), но ложноположительный исход (пропущенная ошибка
	// видимости при случайном совпадении имени) безопаснее
	// ложноотрицательного (сломанный вызов значения).
	locals map[string]bool
}

func (r *rewriter) addLocal(name string) {
	if r.locals == nil {
		r.locals = map[string]bool{}
	}
	r.locals[name] = true
}

// typeParamSet - имена параметров типа params в виде множества, для
// rewriter.typeParams - nil, если params пуст (не generic-объявление).
func typeParamSet(params []ast.GenericParam) map[string]bool {
	if len(params) == 0 {
		return nil
	}
	set := make(map[string]bool, len(params))
	for _, p := range params {
		set[p.Name] = true
	}
	return set
}

// resolveType - resolveTypeRef, но пропускающий типы, равные одному из
// typeParams как есть (см. package doc rewriter).
func (r *rewriter) resolveType(typeName string) (string, error) {
	if r.typeParams[typeName] {
		return typeName, nil
	}
	return resolveTypeRef(typeName, r.names)
}

func (r *rewriter) rewriteParams(params []ast.Param) error {
	for i, p := range params {
		resolved, err := r.resolveType(p.Type)
		if err != nil {
			return fmt.Errorf("параметр %q: %w", p.Name, err)
		}
		params[i].Type = resolved
		r.addLocal(p.Name)
	}
	return nil
}

func (r *rewriter) rewriteFields(fields []ast.Field) error {
	for i, f := range fields {
		resolved, err := r.resolveType(f.Type)
		if err != nil {
			return fmt.Errorf("поле %q: %w", f.Name, err)
		}
		fields[i].Type = resolved
	}
	return nil
}

func (r *rewriter) rewriteReturnType(returnType string) (string, error) {
	return r.resolveType(returnType)
}

// rewriteTypeParams переписывает имена типов внутри ограничений параметров
// типа generic-сущности/функции - сами имена параметров (T, U)
// НЕ переписываются (это не ссылки на объявленные где-то типы, а
// локальные заглушки, вводимые прямо здесь).
func (r *rewriter) rewriteTypeParams(params []ast.GenericParam) error {
	for i, p := range params {
		for j, c := range p.Constraints {
			resolved, err := resolveTypeRef(c, r.names)
			if err != nil {
				return fmt.Errorf("параметр типа %q: %w", p.Name, err)
			}
			params[i].Constraints[j] = resolved
		}
	}
	return nil
}

func (r *rewriter) rewriteStmts(stmts []ast.Stmt) error {
	for _, s := range stmts {
		if err := r.rewriteStmt(s); err != nil {
			return err
		}
	}
	return nil
}

func (r *rewriter) rewriteStmt(s ast.Stmt) error {
	switch s := s.(type) {
	case *ast.PrintStmt:
		return r.rewriteExpr(s.Value)
	case *ast.ShowStmt:
		return r.rewriteExpr(s.Value)
	case *ast.VarDecl:
		r.addLocal(s.Name)
		return r.rewriteExpr(s.Value)
	case *ast.ConstDecl:
		r.addLocal(s.Name)
		return r.rewriteExpr(s.Value)
	case *ast.Assign:
		return r.rewriteExpr(s.Value)
	case *ast.NullCoalesceAssign:
		return r.rewriteExpr(s.Value)
	case *ast.FieldAssign:
		if err := r.rewriteExpr(s.Receiver); err != nil {
			return err
		}
		return r.rewriteExpr(s.Value)
	case *ast.BecomeStmt:
		return r.rewriteExpr(s.Receiver)
	case *ast.DerefAssignStmt:
		if err := r.rewriteExpr(s.Pointer); err != nil {
			return err
		}
		return r.rewriteExpr(s.Value)
	case *ast.AssertStmt:
		return r.rewriteExpr(s.Cond)
	case *ast.IfStmt:
		if err := r.rewriteExpr(s.Cond); err != nil {
			return err
		}
		if err := r.rewriteStmts(s.Then); err != nil {
			return err
		}
		return r.rewriteStmts(s.Else)
	case *ast.WhileStmt:
		if err := r.rewriteExpr(s.Cond); err != nil {
			return err
		}
		return r.rewriteStmts(s.Body)
	case *ast.ForStmt:
		r.addLocal(s.Var)
		if err := r.rewriteExpr(s.From); err != nil {
			return err
		}
		if err := r.rewriteExpr(s.To); err != nil {
			return err
		}
		return r.rewriteStmts(s.Body)
	case *ast.ForEachStmt:
		r.addLocal(s.Var)
		if err := r.rewriteExpr(s.Collection); err != nil {
			return err
		}
		return r.rewriteStmts(s.Body)
	case *ast.ReturnStmt:
		if s.Value == nil {
			return nil
		}
		return r.rewriteExpr(s.Value)
	case *ast.ExprStmt:
		if s.Call != nil {
			return r.rewriteExpr(s.Call)
		}
		if s.MethodCall != nil {
			return r.rewriteExpr(s.MethodCall)
		}
		if s.Await != nil {
			return r.rewriteExpr(s.Await)
		}
		return nil
	case *ast.ThrowStmt:
		return r.rewriteExpr(s.Value)
	case *ast.TryStmt:
		if s.CatchVar != "" {
			r.addLocal(s.CatchVar)
		}
		if err := r.rewriteStmts(s.Body); err != nil {
			return err
		}
		for i, t := range s.CatchTypes {
			resolved, err := resolveTypeRef(t, r.names)
			if err != nil {
				return fmt.Errorf("\"поймать(...)\": %w", err)
			}
			s.CatchTypes[i] = resolved
		}
		return r.rewriteStmts(s.CatchBody)
	case *ast.GerundStmt:
		resolved, err := resolveBareName(s.Callee, r.names)
		if err != nil {
			if !r.locals[s.Callee] {
				return fmt.Errorf("строка %d: %w", s.Line, err)
			}
		} else {
			s.Callee = resolved
		}
		if err := r.rewriteExprs(s.Args); err != nil {
			return err
		}
		return r.rewriteStmts(s.Body)
	case *ast.SwitchStmt:
		if err := r.rewriteExpr(s.Subject); err != nil {
			return err
		}
		for _, c := range s.Cases {
			// Значения ветки "когда" - как и везде в этом rewriter'е, голое
			// имя (*ast.Identifier) не переписывается (см. case *ast.Identifier
			// в rewriteExpr ниже) - для типа, объявленного тут же в файле,
			// этого достаточно (resolveTypeName в internal/semantic ищет по
			// тому же голому имени); импортированный тип, сопоставляемый по
			// типу в "когда" (а не по значению), в этом срезе не
			// квалифицируется - как и голые Identifier-значения нигде в этом
			// rewriter'е не квалифицируются.
			if err := r.rewriteExprs(c.Values); err != nil {
				return err
			}
			if err := r.rewriteStmts(c.Body); err != nil {
				return err
			}
		}
		return r.rewriteStmts(s.Default)
	default:
		return fmt.Errorf("внутренняя ошибка: неизвестная инструкция %T", s)
	}
}

func (r *rewriter) rewriteExprs(exprs []ast.Expr) error {
	for _, e := range exprs {
		if err := r.rewriteExpr(e); err != nil {
			return err
		}
	}
	return nil
}

func (r *rewriter) rewriteExpr(e ast.Expr) error {
	switch e := e.(type) {
	case *ast.StringLiteral, *ast.IntLiteral, *ast.FloatLiteral, *ast.BoolLiteral, *ast.CharLiteral, *ast.NullLiteral, *ast.Identifier, *ast.ThisExpr, *ast.IncDecExpr:
		return nil
	case *ast.CallExpr:
		// Callee здесь может называть либо объявленную функцию (обычный
		// случай), либо локальную переменную типа "Вызываемый"
		// ("ф(<арг>, ...)") - у локальных имён нет записи в r.names (это
		// не файловая/импортированная декларация), и в отличие от истинно
		// необъявленного/неимпортированного имени, отсутствие записи
		// здесь не ошибка, если имя есть среди r.locals (см. package doc
		// rewriter.locals).
		resolved, err := resolveBareName(e.Callee, r.names)
		if err != nil {
			if !r.locals[e.Callee] {
				return fmt.Errorf("строка %d: %w", e.Line, err)
			}
		} else {
			e.Callee = resolved
		}
		for i, t := range e.TypeArgs {
			resolvedType, err := resolveTypeRef(t, r.names)
			if err != nil {
				return fmt.Errorf("строка %d: аргумент типа: %w", e.Line, err)
			}
			e.TypeArgs[i] = resolvedType
		}
		return r.rewriteExprs(e.Args)
	case *ast.NewExpr:
		resolved, err := r.resolveType(e.Type)
		if err != nil {
			return fmt.Errorf("строка %d: %w", e.Line, err)
		}
		e.Type = resolved
		return r.rewriteExprs(e.Args)
	case *ast.FieldAccess:
		return r.rewriteExpr(e.Receiver)
	case *ast.IndexExpr:
		if err := r.rewriteExpr(e.Receiver); err != nil {
			return err
		}
		return r.rewriteExpr(e.Index)
	case *ast.ListLiteral:
		return r.rewriteExprs(e.Elems)
	case *ast.MethodCallExpr:
		if err := r.rewriteExpr(e.Receiver); err != nil {
			return err
		}
		return r.rewriteExprs(e.Args)
	case *ast.BinaryExpr:
		if err := r.rewriteExpr(e.Left); err != nil {
			return err
		}
		return r.rewriteExpr(e.Right)
	case *ast.UnaryExpr:
		return r.rewriteExpr(e.Operand)
	case *ast.CastExpr:
		return r.rewriteExpr(e.Value)
	case *ast.ParseExpr:
		resolved, err := r.resolveType(e.TypeName)
		if err != nil {
			return fmt.Errorf("строка %d: %w", e.Line, err)
		}
		e.TypeName = resolved
		return r.rewriteExpr(e.Source)
	case *ast.SerializeExpr:
		return r.rewriteExpr(e.Value)
	case *ast.IsExpr:
		resolved, err := r.resolveType(e.Target)
		if err != nil {
			return fmt.Errorf("строка %d: %w", e.Line, err)
		}
		e.Target = resolved
		return r.rewriteExpr(e.Value)
	case *ast.LaunchExpr:
		return r.rewriteExpr(e.Call)
	case *ast.AwaitExpr:
		return r.rewriteExpr(e.Task)
	case *ast.FuncRefExpr:
		// "&имя" называет либо объявленную функцию (обычный случай -
		// ссылка на неё как на "Вызываемый"), либо локальную переменную
		// (адрес её значения, "Указатель<T>", docs/SPEC.md, "Указатели") -
		// тот же принцип исключения через r.locals, что и у CallExpr.Callee
		// выше (у локальных имён нет записи в r.names).
		resolved, err := resolveBareName(e.Name, r.names)
		if err != nil {
			if !r.locals[e.Name] {
				return fmt.Errorf("строка %d: %w", e.Line, err)
			}
			return nil
		}
		e.Name = resolved
		return nil
	case *ast.AddressOfExpr:
		// Receiver - всегда простое имя локальной переменной/параметра
		// (docs/SPEC.md, "Указатели") - как и у FuncRefExpr.Name в случае
		// локальной переменной выше, переименованию не подлежит. Index -
		// произвольное выражение (может ссылаться на импортированное имя).
		if e.Index != nil {
			return r.rewriteExpr(e.Index)
		}
		return nil
	case *ast.DerefExpr:
		return r.rewriteExpr(e.Value)
	case *ast.TypeOfExpr:
		resolved, err := r.resolveType(e.TypeName)
		if err != nil {
			return fmt.Errorf("строка %d: %w", e.Line, err)
		}
		e.TypeName = resolved
		return nil
	case *ast.AsExpr:
		resolved, err := r.resolveType(e.TypeName)
		if err != nil {
			return fmt.Errorf("строка %d: %w", e.Line, err)
		}
		e.TypeName = resolved
		return r.rewriteExpr(e.Value)
	case *ast.LambdaExpr:
		if err := r.rewriteParams(e.Params); err != nil {
			return fmt.Errorf("строка %d: лямбда: %w", e.Line, err)
		}
		resolved, err := r.rewriteReturnType(e.ReturnType)
		if err != nil {
			return fmt.Errorf("строка %d: лямбда: %w", e.Line, err)
		}
		e.ReturnType = resolved
		return r.rewriteStmts(e.Body)
	default:
		return fmt.Errorf("внутренняя ошибка: неизвестное выражение %T", e)
	}
}
