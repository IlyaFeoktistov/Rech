package interp

import (
	"os"
	"testing"
	"time"
)

// TestCanReadTimesOutWithoutBlockingFull - настоящий os.Pipe() (не имитация)
// - без данных "можно_прочитать_байт" обязан вернуться ложью РОВНО по истечении
// заданного таймаута, не раньше и не позже (в отличие от блокирующего
// "прочитать", который ждал бы данные сколько угодно).
func TestCanReadTimesOutWithoutBlockingFull(t *testing.T) {
	r, w, err := os.Pipe()
	if err != nil {
		t.Fatalf("os.Pipe: %v", err)
	}
	defer r.Close()
	defer w.Close()

	start := time.Now()
	got := canRead(r, 100)
	elapsed := time.Since(start)

	if got {
		t.Fatal("ожидалась ложь - данных в пайпе нет")
	}
	if elapsed < 90*time.Millisecond || elapsed > 500*time.Millisecond {
		t.Fatalf("ожидалось ожидание ~100мс, получено %v", elapsed)
	}
}

// TestCanReadDetectsAvailableData - если данные УЖЕ есть в пайпе,
// "можно_прочитать_байт" обязан вернуть истину немедленно, не дожидаясь
// таймаута.
func TestCanReadDetectsAvailableData(t *testing.T) {
	r, w, err := os.Pipe()
	if err != nil {
		t.Fatalf("os.Pipe: %v", err)
	}
	defer r.Close()
	defer w.Close()

	if _, err := w.Write([]byte("x")); err != nil {
		t.Fatalf("write: %v", err)
	}

	start := time.Now()
	got := canRead(r, 5000)
	elapsed := time.Since(start)

	if !got {
		t.Fatal("ожидалась истина - данные уже в пайпе")
	}
	if elapsed > 500*time.Millisecond {
		t.Fatalf("ожидался немедленный возврат, получено %v", elapsed)
	}
}

// TestCanReadDetectsDataArrivingDuringWait - данные приходят ПОСРЕДИ
// ожидания (не сразу, не после таймаута) - "можно_прочитать_байт" обязан
// вернуть истину как только данные пришли, не дожидаясь полного таймаута.
func TestCanReadDetectsDataArrivingDuringWait(t *testing.T) {
	r, w, err := os.Pipe()
	if err != nil {
		t.Fatalf("os.Pipe: %v", err)
	}
	defer r.Close()
	defer w.Close()

	go func() {
		time.Sleep(100 * time.Millisecond)
		w.Write([]byte("y"))
	}()

	start := time.Now()
	got := canRead(r, 5000)
	elapsed := time.Since(start)

	if !got {
		t.Fatal("ожидалась истина - данные пришли до таймаута")
	}
	if elapsed < 90*time.Millisecond || elapsed > 2*time.Second {
		t.Fatalf("ожидалось обнаружение данных вскоре после ~100мс, получено %v", elapsed)
	}
}
