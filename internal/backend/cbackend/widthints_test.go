package cbackend

import (
	"strings"
	"testing"

	"github.com/IlyaFeoktistov/Rech/internal/ir"
)

// TestGenerateFromIRFixedWidthArithmeticTruncatesAtExpressionLevel -
// Целое8 + Целое8 - результат явно приводится к int8_t НА УРОВНЕ
// ВЫРАЖЕНИЯ, а не только при присваивании - иначе обычные
// арифметические преобразования C оставили бы промежуточный результат в
// более широком типе.
func TestGenerateFromIRFixedWidthArithmeticTruncatesAtExpressionLevel(t *testing.T) {
	main := &ir.Function{Name: "главная", Body: []ir.Stmt{
		&ir.PrintStmt{Value: &ir.BinaryExpr{
			Op:    "+",
			Left:  &ir.CastExpr{Value: &ir.IntLiteral{Value: 100}, To: ir.TypeInt8},
			Right: &ir.CastExpr{Value: &ir.IntLiteral{Value: 27}, To: ir.TypeInt8},
			Type:  ir.TypeInt8,
		}},
	}}
	out, err := GenerateFromIR(&ir.Program{Functions: []*ir.Function{main}})
	if err != nil {
		t.Fatalf("неожиданная ошибка: %v", err)
	}
	if !strings.Contains(out, "((int8_t)(((int8_t)(100LL)) + ((int8_t)(27LL))))") {
		t.Fatalf("ожидался каст результата в int8_t, получено:\n%s", out)
	}
}

// TestGenerateFromIRUnsignedFixedWidthDivisionUsesUintHelper - Натуральное64
// / Натуральное64 - через rech_uint_div64 (uint64_t), не rech_int_div64
// (знаковый) - последний интерпретировал бы значение выше INT64_MAX как
// отрицательное.
func TestGenerateFromIRUnsignedFixedWidthDivisionUsesUintHelper(t *testing.T) {
	main := &ir.Function{Name: "главная", Body: []ir.Stmt{
		&ir.PrintStmt{Value: &ir.BinaryExpr{
			Op:    "/",
			Left:  &ir.CastExpr{Value: &ir.IntLiteral{Value: 10}, To: ir.TypeUint},
			Right: &ir.CastExpr{Value: &ir.IntLiteral{Value: 2}, To: ir.TypeUint},
			Type:  ir.TypeUint,
		}},
	}}
	out, err := GenerateFromIR(&ir.Program{Functions: []*ir.Function{main}})
	if err != nil {
		t.Fatalf("неожиданная ошибка: %v", err)
	}
	if !strings.Contains(out, "rech_uint_div64(") {
		t.Fatalf("ожидался вызов rech_uint_div64, получено:\n%s", out)
	}
	if !strings.Contains(out, "static uint64_t rech_uint_div64(") {
		t.Fatalf("ожидалось определение rech_uint_div64, получено:\n%s", out)
	}
}

// TestGenerateFromIRSignedFixedWidthDivisionUsesIntHelper - Целое32/Целое32
// - собственный узкий хелпер rech_int_div32 (настоящий int32_t на входе/
// выходе, не общий 64-битный, см. package doc ir_generator.go.intDivModUsage).
func TestGenerateFromIRSignedFixedWidthDivisionUsesIntHelper(t *testing.T) {
	main := &ir.Function{Name: "главная", Body: []ir.Stmt{
		&ir.PrintStmt{Value: &ir.BinaryExpr{
			Op:    "/",
			Left:  &ir.CastExpr{Value: &ir.IntLiteral{Value: 10}, To: ir.TypeInt32},
			Right: &ir.CastExpr{Value: &ir.IntLiteral{Value: 2}, To: ir.TypeInt32},
			Type:  ir.TypeInt32,
		}},
	}}
	out, err := GenerateFromIR(&ir.Program{Functions: []*ir.Function{main}})
	if err != nil {
		t.Fatalf("неожиданная ошибка: %v", err)
	}
	if !strings.Contains(out, "rech_int_div32(") {
		t.Fatalf("ожидался вызов rech_int_div32, получено:\n%s", out)
	}
	if !strings.Contains(out, "static int32_t rech_int_div32(") {
		t.Fatalf("ожидалось определение rech_int_div32, получено:\n%s", out)
	}
	if strings.Contains(out, "rech_uint_div") {
		t.Fatalf("знаковое деление не должно использовать rech_uint_div: %s", out)
	}
}

// TestGenerateFromIRFixedWidthDivModGuardsMinOverMinusOne - Целое8/Целое8 -
// хелпер rech_int_div8/rech_int_mod8 обязан явно проверить "a == INT8_MIN
// && b == -1": в C само это деление (даже НЕ на ноль) - undefined behavior
// (частное не умещается в int8_t), так что без проверки настоящий cc был бы
// вправе упасть по SIGFPE вместо two's-complement переполнения (см. package
// doc intDivModHelperCode, builtins_shared.go).
func TestGenerateFromIRFixedWidthDivModGuardsMinOverMinusOne(t *testing.T) {
	body := func(op string) []ir.Stmt {
		return []ir.Stmt{
			&ir.PrintStmt{Value: &ir.BinaryExpr{
				Op:    op,
				Left:  &ir.CastExpr{Value: &ir.IntLiteral{Value: -128}, To: ir.TypeInt8},
				Right: &ir.CastExpr{Value: &ir.IntLiteral{Value: -1}, To: ir.TypeInt8},
				Type:  ir.TypeInt8,
			}},
		}
	}
	div, err := GenerateFromIR(&ir.Program{Functions: []*ir.Function{{Name: "главная", Body: body("/")}}})
	if err != nil {
		t.Fatalf("неожиданная ошибка: %v", err)
	}
	if !strings.Contains(div, "if (a == INT8_MIN && b == -1) {\n        return INT8_MIN;\n    }") {
		t.Fatalf("ожидалась проверка INT8_MIN/-1 в rech_int_div8, получено:\n%s", div)
	}
	mod, err := GenerateFromIR(&ir.Program{Functions: []*ir.Function{{Name: "главная", Body: body("%")}}})
	if err != nil {
		t.Fatalf("неожиданная ошибка: %v", err)
	}
	if !strings.Contains(mod, "if (a == INT8_MIN && b == -1) {\n        return 0;\n    }") {
		t.Fatalf("ожидалась проверка INT8_MIN/-1 в rech_int_mod8, получено:\n%s", mod)
	}
}

// TestGenerateFromIRPrintUnsignedFixedWidthUsesUnsignedFormat - "вывести"
// Натуральное* - printf с %llu и (unsigned long long), не %lld/(long
// long) - иначе Натуральное64 выше INT64_MAX отобразилось бы отрицательным.
func TestGenerateFromIRPrintUnsignedFixedWidthUsesUnsignedFormat(t *testing.T) {
	main := &ir.Function{Name: "главная", Body: []ir.Stmt{
		&ir.PrintStmt{Value: &ir.CastExpr{Value: &ir.IntLiteral{Value: 200}, To: ir.TypeUint}},
	}}
	out, err := GenerateFromIR(&ir.Program{Functions: []*ir.Function{main}})
	if err != nil {
		t.Fatalf("неожиданная ошибка: %v", err)
	}
	if !strings.Contains(out, `printf("%llu", (unsigned long long)(`) {
		t.Fatalf("ожидался printf с %%llu, получено:\n%s", out)
	}
}

// TestGenerateFromIRPrintSignedFixedWidthUsesSignedFormat - "вывести"
// Целое8/16/32/64 - тот же %lld/(long long), что и у обычного Целое.
func TestGenerateFromIRPrintSignedFixedWidthUsesSignedFormat(t *testing.T) {
	main := &ir.Function{Name: "главная", Body: []ir.Stmt{
		&ir.PrintStmt{Value: &ir.CastExpr{Value: &ir.IntLiteral{Value: -5}, To: ir.TypeInt16}},
	}}
	out, err := GenerateFromIR(&ir.Program{Functions: []*ir.Function{main}})
	if err != nil {
		t.Fatalf("неожиданная ошибка: %v", err)
	}
	if !strings.Contains(out, `printf("%lld", (long long)(`) {
		t.Fatalf("ожидался printf с %%lld, получено:\n%s", out)
	}
}

// TestGenerateFromIRStrToUnsignedCastUsesUintHelper - (Натуральное)
// значение_строки - через rech_cast_str_to_uint (strtoull), не
// rech_cast_str_to_int (strtoll, знаковое) - строка "18446744073709551615"
// не поместилась бы в long long.
func TestGenerateFromIRStrToUnsignedCastUsesUintHelper(t *testing.T) {
	main := &ir.Function{Name: "главная", Body: []ir.Stmt{
		&ir.PrintStmt{Value: &ir.CastExpr{Value: &ir.StringLiteral{Value: "18446744073709551615"}, To: ir.TypeUint}},
	}}
	out, err := GenerateFromIR(&ir.Program{Functions: []*ir.Function{main}})
	if err != nil {
		t.Fatalf("неожиданная ошибка: %v", err)
	}
	if !strings.Contains(out, `rech_cast_str_to_uint("18446744073709551615", "Натуральное")`) {
		t.Fatalf("ожидался вызов rech_cast_str_to_uint, получено:\n%s", out)
	}
	if !strings.Contains(out, "static unsigned long long rech_cast_str_to_uint(") {
		t.Fatalf("ожидалось определение rech_cast_str_to_uint, получено:\n%s", out)
	}
}

// TestGenerateFromIRUnsignedToStrCastUsesUintHelper - (Строка)
// значение_Натуральное64 - через rech_cast_uint_to_str (%llu), не
// rech_cast_int_to_str (%lld) - последний отобразил бы значение выше
// INT64_MAX отрицательным.
func TestGenerateFromIRUnsignedToStrCastUsesUintHelper(t *testing.T) {
	main := &ir.Function{Name: "главная", Body: []ir.Stmt{
		&ir.PrintStmt{Value: &ir.CastExpr{Value: &ir.CastExpr{Value: &ir.IntLiteral{Value: 200}, To: ir.TypeUint}, To: ir.TypeString}},
	}}
	out, err := GenerateFromIR(&ir.Program{Functions: []*ir.Function{main}})
	if err != nil {
		t.Fatalf("неожиданная ошибка: %v", err)
	}
	if !strings.Contains(out, "rech_cast_uint_to_str((unsigned long long)") {
		t.Fatalf("ожидался вызов rech_cast_uint_to_str, получено:\n%s", out)
	}
	if !strings.Contains(out, "static char *rech_cast_uint_to_str(") {
		t.Fatalf("ожидалось определение rech_cast_uint_to_str, получено:\n%s", out)
	}
}
