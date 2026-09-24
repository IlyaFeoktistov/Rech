// Вывод аргументов типа generic-функции/сущности из типов фактических
// аргументов: "первый(список)" вместо "первый<Целое>(список)", "новая
// Коробка(5)" вместо "новая Коробка<Целое>(5)" (docs/SPEC.md, раздел 16).
//
// Мономорфизация (generics.go) - проход по AST ДО анализа типов, а типы
// аргументов известны только во время анализа. Поэтому вывод - через
// перезапуск: анализ, встретив вызов шаблона без "<...>", выводит
// аргументы типа из типов аргументов вызова, записывает их в таблицу
// (genericInference.table - по ключу "объявление#номер узла", узел
// нумеруется один раз и сохраняет номер в копиях тела шаблона) и просит
// перезапуск; следующий проход мономорфизации создаёт нужную инстанциацию
// по таблице, следующий анализ находит её как обычную. Вложенные выводы
// (аргумент зависит от другого вывода, вывод внутри тела шаблона)
// разрешаются за несколько проходов.
package semantic

import (
	"errors"
	"fmt"
	"reflect"
	"strconv"
	"strings"

	"github.com/IlyaFeoktistov/Rech/internal/ast"
	"github.com/IlyaFeoktistov/Rech/internal/ir"
)

// maxInferencePasses - предел перезапусков анализа ради вывода типов.
const maxInferencePasses = 32

// errInferRetry - вывод записал новые аргументы типа: результат этого
// прохода анализа не окончательный.
var errInferRetry = errors.New("вывод аргументов типа: нужен повторный проход")

type genericInference struct {
	table           map[string][]string
	found           bool
	funcTemplates   map[string]*ast.FunctionDecl
	entityTemplates map[string]*ast.EntityDecl
}

func inferKey(scope string, id int) string { return scope + "#" + strconv.Itoa(id) }

// numberCallNodes раздаёт номера (ast.CallExpr.ID/NewExpr.ID) узлам без
// номера - один раз: номер хранится в самом узле и переживает
// перезапуски анализа.
func numberCallNodes(prog *ast.Program) {
	next := 1
	walkCallNodeIDs(prog, func(id *int) {
		if *id >= next {
			next = *id + 1
		}
	})
	walkCallNodeIDs(prog, func(id *int) {
		if *id == 0 {
			*id = next
			next++
		}
	})
}

// walkCallNodeIDs зовёт visit для поля ID каждого CallExpr/NewExpr prog.
func walkCallNodeIDs(prog *ast.Program, visit func(id *int)) {
	seen := map[uintptr]bool{}
	var walk func(v reflect.Value)
	walk = func(v reflect.Value) {
		switch v.Kind() {
		case reflect.Pointer:
			if v.IsNil() || seen[v.Pointer()] {
				return
			}
			seen[v.Pointer()] = true
			switch n := v.Interface().(type) {
			case *ast.CallExpr:
				visit(&n.ID)
			case *ast.NewExpr:
				visit(&n.ID)
			}
			walk(v.Elem())
		case reflect.Interface:
			if !v.IsNil() {
				walk(v.Elem())
			}
		case reflect.Slice:
			for i := range v.Len() {
				walk(v.Index(i))
			}
		case reflect.Struct:
			for i := range v.NumField() {
				if v.Type().Field(i).IsExported() {
					walk(v.Field(i))
				}
			}
		}
	}
	walk(reflect.ValueOf(prog))
}

// unifyTypes сопоставляет тип параметра шаблона param (строка, с именами
// параметров типа) с типом аргумента arg (каноническая строка ir.Type) и
// записывает найденные значения параметров в bindings.
func unifyTypes(param, arg string, params map[string]bool, bindings map[string]string) error {
	if params[param] {
		if prev, ok := bindings[param]; ok && prev != arg {
			return fmt.Errorf("параметр типа %s неоднозначен: %s и %s", param, prev, arg)
		}
		bindings[param] = arg
		return nil
	}
	if rest, ok := strings.CutPrefix(param, "?"); ok {
		return unifyTypes(rest, strings.TrimPrefix(arg, "?"), params, bindings)
	}
	if pp, pr, ok := splitCallableTypeString(param); ok {
		ap, ar, ok := splitCallableTypeString(arg)
		if !ok || len(ap) != len(pp) {
			return nil
		}
		for i := range pp {
			if err := unifyTypes(pp[i], ap[i], params, bindings); err != nil {
				return err
			}
		}
		if pr != "" && ar != "" {
			return unifyTypes(pr, ar, params, bindings)
		}
		return nil
	}
	ptpl, pargs, ok := ParseGenericTypeNameMulti(param)
	if !ok {
		return nil
	}
	atpl, aargs, ok := ParseGenericTypeNameMulti(arg)
	if !ok || atpl != ptpl || len(aargs) != len(pargs) {
		return nil
	}
	for i := range pargs {
		if err := unifyTypes(pargs[i], aargs[i], params, bindings); err != nil {
			return err
		}
	}
	return nil
}

// inferTypeArgs - аргументы типа шаблона (typeParams) по типам параметров
// paramTypes и фактическим аргументам args вызова. what - "функция
// \"имя\""/"сущность \"Имя\"" для сообщений.
func inferTypeArgs(sc *ctx, typeParams []ast.GenericParam, paramTypes []string, args []ast.Expr, what, explicitHint string, line int) ([]string, error) {
	params := make(map[string]bool, len(typeParams))
	for _, tp := range typeParams {
		params[tp.Name] = true
	}
	bindings := map[string]string{}
	for i, a := range args {
		if i >= len(paramTypes) {
			break
		}
		value, err := analyzeExpr(sc, a)
		if err != nil {
			return nil, err
		}
		if err := unifyTypes(paramTypes[i], ir.TypeOf(value).String(), params, bindings); err != nil {
			return nil, errAtLine(line, CodeGenerics, "%s: %s - укажите аргументы типа явно: %s", what, err, explicitHint)
		}
	}
	out := make([]string, len(typeParams))
	for i, tp := range typeParams {
		t, ok := bindings[tp.Name]
		if !ok {
			return nil, errAtLine(line, CodeGenerics, "%s: не удалось вывести параметр типа %s из аргументов - укажите аргументы типа явно: %s", what, tp.Name, explicitHint)
		}
		out[i] = t
	}
	return out, nil
}

// inferredCallee - для вызова шаблона функции без "<...>" мангл-имя
// инстанциации ("первый<Целое>"); ok == false - вызов не такой.
// errInferRetry - аргументы только что выведены, инстанциации ещё нет.
func inferredCallee(sc *ctx, e *ast.CallExpr) (string, bool, error) {
	inf := sc.a.inference
	if inf == nil || len(e.TypeArgs) > 0 {
		return "", false, nil
	}
	tpl, ok := inf.funcTemplates[e.Callee]
	if !ok {
		return "", false, nil
	}
	if _, isVar := sc.lookup(e.Callee); isVar {
		return "", false, nil
	}
	key := inferKey(sc.inferScope, e.ID)
	typeArgs, known := inf.table[key]
	if !known {
		paramTypes, args := variadicInferArgs(tpl.Params, e.Args)
		var err error
		typeArgs, err = inferTypeArgs(sc, tpl.TypeParams, paramTypes, args, fmt.Sprintf("функция %q", e.Callee), e.Callee+"<...>(...)", e.Line)
		if err != nil {
			return "", false, err
		}
		inf.table[key] = typeArgs
	}
	callee := mangleGenericCallName(e.Callee, typeArgs)
	if _, ok := sc.funcs[callee]; !ok {
		inf.found = true
		return "", false, inferPlaceholder(sc, tpl, typeArgs, callee)
	}
	return callee, true, nil
}

// placeholderCall - вызов инстанциации, которой в этом проходе ещё нет:
// анализ продолжается с ним (тип результата известен), чтобы остальные
// выводы в том же объявлении нашлись в этом же проходе; сам проход не
// окончательный (inf.found).
type placeholderCall struct{ call *ir.CallExpr }

func (placeholderCall) Error() string { return errInferRetry.Error() }

// inferPlaceholder - placeholderCall, если тип результата инстанциации
// вычислим без неё самой, иначе errInferRetry.
func inferPlaceholder(sc *ctx, tpl *ast.FunctionDecl, typeArgs []string, callee string) error {
	call := &ir.CallExpr{Callee: callee}
	if tpl.ReturnType != "" {
		subst := make(map[string]string, len(typeArgs))
		for i, tp := range tpl.TypeParams {
			subst[tp.Name] = typeArgs[i]
		}
		t, err := resolveTypeName(sc.entities, sc.methods, sc.aliases, sc.interfaces, sc.enums, substituteTypeString(tpl.ReturnType, subst))
		if err != nil {
			return errInferRetry
		}
		call.Type, call.HasReturn = t, true
	}
	return placeholderCall{call: call}
}

// inferredNewType - то же для "новый Шаблон(...)" без "<...>": типы
// параметров конструктора, без явного конструктора - поля по порядку (или
// по именам FieldNames).
func inferredNewType(sc *ctx, e *ast.NewExpr) (string, bool, error) {
	inf := sc.a.inference
	if inf == nil {
		return "", false, nil
	}
	tpl, ok := inf.entityTemplates[e.Type]
	if !ok {
		return "", false, nil
	}
	key := inferKey(sc.inferScope, e.ID)
	typeArgs, known := inf.table[key]
	if !known {
		var paramTypes []string
		switch {
		case tpl.Ctor != nil:
			for _, p := range tpl.Ctor.Params {
				paramTypes = append(paramTypes, p.Type)
			}
		case len(e.FieldNames) > 0:
			for _, name := range e.FieldNames {
				for _, f := range tpl.Fields {
					if f.Name == name {
						paramTypes = append(paramTypes, f.Type)
					}
				}
			}
		default:
			for _, f := range tpl.Fields {
				if !f.IsStatic {
					paramTypes = append(paramTypes, f.Type)
				}
			}
		}
		var err error
		typeArgs, err = inferTypeArgs(sc, tpl.TypeParams, paramTypes, e.Args, fmt.Sprintf("сущность %q", e.Type), e.Type+"<...>(...)", e.Line)
		if err != nil {
			return "", false, err
		}
		inf.table[key] = typeArgs
	}
	name := mangleGenericCallName(e.Type, typeArgs)
	if _, ok := sc.entities[name]; !ok {
		inf.found = true
		return "", false, errInferRetry
	}
	return name, true, nil
}
