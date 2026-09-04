// Package ir описывает промежуточное представление программы - слой между
// ast.Program и backend'ами (cbackend, будущий interpreter), не зависящий
// ни от русской грамматики (имена - уже проверенные идентификаторы, не
// произвольные словоформы падежного сопоставления), ни от конкретного
// backend'а (нет понятия mangleName/mangleType/C-типов - это решает
// cbackend или interpreter по-своему).
//
// internal/semantic производит *Program из *ast.Program (семантический
// анализ - проверка типов, разрешение имён) - см. пакет semantic для
// деталей. internal/compiler.Compiler.Build идёт через этот путь
// (semantic.Analyze -> cbackend.GenerateFromIR), не напрямую по AST.
package ir

import (
	"github.com/IlyaFeoktistov/Rech/internal/dbscheme"
	"github.com/IlyaFeoktistov/Rech/internal/structtag"
)

// Program - корень IR-дерева программы.
type Program struct {
	Functions []*Function
	Entities  []*Entity
	Generics  []GenericInstantiation
	// Nullables - конкретные "?Тип", встретившиеся в программе, без
	// повторов - см. package doc NullableInstantiation.
	Nullables []NullableInstantiation
	// Pointers - конкретные "Указатель<T>", встретившиеся в программе, без
	// повторов - см. package doc PointerInstantiation.
	Pointers    []PointerInstantiation
	ExternFuncs []*ExternFuncDecl
	Interfaces  []*Interface
	Callables   []CallableSig
	// ParseTargets - типы, встретившиеся хоть раз как <Тип> у "разобрать
	// JSON <выражение> как <Тип>" где-либо в программе, без повторов - см.
	// internal/semantic/parse.go, package doc. cbackend генерирует функцию
	// извлечения для каждого элемента (и рекурсивно для любого достижимого
	// из него типа поля/элемента списка - не входит в этот список отдельно,
	// см. internal/backend/cbackend/json_extract.go).
	ParseTargets []Type
	// SerializeTargets - типы, встретившиеся хоть раз как значение
	// "сериализовать JSON <выражение>" - тот же принцип, что и
	// ParseTargets (см. package doc там), но для обратного направления.
	SerializeTargets []Type
	// XMLParseTargets/XMLSerializeTargets - те же две роли, что
	// ParseTargets/SerializeTargets, но для "разобрать XML"/"сериализовать
	// XML" - отдельные списки (не общие с JSON), потому что backend
	// генерирует РАЗНЫЕ функции извлечения/сериализации для одного и того
	// же Rech-типа в зависимости от формата (rech_json_extract_<T> против
	// rech_xml_extract_<T>) - совместное использование одного списка
	// потребовало бы нести формат в каждой записи.
	XMLParseTargets     []Type
	XMLSerializeTargets []Type
	// ReflectTargets - конкретные сущности/структуры/интерфейсы,
	// встретившиеся хоть раз как аргумент боксирования ("(Значение) x"),
	// как имя у "тип <Имя>", или как цель "<значение> как <Имя>", без
	// повторов - тем же принципом, что и ParseTargets: backend генерирует
	// дескриптор типа (имя, таблицы полей/методов) для каждого элемента и
	// рекурсивно для любого достижимого из него типа поля/параметра
	// метода/результата (internal/backend/cbackend/reflect.go). У
	// интерфейса (Type.Kind == KindInterface) дескриптор несёт только
	// собственные объявленные методы интерфейса (без полей/конструктора,
	// без invoker'ов - вызов метода ВСЕГДА идёт через дескриптор
	// КОНКРЕТНОЙ сущности, боксированной в значение, см.
	// InterfaceImplementors ниже). Примитивы дескрипторы получают всегда,
	// безусловно, но только если UsesReflection ниже - в этот список они
	// не попадают.
	ReflectTargets []Type
	// InterfaceImplementors - имя интерфейса -> имена ВСЕХ сущностей
	// программы, реально его реализующих (напрямую "реализует" или через
	// цепочку "наследует" у сущности/интерфейса - internal/semantic.
	// entitySatisfies, транзитивно, в отличие от Entity.Interfaces
	// (собственный, ОДНОУРОВНЕВЫЙ список каждой сущности)). Нужен для
	// "<значение> как <ИмяИнтерфейса>" (docs/ROADMAP.md) - боксированное
	// значение несёт дескриптор КОНКРЕТНОЙ сущности (v.type), поэтому
	// проверка совпадения при разбоксировании в интерфейс не может быть
	// сравнением с ОДНИМ ожидаемым дескриптором (как у остальных типов,
	// rech_unbox_check) - только "встречается ли v.type среди дескрипторов
	// хотя бы одной сущности из этого списка". Вычисляется один раз в
	// internal/semantic для КАЖДОГО объявленного интерфейса программы
	// (не только зарегистрированных как ReflectTargets - дёшево, не стоит
	// усложнять условием), даже если сам список окажется пуст (интерфейс
	// без единой реализующей сущности в программе).
	InterfaceImplementors map[string][]string
	// EntityInterfaces - обратный индекс к InterfaceImplementors (имя
	// сущности -> имена ВСЕХ интерфейсов, которые она реализует, тем же
	// транзитивным принципом - entitySatisfies, вычислен в ОДНОМ проходе с
	// InterfaceImplementors). Нужен методам "Тип" "число_интерфейсов"/
	// "получить_интерфейс"/"реализует_ли" (docs/ROADMAP.md, паритет с C#
	// Type.GetInterfaces/IsAssignableFrom) - в отличие от Entity.Interfaces
	// (AST-уровня, собственный ОДНОУРОВНЕВЫЙ список "реализует" без
	// раскрытия цепочки "наследует" у самого интерфейса, нужный для ДРУГИХ
	// целей - internal/backend/cbackend/vtable.go, computeDispatch/
	// buildIsaTable), EntityInterfaces уже включает интерфейсы,
	// унаследованные ЧЕРЕЗ "наследует" другого интерфейса (сущность,
	// реализующая только производный интерфейс, всё равно "реализует" и
	// его базовый - тот же принцип, что и entitySatisfies).
	EntityInterfaces map[string][]string
	// UsesReflection - хоть раз в программе встретилась "тип <Имя>",
	// "(Значение) x" или метод "Значение"/"Тип" - в отличие от
	// ReflectTargets (только сущности/структуры/интерфейсы), верно и для
	// программы, которая рефлексирует только примитивы ("тип Целое", без
	// единой сущности) - ReflectTargets в этом случае пуст, но дескрипторы
	// примитивов (rech_type_meta_t) backend всё равно обязан emit'ить.
	UsesReflection bool

	// UsesDB - хоть раз в программе вызвана "подключиться_к_бд"/
	// "запросить"/"выполнить"/"закрыть_бд" - backend'у нужно это знать,
	// чтобы решить, эмитить ли рантайм БД (три драйвера сразу,
	// диспетчеризация по схеме строки подключения в рантайме - см. package
	// doc internal/backend/cbackend/db.go) и линковать ли клиентские
	// библиотеки (-lsqlite3/-lpq/-lmysqlclient).
	UsesDB bool
	// DBBackends - какие конкретно СУБД (SQLite/PostgreSQL/MySQL) реально
	// встречены среди literal-аргументов "подключиться_к_бд(...)" во всей
	// программе (internal/dbscheme.Set) - backend'у это нужно, чтобы
	// эмитировать (и линковать) рантайм только тех драйверов, которые
	// действительно могут понадобиться, а не всех трёх безусловно (см.
	// docs/ROADMAP.md - toolchain раньше линковал -lsqlite3/-lpq/
	// -lmysqlclient безусловно при любом использовании БД). Dynamic (см.
	// package doc dbscheme.Set) - безопасный откат к "все три", когда
	// строка подключения хотя бы раз построена не литералом.
	DBBackends dbscheme.Set
	// DBRowTargets - сущности, встретившиеся хоть раз ЭЛЕМЕНТОМ цели
	// "запросить(...) как Список<Тип>" (DBRowsAsExpr, internal/semantic/
	// db_rows.go), без повторов - тем же принципом, что и ParseTargets:
	// backend генерирует функцию маппинга ОДНОЙ строки результата
	// (Словарь<Строка, Значение>) в Тип один раз на каждый встретившийся
	// Тип, не безусловно для каждой сущности программы.
	DBRowTargets []Type
}

// ExternFuncDecl соответствует "внешняя функция <CName> [как <Alias>](...)"
// (FFI) - объявление сигнатуры уже существующей C-функции без тела, см.
// ast.ExternFuncDecl. Params - только типы (без имён - C-прототип не
// нуждается в именах параметров, а внутри Rech-программы к ним обращаются
// только по имени параметра из AST, которое уже не нужно после того, как
// семантический анализ проверил число/типы аргументов вызова).
// internal/backend/cbackend.GenerateFromIR - единственный потребитель, у
// которого это реально работает: internal/interp не умеет вызывать
// произвольный C-символ без настоящей динамической линковки (dlopen/dlsym),
// поэтому Run явно отказывается сам, увидев непустой ExternFuncs -
// semantic.Analyze производит IR с ним как обычно (это ограничение только
// interpreter-пути, не самого IR).
type ExternFuncDecl struct {
	CName      string
	Alias      string
	Params     []Type
	ReturnType Type
	HasReturn  bool
}

// GenericInstantiation - конкретная инстанциация встроенной generic-
// коллекции ("Список"/"Стопка"/"Очередь"/"Словарь"), использованная в
// программе - "Список<Целое>" и т. п. Backend синтезирует по ней
// C-структуру и функции сам (growable array или хеш-таблица - та же
// реализация, что и в AST-пути, cbackend.listCollectionCCode/
// stackCollectionCCode/queueCollectionCCode/dictCollectionCCode) - IR
// только называет, какой шаблон и с каким элементом встретился, реализация
// не видна в Rech (нет доступных полей), поэтому IR-узла Entity для
// инстанциации не заводится - в отличие от пользовательских сущностей, тут
// нечего показывать через Fields.
type GenericInstantiation struct {
	Name     string // "Список<Целое>"
	Template string // "Список" | "Стопка" | "Очередь" | "Словарь"
	ElemType Type
	// KeyType - тип ключа, валиден только у Template == "Словарь" (тип
	// значения при этом несёт ElemType выше, то же поле, что и у
	// Список/Стопка/Очередь - переиспользуется, не дублируется отдельным
	// ValueType) - нулевое значение у остальных трёх шаблонов.
	KeyType Type
	// NeedsSafeIndex - хотя бы раз в программе эта инстанциация была
	// употреблена через безопасную индексацию "[]" (ListIndexExpr/
	// DictIndexExpr) - backend генерирует дополнительную функцию
	// безопасного доступа (не паникующую, возвращающую "?T"/"?В") только
	// для реально использованных так инстанциаций, тем же принципом
	// "не безусловно", что и у ParseTargets/ReflectTargets.
	NeedsSafeIndex bool
	// NeedsListLiteral - хотя бы раз в программе эта инстанциация была
	// построена литералом "[элемент, ...]" (ListLiteralExpr) - тем же
	// принципом, что и NeedsSafeIndex, но для построения из литерала, не
	// для чтения по индексу. Валидно только у Template == "Список".
	NeedsListLiteral bool
}

// CallableSig - конкретная сигнатура "функции как значения" (
// "Вызываемый<...>..."), встретившаяся в программе - как
// GenericInstantiation, называет одну точку кодогенерации без отдельного
// узла в дереве типов (Type для KindCallable несёт только Name, готовый
// разобранный список Params/Result живёт здесь, потому что Type не может
// хранить slice и остаться сравнимым через "=="). Backend один раз
// генерирует C-структуру (указатель на функцию + указатель на окружение
// замыкания) и функцию вызова по Name.
type CallableSig struct {
	Name   string // "Вызываемый<Целое, Строка>Целое", он же Type.Entity
	Params []Type
	Result *Type // nil - без результата
}

// Function - функция программы после семантического анализа. Name - имя
// ровно как в Rech-исходнике (например, "главная") - маппинг на
// C-специфичное имя (например, main) - решение backend'а, не IR.
//
// Строгий и естественный синтаксис вызова (docs/SPEC.md) уже неразличимы
// на уровне AST (парсер раскладывает падежные аргументы естественного
// вызова по позиции параметра, как и строгий) - Params здесь просто по
// позиции, падежных ролей (ast.Param.CaseWord) IR не хранит вообще, они
// нужны были только парсеру для сопоставления, не после него.
type Function struct {
	Name       string
	Params     []Param
	ReturnType Type
	HasReturn  bool // false - нет объявленного типа результата
	Body       []Stmt
	// File - путь исходного файла, из которого объявлена функция (см.
	// ast.FunctionDecl.File) - нужен отладчику (internal/debugger) для
	// сопоставления точки останова "файл:строка" с конкретным кадром вызова.
	File string
	// Doc - см. ast.FunctionDecl.Doc.
	Doc string
	// Exported - см. ast.FunctionDecl.Exported.
	Exported bool
}

// Param - один параметр функции/метода по позиции.
type Param struct {
	Name string
	Type Type
}

// Entity - сущность программы после семантического анализа. Объявленные
// состояния сами по себе не несут отдельного поля здесь - переход между
// ними (ir.BecomeStmt) уже проверен во время анализа и не требует
// собственного C-представления состояния как такового.
type Entity struct {
	Name string
	// IsStruct - объявлена словом "структура" (передаётся по значению -
	// копия при переменной/присваивании/параметре/поле/возврате), а не
	// "сущность"/"класс" (по умолчанию - по ссылке, один и тот же объект
	// разделяется между всеми именами, ссылающимися на него). См.
	// ast.EntityDecl.IsStruct - переносится сюда без изменений во время
	// семантического анализа.
	IsStruct bool
	// IsAbstract - см. ast.EntityDecl.IsAbstract - нельзя создать экземпляр
	// напрямую (проверено раньше, в internal/semantic/expr.go,
	// analyzeNewExpr - до этой точки программа уже не дошла бы, если бы
	// нарушала это). Здесь нужен только для полноты IR-представления
	// (internal/lsp), cbackend решает всё через Method.IsAbstract выше.
	IsAbstract bool
	// IsMutex - системная сущность "Блокировка" (синхронизация между
	// потоками) - не объявляется пользователем (нет ast.EntityDecl), не
	// имеет Fields/Methods/Ctor - её C-представление (нативный
	// pthread_mutex_t/CRITICAL_SECTION) и оба метода (захватить/освободить)
	// целиком синтезированы internal/backend/cbackend/mutex.go, тем же
	// принципом, что и generic-коллекции (нет ir.Stmt-тел для перевода в
	// C - блокировку/разблокировку не выразить инструкциями Rech). См.
	// internal/semantic/mutex.go.
	IsMutex bool
	// BaseName - имя базовой сущности после "наследует", "" - нет
	// базовой сущности. Fields ниже уже включает унаследованные поля
	// (плоский список - поля базовой сущности первыми, в её собственном
	// порядке, затем собственные новые поля - это и есть встраивание
	// базовой C-структуры первым членом на уровне представления, без
	// отдельного уровня вложенности, см. package doc
	// internal/backend/cbackend/vtable.go). Methods - НЕ включает
	// унаследованные-неизменённые методы (только новые/переопределённые,
	// объявленные прямо в этой сущности) - у неизменённого унаследованного
	// метода уже есть C-функция на базовой сущности, дублировать её не
	// нужно - генератор vtable (cbackend) сам находит нужную по цепочке
	// BaseName при построении таблицы этой сущности.
	BaseName string
	// Interfaces - имена интерфейсов после "реализует", собственные для
	// этой сущности (не включает интерфейсы базовой -
	// cbackend/семантика учитывают их отдельно через BaseName).
	Interfaces []string
	Constants  []Constant
	Fields     []Field
	// StaticFields - поля, объявленные с модификатором "статический"
	// (ast.Field.IsStatic): одно хранилище на всю сущность сразу, не на
	// экземпляр - НЕ входит в Fields/C-структуру экземпляра, backend
	// заводит для каждого отдельную глобальную переменную и инициализирует
	// её Default РОВНО один раз при старте программы (см.
	// internal/backend/cbackend/statics.go, internal/interp/statics.go).
	// Default никогда не nil (поле без "= выражение" получает нулевое
	// значение типа тем же способом, что и обычное поле без значения по
	// умолчанию). Доступ - ir.StaticFieldExpr/ir.StaticFieldAssign
	// ("Тип::имя"), никогда через получатель-экземпляр.
	StaticFields []Field
	Methods      []*Method
	// Ctor - явное тело конструктора этой сущности, nil - нет (у "новый
	// Тип(...)" остаётся неявное позиционное присваивание Fields один-в-
	// один - backend решает это сам по Ctor == nil, не по отдельному
	// флагу). Никогда не наследуется/не сплющивается с базовым - Ctor у
	// потомка без собственного "конструктор" всегда nil, даже если у базы
	// он есть (см. package doc internal/semantic/inheritance.go,
	// applyInheritance) - "родитель(...)" внутри Ctor - явный, осознанный
	// вызов тела ИМЕННО базового конструктора, не подразумеваемый.
	Ctor *Method
	// Doc - см. ast.EntityDecl.Doc - копируется без изменений семантическим
	// анализом, нужен только internal/lsp (hover), сам компилятор его
	// нигде не читает.
	Doc string
	// Exported - см. ast.EntityDecl.Exported.
	Exported bool

	// StateNames/StateBitOffset/HasStateFlags - см. package doc
	// internal/semantic.entitySig (те же поля, скопированы без изменений):
	// раскладка рантайм-хранилища typestate ("state_flags") - нужна там,
	// где established на этапе компиляции не может доказать переход
	// (StateCheckExpr вместо ошибки компиляции).
	StateNames     []string
	StateBitOffset int
	HasStateFlags  bool
}

// StateBit возвращает абсолютный номер бита СОБСТВЕННОГО состояния этой
// сущности name (объявленного её же "состояние:", не унаследованного - как
// и StateNames) в общем "state_flags" - false, если у этой сущности нет
// состояния с таким именем.
func (e *Entity) StateBit(name string) (uint, bool) {
	for i, n := range e.StateNames {
		if n == name {
			return uint(e.StateBitOffset + i), true
		}
	}
	return 0, false
}

// Constant - одно константное поле сущности
type Constant struct {
	Name  string
	Value Expr
	Type  Type
	// Doc - см. ast.ConstDecl.Doc.
	Doc string
}

// Field - одно поле сущности, в порядке объявления (порядок значим -
// позиционный конструктор и порядок полей в C-структуре зависят от него).
type Field struct {
	Name string
	Type Type
	// Doc - см. ast.Field.Doc.
	Doc string
	// Default - разрешённое значение по умолчанию (см. ast.Field.Default),
	// nil - поля без значения по умолчанию. Используется в двух местах: (1)
	// внутри "новый Тип(...)" для поля, пропущенного вызывающей стороной
	// (см. package doc ir.NewExpr) - подставляется вместо позиционного
	// аргумента ещё во время семантического анализа, backend об этом не
	// знает; (2) когда у сущности есть явный Ctor - backend присваивает его
	// полю ДО тела конструктора (см. package doc Entity.Ctor).
	Default Expr
	// Tag - см. ast.Field.Tag - копируется без изменений, разбирается лениво
	// через JSONKey/internal/structtag.Parse там, где нужен конкретный ключ.
	Tag string
}

// JSONKey - внешнее имя поля в JSON: значение ключа "json" тега (см.
// internal/structtag), если оно задано, иначе - само имя поля Rech. Нужен
// и разбору ("разобрать JSON ... как <Тип>"), и сериализации ("сериализовать
// JSON ..."), чтобы оставаться согласованными друг с другом (backend'ы
// вызывают этот же метод, а не смотрят на f.Name напрямую) - см.
// internal/backend/cbackend/json_extract.go, json_serialize.go,
// internal/interp/json.go, json_serialize.go.
func (f Field) JSONKey() string {
	if key := structtag.Parse(f.Tag).Get("json"); key != "" {
		return key
	}
	return f.Name
}

// SQLKey - внешнее имя поля как имя колонки результата запроса: значение
// ключа "sql" тега, если оно задано, иначе - само имя поля - тот же
// принцип, что JSONKey выше, для "запросить(...) как Список<Тип>"
// (internal/semantic/db_rows.go, internal/backend/cbackend/db_rows.go,
// internal/interp/db_rows.go).
func (f Field) SQLKey() string {
	if key := structtag.Parse(f.Tag).Get("sql"); key != "" {
		return key
	}
	return f.Name
}

// Method - метод сущности: как Function, но получатель ("этот") неявный -
// его тип есть Type{Kind: KindEntity, Entity: <имя сущности-владельца>},
// backend передаёт его отдельным, скрытым от Rech параметром (в cbackend -
// указатель, чтобы мутация поля через "этот.поле = ..." была видна после
// возврата).
type Method struct {
	Name       string
	Params     []Param
	ReturnType Type
	HasReturn  bool
	Body       []Stmt
	// IsAbstract - метод без реализации (ast.MethodDecl.IsAbstract, Body
	// пуст не потому, что тело буквально "{}", а потому, что тела не было
	// вообще) - только у сущностей с Entity.IsAbstract == true (см. package
	// doc там). cbackend/ir_generator.go не генерирует ни прототип, ни
	// определение C-функции для такого метода - вызвать его напрямую
	// невозможно, семантический анализ гарантирует, что у любой СОЗДАВАЕМОЙ
	// сущности слот диспетчеризации этого имени указывает на настоящую
	// реализацию дальше по цепочке "наследует" (см. internal/backend/
	// cbackend/vtable.go, findVtableSlotFunc).
	IsAbstract bool
	// IsStatic - объявлен с модификатором "статический" (ast.MethodDecl.
	// IsStatic) - принадлежит самой сущности, не экземпляру: у него нет
	// скрытого параметра "this_" (см. package doc Method выше), backend
	// генерирует его как обычную функцию (cParamListIR, не
	// cMethodParamListIR), вызывается через ir.StaticMethodCallExpr
	// ("Тип::метод(...)"), не ir.MethodCallExpr. Не участвует в vtable
	// (диспетчеризация виртуальна только для методов экземпляра) и не
	// наследуется/не переопределяется.
	IsStatic bool
	// File - файл сущности, объявившей этот метод/конструктор (см.
	// ast.EntityDecl.File) - НЕ обязательно совпадает с файлом сущности
	// получателя вызова: неизменённый унаследованный метод несёт файл
	// БАЗОВОЙ сущности, где он реально написан, не сущности-потомка.
	File string
	// Doc - см. ast.MethodDecl.Doc (тем же принципом, что и File - метод,
	// унаследованный без переопределения, несёт Doc БАЗОВОЙ сущности).
	Doc string
}

// Stmt - общий маркерный интерфейс инструкций IR (как ast.Stmt).
