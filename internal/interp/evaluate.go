// Вычисление произвольного Rech-выражения в контексте остановленного кадра
// ("Debug Console"/watch в VS Code) - Evaluate (debugger.go) разбирает
// текст через parser.ParseExpr (одно выражение, не вся программа) и
// исполняет результат этим файлом, а не через обычный evalExpr
// (internal/interp/eval.go, работает над ir.Expr - типы уже разрешены
// семантическим анализом ЦЕЛОЙ программы наперёд). Здесь типа наперёд нет -
// он у каждого имени известен только В МОМЕНТ вычисления, из уже
// остановленного Env/this, поэтому evalAstExpr сам решает тип по ходу,
// переиспользуя уже существующие операции над Value (arithmetic/
// compareOrdered/valuesEqual, eval.go) - не копия семантики evalExpr, а
// динамический аналог того же самого прочтения синтаксиса.
//
// Сознательно не поддержано (см. docs/ROADMAP.md, шаг 19): "новый Тип(...)"
// (создание сущности), явные касты, "относится к", "запустить"/"дождаться",
// лямбда - watch-выражение читает уже существующее состояние, не строит
// новое; попытка использовать любую из этих конструкций - явная ошибка
// "не поддерживается в Debug Console", а не молчаливое иное поведение.
package interp

import (
	"fmt"

	"github.com/IlyaFeoktistov/Rech/internal/ast"
	"github.com/IlyaFeoktistov/Rech/internal/ir"
	"github.com/IlyaFeoktistov/Rech/internal/lexer"
	"github.com/IlyaFeoktistov/Rech/internal/parser"
)

// Evaluate разбирает и вычисляет expr в контексте кадра frameID
// (Debugger.frameByID - тот же env/this, что видны там во время исполнения)
// - возвращает представление результата тем же форматом, что и "показать"
// (showValue, eval.go): значение вместе с именем типа, рекурсивно для
// сущностей/коллекций.
func (d *Debugger) Evaluate(frameID int, expr string) (string, error) {
	f := d.frameByID(frameID)
	if f == nil {
		return "", fmt.Errorf("кадр %d не найден (поток уже не стоит на паузе)", frameID)
	}
	toks, err := lexer.New(expr).Tokenize()
	if err != nil {
		return "", fmt.Errorf("лексер: %w", err)
	}
	e, err := parser.New(toks).ParseExpr()
	if err != nil {
		return "", fmt.Errorf("разбор выражения: %w", err)
	}
	v, err := d.in.evalAstExpr(e, f.env, f.this)
	if err != nil {
		return "", err
	}
	return d.in.showValue(v), nil
}

// evalAstExpr - см. package doc. env/this - точка остановки (frame.env/
// frame.this), не меняются по ходу вычисления одного выражения (watch
// выражение не исполняет инструкций, только вычисляет значение).
func (in *Interp) evalAstExpr(e ast.Expr, env *Env, this *entityObj) (Value, error) {
	switch e := e.(type) {
	case *ast.StringLiteral:
		return stringValue(e.Value), nil
	case *ast.IntLiteral:
		return intValue(e.Value), nil
	case *ast.FloatLiteral:
		return floatValue(e.Value), nil
	case *ast.BoolLiteral:
		return boolValue(e.Value), nil
	case *ast.CharLiteral:
		return charValue(e.Value), nil

	case *ast.Identifier:
		if v, ok := env.get(e.Name); ok {
			return v, nil
		}
		return Value{}, fmt.Errorf("переменная %q не найдена в остановленном кадре", e.Name)

	case *ast.ThisExpr:
		if this == nil {
			return Value{}, fmt.Errorf("\"этот\" недоступно - кадр не является методом")
		}
		return Value{Type: ir.EntityType(this.typeName), Obj: this}, nil

	case *ast.FieldAccess:
		recv, err := in.evalAstExpr(e.Receiver, env, this)
		if err != nil {
			return Value{}, err
		}
		if recv.Obj == nil {
			return Value{}, fmt.Errorf("значение пусто, у него нет поля %q", e.Field)
		}
		fv, ok := recv.Obj.getFieldOK(e.Field)
		if !ok {
			return Value{}, fmt.Errorf("у сущности %q нет поля %q", recv.Obj.typeName, e.Field)
		}
		return fv, nil

	case *ast.UnaryExpr:
		v, err := in.evalAstExpr(e.Operand, env, this)
		if err != nil {
			return Value{}, err
		}
		switch e.Op {
		case "-":
			if v.Type == ir.TypeFloat || v.Type == ir.TypeFloat32 {
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

	case *ast.BinaryExpr:
		return in.evalAstBinary(e, env, this)

	case *ast.CallExpr:
		args, err := in.evalAstArgs(e.Args, env, this)
		if err != nil {
			return Value{}, err
		}
		if v, ok := in.evalBuiltinCall(e.Callee, args); ok {
			return v, nil
		}
		fn, ok := in.funcs[e.Callee]
		if !ok {
			return Value{}, fmt.Errorf("функция %q не найдена", e.Callee)
		}
		// dbg: nil - вызов из Debug Console не должен САМ рекурсивно
		// остановиться на точке останова внутри вычисляемого выражения
		// (см. package doc).
		return in.callFunction(fn, args, nil)

	case *ast.MethodCallExpr:
		recv, err := in.evalAstExpr(e.Receiver, env, this)
		if err != nil {
			return Value{}, err
		}
		if recv.Obj == nil {
			return Value{}, fmt.Errorf("значение пусто, у него нет метода %q", e.Method)
		}
		args, err := in.evalAstArgs(e.Args, env, this)
		if err != nil {
			return Value{}, err
		}
		if recv.Obj.coll != nil {
			return in.evalAstCollectionMethod(recv.Obj, e.Method, args)
		}
		m := in.findMethod(recv.Obj.typeName, e.Method)
		if m == nil {
			return Value{}, fmt.Errorf("у сущности %q нет метода %q", recv.Obj.typeName, e.Method)
		}
		return in.callMethod(m, recv.Obj, args, nil)

	default:
		return Value{}, fmt.Errorf("это выражение не поддерживается в Debug Console (%T)", e)
	}
}

func (in *Interp) evalAstArgs(exprs []ast.Expr, env *Env, this *entityObj) ([]Value, error) {
	args := make([]Value, len(exprs))
	for i, a := range exprs {
		v, err := in.evalAstExpr(a, env, this)
		if err != nil {
			return nil, err
		}
		args[i] = v
	}
	return args, nil
}

// evalAstBinary - "и"/"или" с коротким замыканием, тот же принцип, что и у
// evalBinary (eval.go) над ir.BinaryExpr - остальные операции переиспользуют
// те же самые свободные функции (arithmetic/valuesEqual/compareOrdered),
// решение типа результата динамическое (по фактическому Value.Type
// операндов), а не заранее известное семантическим анализом.
func (in *Interp) evalAstBinary(e *ast.BinaryExpr, env *Env, this *entityObj) (Value, error) {
	if e.Op == "и" || e.Op == "или" {
		l, err := in.evalAstExpr(e.Left, env, this)
		if err != nil {
			return Value{}, err
		}
		if e.Op == "и" && !l.B {
			return boolValue(false), nil
		}
		if e.Op == "или" && l.B {
			return boolValue(true), nil
		}
		r, err := in.evalAstExpr(e.Right, env, this)
		if err != nil {
			return Value{}, err
		}
		return boolValue(r.B), nil
	}
	l, err := in.evalAstExpr(e.Left, env, this)
	if err != nil {
		return Value{}, err
	}
	r, err := in.evalAstExpr(e.Right, env, this)
	if err != nil {
		return Value{}, err
	}
	switch e.Op {
	case "+":
		if l.Type == ir.TypeString && r.Type == ir.TypeString {
			return stringValue(l.S + r.S), nil
		}
		return arithmetic(e.Op, l, r)
	case "-", "*", "/":
		return arithmetic(e.Op, l, r)
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

// evalAstCollectionMethod - только для чтения (длина/пусто/получить/верх/
// первый у Список/Стопка/Очередь; длина/пусто/получить/содержит у Словарь),
// без мутации (добавить/положить/установить/снять/извлечь/удалить) - watch-
// выражение не должно незаметно менять состояние программы (в отличие от
// вызова обычной функции/метода выше, где это уже риск, на который идут
// осознанно - тот же выбор, что и у настоящих отладчиков).
func (in *Interp) evalAstCollectionMethod(obj *entityObj, method string, args []Value) (Value, error) {
	if obj.coll.template == "Словарь" {
		switch method {
		case "получить":
			v, ok := obj.coll.pairs[args[0]]
			if !ok {
				return Value{}, fmt.Errorf("ключ не найден в %s", obj.typeName)
			}
			return v, nil
		case "содержит":
			_, ok := obj.coll.pairs[args[0]]
			return boolValue(ok), nil
		case "длина":
			return intValue(int64(len(obj.coll.pairs))), nil
		case "пусто":
			return boolValue(len(obj.coll.pairs) == 0), nil
		default:
			return Value{}, fmt.Errorf("%q в Debug Console не поддерживается (изменяет словарь)", method)
		}
	}
	switch method {
	case "получить":
		idx := args[0].I
		if idx < 0 || idx >= int64(len(obj.coll.elems)) {
			return Value{}, fmt.Errorf("индекс вне границ у %s", obj.typeName)
		}
		return obj.coll.elems[idx], nil
	case "верх":
		if len(obj.coll.elems) == 0 {
			return Value{}, fmt.Errorf("%s пуста", obj.typeName)
		}
		return obj.coll.elems[len(obj.coll.elems)-1], nil
	case "первый":
		if len(obj.coll.elems) == 0 {
			return Value{}, fmt.Errorf("%s пуста", obj.typeName)
		}
		return obj.coll.elems[0], nil
	case "длина":
		return intValue(int64(len(obj.coll.elems))), nil
	case "пусто":
		return boolValue(len(obj.coll.elems) == 0), nil
	default:
		return Value{}, fmt.Errorf("%q в Debug Console не поддерживается (изменяет коллекцию)", method)
	}
}
