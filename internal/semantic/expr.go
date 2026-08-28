package semantic

import (
	"fmt"

	"github.com/IlyaFeoktistov/Rech/internal/ast"
	"github.com/IlyaFeoktistov/Rech/internal/ir"
)

func analyzeExpr(sc *ctx, e ast.Expr) (ir.Expr, error) {
	switch e := e.(type) {
	case *ast.StringLiteral:
		return &ir.StringLiteral{Value: e.Value}, nil

	case *ast.IntLiteral:
		return &ir.IntLiteral{Value: e.Value}, nil

	case *ast.FloatLiteral:
		return &ir.FloatLiteral{Value: e.Value}, nil

	case *ast.BoolLiteral:
		return &ir.BoolLiteral{Value: e.Value}, nil

	case *ast.CharLiteral:
		return &ir.CharLiteral{Value: e.Value}, nil

	case *ast.CastExpr:
		return analyzeCastExpr(sc, e)

	case *ast.ParseExpr:
		return analyzeParseExpr(sc, e)

	case *ast.SerializeExpr:
		return analyzeSerializeExpr(sc, e)

	case *ast.IsExpr:
		return analyzeIsExpr(sc, e)

	case *ast.LaunchExpr:
		return analyzeLaunchExpr(sc, e)

	case *ast.AwaitExpr:
		await, err := analyzeAwaitExpr(sc, e)
		if err != nil {
			return nil, err
		}
		if !await.HasReturn {
			return nil, fmt.Errorf("строка %d: функция %q не имеет объявленного типа результата - \"дождаться\" здесь нельзя использовать как выражение", e.Line, await.Callee)
		}
		return await, nil

	case *ast.Identifier:
		typ, exists := sc.lookup(e.Name)
		if !exists {
			return nil, fmt.Errorf("строка %d: переменная %q не объявлена", e.Line, e.Name)
		}
		return &ir.Identifier{Name: e.Name, Type: typ}, nil

	case *ast.UnaryExpr:
		return analyzeUnaryExpr(sc, e)

	case *ast.IncDecExpr:
		return analyzeIncDecExpr(sc, e)

	case *ast.BinaryExpr:
		return analyzeBinaryExpr(sc, e)

	case *ast.CallExpr:
		call, err := analyzeCallExpr(sc, e)
		if err != nil {
			return nil, err
		}
		if !callHasReturn(call) {
			return nil, fmt.Errorf("строка %d: %q не имеет объявленного типа результата - её нельзя использовать как выражение", e.Line, e.Callee)
		}
		return call, nil

	case *ast.FuncRefExpr:
		return analyzeFuncRefExpr(sc, e)

	case *ast.LambdaExpr:
		return analyzeLambdaExpr(sc, e)

	case *ast.NewExpr:
		return analyzeNewExpr(sc, e)

	case *ast.FieldAccess:
		// "Имя.Член" перечисления - синтаксически неотличимо от
		// обычного FieldAccess (получатель - идентификатор), парсер не
		// различает их вообще - разрешается здесь, до попытки
		// проанализировать Receiver как обычное выражение: если бы
		// receiver действительно был переменной с этим именем, ниже он
		// победил бы (переменные всегда сильнее перечисления с тем же
		// именем - то же затенение, что и везде в языке), поэтому
		// проверяем sc.lookup ПЕРЕД sc.enums, не наоборот.
		if ident, ok := e.Receiver.(*ast.Identifier); ok {
			if _, isVar := sc.lookup(ident.Name); !isVar {
				// Сначала проверяем перечисление
				if members, isEnum := sc.enums[ident.Name]; isEnum {
					v, ok := members[e.Field]
					if !ok {
						return nil, fmt.Errorf("строка %d: у перечисления %q нет члена %q", e.Line, ident.Name, e.Field)
					}
					return &ir.IntLiteral{Value: v}, nil
				}
				// Константа, объявленная на уровне сущности (Сущность.Константа)
				if sig, isEnt := sc.entities[ident.Name]; isEnt {
					if idx := sig.constIndex(e.Field); idx != -1 {
						if err := checkConstVisibility(sc, ident.Name, e.Field, sig.ConstVisibility[idx], e.Line); err != nil {
							return nil, err
						}
						return resolveEntityConstant(sc, ident.Name, e.Field, e.Line)
					}
				}
			}
		}
		receiver, err := analyzeExpr(sc, e.Receiver)
		if err != nil {
			return nil, err
		}
		typ := ir.TypeOf(receiver)
		if typ.Kind != ir.KindEntity {
			return nil, fmt.Errorf("строка %d: нельзя обратиться к полю %q - тип выражения %s, не сущность", e.Line, e.Field, typ)
		}
		sig := sc.entities[typ.Entity]
		idx := sig.fieldIndex(e.Field)
		if idx == -1 {
			// Не поле экземпляра - возможно, константа сущности, доступная
			// и через экземпляр (Сущность.Константа и экземпляр.Константа -
			// обе формы разрешены).
			if cidx := sig.constIndex(e.Field); cidx != -1 {
				if err := checkConstVisibility(sc, typ.Entity, e.Field, sig.ConstVisibility[cidx], e.Line); err != nil {
					return nil, err
				}
				return resolveEntityConstant(sc, typ.Entity, e.Field, e.Line)
			}
			return nil, fmt.Errorf("строка %d: у сущности %q нет поля %q", e.Line, typ.Entity, e.Field)
		}
		if err := checkFieldVisibility(sc, typ.Entity, e.Field, sig.FieldVisibility[idx], e.Line); err != nil {
			return nil, err
		}
		return &ir.FieldAccess{Receiver: receiver, Field: e.Field, Type: sig.FieldTypes[idx]}, nil

	case *ast.ThisExpr:
		// Парсер уже гарантирует, что "этот" встречается только внутри
		// тела метода - sc.thisEntity здесь всегда заполнен.
		return &ir.ThisExpr{Entity: sc.thisEntity}, nil

	case *ast.MethodCallExpr:
		call, err := analyzeMethodCallExpr(sc, e)
		if err != nil {
			return nil, err
		}
		if !call.HasReturn {
			return nil, fmt.Errorf("строка %d: метод %q не имеет объявленного типа результата - его нельзя использовать как выражение", e.Line, e.Method)
		}
		return call, nil

	default:
		return nil, fmt.Errorf("выражение %T не поддерживается", e)
	}
}

// analyzeNewExpr разбирает "новый Тип(...)" в любой из форм (позиционной
// или именованной) и нормализует Args в позиционный порядок по полю (см.
// package doc ir.NewExpr) - backend'у после этого не нужно знать, какой
// формой был написан конструктор в исходнике.
func analyzeNewExpr(sc *ctx, e *ast.NewExpr) (ir.Expr, error) {
	if _, ok := sc.interfaces[e.Type]; ok {
		return nil, fmt.Errorf("строка %d: %q - интерфейс, у него нет реализации - создать можно только сущность, которая его \"реализует\"", e.Line, e.Type)
	}
	sig, ok := sc.entities[e.Type]
	if !ok {
		// Не пользовательская сущность и ещё не встречавшаяся generic-
		// инстанциация - resolveTypeName при необходимости
		// синтезирует её и зарегистрирует в sc.entities/sc.methods, как и
		// resolveType в cbackend (AST-путь); если e.Type в принципе не
		// тип - вернёт понятную ошибку.
		if _, err := resolveTypeName(sc.entities, sc.methods, sc.aliases, sc.interfaces, e.Type); err != nil {
			return nil, fmt.Errorf("строка %d: %w", e.Line, err)
		}
		sig, ok = sc.entities[e.Type]
		if !ok {
			return nil, fmt.Errorf("строка %d: сущность %q не объявлена", e.Line, e.Type)
		}
	}
	if sig.IsExternOpaque {
		return nil, fmt.Errorf("строка %d: %q - непрозрачный внешний тип (\"внешняя сущность\") - значение можно только получить от внешней функции, не через \"новый\"", e.Line, e.Type)
	}
	if sig.HasCtor {
		if e.FieldNames != nil {
			return nil, fmt.Errorf("строка %d: у сущности %q явный конструктор - именованная форма (поле: значение) недоступна, аргументы передаются позиционно, по объявлению конструктора", e.Line, e.Type)
		}
		if len(e.Args) != len(sig.CtorParams) {
			return nil, fmt.Errorf("строка %d: конструктор %q ожидает аргументов: %d, передано: %d", e.Line, e.Type, len(sig.CtorParams), len(e.Args))
		}
		args := make([]ir.Expr, len(e.Args))
		for i, a := range e.Args {
			arg, err := analyzeExpr(sc, a)
			if err != nil {
				return nil, err
			}
			if typ := ir.TypeOf(arg); !isAssignable(sc.entities, typ, sig.CtorParams[i]) {
				return nil, fmt.Errorf("строка %d: аргумент %d конструктора %q должен быть типа %s, получен %s", e.Line, i+1, e.Type, sig.CtorParams[i], typ)
			}
			args[i] = arg
		}
		return &ir.NewExpr{Entity: e.Type, Args: args}, nil
	}
	args := make([]ir.Expr, len(sig.FieldNames))
	if e.FieldNames != nil {
		if len(e.FieldNames) > len(sig.FieldNames) {
			return nil, fmt.Errorf("строка %d: конструктор %q ожидает значений полей: не более %d, передано: %d", e.Line, e.Type, len(sig.FieldNames), len(e.FieldNames))
		}
		seen := make([]bool, len(sig.FieldNames))
		for i, label := range e.FieldNames {
			idx := sig.fieldIndex(label)
			if idx == -1 {
				return nil, fmt.Errorf("строка %d: у сущности %q нет поля %q", e.Line, e.Type, label)
			}
			if seen[idx] {
				return nil, fmt.Errorf("строка %d: поле %q конструктора %q указано более одного раза", e.Line, label, e.Type)
			}
			seen[idx] = true
			arg, err := analyzeExpr(sc, e.Args[i])
			if err != nil {
				return nil, err
			}
			if typ := ir.TypeOf(arg); !isAssignable(sc.entities, typ, sig.FieldTypes[idx]) {
				return nil, fmt.Errorf("строка %d: поле %q сущности %q должно быть типа %s, получено %s", e.Line, label, e.Type, sig.FieldTypes[idx], typ)
			}
			args[idx] = arg
		}
		for idx, ok := range seen {
			if ok {
				continue
			}
			if idx >= len(sig.FieldDefaultExprs) || sig.FieldDefaultExprs[idx] == nil {
				return nil, fmt.Errorf("строка %d: конструктор %q: не указано значение поля %q (у поля нет значения по умолчанию)", e.Line, e.Type, sig.FieldNames[idx])
			}
			val, err := resolveFieldDefault(sc, e.Type, sig.FieldNames[idx], e.Line)
			if err != nil {
				return nil, err
			}
			args[idx] = val
		}
		return &ir.NewExpr{Entity: e.Type, Args: args}, nil
	}
	if len(e.Args) > len(sig.FieldNames) {
		return nil, fmt.Errorf("строка %d: конструктор %q ожидает значений полей: не более %d, передано: %d", e.Line, e.Type, len(sig.FieldNames), len(e.Args))
	}
	for i, a := range e.Args {
		arg, err := analyzeExpr(sc, a)
		if err != nil {
			return nil, err
		}
		if typ := ir.TypeOf(arg); !isAssignable(sc.entities, typ, sig.FieldTypes[i]) {
			return nil, fmt.Errorf("строка %d: аргумент %d конструктора %q должен быть типа %s, получен %s", e.Line, i+1, e.Type, sig.FieldTypes[i], typ)
		}
		args[i] = arg
	}
	for i := len(e.Args); i < len(sig.FieldNames); i++ {
		if i >= len(sig.FieldDefaultExprs) || sig.FieldDefaultExprs[i] == nil {
			return nil, fmt.Errorf("строка %d: конструктор %q: пропущено значение поля %q (у поля нет значения по умолчанию)", e.Line, e.Type, sig.FieldNames[i])
		}
		val, err := resolveFieldDefault(sc, e.Type, sig.FieldNames[i], e.Line)
		if err != nil {
			return nil, err
		}
		args[i] = val
	}
	return &ir.NewExpr{Entity: e.Type, Args: args}, nil
}

// analyzeMethodCallExpr - общий разбор вызова метода для ExprStmt
// (результат отбрасывается) и для MethodCallExpr-выражения (требует
// HasReturn - проверяется вызывающей стороной, analyzeExpr) - тот же
// принцип, что и analyzeCallExpr для функций.
func analyzeMethodCallExpr(sc *ctx, e *ast.MethodCallExpr) (*ir.MethodCallExpr, error) {
	if !receiverAddressable(e.Receiver) {
		return nil, fmt.Errorf("строка %d: получателем вызова метода может быть только переменная, поле или \"этот\"", e.Line)
	}
	receiver, err := analyzeExpr(sc, e.Receiver)
	if err != nil {
		return nil, err
	}
	typ := ir.TypeOf(receiver)
	var sig funcSig
	switch typ.Kind {
	case ir.KindInterface:
		// Интерфейс - методы без видимости (не бывает "приватный
		// метод интерфейса", это просто контракт), поэтому checkMethodVisibility
		// не вызывается вовсе, не только пропускается с sig.Visibility == ast.Public.
		iface, ok := sc.interfaces[typ.Entity]
		if !ok {
			return nil, fmt.Errorf("строка %d: интерфейс %q не объявлен", e.Line, typ.Entity)
		}
		idx := iface.methodIndex(e.Method)
		if idx == -1 {
			return nil, fmt.Errorf("строка %d: у интерфейса %q нет метода %q", e.Line, typ.Entity, e.Method)
		}
		sig = iface.MethodSigs[idx]
	case ir.KindEntity:
		var ok bool
		sig, ok = sc.methods[typ.Entity][e.Method]
		if !ok {
			return nil, fmt.Errorf("строка %d: у сущности %q нет метода %q", e.Line, typ.Entity, e.Method)
		}
		if err := checkMethodVisibility(sc, typ.Entity, e.Method, sig.Visibility, e.Line); err != nil {
			return nil, err
		}
	default:
		return nil, fmt.Errorf("строка %d: нельзя вызвать метод %q - тип %s не сущность и не интерфейс", e.Line, e.Method, typ)
	}
	args, err := checkCallArgs(sc, e.Args, sig, e.Line, fmt.Sprintf("метод %q", e.Method), fmt.Sprintf("вызова метода %q", e.Method))
	if err != nil {
		return nil, err
	}
	// Постусловие метода (sig.GuaranteedThisStates, см. analyzeMethod)
	// переносится на переменную-получателя - safe только потому, что "этот"
	// передаётся по ссылке (в отличие от обычных параметров, которые
	// копируются, см. package doc funcSig.GuaranteedThisStates).
	if len(sig.GuaranteedThisStates) > 0 {
		switch r := receiver.(type) {
		case *ir.Identifier:
			next := cloneStates(sc.getEstablished(r.Name))
			for st := range sig.GuaranteedThisStates {
				next[st] = true
			}
			sc.setEstablished(r.Name, next)
		case *ir.ThisExpr:
			next := cloneStates(sc.thisEstablished)
			for st := range sig.GuaranteedThisStates {
				next[st] = true
			}
			sc.thisEstablished = next
		}
	}
	return &ir.MethodCallExpr{Receiver: receiver, Entity: typ.Entity, Method: e.Method, Args: args, Type: sig.ReturnType, HasReturn: sig.HasReturn}, nil
}

// analyzeCallExpr - общий разбор вызова функции для ExprStmt (результат
// отбрасывается, HasReturn не важен) и для CallExpr-выражения (требует
// HasReturn - проверяется вызывающей стороной, analyzeExpr). callee - имя,
// под которым сигнатура зарегистрирована в sc.funcs: для обычного вызова -
// e.Callee как есть, для generic-вызова ("первый<Целое>(...)") - мангл-имя
// самой инстанциации ("первый<Целое>"), под которым её уже раскрыл
// instantiateGenerics в обычную функцию, неотличимую для остального
// анализа от любой другой.
// analyzeCallExpr разбирает "имя(<арг>, ...)" - вызов по статически
// известному имени функции (обычный случай) ИЛИ, если имя не найдено среди
// объявленных функций, но названо локальной переменной/полем типа
// "Вызываемый" - вызов ЗНАЧЕНИЯ ("ф(<арг>, ...)", где ф - не имя
// функции). Естественный (падежный) вызов ко второй форме не относится -
// falls через ту же грамматику "имя(...)", но analyzeExpr(e.Callee) как
// произвольного выражения делает падежные роли параметров бессмысленными,
// поэтому вызов значения поддерживает только позиционные аргументы (как и
// решено при проектировании - "только обычный вызов ф(...)").
func analyzeCallExpr(sc *ctx, e *ast.CallExpr) (ir.Expr, error) {
	callee := e.Callee
	if len(e.TypeArgs) > 0 {
		callee = mangleGenericCallName(e.Callee, e.TypeArgs)
	}
	if sig, ok := sc.funcs[callee]; ok {
		if callee == "параметры_пути" {
			if err := registerQueryParamsDictTypeOnCall(sc.entities, sc.methods, sc.aliases, sc.interfaces); err != nil {
				return nil, fmt.Errorf("строка %d: встроенная функция \"параметры_пути\": %w", e.Line, err)
			}
		}
		args, err := checkCallArgs(sc, e.Args, sig, e.Line, fmt.Sprintf("функция %q", callee), fmt.Sprintf("вызова %q", callee))
		if err != nil {
			return nil, err
		}
		return &ir.CallExpr{Callee: callee, Args: args, Type: sig.ReturnType, HasReturn: sig.HasReturn}, nil
	}
	if len(e.TypeArgs) == 0 {
		if typ, ok := sc.lookup(e.Callee); ok && typ.Kind == ir.KindCallable {
			return analyzeCallValueExpr(sc, &ir.Identifier{Name: e.Callee, Type: typ}, e)
		}
	}
	return nil, fmt.Errorf("строка %d: функция %q не объявлена", e.Line, callee)
}

// analyzeCallValueExpr - общая часть analyzeCallExpr (Callee - простое имя
// переменной) для вызова значения типа "Вызываемый" - value уже
// проанализирован (Identifier выше, либо в будущем произвольное выражение,
// если естественный вызов расширится на .поле и т. п.). Сигнатура value
// (Params/Result) берётся из Program.Callables по каноническому имени
// (value.Type.Entity) - a.resolveCallableSig, та же функция, что и
// registerCallable, дважды не резолвит по-разному.
func analyzeCallValueExpr(sc *ctx, value ir.Expr, e *ast.CallExpr) (*ir.CallValueExpr, error) {
	typ := ir.TypeOf(value)
	sig, err := sc.a.resolveCallableSig(typ.Entity)
	if err != nil {
		return nil, fmt.Errorf("строка %d: %w", e.Line, err)
	}
	if len(e.Args) != len(sig.Params) {
		return nil, fmt.Errorf("строка %d: %s ожидает аргументов: %d, передано: %d", e.Line, typ, len(sig.Params), len(e.Args))
	}
	args := make([]ir.Expr, len(e.Args))
	for i, a := range e.Args {
		arg, err := analyzeExpr(sc, a)
		if err != nil {
			return nil, err
		}
		if t := ir.TypeOf(arg); !isAssignable(sc.entities, t, sig.Params[i]) {
			return nil, fmt.Errorf("строка %d: аргумент %d вызова %s должен быть типа %s, получен %s", e.Line, i+1, typ, sig.Params[i], t)
		}
		args[i] = arg
	}
	resultType := ir.TypeUnknown
	hasReturn := sig.Result != nil
	if hasReturn {
		resultType = *sig.Result
	}
	return &ir.CallValueExpr{Value: value, Args: args, Type: resultType, HasReturn: hasReturn}, nil
}

// callHasReturn - HasReturn общего вида для обеих конкретных форм, в
// которые может развернуться analyzeCallExpr (обычный вызов по имени или
// вызов значения) - используется там, где раньше был прямой доступ к
// (*ir.CallExpr).HasReturn.
func callHasReturn(e ir.Expr) bool {
	switch e := e.(type) {
	case *ir.CallExpr:
		return e.HasReturn
	case *ir.CallValueExpr:
		return e.HasReturn
	default:
		return false
	}
}

// checkCallArgs анализирует и проверяет аргументы вызова (обычной функции
// или метода) против сигнатуры sig - количество, тип каждого по позиции, и
// (для параметра, объявленного производным типом состояния) доказанность
// перехода на месте вызова (flow-sensitive typestate). Общая для
// analyzeCallExpr/analyzeMethodCallExpr, которые отличаются только тем, как
// называть вызываемое в тексте ошибки: callee (для "%s ожидает
// аргументов...") и argLabel (для "аргумент N <argLabel> должен быть...").
func checkCallArgs(sc *ctx, args []ast.Expr, sig funcSig, line int, callee, argLabel string) ([]ir.Expr, error) {
	if len(args) != len(sig.Params) {
		return nil, fmt.Errorf("строка %d: %s ожидает аргументов: %d, передано: %d", line, callee, len(sig.Params), len(args))
	}
	out := make([]ir.Expr, len(args))
	for i, a := range args {
		arg, err := analyzeExpr(sc, a)
		if err != nil {
			return nil, err
		}
		if t := ir.TypeOf(arg); !isAssignable(sc.entities, t, sig.Params[i]) {
			return nil, fmt.Errorf("строка %d: аргумент %d %s должен быть типа %s, получен %s", line, i+1, argLabel, sig.Params[i], t)
		}
		if i < len(sig.ParamStates) && sig.ParamStates[i] != "" {
			required := sig.ParamStates[i]
			if !establishedStatesOf(sc, arg)[required] {
				return nil, fmt.Errorf("строка %d: аргумент %d %s требует значение, уже прошедшее переход в состояние %q - для этого выражения такой гарантии нет", line, i+1, argLabel, required)
			}
		}
		out[i] = arg
	}
	return out, nil
}

func analyzeUnaryExpr(sc *ctx, e *ast.UnaryExpr) (ir.Expr, error) {
	operand, err := analyzeExpr(sc, e.Operand)
	if err != nil {
		return nil, err
	}
	typ := ir.TypeOf(operand)
	switch e.Op {
	case "-":
		_, _, isFixedWidth := typ.Kind.IntWidthSigned()
		_, isRanked := numericRanks[typ]
		if !isRanked && !(isFixedWidth && typ.Kind != ir.KindInt) {
			return nil, fmt.Errorf("строка %d: унарный \"-\" применим только к числам, получен %s", e.Line, typ)
		}
		return &ir.UnaryExpr{Op: e.Op, Operand: operand, Type: typ}, nil
	case "не":
		if typ != ir.TypeBool {
			return nil, fmt.Errorf("строка %d: \"не\" применимо только к логическому значению, получен %s", e.Line, typ)
		}
		return &ir.UnaryExpr{Op: e.Op, Operand: operand, Type: ir.TypeBool}, nil
	case "~":
		if typ != ir.TypeInt {
			return nil, fmt.Errorf("строка %d: побитовый \"~\" применим только к Целое, получен %s", e.Line, typ)
		}
		return &ir.UnaryExpr{Op: e.Op, Operand: operand, Type: ir.TypeInt}, nil
	default:
		return nil, fmt.Errorf("строка %d: неизвестная унарная операция %q", e.Line, e.Op)
	}
}

// analyzeIncDecExpr - "имя++"/"имя--"/"++имя"/"--имя" - применимо только к
// уже объявленной переменной типа Целое/Число, как и Assign (не к полю
// сущности, не к результату вызова).
func analyzeIncDecExpr(sc *ctx, e *ast.IncDecExpr) (ir.Expr, error) {
	typ, ok := sc.lookup(e.Name)
	if !ok {
		return nil, fmt.Errorf("строка %d: переменная %q не объявлена", e.Line, e.Name)
	}
	if sc.isConst(e.Name) {
		return nil, fmt.Errorf("строка %d: %q объявлено как \"константа\" - нельзя изменить (%s)", e.Line, e.Name, e.Op)
	}
	_, _, isFixedWidth := typ.Kind.IntWidthSigned()
	_, isRanked := numericRanks[typ]
	if !isRanked && !(isFixedWidth && typ.Kind != ir.KindInt) {
		return nil, fmt.Errorf("строка %d: оператор %q применим только к Целое/Дробное/Действительное, получено %s", e.Line, e.Op, typ)
	}
	return &ir.IncDecExpr{Name: e.Name, Op: e.Op, Prefix: e.Prefix, Type: typ}, nil
}

func analyzeBinaryExpr(sc *ctx, e *ast.BinaryExpr) (ir.Expr, error) {
	left, err := analyzeExpr(sc, e.Left)
	if err != nil {
		return nil, err
	}
	right, err := analyzeExpr(sc, e.Right)
	if err != nil {
		return nil, err
	}
	lt, rt := ir.TypeOf(left), ir.TypeOf(right)

	switch e.Op {
	case "+":
		if lt == ir.TypeString && rt == ir.TypeString {
			return &ir.BinaryExpr{Op: "+", Left: left, Right: right, Type: ir.TypeString}, nil
		}
		return analyzeArithmetic(e, left, lt, right, rt)

	case "-", "*", "/":
		return analyzeArithmetic(e, left, lt, right, rt)

	case "==", "!=":
		if lt == ir.TypeString && rt == ir.TypeString {
			return &ir.BinaryExpr{Op: e.Op, Left: left, Right: right, Type: ir.TypeBool}, nil
		}
		if lt != rt || !equatableTypes[lt] {
			return nil, fmt.Errorf("строка %d: оператор %q не поддерживается для типов %s и %s", e.Line, e.Op, lt, rt)
		}
		return &ir.BinaryExpr{Op: e.Op, Left: left, Right: right, Type: ir.TypeBool}, nil

	case "<", ">", "<=", ">=":
		if lt != rt || !orderableTypes[lt] {
			return nil, fmt.Errorf("строка %d: оператор %q применим только к числам одного типа, получены %s и %s", e.Line, e.Op, lt, rt)
		}
		return &ir.BinaryExpr{Op: e.Op, Left: left, Right: right, Type: ir.TypeBool}, nil

	case "и", "или":
		if lt != ir.TypeBool || rt != ir.TypeBool {
			return nil, fmt.Errorf("строка %d: оператор %q применим только к логическим значениям, получены %s и %s", e.Line, e.Op, lt, rt)
		}
		return &ir.BinaryExpr{Op: e.Op, Left: left, Right: right, Type: ir.TypeBool}, nil

	case "&", "|", "^", "<<", ">>":
		if lt != ir.TypeInt || rt != ir.TypeInt {
			return nil, fmt.Errorf("строка %d: побитовый оператор %q применим только к Целое, получены %s и %s", e.Line, e.Op, lt, rt)
		}
		return &ir.BinaryExpr{Op: e.Op, Left: left, Right: right, Type: ir.TypeInt}, nil

	default:
		return nil, fmt.Errorf("строка %d: неизвестная операция %q", e.Line, e.Op)
	}
}

// analyzeArithmetic - "+" (нестроковый)/"-"/"*"/"/": результат - более
// широкий из двух операндов по numericRanks (Целое -> Дробное ->
// Действительное), тип в IR-узле, без самого приведения ((double)...) -
// приведение операнда, если нужно, решает backend при генерации кода,
// глядя на Type каждого операнда и Type результата.
func analyzeArithmetic(e *ast.BinaryExpr, left ir.Expr, lt ir.Type, right ir.Expr, rt ir.Type) (ir.Expr, error) {
	// Целое8/16/32/64, Натуральное/8/16/32/64 - строго между операндами
	// ОДНОГО И ТОГО ЖЕ Kind, без неявной промоции (в отличие от Целое/
	// Дробное/Действительное ниже, numericRanks) - даже Целое64 к Целое
	// не промоутится, хотя оба 64-битные.
	if lt == rt {
		if _, _, ok := lt.Kind.IntWidthSigned(); ok && lt.Kind != ir.KindInt {
			return &ir.BinaryExpr{Op: e.Op, Left: left, Right: right, Type: lt}, nil
		}
	}
	lr, lok := numericRanks[lt]
	if !lok {
		return nil, fmt.Errorf("строка %d: оператор %q не поддерживается для типа %s", e.Line, e.Op, lt)
	}
	rr, rok := numericRanks[rt]
	if !rok {
		return nil, fmt.Errorf("строка %d: оператор %q не поддерживается для типа %s", e.Line, e.Op, rt)
	}
	result := lt
	if rr > lr {
		result = rt
	}
	return &ir.BinaryExpr{Op: e.Op, Left: left, Right: right, Type: result}, nil
}
