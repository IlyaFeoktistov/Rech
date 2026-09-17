// Мини-отладчик, tier3 - регистры процессора и точки останова - см.
// package doc internal/semantic/procregs.go. Три ветки на платформу тем
// же приёмом, что и runtime/procmem (tier1/tier2) - ЯВНО #if defined(__linux__)
// /#elif defined(_WIN32)/#else, не "#ifndef _WIN32".
//
// Таблица точек останова (rech_bp_table) - общая для обеих платформ,
// обычные данные (pid/адрес/исходный байт), без единого системного
// вызова - хранит, чем восстановить байт при "снять_точку_останова"/
// внутреннем шаге через уже поставленную точку в "возобновить".
//
// Таблица Windows-сессий DebugActiveProcess (rech_win_dbg_t/rech_win_
// session_*) объявлена в runtime/procmem (tier1), не здесь - "подключиться_к_
// процессу" на Windows теперь тоже реально останавливает процесс через
// DebugActiveProcess (нужно ОБЕИМ сторонам - и tier1 attach/detach, и
// этому файлу), см. её package doc там же про причину. "прочитать_
// память"/"записать_память"/"найти_байты" (tier1/tier2) от этой таблицы
// не зависят вовсе (свои OpenProcess).
package procregs

import (
	"fmt"
	"strings"
)

// RegisterFieldNames - имена полей "Регистры" (internal/semantic/
// procregs.go, RegistersFields) - курируемый список ЗДЕСЬ ЖЕ (cbackend не
// импортирует internal/semantic) - тот же принцип дублирования имени,
// что и byteListType/httpserver.QueryParamsDictType (см. их package doc).
// Вызывающая сторона манглит каждое имя (mangleField - внутренняя,
// неэкспортированная инфраструктура cbackend) и передаёт результат
// параллельным срезом в RuntimeCode - этот пакет от cbackend не зависит.
var RegisterFieldNames = []string{
	"rax", "rbx", "rcx", "rdx", "rsi", "rdi", "rbp", "rsp", "rip",
	"r8", "r9", "r10", "r11", "r12", "r13", "r14", "r15", "eflags",
}

// linuxRegsCopyToOut/linuxRegsCopyFromIn - "out.f_X = (long long)regs.X;"/
// "regs.X = (unsigned long long)r.f_X;" на каждое поле - struct
// user_regs_struct (<sys/user.h>) называет поля СТРОЧНЫМИ буквами, точно
// как RegisterFieldNames, кроме "eflags" - тоже строчными там же.
// mangledFields - уже манглированные имена (mangleField), в ТОМ ЖЕ
// порядке, что и RegisterFieldNames (см. package doc выше).
func linuxRegsCopyToOut(structVar, outVar string, mangledFields []string) string {
	var sb strings.Builder
	for i, name := range RegisterFieldNames {
		fmt.Fprintf(&sb, "    %s.%s = (long long)%s.%s;\n", outVar, mangledFields[i], structVar, name)
	}
	return sb.String()
}

func linuxRegsCopyFromIn(structVar, inVar string, mangledFields []string) string {
	var sb strings.Builder
	for i, name := range RegisterFieldNames {
		fmt.Fprintf(&sb, "    %s.%s = (unsigned long long)%s.%s;\n", structVar, name, inVar, mangledFields[i])
	}
	return sb.String()
}

// windowsRegsCopyToOut/windowsRegsCopyFromIn - то же самое для Windows
// CONTEXT (WinNT.h) - поля названы С ЗАГЛАВНОЙ буквы (Rax/Rbx/.../Rip/
// R8..R15/EFlags), сама CONTEXT уже объявлена системным <windows.h>.
func windowsRegsCopyToOut(structVar, outVar string, mangledFields []string) string {
	var sb strings.Builder
	for i, name := range RegisterFieldNames {
		fmt.Fprintf(&sb, "    %s.%s = (long long)%s.%s;\n", outVar, mangledFields[i], structVar, windowsContextFieldName(name))
	}
	return sb.String()
}

func windowsRegsCopyFromIn(structVar, inVar string, mangledFields []string) string {
	var sb strings.Builder
	for i, name := range RegisterFieldNames {
		fmt.Fprintf(&sb, "    %s.%s = (DWORD64)%s.%s;\n", structVar, windowsContextFieldName(name), inVar, mangledFields[i])
	}
	return sb.String()
}

// windowsContextFieldName - "rax"->"Rax", "r8"->"R8", "eflags"->"EFlags" -
// точное написание полей CONTEXT (WinNT.h), не общее правило capitalize
// (EFlags - с большой "F" тоже, единственное исключение).
func windowsContextFieldName(name string) string {
	if name == "eflags" {
		return "EFlags"
	}
	return strings.ToUpper(name[:1]) + name[1:]
}

// RuntimeCode - "регистры_процесса"/"установить_регистры_процесса"/
// "установить_точку_останова"/"снять_точку_останова"/"возобновить_процесс" -
// см. package doc выше. regsType/procType/pidField/mangledFields -
// манглированные C-имена (mangleType/mangleField), *Msg - уже
// экранированные (escapeCString) сообщения об ошибке - вся эта
// инфраструктура неэкспортирована в cbackend, этот пакет от неё не
// зависит.
func RuntimeCode(regsType, procType, pidField string, mangledFields []string,
	readFailedMsg, writeFailedMsg, setBreakpointFailedMsg, removeBreakpointFailedMsg, resumeFailedMsg string) string {
	return fmt.Sprintf(`typedef struct { long long pid; long long addr; unsigned char orig; } rech_bp_entry;
static rech_bp_entry *rech_bp_table = NULL;
static long long rech_bp_count = 0, rech_bp_cap = 0;

static void rech_bp_add(long long pid, long long addr, unsigned char orig) {
    if (rech_bp_count == rech_bp_cap) {
        rech_bp_cap = rech_bp_cap == 0 ? 8 : rech_bp_cap * 2;
        rech_bp_table = realloc(rech_bp_table, sizeof(rech_bp_entry) * (size_t)rech_bp_cap);
    }
    rech_bp_table[rech_bp_count].pid = pid;
    rech_bp_table[rech_bp_count].addr = addr;
    rech_bp_table[rech_bp_count].orig = orig;
    rech_bp_count++;
}

// rech_bp_find - remove!=0 удаляет запись из таблицы (снятие точки
// останова/подготовка её адреса под "исходный вид" - обе операции больше
// не нуждаются в записи).
static int rech_bp_find(long long pid, long long addr, unsigned char *out, int remove) {
    for (long long i = 0; i < rech_bp_count; i++) {
        if (rech_bp_table[i].pid == pid && rech_bp_table[i].addr == addr) {
            if (out) { *out = rech_bp_table[i].orig; }
            if (remove) {
                rech_bp_table[i] = rech_bp_table[rech_bp_count - 1];
                rech_bp_count--;
            }
            return 1;
        }
    }
    return 0;
}

#if defined(__linux__)
// rech_ptrace_peek_byte/rech_ptrace_poke_byte - ЕДИНСТВЕННЫЙ надёжный
// способ править код исполняемого процесса через ptrace -
// process_vm_writev (в отличие от process_vm_readv) уважает обычные права
// страницы, а текстовый сегмент почти всегда r-xp (без записи) - живой
// найденный баг: "точка_останова" писала 0xCC через process_vm_writev,
// падала на readonly-странице кода. PTRACE_POKETEXT/PEEKTEXT - единственная
// операция ядра, которой явно разрешено писать в code-страницы процесса
// вне зависимости от их обычных прав (специально для отладчиков) - работает
// целыми машинными словами (sizeof(long) байт), не отдельными байтами,
// поэтому здесь - "прочитать слово, поправить младший байт, записать
// слово обратно", не прямая запись одного байта.
static int rech_ptrace_peek_byte(pid_t pid, long long addr, unsigned char *out) {
    errno = 0;
    long word = ptrace(PTRACE_PEEKTEXT, pid, (void *)(intptr_t)addr, NULL);
    if (word == -1 && errno != 0) {
        return 0;
    }
    *out = (unsigned char)(word & 0xFF);
    return 1;
}

static int rech_ptrace_poke_byte(pid_t pid, long long addr, unsigned char value) {
    errno = 0;
    long word = ptrace(PTRACE_PEEKTEXT, pid, (void *)(intptr_t)addr, NULL);
    if (word == -1 && errno != 0) {
        return 0;
    }
    long patched = (word & ~(long)0xFF) | (long)value;
    return ptrace(PTRACE_POKETEXT, pid, (void *)(intptr_t)addr, (void *)patched) == 0;
}
#endif

static %[1]s rech_proc_get_regs(%[2]s p) {
    %[1]s out;
#if defined(__linux__)
    struct user_regs_struct regs;
    if (ptrace(PTRACE_GETREGS, (pid_t)p.%[3]s, NULL, &regs) != 0) {
        rech_panic("%[4]s");
    }
%[5]s#elif defined(_WIN32)
    rech_win_dbg_t *s = rech_win_session_find(p.%[3]s);
    if (s == NULL) {
        rech_panic("%[4]s");
    }
    HANDLE th = OpenThread(THREAD_GET_CONTEXT | THREAD_SET_CONTEXT, FALSE, s->tid);
    if (th == NULL) {
        rech_panic("%[4]s");
    }
    CONTEXT ctx;
    ctx.ContextFlags = CONTEXT_FULL;
    if (!GetThreadContext(th, &ctx)) {
        CloseHandle(th);
        rech_panic("%[4]s");
    }
    CloseHandle(th);
%[6]s#else
    rech_panic("%[4]s");
#endif
    return out;
}

static void rech_proc_set_regs(%[2]s p, %[1]s r) {
#if defined(__linux__)
    struct user_regs_struct regs;
    if (ptrace(PTRACE_GETREGS, (pid_t)p.%[3]s, NULL, &regs) != 0) {
        rech_panic("%[7]s");
    }
%[8]s    if (ptrace(PTRACE_SETREGS, (pid_t)p.%[3]s, NULL, &regs) != 0) {
        rech_panic("%[7]s");
    }
#elif defined(_WIN32)
    rech_win_dbg_t *s = rech_win_session_find(p.%[3]s);
    if (s == NULL) {
        rech_panic("%[7]s");
    }
    HANDLE th = OpenThread(THREAD_GET_CONTEXT | THREAD_SET_CONTEXT, FALSE, s->tid);
    if (th == NULL) {
        rech_panic("%[7]s");
    }
    CONTEXT ctx;
    ctx.ContextFlags = CONTEXT_FULL;
    if (!GetThreadContext(th, &ctx)) {
        CloseHandle(th);
        rech_panic("%[7]s");
    }
%[9]s    if (!SetThreadContext(th, &ctx)) {
        CloseHandle(th);
        rech_panic("%[7]s");
    }
    CloseHandle(th);
#else
    (void)p;
    (void)r;
    rech_panic("%[7]s");
#endif
}

static void rech_proc_set_breakpoint(%[2]s p, long long addr) {
    long long pid = p.%[3]s;
    unsigned char orig;
#if defined(__linux__)
    if (!rech_ptrace_peek_byte((pid_t)pid, addr, &orig) || !rech_ptrace_poke_byte((pid_t)pid, addr, 0xCC)) {
        rech_panic("%[10]s");
    }
#elif defined(_WIN32)
    HANDLE h = OpenProcess(PROCESS_VM_READ | PROCESS_VM_WRITE | PROCESS_VM_OPERATION, FALSE, (DWORD)pid);
    if (h == NULL) {
        rech_panic("%[10]s");
    }
    SIZE_T n;
    if (!ReadProcessMemory(h, (LPCVOID)(intptr_t)addr, &orig, 1, &n) || n != 1) {
        CloseHandle(h);
        rech_panic("%[10]s");
    }
    // VirtualProtectEx - страница кода почти всегда PAGE_EXECUTE_READ (без
    // записи) - WriteProcessMemory сама по себе не гарантирует обход этого
    // (в отличие от PTRACE_POKETEXT на Linux, у которого это разрешение
    // встроено в саму операцию ядра) - явно делаем страницу временно
    // записываемой, пишем 0xCC, возвращаем исходную защиту.
    DWORD old_protect;
    BOOL protected_ok = VirtualProtectEx(h, (LPVOID)(intptr_t)addr, 1, PAGE_EXECUTE_READWRITE, &old_protect);
    unsigned char int3 = 0xCC;
    BOOL wrote = WriteProcessMemory(h, (LPVOID)(intptr_t)addr, &int3, 1, &n) && n == 1;
    if (protected_ok) {
        DWORD unused;
        VirtualProtectEx(h, (LPVOID)(intptr_t)addr, 1, old_protect, &unused);
    }
    if (!wrote) {
        CloseHandle(h);
        rech_panic("%[10]s");
    }
    FlushInstructionCache(h, (LPCVOID)(intptr_t)addr, 1);
    CloseHandle(h);
#else
    rech_panic("%[10]s");
#endif
    rech_bp_add(pid, addr, orig);
}

static void rech_proc_remove_breakpoint(%[2]s p, long long addr) {
    long long pid = p.%[3]s;
    unsigned char orig;
    if (!rech_bp_find(pid, addr, &orig, 1)) {
        rech_panic("%[11]s");
    }
#if defined(__linux__)
    rech_ptrace_poke_byte((pid_t)pid, addr, orig);
#elif defined(_WIN32)
    HANDLE h = OpenProcess(PROCESS_VM_WRITE | PROCESS_VM_OPERATION, FALSE, (DWORD)pid);
    if (h != NULL) {
        DWORD old_protect;
        BOOL protected_ok = VirtualProtectEx(h, (LPVOID)(intptr_t)addr, 1, PAGE_EXECUTE_READWRITE, &old_protect);
        SIZE_T n;
        WriteProcessMemory(h, (LPVOID)(intptr_t)addr, &orig, 1, &n);
        if (protected_ok) {
            DWORD unused;
            VirtualProtectEx(h, (LPVOID)(intptr_t)addr, 1, old_protect, &unused);
        }
        FlushInstructionCache(h, (LPCVOID)(intptr_t)addr, 1);
        CloseHandle(h);
    }
#endif
}

static long long rech_proc_resume(%[2]s p) {
    long long pid = p.%[3]s;
#if defined(__linux__)
    struct user_regs_struct regs;
    if (ptrace(PTRACE_GETREGS, (pid_t)pid, NULL, &regs) == 0) {
        unsigned char orig;
        if (rech_bp_find(pid, (long long)regs.rip, &orig, 0)) {
            rech_ptrace_poke_byte((pid_t)pid, (long long)regs.rip, orig);
            ptrace(PTRACE_SINGLESTEP, (pid_t)pid, NULL, NULL);
            int step_status;
            waitpid((pid_t)pid, &step_status, 0);
            rech_ptrace_poke_byte((pid_t)pid, (long long)regs.rip, 0xCC);
        }
    }
    if (ptrace(PTRACE_CONT, (pid_t)pid, NULL, NULL) != 0) {
        rech_panic("%[12]s");
    }
    int status;
    if (waitpid((pid_t)pid, &status, 0) < 0) {
        rech_panic("%[12]s");
    }
    if (WIFEXITED(status) || WIFSIGNALED(status)) {
        return -1;
    }
    if (WIFSTOPPED(status) && WSTOPSIG(status) == SIGTRAP) {
        struct user_regs_struct after;
        ptrace(PTRACE_GETREGS, (pid_t)pid, NULL, &after);
        after.rip -= 1;
        ptrace(PTRACE_SETREGS, (pid_t)pid, NULL, &after);
        return (long long)after.rip;
    }
    rech_panic("%[12]s");
    return -1;
#elif defined(_WIN32)
    rech_win_dbg_t *s = rech_win_session_find(pid);
    if (s == NULL) {
        rech_panic("%[12]s");
    }
    {
        HANDLE th = OpenThread(THREAD_GET_CONTEXT | THREAD_SET_CONTEXT, FALSE, s->tid);
        if (th != NULL) {
            CONTEXT ctx;
            ctx.ContextFlags = CONTEXT_FULL;
            if (GetThreadContext(th, &ctx)) {
                unsigned char orig;
                if (rech_bp_find(pid, (long long)ctx.Rip, &orig, 0)) {
                    HANDLE ph = OpenProcess(PROCESS_VM_READ | PROCESS_VM_WRITE | PROCESS_VM_OPERATION, FALSE, (DWORD)pid);
                    SIZE_T n;
                    DWORD old_protect;
                    BOOL protected_ok = VirtualProtectEx(ph, (LPVOID)(intptr_t)ctx.Rip, 1, PAGE_EXECUTE_READWRITE, &old_protect);
                    WriteProcessMemory(ph, (LPVOID)(intptr_t)ctx.Rip, &orig, 1, &n);
                    FlushInstructionCache(ph, (LPCVOID)(intptr_t)ctx.Rip, 1);
                    ctx.EFlags |= 0x100;
                    SetThreadContext(th, &ctx);
                    if (s->has_pending) {
                        ContinueDebugEvent(s->pending.dwProcessId, s->pending.dwThreadId, DBG_CONTINUE);
                        s->has_pending = 0;
                    }
                    DEBUG_EVENT step_ev;
                    for (;;) {
                        if (!WaitForDebugEvent(&step_ev, INFINITE)) {
                            rech_panic("%[12]s");
                        }
                        if (step_ev.dwDebugEventCode == EXCEPTION_DEBUG_EVENT &&
                            step_ev.u.Exception.ExceptionRecord.ExceptionCode == EXCEPTION_SINGLE_STEP) {
                            break;
                        }
                        ContinueDebugEvent(step_ev.dwProcessId, step_ev.dwThreadId, DBG_CONTINUE);
                    }
                    unsigned char int3 = 0xCC;
                    WriteProcessMemory(ph, (LPVOID)(intptr_t)ctx.Rip, &int3, 1, &n);
                    if (protected_ok) {
                        DWORD unused;
                        VirtualProtectEx(ph, (LPVOID)(intptr_t)ctx.Rip, 1, old_protect, &unused);
                    }
                    FlushInstructionCache(ph, (LPCVOID)(intptr_t)ctx.Rip, 1);
                    CloseHandle(ph);
                    s->tid = step_ev.dwThreadId;
                    s->pending = step_ev;
                    s->has_pending = 1;
                }
            }
            CloseHandle(th);
        }
    }
    if (s->has_pending) {
        ContinueDebugEvent(s->pending.dwProcessId, s->pending.dwThreadId, DBG_CONTINUE);
        s->has_pending = 0;
    }
    for (;;) {
        DEBUG_EVENT ev;
        if (!WaitForDebugEvent(&ev, INFINITE)) {
            rech_panic("%[12]s");
        }
        s->tid = ev.dwThreadId;
        s->pending = ev;
        s->has_pending = 1;
        if (ev.dwDebugEventCode == EXIT_PROCESS_DEBUG_EVENT) {
            ContinueDebugEvent(ev.dwProcessId, ev.dwThreadId, DBG_CONTINUE);
            s->has_pending = 0;
            rech_win_session_remove(pid);
            return -1;
        }
        if (ev.dwDebugEventCode == EXCEPTION_DEBUG_EVENT &&
            ev.u.Exception.ExceptionRecord.ExceptionCode == EXCEPTION_BREAKPOINT) {
            // Живой найденный баг: ЛЮБОЙ EXCEPTION_BREAKPOINT здесь - не
            // обязательно НАША точка останова - DebugActiveProcess на уже
            // запущенном процессе сама вставляет один синтетический
            // "attach breakpoint" (ntdll!DbgBreakPoint, стандартное
            // поведение Windows debug API - сигнал "подключение
            // завершено") - принимать его за нашу точку останова
            // неверно. Проверяем адрес по таблице ПЕРЕД тем, как считать
            // остановку "нашей".
            HANDLE th2 = OpenThread(THREAD_GET_CONTEXT | THREAD_SET_CONTEXT, FALSE, ev.dwThreadId);
            CONTEXT ctx2;
            ctx2.ContextFlags = CONTEXT_FULL;
            if (GetThreadContext(th2, &ctx2)) {
                long long hit_addr = (long long)ctx2.Rip - 1;
                if (rech_bp_find(pid, hit_addr, NULL, 0)) {
                    ctx2.Rip = (DWORD64)hit_addr;
                    SetThreadContext(th2, &ctx2);
                    CloseHandle(th2);
                    return hit_addr;
                }
            }
            CloseHandle(th2);
            // не наша точка останова (например, синтетический "attach
            // breakpoint") - продолжить дальше молча, как и любое другое
            // неинтересное событие.
        }
        ContinueDebugEvent(ev.dwProcessId, ev.dwThreadId, DBG_CONTINUE);
        s->has_pending = 0;
    }
#else
    rech_panic("%[12]s");
    return -1;
#endif
}
`, regsType, procType, pidField,
		readFailedMsg, linuxRegsCopyToOut("regs", "out", mangledFields), windowsRegsCopyToOut("ctx", "out", mangledFields),
		writeFailedMsg, linuxRegsCopyFromIn("regs", "r", mangledFields), windowsRegsCopyFromIn("ctx", "r", mangledFields),
		setBreakpointFailedMsg, removeBreakpointFailedMsg,
		resumeFailedMsg)
}
