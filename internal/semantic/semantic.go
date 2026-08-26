// Package semantic производит internal/ir.Program из ast.Program (шаг 5.b,
// docs/ROADMAP.md) - проверка типов и разрешение имён на отдельном слое,
// а не внутри backend'а, как сейчас (internal/backend/cbackend делает это
// сам во время генерации C, шаг 1).
//
// Реализуется вертикальным срезом за срезом по конструкциям языка - см.
// internal/ir, package doc. Analyze возвращает понятную ошибку "не
// реализовано в IR" для всего, что ещё не перенесено - это не тихая
// деградация, а явная граница текущего покрытия.
package semantic

import (
	"fmt"
	"slices"
	"strings"

	"github.com/IlyaFeoktistov/Rech/internal/ast"
	"github.com/IlyaFeoktistov/Rech/internal/builtins"
	"github.com/IlyaFeoktistov/Rech/internal/ir"
)

// builtinIRType переводит internal/builtins.Kind (общий для cbackend и
// этого пакета, ничего не знающий про конкретный ir.Type/valueType) в
// ir.Type.
func builtinIRType(k builtins.Kind) ir.Type {
	switch k {
	case builtins.KindInt:
		return ir.TypeInt
	case builtins.KindFloat:
		return ir.TypeFloat
	case builtins.KindString:
		return ir.TypeString
	case builtins.KindBool:
		return ir.TypeBool
	case builtins.KindFile:
		return ir.TypeFile
	case builtins.KindSocket:
		return ir.TypeSocket
	default:
		return ir.Type{}
	}
}

// registerBuiltinFuncs заполняет funcs встроенными функциями шага 8
// (internal/builtins) - тот же список и тот же принцип регистрации, что и
// cbackend.registerBuiltins (AST-путь) - до разбора пользовательских
// функций программы.
func registerBuiltinFuncs(funcs map[string]funcSig) {
	for _, f := range builtins.All {
		params := make([]ir.Type, len(f.Params))
		for i, k := range f.Params {
			params[i] = builtinIRType(k)
		}
		funcs[f.Name] = funcSig{Params: params, ReturnType: builtinIRType(f.ReturnKind), HasReturn: f.HasReturn}
	}
}

// checkFieldVisibility возвращает ошибку, если поле fieldName сущности
// entityName приватное (шаг 4.3) и обращение происходит не изнутри метода
// этой же сущности (sc.thisEntity != entityName) - см. одноимённый хелпер в
// cbackend/generator.go (проверка структурно одна и та же, но не вынесена в
// общий пакет - ir не должен знать про AST-путь, а cbackend/generator.go не
// знает про internal/ir).
func checkFieldVisibility(sc *ctx, entityName, fieldName string, private bool, line int) error {
	if !private || sc.thisEntity == entityName {
		return nil
	}
	return fmt.Errorf("строка %d: поле %q сущности %q приватное - доступ только из методов самой сущности", line, fieldName, entityName)
}

// checkConstVisibility - то же самое, что checkFieldVisibility, но для
// константы сущности (отдельная функция, а не переиспользование
// checkFieldVisibility "как есть" - иначе сообщение об ошибке называло бы
// константу полем).
func checkConstVisibility(sc *ctx, entityName, constName string, private bool, line int) error {
	if !private || sc.thisEntity == entityName {
		return nil
	}
	return fmt.Errorf("строка %d: константа %q сущности %q приватная - доступ только из методов самой сущности", line, constName, entityName)
}

// resolveEntityConstant возвращает значение константы entityName.constName.
// Значение вычисляется РОВНО один раз на весь Analyze: результат кладётся в
// sc.resolvedConsts (общая для всей программы карта, см. newCtx) и на
// повторных обращениях просто читается оттуда - без этого выражение
// константы (если это не литерал, а что-то посложнее) вычислялось бы заново
// на каждое обращение "Сущность.Константа"/"экземпляр.Константа".
//
// sc.resolvingConsts защищает от циклической ссылки константы саму на себя
// (напрямую или через цепочку других констант, в т.ч. чужих сущностей):
// ключ ставится перед рекурсивным analyzeExpr и снимается сразу после -
// если тот же ключ встретится повторно, пока он ещё не снят, значит
// вычисление зациклилось - без этой проверки analyzeExpr вызывал бы сам
// себя без остановки и падал стек-оверфлоу самого компилятора вместо
// понятной ошибки.
//
// Видимость (приватная/публичная) здесь не проверяется - это забота
// вызывающей стороны (обеих точек доступа в analyzeExpr выше): при
// построении ir.Entity.Constants для собственной сущности видимость
// проверять не нужно (сущность имеет право использовать собственную
// приватную константу для вычисления другой своей константы).
func resolveEntityConstant(sc *ctx, entityName, constName string, line int) (ir.Expr, error) {
	key := entityName + "." + constName
	if v, ok := sc.resolvedConsts[key]; ok {
		return v, nil
	}
	if sc.resolvingConsts[key] {
		return nil, fmt.Errorf("строка %d: константа %q.%q ссылается сама на себя (прямо или через другую константу) - вычислить значение невозможно", line, entityName, constName)
	}
	sig := sc.entities[entityName]
	idx := sig.constIndex(constName)
	sc.resolvingConsts[key] = true
	val, err := analyzeExpr(sc, sig.ConstExprs[idx])
	delete(sc.resolvingConsts, key)
	if err != nil {
		return nil, fmt.Errorf("строка %d: при вычислении константы %q.%q: %w", line, entityName, constName, err)
	}
	sc.resolvedConsts[key] = val
	return val, nil
}

// checkMethodVisibility - то же самое для вызова приватного метода.
func checkMethodVisibility(sc *ctx, entityName, methodName string, private bool, line int) error {
	if !private || sc.thisEntity == entityName {
		return nil
	}
	return fmt.Errorf("строка %d: метод %q сущности %q приватный - вызов только из методов самой сущности", line, methodName, entityName)
}

func (s entitySig) fieldIndex(name string) int {
	for i, n := range s.FieldNames {
		if n == name {
			return i
		}
	}
	return -1
}

// scope - символьная таблица одной области видимости: имя переменной ->
// тип.
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
	funcs    map[string]funcSig
	entities map[string]entitySig
	methods  map[string]map[string]funcSig
	aliases  map[string]string
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

func newCtx(funcs map[string]funcSig, entities map[string]entitySig, methods map[string]map[string]funcSig, aliases map[string]string, enums map[string]map[string]int64, thisEntity string, returnType ir.Type, hasReturn bool, resolvedConsts map[string]ir.Expr, resolvingConsts map[string]bool) *ctx {
	return &ctx{
		funcs: funcs, entities: entities, methods: methods, aliases: aliases, enums: enums, thisEntity: thisEntity,
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

// resolveTypeName - единственное место, где имя типа Rech (строка из AST)
// маппится на ir.Type: встроенные примитивы, уже собранная сущность
// (entities - сигнатуры, собранные раньше по файлу, см. collectEntities;
// сущность, объявленная позже в файле, чем место, где её имя встретилось
// как тип - та же граница, что и в cbackend.resolveType) или производный
// тип состояния (aliases - "АвторизованныйПользователь" -> "Пользователь",
// шаг 4.1, см. collectEntities) - резолвится в тип БАЗОВОЙ сущности, не в
// собственный тип: производный тип - псевдоним без своего
// представления, как и в cbackend (genCtx.stateAliases), - или встроенная
// generic-коллекция (шаг 4.2, resolveGenericTypeName).
func resolveTypeName(entities map[string]entitySig, methods map[string]map[string]funcSig, aliases map[string]string, name string) (ir.Type, error) {
	switch name {
	case "Строка":
		return ir.TypeString, nil
	case "Целое":
		return ir.TypeInt, nil
	case "Число":
		return ir.TypeFloat, nil
	case "Логическое":
		return ir.TypeBool, nil
	case "Файл":
		return ir.TypeFile, nil
	case "Сокет":
		return ir.TypeSocket, nil
	case "Запрос":
		registerHTTPEntity(entities, "Запрос", httpRequestFields)
		return ir.EntityType("Запрос"), nil
	case "Ответ":
		registerHTTPEntity(entities, "Ответ", httpResponseFields)
		return ir.EntityType("Ответ"), nil
	}
	if _, ok := entities[name]; ok {
		return ir.EntityType(name), nil
	}
	if base, ok := aliases[name]; ok {
		return ir.EntityType(base), nil
	}
	return resolveGenericTypeName(entities, methods, aliases, name)
}

// httpRequestFields/httpResponseFields - фиксированные поля встроенных
// сущностей "Запрос"/"Ответ" - в этом порядке
// (позиционный конструктор "новый Запрос(...)" следует ему) и без
// падежного согласования (обычные имена полей). Заголовки - одна `Строка`
// (каждый заголовок на своей строке, разделитель "\r\n"), не список пар
// ключ/значение - в языке пока нет словаря/хеш-карты (docs/ROADMAP.md), а
// заводить отдельный тип только для заголовков было бы преждевременно.
var httpRequestFields = []ir.Field{
	{Name: "метод", Type: ir.TypeString},
	{Name: "адрес", Type: ir.TypeString},
	{Name: "порт", Type: ir.TypeInt},
	{Name: "путь", Type: ir.TypeString},
	{Name: "заголовки", Type: ir.TypeString},
	{Name: "тело", Type: ir.TypeString},
}

var httpResponseFields = []ir.Field{
	{Name: "код", Type: ir.TypeInt},
	{Name: "заголовки", Type: ir.TypeString},
	{Name: "тело", Type: ir.TypeString},
}

// registerHTTPEntity - лениво синтезирует entitySig встроенной сущности
// (мемоизация, как resolveGenericTypeName у generic-коллекций) - вызывается
// при первой встрече имени "Запрос"/"Ответ" где угодно (параметр, поле,
// тип результата, "новый Запрос(...)"), не только при вызове самой
// "запрос" - пользователь может объявить свою функцию, принимающую/
// возвращающую эти сущности, без единого вызова встроенной "запрос".
func registerHTTPEntity(entities map[string]entitySig, name string, fields []ir.Field) {
	if _, exists := entities[name]; exists {
		return
	}
	sig := entitySig{}
	for _, f := range fields {
		sig.FieldNames = append(sig.FieldNames, f.Name)
		sig.FieldTypes = append(sig.FieldTypes, f.Type)
		sig.FieldPrivate = append(sig.FieldPrivate, false)
	}
	entities[name] = sig
}

// standardEnums - встроенные перечисления -
// доступны в любой программе без объявления, тем же принципом, что и
// Файл/Сокет/Запрос/Ответ (пользователь не объявляет их сам, но может
// объявить СВОЁ перечисление с любым другим именем обычным
// "перечисление" - collectEnums). Курируемый список, не механизм
// пользовательских generic-типов - обычные перечисления того же вида,
// что пользователь мог бы написать сам, просто уже готовые для типичных,
// заранее известных наборов значений.
var standardEnums = map[string]map[string]int64{
	// HTTPКод - для сравнения с "запрос(...).код" (шаг 9, HTTP-клиент) без
	// голых магических чисел в исходнике программы - только самые частые
	// коды, не полный список IANA (курируемый минимум, не исчерпывающий).
	"HTTPКод": {
		"ОК":                 200,
		"Создано":            201,
		"Принято":            202,
		"БезСодержимого":     204,
		"ПеремещёнПостоянно": 301,
		"НайденВременно":     302,
		"НеИзменялось":       304,
		"НеправильныйЗапрос": 400,
		"НеАвторизован":      401,
		"Запрещено":          403,
		"НеНайдено":          404,
		"МетодНеДопустим":    405,
		"Конфликт":           409,
		"ОшибкаСервера":      500,
		"НеРеализовано":      501,
		"СервисНедоступен":   503,
	},
	// ДеньНедели/Месяц - нет ни одной функции даты/времени в языке пока
	// (docs/ROADMAP.md), но сами имена дней/месяцев не изменятся, когда она
	// появится - разумно завести сейчас, раз запрошены "заранее известные"
	// перечисления. Понедельник == 1 - ISO 8601, не "воскресенье == 0",
	// как в некоторых языках.
	"ДеньНедели": {
		"Понедельник": 1, "Вторник": 2, "Среда": 3, "Четверг": 4,
		"Пятница": 5, "Суббота": 6, "Воскресенье": 7,
	},
	"Месяц": {
		"Январь": 1, "Февраль": 2, "Март": 3, "Апрель": 4,
		"Май": 5, "Июнь": 6, "Июль": 7, "Август": 8,
		"Сентябрь": 9, "Октябрь": 10, "Ноябрь": 11, "Декабрь": 12,
	},
}

// parseGenericTypeName разбирает закодированную парсером строку вида
// "Список<Целое>" на (шаблон, аргумент) - тот же формат, что и
// cbackend.parseGenericTypeName (AST-путь) - IR не должен знать про
// cbackend, поэтому не переиспользует ту функцию напрямую, но формат
// строки - общий инвариант всего компилятора (parser.parseTypeName).
func parseGenericTypeName(name string) (template, arg string, ok bool) {
	i := strings.IndexByte(name, '<')
	if i < 0 || !strings.HasSuffix(name, ">") {
		return "", "", false
	}
	return name[:i], name[i+1 : len(name)-1], true
}

// genericMethodSigs - сигнатуры методов встроенного generic-шаблона
// (шаг 4.2) для конкретного типа элемента elem - курируемый список, как и
// genericTemplates в cbackend (AST-путь), только здесь сигнатуры, а не
// C-код (C-код синтезирует cbackend.GenerateFromIR по Program.Generics,
// переиспользуя cbackend.listCollectionCCode/stackCollectionCCode/
// queueCollectionCCode - ту же реализацию, что и у AST-пути).
func genericMethodSigs(template string, elem ir.Type) (map[string]funcSig, bool) {
	switch template {
	case "Список":
		return map[string]funcSig{
			"добавить":   {Params: []ir.Type{elem}},
			"получить":   {Params: []ir.Type{ir.TypeInt}, ReturnType: elem, HasReturn: true},
			"установить": {Params: []ir.Type{ir.TypeInt, elem}},
			"длина":      {ReturnType: ir.TypeInt, HasReturn: true},
			"пусто":      {ReturnType: ir.TypeBool, HasReturn: true},
		}, true
	case "Стопка":
		return map[string]funcSig{
			"положить": {Params: []ir.Type{elem}},
			"снять":    {ReturnType: elem, HasReturn: true},
			"верх":     {ReturnType: elem, HasReturn: true},
			"длина":    {ReturnType: ir.TypeInt, HasReturn: true},
			"пусто":    {ReturnType: ir.TypeBool, HasReturn: true},
		}, true
	case "Очередь":
		return map[string]funcSig{
			"добавить": {Params: []ir.Type{elem}},
			"извлечь":  {ReturnType: elem, HasReturn: true},
			"первый":   {ReturnType: elem, HasReturn: true},
			"длина":    {ReturnType: ir.TypeInt, HasReturn: true},
			"пусто":    {ReturnType: ir.TypeBool, HasReturn: true},
		}, true
	default:
		return nil, false
	}
}

// resolveGenericTypeName - как cbackend.resolveType (AST-путь): при первой
// встрече конкретной инстанциации builtin-коллекции ("Список<Целое>")
// синтезирует её сигнатуру (entitySig/funcSig) и регистрирует в entities/
// methods (мемоизация - синтез только один раз, дальше просто найдётся в
// карте) - дальше остальной internal/semantic обращается с ней как с
// обычной сущностью, не зная, пользовательская она или встроенная.
func resolveGenericTypeName(entities map[string]entitySig, methods map[string]map[string]funcSig, aliases map[string]string, name string) (ir.Type, error) {
	template, argName, ok := parseGenericTypeName(name)
	if !ok {
		return ir.TypeUnknown, fmt.Errorf("неизвестный тип %q", name)
	}
	elem, err := resolveTypeName(entities, methods, aliases, argName)
	if err != nil {
		return ir.TypeUnknown, fmt.Errorf("тип элемента %q в %q: %w", argName, name, err)
	}
	if elem.Kind == ir.KindEntity {
		return ir.TypeUnknown, fmt.Errorf("%q: элементы встроенных коллекций поддерживаются только для Строка/Целое/Число/Логическое, не для сущностей", name)
	}
	sigs, ok := genericMethodSigs(template, elem)
	if !ok {
		return ir.TypeUnknown, fmt.Errorf("неизвестный generic-тип %q", template)
	}
	if _, exists := entities[name]; !exists {
		entities[name] = entitySig{IsList: template == "Список", ElemType: elem}
		methods[name] = sigs
	}
	return ir.EntityType(name), nil
}

// Analyze производит IR из AST. Покрытие на данный момент - шаги 0-4
// (`вывести`, переменные/арифметика, если/пока/для, функции с параметрами
// и типом результата, вызов, сущности/поля/конструктор/методы/"этот"),
// шаг 4.1 (состояния, упрощённая версия) и шаг 4.2 (встроенные generic-
// коллекции) - см. internal/ir, package doc. Расширяется вместе с
// internal/backend/cbackend.GenerateFromIR подшаг за подшагом, шаг 5.
func Analyze(prog *ast.Program) (*ir.Program, error) {
	a := &Analyzer{
		resolvedConsts:  map[string]ir.Expr{},
		resolvingConsts: map[string]bool{},
	}

	if err := a.collectEntities(prog); err != nil {
		return nil, err
	}

	if err := a.collectFuncSigs(prog); err != nil {
		return nil, err
	}

	if err := a.collectEnums(prog); err != nil {
		return nil, err
	}

	return a.buildIR(prog)
}

// isFFIScalar - типы, допустимые для параметра/результата внешней функции
// (шаг 7, FFI) - тот же принцип, что и cbackend.isFFIScalar (AST-путь):
// ни сущности, ни generic-коллекции не имеют стабильного, задокументированного
// C-представления, на которое мог бы полагаться сторонний C-код.
func isFFIScalar(t ir.Type) bool {
	switch t.Kind {
	case ir.KindInt, ir.KindFloat, ir.KindString, ir.KindBool:
		return true
	default:
		return false
	}
}

// endsWithReturnValue - true, если последняя инструкция тела (верхнего
// уровня, не внутри вложенных если/пока/для) - "вернуть <значение>". Как и
// в AST-пути (cbackend.endsWithReturnValue) - простая синтаксическая
// проверка, не flow-sensitive анализ всех путей выполнения (это шаг 5,
// когда появится сам flow-sensitive слой - см. docs/ROADMAP.md).
func endsWithReturnValue(body []ir.Stmt) bool {
	if len(body) == 0 {
		return false
	}
	last, ok := body[len(body)-1].(*ir.ReturnStmt)
	return ok && last.Value != nil
}

// analyzeBlock анализирует список инструкций внутри уже открытой области
// видимости - вызывающий код сам решает, нужно ли sc.push()/sc.pop()
// вокруг вызова (тело функции - нет, тело если/пока/для - да).
func analyzeBlock(sc *ctx, stmts []ast.Stmt) ([]ir.Stmt, error) {
	body := make([]ir.Stmt, len(stmts))
	for i, s := range stmts {
		stmt, err := analyzeStmt(sc, s)
		if err != nil {
			return nil, err
		}
		body[i] = stmt
	}
	return body, nil
}

func analyzeStmt(sc *ctx, s ast.Stmt) (ir.Stmt, error) {
	switch s := s.(type) {
	case *ast.PrintStmt:
		value, err := analyzeExpr(sc, s.Value)
		if err != nil {
			return nil, err
		}
		return &ir.PrintStmt{Value: value}, nil

	case *ast.VarDecl:
		value, err := analyzeExpr(sc, s.Value)
		if err != nil {
			return nil, err
		}
		// До sc.declare - иначе при затенении (переменная %s уже с этим же
		// именем во внешней области) established читался бы уже с только
		// что объявленной (пустой) внутренней переменной вместо той, на
		// которую на самом деле ссылается value.
		newEstablished := establishedForNewBinding(sc, s.Name, value)
		if err := sc.declare(s.Name, ir.TypeOf(value)); err != nil {
			return nil, fmt.Errorf("строка %d: %w", s.Line, err)
		}
		sc.setEstablished(s.Name, newEstablished)
		return &ir.VarDecl{Name: s.Name, Value: value}, nil

	case *ast.ConstDecl:
		// Полностью тот же код, что и VarDecl выше (одна и та же ir.VarDecl -
		// internal/ir не различает константу и переменную вообще, см.
		// package doc ast.ConstDecl), плюс markConst сразу после declare -
		// дальше любой Assign этому имени должен быть отвергнут (см. ниже).
		value, err := analyzeExpr(sc, s.Value)
		if err != nil {
			return nil, err
		}
		newEstablished := establishedForNewBinding(sc, s.Name, value)
		if err := sc.declare(s.Name, ir.TypeOf(value)); err != nil {
			return nil, fmt.Errorf("строка %d: %w", s.Line, err)
		}
		sc.setEstablished(s.Name, newEstablished)
		sc.markConst(s.Name)
		return &ir.VarDecl{Name: s.Name, Value: value}, nil

	case *ast.Assign:
		declared, exists := sc.lookup(s.Name)
		if !exists {
			return nil, fmt.Errorf("строка %d: переменная %q не объявлена - используйте \"переменная\" для объявления", s.Line, s.Name)
		}
		if sc.isConst(s.Name) {
			return nil, fmt.Errorf("строка %d: %q объявлено как \"константа\" - нельзя присвоить новое значение", s.Line, s.Name)
		}
		value, err := analyzeExpr(sc, s.Value)
		if err != nil {
			return nil, err
		}
		if typ := ir.TypeOf(value); typ != declared {
			return nil, fmt.Errorf("строка %d: переменной %q типа %s нельзя присвоить значение типа %s", s.Line, s.Name, declared, typ)
		}
		sc.setEstablished(s.Name, establishedForNewBinding(sc, s.Name, value))
		return &ir.Assign{Name: s.Name, Value: value}, nil

	case *ast.AssertStmt:
		cond, err := analyzeExpr(sc, s.Cond)
		if err != nil {
			return nil, err
		}
		if typ := ir.TypeOf(cond); typ != ir.TypeBool {
			return nil, fmt.Errorf("строка %d: условие \"утверждать\" должно быть логическим, получено %s", s.Line, typ)
		}
		return &ir.AssertStmt{Cond: cond, Line: s.Line}, nil

	case *ast.IfStmt:
		cond, err := analyzeExpr(sc, s.Cond)
		if err != nil {
			return nil, err
		}
		if typ := ir.TypeOf(cond); typ != ir.TypeBool {
			return nil, fmt.Errorf("строка %d: условие \"если\" должно быть логическим, получено %s", s.Line, typ)
		}
		// Слияние established после "если/иначе" - переменная доказанно
		// прошла состояние X дальше по коду только если прошла его НА ОБОИХ
		// путях выполнения (intersectEstablished) - ветки анализируются от
		// одного и того же снапшота (preIf), не одна поверх результата
		// другой, иначе Else "видел" бы эффект Then.
		preIf := snapshotEstablished(sc.established)
		sc.push()
		then, err := analyzeBlock(sc, s.Then)
		sc.pop()
		if err != nil {
			return nil, err
		}
		postThen := snapshotEstablished(sc.established)
		sc.established = preIf
		var elseBody []ir.Stmt
		postElse := preIf
		if s.Else != nil {
			sc.push()
			elseBody, err = analyzeBlock(sc, s.Else)
			sc.pop()
			if err != nil {
				return nil, err
			}
			postElse = snapshotEstablished(sc.established)
		}
		sc.established = intersectEstablished(postThen, postElse)
		return &ir.IfStmt{Cond: cond, Then: then, Else: elseBody}, nil

	case *ast.WhileStmt:
		cond, err := analyzeExpr(sc, s.Cond)
		if err != nil {
			return nil, err
		}
		if typ := ir.TypeOf(cond); typ != ir.TypeBool {
			return nil, fmt.Errorf("строка %d: условие \"пока\" должно быть логическим, получено %s", s.Line, typ)
		}
		// "пока" может не выполниться ни разу - established, доказанное
		// внутри тела, не гарантировано ПОСЛЕ цикла (консервативный откат к
		// состоянию до цикла, а не fixed-point анализ по итерациям - см.
		// package doc ctx.established); внутри самого тела, в пределах
		// одного прохода, накопленные по ходу established всё ещё видны
		// (обычный sc.push/analyzeBlock/sc.pop).
		preLoop := snapshotEstablished(sc.established)
		sc.push()
		body, err := analyzeBlock(sc, s.Body)
		sc.pop()
		if err != nil {
			return nil, err
		}
		sc.established = preLoop
		return &ir.WhileStmt{Cond: cond, Body: body}, nil

	case *ast.ForStmt:
		from, err := analyzeExpr(sc, s.From)
		if err != nil {
			return nil, err
		}
		if typ := ir.TypeOf(from); typ != ir.TypeInt {
			return nil, fmt.Errorf("строка %d: граница диапазона после \"от\" должна быть целым числом, получено %s", s.Line, typ)
		}
		to, err := analyzeExpr(sc, s.To)
		if err != nil {
			return nil, err
		}
		if typ := ir.TypeOf(to); typ != ir.TypeInt {
			return nil, fmt.Errorf("строка %d: граница диапазона после \"до\" должна быть целым числом, получено %s", s.Line, typ)
		}
		preLoop := snapshotEstablished(sc.established)
		sc.push()
		if err := sc.declare(s.Var, ir.TypeInt); err != nil {
			sc.pop()
			return nil, fmt.Errorf("строка %d: %w", s.Line, err)
		}
		body, err := analyzeBlock(sc, s.Body)
		sc.pop()
		if err != nil {
			return nil, err
		}
		sc.established = preLoop
		return &ir.ForStmt{Var: s.Var, From: from, To: to, Body: body}, nil

	case *ast.ForEachStmt:
		collection, err := analyzeExpr(sc, s.Collection)
		if err != nil {
			return nil, err
		}
		typ := ir.TypeOf(collection)
		// "для <строка> в <Файл>" - потоковое построчное чтение (шаг 8), а
		// не индексированный доступ, как у Список<T> ниже - файл не
		// известной заранее длины. ElemType всегда Строка (сама строка
		// файла), сигнатуры Список<T> здесь ни при чём.
		if typ.Kind == ir.KindFile {
			preLoop := snapshotEstablished(sc.established)
			sc.push()
			if err := sc.declare(s.Var, ir.TypeString); err != nil {
				sc.pop()
				return nil, fmt.Errorf("строка %d: %w", s.Line, err)
			}
			body, err := analyzeBlock(sc, s.Body)
			sc.pop()
			if err != nil {
				return nil, err
			}
			sc.established = preLoop
			return &ir.ForEachStmt{Var: s.Var, Collection: collection, ElemType: ir.TypeString, Body: body}, nil
		}
		sig := sc.entities[typ.Entity]
		if typ.Kind != ir.KindEntity || !sig.IsList {
			return nil, fmt.Errorf("строка %d: цикл \"для %s в ...\" ожидает переменную-Список<T> или Файл, получен тип %s", s.Line, s.Var, typ)
		}
		preLoop := snapshotEstablished(sc.established)
		sc.push()
		if err := sc.declare(s.Var, sig.ElemType); err != nil {
			sc.pop()
			return nil, fmt.Errorf("строка %d: %w", s.Line, err)
		}
		body, err := analyzeBlock(sc, s.Body)
		sc.pop()
		if err != nil {
			return nil, err
		}
		sc.established = preLoop
		return &ir.ForEachStmt{Var: s.Var, Collection: collection, ElemType: sig.ElemType, Body: body}, nil

	case *ast.ReturnStmt:
		if s.Value == nil {
			if sc.hasReturn {
				return nil, fmt.Errorf("строка %d: функция с объявленным типом результата должна возвращать значение (\"вернуть <значение>\")", s.Line)
			}
			return &ir.ReturnStmt{}, nil
		}
		if !sc.hasReturn {
			return nil, fmt.Errorf("строка %d: у функции не объявлен тип результата - используйте \"вернуть\" без значения", s.Line)
		}
		value, err := analyzeExpr(sc, s.Value)
		if err != nil {
			return nil, err
		}
		if typ := ir.TypeOf(value); typ != sc.returnType {
			return nil, fmt.Errorf("строка %d: функция объявлена с типом результата %s, но возвращается значение типа %s", s.Line, sc.returnType, typ)
		}
		return &ir.ReturnStmt{Value: value}, nil

	case *ast.ExprStmt:
		if s.MethodCall != nil {
			call, err := analyzeMethodCallExpr(sc, s.MethodCall)
			if err != nil {
				return nil, err
			}
			return &ir.ExprStmt{MethodCall: call}, nil
		}
		if s.IncDec != nil {
			incDec, err := analyzeIncDecExpr(sc, s.IncDec)
			if err != nil {
				return nil, err
			}
			return &ir.ExprStmt{IncDec: incDec.(*ir.IncDecExpr)}, nil
		}
		call, err := analyzeCallExpr(sc, s.Call)
		if err != nil {
			return nil, err
		}
		return &ir.ExprStmt{Call: call}, nil

	case *ast.FieldAssign:
		if !receiverAddressable(s.Receiver) {
			return nil, fmt.Errorf("строка %d: получателем присваивания полю может быть только переменная, поле или \"этот\"", s.Line)
		}
		receiver, err := analyzeExpr(sc, s.Receiver)
		if err != nil {
			return nil, err
		}
		typ := ir.TypeOf(receiver)
		if typ.Kind != ir.KindEntity {
			return nil, fmt.Errorf("строка %d: нельзя присвоить полю %q - тип %s не сущность", s.Line, s.Field, typ)
		}
		sig := sc.entities[typ.Entity]

		// Нельзя присваивать значение константе сущности
		if cidx := sig.constIndex(s.Field); cidx != -1 {
			return nil, fmt.Errorf("строка %d: нельзя присвоить значение константе %q.%q", s.Line, typ.Entity, s.Field)
		}

		idx := sig.fieldIndex(s.Field)
		if idx == -1 {
			return nil, fmt.Errorf("строка %d: у сущности %q нет поля %q", s.Line, typ.Entity, s.Field)
		}
		if err := checkFieldVisibility(sc, typ.Entity, s.Field, sig.FieldPrivate[idx], s.Line); err != nil {
			return nil, err
		}
		value, err := analyzeExpr(sc, s.Value)
		if err != nil {
			return nil, err
		}
		if vt := ir.TypeOf(value); vt != sig.FieldTypes[idx] {
			return nil, fmt.Errorf("строка %d: полю %q типа %s нельзя присвоить значение типа %s", s.Line, s.Field, sig.FieldTypes[idx], vt)
		}
		return &ir.FieldAssign{Receiver: receiver, Field: s.Field, Value: value}, nil

	case *ast.BecomeStmt:
		receiver, err := analyzeExpr(sc, s.Receiver)
		if err != nil {
			return nil, err
		}
		typ := ir.TypeOf(receiver)
		if typ.Kind != ir.KindEntity {
			return nil, fmt.Errorf("строка %d: \"становится\" применимо только к сущности, получен тип %s", s.Line, typ)
		}
		sig := sc.entities[typ.Entity]
		if !sig.States[s.State] {
			return nil, fmt.Errorf("строка %d: у сущности %q не объявлено состояние %q", s.Line, typ.Entity, s.State)
		}
		// Фиксируем доказанный переход - только для отслеживаемых форм
		// получателя (простая переменная/"этот", см. package doc ctx.
		// established); получатель-поле сущности молча не отслеживается
		// (безопасный дефолт "ничего не доказано" дальше по коду).
		switch r := receiver.(type) {
		case *ir.Identifier:
			next := cloneStates(sc.getEstablished(r.Name))
			next[s.State] = true
			sc.setEstablished(r.Name, next)
		case *ir.ThisExpr:
			next := cloneStates(sc.thisEstablished)
			next[s.State] = true
			sc.thisEstablished = next
		}
		return &ir.BecomeStmt{Receiver: receiver, State: s.State}, nil

	default:
		return nil, fmt.Errorf("инструкция %T не поддерживается", s)
	}
}

// receiverAddressable - получатели вызова метода/присваивания полю, у
// которых в C-пути есть постоянный адрес (переменная/"этот"/доступ к
// полю) - тот же список, что и cbackend.receiverAddressable, но здесь, до
// backend'а: результат вызова функции/конструктора/метода сюда не входит
// (временное значение), явная ошибка Rech лучше невнятной ошибки
// компилятора C.
func receiverAddressable(e ast.Expr) bool {
	switch e.(type) {
	case *ast.Identifier, *ast.ThisExpr, *ast.FieldAccess:
		return true
	default:
		return false
	}
}

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
		if !call.HasReturn {
			return nil, fmt.Errorf("строка %d: функция %q не имеет объявленного типа результата - её нельзя использовать как выражение", e.Line, e.Callee)
		}
		return call, nil

	case *ast.NewExpr:
		return analyzeNewExpr(sc, e)

	case *ast.FieldAccess:
		// "Имя.Член" перечисления (шаг 12) - синтаксически неотличимо от
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
						if err := checkConstVisibility(sc, ident.Name, e.Field, sig.ConstPrivate[idx], e.Line); err != nil {
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
				if err := checkConstVisibility(sc, typ.Entity, e.Field, sig.ConstPrivate[cidx], e.Line); err != nil {
					return nil, err
				}
				return resolveEntityConstant(sc, typ.Entity, e.Field, e.Line)
			}
			return nil, fmt.Errorf("строка %d: у сущности %q нет поля %q", e.Line, typ.Entity, e.Field)
		}
		if err := checkFieldVisibility(sc, typ.Entity, e.Field, sig.FieldPrivate[idx], e.Line); err != nil {
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
	sig, ok := sc.entities[e.Type]
	if !ok {
		// Не пользовательская сущность и ещё не встречавшаяся generic-
		// инстанциация (шаг 4.2) - resolveTypeName при необходимости
		// синтезирует её и зарегистрирует в sc.entities/sc.methods, как и
		// resolveType в cbackend (AST-путь); если e.Type в принципе не
		// тип - вернёт понятную ошибку.
		if _, err := resolveTypeName(sc.entities, sc.methods, sc.aliases, e.Type); err != nil {
			return nil, fmt.Errorf("строка %d: %w", e.Line, err)
		}
		sig, ok = sc.entities[e.Type]
		if !ok {
			return nil, fmt.Errorf("строка %d: сущность %q не объявлена", e.Line, e.Type)
		}
	}
	args := make([]ir.Expr, len(sig.FieldNames))
	if e.FieldNames != nil {
		if len(e.FieldNames) != len(sig.FieldNames) {
			return nil, fmt.Errorf("строка %d: конструктор %q ожидает значений полей: %d, передано: %d", e.Line, e.Type, len(sig.FieldNames), len(e.FieldNames))
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
			if typ := ir.TypeOf(arg); typ != sig.FieldTypes[idx] {
				return nil, fmt.Errorf("строка %d: поле %q сущности %q должно быть типа %s, получено %s", e.Line, label, e.Type, sig.FieldTypes[idx], typ)
			}
			args[idx] = arg
		}
		return &ir.NewExpr{Entity: e.Type, Args: args}, nil
	}
	if len(e.Args) != len(sig.FieldNames) {
		return nil, fmt.Errorf("строка %d: конструктор %q ожидает значений полей: %d, передано: %d", e.Line, e.Type, len(sig.FieldNames), len(e.Args))
	}
	for i, a := range e.Args {
		arg, err := analyzeExpr(sc, a)
		if err != nil {
			return nil, err
		}
		if typ := ir.TypeOf(arg); typ != sig.FieldTypes[i] {
			return nil, fmt.Errorf("строка %d: аргумент %d конструктора %q должен быть типа %s, получен %s", e.Line, i+1, e.Type, sig.FieldTypes[i], typ)
		}
		args[i] = arg
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
	if typ.Kind != ir.KindEntity {
		return nil, fmt.Errorf("строка %d: нельзя вызвать метод %q - тип %s не сущность", e.Line, e.Method, typ)
	}
	sig, ok := sc.methods[typ.Entity][e.Method]
	if !ok {
		return nil, fmt.Errorf("строка %d: у сущности %q нет метода %q", e.Line, typ.Entity, e.Method)
	}
	if err := checkMethodVisibility(sc, typ.Entity, e.Method, sig.Private, e.Line); err != nil {
		return nil, err
	}
	if len(e.Args) != len(sig.Params) {
		return nil, fmt.Errorf("строка %d: метод %q ожидает аргументов: %d, передано: %d", e.Line, e.Method, len(sig.Params), len(e.Args))
	}
	args := make([]ir.Expr, len(e.Args))
	for i, a := range e.Args {
		arg, err := analyzeExpr(sc, a)
		if err != nil {
			return nil, err
		}
		if t := ir.TypeOf(arg); t != sig.Params[i] {
			return nil, fmt.Errorf("строка %d: аргумент %d вызова метода %q должен быть типа %s, получен %s", e.Line, i+1, e.Method, sig.Params[i], t)
		}
		if i < len(sig.ParamStates) && sig.ParamStates[i] != "" {
			required := sig.ParamStates[i]
			if !establishedStatesOf(sc, arg)[required] {
				return nil, fmt.Errorf("строка %d: аргумент %d вызова метода %q требует значение, уже прошедшее переход в состояние %q - для этого выражения такой гарантии нет", e.Line, i+1, e.Method, required)
			}
		}
		args[i] = arg
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
// HasReturn - проверяется вызывающей стороной, analyzeExpr).
func analyzeCallExpr(sc *ctx, e *ast.CallExpr) (*ir.CallExpr, error) {
	sig, ok := sc.funcs[e.Callee]
	if !ok {
		return nil, fmt.Errorf("строка %d: функция %q не объявлена", e.Line, e.Callee)
	}
	if len(e.Args) != len(sig.Params) {
		return nil, fmt.Errorf("строка %d: функция %q ожидает аргументов: %d, передано: %d", e.Line, e.Callee, len(sig.Params), len(e.Args))
	}
	args := make([]ir.Expr, len(e.Args))
	for i, a := range e.Args {
		arg, err := analyzeExpr(sc, a)
		if err != nil {
			return nil, err
		}
		if typ := ir.TypeOf(arg); typ != sig.Params[i] {
			return nil, fmt.Errorf("строка %d: аргумент %d вызова %q должен быть типа %s, получен %s", e.Line, i+1, e.Callee, sig.Params[i], typ)
		}
		if i < len(sig.ParamStates) && sig.ParamStates[i] != "" {
			required := sig.ParamStates[i]
			if !establishedStatesOf(sc, arg)[required] {
				return nil, fmt.Errorf("строка %d: аргумент %d вызова %q требует значение, уже прошедшее переход в состояние %q - для этого выражения такой гарантии нет", e.Line, i+1, e.Callee, required)
			}
		}
		args[i] = arg
	}
	return &ir.CallExpr{Callee: e.Callee, Args: args, Type: sig.ReturnType, HasReturn: sig.HasReturn}, nil
}

func analyzeUnaryExpr(sc *ctx, e *ast.UnaryExpr) (ir.Expr, error) {
	operand, err := analyzeExpr(sc, e.Operand)
	if err != nil {
		return nil, err
	}
	typ := ir.TypeOf(operand)
	switch e.Op {
	case "-":
		if typ != ir.TypeInt && typ != ir.TypeFloat {
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
	if typ != ir.TypeInt && typ != ir.TypeFloat {
		return nil, fmt.Errorf("строка %d: оператор %q применим только к Целое/Число, получено %s", e.Line, e.Op, typ)
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
		if lt != rt || (lt != ir.TypeInt && lt != ir.TypeFloat && lt != ir.TypeBool) {
			return nil, fmt.Errorf("строка %d: оператор %q не поддерживается для типов %s и %s", e.Line, e.Op, lt, rt)
		}
		return &ir.BinaryExpr{Op: e.Op, Left: left, Right: right, Type: ir.TypeBool}, nil

	case "<", ">", "<=", ">=":
		if lt != rt || (lt != ir.TypeInt && lt != ir.TypeFloat) {
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

// analyzeArithmetic - "+" (нестроковый)/"-"/"*"/"/": Целое+Целое -> Целое,
// иначе (хотя бы один операнд Число) -> Число - та же промоция, что и в
// cbackend.genArithmetic (шаг 1), только результат - тип в IR-узле, без
// самого приведения ((double)...) - приведение операнда, если нужно,
// решает backend при генерации кода конкретной платформы, глядя на Type
// каждого операнда и Type результата.
func analyzeArithmetic(e *ast.BinaryExpr, left ir.Expr, lt ir.Type, right ir.Expr, rt ir.Type) (ir.Expr, error) {
	if lt != ir.TypeInt && lt != ir.TypeFloat {
		return nil, fmt.Errorf("строка %d: оператор %q не поддерживается для типа %s", e.Line, e.Op, lt)
	}
	if rt != ir.TypeInt && rt != ir.TypeFloat {
		return nil, fmt.Errorf("строка %d: оператор %q не поддерживается для типа %s", e.Line, e.Op, rt)
	}
	result := ir.TypeInt
	if lt == ir.TypeFloat || rt == ir.TypeFloat {
		result = ir.TypeFloat
	}
	return &ir.BinaryExpr{Op: e.Op, Left: left, Right: right, Type: result}, nil
}
