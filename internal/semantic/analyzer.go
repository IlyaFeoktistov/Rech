package semantic

import (
	"fmt"
	"maps"
	"strings"

	"slices"

	"github.com/IlyaFeoktistov/Rech/internal/ast"
	"github.com/IlyaFeoktistov/Rech/internal/builtins"
	"github.com/IlyaFeoktistov/Rech/internal/ir"
)

// Analyzer инкапсулирует состояние семантического анализа.
type Analyzer struct {
	entities    map[string]entitySig
	methods     map[string]map[string]funcSig
	funcs       map[string]funcSig
	aliases     map[string]string
	aliasState  map[string]string
	enums       map[string]map[string]int64
	interfaces  map[string]interfaceSig
	externFuncs []*ir.ExternFuncDecl

	// resolvedConsts/resolvingConsts - кэш и защита от цикла для констант
	// уровня сущности (см. resolveEntityConstant), общие на весь Analyze -
	// передаются по ссылке в каждый newCtx этого прохода.
	resolvedConsts  map[string]ir.Expr
	resolvingConsts map[string]bool

	// resolvedFieldDefaults - кэш значений по умолчанию полей ("Сущность.
	// поле" -> уже разрешённый ir.Expr), тем же принципом, что и
	// resolvedConsts (см. resolveFieldDefault) - без своего множества
	// "resolvingFieldDefaults": в отличие от константы, значение поля не
	// видит соседние поля (thisEntity пуст при разборе), значит сослаться
	// само на себя оно не может - цикл через resolveEntityConstant внутри
	// уже защищён resolvingConsts.
	resolvedFieldDefaults map[string]ir.Expr

	// genericConstraints - ограничения параметров типа каждой инстанциации
	// пользовательского generic-типа ("Коробка<Целое>" -> ограничения T),
	// по мангл-имени - собраны заранее (instantiateGenerics, до того как
	// entities/interfaces вообще существуют), проверяются здесь, в
	// processEntityDecl/processFunctionSig, когда до этой инстанциации
	// доходит очередь и entities/interfaces уже известны.
	genericConstraints map[string][]constraintCheck

	// callablesSeen/callableOrder - дедупликация сигнатур "Вызываемый"
	// на весь Analyze, тем же принципом, что entities/methods -
	// см. registerCallable. callableOrder идёт прямо в Program.Callables
	// (buildIR) в порядке первой встречи.
	callablesSeen map[string]bool
	callableOrder []ir.CallableSig

	// lambdaCounter - следующий порядковый номер для имени лямбды
	// "__лямбда_N") - общий на весь Analyze счётчик, чтобы имя было
	// уникально во всей программе (backend использует его напрямую как
	// базовое имя генерируемой C-функции, не изобретая собственный
	// счётчик - иначе cbackend/interp завели бы два независимых способа
	// нумерации одних и тех же лямбд).
	lambdaCounter int

	// parseTargets/serializeTargets/xmlParseTargets/xmlSerializeTargets -
	// дедуплицированные по имени типа списки целей "разобрать"/
	// "сериализовать" (JSON/XML отдельно - backend генерирует разные
	// функции для одного и того же Rech-типа в зависимости от формата) -
	// тем же принципом, что callablesSeen/callableOrder. См. dedupTypeList.
	parseTargets        dedupTypeList
	serializeTargets    dedupTypeList
	xmlParseTargets     dedupTypeList
	xmlSerializeTargets dedupTypeList

	// reflectTargets - дедуплицированный список сущностей/структур,
	// встретившихся хоть раз как аргумент боксирования ("(Значение) x")
	// или как имя у "тип <Имя>"/"значение как <Имя>" - тем же принципом,
	// что parseTargets (см. package doc ir.Program.ReflectTargets).
	// Примитивы не попадают сюда - их дескрипторы cbackend эмитит всегда,
	// безусловно, но только если usesReflection ниже.
	reflectTargets dedupTypeList
	// usesReflection - см. package doc ir.Program.UsesReflection.
	usesReflection bool

	// indexedCollections - имена generic-инстанциаций (Список<T>/
	// Словарь<К, В>), встретившихся хоть раз как получатель безопасной
	// индексации "[]" (ir.ListIndexExpr/ir.DictIndexExpr) - см. package doc
	// ir.GenericInstantiation.NeedsSafeIndex.
	indexedCollections map[string]bool

	// listLiteralCollections - имена инстанциаций Список<T>, построенных
	// хоть раз литералом "[элемент, ...]" - см. package doc
	// ir.GenericInstantiation.NeedsListLiteral.
	listLiteralCollections map[string]bool
}

// markIndexUsed - см. package doc Analyzer.indexedCollections.
func (a *Analyzer) markIndexUsed(name string) {
	if a.indexedCollections == nil {
		a.indexedCollections = map[string]bool{}
	}
	a.indexedCollections[name] = true
}

// markListLiteralUsed - см. package doc Analyzer.listLiteralCollections.
func (a *Analyzer) markListLiteralUsed(name string) {
	if a.listLiteralCollections == nil {
		a.listLiteralCollections = map[string]bool{}
	}
	a.listLiteralCollections[name] = true
}

// dedupTypeList - список ir.Type без повторов (по Type.String()), в
// порядке первой встречи - используется для Program.ParseTargets и его
// трёх аналогов (SerializeTargets/XMLParseTargets/XMLSerializeTargets) -
// каждый список независим (один и тот же Rech-тип может встретиться как
// цель JSON-разбора, XML-разбора и сериализации одновременно, backend
// генерирует под каждую комбинацию свою функцию).
type dedupTypeList struct {
	seen  map[string]bool
	order []ir.Type
}

func (l *dedupTypeList) add(t ir.Type) {
	if l.seen == nil {
		l.seen = map[string]bool{}
	}
	key := t.String()
	if l.seen[key] {
		return
	}
	l.seen[key] = true
	l.order = append(l.order, t)
}

func (a *Analyzer) registerParseTarget(t ir.Type)        { a.parseTargets.add(t) }
func (a *Analyzer) registerSerializeTarget(t ir.Type)    { a.serializeTargets.add(t) }
func (a *Analyzer) registerXMLParseTarget(t ir.Type)     { a.xmlParseTargets.add(t) }
func (a *Analyzer) registerXMLSerializeTarget(t ir.Type) { a.xmlSerializeTargets.add(t) }

// registerReflectTarget регистрирует t (сущность/структура) как нуждающийся
// в дескрипторе рефлексии - примитивы регистрировать не нужно (см. package
// doc reflectTargets), но вызывающая сторона всё равно может передать
// любой Type - add сам дедуплицирует по имени.
func (a *Analyzer) registerReflectTarget(t ir.Type) { a.reflectTargets.add(t) }

// markUsesReflection взводит usesReflection - см. package doc
// ir.Program.UsesReflection. Отдельно от registerReflectTarget - вызывается
// и для примитивов (тип Целое, значение как Целое), которые в
// reflectTargets не попадают.
func (a *Analyzer) markUsesReflection() { a.usesReflection = true }

// collectEntities - предварительный проход: поля, сигнатуры методов и
// производные типы состояний (aliases - "АвторизованныйПользователь" ->
// "Пользователь") всех сущностей программы, до анализа любого
// тела. Одним циклом по prog.Entities, как и в cbackend.Generate -
// поле/параметр/тип результата метода сущности N может ссылаться только
// на сущность (или производный тип), уже занесённую к этому моменту
// (объявленную раньше в файле, включая саму N) - та же граница, что и в
// cbackend.
func (a *Analyzer) collectEntities(prog *ast.Program) error {
	entities := map[string]entitySig{}
	methods := map[string]map[string]funcSig{}
	aliases := map[string]string{}
	aliasState := map[string]string{}
	// "Исключение" - системная базовая сущность выброшенных значений
	// (internal/ir.ThrowStmt/TryStmt), доступна в любой программе без
	// объявления, тем же принципом, что и Запрос/Ответ (registerHTTPEntity)
	// - но, в отличие от них, засеяна ДО цикла ниже, а не лениво по первому
	// упоминанию: "сущность Б наследует Исключение" резолвит базовую
	// сущность прямым поиском в entities (applyInheritance), не через
	// resolveTypeName, значит запись обязана существовать здесь заранее,
	// иначе любой пользовательский наследник, объявленный раньше в файле,
	// чем гипотетическое "первое упоминание", не нашёл бы её вовсе.
	entities["Исключение"] = entitySig{
		FieldNames:      []string{"сообщение"},
		FieldTypes:      []ir.Type{ir.TypeString},
		FieldVisibility: []ast.Visibility{ast.Public},
	}
	methods["Исключение"] = map[string]funcSig{}
	// "внешняя сущность" - непрозрачные внешние типы (FFI) - регистрируются
	// раньше пользовательских сущностей ниже той же коллизией имён
	// (entities/aliases/interfaces): у них нет полей/методов, resolveTypeName
	// узнаёт их по entitySig.IsExternOpaque и резолвит в ir.ExternEntityType,
	// не ir.EntityType.
	for _, extEnt := range prog.ExternEntities {
		if builtins.ReservedEntityNames[extEnt.Alias] {
			return fmt.Errorf("внешняя сущность %q - зарезервированное имя встроенной сущности, нельзя объявить с этим именем", extEnt.Alias)
		}
		if _, exists := entities[extEnt.Alias]; exists {
			return fmt.Errorf("внешняя сущность %q: имя уже занято", extEnt.Alias)
		}
		if other, exists := aliases[extEnt.Alias]; exists {
			return fmt.Errorf("внешняя сущность %q: имя совпадает с производным типом состояния, уже занятым сущностью %q", extEnt.Alias, other)
		}
		if _, exists := a.interfaces[extEnt.Alias]; exists {
			return fmt.Errorf("внешняя сущность %q: имя совпадает с уже объявленным интерфейсом", extEnt.Alias)
		}
		entities[extEnt.Alias] = entitySig{IsExternOpaque: true}
	}
	for _, ent := range prog.Entities {
		if builtins.ReservedEntityNames[ent.Name] {
			return fmt.Errorf("сущность %q - зарезервированное имя встроенной сущности, нельзя объявить свою сущность с этим именем", ent.Name)
		}
		if _, exists := entities[ent.Name]; exists {
			return fmt.Errorf("сущность %q объявлена более одного раза", ent.Name)
		}
		if other, exists := aliases[ent.Name]; exists {
			return fmt.Errorf("сущность %q: имя совпадает с производным типом состояния, уже занятым сущностью %q", ent.Name, other)
		}
		if _, exists := a.interfaces[ent.Name]; exists {
			return fmt.Errorf("сущность %q: имя совпадает с уже объявленным интерфейсом", ent.Name)
		}
		if err := a.checkGenericConstraints(entities, methods, aliases, ent.Name); err != nil {
			return err
		}
		var sig entitySig

		// Собираем константы, если есть
		for _, c := range ent.Constants {
			if sig.fieldIndex(c.Name) != -1 {
				return fmt.Errorf("сущность %q: константа %q конфликтует с полем с тем же именем", ent.Name, c.Name)
			}
			if sig.constIndex(c.Name) != -1 {
				return fmt.Errorf("сущность %q: константа %q объявлена более одного раза", ent.Name, c.Name)
			}
			sig.ConstNames = append(sig.ConstNames, c.Name)
			sig.ConstExprs = append(sig.ConstExprs, c.Value)
			sig.ConstDocs = append(sig.ConstDocs, c.Doc)
			sig.ConstVisibility = append(sig.ConstVisibility, c.Visibility)
		}

		for _, f := range ent.Fields {
			if sig.fieldIndex(f.Name) != -1 {
				return fmt.Errorf("сущность %q: поле %q объявлено более одного раза", ent.Name, f.Name)
			}
			typ, err := resolveTypeName(entities, methods, aliases, a.interfaces, f.Type)
			if err != nil {
				return fmt.Errorf("сущность %q: поле %q: %w", ent.Name, f.Name, err)
			}
			if ent.IsExternStruct && !isFFIType(entities, typ) {
				return fmt.Errorf("внешняя структура %q: поле %q имеет тип %s - допустимы только примитивы (Целое/Действительное/Дробное/Логическое/Байт/Символ/Строка) и внешние сущности (\"внешняя сущность\") - иначе реальный размер C-структуры был бы искажён", ent.Name, f.Name, typ)
			}
			sig.FieldNames = append(sig.FieldNames, f.Name)
			sig.FieldTypes = append(sig.FieldTypes, typ)
			sig.FieldDocs = append(sig.FieldDocs, f.Doc)
			sig.FieldTags = append(sig.FieldTags, f.Tag)
			sig.FieldVisibility = append(sig.FieldVisibility, f.Visibility)
			sig.FieldDefaultExprs = append(sig.FieldDefaultExprs, f.Default)
		}
		if len(ent.States) > 0 {
			sig.States = map[string]bool{}
			for _, st := range ent.States {
				sig.States[st.Name] = true
				if _, exists := entities[st.TypeName]; exists {
					return fmt.Errorf("сущность %q: производный тип состояния %q совпадает с именем уже объявленной сущности", ent.Name, st.TypeName)
				}
				if other, exists := aliases[st.TypeName]; exists && other != ent.Name {
					return fmt.Errorf("сущность %q: производный тип состояния %q уже используется сущностью %q", ent.Name, st.TypeName, other)
				}
				aliases[st.TypeName] = ent.Name
				aliasState[st.TypeName] = st.Name
			}
		}
		sig.IsStruct = ent.IsStruct
		sig.IsExternStruct = ent.IsExternStruct
		// Регистрируется уже сейчас (до разбора методов), не только после
		// сплющивания с базовой сущностью ниже - иначе метод, ссылающийся
		// на тип самой этой сущности в своей сигнатуре (параметр/результат),
		// не смог бы её найти. Флаттенинг ниже просто перезапишет запись,
		// когда будет готова - Type{Kind:KindEntity, Entity: ent.Name},
		// который уже вернул resolveTypeName где угодно раньше, от этого
		// не меняется (имя типа - не сама сигнатура по значению).
		entities[ent.Name] = sig

		ownMethods := map[string]funcSig{}
		for _, m := range ent.Methods {
			if _, exists := ownMethods[m.Name]; exists {
				return fmt.Errorf("сущность %q: метод %q объявлен более одного раза", ent.Name, m.Name)
			}
			params := make([]ir.Type, len(m.Params))
			paramStates := make([]string, len(m.Params))
			for i, p := range m.Params {
				typ, err := resolveTypeName(entities, methods, aliases, a.interfaces, p.Type)
				if err != nil {
					return fmt.Errorf("сущность %q: метод %q: параметр %q: %w", ent.Name, m.Name, p.Name, err)
				}
				params[i] = typ
				paramStates[i] = aliasState[p.Type]
			}
			msig := funcSig{Params: params, ParamStates: paramStates, Visibility: m.Visibility}
			switch {
			case m.SelfReturn:
				// "этот" как тип результата - собственный метод сущности X
				// резолвится в EntityType(X) сразу же: applyInheritance
				// перерезолвит копию этой сигнатуры в EntityType(Y) для
				// каждой Y, которая унаследует метод без переопределения -
				// см. package doc funcSig.SelfReturn.
				msig.ReturnType = ir.EntityType(ent.Name)
				msig.HasReturn = true
				msig.SelfReturn = true
			case m.ReturnType != "":
				typ, err := resolveTypeName(entities, methods, aliases, a.interfaces, m.ReturnType)
				if err != nil {
					return fmt.Errorf("сущность %q: метод %q: тип результата: %w", ent.Name, m.Name, err)
				}
				msig.ReturnType = typ
				msig.HasReturn = true
			}
			ownMethods[m.Name] = msig
		}

		if ent.Ctor != nil {
			ctorParams := make([]ir.Type, len(ent.Ctor.Params))
			for i, p := range ent.Ctor.Params {
				typ, err := resolveTypeName(entities, methods, aliases, a.interfaces, p.Type)
				if err != nil {
					return fmt.Errorf("сущность %q: конструктор: параметр %q: %w", ent.Name, p.Name, err)
				}
				ctorParams[i] = typ
			}
			sig.HasCtor = true
			sig.CtorParams = ctorParams
		}

		// Наследование ("наследует")/интерфейсы ("реализует") -
		// сплющивает sig/ownMethods с базовой сущностью, если она есть, и
		// проверяет соответствие каждому объявленному интерфейсу - см.
		// internal/semantic/inheritance.go.
		flatSig, flatMethods, err := a.applyInheritance(entities, methods, ent, sig, ownMethods)
		if err != nil {
			return err
		}
		entities[ent.Name] = flatSig
		methods[ent.Name] = flatMethods
	}
	a.entities = entities
	a.methods = methods
	a.aliases = aliases
	a.aliasState = aliasState
	return nil
}

// collectFuncSigs - предварительный проход: сигнатуры всех функций
// программы, до анализа любого тела (нужно для вызовов вперёд/взаимной
// рекурсии - как и в cbackend.Generate). entities/methods/aliases уже
// полностью собраны (collectEntities) - функция может ссылаться на любую
// сущность, производный тип состояния или generic-коллекцию, независимо
// от того, где та объявлена в файле относительно функции.
func (a *Analyzer) collectFuncSigs(prog *ast.Program) error {
	funcs := map[string]funcSig{}
	registerBuiltinFuncs(funcs)
	registerHTTPEntity(a.entities, "Запрос", httpRequestFields)
	registerHTTPEntity(a.entities, "Ответ", httpResponseFields)
	funcs["запрос"] = funcSig{Params: []ir.Type{ir.EntityType("Запрос")}, ReturnType: ir.EntityType("Ответ"), HasReturn: true}
	registerHTTPServerFuncs(funcs)
	registerDateTimeFuncs(a.entities, funcs)
	for _, fn := range prog.Functions {
		if _, exists := funcs[fn.Name]; exists {
			if builtins.IsReservedName(fn.Name) {
				return fmt.Errorf("функция %q - зарезервированное имя встроенной функции, нельзя объявить свою функцию с этим именем", fn.Name)
			}
			return fmt.Errorf("функция %q объявлена более одного раза", fn.Name)
		}
		if err := a.checkGenericConstraints(a.entities, a.methods, a.aliases, fn.Name); err != nil {
			return err
		}
		params := make([]ir.Type, len(fn.Params))
		paramStates := make([]string, len(fn.Params))
		for i, p := range fn.Params {
			typ, err := resolveTypeName(a.entities, a.methods, a.aliases, a.interfaces, p.Type)
			if err != nil {
				return fmt.Errorf("функция %q: параметр %q: %w", fn.Name, p.Name, err)
			}
			params[i] = typ
			paramStates[i] = a.aliasState[p.Type]
		}
		sig := funcSig{Params: params, ParamStates: paramStates}
		if fn.ReturnType != "" {
			typ, err := resolveTypeName(a.entities, a.methods, a.aliases, a.interfaces, fn.ReturnType)
			if err != nil {
				return fmt.Errorf("функция %q: тип результата: %w", fn.Name, err)
			}
			sig.ReturnType = typ
			sig.HasReturn = true
		}
		funcs[fn.Name] = sig
	}
	externFuncs, err := a.collectExternFuncSigs(funcs, a.entities, a.methods, a.aliases, prog)
	if err != nil {
		return err
	}
	a.funcs = funcs
	a.externFuncs = externFuncs
	return nil
}

// collectExternFuncSigs - "внешняя функция": та же коллизия имени с
// builtin/уже объявленной функцией, что и у обычных функций выше, плюс
// проверка, что параметры/результат допустимы на границе FFI (isFFIType).
// Регистрирует Alias в funcs (для проверки вызовов), возвращает
// []*ir.ExternFuncDecl для ir.Program.ExternFuncs - настоящий C-символ
// (CName) для кодогенерации берёт оттуда только cbackend.GenerateFromIR,
// семантическому анализу он не нужен.
func (a *Analyzer) collectExternFuncSigs(funcs map[string]funcSig, entities map[string]entitySig, methods map[string]map[string]funcSig, aliases map[string]string, prog *ast.Program) ([]*ir.ExternFuncDecl, error) {
	var out []*ir.ExternFuncDecl
	for _, ext := range prog.ExternFuncs {
		label := fmt.Sprintf("внешняя функция %q", ext.Alias)
		if _, exists := funcs[ext.Alias]; exists {
			if builtins.IsReservedName(ext.Alias) {
				return nil, fmt.Errorf("%s - зарезервированное имя встроенной функции, нельзя объявить с этим именем", label)
			}
			return nil, fmt.Errorf("имя %q уже объявлено (как функция) - нельзя повторно объявить его как внешнюю функцию", ext.Alias)
		}
		params := make([]ir.Type, len(ext.Params))
		for i, p := range ext.Params {
			typ, err := resolveTypeName(entities, methods, aliases, a.interfaces, p.Type)
			if err != nil {
				return nil, fmt.Errorf("%s: параметр %q: %w", label, p.Name, err)
			}
			if !isFFIType(entities, typ) {
				return nil, fmt.Errorf("%s: параметр %q имеет тип %s - у внешней функции допустимы только примитивы (Целое/Действительное/Дробное/Логическое/Байт/Символ/Строка) и внешние сущности (\"внешняя сущность\")", label, p.Name, typ)
			}
			params[i] = typ
		}
		decl := &ir.ExternFuncDecl{CName: ext.CName, Alias: ext.Alias, Params: params}
		sig := funcSig{Params: params, IsExtern: true}
		if ext.ReturnType != "" {
			typ, err := resolveTypeName(entities, methods, aliases, a.interfaces, ext.ReturnType)
			if err != nil {
				return nil, fmt.Errorf("%s: тип результата: %w", label, err)
			}
			if !isFFIType(entities, typ) {
				return nil, fmt.Errorf("%s: тип результата %s не поддерживается - у внешней функции допустимы только примитивы (Целое/Действительное/Дробное/Логическое/Байт/Символ/Строка) и внешние сущности (\"внешняя сущность\")", label, typ)
			}
			decl.ReturnType = typ
			decl.HasReturn = true
			sig.ReturnType = typ
			sig.HasReturn = true
		}
		funcs[ext.Alias] = sig
		out = append(out, decl)
	}
	return out, nil
}

// collectEnums собирает все перечисления программы (шаг 12) - стандартные
// (standardEnums, всегда присутствуют) плюс объявленные пользователем
// (prog.Enums, копия standardEnums на входе - чтобы мутация ниже не
// затронула сам standardEnums между вызовами Analyze). Дубли членов
// внутри одного перечисления уже проверены парсером (см. parseEnumDecl) -
// здесь только коллизии ИМЕНИ самого перечисления с сущностью или другим
// перечислением (в т. ч. встроенным).
func (a *Analyzer) collectEnums(prog *ast.Program) error {
	enums := make(map[string]map[string]int64, len(standardEnums)+len(prog.Enums))
	for name, members := range standardEnums {
		copied := make(map[string]int64, len(members))
		maps.Copy(copied, members)
		enums[name] = copied
	}
	for _, e := range prog.Enums {
		if _, exists := a.entities[e.Name]; exists {
			return fmt.Errorf("строка %d: перечисление %q совпадает с именем уже объявленной сущности", e.Line, e.Name)
		}
		if _, exists := enums[e.Name]; exists {
			return fmt.Errorf("строка %d: перечисление %q уже объявлено (возможно, совпадает со встроенным перечислением)", e.Line, e.Name)
		}
		members := make(map[string]int64, len(e.Members))
		for _, m := range e.Members {
			members[m.Name] = m.Value
		}
		enums[e.Name] = members
	}
	a.enums = enums
	return nil
}

// analyzeParamsAndBody выполняет общую работу для функций и методов:
// регистрирует параметры в контексте, отмечает установленные состояния
// и анализирует тело, возвращая готовый список ir.Param и проанализированные
// ir.Stmt тела. Не обновляет внешние таблицы сигнатур — это делает вызывающая обёртка.
func (a *Analyzer) analyzeParamsAndBody(sc *ctx, astParams []ast.Param, sig funcSig, body []ast.Stmt) ([]ir.Param, []ir.Stmt, funcSig, error) {
	params := make([]ir.Param, len(astParams))
	for i, p := range astParams {
		params[i] = ir.Param{Name: p.Name, Type: sig.Params[i]}
		if err := sc.declare(p.Name, sig.Params[i]); err != nil {
			return nil, nil, sig, fmt.Errorf("строка %d: параметр %q: %w", p.Line, p.Name, err)
		}
		if i < len(sig.ParamStates) && sig.ParamStates[i] != "" {
			sc.setEstablished(p.Name, map[string]bool{sig.ParamStates[i]: true})
		}
	}
	b, err := analyzeBlock(sc, body)
	if err != nil {
		return nil, nil, sig, err
	}
	return params, b, sig, nil
}

func (a *Analyzer) analyzeEntity(ent *ast.EntityDecl) (*ir.Entity, error) {
	sig := a.entities[ent.Name]
	fields := make([]ir.Field, len(sig.FieldNames))
	fsc := newCtx(a, a.funcs, a.entities, a.methods, a.aliases, a.interfaces, a.enums, "", ir.Type{}, false, a.resolvedConsts, a.resolvingConsts)
	for i, name := range sig.FieldNames {
		var doc string
		if i < len(sig.FieldDocs) {
			doc = sig.FieldDocs[i]
		}
		var def ir.Expr
		if i < len(sig.FieldDefaultExprs) && sig.FieldDefaultExprs[i] != nil {
			val, err := resolveFieldDefault(fsc, ent.Name, name, ent.Line)
			if err != nil {
				return nil, fmt.Errorf("поле %q: %w", name, err)
			}
			def = val
		}
		var tag string
		if i < len(sig.FieldTags) {
			tag = sig.FieldTags[i]
		}
		fields[i] = ir.Field{Name: name, Type: sig.FieldTypes[i], Doc: doc, Default: def, Tag: tag}
	}
	// Константы сущности - через resolveEntityConstant, ту же функцию, что
	// использует доступ "Сущность.Константа"/"экземпляр.Константа" в
	// analyzeExpr, чтобы значение было одно и то же и вычислялось один раз
	// (sc.resolvedConsts), с защитой от циклической ссылки.
	constants := make([]ir.Constant, 0, len(sig.ConstNames))
	if len(sig.ConstNames) > 0 {
		sc := newCtx(a, a.funcs, a.entities, a.methods, a.aliases, a.interfaces, a.enums, "", ir.Type{}, false, a.resolvedConsts, a.resolvingConsts)
		for i, cname := range sig.ConstNames {
			val, err := resolveEntityConstant(sc, ent.Name, cname, ent.Line)
			if err != nil {
				return nil, fmt.Errorf("константа %q: %w", cname, err)
			}
			var doc string
			if i < len(sig.ConstDocs) {
				doc = sig.ConstDocs[i]
			}
			constants = append(constants, ir.Constant{Name: cname, Value: val, Type: ir.TypeOf(val), Doc: doc})
		}
	}
	irMethods := make([]*ir.Method, len(ent.Methods))
	for i, m := range ent.Methods {
		irM, err := a.analyzeMethod(ent, m)
		if err != nil {
			return nil, fmt.Errorf("метод %q: %w", m.Name, err)
		}
		irMethods[i] = irM
	}
	var ctor *ir.Method
	if ent.Ctor != nil {
		irCtor, err := a.analyzeConstructor(ent)
		if err != nil {
			return nil, fmt.Errorf("конструктор: %w", err)
		}
		ctor = irCtor
	}
	return &ir.Entity{Name: ent.Name, IsStruct: ent.IsStruct, BaseName: ent.BaseName, Interfaces: ent.Interfaces, Constants: constants, Fields: fields, Methods: irMethods, Ctor: ctor, Doc: ent.Doc, Exported: ent.Exported}, nil
}

// analyzeConstructor анализирует явное тело конструктора ent.Ctor - как
// analyzeMethod (тот же ctx с ent.Name как thisEntity, тот же путь
// параметр-объявление/тело), но без типа результата (конструктор не
// возвращает значение - объект уже выделен вызывающей стороной backend'а)
// и с ручным проходом по инструкциям верхнего уровня тела, а не через
// analyzeBlock: sc.allowParentCall включается ровно на инструкцию с
// индексом 0 - см. package doc ctx.allowParentCall/ast.ParentCallStmt.
func (a *Analyzer) analyzeConstructor(ent *ast.EntityDecl) (*ir.Method, error) {
	c := ent.Ctor
	sig := a.entities[ent.Name]
	sc := newCtx(a, a.funcs, a.entities, a.methods, a.aliases, a.interfaces, a.enums, ent.Name, ir.Type{}, false, a.resolvedConsts, a.resolvingConsts)
	params := make([]ir.Param, len(c.Params))
	for i, p := range c.Params {
		params[i] = ir.Param{Name: p.Name, Type: sig.CtorParams[i]}
		if err := sc.declare(p.Name, sig.CtorParams[i]); err != nil {
			return nil, fmt.Errorf("строка %d: параметр %q: %w", p.Line, p.Name, err)
		}
	}
	body := make([]ir.Stmt, len(c.Body))
	for i, st := range c.Body {
		sc.allowParentCall = i == 0
		irst, err := analyzeStmt(sc, st)
		if err != nil {
			return nil, err
		}
		body[i] = irst
	}
	return &ir.Method{Params: params, Body: body, File: ent.File}, nil
}

func (a *Analyzer) analyzeMethod(ent *ast.EntityDecl, m *ast.MethodDecl) (*ir.Method, error) {
	sig := a.methods[ent.Name][m.Name]
	sc := newCtx(a, a.funcs, a.entities, a.methods, a.aliases, a.interfaces, a.enums, ent.Name, sig.ReturnType, sig.HasReturn, a.resolvedConsts, a.resolvingConsts)
	params, body, sig, err := a.analyzeParamsAndBody(sc, m.Params, sig, m.Body)
	if err != nil {
		return nil, err
	}
	if sig.HasReturn && !endsWithReturnValue(body) {
		return nil, fmt.Errorf("с объявленным типом результата %s должен заканчиваться \"вернуть <значение>\"", sig.ReturnType)
	}
	sig.GuaranteedThisStates = sc.thisEstablished
	a.methods[ent.Name][m.Name] = sig
	return &ir.Method{
		Name:       m.Name,
		Params:     params,
		ReturnType: sig.ReturnType,
		HasReturn:  sig.HasReturn,
		Body:       body,
		File:       ent.File,
		Doc:        m.Doc,
	}, nil
}

func (a *Analyzer) analyzeFunction(fn *ast.FunctionDecl) (*ir.Function, error) {
	sig := a.funcs[fn.Name]
	sc := newCtx(a, a.funcs, a.entities, a.methods, a.aliases, a.interfaces, a.enums, "", sig.ReturnType, sig.HasReturn, a.resolvedConsts, a.resolvingConsts)
	params, body, sig, err := a.analyzeParamsAndBody(sc, fn.Params, sig, fn.Body)
	if err != nil {
		return nil, err
	}
	if sig.HasReturn && !endsWithReturnValue(body) {
		return nil, fmt.Errorf("с объявленным типом результата %s должна заканчиваться \"вернуть <значение>\"", sig.ReturnType)
	}
	return &ir.Function{
		Name:       fn.Name,
		Params:     params,
		ReturnType: sig.ReturnType,
		HasReturn:  sig.HasReturn,
		Body:       body,
		File:       fn.File,
		Doc:        fn.Doc,
		Exported:   fn.Exported,
	}, nil
}

func (a *Analyzer) buildIR(prog *ast.Program) (*ir.Program, error) {
	out := &ir.Program{ExternFuncs: a.externFuncs}
	for _, iface := range prog.Interfaces {
		sig := a.interfaces[iface.Name]
		irIface := &ir.Interface{Name: iface.Name}
		for i, name := range sig.MethodNames {
			m := sig.MethodSigs[i]
			irIface.Methods = append(irIface.Methods, ir.InterfaceMethod{
				Name: name, Params: paramsFromTypes(iface.Methods[i].Params, m.Params),
				ReturnType: m.ReturnType, HasReturn: m.HasReturn,
			})
		}
		out.Interfaces = append(out.Interfaces, irIface)
	}
	// declaredEntityNames - имена, уже получившие настоящее ir.Entity в
	// цикле ниже (обычные сущности программы, включая раскрытые
	// generic-инстанциации - instantiateGenerics добавляет их в
	// prog.Entities как обычные объявления) - исключены из скана
	// "generic-имя в a.entities" дальше по функции: тот скан - только для
	// ВСТРОЕННЫХ generic-коллекций (Список<T>/Стопка<T>/Очередь<T>), у
	// которых нет собственного ast.EntityDecl вовсе (их C-представление -
	// строковый шаблон, cbackend.listCollectionCCode и т.п., не обычная
	// структура полей) - пользовательская generic-сущность ("Коробка<T>")
	// уже получила настоящий ir.Entity здесь же, второй раз через тот
	// механизм её эмитить не нужно (и нельзя - имя шаблона "Коробка" не
	// входит в курируемый список cbackend, генерация упала бы).
	declaredEntityNames := make(map[string]bool, len(prog.Entities))
	for _, ent := range prog.Entities {
		irEnt, err := a.analyzeEntity(ent)
		if err != nil {
			return nil, fmt.Errorf("сущность %q: %w", ent.Name, err)
		}
		out.Entities = append(out.Entities, irEnt)
		declaredEntityNames[ent.Name] = true
	}
	// Запрос/Ответ - структуры (по значению): простые поля-контейнеры без
	// методов и без сценария разделяемой мутации, копирование - естественное
	// и более предсказуемое поведение, чем ссылка на общий буфер.
	out.Entities = append(out.Entities,
		&ir.Entity{Name: "Запрос", IsStruct: true, Fields: httpRequestFields},
		&ir.Entity{Name: "Ответ", IsStruct: true, Fields: httpResponseFields},
		&ir.Entity{Name: "ДатаВремя", IsStruct: true, Fields: dateTimeFields},
	)
	// "Исключение" - ссылочная сущность (не структура): "выбросить"
	// передаёт указатель на неё дальше по стеку вызовов без копирования,
	// как и любую другую сущность/класс.
	out.Entities = append(out.Entities,
		&ir.Entity{Name: "Исключение", Fields: []ir.Field{{Name: "сообщение", Type: ir.TypeString}}},
	)
	for _, fn := range prog.Functions {
		irFn, err := a.analyzeFunction(fn)
		if err != nil {
			return nil, fmt.Errorf("функция %q: %w", fn.Name, err)
		}
		out.Functions = append(out.Functions, irFn)
	}

	names := make([]string, 0, len(a.entities))
	for name := range a.entities {
		if strings.Contains(name, "<") && !declaredEntityNames[name] {
			names = append(names, name)
		}
	}
	names = topoSortGenericNames(names, a.entities)
	for _, name := range names {
		template, _, _ := parseGenericTypeName(name)
		out.Generics = append(out.Generics, ir.GenericInstantiation{
			Name:             name,
			Template:         template,
			ElemType:         a.entities[name].ElemType,
			KeyType:          a.entities[name].KeyType,
			NeedsSafeIndex:   a.indexedCollections[name],
			NeedsListLiteral: a.listLiteralCollections[name],
		})
	}

	// "?Тип" - зарегистрированы resolveNullableTypeName тем же принципом
	// ленивой мемоизации, отдельным сканом (ключ - "?..." строкой, не
	// содержит "<" сам по себе, если только внутренний тип - не generic-
	// коллекция).
	var nullableNames []string
	for name, sig := range a.entities {
		if sig.IsNullable {
			nullableNames = append(nullableNames, name)
		}
	}
	slices.Sort(nullableNames)
	for _, name := range nullableNames {
		out.Nullables = append(out.Nullables, ir.NullableInstantiation{
			Name:  name,
			Inner: a.entities[name].ElemType,
		})
	}

	// "ОшибкаРазбора" - в отличие от Запрос/Ответ/ДатаВремя/Исключение выше,
	// НЕ добавлена безусловно - зарегистрирована в a.entities только если
	// программа хоть раз использовала "разобрать JSON" (см.
	// registerParseErrorEntity) - добавление её в out.Entities для КАЖДОЙ
	// программы сделало бы "Исключение" безусловным участником
	// диспетчеризации (computeDispatch) даже там, где исключения вообще не
	// используются.
	if _, ok := a.entities[parseErrorEntity]; ok {
		out.Entities = append(out.Entities, &ir.Entity{
			Name: parseErrorEntity, BaseName: systemExceptionEntity,
			Fields: []ir.Field{{Name: "сообщение", Type: ir.TypeString}},
		})
	}
	out.ParseTargets = a.parseTargets.order
	out.SerializeTargets = a.serializeTargets.order
	out.XMLParseTargets = a.xmlParseTargets.order
	out.XMLSerializeTargets = a.xmlSerializeTargets.order
	out.ReflectTargets = a.reflectTargets.order
	out.UsesReflection = a.usesReflection

	if err := a.collectCallables(out); err != nil {
		return nil, err
	}
	out.Callables = a.callableOrder
	return out, nil
}

// collectCallables регистрирует (см. registerCallable) сигнатуры
// "Вызываемый", объявленные прямо в сигнатуре программы -
// параметр/поле/тип результата функции, метода или конструктора сущности,
// метода интерфейса - независимо от того, построено ли где-то в программе
// хоть одно значение именно такой сигнатуры (analyzeFuncRefExpr/
// analyzeLambdaExpr регистрируют свои сигнатуры сами, в момент построения
// значения, но объявленный, а не построенный, тип без этого обхода остался
// бы без сгенерированной под него C-структуры).
func (a *Analyzer) collectCallables(prog *ir.Program) error {
	registerParams := func(params []ir.Param) error {
		for _, p := range params {
			if err := a.registerCallable(p.Type); err != nil {
				return err
			}
		}
		return nil
	}
	registerFunc := func(params []ir.Param, returnType ir.Type, hasReturn bool) error {
		if err := registerParams(params); err != nil {
			return err
		}
		if hasReturn {
			return a.registerCallable(returnType)
		}
		return nil
	}
	for _, fn := range prog.Functions {
		if err := registerFunc(fn.Params, fn.ReturnType, fn.HasReturn); err != nil {
			return err
		}
	}
	for _, ent := range prog.Entities {
		for _, f := range ent.Fields {
			if err := a.registerCallable(f.Type); err != nil {
				return err
			}
		}
		for _, m := range ent.Methods {
			if err := registerFunc(m.Params, m.ReturnType, m.HasReturn); err != nil {
				return err
			}
		}
		if ent.Ctor != nil {
			if err := registerParams(ent.Ctor.Params); err != nil {
				return err
			}
		}
	}
	for _, iface := range prog.Interfaces {
		for _, m := range iface.Methods {
			if err := registerFunc(m.Params, m.ReturnType, m.HasReturn); err != nil {
				return err
			}
		}
	}
	return nil
}
