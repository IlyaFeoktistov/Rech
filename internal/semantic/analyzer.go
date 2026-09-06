package semantic

import (
	"fmt"
	"maps"
	"strings"

	"slices"

	"github.com/IlyaFeoktistov/Rech/internal/ast"
	"github.com/IlyaFeoktistov/Rech/internal/builtins"
	"github.com/IlyaFeoktistov/Rech/internal/dbscheme"
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

	// moduleConstExprs - имя -> ещё НЕ проанализированное выражение
	// константы уровня модуля (ast.Program.Consts, "константа <имя> =
	// <значение>" вне функции/сущности) - собрано целиком до анализа тел
	// (см. collectModuleConsts), поэтому функция может ссылаться на
	// константу, объявленную НИЖЕ по файлу. Разрешение в ir.Expr - лениво,
	// resolveModuleConstant, тем же кэшем resolvedConsts/resolvingConsts,
	// что и у констант уровня сущности (ключ - голое имя константы, без
	// точки - у EntityDecl.Constants ключ всегда "Сущность.Константа",
	// коллизий по формату быть не может).
	moduleConstExprs map[string]ast.Expr

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

	// nextStaticVarID - следующий уникальный номер для "статическая
	// переменная" (ir.VarDecl.StaticID) - общий на весь Analyze счётчик,
	// тем же принципом, что и lambdaCounter: два разных объявления "статическая
	// переменная x" (в разных функциях, либо даже в разных ветках одной)
	// обязаны получить разные хранилища у backend'а, см. package doc
	// ir.VarDecl.IsStatic.
	nextStaticVarID int

	// parseTargets/serializeTargets/xmlParseTargets/xmlSerializeTargets/
	// yamlParseTargets/yamlSerializeTargets - дедуплицированные по имени
	// типа списки целей "разобрать"/"сериализовать" (JSON/XML/YAML отдельно
	// - backend генерирует разные функции для одного и того же Rech-типа в
	// зависимости от формата) - тем же принципом, что callablesSeen/
	// callableOrder. См. dedupTypeList.
	parseTargets         dedupTypeList
	serializeTargets     dedupTypeList
	xmlParseTargets      dedupTypeList
	xmlSerializeTargets  dedupTypeList
	yamlParseTargets     dedupTypeList
	yamlSerializeTargets dedupTypeList

	// dbRowTargets - дедуплицированный список сущностей, встретившихся хоть
	// раз как элемент цели "запросить(...) как Список<Тип>" (db_rows.go) -
	// тем же принципом, что parseTargets (backend генерирует функцию
	// маппинга строки результата в Тип один раз на каждый встретившийся
	// Тип, не безусловно для каждой сущности программы).
	dbRowTargets dedupTypeList

	// reflectTargets - дедуплицированный список сущностей/структур,
	// встретившихся хоть раз как аргумент боксирования ("(Значение) x")
	// или как имя у "тип <Имя>"/"значение как <Имя>" - тем же принципом,
	// что parseTargets (см. package doc ir.Program.ReflectTargets).
	// Примитивы не попадают сюда - их дескрипторы cbackend эмитит всегда,
	// безусловно, но только если usesReflection ниже.
	reflectTargets dedupTypeList
	// usesReflection - см. package doc ir.Program.UsesReflection.
	usesReflection bool

	// usesDB - см. package doc ir.Program.UsesDB.
	usesDB bool
	// dbBackends - см. package doc ir.Program.DBBackends.
	dbBackends dbscheme.Set

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

func (a *Analyzer) registerParseTarget(t ir.Type)         { a.parseTargets.add(t) }
func (a *Analyzer) registerSerializeTarget(t ir.Type)     { a.serializeTargets.add(t) }
func (a *Analyzer) registerXMLParseTarget(t ir.Type)      { a.xmlParseTargets.add(t) }
func (a *Analyzer) registerXMLSerializeTarget(t ir.Type)  { a.xmlSerializeTargets.add(t) }
func (a *Analyzer) registerYAMLParseTarget(t ir.Type)     { a.yamlParseTargets.add(t) }
func (a *Analyzer) registerYAMLSerializeTarget(t ir.Type) { a.yamlSerializeTargets.add(t) }
func (a *Analyzer) registerDBRowTarget(t ir.Type)         { a.dbRowTargets.add(t) }

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

// markUsesDB взводит usesDB - см. package doc ir.Program.UsesDB.
func (a *Analyzer) markUsesDB() { a.usesDB = true }

// markDBConnString взводит dbBackends по literal-аргументу
// "подключиться_к_бд(...)" - lit == nil, если аргумент НЕ строковый
// литерал (переменная/выражение) - см. package doc ir.Program.DBBackends/
// dbscheme.Set.
func (a *Analyzer) markDBConnString(lit *ast.StringLiteral) {
	if lit == nil {
		a.dbBackends.Dynamic = true
		return
	}
	a.dbBackends.Mark(lit.Value)
}

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
	// nullableIntType - "?Целое", тип поля "код" ниже - зарегистрирована тем
	// же путём, что и любое другое "?Тип" встреченное в исходнике
	// (resolveNullableTypeName, синтезирует entitySig{IsNullable, ElemType}
	// при первой встрече) - здесь она встречена программно, не через текст
	// исходника, но регистрация нужна ровно та же (backend читает поле
	// "код" сущности "Исключение" точно так же, как любое другое nullable-
	// поле - см. systemExceptionCodeType).
	nullableIntType, err := resolveNullableTypeName(entities, methods, aliases, a.interfaces, "?Целое")
	if err != nil {
		return fmt.Errorf("внутренняя ошибка: %w", err)
	}
	entities["Исключение"] = entitySig{
		FieldNames:      []string{"сообщение", "код"},
		FieldTypes:      []ir.Type{ir.TypeString, nullableIntType},
		FieldVisibility: []ast.Visibility{ast.Public, ast.Public},
		// HasCtor/CtorParams - "Исключение" не остаётся без явного
		// конструктора, как обычная сущность без своего "конструктор(...)"
		// (docs/SPEC.md, "Явные конструкторы") - "новое Исключение(сообщение)"
		// должно продолжать работать без объявления пользователем ни
		// единой строки, тем же принципом, что у `Exception(String message)`
		// в Java/C# - см. Ctor у ir.Entity "Исключение" ниже (systemExceptionCtor).
		// "код" не входит в параметры конструктора (по умолчанию "Ничто",
		// docs/SPEC.md) - присваивается отдельно, полю, уже после создания
		// (`искл.код = 1`), как и любому другому обычному полю.
		HasCtor:        true,
		CtorParams:     []ir.Type{ir.TypeString},
		CtorParamNames: []string{"сообщение"},
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
			if sig.fieldIndex(f.Name) != -1 || sig.staticFieldIndex(f.Name) != -1 || sig.constIndex(f.Name) != -1 {
				return fmt.Errorf("сущность %q: поле %q объявлено более одного раза", ent.Name, f.Name)
			}
			typ, err := resolveTypeName(entities, methods, aliases, a.interfaces, f.Type)
			if err != nil {
				return fmt.Errorf("сущность %q: поле %q: %w", ent.Name, f.Name, err)
			}
			if elemEnt, ok := collectionRealEntityElem(entities, typ); ok {
				return fmt.Errorf("сущность %q: поле %q имеет тип %s - встроенная коллекция с сущностью %q элементом допустима только вне полей сущностей/структур (локальная переменная, параметр, тип результата функции), не полем другой сущности/структуры", ent.Name, f.Name, typ, elemEnt)
			}
			if ent.IsExternStruct && typ.Kind == ir.KindCallable {
				// "Вызываемый(...)" полем внешней структуры (WNDPROC-в-структуре
				// стиль, WNDCLASS.lpfnWndProc и т. п.) - НЕ реализовано (docs/
				// ROADMAP.md, "FFI: маршалинг колбэков") - в отличие от
				// параметра "внешняя функция" (isFFIType ниже её уже пускает,
				// тип результата - НЕ пускает, см. collectExternFuncSigs),
				// полю нужен голый C-указатель на функцию в самой
				// typedef-структуре (не {fn, env} по значению - это исказило бы
				// реальный layout C-структуры), а cbackend сегодня объявляет
				// ЛЮБОЕ поле-Callable только как {fn, env} - отдельная, более
				// крупная доработка IR/кодогенерации, сознательно отложена.
				return fmt.Errorf("внешняя структура %q: поле %q - \"Вызываемый(...)\" пока допустима только параметром \"внешняя функция\", не полем внешней структуры", ent.Name, f.Name)
			}
			if ent.IsExternStruct && !isFFIType(entities, methods, aliases, a.interfaces, typ) {
				return fmt.Errorf("внешняя структура %q: поле %q имеет тип %s - допустимы только примитивы (Целое/Действительное/Дробное/Логическое/Байт/Символ/Строка), внешние сущности (\"внешняя сущность\") и \"Указатель<T>\" над FFI-совместимым T - иначе реальный размер C-структуры был бы искажён", ent.Name, f.Name, typ)
			}
			if f.IsStatic {
				if ent.IsExternStruct {
					return fmt.Errorf("внешняя структура %q: статическое поле %q недопустимо - у внешней структуры разрешены только поля FFI-совместимого C-представления", ent.Name, f.Name)
				}
				sig.StaticFieldNames = append(sig.StaticFieldNames, f.Name)
				sig.StaticFieldTypes = append(sig.StaticFieldTypes, typ)
				sig.StaticFieldVisibility = append(sig.StaticFieldVisibility, f.Visibility)
				sig.StaticFieldDefaultExprs = append(sig.StaticFieldDefaultExprs, f.Default)
				continue
			}
			sig.FieldNames = append(sig.FieldNames, f.Name)
			sig.FieldTypes = append(sig.FieldTypes, typ)
			sig.FieldDocs = append(sig.FieldDocs, f.Doc)
			sig.FieldTags = append(sig.FieldTags, f.Tag)
			sig.FieldVisibility = append(sig.FieldVisibility, f.Visibility)
			sig.FieldDefaultExprs = append(sig.FieldDefaultExprs, f.Default)
		}
		if len(ent.States) > 0 {
			sig.States = map[string]string{}
			for _, st := range ent.States {
				sig.States[st.BecomeForm] = st.Name
				sig.StateNames = append(sig.StateNames, st.Name)
				if _, exists := entities[st.TypeName]; exists {
					return fmt.Errorf("сущность %q: производный тип состояния %q совпадает с именем уже объявленной сущности", ent.Name, st.TypeName)
				}
				if other, exists := aliases[st.TypeName]; exists && other != ent.Name {
					return fmt.Errorf("сущность %q: производный тип состояния %q уже используется сущностью %q", ent.Name, st.TypeName, other)
				}
				aliases[st.TypeName] = ent.Name
				aliasState[st.TypeName] = st.Name
			}
			sig.HasStateFlags = true
		}
		sig.IsStruct = ent.IsStruct
		sig.IsAbstract = ent.IsAbstract
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
			paramDefaults := make([]ast.Expr, len(m.Params))
			for i, p := range m.Params {
				typ, err := resolveTypeName(entities, methods, aliases, a.interfaces, p.Type)
				if err != nil {
					return fmt.Errorf("сущность %q: метод %q: параметр %q: %w", ent.Name, m.Name, p.Name, err)
				}
				params[i] = typ
				paramStates[i] = aliasState[p.Type]
				paramDefaults[i] = p.Default
			}
			revokes, err := resolveOpposesTag(m, ent.Methods, sig.States)
			if err != nil {
				return fmt.Errorf("сущность %q: %w", ent.Name, err)
			}
			msig := funcSig{Params: params, ParamStates: paramStates, ParamDefaults: paramDefaults, Visibility: m.Visibility, IsAbstract: m.IsAbstract, IsStatic: m.IsStatic, RevokesStates: revokes}
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
			ctorParamNames := make([]string, len(ent.Ctor.Params))
			ctorParamDefaults := make([]ast.Expr, len(ent.Ctor.Params))
			for i, p := range ent.Ctor.Params {
				typ, err := resolveTypeName(entities, methods, aliases, a.interfaces, p.Type)
				if err != nil {
					return fmt.Errorf("сущность %q: конструктор: параметр %q: %w", ent.Name, p.Name, err)
				}
				ctorParams[i] = typ
				ctorParamNames[i] = p.Name
				ctorParamDefaults[i] = p.Default
			}
			sig.HasCtor = true
			sig.CtorParams = ctorParams
			sig.CtorParamNames = ctorParamNames
			sig.CtorParamDefaults = ctorParamDefaults
		}

		// Наследование ("наследует")/интерфейсы ("реализует") -
		// сплющивает sig/ownMethods с базовой сущностью, если она есть, и
		// проверяет соответствие каждому объявленному интерфейсу - см.
		// internal/semantic/inheritance.go.
		flatSig, flatMethods, err := a.applyInheritance(entities, methods, ent, sig, ownMethods)
		if err != nil {
			return err
		}
		if !ent.IsAbstract {
			for name, msig := range flatMethods {
				if msig.IsAbstract {
					return fmt.Errorf("сущность %q: не реализован абстрактный метод %q - дайте ему тело или объявите саму сущность абстрактной", ent.Name, name)
				}
			}
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
	registerStringSplitFunc(funcs)
	registerRegexFunc(funcs)
	registerDateTimeFuncs(a.entities, funcs)
	registerCookieFuncs(a.entities, funcs)
	registerHTTPHeaderFunc(funcs)
	registerEnvFuncs(funcs)
	registerDBFuncs(funcs)
	registerTransactionFuncs(funcs)
	registerNetworkFuncs(a.entities, funcs)
	registerBytesIOFuncs(funcs)
	registerHTTPBytesFuncs(funcs)
	registerMultipartFunc(funcs)
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
		paramDefaults := make([]ast.Expr, len(fn.Params))
		for i, p := range fn.Params {
			typ, err := resolveTypeName(a.entities, a.methods, a.aliases, a.interfaces, p.Type)
			if err != nil {
				return fmt.Errorf("функция %q: параметр %q: %w", fn.Name, p.Name, err)
			}
			params[i] = typ
			paramStates[i] = a.aliasState[p.Type]
			paramDefaults[i] = p.Default
		}
		sig := funcSig{Params: params, ParamStates: paramStates, ParamDefaults: paramDefaults}
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
			if !isFFIType(entities, methods, aliases, a.interfaces, typ) {
				return nil, fmt.Errorf("%s: параметр %q имеет тип %s - у внешней функции допустимы только примитивы (Целое/Действительное/Дробное/Логическое/Байт/Символ/Строка), внешние сущности (\"внешняя сущность\"), \"Указатель<T>\" над FFI-совместимым T и \"Вызываемый(...)\" с FFI-совместимыми параметрами/результатом", label, p.Name, typ)
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
			if typ.Kind == ir.KindCallable {
				// "Вызываемый(...)" ТИПОМ РЕЗУЛЬТАТА внешней функции (сырой
				// C-указатель на функцию, приходящий ОТ C, например возврат
				// прежнего обработчика у signal()) - НЕ реализовано (docs/
				// ROADMAP.md, "FFI: маршалинг колбэков") - обратное
				// направление ("превратить чужой сырой указатель в
				// вызываемое Rech-значение") нуждается в отдельном,
				// анмаршалинг-trampoline, не в том же регистрационном
				// механизме, что и параметр (marshalCallbackArg,
				// ffi_callback.go) - сознательно отложено, параметром
				// "Вызываемый(...)" уже поддержана (ниже).
				return nil, fmt.Errorf("%s: тип результата - \"Вызываемый(...)\" пока допустима только ПАРАМЕТРОМ внешней функции, не типом результата", label)
			}
			if !isFFIType(entities, methods, aliases, a.interfaces, typ) {
				return nil, fmt.Errorf("%s: тип результата %s не поддерживается - у внешней функции допустимы только примитивы (Целое/Действительное/Дробное/Логическое/Байт/Символ/Строка), внешние сущности (\"внешняя сущность\") и \"Указатель<T>\" над FFI-совместимым T", label, typ)
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
// (StandardEnums, всегда присутствуют) плюс объявленные пользователем
// (prog.Enums, копия StandardEnums на входе - чтобы мутация ниже не
// затронула сам StandardEnums между вызовами Analyze). Дубли членов
// внутри одного перечисления уже проверены парсером (см. parseEnumDecl) -
// здесь только коллизии ИМЕНИ самого перечисления с сущностью или другим
// перечислением (в т. ч. встроенным).
func (a *Analyzer) collectEnums(prog *ast.Program) error {
	enums := make(map[string]map[string]int64, len(StandardEnums)+len(prog.Enums))
	for name, members := range StandardEnums {
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

// collectModuleConsts регистрирует имена констант уровня модуля
// (ast.Program.Consts) - только СОБИРАЕТ ast.Expr по имени, само значение
// разрешается лениво при первом обращении (resolveModuleConstant) - тем же
// принципом, что и константы уровня сущности (collectEntities не
// анализирует ConstExprs сразу). Вызывается после collectEntities/
// collectFuncSigs/collectEnums - все три таблицы уже готовы для проверки
// коллизии имени.
func (a *Analyzer) collectModuleConsts(prog *ast.Program) error {
	if len(prog.Consts) == 0 {
		return nil
	}
	consts := make(map[string]ast.Expr, len(prog.Consts))
	for _, c := range prog.Consts {
		if _, exists := consts[c.Name]; exists {
			return fmt.Errorf("строка %d: константа %q уровня модуля объявлена более одного раза", c.Line, c.Name)
		}
		if _, exists := a.entities[c.Name]; exists {
			return fmt.Errorf("строка %d: константа %q уровня модуля: имя совпадает с уже объявленной сущностью", c.Line, c.Name)
		}
		if _, exists := a.funcs[c.Name]; exists {
			return fmt.Errorf("строка %d: константа %q уровня модуля: имя совпадает с уже объявленной функцией", c.Line, c.Name)
		}
		if _, exists := a.enums[c.Name]; exists {
			return fmt.Errorf("строка %d: константа %q уровня модуля: имя совпадает с уже объявленным перечислением", c.Line, c.Name)
		}
		if _, exists := a.interfaces[c.Name]; exists {
			return fmt.Errorf("строка %d: константа %q уровня модуля: имя совпадает с уже объявленным интерфейсом", c.Line, c.Name)
		}
		consts[c.Name] = c.Value
	}
	a.moduleConstExprs = consts
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
	staticFields := make([]ir.Field, len(sig.StaticFieldNames))
	for i, name := range sig.StaticFieldNames {
		var def ir.Expr
		if i < len(sig.StaticFieldDefaultExprs) && sig.StaticFieldDefaultExprs[i] != nil {
			val, err := resolveStaticFieldDefault(fsc, ent.Name, name, ent.Line)
			if err != nil {
				return nil, fmt.Errorf("статическое поле %q: %w", name, err)
			}
			def = val
		}
		staticFields[i] = ir.Field{Name: name, Type: sig.StaticFieldTypes[i], Default: def}
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
	} else if !ent.IsStruct {
		// "как в нормальных языках" (docs/SPEC.md, "Явные конструкторы") -
		// у сущности/класса без своего "конструктор(...)" в исходнике всё
		// равно есть конструктор на уровне IR/backend'ов - синтетический,
		// без параметров и без тела. И cbackend (generateIREntityCtorInit),
		// и internal/interp (constructEntity) уже применяют "имеет: поле =
		// значение" и зануляют остальное ДО тела конструктора - с пустым
		// телом это ровно "новый Тип()" без аргументов, поля - только из
		// своих значений по умолчанию/нулей. "структура" не участвует - у
		// неё конструкторов нет вообще (generateIRNewExpr - compound
		// literal, см. её package doc), analyzeNewExpr держит её отдельной
		// веткой (sig.IsStruct).
		ctor = &ir.Method{}
	}
	return &ir.Entity{
		Name: ent.Name, IsStruct: ent.IsStruct, IsAbstract: ent.IsAbstract, BaseName: ent.BaseName, Interfaces: ent.Interfaces,
		Constants: constants, Fields: fields, StaticFields: staticFields, Methods: irMethods, Ctor: ctor, Doc: ent.Doc, Exported: ent.Exported,
		StateNames: sig.StateNames, StateBitOffset: sig.StateBitOffset, HasStateFlags: sig.HasStateFlags, Tag: ent.Tag,
	}, nil
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
	if !m.IsAbstract && sig.HasReturn && !endsWithReturnValue(body) {
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
		IsAbstract: m.IsAbstract,
		IsStatic:   m.IsStatic,
		File:       ent.File,
		Doc:        m.Doc,
		Tag:        m.Tag,
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
	out := &ir.Program{ExternFuncs: a.externFuncs, DBPoolSize: DefaultDBPoolSize, DBSQLiteBusyTimeoutMs: DefaultDBSQLiteBusyTimeoutMs, PoolShrinkEnabled: true, PoolIdleTimeoutMs: DefaultPoolIdleTimeoutMs, HTTPMaxRequestBytes: DefaultHTTPMaxRequestBytes}
	for _, iface := range prog.Interfaces {
		sig := a.interfaces[iface.Name]
		// sig.IRMethods - уже готовые ir.InterfaceMethod, включая
		// унаследованные через "наследует" (см. package doc
		// interfaceSig.IRMethods, collectInterfaces) - не переизвлекаются
		// из iface.Methods здесь.
		out.Interfaces = append(out.Interfaces, &ir.Interface{Name: iface.Name, Methods: sig.IRMethods})
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
		&ir.Entity{Name: "ОпцииКуки", IsStruct: true, Fields: cookieOptionsFields},
		&ir.Entity{Name: "Дейтаграмма", IsStruct: true, Fields: datagramFields},
	)
	// "Исключение" - ссылочная сущность (не структура): "выбросить"
	// передаёт указатель на неё дальше по стеку вызовов без копирования,
	// как и любую другую сущность/класс. Порядок в out.Entities (после
	// пользовательских) не создаёт проблему для "родитель(...)" в
	// пользовательском наследнике - её C-прототип (rech_init_isklyuchenie)
	// эмитится в общий "protos" раньше вообще всех тел (см. package doc
	// internal/backend/cbackend/ir_generator.go, generateSource), только
	// само ОПРЕДЕЛЕНИЕ идёт позже, порядок определений здесь значения не
	// имеет.
	out.Entities = append(out.Entities,
		// Ctor - НЕ пустой синтетический (в отличие от пользовательских
		// сущностей без своего "конструктор(...)", см. analyzeEntity выше) -
		// "новое Исключение(сообщение)" продолжает работать без единой
		// строки пользовательского кода, тем же принципом, что и у
		// `Exception(String message)` в Java/C# - см. systemExceptionCtor.
		&ir.Entity{Name: "Исключение", Fields: systemExceptionFields(), Ctor: systemExceptionCtor()},
	)
	for _, fn := range prog.Functions {
		irFn, err := a.analyzeFunction(fn)
		if err != nil {
			return nil, fmt.Errorf("функция %q: %w", fn.Name, err)
		}
		out.Functions = append(out.Functions, irFn)
	}

	names := make([]string, 0, len(a.entities))
	for name, sig := range a.entities {
		// IsNullable/IsPointer - тоже синтезированные записи, имя которых
		// может содержать "<" (внутренний тип - сам generic-коллекция или
		// сама "Указатель<T>"), но у них собственные, отдельные списки
		// (out.Nullables/out.Pointers, ниже) - не Program.Generics, у них
		// нет ни typedef, ни методов growable-коллекции.
		if strings.Contains(name, "<") && !declaredEntityNames[name] && !sig.IsNullable && !sig.IsPointer {
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

	// "Указатель<T>" - зарегистрированы resolvePointerTypeName, тот же
	// принцип отдельного скана, что и у nullableNames выше.
	var pointerNames []string
	for name, sig := range a.entities {
		if sig.IsPointer {
			pointerNames = append(pointerNames, name)
		}
	}
	slices.Sort(pointerNames)
	for _, name := range pointerNames {
		out.Pointers = append(out.Pointers, ir.PointerInstantiation{
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
			Fields: systemExceptionFields(),
			// Ctor - тот же принцип, что и у "Исключение" (systemExceptionCtor) -
			// rech_struct_throw (internal/backend/cbackend/json_extract.go)
			// вызывает mangleCtor(parseErrorEntity) с одним аргументом
			// (сообщением) напрямую, минуя "новый" - конструктор обязан
			// принимать ровно этот один параметр.
			Ctor: systemExceptionCtor(),
		})
	}
	// "ОшибкаДеленияНаНоль" - тот же принцип лениво-условной регистрации,
	// что и "ОшибкаРазбора" выше (см. package doc divByZeroExceptionEntity,
	// registerDivByZeroException).
	if _, ok := a.entities[divByZeroExceptionEntity]; ok {
		out.Entities = append(out.Entities, &ir.Entity{
			Name: divByZeroExceptionEntity, BaseName: systemExceptionEntity,
			Fields: systemExceptionFields(),
			// Ctor - тот же принцип, что и у "ОшибкаРазбора" выше -
			// internal/backend/cbackend/builtins_shared.go
			// (rech_int_div<N>/rech_uint_div<N>/...mod<N>, через
			// rech_divzero_throw) вызывает mangleCtor(divByZeroExceptionEntity)
			// с одним аргументом (сообщением) напрямую.
			Ctor: systemExceptionCtor(),
		})
	}
	// "Совпадение" - тот же принцип лениво-условной регистрации, что и
	// "ОшибкаРазбора"/"ОшибкаДеленияНаНоль" выше - добавлена в out.Entities,
	// только если программа хоть раз упомянула "Совпадение" (вызовом
	// "регулярное" или явной аннотацией типа, см. её case в
	// resolveTypeName). Её собственное поле "группы" (Список<Строка>) не
	// требует особого порядка относительно других typedef сущностей -
	// generic-коллекция примитива (не сущности) эмитится в отдельный блок
	// ДО всех typedef сущностей (genericsCode, cbackend/ir_generator.go),
	// независимо от места regex.go в этом списке.
	if _, ok := a.entities["Совпадение"]; ok {
		out.Entities = append(out.Entities, &ir.Entity{Name: "Совпадение", IsStruct: true, Fields: matchFields})
	}
	// "ЧастьФормы" - тот же принцип, что "Совпадение" выше.
	if _, ok := a.entities[formPartType]; ok {
		out.Entities = append(out.Entities, &ir.Entity{Name: formPartType, IsStruct: true, Fields: formPartFields})
	}
	// "ЗапросБайты"/"ОтветБайты" - тот же принцип, что "Совпадение" выше -
	// см. package doc httpbytes.go про то, почему это отдельные сущности, а
	// не поле в уже существующих Запрос/Ответ.
	if _, ok := a.entities[httpBytesRequestType]; ok {
		out.Entities = append(out.Entities, &ir.Entity{Name: httpBytesRequestType, IsStruct: true, Fields: httpBytesRequestFields})
	}
	if _, ok := a.entities[httpBytesResponseType]; ok {
		out.Entities = append(out.Entities, &ir.Entity{Name: httpBytesResponseType, IsStruct: true, Fields: httpBytesResponseFields})
	}
	// "Блокировка" - проверка ЗДЕСЬ (не раньше цикла по prog.Entities/
	// prog.Functions выше), потому что она может встретиться впервые не
	// только как тип поля (это уже зарегистрировало бы её в a.entities к
	// началу этой функции, через collectEntities), но и внутри тела метода/
	// функции/конструктора ("переменная м = новая Блокировка()" без единого
	// поля такого типа нигде) - тела анализируются как раз в циклах выше.
	// В отличие от "ОшибкаРазбора" (просто append, ссылка на неё не бывает
	// типом ПОЛЯ другой сущности) - "Блокировка" ставится ПЕРВОЙ в
	// out.Entities (не последней): если сущность, объявленная в файле
	// раньше, несёт поле типа "Блокировка", её C-структура (typedefs,
	// cbackend) ссылается на "rech_T_blokirovka" - этот typedef обязан идти
	// раньше в файле, что гарантирует только позиция в самом срезе
	// (порядок конструкторов той же строкой не важен - см. mutexPrototypes,
	// internal/backend/cbackend/mutex.go, отдельная защита независимо от
	// текстовой позиции).
	if sig, ok := a.entities[mutexEntityName]; ok && sig.IsMutex {
		out.Entities = append([]*ir.Entity{mutexIRType()}, out.Entities...)
	}
	// "Семафор" - та же причина/тот же приём, что у "Блокировки" чуть выше
	// (см. её комментарий) - собственная проверка, отдельная от неё, чтобы
	// программа с ОДНОЙ только "Блокировкой" не платила бы за "Семафор" и
	// наоборот.
	if sig, ok := a.entities[semaphoreEntityName]; ok && sig.IsSemaphore {
		out.Entities = append([]*ir.Entity{semaphoreIRType()}, out.Entities...)
	}
	// "Транзакция" - та же причина/тот же приём, что у "Семафора"/
	// "Блокировки" выше.
	if sig, ok := a.entities[transactionEntityName]; ok && sig.IsTransaction {
		out.Entities = append([]*ir.Entity{transactionIRType()}, out.Entities...)
	}
	out.ParseTargets = a.parseTargets.order
	out.SerializeTargets = a.serializeTargets.order
	out.XMLParseTargets = a.xmlParseTargets.order
	out.XMLSerializeTargets = a.xmlSerializeTargets.order
	out.YAMLParseTargets = a.yamlParseTargets.order
	out.YAMLSerializeTargets = a.yamlSerializeTargets.order
	out.ReflectTargets = a.reflectTargets.order
	// InterfaceImplementors - см. package doc ir.Program.InterfaceImplementors -
	// для КАЖДОГО объявленного интерфейса программы (не только
	// зарегистрированных как ReflectTargets - дёшево посчитать для всех),
	// entitySatisfies уже умеет транзитивно проверить и цепочку "наследует"
	// у сущности, и цепочку "наследует" у самого интерфейса
	// (interfaceIsOrExtends, interfaces.go) - тот же курируемый источник
	// истины, что уже используется для проверки "реализует" на этапе
	// компиляции, не отдельная параллельная реализация.
	if len(out.Interfaces) > 0 {
		implementors := make(map[string][]string, len(out.Interfaces))
		// entityInterfaces - обратный индекс к implementors (имя сущности ->
		// имена интерфейсов, которые она реализует) - см. package doc
		// ir.Program.EntityInterfaces - строится в ТОМ ЖЕ проходе, entitySatisfies
		// уже отвечает "да" транзитивно (и через "наследует" у сущности, и
		// через "наследует" МЕЖДУ интерфейсами), поэтому "Кот реализует
		// Звучащий" (Звучащий наследует Именуемый) корректно засчитывает и
		// "Именуемый" тоже - в отличие от ent.Interfaces (AST-уровня,
		// собственный ОДНОУРОВНЕВЫЙ список "реализует", без раскрытия цепочки
		// "наследует" интерфейса, нужный для ДРУГИХ целей - vtable.go,
		// computeDispatch/buildIsaTable).
		entityInterfaces := make(map[string][]string, len(out.Entities))
		for _, iface := range out.Interfaces {
			for _, ent := range out.Entities {
				if entitySatisfies(a.entities, a.interfaces, ent.Name, iface.Name) {
					implementors[iface.Name] = append(implementors[iface.Name], ent.Name)
					entityInterfaces[ent.Name] = append(entityInterfaces[ent.Name], iface.Name)
				}
			}
		}
		out.InterfaceImplementors = implementors
		out.EntityInterfaces = entityInterfaces
	}
	out.UsesReflection = a.usesReflection
	out.UsesDB = a.usesDB
	out.DBBackends = a.dbBackends
	out.DBRowTargets = a.dbRowTargets.order

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
	// prog.ExternFuncs - параметр "Вызываемый(...)" (docs/ROADMAP.md, "FFI:
	// маршалинг колбэков") обязан попасть в Program.Callables ДАЖЕ если эта
	// внешняя функция нигде фактически не вызвана (externPrototypeOfIR,
	// cbackend, эмитит прототип КАЖДОЙ объявленной внешней функции
	// безусловно - ей нужна собственная сигнатура trampoline'а независимо
	// от того, есть ли реальный вызов). ExternFuncDecl.Params - только
	// типы, нет structuры Param{Name, Type} - registerCallable вызывается
	// напрямую, не через registerParams/registerFunc (тип результата не
	// регистрируется - "Вызываемый(...)" типом результата внешней функции
	// запрещена семантикой раньше, collectExternFuncSigs).
	for _, ext := range prog.ExternFuncs {
		for _, p := range ext.Params {
			if err := a.registerCallable(p); err != nil {
				return err
			}
		}
	}
	return nil
}
