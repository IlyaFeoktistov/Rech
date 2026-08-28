// Package interp: рефлексия ("тип <Имя>", "значение как <Имя>", методы
// встроенных "Значение"/"Тип") - см. docs/SPEC.md и internal/semantic/
// reflect.go (package doc). Представление здесь проще, чем в cbackend
// (internal/backend/cbackend/reflect.go): "Тип" - просто имя типа строкой
// (Value.S), "Значение" - указатель на настоящее боксированное значение
// (Value.Boxed) - вся нужная информация о сущностях (поля/методы) уже
// лежит в in.entities, отдельные таблицы метаданных не нужны.
package interp

import (
	"fmt"

	"github.com/IlyaFeoktistov/Rech/internal/ir"
	"github.com/IlyaFeoktistov/Rech/internal/rtmsg"
	"github.com/IlyaFeoktistov/Rech/internal/structtag"
)

// typeInfoValue строит "Тип" для t - имя берётся из ir.Type.String()
// (для KindEntity это и есть t.Entity, см. его определение).
func typeInfoValue(t ir.Type) Value {
	return Value{Type: ir.TypeTypeInfo, S: t.String()}
}

// evalTypeOfExpr - "тип <Имя>" (e.Type уже разрешён семантическим
// анализом в конкретный ir.Type).
func evalTypeOfExpr(e *ir.TypeOfExpr) Value {
	return typeInfoValue(e.Type)
}

// evalAsExpr - "<значение> как <Тип>": разбоксирование "Значение" -
// паника при несовпадении фактического типа боксированного значения с e.To
// (тот же принцип, что и у прочих несовпадений во время явных кастов,
// internal/rtmsg).
func (in *Interp) evalAsExpr(e *ir.AsExpr, env *Env, cc *callCtx) (Value, error) {
	v, err := in.evalExpr(e.Value, env, cc)
	if err != nil {
		return Value{}, err
	}
	if v.Boxed.Type != e.To {
		panicRuntime("%s", rtmsg.AsTypeMismatchFormat(v.Boxed.Type.String(), e.To.String()))
	}
	return *v.Boxed, nil
}

// effectiveMethods - методы typeName, включая унаследованные (в отличие
// от ent.Methods, который несёт только новые/переопределённые) - обходит
// цепочку BaseName от typeName (самый производный) к корню, метод
// производной сущности с тем же именем перекрывает метод базовой (первое
// вхождение имени в этом порядке обхода и есть эффективная версия).
func (in *Interp) effectiveMethods(typeName string) []*ir.Method {
	var chain []string
	for n := typeName; n != ""; {
		ent, ok := in.entities[n]
		if !ok {
			break
		}
		chain = append(chain, n)
		n = ent.BaseName
	}
	seen := map[string]bool{}
	var result []*ir.Method
	for _, n := range chain {
		for _, m := range in.entities[n].Methods {
			if seen[m.Name] {
				continue
			}
			seen[m.Name] = true
			result = append(result, m)
		}
	}
	return result
}

// evalTypeInfoMethod - методы "Тип" (docs/SPEC.md) - recv.S несёт имя
// типа, число_полей/имя_поля/тип_поля читают in.entities[recv.S].Fields
// напрямую (уже сплющены с унаследованными, см. package doc ir.Entity.
// Fields), число_методов/... - через effectiveMethods выше. Примитив или
// синглтон "Пусто" (тип результата метода без результата) - in.entities не
// содержит такого имени, счётчики полей/методов для них равны 0.
func (in *Interp) evalTypeInfoMethod(recv Value, e *ir.MethodCallExpr, env *Env, cc *callCtx) (Value, error) {
	args, err := in.evalArgs(e.Args, env, cc)
	if err != nil {
		return Value{}, err
	}
	ent := in.entities[recv.S]
	switch e.Method {
	case "имя":
		return stringValue(recv.S), nil
	case "число_полей":
		if ent == nil {
			return intValue(0), nil
		}
		return intValue(int64(len(ent.Fields))), nil
	case "имя_поля":
		f := indexField(ent, args[0].I, recv.S)
		return stringValue(f.Name), nil
	case "тип_поля":
		f := indexField(ent, args[0].I, recv.S)
		return typeInfoValue(f.Type), nil
	case "тег_поля":
		f := indexField(ent, args[0].I, recv.S)
		return stringValue(structtag.Parse(f.Tag).Get(args[1].S)), nil
	case "число_методов":
		if ent == nil {
			return intValue(0), nil
		}
		return intValue(int64(len(in.effectiveMethods(recv.S)))), nil
	case "имя_метода":
		m := indexMethod(in.effectiveMethods(recv.S), args[0].I, recv.S)
		return stringValue(m.Name), nil
	case "число_параметров_метода":
		m := indexMethod(in.effectiveMethods(recv.S), args[0].I, recv.S)
		return intValue(int64(len(m.Params))), nil
	case "тип_параметра_метода":
		m := indexMethod(in.effectiveMethods(recv.S), args[0].I, recv.S)
		p := indexParam(m, args[1].I)
		return typeInfoValue(p.Type), nil
	case "тип_результата_метода":
		m := indexMethod(in.effectiveMethods(recv.S), args[0].I, recv.S)
		if !m.HasReturn {
			return Value{Type: ir.TypeTypeInfo, S: "Пусто"}, nil
		}
		return typeInfoValue(m.ReturnType), nil
	case "создать":
		return in.reflectCreate(recv.S, args[0], env, cc)
	default:
		return Value{}, fmt.Errorf("у \"Тип\" нет метода %q", e.Method)
	}
}

// evalAnyMethod - методы "Значение" (docs/SPEC.md) - recv.Boxed несёт
// боксированное значение целиком (со своим настоящим Type).
func (in *Interp) evalAnyMethod(recv Value, e *ir.MethodCallExpr, env *Env, cc *callCtx) (Value, error) {
	args, err := in.evalArgs(e.Args, env, cc)
	if err != nil {
		return Value{}, err
	}
	switch e.Method {
	case "тип_значения":
		return typeInfoValue(recv.Boxed.Type), nil
	case "получить_поле":
		return in.reflectGetField(recv.Boxed, args[0].S)
	case "установить_поле":
		in.reflectSetField(recv.Boxed, args[0].S, args[1])
		return Value{}, nil
	case "вызвать":
		return in.reflectInvoke(recv.Boxed, args[0].S, args[1], cc)
	default:
		return Value{}, fmt.Errorf("у \"Значение\" нет метода %q", e.Method)
	}
}

// evalArgs вычисляет аргументы вызова метода в порядке слева направо -
// общий маленький хелпер для evalTypeInfoMethod/evalAnyMethod, у которых
// (в отличие от evalMethodCallExpr для пользовательских методов) нет
// своего получателя-*entityObj, поэтому переиспользовать сам
// evalMethodCallExpr нельзя.
func (in *Interp) evalArgs(exprs []ir.Expr, env *Env, cc *callCtx) ([]Value, error) {
	args := make([]Value, len(exprs))
	for i, a := range exprs {
		v, err := in.evalExpr(a, env, cc)
		if err != nil {
			return nil, err
		}
		args[i] = v
	}
	return args, nil
}

// indexField/indexMethod/indexParam - доступ по индексу с проверкой границ
// (выход за границы - паника, тот же принцип, что и у "получить" вне
// границ Список/Стопка/Очередь).
func indexField(ent *ir.Entity, idx int64, typeName string) ir.Field {
	if ent == nil || idx < 0 || idx >= int64(len(ent.Fields)) {
		panicRuntime("%s", rtmsg.ReflectIndexOutOfRangeFormat("поле", typeName))
	}
	return ent.Fields[idx]
}

func indexMethod(methods []*ir.Method, idx int64, typeName string) *ir.Method {
	if idx < 0 || idx >= int64(len(methods)) {
		panicRuntime("%s", rtmsg.ReflectIndexOutOfRangeFormat("метод", typeName))
	}
	return methods[idx]
}

func indexParam(m *ir.Method, idx int64) ir.Param {
	if idx < 0 || idx >= int64(len(m.Params)) {
		panicRuntime("%s", rtmsg.ReflectIndexOutOfRangeFormat("параметр", m.Name))
	}
	return m.Params[idx]
}

// reflectListElems читает элементы Список<Значение> lst - паникует, если
// lst на самом деле не список (не должно происходить - семантика уже
// проверила тип аргумента статически).
func reflectListElems(lst Value) []Value {
	return lst.Obj.coll.elems
}

// reflectGetField/reflectSetField - "значение.получить_поле(имя)"/
// "значение.установить_поле(имя, значение)" - доступ к полю боксированной
// сущности v по имени, минуя статическую проверку типа (та и есть смысл
// рефлексии) - отсутствие поля с таким именем у v - паника.
func (in *Interp) reflectGetField(v *Value, name string) (Value, error) {
	if v.Obj == nil || v.Obj.fields == nil {
		panicRuntime("%s", rtmsg.ReflectFieldNotFoundFormat(name, v.Type.String()))
	}
	fv, ok := v.Obj.fields[name]
	if !ok {
		panicRuntime("%s", rtmsg.ReflectFieldNotFoundFormat(name, v.Type.String()))
	}
	return Value{Type: ir.TypeAny, Boxed: &fv}, nil
}

func (in *Interp) reflectSetField(v *Value, name string, boxed Value) {
	if v.Obj == nil || v.Obj.fields == nil {
		panicRuntime("%s", rtmsg.ReflectFieldNotFoundFormat(name, v.Type.String()))
	}
	if _, ok := v.Obj.fields[name]; !ok {
		panicRuntime("%s", rtmsg.ReflectFieldNotFoundFormat(name, v.Type.String()))
	}
	v.Obj.fields[name] = in.cloneValue(*boxed.Boxed)
}

// reflectInvoke - "значение.вызвать(имя, аргументы)" - находит метод имя
// у фактического типа v (findMethod - та же цепочка BaseName, что и у
// обычного вызова через "." - метод не найден по имени - паника), вызывает
// его как обычный метод (callMethod) с уже разбоксированными аргументами.
// Число аргументов не совпадает с объявленным - паника (checkCallArgs
// делает эту проверку для обычных вызовов на этапе компиляции - здесь имя
// метода известно только в рантайме, поэтому и проверка тоже только в
// рантайме).
func (in *Interp) reflectInvoke(v *Value, methodName string, argsList Value, cc *callCtx) (Value, error) {
	m := in.findMethod(v.Obj.typeName, methodName)
	if m == nil {
		panicRuntime("%s", rtmsg.ReflectMethodNotFoundFormat(methodName, v.Type.String()))
	}
	boxedArgs := reflectListElems(argsList)
	if len(boxedArgs) != len(m.Params) {
		panicRuntime("%s", rtmsg.ReflectArgCountMismatchFormat(methodName, len(m.Params), len(boxedArgs)))
	}
	args := make([]Value, len(boxedArgs))
	for i, b := range boxedArgs {
		args[i] = *b.Boxed
	}
	result, err := in.callMethod(m, v.Obj, args, cc.dbg)
	if err != nil {
		return Value{}, err
	}
	if !m.HasReturn {
		return Value{Type: ir.TypeAny, Boxed: &Value{Type: ir.TypeTypeInfo, S: "Пусто"}}, nil
	}
	return Value{Type: ir.TypeAny, Boxed: &result}, nil
}

// reflectCreate - "тип.создать(аргументы)" (Activator.CreateInstance) -
// разбоксирует Список<Значение> позиционно, строит новый экземпляр через
// constructEntity (общий код с "новый Тип(...)", см. package doc eval.go).
func (in *Interp) reflectCreate(typeName string, argsList Value, env *Env, cc *callCtx) (Value, error) {
	ent, ok := in.entities[typeName]
	if !ok {
		panicRuntime("%s", rtmsg.ReflectMethodNotFoundFormat("создать", typeName))
	}
	boxedArgs := reflectListElems(argsList)
	args := make([]Value, len(boxedArgs))
	for i, b := range boxedArgs {
		args[i] = *b.Boxed
	}
	result, err := in.constructEntity(ent, typeName, args, env, cc)
	if err != nil {
		return Value{}, err
	}
	return Value{Type: ir.TypeAny, Boxed: &result}, nil
}
