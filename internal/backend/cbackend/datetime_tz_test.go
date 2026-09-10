package cbackend

import (
	"strings"
	"testing"

	"github.com/IlyaFeoktistov/Rech/internal/ir"
)

func TestGenerateFromIRParseDateBuiltinCall(t *testing.T) {
	prog := &ir.Program{
		Entities: []*ir.Entity{{Name: "ДатаВремя", IsStruct: true, Fields: dateTimeFieldsForTest()}},
		Nullables: []ir.NullableInstantiation{
			{Name: "?ДатаВремя", Inner: ir.EntityType("ДатаВремя")},
		},
		Functions: []*ir.Function{{Name: "главная", Body: []ir.Stmt{
			&ir.VarDecl{Name: "д", Value: &ir.CallExpr{
				Callee: "разобрать_дату", Args: []ir.Expr{&ir.StringLiteral{Value: "2023-11-14 22:13:20"}},
				Type: ir.NullableType(ir.EntityType("ДатаВремя")), HasReturn: true,
			}},
		}}},
	}
	out, err := GenerateFromIR(prog)
	if err != nil {
		t.Fatalf("неожиданная ошибка: %v", err)
	}
	for _, want := range []string{"rech_parse_date(", "rech_parse_date_digits("} {
		if !strings.Contains(out, want) {
			t.Fatalf("ожидалось вхождение %q, получено:\n%s", want, out)
		}
	}
	// НЕ должен тянуть за собой TZif-движок - разбор строки от него не
	// зависит (usesDateParse != usesTZ, package doc tzHelpers).
	if strings.Contains(out, "rech_tz_load") {
		t.Fatalf("разбор строки не должен эмитировать TZif-читатель, получено:\n%s", out)
	}
}

func TestGenerateFromIRTimeAsDateInZoneBuiltinCall(t *testing.T) {
	prog := &ir.Program{
		Entities: []*ir.Entity{{Name: "ДатаВремя", IsStruct: true, Fields: dateTimeFieldsForTest()}},
		Nullables: []ir.NullableInstantiation{
			{Name: "?ДатаВремя", Inner: ir.EntityType("ДатаВремя")},
		},
		Functions: []*ir.Function{{Name: "главная", Body: []ir.Stmt{
			&ir.VarDecl{Name: "д", Value: &ir.CallExpr{
				Callee: "время_как_дата_в_поясе",
				Args:   []ir.Expr{&ir.IntLiteral{Value: 0}, &ir.StringLiteral{Value: "Europe/Moscow"}},
				Type:   ir.NullableType(ir.EntityType("ДатаВремя")), HasReturn: true,
			}},
		}}},
	}
	out, err := GenerateFromIR(prog)
	if err != nil {
		t.Fatalf("неожиданная ошибка: %v", err)
	}
	for _, want := range []string{
		"rech_time_to_date_tz(",
		"rech_tz_load(",
		"rech_tz_posix_offset(",
		"rech_days_from_civil", // датарифметика из datetimeHelpers переиспользуется
	} {
		if !strings.Contains(out, want) {
			t.Fatalf("ожидалось вхождение %q, получено:\n%s", want, out)
		}
	}
}

func TestGenerateFromIRDayOfWeekAndHumanDate(t *testing.T) {
	prog := &ir.Program{
		Entities: []*ir.Entity{{Name: "ДатаВремя", IsStruct: true, Fields: dateTimeFieldsForTest()}},
		Functions: []*ir.Function{{Name: "главная", Body: []ir.Stmt{
			&ir.VarDecl{Name: "д", Value: &ir.CallExpr{Callee: "время_как_дата", Args: []ir.Expr{&ir.IntLiteral{Value: 0}}, Type: ir.EntityType("ДатаВремя"), HasReturn: true}},
			&ir.PrintStmt{Value: &ir.CallExpr{
				Callee: "день_недели", Args: []ir.Expr{&ir.Identifier{Name: "д", Type: ir.EntityType("ДатаВремя")}},
				Type: ir.TypeInt, HasReturn: true,
			}},
			&ir.PrintStmt{Value: &ir.CallExpr{
				Callee: "дату_в_читаемом_виде", Args: []ir.Expr{&ir.Identifier{Name: "д", Type: ir.EntityType("ДатаВремя")}},
				Type: ir.TypeString, HasReturn: true,
			}},
		}}},
	}
	out, err := GenerateFromIR(prog)
	if err != nil {
		t.Fatalf("неожиданная ошибка: %v", err)
	}
	for _, want := range []string{"rech_day_of_week(", "rech_date_to_human(", "rech_ru_plural("} {
		if !strings.Contains(out, want) {
			t.Fatalf("ожидалось вхождение %q, получено:\n%s", want, out)
		}
	}
}

// TestGenerateFromIRWithoutTZOmitsTZifReader - НАЙДЕННЫЙ структурный риск
// (см. package doc registerOptionalStringOnCall, internal/semantic/
// nullable.go) уже один раз стоил байта каждой программе для "?Строка" -
// здесь то же самое для TZif-читателя: программа, ни разу не звавшая
// время_как_дата_в_поясе/дата_в_поясе_как_время (и, значит, ни разу не
// вызвавшая "д.в_поясе(...)" - оно desugar'ится в первую из них), не
// должна тянуть за собой rech_tz_load/rech_tz_posix_offset вовсе - даже
// если сама "ДатаВремя" используется (время_как_дата без поясов).
func TestGenerateFromIRWithoutTZOmitsTZifReader(t *testing.T) {
	prog := &ir.Program{
		Entities: []*ir.Entity{{Name: "ДатаВремя", IsStruct: true, Fields: dateTimeFieldsForTest()}},
		Functions: []*ir.Function{{Name: "главная", Body: []ir.Stmt{
			&ir.VarDecl{Name: "д", Value: &ir.CallExpr{Callee: "время_как_дата", Args: []ir.Expr{&ir.IntLiteral{Value: 0}}, Type: ir.EntityType("ДатаВремя"), HasReturn: true}},
			&ir.PrintStmt{Value: &ir.CallExpr{
				Callee: "дату_в_строку", Args: []ir.Expr{&ir.Identifier{Name: "д", Type: ir.EntityType("ДатаВремя")}},
				Type: ir.TypeString, HasReturn: true,
			}},
		}}},
	}
	out, err := GenerateFromIR(prog)
	if err != nil {
		t.Fatalf("неожиданная ошибка: %v", err)
	}
	for _, notWant := range []string{"rech_tz_load", "rech_parse_date", "rech_day_of_week", "rech_date_to_human"} {
		if strings.Contains(out, notWant) {
			t.Fatalf("время_как_дата без часовых поясов не должно эмитировать %q, получено:\n%s", notWant, out)
		}
	}
}
