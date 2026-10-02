package interp

import (
	"fmt"
	"math"
	"strconv"
	"strings"

	"github.com/IlyaFeoktistov/Rech/internal/ir"
	"github.com/IlyaFeoktistov/Rech/internal/rtmsg"
)

// evalExpr вычисляет e. Не клонирует значения сущностей сам по себе -
// клонирование (cloneValue) происходит только в точках "коммита" в новое
// хранилище (VarDecl/Assign/FieldAssign.Value/параметр вызова/вернуть, см.
// exec.go/callFunction/callMethod) - навигация по цепочке (Identifier,
// FieldAccess.Receiver, получатель вызова метода) обязана возвращать ту же
// самую *entityObj, иначе метод не мог бы мутировать получатель через
// "этот" (docs/SPEC.md, "Методы и этот").
func (in *Interp) evalExpr(e ir.Expr, env *Env, cc *callCtx) (Value, error) {
	switch e := e.(type) {
	case *ir.StringLiteral:
		return stringValue(e.Value), nil
	case *ir.IntLiteral:
		return intValue(e.Value), nil
	case *ir.FloatLiteral:
		return floatValue(e.Value), nil
	case *ir.BoolLiteral:
		return boolValue(e.Value), nil

	case *ir.Identifier:
		v, ok := env.get(e.Name)
		if !ok {
			return Value{}, fmt.Errorf("переменная %q не найдена во время исполнения", e.Name)
		}
		return v, nil

	case *ir.ParseExpr:
		return in.evalParseExpr(e, env, cc)

	case *ir.SerializeExpr:
		return in.evalSerializeExpr(e, env, cc)

	case *ir.UnaryExpr:
		return in.evalUnary(e, env, cc)

	case *ir.IncDecExpr:
		return in.evalIncDec(e, env)

	case *ir.BinaryExpr:
		return in.evalBinary(e, env, cc)

	case *ir.TernaryExpr:
		cond, err := in.evalExpr(e.Cond, env, cc)
		if err != nil {
			return Value{}, err
		}
		// Короткое замыкание - только одна из веток вычисляется, тем же
		// принципом, что и "и"/"или" в evalBinary выше.
		if cond.B {
			return in.evalExpr(e.Then, env, cc)
		}
		return in.evalExpr(e.Else, env, cc)

	case *ir.CallExpr:
		args := make([]Value, len(e.Args))
		for i, a := range e.Args {
			v, err := in.evalExpr(a, env, cc)
			if err != nil {
				return Value{}, err
			}
			args[i] = v
		}
		// "начать_транзакцию" - единственный builtin, которому может
		// понадобиться пробросить НАСТОЯЩУЮ ошибку ("выбросить" внутри
		// колбэка - обычная error, не panic, docs internal/interp/
		// exception.go) - evalBuiltinCall ниже возвращает только (Value,
		// bool), без канала ошибки (ни один из остальных builtin'ов в нём
		// не вызывает пользовательский код колбэком), поэтому обрабатывается
		// отдельно, до него, тем же принципом сигнатуры (Value, error), что
		// и у самого evalExpr.
		if e.Callee == beginTransactionFuncName {
			return in.dbBeginTransaction(args[0].DB, args[1], cc.dbg)
		}
		if v, ok := in.evalBuiltinCall(e.Callee, args); ok {
			return v, nil
		}
		if decl, ok := in.externFuncs[e.Callee]; ok {
			return in.callExternFunc(decl, args)
		}
		fn, ok := in.funcs[e.Callee]
		if !ok {
			return Value{}, fmt.Errorf("функция %q не найдена во время исполнения", e.Callee)
		}
		return in.callFunction(fn, args, cc.dbg)

	case *ir.NewExpr:
		return in.evalNewExpr(e, env, cc)

	case *ir.FieldAccess:
		recv, err := in.evalExpr(e.Receiver, env, cc)
		if err != nil {
			return Value{}, err
		}
		v := recv.Obj.getField(e.Field)
		in.checkFFIFuncField(recv, e.Field, v)
		return v, nil

	case *ir.ThisExpr:
		return Value{Type: ir.EntityType(e.Entity), Obj: cc.this}, nil

	case *ir.MethodCallExpr:
		return in.evalMethodCallExpr(e, env, cc)

	case *ir.StaticFieldExpr:
		return in.staticFields[e.Entity][e.Field], nil

	case *ir.StaticMethodCallExpr:
		return in.evalStaticMethodCallExpr(e, env, cc)

	case *ir.FuncRefExpr:
		return in.evalFuncRefExpr(e)

	case *ir.LambdaExpr:
		return in.evalLambdaExpr(e, env, cc)

	case *ir.CallValueExpr:
		return in.evalCallValueExpr(e, env, cc)

	case *ir.CharLiteral:
		return charValue(e.Value), nil

	case *ir.CastExpr:
		return in.evalCastExpr(e, env, cc)

	case *ir.IsExpr:
		return in.evalIsExpr(e, env, cc)

	case *ir.StateCheckExpr:
		return in.evalStateCheckExpr(e, env, cc)

	case *ir.HasStateExpr:
		return in.evalHasStateExpr(e, env, cc)

	case *ir.LaunchExpr:
		return in.evalLaunchExpr(e, env, cc)

	case *ir.AwaitExpr:
		return in.evalAwaitExpr(e, env, cc)

	case *ir.TypeOfExpr:
		return evalTypeOfExpr(e), nil

	case *ir.AsExpr:
		return in.evalAsExpr(e, env, cc)

	case *ir.DBRowsAsExpr:
		return in.evalDBRowsAsExpr(e, env, cc)

	case *ir.NullLiteral:
		return evalNullLiteral(e), nil

	case *ir.UpcastExpr:
		// Фактический тип значения во время исполнения не меняется
		// (виртуальные вызовы, "относится к").
		return in.evalExpr(e.Value, env, cc)

	case *ir.NullWrapExpr:
		return in.evalNullWrapExpr(e, env, cc)

	case *ir.NullUnwrapExpr:
		return in.evalNullUnwrapExpr(e, env, cc)

	case *ir.NullCheckExpr:
		return in.evalNullCheckExpr(e, env, cc)

	case *ir.NullCoalesceExpr:
		return in.evalNullCoalesceExpr(e, env, cc)

	case *ir.ListIndexExpr:
		return in.evalListIndexExpr(e, env, cc)

	case *ir.DictIndexExpr:
		return in.evalDictIndexExpr(e, env, cc)

	case *ir.ListLiteralExpr:
		return in.evalListLiteralExpr(e, env, cc)

	case *ir.AddressOfExpr:
		return in.evalAddressOfExpr(e, env, cc)

	case *ir.DerefExpr:
		return in.evalDerefExpr(e, env, cc)

	case *ir.PointerOffsetExpr:
		return in.evalPointerOffsetExpr(e, env, cc)

	case *ir.ArrayIndexExpr:
		return in.evalArrayIndexExpr(e, env, cc)

	case *ir.ArrayLiteralExpr:
		return in.evalArrayLiteralExpr(e, env, cc)

	default:
		return Value{}, fmt.Errorf("вычисление: выражение %T не поддерживается интерпретатором", e)
	}
}

func (in *Interp) evalUnary(e *ir.UnaryExpr, env *Env, cc *callCtx) (Value, error) {
	v, err := in.evalExpr(e.Operand, env, cc)
	if err != nil {
		return Value{}, err
	}
	switch e.Op {
	case "-":
		switch {
		case v.Type == ir.TypeFloat:
			return floatValue(-v.F), nil
		case v.Type == ir.TypeFloat32:
			return float32Value(-v.F), nil
		case isFixedWidthInt(v.Type.Kind):
			return Value{Type: v.Type, I: wrapFixedWidth(v.Type.Kind, -v.I)}, nil
		default:
			return intValue(-v.I), nil
		}
	case "не":
		return boolValue(!v.B), nil
	case "~":
		return bitwiseUnaryResult(v.Type, ^v.I), nil
	default:
		return Value{}, fmt.Errorf("неизвестная унарная операция %q", e.Op)
	}
}

// evalIncDec - "имя++"/"имя--"/"++имя"/"--имя": читает текущее значение
// переменной, вычисляет новое (+-1, с учётом Целое/Число), записывает его
// обратно и возвращает старое значение (постфикс) или новое (префикс) -
// та же семантика, что и у "++"/"--" в C (cbackend.genIncDecExpr просто
// переиспользует C-операторы напрямую - здесь, в отличие от C, нужно
// явно прочитать/записать переменную самому).
func (in *Interp) evalIncDec(e *ir.IncDecExpr, env *Env) (Value, error) {
	v, ok := env.get(e.Name)
	if !ok {
		return Value{}, fmt.Errorf("переменная %q не найдена во время исполнения", e.Name)
	}
	delta := int64(1)
	if e.Op == "--" {
		delta = -1
	}
	var updated Value
	switch {
	case v.Type == ir.TypeFloat:
		updated = floatValue(v.F + float64(delta))
	case v.Type == ir.TypeFloat32:
		updated = float32Value(v.F + float64(delta))
	case isFixedWidthInt(v.Type.Kind):
		updated = Value{Type: v.Type, I: wrapFixedWidth(v.Type.Kind, v.I+delta)}
	default:
		updated = intValue(v.I + delta)
	}
	env.set(e.Name, updated)
	if id, ok := env.lookupStaticID(e.Name); ok {
		in.staticLocals[id] = updated
	}
	if e.Prefix {
		return updated, nil
	}
	return v, nil
}

// evalBinary - "и"/"или" вычисляются с коротким замыканием (как && / || в
// сгенерированном C, cbackend.generateIRBinaryExpr) - правый операнд не
// вычисляется, если левый уже решает результат.
func (in *Interp) evalBinary(e *ir.BinaryExpr, env *Env, cc *callCtx) (Value, error) {
	if e.Op == "и" || e.Op == "или" {
		l, err := in.evalExpr(e.Left, env, cc)
		if err != nil {
			return Value{}, err
		}
		if e.Op == "и" && !l.B {
			return boolValue(false), nil
		}
		if e.Op == "или" && l.B {
			return boolValue(true), nil
		}
		r, err := in.evalExpr(e.Right, env, cc)
		if err != nil {
			return Value{}, err
		}
		return boolValue(r.B), nil
	}

	l, err := in.evalExpr(e.Left, env, cc)
	if err != nil {
		return Value{}, err
	}
	r, err := in.evalExpr(e.Right, env, cc)
	if err != nil {
		return Value{}, err
	}

	switch e.Op {
	case "+":
		if l.Type == ir.TypeString && r.Type == ir.TypeString {
			return stringValue(l.S + r.S), nil
		}
		return arithmetic(e.Op, l, r)
	case "-", "*", "/", "%":
		return arithmetic(e.Op, l, r)
	case "==":
		return boolValue(in.valuesEqual(l, r)), nil
	case "!=":
		return boolValue(!in.valuesEqual(l, r)), nil
	case "<", ">", "<=", ">=":
		return compareOrdered(e.Op, l, r), nil
	case "&", "|", "^", "<<", ">>":
		return bitwiseBinaryResult(e.Op, l, r), nil
	default:
		return Value{}, fmt.Errorf("неизвестная операция %q", e.Op)
	}
}

// arithmetic - "+" (нестроковый)/"-"/"*"/"/"/"%": та же трёхуровневая промоция
// Целое -> Дробное -> Действительное при смешении, что и в
// cbackend.generateIRArithmetic - более широкий из двух операндов
// определяет и тип результата, и точность вычисления (float32Value
// округляет результат Дробное-арифметики до одинарной точности, как C
// float).
func arithmetic(op string, l, r Value) (Value, error) {
	if l.Type == ir.TypeFloat || r.Type == ir.TypeFloat {
		lf, rf := toFloat(l), toFloat(r)
		switch op {
		case "+":
			return floatValue(lf + rf), nil
		case "-":
			return floatValue(lf - rf), nil
		case "*":
			return floatValue(lf * rf), nil
		case "/":
			return floatValue(lf / rf), nil
		case "%":
			return floatValue(math.Mod(lf, rf)), nil
		}
	}
	if l.Type == ir.TypeFloat32 || r.Type == ir.TypeFloat32 {
		lf, rf := toFloat(l), toFloat(r)
		switch op {
		case "+":
			return float32Value(lf + rf), nil
		case "-":
			return float32Value(lf - rf), nil
		case "*":
			return float32Value(lf * rf), nil
		case "/":
			return float32Value(lf / rf), nil
		case "%":
			return float32Value(math.Mod(lf, rf)), nil
		}
	}
	if l.Type == r.Type && isFixedWidthInt(l.Type.Kind) {
		return fixedWidthArithmetic(op, l, r)
	}
	switch op {
	case "+":
		return intValue(l.I + r.I), nil
	case "-":
		return intValue(l.I - r.I), nil
	case "*":
		return intValue(l.I * r.I), nil
	case "/":
		if r.I == 0 {
			panicRuntimeAs(rtmsg.PanicDivisionByZero, "%s", rtmsg.DivisionByZero)
		}
		return intValue(l.I / r.I), nil
	case "%":
		if r.I == 0 {
			panicRuntimeAs(rtmsg.PanicDivisionByZero, "%s", rtmsg.DivisionByZero)
		}
		return intValue(l.I % r.I), nil
	}
	return Value{}, nil
}

// toFloat - числовое значение v как float64, независимо от того, в каком
// поле оно физически хранится (I - Целое/Байт/Символ, F - Дробное/
// Действительное).
func toFloat(v Value) float64 {
	if v.Type == ir.TypeFloat || v.Type == ir.TypeFloat32 {
		return v.F
	}
	return float64(v.I)
}

// valuesEqual - метод (не свободная функция), потому что "структура" (ниже)
// нуждается в entities (IsStruct/FieldNames) для рекурсивного сравнения поля
// за полем - того же доступа, что уже есть у *Interp везде в этом пакете.
func (in *Interp) valuesEqual(l, r Value) bool {
	switch {
	case l.Type.Kind == ir.KindAny:
		// Тот же тип и равное содержимое (cbackend/any_value.go).
		lb, rb := l.Boxed, r.Boxed
		if lb == nil || rb == nil || lb.Type.Kind == ir.KindNull || rb.Type.Kind == ir.KindNull {
			return (lb == nil || lb.Type.Kind == ir.KindNull) == (rb == nil || rb.Type.Kind == ir.KindNull)
		}
		return lb.Type == rb.Type && in.valuesEqual(*lb, *rb)
	case l.Type == ir.TypeString:
		return l.S == r.S
	case l.Type == ir.TypeTypeInfo:
		// "Тип" - имя типа (см. package doc Value.Boxed).
		return l.S == r.S
	case l.Type == ir.TypeBool:
		return l.B == r.B
	case l.Type == ir.TypeFloat || r.Type == ir.TypeFloat || l.Type == ir.TypeFloat32 || r.Type == ir.TypeFloat32:
		return toFloat(l) == toFloat(r)
	case l.Type.Kind == ir.KindEntity:
		if ent, ok := in.entities[l.Type.Entity]; ok && ent.IsStruct {
			// "структура" - значение, не identity ("==" для
			// структур - поле-в-поле) - семантический анализ
			// (structFieldsEquatable, cast.go) уже гарантировал, что КАЖДОЕ
			// поле (рекурсивно, у вложенных структур тоже) - сравнимый тип,
			// значит рекурсивный valuesEqual по каждому полю корректен без
			// дополнительных проверок здесь.
			for _, f := range ent.Fields {
				if !in.valuesEqual(l.Obj.fields[f.Name], r.Obj.fields[f.Name]) {
					return false
				}
			}
			return true
		}
		// Сравнение по ССЫЛКЕ ("==" для сущности/класса, НАЙДЕНО по ходу
		// examples/проекты/чат_сервер - раньше не поддерживалось вовсе, см.
		// её проверку в analyzeBinaryExpr, expr.go) - Obj это тот же
		// Go-указатель, что несёт identity объекта (см. package doc Value.
		// Obj, interp.go) - идентично cbackend, где сущность-ссылка - C-
		// указатель, "==" на ней уже безусловно пойнтерное сравнение
		// (generateIRBinaryExpr, default-ветка).
		return l.Obj == r.Obj
	default:
		return l.I == r.I
	}
}

func compareOrdered(op string, l, r Value) Value {
	if l.Type == ir.TypeString {
		// Байтовое сравнение UTF-8 - порядок кодовых точек, как strcmp в cbackend.
		c := strings.Compare(l.S, r.S)
		return boolValue(map[string]bool{"<": c < 0, ">": c > 0, "<=": c <= 0, ">=": c >= 0}[op])
	}
	if l.Type == ir.TypeFloat || r.Type == ir.TypeFloat || l.Type == ir.TypeFloat32 || r.Type == ir.TypeFloat32 {
		lf, rf := toFloat(l), toFloat(r)
		switch op {
		case "<":
			return boolValue(lf < rf)
		case ">":
			return boolValue(lf > rf)
		case "<=":
			return boolValue(lf <= rf)
		case ">=":
			return boolValue(lf >= rf)
		}
	}
	// Натуральное* (беззнаковые шага 12) - сравнение через uint64, не
	// int64: Натуральное64 > INT64_MAX выглядит отрицательным в l.I/r.I -
	// та же причина, что и у fixedWidthArithmetic."/".
	if isUnsignedFixedWidth(l.Type.Kind) {
		lu, ru := uint64(l.I), uint64(r.I)
		switch op {
		case "<":
			return boolValue(lu < ru)
		case ">":
			return boolValue(lu > ru)
		case "<=":
			return boolValue(lu <= ru)
		case ">=":
			return boolValue(lu >= ru)
		}
	}
	switch op {
	case "<":
		return boolValue(l.I < r.I)
	case ">":
		return boolValue(l.I > r.I)
	case "<=":
		return boolValue(l.I <= r.I)
	case ">=":
		return boolValue(l.I >= r.I)
	}
	return Value{}
}

// evalNewExpr - "новый Тип(...)" - две формы, обычная сущность (поля по
// позиции, как ir.NewExpr.Args уже разложены семантическим анализом) или
// встроенная generic-коллекция (Args всегда пуст, буфер/хеш-таблица
// начинает пустой), см. package doc про то, почему у генериков нет
// ir.Entity-узла.
func (in *Interp) evalNewExpr(e *ir.NewExpr, env *Env, cc *callCtx) (Value, error) {
	if e.Entity == mutexEntityName {
		return newMutexValue(), nil
	}
	if e.Entity == semaphoreEntityName {
		n, err := in.evalExpr(e.Args[0], env, cc)
		if err != nil {
			return Value{}, err
		}
		return newSemaphoreValue(n.I), nil
	}
	if ent, ok := in.entities[e.Entity]; ok {
		args := make([]Value, len(e.Args))
		for i, a := range e.Args {
			v, err := in.evalExpr(a, env, cc)
			if err != nil {
				return Value{}, err
			}
			args[i] = v
		}
		return in.constructEntity(ent, e.Entity, args, env, cc)
	}
	if g, ok := in.generics[e.Entity]; ok {
		return in.newEmptyCollection(e.Entity, g), nil
	}
	return Value{}, fmt.Errorf("сущность %q не найдена во время исполнения", e.Entity)
}

// newEmptyCollection - "новый Список<T>()"/"новая Стопка<T>()"/... с пустым
// содержимым - entName уже конкретная инстанциация ("Список<Целое>"), g -
// её запись в in.generics (Template решает, нужна ли map для "Словарь").
// Общий код для evalNewExpr (буквальный "новый ...()") и zeroFieldValue
// (поле generic-коллекции без "имеет: ... = ..." - см. её package doc).
func (in *Interp) newEmptyCollection(entName string, g ir.GenericInstantiation) Value {
	coll := &collection{template: g.Template}
	if g.Template == "Словарь" || g.Template == "Множество" {
		coll.pairs = newDictPairs()
	}
	return Value{Type: ir.EntityType(entName), Obj: &entityObj{typeName: entName, coll: coll}}
}

// constructEntity строит новый экземпляр ent (имя entName - отдельно от
// ent, потому что generic-инстанциация встроенной коллекции здесь не
// нужна, но имя типа результата всё равно нужно, см. вызов из evalNewExpr)
// из уже вычисленных args - общий код для "новый Тип(...)" (evalNewExpr,
// args - позиционные аргументы конструктора/полей, как их разложил
// семантический анализ) и "тип.создать(аргументы)" по рефлексии
// (internal/interp/reflect.go, args - уже разбоксированные из
// Список<Значение>): у явного конструктора (ent.Ctor != nil) поля НЕ
// заполняются один-в-один из args (это аргументы самого конструктора, не
// обязаны совпадать с полями числом/порядком) - объект заводится с
// нулевыми значениями всех полей (та же гарантия, что и GC_MALLOC в
// cbackend - тело конструктора должно видеть согласованный ноль, а не
// отсутствие ключа в map), кроме полей с "имеет: имя: Тип = выражение" -
// такое поле начинается со своего значения по умолчанию, не с нуля. Без
// явного конструктора - обычное позиционное присваивание args полям.
func (in *Interp) constructEntity(ent *ir.Entity, entName string, args []Value, env *Env, cc *callCtx) (Value, error) {
	if ent.Ctor == nil {
		fields := make(map[string]Value, len(ent.Fields))
		for i, f := range ent.Fields {
			fields[f.Name] = in.cloneValue(args[i])
		}
		return Value{Type: ir.EntityType(entName), Obj: &entityObj{typeName: entName, fields: fields}}, nil
	}
	obj := &entityObj{typeName: entName, fields: make(map[string]Value, len(ent.Fields))}
	for _, f := range ent.Fields {
		if f.Default != nil {
			v, err := in.evalExpr(f.Default, env, cc)
			if err != nil {
				return Value{}, err
			}
			obj.setField(f.Name, in.cloneValue(v))
			continue
		}
		obj.setField(f.Name, in.zeroFieldValue(f.Type))
	}
	if _, err := in.callMethod(ent.Ctor, obj, args, cc.dbg); err != nil {
		return Value{}, err
	}
	return Value{Type: ir.EntityType(entName), Obj: obj}, nil
}

// bareStruct - структура, объявленная без значения ("Пара п"): поля -
// значения по умолчанию или нулевые, дальше заполняются по одному.
func (in *Interp) bareStruct(ent *ir.Entity, entName string, env *Env, cc *callCtx) (Value, error) {
	obj := &entityObj{typeName: entName, fields: make(map[string]Value, len(ent.Fields))}
	for _, f := range ent.Fields {
		if f.Default != nil {
			v, err := in.evalExpr(f.Default, env, cc)
			if err != nil {
				return Value{}, err
			}
			obj.setField(f.Name, in.cloneValue(v))
			continue
		}
		obj.setField(f.Name, in.zeroFieldValue(f.Type))
	}
	return Value{Type: ir.EntityType(entName), Obj: obj}, nil
}

// zeroFieldValue - нулевое значение поля БЕЗ "имеет: имя: Тип = выражение"
// (см. package doc constructEntity) - zeroValue(t) для примитива (0/""/
// ложь/...), НО для generic-коллекции (Список<T>/Словарь<К,В>/Стопка<T>/
// Очередь<T>) - настоящий пустой экземпляр (newEmptyCollection), а не
// zeroValue(t)'ин голый Value{Type: t} с Obj == nil: ЛЮБОЙ метод такого
// поля (`.добавить`/`.длина`/...) сразу разыменовывал бы nil recv.Obj -
// найдено живой проверкой (`сущность` с полем `Список<Целое>` без
// значения по умолчанию падала с "invalid memory address or nil pointer
// dereference" при первом же обращении к полю, даже до вызова метода
// самого конструктора). Сущность/интерфейс-поле без значения по умолчанию
// по-прежнему остаётся nil-ссылкой (Value{Type: t}, zeroValue's default) -
// это ожидаемая, а не ошибочная семантика ("ещё не присвоено", как
// неинициализированная ссылка в Java/C#) - падать может только НЕВЕРНОЕ
// использование (обращение до присваивания), не сам факт существования
// такого поля.
func (in *Interp) zeroFieldValue(t ir.Type) Value {
	if t.Kind == ir.KindArray {
		return in.zeroArray(t)
	}
	if t.Kind == ir.KindEntity {
		if g, ok := in.generics[t.Entity]; ok {
			return in.newEmptyCollection(t.Entity, g)
		}
	}
	return zeroValue(t)
}

// evalMethodCallExpr - "receiver.метод(...)". Receiver вычисляется без
// клонирования (нужна та же *entityObj, что хранится в переменной/поле) -
// дальше расходится на пользовательский метод (тело - ir.Method, как
// обычный вызов функции со скрытым получателем) или встроенный метод
// generic-коллекции (evalCollectionMethod - обе ветки различаются по тому,
// заполнено ли recv.Obj.coll, а не по имени сущности).
func (in *Interp) evalMethodCallExpr(e *ir.MethodCallExpr, env *Env, cc *callCtx) (Value, error) {
	recv, err := in.evalExpr(e.Receiver, env, cc)
	if err != nil {
		return Value{}, err
	}
	switch recv.Type.Kind {
	case ir.KindAny:
		return in.evalAnyMethod(recv, e, env, cc)
	case ir.KindTypeInfo:
		return in.evalTypeInfoMethod(recv, e, env, cc)
	}
	if recv.Obj.mutex != nil {
		return evalMutexMethod(recv.Obj, e)
	}
	if recv.Obj.semaphore != nil {
		return in.evalSemaphoreMethod(recv.Obj, e, env, cc)
	}
	if recv.Obj.dbTx != nil {
		return in.evalTransactionMethod(recv.Obj, e, env, cc)
	}
	if recv.Obj.coll != nil {
		if recv.Obj.coll.template == "Словарь" {
			return in.evalDictMethod(recv.Obj, e, env, cc)
		}
		if recv.Obj.coll.template == "Множество" {
			return in.evalSetMethod(recv.Obj, e, env, cc)
		}
		return in.evalCollectionMethod(recv.Obj, e, env, cc)
	}
	m := in.findMethod(recv.Obj.typeName, e.Method)
	if m == nil {
		return Value{}, fmt.Errorf("у сущности %q нет метода %q", recv.Obj.typeName, e.Method)
	}
	// RevokesStates - см. package doc ir.MethodCallExpr.RevokesStates - ДО
	// вызова тела метода, чтобы собственное "становится" метода-
	// "противоположности" (если есть) выполнилось позже и победило (тот же
	// порядок, что и в cbackend, wrapRevokeStates).
	for _, st := range e.RevokesStates {
		recv.Obj.clearState(st)
	}
	args := make([]Value, len(e.Args))
	for i, a := range e.Args {
		v, err := in.evalExpr(a, env, cc)
		if err != nil {
			return Value{}, err
		}
		args[i] = v
	}
	return in.callMethod(m, recv.Obj, args, cc.dbg)
}

// evalStaticMethodCallExpr - "Тип::метод(...)" (см. package doc
// ir.StaticMethodCallExpr).
func (in *Interp) evalStaticMethodCallExpr(e *ir.StaticMethodCallExpr, env *Env, cc *callCtx) (Value, error) {
	m := in.findMethod(e.Entity, e.Method)
	if m == nil {
		return Value{}, fmt.Errorf("у сущности %q нет статического метода %q", e.Entity, e.Method)
	}
	args := make([]Value, len(e.Args))
	for i, a := range e.Args {
		v, err := in.evalExpr(a, env, cc)
		if err != nil {
			return Value{}, err
		}
		args[i] = v
	}
	return in.callStaticMethod(m, e.Entity, args, cc.dbg)
}

// evalCollectionMethod - встроенные методы Список/Стопка/Очередь
// (docs/SPEC.md, раздел 8) - реализованы напрямую на Go-слайсе, без
// собственного IR-представления тела метода (та же причина, по которой у
// генериков нет ir.Entity - см. package doc ir.GenericInstantiation).
// Выход за границы/операция на пустой коллекции - panicRuntime (аварийное
// завершение процесса), как и rech_panic в сгенерированном C.
func (in *Interp) evalCollectionMethod(obj *entityObj, e *ir.MethodCallExpr, env *Env, cc *callCtx) (Value, error) {
	args := make([]Value, len(e.Args))
	for i, a := range e.Args {
		v, err := in.evalExpr(a, env, cc)
		if err != nil {
			return Value{}, err
		}
		args[i] = v
	}
	switch e.Method {
	case "добавить", "положить":
		obj.coll.elems = append(obj.coll.elems, in.cloneValue(args[0]))
		return Value{}, nil

	case "получить":
		idx := args[0].I
		if idx < 0 || idx >= int64(len(obj.coll.elems)) {
			panicRuntimeAs(rtmsg.PanicOutOfRange, "%s", rtmsg.IndexOutOfRange(obj.typeName))
		}
		return in.cloneValue(obj.coll.elems[idx]), nil

	case "установить":
		idx := args[0].I
		if idx < 0 || idx >= int64(len(obj.coll.elems)) {
			panicRuntimeAs(rtmsg.PanicOutOfRange, "%s", rtmsg.IndexOutOfRange(obj.typeName))
		}
		obj.coll.elems[idx] = in.cloneValue(args[1])
		return Value{}, nil

	case "удалить":
		idx := args[0].I
		if idx < 0 || idx >= int64(len(obj.coll.elems)) {
			panicRuntimeAs(rtmsg.PanicOutOfRange, "%s", rtmsg.IndexOutOfRange(obj.typeName))
		}
		v := obj.coll.elems[idx]
		obj.coll.elems = append(obj.coll.elems[:idx], obj.coll.elems[idx+1:]...)
		return v, nil

	case "снять":
		n := len(obj.coll.elems)
		if n == 0 {
			panicRuntimeAs(rtmsg.PanicEmptyCollection, "%s", rtmsg.EmptyStackOp(obj.typeName))
		}
		v := obj.coll.elems[n-1]
		obj.coll.elems = obj.coll.elems[:n-1]
		return v, nil

	case "значения":
		if obj.coll.template == "Стопка" {
			return in.listResult(ir.TypeOf(e), reversedValues(obj.coll.elems)), nil
		}
		return in.listResult(ir.TypeOf(e), obj.coll.elems), nil

	case "верх":
		n := len(obj.coll.elems)
		if n == 0 {
			panicRuntimeAs(rtmsg.PanicEmptyCollection, "%s", rtmsg.EmptyStackOp(obj.typeName))
		}
		return in.cloneValue(obj.coll.elems[n-1]), nil

	case "извлечь":
		if len(obj.coll.elems) == 0 {
			panicRuntimeAs(rtmsg.PanicEmptyCollection, "%s", rtmsg.EmptyQueueOp(obj.typeName))
		}
		v := obj.coll.elems[0]
		obj.coll.elems = obj.coll.elems[1:]
		return v, nil

	case "первый":
		// Общий метод-имя у Очереди ("первый" - голова, БЕЗ извлечения) и
		// Списка ("первый" - тот же элемент, что "получить(0)",
		// пользователь явно попросил - много мест писали "получить(0)")
		// - только паническое сообщение на пустой коллекции разное
		// (obj.coll.template отличает их, метод-имя одно и то же).
		if len(obj.coll.elems) == 0 {
			if obj.coll.template == "Очередь" {
				panicRuntimeAs(rtmsg.PanicEmptyCollection, "%s", rtmsg.EmptyQueueOp(obj.typeName))
			}
			panicRuntimeAs(rtmsg.PanicOutOfRange, "%s", rtmsg.IndexOutOfRange(obj.typeName))
		}
		return in.cloneValue(obj.coll.elems[0]), nil

	case "последний":
		if len(obj.coll.elems) == 0 {
			panicRuntimeAs(rtmsg.PanicOutOfRange, "%s", rtmsg.IndexOutOfRange(obj.typeName))
		}
		return in.cloneValue(obj.coll.elems[len(obj.coll.elems)-1]), nil

	case "длина":
		return intValue(int64(len(obj.coll.elems))), nil

	case "пусто":
		return boolValue(len(obj.coll.elems) == 0), nil

	case "равен":
		return boolValue(in.collectionsDeepEqual(obj.coll, args[0].Obj.coll)), nil

	default:
		return Value{}, fmt.Errorf("встроенная коллекция %q: неизвестный метод %q", obj.typeName, e.Method)
	}
}

// evalDictMethod - встроенные методы "Словарь<К, В>" - отдельная от
// evalCollectionMethod функция (не общий switch), потому что хранилище
// (obj.coll.pairs, dictPairs) и семантика операций (по ключу, не по индексу)
// не совпадают со списочными Список/Стопка/Очередь, хотя часть имён
// методов повторяется ("длина"/"пусто" - тот же смысл, разное хранилище).
func (in *Interp) evalDictMethod(obj *entityObj, e *ir.MethodCallExpr, env *Env, cc *callCtx) (Value, error) {
	args := make([]Value, len(e.Args))
	for i, a := range e.Args {
		v, err := in.evalExpr(a, env, cc)
		if err != nil {
			return Value{}, err
		}
		args[i] = v
	}
	switch e.Method {
	case "установить":
		obj.coll.pairs.set(args[0], in.cloneValue(args[1]))
		return Value{}, nil

	case "получить":
		v, ok := obj.coll.pairs.get(args[0])
		if !ok {
			panicRuntimeAs(rtmsg.PanicKeyNotFound, "%s", rtmsg.DictKeyNotFound(obj.typeName))
		}
		return in.cloneValue(v), nil

	case "содержит":
		ok := obj.coll.pairs.has(args[0])
		return boolValue(ok), nil

	case "удалить":
		obj.coll.pairs.del(args[0])
		return Value{}, nil

	case "длина":
		return intValue(int64(obj.coll.pairs.size())), nil

	case "пусто":
		return boolValue(obj.coll.pairs.size() == 0), nil

	case "равен":
		return boolValue(in.collectionsDeepEqual(obj.coll, args[0].Obj.coll)), nil

	case "ключи":
		return in.listResult(ir.TypeOf(e), obj.coll.pairs.keys), nil

	case "значения":
		return in.listResult(ir.TypeOf(e), obj.coll.pairs.vals), nil

	default:
		return Value{}, fmt.Errorf("встроенный словарь %q: неизвестный метод %q", obj.typeName, e.Method)
	}
}

// collectionsDeepEqual - сравнение СОДЕРЖИМОГО двух встроенных коллекций
// (метод "равен" у Список<T>/Словарь<К, В>, evalCollectionMethod/
// evalDictMethod выше) - НАМЕРЕННО отдельная от valuesEqual (которая
// обслуживает "==" и оставляет коллекции сравнением по identity, как и
// раньше - см. её package doc, l.Obj == r.Obj в общем случае KindEntity):
// смешивать их означало бы незаметно поменять семантику уже
// существовавшего "==" для Список/Словарь, чего не просили. a.pairs != nil
// отличает Словарь (сравнение по ключам - порядок вставки не влияет) от
// Список/Стопка/Очередь (a.elems, поэлементно по индексу - упорядоченная
// коллекция) - internal/semantic регистрирует "равен" только у Список и
// Словарь (entitySig.Equatable), но сама проверка здесь годится для любого
// collection-хранилища одинаково.
func (in *Interp) collectionsDeepEqual(a, b *collection) bool {
	if a.pairs != nil || b.pairs != nil {
		if a.pairs.size() != b.pairs.size() {
			return false
		}
		for i, k := range a.pairs.keys {
			bv, ok := b.pairs.get(k)
			if !ok || !in.valueDeepEqual(a.pairs.vals[i], bv) {
				return false
			}
		}
		return true
	}
	if len(a.elems) != len(b.elems) {
		return false
	}
	for i := range a.elems {
		if !in.valueDeepEqual(a.elems[i], b.elems[i]) {
			return false
		}
	}
	return true
}

// valueDeepEqual - как valuesEqual, но рекурсивно сравнивает вложенную
// встроенную коллекцию (элемент Список<Список<T>>/значение Словарь<К,
// Список<T>>) по СОДЕРЖИМОМУ (collectionsDeepEqual), а не по identity -
// иначе Список<Список<Целое>>.равен(...) сравнил бы вложенные списки
// указателями и был бы бесполезен для ровно того случая, ради которого
// "равен" завели. Не подменяет valuesEqual саму - только
// используется РЯДОМ с ней, из collectionsDeepEqual выше.
func (in *Interp) valueDeepEqual(l, r Value) bool {
	if l.Type.Kind == ir.KindNullable {
		if l.Nullable == nil || r.Nullable == nil {
			return l.Nullable == nil && r.Nullable == nil
		}
		return in.valueDeepEqual(*l.Nullable, *r.Nullable)
	}
	if l.Type.Kind == ir.KindEntity && l.Obj != nil && l.Obj.coll != nil {
		if r.Obj == nil || r.Obj.coll == nil {
			return false
		}
		return in.collectionsDeepEqual(l.Obj.coll, r.Obj.coll)
	}
	return in.valuesEqual(l, r)
}

// printValue печатает v в in.out ровно в том же текстовом виде, что и
// printf в сгенерированном C (cbackend.printCall) - %lld для Целое, %g
// (точность 6 значащих цифр, дефолт printf) для Число, "истина"/"ложь" для
// Логическое - иначе сравнение вывода интерпретатора со скомпилированной
// программой byte-for-byte не имело бы смысла.
// formatFloatForPrint - как %g в сгенерированном C (printCall), включая
// бесконечности: printf("%g", ...) печатает "inf"/"-inf", а
// strconv.FormatFloat - "+Inf"/"-Inf" - без этой поправки печать
// разошлась бы между интерпретатором и скомпилированной программой.
// NaN печатается как "nan" без знака - у glibc есть свой (платформенно-
// зависимый, не гарантированный ни C, ни IEEE 754) выбор знакового бита
// для NaN, полученного как результат вычисления (например, 0.0/0.0) - это
// деталь конкретной реализации libm, а не что-то, что язык обещает
// воспроизводить побитово.
func formatFloatForPrint(f float64) string {
	switch {
	case math.IsInf(f, 1):
		return "inf"
	case math.IsInf(f, -1):
		return "-inf"
	case math.IsNaN(f):
		return "nan"
	default:
		return strconv.FormatFloat(f, 'g', 6, 64)
	}
}

// printValue - "вывести <выражение>": один формат на примитив, дружелюбный
// плейсхолдер на сущность/файл/сокет - вместо ошибки или печати внутреннего
// Go-представления; подробности значения - через "показать" (showValue).
// Без переноса строки после значения - управляет им сам вызывающий Rech-код
// через "\n" в строке (в отличие от "показать", отладочного инструмента,
// который его сохраняет).
func (in *Interp) printValue(v Value) {
	fmt.Fprint(in.out, in.printText(v))
}

// printText - текст значения v ровно в том виде, что печатает "вывести"
// (и вставка "${...}" для значений без приведения к Строка, см.
// ir.CastExpr.AsPrinted).
func (in *Interp) printText(v Value) string {
	switch v.Type.Kind {
	case ir.KindString:
		return v.S
	case ir.KindInt, ir.KindByte:
		return strconv.FormatInt(v.I, 10)
	case ir.KindInt8, ir.KindInt16, ir.KindInt32:
		return strconv.FormatInt(v.I, 10)
	case ir.KindUint, ir.KindUint16, ir.KindUint32:
		// FormatUint(uint64(v.I)), не FormatInt - Натуральное64 может
		// нести значение выше INT64_MAX, отображаемое как отрицательное,
		// если печатать v.I напрямую как знаковое (см. wrapFixedWidth).
		return strconv.FormatUint(uint64(v.I), 10)
	case ir.KindFloat, ir.KindFloat32:
		return formatFloatForPrint(v.F)
	case ir.KindChar:
		return string(rune(v.I))
	case ir.KindBool:
		if v.B {
			return "истина"
		}
		return "ложь"
	case ir.KindFile:
		return "[файл]"
	case ir.KindSocket:
		return "[сокет]"
	case ir.KindDB:
		return "[бд]"
	case ir.KindHTTP2Session:
		return "[http2-сессия]"
	case ir.KindAny:
		// Содержимое "Значение" - как само значение (cbackend/any_value.go).
		if v.Boxed == nil || v.Boxed.Type.Kind == ir.KindNull {
			return "Ничто"
		}
		return in.printText(*v.Boxed)
	case ir.KindPointer:
		// В отличие от cbackend (настоящий C-указатель, "%p") -
		// pointerTarget интерпретатора не настоящий адрес памяти
		// (package doc pointer.go), печатать нечего, кроме факта "это
		// указатель" - тем же принципом, что и у файла/сокета/БД выше.
		return "[указатель]"
	case ir.KindEntity:
		kind := "сущность"
		if ent, ok := in.entities[v.Obj.typeName]; ok && ent.IsStruct {
			kind = "структура"
		}
		return fmt.Sprintf("[%s]<%s>", kind, v.Obj.typeName)
	case ir.KindArray:
		return fmt.Sprintf("[массив]<%s>", v.Type)
	default:
		return v.S
	}
}

// showValue строит отладочное представление v ("показать") - значение
// вместе с именем типа, рекурсивно для сущностей (поля в порядке
// объявления, взятом из in.entities - у map нет своего порядка) и
// встроенных коллекций (элементы по порядку). Различие коллекции от
// обычной сущности - v.Obj.coll != nil, тот же признак, что и в cloneValue.
func (in *Interp) showValue(v Value) string {
	switch v.Type.Kind {
	case ir.KindString:
		return fmt.Sprintf("\"%s\" (Строка)", v.S)
	case ir.KindInt:
		return fmt.Sprintf("%s (Целое)", strconv.FormatInt(v.I, 10))
	case ir.KindByte:
		return fmt.Sprintf("%s (Байт)", strconv.FormatInt(v.I, 10))
	case ir.KindInt8, ir.KindInt16, ir.KindInt32:
		return fmt.Sprintf("%s (%s)", strconv.FormatInt(v.I, 10), v.Type.String())
	case ir.KindUint, ir.KindUint16, ir.KindUint32:
		return fmt.Sprintf("%s (%s)", strconv.FormatUint(uint64(v.I), 10), v.Type.String())
	case ir.KindFloat32:
		return fmt.Sprintf("%s (Дробное)", formatFloatForPrint(v.F))
	case ir.KindFloat:
		return fmt.Sprintf("%s (Действительное)", formatFloatForPrint(v.F))
	case ir.KindChar:
		return fmt.Sprintf("%s (Символ)", string(rune(v.I)))
	case ir.KindBool:
		word := "ложь"
		if v.B {
			word = "истина"
		}
		return fmt.Sprintf("%s (Логическое)", word)
	case ir.KindEntity:
		if v.Obj.coll != nil {
			if v.Obj.coll.template == "Множество" {
				parts := make([]string, 0, v.Obj.coll.pairs.size())
				for _, key := range v.Obj.coll.pairs.keys {
					parts = append(parts, in.showValue(key))
				}
				return fmt.Sprintf("%s {%s}", v.Obj.typeName, strings.Join(parts, ", "))
			}
			if v.Obj.coll.template == "Словарь" {
				parts := make([]string, 0, v.Obj.coll.pairs.size())
				for i, key := range v.Obj.coll.pairs.keys {
					parts = append(parts, fmt.Sprintf("%s: %s", in.showValue(key), in.showValue(v.Obj.coll.pairs.vals[i])))
				}
				return fmt.Sprintf("%s {%s}", v.Obj.typeName, strings.Join(parts, ", "))
			}
			parts := make([]string, len(v.Obj.coll.elems))
			for i, elem := range v.Obj.coll.elems {
				parts[i] = in.showValue(elem)
			}
			return fmt.Sprintf("%s [%s]", v.Obj.typeName, strings.Join(parts, ", "))
		}
		ent := in.entities[v.Obj.typeName]
		parts := make([]string, len(ent.Fields))
		for i, f := range ent.Fields {
			parts[i] = fmt.Sprintf("%s: %s", f.Name, in.showValue(v.Obj.getField(f.Name)))
		}
		return fmt.Sprintf("%s { %s }", v.Obj.typeName, strings.Join(parts, ", "))
	case ir.KindArray:
		parts := make([]string, len(v.Obj.coll.elems))
		for i, elem := range v.Obj.coll.elems {
			parts[i] = in.showValue(elem)
		}
		return fmt.Sprintf("%s [%s]", v.Type, strings.Join(parts, ", "))
	case ir.KindNullable:
		// Есть значение - показывается как есть (уже несёт собственную
		// пометку типа, см. рекурсию выше) - пометка "(?Тип)" добавляется
		// только у "Ничто", где показывать больше нечего. Тот же принцип,
		// что и в cbackend (generateIRShowExpr, ir.KindNullable).
		if v.Nullable == nil {
			return fmt.Sprintf("Ничто (%s)", v.Type)
		}
		return in.showValue(*v.Nullable)
	default:
		return fmt.Sprintf("%s (%s)", v.S, v.Type)
	}
}
