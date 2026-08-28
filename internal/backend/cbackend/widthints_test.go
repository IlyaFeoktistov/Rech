package cbackend

import (
	"strings"
	"testing"

	"github.com/IlyaFeoktistov/Rech/internal/ir"
)

// TestGenerateFromIRFixedWidthArithmeticTruncatesAtExpressionLevel -
// Целое8 + Целое8 - результат явно приводится к int8_t НА УРОВНЕ
// ВЫРАЖЕНИЯ (шаг 12), а не только при присваивании - иначе обычные
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
// / Натуральное64 - через rech_uint_div (uint64_t), не rech_int_div
// (long long) - последний интерпретировал бы значение выше INT64_MAX как
// отрицательное.
func TestGenerateFromIRUnsignedFixedWidthDivisionUsesUintHelper(t *testing.T) {
	main := &ir.Function{Name: "главная", Body: []ir.Stmt{
		&ir.PrintStmt{Value: &ir.BinaryExpr{
			Op:    "/",
			Left:  &ir.CastExpr{Value: &ir.IntLiteral{Value: 10}, To: ir.TypeUint64},
			Right: &ir.CastExpr{Value: &ir.IntLiteral{Value: 2}, To: ir.TypeUint64},
			Type:  ir.TypeUint64,
		}},
	}}
	out, err := GenerateFromIR(&ir.Program{Functions: []*ir.Function{main}})
	if err != nil {
		t.Fatalf("неожиданная ошибка: %v", err)
	}
	if !strings.Contains(out, "rech_uint_div((uint64_t)") {
		t.Fatalf("ожидался вызов rech_uint_div, получено:\n%s", out)
	}
	if !strings.Contains(out, "static uint64_t rech_uint_div(") {
		t.Fatalf("ожидалось определение rech_uint_div, получено:\n%s", out)
	}
}

// TestGenerateFromIRSignedFixedWidthDivisionUsesIntHelper - Целое32/Целое32
// - переиспользует существующий rech_int_div (как и обычное Целое), не
// заводит отдельный хелпер под каждую знаковую ширину.
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
	if !strings.Contains(out, "rech_int_div((long long)") {
		t.Fatalf("ожидался вызов rech_int_div, получено:\n%s", out)
	}
	if strings.Contains(out, "rech_uint_div") {
		t.Fatalf("знаковое деление не должно использовать rech_uint_div: %s", out)
	}
}

// TestGenerateFromIRPrintUnsignedFixedWidthUsesUnsignedFormat - "вывести"
// Натуральное* - printf с %llu и (unsigned long long), не %lld/(long
// long) - иначе Натуральное64 выше INT64_MAX отобразилось бы отрицательным.
func TestGenerateFromIRPrintUnsignedFixedWidthUsesUnsignedFormat(t *testing.T) {
	main := &ir.Function{Name: "главная", Body: []ir.Stmt{
		&ir.PrintStmt{Value: &ir.CastExpr{Value: &ir.IntLiteral{Value: 200}, To: ir.TypeUint64}},
	}}
	out, err := GenerateFromIR(&ir.Program{Functions: []*ir.Function{main}})
	if err != nil {
		t.Fatalf("неожиданная ошибка: %v", err)
	}
	if !strings.Contains(out, `printf("%llu\n", (unsigned long long)(`) {
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
	if !strings.Contains(out, `printf("%lld\n", (long long)(`) {
		t.Fatalf("ожидался printf с %%lld, получено:\n%s", out)
	}
}

// TestGenerateFromIRStrToUnsignedCastUsesUintHelper - (Натуральное64)
// значение_строки - через rech_cast_str_to_uint (strtoull), не
// rech_cast_str_to_int (strtoll, знаковое) - строка "18446744073709551615"
// не поместилась бы в long long.
func TestGenerateFromIRStrToUnsignedCastUsesUintHelper(t *testing.T) {
	main := &ir.Function{Name: "главная", Body: []ir.Stmt{
		&ir.PrintStmt{Value: &ir.CastExpr{Value: &ir.StringLiteral{Value: "18446744073709551615"}, To: ir.TypeUint64}},
	}}
	out, err := GenerateFromIR(&ir.Program{Functions: []*ir.Function{main}})
	if err != nil {
		t.Fatalf("неожиданная ошибка: %v", err)
	}
	if !strings.Contains(out, `rech_cast_str_to_uint("18446744073709551615", "Натуральное64")`) {
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
		&ir.PrintStmt{Value: &ir.CastExpr{Value: &ir.CastExpr{Value: &ir.IntLiteral{Value: 200}, To: ir.TypeUint64}, To: ir.TypeString}},
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
