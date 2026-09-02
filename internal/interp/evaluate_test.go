package interp

import (
	"bytes"
	"strings"
	"testing"
	"time"
)

func TestDebuggerEvaluateLocalsAndFields(t *testing.T) {
	prog := irProgForDebug(t, `
сущность Точка
{
    имеет:
        x: Целое
        y: Целое

    конструктор(x: Целое, y: Целое)
    {
        этот.x = x
        этот.y = y
    }
}

функция главная
{
    переменная а = 10
    переменная точка = новая Точка(3, 4)
    вывести а
}
`)
	d, err := NewDebugger(prog, &bytes.Buffer{}, strings.NewReader(""))
	if err != nil {
		t.Fatalf("NewDebugger: %v", err)
	}
	d.SetBreakpoints(debugTestFile, []int{19})
	d.Start(false)
	waitStopped(t, d, 1, 2*time.Second)
	frames := d.StackTrace(1)
	if len(frames) != 1 {
		t.Fatalf("ожидался один кадр, получено %+v", frames)
	}

	cases := []struct {
		expr string
		want string
	}{
		{"а", "10 (Целое)"},
		{"а + 5", "15 (Целое)"},
		{"точка.x", "3 (Целое)"},
		{"точка.x + точка.y", "7 (Целое)"},
		{"а > 5", "истина (Логическое)"},
	}
	for _, c := range cases {
		got, err := d.Evaluate(frames[0].ID, c.expr)
		if err != nil {
			t.Fatalf("Evaluate(%q): %v", c.expr, err)
		}
		if got != c.want {
			t.Fatalf("Evaluate(%q) = %q, ожидалось %q", c.expr, got, c.want)
		}
	}

	d.Continue(1)
	for {
		e := waitEvent(t, d, 2*time.Second)
		if e.Kind == "terminated" {
			break
		}
	}
}

func TestDebuggerEvaluateMethodCall(t *testing.T) {
	prog := irProgForDebug(t, `
сущность Счётчик
{
    имеет:
        значение: Целое

    конструктор(значение: Целое)
    {
        этот.значение = значение
    }

    удвоить(): Целое
    {
        вернуть этот.значение * 2
    }
}

функция главная
{
    переменная счётчик = новый Счётчик(21)
    вывести счётчик.значение
}
`)
	d, err := NewDebugger(prog, &bytes.Buffer{}, strings.NewReader(""))
	if err != nil {
		t.Fatalf("NewDebugger: %v", err)
	}
	d.SetBreakpoints(debugTestFile, []int{21})
	d.Start(false)
	waitStopped(t, d, 1, 2*time.Second)
	frames := d.StackTrace(1)

	got, err := d.Evaluate(frames[0].ID, "счётчик.удвоить()")
	if err != nil {
		t.Fatalf("Evaluate: %v", err)
	}
	if got != "42 (Целое)" {
		t.Fatalf("Evaluate(\"с.удвоить()\") = %q, ожидалось \"42 (Целое)\"", got)
	}

	d.Continue(1)
	for {
		e := waitEvent(t, d, 2*time.Second)
		if e.Kind == "terminated" {
			break
		}
	}
}
