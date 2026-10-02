package semantic

import (
	"errors"
	"fmt"

	"github.com/IlyaFeoktistov/Rech/internal/ast"
	"github.com/IlyaFeoktistov/Rech/internal/diagnostics"
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

	case *ast.NullLiteral:
		// Без контекста ожидаемого типа "Ничто" не разбирается вообще -
		// каждое место, где оно синтаксически валидно (объявление/
		// присваивание/аргумент/поле/результат/сравнение "== Ничто"),
		// перехватывает его САМО, до попытки вызвать analyzeExpr вообще
		// (см. coerceExpr/analyzeNullCheck, internal/semantic/nullable.go) -
		// если дошло досюда, значит употреблено там, где типа взять неоткуда
		// ("вывести Ничто", аргумент "??" справа и т. п.).
		return nil, errAtLine(e.Line, CodeExprCheck, "\"Ничто\" здесь неуместно - тип неоткуда вывести (нужен явный контекст: объявление/присваивание nullable-переменной, сравнение с nullable-значением)")

	case *ast.CastExpr:
		return analyzeCastExpr(sc, e)

	case *ast.ParseExpr:
		return analyzeParseExpr(sc, e)

	case *ast.SerializeExpr:
		return analyzeSerializeExpr(sc, e)

	case *ast.IsExpr:
		return analyzeIsExpr(sc, e)

	case *ast.LaunchExpr:
		launch, err := analyzeLaunchExpr(sc, e)
		if err != nil {
			return nil, err
		}
		return launch, nil

	case *ast.CoroLaunchExpr:
		return analyzeCoroLaunchExpr(sc, e)

	case *ast.AwaitExpr:
		await, err := analyzeAwaitExpr(sc, e)
		if err != nil {
			return nil, err
		}
		if !await.HasReturn {
			return nil, errAtLine(e.Line, CodeExprCheck, "у задачи нет результата (сигнатура без типа результата) - \"дождаться\" можно только отдельной инструкцией, не как выражение")
		}
		return await, nil

	case *ast.Identifier:
		typ, exists := sc.lookup(e.Name)
		if !exists {
			if _, isModuleConst := sc.a.moduleConstExprs[e.Name]; isModuleConst {
				return resolveModuleConstant(sc, e.Name, e.Line)
			}
			if sc.a.laterModuleVars[e.Name] {
				return nil, errModuleVarDeclaredLater(e.Name, e.Line)
			}
			d := errAtLine(e.Line, CodeNameResolution, "переменная %q не объявлена", e.Name)
			if hint := suggestNameHint(e.Name, sc.visibleNames()); hint != "" {
				d.WithHint("%s", hint)
			}
			return nil, d
		}
		if sc.readBeforeAssign(e.Name) {
			return nil, errReadBeforeAssign(e.Name, e.Line)
		}
		// Захвачено лямбдой по ссылке ("&имя" в явном списке захвата,
		// package doc ir.CaptureVar) - чтение прозрачно идёт через
		// указатель (DerefExpr), не напрямую - тем же приёмом, что и у
		// ручного "*указ".
		if ptrType, isByRef := sc.byRefPointerType(e.Name); isByRef {
			return &ir.DerefExpr{Value: &ir.Identifier{Name: e.Name, Type: ptrType}, Type: typ, Line: e.Line}, nil
		}
		return &ir.Identifier{Name: e.Name, Type: typ}, nil

	case *ast.UnaryExpr:
		return analyzeUnaryExpr(sc, e)

	case *ast.IncDecExpr:
		return analyzeIncDecExpr(sc, e)

	case *ast.BinaryExpr:
		return analyzeBinaryExpr(sc, e)

	case *ast.TernaryExpr:
		return analyzeTernaryExpr(sc, e)

	case *ast.CallExpr:
		call, err := analyzeCallExpr(sc, e)
		if err != nil {
			return nil, err
		}
		if !callHasReturn(call) {
			return nil, errAtLine(e.Line, CodeExprCheck, "%q не имеет объявленного типа результата - её нельзя использовать как выражение", e.Callee)
		}
		return call, nil

	case *ast.FuncRefExpr:
		return analyzeFuncRefExpr(sc, e)

	case *ast.AddressOfExpr:
		return analyzeAddressOfExpr(sc, e)

	case *ast.DerefExpr:
		return analyzeDerefExpr(sc, e)

	case *ast.LambdaExpr:
		return analyzeLambdaExpr(sc, e)

	case *ast.NewExpr:
		return analyzeNewExpr(sc, e)

	case *ast.FieldAccess:
		if e.Static {
			return analyzeStaticFieldAccess(sc, e)
		}
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
						return nil, errAtLine(e.Line, CodeExprCheck, "у перечисления %q нет члена %q", ident.Name, e.Field)
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
		receiver = maybeUnwrapNarrowed(sc, receiver)
		typ := ir.TypeOf(receiver)
		if typ.Kind != ir.KindEntity {
			return nil, errAtLine(e.Line, CodeExprCheck, "нельзя обратиться к полю %q - тип выражения %s, не сущность", e.Field, typ)
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
			if sig.staticFieldIndex(e.Field) != -1 {
				return nil, errAtLine(e.Line, CodeExprCheck, "%q - статическое поле сущности %q, доступ только через \"%s::%s\", не через экземпляр", e.Field, typ.Entity, typ.Entity, e.Field)
			}
			d := errAtLine(e.Line, CodeNameResolution, "у сущности %q нет поля %q", typ.Entity, e.Field)
			if hint := suggestNameHint(e.Field, sig.FieldNames); hint != "" {
				d.WithHint("%s", hint)
			}
			return nil, d
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
		if e.Static {
			call, err := analyzeStaticMethodCallExpr(sc, e)
			if err != nil {
				return nil, err
			}
			if !call.HasReturn {
				return nil, errAtLine(e.Line, CodeExprCheck, "статический метод %q не имеет объявленного типа результата - его нельзя использовать как выражение", e.Method)
			}
			return call, nil
		}
		if desugared, handled, derr := maybeDesugarHandleMethod(sc, e); handled {
			return desugared, derr
		}
		if desugared, handled, derr := maybeDesugarDateTimeMethod(sc, e); handled {
			return desugared, derr
		}
		call, err := analyzeMethodCallExpr(sc, e)
		if err != nil {
			return nil, err
		}
		if !callHasReturn(call) {
			return nil, errAtLine(e.Line, CodeExprCheck, "метод %q не имеет объявленного типа результата - его нельзя использовать как выражение", e.Method)
		}
		return call, nil

	case *ast.TypeOfExpr:
		return analyzeTypeOfExpr(sc, e)

	case *ast.AsExpr:
		return analyzeAsExpr(sc, e)

	case *ast.IndexExpr:
		return analyzeIndexExpr(sc, e)

	case *ast.ListLiteral:
		return analyzeListLiteralExpr(sc, e, ir.Type{}, false)
	case *ast.SpreadExpr:
		return nil, errAtLine(e.Line, CodeExprCheck, "\"...\" допустимо только в литерале списка и в вариативной части аргументов вызова")

	default:
		return nil, errNoPos(diagnostics.CodeInternal, "выражение %T не поддерживается", e)
	}
}

// analyzeNewExpr разбирает "новый Тип(...)" в любой из форм (позиционной
// или именованной) и нормализует Args в позиционный порядок по полю (см.
// package doc ir.NewExpr) - backend'у после этого не нужно знать, какой
// формой был написан конструктор в исходнике.
func analyzeNewExpr(sc *ctx, e *ast.NewExpr) (ir.Expr, error) {
	if inferred, ok, err := inferredNewType(sc, e); err != nil {
		return nil, err
	} else if ok {
		withType := *e
		withType.Type = inferred
		e = &withType
	}
	// "новый Словарь<Цвет, Строка>()"/"новый Список<Целое64>()" - то же
	// каноническое имя инстанциации, что у аннотации типа (resolveTypeName).
	if canon := enumArgsAsInt(canonicalTypeName(e.Type), sc.enums); canon != e.Type {
		withType := *e
		withType.Type = canon
		e = &withType
	}
	if _, ok := sc.interfaces[e.Type]; ok {
		return nil, errAtLine(e.Line, CodeExprCheck, "%q - интерфейс, у него нет реализации - создать можно только сущность, которая его \"реализует\"", e.Type)
	}
	sig, ok := sc.entities[e.Type]
	if !ok {
		// Не пользовательская сущность и ещё не встречавшаяся generic-
		// инстанциация - resolveTypeName при необходимости
		// синтезирует её и зарегистрирует в sc.entities/sc.methods, как и
		// resolveType в cbackend (AST-путь); если e.Type в принципе не
		// тип - вернёт понятную ошибку.
		if _, err := resolveTypeName(sc.entities, sc.methods, sc.aliases, sc.interfaces, sc.enums, e.Type); err != nil {
			return nil, diagnostics.WithPos(err, diagnostics.Position{File: currentFile, Line: e.Line})
		}
		sig, ok = sc.entities[e.Type]
		if !ok {
			return nil, errAtLine(e.Line, CodeNameResolution, "сущность %q не объявлена", e.Type)
		}
	}
	if sig.IsExternOpaque {
		return nil, errAtLine(e.Line, CodeExprCheck, "%q - непрозрачный внешний тип (\"внешняя сущность\") - значение можно только получить от внешней функции, не через \"новый\"", e.Type)
	}
	if sig.IsTransaction {
		return nil, errAtLine(e.Line, CodeExprCheck, "%q - значение можно получить только параметром колбэка \"%s\", не через \"новый\"", e.Type, beginTransactionFuncName)
	}
	if sig.IsAbstract {
		return nil, errAtLine(e.Line, CodeExprCheck, "%q - абстрактная сущность, создать экземпляр напрямую нельзя - унаследуйте её и реализуйте недостающие методы", e.Type)
	}
	if sig.HasCtor {
		if e.FieldNames != nil {
			return analyzeNewExprCtorNamed(sc, e, sig)
		}
		// Синтетическая funcSig, а не собственная копия проверки - те же
		// правила (в т.ч. заполнение хвоста CtorParamDefaults), что и у
		// обычного вызова функции/метода, одним местом (checkCallArgs).
		ctorSig := funcSig{Params: sig.CtorParams, ParamDefaults: sig.CtorParamDefaults}
		args, err := checkCallArgs(sc, e.Args, ctorSig, e.Line, fmt.Sprintf("конструктор %q", e.Type), fmt.Sprintf("конструктора %q", e.Type))
		if err != nil {
			return nil, err
		}
		return &ir.NewExpr{Entity: e.Type, Args: args}, nil
	}
	if !sig.IsStruct {
		// Сущность/класс без явного конструктора - только "новый Тип()", без
		// аргументов (docs/SPEC.md, "Явные конструкторы"): неявный
		// конструктор "как в нормальных языках" ничего не знает про поля -
		// они получают значения по умолчанию ("имеет: поле: Тип = выражение")
		// либо зануляются GC_MALLOC'ом (тот же путь, что и необъявленные в
		// теле явного конструктора поля, см. generateIREntityCtorInit).
		// Полю без значения по умолчанию, которое обязательно нужно
		// задать при создании - ровно тот случай, ради которого стоит
		// написать явный "конструктор(...)".
		if len(e.Args) > 0 || e.FieldNames != nil {
			return nil, errAtLine(e.Line, CodeExprCheck, "у сущности %q нет явного конструктора - объявите \"конструктор(...)\" в её теле, либо создайте без аргументов (\"новый %s()\"), поля получат значения по умолчанию", e.Type, e.Type)
		}
		return &ir.NewExpr{Entity: e.Type}, nil
	}
	args := make([]ir.Expr, len(sig.FieldNames))
	if e.FieldNames != nil {
		if len(e.FieldNames) > len(sig.FieldNames) {
			return nil, errAtLine(e.Line, CodeExprCheck, "конструктор %q ожидает значений полей: не более %d, передано: %d", e.Type, len(sig.FieldNames), len(e.FieldNames))
		}
		seen := make([]bool, len(sig.FieldNames))
		for i, label := range e.FieldNames {
			idx := sig.fieldIndex(label)
			if idx == -1 {
				return nil, errAtLine(e.Line, CodeNameResolution, "у сущности %q нет поля %q", e.Type, label)
			}
			if seen[idx] {
				return nil, errAtLine(e.Line, CodeDuplicateDecl, "поле %q конструктора %q указано более одного раза", label, e.Type)
			}
			seen[idx] = true
			arg, err := coerceExpr(sc, e.Args[i], sig.FieldTypes[idx])
			if err != nil {
				return nil, err
			}
			if typ := ir.TypeOf(arg); !isAssignable(sc.entities, sc.interfaces, typ, sig.FieldTypes[idx]) {
				return nil, errAtLine(e.Line, CodeExprCheck, "поле %q сущности %q должно быть типа %s, получено %s", label, e.Type, sig.FieldTypes[idx], typ)
			}
			args[idx] = arg
		}
		for idx, ok := range seen {
			if ok {
				continue
			}
			if idx >= len(sig.FieldDefaultExprs) || sig.FieldDefaultExprs[idx] == nil {
				return nil, errAtLine(e.Line, CodeExprCheck, "конструктор %q: не указано значение поля %q (у поля нет значения по умолчанию)", e.Type, sig.FieldNames[idx])
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
		return nil, errAtLine(e.Line, CodeExprCheck, "конструктор %q ожидает значений полей: не более %d, передано: %d", e.Type, len(sig.FieldNames), len(e.Args))
	}
	for i, a := range e.Args {
		arg, err := coerceExpr(sc, a, sig.FieldTypes[i])
		if err != nil {
			return nil, err
		}
		if typ := ir.TypeOf(arg); !isAssignable(sc.entities, sc.interfaces, typ, sig.FieldTypes[i]) {
			return nil, errAtLine(e.Line, CodeExprCheck, "аргумент %d конструктора %q должен быть типа %s, получен %s", i+1, e.Type, sig.FieldTypes[i], typ)
		}
		args[i] = arg
	}
	for i := len(e.Args); i < len(sig.FieldNames); i++ {
		if i >= len(sig.FieldDefaultExprs) || sig.FieldDefaultExprs[i] == nil {
			return nil, errAtLine(e.Line, CodeExprCheck, "конструктор %q: пропущено значение поля %q (у поля нет значения по умолчанию)", e.Type, sig.FieldNames[i])
		}
		val, err := resolveFieldDefault(sc, e.Type, sig.FieldNames[i], e.Line)
		if err != nil {
			return nil, err
		}
		args[i] = val
	}
	return &ir.NewExpr{Entity: e.Type, Args: args}, nil
}

// analyzeNewExprCtorNamed разбирает именованную форму "новый Тип(параметр:
// значение, ...)" у сущности с явным конструктором - по именам ПАРАМЕТРОВ
// конструктора (sig.CtorParamNames), не полей (они не обязаны совпадать, см.
// package doc entitySig.HasCtor). Параметр, не указанный явно, - ошибка,
// если у него нет собственного значения по умолчанию (CtorParamDefaults),
// иначе - resolveParamDefault, тем же принципом, что и у пропущенного поля
// в именованной форме "новый" без конструктора (analyzeNewExpr выше).
func analyzeNewExprCtorNamed(sc *ctx, e *ast.NewExpr, sig entitySig) (ir.Expr, error) {
	if len(e.FieldNames) > len(sig.CtorParamNames) {
		return nil, errAtLine(e.Line, CodeExprCheck, "конструктор %q ожидает аргументов: не более %d, передано: %d", e.Type, len(sig.CtorParamNames), len(e.FieldNames))
	}
	args := make([]ir.Expr, len(sig.CtorParamNames))
	seen := make([]bool, len(sig.CtorParamNames))
	for i, label := range e.FieldNames {
		idx := -1
		for j, name := range sig.CtorParamNames {
			if name == label {
				idx = j
				break
			}
		}
		if idx == -1 {
			return nil, errAtLine(e.Line, CodeExprCheck, "у конструктора %q нет параметра %q", e.Type, label)
		}
		if seen[idx] {
			return nil, errAtLine(e.Line, CodeDuplicateDecl, "параметр %q конструктора %q указан более одного раза", label, e.Type)
		}
		seen[idx] = true
		arg, err := coerceExpr(sc, e.Args[i], sig.CtorParams[idx])
		if err != nil {
			return nil, err
		}
		if typ := ir.TypeOf(arg); !isAssignable(sc.entities, sc.interfaces, typ, sig.CtorParams[idx]) {
			return nil, errAtLine(e.Line, CodeExprCheck, "параметр %q конструктора %q должен быть типа %s, получен %s", label, e.Type, sig.CtorParams[idx], typ)
		}
		args[idx] = arg
	}
	for idx, ok := range seen {
		if ok {
			continue
		}
		if idx >= len(sig.CtorParamDefaults) || sig.CtorParamDefaults[idx] == nil {
			return nil, errAtLine(e.Line, CodeExprCheck, "конструктор %q: не указан параметр %q (у параметра нет значения по умолчанию)", e.Type, sig.CtorParamNames[idx])
		}
		val, err := resolveParamDefault(sc, sig.CtorParamDefaults, idx, sig.CtorParams[idx], e.Line)
		if err != nil {
			return nil, err
		}
		args[idx] = val
	}
	return &ir.NewExpr{Entity: e.Type, Args: args}, nil
}

// analyzeMethodCallExpr - общий разбор вызова метода для ExprStmt
// (результат отбрасывается) и для MethodCallExpr-выражения (требует
// callHasReturn - проверяется вызывающей стороной, analyzeExpr) - тот же
// принцип, что и analyzeCallExpr для функций. Возвращает ir.Expr, не
// *ir.MethodCallExpr - "получатель.имя(...)" сущности, у которой нет
// метода "имя", но есть ПОЛЕ "имя" типа "Вызываемый(...)" разворачивается в
// *ir.CallValueExpr (см. ветку ir.KindEntity ниже) - тот же приём, что и у
// maybeDesugarHandleMethod/maybeDesugarDateTimeMethod, только не отдельной
// функцией "maybeDesugar...", а прямо здесь: решение "это метод или поле"
// зависит от той же сигнатуры сущности (sc.methods/sc.entities), что и
// сам обычный путь метода, второй раз резолвить её незачем.
func analyzeMethodCallExpr(sc *ctx, e *ast.MethodCallExpr) (ir.Expr, error) {
	receiver, err := analyzeExpr(sc, e.Receiver)
	if err != nil {
		return nil, err
	}
	receiver = maybeUnwrapNarrowed(sc, receiver)
	typ := ir.TypeOf(receiver)
	// Получатель НЕ обязан быть адресуемым (переменная/поле/"этот") на
	// уровне семантики - cbackend сам решает, брать ли "&" от C-кода
	// получателя напрямую (сущность-ссылка/интерфейс - уже указатель) или
	// материализовать временную переменную (получатель - "структура",
	// результат вызова без постоянного адреса - см. generateIRAddressableExpr,
	// internal/backend/cbackend/ir_generator.go). Поэтому цепочка
	// `x.метод1().метод2()` любой глубины допустима для ЛЮБОГО типа
	// получателя, не только ссылочного.
	var sig funcSig
	// method - имя, которое реально попадёт в ir.MethodCallExpr.Method -
	// мангл-имя у перегруженного метода, e.Method (как есть) иначе (см.
	// package doc funcSig.MangledName, resolveFuncSigForCall).
	method := e.Method
	switch typ.Kind {
	case ir.KindInterface:
		// Интерфейс - методы без видимости (не бывает "приватный
		// метод интерфейса", это просто контракт), поэтому checkMethodVisibility
		// не вызывается вовсе, не только пропускается с sig.Visibility == ast.Public.
		iface, ok := sc.interfaces[typ.Entity]
		if !ok {
			return nil, errAtLine(e.Line, CodeNameResolution, "интерфейс %q не объявлен", typ.Entity)
		}
		idx := iface.methodIndex(e.Method)
		if idx == -1 {
			return nil, errAtLine(e.Line, CodeNameResolution, "у интерфейса %q нет метода %q", typ.Entity, e.Method)
		}
		sig = iface.MethodSigs[idx]
	case ir.KindEntity:
		osig, mangled, overloaded, oerr := resolveFuncSigForCall(sc, e.Method, sc.a.methodOverloads[typ.Entity][e.Method], e.Args, e.Line)
		if overloaded {
			if oerr != nil {
				return nil, oerr
			}
			sig = osig
			method = mangled
		} else {
			var ok bool
			sig, ok = sc.methods[typ.Entity][e.Method]
			if !ok {
				// Нет метода с таким именем - последняя попытка ПЕРЕД
				// ошибкой: поле "имя" типа "Вызываемый(...)" - постфиксный
				// вызов ".имя(...)" тогда читает поле и зовёт получившееся
				// значение (analyzeCallValueExpr, тот же путь, что и у
				// вызова обычной переменной-Вызываемого), а не пытается
				// найти метод "имя" - иначе "этот.колбэк(...)" (ленивая
				// DI-фабрика полем, docs/SPEC.md) требовал бы обхода вида
				// "переменная ф = этот.колбэк; ф(...)". Метод с тем же
				// именем, если он есть, побеждает - эта ветка достижима
				// только когда его точно нет.
				entSig := sc.entities[typ.Entity]
				if idx := entSig.fieldIndex(e.Method); idx != -1 && entSig.FieldTypes[idx].Kind == ir.KindCallable {
					if err := checkFieldVisibility(sc, typ.Entity, e.Method, entSig.FieldVisibility[idx], e.Line); err != nil {
						return nil, err
					}
					field := &ir.FieldAccess{Receiver: receiver, Field: e.Method, Type: entSig.FieldTypes[idx]}
					return analyzeCallValueExpr(sc, field, &ast.CallExpr{Args: e.Args, Line: e.Line})
				}
				return nil, errAtLine(e.Line, CodeNameResolution, "у сущности %q нет метода %q", typ.Entity, e.Method)
			}
		}
		if err := checkMethodVisibility(sc, typ.Entity, e.Method, sig.Visibility, e.Line); err != nil {
			return nil, err
		}
		if err := registerCollectionResultType(sc, typ, sig); err != nil {
			return nil, diagnostics.WrapAt(err, diagnostics.Position{File: currentFile, Line: e.Line}, "метод %q", e.Method)
		}
	case ir.KindAny:
		var ok bool
		sig, ok = anyMethodSigs[e.Method]
		if !ok {
			return nil, errAtLine(e.Line, CodeNameResolution, "у \"Значение\" нет метода %q", e.Method)
		}
		if e.Method == "получить_поле_по_тегу" {
			if err := registerOptionalAnyOnCall(sc.entities, sc.methods, sc.aliases, sc.interfaces, sc.enums); err != nil {
				return nil, diagnostics.WrapAt(err, diagnostics.Position{File: currentFile, Line: e.Line}, "метод \"получить_поле_по_тегу\"")
			}
		}
		sc.a.markUsesReflection()
	case ir.KindTypeInfo:
		var ok bool
		sig, ok = typeInfoMethodSigs[e.Method]
		if !ok {
			return nil, errAtLine(e.Line, CodeNameResolution, "у \"Тип\" нет метода %q", e.Method)
		}
		sc.a.markUsesReflection()
	default:
		return nil, errAtLine(e.Line, CodeExprCheck, "нельзя вызвать метод %q - тип %s не сущность и не интерфейс", e.Method, typ)
	}
	args, err := checkCallArgs(sc, e.Args, sig, e.Line, fmt.Sprintf("метод %q", e.Method), fmt.Sprintf("вызова метода %q", e.Method))
	if err != nil {
		return nil, err
	}
	// Постусловие метода (sig.GuaranteedThisStates, см. analyzeMethod)
	// переносится на переменную-получателя - safe только потому, что "этот"
	// передаётся по ссылке (в отличие от обычных параметров, которые
	// копируются, см. package doc funcSig.GuaranteedThisStates).
	// sig.RevokesStates (тег "противоположность") - наоборот, снимается с
	// получателя ДО добавления собственного постусловия -
	// собственный "становится" метода (если есть) всегда сильнее, чем
	// автоматическая отмена состояния другого метода.
	if len(sig.GuaranteedThisStates) > 0 || len(sig.RevokesStates) > 0 {
		switch r := receiver.(type) {
		case *ir.Identifier:
			next := cloneStates(sc.getEstablished(r.Name))
			for st := range sig.RevokesStates {
				delete(next, st)
			}
			for st := range sig.GuaranteedThisStates {
				next[st] = true
			}
			sc.setEstablished(r.Name, next)
		case *ir.ThisExpr:
			next := cloneStates(sc.thisEstablished)
			for st := range sig.RevokesStates {
				delete(next, st)
			}
			for st := range sig.GuaranteedThisStates {
				next[st] = true
			}
			sc.thisEstablished = next
		}
	}
	// RevokesStates переносится и в сам IR (не только в established) -
	// рантайм-хранилище ("state_flags") должно СНЯТЬ бит при реальном
	// вызове метода-противоположности, даже если в его собственном теле нет
	// ни одного "становится" (см. package doc ir.MethodCallExpr.
	// RevokesStates) - иначе рантайм-проверка (StateCheckExpr/HasStateExpr)
	// после такого вызова читала бы устаревший, всё ещё установленный бит.
	var revokes []string
	for st := range sig.RevokesStates {
		revokes = append(revokes, st)
	}
	return &ir.MethodCallExpr{Receiver: receiver, Entity: typ.Entity, Method: method, Args: args, Type: sig.ReturnType, HasReturn: sig.HasReturn, RevokesStates: revokes}, nil
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
	} else if inferred, ok, err := inferredCallee(sc, e); err != nil {
		var ph placeholderCall
		if errors.As(err, &ph) {
			return ph.call, nil
		}
		return nil, err
	} else if ok {
		callee = inferred
	}
	// Перегруженное имя (funcOverloads[callee] != nil) резолвится ДО
	// обычной ветки sc.funcs[callee] ниже - под голым именем в sc.funcs нет
	// записи вовсе (см. package doc funcSig.MangledName). Перегруженное имя
	// НИКОГДА не встроенное (builtin никогда не заводится в funcOverloads) -
	// все спецслучаи "if callee == X { registerXOnCall }" ниже относятся
	// только к builtin-функциям и намеренно пропускаются для этой ветки.
	if osig, mangled, ok, err := resolveFuncSigForCall(sc, callee, sc.a.funcOverloads[callee], e.Args, e.Line); ok {
		if err != nil {
			return nil, err
		}
		args, err := checkCallArgs(sc, e.Args, osig, e.Line, fmt.Sprintf("функция %q", callee), fmt.Sprintf("вызова %q", callee))
		if err != nil {
			return nil, err
		}
		return &ir.CallExpr{Callee: mangled, Args: args, Type: osig.ReturnType, HasReturn: osig.HasReturn}, nil
	}
	if sig, ok := sc.funcs[callee]; ok {
		if callee == "параметры_пути" {
			if err := registerQueryParamsDictTypeOnCall(sc.entities, sc.methods, sc.aliases, sc.interfaces, sc.enums); err != nil {
				return nil, diagnostics.WrapAt(err, diagnostics.Position{File: currentFile, Line: e.Line}, "встроенная функция \"параметры_пути\"")
			}
		}
		if callee == "разбить" {
			if err := registerSplitTypeOnCall(sc.entities, sc.methods, sc.aliases, sc.interfaces, sc.enums); err != nil {
				return nil, diagnostics.WrapAt(err, diagnostics.Position{File: currentFile, Line: e.Line}, "встроенная функция \"разбить\"")
			}
		}
		if callee == "значения_заголовка" {
			if err := registerHeaderValuesTypeOnCall(sc.entities, sc.methods, sc.aliases, sc.interfaces, sc.enums); err != nil {
				return nil, diagnostics.WrapAt(err, diagnostics.Position{File: currentFile, Line: e.Line}, "встроенная функция \"значения_заголовка\"")
			}
		}
		if callee == "регулярное" {
			if err := registerRegexTypesOnCall(sc.entities, sc.methods, sc.aliases, sc.interfaces, sc.enums); err != nil {
				return nil, diagnostics.WrapAt(err, diagnostics.Position{File: currentFile, Line: e.Line}, "встроенная функция \"регулярное\"")
			}
		}
		if callee == "запустить_процесс" {
			if err := registerSubprocessTypesOnCall(sc.entities, sc.methods, sc.aliases, sc.interfaces, sc.enums); err != nil {
				return nil, diagnostics.WrapAt(err, diagnostics.Position{File: currentFile, Line: e.Line}, "встроенная функция \"запустить_процесс\"")
			}
		}
		if bytesIOFuncNames[callee] {
			if err := registerByteListTypeOnCall(sc.entities, sc.methods, sc.aliases, sc.interfaces, sc.enums); err != nil {
				return nil, diagnostics.WrapAt(err, diagnostics.Position{File: currentFile, Line: e.Line}, "встроенная функция %q", callee)
			}
		}
		if procMemFuncNames[callee] {
			if err := registerByteListTypeOnCall(sc.entities, sc.methods, sc.aliases, sc.interfaces, sc.enums); err != nil {
				return nil, diagnostics.WrapAt(err, diagnostics.Position{File: currentFile, Line: e.Line}, "встроенная функция %q", callee)
			}
		}
		if callee == "найти_байты" {
			if err := registerIntListTypeOnCall(sc.entities, sc.methods, sc.aliases, sc.interfaces, sc.enums); err != nil {
				return nil, diagnostics.WrapAt(err, diagnostics.Position{File: currentFile, Line: e.Line}, "встроенная функция %q", callee)
			}
		}
		if imageCodecFuncNames[callee] {
			if err := registerImageCodecEntityOnCall(sc.entities, sc.methods, sc.aliases, sc.interfaces, sc.enums); err != nil {
				return nil, diagnostics.WrapAt(err, diagnostics.Position{File: currentFile, Line: e.Line}, "встроенная функция %q", callee)
			}
		}
		if callee == "принять_http_байты" || callee == "отправить_http_ответ_байты" {
			if err := registerHTTPBytesTypesOnCall(sc.entities, sc.methods, sc.aliases, sc.interfaces, sc.enums); err != nil {
				return nil, diagnostics.WrapAt(err, diagnostics.Position{File: currentFile, Line: e.Line}, "встроенная функция %q", callee)
			}
		}
		if callee == "разобрать_multipart" {
			if err := registerMultipartTypesOnCall(sc.entities, sc.methods, sc.aliases, sc.interfaces, sc.enums); err != nil {
				return nil, diagnostics.WrapAt(err, diagnostics.Position{File: currentFile, Line: e.Line}, "встроенная функция %q", callee)
			}
		}
		if dbFuncNames[callee] {
			if err := registerDBTypesOnCall(sc); err != nil {
				return nil, diagnostics.WrapAt(err, diagnostics.Position{File: currentFile, Line: e.Line}, "встроенная функция %q", callee)
			}
			if callee == "подключиться_к_бд" && len(e.Args) == 1 {
				lit, _ := e.Args[0].(*ast.StringLiteral)
				sc.a.markDBConnString(lit)
			}
			if callee == beginTransactionFuncName {
				registerTransactionEntity(sc.entities, sc.methods)
				txCallableType := ir.CallableType([]ir.Type{ir.EntityType(transactionEntityName)}, nil)
				if err := sc.a.registerCallable(txCallableType); err != nil {
					return nil, diagnostics.WrapAt(err, diagnostics.Position{File: currentFile, Line: e.Line}, "встроенная функция %q", callee)
				}
			}
		}
		// "новая_http2_сессия"/"новая_grpcs_сессия" - тем же случаем, что и
		// "запрос" выше: обе дозваниваются через сокет ВНУТРИ реализации, но
		// в их СОБСТВЕННОЙ сигнатуре типа "Сокет" нет вовсе (HTTP2Сессия -
		// другой тип) - funcSigTouchesSocket это не ловит. ЖИВОЙ НАЙДЕННЫЙ
		// БАГ: "?Сокет" нужен ЛЮБОЙ программе с usesSockets (socketHelpers/
		// socketHelpersTLS эмитируются ОДНИМ блоком, rech_accept внутри
		// него безусловно возвращает "?Сокет") - до сих пор маскировался
		// тем, что каждая проверенная программа заодно звала и "http2_
		// принять_сессию" (та ловится funcSigTouchesSocket, Сокет-параметр
		// есть) - здесь найдено на чисто клиентской программе, ни разу не
		// принимающей соединения.
		if funcSigTouchesSocket(sig) || callee == "запрос" || callee == "новая_http2_сессия" || callee == "новая_grpcs_сессия" {
			if err := registerSocketNullableOnCall(sc.entities, sc.methods, sc.aliases, sc.interfaces, sc.enums); err != nil {
				return nil, diagnostics.WrapAt(err, diagnostics.Position{File: currentFile, Line: e.Line}, "встроенная функция %q", callee)
			}
		}
		if http2FuncNames[callee] {
			if err := registerHTTP2TypesOnCall(sc.entities, sc.methods, sc.aliases, sc.interfaces, sc.enums); err != nil {
				return nil, diagnostics.WrapAt(err, diagnostics.Position{File: currentFile, Line: e.Line}, "встроенная функция %q", callee)
			}
		}
		if http2BytesFuncNames[callee] {
			if err := registerByteListTypeOnCall(sc.entities, sc.methods, sc.aliases, sc.interfaces, sc.enums); err != nil {
				return nil, diagnostics.WrapAt(err, diagnostics.Position{File: currentFile, Line: e.Line}, "встроенная функция %q", callee)
			}
		}
		if grpcFuncNames[callee] {
			if err := registerGRPCTypesOnCall(sc.entities, sc.methods, sc.aliases, sc.interfaces, sc.enums); err != nil {
				return nil, diagnostics.WrapAt(err, diagnostics.Position{File: currentFile, Line: e.Line}, "встроенная функция %q", callee)
			}
		}
		if grpcBytesFuncNames[callee] {
			if err := registerByteListTypeOnCall(sc.entities, sc.methods, sc.aliases, sc.interfaces, sc.enums); err != nil {
				return nil, diagnostics.WrapAt(err, diagnostics.Position{File: currentFile, Line: e.Line}, "встроенная функция %q", callee)
			}
		}
		if optionalStringFuncNames[callee] {
			if err := registerOptionalStringOnCall(sc.entities, sc.methods, sc.aliases, sc.interfaces, sc.enums); err != nil {
				return nil, diagnostics.WrapAt(err, diagnostics.Position{File: currentFile, Line: e.Line}, "встроенная функция %q", callee)
			}
		}
		if optionalDateTimeFuncNames[callee] {
			if err := registerOptionalDateTimeOnCall(sc.entities, sc.methods, sc.aliases, sc.interfaces, sc.enums); err != nil {
				return nil, diagnostics.WrapAt(err, diagnostics.Position{File: currentFile, Line: e.Line}, "встроенная функция %q", callee)
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
		} else if ok && typ.Kind == ir.KindNullable && nullableInner(sc, typ).Kind == ir.KindCallable {
			// "?Вызываемый" - вызывается только доказанно непустое значение
			// ("если ф != Ничто { ф(...) }"), тем же сужением, что и везде.
			value := maybeUnwrapNarrowed(sc, &ir.Identifier{Name: e.Callee, Type: typ})
			if ir.TypeOf(value).Kind != ir.KindCallable {
				return nil, errAtLine(e.Line, CodeNullable, "%q имеет тип %s и может быть Ничто - вызывать можно только после проверки (\"если %s != Ничто { ... }\")", e.Callee, typ, e.Callee)
			}
			return analyzeCallValueExpr(sc, value, e)
		}
	}
	return nil, errAtLine(e.Line, CodeNameResolution, "функция %q не объявлена", callee)
}

// analyzeCallValueExpr - общая часть analyzeCallExpr (Callee - простое имя
// переменной) для вызова значения типа "Вызываемый" - value уже
// проанализирован (Identifier выше, либо в будущем произвольное выражение,
// если естественный вызов расширится на .поле и т. п.). Сигнатура value
// (Params/Result) берётся из Program.Callables по каноническому имени
// (value.Type.Entity) - a.resolveCallableSig, та же функция, что и
// registerCallable, дважды не резолвит по-разному. Аргументы проходят
// coerceExpr/invalidateEstablishedForOpaqueArg - те же, что и checkCallArgs
// (обычный вызов по имени) - иначе доказанно непустой nullable-аргумент
// (см. maybeUnwrapNarrowed) не разворачивался бы при вызове ЗНАЧЕНИЯ типа
// "Вызываемый" (например, параметра "следующий" у middleware), хотя тот же
// аргумент у вызова обычной функции уже разворачивается.
func analyzeCallValueExpr(sc *ctx, value ir.Expr, e *ast.CallExpr) (*ir.CallValueExpr, error) {
	typ := ir.TypeOf(value)
	sig, err := sc.a.resolveCallableSig(typ.Entity)
	if err != nil {
		return nil, diagnostics.WithPos(err, diagnostics.Position{File: currentFile, Line: e.Line})
	}
	if len(e.Args) != len(sig.Params) {
		return nil, errAtLine(e.Line, CodeExprCheck, "%s ожидает аргументов: %d, передано: %d", typ, len(sig.Params), len(e.Args))
	}
	args := make([]ir.Expr, len(e.Args))
	for i, a := range e.Args {
		arg, err := coerceExpr(sc, a, sig.Params[i])
		if err != nil {
			return nil, err
		}
		if t := ir.TypeOf(arg); !isAssignable(sc.entities, sc.interfaces, t, sig.Params[i]) {
			return nil, errAtLine(e.Line, CodeExprCheck, "аргумент %d вызова %s должен быть типа %s, получен %s", i+1, typ, sig.Params[i], t)
		}
		invalidateEstablishedForOpaqueArg(sc, arg)
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
	case *ir.MethodCallExpr:
		return e.HasReturn
	default:
		return false
	}
}

// checkCallArgs анализирует и проверяет аргументы вызова (обычной функции
// или метода) против сигнатуры sig - количество, тип каждого по позиции, и
// (для параметра, объявленного производным типом состояния) переход в
// требуемое состояние (flow-sensitive typestate). Общая для
// analyzeCallExpr/analyzeMethodCallExpr, которые отличаются только тем, как
// называть вызываемое в тексте ошибки: callee (для "%s ожидает
// аргументов...") и argLabel (для "аргумент N <argLabel> должен быть...").
//
// Переход, доказанный established на месте вызова, компилируется как
// обычно, без каких-либо накладных расходов. Недоказанный - НЕ ошибка
// компиляции: аргумент оборачивается в ir.StateCheckExpr, откладывающий
// саму проверку до рантайма (rech_require_state в cbackend, эквивалент в
// interp) - при реальном несовпадении бросает перехватываемое
// "Исключение", а не роняет сборку программы, которая на самом деле могла
// бы быть верной (established - консервативное над-приближение: не
// доказано - не значит "точно неверно", просто компилятор не проследил всю
// историю значения, например, через непрозрачный вызов чужой функции).
//
// Симметрично: established для каждого аргумента-сущности/интерфейса,
// переданного по ссылке ПРОСТЫМ именем (не получателем метода - там
// established переносится/снимается ТОЧНО через GuaranteedThisStates/
// RevokesStates, analyzeMethodCallExpr), инвалидируется ЦЕЛИКОМ сразу
// после вызова (см. invalidateEstablishedForOpaqueArg) - опаque-функция
// могла установить ЛЮБОЕ состояние объекта своим собственным
// "становится"/вызовом метода-противоположности внутри своего тела,
// компилятор не видит её тела здесь. Без этого established остался бы
// (ошибочно) доказанным даже после вызова, который его на самом деле снял -
// established может только РАСТИ через известные операции (становится,
// пересечение если/иначе), но не может СОКРАЩАТЬСЯ от факта "передан кому-
// то ещё", если явно не проверить это здесь.
func checkCallArgs(sc *ctx, args []ast.Expr, sig funcSig, line int, callee, argLabel string) ([]ir.Expr, error) {
	if err := checkSpreadPlacement(args, sig, line, callee); err != nil {
		return nil, err
	}
	var variadicTail []ast.Expr
	if sig.Variadic {
		fixed := len(sig.Params) - 1
		if len(args) < fixed {
			return nil, errAtLine(line, CodeExprCheck, "%s ожидает аргументов: минимум %d, передано: %d", callee, fixed, len(args))
		}
		args, variadicTail = args[:fixed], args[fixed:]
	}
	if len(args) > len(sig.Params) {
		return nil, errAtLine(line, CodeExprCheck, "%s ожидает аргументов: %d, передано: %d", callee, len(sig.Params), len(args))
	}
	if len(args) < len(sig.Params) && !sig.Variadic {
		// minRequired - позиция первого параметра со значением по
		// умолчанию (ParamDefaults только хвостом, см. её package doc) -
		// всё, что до него, обязательно должно быть передано.
		minRequired := len(sig.Params)
		for i := len(sig.Params) - 1; i >= 0 && i < len(sig.ParamDefaults) && sig.ParamDefaults[i] != nil; i-- {
			minRequired = i
		}
		if len(args) < minRequired {
			return nil, errAtLine(line, CodeExprCheck, "%s ожидает аргументов: %d (минимум %d, остальные - по умолчанию), передано: %d", callee, len(sig.Params), minRequired, len(args))
		}
	}
	out := make([]ir.Expr, len(sig.Params))
	for i := range sig.Params {
		var arg ir.Expr
		var err error
		if sig.Variadic && i == len(sig.Params)-1 {
			arg, err = analyzeVariadicTail(sc, variadicTail, sig.Params[i], i, line, argLabel)
		} else if i < len(args) {
			arg, err = coerceExpr(sc, args[i], sig.Params[i])
		} else {
			// Аргумент не передан - значение по умолчанию (уже проверено
			// выше, что оно есть): resolveParamDefault сама делает
			// coerceExpr/isAssignable, дальнейшая типовая проверка ниже
			// для этого случая избыточна, но безобидна - не заводит
			// отдельную ветку ради экономии одной проверки.
			arg, err = resolveParamDefault(sc, sig.ParamDefaults, i, sig.Params[i], line)
		}
		if err != nil {
			return nil, err
		}
		if t := ir.TypeOf(arg); !isAssignable(sc.entities, sc.interfaces, t, sig.Params[i]) {
			return nil, errAtLine(line, CodeExprCheck, "аргумент %d %s должен быть типа %s, получен %s", i+1, argLabel, sig.Params[i], t)
		}
		if i < len(sig.ParamStates) && sig.ParamStates[i] != "" {
			required := sig.ParamStates[i]
			if !establishedStatesOf(sc, arg)[required] {
				arg = &ir.StateCheckExpr{Value: arg, Entity: sig.Params[i].Entity, State: required, Line: line}
			}
		}
		invalidateEstablishedForOpaqueArg(sc, arg)
		out[i] = arg
	}
	return out, nil
}

// invalidateEstablishedForOpaqueArg - см. package doc checkCallArgs. arg -
// уже проанализированный аргумент (возможно, уже обёрнутый в
// ir.StateCheckExpr самим checkCallArgs выше) - разворачивает такую
// обёртку, чтобы найти исходное отслеживаемое выражение под ней.
func invalidateEstablishedForOpaqueArg(sc *ctx, arg ir.Expr) {
	value := arg
	if check, ok := arg.(*ir.StateCheckExpr); ok {
		value = check.Value
	}
	t := ir.TypeOf(value)
	if t.Kind != ir.KindEntity && t.Kind != ir.KindInterface {
		return
	}
	switch v := value.(type) {
	case *ir.Identifier:
		sc.setEstablished(v.Name, nil)
	case *ir.ThisExpr:
		sc.thisEstablished = nil
	}
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
			return nil, errAtLine(e.Line, CodeExprCheck, "унарный \"-\" применим только к числам, получен %s", typ)
		}
		return &ir.UnaryExpr{Op: e.Op, Operand: operand, Type: typ}, nil
	case "не":
		if typ != ir.TypeBool {
			return nil, errAtLine(e.Line, CodeExprCheck, "\"не\" применимо только к логическому значению, получен %s", typ)
		}
		return &ir.UnaryExpr{Op: e.Op, Operand: operand, Type: ir.TypeBool}, nil
	case "~":
		if typ == ir.TypeByte {
			return &ir.UnaryExpr{Op: e.Op, Operand: operand, Type: ir.TypeByte}, nil
		}
		if _, _, ok := typ.Kind.IntWidthSigned(); ok {
			return &ir.UnaryExpr{Op: e.Op, Operand: operand, Type: typ}, nil
		}
		return nil, errAtLine(e.Line, CodeExprCheck, "побитовый \"~\" применим только к целочисленным типам (Целое/Целое8..64/Натуральное.../Байт), получен %s", typ)
	default:
		return nil, errAtLine(e.Line, CodeExprCheck, "неизвестная унарная операция %q", e.Op)
	}
}

// analyzeIncDecExpr - "имя++"/"имя--"/"++имя"/"--имя" - применимо только к
// уже объявленной переменной типа Целое/Число, как и Assign (не к полю
// сущности, не к результату вызова). Явно НЕ поддержано (осознанный срез,
// не запрошено) - имя, захваченное лямбдой по ссылке (package doc
// ir.CaptureVar): ir.IncDecExpr читает/пишет по ИМЕНИ напрямую (varRef/
// env.get-set), не через Deref(Assign) - расширить его до работы через
// указатель значило бы завести отдельный C-путь ради редкого сочетания
// двух фич сразу, вместо ясной ошибки с очевидным обходом ("y = y + 1").
func analyzeIncDecExpr(sc *ctx, e *ast.IncDecExpr) (ir.Expr, error) {
	typ, ok := sc.lookup(e.Name)
	if !ok {
		d := errAtLine(e.Line, CodeNameResolution, "переменная %q не объявлена", e.Name)
		if hint := suggestNameHint(e.Name, sc.visibleNames()); hint != "" {
			d.WithHint("%s", hint)
		}
		return nil, d
	}
	if sc.readBeforeAssign(e.Name) {
		return nil, errReadBeforeAssign(e.Name, e.Line)
	}
	if _, isByRef := sc.byRefPointerType(e.Name); isByRef {
		baseOp := "+"
		if e.Op == "--" {
			baseOp = "-"
		}
		return nil, errAtLine(e.Line, CodeExprCheck, "%q захвачено по ссылке (\"&%s\" в списке захвата лямбды) - %q для таких переменных не поддержан, используйте \"%s = %s %s 1\"", e.Name, e.Name, e.Op, e.Name, e.Name, baseOp)
	}
	if sc.isConst(e.Name) {
		return nil, errAtLine(e.Line, CodeExprCheck, "%q объявлено как \"константа\" - нельзя изменить (%s)", e.Name, e.Op)
	}
	_, _, isFixedWidth := typ.Kind.IntWidthSigned()
	_, isRanked := numericRanks[typ]
	if !isRanked && !(isFixedWidth && typ.Kind != ir.KindInt) {
		return nil, errAtLine(e.Line, CodeExprCheck, "оператор %q применим только к Целое/Дробное/Действительное, получено %s", e.Op, typ)
	}
	return &ir.IncDecExpr{Name: e.Name, Op: e.Op, Prefix: e.Prefix, Type: typ}, nil
}

func analyzeBinaryExpr(sc *ctx, e *ast.BinaryExpr) (ir.Expr, error) {
	// "== Ничто"/"!= Ничто" - особый случай, перехватывается ДО обычного
	// анализа обеих сторон: "Ничто" само по себе не разбирается analyzeExpr
	// (нет контекста типа), см. analyzeNullCheck.
	if e.Bang {
		return analyzeBang(sc, e.Left, e.Line)
	}
	if e.Op == "==" || e.Op == "!=" {
		if _, ok := e.Left.(*ast.NullLiteral); ok {
			return analyzeNullCheck(sc, e.Right, e.Op == "!=", e.Line)
		}
		if _, ok := e.Right.(*ast.NullLiteral); ok {
			return analyzeNullCheck(sc, e.Left, e.Op == "!=", e.Line)
		}
	}
	left, err := analyzeExpr(sc, e.Left)
	if err != nil {
		return nil, err
	}
	right, err := analyzeExpr(sc, e.Right)
	if err != nil {
		return nil, err
	}

	// "??" - левая сторона обязана ОСТАТЬСЯ nullable (см.
	// analyzeNullCoalesce) - до maybeUnwrapNarrowed ниже, который иначе
	// развернул бы уже доказанно непустую левую часть.
	if e.Op == "??" {
		return analyzeNullCoalesce(e, sc, left, right)
	}

	// Доказанная непустота (established, "если а != Ничто {...}") позволяет
	// употребить nullable-значение как обычный Тип без явного каста - см.
	// package doc maybeUnwrapNarrowed. Не влияет на "==,!=,??" выше (уже
	// обработаны раньше).
	left = maybeUnwrapNarrowed(sc, left)
	right = maybeUnwrapNarrowed(sc, right)
	if left, right, err = coerceNumericConstPair(left, right, e.Line); err != nil {
		return nil, err
	}
	lt, rt := ir.TypeOf(left), ir.TypeOf(right)

	// Перегрузка оператора (internal/semantic/operator.go) - ДО встроенной
	// резолюции ниже: сущность/структура с объявленным "статический
	// оператор <op>" перехватывает "+"/"-"/"=="/... целиком, даже там, где
	// у типа уже есть встроенное поведение (например "==" для структур,
	// see ниже) - явно написанный оператор пользователя главнее умолчания.
	if expr, found, err := tryOperatorOverload(sc, e.Op, left, right, lt, e.Line); found {
		return expr, err
	}

	switch e.Op {
	case "+":
		if lt == ir.TypeString && rt == ir.TypeString {
			return &ir.BinaryExpr{Op: "+", Left: left, Right: right, Type: ir.TypeString}, nil
		}
		if (lt == ir.TypeString) != (rt == ir.TypeString) {
			// Ровно один операнд - Строка, другой - нет: похоже на попытку
			// конкатенации без явного приведения ("+" не приводит операнды
			// автоматически, docs/SPEC.md) - частая грабля новичка, у неё
			// есть единственное, безошибочное исправление, поэтому - подсказка,
			// а не просто generic "оператор не поддерживается" из
			// analyzeArithmetic ниже.
			other := rt
			if lt != ir.TypeString {
				other = lt
			}
			return nil, errAtLine(e.Line, CodeExprCheck, "\"+\" не приводит операнды автоматически - Строка и %s нельзя сложить напрямую", other).
				WithHint("оберните операнд в явное приведение: ... + (Строка) ...")
		}
		return analyzeArithmetic(e, left, lt, right, rt)

	case "-", "*", "/", "%":
		return analyzeArithmetic(e, left, lt, right, rt)

	case "==", "!=":
		if lt == ir.TypeString && rt == ir.TypeString {
			return &ir.BinaryExpr{Op: e.Op, Left: left, Right: right, Type: ir.TypeBool}, nil
		}
		// Сущность/класс (не "структура" - см. entityIsRefType ниже) -
		// сравнение по ССЫЛКЕ (НАЙДЕНО по ходу examples/проекты/чат_сервер:
		// раньше "==" не поддерживался вообще ни для одной сущности) - тот
		// же принцип, что и у Go/Java/C# (ссылочные типы сравниваются по
		// identity через "==" безусловно, содержимое - отдельным методом,
		// которого здесь пока нет).
		if lt.Kind == ir.KindEntity && lt == rt && entityIsRefType(sc.entities, lt.Entity) {
			return &ir.BinaryExpr{Op: e.Op, Left: left, Right: right, Type: ir.TypeBool}, nil
		}
		// "структура" (значение, не identity) - поле-в-поле (docs/
		// "== для структур") - тот же ir.BinaryExpr, что и у
		// ссылочной сущности выше: cbackend/interp сами решают, как именно
		// сравнивать конкретный KindEntity (указатель vs рекурсивно по
		// полям) по IsStruct, см. structFieldsEquatable ниже.
		if lt.Kind == ir.KindEntity && lt == rt && !entityIsRefType(sc.entities, lt.Entity) {
			if err := structFieldsEquatable(sc.entities, lt.Entity); err != nil {
				return nil, diagnostics.WrapAt(err, diagnostics.Position{File: currentFile, Line: e.Line}, "оператор %q не поддерживается для структуры %s", e.Op, lt)
			}
			return &ir.BinaryExpr{Op: e.Op, Left: left, Right: right, Type: ir.TypeBool}, nil
		}
		// "Тип" - один и тот же тип (синонимы - тоже: "тип Натуральное8 ==
		// тип Байт"), сравнение по имени дескриптора в обоих backend'ах.
		if lt == ir.TypeTypeInfo && rt == ir.TypeTypeInfo {
			return &ir.BinaryExpr{Op: e.Op, Left: left, Right: right, Type: ir.TypeBool}, nil
		}
		if lt != rt || !equatableTypes[lt] {
			return nil, errAtLine(e.Line, CodeExprCheck, "оператор %q не поддерживается для типов %s и %s", e.Op, lt, rt)
		}
		return &ir.BinaryExpr{Op: e.Op, Left: left, Right: right, Type: ir.TypeBool}, nil

	case "<", ">", "<=", ">=":
		if lt != rt || !orderableTypes[lt] {
			return nil, errAtLine(e.Line, CodeExprCheck, "оператор %q применим только к числам одного типа, получены %s и %s", e.Op, lt, rt)
		}
		return &ir.BinaryExpr{Op: e.Op, Left: left, Right: right, Type: ir.TypeBool}, nil

	case "и", "или":
		if lt != ir.TypeBool || rt != ir.TypeBool {
			return nil, errAtLine(e.Line, CodeExprCheck, "оператор %q применим только к логическим значениям, получены %s и %s", e.Op, lt, rt)
		}
		return &ir.BinaryExpr{Op: e.Op, Left: left, Right: right, Type: ir.TypeBool}, nil

	case "&", "|", "^", "<<", ">>":
		// Побитовые - только между операндами ОДНОГО И ТОГО ЖЕ
		// целочисленного типа, без неявной промоции (тот же принцип, что и
		// у арифметики фикс-ширинных типов, analyzeArithmetic) - Целое/
		// Целое8..64/Натуральное/Натуральное8..64 (ir.TypeKind.
		// IntWidthSigned) и отдельно Байт (своя семантика "код/индекс, не
		// число", не входит в IntWidthSigned) - результат везде того же
		// типа, что и операнды, не растёт до Целое (найдено на WebSocket-
		// маскировании кадра, Список<Байт> - незачем растить байт до Целое
		// и обратно только ради маскирования/чтения битов).
		if lt != rt {
			return nil, errAtLine(e.Line, CodeExprCheck, "побитовый оператор %q применим только между операндами одного целочисленного типа, получены %s и %s", e.Op, lt, rt)
		}
		if lt == ir.TypeByte {
			return &ir.BinaryExpr{Op: e.Op, Left: left, Right: right, Type: ir.TypeByte}, nil
		}
		if _, _, ok := lt.Kind.IntWidthSigned(); ok {
			return &ir.BinaryExpr{Op: e.Op, Left: left, Right: right, Type: lt}, nil
		}
		return nil, errAtLine(e.Line, CodeExprCheck, "побитовый оператор %q применим только к целочисленным типам (Целое/Целое8..64/Натуральное.../Байт), получен %s", e.Op, lt)

	default:
		return nil, errAtLine(e.Line, CodeExprCheck, "неизвестная операция %q", e.Op)
	}
}

// isStrictWidthInt - целый тип фиксированной ширины со строгой (без
// промоции) арифметикой: все, кроме Целое.
func isStrictWidthInt(t ir.Type) bool {
	_, _, ok := t.Kind.IntWidthSigned()
	return ok && t.Kind != ir.KindInt
}

// analyzeArithmetic - "+" (нестроковый)/"-"/"*"/"/": результат - более
// широкий из двух операндов по numericRanks (Целое -> Дробное ->
// Действительное), тип в IR-узле, без самого приведения ((double)...) -
// приведение операнда, если нужно, решает backend при генерации кода,
// глядя на Type каждого операнда и Type результата.
func analyzeArithmetic(e *ast.BinaryExpr, left ir.Expr, lt ir.Type, right ir.Expr, rt ir.Type) (ir.Expr, error) {
	// Целое8/16/32, Натуральное/8/16/32 - строго между операндами ОДНОГО И
	// ТОГО ЖЕ Kind, без неявной промоции (в отличие от Целое/Дробное/
	// Действительное ниже, numericRanks).
	if lt == rt && isStrictWidthInt(lt) {
		return &ir.BinaryExpr{Op: e.Op, Left: left, Right: right, Type: lt}, nil
	}
	if isStrictWidthInt(lt) || isStrictWidthInt(rt) {
		return nil, errAtLine(e.Line, CodeExprCheck, "оператор %q: %s и %s - разные типы, без неявного приведения; приведите один операнд к типу другого, например (%s) значение", e.Op, lt, rt, lt)
	}
	lr, lok := numericRanks[lt]
	if !lok {
		return nil, errAtLine(e.Line, CodeExprCheck, "оператор %q не поддерживается для типа %s", e.Op, lt)
	}
	rr, rok := numericRanks[rt]
	if !rok {
		return nil, errAtLine(e.Line, CodeExprCheck, "оператор %q не поддерживается для типа %s", e.Op, rt)
	}
	result := lt
	if rr > lr {
		result = rt
	}
	return &ir.BinaryExpr{Op: e.Op, Left: left, Right: right, Type: result}, nil
}
