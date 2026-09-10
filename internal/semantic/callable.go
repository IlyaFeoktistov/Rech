package semantic

import (
	"fmt"
	"strings"

	"github.com/IlyaFeoktistov/Rech/internal/ast"
	"github.com/IlyaFeoktistov/Rech/internal/builtins"
	"github.com/IlyaFeoktistov/Rech/internal/ir"
)

// splitCallableTypeString разбирает каноническую строку типа "Вызываемый"
// (см. ir.CallableType/ir.CallableTypeName - "Вызываемый<Целое, Строка>Целое",
// параметры через запятую внутри "<...>", тип результата сразу за
// закрывающей ">" без разделителя, пусто - без результата) на список
// строк-параметров и строку результата - "" false, если name вообще не
// начинается с "Вызываемый<" (не Callable-тип) или скобка не закрыта.
// Собственный разбор, не parseGenericTypeNameMulti (types.go) - та функция
// требует, чтобы имя заканчивалось на ">" (обычный generic-механизм), а
// здесь после закрывающей "<...>" может идти ещё тип результата.
func splitCallableTypeString(name string) (params []string, result string, ok bool) {
	prefix := ir.CallableTypeName + "<"
	if !strings.HasPrefix(name, prefix) {
		return nil, "", false
	}
	rest := name[len(prefix):]
	depth := 1
	closeAt := -1
	for i := 0; i < len(rest); i++ {
		switch rest[i] {
		case '<':
			depth++
		case '>':
			depth--
			if depth == 0 {
				closeAt = i
			}
		}
		if closeAt >= 0 {
			break
		}
	}
	if closeAt < 0 {
		return nil, "", false
	}
	if paramsPart := rest[:closeAt]; paramsPart != "" {
		params = splitTopLevelArgs(paramsPart)
	}
	return params, rest[closeAt+1:], true
}

// resolveCallableTypeName рекурсивно резолвит список типов-параметров и тип
// результата строки "Вызываемый<...>..." - общая часть между resolveTypeName
// (валидация на месте объявления параметра/поля/типа результата - вызывает
// это только затем, чтобы убедиться, что имя корректно, и построить
// ir.Type) и Analyzer.resolveCallableSig (полный разбор для
// Program.Callables, нужен backend'ам для кодогенерации).
func resolveCallableTypeName(entities map[string]entitySig, methods map[string]map[string]funcSig, aliases map[string]string, interfaces map[string]interfaceSig, enums map[string]map[string]int64, name string) (params []ir.Type, result *ir.Type, err error) {
	paramNames, resultName, ok := splitCallableTypeString(name)
	if !ok {
		return nil, nil, fmt.Errorf("неизвестный тип %q", name)
	}
	params = make([]ir.Type, len(paramNames))
	for i, pn := range paramNames {
		t, err := resolveTypeName(entities, methods, aliases, interfaces, enums, pn)
		if err != nil {
			return nil, nil, fmt.Errorf("параметр %d типа %q: %w", i+1, name, err)
		}
		params[i] = t
	}
	if resultName != "" {
		t, err := resolveTypeName(entities, methods, aliases, interfaces, enums, resultName)
		if err != nil {
			return nil, nil, fmt.Errorf("тип результата %q: %w", name, err)
		}
		result = &t
	}
	return params, result, nil
}

// resolveCallableSig - то же самое, что resolveCallableTypeName, но поверх
// собственных карт Analyzer'а (a.entities и т.д. - к моменту вызова уже
// окончательно заполнены) и в форме ir.CallableSig - результат идёт прямо
// в Program.Callables (см. registerCallable).
func (a *Analyzer) resolveCallableSig(name string) (ir.CallableSig, error) {
	params, result, err := resolveCallableTypeName(a.entities, a.methods, a.aliases, a.interfaces, a.enums, name)
	if err != nil {
		return ir.CallableSig{}, err
	}
	return ir.CallableSig{Name: ir.CallableType(params, result).Entity, Params: params, Result: result}, nil
}

// registerCallable добавляет сигнатуру t (если t.Kind == KindCallable и она
// ещё не встречалась) в a.callableOrder - единый список, из которого в
// конце buildIR собирается Program.Callables. Вызывается из двух мест:
// напрямую в точке построения значения (analyzeFuncRefExpr/
// analyzeLambdaExpr - там сигнатура уже разобрана) и из top-level обхода
// сигнатур программы в buildIR (параметр/поле/тип результата, объявленный
// типа "Вызываемый", но без единого фактического значения такого типа,
// построенного где-либо в программе - без этого обхода backend не узнал бы,
// что структуру для него вообще нужно сгенерировать).
func (a *Analyzer) registerCallable(t ir.Type) error {
	if t.Kind != ir.KindCallable || a.callablesSeen[t.Entity] {
		return nil
	}
	// Помечено ДО разбора собственных параметров/результата - защита от
	// бесконечной рекурсии у само-ссылающейся сигнатуры (в общем случае
	// цепочка "Вызываемый<Вызываемый<...>...>..." конечна, но не обязана
	// быть неглубокой).
	a.callablesSeen[t.Entity] = true
	sig, err := a.resolveCallableSig(t.Entity)
	if err != nil {
		return err
	}
	// Параметр/результат сам может быть типом "Вызываемый" (обратный
	// вызов, принимающий/возвращающий другой обратный вызов) - его C-typedef
	// обязан существовать РАНЬШЕ, чем typedef sig (он ссылается на него по
	// имени в поле "fn") - добавляем в callableOrder ПОСЛЕ того, как все
	// вложенные сигнатуры уже в нём (тот же принцип post-order, что и
	// instantiator.ensureEntity/ensureFunc у пользовательских generic-типов,
	// internal/semantic/generics.go).
	for _, p := range sig.Params {
		if err := a.registerCallable(p); err != nil {
			return err
		}
	}
	if sig.Result != nil {
		if err := a.registerCallable(*sig.Result); err != nil {
			return err
		}
	}
	a.callableOrder = append(a.callableOrder, sig)
	return nil
}

// analyzeFuncRefExpr резолвит "&имя" - ссылку на уже объявленную функцию
// программы как на значение типа "Вызываемый", без вызова, ЛИБО (если
// "имя" - не функция, а уже объявленная переменная/параметр) - адрес её
// значения ("Указатель<T>", docs/SPEC.md, "Указатели") - функции
// проверяются первыми: перекрыть имя функции локальной переменной с тем же
// именем нельзя (как и везде в языке - функции/сущности верхнего уровня
// делят одно пространство имён с локальными переменными только в свою
// пользу, не наоборот). Те же ограничения, что у "запустить"
// (analyzeLaunchExpr) и по тем же причинам - не встроенная функция (нет
// единой Rech-сигнатуры для checkCallArgs/C-обёртки), не внешняя (нет
// гарантии, что произвольный C-символ можно безопасно завернуть в общее
// соглашение о вызове через окружение замыкания). Generic-шаблон (не
// инстанциация) отсеивается сам собой - под голым именем в sc.funcs
// зарегистрированы только конкретные функции (инстанциации ищутся под
// мангл-именем, см. instantiateGenerics), у шаблона такой записи нет
// вообще.
func analyzeFuncRefExpr(sc *ctx, e *ast.FuncRefExpr) (ir.Expr, error) {
	if builtins.IsReservedName(e.Name) {
		return nil, fmt.Errorf("строка %d: \"&%s\" не поддерживает встроенные функции - только обычные функции программы", e.Line, e.Name)
	}
	if _, overloaded := sc.a.funcOverloads[e.Name]; overloaded {
		return nil, fmt.Errorf("строка %d: функция %q перегружена - \"&\" не поддерживает перегрузку в этой версии, снимать адрес можно только с неперегруженной функции", e.Line, e.Name)
	}
	sig, ok := sc.funcs[e.Name]
	if !ok {
		if varType, isVar := sc.lookup(e.Name); isVar {
			ptrType, err := pointerTypeFor(sc, varType)
			if err != nil {
				return nil, fmt.Errorf("строка %d: %w", e.Line, err)
			}
			receiver := &ir.Identifier{Name: e.Name, Type: varType}
			return &ir.AddressOfExpr{Receiver: receiver, Type: ptrType, Line: e.Line}, nil
		}
		return nil, fmt.Errorf("строка %d: функция %q не объявлена", e.Line, e.Name)
	}
	if sig.IsExtern {
		return nil, fmt.Errorf("строка %d: \"&%s\" не поддерживает внешние функции (\"внешняя функция\")", e.Line, e.Name)
	}
	var result *ir.Type
	if sig.HasReturn {
		rt := sig.ReturnType
		result = &rt
	}
	t := ir.CallableType(sig.Params, result)
	if err := sc.a.registerCallable(t); err != nil {
		return nil, fmt.Errorf("строка %d: %w", e.Line, err)
	}
	return &ir.FuncRefExpr{FuncName: e.Name, Type: t}, nil
}

// analyzeLambdaExpr анализирует "функция(<парам>: <Тип>, ...): <ТипРезультата>
// { <тело> }" без имени - параметры/тело так же, как у обычной функции
// (analyzeParamsAndBody), но в собственном ctx с parent = sc (newLambdaCtx):
// свободные переменные тела (найденные через ctx.lookup, см. package doc
// ctx.parent) становятся Captures лямбды - копией значения на момент
// создания, тем же правилом, что и обычное присваивание (см. package doc
// ir.CaptureVar). Имя (Name) - уникально на всю программу (a.lambdaCounter),
// backend использует его как есть для C-функции тела лямбды.
func analyzeLambdaExpr(sc *ctx, e *ast.LambdaExpr) (*ir.LambdaExpr, error) {
	params := make([]ir.Type, len(e.Params))
	for i, p := range e.Params {
		t, err := resolveTypeName(sc.entities, sc.methods, sc.aliases, sc.interfaces, sc.enums, p.Type)
		if err != nil {
			return nil, fmt.Errorf("строка %d: лямбда: параметр %q: %w", p.Line, p.Name, err)
		}
		params[i] = t
	}
	var result ir.Type
	hasReturn := e.ReturnType != ""
	if hasReturn {
		t, err := resolveTypeName(sc.entities, sc.methods, sc.aliases, sc.interfaces, sc.enums, e.ReturnType)
		if err != nil {
			return nil, fmt.Errorf("строка %d: лямбда: тип результата: %w", e.Line, err)
		}
		result = t
	}

	lc := newLambdaCtx(sc, result, hasReturn)
	irParams := make([]ir.Param, len(e.Params))
	for i, p := range e.Params {
		irParams[i] = ir.Param{Name: p.Name, Type: params[i]}
		if err := lc.declare(p.Name, params[i]); err != nil {
			return nil, fmt.Errorf("строка %d: параметр %q: %w", p.Line, p.Name, err)
		}
	}
	body, err := analyzeBlock(lc, e.Body)
	if err != nil {
		return nil, err
	}
	if hasReturn && !endsWithReturnValue(body) {
		return nil, fmt.Errorf("строка %d: лямбда с объявленным типом результата %s должна заканчиваться \"вернуть <значение>\"", e.Line, result)
	}

	var resultPtr *ir.Type
	if hasReturn {
		resultPtr = &result
	}
	t := ir.CallableType(params, resultPtr)
	if err := sc.a.registerCallable(t); err != nil {
		return nil, fmt.Errorf("строка %d: %w", e.Line, err)
	}

	captures := make([]ir.CaptureVar, len(lc.captureOrder))
	for i, name := range lc.captureOrder {
		captures[i] = ir.CaptureVar{Name: name, Type: lc.captures[name]}
	}

	sc.a.lambdaCounter++
	name := fmt.Sprintf("__лямбда_%d", sc.a.lambdaCounter)
	return &ir.LambdaExpr{
		Name: name, Params: irParams, ReturnType: result, HasReturn: hasReturn,
		Captures: captures, Body: body, Type: t,
	}, nil
}
