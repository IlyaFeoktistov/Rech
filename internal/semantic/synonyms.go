// Синонимы типов ("синоним Имя = Тип", "синоним Имя<T> = Тип") - другое
// имя того же самого типа. Раскрываются в самом начале Analyze, до
// инстанциации generic'ов: каждое поле AST, несущее имя типа,
// переписывается на месте в цель синонима, дальше по конвейеру синонимов
// нет вовсе (ни в IR, ни в backend'ах). Раскрытие идемпотентно - повторный
// Analyze того же AST ничего не меняет.
package semantic

import (
	"fmt"
	"reflect"
	"strings"

	"github.com/IlyaFeoktistov/Rech/internal/ast"
	"github.com/IlyaFeoktistov/Rech/internal/builtins"
	"github.com/IlyaFeoktistov/Rech/internal/ir"
)

// maxSynonymDepth - предел вложенности раскрытия; превышение - цикл
// синонимов (A = Список<B>, B = A).
const maxSynonymDepth = 64

// synonymTypeFields - имена полей узлов AST, несущих имя типа (string или
// []string). ast.StateDecl.TypeName - имя, синтезированное парсером, не
// ссылка на тип - пропускается отдельно (см. expandSynonymsIn).
var synonymTypeFields = map[string]bool{
	"Type": true, "ReturnType": true, "TypeName": true, "Target": true,
	"CatchTypes": true, "TypeArgs": true, "Constraints": true,
}

type synonymExpander struct {
	decls map[string]*ast.SynonymDecl
}

// expandSynonyms проверяет объявления синонимов prog и раскрывает все их
// использования в prog.
func expandSynonyms(prog *ast.Program) error {
	if len(prog.Synonyms) == 0 {
		return nil
	}
	x := &synonymExpander{decls: map[string]*ast.SynonymDecl{}}
	taken := map[string]bool{}
	for _, e := range prog.Entities {
		taken[e.Name] = true
	}
	for _, i := range prog.Interfaces {
		taken[i.Name] = true
	}
	for _, e := range prog.Enums {
		taken[e.Name] = true
	}
	for _, e := range prog.ExternEntities {
		taken[e.Alias] = true
	}
	for _, s := range prog.Synonyms {
		if _, dup := x.decls[s.Name]; dup {
			return errAtLine(s.Line, CodeDuplicateDecl, "синоним %q объявлен более одного раза", s.Name)
		}
		if _, prim := primitiveTypes[s.Name]; prim || taken[s.Name] || builtins.IsReservedName(s.Name) || s.Name == ir.CallableTypeName {
			return errAtLine(s.Line, CodeSynonym, "синоним %q: имя уже занято типом", s.Name)
		}
		x.decls[s.Name] = s
	}
	for _, s := range prog.Synonyms {
		params := map[string]string{}
		for _, p := range s.TypeParams {
			params[p] = p
		}
		if isNullable := strings.HasPrefix(s.Target, "?"); isNullable != s.Nullable {
			if isNullable {
				return errAtLine(s.Line, CodeSynonym, "синоним %q: цель - nullable-тип, синоним тоже пишется с \"?\"", s.Name).
					WithHint("синоним ?%s = %s", s.Name, s.Target)
			}
			return errAtLine(s.Line, CodeSynonym, "синоним \"?%s\": цель %q - не nullable-тип", s.Name, s.Target).
				WithHint("синоним ?%s = ?%s", s.Name, s.Target)
		}
		target, err := x.expand(s.Target, 0, params)
		if err != nil {
			return errAtLine(s.Line, CodeSynonym, "синоним %q: %s", s.Name, err)
		}
		s.Target = target
	}
	return x.walk(reflect.ValueOf(prog), 0)
}

// expand раскрывает все синонимы в строке имени типа s. params -
// параметры generic-синонима, чья цель сейчас раскрывается, -> их
// аргументы (уже раскрытые, подставляются как есть): параметр - не
// синоним, даже если совпадает с именем синонима.
func (x *synonymExpander) expand(s string, depth int, params map[string]string) (string, error) {
	return x.expandNode(s, depth, params, false)
}

// expandNode - expand для s, перед которым стоял "?" (nullable = true) или
// нет. Nullable-синоним ("синоним ?Имя = ?Тип") пишется только с "?", и
// этот "?" не удваивается: "?Имя" раскрывается ровно в цель.
func (x *synonymExpander) expandNode(s string, depth int, params map[string]string, nullable bool) (string, error) {
	if depth > maxSynonymDepth {
		return "", fmt.Errorf("синонимы ссылаются друг на друга по кругу")
	}
	wrap := func(t string) string {
		if nullable {
			return "?" + t
		}
		return t
	}
	if s == "" {
		return s, nil
	}
	if arg, ok := params[s]; ok {
		return wrap(arg), nil
	}
	if rest, ok := strings.CutPrefix(s, "?"); ok {
		if nullable {
			return wrap(s), nil
		}
		return x.expandNode(rest, depth, params, true)
	}
	if ps, result, ok := splitCallableTypeString(s); ok {
		out := make([]string, len(ps))
		for i, p := range ps {
			e, err := x.expand(p, depth, params)
			if err != nil {
				return "", err
			}
			out[i] = e
		}
		res, err := x.expand(result, depth, params)
		if err != nil {
			return "", err
		}
		return wrap(fmt.Sprintf("%s<%s>%s", ir.CallableTypeName, strings.Join(out, ", "), res)), nil
	}
	tpl, args, generic := ParseGenericTypeNameMulti(s)
	if !generic {
		tpl = s
	}
	var expArgs []string
	for _, a := range args {
		e, err := x.expand(a, depth, params)
		if err != nil {
			return "", err
		}
		expArgs = append(expArgs, e)
	}
	decl, isSyn := x.decls[tpl]
	if !isSyn {
		if !generic {
			return wrap(s), nil
		}
		return wrap(tpl + "<" + strings.Join(expArgs, ", ") + ">"), nil
	}
	if len(expArgs) != len(decl.TypeParams) {
		if len(decl.TypeParams) == 0 {
			return "", fmt.Errorf("синоним %q не принимает аргументов типа", tpl)
		}
		return "", fmt.Errorf("синоним %q ожидает %d аргумент(а) типа (%s), получено %d", tpl, len(decl.TypeParams), strings.Join(decl.TypeParams, ", "), len(expArgs))
	}
	subst := make(map[string]string, len(expArgs))
	for i, p := range decl.TypeParams {
		subst[p] = expArgs[i]
	}
	if decl.Nullable {
		if !nullable {
			return "", fmt.Errorf("синоним %q обозначает nullable-тип - пишите \"?%s\"", tpl, s)
		}
		return x.expandNode(decl.Target, depth+1, subst, false)
	}
	return x.expandNode(decl.Target, depth+1, subst, nullable)
}

// walk обходит узел AST (через reflect - полей с именами типов слишком
// много в разных узлах, чтобы перечислять их вручную) и переписывает
// поля из synonymTypeFields. line - строка ближайшего объемлющего узла,
// для позиции ошибки.
func (x *synonymExpander) walk(v reflect.Value, line int) error {
	switch v.Kind() {
	case reflect.Pointer, reflect.Interface:
		if v.IsNil() {
			return nil
		}
		return x.walk(v.Elem(), line)
	case reflect.Slice:
		for i := range v.Len() {
			if err := x.walk(v.Index(i), line); err != nil {
				return err
			}
		}
	case reflect.Struct:
		if v.Type() == reflect.TypeFor[ast.SynonymDecl]() {
			return nil
		}
		if f := v.FieldByName("Line"); f.IsValid() && f.Kind() == reflect.Int && f.Int() > 0 {
			line = int(f.Int())
		}
		isState := v.Type() == reflect.TypeFor[ast.StateDecl]()
		t := v.Type()
		for i := range v.NumField() {
			f := v.Field(i)
			name := t.Field(i).Name
			if !t.Field(i).IsExported() {
				continue
			}
			if synonymTypeFields[name] && !(isState && name == "TypeName") {
				if err := x.rewriteField(f, line); err != nil {
					return err
				}
				continue
			}
			if err := x.walk(f, line); err != nil {
				return err
			}
		}
	}
	return nil
}

func (x *synonymExpander) rewriteField(f reflect.Value, line int) error {
	switch {
	case f.Kind() == reflect.String:
		e, err := x.expand(f.String(), 0, nil)
		if err != nil {
			return errAtLine(line, CodeSynonym, "%s", err)
		}
		f.SetString(e)
	case f.Kind() == reflect.Slice && f.Type().Elem().Kind() == reflect.String:
		for i := range f.Len() {
			if err := x.rewriteField(f.Index(i), line); err != nil {
				return err
			}
		}
	default:
		// Поле с тем же именем, но не строковое (например, Type у узла,
		// где это вложенная структура) - обычный обход.
		return x.walk(f, line)
	}
	return nil
}
