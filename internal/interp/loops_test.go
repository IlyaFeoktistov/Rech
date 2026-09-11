package interp

import (
	"testing"

	"github.com/IlyaFeoktistov/Rech/internal/lexer"
	"github.com/IlyaFeoktistov/Rech/internal/parser"
	"github.com/IlyaFeoktistov/Rech/internal/semantic"
)

// TestRunBreakStopsWhileLoop - "прервать" внутри "пока" немедленно
// прекращает цикл, не выполняя оставшиеся итерации.
func TestRunBreakStopsWhileLoop(t *testing.T) {
	got := runSrc(t, `
функция главная
{
    переменная i = 0
    пока i < 10 {
        если i == 3 {
            прервать
        }
        вывести i
        i = i + 1
    }
    вывести "конец"
}
`)
	want := "0\n1\n2\nконец\n"
	if got != want {
		t.Fatalf("получено %q, ожидалось %q", got, want)
	}
}

// TestRunContinueSkipsRestOfIteration - "продолжить" внутри "пока"
// пропускает оставшуюся часть тела, но не сам цикл.
func TestRunContinueSkipsRestOfIteration(t *testing.T) {
	got := runSrc(t, `
функция главная
{
    переменная i = 0
    пока i < 5 {
        i = i + 1
        если i % 2 == 0 {
            продолжить
        }
        вывести i
    }
}
`)
	want := "1\n3\n5\n"
	if got != want {
		t.Fatalf("получено %q, ожидалось %q", got, want)
	}
}

// TestRunBreakStopsRangeForLoop - "прервать" работает и в "для <имя> от
// <от> до <до>", не только в "пока".
func TestRunBreakStopsRangeForLoop(t *testing.T) {
	got := runSrc(t, `
функция главная
{
    для i от 1 до 10 {
        если i > 3 {
            прервать
        }
        вывести i
    }
}
`)
	want := "1\n2\n3\n"
	if got != want {
		t.Fatalf("получено %q, ожидалось %q", got, want)
	}
}

// TestRunCStyleForBasic - общий "для <init>; <условие>; <шаг>" ведёт себя
// как в C: Init выполняется один раз, Cond проверяется перед каждой
// итерацией, Post - после тела, перед повторной проверкой Cond.
func TestRunCStyleForBasic(t *testing.T) {
	got := runSrc(t, `
функция главная
{
    для i = 0; i < 5; i = i + 1 {
        вывести i
    }
}
`)
	want := "0\n1\n2\n3\n4\n"
	if got != want {
		t.Fatalf("получено %q, ожидалось %q", got, want)
	}
}

// TestRunCStyleForDescendingStep - произвольный шаг/направление - то, чего
// не умеет диапазонный "для <имя> от <от> до <до>" (только по возрастанию,
// шаг 1).
func TestRunCStyleForDescendingStep(t *testing.T) {
	got := runSrc(t, `
функция главная
{
    для i = 10; i > 0; i = i - 3 {
        вывести i
    }
}
`)
	want := "10\n7\n4\n1\n"
	if got != want {
		t.Fatalf("получено %q, ожидалось %q", got, want)
	}
}

// TestRunCStyleForContinueRunsPost - "продолжить" внутри общего "для" всё
// равно выполняет Post перед повторной проверкой условия, ровно как в C
// (не пропускает шаг счётчика).
func TestRunCStyleForContinueRunsPost(t *testing.T) {
	got := runSrc(t, `
функция главная
{
    для i = 0; i < 6; i = i + 1 {
        если i % 2 == 0 {
            продолжить
        }
        вывести i
    }
}
`)
	want := "1\n3\n5\n"
	if got != want {
		t.Fatalf("получено %q, ожидалось %q", got, want)
	}
}

// TestRunCStyleForInfiniteWithBreak - все три секции пустые ("для ;; { ... }")
// - бесконечный цикл, выйти из которого можно только через "прервать".
func TestRunCStyleForInfiniteWithBreak(t *testing.T) {
	got := runSrc(t, `
функция главная
{
    переменная i = 0
    для ;; {
        если i >= 3 {
            прервать
        }
        вывести i
        i = i + 1
    }
}
`)
	want := "0\n1\n2\n"
	if got != want {
		t.Fatalf("получено %q, ожидалось %q", got, want)
	}
}

// TestRunCStyleForOnlyCondition - только условие задано (Init/Post пустые) -
// эквивалент "пока", но через синтаксис общего "для".
func TestRunCStyleForOnlyCondition(t *testing.T) {
	got := runSrc(t, `
функция главная
{
    переменная i = 0
    для ; i < 3; {
        вывести i
        i = i + 1
    }
}
`)
	want := "0\n1\n2\n"
	if got != want {
		t.Fatalf("получено %q, ожидалось %q", got, want)
	}
}

// TestRunBreakInsideTryRunsFinally - "прервать" изнутри "попытаться" внутри
// цикла всё равно должен выполнить "окончательно" (тот же принцип, что и у
// раннего "вернуть" - Шаг 71), прежде чем прервать цикл.
func TestRunBreakInsideTryRunsFinally(t *testing.T) {
	got := runSrc(t, `
функция главная
{
    переменная i = 0
    пока i < 5 {
        попытаться {
            если i == 2 {
                прервать
            }
        } поймать(x: Паника) {
        } окончательно {
            вывести "окончательно " + (Строка) i
        }
        i = i + 1
    }
    вывести "конец"
}
`)
	want := "окончательно 0\nокончательно 1\nокончательно 2\nконец\n"
	if got != want {
		t.Fatalf("получено %q, ожидалось %q", got, want)
	}
}

// TestRunBreakDepthStopsOuterLoop - "прервать N" прерывает
// N-й по счёту охватывающий цикл, считая текущий - "прервать 2" изнутри
// вложенного "для" прекращает ОБА цикла сразу, не только ближний.
func TestRunBreakDepthStopsOuterLoop(t *testing.T) {
	got := runSrc(t, `
функция главная
{
    для i от 1 до 3 {
        для j от 1 до 3 {
            если j == 2 {
                прервать 2
            }
            вывести i
            вывести j
        }
        вывести 900
    }
    вывести "конец"
}
`)
	want := "1\n1\nконец\n"
	if got != want {
		t.Fatalf("получено %q, ожидалось %q", got, want)
	}
}

// TestRunContinueDepthSkipsOuterIteration - "продолжить N" прекращает ТЕКУЩИЙ
// (ближний) цикл целиком и переходит к следующей итерации N-го по счёту
// цикла - "продолжить 2" изнутри вложенного "для" переходит сразу к
// следующему i, пропуская остаток тела внешнего цикла ПОСЛЕ внутреннего
// "для" (900 не печатается ни разу).
func TestRunContinueDepthSkipsOuterIteration(t *testing.T) {
	got := runSrc(t, `
функция главная
{
    для i от 1 до 3 {
        для j от 1 до 3 {
            если j == 2 {
                продолжить 2
            }
            вывести i
            вывести j
        }
        вывести 900
    }
}
`)
	want := "1\n1\n2\n1\n3\n1\n"
	if got != want {
		t.Fatalf("получено %q, ожидалось %q", got, want)
	}
}

// TestRunBreakDepthThreeLevels - "прервать 3" через три уровня вложенности -
// каждый промежуточный цикл сам останавливается и пробрасывает сигнал дальше
// (handleLoopCtrl, exec.go), не только ближайший к целевому.
func TestRunBreakDepthThreeLevels(t *testing.T) {
	got := runSrc(t, `
функция главная
{
    для i от 1 до 2 {
        для j от 1 до 2 {
            для k от 1 до 2 {
                если i == 2 && j == 1 && k == 1 {
                    прервать 3
                }
                вывести i
                вывести j
                вывести k
            }
        }
    }
    вывести "конец"
}
`)
	want := "1\n1\n1\n1\n1\n2\n1\n2\n1\n1\n2\n2\nконец\n"
	if got != want {
		t.Fatalf("получено %q, ожидалось %q", got, want)
	}
}

// TestRunBreakDepthInsideTryRunsFinally - "прервать N" изнутри "попытаться"
// вложенного цикла всё равно обязан выполнить "окончательно" каждого
// пройденного "попытаться" на пути наружу (тот же принцип, что и у обычного
// "прервать" - TestRunBreakInsideTryRunsFinally выше, cleanupStack/
// cleanupBase - см. package doc irGenCtx.loopStack в cbackend).
func TestRunBreakDepthInsideTryRunsFinally(t *testing.T) {
	got := runSrc(t, `
функция главная
{
    для i от 1 до 3 {
        для j от 1 до 3 {
            попытаться {
                если j == 2 {
                    прервать 2
                }
            } окончательно {
                вывести "ок " + (Строка) i + " " + (Строка) j
            }
        }
    }
    вывести "конец"
}
`)
	want := "ок 1 1\nок 1 2\nконец\n"
	if got != want {
		t.Fatalf("получено %q, ожидалось %q", got, want)
	}
}

// TestRunBreakDepthExceedsNestingIsCompileError - "прервать N" с N больше
// реальной вложенности циклов в этой точке - ошибка семантического анализа
// (checkLoopJumpDepth), не паника/некорректный индекс во время выполнения.
func TestRunBreakDepthExceedsNestingIsCompileError(t *testing.T) {
	src := `
функция главная
{
    для i от 1 до 3 {
        прервать 2
    }
}
`
	toks, err := lexer.New(src).Tokenize()
	if err != nil {
		t.Fatalf("лексер: %v", err)
	}
	prog, err := parser.New(toks).ParseProgram()
	if err != nil {
		t.Fatalf("парсер: %v", err)
	}
	if _, err := semantic.Analyze(prog); err == nil {
		t.Fatal("ожидалась ошибка семантического анализа для \"прервать 2\" при вложенности 1, получено nil")
	}
}

// TestRunBreakOutsideLoopIsCompileError - "прервать"/"продолжить" вне
// цикла - ошибка семантического анализа, не паника времени выполнения.
func TestRunBreakOutsideLoopIsCompileError(t *testing.T) {
	src := `
функция главная
{
    прервать
}
`
	toks, err := lexer.New(src).Tokenize()
	if err != nil {
		t.Fatalf("лексер: %v", err)
	}
	prog, err := parser.New(toks).ParseProgram()
	if err != nil {
		t.Fatalf("парсер: %v", err)
	}
	if _, err := semantic.Analyze(prog); err == nil {
		t.Fatal("ожидалась ошибка семантического анализа для \"прервать\" вне цикла, получено nil")
	}
}
