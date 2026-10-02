package cbackend

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

// earlyReturnSrc - "вернуть" без значения в конструкторе (в том числе
// внутри "попытаться") и в лямбдах с результатом и без.
const earlyReturnSrc = `сущность Точка
{
    имеет:
        Целое x
        Целое y = 9

    конструктор(x: Целое)
    {
        этот.x = x
        если x > 5 {
            вернуть
        }
        попытаться {
            если x < 0 {
                вернуть
            }
        } поймать (е: Исключение) {
        }
        этот.y = 2
    }
}

функция главная()
{
    Вызываемый() ф = функция() {
        вернуть
    }
    Вызываемый(Целое): Целое г = функция(а: Целое): Целое {
        если а > 0 {
            вернуть а
        }
        вернуть 0
    }
    ф()
    г(3)
    Точка т = новая Точка(7)
    вывести(` + "`${т.y}\\n`" + `)
}
`

// cFunctionBody - тело C-функции, чья сигнатура начинается с prefix
// (определение, не прототип).
func cFunctionBody(t *testing.T, out, prefix string) string {
	t.Helper()
	start := -1
	for i := 0; ; {
		idx := strings.Index(out[i:], prefix)
		if idx == -1 {
			t.Fatalf("не найдено определение C-функции %q, получено:\n%s", prefix, out)
		}
		lineEnd := strings.IndexByte(out[i+idx:], '\n')
		if lineEnd != -1 && strings.HasSuffix(out[i+idx:i+idx+lineEnd], "{") {
			start = i + idx
			break
		}
		i += idx + len(prefix)
	}
	end := strings.Index(out[start:], "\n}\n")
	if end == -1 {
		t.Fatalf("не найден конец C-функции %q", prefix)
	}
	return out[start : start+end]
}

// TestGenerateFromIRCtorBareReturnIsVoidReturn - тело конструктора - C-функция
// "void": "вернуть" без значения - "return;", не "return 0;" (GCC 14+/clang
// отвергают "return" со значением в void-функции).
func TestGenerateFromIRCtorBareReturnIsVoidReturn(t *testing.T) {
	out := generateFromSrc(t, earlyReturnSrc)
	body := cFunctionBody(t, out, "static void rech_init_tochka(")
	if strings.Contains(body, "return 0;") {
		t.Fatalf("в void-конструкторе не должно быть \"return 0;\", получено:\n%s", body)
	}
	if strings.Count(body, "return;") != 2 {
		t.Fatalf("ожидалось два \"return;\" (оба \"вернуть\"), получено:\n%s", body)
	}
	if strings.Count(body, "rech_trace_pop();") != 3 {
		t.Fatalf("ожидался pop кадра на каждом из двух \"вернуть\" и в конце тела, получено:\n%s", body)
	}
}

// TestGenerateFromIRLambdaReturnKeepsCallerTraceFrame - у лямбды нет
// собственного кадра стека вызовов (см. package doc trace.go): "вернуть"
// внутри неё не снимает кадр вызывающей функции, а без результата - это
// "return;" void-функции.
func TestGenerateFromIRLambdaReturnKeepsCallerTraceFrame(t *testing.T) {
	out := generateFromSrc(t, earlyReturnSrc)
	for _, prefix := range []string{"static void rech___lyambda_1(", "static long long rech___lyambda_2("} {
		body := cFunctionBody(t, out, prefix)
		if strings.Contains(body, "rech_trace_pop") {
			t.Fatalf("лямбда не должна снимать кадр стека вызовов, получено:\n%s", body)
		}
		if strings.Contains(body, "return 0;") {
			t.Fatalf("в лямбде не должно быть технического \"return 0;\", получено:\n%s", body)
		}
	}
}

// TestGenerateFromIREarlyReturnCompilesWithoutReturnMismatch - сгенерированный
// C проходит компилятор с "return"-несоответствием как ошибкой (поведение
// GCC 14+ по умолчанию).
func TestGenerateFromIREarlyReturnCompilesWithoutReturnMismatch(t *testing.T) {
	out := generateFromSrc(t, earlyReturnSrc)
	ccPath := findTestCCompiler(t)
	cPath := filepath.Join(t.TempDir(), "early_return.c")
	if err := os.WriteFile(cPath, []byte(out), 0o644); err != nil {
		t.Fatalf("запись early_return.c: %v", err)
	}
	// #nosec G204 -- ccPath найден локальным поиском (CC/cc/clang/gcc), cPath - файл, только что написанный этим же тестом.
	cmd := exec.Command(ccPath, "-fsyntax-only", "-Werror=return-type", cPath)
	if msg, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("сгенерированный C не прошёл -Werror=return-type: %v\n%s", err, msg)
	}
}
