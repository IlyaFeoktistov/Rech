package interp

import (
	"sync"
	"time"
)

// activeTasksCount - число когда-либо запущенных ("запустить"), ещё не
// завершившихся горутин-задач - "дождаться_всех_задач" ждёт, пока оно не
// станет 0 (или не истечёт таймаут), даже для задач, чей результат никогда
// не был передан "дождаться" (фоновая задача - поток на HTTP-соединение,
// см. package doc internal/backend/cbackend/active_tasks.go, тот же смысл
// на этом backend'е). Простой опрос каждые 10мс, не sync.Cond с ожиданием
// по сигналу - Go не даёт sync.Cond.Wait с таймаутом без отдельной горутины
// на каждый вызов, а это не путь с частыми вызовами (только graceful
// shutdown), опрос проще и достаточен.
var (
	activeTasksMu    sync.Mutex
	activeTasksCount int64
)

func activeTasksInc() {
	activeTasksMu.Lock()
	activeTasksCount++
	activeTasksMu.Unlock()
}

func activeTasksDec() {
	activeTasksMu.Lock()
	activeTasksCount--
	activeTasksMu.Unlock()
}

func activeTasksZero() bool {
	activeTasksMu.Lock()
	defer activeTasksMu.Unlock()
	return activeTasksCount == 0
}

// waitAllTasks - "дождаться_всех_задач(миллисекунды): Логическое".
func waitAllTasks(ms int64) bool {
	if activeTasksZero() {
		return true
	}
	if ms <= 0 {
		return false
	}
	deadline := time.Now().Add(time.Duration(ms) * time.Millisecond)
	for time.Now().Before(deadline) {
		time.Sleep(10 * time.Millisecond)
		if activeTasksZero() {
			return true
		}
	}
	return activeTasksZero()
}
