package semantic

import "testing"

// TestAnalyzeValuePrintEqAndNullableBox - "${з}", "==" у "Значение" и
// боксирование "?Тип".
func TestAnalyzeValuePrintEqAndNullableBox(t *testing.T) {
	if _, err := AnalyzeSource(t, "функция главная\n{\n    ?Целое а = Ничто\n    Значение з = (Значение) а\n    вывести `${з} ${з == (Значение) 5}`\n}\n"); err != nil {
		t.Fatalf("неожиданная ошибка: %v", err)
	}
}
