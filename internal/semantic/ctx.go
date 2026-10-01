package semantic

import (
	"slices"

	"github.com/IlyaFeoktistov/Rech/internal/ir"
)

type scope map[string]ir.Type

// ctx - состояние анализа одной функции/метода: стек областей видимости
// (шаг 2 - если/пока/для вводят вложенные блоки, затенение разрешено -
// declare проверяет только текущую область, не внешние), сигнатуры всех
// функций/сущностей/методов программы (для вызовов и доступа к полям,
// шаги 3-4), тип результата текущей функции/метода (для проверки "вернуть
// <значение>") и имя сущности-владельца, если сейчас анализируется метод
// ("" - обычная функция, "этот" внутри неё синтаксически невозможен -
// парсер уже это гарантирует). Аналог genCtx в cbackend, только на уровне
// IR/семантики, не C.
type ctx struct {
	// inferScope - объявление, в теле которого идёт анализ ("функция",
	// "Сущность.метод") - часть ключа выведенных аргументов типа
	// (generic_infer.go); лямбда наследует его у объемлющего.
	inferScope string
	funcs      map[string]funcSig
	entities   map[string]entitySig
	methods    map[string]map[string]funcSig
	aliases    map[string]string
	interfaces map[string]interfaceSig
	// enums - имя перечисления (шаг 12) -> имя члена -> значение (Целое,
	// уже вычисленное - см. package doc ast.EnumDecl). Общая для всей
	// программы карта (не меняется во время анализа тел, в отличие от
	// entities/methods, которые могут пополняться generic-инстанциациями/
	// встроенными сущностями Запрос/Ответ) - разрешение "Имя.Член" в
	// analyzeExpr (*ast.FieldAccess) читает её напрямую, без резолюции
	// типа: перечисление не является типом значения (оно логически хранится
	// как Целое), только именованным набором констант.
	enums      map[string]map[string]int64
	thisEntity string
	scopes     []scope
	returnType ir.Type
	hasReturn  bool
	// inCtor - анализируется тело конструктора (не лямбда внутри него -
	// у лямбды свой ctx): нужен только тексту ошибки "вернуть <значение>".
	inCtor bool

	// consts - параллельный scopes стек (как established) - consts[i][name]
	// == true, если переменная name (объявленная в scopes[i]) объявлена
	// "константа", не "переменная" - Assign проверяет его перед тем, как
	// разрешить переприсваивание (шаг 12: "попытка присвоить заново -
	// ошибка компиляции"). В отличие от established, здесь не нужно
	// пересечение по if/else или откат после цикла - это статическое
	// свойство самого объявления (каким словом оно введено), не факт,
	// который "устанавливается" по ходу потока управления.
	consts []map[string]bool

	// established - параллельный scopes стек: established[i][name] - набор
	// кратких форм состояний, про которые на данный момент анализа
	// ДОКАЗАНО, что переменная name (объявленная в scopes[i]) через них
	// прошла ("получатель становится <состояние>", BecomeStmt). Это и есть
	// flow-sensitive проверка typestate: параметр, объявленный производным
	// типом состояния (funcSig.ParamStates), компилируется без накладных
	// расходов, если established-набор аргумента содержит требуемое
	// состояние, иначе - см. checkCallArgs - аргумент оборачивается в
	// ir.StateCheckExpr (рантайм-проверка вместо ошибки компиляции).
	// Отсутствие записи - то же самое, что пустой набор (ничего не
	// доказано) - тот же безопасный дефолт, что и everywhere else в этом
	// анализе. Значение отслеживается только для простого идентификатора
	// (случай "этот" - отдельно, thisEstablished ниже): поле сущности,
	// результат вызова и любое другое выражение не отслеживаются вообще
	// (getEstablished/establishedStatesOf возвращают nil) - осознанное
	// текущее ограничение (доказательство только для локальных переменных
	// и "этот", не межпроцедурное и не через поля) - для них checkCallArgs
	// всегда вставит рантайм-проверку, а не поспешно откажет в компиляции.
	established []map[string]map[string]bool

	// thisEstablished - тот же смысл, что established, но для "этот":
	// получателя нет смысла держать в scopes/established (не переменная,
	// не может быть переобъявлен/затенён в рамках одного метода), поэтому
	// отдельное поле, без стека (метод не бывает вложен в другой метод).
	thisEstablished map[string]bool

	// resolvedConsts/resolvingConsts - вычисление константы уровня сущности
	// (ключ "Сущность.Константа", см. resolveEntityConstant): resolvedConsts -
	// кэш уже вычисленных значений (константа вычисляется РОВНО один раз на
	// весь Analyze, а не заново на каждое обращение), resolvingConsts -
	// множество констант, вычисление которых сейчас идёт (ключ появляется
	// перед рекурсивным analyzeExpr и снимается сразу после) - если во время
	// вычисления встречается тот же ключ, значит константа (прямо или через
	// цепочку других констант) ссылается сама на себя, что иначе ушло бы в
	// бесконечную рекурсию analyzeExpr. Обе карты созданы один раз в
	// Analyze и переданы по ссылке во все newCtx этого прохода (в отличие от
	// established/consts выше, это не стек по scope, а общее для всей
	// программы состояние - как entities/enums).
	resolvedConsts  map[string]ir.Expr
	resolvingConsts map[string]bool

	// allowParentCall - true РОВНО во время анализа инструкции с индексом 0
	// тела конструктора (analyzeConstructor) - то, что делает "родитель(...)"
	// (ParentCallStmt) допустимым только там: сама case-ветка сбрасывает
	// его сразу после использования (не более одного вызова), а push()
	// ниже сбрасывает его при входе в любой вложенный блок (если/пока/для)
	// - "родитель(...)" внутри "если ... { родитель(...) }" на первом месте
	// тела конструктора тоже отклоняется, не только "не первым по счёту".
	allowParentCall bool

	// loopDepth - число охватывающих циклов (пока/для/для-в/C-подобный для)
	// в точке анализа текущей инструкции - 0 означает "прервать"/
	// "продолжить" здесь недопустимы (analyzeStmt, case *ast.BreakStmt/
	// *ast.ContinueStmt). Растёт/убывает в analyzeLoopBody - единой точке
	// входа в тело ЛЮБОГО из четырёх видов цикла - сбрасывается в 0 у
	// тела лямбды (newLambdaCtx строит СВЕЖИЙ ctx) - "прервать" внутри
	// лямбды, объявленной внутри цикла, не может видеть охватывающий цикл
	// снаружи (та же граница, что и у "вернуть"/hasReturn).
	loopDepth int

	// a - владеющий Analyzer, нужен свободным функциям analyzeExpr/
	// analyzeStmt (принимают только *ctx, не *Analyzer) для доступа к
	// состоянию на весь Analyze, для которого нет места среди
	// entities/methods/... - сейчас единственный потребитель -
	// registerCallable ("Вызываемый" - см. analyzeFuncRefExpr/
	// analyzeLambdaExpr).
	a *Analyzer

	// parent - объемлющий ctx - не nil только для тела лямбды:
	// lookup, не найдя имя в собственных scopes, ищет его в parent и, если
	// нашёл, запоминает как захват (captures/captureOrder) - так лямбда
	// узнаёт, какие внешние переменные скопировать в собственное окружение
	// замыкания при кодогенерации (см. analyzeLambdaExpr).
	parent       *ctx
	captures     map[string]ir.Type
	captureOrder []string

	// byRefCaptures - имя внешней переменной (у ЭТОЙ лямбды, явно
	// захваченной по ссылке - "&имя" в списке захвата, см. package doc
	// ir.CaptureVar) -> её тип-указатель ("Указатель<T>"). Только у ctx
	// лямбды с явным списком захвата - lookup/declare выше по-прежнему
	// видят T (обычный тип значения, объявлен через declare как всегда) -
	// эта карта нужна ДОПОЛНИТЕЛЬНО, ровно в тех нескольких местах
	// семантического анализа, что оборачивают Identifier/Assign этого
	// имени в Deref(Assign)Expr вместо обычного чтения/записи (см. package
	// doc CaptureVar). НЕ стек по scope (в отличие от captures/consts
	// выше) - byRefResolve ниже проверяет её напрямую, без обхода
	// scopes - declare() отдельно отвергает попытку переобъявить то же имя
	// вложенным блоком (VarDecl/параметр/и т.п.), поэтому спутать
	// захваченное имя с затеняющей его локальной переменной той же лямбды
	// невозможно.
	byRefCaptures map[string]ir.Type
}

// byRefPointerType - true и тип-указатель ("Указатель<T>"), если name -
// имя, явно захваченное этой лямбдой по ссылке (byRefCaptures) - см.
// package doc ir.CaptureVar/ctx.byRefCaptures.
func (c *ctx) byRefPointerType(name string) (ir.Type, bool) {
	t, ok := c.byRefCaptures[name]
	return t, ok
}

func newCtx(a *Analyzer, funcs map[string]funcSig, entities map[string]entitySig, methods map[string]map[string]funcSig, aliases map[string]string, interfaces map[string]interfaceSig, enums map[string]map[string]int64, thisEntity string, returnType ir.Type, hasReturn bool, resolvedConsts map[string]ir.Expr, resolvingConsts map[string]bool) *ctx {
	return &ctx{
		a:     a,
		funcs: funcs, entities: entities, methods: methods, aliases: aliases, interfaces: interfaces, enums: enums, thisEntity: thisEntity,
		scopes: []scope{{}}, returnType: returnType, hasReturn: hasReturn,
		established:     []map[string]map[string]bool{{}},
		thisEstablished: map[string]bool{},
		consts:          []map[string]bool{{}},
		resolvedConsts:  resolvedConsts,
		resolvingConsts: resolvingConsts,
	}
}

// newLambdaCtx строит ctx для тела лямбды - как newCtx, но с
// parent = sc (захват свободных переменных, см. package doc ctx.parent) и
// без доступа к "этот"/allowParentCall объемлющего метода: лямбда внутри
// метода не наследует его получателя (thisEntity: "") - оставлено на
// будущее расширение, не решение этого среза.
func newLambdaCtx(sc *ctx, returnType ir.Type, hasReturn bool) *ctx {
	c := newCtx(sc.a, sc.funcs, sc.entities, sc.methods, sc.aliases, sc.interfaces, sc.enums, "", returnType, hasReturn, sc.resolvedConsts, sc.resolvingConsts)
	c.parent = sc
	c.inferScope = sc.inferScope
	return c
}

func (c *ctx) push() {
	c.scopes = append(c.scopes, scope{})
	c.established = append(c.established, map[string]map[string]bool{})
	c.consts = append(c.consts, map[string]bool{})
	c.allowParentCall = false
}

func (c *ctx) pop() {
	c.scopes = c.scopes[:len(c.scopes)-1]
	c.established = c.established[:len(c.established)-1]
	c.consts = c.consts[:len(c.consts)-1]
}

// getEstablished возвращает набор доказанных состояний переменной name (см.
// established) - ищет от самой внутренней области видимости к внешним, тем
// же порядком, что и lookup (переменная резолвится в ту же область
// видимости, что и её тип).
func (c *ctx) getEstablished(name string) map[string]bool {
	for i, sc := range slices.Backward(c.scopes) {
		if _, ok := sc[name]; ok {
			return c.established[i][name]
		}
	}
	return nil
}

// setEstablished записывает набор доказанных состояний переменной name - в
// ту же область видимости, где она объявлена (не обязательно текущую самую
// внутреннюю - Assign может присваивать переменной из внешней области).
// Молча ничего не делает, если name не найдено ни в одной области (не
// должно происходить - вызывается только после успешного declare/lookup).
func (c *ctx) setEstablished(name string, states map[string]bool) {
	for i, sc := range slices.Backward(c.scopes) {
		if _, ok := sc[name]; ok {
			if c.established[i] == nil {
				c.established[i] = map[string]map[string]bool{}
			}
			c.established[i][name] = states
			return
		}
	}
}

// establishedStatesOf возвращает набор доказанных состояний для выражения
// e, если оно - отслеживаемая форма (простой идентификатор или "этот"), nil
// для всего остального (безопасный дефолт "ничего не доказано" - поле
// сущности, результат вызова, "новый" и т.д.).
func establishedStatesOf(sc *ctx, e ir.Expr) map[string]bool {
	switch e := e.(type) {
	case *ir.Identifier:
		return sc.getEstablished(e.Name)
	case *ir.ThisExpr:
		return sc.thisEstablished
	default:
		return nil
	}
}

// establishedForNewBinding - установленные состояния для переменной name
// сразу после того, как ей присвоено значение value (Assign) или она
// объявлена с ним (VarDecl, name - ещё не в scopes на момент вызова, чтобы
// "переменная x = x.метод()" при затенении внешнего x резолвился в него, не
// в себя). Обычные формы - через establishedStatesOf; особый случай -
// "x = x.метод()"/"переменная x = x.метод()": получатель вызова метода
// (Receiver) - это же самое имя name - в этом случае analyzeMethodCallExpr
// уже (по-настоящему) перенёс постусловие метода на established[name] по
// ходу вычисления value (получатель метода передаётся по ссылке, см.
// funcSig.GuaranteedThisStates) - наследуем то, что там уже есть, а не
// сбрасываем в nil только из-за того, что value как выражение (результат
// вызова метода) сам по себе не отслеживаемая форма.
func establishedForNewBinding(sc *ctx, name string, value ir.Expr) map[string]bool {
	if mc, ok := value.(*ir.MethodCallExpr); ok {
		if recv, ok := mc.Receiver.(*ir.Identifier); ok && recv.Name == name {
			return sc.getEstablished(name)
		}
	}
	return establishedStatesOf(sc, value)
}

// cloneStates - копия набора состояний (established хранит эти наборы по
// значению карты, но карты в Go - ссылочный тип: без явного копирования
// становится("X") у одной переменной мутировал бы набор, на который могла
// ссылаться established-запись другой переменной/снапшота).
func cloneStates(states map[string]bool) map[string]bool {
	out := make(map[string]bool, len(states))
	for s := range states {
		out[s] = true
	}
	return out
}

// snapshotEstablished - глубокая копия established (или thisEstablished) -
// нужна для слияния веток если/иначе (intersectEstablished) и для отката
// эффекта тела цикла (пока/для/для-в) на переменные, объявленные до цикла -
// см. соответствующие case в analyzeStmt.
func snapshotEstablished(layers []map[string]map[string]bool) []map[string]map[string]bool {
	out := make([]map[string]map[string]bool, len(layers))
	for i, layer := range layers {
		copyLayer := make(map[string]map[string]bool, len(layer))
		for name, states := range layer {
			copyLayer[name] = cloneStates(states)
		}
		out[i] = copyLayer
	}
	return out
}

// intersectEstablished - пересечение двух снапшотов established одной и той
// же формы (одинаковое число слоёв - снимались при одной и той же глубине
// стека областей видимости, до входа в ветку если/иначе): переменная после
// "если/иначе" доказанно прошла состояние X только если прошла его НА ОБОИХ
// путях выполнения - тот же принцип, что definite-assignment analysis.
func intersectEstablished(a, b []map[string]map[string]bool) []map[string]map[string]bool {
	out := make([]map[string]map[string]bool, len(a))
	for i := range a {
		layer := make(map[string]map[string]bool, len(a[i]))
		for name, statesA := range a[i] {
			statesB := b[i][name]
			merged := map[string]bool{}
			for s := range statesA {
				if statesB[s] {
					merged[s] = true
				}
			}
			layer[name] = merged
		}
		out[i] = layer
	}
	return out
}

func (c *ctx) declare(name string, typ ir.Type) error {
	top := c.scopes[len(c.scopes)-1]
	if _, exists := top[name]; exists {
		return errNoPos(CodeDuplicateDecl, "переменная %q уже объявлена в этой области видимости", name)
	}
	// Захваченное по ссылке имя (byRefCaptures) - НЕ по scope (см. package
	// doc) - переобъявление ВЛОЖЕННЫМ блоком (если/пока/для) с тем же
	// именем формально разрешено обычным затенением, но здесь запрещено
	// явно: иначе byRefPointerType продолжил бы считать затенённое имя
	// ссылкой на ВНЕШНЮЮ переменную, хотя лежащий в scopes тип - уже тип
	// новой, отдельной локальной переменной - тихая порча вместо
	// затенения. Сама лямбда с явным списком захвата не имеет "своих"
	// вложенных byRefCaptures (declare для самих Captures вызывается один
	// раз, до анализа тела, top в этот момент - единственная область
	// видимости лямбды) - эта проверка защищает только от ПОЗДНЕЙШЕГО
	// переобъявления внутри тела.
	if _, isByRef := c.byRefCaptures[name]; isByRef {
		return errNoPos(CodeDuplicateDecl, "переменная %q уже объявлена (захвачена по ссылке, \"&%s\" в списке захвата лямбды) - переобъявить её нельзя", name, name)
	}
	top[name] = typ
	return nil
}

// lookup ищет name в собственных scopes (от самой внутренней области видимости
// к внешним), а если не нашёл и c.parent != nil (тело лямбды) -
// рекурсивно в объемлющем ctx: найденное там имя - свободная переменная тела
// лямбды, запоминается в c.captures/c.captureOrder (в порядке первого
// использования, без повторов) - см. package doc ctx.parent.
func (c *ctx) lookup(name string) (ir.Type, bool) {
	for _, sc := range slices.Backward(c.scopes) {
		if typ, ok := sc[name]; ok {
			return typ, true
		}
	}
	if c.parent == nil {
		return ir.TypeUnknown, false
	}
	typ, ok := c.parent.lookup(name)
	if !ok {
		return ir.TypeUnknown, false
	}
	if _, already := c.captures[name]; !already {
		if c.captures == nil {
			c.captures = map[string]ir.Type{}
		}
		c.captures[name] = typ
		c.captureOrder = append(c.captureOrder, name)
	}
	return typ, true
}

// visibleNames возвращает имена всех переменных, видимых в текущей точке
// (собственные scopes, от внешних к внутренним - порядок не важен для
// единственного вызывающего места, suggestNameHint) плюс, если есть,
// видимые из объемлющего ctx (тело лямбды - см. package doc ctx.parent) -
// используется только для подсказки "возможно, имелось в виду %q" у
// необъявленного имени (см. internal/semantic/suggest.go), не для
// разрешения имён.
func (c *ctx) visibleNames() []string {
	var out []string
	for _, sc := range c.scopes {
		for name := range sc {
			out = append(out, name)
		}
	}
	if c.parent != nil {
		out = append(out, c.parent.visibleNames()...)
	}
	return out
}

// markConst помечает уже объявленную (declare уже вызван) переменную name
// в самой внутренней области видимости как константу - вызывается сразу
// после declare в analyzeStmt (*ast.ConstDecl), никогда отдельно.
func (c *ctx) markConst(name string) {
	c.consts[len(c.consts)-1][name] = true
}

// isConst - true, если name объявлено "константа", не "переменная". Ищет
// область видимости через c.scopes (не напрямую через c.consts) - иначе
// затенение (внутренняя "переменная x", внешняя "константа x") дало бы
// неверный ответ: у внутренней области нет своей записи в consts вообще
// (markConst не вызывался), поиск бы провалился дальше и нашёл внешнюю
// константу, хотя x в этой области видимости - обычная переменная.
func (c *ctx) isConst(name string) bool {
	for i, sc := range slices.Backward(c.scopes) {
		if _, ok := sc[name]; ok {
			return c.consts[i][name]
		}
	}
	return false
}
