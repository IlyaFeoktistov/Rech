// Package cbackend транслирует ast.Program в исходник на C.
//
// Это первый и на сегодня единственный backend Rech. По архитектуре из
// docs/ROADMAP.md он не единственный на будущее: AST специально не содержит
// ничего C-специфичного, чтобы позже рядом можно было добавить interpreter
// и VM backend без изменения lexer/parser/ast.
//
// Типы (valueType) выводятся здесь же, прямо во время генерации кода -
// простой literal-based inference по docs/ROADMAP.md, шаг 1. Настоящая
// проверка типов на отдельном IR-слое появится на шаге 5 ("Semantic
// analysis"); до тех пор эта логика намеренно живёт в backend'е, а не в AST.
package cbackend

import (
	"fmt"
	"slices"
	"strconv"
	"strings"

	"github.com/IlyaFeoktistov/Rech/internal/ast"
	"github.com/IlyaFeoktistov/Rech/internal/rtmsg"
)

// entryPointName - функция с этим именем становится точкой входа программы
// (C main). Остальные имена транслитерируются в валидные C-идентификаторы.
const entryPointName = "главная"

// translit - таблица транслитерации кириллицы для имён функций и переменных.
// Не претендует на лингвистическую точность (это не морфология, см.
// docs/ROADMAP.md), задача - дать читаемый и уникальный C-идентификатор.
var translit = map[rune]string{
	'а': "a", 'б': "b", 'в': "v", 'г': "g", 'д': "d", 'е': "e", 'ё': "e",
	'ж': "zh", 'з': "z", 'и': "i", 'й': "j", 'к': "k", 'л': "l", 'м': "m",
	'н': "n", 'о': "o", 'п': "p", 'р': "r", 'с': "s", 'т': "t", 'у': "u",
	'ф': "f", 'х': "h", 'ц': "c", 'ч': "ch", 'ш': "sh", 'щ': "sch", 'ъ': "",
	'ы': "y", 'ь': "", 'э': "e", 'ю': "yu", 'я': "ya",

	// Символы generic-инстанциации ("Список<Целое>", шаг 4.2) - имя типа с
	// параметром кодируется как строка в AST (docs, "Синтаксис типа с
	// параметром") и мангируется тем же transliterate, что и обычные имена;
	// без этих записей "<"/">"/"," попали бы в C-идентификатор как есть,
	// что невалидно.
	'<': "_of_", '>': "", ',': "_",
}

func transliterate(name string) string {
	var sb strings.Builder
	for _, r := range strings.ToLower(name) {
		if lat, ok := translit[r]; ok {
			sb.WriteString(lat)
			continue
		}
		sb.WriteRune(r)
	}
	return sb.String()
}

func mangleName(name string) string {
	if name == entryPointName {
		return "main"
	}
	return "rech_" + transliterate(name)
}

func mangleVar(name string) string {
	return "v_" + transliterate(name)
}

// mangleType мангирует имя сущности в C typedef-имя. Отдельный префикс
// (rech_T_, не rech_ у функций) - в C typedef-имена и имена функций делят
// один неймспейс, разные префиксы структурно исключают коллизию сущности
// и функции с одним и тем же именем без отдельной проверки.
func mangleType(name string) string {
	return "rech_T_" + transliterate(name)
}

// mangleField мангирует имя поля сущности в имя члена C-структуры.
// Отдельный префикс от mangleVar (f_, не v_) - чисто для читаемости
// сгенерированного C, коллизии с переменными в C всё равно невозможны
// (члены структуры - собственный неймспейс).
func mangleField(name string) string {
	return "f_" + transliterate(name)
}

// mangleMethod мангирует пару (сущность, метод) в имя C-функции. Отдельный
// префикс (rech_M_, не rech_ у функций и не rech_T_ у типов) - в C
// typedef-имена и имена функций делят один неймспейс, разные префиксы
// структурно исключают коллизию метода с функцией/типом того же имени.
func mangleMethod(entity, method string) string {
	return "rech_M_" + transliterate(entity) + "_" + transliterate(method)
}

// valueTypeKind - "форма" значения: один из встроенных примитивов или
// kindEntity (конкретная сущность, имя которой несёт само значение
// valueType.entity - иначе структуре пришлось бы жить снаружи, в отдельном
// реестре, доступном отовсюду, где сравниваются типы).
type valueTypeKind int

const (
	kindUnknown valueTypeKind = iota
	kindString
	kindInt
	kindFloat
	kindBool
	kindEntity
)

// valueType - тип значения выражения, выведенный во время генерации C-кода.
// Структура (не голое число), потому что с шага 4 типов больше, чем
// фиксированный список примитивов - у сущностей есть имя, и оно должно
// сохраняться в самом значении typeType, чтобы FieldAccess/NewExpr могли
// сравнивать "тип этого значения" с "тип поля Х" без внешнего контекста.
// Сравнение (==, !=), использование как ключа map - всё продолжает
// работать, потому что структура с сравнимыми полями сравнима сама.
type valueType struct {
	kind   valueTypeKind
	entity string // валиден только при kind == kindEntity - оригинальное имя сущности
}

var (
	typeUnknown = valueType{kind: kindUnknown}
	typeString  = valueType{kind: kindString}
	typeInt     = valueType{kind: kindInt}
	typeFloat   = valueType{kind: kindFloat}
	typeBool    = valueType{kind: kindBool}
)

// entityType возвращает valueType, соответствующий значению сущности name.
func entityType(name string) valueType {
	return valueType{kind: kindEntity, entity: name}
}

// typeNames - единственное место, где ВСТРОЕННЫЕ типы Rech связаны с их
// написанием в исходнике (литералы -> вывод типа в шаге 1, аннотации
// параметра/результата в шаге 3). valueType.String() и typeFromName - две
// стороны одной и той же таблицы для примитивов; сущности (шаг 4) в неё не
// входят - имя сущности как имя типа разрешается через ctx.entities
// (typeFromNameOrEntity), а не через эту таблицу.
var typeNames = map[valueType]string{
	typeString: "Строка",
	typeInt:    "Целое",
	typeFloat:  "Число",
	typeBool:   "Логическое",
}

func (t valueType) String() string {
	if t.kind == kindEntity {
		return t.entity
	}
	if name, ok := typeNames[t]; ok {
		return name
	}
	return "неизвестный тип"
}

// typeFromName маппит имя ВСТРОЕННОГО типа Rech (как оно написано в
// аннотации параметра, поля или типа результата) на valueType - не знает
// про сущности, см. typeFromNameOrEntity.
func typeFromName(name string) (valueType, bool) {
	for t, n := range typeNames {
		if n == name {
			return t, true
		}
	}
	return typeUnknown, false
}

// genericTemplates - встроенные generic-шаблоны (шаг 4.2), каждый - функция
// синтеза C-структуры и методов для конкретной инстанциации. Курируемый
// список (как SystemVerbs/naturalCallPrepositions в остальном компиляторе),
// не механизм пользовательских generic-типов - тех в языке пока нет вообще.
var genericTemplates = map[string]func(ctx *genCtx, name string, elem valueType){
	"Список":  synthesizeList,
	"Стопка":  synthesizeStack,
	"Очередь": synthesizeQueue,
}

// parseGenericTypeName разбирает закодированную парсером строку вида
// "Список<Целое>" на (шаблон, аргумент) - обратная операция к
// parser.parseTypeName. ok == false - name не generic-инстанциация (нет
// "<...>"), тогда это либо примитив, либо неизвестное имя - решает
// вызывающий код (resolveType).
func parseGenericTypeName(name string) (template, arg string, ok bool) {
	i := strings.IndexByte(name, '<')
	if i < 0 || !strings.HasSuffix(name, ">") {
		return "", "", false
	}
	return name[:i], name[i+1 : len(name)-1], true
}

// resolveType - единая точка резолюции имени типа (примитив, сущность или
// generic-инстанциация builtin-коллекции, шаг 4.2), заменяет прямые вызовы
// typeFromName везде, где раньше типы могли быть только примитивами
// (signatureFromDecl, entitySignatureOf, genNewExpr). При первой встрече
// конкретной инстанциации ("Список<Целое>") синтезирует её C-код в
// ctx.genericsCode и регистрирует в ctx.entities/ctx.methods - дальше она
// неотличима от обычной сущности для остального backend'а (genMethodCall,
// genFieldAccess и т.д. читают ctx.entities/ctx.methods по имени, не зная,
// пользовательская это сущность или встроенная коллекция).
func resolveType(ctx *genCtx, name string) (valueType, error) {
	if t, ok := typeFromName(name); ok {
		return t, nil
	}
	if _, exists := ctx.entities[name]; exists {
		return entityType(name), nil
	}
	if base, exists := ctx.stateAliases[name]; exists {
		return entityType(base), nil
	}
	template, argName, ok := parseGenericTypeName(name)
	if !ok {
		return typeUnknown, fmt.Errorf("неизвестный тип %q", name)
	}
	synth, ok := genericTemplates[template]
	if !ok {
		return typeUnknown, fmt.Errorf("неизвестный generic-тип %q", template)
	}
	elem, err := resolveType(ctx, argName)
	if err != nil {
		return typeUnknown, fmt.Errorf("тип элемента %q в %q: %w", argName, name, err)
	}
	if elem.kind == kindEntity {
		return typeUnknown, fmt.Errorf("%q: элементы встроенных коллекций пока поддерживаются только для Строка/Целое/Число/Логическое, не для сущностей", name)
	}
	synth(ctx, name, elem)
	return entityType(name), nil
}

func (t valueType) cType() string {
	if t.kind == kindEntity {
		return mangleType(t.entity)
	}
	switch t {
	case typeString:
		return "const char *"
	case typeInt:
		return "long long"
	case typeFloat:
		return "double"
	case typeBool:
		return "bool"
	default:
		return "void *"
	}
}

// funcSig - сигнатура функции программы: типы параметров (по позиции,
// совпадающей с FunctionDecl.Params) и тип результата, если он объявлен.
// Собирается один раз в Generate до генерации тел функций - вызов функции
// может встретиться раньше её объявления в исходнике (в т.ч. взаимная
// рекурсия), поэтому проверка существования/сигнатуры не может ждать, пока
// до объявления дойдёт очередь в generateFunction.
type funcSig struct {
	ParamTypes []valueType
	ReturnType valueType
	HasReturn  bool
}

// genCtx - состояние генерации одной функции: стек областей видимости
// (шаг 2 - если/иначе/пока/для вводят вложенные блоки, как в C) и флаги
// использованных runtime-хелперов (влияют на пролог файла).
type genCtx struct {
	scopes []map[string]valueType

	// mangledScopes - параллельный scopes стек: мангл-имя переменной (см.
	// mangleVar) -> оригинальное русское имя в той же области видимости.
	// Устойчивое мангирование имён (шаг 3.5): ловит коллизию двух РАЗНЫХ
	// имён, транслитерирующихся в одно и то же C-имя, явной ошибкой Rech.
	// declare с одинаковым исходным именем в одной области уже ловится
	// через scopes отдельно - это не то же самое, что коллизия мангла.
	mangledScopes []map[string]string

	usesConcat bool
	usesStrCmp bool
	// usesPanic - нужен ли rech_panic (helper аварийного завершения
	// процесса): встроенные generic-коллекции (выход за границы/операция на
	// пустой коллекции, шаг 4.2), "утверждать" (ложное условие) и
	// целочисленное деление на ноль - все случаи используют один и тот же
	// helper.
	usesPanic bool
	// usesIntDiv - нужен ли rech_int_div (целочисленное "/" с проверкой на
	// ноль, в отличие от "/" между Число, где деление на 0.0 - IEEE 754
	// +-Inf/NaN, не ошибка, и генерируется как обычный оператор C).
	usesIntDiv bool

	// genericsCode - C-код (typedef struct + функции), синтезированный для
	// встроенных generic-коллекций (Список/Стопка/Очередь, шаг 4.2) по мере
	// того, как resolveType встречает новую конкретную инстанциацию
	// ("Список<Целое>") - с мемоизацией по ctx.entities (та же карта, что
	// и для обычных сущностей: коллизия имён исключена, "<>," не входят в
	// алфавит идентификаторов Rech). Порядок появления записей в
	// genericsCode не важен для корректности C - весь текст печатается
	// одним блоком сразу после typedef'ов пользовательских сущностей.
	genericsCode strings.Builder

	// funcs - сигнатуры всех функций программы, собранные до генерации
	// тел (см. funcSig). Не меняется во время генерации тел - только
	// читается при разборе вызовов.
	funcs map[string]funcSig

	// entities - сигнатуры всех сущностей программы (entitySig), собранные
	// до генерации тел - по тем же причинам, что и funcs.
	entities map[string]entitySig

	// methods - сигнатуры методов всех сущностей, собранные до генерации
	// тел: methods[Сущность][Метод] (шаг 4.d).
	methods map[string]map[string]funcSig

	// stateAliases - производный тип состояния (шаг 4.1, упрощённая
	// версия, "АвторизованныйПользователь") -> имя базовой сущности
	// ("Пользователь"). resolveType резолвит алиас в entityType(базовой
	// сущности) - НЕ в entityType(имени алиаса): производный тип не имеет
	// собственного C-представления, это чисто компилятивный псевдоним
	// (typestate без времени выполнения) - genNewExpr/genMethodCall/
	// genFieldAccess после resolveType видят обычный valueType базовой
	// сущности и не знают о существовании алиаса вообще. Отсюда и
	// документированное ограничение шага: значение базовой сущности можно
	// передать в параметр производного типа без прохождения "становится" -
	// valueType у них равны, различие есть только по имени в исходнике, не
	// в valueType; настоящая проверка перехода - шаг 5.
	stateAliases map[string]string

	// thisEntity - имя сущности, метод которой сейчас генерируется
	// (generateMethod), "" - вне метода. Даёт genExpr(*ast.ThisExpr) тип
	// значения "этот" без внешнего контекста.
	thisEntity string

	// returnType/hasReturnType - тип результата функции, которая сейчас
	// генерируется (сбрасывается в начале каждого generateFunction).
	// hasReturnType == false - функция без объявленного типа результата
	// (шаги 0-2), "вернуть" допустим только без значения.
	returnType    valueType
	hasReturnType bool
}

func (ctx *genCtx) pushScope() {
	ctx.scopes = append(ctx.scopes, map[string]valueType{})
	ctx.mangledScopes = append(ctx.mangledScopes, map[string]string{})
}

func (ctx *genCtx) popScope() {
	ctx.scopes = ctx.scopes[:len(ctx.scopes)-1]
	ctx.mangledScopes = ctx.mangledScopes[:len(ctx.mangledScopes)-1]
}

// declare добавляет переменную в текущую (самую внутреннюю) область
// видимости. Ошибка - если переменная с этим именем уже объявлена в этой
// же области (повторное объявление того же имени во вложенном блоке
// разрешено - затенение, как в C), или если её мангл-имя (mangleVar)
// совпадает с мангл-именем уже объявленной в этой области переменной с
// ДРУГИМ оригинальным именем (устойчивое мангирование, шаг 3.5).
func (ctx *genCtx) declare(name string, typ valueType) error {
	top := ctx.scopes[len(ctx.scopes)-1]
	if _, exists := top[name]; exists {
		return fmt.Errorf("переменная %q уже объявлена в этой области видимости", name)
	}
	mangled := mangleVar(name)
	topMangled := ctx.mangledScopes[len(ctx.mangledScopes)-1]
	if other, exists := topMangled[mangled]; exists {
		return fmt.Errorf("переменные %q и %q транслитерируются в одно и то же имя %q в C - переименуйте одну из них", other, name, mangled)
	}
	top[name] = typ
	topMangled[mangled] = name
	return nil
}

// lookup ищет переменную от самой внутренней области видимости к внешним.
func (ctx *genCtx) lookup(name string) (valueType, bool) {
	for _, scope := range slices.Backward(ctx.scopes) {
		if typ, ok := scope[name]; ok {
			return typ, true
		}
	}
	return typeUnknown, false
}

func indent(depth int) string {
	return strings.Repeat("    ", depth)
}

// Generate возвращает исходник на C, соответствующий программе prog.
func Generate(prog *ast.Program) (string, error) {
	ctx := &genCtx{funcs: map[string]funcSig{}, entities: map[string]entitySig{}, methods: map[string]map[string]funcSig{}, stateAliases: map[string]string{}}

	// funcMangled - устойчивое мангирование имён функций (шаг 3.5):
	// мангл-имя -> оригинальное русское имя. Ловит коллизию двух РАЗНЫХ
	// имён, транслитерирующихся в одно и то же C-имя (например, из-за
	// свёртки "ё"/"е" в translit), явной ошибкой Rech - а не тихим
	// переопределением символа или невнятной ошибкой компилятора C о
	// повторном определении.
	funcMangled := map[string]string{}

	// entityMangled - тот же принцип устойчивого мангирования для сущностей.
	// Отдельного реестра с funcMangled достаточно избегать явно - разные
	// префиксы (rech_T_ у типов, rech_ у функций) уже структурно исключают
	// коллизию сущности с функцией; здесь ловится коллизия двух сущностей
	// друг с другом.
	entityMangled := map[string]string{}
	seenEntities := map[string]bool{}
	var typedefs strings.Builder
	for _, ent := range prog.Entities {
		if seenEntities[ent.Name] {
			return "", fmt.Errorf("сущность %q объявлена более одного раза", ent.Name)
		}
		seenEntities[ent.Name] = true
		if other, exists := ctx.stateAliases[ent.Name]; exists {
			return "", fmt.Errorf("сущность %q: имя совпадает с производным типом состояния, уже занятым сущностью %q", ent.Name, other)
		}
		mangledType := mangleType(ent.Name)
		if other, exists := entityMangled[mangledType]; exists {
			return "", fmt.Errorf("сущности %q и %q транслитерируются в одно и то же имя %q в C - переименуйте одну из них", other, ent.Name, mangledType)
		}
		entityMangled[mangledType] = ent.Name

		sig, err := entitySignatureOf(ctx, ent)
		if err != nil {
			return "", err
		}
		if len(ent.States) > 0 {
			sig.States = map[string]bool{}
			for _, st := range ent.States {
				sig.States[st.Name] = true
			}
		}
		ctx.entities[ent.Name] = sig
		typedefs.WriteString(generateEntityStruct(ent, sig))

		// Производные типы состояний (шаг 4.1) - псевдонимы, не отдельные
		// сущности (resolveType резолвит их в entityType(ent.Name), см.
		// genCtx.stateAliases) - но имя всё равно должно быть уникальным
		// среди всех типов программы, иначе один из двух смыслов "имени X"
		// был бы молча потерян.
		for _, st := range ent.States {
			if _, exists := ctx.entities[st.TypeName]; exists {
				return "", fmt.Errorf("сущность %q: производный тип состояния %q совпадает с именем уже объявленной сущности", ent.Name, st.TypeName)
			}
			if other, exists := ctx.stateAliases[st.TypeName]; exists && other != ent.Name {
				return "", fmt.Errorf("сущность %q: производный тип состояния %q уже используется сущностью %q", ent.Name, st.TypeName, other)
			}
			ctx.stateAliases[st.TypeName] = ent.Name
		}

		// Сигнатуры методов этой сущности (шаг 4.d) - устойчивое
		// мангирование в рамках одной сущности (methodMangled): коллизия
		// между сущностями структурно исключена мангл-именем самой сущности
		// внутри mangleMethod, уже проверенным выше через entityMangled.
		ctx.methods[ent.Name] = map[string]funcSig{}
		methodMangled := map[string]string{}
		for _, m := range ent.Methods {
			if _, exists := ctx.methods[ent.Name][m.Name]; exists {
				return "", fmt.Errorf("сущность %q: метод %q объявлен более одного раза", ent.Name, m.Name)
			}
			mangledMethod := mangleMethod(ent.Name, m.Name)
			if other, exists := methodMangled[mangledMethod]; exists {
				return "", fmt.Errorf("сущность %q: методы %q и %q транслитерируются в одно и то же имя %q в C - переименуйте один из них", ent.Name, other, m.Name, mangledMethod)
			}
			methodMangled[mangledMethod] = m.Name
			msig, err := methodSignatureOf(ctx, ent, m)
			if err != nil {
				return "", err
			}
			ctx.methods[ent.Name][m.Name] = msig
		}
	}

	hasEntryPoint := false
	for _, fn := range prog.Functions {
		if fn.Name == entryPointName {
			hasEntryPoint = true
		}
		if _, exists := ctx.funcs[fn.Name]; exists {
			return "", fmt.Errorf("функция %q объявлена более одного раза", fn.Name)
		}
		if other, exists := funcMangled[mangleName(fn.Name)]; exists {
			return "", fmt.Errorf("функции %q и %q транслитерируются в одно и то же имя %q в C - переименуйте одну из них", other, fn.Name, mangleName(fn.Name))
		}
		funcMangled[mangleName(fn.Name)] = fn.Name
		sig, err := signatureOf(ctx, fn)
		if err != nil {
			return "", err
		}
		ctx.funcs[fn.Name] = sig
	}
	if !hasEntryPoint {
		return "", fmt.Errorf("в программе не найдена функция %q - она служит точкой входа", entryPointName)
	}

	// Прототипы (forward declarations) для всех функций, кроме точки входа -
	// иначе функции, вызывающие друг друга в порядке, отличном от порядка
	// объявления в исходнике (включая взаимную рекурсию), не собрались бы в
	// C: компилятору C нужно видеть сигнатуру функции до места вызова.
	var protos strings.Builder
	for _, fn := range prog.Functions {
		if fn.Name == entryPointName {
			continue
		}
		protos.WriteString(prototypeOf(fn, ctx.funcs[fn.Name]))
	}
	for _, ent := range prog.Entities {
		for _, m := range ent.Methods {
			protos.WriteString(prototypeOfMethod(ent, m, ctx.methods[ent.Name][m.Name]))
		}
	}

	var body strings.Builder
	for _, fn := range prog.Functions {
		if err := generateFunction(&body, ctx, fn); err != nil {
			return "", err
		}
	}
	for _, ent := range prog.Entities {
		for _, m := range ent.Methods {
			if err := generateMethod(&body, ctx, ent, m); err != nil {
				return "", err
			}
		}
	}

	var sb strings.Builder
	sb.WriteString("// Автоматически сгенерировано компилятором rech. Не редактировать руками.\n")
	sb.WriteString("#include <stdio.h>\n")
	sb.WriteString("#include <stdbool.h>\n")
	if ctx.usesConcat || ctx.usesStrCmp {
		sb.WriteString("#include <string.h>\n")
	}
	if ctx.usesConcat || ctx.usesPanic {
		sb.WriteString("#include <stdlib.h>\n")
	}
	sb.WriteString("\n")
	if ctx.usesConcat {
		sb.WriteString(strConcatHelper)
		sb.WriteString("\n")
	}
	if ctx.usesPanic {
		sb.WriteString(panicHelper)
		sb.WriteString("\n")
	}
	if ctx.usesIntDiv {
		sb.WriteString(intDivHelper)
		sb.WriteString("\n")
	}
	if typedefs.Len() > 0 {
		sb.WriteString(typedefs.String())
	}
	if ctx.genericsCode.Len() > 0 {
		sb.WriteString(ctx.genericsCode.String())
	}
	if protos.Len() > 0 {
		sb.WriteString(protos.String())
		sb.WriteString("\n")
	}
	sb.WriteString(body.String())
	return sb.String(), nil
}

// signatureFromDecl выводит funcSig из аннотаций типов параметров/типа
// результата - общая логика для функций (signatureOf) и методов
// (methodSignatureOf, шаг 4.d): у них одна и та же форма сигнатуры, разница
// только в том, откуда берутся Params/ReturnType (FunctionDecl или
// MethodDecl - разные Go-типы без общего интерфейса, поэтому берём здесь
// уже распакованные срезы/строку, а не сам узел AST). label - для текста
// ошибки (имя функции или "Сущность.метод").
func signatureFromDecl(ctx *genCtx, label string, params []ast.Param, returnType string) (funcSig, error) {
	paramTypes := make([]valueType, len(params))
	for i, param := range params {
		typ, err := resolveType(ctx, param.Type)
		if err != nil {
			return funcSig{}, fmt.Errorf("%s: параметр %q имеет неизвестный тип %q: %w", label, param.Name, param.Type, err)
		}
		paramTypes[i] = typ
	}
	sig := funcSig{ParamTypes: paramTypes}
	if returnType != "" {
		typ, err := resolveType(ctx, returnType)
		if err != nil {
			return funcSig{}, fmt.Errorf("%s: неизвестный тип результата %q: %w", label, returnType, err)
		}
		sig.ReturnType = typ
		sig.HasReturn = true
	}
	return sig, nil
}

// signatureOf выводит funcSig из аннотаций типов в объявлении функции.
func signatureOf(ctx *genCtx, fn *ast.FunctionDecl) (funcSig, error) {
	return signatureFromDecl(ctx, fmt.Sprintf("функция %q", fn.Name), fn.Params, fn.ReturnType)
}

// methodSignatureOf - то же самое для метода сущности (шаг 4.d).
func methodSignatureOf(ctx *genCtx, ent *ast.EntityDecl, m *ast.MethodDecl) (funcSig, error) {
	return signatureFromDecl(ctx, fmt.Sprintf("сущность %q: метод %q", ent.Name, m.Name), m.Params, m.ReturnType)
}

// combineParamList добавляет extraParam (скрытый первый параметр this_ у
// метода, шаг 4.d) перед остальными параметрами paramList (уже собранными
// cParamList). extraParam == "" - обычная функция, без изменений.
func combineParamList(extraParam, paramList string) string {
	if extraParam == "" {
		return paramList
	}
	if paramList == "void" {
		return extraParam
	}
	return extraParam + ", " + paramList
}

// prototypeOf формирует C forward declaration для fn.
func prototypeOf(fn *ast.FunctionDecl, sig funcSig) string {
	cReturnType := "int"
	if sig.HasReturn {
		cReturnType = sig.ReturnType.cType()
	}
	return fmt.Sprintf("%s %s(%s);\n", cReturnType, mangleName(fn.Name), cParamList(fn.Params, sig.ParamTypes))
}

// prototypeOfMethod - то же самое для метода: C-сигнатура получает
// дополнительный первый параметр "Тип *this_" (mangleType(ent.Name)).
func prototypeOfMethod(ent *ast.EntityDecl, m *ast.MethodDecl, sig funcSig) string {
	cReturnType := "int"
	if sig.HasReturn {
		cReturnType = sig.ReturnType.cType()
	}
	extraParam := fmt.Sprintf("%s *this_", mangleType(ent.Name))
	paramList := combineParamList(extraParam, cParamList(m.Params, sig.ParamTypes))
	return fmt.Sprintf("%s %s(%s);\n", cReturnType, mangleMethod(ent.Name, m.Name), paramList)
}

// entitySig - сигнатура сущности: поля в порядке объявления (для
// конструктора и доступа к полю, шаг 4.b) с их типами.
//
// ElemType/IsList заполнены только для синтезированной инстанциации
// "Список<T>" (шаг 4.2) - нулевые у обычных сущностей и у Стопки/Очереди.
// Список - единственная из встроенных коллекций, которую можно перебрать
// циклом "для x в список" (generateStmt, *ast.ForStmt), поэтому именно ему
// нужно нести тип элемента там, где это увидит кодогенерация цикла.
type entitySig struct {
	FieldNames []string
	FieldTypes []valueType

	IsList   bool
	ElemType valueType

	// States - краткие формы, объявленные под "состояние:" (шаг 4.1,
	// упрощённая версия) - используется только для проверки BecomeStmt
	// ("получатель становится <состояние>" - состояние обязано быть
	// объявлено у этой сущности). nil у сущностей без "состояние:".
	States map[string]bool
}

// fieldIndex возвращает индекс поля name в sig, или -1, если такого поля
// нет. Общий хелпер для FieldAccess (шаг 4.b) и FieldAssign (шаг 4.c).
func (sig entitySig) fieldIndex(name string) int {
	for i, n := range sig.FieldNames {
		if n == name {
			return i
		}
	}
	return -1
}

// entitySignatureOf выводит entitySig из аннотаций типов полей ent и заодно
// проверяет устойчивость мангирования имён полей (шаг 3.5, тот же принцип,
// что и для функций/переменных/сущностей) - здесь, а не в
// generateEntityStruct, чтобы сигнатура (нужна и NewExpr, и FieldAccess) не
// зависела от того, генерируется ли typedef прямо сейчас.
func entitySignatureOf(ctx *genCtx, ent *ast.EntityDecl) (entitySig, error) {
	fieldMangled := map[string]string{}
	var sig entitySig
	for _, f := range ent.Fields {
		typ, err := resolveType(ctx, f.Type)
		if err != nil {
			return entitySig{}, fmt.Errorf("сущность %q: поле %q имеет неизвестный тип %q: %w", ent.Name, f.Name, f.Type, err)
		}
		mangled := mangleField(f.Name)
		if other, exists := fieldMangled[mangled]; exists {
			return entitySig{}, fmt.Errorf("сущность %q: поля %q и %q транслитерируются в одно и то же имя %q в C - переименуйте одно из них", ent.Name, other, f.Name, mangled)
		}
		fieldMangled[mangled] = f.Name
		sig.FieldNames = append(sig.FieldNames, f.Name)
		sig.FieldTypes = append(sig.FieldTypes, typ)
	}
	return sig, nil
}

// generateEntityStruct возвращает C typedef struct для ent по уже
// посчитанной sig (entitySignatureOf).
func generateEntityStruct(ent *ast.EntityDecl, sig entitySig) string {
	var sb strings.Builder
	sb.WriteString("typedef struct {\n")
	if len(sig.FieldNames) == 0 {
		// В C нет пустых структур - техническое поле-заглушка, пока у
		// сущности ещё не появилось ни одного поля.
		sb.WriteString("    char _rech_empty;\n")
	}
	for i, name := range sig.FieldNames {
		fmt.Fprintf(&sb, "    %s %s;\n", sig.FieldTypes[i].cType(), mangleField(name))
	}
	fmt.Fprintf(&sb, "} %s;\n\n", mangleType(ent.Name))
	return sb.String()
}

const strConcatHelper = `static char *rech_str_concat(const char *a, const char *b) {
    size_t la = strlen(a), lb = strlen(b);
    char *out = malloc(la + lb + 1);
    memcpy(out, a, la);
    memcpy(out + la, b, lb);
    out[la + lb] = '\0';
    return out;
}
`

// panicHelper - аварийное завершение процесса с понятным сообщением (шаг
// 4.2, встроенные коллекции): снять/извлечь с пустой Стопки/Очереди,
// индекс вне границ у Списка. В языке ещё нет исключений (шаг 14) - это
// осознанно временное, но безопасное поведение (аналог паники в Go/Rust),
// которое естественно заменится на выброс исключения, когда шаг 14
// появится - вызывающий Rech-код уже сейчас не может ни поймать, ни
// проигнорировать эту ошибку, так что замена не будет ломать поведение
// корректных программ.
var panicHelper = fmt.Sprintf(`static void rech_panic(const char *msg) {
    fprintf(stderr, "%s%%s\n", msg);
    exit(1);
}
`, rtmsg.PanicPrefix)

// intDivHelper - целочисленное "/" с проверкой делителя на ноль (без него
// это UB в C - обычно SIGFPE, платформенно-зависимое поведение, а не
// понятная ошибка) - требует rech_panic (usesPanic ставится вместе с
// usesIntDiv, см. вызывающий код).
var intDivHelper = fmt.Sprintf(`static long long rech_int_div(long long a, long long b) {
    if (b == 0) {
        rech_panic("%s");
    }
    return a / b;
}
`, rtmsg.DivisionByZero)

// synthesizeList синтезирует C typedef struct + функции для конкретной
// инстанциации "Список<T>" (name, например "Список<Целое>") и регистрирует
// её в ctx.entities/ctx.methods - дальше genMethodCall/genFieldAccess и
// цикл "для x в список" (шаг 4.2.b) обращаются с ней как с обычной
// сущностью. Внутреннее представление - growable array (data/len/cap,
// realloc с удвоением ёмкости) - деталь реализации, не видна в Rech: у
// сущности нет доступных из Rech полей (entitySig.FieldNames пустой),
// только методы.
func synthesizeList(ctx *genCtx, name string, elem valueType) {
	ctx.usesPanic = true
	ctx.genericsCode.WriteString(listCollectionCCode(name, elem.cType()))
	ctx.entities[name] = entitySig{IsList: true, ElemType: elem}
	ctx.methods[name] = map[string]funcSig{
		"добавить":   {ParamTypes: []valueType{elem}},
		"получить":   {ParamTypes: []valueType{typeInt}, ReturnType: elem, HasReturn: true},
		"установить": {ParamTypes: []valueType{typeInt, elem}},
		"длина":      {ReturnType: typeInt, HasReturn: true},
		"пусто":      {ReturnType: typeBool, HasReturn: true},
	}
}

// listCollectionCCode строит C-текст (typedef struct + функции) для
// конкретной инстанциации "Список<T>" по одному только имени и C-типу
// элемента - без зависимости от genCtx/valueType, чтобы его можно было
// использовать и из IR-пути (cbackend.GenerateFromIR, шаг 5.c), где нет
// genCtx вообще - там сигнатуры методов уже целиком в самих IR-узлах
// (MethodCallExpr), этому коду для кодогенерации нужен только сам C-текст.
func listCollectionCCode(name, elemC string) string {
	structName := mangleType(name)
	add := mangleMethod(name, "добавить")
	get := mangleMethod(name, "получить")
	set := mangleMethod(name, "установить")
	length := mangleMethod(name, "длина")
	empty := mangleMethod(name, "пусто")
	outOfRange := escapeCString(rtmsg.IndexOutOfRange(name))

	return fmt.Sprintf(`typedef struct {
    %[1]s *data;
    long long len;
    long long cap;
} %[2]s;

static void %[3]s(%[2]s *this_, %[1]s value) {
    if (this_->len == this_->cap) {
        this_->cap = this_->cap == 0 ? 4 : this_->cap * 2;
        this_->data = realloc(this_->data, sizeof(%[1]s) * this_->cap);
    }
    this_->data[this_->len] = value;
    this_->len++;
}

static %[1]s %[4]s(%[2]s *this_, long long index) {
    if (index < 0 || index >= this_->len) {
        rech_panic("%[7]s");
    }
    return this_->data[index];
}

static void %[5]s(%[2]s *this_, long long index, %[1]s value) {
    if (index < 0 || index >= this_->len) {
        rech_panic("%[7]s");
    }
    this_->data[index] = value;
}

static long long %[6]s(%[2]s *this_) {
    return this_->len;
}

static bool %[8]s(%[2]s *this_) {
    return this_->len == 0;
}

`, elemC, structName, add, get, set, length, outOfRange, empty)
}

// synthesizeStack синтезирует C typedef struct + функции для конкретной
// инстанциации "Стопка<T>" (шаг 4.2.c) - тот же growable array, что и у
// synthesizeList (data/len/cap, realloc с удвоением ёмкости), но без
// доступа по индексу: только с конца (LIFO). IsList у entitySig остаётся
// false (нулевое значение) - в отличие от Список<T>, Стопка<T> не
// участвует в цикле "для x в списке" (шаг 4.2.b), это ей и не нужно.
func synthesizeStack(ctx *genCtx, name string, elem valueType) {
	ctx.usesPanic = true
	ctx.genericsCode.WriteString(stackCollectionCCode(name, elem.cType()))
	ctx.entities[name] = entitySig{}
	ctx.methods[name] = map[string]funcSig{
		"положить": {ParamTypes: []valueType{elem}},
		"снять":    {ReturnType: elem, HasReturn: true},
		"верх":     {ReturnType: elem, HasReturn: true},
		"длина":    {ReturnType: typeInt, HasReturn: true},
		"пусто":    {ReturnType: typeBool, HasReturn: true},
	}
}

// stackCollectionCCode - как listCollectionCCode, для "Стопка<T>" (LIFO).
func stackCollectionCCode(name, elemC string) string {
	structName := mangleType(name)
	push := mangleMethod(name, "положить")
	pop := mangleMethod(name, "снять")
	top := mangleMethod(name, "верх")
	length := mangleMethod(name, "длина")
	empty := mangleMethod(name, "пусто")
	onEmpty := escapeCString(rtmsg.EmptyStackOp(name))

	return fmt.Sprintf(`typedef struct {
    %[1]s *data;
    long long len;
    long long cap;
} %[2]s;

static void %[3]s(%[2]s *this_, %[1]s value) {
    if (this_->len == this_->cap) {
        this_->cap = this_->cap == 0 ? 4 : this_->cap * 2;
        this_->data = realloc(this_->data, sizeof(%[1]s) * this_->cap);
    }
    this_->data[this_->len] = value;
    this_->len++;
}

static %[1]s %[4]s(%[2]s *this_) {
    if (this_->len == 0) {
        rech_panic("%[7]s");
    }
    this_->len--;
    return this_->data[this_->len];
}

static %[1]s %[5]s(%[2]s *this_) {
    if (this_->len == 0) {
        rech_panic("%[7]s");
    }
    return this_->data[this_->len - 1];
}

static long long %[6]s(%[2]s *this_) {
    return this_->len;
}

static bool %[8]s(%[2]s *this_) {
    return this_->len == 0;
}

`, elemC, structName, push, pop, top, length, onEmpty, empty)
}

// synthesizeQueue синтезирует C typedef struct + функции для конкретной
// инстанциации "Очередь<T>" (шаг 4.2.d) - тот же growable array, что и у
// synthesizeList/synthesizeStack (data/len/cap, realloc с удвоением
// ёмкости), но FIFO: "добавить" - в конец (как у Список<T>), "извлечь" -
// снимает и возвращает с начала. Снятие с начала сдвигает остальные
// элементы на один влево простым циклом (O(len)) - осознанно просто, не
// кольцевой буфер: у встроенных коллекций этого шага нет цели быть
// эффективными, только корректными (как и линейный поиск был бы, если бы
// он тут был). Как и у Стопки, IsList не выставлен - не участвует в цикле
// "для x в списке" (только Список<T>, шаг 4.2.b).
func synthesizeQueue(ctx *genCtx, name string, elem valueType) {
	ctx.usesPanic = true
	ctx.genericsCode.WriteString(queueCollectionCCode(name, elem.cType()))
	ctx.entities[name] = entitySig{}
	ctx.methods[name] = map[string]funcSig{
		"добавить": {ParamTypes: []valueType{elem}},
		"извлечь":  {ReturnType: elem, HasReturn: true},
		"первый":   {ReturnType: elem, HasReturn: true},
		"длина":    {ReturnType: typeInt, HasReturn: true},
		"пусто":    {ReturnType: typeBool, HasReturn: true},
	}
}

// queueCollectionCCode - как listCollectionCCode, для "Очередь<T>" (FIFO).
func queueCollectionCCode(name, elemC string) string {
	structName := mangleType(name)
	add := mangleMethod(name, "добавить")
	remove := mangleMethod(name, "извлечь")
	first := mangleMethod(name, "первый")
	length := mangleMethod(name, "длина")
	empty := mangleMethod(name, "пусто")
	onEmpty := escapeCString(rtmsg.EmptyQueueOp(name))

	return fmt.Sprintf(`typedef struct {
    %[1]s *data;
    long long len;
    long long cap;
} %[2]s;

static void %[3]s(%[2]s *this_, %[1]s value) {
    if (this_->len == this_->cap) {
        this_->cap = this_->cap == 0 ? 4 : this_->cap * 2;
        this_->data = realloc(this_->data, sizeof(%[1]s) * this_->cap);
    }
    this_->data[this_->len] = value;
    this_->len++;
}

static %[1]s %[4]s(%[2]s *this_) {
    if (this_->len == 0) {
        rech_panic("%[7]s");
    }
    %[1]s value = this_->data[0];
    for (long long i = 1; i < this_->len; i++) {
        this_->data[i - 1] = this_->data[i];
    }
    this_->len--;
    return value;
}

static %[1]s %[5]s(%[2]s *this_) {
    if (this_->len == 0) {
        rech_panic("%[7]s");
    }
    return this_->data[0];
}

static long long %[6]s(%[2]s *this_) {
    return this_->len;
}

static bool %[8]s(%[2]s *this_) {
    return this_->len == 0;
}

`, elemC, structName, add, remove, first, length, onEmpty, empty)
}

func generateFunction(sb *strings.Builder, ctx *genCtx, fn *ast.FunctionDecl) error {
	sig := ctx.funcs[fn.Name] // уже посчитана в Generate (signatureOf)
	fmt.Fprintf(sb, "// %s\n", fn.Name)
	errLabel := fmt.Sprintf("функция %q", fn.Name)
	return generateCallable(sb, ctx, mangleName(fn.Name), "", fn.Params, sig, fn.Body, errLabel)
}

// generateMethod генерирует C-функцию для метода сущности (шаг 4.d) -
// как generateFunction, но с дополнительным первым C-параметром
// "Тип *this_" (получатель - по указателю, чтобы мутация поля внутри
// метода была видна вызывающему коду после возврата, docs/GRAMMAR.md) и с
// ctx.thisEntity, включённым на время генерации тела - это то, что делает
// "этот" внутри метода валидным выражением (genExpr, case *ast.ThisExpr).
func generateMethod(sb *strings.Builder, ctx *genCtx, ent *ast.EntityDecl, m *ast.MethodDecl) error {
	sig := ctx.methods[ent.Name][m.Name] // уже посчитана в Generate (methodSignatureOf)
	fmt.Fprintf(sb, "// %s.%s\n", ent.Name, m.Name)
	extraParam := fmt.Sprintf("%s *this_", mangleType(ent.Name))
	errLabel := fmt.Sprintf("сущность %q: метод %q", ent.Name, m.Name)

	ctx.thisEntity = ent.Name
	defer func() { ctx.thisEntity = "" }()
	return generateCallable(sb, ctx, mangleMethod(ent.Name, m.Name), extraParam, m.Params, sig, m.Body, errLabel)
}

// generateCallable - общая генерация C-функции для функции и метода:
// пролог (сигнатура), проверка "заканчивается вернуть <значение>" у
// типизированного результата, тело, эпилог. extraParam - дополнительный
// первый C-параметр метода (this_) или "" для обычной функции.
func generateCallable(sb *strings.Builder, ctx *genCtx, cName string, extraParam string, params []ast.Param, sig funcSig, body []ast.Stmt, errLabel string) error {
	ctx.scopes = nil
	ctx.mangledScopes = nil
	ctx.pushScope()

	for i, param := range params {
		// Свежая область видимости, а имена параметров - падежи, уникальность
		// которых уже проверил парсер (parseParams) - declare не может
		// вернуть ошибку здесь.
		ctx.declare(param.Name, sig.ParamTypes[i])
	}

	ctx.returnType = sig.ReturnType
	ctx.hasReturnType = sig.HasReturn
	cReturnType := "int"
	if ctx.hasReturnType {
		cReturnType = sig.ReturnType.cType()
		if !endsWithReturnValue(body) {
			return fmt.Errorf("%s: с объявленным типом результата %s должна заканчиваться \"вернуть <значение>\"", errLabel, sig.ReturnType)
		}
	}

	fmt.Fprintf(sb, "%s %s(%s) {\n", cReturnType, cName, combineParamList(extraParam, cParamList(params, sig.ParamTypes)))
	for _, stmt := range body {
		if err := generateStmt(sb, ctx, stmt, 1); err != nil {
			return fmt.Errorf("%s: %w", errLabel, err)
		}
	}
	if !ctx.hasReturnType {
		// Функция/метод без объявленного типа результата, поэтому 0 в конце -
		// не значение "результата", а просто способ не оставлять int-функцию
		// без return (что для строгого C - undefined behavior).
		sb.WriteString("    return 0;\n")
	}
	sb.WriteString("}\n\n")
	return nil
}

// endsWithReturnValue - true, если последняя инструкция тела функции
// (верхнего уровня, не внутри вложенных если/пока/для) - "вернуть
// <значение>". Это простая синтаксическая проверка, а не flow-sensitive
// анализ всех путей выполнения (это - шаг 5, docs/ROADMAP.md): она не ловит,
// например, функцию, которая заканчивается "если ... { вернуть X } иначе {
// вернуть Y }" - её тоже придётся переписать, пока не появится настоящий
// анализ.
func endsWithReturnValue(body []ast.Stmt) bool {
	if len(body) == 0 {
		return false
	}
	last, ok := body[len(body)-1].(*ast.ReturnStmt)
	return ok && last.Value != nil
}

// cParamList формирует список параметров C-функции. types[i] - тип,
// выведенный из аннотации params[i].Type (typeFromName), в том же порядке.
func cParamList(params []ast.Param, types []valueType) string {
	if len(params) == 0 {
		return "void"
	}
	parts := make([]string, len(params))
	for i, param := range params {
		parts[i] = fmt.Sprintf("%s %s", types[i].cType(), mangleVar(param.Name))
	}
	return strings.Join(parts, ", ")
}

func generateStmt(sb *strings.Builder, ctx *genCtx, stmt ast.Stmt, depth int) error {
	ind := indent(depth)
	switch s := stmt.(type) {
	case *ast.PrintStmt:
		code, typ, err := genExpr(ctx, s.Value)
		if err != nil {
			return err
		}
		fmt.Fprintf(sb, "%s%s\n", ind, printCall(typ, code))
		return nil

	case *ast.VarDecl:
		code, typ, err := genExpr(ctx, s.Value)
		if err != nil {
			return err
		}
		if err := ctx.declare(s.Name, typ); err != nil {
			return fmt.Errorf("строка %d: %w", s.Line, err)
		}
		fmt.Fprintf(sb, "%s%s %s = %s;\n", ind, typ.cType(), mangleVar(s.Name), code)
		return nil

	case *ast.Assign:
		declared, exists := ctx.lookup(s.Name)
		if !exists {
			return fmt.Errorf("строка %d: переменная %q не объявлена - используйте \"переменная\" для объявления", s.Line, s.Name)
		}
		code, typ, err := genExpr(ctx, s.Value)
		if err != nil {
			return err
		}
		if typ != declared {
			return fmt.Errorf("строка %d: переменной %q типа %s нельзя присвоить значение типа %s", s.Line, s.Name, declared, typ)
		}
		fmt.Fprintf(sb, "%s%s = %s;\n", ind, mangleVar(s.Name), code)
		return nil

	case *ast.FieldAssign:
		return generateFieldAssign(sb, ctx, s, ind)

	case *ast.BecomeStmt:
		_, receiverType, err := genExpr(ctx, s.Receiver)
		if err != nil {
			return err
		}
		if receiverType.kind != kindEntity {
			return fmt.Errorf("строка %d: \"становится\" применимо только к сущности, получен тип %s", s.Line, receiverType)
		}
		sig := ctx.entities[receiverType.entity]
		if !sig.States[s.State] {
			return fmt.Errorf("строка %d: у сущности %q не объявлено состояние %q", s.Line, receiverType.entity, s.State)
		}
		// Шаг 4.1, упрощённая версия: у перехода состояния нет отдельного
		// runtime-представления (typestate - гарантия компилятора, не флаг
		// в структуре) - проверка только структурная (состояние объявлено),
		// без потока выполнения (полная проверка - шаг 5). Поэтому в C-код
		// эта инструкция ничего не эмитит.
		return nil

	case *ast.AssertStmt:
		cond, typ, err := genExpr(ctx, s.Cond)
		if err != nil {
			return err
		}
		if typ != typeBool {
			return fmt.Errorf("строка %d: условие \"утверждать\" должно быть логическим, получено %s", s.Line, typ)
		}
		ctx.usesPanic = true
		fmt.Fprintf(sb, "%sif (!(%s)) { rech_panic(\"%s\"); }\n", ind, cond, escapeCString(rtmsg.AssertFailed(s.Line)))
		return nil

	case *ast.IfStmt:
		cond, typ, err := genExpr(ctx, s.Cond)
		if err != nil {
			return err
		}
		if typ != typeBool {
			return fmt.Errorf("строка %d: условие \"если\" должно быть логическим, получено %s", s.Line, typ)
		}
		fmt.Fprintf(sb, "%sif (%s) {\n", ind, cond)
		ctx.pushScope()
		for _, st := range s.Then {
			if err := generateStmt(sb, ctx, st, depth+1); err != nil {
				return err
			}
		}
		ctx.popScope()
		if s.Else != nil {
			fmt.Fprintf(sb, "%s} else {\n", ind)
			ctx.pushScope()
			for _, st := range s.Else {
				if err := generateStmt(sb, ctx, st, depth+1); err != nil {
					return err
				}
			}
			ctx.popScope()
		}
		fmt.Fprintf(sb, "%s}\n", ind)
		return nil

	case *ast.WhileStmt:
		cond, typ, err := genExpr(ctx, s.Cond)
		if err != nil {
			return err
		}
		if typ != typeBool {
			return fmt.Errorf("строка %d: условие \"пока\" должно быть логическим, получено %s", s.Line, typ)
		}
		fmt.Fprintf(sb, "%swhile (%s) {\n", ind, cond)
		ctx.pushScope()
		for _, st := range s.Body {
			if err := generateStmt(sb, ctx, st, depth+1); err != nil {
				return err
			}
		}
		ctx.popScope()
		fmt.Fprintf(sb, "%s}\n", ind)
		return nil

	case *ast.ForStmt:
		fromCode, fromType, err := genExpr(ctx, s.From)
		if err != nil {
			return err
		}
		if fromType != typeInt {
			return fmt.Errorf("строка %d: граница диапазона после \"от\" должна быть целым числом, получено %s", s.Line, fromType)
		}
		toCode, toType, err := genExpr(ctx, s.To)
		if err != nil {
			return err
		}
		if toType != typeInt {
			return fmt.Errorf("строка %d: граница диапазона после \"до\" должна быть целым числом, получено %s", s.Line, toType)
		}
		ctx.pushScope()
		ctx.declare(s.Var, typeInt) // область свежая - имя точно свободно
		varC := mangleVar(s.Var)
		fmt.Fprintf(sb, "%sfor (long long %s = %s; %s <= %s; %s++) {\n", ind, varC, fromCode, varC, toCode, varC)
		for _, st := range s.Body {
			if err := generateStmt(sb, ctx, st, depth+1); err != nil {
				return err
			}
		}
		ctx.popScope()
		fmt.Fprintf(sb, "%s}\n", ind)
		return nil

	case *ast.ForEachStmt:
		collCode, collType, err := genExpr(ctx, s.Collection)
		if err != nil {
			return err
		}
		sig, ok := ctx.entities[collType.entity]
		if collType.kind != kindEntity || !ok || !sig.IsList {
			return fmt.Errorf("строка %d: цикл \"для %s в ...\" ожидает переменную-Список<T>, получен тип %s", s.Line, s.Var, collType)
		}
		ctx.pushScope()
		if err := ctx.declare(s.Var, sig.ElemType); err != nil {
			return err
		}
		varC := mangleVar(s.Var)
		idxC := "rech_fe_i_" + varC
		fmt.Fprintf(sb, "%sfor (long long %s = 0; %s < %s.len; %s++) {\n", ind, idxC, idxC, collCode, idxC)
		fmt.Fprintf(sb, "%s%s %s = %s.data[%s];\n", indent(depth+1), sig.ElemType.cType(), varC, collCode, idxC)
		for _, st := range s.Body {
			if err := generateStmt(sb, ctx, st, depth+1); err != nil {
				return err
			}
		}
		ctx.popScope()
		fmt.Fprintf(sb, "%s}\n", ind)
		return nil

	case *ast.ReturnStmt:
		if s.Value == nil {
			if ctx.hasReturnType {
				return fmt.Errorf("строка %d: функция с объявленным типом результата должна возвращать значение (\"вернуть <значение>\")", s.Line)
			}
			fmt.Fprintf(sb, "%sreturn 0;\n", ind)
			return nil
		}
		if !ctx.hasReturnType {
			return fmt.Errorf("строка %d: у функции не объявлен тип результата - используйте \"вернуть\" без значения", s.Line)
		}
		code, typ, err := genExpr(ctx, s.Value)
		if err != nil {
			return err
		}
		if typ != ctx.returnType {
			return fmt.Errorf("строка %d: функция объявлена с типом результата %s, но возвращается значение типа %s", s.Line, ctx.returnType, typ)
		}
		fmt.Fprintf(sb, "%sreturn %s;\n", ind, code)
		return nil

	case *ast.ExprStmt:
		var code string
		var err error
		switch {
		case s.Call != nil:
			code, _, err = genCall(ctx, s.Call)
		case s.MethodCall != nil:
			code, _, err = genMethodCall(ctx, s.MethodCall)
		default:
			code, _, err = genIncDecExpr(ctx, s.IncDec)
		}
		if err != nil {
			return err
		}
		fmt.Fprintf(sb, "%s%s;\n", ind, code)
		return nil

	default:
		return fmt.Errorf("строка %d: инструкция %T пока не поддерживается C backend'ом", lineOf(stmt), stmt)
	}
}

func printCall(typ valueType, code string) string {
	switch typ {
	case typeString:
		return fmt.Sprintf(`printf("%%s\n", %s);`, code)
	case typeInt:
		return fmt.Sprintf(`printf("%%lld\n", %s);`, code)
	case typeFloat:
		return fmt.Sprintf(`printf("%%g\n", %s);`, code)
	case typeBool:
		return fmt.Sprintf(`printf("%%s\n", (%s) ? "истина" : "ложь");`, code)
	default:
		return fmt.Sprintf(`printf("%%s\n", %s);`, code)
	}
}

// genExpr генерирует C-выражение для expr и возвращает вместе с ним
// выведенный тип значения.
func genExpr(ctx *genCtx, expr ast.Expr) (string, valueType, error) {
	switch e := expr.(type) {
	case *ast.StringLiteral:
		return fmt.Sprintf(`"%s"`, escapeCString(e.Value)), typeString, nil

	case *ast.IntLiteral:
		return fmt.Sprintf("%dLL", e.Value), typeInt, nil

	case *ast.FloatLiteral:
		return formatFloatLiteral(e.Value), typeFloat, nil

	case *ast.BoolLiteral:
		if e.Value {
			return "true", typeBool, nil
		}
		return "false", typeBool, nil

	case *ast.Identifier:
		typ, ok := ctx.lookup(e.Name)
		if !ok {
			return "", typeUnknown, fmt.Errorf("строка %d: переменная %q не объявлена", e.Line, e.Name)
		}
		return mangleVar(e.Name), typ, nil

	case *ast.IncDecExpr:
		return genIncDecExpr(ctx, e)

	case *ast.UnaryExpr:
		return genUnaryExpr(ctx, e)

	case *ast.BinaryExpr:
		return genBinaryExpr(ctx, e)

	case *ast.CallExpr:
		code, sig, err := genCall(ctx, e)
		if err != nil {
			return "", typeUnknown, err
		}
		if !sig.HasReturn {
			return "", typeUnknown, fmt.Errorf("строка %d: функция %q не имеет объявленного типа результата - её нельзя использовать как выражение", e.Line, e.Callee)
		}
		return code, sig.ReturnType, nil

	case *ast.NewExpr:
		return genNewExpr(ctx, e)

	case *ast.FieldAccess:
		return genFieldAccess(ctx, e)

	case *ast.ThisExpr:
		// "(*this_)" - разыменованный указатель this_ ведёт себя как
		// lvalue сущностного типа, поэтому genFieldAccess/generateFieldAssign
		// работают с ним через тот же "." (без отдельной ветки на "->") -
		// парсер уже гарантирует, что ThisExpr встречается только внутри
		// тела метода (p.inMethodBody), так что ctx.thisEntity здесь всегда
		// заполнен.
		return "(*this_)", entityType(ctx.thisEntity), nil

	case *ast.MethodCallExpr:
		code, sig, err := genMethodCall(ctx, e)
		if err != nil {
			return "", typeUnknown, err
		}
		if !sig.HasReturn {
			return "", typeUnknown, fmt.Errorf("строка %d: метод %q не имеет объявленного типа результата - его нельзя использовать как выражение", e.Line, e.Method)
		}
		return code, sig.ReturnType, nil

	default:
		return "", typeUnknown, fmt.Errorf("строка %d: выражение %T пока не поддерживается C backend'ом", lineOf(expr), expr)
	}
}

// genCall генерирует C-код вызова функции (общий для CallExpr-выражения и
// ExprStmt-инструкции) и возвращает вместе с ним сигнатуру вызванной
// функции - у выражения и инструкции разные требования к HasReturn
// (выражению нужно значение, инструкция вправе его отбросить), поэтому это
// решает уже вызывающая сторона, а не genCall.
func genCall(ctx *genCtx, e *ast.CallExpr) (string, funcSig, error) {
	sig, ok := ctx.funcs[e.Callee]
	if !ok {
		return "", funcSig{}, fmt.Errorf("строка %d: функция %q не объявлена", e.Line, e.Callee)
	}
	if len(e.Args) != len(sig.ParamTypes) {
		return "", funcSig{}, fmt.Errorf("строка %d: функция %q ожидает аргументов: %d, передано: %d", e.Line, e.Callee, len(sig.ParamTypes), len(e.Args))
	}
	argCodes := make([]string, len(e.Args))
	for i, arg := range e.Args {
		code, typ, err := genExpr(ctx, arg)
		if err != nil {
			return "", funcSig{}, err
		}
		if typ != sig.ParamTypes[i] {
			return "", funcSig{}, fmt.Errorf("строка %d: аргумент %d вызова %q должен быть типа %s, получен %s", e.Line, i+1, e.Callee, sig.ParamTypes[i], typ)
		}
		argCodes[i] = code
	}
	return fmt.Sprintf("%s(%s)", mangleName(e.Callee), strings.Join(argCodes, ", ")), sig, nil
}

// receiverAddressable - получатели, которые в сгенерированном C - lvalue:
// и для взятия адреса (передача в метод указателем this_), и для
// присваивания полю (FieldAssign). Сознательно узкий список - переменная,
// "этот", доступ к полю. Результат вызова функции/конструктора/другого
// метода - нет (временное значение, в C нельзя ни взять его адрес, ни
// присвоить его полю) - для них явная ошибка Rech лучше невнятной ошибки
// компилятора C про "lvalue required".
func receiverAddressable(e ast.Expr) bool {
	switch e.(type) {
	case *ast.Identifier, *ast.ThisExpr, *ast.FieldAccess:
		return true
	default:
		return false
	}
}

// genReceiverAddress генерирует адрес получателя вызова метода - указатель,
// который метод получает как this_ (docs/GRAMMAR.md, "получатель метода
// передаётся по указателю").
func genReceiverAddress(ctx *genCtx, receiver ast.Expr) (string, valueType, error) {
	if !receiverAddressable(receiver) {
		return "", typeUnknown, fmt.Errorf("строка %d: получателем вызова метода может быть только переменная, поле или \"этот\"", lineOf(receiver))
	}
	code, typ, err := genExpr(ctx, receiver)
	if err != nil {
		return "", typeUnknown, err
	}
	return fmt.Sprintf("&(%s)", code), typ, nil
}

// genMethodCall генерирует C-код вызова метода (общий для MethodCallExpr-
// выражения и ExprStmt-инструкции, как genCall - для обычных функций).
func genMethodCall(ctx *genCtx, e *ast.MethodCallExpr) (string, funcSig, error) {
	receiverCode, receiverType, err := genReceiverAddress(ctx, e.Receiver)
	if err != nil {
		return "", funcSig{}, err
	}
	if receiverType.kind != kindEntity {
		return "", funcSig{}, fmt.Errorf("строка %d: нельзя вызвать метод %q - тип %s не сущность", e.Line, e.Method, receiverType)
	}
	sig, ok := ctx.methods[receiverType.entity][e.Method]
	if !ok {
		return "", funcSig{}, fmt.Errorf("строка %d: у сущности %q нет метода %q", e.Line, receiverType.entity, e.Method)
	}
	if len(e.Args) != len(sig.ParamTypes) {
		return "", funcSig{}, fmt.Errorf("строка %d: метод %q ожидает аргументов: %d, передано: %d", e.Line, e.Method, len(sig.ParamTypes), len(e.Args))
	}
	argCodes := make([]string, len(e.Args)+1)
	argCodes[0] = receiverCode
	for i, arg := range e.Args {
		code, typ, err := genExpr(ctx, arg)
		if err != nil {
			return "", funcSig{}, err
		}
		if typ != sig.ParamTypes[i] {
			return "", funcSig{}, fmt.Errorf("строка %d: аргумент %d вызова метода %q должен быть типа %s, получен %s", e.Line, i+1, e.Method, sig.ParamTypes[i], typ)
		}
		argCodes[i+1] = code
	}
	return fmt.Sprintf("%s(%s)", mangleMethod(receiverType.entity, e.Method), strings.Join(argCodes, ", ")), sig, nil
}

// genNewExpr генерирует C99 compound literal для "создать Тип(...)" - без
// отдельной сгенерированной C-функции-конструктора и без прототипа,
// compound literal - валидное C-выражение, встраиваемое прямо в место
// вызова (docs/ROADMAP.md, шаг 4). Аргументы - позиционные значения полей
// в порядке объявления в "имеет:", как обычный вызов функции (без
// падежного согласования имени типа - это отложено, docs/GRAMMAR.md).
func genNewExpr(ctx *genCtx, e *ast.NewExpr) (string, valueType, error) {
	sig, ok := ctx.entities[e.Type]
	if !ok {
		// Не пользовательская сущность и ещё не встречавшаяся generic-
		// инстанциация (шаг 4.2) - resolveType при необходимости синтезирует
		// её (typedef struct + методы в ctx.genericsCode) и зарегистрирует в
		// ctx.entities; если e.Type в принципе не тип (ни примитив, ни
		// сущность, ни известный generic-шаблон) - вернёт понятную ошибку.
		if _, err := resolveType(ctx, e.Type); err != nil {
			return "", typeUnknown, fmt.Errorf("строка %d: %w", e.Line, err)
		}
		sig, ok = ctx.entities[e.Type]
		if !ok {
			return "", typeUnknown, fmt.Errorf("строка %d: сущность %q не объявлена", e.Line, e.Type)
		}
	}
	var parts []string
	if e.FieldNames != nil {
		var err error
		parts, err = genNewExprLabeled(ctx, e, sig)
		if err != nil {
			return "", typeUnknown, err
		}
	} else {
		if len(e.Args) != len(sig.FieldNames) {
			return "", typeUnknown, fmt.Errorf("строка %d: конструктор %q ожидает значений полей: %d, передано: %d", e.Line, e.Type, len(sig.FieldNames), len(e.Args))
		}
		parts = make([]string, len(e.Args))
		for i, arg := range e.Args {
			code, typ, err := genExpr(ctx, arg)
			if err != nil {
				return "", typeUnknown, err
			}
			if typ != sig.FieldTypes[i] {
				return "", typeUnknown, fmt.Errorf("строка %d: поле %q сущности %q должно быть типа %s, получено %s", e.Line, sig.FieldNames[i], e.Type, sig.FieldTypes[i], typ)
			}
			parts[i] = fmt.Sprintf(".%s = %s", mangleField(sig.FieldNames[i]), code)
		}
	}
	initList := strings.Join(parts, ", ")
	if initList == "" {
		// Сущность без полей - зануляем техническую заглушку структуры
		// (generateEntityStruct), а не полагаемся на "{}" (расширение GNU,
		// не входящее в стандартный C99).
		initList = "0"
	}
	return fmt.Sprintf("(%s){%s}", mangleType(e.Type), initList), entityType(e.Type), nil
}

// genNewExprLabeled генерирует именованную форму конструктора
// ("новый Тип(поле: значение, ...)") - designated-initializer части
// compound literal в том порядке, в котором поля перечислены в вызове (C
// не требует порядка инициализаторов). Число меток должно совпадать с
// числом полей сущности - вместе с проверкой "поле указано не более
// одного раза" это гарантирует, что каждое поле получает ровно одно
// значение, без отдельной проверки "все поля покрыты".
func genNewExprLabeled(ctx *genCtx, e *ast.NewExpr, sig entitySig) ([]string, error) {
	if len(e.FieldNames) != len(sig.FieldNames) {
		return nil, fmt.Errorf("строка %d: конструктор %q ожидает значений полей: %d, передано: %d", e.Line, e.Type, len(sig.FieldNames), len(e.FieldNames))
	}
	seen := make([]bool, len(sig.FieldNames))
	parts := make([]string, len(e.Args))
	for i, label := range e.FieldNames {
		idx := sig.fieldIndex(label)
		if idx == -1 {
			return nil, fmt.Errorf("строка %d: у сущности %q нет поля %q", e.Line, e.Type, label)
		}
		if seen[idx] {
			return nil, fmt.Errorf("строка %d: поле %q конструктора %q указано более одного раза", e.Line, label, e.Type)
		}
		seen[idx] = true
		code, typ, err := genExpr(ctx, e.Args[i])
		if err != nil {
			return nil, err
		}
		if typ != sig.FieldTypes[idx] {
			return nil, fmt.Errorf("строка %d: поле %q сущности %q должно быть типа %s, получено %s", e.Line, label, e.Type, sig.FieldTypes[idx], typ)
		}
		parts[i] = fmt.Sprintf(".%s = %s", mangleField(label), code)
	}
	return parts, nil
}

// genFieldAccess генерирует чтение поля сущности ("receiver.поле").
func genFieldAccess(ctx *genCtx, e *ast.FieldAccess) (string, valueType, error) {
	code, typ, err := genExpr(ctx, e.Receiver)
	if err != nil {
		return "", typeUnknown, err
	}
	if typ.kind != kindEntity {
		return "", typeUnknown, fmt.Errorf("строка %d: нельзя обратиться к полю %q - тип выражения %s, не сущность", e.Line, e.Field, typ)
	}
	sig := ctx.entities[typ.entity]
	i := sig.fieldIndex(e.Field)
	if i == -1 {
		return "", typeUnknown, fmt.Errorf("строка %d: у сущности %q нет поля %q", e.Line, typ.entity, e.Field)
	}
	return fmt.Sprintf("(%s).%s", code, mangleField(e.Field)), sig.FieldTypes[i], nil
}

// generateFieldAssign генерирует "receiver.поле = выражение". Получатель в
// шаге 4.c ограничен переменной (уже объявленной, сущностного типа) -
// этот появится в 4.d вместе с методами; для прочих получателей (пока
// таких по грамматике и не бывает - парсер строит FieldAssign только из
// IDENT) была бы понятная ошибка Rech, а не невнятная C-ошибка.
func generateFieldAssign(sb *strings.Builder, ctx *genCtx, s *ast.FieldAssign, ind string) error {
	if !receiverAddressable(s.Receiver) {
		return fmt.Errorf("строка %d: получателем присваивания полю может быть только переменная, поле или \"этот\"", s.Line)
	}
	receiverCode, receiverType, err := genExpr(ctx, s.Receiver)
	if err != nil {
		return err
	}
	if receiverType.kind != kindEntity {
		return fmt.Errorf("строка %d: нельзя присвоить полю %q - тип %s не сущность", s.Line, s.Field, receiverType)
	}
	sig := ctx.entities[receiverType.entity]
	i := sig.fieldIndex(s.Field)
	if i == -1 {
		return fmt.Errorf("строка %d: у сущности %q нет поля %q", s.Line, receiverType.entity, s.Field)
	}
	code, typ, err := genExpr(ctx, s.Value)
	if err != nil {
		return err
	}
	if typ != sig.FieldTypes[i] {
		return fmt.Errorf("строка %d: полю %q типа %s нельзя присвоить значение типа %s", s.Line, s.Field, sig.FieldTypes[i], typ)
	}
	fmt.Fprintf(sb, "%s(%s).%s = %s;\n", ind, receiverCode, mangleField(s.Field), code)
	return nil
}

// genIncDecExpr генерирует "имя++"/"имя--"/"++имя"/"--имя" - напрямую в
// C-постфикс/префикс "++"/"--" (у него ровно та же семантика "старое/новое
// значение как результат", возвращаемый и мутирующий один и тот же
// lvalue), поэтому дополнительной логики не нужно. Применимо только к уже
// объявленной переменной типа Целое/Число - не к полю сущности и не к
// результату вызова (как и Assign - только простое имя).
func genIncDecExpr(ctx *genCtx, e *ast.IncDecExpr) (string, valueType, error) {
	typ, ok := ctx.lookup(e.Name)
	if !ok {
		return "", typeUnknown, fmt.Errorf("строка %d: переменная %q не объявлена", e.Line, e.Name)
	}
	if typ != typeInt && typ != typeFloat {
		return "", typeUnknown, fmt.Errorf("строка %d: оператор %q применим только к Целое/Число, получено %s", e.Line, e.Op, typ)
	}
	name := mangleVar(e.Name)
	if e.Prefix {
		return fmt.Sprintf("(%s%s)", e.Op, name), typ, nil
	}
	return fmt.Sprintf("(%s%s)", name, e.Op), typ, nil
}

func genUnaryExpr(ctx *genCtx, e *ast.UnaryExpr) (string, valueType, error) {
	code, typ, err := genExpr(ctx, e.Operand)
	if err != nil {
		return "", typeUnknown, err
	}
	switch e.Op {
	case "-":
		if typ != typeInt && typ != typeFloat {
			return "", typeUnknown, fmt.Errorf("строка %d: унарный \"-\" применим только к числам, получен %s", e.Line, typ)
		}
		return fmt.Sprintf("(-%s)", code), typ, nil
	case "не":
		if typ != typeBool {
			return "", typeUnknown, fmt.Errorf("строка %d: \"не\" применимо только к логическому значению, получен %s", e.Line, typ)
		}
		return fmt.Sprintf("(!%s)", code), typeBool, nil
	case "~":
		if typ != typeInt {
			return "", typeUnknown, fmt.Errorf("строка %d: побитовый \"~\" применим только к Целое, получен %s", e.Line, typ)
		}
		return fmt.Sprintf("(~%s)", code), typeInt, nil
	default:
		return "", typeUnknown, fmt.Errorf("строка %d: неизвестная унарная операция %q", e.Line, e.Op)
	}
}

func genBinaryExpr(ctx *genCtx, e *ast.BinaryExpr) (string, valueType, error) {
	lc, lt, err := genExpr(ctx, e.Left)
	if err != nil {
		return "", typeUnknown, err
	}
	rc, rt, err := genExpr(ctx, e.Right)
	if err != nil {
		return "", typeUnknown, err
	}

	switch e.Op {
	case "+":
		if lt == typeString && rt == typeString {
			ctx.usesConcat = true
			return fmt.Sprintf("rech_str_concat(%s, %s)", lc, rc), typeString, nil
		}
		return genArithmetic(ctx, e, "+", lc, lt, rc, rt)
	case "-", "*", "/":
		return genArithmetic(ctx, e, e.Op, lc, lt, rc, rt)

	case "==", "!=":
		if lt == typeString && rt == typeString {
			ctx.usesStrCmp = true
			cmp := fmt.Sprintf("strcmp(%s, %s)", lc, rc)
			if e.Op == "==" {
				return fmt.Sprintf("(%s == 0)", cmp), typeBool, nil
			}
			return fmt.Sprintf("(%s != 0)", cmp), typeBool, nil
		}
		if lt != rt || (lt != typeInt && lt != typeFloat && lt != typeBool) {
			return "", typeUnknown, fmt.Errorf("строка %d: оператор %q не поддерживается для типов %s и %s", e.Line, e.Op, lt, rt)
		}
		return fmt.Sprintf("(%s %s %s)", lc, e.Op, rc), typeBool, nil

	case "<", ">", "<=", ">=":
		if lt != rt || (lt != typeInt && lt != typeFloat) {
			return "", typeUnknown, fmt.Errorf("строка %d: оператор %q применим только к числам одного типа, получены %s и %s", e.Line, e.Op, lt, rt)
		}
		return fmt.Sprintf("(%s %s %s)", lc, e.Op, rc), typeBool, nil

	case "и", "или":
		if lt != typeBool || rt != typeBool {
			return "", typeUnknown, fmt.Errorf("строка %d: оператор %q применим только к логическим значениям, получены %s и %s", e.Line, e.Op, lt, rt)
		}
		cOp := "&&"
		if e.Op == "или" {
			cOp = "||"
		}
		return fmt.Sprintf("(%s %s %s)", lc, cOp, rc), typeBool, nil

	case "&", "|", "^", "<<", ">>":
		if lt != typeInt || rt != typeInt {
			return "", typeUnknown, fmt.Errorf("строка %d: побитовый оператор %q применим только к Целое, получены %s и %s", e.Line, e.Op, lt, rt)
		}
		return fmt.Sprintf("(%s %s %s)", lc, e.Op, rc), typeInt, nil

	default:
		return "", typeUnknown, fmt.Errorf("строка %d: неизвестная операция %q", e.Line, e.Op)
	}
}

func genArithmetic(ctx *genCtx, e *ast.BinaryExpr, op string, lc string, lt valueType, rc string, rt valueType) (string, valueType, error) {
	if lt != typeInt && lt != typeFloat {
		return "", typeUnknown, fmt.Errorf("строка %d: оператор %q не поддерживается для типа %s", e.Line, op, lt)
	}
	if rt != typeInt && rt != typeFloat {
		return "", typeUnknown, fmt.Errorf("строка %d: оператор %q не поддерживается для типа %s", e.Line, op, rt)
	}
	if lt == typeFloat || rt == typeFloat {
		if lt == typeInt {
			lc = fmt.Sprintf("((double)%s)", lc)
		}
		if rt == typeInt {
			rc = fmt.Sprintf("((double)%s)", rc)
		}
		return fmt.Sprintf("(%s %s %s)", lc, op, rc), typeFloat, nil
	}
	if op == "/" {
		ctx.usesPanic = true
		ctx.usesIntDiv = true
		return fmt.Sprintf("rech_int_div(%s, %s)", lc, rc), typeInt, nil
	}
	return fmt.Sprintf("(%s %s %s)", lc, op, rc), typeInt, nil
}

func formatFloatLiteral(v float64) string {
	s := strconv.FormatFloat(v, 'g', -1, 64)
	if !strings.ContainsAny(s, ".eE") {
		s += ".0"
	}
	return s
}

func escapeCString(s string) string {
	var sb strings.Builder
	for _, r := range s {
		switch r {
		case '"':
			sb.WriteString(`\"`)
		case '\\':
			sb.WriteString(`\\`)
		case '\n':
			sb.WriteString(`\n`)
		case '\t':
			sb.WriteString(`\t`)
		default:
			sb.WriteRune(r)
		}
	}
	return sb.String()
}

func lineOf(n ast.Node) int {
	switch v := n.(type) {
	case *ast.PrintStmt:
		return v.Line
	case *ast.VarDecl:
		return v.Line
	case *ast.Assign:
		return v.Line
	case *ast.FieldAssign:
		return v.Line
	case *ast.BecomeStmt:
		return v.Line
	case *ast.AssertStmt:
		return v.Line
	case *ast.IfStmt:
		return v.Line
	case *ast.WhileStmt:
		return v.Line
	case *ast.ForStmt:
		return v.Line
	case *ast.ForEachStmt:
		return v.Line
	case *ast.ReturnStmt:
		return v.Line
	case *ast.ExprStmt:
		return v.Line
	case *ast.IncDecExpr:
		return v.Line
	case *ast.CallExpr:
		return v.Line
	case *ast.NewExpr:
		return v.Line
	case *ast.FieldAccess:
		return v.Line
	case *ast.ThisExpr:
		return v.Line
	case *ast.MethodCallExpr:
		return v.Line
	case *ast.StringLiteral:
		return v.Line
	case *ast.IntLiteral:
		return v.Line
	case *ast.FloatLiteral:
		return v.Line
	case *ast.BoolLiteral:
		return v.Line
	case *ast.Identifier:
		return v.Line
	case *ast.UnaryExpr:
		return v.Line
	case *ast.BinaryExpr:
		return v.Line
	default:
		return 0
	}
}
