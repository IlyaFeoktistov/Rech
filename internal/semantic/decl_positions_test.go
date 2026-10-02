package semantic

import (
	"errors"
	"strings"
	"testing"

	"github.com/IlyaFeoktistov/Rech/internal/diagnostics"
)

// TestDeclarationErrorsCarryLine - ошибки объявлений (тип поля, параметра,
// результата, повтор сигнатуры) несут строку своего объявления, а не остаются
// без позиции.
func TestDeclarationErrorsCarryLine(t *testing.T) {
	_, err := AnalyzeSource(t, "сущность Точка\n{\n    имеет:\n        НетТипаПоля x\n}\n\nфункция главная\n{\n}\n")
	if d := diagnostics.AsDiagnostic(err); d == nil || !strings.Contains(d.Message, "НетТипаПоля") || d.Pos.Line != 4 {
		t.Fatalf("ошибка типа поля должна быть на строке 4, получено %v", err)
	}

	_, err = AnalyzeSource(t, `функция ф(а: Целое,
          б: НетТипаПараметра)
{
}

функция г(): НетТипаРезультата
{
}

функция д()
{
}

функция д()
{
}

функция главная
{
}
`)
	var bag *diagnostics.Bag
	if !errors.As(err, &bag) {
		t.Fatalf("ожидался набор диагностик, получено %v", err)
	}
	want := map[string]int{"НетТипаПараметра": 2, "НетТипаРезультата": 6, "две перегрузки с одинаковым списком": 14}
	for _, d := range bag.Diagnostics() {
		for needle, line := range want {
			if strings.Contains(d.Message, needle) {
				if d.Pos.Line != line {
					t.Errorf("%q: строка %d, ожидалась %d", needle, d.Pos.Line, line)
				}
				delete(want, needle)
			}
		}
	}
	if len(want) != 0 {
		t.Fatalf("не нашлись ошибки %v среди %v", want, bag.Diagnostics())
	}
}
