package cbackend

import (
	"strings"
	"testing"

	"github.com/IlyaFeoktistov/Rech/internal/ir"
)

// TestGenerateFromIRCallableStructTypedef - сигнатура Program.Callables
// получает C-структуру {указатель на функцию, указатель на окружение} с
// точным соответствием сигнатуры (тип результата, типы параметров, "void
// *" первым - под окружение) - typedef эмитится безусловно для всех
// сигнатур программы, не лениво.
func TestGenerateFromIRCallableStructTypedef(t *testing.T) {
	sig := ir.CallableSig{Name: "Вызываемый<Целое>Целое", Params: []ir.Type{ir.TypeInt}, Result: &ir.Type{Kind: ir.KindInt}}
	main := &ir.Function{Name: "главная", Body: []ir.Stmt{&ir.ReturnStmt{}}}
	out, err := GenerateFromIR(&ir.Program{Functions: []*ir.Function{main}, Callables: []ir.CallableSig{sig}})
	if err != nil {
		t.Fatalf("неожиданная ошибка: %v", err)
	}
	if !strings.Contains(out, "long long (*fn)(void *, long long);") {
		t.Fatalf("ожидался fn-указатель структуры Callable, получено:\n%s", out)
	}
}

// TestGenerateFromIRFuncRefTrampoline - "&имя" эмитит trampoline-функцию
// (окружение первым параметром, отбрасывается, дальше зовёт настоящую
// функцию как есть) и строит значение через compound literal с этим
// trampoline и env = NULL (ссылка на функцию ничего не захватывает).
func TestGenerateFromIRFuncRefTrampoline(t *testing.T) {
	callableT := ir.CallableType([]ir.Type{ir.TypeInt}, &ir.Type{Kind: ir.KindInt})
	удвоить := &ir.Function{Name: "удвоить", Params: []ir.Param{{Name: "x", Type: ir.TypeInt}}, ReturnType: ir.TypeInt, HasReturn: true,
		Body: []ir.Stmt{&ir.ReturnStmt{Value: &ir.BinaryExpr{Op: "*", Left: &ir.Identifier{Name: "x", Type: ir.TypeInt}, Right: &ir.IntLiteral{Value: 2}, Type: ir.TypeInt}}}}
	main := &ir.Function{Name: "главная", Body: []ir.Stmt{
		&ir.VarDecl{Name: "ф", Value: &ir.FuncRefExpr{FuncName: "удвоить", Type: callableT}},
	}}
	out, err := GenerateFromIR(&ir.Program{Functions: []*ir.Function{удвоить, main}, Callables: []ir.CallableSig{{Name: callableT.Entity, Params: []ir.Type{ir.TypeInt}, Result: &ir.Type{Kind: ir.KindInt}}}})
	if err != nil {
		t.Fatalf("неожиданная ошибка: %v", err)
	}
	if !strings.Contains(out, "rech_fref_udvoit") {
		t.Fatalf("ожидался trampoline rech_fref_udvoit, получено:\n%s", out)
	}
	if !strings.Contains(out, ".env = NULL") {
		t.Fatalf("ожидалось env = NULL у ссылки на функцию, получено:\n%s", out)
	}
}

// TestGenerateFromIRLambdaCaptureEnv - лямбда с захватом строит окружение
// через memcpy(GC_MALLOC(...), &(ENV){...}, ...) - env НЕ NULL, значение
// захваченной переменной читается прямо из текущей C-переменной по имени
// (та же самая, что видна в этой точке кода).
func TestGenerateFromIRLambdaCaptureEnv(t *testing.T) {
	callableT := ir.CallableType([]ir.Type{ir.TypeInt}, &ir.Type{Kind: ir.KindInt})
	lambda := &ir.LambdaExpr{
		Name: "__лямбда_1", Params: []ir.Param{{Name: "x", Type: ir.TypeInt}}, ReturnType: ir.TypeInt, HasReturn: true,
		Captures: []ir.CaptureVar{{Name: "база", Type: ir.TypeInt}},
		Body: []ir.Stmt{&ir.ReturnStmt{Value: &ir.BinaryExpr{
			Op: "+", Left: &ir.Identifier{Name: "x", Type: ir.TypeInt}, Right: &ir.Identifier{Name: "база", Type: ir.TypeInt}, Type: ir.TypeInt,
		}}},
		Type: callableT,
	}
	main := &ir.Function{Name: "главная", Body: []ir.Stmt{
		&ir.VarDecl{Name: "база", Value: &ir.IntLiteral{Value: 10}},
		&ir.VarDecl{Name: "ф", Value: lambda},
	}}
	out, err := GenerateFromIR(&ir.Program{Functions: []*ir.Function{main}, Callables: []ir.CallableSig{{Name: callableT.Entity, Params: []ir.Type{ir.TypeInt}, Result: &ir.Type{Kind: ir.KindInt}}}})
	if err != nil {
		t.Fatalf("неожиданная ошибка: %v", err)
	}
	if !strings.Contains(out, "memcpy(GC_MALLOC(sizeof(") {
		t.Fatalf("ожидалось выделение окружения через memcpy/GC_MALLOC, получено:\n%s", out)
	}
	if !strings.Contains(out, "v_baza") {
		t.Fatalf("ожидалось чтение текущей C-переменной захваченного имени, получено:\n%s", out)
	}
	if !strings.Contains(out, "rech_captured->f_baza") {
		t.Fatalf("ожидалось чтение/запись захвата через rech_captured->..., получено:\n%s", out)
	}
}

// TestGenerateFromIRLambdaCaptureByRefEnv - CaptureVar.ByRef true (явный
// список захвата "&имя", см. package doc ir.CaptureVar) - поле окружения
// само типа "Указатель<T>" (не T), значение строится через "&<текущая
// C-переменная>" (адрес, не значение) - тот же C-код, что и у обычного
// "&а" (AddressOfExpr, см. TestGenerateFromIRPointerAddressAndDeref,
// pointer_test.go), не отдельный механизм.
func TestGenerateFromIRLambdaCaptureByRefEnv(t *testing.T) {
	ptrType := ir.Type{Kind: ir.KindPointer, Entity: "Указатель<Целое>"}
	callableT := ir.CallableType(nil, nil)
	lambda := &ir.LambdaExpr{
		Name:     "__лямбда_1",
		Captures: []ir.CaptureVar{{Name: "у", Type: ptrType, ByRef: true}},
		Body: []ir.Stmt{&ir.DerefAssignStmt{
			Pointer: &ir.Identifier{Name: "у", Type: ptrType},
			Value:   &ir.IntLiteral{Value: 1},
		}},
		Type: callableT,
	}
	main := &ir.Function{Name: "главная", Body: []ir.Stmt{
		&ir.VarDecl{Name: "у", Value: &ir.IntLiteral{Value: 10}},
		&ir.VarDecl{Name: "ф", Value: lambda},
	}}
	out, err := GenerateFromIR(&ir.Program{
		Functions: []*ir.Function{main},
		Callables: []ir.CallableSig{{Name: callableT.Entity}},
		Pointers:  []ir.PointerInstantiation{{Name: ptrType.Entity, Inner: ir.TypeInt}},
	})
	if err != nil {
		t.Fatalf("неожиданная ошибка: %v", err)
	}
	if !strings.Contains(out, "long long * f_u") {
		t.Fatalf("ожидалось поле окружения типа long long *, получено:\n%s", out)
	}
	if !strings.Contains(out, "= &v_u") {
		t.Fatalf("ожидалось построение окружения через адрес (\"&v_u\"), получено:\n%s", out)
	}
	if !strings.Contains(out, "*rech_captured->f_u = 1LL;") {
		t.Fatalf("ожидалась запись через разыменование поля окружения, получено:\n%s", out)
	}
}

// TestGenerateFromIRCallableParamEntityCollectionOrdering - сигнатура
// "Вызываемый", принимающая generic-коллекцию с сущностью-элементом
// (Список<T> у examples/бд/чанкование.rech, "чанковать_по_полю<T>(...,
// обработчик: Вызываемый(Список<T>))") - typedef этой коллекции обязан
// оказаться в тексте C РАНЬШЕ typedef самой сигнатуры (её "fn" - указатель
// на функцию, принимающую "rech_T_... *" этой коллекции - C требует полный
// тип под указателем-членом структуры). До фикса обе группы эмитились в
// фиксированном порядке "сначала все Callable, потом коллекции с сущностью-
// элементом" - типичная сигнатура вида "Вызываемый(Список<Сущность>)"
// ссылалась на ещё не объявленный typedef.
func TestGenerateFromIRCallableParamEntityCollectionOrdering(t *testing.T) {
	entity := &ir.Entity{Name: "Пользователь", IsStruct: true, Fields: []ir.Field{{Name: "id", Type: ir.TypeInt}}}
	listType := ir.Type{Kind: ir.KindEntity, Entity: "Список<Пользователь>"}
	callableT := ir.CallableType([]ir.Type{listType}, nil)
	main := &ir.Function{Name: "главная", Body: []ir.Stmt{&ir.ReturnStmt{}}}
	prog := &ir.Program{
		Entities:  []*ir.Entity{entity},
		Generics:  []ir.GenericInstantiation{{Name: "Список<Пользователь>", Template: "Список", ElemType: ir.Type{Kind: ir.KindEntity, Entity: "Пользователь"}}},
		Callables: []ir.CallableSig{{Name: callableT.Entity, Params: []ir.Type{listType}, Result: nil}},
		Functions: []*ir.Function{main},
	}
	out, err := GenerateFromIR(prog)
	if err != nil {
		t.Fatalf("неожиданная ошибка: %v", err)
	}
	collectionIdx := strings.Index(out, "struct rech_T_spisok_of_polzovatel {")
	callableIdx := strings.Index(out, "rech_T_spisok_of_polzovatel *);")
	if collectionIdx == -1 || callableIdx == -1 {
		t.Fatalf("ожидались оба typedef в выводе, получено:\n%s", out)
	}
	if collectionIdx > callableIdx {
		t.Fatalf("typedef коллекции (позиция %d) обязан идти РАНЬШЕ typedef сигнатуры Callable (позиция %d)", collectionIdx, callableIdx)
	}
}

// TestGenerateFromIRCallValueExpr - вызов значения типа Callable ("ф(...)")
// зовёт через .fn(.env, ...аргументы...), не по статически известному
// имени функции.
func TestGenerateFromIRCallValueExpr(t *testing.T) {
	callableT := ir.CallableType([]ir.Type{ir.TypeInt}, &ir.Type{Kind: ir.KindInt})
	main := &ir.Function{Name: "главная", HasReturn: false, Body: []ir.Stmt{
		&ir.VarDecl{Name: "ф", Value: &ir.FuncRefExpr{FuncName: "удвоить", Type: callableT}},
		&ir.PrintStmt{Value: &ir.CallValueExpr{
			Value: &ir.Identifier{Name: "ф", Type: callableT}, Args: []ir.Expr{&ir.IntLiteral{Value: 21}},
			Type: ir.TypeInt, HasReturn: true,
		}},
	}}
	удвоить := &ir.Function{Name: "удвоить", Params: []ir.Param{{Name: "x", Type: ir.TypeInt}}, ReturnType: ir.TypeInt, HasReturn: true,
		Body: []ir.Stmt{&ir.ReturnStmt{Value: &ir.Identifier{Name: "x", Type: ir.TypeInt}}}}
	out, err := GenerateFromIR(&ir.Program{Functions: []*ir.Function{удвоить, main}, Callables: []ir.CallableSig{{Name: callableT.Entity, Params: []ir.Type{ir.TypeInt}, Result: &ir.Type{Kind: ir.KindInt}}}})
	if err != nil {
		t.Fatalf("неожиданная ошибка: %v", err)
	}
	if !strings.Contains(out, ").fn((v_f).env, ") && !strings.Contains(out, ").fn((v_f).env,") {
		t.Fatalf("ожидался вызов через .fn(.env, ...), получено:\n%s", out)
	}
}
