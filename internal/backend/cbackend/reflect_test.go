package cbackend

import (
	"strings"
	"testing"

	"github.com/IlyaFeoktistov/Rech/internal/ir"
)

// TestGenerateFromIRTypeOfPrimitiveEmitsDescriptor - "тип Целое" - адрес
// статического дескриптора примитива, эмитится безусловно при
// UsesReflection (даже без единой сущности в ReflectTargets).
func TestGenerateFromIRTypeOfPrimitiveEmitsDescriptor(t *testing.T) {
	main := &ir.Function{Name: "главная", Body: []ir.Stmt{
		&ir.VarDecl{Name: "т", Value: &ir.TypeOfExpr{Type: ir.TypeInt}},
	}}
	out, err := GenerateFromIR(&ir.Program{Functions: []*ir.Function{main}, UsesReflection: true})
	if err != nil {
		t.Fatalf("неожиданная ошибка: %v", err)
	}
	if !strings.Contains(out, "v_t_INIT = (&rech_type_meta_celoe);") {
		t.Fatalf("ожидался адрес дескриптора Целое, получено:\n%s", out)
	}
	if !strings.Contains(out, `static const rech_type_meta_t rech_type_meta_celoe = {"Целое", NULL, 0, NULL, 0, NULL, 0, NULL, NULL, 0, ""};`) {
		t.Fatalf("ожидался статический дескриптор примитива Целое, получено:\n%s", out)
	}
}

// TestGenerateFromIRBoxPrimitiveCast - "(Значение) 5" - инлайновый
// struct-литерал rech_any_t, без рантайм-хелпера (примитив).
func TestGenerateFromIRBoxPrimitiveCast(t *testing.T) {
	main := &ir.Function{Name: "главная", Body: []ir.Stmt{
		&ir.PrintStmt{Value: &ir.CastExpr{Value: &ir.IntLiteral{Value: 5}, To: ir.TypeAny}},
	}}
	out, err := GenerateFromIR(&ir.Program{Functions: []*ir.Function{main}, UsesReflection: true})
	if err != nil {
		t.Fatalf("неожиданная ошибка: %v", err)
	}
	if !strings.Contains(out, "(rech_any_t){.type = &rech_type_meta_celoe, .v.i = (long long)(5LL)}") {
		t.Fatalf("ожидалось боксирование Целое в rech_any_t, получено:\n%s", out)
	}
}

// TestGenerateFromIRAsExprUsesCheckedUnbox - "значение как Целое" -
// rech_unbox_check (паника при несовпадении типа) перед чтением union.
func TestGenerateFromIRAsExprUsesCheckedUnbox(t *testing.T) {
	main := &ir.Function{Name: "главная", Body: []ir.Stmt{
		&ir.VarDecl{Name: "коробка", Value: &ir.CastExpr{Value: &ir.IntLiteral{Value: 5}, To: ir.TypeAny}},
		&ir.PrintStmt{Value: &ir.AsExpr{Value: &ir.Identifier{Name: "коробка", Type: ir.TypeAny}, To: ir.TypeInt}},
	}}
	out, err := GenerateFromIR(&ir.Program{Functions: []*ir.Function{main}, UsesReflection: true})
	if err != nil {
		t.Fatalf("неожиданная ошибка: %v", err)
	}
	if !strings.Contains(out, "((long long)(rech_unbox_check(v_korobka, &rech_type_meta_celoe)).v.i)") {
		t.Fatalf("ожидался rech_unbox_check перед распаковкой, получено:\n%s", out)
	}
}

// TestGenerateFromIRAnyMethodsDispatchToReflectHelpers - методы "Значение"
// генерируются как вызовы отдельных рантайм-хелперов, не через обычный
// mangleMethod-путь сущности/интерфейса.
func TestGenerateFromIRAnyMethodsDispatchToReflectHelpers(t *testing.T) {
	recv := &ir.Identifier{Name: "коробка", Type: ir.TypeAny}
	main := &ir.Function{Name: "главная", Body: []ir.Stmt{
		&ir.VarDecl{Name: "коробка", Value: &ir.CastExpr{Value: &ir.IntLiteral{Value: 5}, To: ir.TypeAny}},
		&ir.PrintStmt{Value: &ir.MethodCallExpr{Receiver: recv, Method: "тип_значения", Type: ir.TypeTypeInfo, HasReturn: true}},
		&ir.PrintStmt{Value: &ir.MethodCallExpr{Receiver: recv, Method: "получить_поле", Args: []ir.Expr{&ir.StringLiteral{Value: "имя"}}, Type: ir.TypeAny, HasReturn: true}},
		&ir.ExprStmt{MethodCall: &ir.MethodCallExpr{Receiver: recv, Method: "установить_поле", Args: []ir.Expr{&ir.StringLiteral{Value: "имя"}, &ir.CastExpr{Value: &ir.IntLiteral{Value: 1}, To: ir.TypeAny}}}},
	}}
	out, err := GenerateFromIR(&ir.Program{Functions: []*ir.Function{main}, UsesReflection: true})
	if err != nil {
		t.Fatalf("неожиданная ошибка: %v", err)
	}
	if !strings.Contains(out, "(v_korobka).type") {
		t.Fatalf("ожидалось чтение .type у тип_значения, получено:\n%s", out)
	}
	if !strings.Contains(out, `rech_reflect_get_field(v_korobka, "имя")`) {
		t.Fatalf("ожидался вызов rech_reflect_get_field, получено:\n%s", out)
	}
	if !strings.Contains(out, `rech_reflect_set_field(v_korobka, "имя",`) {
		t.Fatalf("ожидался вызов rech_reflect_set_field, получено:\n%s", out)
	}
}

// TestGenerateFromIRTypeInfoMethodsDispatchToReflectHelpers - методы "Тип"
// (имя/число_полей/...) читают поля дескриптора напрямую или через
// индексные хелперы rech_field_at/rech_method_at/rech_param_at.
func TestGenerateFromIRTypeInfoMethodsDispatchToReflectHelpers(t *testing.T) {
	recv := &ir.Identifier{Name: "т", Type: ir.TypeTypeInfo}
	main := &ir.Function{Name: "главная", Body: []ir.Stmt{
		&ir.VarDecl{Name: "т", Value: &ir.TypeOfExpr{Type: ir.TypeInt}},
		&ir.PrintStmt{Value: &ir.MethodCallExpr{Receiver: recv, Method: "имя", Type: ir.TypeString, HasReturn: true}},
		&ir.PrintStmt{Value: &ir.MethodCallExpr{Receiver: recv, Method: "число_полей", Type: ir.TypeInt, HasReturn: true}},
		&ir.PrintStmt{Value: &ir.MethodCallExpr{Receiver: recv, Method: "имя_поля", Args: []ir.Expr{&ir.IntLiteral{Value: 0}}, Type: ir.TypeString, HasReturn: true}},
	}}
	out, err := GenerateFromIR(&ir.Program{Functions: []*ir.Function{main}, UsesReflection: true})
	if err != nil {
		t.Fatalf("неожиданная ошибка: %v", err)
	}
	if !strings.Contains(out, "(v_t)->name") {
		t.Fatalf("ожидалось чтение ->name, получено:\n%s", out)
	}
	if !strings.Contains(out, "(v_t)->field_count") {
		t.Fatalf("ожидалось чтение ->field_count, получено:\n%s", out)
	}
	if !strings.Contains(out, "rech_field_at(v_t, 0LL)->name") {
		t.Fatalf("ожидался вызов rech_field_at, получено:\n%s", out)
	}
}

// TestGenerateFromIREntityReflectTargetGeneratesAccessorsAndDescriptor -
// сущность из prog.ReflectTargets получает геттер/сеттер поля, инвокер
// метода, обёртку конструктора и собственный дескриптор типа.
func TestGenerateFromIREntityReflectTargetGeneratesAccessorsAndDescriptor(t *testing.T) {
	entity := &ir.Entity{
		Name:   "Точка",
		Fields: []ir.Field{{Name: "x", Type: ir.TypeInt}},
		Methods: []*ir.Method{
			{Name: "координата", ReturnType: ir.TypeInt, HasReturn: true,
				Body: []ir.Stmt{&ir.ReturnStmt{Value: &ir.FieldAccess{Receiver: &ir.ThisExpr{Entity: "Точка"}, Field: "x", Type: ir.TypeInt}}}},
		},
	}
	main := &ir.Function{Name: "главная", Body: []ir.Stmt{
		&ir.VarDecl{Name: "т", Value: &ir.TypeOfExpr{Type: ir.EntityType("Точка")}},
	}}
	out, err := GenerateFromIR(&ir.Program{
		Entities: []*ir.Entity{entity}, Functions: []*ir.Function{main},
		UsesReflection: true, ReflectTargets: []ir.Type{ir.EntityType("Точка")},
	})
	if err != nil {
		t.Fatalf("неожиданная ошибка: %v", err)
	}
	if !strings.Contains(out, "static rech_any_t rech_reflect_get_tochka_x(void *self) {") {
		t.Fatalf("ожидался геттер поля x, получено:\n%s", out)
	}
	if !strings.Contains(out, "static void rech_reflect_set_tochka_x(void *self, rech_any_t v) {") {
		t.Fatalf("ожидался сеттер поля x, получено:\n%s", out)
	}
	if !strings.Contains(out, "static rech_any_t rech_reflect_invoke_tochka_koordinata(void *self, rech_any_t *args, int argc) {") {
		t.Fatalf("ожидался инвокер метода координата, получено:\n%s", out)
	}
	if !strings.Contains(out, "static rech_any_t rech_reflect_ctor_tochka(rech_any_t *args, int argc) {") {
		t.Fatalf("ожидалась обёртка конструктора, получено:\n%s", out)
	}
	if !strings.Contains(out, `static const rech_type_meta_t rech_type_meta_tochka = {"Точка",`) {
		t.Fatalf("ожидался дескриптор типа Точка, получено:\n%s", out)
	}
}

// TestGenerateFromIREntityFieldOfGenericCollectionReflects - НАЙДЕННЫЙ И
// ИСПРАВЛЕННЫЙ баг (Шаг 41/47): сущность с полем типа
// встроенной generic-коллекции ("Словарь<Строка, Значение>") среди
// ReflectTargets раньше падала "рефлексия по неизвестному типу" -
// reflectGen.ensure не отличал generic-коллекцию (нет записи в
// ctx.entities) от обычной сущности.
func TestGenerateFromIREntityFieldOfGenericCollectionReflects(t *testing.T) {
	dictType := "Словарь<Строка, Значение>"
	entity := &ir.Entity{
		Name:   "Хранилище",
		Fields: []ir.Field{{Name: "данные", Type: ir.EntityType(dictType)}},
	}
	main := &ir.Function{Name: "главная", Body: []ir.Stmt{
		&ir.VarDecl{Name: "х", Value: &ir.NewExpr{Entity: "Хранилище"}},
		&ir.VarDecl{Name: "б", Value: &ir.CastExpr{Value: &ir.Identifier{Name: "х", Type: ir.EntityType("Хранилище")}, To: ir.TypeAny}},
	}}
	out, err := GenerateFromIR(&ir.Program{
		Entities:       []*ir.Entity{entity},
		Functions:      []*ir.Function{main},
		UsesReflection: true,
		ReflectTargets: []ir.Type{ir.EntityType("Хранилище")},
		Generics:       []ir.GenericInstantiation{{Name: dictType, Template: "Словарь", KeyType: ir.TypeString, ElemType: ir.TypeAny}},
	})
	if err != nil {
		t.Fatalf("неожиданная ошибка: %v", err)
	}
	if !strings.Contains(out, `static const rech_type_meta_t `+reflectMetaName(dictType)+` = {"`+dictType+`", NULL, 0, NULL, 0, NULL, 0, NULL, NULL, 0, ""};`) {
		t.Fatalf("ожидался тривиальный дескриптор Словарь<Строка, Значение>, получено:\n%s", out)
	}
	if !strings.Contains(out, "static rech_any_t rech_reflect_get_hranilische_dannye(void *self) {") {
		t.Fatalf("ожидался геттер поля \"данные\", получено:\n%s", out)
	}
}

// TestGenerateFromIRReflectMethodWithNullableReturn - НАЙДЕННЫЙ И
// ИСПРАВЛЕННЫЙ баг: сущность среди ReflectTargets с методом, возвращающим
// "?Тип" ("статус(id): ?Задача" - examples/проекты/http_сервер), раньше
// падала компиляцией сгенерированного C двумя разными способами по
// очереди - сперва "undeclared identifier" (дескриптор "?Тип" никто не
// эмитировал, reflectMetaVarFor строит имя, но primitiveReflectDescriptors
// не знает про nullable-инстанциации), затем "aggregate value used where
// an integer was expected" (reflectBoxExprCode боксировала СТРУКТУРУ
// nullable-обёртки через ".v.i = (long long)(...)", тот же путь, что и для
// целых чисел). Живая проверка настоящим cc - examples/проекты/
// http_сервер/исходники/контейнер.rech (DI-контейнер на рефлексии).
func TestGenerateFromIRReflectMethodWithNullableReturn(t *testing.T) {
	nullableName := "?Задача"
	taskEntity := &ir.Entity{Name: "Задача", Fields: []ir.Field{{Name: "id", Type: ir.TypeString}}}
	storeEntity := &ir.Entity{
		Name: "Хранилище",
		Methods: []*ir.Method{
			{Name: "статус", Params: []ir.Param{{Name: "id", Type: ir.TypeString}},
				ReturnType: ir.NullableType(ir.EntityType("Задача")), HasReturn: true,
				Body: []ir.Stmt{&ir.ReturnStmt{Value: &ir.NullLiteral{Type: ir.NullableType(ir.EntityType("Задача"))}}}},
		},
	}
	main := &ir.Function{Name: "главная", Body: []ir.Stmt{
		&ir.VarDecl{Name: "х", Value: &ir.NewExpr{Entity: "Хранилище"}},
		&ir.VarDecl{Name: "б", Value: &ir.CastExpr{Value: &ir.Identifier{Name: "х", Type: ir.EntityType("Хранилище")}, To: ir.TypeAny}},
	}}
	out, err := GenerateFromIR(&ir.Program{
		Entities:       []*ir.Entity{taskEntity, storeEntity},
		Functions:      []*ir.Function{main},
		UsesReflection: true,
		ReflectTargets: []ir.Type{ir.EntityType("Хранилище")},
		Nullables:      []ir.NullableInstantiation{{Name: nullableName, Inner: ir.EntityType("Задача")}},
	})
	if err != nil {
		t.Fatalf("неожиданная ошибка: %v", err)
	}
	if !strings.Contains(out, `static const rech_type_meta_t `+reflectMetaName(nullableName)+` = {"`+nullableName+`", NULL, 0, NULL, 0, NULL, 0, NULL, NULL, 0, ""};`) {
		t.Fatalf("ожидался дескриптор %q, получено:\n%s", nullableName, out)
	}
	protoIdx := strings.Index(out, "static rech_any_t "+reflectNullableBoxHelperName(nullableName)+"(")
	if protoIdx < 0 {
		t.Fatalf("ожидался прототип/тело хелпера боксирования %q, получено:\n%s", nullableName, out)
	}
	invokerIdx := strings.Index(out, "static rech_any_t rech_reflect_invoke_hranilische_status(")
	if invokerIdx < 0 {
		t.Fatalf("ожидался инвокер метода статус, получено:\n%s", out)
	}
	if protoIdx >= invokerIdx {
		t.Fatalf("прототип хелпера боксирования должен появляться РАНЬШЕ инвокера метода, который его вызывает")
	}
	if !strings.Contains(out, "if (!v.has)") {
		t.Fatalf("ожидалось тело хелпера с проверкой v.has, получено:\n%s", out)
	}
}

// TestGenerateFromIRReflectTypedefsBeforeGenerics - rech_any_t обязан быть
// объявлен раньше genericsCode (Список<Значение> ссылается на него как на
// свой C-тип элемента) - иначе "unknown type name".
func TestGenerateFromIRReflectTypedefsBeforeGenerics(t *testing.T) {
	main := &ir.Function{Name: "главная", Body: []ir.Stmt{
		&ir.VarDecl{Name: "список", Value: &ir.NewExpr{Entity: "Список<Значение>"}},
	}}
	out, err := GenerateFromIR(&ir.Program{
		Functions: []*ir.Function{main}, UsesReflection: true,
		Generics: []ir.GenericInstantiation{{Name: "Список<Значение>", Template: "Список", ElemType: ir.TypeAny}},
	})
	if err != nil {
		t.Fatalf("неожиданная ошибка: %v", err)
	}
	typedefPos := strings.Index(out, "typedef struct {\n    const rech_type_meta_t *type;")
	listPos := strings.Index(out, "rech_any_t *data;")
	if typedefPos == -1 || listPos == -1 || typedefPos >= listPos {
		t.Fatalf("ожидался rech_any_t typedef раньше Список<Значение>, получено:\n%s", out)
	}
}

// TestGenerateFromIRInterfaceReflectTargetGeneratesDescriptor -
// найденная и исправленная находка ("тип ИмяИнтерфейса
// не существует") - "тип <ИмяИнтерфейса>" эмитит дескриптор БЕЗ полей и
// БЕЗ конструктора (в отличие от сущности), с собственными методами
// интерфейса, у которых invoke = NULL (диспетчеризация "значение.
// вызвать(...)" никогда не идёт через дескриптор интерфейса - см. её же
// package doc ensureInterface).
func TestGenerateFromIRInterfaceReflectTargetGeneratesDescriptor(t *testing.T) {
	iface := &ir.Interface{
		Name: "Звучащий",
		Methods: []ir.InterfaceMethod{
			{Name: "издать_звук", ReturnType: ir.TypeString, HasReturn: true},
		},
	}
	main := &ir.Function{Name: "главная", Body: []ir.Stmt{
		&ir.VarDecl{Name: "т", Value: &ir.TypeOfExpr{Type: ir.InterfaceType("Звучащий")}},
	}}
	out, err := GenerateFromIR(&ir.Program{
		Interfaces: []*ir.Interface{iface}, Functions: []*ir.Function{main},
		UsesReflection: true, ReflectTargets: []ir.Type{ir.InterfaceType("Звучащий")},
	})
	if err != nil {
		t.Fatalf("неожиданная ошибка: %v", err)
	}
	if !strings.Contains(out, `static const rech_type_meta_t rech_type_meta_zvuchaschij = {"Звучащий", NULL, 0,`) {
		t.Fatalf("ожидался дескриптор интерфейса Звучащий без полей, получено:\n%s", out)
	}
	if !strings.Contains(out, `{"издать_звук", 0, NULL, &rech_type_meta_stroka, NULL, ""},`) {
		t.Fatalf("ожидался метод \"издать_звук\" с invoke=NULL, получено:\n%s", out)
	}
}

// TestGenerateFromIRAsInterfaceUsesImplementorsCheck - найденная и
// исправленная находка ("значение как ИмяИнтерфейса не
// распаковывает боксированную реализацию") - в отличие от обычного "как
// Тип" (rech_unbox_check, точное совпадение с ОДНИМ ожидаемым
// дескриптором), использует rech_unbox_check_iface со статическим
// массивом дескрипторов ВСЕХ реализующих сущностей (Program.
// InterfaceImplementors).
func TestGenerateFromIRAsInterfaceUsesImplementorsCheck(t *testing.T) {
	iface := &ir.Interface{Name: "Звучащий", Methods: []ir.InterfaceMethod{
		{Name: "издать_звук", ReturnType: ir.TypeString, HasReturn: true},
	}}
	entity := &ir.Entity{Name: "Кот", Interfaces: []string{"Звучащий"}, Methods: []*ir.Method{
		{Name: "издать_звук", ReturnType: ir.TypeString, HasReturn: true,
			Body: []ir.Stmt{&ir.ReturnStmt{Value: &ir.StringLiteral{Value: "мяу"}}}},
	}}
	main := &ir.Function{Name: "главная", Body: []ir.Stmt{
		&ir.VarDecl{Name: "кот", Value: &ir.NewExpr{Entity: "Кот"}},
		&ir.VarDecl{Name: "коробка", Value: &ir.CastExpr{Value: &ir.Identifier{Name: "кот", Type: ir.EntityType("Кот")}, To: ir.TypeAny}},
		&ir.VarDecl{Name: "звучащий", Value: &ir.AsExpr{Value: &ir.Identifier{Name: "коробка", Type: ir.TypeAny}, To: ir.InterfaceType("Звучащий")}},
	}}
	out, err := GenerateFromIR(&ir.Program{
		Entities: []*ir.Entity{entity}, Interfaces: []*ir.Interface{iface}, Functions: []*ir.Function{main},
		UsesReflection:        true,
		ReflectTargets:        []ir.Type{ir.EntityType("Кот"), ir.InterfaceType("Звучащий")},
		InterfaceImplementors: map[string][]string{"Звучащий": {"Кот"}},
	})
	if err != nil {
		t.Fatalf("неожиданная ошибка: %v", err)
	}
	if !strings.Contains(out, "static const rech_type_meta_t *const rech_iface_impls_zvuchaschij[] = {&rech_type_meta_kot};") {
		t.Fatalf("ожидался массив реализаций rech_iface_impls_zvuchaschij, получено:\n%s", out)
	}
	if !strings.Contains(out, `rech_unbox_check_iface(v_korobka, rech_iface_impls_zvuchaschij, 1, "Звучащий")`) {
		t.Fatalf("ожидался вызов rech_unbox_check_iface, получено:\n%s", out)
	}
}

// TestGenerateFromIREntityDescriptorCarriesBaseAndInterfaces -
// паритет с C# Type.BaseType/GetInterfaces: дескриптор сущности несёт
// base (адрес дескриптора BaseName) и interfaces/interface_count
// (транзитивный список из Program.EntityInterfaces), не только
// поля/методы/конструктор, как раньше.
func TestGenerateFromIREntityDescriptorCarriesBaseAndInterfaces(t *testing.T) {
	iface := &ir.Interface{Name: "Звучащий", Methods: []ir.InterfaceMethod{
		{Name: "издать_звук", ReturnType: ir.TypeString, HasReturn: true},
	}}
	base := &ir.Entity{Name: "Животное"}
	derived := &ir.Entity{Name: "Кот", BaseName: "Животное", Interfaces: []string{"Звучащий"}, Methods: []*ir.Method{
		{Name: "издать_звук", ReturnType: ir.TypeString, HasReturn: true,
			Body: []ir.Stmt{&ir.ReturnStmt{Value: &ir.StringLiteral{Value: "мяу"}}}},
	}}
	main := &ir.Function{Name: "главная", Body: []ir.Stmt{
		&ir.VarDecl{Name: "т", Value: &ir.TypeOfExpr{Type: ir.EntityType("Кот")}},
	}}
	out, err := GenerateFromIR(&ir.Program{
		Entities: []*ir.Entity{base, derived}, Interfaces: []*ir.Interface{iface}, Functions: []*ir.Function{main},
		UsesReflection:   true,
		ReflectTargets:   []ir.Type{ir.EntityType("Кот")},
		EntityInterfaces: map[string][]string{"Кот": {"Звучащий"}},
	})
	if err != nil {
		t.Fatalf("неожиданная ошибка: %v", err)
	}
	if !strings.Contains(out, "static const rech_type_meta_t *const rech_type_meta_kot_interfaces[] = {&rech_type_meta_zvuchaschij};") {
		t.Fatalf("ожидался массив rech_type_meta_kot_interfaces, получено:\n%s", out)
	}
	if !strings.Contains(out, `rech_type_meta_kot = {"Кот", NULL, 0,`) ||
		!strings.Contains(out, `&rech_type_meta_zhivotnoe, rech_type_meta_kot_interfaces, 1, ""};`) {
		t.Fatalf("ожидался дескриптор Кот с base=&rech_type_meta_zhivotnoe и interfaces/interface_count=1, получено:\n%s", out)
	}
}

// TestGenerateFromIRTypeInfoBaseAndInterfaceReflectionMethods - "базовый_тип"/
// "наследует_ли"/"число_интерфейсов"/"получить_интерфейс"/"реализует_ли"
// (паритет с C# Type.BaseType/IsSubclassOf/GetInterfaces/
// IsAssignableFrom) компилируются в ожидаемые обращения к полям/хелперам
// дескриптора.
func TestGenerateFromIRTypeInfoBaseAndInterfaceReflectionMethods(t *testing.T) {
	recv := &ir.Identifier{Name: "т", Type: ir.TypeTypeInfo}
	arg := &ir.Identifier{Name: "т2", Type: ir.TypeTypeInfo}
	main := &ir.Function{Name: "главная", Body: []ir.Stmt{
		&ir.PrintStmt{Value: &ir.MethodCallExpr{Receiver: recv, Method: "базовый_тип", Type: ir.TypeTypeInfo, HasReturn: true}},
		&ir.PrintStmt{Value: &ir.MethodCallExpr{Receiver: recv, Method: "наследует_ли", Args: []ir.Expr{arg}, Type: ir.TypeBool, HasReturn: true}},
		&ir.PrintStmt{Value: &ir.MethodCallExpr{Receiver: recv, Method: "число_интерфейсов", Type: ir.TypeInt, HasReturn: true}},
		&ir.PrintStmt{Value: &ir.MethodCallExpr{Receiver: recv, Method: "получить_интерфейс", Args: []ir.Expr{&ir.IntLiteral{Value: 0}}, Type: ir.TypeTypeInfo, HasReturn: true}},
		&ir.PrintStmt{Value: &ir.MethodCallExpr{Receiver: recv, Method: "реализует_ли", Args: []ir.Expr{arg}, Type: ir.TypeBool, HasReturn: true}},
	}}
	out, err := GenerateFromIR(&ir.Program{Functions: []*ir.Function{main}, UsesReflection: true})
	if err != nil {
		t.Fatalf("неожиданная ошибка: %v", err)
	}
	for _, want := range []string{
		"rech_type_base_or_pusto(v_t)",
		"rech_type_is_subclass((v_t)->base, v_t2)",
		"(v_t)->interface_count",
		"rech_iface_at(v_t, 0LL)",
		"rech_type_implements(v_t, v_t2)",
	} {
		if !strings.Contains(out, want) {
			t.Fatalf("ожидалось %q, получено:\n%s", want, out)
		}
	}
}

// TestGenerateFromIREntityInterfaceCircularReflectOrderingRegression -
// НАЙДЕННЫЙ И ИСПРАВЛЕННЫЙ баг: сущность, реализующая
// интерфейс, чей InterfaceImplementors указывает на ТУ ЖЕ сущность
// (обычный случай - "Кот реализует Звучащий", "Звучащий".implementors ==
// ["Кот"]) - раньше тентативное объявление "static const rech_type_meta_t
// rech_type_meta_kot;" писалось ПОСЛЕ ensure() полей/методов/base/
// interfaces сущности, а не сразу после появления записи в g.generated -
// ensure(Кот) помечал себя сгенерированным, рекурсивно вызывал
// ensure(Звучащий), тот - ensure(Кот) СНОВА (implementors), видел
// g.generated[Кот]==true и возвращался БЕЗ единой написанной строки -
// ensureInterface(Звучащий) продолжал и писал "rech_iface_impls_zvuchaschij
// = {&rech_type_meta_kot}", ссылаясь на СИМВОЛ, ЕЩЁ НЕ ОБЪЯВЛЕННЫЙ НИ
// ТЕНТАТИВНО, НИ ПОЛНОСТЬЮ (настоящий cc падал "undeclared identifier").
// Тест проверяет ПОРЯДОК текста - тентативное объявление обязано появиться
// РАНЬШЕ первой ссылки на него.
func TestGenerateFromIREntityInterfaceCircularReflectOrderingRegression(t *testing.T) {
	iface := &ir.Interface{Name: "Звучащий", Methods: []ir.InterfaceMethod{
		{Name: "издать_звук", ReturnType: ir.TypeString, HasReturn: true},
	}}
	entity := &ir.Entity{Name: "Кот", Interfaces: []string{"Звучащий"}, Methods: []*ir.Method{
		{Name: "издать_звук", ReturnType: ir.TypeString, HasReturn: true,
			Body: []ir.Stmt{&ir.ReturnStmt{Value: &ir.StringLiteral{Value: "мяу"}}}},
	}}
	main := &ir.Function{Name: "главная", Body: []ir.Stmt{
		&ir.VarDecl{Name: "т", Value: &ir.TypeOfExpr{Type: ir.EntityType("Кот")}},
	}}
	out, err := GenerateFromIR(&ir.Program{
		Entities: []*ir.Entity{entity}, Interfaces: []*ir.Interface{iface}, Functions: []*ir.Function{main},
		UsesReflection:        true,
		ReflectTargets:        []ir.Type{ir.EntityType("Кот")},
		InterfaceImplementors: map[string][]string{"Звучащий": {"Кот"}},
		EntityInterfaces:      map[string][]string{"Кот": {"Звучащий"}},
	})
	if err != nil {
		t.Fatalf("неожиданная ошибка: %v", err)
	}
	declPos := strings.Index(out, "static const rech_type_meta_t rech_type_meta_kot;")
	usePos := strings.Index(out, "rech_iface_impls_zvuchaschij[] = {&rech_type_meta_kot};")
	if declPos == -1 || usePos == -1 || declPos >= usePos {
		t.Fatalf("ожидалось опережающее объявление rech_type_meta_kot РАНЬШЕ ссылки на него в rech_iface_impls_zvuchaschij, получено:\n%s", out)
	}
}

// TestGenerateFromIRPrimitiveTagIsEmptyStringNotNull - НАЙДЕННЫЙ И
// ИСПРАВЛЕННЫЙ баг: поле "tag" дескриптора примитива/
// generic-коллекции/интерфейса изначально эмитилось как буквальный C `NULL`
// (в отличие от поля/метода/сущности, у которых "нет тега" ВСЕГДА "" -
// пустая СТРОКА, никогда NULL-указатель) - "тег_текст()"/"->tag" на такой
// дескриптор читает `->tag` НАПРЯМУЮ, без проверки на NULL (в отличие от
// rech_tag_lookup, который её делает) - настоящий cc падал сегфолтом при
// попытке показать/использовать результат как Строка. Тест проверяет
// текстово - дескриптор примитива не должен содержать буквальный "NULL"
// последним полем.
func TestGenerateFromIRPrimitiveTagIsEmptyStringNotNull(t *testing.T) {
	main := &ir.Function{Name: "главная", Body: []ir.Stmt{
		&ir.VarDecl{Name: "т", Value: &ir.TypeOfExpr{Type: ir.TypeInt}},
	}}
	out, err := GenerateFromIR(&ir.Program{Functions: []*ir.Function{main}, UsesReflection: true})
	if err != nil {
		t.Fatalf("неожиданная ошибка: %v", err)
	}
	if strings.Contains(out, `rech_type_meta_celoe = {"Целое", NULL, 0, NULL, 0, NULL, 0, NULL, NULL, 0, NULL};`) {
		t.Fatalf("поле tag дескриптора примитива - буквальный NULL, ожидалась пустая строка \"\", получено:\n%s", out)
	}
	if !strings.Contains(out, `rech_type_meta_celoe = {"Целое", NULL, 0, NULL, 0, NULL, 0, NULL, NULL, 0, ""};`) {
		t.Fatalf("ожидался дескриптор Целое с tag = \"\", получено:\n%s", out)
	}
}

// TestGenerateFromIREntityMethodAndFieldTagReflection -
// паритет с C#-атрибутами на классе/методе/поле (тег сущности - новый
// синтаксис "сущность Имя `ключ:значение`"; тег метода - синтаксис уже
// существовал, но не был виден рефлексии): "тег(ключ)"/"тег_текст()" на
// самой сущности, "тег_метода(i, ключ)"/"тег_метода_текст(i)" на методе,
// "тег_поля_текст(i)" - сырой текст тега поля (в дополнение к уже
// существующему "тег_поля(i, ключ)").
func TestGenerateFromIREntityMethodAndFieldTagReflection(t *testing.T) {
	recv := &ir.Identifier{Name: "т", Type: ir.TypeTypeInfo}
	entity := &ir.Entity{
		Name: "Пользователь",
		Tag:  "таблица:users",
		Fields: []ir.Field{
			{Name: "имя", Type: ir.TypeString, Tag: "json:name"},
		},
		Methods: []*ir.Method{
			{Name: "сохранить", ReturnType: ir.TypeString, HasReturn: true, Tag: "выполняет:запись",
				Body: []ir.Stmt{&ir.ReturnStmt{Value: &ir.StringLiteral{Value: "ok"}}}},
		},
	}
	main := &ir.Function{Name: "главная", Body: []ir.Stmt{
		&ir.VarDecl{Name: "т", Value: &ir.TypeOfExpr{Type: ir.EntityType("Пользователь")}},
		&ir.PrintStmt{Value: &ir.MethodCallExpr{Receiver: recv, Method: "тег", Args: []ir.Expr{&ir.StringLiteral{Value: "таблица"}}, Type: ir.TypeString, HasReturn: true}},
		&ir.PrintStmt{Value: &ir.MethodCallExpr{Receiver: recv, Method: "тег_текст", Type: ir.TypeString, HasReturn: true}},
		&ir.PrintStmt{Value: &ir.MethodCallExpr{Receiver: recv, Method: "тег_поля_текст", Args: []ir.Expr{&ir.IntLiteral{Value: 0}}, Type: ir.TypeString, HasReturn: true}},
		&ir.PrintStmt{Value: &ir.MethodCallExpr{Receiver: recv, Method: "тег_метода", Args: []ir.Expr{&ir.IntLiteral{Value: 0}, &ir.StringLiteral{Value: "выполняет"}}, Type: ir.TypeString, HasReturn: true}},
		&ir.PrintStmt{Value: &ir.MethodCallExpr{Receiver: recv, Method: "тег_метода_текст", Args: []ir.Expr{&ir.IntLiteral{Value: 0}}, Type: ir.TypeString, HasReturn: true}},
	}}
	out, err := GenerateFromIR(&ir.Program{
		Entities: []*ir.Entity{entity}, Functions: []*ir.Function{main},
		UsesReflection: true, ReflectTargets: []ir.Type{ir.EntityType("Пользователь")},
	})
	if err != nil {
		t.Fatalf("неожиданная ошибка: %v", err)
	}
	for _, want := range []string{
		`rech_tag_lookup((v_t)->tag, "таблица")`,
		"(v_t)->tag",
		"rech_field_at(v_t, 0LL)->tag",
		`rech_tag_lookup(rech_method_at(v_t, 0LL)->tag, "выполняет")`,
		"rech_method_at(v_t, 0LL)->tag",
		`, "таблица:users"};`,
		`"имя", &rech_type_meta_stroka, rech_reflect_get_polzovatel_imya, rech_reflect_set_polzovatel_imya, "json:name"`,
		`{"сохранить", 0, NULL, &rech_type_meta_stroka, rech_reflect_invoke_polzovatel_sohranit, "выполняет:запись"}`,
	} {
		if !strings.Contains(out, want) {
			t.Fatalf("ожидалось %q, получено:\n%s", want, out)
		}
	}
}

// TestGenerateFromIRFieldByTagReflection - "индекс_поля_по_тегу"/
// "получить_поле_по_тегу" (обратное направление к
// тег_поля/получить_поле - дан ключ/значение тега, нужно поле, не
// наоборот) - обе возвращают nullable ("?Целое"/"?Значение"), их
// C-хелперы (reflect.go, reflectFieldIndexByTagHelperCode/
// reflectGetFieldByTagHelperCode) эмитируются ПОСЛЕ обёрток nullable
// (nullablesCode, ir_generator.go), в отличие от остальных методов "Тип"/
// "Значение" выше - тест проверяет, что GenerateFromIR не падает на этом
// порядке эмиссии и оба хелпера действительно вызываются.
func TestGenerateFromIRFieldByTagReflection(t *testing.T) {
	recv := &ir.Identifier{Name: "т", Type: ir.TypeTypeInfo}
	boxed := &ir.Identifier{Name: "бокс", Type: ir.TypeAny}
	entity := &ir.Entity{
		Name: "Пользователь",
		Fields: []ir.Field{
			{Name: "имя", Type: ir.TypeString, Tag: "sql:user_name"},
		},
	}
	main := &ir.Function{Name: "главная", Body: []ir.Stmt{
		&ir.VarDecl{Name: "т", Value: &ir.TypeOfExpr{Type: ir.EntityType("Пользователь")}},
		&ir.VarDecl{Name: "бокс", Value: &ir.NullLiteral{Type: ir.TypeAny}},
		&ir.PrintStmt{Value: &ir.MethodCallExpr{Receiver: recv, Method: "индекс_поля_по_тегу", Args: []ir.Expr{&ir.StringLiteral{Value: "sql"}, &ir.StringLiteral{Value: "user_name"}}, Type: ir.NullableType(ir.TypeInt), HasReturn: true}},
		&ir.PrintStmt{Value: &ir.MethodCallExpr{Receiver: boxed, Method: "получить_поле_по_тегу", Args: []ir.Expr{&ir.StringLiteral{Value: "sql"}, &ir.StringLiteral{Value: "user_name"}}, Type: ir.NullableType(ir.TypeAny), HasReturn: true}},
	}}
	out, err := GenerateFromIR(&ir.Program{
		Entities: []*ir.Entity{entity}, Functions: []*ir.Function{main},
		UsesReflection: true, ReflectTargets: []ir.Type{ir.EntityType("Пользователь")},
		Nullables: []ir.NullableInstantiation{
			{Name: "?Целое", Inner: ir.TypeInt},
			{Name: "?Значение", Inner: ir.TypeAny},
		},
	})
	if err != nil {
		t.Fatalf("неожиданная ошибка: %v", err)
	}
	for _, want := range []string{
		`rech_field_index_by_tag(v_t, "sql", "user_name")`,
		`rech_reflect_get_field_by_tag(v_boks, "sql", "user_name")`,
		"static rech_T_n_celoe rech_field_index_by_tag(",
		"static rech_T_n_znachenie rech_reflect_get_field_by_tag(",
	} {
		if !strings.Contains(out, want) {
			t.Fatalf("ожидалось %q, получено:\n%s", want, out)
		}
	}
}
