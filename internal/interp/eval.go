package interp

import (
	"fmt"
	"math"
	"strconv"

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

	case *ir.UnaryExpr:
		return in.evalUnary(e, env, cc)

	case *ir.IncDecExpr:
		return in.evalIncDec(e, env)

	case *ir.BinaryExpr:
		return in.evalBinary(e, env, cc)

	case *ir.CallExpr:
		fn, ok := in.funcs[e.Callee]
		if !ok {
			return Value{}, fmt.Errorf("функция %q не найдена во время исполнения", e.Callee)
		}
		args := make([]Value, len(e.Args))
		for i, a := range e.Args {
			v, err := in.evalExpr(a, env, cc)
			if err != nil {
				return Value{}, err
			}
			args[i] = v
		}
		return in.callFunction(fn, args)

	case *ir.NewExpr:
		return in.evalNewExpr(e, env, cc)

	case *ir.FieldAccess:
		recv, err := in.evalExpr(e.Receiver, env, cc)
		if err != nil {
			return Value{}, err
		}
		return recv.Obj.fields[e.Field], nil

	case *ir.ThisExpr:
		return Value{Type: ir.EntityType(e.Entity), Obj: cc.this}, nil

	case *ir.MethodCallExpr:
		return in.evalMethodCallExpr(e, env, cc)

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
		if v.Type == ir.TypeFloat {
			return floatValue(-v.F), nil
		}
		return intValue(-v.I), nil
	case "не":
		return boolValue(!v.B), nil
	case "~":
		return intValue(^v.I), nil
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
	if v.Type == ir.TypeFloat {
		updated = floatValue(v.F + float64(delta))
	} else {
		updated = intValue(v.I + delta)
	}
	env.set(e.Name, updated)
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
		return arithmetic(e.Op, l, r), nil
	case "-", "*", "/":
		return arithmetic(e.Op, l, r), nil
	case "==":
		return boolValue(valuesEqual(l, r)), nil
	case "!=":
		return boolValue(!valuesEqual(l, r)), nil
	case "<", ">", "<=", ">=":
		return compareOrdered(e.Op, l, r), nil
	case "&":
		return intValue(l.I & r.I), nil
	case "|":
		return intValue(l.I | r.I), nil
	case "^":
		return intValue(l.I ^ r.I), nil
	case "<<":
		return intValue(l.I << uint64(r.I)), nil
	case ">>":
		return intValue(l.I >> uint64(r.I)), nil
	default:
		return Value{}, fmt.Errorf("неизвестная операция %q", e.Op)
	}
}

// arithmetic - "+" (нестроковый)/"-"/"*"/"/": та же промоция Целое->Число
// при смешении, что и в cbackend.generateIRArithmetic (AST/IR-пути).
func arithmetic(op string, l, r Value) Value {
	if l.Type == ir.TypeFloat || r.Type == ir.TypeFloat {
		lf, rf := toFloat(l), toFloat(r)
		switch op {
		case "+":
			return floatValue(lf + rf)
		case "-":
			return floatValue(lf - rf)
		case "*":
			return floatValue(lf * rf)
		case "/":
			return floatValue(lf / rf)
		}
	}
	switch op {
	case "+":
		return intValue(l.I + r.I)
	case "-":
		return intValue(l.I - r.I)
	case "*":
		return intValue(l.I * r.I)
	case "/":
		if r.I == 0 {
			panicRuntime("%s", rtmsg.DivisionByZero)
		}
		return intValue(l.I / r.I)
	}
	return Value{}
}

func toFloat(v Value) float64 {
	if v.Type == ir.TypeInt {
		return float64(v.I)
	}
	return v.F
}

func valuesEqual(l, r Value) bool {
	switch {
	case l.Type == ir.TypeString:
		return l.S == r.S
	case l.Type == ir.TypeBool:
		return l.B == r.B
	case l.Type == ir.TypeFloat || r.Type == ir.TypeFloat:
		return toFloat(l) == toFloat(r)
	default:
		return l.I == r.I
	}
}

func compareOrdered(op string, l, r Value) Value {
	if l.Type == ir.TypeFloat || r.Type == ir.TypeFloat {
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
// встроенная generic-коллекция (шаг 4.2 - Args всегда пуст, буфер начинает
// пустым), см. package doc про то, почему у генериков нет ir.Entity-узла.
func (in *Interp) evalNewExpr(e *ir.NewExpr, env *Env, cc *callCtx) (Value, error) {
	if ent, ok := in.entities[e.Entity]; ok {
		fields := make(map[string]Value, len(ent.Fields))
		for i, f := range ent.Fields {
			v, err := in.evalExpr(e.Args[i], env, cc)
			if err != nil {
				return Value{}, err
			}
			fields[f.Name] = cloneValue(v)
		}
		return Value{Type: ir.EntityType(e.Entity), Obj: &entityObj{typeName: e.Entity, fields: fields}}, nil
	}
	if g, ok := in.generics[e.Entity]; ok {
		return Value{Type: ir.EntityType(e.Entity), Obj: &entityObj{typeName: e.Entity, coll: &collection{template: g.Template}}}, nil
	}
	return Value{}, fmt.Errorf("сущность %q не найдена во время исполнения", e.Entity)
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
	if recv.Obj.coll != nil {
		return in.evalCollectionMethod(recv.Obj, e, env, cc)
	}
	ent := in.entities[recv.Obj.typeName]
	var m *ir.Method
	for _, mm := range ent.Methods {
		if mm.Name == e.Method {
			m = mm
			break
		}
	}
	if m == nil {
		return Value{}, fmt.Errorf("у сущности %q нет метода %q", recv.Obj.typeName, e.Method)
	}
	args := make([]Value, len(e.Args))
	for i, a := range e.Args {
		v, err := in.evalExpr(a, env, cc)
		if err != nil {
			return Value{}, err
		}
		args[i] = v
	}
	return in.callMethod(m, recv.Obj, args)
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
		obj.coll.elems = append(obj.coll.elems, cloneValue(args[0]))
		return Value{}, nil

	case "получить":
		idx := args[0].I
		if idx < 0 || idx >= int64(len(obj.coll.elems)) {
			panicRuntime("%s", rtmsg.IndexOutOfRange(obj.typeName))
		}
		return cloneValue(obj.coll.elems[idx]), nil

	case "установить":
		idx := args[0].I
		if idx < 0 || idx >= int64(len(obj.coll.elems)) {
			panicRuntime("%s", rtmsg.IndexOutOfRange(obj.typeName))
		}
		obj.coll.elems[idx] = cloneValue(args[1])
		return Value{}, nil

	case "снять":
		n := len(obj.coll.elems)
		if n == 0 {
			panicRuntime("%s", rtmsg.EmptyStackOp(obj.typeName))
		}
		v := obj.coll.elems[n-1]
		obj.coll.elems = obj.coll.elems[:n-1]
		return v, nil

	case "верх":
		n := len(obj.coll.elems)
		if n == 0 {
			panicRuntime("%s", rtmsg.EmptyStackOp(obj.typeName))
		}
		return cloneValue(obj.coll.elems[n-1]), nil

	case "извлечь":
		if len(obj.coll.elems) == 0 {
			panicRuntime("%s", rtmsg.EmptyQueueOp(obj.typeName))
		}
		v := obj.coll.elems[0]
		obj.coll.elems = obj.coll.elems[1:]
		return v, nil

	case "первый":
		if len(obj.coll.elems) == 0 {
			panicRuntime("%s", rtmsg.EmptyQueueOp(obj.typeName))
		}
		return cloneValue(obj.coll.elems[0]), nil

	case "длина":
		return intValue(int64(len(obj.coll.elems))), nil

	case "пусто":
		return boolValue(len(obj.coll.elems) == 0), nil

	default:
		return Value{}, fmt.Errorf("встроенная коллекция %q: неизвестный метод %q", obj.typeName, e.Method)
	}
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

func (in *Interp) printValue(v Value) {
	switch v.Type.Kind {
	case ir.KindString:
		fmt.Fprintln(in.out, v.S)
	case ir.KindInt:
		fmt.Fprintln(in.out, strconv.FormatInt(v.I, 10))
	case ir.KindFloat:
		fmt.Fprintln(in.out, formatFloatForPrint(v.F))
	case ir.KindBool:
		if v.B {
			fmt.Fprintln(in.out, "истина")
		} else {
			fmt.Fprintln(in.out, "ложь")
		}
	default:
		fmt.Fprintln(in.out, v.S)
	}
}
