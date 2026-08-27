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
