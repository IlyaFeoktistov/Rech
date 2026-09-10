package semantic

import "github.com/IlyaFeoktistov/Rech/internal/ir"

// dateTimeFields - фиксированные поля встроенной сущности "ДатаВремя", в
// этом порядке (позиционный конструктор "новый ДатаВремя(...)" следует
// ему) - календарные поля (не единственный timestamp), тем же выбором,
// что и "Запрос"/"Ответ": бесплатный доступ к полю без отдельных функций-
// геттеров, ценой фиксированного представления (всегда распакован в
// год/месяц/день/..., не хранится как единое число секунд с эпохи). Сама
// "ДатаВремя" всегда несёт поля БЕЗ пояса (либо UTC - время_как_дата/
// дата_как_время, либо уже локализованные календарные поля произвольного
// пояса - время_как_дата_в_поясе/"д.в_поясе(...)" ниже) - разница между
// "какой это момент" и "в каком поясе показан" не хранится в самом
// значении, только в том, КАКАЯ функция его построила (тот же принцип,
// что у time.Time БЕЗ Location в языках, не хранящих зону в значении).
var dateTimeFields = []ir.Field{
	{Name: "год", Type: ir.TypeInt},
	{Name: "месяц", Type: ir.TypeInt},
	{Name: "день", Type: ir.TypeInt},
	{Name: "час", Type: ir.TypeInt},
	{Name: "минута", Type: ir.TypeInt},
	{Name: "секунда", Type: ir.TypeInt},
}

// registerDateTimeFuncs регистрирует сущность "ДатаВремя" и функции
// "время_как_дата"/"дата_как_время"/"дату_в_строку" - тот же принцип, что
// и у "запрос" (HTTP-клиент): они принимают/возвращают сущность, не
// примитив, значит не входят в internal/builtins.All (там только Kind
// примитивов) и регистрируются отдельно в каждом из трёх потребителей
// (semantic/cbackend/interp). "время" (текущее время, без сущности в
// сигнатуре) - в internal/builtins.All как обычно.
func registerDateTimeFuncs(entities map[string]entitySig, funcs map[string]funcSig) {
	registerHTTPEntity(entities, "ДатаВремя", dateTimeFields)
	funcs["время_как_дата"] = funcSig{Params: []ir.Type{ir.TypeInt}, ReturnType: ir.EntityType("ДатаВремя"), HasReturn: true}
	funcs["дата_как_время"] = funcSig{Params: []ir.Type{ir.EntityType("ДатаВремя")}, ReturnType: ir.TypeInt, HasReturn: true}
	funcs["дату_в_строку"] = funcSig{Params: []ir.Type{ir.EntityType("ДатаВремя")}, ReturnType: ir.TypeString, HasReturn: true}
	// "разобрать_дату"/"время_как_дата_в_поясе"/"дата_в_поясе_как_время" -
	// возвращают/принимают nullable (?ДатаВремя/?Целое), не голое Тип -
	// тем же принципом, что и остальные funcSig этого файла (не входят в
	// internal/builtins.All), но САМИ "?Тип" регистрируются ЛЕНИВО, при
	// реальном вызове (registerOptionalDateTimeOnCall, nullable.go) - "?Целое"
	// уже зарегистрирован безусловно (поле "код" встроенного "Исключение",
	// analyzer.go), для него отдельная ленивая регистрация не нужна.
	funcs["разобрать_дату"] = funcSig{Params: []ir.Type{ir.TypeString}, ReturnType: ir.NullableType(ir.EntityType("ДатаВремя")), HasReturn: true}
	funcs["время_как_дата_в_поясе"] = funcSig{Params: []ir.Type{ir.TypeInt, ir.TypeString}, ReturnType: ir.NullableType(ir.EntityType("ДатаВремя")), HasReturn: true}
	funcs["дата_в_поясе_как_время"] = funcSig{Params: []ir.Type{ir.EntityType("ДатаВремя"), ir.TypeString}, ReturnType: ir.NullableType(ir.TypeInt), HasReturn: true}
	// "день_недели"/"дату_в_читаемом_виде" - обычные функции (не nullable),
	// но также ЯДРО desugaring'а Carbon-стиля методов "ДатаВремя"
	// ("д.является_выходным()"/"д.в_читаемом_виде()", см. package doc
	// datetime_methods.go) - у самой "ДатаВремя" нет вовсе ir.Method,
	// методы существуют только как синтаксический сахар поверх свободных
	// функций этого файла.
	funcs["день_недели"] = funcSig{Params: []ir.Type{ir.EntityType("ДатаВремя")}, ReturnType: ir.TypeInt, HasReturn: true}
	funcs["дату_в_читаемом_виде"] = funcSig{Params: []ir.Type{ir.EntityType("ДатаВремя")}, ReturnType: ir.TypeString, HasReturn: true}
}

// optionalDateTimeFuncNames - функции, возвращающие "?ДатаВремя" - тем же
// принципом lookup-таблицы, что optionalStringFuncNames (nullable.go),
// проверяется в analyzeCallExpr (expr.go) при каждом реальном вызове.
var optionalDateTimeFuncNames = map[string]bool{
	"разобрать_дату":         true,
	"время_как_дата_в_поясе": true,
}

// registerOptionalDateTimeOnCall - регистрирует entitySig "?ДатаВремя"
// лениво, при первом РЕАЛЬНОМ вызове одной из optionalDateTimeFuncNames -
// тот же приём, что registerOptionalStringOnCall (nullable.go).
func registerOptionalDateTimeOnCall(entities map[string]entitySig, methods map[string]map[string]funcSig, aliases map[string]string, interfaces map[string]interfaceSig, enums map[string]map[string]int64) error {
	_, err := resolveTypeName(entities, methods, aliases, interfaces, enums, "?ДатаВремя")
	return err
}
