// procMemRuntimeCode - "подключиться_к_процессу"/"отключиться_от_процесса"/
// "прочитать_память"/"записать_память" (internal/semantic/procmem.go,
// package doc там) - мини-отладчик, доступ к памяти уже запущенного
// процесса по pid, поверх уже существующей сущности "Процесс" (только
// поле pid, mangleField("pid")).
//
// Три ветки на платформу, ЯВНО (#if defined(__linux__)/#elif defined(_WIN32)
// /#else), не "#ifndef _WIN32" - живой найденный баг того же класса уже
// был (SIGSTKFLT/SIGPWR, internal/backend/cbackend/signals.go, не
// собирались на macOS под "#ifndef _WIN32"): process_vm_readv/writev и
// ptrace(PTRACE_ATTACH/PTRACE_DETACH) - расширения Linux, не обязаны
// существовать (или вести себя так же) на других POSIX-системах - третья
// ветка (macOS и все остальные) собирается, но паникует при первой же
// попытке реально подключиться, вместо того чтобы либо не собраться
// вовсе, либо тихо повести себя иначе.
//
//   - rech_proc_attach - ОСТАНАВЛИВАЕТ целевой процесс на ОБЕИХ
//     платформах (нужно единообразно - не только для tier1, но и для
//     регистров/точек останова, internal/backend/cbackend/procregs.go).
//     Linux - ptrace(PTRACE_ATTACH) + waitpid. Windows -
//     DebugActiveProcess (вызывающий становится ЕДИНСТВЕННЫМ отладчиком
//     процесса) + первый WaitForDebugEvent (CREATE_PROCESS_DEBUG_EVENT,
//     генерируется автоматически при подключении) - процесс остаётся
//     остановленным, пока за него не вызван ContinueDebugEvent (см.
//     rech_win_dbg_t/rech_win_session_* ниже - та же таблица сессий,
//     нужна и здесь, и в procregs.go).
//   - rech_proc_detach - Linux: PTRACE_DETACH (возобновляет процесс).
//     Windows: ContinueDebugEvent за необработанное событие (иначе
//     процесс так и останется висеть остановленным навсегда) +
//     DebugActiveProcessStop (перестать быть отладчиком - процесс
//     продолжает жить самостоятельно).
//   - rech_proc_read/rech_proc_write - Linux: process_vm_readv/writev
//     (не PTRACE_PEEKDATA/POKEDATA - тот работает только словами
//     машинного размера, эти читают/пишут произвольное количество байт
//     одним вызовом). Windows: ReadProcessMemory/WriteProcessMemory по
//     хендлу, открытому и закрытому ВНУТРИ каждого вызова (Процесс несёт
//     только pid, не HANDLE - тот же принцип, что и rech_wait_process,
//     subprocess.go).
package cbackend

import (
	"fmt"

	"github.com/IlyaFeoktistov/Rech/internal/rtmsg"
)

// intListType - см. internal/semantic/procmem.go - каноническое имя
// generic-инстанциации, возвращаемой "найти_байты" (адреса совпадений).
const intListType = "Список<Целое>"

var procMemRuntimeCode = fmt.Sprintf(`#ifdef _WIN32
// rech_win_dbg_t - сессия DebugActiveProcess - "tid"/"pending"/
// "has_pending" - поток и событие ПОСЛЕДНЕЙ остановки (нужны
// GetThreadContext/SetThreadContext и ContinueDebugEvent, который обязан
// указать ИМЕННО тот pid/tid, что был в событии) - используется и здесь
// (attach/detach), и в internal/backend/cbackend/procregs.go (регистры/
// точки останова/возобновить), поэтому таблица - здесь, в tier1.
typedef struct {
    long long pid;
    DWORD tid;
    int has_pending;
    DEBUG_EVENT pending;
} rech_win_dbg_t;
static rech_win_dbg_t *rech_win_sessions = NULL;
static long long rech_win_session_count = 0, rech_win_session_cap = 0;

static rech_win_dbg_t *rech_win_session_find(long long pid) {
    for (long long i = 0; i < rech_win_session_count; i++) {
        if (rech_win_sessions[i].pid == pid) { return &rech_win_sessions[i]; }
    }
    return NULL;
}

static rech_win_dbg_t *rech_win_session_add(long long pid) {
    if (rech_win_session_count == rech_win_session_cap) {
        rech_win_session_cap = rech_win_session_cap == 0 ? 4 : rech_win_session_cap * 2;
        rech_win_sessions = realloc(rech_win_sessions, sizeof(rech_win_dbg_t) * (size_t)rech_win_session_cap);
    }
    rech_win_dbg_t *s = &rech_win_sessions[rech_win_session_count++];
    s->pid = pid;
    s->tid = 0;
    s->has_pending = 0;
    return s;
}

static void rech_win_session_remove(long long pid) {
    for (long long i = 0; i < rech_win_session_count; i++) {
        if (rech_win_sessions[i].pid == pid) {
            rech_win_sessions[i] = rech_win_sessions[rech_win_session_count - 1];
            rech_win_session_count--;
            return;
        }
    }
}

// rech_enable_debug_privilege - живой найденный баг: DebugActiveProcess
// падает с ERROR_ACCESS_DENIED даже на СВОЙ собственный дочерний процесс
// (запущенный обычным CreateProcess, БЕЗ флага DEBUG_PROCESS) - в отличие
// от "родитель отлаживает потомка через CREATE_PROCESS с DEBUG_PROCESS",
// подключение ПОСТФАКТУМ через DebugActiveProcess требует явно включённой
// привилегии SeDebugPrivilege в токене текущего процесса - она есть у
// большинства процессов (в т. ч. без прав администратора), но НЕ включена
// по умолчанию (доступна - не значит активна, Windows-специфика
// привилегий). Идемпотентно (статический флаг - один раз на процесс).
static void rech_enable_debug_privilege(void) {
    static int done = 0;
    if (done) {
        return;
    }
    done = 1;
    HANDLE token;
    if (!OpenProcessToken(GetCurrentProcess(), TOKEN_ADJUST_PRIVILEGES | TOKEN_QUERY, &token)) {
        return;
    }
    LUID luid;
    if (LookupPrivilegeValueA(NULL, "SeDebugPrivilege", &luid)) {
        TOKEN_PRIVILEGES tp;
        tp.PrivilegeCount = 1;
        tp.Privileges[0].Luid = luid;
        tp.Privileges[0].Attributes = SE_PRIVILEGE_ENABLED;
        AdjustTokenPrivileges(token, FALSE, &tp, 0, NULL, NULL);
    }
    CloseHandle(token);
}
#endif

static %[1]s rech_proc_attach(long long pid) {
    %[1]s out;
    out.%[2]s = pid;
#if defined(__linux__)
    if (ptrace(PTRACE_ATTACH, (pid_t)pid, NULL, NULL) != 0) {
        rech_panic("%[3]s");
    }
    int status;
    if (waitpid((pid_t)pid, &status, 0) < 0) {
        rech_panic("%[3]s");
    }
#elif defined(_WIN32)
    rech_enable_debug_privilege();
    if (!DebugActiveProcess((DWORD)pid)) {
        rech_panic("%[3]s");
    }
    rech_win_dbg_t *s = rech_win_session_add(pid);
    if (!WaitForDebugEvent(&s->pending, INFINITE)) {
        rech_win_session_remove(pid);
        rech_panic("%[3]s");
    }
    s->tid = s->pending.dwThreadId;
    s->has_pending = 1;
#else
    rech_panic("%[3]s");
#endif
    return out;
}

static void rech_proc_detach(%[1]s p) {
#if defined(__linux__)
    if (ptrace(PTRACE_DETACH, (pid_t)p.%[2]s, NULL, NULL) != 0) {
        rech_panic("%[4]s");
    }
#elif defined(_WIN32)
    rech_win_dbg_t *s = rech_win_session_find(p.%[2]s);
    if (s != NULL) {
        if (s->has_pending) {
            ContinueDebugEvent(s->pending.dwProcessId, s->pending.dwThreadId, DBG_CONTINUE);
        }
        rech_win_session_remove(p.%[2]s);
    }
    if (!DebugActiveProcessStop((DWORD)p.%[2]s)) {
        rech_panic("%[4]s");
    }
#else
    (void)p;
    rech_panic("%[4]s");
#endif
}

static %[5]s *rech_proc_read(%[1]s p, long long addr, long long size) {
    if (size <= 0) {
        rech_panic("%[6]s");
    }
    unsigned char *buf = malloc((size_t)size);
#if defined(__linux__)
    struct iovec local = {buf, (size_t)size};
    struct iovec remote = {(void *)(intptr_t)addr, (size_t)size};
    ssize_t n = process_vm_readv((pid_t)p.%[2]s, &local, 1, &remote, 1, 0);
    if (n != (ssize_t)size) {
        free(buf);
        rech_panic("%[6]s");
    }
#elif defined(_WIN32)
    HANDLE h = OpenProcess(PROCESS_VM_READ, FALSE, (DWORD)p.%[2]s);
    if (h == NULL) {
        free(buf);
        rech_panic("%[6]s");
    }
    SIZE_T got = 0;
    BOOL ok = ReadProcessMemory(h, (LPCVOID)(intptr_t)addr, buf, (SIZE_T)size, &got);
    CloseHandle(h);
    if (!ok || got != (SIZE_T)size) {
        free(buf);
        rech_panic("%[6]s");
    }
#else
    free(buf);
    rech_panic("%[6]s");
    return NULL;
#endif
    %[5]s *out = rech_bytes_wrap(buf, size);
    free(buf);
    return out;
}

static void rech_proc_write(%[1]s p, long long addr, %[5]s *b) {
#if defined(__linux__)
    struct iovec local = {b->data, (size_t)b->len};
    struct iovec remote = {(void *)(intptr_t)addr, (size_t)b->len};
    ssize_t n = process_vm_writev((pid_t)p.%[2]s, &local, 1, &remote, 1, 0);
    if (n != (ssize_t)b->len) {
        rech_panic("%[7]s");
    }
#elif defined(_WIN32)
    HANDLE h = OpenProcess(PROCESS_VM_WRITE | PROCESS_VM_OPERATION, FALSE, (DWORD)p.%[2]s);
    if (h == NULL) {
        rech_panic("%[7]s");
    }
    SIZE_T written = 0;
    BOOL ok = WriteProcessMemory(h, (LPVOID)(intptr_t)addr, b->data, (SIZE_T)b->len, &written);
    CloseHandle(h);
    if (!ok || written != (SIZE_T)b->len) {
        rech_panic("%[7]s");
    }
#else
    (void)p;
    (void)addr;
    (void)b;
    rech_panic("%[7]s");
#endif
}
`, mangleType("Процесс"), mangleField("pid"),
	escapeCString(rtmsg.ProcAttachFailed), escapeCString(rtmsg.ProcDetachFailed),
	mangleType(byteListType), escapeCString(rtmsg.ProcMemReadFailed), escapeCString(rtmsg.ProcMemWriteFailed))

// procMemFindHelper - "найти_байты(процесс, шаблон): Список<Целое>" -
// сканирует ВСЕ читаемые страницы памяти процесса в поиске байтовой
// сигнатуры "шаблон", возвращает адреса всех вхождений (наивный O(n*m)
// memcmp по каждой странице - тот же класс сложности, что и у настоящих
// "трейнеров"/сканеров памяти, не оптимизация под конкретный процессор).
// Список<Целое> строится через уже автоматически сгенерированный метод
// "добавить" ЭТОЙ ЖЕ generic-инстанциации (mangleMethod(intListType,
// "добавить")) - тем же приёмом, что rech_split_string (stringsplit.go)
// строит Список<Строка>, не отдельная копия логики роста массива.
//
// Регион больше 256 МиБ - пропускается (не читается вовсе) - защита от
// патологически огромных отображений (файлы, смэпленные целиком) без
// отдельного согласования лимита - реальные сигнатуры ищут в куче/данных
// программы, не в мультигигабайтных mmap-файлах.
//
// Linux - разбирает "/proc/<pid>/maps" построчно (fopen/fgets/sscanf -
// формат "начало-конец права ..."), читает только страницы с правом "r"
// через process_vm_readv (см. package doc procMemRuntimeCode выше).
// Недоступный для чтения регион (сбой process_vm_readv) - пропускается,
// не паника (одна нечитаемая страница не должна прерывать весь поиск).
// Windows - VirtualQueryEx перебирает регионы адресного пространства с
// начала (BaseAddress+RegionSize - следующий адрес), читает через
// ReadProcessMemory только MEM_COMMIT-регионы без PAGE_NOACCESS/
// PAGE_GUARD.
func procMemFindHelper() string {
	return fmt.Sprintf(`static %[1]s *rech_proc_find(%[2]s p, %[3]s *needle) {
    if (needle->len <= 0) {
        rech_panic("%[4]s");
    }
    %[1]s *out = GC_MALLOC(sizeof(%[1]s));
#if defined(__linux__)
    char path[64];
    snprintf(path, sizeof(path), "/proc/%%lld/maps", (long long)p.%[5]s);
    FILE *f = fopen(path, "r");
    if (f == NULL) {
        return out;
    }
    char line[512];
    while (fgets(line, sizeof(line), f) != NULL) {
        unsigned long long start = 0, end = 0;
        char perms[8] = {0};
        if (sscanf(line, "%%llx-%%llx %%7s", &start, &end, perms) != 3) {
            continue;
        }
        if (perms[0] != 'r') {
            continue;
        }
        long long size = (long long)(end - start);
        if (size <= 0 || size > (256LL * 1024 * 1024)) {
            continue;
        }
        unsigned char *buf = malloc((size_t)size);
        struct iovec local = {buf, (size_t)size};
        struct iovec remote = {(void *)(intptr_t)start, (size_t)size};
        ssize_t n = process_vm_readv((pid_t)p.%[5]s, &local, 1, &remote, 1, 0);
        if (n == (ssize_t)size) {
            for (long long i = 0; i + needle->len <= size; i++) {
                if (memcmp(buf + i, needle->data, (size_t)needle->len) == 0) {
                    %[6]s(out, (long long)start + i);
                }
            }
        }
        free(buf);
    }
    fclose(f);
#elif defined(_WIN32)
    HANDLE h = OpenProcess(PROCESS_VM_READ | PROCESS_QUERY_INFORMATION, FALSE, (DWORD)p.%[5]s);
    if (h == NULL) {
        return out;
    }
    unsigned char *addr = NULL;
    MEMORY_BASIC_INFORMATION mbi;
    while (VirtualQueryEx(h, addr, &mbi, sizeof(mbi)) == sizeof(mbi)) {
        if (mbi.State == MEM_COMMIT && (mbi.Protect & (PAGE_NOACCESS | PAGE_GUARD)) == 0) {
            long long size = (long long)mbi.RegionSize;
            if (size > 0 && size <= (256LL * 1024 * 1024)) {
                unsigned char *buf = malloc((size_t)size);
                SIZE_T got = 0;
                if (ReadProcessMemory(h, mbi.BaseAddress, buf, (SIZE_T)size, &got) && (long long)got == size) {
                    for (long long i = 0; i + needle->len <= size; i++) {
                        if (memcmp(buf + i, needle->data, (size_t)needle->len) == 0) {
                            %[6]s(out, (long long)(intptr_t)mbi.BaseAddress + i);
                        }
                    }
                }
                free(buf);
            }
        }
        unsigned char *next = (unsigned char *)mbi.BaseAddress + mbi.RegionSize;
        if (next <= addr) {
            break;
        }
        addr = next;
    }
    CloseHandle(h);
#endif
    return out;
}
`, mangleType(intListType), mangleType("Процесс"), mangleType(byteListType),
		escapeCString(rtmsg.ProcMemPatternEmpty), mangleField("pid"), mangleMethod(intListType, "добавить"))
}
