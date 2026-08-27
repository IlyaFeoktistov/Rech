package semantic

import (
	"fmt"
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
	// как Целое, docs/ROADMAP.md), только именованным набором констант.
	enums      map[string]map[string]int64
	thisEntity string
	scopes     []scope
	returnType ir.Type
	hasReturn  bool

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
	// кратких форм состояний (шаг 4.1), про которые на данный момент
	// анализа ДОКАЗАНО, что переменная name (объявленная в scopes[i]) через
	// них прошла ("получатель становится <состояние>", BecomeStmt). Это и
	// есть flow-sensitive проверка typestate из шага 5: параметр,
	// объявленный производным типом состояния (funcSig.ParamStates),
	// принимает аргумент только если его established-набор содержит
	// требуемое состояние - см. analyzeCallExpr/analyzeMethodCallExpr.
	// Отсутствие записи - то же самое, что пустой набор (ничего не
	// доказано) - тот же безопасный дефолт, что и everywhere else в этом
	// анализе. Значение отслеживается только для простого идентификатора
	// (случай "этот" - отдельно, thisEstablished ниже): поле сущности,
	// результат вызова и любое другое выражение не отслеживаются вообще
	// (getEstablished/establishedStatesOf возвращают nil) - осознанное
	// текущее ограничение (доказательство только для локальных переменных
	// и "этот", не межпроцедурное и не через поля), совместимое с любым
	// будущим расширением "жёстче".
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
}

func newCtx(funcs map[string]funcSig, entities map[string]entitySig, methods map[string]map[string]funcSig, aliases map[string]string, interfaces map[string]interfaceSig, enums map[string]map[string]int64, thisEntity string, returnType ir.Type, hasReturn bool, resolvedConsts map[string]ir.Expr, resolvingConsts map[string]bool) *ctx {
	return &ctx{
		funcs: funcs, entities: entities, methods: methods, aliases: aliases, interfaces: interfaces, enums: enums, thisEntity: thisEntity,
		scopes: []scope{{}}, returnType: returnType, hasReturn: hasReturn,
		established:     []map[string]map[string]bool{{}},
		thisEstablished: map[string]bool{},
		consts:          []map[string]bool{{}},
		resolvedConsts:  resolvedConsts,
		resolvingConsts: resolvingConsts,
	}
}

func (c *ctx) push() {
	c.scopes = append(c.scopes, scope{})
	c.established = append(c.established, map[string]map[string]bool{})
	c.consts = append(c.consts, map[string]bool{})
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
		return fmt.Errorf("переменная %q уже объявлена в этой области видимости", name)
	}
	top[name] = typ
	return nil
}

func (c *ctx) lookup(name string) (ir.Type, bool) {
	for _, sc := range slices.Backward(c.scopes) {
		if typ, ok := sc[name]; ok {
			return typ, true
		}
	}
	return ir.TypeUnknown, false
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
