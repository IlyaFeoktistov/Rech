// Сигнатуры (подписи) функций, методов, конструкторов и т.д. в языке Речь.
package semantic

import (
	"github.com/IlyaFeoktistov/Rech/internal/ast"
	"github.com/IlyaFeoktistov/Rech/internal/ir"
)

// funcSig - сигнатура функции, собранная предварительным проходом
// (collectFuncSigs) до анализа тел - функции могут вызывать друг друга
// независимо от порядка объявления в файле (включая взаимную рекурсию),
// как и в AST-пути (cbackend.funcSig, тот же принцип, отдельный тип - IR
// не должен знать про cbackend).
type funcSig struct {
	Params     []ir.Type
	ReturnType ir.Type
	HasReturn  bool

	// ParamDefaults - значение по умолчанию параметра ("параметр: Тип =
	// <выражение>", docs/SPEC.md) - той же длины, что и Params, nil на
	// позиции параметра без значения по умолчанию. Только ХВОСТ параметров
	// может иметь default (parser.checkTrailingDefaults уже проверяет это
	// при разборе) - значит "первый ненулевой индекс" однозначно
	// определяет минимально необходимое число аргументов вызова (см.
	// checkCallArgs). Разрешается лениво, один раз на сигнатуру
	// (resolveParamDefault, кэш по адресу этого самого среза - стабилен
	// для всех вызовов одной сигнатуры: funcSig копируется по значению,
	// но срез внутри указывает на тот же исходный backing-array).
	ParamDefaults []ast.Expr

	// Visibility - видимость метода (ast.MethodDecl.Visibility); всегда
	// ast.Public для обычных функций - у них нет понятия видимости.
	Visibility ast.Visibility

	// SelfReturn - тип результата объявлен как "этот" (ast.MethodDecl.
	// SelfReturn) - ReturnType здесь уже РАЗРЕШЁН в конкретный тип (см.
	// package doc ast.MethodDecl.SelfReturn): собственный метод сущности X -
	// в EntityType(X) (interfaceSig - в InterfaceType(X)), унаследованный
	// без переопределения метод - каждый раз заново в EntityType той
	// сущности, для которой строится ЕЁ ЭКЗЕМПЛЯР этой сигнатуры
	// (applyInheritance) - поэтому этот флаг обязан пережить копирование
	// сигнатуры при флаттенинге, а не просто быть использован один раз и
	// забыт: без него applyInheritance не отличила бы "унаследованный метод
	// с ковариантным self-результатом" (нужно перерезолвить ReturnType) от
	// "унаследованного метода, честно возвращающего конкретно базовый тип"
	// (перерезолвливать не нужно). sigsMatch (переопределение/соответствие
	// интерфейсу) сравнивает этот флаг вместо ReturnType напрямую - см. её
	// package doc.
	SelfReturn bool

	// ParamStates - для параметра, объявленного производным типом
	// состояния ("параметр: АвторизованныйПользователь"), краткая форма
	// самого состояния ("авторизован") - тот же индекс, что и Params
	// (Params[i] уже резолвлен в тип БАЗОВОЙ сущности, alias-имя потеряно,
	// см. resolveTypeName - без этого поля flow-sensitive проверка
	// (checkCallArgs) не знала бы, какой именно переход состояния требует
	// параметр). "" - параметр объявлен без производного типа состояния
	// (обычный тип, в т.ч. обычная сущность без каких-либо требований к
	// состоянию).
	ParamStates []string

	// GuaranteedThisStates - только для методов (funcSig как элемент
	// methods[Сущность][Метод]): набор состояний, которые этот метод
	// доказанно устанавливает на "этот" на всех путях выполнения (см.
	// analyzeMethod) - используется analyzeMethodCallExpr, чтобы после
	// вызова метода-транзиции (например, "получатель.авторизовать()")
	// перенести доказанный переход на переменную-получателя вызывающей
	// стороны. nil у обычных функций (никогда не заполняется) и у методов,
	// ещё не проанализированных к моменту вызова (взаимная рекурсия
	// методов - безопасный дефолт "ничего не доказано").
	GuaranteedThisStates map[string]bool

	// RevokesStates - только для методов, объявленных с тегом
	// `` `противоположность:<другойМетод>` ``: набор состояний,
	// которые <другойМетод> устанавливает через "этот становится ..." в
	// своём теле (см. verbfamily.go, collectBecomeStates) - при вызове ЭТОГО
	// метода они снимаются с получателя (analyzeMethodCallExpr), а не
	// добавляются, в отличие от GuaranteedThisStates. Вычисляется один раз
	// при сборе сигнатур, до анализа тел (простой обход AST тела
	// <другогоМетода> в поисках BecomeStmt, не flow-sensitive established -
	// поэтому не подвержен зависимости от порядка анализа методов, в
	// отличие от GuaranteedThisStates). nil у методов без такого тега.
	RevokesStates map[string]bool

	// IsExtern - true у "внешняя функция" (шаг 7, FFI) - только у элементов
	// funcs, никогда у methods (методов не бывает внешних). Нужен
	// analyzeLaunchExpr ("запустить" не поддерживает внешние функции - нет
	// гарантии потокобезопасности произвольного C-символа).
	IsExtern bool

	// IsAbstract - метод объявлен без тела (ast.MethodDecl.IsAbstract) -
	// только у элементов methods, никогда у funcs (у обычной функции не
	// бывает отсутствующего тела). После applyInheritance (собственный
	// метод накладывается на унаследованный) флаг несёт актуальное
	// состояние САМОЙ ПОЗДНЕЙ реализации по цепочке: сущность, которая
	// сама не объявлена "абстрактная", обязана не иметь в своих
	// flatMethods ни одной записи с IsAbstract == true - см.
	// internal/semantic/analyzer.go, проверка после applyInheritance.
	IsAbstract bool

	// IsStatic - только для элементов methods (funcSig как сигнатура
	// метода): объявлен с модификатором "статический" (ast.MethodDecl.
	// IsStatic) - вызывается "Тип::метод(...)" (analyzeExpr/
	// *ast.MethodCallExpr со Static == true), не через получатель-
	// экземпляр, и не участвует в диспетчеризации/наследовании (тем же
	// принципом, что и обычная функция, просто зарегистрирован под именем
	// сущности). Всегда false у элементов funcs.
	IsStatic bool
}

// entitySig - сигнатура сущности: поля в порядке объявления (для
// конструктора и доступа к полю), собранная предварительным
// проходом (collectEntitySigs) - тот же принцип, что и funcSig, отдельный
// тип от cbackend.entitySig (IR не должен знать про cbackend).
type entitySig struct {
	FieldNames []string
	FieldTypes []ir.Type
	// FieldDocs - doc-комментарий соответствующего поля (тот же индекс, что
	// и FieldNames/FieldTypes; "" - его не было) - см. ast.Field.Doc, нужен
	// только internal/lsp (hover), в остальном семантическом анализе не
	// участвует.
	FieldDocs []string

	// FieldTags - сырой тег соответствующего поля (тот же индекс, что и
	// FieldNames/FieldTypes; "" - тега не было), см. ast.Field.Tag.
	FieldTags []string

	// FieldVisibility - видимость соответствующего поля (тот же индекс,
	// что и FieldNames/FieldTypes); nil у сущностей без ни одного
	// непубличного поля.
	FieldVisibility []ast.Visibility

	// FieldDefaultExprs - AST-выражение значения по умолчанию поля (после
	// "=" в объявлении, см. ast.Field.Default), тот же индекс, что и
	// FieldNames/FieldTypes; nil-элемент - у поля нет значения по
	// умолчанию. Разрешается в ir.Expr лениво и ровно один раз на весь
	// Analyze - тем же принципом, что и ConstExprs, см. resolveFieldDefault.
	FieldDefaultExprs []ast.Expr

	// StaticFieldNames/Types/Visibility/DefaultExprs - поля, объявленные с
	// модификатором "статический" (ast.Field.IsStatic) - параллельные
	// массивы, тот же принцип, что и FieldNames/... выше, но отдельные (не
	// подмешаны в FieldNames): у статического поля нет места в C-структуре
	// экземпляра, доступ только "Тип::имя" (см. staticFieldIndex,
	// analyzeExpr/*ast.FieldAccess). DefaultExprs[i] == nil - поле без "="
	// получает нулевое значение типа (см. resolveStaticFieldDefault).
	StaticFieldNames        []string
	StaticFieldTypes        []ir.Type
	StaticFieldVisibility   []ast.Visibility
	StaticFieldDefaultExprs []ast.Expr

	// States - для проверки BecomeStmt ("получатель становится
	// <форма>") - ключ ast.StateDecl.BecomeForm (полная форма,
	// согласованная по роду/числу И творительным падежом - ровно то, что
	// обязан написать программист, см. package doc ast.StateDecl.BecomeForm),
	// значение - краткая форма (ast.StateDecl.Name, канонический
	// идентификатор состояния, используемый дальше везде - established,
	// ir.BecomeStmt.State, ParamStates). nil у сущностей без "состояние:".
	States map[string]string

	// StateNames - те же краткие формы, что и значения States, но в
	// порядке объявления (States - map, порядок не определён) и только
	// СОБСТВЕННЫЕ (не унаследованные - States тоже не наследуется, см.
	// applyInheritance) - индекс имени в этом списке + StateBitOffset даёт
	// номер бита в общем рантайм-хранилище состояний ("state_flags",
	// internal/backend/cbackend/typestate.go/internal/interp) - там, где
	// established на этапе компиляции не может доказать переход и вместо
	// ошибки компиляции вставляется рантайм-проверка. nil у сущности без
	// "состояние:".
	StateNames []string
	// StateBitOffset - номер первого свободного бита ПОСЛЕ всех состояний
	// базовой сущности (0, если базовой нет или у неё нет состояний) -
	// собственные StateNames занимают биты [StateBitOffset,
	// StateBitOffset+len(StateNames)) в ОБЩЕМ на всю цепочку наследования
	// "state_flags" (сущность-потомок с собственным "состояние:" не
	// конфликтует по битам с состояниями предка).
	StateBitOffset int
	// HasStateFlags - у этой сущности ИЛИ у кого-то из предков есть хотя бы
	// одно состояние - её C-структура несёт скрытое поле "state_flags"
	// (нужно и потомку, даже если сам он не объявляет "состояние:" -
	// указатель на потомка используется как указатель на предка при
	// рантайм-проверке, структура обязана быть совместима).
	HasStateFlags bool

	// IsList/ElemType - заполнены только для синтезированных инстанциаций
	// встроенных generic-коллекций (resolveTypeName) - IsList
	// true только у "Список<T>" (единственная коллекция, участвующая в
	// цикле "для x в списке"), ElemType - у всех трёх (Список/Стопка/
	// Очередь), чтобы Analyze мог собрать Program.Generics в конце без
	// повторного разбора имени. У обычных пользовательских сущностей -
	// нулевые значения.
	IsList   bool
	ElemType ir.Type
	// IsDict/KeyType - тот же принцип, что IsList/ElemType выше, но для
	// "Словарь<К, В>" (resolveDictTypeName) - двухаргументного встроенного
	// generic-типа: ElemType несёт тип ЗНАЧЕНИЯ (то же поле, что и у
	// Список/Стопка/Очередь - переиспользуется, не дублируется отдельным
	// ValueType), KeyType - тип ключа. У "Словарь" нет цикла "для x в Y"
	// (см. package doc dictCollectionCCode) - IsDict не участвует в той же
	// проверке, что и IsList.
	IsDict  bool
	KeyType ir.Type

	// IsNullable - синтезированная запись для "?Тип" (resolveNullableTypeName,
	// internal/semantic/nullable.go), тем же принципом лениво-регистрируемой
	// entitySig, что IsList/IsDict - ElemType несёт внутренний Тип (то же
	// поле, переиспользуется, не заводится отдельное). У nullable-"сущности"
	// нет ни полей/методов, ни State - зарегистрирована только затем, чтобы
	// внутренний тип можно было достать по каноническому имени (Type.Entity)
	// в любой точке анализа/кодогенерации, не таская его в самом ir.Type.
	IsNullable bool

	// IsPointer - синтезированная запись для "Указатель<T>"
	// (resolvePointerTypeName, internal/semantic/pointer.go), тот же принцип,
	// что и IsNullable выше - ElemType несёт T (само поле переиспользуется,
	// не заводится отдельное).
	IsPointer bool

	// ConstNames/ConstExprs - константы, объявленные в теле сущности
	// (последовательность объявления). ConstExprs хранит AST-выражения
	// для значения константы, чтобы их можно было проанализировать позже
	// в контексте (analyzeEntity / analyzeExpr при обращении через
	// Сущность.Константа).
	ConstNames []string
	ConstExprs []ast.Expr
	// ConstDocs - doc-комментарий соответствующей константы (тот же индекс,
	// что и ConstNames/ConstExprs; "" - его не было) - см. ast.ConstDecl.Doc.
	ConstDocs []string
	// ConstVisibility - видимость соответствующей константы
	ConstVisibility []ast.Visibility

	// IsExternStruct - см. ast.EntityDecl.IsExternStruct - нужно здесь,
	// чтобы isFFIType мог разрешить эту "структура" саму по себе как поле
	// другой "внешняя структура" или как тип в сигнатуре "внешняя функция"
	// (её собственные поля уже проверены FFI-безопасными на этапе
	// объявления, см. collectEntities).
	IsExternStruct bool
	// IsStruct - см. ast.EntityDecl.IsStruct - нужно здесь, чтобы
	// collectEntities мог отказать в "наследует"/"реализует" на базовую
	// сущность, которая сама - "структура" (наследование/интерфейсы
	// ограничены ссылочными типами), не только в обратную сторону
	// (парсер уже не пропускает "структура ... наследует ...").
	IsStruct bool
	// IsAbstract - см. ast.EntityDecl.IsAbstract - нужно здесь, чтобы
	// analyzeNewExpr мог отказать в "новый Тип(...)" на абстрактную
	// сущность (internal/semantic/expr.go).
	IsAbstract bool
	// IsMutex - см. internal/semantic/mutex.go - системная сущность
	// "Блокировка", регистрируется лениво при первой встрече имени типа,
	// тем же принципом, что IsList/IsDict/IsNullable/IsPointer выше.
	IsMutex bool
	// IsSemaphore - см. internal/semantic/semaphore.go - системная
	// сущность "Семафор", регистрируется лениво при первой встрече имени
	// типа, тем же принципом, что IsMutex выше.
	IsSemaphore bool
	// IsTransaction - см. internal/semantic/dbtx.go - системная сущность
	// "Транзакция", регистрируется лениво при первой встрече имени типа
	// (обычно - как объявленный тип параметра колбэка "начать_транзакцию"),
	// тем же принципом, что IsSemaphore выше.
	IsTransaction bool
	// BaseName - имя базовой сущности ("наследует"), "" - нет
	// базовой. FieldNames/FieldTypes/... выше уже включают унаследованные
	// поля (плоский список, база первой) - см. collectEntities.
	BaseName string
	// OwnInterfaces - имена интерфейсов из СОБСТВЕННОГО "реализует" этой
	// сущности (не включает унаследованные через BaseName - см.
	// entitySatisfies, которая обходит цепочку BaseName сама, а не
	// полагается на предварительно свёрнутый список).
	OwnInterfaces []string

	// HasCtor/CtorParams/CtorParamNames - явный конструктор
	// ("конструктор(...) { ... }"), собственный для ЭТОЙ сущности - НИКОГДА
	// не наследуется/не сплющивается с базовым (applyInheritance намеренно
	// не копирует их из baseSig - см. её package doc): у сущности без
	// своего "конструктор" HasCtor всегда false, даже если он есть у
	// базовой.
	//
	// HasCtor==false - "новый Тип(...)" допускает только "новый Тип()" без
	// аргументов (поля получают значения по умолчанию/зануляются - см.
	// analyzeNewExpr, docs/SPEC.md) - "как в нормальных языках", неявный
	// конструктор ничего не знает про поля. Если у сущности есть поле без
	// значения по умолчанию, которое обязательно нужно задать - это ровно
	// причина написать явный "конструктор(...)".
	//
	// HasCtor==true - "новый Тип(...)" вызывает тело конструктора: либо
	// позиционно (аргументы по порядку CtorParams), либо именованно (по
	// именам CtorParamNames, любой порядок) - параметры конструктора не
	// обязаны совпадать с полями сущности - см. analyzeNewExpr. Хвостовые
	// параметры МОГУТ иметь собственное значение по умолчанию
	// (CtorParamDefaults) - тем же принципом, что и у обычных функций
	// (funcSig.ParamDefaults), не путать со значением по умолчанию ПОЛЯ
	// (FieldDefaultExprs) - это про параметры самого явного "конструктор(...)".
	HasCtor           bool
	CtorParams        []ir.Type
	CtorParamNames    []string
	CtorParamDefaults []ast.Expr

	// IsExternOpaque - зарегистрирована "внешняя сущность <ИмяC> [как
	// <Псевдоним>]" (FFI), не обычная сущность - у неё нет ни полей, ни
	// методов, ни конструктора (FieldNames/Methods выше остаются пустыми,
	// HasCtor - false): resolveTypeName резолвит такое имя в
	// ir.ExternEntityType, не ir.EntityType, и analyzeNewExpr отдельно
	// отклоняет "новый" на этот флаг - единственная причина, по которой
	// этого признака недостаточно вывести из "пустых полей" (у структуры с
	// нулём полей тоже пустой FieldNames, но "новый" для неё легален).
	IsExternOpaque bool
}

func (s entitySig) constIndex(name string) int {
	for i, n := range s.ConstNames {
		if n == name {
			return i
		}
	}
	return -1
}

func (s entitySig) fieldIndex(name string) int {
	for i, n := range s.FieldNames {
		if n == name {
			return i
		}
	}
	return -1
}

func (s entitySig) staticFieldIndex(name string) int {
	for i, n := range s.StaticFieldNames {
		if n == name {
			return i
		}
	}
	return -1
}
