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
	// состояния (шаг 4.1, "параметр: АвторизованныйПользователь"), краткая
	// форма самого состояния ("авторизован") - тот же индекс, что и Params
	// (Params[i] уже резолвлен в тип БАЗОВОЙ сущности, alias-имя потеряно,
	// см. resolveTypeName - без этого поля flow-sensitive проверка шага 5
	// не знала бы, какой именно переход состояния требует параметр).
	// "" - параметр объявлен без производного типа состояния (обычный тип,
	// в т.ч. обычная сущность без каких-либо требований к состоянию).
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

	// States - для проверки BecomeStmt ("получатель становится
	// <форма>") - ключ ast.StateDecl.BecomeForm (полная форма,
	// согласованная по роду/числу И творительным падежом - ровно то, что
	// обязан написать программист, см. package doc ast.StateDecl.BecomeForm),
	// значение - краткая форма (ast.StateDecl.Name, канонический
	// идентификатор состояния, используемый дальше везде - established,
	// ir.BecomeStmt.State, ParamStates). nil у сущностей без "состояние:".
	States map[string]string

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
	// BaseName - имя базовой сущности ("наследует"), "" - нет
	// базовой. FieldNames/FieldTypes/... выше уже включают унаследованные
	// поля (плоский список, база первой) - см. collectEntities.
	BaseName string
	// OwnInterfaces - имена интерфейсов из СОБСТВЕННОГО "реализует" этой
	// сущности (не включает унаследованные через BaseName - см.
	// entitySatisfies, которая обходит цепочку BaseName сама, а не
	// полагается на предварительно свёрнутый список).
	OwnInterfaces []string

	// HasCtor/CtorParams - явный конструктор ("конструктор(...) { ... }"),
	// собственный для ЭТОЙ сущности - НИКОГДА не наследуется/не
	// сплющивается с базовым (applyInheritance намеренно не копирует их из
	// baseSig - см. её package doc): у сущности без своего "конструктор"
	// HasCtor всегда false, даже если он есть у базовой. HasCtor==false -
	// "новый Тип(...)" остаётся неявным позиционным/именованным
	// присваиванием полей (FieldNames/FieldTypes выше); HasCtor==true -
	// "новый Тип(...)" вызывает тело конструктора, аргументы - строго
	// позиционные по CtorParams (именованная форма недоступна - параметры
	// конструктора не обязаны совпадать с полями).
	HasCtor    bool
	CtorParams []ir.Type

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
