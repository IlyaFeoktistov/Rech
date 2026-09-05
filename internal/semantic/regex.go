package semantic

import "github.com/IlyaFeoktistov/Rech/internal/ir"

// regexGroupsType/regexResultType - имена generic-инстанциаций, которые
// тянет за собой встроенная сущность "Совпадение"/функция "регулярное" -
// зарегистрированы лениво (resolveTypeName/registerRegexTypesOnCall), тем
// же принципом, что и splitResultType (stringsplit.go) - иначе КАЖДАЯ
// программа получала бы неиспользуемые typedef/методы обеих коллекций
// просто из факта существования этих встроенных имён в языке.
const (
	regexGroupsType = "Список<Строка>"
	regexResultType = "Список<Совпадение>"
)

// matchFields - поля встроенной сущности "Совпадение" (результат
// "регулярное(строка, шаблон): Список<Совпадение>") - в этом порядке
// (позиционный конструктор "новый Совпадение(...)" следует ему, хотя на
// практике значения всегда приходят от "регулярное", не от явного
// "новый"): "текст" - весь совпавший фрагмент, "начало" - позиция начала
// совпадения (та же индексация, что и "позиция_в_строке"), "группы" -
// захваченные скобочные группы шаблона (группа 0, весь матч, НЕ
// дублируется сюда - она уже есть в "текст"; группа без совпадения при
// необязательной части шаблона, "(...)?" - пустая строка, не null -
// упрощает код вызывающей стороны ценой неотличимости "не участвовала" от
// "совпала с пустой строкой").
var matchFields = []ir.Field{
	{Name: "текст", Type: ir.TypeString},
	{Name: "начало", Type: ir.TypeInt},
	{Name: "группы", Type: ir.EntityType(regexGroupsType)},
}

// registerRegexFunc - "регулярное(строка, шаблон): Список<Совпадение>" -
// только сигнатура функции, без регистрации самой сущности "Совпадение"/
// generic-коллекций - те лениво регистрируются при первом реальном
// упоминании (registerRegexTypesOnCall/resolveTypeName), тем же принципом,
// что и "разбить"/registerStringSplitFunc.
func registerRegexFunc(funcs map[string]funcSig) {
	funcs["регулярное"] = funcSig{Params: []ir.Type{ir.TypeString, ir.TypeString}, ReturnType: ir.EntityType(regexResultType), HasReturn: true}
}

// registerRegexTypesOnCall - вызывается из analyzeCallExpr при каждом
// вызове "регулярное" (тем же принципом, что и registerSplitTypeOnCall) -
// резолвит "Совпадение" (которая сама тянет regexGroupsType, см. её case в
// resolveTypeName) и regexResultType.
func registerRegexTypesOnCall(entities map[string]entitySig, methods map[string]map[string]funcSig, aliases map[string]string, interfaces map[string]interfaceSig) error {
	if _, err := resolveTypeName(entities, methods, aliases, interfaces, "Совпадение"); err != nil {
		return err
	}
	_, err := resolveTypeName(entities, methods, aliases, interfaces, regexResultType)
	return err
}
