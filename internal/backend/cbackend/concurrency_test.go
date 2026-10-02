package cbackend

import (
	"strings"
	"testing"
)

const sumSig = "Вызываемый<Целое, Целое>Целое"

// "запустить"/"дождаться": структура задачи сигнатуры, запуск на пуле
// воркеров, ожидание через t->wait. Регистрация потока в Boehm GC -
// автоматическая (GC_THREADS), не ручная.
func TestGenerateLaunchAwaitEmitsTaskOfSignature(t *testing.T) {
	out := generateFromSrc(t, `
функция посчитать(а: Целое, б: Целое): Целое
{
    вернуть а + б
}

функция главная
{
    переменная задача = запустить посчитать(2, 3)
    переменная результат = дождаться задача
    вывести результат
}
`)
	task := mangleTaskType(sumSig)
	for _, want := range []string{
		"#define GC_THREADS",
		"typedef struct " + task + " " + task + ";",
		"struct " + task + " {\n    long long result;\n    void *exc;\n    void (*wait)(struct " + task + " *t);\n    " + mangleCallableType(sumSig) + " fn;\n    long long a0;\n    long long a1;\n    void *sync;\n    int coro_done;\n};",
		"static " + task + " *" + mangleTaskLaunch(sumSig) + "(" + mangleCallableType(sumSig) + " fn, long long a0, long long a1) {",
		"static long long " + mangleTaskAwait(sumSig) + "(" + task + " *t) {",
		"    t->wait(t);\n",
		"rech_pool_submit(rech_task_entry_",
		"rech_catch_frame rech_frame = {0};",
	} {
		if !strings.Contains(out, want) {
			t.Fatalf("ожидался фрагмент %q, получено:\n%s", want, out)
		}
	}
	if strings.Contains(out, "GC_register_my_thread") {
		t.Fatalf("ручная регистрация потока в GC не нужна, получено:\n%s", out)
	}
	if strings.Contains(out, "rech_coro_spawn(rech_coro_entry_") {
		t.Fatalf("программа без корутин не должна эмитить корутинный запуск, получено:\n%s", out)
	}
}

// Корутинный запуск той же сигнатуры - та же структура задачи и тот же
// rech_await_..., отличается только функция ожидания; pthread не нужен.
func TestGenerateCoroLaunchSharesTaskTypeWithoutThreads(t *testing.T) {
	out := generateFromSrc(t, `
асинхронная функция посчитать(а: Целое, б: Целое): Целое
{
    вернуть а + б
}

функция главная
{
    переменная задача = поручить посчитать(2, 3)
    вывести дождаться задача
    вывести дождаться посчитать(4, 5)
}
`)
	for _, want := range []string{
		"static " + mangleTaskType(sumSig) + " *" + mangleCoroLaunch(sumSig) + "(",
		"rech_coro_wait_done(&t->coro_done);",
		"rech_coro_spawn(rech_coro_entry_",
		mangleTaskAwait(sumSig) + "(" + mangleCoroLaunch(sumSig) + "(",
	} {
		if !strings.Contains(out, want) {
			t.Fatalf("ожидался фрагмент %q, получено:\n%s", want, out)
		}
	}
	if strings.Contains(out, "rech_task_sync") || strings.Contains(out, "#include <pthread.h>") {
		t.Fatalf("программа только с корутинами не должна тянуть синхронизацию потоков, получено:\n%s", out)
	}
}

// Исключение, не пойманное внутри задачи, поднимается заново в
// rech_await_... тем же rech_throw, что и обычное "выбросить".
func TestGenerateAwaitRethrowsUncaughtException(t *testing.T) {
	out := generateFromSrc(t, `
функция сломать()
{
    выбросить новое Исключение("необработано")
}

функция главная
{
    переменная задача = запустить сломать()
    дождаться задача
}
`)
	if !strings.Contains(out, "rech_throw(t->exc, rech_type_names[") {
		t.Fatalf("ожидался rech_throw(t->exc, <фактический тип>), получено:\n%s", out)
	}
	if !strings.Contains(out, "static void "+mangleTaskAwait("Вызываемый<>")+"(") {
		t.Fatalf("задача без результата - rech_await_... без результата, получено:\n%s", out)
	}
}

// Абстрактная "Задача" (элемент Список<Задача>) - "void *", не
// несуществующий C-тип "rech_TASK_".
func TestGenerateAbstractTaskListElementUsesVoidPointer(t *testing.T) {
	out := generateFromSrc(t, `
функция посчитать(): Целое
{
    вернуть 42
}

функция главная
{
    переменная задачи = новый Список<Задача>()
    задачи.добавить(запустить посчитать())
}
`)
	if strings.Contains(out, "rech_TASK_ ") {
		t.Fatalf("абстрактная \"Задача\" не должна порождать C-тип \"rech_TASK_\", получено:\n%s", out)
	}
	if !strings.Contains(out, "rech_T_spisok_of_zadacha *this_, void * value) {") {
		t.Fatalf("ожидался элемент коллекции типа \"void *\", получено:\n%s", out)
	}
}

// Метод в задаче - лямбда с получателем, захваченным в момент запуска.
func TestGenerateLaunchMethodCapturesReceiverAtLaunch(t *testing.T) {
	out := generateFromSrc(t, `
сущность Журнал
{
    имеет:
        Целое записи

    записать()
    {
        этот.записи++
    }
}

функция главная
{
    переменная журнал = новый Журнал()
    дождаться запустить журнал.записать()
}
`)
	if !strings.Contains(out, mangleTaskLaunch("Вызываемый<>")+"((rech_C_") || !strings.Contains(out, "GC_MALLOC(sizeof(rech_ENV_") {
		t.Fatalf("ожидался запуск лямбды с окружением-получателем, получено:\n%s", out)
	}
}
