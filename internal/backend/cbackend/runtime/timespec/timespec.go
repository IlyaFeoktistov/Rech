// Абсолютный дедлайн (struct timespec) из миллисекунд, для
// pthread_cond_timedwait на POSIX - общий кусок между rech_wait_all_tasks
// (active_tasks.go, "дождаться_всех_задач") и rech_db_checkout (db.go, "бд.
// таймаут_пула_мс") - оба ждут на condvar с ограничением по времени, ни один
// не должен нести собственную копию арифметики "миллисекунды -> секунды +
// наносекунды" с голыми числами по месту.
package timespec

import "fmt"

const (
	// rechMsPerSec/rechNsPerMs/rechNsPerSec - обычные константы перевода
	// единиц времени (не значения, специфичные для Rech) - названы, а не
	// оставлены голыми числами по месту использования (DeadlineCode
	// ниже - единственное место, где они нужны).
	rechMsPerSec = 1000
	rechNsPerMs  = 1_000_000
	rechNsPerSec = 1_000_000_000
)

// DeadlineCode строит C-инструкции, вычисляющие varName (уже
// объявленную вызывающим кодом "struct timespec") - абсолютный момент
// времени через msExpr миллисекунд от clock_gettime(CLOCK_REALTIME). msExpr -
// произвольное C-выражение (имя переменной или #define), не обязательно
// целочисленный литерал - вызывающий код сам решает, откуда берётся число
// миллисекунд (параметр функции у rech_wait_all_tasks, макрос
// RECH_DB_POOL_TIMEOUT_MS у rech_db_checkout).
func DeadlineCode(varName, msExpr string) string {
	return fmt.Sprintf(`    clock_gettime(CLOCK_REALTIME, &%[1]s);
    %[1]s.tv_sec += %[2]s / %[3]d;
    %[1]s.tv_nsec += (%[2]s %% %[3]d) * %[4]d;
    if (%[1]s.tv_nsec >= %[5]d) {
        %[1]s.tv_sec++;
        %[1]s.tv_nsec -= %[5]d;
    }
`, varName, msExpr, rechMsPerSec, rechNsPerMs, rechNsPerSec)
}
