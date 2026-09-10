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
	"slices"

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
//
// e.To.Kind == ir.KindInterface (docs/ROADMAP.md - "значение как
// ИмяИнтерфейса не распаковывает боксированную реализацию") - ОТДЕЛЬНАЯ
// проверка: боксированное значение всегда несёт дескриптор КОНКРЕТНОЙ
// сущности (v.Boxed.Type, никогда сам интерфейс - боксирование
// интерфейсно-типизированного ВЫРАЖЕНИЯ не поддержано отдельно, см. package
// doc reflectBoxExprCode в cbackend), поэтому точное совпадение "!=" здесь
// в принципе никогда не сработает - нужно "реализует ли фактический тип
// этот интерфейс" (entityImplementsInterface, транзитивно - тот же список,
// что и in.interfaceImplementors, посчитанный один раз в internal/semantic).
// Результат разбоксирования - тот же *v.Boxed БЕЗ переобёртывания в
// "интерфейсный" тип - interp не различает интерфейсно-типизированное и
// сущностно-типизированное значение в рантайме вообще (диспетчеризация
// методов везде идёт по фактическому Obj.typeName, см. findMethod), только
// на этапе статической проверки типов.
func (in *Interp) evalAsExpr(e *ir.AsExpr, env *Env, cc *callCtx) (Value, error) {
	v, err := in.evalExpr(e.Value, env, cc)
	if err != nil {
		return Value{}, err
	}
	if e.To.Kind == ir.KindInterface {
		if !in.entityImplementsInterface(v.Boxed.Type, e.To.Entity) {
			panicRuntime("%s", rtmsg.AsInterfaceMismatchFormat(e.To.Entity, v.Boxed.Type.String()))
		}
		return *v.Boxed, nil
	}
	if v.Boxed.Type != e.To {
		panicRuntime("%s", rtmsg.AsTypeMismatchFormat(v.Boxed.Type.String(), e.To.String()))
	}
	return *v.Boxed, nil
}

// entityImplementsInterface - t (обязан быть KindEntity - боксированное
// значение никогда не несёт сам интерфейс, см. package doc evalAsExpr выше)
// реализует ли ifaceName, по уже посчитанному in.interfaceImplementors.
func (in *Interp) entityImplementsInterface(t ir.Type, ifaceName string) bool {
	if t.Kind != ir.KindEntity {
		return false
	}
	return slices.Contains(in.interfaceImplementors[ifaceName], t.Entity)
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
//
// iface != nil (docs/ROADMAP.md - находка "тип ИмяИнтерфейса не
// существует") - recv.S называет интерфейс, не сущность: у интерфейса
// НЕТ полей и НЕТ конструктора (".создать" здесь неприменимо - falls
// through в reflectCreate, которая паникует "метод не найден", раз имени
// нет в in.entities, ровно тем же путём, что и для примитива), но ЕСТЬ
// собственные (уже сплющенные с унаследованными по "наследует" - см.
// interfaceSig.IRMethods, internal/semantic/interfaces.go) методы -
// iface.Methods, того же вида (Name/Params/ReturnType/HasReturn), что и
// ir.Method, но без Body - отдельный indexInterfaceMethod ниже, тот же
// принцип, что и у indexMethod.
func (in *Interp) evalTypeInfoMethod(recv Value, e *ir.MethodCallExpr, env *Env, cc *callCtx) (Value, error) {
	args, err := in.evalArgs(e.Args, env, cc)
	if err != nil {
		return Value{}, err
	}
	ent := in.entities[recv.S]
	iface := in.interfaces[recv.S]
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
	case "тег_поля_текст":
		f := indexField(ent, args[0].I, recv.S)
		return stringValue(f.Tag), nil
	case "индекс_поля_по_тегу":
		idx := findFieldIndexByTag(ent, args[0].S, args[1].S)
		if idx < 0 {
			return Value{Type: ir.NullableType(ir.TypeInt)}, nil
		}
		i := intValue(int64(idx))
		return Value{Type: ir.NullableType(ir.TypeInt), Nullable: &i}, nil
	case "число_методов":
		if iface != nil {
			return intValue(int64(len(iface.Methods))), nil
		}
		if ent == nil {
			return intValue(0), nil
		}
		return intValue(int64(len(in.effectiveMethods(recv.S)))), nil
	case "имя_метода":
		if iface != nil {
			return stringValue(indexInterfaceMethod(iface.Methods, args[0].I, recv.S).Name), nil
		}
		m := indexMethod(in.effectiveMethods(recv.S), args[0].I, recv.S)
		return stringValue(m.Name), nil
	case "число_параметров_метода":
		if iface != nil {
			return intValue(int64(len(indexInterfaceMethod(iface.Methods, args[0].I, recv.S).Params))), nil
		}
		m := indexMethod(in.effectiveMethods(recv.S), args[0].I, recv.S)
		return intValue(int64(len(m.Params))), nil
	case "тип_параметра_метода":
		if iface != nil {
			m := indexInterfaceMethod(iface.Methods, args[0].I, recv.S)
			if args[1].I < 0 || args[1].I >= int64(len(m.Params)) {
				panicRuntime("%s", rtmsg.ReflectIndexOutOfRangeFormat("параметр", m.Name))
			}
			return typeInfoValue(m.Params[args[1].I].Type), nil
		}
		m := indexMethod(in.effectiveMethods(recv.S), args[0].I, recv.S)
		p := indexParam(m, args[1].I)
		return typeInfoValue(p.Type), nil
	case "тип_результата_метода":
		if iface != nil {
			m := indexInterfaceMethod(iface.Methods, args[0].I, recv.S)
			if !m.HasReturn {
				return Value{Type: ir.TypeTypeInfo, S: "Пусто"}, nil
			}
			return typeInfoValue(m.ReturnType), nil
		}
		m := indexMethod(in.effectiveMethods(recv.S), args[0].I, recv.S)
		if !m.HasReturn {
			return Value{Type: ir.TypeTypeInfo, S: "Пусто"}, nil
		}
		return typeInfoValue(m.ReturnType), nil
	// "тег_метода"/"тег_метода_текст" (docs/ROADMAP.md, паритет с
	// C#-атрибутами на методе) - у интерфейса всегда "" (ir.InterfaceMethod
	// не несёт Tag - у самого языка тег метода интерфейса синтаксически
	// не существует, только у метода СУЩНОСТИ).
	case "тег_метода":
		if iface != nil {
			return stringValue(""), nil
		}
		m := indexMethod(in.effectiveMethods(recv.S), args[0].I, recv.S)
		return stringValue(structtag.Parse(m.Tag).Get(args[1].S)), nil
	case "тег_метода_текст":
		if iface != nil {
			return stringValue(""), nil
		}
		m := indexMethod(in.effectiveMethods(recv.S), args[0].I, recv.S)
		return stringValue(m.Tag), nil
	case "создать":
		return in.reflectCreate(recv.S, args[0], env, cc)
	// "тег"/"тег_текст" (docs/ROADMAP.md, паритет с C#-атрибутами на
	// классе) - тег самой сущности целиком ("сущность Имя `ключ:значение`",
	// ast.EntityDecl.Tag) - "" у примитива/интерфейса (ent == nil), тем же
	// принципом, что у остальных методов, действующих только на сущность.
	case "тег":
		if ent == nil {
			return stringValue(""), nil
		}
		return stringValue(structtag.Parse(ent.Tag).Get(args[0].S)), nil
	case "тег_текст":
		if ent == nil {
			return stringValue(""), nil
		}
		return stringValue(ent.Tag), nil
	// "базовый_тип"/"наследует_ли"/"число_интерфейсов"/"получить_интерфейс"/"реализует_ли" -
	// docs/ROADMAP.md, паритет с C# Type.BaseType/IsSubclassOf/GetInterfaces/
	// IsAssignableFrom - действуют только на сущность/структуру (ent !=
	// nil); примитив/интерфейс (ent == nil) - базовый_тип "Пусто", число_
	// интерфейсов 0, наследует/реализует всегда "ложь", тем же принципом,
	// что у число_полей/число_методов выше.
	case "базовый_тип":
		if ent == nil || ent.BaseName == "" {
			return Value{Type: ir.TypeTypeInfo, S: "Пусто"}, nil
		}
		return typeInfoValue(ir.EntityType(ent.BaseName)), nil
	case "наследует_ли":
		if ent == nil {
			return boolValue(false), nil
		}
		for n := ent.BaseName; n != ""; {
			if n == args[0].S {
				return boolValue(true), nil
			}
			base, ok := in.entities[n]
			if !ok {
				break
			}
			n = base.BaseName
		}
		return boolValue(false), nil
	case "число_интерфейсов":
		if ent == nil {
			return intValue(0), nil
		}
		return intValue(int64(len(in.entityInterfaces[recv.S]))), nil
	case "получить_интерфейс":
		names := in.entityInterfaces[recv.S]
		if args[0].I < 0 || args[0].I >= int64(len(names)) {
			panicRuntime("%s", rtmsg.ReflectIndexOutOfRangeFormat("интерфейс", recv.S))
		}
		return typeInfoValue(ir.InterfaceType(names[args[0].I])), nil
	case "реализует_ли":
		return boolValue(in.entityImplementsInterface(ir.EntityType(recv.S), args[0].S)), nil
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
	case "получить_поле_по_тегу":
		nullableType := ir.NullableType(ir.TypeAny)
		ent := in.entities[recv.Boxed.Type.Entity]
		idx := findFieldIndexByTag(ent, args[0].S, args[1].S)
		if idx < 0 {
			return Value{Type: nullableType}, nil
		}
		fv, err := in.reflectGetField(recv.Boxed, ent.Fields[idx].Name)
		if err != nil {
			return Value{}, err
		}
		return Value{Type: nullableType, Nullable: &fv}, nil
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

// findFieldIndexByTag ищет индекс поля ent с тегом key:value - обратное
// направление к indexField+tag-разбору выше (дан ключ/значение тега,
// нужно поле/его индекс, не наоборот) - используется индекс_поля_по_тегу/
// получить_поле_по_тегу. -1 - совпадения нет, в том числе ent == nil
// (примитив/значение не сущность - у него нет полей вообще).
func findFieldIndexByTag(ent *ir.Entity, key, value string) int {
	if ent == nil {
		return -1
	}
	for i, f := range ent.Fields {
		if structtag.Parse(f.Tag).Get(key) == value {
			return i
		}
	}
	return -1
}

func indexMethod(methods []*ir.Method, idx int64, typeName string) *ir.Method {
	if idx < 0 || idx >= int64(len(methods)) {
		panicRuntime("%s", rtmsg.ReflectIndexOutOfRangeFormat("метод", typeName))
	}
	return methods[idx]
}

// indexInterfaceMethod - то же самое, что indexMethod выше, но для
// iface.Methods ([]ir.InterfaceMethod - та же форма Name/Params/
// ReturnType/HasReturn, что и у ir.Method, но без Body, см. её package
// doc) - отдельная функция, не переиспользование indexMethod, потому что
// это разные Go-типы (*ir.Method против ir.InterfaceMethod).
func indexInterfaceMethod(methods []ir.InterfaceMethod, idx int64, typeName string) ir.InterfaceMethod {
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
	fv, ok := v.Obj.getFieldOK(name)
	if !ok {
		panicRuntime("%s", rtmsg.ReflectFieldNotFoundFormat(name, v.Type.String()))
	}
	return Value{Type: ir.TypeAny, Boxed: &fv}, nil
}

func (in *Interp) reflectSetField(v *Value, name string, boxed Value) {
	if v.Obj == nil || v.Obj.fields == nil {
		panicRuntime("%s", rtmsg.ReflectFieldNotFoundFormat(name, v.Type.String()))
	}
	if _, ok := v.Obj.getFieldOK(name); !ok {
		panicRuntime("%s", rtmsg.ReflectFieldNotFoundFormat(name, v.Type.String()))
	}
	v.Obj.setField(name, in.cloneValue(*boxed.Boxed))
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
