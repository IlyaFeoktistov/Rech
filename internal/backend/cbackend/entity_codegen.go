// Кодогенерация C для сущностей (typedef struct, конструктор) и функций/
// методов верхнего уровня из internal/ir - выделено из ir_generator.go как
// отдельный домен (см. package doc там же про irGenCtx/помощники пакета).
package cbackend

import (
	"fmt"
	"strings"

	"github.com/IlyaFeoktistov/Rech/internal/backend/cbackend/codegen"
	"github.com/IlyaFeoktistov/Rech/internal/ir"
)

// generateIREntityStruct - C typedef struct для ent, как
// C typedef struct для ent, по уже собранным internal/ir.Field.
func generateIREntityStruct(ctx *irGenCtx, ent *ir.Entity) (string, error) {
	if ent.IsMutex {
		return mutexStructTypedef(), nil
	}
	if ent.IsSemaphore {
		return semaphoreStructTypedef(), nil
	}
	if ent.IsTransaction {
		return dbtxTypedefCode(), nil
	}
	s := codegen.Struct(entityCName(ctx, ent.Name))
	participates := ctx.dispatchParticipants[ent.Name]
	if participates {
		// "vt" - первым полем и всегда (не только у собственно
		// объявляющей сущности, но и у каждого её потомка) - общий
		// начальный кусок структуры делает приведение к vtableHeaderType
		// определённым для чтения (см. package doc vtable.go).
		s.Field("const "+vtableType+" *", "vt")
	}
	if ent.HasStateFlags {
		// "state_flags" - позиционно инвариантна той же логике, что и "vt"
		// выше: потомок сущности, объявляющей "состояние:", несёт это поле
		// даже без своего "состояние:" (см. package doc ir.Entity.
		// HasStateFlags) - указатель на потомка используется как указатель
		// на предка в рантайм-проверке (internal/backend/cbackend/
		// typestate.go, offsetof(<предок>, state_flags)).
		s.Field("uint64_t", "state_flags")
	}
	if len(ent.Fields) == 0 && !participates && !ent.HasStateFlags {
		s.Field("char", "_rech_empty")
	}
	for _, f := range ent.Fields {
		if ent.IsExternStruct && f.Type.Kind == ir.KindCallable {
			// "Вызываемый(...)" полем "внешняя структура" (WNDCLASS.
			// lpfnWndProc-стиль) - реальный C API читает/пишет ГОЛЫЙ
			// указатель на функцию в самой typedef-структуре, не {fn, env}
			// по значению (fromIRType(...).cType - callableType, см.
			// callable.go) - декларатор с именем ВНУТРИ скобок
			// (ffiFieldDeclarator, ffi_callback.go), FieldDeclarator (не
			// обычный Field - см. её package doc, codegen/struct.go).
			decl, err := ffiFieldDeclarator(ctx, f.Type, mangleField(f.Name))
			if err != nil {
				return "", err
			}
			s.FieldDeclarator(decl)
			continue
		}
		s.Field(fromIRType(f.Type).cType(ctx), mangleField(f.Name))
	}
	w := codegen.NewWriter()
	s.Write(w)
	return w.String() + "\n", nil
}

// generateIREntityCtor - конструктор ссылочной сущности ("сущность"/
// "класс", ctx.entityIsRef(ent.Name) == true): выделяет память под структуру
// через GC_MALLOC (Boehm GC - решение пользователя, шаг 12, "Указатели/
// ссылки... сущность по умолчанию") и возвращает указатель. GC_MALLOC (не
// GC_MALLOC_ATOMIC) - поля сущности сами могут быть указателями на другие
// GC-объекты (вложенная сущность), нужно сканирование; она же зануляет
// выделенную память (документированная гарантия Boehm GC, в отличие от
// обычного malloc) - опора для тела конструктора ниже, чьё тело не обязано
// присвоить каждое поле явно.
//
// ent.Ctor никогда не nil здесь (ctx.entityIsRef(ent.Name) == true) - у
// сущности без своего "конструктор(...)" в исходнике internal/semantic
// подставляет синтетический пустой (0 параметров, пустое тело, см. package
// doc analyzer.go) - "как в нормальных языках" неявный конструктор не
// зависит от полей вообще, а "новый Тип()" без аргументов - единственная
// форма, доступная без явного "конструктор(...)" (docs/SPEC.md, "Явные
// конструкторы"). Выделение памяти отделено от самого тела конструктора
// (generateIREntityCtorInit) - см. её package doc про то, зачем именно.
//
// В отличие от "структура" (generateIRNewExpr - compound literal прямо в
// месте вызова, структуры не участвуют в конструкторах вообще - см.
// ast.EntityDecl.Ctor), у ссылочной сущности ровно одна такая функция на
// тип, не код на каждый "новый" - тот же принцип, что и у show-функций.
func generateIREntityCtor(ctx *irGenCtx, ent *ir.Entity) (string, error) {
	if ent.IsMutex {
		return mutexRuntimeCode(), nil
	}
	if ent.IsSemaphore {
		return semaphoreRuntimeCode(), nil
	}
	if ent.IsTransaction {
		// "Транзакция" не имеет ни конструктора, ни обычных полей вообще
		// (см. internal/semantic/expr.go, analyzeNewExpr, IsTransaction -
		// "новый" запрещён) - её C-структура (dbtxTypedefCode,
		// generateIREntityStruct выше) устроена совсем иначе, чем обычная
		// сущность (нет "vt", свои поля mini/real_db/real_idx/resolved) -
		// обычный путь ниже (generateIREntityCtorInit и т. п.) писал бы код
		// для несуществующих полей. Сам рантайм (rech_db_run_transaction +
		// три метода) эмитируется отдельно, из GenerateFromIR, рядом с
		// dbRuntimeCode (там доступен prog.DBBackends, здесь - нет).
		return "", nil
	}
	if ent.Ctor == nil {
		// internal/semantic держит этот инвариант (каждая ссылочная
		// сущность получает Ctor - явный или синтетический пустой, см.
		// analyzeEntity) - запасной вариант здесь только на случай
		// системной сущности, заведённой напрямую литералом ir.Entity{...}
		// в обход analyzeEntity (как когда-то было с "Исключение").
		ent.Ctor = &ir.Method{}
	}
	initCode, err := generateIREntityCtorInit(ctx, ent)
	if err != nil {
		return "", err
	}
	return initCode + generateIREntityCtorNewWithInit(ctx, ent), nil
}

// gcAllocStmts - "Тип *p = GC_MALLOC(sizeof(Тип));" + (если ent участвует в
// диспетчеризации) "p->vt = &rech_VT_...;" - общее начало тела и у
// позиционного конструктора (generateIREntityCtor), и у обёртки над явным
// (generateIREntityCtorNewWithInit) - единственная разница между ними дальше
// (позиционное заполнение полей против вызова rech_init_...).
func gcAllocStmts(ctx *irGenCtx, ent *ir.Entity, cType string) []codegen.Stmt {
	stmts := []codegen.Stmt{
		codegen.Decl(cType+" *", "p", codegen.Call("GC_MALLOC", codegen.Sizeof(codegen.Raw(cType)))),
	}
	if ctx.dispatchParticipants[ent.Name] {
		stmts = append(stmts, codegen.Assign(codegen.PointerField(codegen.Var("p"), "vt"), codegen.Address(codegen.Raw(vtableInstanceName(ent.Name)))))
	}
	return stmts
}

// zeroFieldValueExpr - выражение для явного зануления поля БЕЗ "= выражение"
// (см. package doc generateIREntityCtorInit) - nil, если голых нулевых
// байт GC_MALLOC'а уже достаточно (Целое/Дробное/Действительное/
// Логическое/Символ/Байт - всё это просто числовой 0, а также ссылка на
// другую сущность/интерфейс, которой намеренно разрешено быть nil "ещё не
// присвоено" - тем же принципом, что и internal/interp/eval.go,
// zeroFieldValue). Строка - нулевые байты дают NULL char*, не то же самое,
// что "" (segfault при первом же обращении к полю - живая проверка нашла
// именно это); generic-коллекция (Список<T>/Стопка<T>/Очередь<T>/
// Словарь<К,В>, ctx.generics) - нулевые байты дают несогласованную C-
// структуру буфера/карты (например, len/cap ненулевые при указателе-NULL,
// в зависимости от представления) - первый же вызов метода тоже падает.
func zeroFieldValueExpr(ctx *irGenCtx, t ir.Type) ir.Expr {
	switch {
	case t.Kind == ir.KindString:
		return &ir.StringLiteral{Value: ""}
	case t.Kind == ir.KindEntity && ctx.generics[t.Entity]:
		return &ir.NewExpr{Entity: t.Entity}
	default:
		return nil
	}
}

// generateIREntityCtorInit - тело конструктора ent.Ctor (явного или
// синтетического пустого, см. package doc generateIREntityCtor) как
// отдельная C-функция, с тем же "Тип *this_" первым скрытым параметром, что
// и у
// метода (cMethodParamListIR/ctx.thisEntity - буквально тот же путь
// кодогенерации тела, что и generateIRMethod, включая "этот"). Отделена от
// выделения памяти (generateIREntityCtorNewWithInit) ровно для того, чтобы
// "родитель(...)" производного конструктора (ir.ParentCallStmt) мог вызвать
// её напрямую на уже существующем объекте, не создавая новый - единый
// объект строится последовательными вызовами тел конструкторов вверх по
// цепочке BaseName, а не вложенными "новый".
func generateIREntityCtorInit(ctx *irGenCtx, ent *ir.Entity) (string, error) {
	var sb strings.Builder
	fmt.Fprintf(&sb, "// %s: конструктор\n", ent.Name)
	fmt.Fprintf(&sb, "static void %s(%s) {\n", mangleCtorInit(ent.Name), cMethodParamListIR(ctx, ent.Name, ent.Ctor.Params))
	fmt.Fprintf(&sb, "    %s", tracePush(ent.Name+".конструктор"))
	ctx.thisEntity = ent.Name
	defer func() { ctx.thisEntity = "" }()
	// Поле с "имеет: имя: Тип = выражение" начинается со своего значения по
	// умолчанию ещё до тела явного конструктора (docs/SPEC.md) - GC_MALLOC
	// зануляет память, этого достаточно для полей без default ТОЛЬКО когда
	// нулевые байты - валидное нулевое значение типа (Целое/Дробное/
	// Действительное/Логическое/Символ/Байт, а также ссылка на другую
	// сущность/интерфейс - см. zeroFieldValueExpr) - Строка/generic-
	// коллекция нуждаются в явном занулении (zeroFieldValueExpr, nil для
	// всех остальных типов - для них ничего не генерируется, как и раньше).
	// Тело конструктора (ниже) может переопределить любое из полей через
	// "этот.поле = ...", включая эти явно занулённые.
	for _, f := range ent.Fields {
		value := f.Default
		if value == nil {
			value = zeroFieldValueExpr(ctx, f.Type)
		}
		if value == nil {
			continue
		}
		assign := &ir.FieldAssign{Receiver: &ir.ThisExpr{Entity: ent.Name}, Field: f.Name, Value: value}
		if err := generateIRStmt(&sb, ctx, assign, 1); err != nil {
			return "", err
		}
	}
	for _, s := range ent.Ctor.Body {
		if err := generateIRStmt(&sb, ctx, s, 1); err != nil {
			return "", err
		}
	}
	fmt.Fprintf(&sb, "    %s", tracePop)
	sb.WriteString("}\n\n")
	return sb.String(), nil
}

// generateIREntityCtorNewWithInit - "rech_new_X" когда есть явный
// конструктор: GC_MALLOC (зануляет память) + вызов тела конструктора
// (generateIREntityCtorInit) на свежевыделенном указателе + возврат его.
// Определена ПОСЛЕ generateIREntityCtorInit в общем тексте (см.
// generateIREntityCtor) - обращение к mangleCtorInit(ent.Name) здесь уже
// после её определения, C не нуждается в отдельном прототипе. Порядок
// сущностей в prog.Entities - порядок объявления в файле (buildIR сохраняет
// его как есть), а базовая сущность обязана быть объявлена раньше
// производной (semantic.applyInheritance) - поэтому и "родитель(...)"
// производного конструктора всегда ссылается на уже определённую
// rech_init_БАЗА к моменту, когда до неё доходит очередь в этом же цикле
// (GenerateFromIR), без отдельного прототипа и для этого случая тоже.
func generateIREntityCtorNewWithInit(ctx *irGenCtx, ent *ir.Entity) string {
	cType := entityCName(ctx, ent.Name)
	exported := ctx.exportedCtors[ent.Name]
	fn := codegen.Func(ctorCName(ctx, ent.Name)).ReturnType(exportAttr(ctx, exported) + cType + " *")
	if !exported {
		fn.Static()
	}
	params := make([]codegen.Param, len(ent.Ctor.Params))
	for i, p := range ent.Ctor.Params {
		params[i] = codegen.Parameter(fromIRType(p.Type).cType(ctx), mangleVar(p.Name))
	}
	fn.Params(params...)
	initArgs := make([]codegen.Expr, len(ent.Ctor.Params)+1)
	initArgs[0] = codegen.Var("p")
	for i, p := range ent.Ctor.Params {
		initArgs[i+1] = codegen.Var(mangleVar(p.Name))
	}
	stmts := gcAllocStmts(ctx, ent, cType)
	stmts = append(stmts,
		codegen.ExprStatement(codegen.Call(mangleCtorInit(ent.Name), initArgs...)),
		codegen.Return(codegen.Var("p")),
	)
	fn.Statements(stmts...)
	w := codegen.NewWriter()
	fn.Write(w)
	return w.String() + "\n"
}

// cReturnTypeFor - C-тип результата функции/метода: сама Type, если он
// объявлен (hasReturn), иначе "int" - технический 0 в конце (не значение
// "результата"), чтобы int-функция не осталась без return (для строгого
// C - undefined behavior), как и в AST-пути.
func cReturnTypeFor(ctx *irGenCtx, hasReturn bool, returnType ir.Type) string {
	if hasReturn {
		return fromIRType(returnType).cType(ctx)
	}
	return "int"
}

func cParamListIR(ctx *irGenCtx, params []ir.Param) string {
	if len(params) == 0 {
		return "void"
	}
	parts := make([]string, len(params))
	for i, p := range params {
		parts[i] = fmt.Sprintf("%s %s", fromIRType(p.Type).cType(ctx), mangleVar(p.Name))
	}
	return strings.Join(parts, ", ")
}

// externPrototypeOfIR - "extern <тип> <CName>(<типы>);" для внешней
// функции - без имён параметров (в отличие от cbackend.externPrototypeOf,
// C допускает анонимные параметры в прототипе, а сами имена
// параметров Rech после семантического анализа уже не нужны ни для чего,
// кроме сообщений об ошибках, которые уже показаны на этапе Analyze.
func externPrototypeOfIR(ctx *irGenCtx, ext *ir.ExternFuncDecl) (string, error) {
	if externPrototypeAlreadyProvided(ctx, ext.CName) {
		// Стандартный символ (strlen/malloc/sqrt/...), а заголовок,
		// объявляющий его, программа и так включает по другой причине -
		// см. package doc extern_headers.go - НЕ дублируем прототип,
		// полагаемся на уже видимый из заголовка.
		return "", nil
	}
	cReturnType := "void"
	if ext.HasReturn {
		// ext.ReturnType никогда KindCallable (semantic отклоняет - см.
		// collectExternFuncSigs) - ffiParamCType здесь ведёт себя как
		// обычный fromIRType(...).cType(ctx).
		cReturnType = fromIRType(ext.ReturnType).cType(ctx)
	}
	types := make([]string, len(ext.Params))
	for i, p := range ext.Params {
		t, err := ffiParamCType(ctx, p)
		if err != nil {
			return "", err
		}
		types[i] = t
	}
	return fmt.Sprintf("extern %s %s(%s);\n", cReturnType, ext.CName, strings.Join(types, ", ")), nil
}

// cMethodParamListIR - как cParamListIR, но с обязательным скрытым первым
// параметром "Тип *this_" (получатель по указателю, как и в AST-пути) -
// у метода он есть всегда, поэтому в отличие от cParamListIR/
// combineParamList не нужен отдельный случай "нет параметров -> void".
func cMethodParamListIR(ctx *irGenCtx, entityName string, params []ir.Param) string {
	parts := make([]string, len(params)+1)
	parts[0] = fmt.Sprintf("%s *this_", entityCName(ctx, entityName))
	for i, p := range params {
		parts[i+1] = fmt.Sprintf("%s %s", fromIRType(p.Type).cType(ctx), mangleVar(p.Name))
	}
	return strings.Join(parts, ", ")
}

func generateIRFunction(ctx *irGenCtx, fn *ir.Function) (string, error) {
	var sb strings.Builder
	fmt.Fprintf(&sb, "// %s\n", fn.Name)
	cName := funcCName(ctx, fn.Name)
	paramList := cParamListIR(ctx, fn.Params)
	if cName == "main" {
		// "главная" всегда получает настоящую C-сигнатуру main(int argc,
		// char **argv) - как и в AST-пути (generateCallable) - количество_
		// аргументов/аргумент читают их из глобальных rech_argc/rech_argv
		// (argsGlobals), вызываемых из любой функции программы, не только
		// из главная.
		paramList = "int argc, char **argv"
	}
	fmt.Fprintf(&sb, "%s%s %s(%s) {\n", exportAttr(ctx, fn.Exported), cReturnTypeFor(ctx, fn.HasReturn, fn.ReturnType), cName, paramList)
	fmt.Fprintf(&sb, "    %s", tracePush(fn.Name))
	if cName == "main" {
		if ctx.usesGC {
			sb.WriteString("    GC_INIT();\n")
		}
		sb.WriteString("    rech_init_console();\n")
		sb.WriteString("    rech_init_args(argc, argv);\n")
		if ctx.usesStaticInit {
			sb.WriteString("    rech_init_statics();\n")
		}
	}
	for _, s := range fn.Body {
		if err := generateIRStmt(&sb, ctx, s, 1); err != nil {
			return "", err
		}
	}
	if !fn.HasReturn {
		fmt.Fprintf(&sb, "    %s", tracePop)
		sb.WriteString("    return 0;\n")
	}
	sb.WriteString("}\n\n")
	return sb.String(), nil
}

// generateIRMethod - как generateIRFunction, но со скрытым параметром
// "Тип *this_" и ctx.thisEntity, включённым на время генерации тела - то,
// что делает ThisExpr типизируемым (generateIRExpr, case *ir.ThisExpr),
// как и ctx.thisEntity в AST-пути (generateMethod).
func generateIRMethod(ctx *irGenCtx, entityName string, m *ir.Method) (string, error) {
	var sb strings.Builder
	fmt.Fprintf(&sb, "// %s.%s\n", entityName, m.Name)
	paramList := cMethodParamListIR(ctx, entityName, m.Params)
	if m.IsStatic {
		// Статический метод - обычная C-функция, без скрытого "this_" (см.
		// package doc ir.Method.IsStatic) - "этот" внутри тела грамматически
		// невозможен (парсер), поэтому ctx.thisEntity не выставляется.
		paramList = cParamListIR(ctx, m.Params)
	}
	fmt.Fprintf(&sb, "%s%s %s(%s) {\n", exportAttr(ctx, m.Exported), cReturnTypeFor(ctx, m.HasReturn, m.ReturnType), methodCName(ctx, entityName, m.Name), paramList)
	fmt.Fprintf(&sb, "    %s", tracePush(entityName+"."+m.Name))
	if !m.IsStatic {
		ctx.thisEntity = entityName
		defer func() { ctx.thisEntity = "" }()
	}
	for _, s := range m.Body {
		if err := generateIRStmt(&sb, ctx, s, 1); err != nil {
			return "", err
		}
	}
	if !m.HasReturn {
		fmt.Fprintf(&sb, "    %s", tracePop)
		sb.WriteString("    return 0;\n")
	}
	sb.WriteString("}\n\n")
	return sb.String(), nil
}
