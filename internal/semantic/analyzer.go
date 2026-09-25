package semantic

import (
	"fmt"
	"maps"
	"strings"

	"slices"

	"github.com/IlyaFeoktistov/Rech/internal/ast"
	"github.com/IlyaFeoktistov/Rech/internal/builtins"
	"github.com/IlyaFeoktistov/Rech/internal/dbscheme"
	"github.com/IlyaFeoktistov/Rech/internal/diagnostics"
	"github.com/IlyaFeoktistov/Rech/internal/ir"
)

// Analyzer инкапсулирует состояние семантического анализа.
type Analyzer struct {
	// noFlowChecks - не проверять недостижимый код и "вернуть" на каждом
	// пути (AnalyzeForCompletion: тело обрезано у курсора).
	noFlowChecks bool
	// inference - вывод аргументов типа (generic_infer.go), общий для всех
	// проходов анализа одной программы.
	inference *genericInference
	entities  map[string]entitySig
	methods   map[string]map[string]funcSig
	funcs     map[string]funcSig
	// funcOverloads - имя перегруженной свободной функции -> её сигнатуры
	// (funcSig.MangledName у каждой уже заполнено) - пусто (nil-запись) для
	// любого имени без перегрузки, funcs хранит его сигнатуру напрямую под
	// голым именем, как и раньше (см. package doc funcSig.MangledName,
	// resolveFuncSigForCall).
	funcOverloads map[string][]funcSig
	// funcDeclNames - *ast.FunctionDecl -> ключ, под которым его сигнатура
	// зарегистрирована в funcs (голое имя - без перегрузки этого имени,
	// иначе MangledName) - собственный анализ тела функции (analyzeFunction)
	// не может просто взять funcs[fn.Name]: при перегрузке под голым именем
	// в funcs нет записи вовсе, а с каким именно объявлением (fn) из группы
	// сейчас работает вызывающая сторона, funcs по одному голому имени не
	// определить.
	funcDeclNames map[*ast.FunctionDecl]string
	// methodOverloads - имя сущности -> имя метода -> её сигнатуры
	// (funcSig.MangledName у каждой уже заполнено) - ТРАНЗИТИВНО сплющенные
	// с перегрузками БАЗОВОЙ сущности (applyInheritance), тем же принципом,
	// что и methods[Сущность] в целом. Пусто (нет записи под именем метода)
	// для любого имени без перегрузки - methods[Сущность][имя] хранит
	// сигнатуру напрямую под голым именем, как и раньше.
	methodOverloads map[string]map[string][]funcSig
	// methodDeclNames - *ast.MethodDecl -> ключ, под которым его сигнатура
	// зарегистрирована в methods[ent.Name] (голое имя без перегрузки этого
	// имени у ЭТОЙ сущности, иначе MangledName) - тот же принцип, что и
	// funcDeclNames у свободных функций (см. её package doc), нужен
	// analyzeMethod по той же причине: methods[ent.Name][m.Name] не годится
	// напрямую при перегрузке.
	methodDeclNames map[*ast.MethodDecl]string
	aliases         map[string]string
	aliasState      map[string]string
	enums           map[string]map[string]int64
	interfaces      map[string]interfaceSig
	externFuncs     []*ir.ExternFuncDecl

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
	// protobufParseTargets/protobufSerializeTargets - те же роли, что
	// yamlParseTargets/yamlSerializeTargets, для "разобрать PROTOBUF"/
	// "сериализовать PROTOBUF" (см. internal/semantic/protobuf.go).
	protobufParseTargets     dedupTypeList
	protobufSerializeTargets dedupTypeList

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

func (a *Analyzer) registerParseTarget(t ir.Type)             { a.parseTargets.add(t) }
func (a *Analyzer) registerSerializeTarget(t ir.Type)         { a.serializeTargets.add(t) }
func (a *Analyzer) registerXMLParseTarget(t ir.Type)          { a.xmlParseTargets.add(t) }
func (a *Analyzer) registerXMLSerializeTarget(t ir.Type)      { a.xmlSerializeTargets.add(t) }
func (a *Analyzer) registerYAMLParseTarget(t ir.Type)         { a.yamlParseTargets.add(t) }
func (a *Analyzer) registerYAMLSerializeTarget(t ir.Type)     { a.yamlSerializeTargets.add(t) }
func (a *Analyzer) registerProtobufParseTarget(t ir.Type)     { a.protobufParseTargets.add(t) }
func (a *Analyzer) registerProtobufSerializeTarget(t ir.Type) { a.protobufSerializeTargets.add(t) }
func (a *Analyzer) registerDBRowTarget(t ir.Type)             { a.dbRowTargets.add(t) }

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
//
// Ошибка ОДНОЙ сущности (collectOneEntity) не останавливает весь проход:
// собирается в bag, а сама сущность получает entitySig.Poisoned-заглушку
// вместо настоящей сигнатуры, и цикл переходит к следующей - тем же
// принципом, что и buildIR (см. её package doc), НО с дополнительной
// заглушкой вместо простого "не попадает в out": здесь, в отличие от
// buildIR, последующие сущности программы ЗАВИСят от того, что имя уже
// зарегистрировано (поле/параметр может ссылаться на сломанную сущность
// как на тип) - без заглушки такая ссылка дала бы вторую, вводящую в
// заблуждение диагностику "необъявленное имя" вдобавок к настоящей
// причине. Если bag непуст к концу прохода, дальнейшие фазы Analyze
// (collectFuncSigs и т.д.) не запускаются вовсе - заглушки существуют
// только чтобы не портить диагностику ЭТОГО прохода, не как основа для
// продолжения компиляции.
func (a *Analyzer) collectEntities(prog *ast.Program) error {
	entities := map[string]entitySig{}
	methods := map[string]map[string]funcSig{}
	aliases := map[string]string{}
	aliasState := map[string]string{}
	a.methodOverloads = map[string]map[string][]funcSig{}
	a.methodDeclNames = map[*ast.MethodDecl]string{}
	// enums - собран здесь, а не позже (a.enums, collectEnums идёт ПОСЛЕ
	// collectEntities) - тем же приёмом-плейсхолдером, что paramEntities у
	// collectInterfaces: полю/параметру/типу результата метода сущности
	// нужно видеть имена перечислений уже сейчас, полная, с проверкой
	// коллизий, сборка a.enums по-прежнему происходит только в collectEnums.
	enums := buildEnumTable(prog)
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
	nullableIntType, err := resolveNullableTypeName(entities, methods, aliases, a.interfaces, enums, "?Целое")
	if err != nil {
		return diagnostics.Wrap(err, "внутренняя ошибка")
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
	registerPanicEntities(entities, methods)
	registerErrorEntities(entities, methods)
	// bag - собирает диагностики НЕЗАВИСИМЫХ объявлений (и "внешняя
	// сущность" ниже, и обычных сущностей дальше) за один прогон, вместо
	// остановки на первой - тем же принципом, что и buildIR (см. её package
	// doc). "Независимых" - в смысле их СОБСТВЕННЫХ ошибок объявления
	// (дублирующееся имя, недопустимый тип поля и т.п.): сигнатура ОДНОЙ
	// сущности по-прежнему может быть НУЖНА для проверки другой
	// (наследование, поле-тип) - именно поэтому упавшее объявление не
	// просто пропускается, а получает entitySig.Poisoned-заглушку в
	// entities/methods (см. collectOneEntity ниже) - ссылка на неё из
	// корректного объявления резолвится как обычно, не порождая вторую,
	// вводящую в заблуждение ошибку "необъявленное имя" вдобавок к
	// настоящей причине.
	bag := diagnostics.NewBag()
	// "внешняя сущность" - непрозрачные внешние типы (FFI) - регистрируются
	// раньше пользовательских сущностей ниже той же коллизией имён
	// (entities/aliases/interfaces): у них нет полей/методов, resolveTypeName
	// узнаёт их по entitySig.IsExternOpaque и резолвит в ir.ExternEntityType,
	// не ir.EntityType.
	for _, extEnt := range prog.ExternEntities {
		var err error
		switch {
		case builtins.ReservedEntityNames[extEnt.Alias]:
			err = errNoPos(CodeEntityDecl, "внешняя сущность %q - зарезервированное имя встроенной сущности, нельзя объявить с этим именем", extEnt.Alias)
		default:
			if _, exists := entities[extEnt.Alias]; exists {
				err = errNoPos(CodeDuplicateDecl, "внешняя сущность %q: имя уже занято", extEnt.Alias)
			} else if other, exists := aliases[extEnt.Alias]; exists {
				err = errNoPos(CodeDuplicateDecl, "внешняя сущность %q: имя совпадает с производным типом состояния, уже занятым сущностью %q", extEnt.Alias, other)
			} else if _, exists := a.interfaces[extEnt.Alias]; exists {
				err = errNoPos(CodeDuplicateDecl, "внешняя сущность %q: имя совпадает с уже объявленным интерфейсом", extEnt.Alias)
			}
		}
		if err != nil {
			bag.AddErr(err)
			if _, exists := entities[extEnt.Alias]; !exists {
				entities[extEnt.Alias] = entitySig{IsExternOpaque: true, Poisoned: true}
			}
			continue
		}
		entities[extEnt.Alias] = entitySig{IsExternOpaque: true}
	}
	for _, ent := range prog.Entities {
		currentFile = ent.File
		if err := a.collectOneEntity(entities, methods, aliases, aliasState, enums, ent); err != nil {
			// err уже несёт "сущность %q: ..." (каждый errNoPos/diagnostics.Wrap
			// внутри collectOneEntity формирует его сам) - здесь оборачивать
			// повторно не нужно, в отличие от buildIR (см. её package doc),
			// чьи вызовы analyzeEntity - универсальные, без такого префикса.
			bag.AddErr(err)
			// Заглушка - см. package doc bag выше и entitySig.Poisoned.
			// "Только если ещё не зарегистрирована" - для ошибки "уже
			// объявлена" запись под этим именем принадлежит РАНЕЕ
			// обработанной, настоящей сущности - затирать её нельзя.
			if _, exists := entities[ent.Name]; !exists {
				entities[ent.Name] = entitySig{Poisoned: true}
			}
			if _, exists := methods[ent.Name]; !exists {
				methods[ent.Name] = map[string]funcSig{}
			}
			if _, exists := a.methodOverloads[ent.Name]; !exists {
				a.methodOverloads[ent.Name] = map[string][]funcSig{}
			}
		}
	}
	if bag.Len() > 0 {
		return bag.AsError()
	}
	a.entities = entities
	a.methods = methods
	a.aliases = aliases
	a.aliasState = aliasState
	return nil
}

// collectOneEntity разбирает ОДНУ сущность ent в готовую (сплющенную по
// инвариантам наследования) сигнатуру - вынесена из collectEntities, чтобы
// вызывающая сторона могла поймать ошибку, поставить entitySig.Poisoned-
// заглушку вместо остановки всего прохода и перейти к следующей сущности
// (см. package doc collectEntities про bag/poison-маркеры). Пишет в
// entities/methods НАПРЯМУЮ по ходу разбора, тем же принципом, что и раньше
// (единый цикл в collectEntities): в частности, промежуточная, ещё не
// сплющенная запись entities[ent.Name] (см. комментарий у неё ниже)
// остаётся видна ДРУГИМ вызовам этой же функции для последующих сущностей.
func (a *Analyzer) collectOneEntity(entities map[string]entitySig, methods map[string]map[string]funcSig, aliases map[string]string, aliasState map[string]string, enums map[string]map[string]int64, ent *ast.EntityDecl) error {
	if builtins.ReservedEntityNames[ent.Name] && !builtins.ShadowableEntityNames[ent.Name] {
		return errNoPos(CodeEntityDecl, "сущность %q - зарезервированное имя встроенной сущности, нельзя объявить свою сущность с этим именем", ent.Name)
	}
	if _, exists := entities[ent.Name]; exists {
		return errNoPos(CodeDuplicateDecl, "сущность %q объявлена более одного раза", ent.Name)
	}
	if other, exists := aliases[ent.Name]; exists {
		return errNoPos(CodeDuplicateDecl, "сущность %q: имя совпадает с производным типом состояния, уже занятым сущностью %q", ent.Name, other)
	}
	if _, exists := a.interfaces[ent.Name]; exists {
		return errNoPos(CodeDuplicateDecl, "сущность %q: имя совпадает с уже объявленным интерфейсом", ent.Name)
	}
	if err := a.checkGenericConstraints(entities, methods, aliases, enums, ent.Name); err != nil {
		return err
	}
	var sig entitySig

	// Собираем константы, если есть
	for _, c := range ent.Constants {
		if sig.fieldIndex(c.Name) != -1 {
			return errNoPos(CodeDuplicateDecl, "сущность %q: константа %q конфликтует с полем с тем же именем", ent.Name, c.Name)
		}
		if sig.constIndex(c.Name) != -1 {
			return errNoPos(CodeDuplicateDecl, "сущность %q: константа %q объявлена более одного раза", ent.Name, c.Name)
		}
		sig.ConstNames = append(sig.ConstNames, c.Name)
		sig.ConstExprs = append(sig.ConstExprs, c.Value)
		sig.ConstDocs = append(sig.ConstDocs, c.Doc)
		sig.ConstVisibility = append(sig.ConstVisibility, c.Visibility)
	}

	for _, f := range ent.Fields {
		if sig.fieldIndex(f.Name) != -1 || sig.staticFieldIndex(f.Name) != -1 || sig.constIndex(f.Name) != -1 {
			return errNoPos(CodeDuplicateDecl, "сущность %q: поле %q объявлено более одного раза", ent.Name, f.Name)
		}
		typ, err := resolveTypeName(entities, methods, aliases, a.interfaces, enums, f.Type)
		if err != nil {
			return diagnostics.Wrap(err, "сущность %q: поле %q", ent.Name, f.Name)
		}
		if ent.IsExternStruct && !isFFIType(entities, methods, aliases, a.interfaces, enums, typ) && !ffiCallableResult(entities, methods, aliases, a.interfaces, enums, typ) {
			return errNoPos(CodeEntityDecl, "внешняя структура %q: поле %q имеет тип %s - допустимы только примитивы (Целое/Целое8/16/32/64/Натуральное/Натуральное8/16/32/64/Действительное/Дробное/Логическое/Байт/Символ/Строка), внешние сущности (\"внешняя сущность\") и \"Указатель<T>\" над FFI-совместимым T - иначе реальный размер C-структуры был бы искажён", ent.Name, f.Name, typ)
		}
		if f.IsStatic {
			if ent.IsExternStruct {
				return errNoPos(CodeEntityDecl, "внешняя структура %q: статическое поле %q недопустимо - у внешней структуры разрешены только поля FFI-совместимого C-представления", ent.Name, f.Name)
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
				return errNoPos(CodeDuplicateDecl, "сущность %q: производный тип состояния %q совпадает с именем уже объявленной сущности", ent.Name, st.TypeName)
			}
			if other, exists := aliases[st.TypeName]; exists && other != ent.Name {
				return errNoPos(CodeEntityDecl, "сущность %q: производный тип состояния %q уже используется сущностью %q", ent.Name, st.TypeName, other)
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

	// byMethodName/methodOrder - та же группировка по имени, что и в
	// collectFuncSigs (см. package doc funcSig.MangledName) - метод
	// может быть объявлен более одного раза ТОЛЬКО как перегрузка
	// (разный список типов параметров).
	byMethodName := map[string][]*ast.MethodDecl{}
	var methodOrder []string
	for _, m := range ent.Methods {
		if _, seen := byMethodName[m.Name]; !seen {
			methodOrder = append(methodOrder, m.Name)
		}
		byMethodName[m.Name] = append(byMethodName[m.Name], m)
	}
	// overloadedMethodNames - нужно ДО построения любой сигнатуры:
	// resolveOpposesTag (тег `` `противоположность:X` ``) ищет X ПО
	// ИМЕНИ среди ent.Methods - при перегруженном X нашёл бы ПЕРВОЕ
	// попавшееся объявление с этим именем, произвольно (см. package doc
	// resolveOpposesTag) - явно запрещаем такую комбинацию, а не тихо
	// выбираем не ту перегрузку.
	overloadedMethodNames := map[string]bool{}
	for _, name := range methodOrder {
		if len(byMethodName[name]) > 1 {
			overloadedMethodNames[name] = true
		}
	}
	ownMethods := map[string]funcSig{}
	ownOverloads := map[string][]funcSig{}
	for _, name := range methodOrder {
		decls := byMethodName[name]
		if len(decls) == 1 {
			msig, err := a.buildMethodSig(entities, methods, aliases, aliasState, enums, ent, decls[0], sig.States, overloadedMethodNames)
			if err != nil {
				return err
			}
			ownMethods[name] = msig
			a.methodDeclNames[decls[0]] = name
			continue
		}
		// Перегрузка - несколько методов с одним и тем же именем, разным
		// списком типов параметров (см. package doc funcSig.MangledName).
		var group []funcSig
		for _, m := range decls {
			if m.IsAbstract || m.IsStatic {
				return errNoPos(CodeEntityDecl, "сущность %q: метод %q: перегрузка не поддерживается для абстрактных/статических методов в этой версии", ent.Name, name)
			}
			if hasVariadicParam(m.Params) {
				return errNoPos(CodeEntityDecl, "сущность %q: метод %q: вариативный метод нельзя перегружать", ent.Name, name)
			}
			msig, err := a.buildMethodSig(entities, methods, aliases, aliasState, enums, ent, m, sig.States, overloadedMethodNames)
			if err != nil {
				return err
			}
			for _, other := range group {
				if sameParamTypeList(other.Params, msig.Params) {
					return errNoPos(CodeEntityDecl, "сущность %q: метод %q: две перегрузки с одинаковым списком типов параметров (%s) - неотличимы ни на одном вызове", ent.Name, name, paramTypesString(msig.Params))
				}
			}
			msig.MangledName = overloadMangledName(name, msig.Params)
			a.methodDeclNames[m] = msig.MangledName
			group = append(group, msig)
		}
		ownOverloads[name] = group
	}

	if ent.Ctor != nil {
		ctorParams := make([]ir.Type, len(ent.Ctor.Params))
		ctorParamNames := make([]string, len(ent.Ctor.Params))
		ctorParamDefaults := make([]ast.Expr, len(ent.Ctor.Params))
		for i, p := range ent.Ctor.Params {
			typ, err := resolveTypeName(entities, methods, aliases, a.interfaces, enums, p.Type)
			if err != nil {
				return diagnostics.Wrap(err, "сущность %q: конструктор: параметр %q", ent.Name, p.Name)
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
	// internal/semantic/inheritance.go. methodOverloads - ТРАНЗИТИВНО
	// сплющенные (своя группа + унаследованные от базовой) перегрузки
	// методов этой сущности, см. package doc Analyzer.methodOverloads.
	if err := a.alignOverloadsWithBase(methods, ent, byMethodName, ownMethods, ownOverloads); err != nil {
		return err
	}
	flatSig, flatMethods, methodOverloads, err := a.applyInheritance(entities, methods, ent, sig, ownMethods, ownOverloads)
	if err != nil {
		return err
	}
	a.methodOverloads[ent.Name] = methodOverloads
	if !ent.IsAbstract {
		for name, msig := range flatMethods {
			if msig.IsAbstract {
				return errNoPos(CodeEntityDecl, "сущность %q: не реализован абстрактный метод %q - дайте ему тело или объявите саму сущность абстрактной", ent.Name, name)
			}
		}
	}
	entities[ent.Name] = flatSig
	methods[ent.Name] = flatMethods
	return nil
}

// collectFuncSigs - предварительный проход: сигнатуры всех функций
// программы, до анализа любого тела (нужно для вызовов вперёд/взаимной
// рекурсии - как и в cbackend.Generate). entities/methods/aliases уже
// полностью собраны (collectEntities) - функция может ссылаться на любую
// сущность, производный тип состояния или generic-коллекцию, независимо
// от того, где та объявлена в файле относительно функции.
//
// Ошибка ОДНОГО имени/перегрузки/"внешняя функция" не останавливает весь
// проход - собирается в общий bag, само объявление просто не попадает в
// funcs (или получает пустую funcSig{}-заглушку, только чтобы имя
// продолжало считаться "занятым" для последующей проверки коллизий - см.
// комментарии у bag внутри). В отличие от collectEntities, свободная
// функция никогда не служит ТИПОМ для сигнатуры другой (только целью
// вызова, разрешаемой в buildIR) - настоящий poison-механизм здесь не
// нужен, достаточно просто пропустить сломанное объявление.
func (a *Analyzer) collectFuncSigs(prog *ast.Program) error {
	funcs := map[string]funcSig{}
	// enums - a.enums ещё не собран (collectEnums идёт ПОСЛЕ этого прохода) -
	// см. тот же приём в collectEntities.
	enums := buildEnumTable(prog)
	registerBuiltinFuncs(funcs)
	registerHTTPEntityWithDefaults(a.entities, "Запрос", httpRequestFields, httpRequestDefaults)
	registerHTTPEntityWithDefaults(a.entities, "Ответ", httpResponseFields, httpResponseDefaults)
	funcs["запрос"] = funcSig{Params: []ir.Type{ir.EntityType("Запрос")}, ReturnType: ir.EntityType("Ответ"), HasReturn: true}
	registerHTTPServerFuncs(funcs)
	registerStringSplitFunc(funcs)
	registerRegexFunc(funcs)
	registerDateTimeFuncs(a.entities, funcs)
	registerColorFuncs(a.entities, funcs)
	registerCookieFuncs(a.entities, funcs)
	registerHTTPHeaderFunc(funcs)
	registerEnvFuncs(funcs)
	registerDBFuncs(funcs)
	registerDBStateFunc(a.entities, funcs)
	registerTransactionFuncs(funcs)
	registerNetworkFuncs(a.entities, funcs)
	registerBytesIOFuncs(funcs)
	registerHTTPBytesFuncs(funcs)
	registerMultipartFunc(funcs)
	registerHTTP2Funcs(funcs)
	registerGRPCFuncs(funcs)
	registerGRPCSFuncs(funcs)
	registerNumberBaseFuncs(funcs)
	registerProcessFuncs(funcs)
	registerSubprocessFuncs(a.entities, funcs)
	registerProcMemFuncs(funcs)
	registerProcRegsFuncs(a.entities, funcs)
	registerImageCodecFuncs(funcs)
	// byName - объявления функций программы, сгруппированные по имени, в
	// порядке первого появления имени (order) - функция может быть
	// объявлена более одного раза ТОЛЬКО как перегрузка (разный список
	// типов параметров, см. ниже); порядок внутри группы - порядок
	// объявления в файле (не важен для резолюции вызова - только для
	// сообщений об ошибках).
	byName := map[string][]*ast.FunctionDecl{}
	var order []string
	for _, fn := range prog.Functions {
		if _, seen := byName[fn.Name]; !seen {
			order = append(order, fn.Name)
		}
		byName[fn.Name] = append(byName[fn.Name], fn)
	}
	declNames := map[*ast.FunctionDecl]string{}
	funcOverloads := map[string][]funcSig{}
	// bag - собирает диагностики НЕЗАВИСИМЫХ объявлений (включая "внешняя
	// функция" ниже, collectExternFuncSigs) за один прогон, тем же
	// принципом, что и bag в collectEntities (см. её package doc) - ошибка
	// ОДНОГО имени/перегрузки/внешней функции не мешает разобрать
	// следующую. В отличие от сущностей, свободная функция никогда не
	// служит ТИПОМ для сигнатуры другой (только целью вызова, разрешаемой
	// уже в buildIR) - poison-заглушка в funcs нужна значительно реже, лишь
	// для того, чтобы имя, под которым сборка сигнатуры провалилась,
	// по-прежнему считалось "уже занятым" при коллизии с "внешняя функция"
	// того же имени ниже (funcSig{} - пустое, ничего сверх самого факта
	// присутствия ключа от неё не требуется).
	bag := diagnostics.NewBag()
	for _, name := range order {
		decls := byName[name]
		currentFile = decls[0].File
		if _, exists := funcs[name]; exists {
			if builtins.IsReservedName(name) {
				bag.AddErr(errNoPos(CodeEntityDecl, "функция %q - зарезервированное имя встроенной функции, нельзя объявить свою функцию с этим именем", name))
			} else {
				bag.AddErr(errNoPos(CodeDuplicateDecl, "функция %q объявлена более одного раза", name))
			}
			continue
		}
		if err := a.checkGenericConstraints(a.entities, a.methods, a.aliases, enums, name); err != nil {
			bag.AddErr(err)
			continue
		}
		if len(decls) == 1 {
			sig, err := a.buildFreeFuncSig(decls[0], enums)
			if err != nil {
				bag.AddErr(err)
				funcs[name] = funcSig{}
				continue
			}
			funcs[name] = sig
			declNames[decls[0]] = name
			continue
		}
		// Перегрузка - несколько объявлений с одним и тем же именем, разным
		// списком типов параметров (см. package doc funcSig.MangledName).
		// Каждое объявление группы разбирается НЕЗАВИСИМО от соседних - на
		// ошибке одного (тип параметра/"экспортировать"+перегрузка/...)
		// просто пропускается ОНО, остальные перегрузки того же имени
		// продолжают собираться (тот же принцип, что и entities/methods в
		// collectOneEntity).
		var group []funcSig
		for _, fn := range decls {
			currentFile = fn.File
			if fn.Exported {
				bag.AddErr(errNoPos(CodeEntityDecl, "функция %q: перегрузка не поддерживается для \"экспортировать\" в этой версии - C-символ вычисляется из настоящего имени функции без учёта списка параметров, коллизия с другой перегрузкой той же группы", name))
				continue
			}
			if hasVariadicParam(fn.Params) {
				bag.AddErr(errNoPos(CodeEntityDecl, "функция %q: вариативную функцию нельзя перегружать", name))
				continue
			}
			if fn.IsAsync {
				bag.AddErr(errNoPos(CodeEntityDecl, "функция %q: перегрузка не поддерживается для \"асинхронная\" в этой версии - \"дождаться %s(...)\" резолвит целевую функцию по голому имени, без учёта списка параметров", name, name))
				continue
			}
			sig, err := a.buildFreeFuncSig(fn, enums)
			if err != nil {
				bag.AddErr(err)
				continue
			}
			dup := false
			for _, other := range group {
				if sameParamTypeList(other.Params, sig.Params) {
					bag.AddErr(errNoPos(CodeEntityDecl, "функция %q: две перегрузки с одинаковым списком типов параметров (%s) - неотличимы ни на одном вызове", name, paramTypesString(sig.Params)))
					dup = true
					break
				}
			}
			if dup {
				continue
			}
			sig.MangledName = overloadMangledName(name, sig.Params)
			funcs[sig.MangledName] = sig
			declNames[fn] = sig.MangledName
			group = append(group, sig)
		}
		funcOverloads[name] = group
	}
	a.funcOverloads = funcOverloads
	a.funcDeclNames = declNames
	externFuncs := a.collectExternFuncSigs(bag, funcs, a.entities, a.methods, a.aliases, enums, prog)
	if bag.Len() > 0 {
		return bag.AsError()
	}
	a.funcs = funcs
	a.externFuncs = externFuncs
	return nil
}

// buildFreeFuncSig разбирает ОДНО объявление свободной функции в funcSig -
// без учёта перегрузки (коллизия имени с builtin/другой функцией
// проверяется вызывающей стороной, collectFuncSigs, один раз на имя, не на
// каждое объявление группы).
func (a *Analyzer) buildFreeFuncSig(fn *ast.FunctionDecl, enums map[string]map[string]int64) (funcSig, error) {
	params := make([]ir.Type, len(fn.Params))
	paramStates := make([]string, len(fn.Params))
	paramDefaults := make([]ast.Expr, len(fn.Params))
	for i, p := range fn.Params {
		typ, err := resolveTypeName(a.entities, a.methods, a.aliases, a.interfaces, enums, p.Type)
		if err != nil {
			return funcSig{}, diagnostics.Wrap(err, "функция %q: параметр %q", fn.Name, p.Name)
		}
		params[i] = typ
		paramStates[i] = a.aliasState[p.Type]
		paramDefaults[i] = p.Default
	}
	sig := funcSig{Params: params, ParamStates: paramStates, ParamDefaults: paramDefaults, IsAsync: fn.IsAsync, Variadic: hasVariadicParam(fn.Params)}
	if fn.ReturnType != "" {
		typ, err := resolveTypeName(a.entities, a.methods, a.aliases, a.interfaces, enums, fn.ReturnType)
		if err != nil {
			return funcSig{}, diagnostics.Wrap(err, "функция %q: тип результата", fn.Name)
		}
		sig.ReturnType = typ
		sig.HasReturn = true
	}
	return sig, nil
}

// buildMethodSig разбирает ОДНО объявление метода сущности ent в funcSig -
// без учёта перегрузки (арность/уникальность имени внутри сущности
// проверяется вызывающей стороной, collectEntities). states - sig.States
// СОБСТВЕННОЙ (ещё не сплющенной с базовой) сигнатуры ent, нужен
// resolveOpposesTag для перевода полной формы “ `становится` “ в краткую.
// overloadedNames - имена методов ent, перегруженных (2+ объявления) в этой
// же сущности - resolveOpposesTag отказывает явной ошибкой, если тег
// “ `противоположность:X` “ называет одно из них (см. package doc вызова
// в collectEntities: X ПО ИМЕНИ среди siblings, при перегруженном X
// нашёл бы первое попавшееся объявление произвольно).
func (a *Analyzer) buildMethodSig(entities map[string]entitySig, methods map[string]map[string]funcSig, aliases map[string]string, aliasState map[string]string, enums map[string]map[string]int64, ent *ast.EntityDecl, m *ast.MethodDecl, states map[string]string, overloadedNames map[string]bool) (funcSig, error) {
	params := make([]ir.Type, len(m.Params))
	paramStates := make([]string, len(m.Params))
	paramDefaults := make([]ast.Expr, len(m.Params))
	for i, p := range m.Params {
		typ, err := resolveTypeName(entities, methods, aliases, a.interfaces, enums, p.Type)
		if err != nil {
			return funcSig{}, diagnostics.Wrap(err, "сущность %q: метод %q: параметр %q", ent.Name, m.Name, p.Name)
		}
		params[i] = typ
		paramStates[i] = aliasState[p.Type]
		paramDefaults[i] = p.Default
	}
	revokes, err := resolveOpposesTag(m, ent.Methods, states, overloadedNames)
	if err != nil {
		return funcSig{}, diagnostics.Wrap(err, "сущность %q", ent.Name)
	}
	msig := funcSig{Params: params, ParamStates: paramStates, ParamDefaults: paramDefaults, Visibility: m.Visibility, IsAbstract: m.IsAbstract, IsStatic: m.IsStatic, RevokesStates: revokes, Variadic: hasVariadicParam(m.Params)}
	switch {
	case m.SelfReturn:
		// "этот" как тип результата - собственный метод сущности X
		// резолвится в EntityType(X) сразу же: applyInheritance перерезолвит
		// копию этой сигнатуры в EntityType(Y) для каждой Y, которая
		// унаследует метод без переопределения - см. package doc
		// funcSig.SelfReturn.
		msig.ReturnType = ir.EntityType(ent.Name)
		msig.HasReturn = true
		msig.SelfReturn = true
	case m.ReturnType != "":
		typ, err := resolveTypeName(entities, methods, aliases, a.interfaces, enums, m.ReturnType)
		if err != nil {
			return funcSig{}, diagnostics.Wrap(err, "сущность %q: метод %q: тип результата", ent.Name, m.Name)
		}
		msig.ReturnType = typ
		msig.HasReturn = true
	}
	return msig, nil
}

// collectExternFuncSigs - "внешняя функция": та же коллизия имени с
// builtin/уже объявленной функцией, что и у обычных функций выше, плюс
// проверка, что параметры/результат допустимы на границе FFI (isFFIType).
// Регистрирует Alias в funcs (для проверки вызовов), возвращает
// []*ir.ExternFuncDecl для ir.Program.ExternFuncs - настоящий C-символ
// (CName) для кодогенерации берёт оттуда только cbackend.GenerateFromIR,
// семантическому анализу он не нужен. Ошибка ОДНОЙ "внешняя функция" -
// независима от остальных (нет ни перегрузки, ни ссылки одной на другую
// внутри этого прохода) - копится в bag (тем же принципом, что остальная
// часть collectFuncSigs, см. её package doc), не прерывая разбор следующих.
func (a *Analyzer) collectExternFuncSigs(bag *diagnostics.Bag, funcs map[string]funcSig, entities map[string]entitySig, methods map[string]map[string]funcSig, aliases map[string]string, enums map[string]map[string]int64, prog *ast.Program) []*ir.ExternFuncDecl {
	var out []*ir.ExternFuncDecl
	for _, ext := range prog.ExternFuncs {
		label := fmt.Sprintf("внешняя функция %q", ext.Alias)
		if _, exists := funcs[ext.Alias]; exists {
			if builtins.IsReservedName(ext.Alias) {
				bag.AddErr(errNoPos(CodeEntityDecl, "%s - зарезервированное имя встроенной функции, нельзя объявить с этим именем", label))
			} else {
				bag.AddErr(errNoPos(CodeDuplicateDecl, "имя %q уже объявлено (как функция) - нельзя повторно объявить его как внешнюю функцию", ext.Alias))
			}
			continue
		}
		params := make([]ir.Type, len(ext.Params))
		failed := false
		for i, p := range ext.Params {
			typ, err := resolveTypeName(entities, methods, aliases, a.interfaces, enums, p.Type)
			if err != nil {
				bag.AddErr(diagnostics.Wrap(err, "%s: параметр %q", label, p.Name))
				failed = true
				break
			}
			if !isFFIType(entities, methods, aliases, a.interfaces, enums, typ) {
				bag.AddErr(errNoPos(CodeEntityDecl, "%s: параметр %q имеет тип %s - у внешней функции допустимы только примитивы (Целое/Целое8/16/32/64/Натуральное/Натуральное8/16/32/64/Действительное/Дробное/Логическое/Байт/Символ/Строка), внешние сущности (\"внешняя сущность\"), \"Указатель<T>\" над FFI-совместимым T и \"Вызываемый(...)\" с FFI-совместимыми параметрами/результатом", label, p.Name, typ))
				failed = true
				break
			}
			params[i] = typ
		}
		if failed {
			funcs[ext.Alias] = funcSig{IsExtern: true}
			continue
		}
		decl := &ir.ExternFuncDecl{CName: ext.CName, Alias: ext.Alias, Params: params}
		sig := funcSig{Params: params, IsExtern: true}
		if ext.ReturnType != "" {
			typ, err := resolveTypeName(entities, methods, aliases, a.interfaces, enums, ext.ReturnType)
			if err != nil {
				bag.AddErr(diagnostics.Wrap(err, "%s: тип результата", label))
				funcs[ext.Alias] = sig
				continue
			}
			// "Вызываемый(...)"/"?Вызываемый(...)" результатом - сырой
			// указатель на функцию от C (прежний обработчик у signal())
			// превращается в вызываемое Rech-значение (демаршалинг,
			// ffi_callback.go обоих backend'ов).
			if !ffiCallableResult(entities, methods, aliases, a.interfaces, enums, typ) && !isFFIType(entities, methods, aliases, a.interfaces, enums, typ) {
				bag.AddErr(errNoPos(CodeEntityDecl, "%s: тип результата %s не поддерживается - у внешней функции допустимы только примитивы (Целое/Целое8/16/32/64/Натуральное/Натуральное8/16/32/64/Действительное/Дробное/Логическое/Байт/Символ/Строка), внешние сущности (\"внешняя сущность\") и \"Указатель<T>\" над FFI-совместимым T", label, typ))
				funcs[ext.Alias] = sig
				continue
			}
			decl.ReturnType = typ
			decl.HasReturn = true
			sig.ReturnType = typ
			sig.HasReturn = true
		}
		funcs[ext.Alias] = sig
		out = append(out, decl)
	}
	return out
}

// collectEnums собирает все перечисления программы (шаг 12) - стандартные
// (StandardEnums, всегда присутствуют) плюс объявленные пользователем
// (prog.Enums, копия StandardEnums на входе - чтобы мутация ниже не
// затронула сам StandardEnums между вызовами Analyze). Дубли членов
// внутри одного перечисления уже проверены парсером (см. parseEnumDecl) -
// здесь только коллизии ИМЕНИ самого перечисления с сущностью или другим
// перечислением (в т. ч. встроенным).
func (a *Analyzer) collectEnums(prog *ast.Program) error {
	seen := make(map[string]bool, len(StandardEnums))
	for name := range StandardEnums {
		seen[name] = true
	}
	for _, e := range prog.Enums {
		currentFile = e.File
		if _, exists := a.entities[e.Name]; exists {
			return errAtLine(e.Line, CodeDuplicateDecl, "перечисление %q совпадает с именем уже объявленной сущности", e.Name)
		}
		if seen[e.Name] {
			return errAtLine(e.Line, CodeDuplicateDecl, "перечисление %q уже объявлено (возможно, совпадает со встроенным перечислением)", e.Name)
		}
		seen[e.Name] = true
	}
	a.enums = buildEnumTable(prog)
	return nil
}

// buildEnumTable объединяет StandardEnums (встроенные перечисления) с
// пользовательскими перечислениями программы (prog.Enums) в одну карту имя
// перечисления -> имя члена -> значение - БЕЗ проверки коллизий (её делает
// collectEnums, которая идёт позже и присваивает результат в a.enums).
// Отдельная функция нужна затем, что resolveTypeName должен видеть имена
// перечислений уже во ВРЕМЯ collectInterfaces/collectEntities/
// collectFuncSigs (все три - до collectEnums, см. Analyze) - тот же приём-
// плейсхолдер, что paramEntities у collectInterfaces для имён сущностей:
// коллизии всё равно поймает collectEnums следом, независимо от того, какой
// из двух проходов первым столкнулся с конфликтующим именем.
func buildEnumTable(prog *ast.Program) map[string]map[string]int64 {
	enums := make(map[string]map[string]int64, len(StandardEnums)+len(prog.Enums))
	for name, members := range StandardEnums {
		copied := make(map[string]int64, len(members))
		maps.Copy(copied, members)
		enums[name] = copied
	}
	for _, e := range prog.Enums {
		members := make(map[string]int64, len(e.Members))
		for _, m := range e.Members {
			members[m.Name] = m.Value
		}
		enums[e.Name] = members
	}
	return enums
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
		currentFile = c.File
		if _, exists := consts[c.Name]; exists {
			return errAtLine(c.Line, CodeDuplicateDecl, "константа %q уровня модуля объявлена более одного раза", c.Name)
		}
		if _, exists := a.entities[c.Name]; exists {
			return errAtLine(c.Line, CodeDuplicateDecl, "константа %q уровня модуля: имя совпадает с уже объявленной сущностью", c.Name)
		}
		if _, exists := a.funcs[c.Name]; exists {
			return errAtLine(c.Line, CodeDuplicateDecl, "константа %q уровня модуля: имя совпадает с уже объявленной функцией", c.Name)
		}
		if _, exists := a.enums[c.Name]; exists {
			return errAtLine(c.Line, CodeDuplicateDecl, "константа %q уровня модуля: имя совпадает с уже объявленным перечислением", c.Name)
		}
		if _, exists := a.interfaces[c.Name]; exists {
			return errAtLine(c.Line, CodeDuplicateDecl, "константа %q уровня модуля: имя совпадает с уже объявленным интерфейсом", c.Name)
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
		params[i] = ir.Param{Name: p.Name, Type: sig.Params[i], Variadic: p.Variadic}
		if err := sc.declare(p.Name, sig.Params[i]); err != nil {
			return nil, nil, sig, diagnostics.WrapAt(err, diagnostics.Position{File: currentFile, Line: p.Line}, "параметр %q", p.Name)
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
	fsc.inferScope = ent.Name
	for i, name := range sig.FieldNames {
		var doc string
		if i < len(sig.FieldDocs) {
			doc = sig.FieldDocs[i]
		}
		var def ir.Expr
		if i < len(sig.FieldDefaultExprs) && sig.FieldDefaultExprs[i] != nil {
			val, err := resolveFieldDefault(fsc, ent.Name, name, ent.Line)
			if err != nil {
				return nil, diagnostics.Wrap(err, "поле %q", name)
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
				return nil, diagnostics.Wrap(err, "статическое поле %q", name)
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
		sc.inferScope = ent.Name
		for i, cname := range sig.ConstNames {
			val, err := resolveEntityConstant(sc, ent.Name, cname, ent.Line)
			if err != nil {
				return nil, diagnostics.Wrap(err, "константа %q", cname)
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
			return nil, diagnostics.Wrap(err, "метод %q", m.Name)
		}
		irMethods[i] = irM
	}
	var ctor *ir.Method
	if ent.Ctor != nil {
		irCtor, err := a.analyzeConstructor(ent)
		if err != nil {
			return nil, diagnostics.Wrap(err, "конструктор")
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
		StateNames: sig.StateNames, StateBitOffset: sig.StateBitOffset, HasStateFlags: sig.HasStateFlags, Tag: ent.Tag, File: ent.File,
		IsExternStruct: ent.IsExternStruct, ExternCName: ent.ExternCName,
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
	sc.inferScope = ent.Name + ".конструктор"
	params := make([]ir.Param, len(c.Params))
	for i, p := range c.Params {
		params[i] = ir.Param{Name: p.Name, Type: sig.CtorParams[i]}
		if err := sc.declare(p.Name, sig.CtorParams[i]); err != nil {
			return nil, diagnostics.WrapAt(err, diagnostics.Position{File: currentFile, Line: p.Line}, "параметр %q", p.Name)
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
	return &ir.Method{Params: params, Body: body, File: ent.File, Exported: c.Exported}, nil
}

func (a *Analyzer) analyzeMethod(ent *ast.EntityDecl, m *ast.MethodDecl) (*ir.Method, error) {
	// name - ключ, под которым сигнатура ЭТОГО КОНКРЕТНОГО объявления
	// зарегистрирована в methods[ent.Name]: голое m.Name без перегрузки
	// этого имени у ent, иначе - мангл-имя (см. package doc
	// Analyzer.methodDeclNames) - тот же приём, что и analyzeFunction.
	name := a.methodDeclNames[m]
	sig := a.methods[ent.Name][name]
	sc := newCtx(a, a.funcs, a.entities, a.methods, a.aliases, a.interfaces, a.enums, ent.Name, sig.ReturnType, sig.HasReturn, a.resolvedConsts, a.resolvingConsts)
	sc.inferScope = ent.Name + "." + m.Name
	params, body, sig, err := a.analyzeParamsAndBody(sc, m.Params, sig, m.Body)
	if err != nil {
		return nil, err
	}
	if !m.IsAbstract && !a.noFlowChecks {
		unreachable, fallsThrough := analyzeFlow(body)
		if unreachable != nil {
			return nil, unreachable
		}
		if sig.HasReturn && fallsThrough {
			return nil, errAtLine(m.Line, CodeEntityDecl, "с объявленным типом результата %s должен заканчиваться \"вернуть <значение>\" на каждом пути выполнения", sig.ReturnType)
		}
	}
	sig.GuaranteedThisStates = sc.thisEstablished
	a.methods[ent.Name][name] = sig
	return &ir.Method{
		Name:       name,
		Params:     params,
		ReturnType: sig.ReturnType,
		HasReturn:  sig.HasReturn,
		Body:       body,
		IsAbstract: m.IsAbstract,
		IsStatic:   m.IsStatic,
		File:       ent.File,
		Doc:        m.Doc,
		Tag:        m.Tag,
		Exported:   m.Exported,
	}, nil
}

func (a *Analyzer) analyzeFunction(fn *ast.FunctionDecl) (*ir.Function, error) {
	// name - ключ, под которым сигнатура ЭТОГО КОНКРЕТНОГО объявления
	// зарегистрирована в a.funcs: голое fn.Name без перегрузки этого имени,
	// иначе - мангл-имя (см. package doc Analyzer.funcDeclNames) -
	// funcs[fn.Name] не годится напрямую: при перегрузке под голым именем в
	// funcs нет записи вовсе.
	name := a.funcDeclNames[fn]
	sig := a.funcs[name]
	sc := newCtx(a, a.funcs, a.entities, a.methods, a.aliases, a.interfaces, a.enums, "", sig.ReturnType, sig.HasReturn, a.resolvedConsts, a.resolvingConsts)
	sc.inferScope = fn.Name
	params, body, sig, err := a.analyzeParamsAndBody(sc, fn.Params, sig, fn.Body)
	if err != nil {
		return nil, err
	}
	unreachable, fallsThrough := a.flow(body)
	if unreachable != nil {
		return nil, unreachable
	}
	if sig.HasReturn && fallsThrough {
		return nil, errAtLine(fn.Line, CodeEntityDecl, "с объявленным типом результата %s должна заканчиваться \"вернуть <значение>\" на каждом пути выполнения", sig.ReturnType)
	}
	return &ir.Function{
		Name:       name,
		Params:     params,
		ReturnType: sig.ReturnType,
		HasReturn:  sig.HasReturn,
		Body:       body,
		File:       fn.File,
		Doc:        fn.Doc,
		Exported:   fn.Exported,
	}, nil
}

// buildIR строит *ir.Program из уже собранных сигнатур (a.entities/a.funcs/
// a.interfaces, заполненных предыдущими collect*-проходами Analyze).
//
// Тела сущностей и функций программы (analyzeEntity/analyzeFunction ниже) -
// НЕЗАВИСИМЫ друг от друга: сигнатуры ВСЕХ сущностей/функций/методов уже
// собраны раньше (collectEntities/collectFuncSigs), значит ошибка внутри
// тела ОДНОЙ функции не может исказить анализ другой - это ровно то
// условие, при котором можно собрать НЕСКОЛЬКО независимых диагностик за
// один прогон вместо остановки на первой (тот же принцип, что и у
// parser.ParseProgramLenient - см. internal/diagnostics.Bag). Ошибка
// сохраняется в bag, соответствующая сущность/функция просто не попадает в
// out - если bag непуст к концу обоих циклов, buildIR возвращает все
// диагностики сразу, не продолжая до generic-инстанциаций/callables/
// экспорта ниже (они полагаются на ПОЛНЫЙ, а не частичный out.Entities/
// out.Functions).
//
// Более РАННИЕ проходы Analyze (вызываются до buildIR) делятся на две
// группы по тому же признаку. collectEntities/collectFuncSigs - результат
// ОДНОГО объявления (например, поле сущности) может быть НУЖЕН для
// проверки другого (сущность-наследник) - поэтому пропуск сломанного
// объявления сам по себе дал бы каскад ложных ошибок у всех, кто на него
// ссылается; вместо этого они СТАВЯТ poison-заглушку (entitySig.Poisoned,
// пустая запись в funcs) вместо настоящей сигнатуры и продолжают со
// следующего объявления - ссылка на заглушку резолвится как обычно, не
// порождая вторую диагностику (см. package doc collectEntities/
// collectFuncSigs). collectInterfaces/collectEnums/collectModuleConsts -
// по-прежнему останавливаются на первой ошибке: их объявления с тем же
// каскадным риском (наследование интерфейса, коллизия имени с уже
// известными таблицами) редки в реальном коде, заводить для них
// собственный poison-механизм отдельной задачей сознательно не стали.
func (a *Analyzer) buildIR(prog *ast.Program) (*ir.Program, error) {
	out := &ir.Program{ExternFuncs: a.externFuncs, DBPoolSize: DefaultDBPoolSize, DBSQLiteBusyTimeoutMs: DefaultDBSQLiteBusyTimeoutMs, PoolShrinkEnabled: true, PoolIdleTimeoutMs: DefaultPoolIdleTimeoutMs, HTTPMaxRequestBytes: DefaultHTTPMaxRequestBytes}
	bag := diagnostics.NewBag()
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
		currentFile = ent.File
		irEnt, err := a.analyzeEntity(ent)
		if err != nil {
			bag.AddErr(diagnostics.Wrap(err, "сущность %q", ent.Name))
			continue
		}
		out.Entities = append(out.Entities, irEnt)
		declaredEntityNames[ent.Name] = true
	}
	// Запрос/Ответ - структуры (по значению): простые поля-контейнеры без
	// методов и без сценария разделяемой мутации, копирование - естественное
	// и более предсказуемое поведение, чем ссылка на общий буфер.
	//
	// Все десять здесь эмитируются безусловно (в отличие от "Совпадение"/
	// "ЧастьФормы"/... ниже - лениво, только при реальном использовании) -
	// историческая асимметрия, не связанная с затенением. Шесть из них -
	// internal/builtins.ShadowableEntityNames - пропускаются, если
	// declaredEntityNames уже содержит это имя (пользователь объявил свою
	// сущность с этим именем, цикл по prog.Entities выше уже добавил ЕЁ
	// ir.Entity в out.Entities) - без этой проверки получилась бы вторая,
	// "официальная" ir.Entity с тем же именем и коллизия транслитерации в
	// cbackend ("сущности X и X транслитерируются в одно и то же имя").
	standardEntities := []*ir.Entity{
		{Name: "Запрос", IsStruct: true, Fields: httpRequestFields},
		{Name: "Ответ", IsStruct: true, Fields: httpResponseFields},
		{Name: "ДатаВремя", IsStruct: true, Fields: dateTimeFields},
		{Name: "ОпцииКуки", IsStruct: true, Fields: CookieOptionsFields},
		{Name: "Дейтаграмма", IsStruct: true, Fields: DatagramFields},
		{Name: "RGBЦвет", IsStruct: true, Fields: ColorFields},
		{Name: "Процесс", IsStruct: true, Fields: ProcessFields},
		{Name: "РезультатКоманды", IsStruct: true, Fields: CommandResultFields},
		{Name: "Регистры", IsStruct: true, Fields: RegistersFields},
		{Name: dbStateEntityName, IsStruct: true, Fields: DBStateFields},
	}
	for _, ent := range standardEntities {
		if declaredEntityNames[ent.Name] {
			continue
		}
		out.Entities = append(out.Entities, ent)
	}
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
	out.Entities = append(out.Entities, panicIREntities()...)
	out.Entities = append(out.Entities, errorIREntities()...)
	for _, fn := range prog.Functions {
		currentFile = fn.File
		irFn, err := a.analyzeFunction(fn)
		if err != nil {
			bag.AddErr(diagnostics.Wrap(err, "функция %q", fn.Name))
			continue
		}
		out.Functions = append(out.Functions, irFn)
	}
	if bag.Len() > 0 {
		// Дальнейшие шаги (generic-инстанциации/callables/экспорт ниже)
		// полагаются на ПОЛНЫЙ out.Entities/out.Functions - сломанная
		// сущность/функция просто отсутствует в них (см. continue выше),
		// поэтому при любой накопленной ошибке анализ останавливается
		// здесь, но с ПОЛНЫМ списком независимых диагностик за один
		// прогон, а не только первой (см. package doc buildIR).
		return nil, bag.AsError()
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
			Equatable:        a.entities[name].Equatable,
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

	// "Совпадение" - лениво-условная регистрация - добавлена в out.Entities,
	// только если программа хоть раз упомянула "Совпадение" (вызовом
	// "регулярное" или явной аннотацией типа, см. её case в
	// resolveTypeName). Её собственное поле "группы" (Список<Строка>) не
	// требует особого порядка относительно других typedef сущностей -
	// generic-коллекция примитива (не сущности) эмитится в отдельный блок
	// ДО всех typedef сущностей (genericsCode, cbackend/ir_generator.go),
	// независимо от места regex.go в этом списке.
	if _, ok := a.entities["Совпадение"]; ok && !declaredEntityNames["Совпадение"] {
		out.Entities = append(out.Entities, &ir.Entity{Name: "Совпадение", IsStruct: true, Fields: MatchFields})
	}
	// "ЧастьФормы" - тот же принцип, что "Совпадение" выше.
	if _, ok := a.entities[formPartType]; ok && !declaredEntityNames[formPartType] {
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
	// "HTTP2Поток" - тот же принцип, что "ЗапросБайты"/"ОтветБайты" выше.
	if _, ok := a.entities[http2StreamType]; ok {
		out.Entities = append(out.Entities, &ir.Entity{Name: http2StreamType, IsStruct: true, Fields: HTTP2StreamFields})
	}
	// "ДекодированноеИзображение" - тот же принцип, что "ЗапросБайты"/
	// "ОтветБайты"/"HTTP2Поток" выше - зарегистрирована в a.entities только
	// при реальном вызове "декодировать_изображение"/"закодировать_
	// изображение" (registerImageCodecEntityOnCall, imagecodec.go).
	if _, ok := a.entities[imageCodecEntityName]; ok && !declaredEntityNames[imageCodecEntityName] {
		out.Entities = append(out.Entities, &ir.Entity{Name: imageCodecEntityName, IsStruct: true, Fields: ImageCodecFields})
	}
	// "Блокировка" - проверка ЗДЕСЬ (не раньше цикла по prog.Entities/
	// prog.Functions выше), потому что она может встретиться впервые не
	// только как тип поля (это уже зарегистрировало бы её в a.entities к
	// началу этой функции, через collectEntities), но и внутри тела метода/
	// функции/конструктора ("переменная м = новая Блокировка()" без единого
	// поля такого типа нигде) - тела анализируются как раз в циклах выше.
	// В отличие от лениво-условных сущностей выше (просто append) -
	// "Блокировка" ставится ПЕРВОЙ в
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
	out.ProtobufParseTargets = a.protobufParseTargets.order
	out.ProtobufSerializeTargets = a.protobufSerializeTargets.order
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
	// prog.ExternFuncs - параметр "Вызываемый(...)" ("FFI:
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
		if ext.HasReturn {
			if err := a.registerCallable(a.ffiCallableInner(ext.ReturnType)); err != nil {
				return err
			}
		}
	}
	// Поле "Вызываемый"/"?Вызываемый" внешней структуры - C-typedef поля
	// объявляет голый указатель на функцию этой сигнатуры.
	for _, ent := range prog.Entities {
		if !ent.IsExternStruct {
			continue
		}
		for _, f := range ent.Fields {
			if err := a.registerCallable(a.ffiCallableInner(f.Type)); err != nil {
				return err
			}
		}
	}
	return nil
}

// flow - analyzeFlow, если проверки потока включены (см. noFlowChecks);
// иначе - ничего не найдено.
func (a *Analyzer) flow(body []ir.Stmt) (error, bool) {
	if a.noFlowChecks {
		return nil, false
	}
	return analyzeFlow(body)
}
