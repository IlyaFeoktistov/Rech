package subprocess

import "fmt"

// RuntimeCode - "запустить_процесс"/"дождаться_процесса"/
// "выполнить_команду" (internal/semantic/subprocess.go, package doc там) -
// три функции работы с внешними процессами:
//
//   - rech_spawn_process - неблокирующий spawn БЕЗ системной оболочки.
//     POSIX: fork()+execvp(). Ошибку самого execvp (программа не найдена)
//     родитель узнаёт СИНХРОННО, тем же "самопайпом", что использует
//     os/exec в стандартной библиотеке Go (internal/interp/subprocess.go
//     получает ту же ошибку от Start() бесплатно) - пайп с FD_CLOEXEC:
//     потомок пишет errno в него, только если execvp() вернул управление
//     (то есть провалился) - при УСПЕШНОМ execvp пайп закрывается сам,
//     заменой всего образа процесса, родитель тогда читает EOF (0 байт) и
//     не паникует. Без этого трюка родитель либо не узнал бы об ошибке
//     вовсе, либо узнал бы только из "дождаться_процесса" (код 127) -
//     хуже (запустить_процесс валилась бы МОЛЧА, ошибка проявилась бы
//     только у другого builtin'а). Windows: CreateProcessA - argv
//     потомка собирается в одну command line-строку (Win32 не знает
//     execvp-стиль массив аргументов), каждый аргумент в кавычках -
//     минимально достаточно для обычных аргументов без вложенных кавычек,
//     не полный алгоритм CommandLineToArgvW.
//
//   - rech_wait_process - блокируется до завершения потомка. POSIX -
//     waitpid(pid, ...) по pid напрямую (это прямой потомок ЭТОГО
//     процесса - fork() гарантирует это, отдельный сохранённый дескриптор
//     не нужен). Код возврата - WEXITSTATUS при обычном завершении,
//     128+WTERMSIG при завершении сигналом (соглашение обычной POSIX-
//     оболочки, $? после "kill"). Windows - OpenProcess по pid заново
//     (Процесс несёт только pid, не HANDLE - тот же принцип, что и
//     rech_send_signal, runtime/signals), WaitForSingleObject+
//     GetExitCodeProcess.
//
//   - rech_run_command - блокирующий запуск ЧЕРЕЗ системную оболочку
//     (sh -c/cmd /C), вывод и ошибка ПЕРЕХВАТЫВАЮТСЯ раздельно (не
//     наследуют стандартные потоки самой программы, в отличие от
//     rech_spawn_process). POSIX - два пайпа (stdout/stderr), poll() в
//     цикле читает оба одновременно до закрытия обоих (раздельное
//     последовательное чтение до EOF одного, потом другого - классический
//     дедлок, если потомок пишет в ОБА одновременно больше, чем влезает в
//     буфер ядра одного из них, пока родитель ещё занят другим) - тот же
//     poll(), что уже используют rech_can_read (runtime/console,
//     usesNonBlockingRead), но здесь эмитится безусловно вместе с
//     usesSubprocess, не завязан на тот флаг. Windows - тот же дедлок
//     возможен теоретически (два анонимных пайпа, ReadFile блокирует), но
//     у Windows нет одного эквивалента POSIX poll() для анонимных пайпов -
//     решается отдельным потоком (CreateThread), который дочитывает
//     stdout, пока главный поток читает stderr - тот же принцип, что и
//     usesThreads (concurrency.go), самостоятельный поток без пула.
//
// procType/splitResultC/pidField/resultType/outputField/errorField/
// exitCodeField/spawnFailedMsg/waitFailedMsg/runFailedMsg - уже вычислены
// вызывающей стороной (mangleType/mangleField/escapeCString - внутренняя,
// неэкспортированная инфраструктура cbackend, этот пакет от неё не
// зависит).
func RuntimeCode(procType, splitResultC, pidField, resultType, outputField, errorField, exitCodeField,
	spawnFailedMsg, waitFailedMsg, runFailedMsg string) string {
	return fmt.Sprintf(`static %[1]s rech_spawn_process(const char *cmd, %[2]s *args) {
#ifndef _WIN32
    int errpipe[2];
    if (pipe(errpipe) != 0) {
        rech_panic("%[8]s");
    }
    fcntl(errpipe[1], F_SETFD, FD_CLOEXEC);
    pid_t pid = fork();
    if (pid < 0) {
        close(errpipe[0]);
        close(errpipe[1]);
        rech_panic("%[8]s");
    }
    if (pid == 0) {
        close(errpipe[0]);
        long long argc = args->len;
        char **argv = malloc(sizeof(char *) * (size_t)(argc + 2));
        argv[0] = (char *)cmd;
        for (long long i = 0; i < argc; i++) {
            argv[i + 1] = args->data[i];
        }
        argv[argc + 1] = NULL;
        execvp(cmd, argv);
        int child_errno = errno;
        ssize_t written = write(errpipe[1], &child_errno, sizeof(child_errno));
        (void)written;
        _exit(127);
    }
    close(errpipe[1]);
    int child_errno = 0;
    ssize_t n = read(errpipe[0], &child_errno, sizeof(child_errno));
    close(errpipe[0]);
    if (n > 0) {
        rech_panic("%[8]s");
    }
    %[1]s out;
    out.%[3]s = (long long)pid;
    return out;
#else
    STARTUPINFOA si;
    PROCESS_INFORMATION pi;
    ZeroMemory(&si, sizeof(si));
    si.cb = sizeof(si);
    ZeroMemory(&pi, sizeof(pi));

    size_t cmdline_len = strlen(cmd) + 3;
    for (long long i = 0; i < args->len; i++) {
        cmdline_len += strlen(args->data[i]) + 3;
    }
    char *cmdline = malloc(cmdline_len);
    cmdline[0] = '\0';
    strcat(cmdline, "\"");
    strcat(cmdline, cmd);
    strcat(cmdline, "\"");
    for (long long i = 0; i < args->len; i++) {
        strcat(cmdline, " \"");
        strcat(cmdline, args->data[i]);
        strcat(cmdline, "\"");
    }
    if (!CreateProcessA(NULL, cmdline, NULL, NULL, FALSE, 0, NULL, NULL, &si, &pi)) {
        rech_panic("%[8]s");
    }
    CloseHandle(pi.hThread);
    CloseHandle(pi.hProcess);
    %[1]s out;
    out.%[3]s = (long long)pi.dwProcessId;
    return out;
#endif
}

static long long rech_wait_process(%[1]s p) {
#ifndef _WIN32
    int status;
    if (waitpid((pid_t)p.%[3]s, &status, 0) < 0) {
        rech_panic("%[9]s");
    }
    if (WIFEXITED(status)) {
        return (long long)WEXITSTATUS(status);
    }
    if (WIFSIGNALED(status)) {
        return (long long)(128 + WTERMSIG(status));
    }
    return -1;
#else
    HANDLE h = OpenProcess(SYNCHRONIZE | PROCESS_QUERY_INFORMATION, FALSE, (DWORD)p.%[3]s);
    if (h == NULL) {
        rech_panic("%[9]s");
    }
    if (WaitForSingleObject(h, INFINITE) != WAIT_OBJECT_0) {
        CloseHandle(h);
        rech_panic("%[9]s");
    }
    DWORD code = 0;
    if (!GetExitCodeProcess(h, &code)) {
        CloseHandle(h);
        rech_panic("%[9]s");
    }
    CloseHandle(h);
    return (long long)code;
#endif
}

#ifdef _WIN32
typedef struct {
    HANDLE h;
    char *result;
} rech_pipe_reader_arg;

static DWORD WINAPI rech_read_pipe_thread(LPVOID arg) {
    rech_pipe_reader_arg *a = (rech_pipe_reader_arg *)arg;
    size_t cap = 4096, len = 0;
    char *buf = malloc(cap);
    for (;;) {
        if (len + 1 >= cap) {
            cap *= 2;
            buf = realloc(buf, cap);
        }
        DWORD n = 0;
        BOOL ok = ReadFile(a->h, buf + len, (DWORD)(cap - len - 1), &n, NULL);
        if (!ok || n == 0) {
            break;
        }
        len += (size_t)n;
    }
    a->result = rech_text_take(buf, len);
    return 0;
}
#endif

static %[4]s rech_run_command(const char *cmd, const char *dir) {
#ifndef _WIN32
    int out_pipe[2], err_pipe[2];
    if (pipe(out_pipe) != 0 || pipe(err_pipe) != 0) {
        rech_panic("%[10]s");
    }
    pid_t pid = fork();
    if (pid < 0) {
        rech_panic("%[10]s");
    }
    if (pid == 0) {
        close(out_pipe[0]);
        close(err_pipe[0]);
        dup2(out_pipe[1], STDOUT_FILENO);
        dup2(err_pipe[1], STDERR_FILENO);
        close(out_pipe[1]);
        close(err_pipe[1]);
        if (dir[0] != '\0' && chdir(dir) != 0) {
            _exit(127);
        }
        execl("/bin/sh", "sh", "-c", cmd, (char *)NULL);
        _exit(127);
    }
    close(out_pipe[1]);
    close(err_pipe[1]);

    char *out_buf = malloc(1);
    out_buf[0] = '\0';
    char *err_buf = malloc(1);
    err_buf[0] = '\0';
    size_t out_len = 0, out_cap = 1, err_len = 0, err_cap = 1;
    int out_fd = out_pipe[0], err_fd = err_pipe[0];
    int out_open = 1, err_open = 1;
    struct pollfd fds[2];
    while (out_open || err_open) {
        int nfds = 0;
        int out_idx = -1, err_idx = -1;
        if (out_open) {
            fds[nfds].fd = out_fd;
            fds[nfds].events = POLLIN;
            out_idx = nfds++;
        }
        if (err_open) {
            fds[nfds].fd = err_fd;
            fds[nfds].events = POLLIN;
            err_idx = nfds++;
        }
        if (poll(fds, (nfds_t)nfds, -1) < 0) {
            break;
        }
        char chunk[4096];
        if (out_idx >= 0 && (fds[out_idx].revents & (POLLIN | POLLHUP | POLLERR))) {
            ssize_t n = read(out_fd, chunk, sizeof(chunk));
            if (n <= 0) {
                close(out_fd);
                out_open = 0;
            } else {
                if (out_len + (size_t)n + 1 > out_cap) {
                    while (out_len + (size_t)n + 1 > out_cap) {
                        out_cap *= 2;
                    }
                    out_buf = realloc(out_buf, out_cap);
                }
                memcpy(out_buf + out_len, chunk, (size_t)n);
                out_len += (size_t)n;
                out_buf[out_len] = '\0';
            }
        }
        if (err_idx >= 0 && (fds[err_idx].revents & (POLLIN | POLLHUP | POLLERR))) {
            ssize_t n = read(err_fd, chunk, sizeof(chunk));
            if (n <= 0) {
                close(err_fd);
                err_open = 0;
            } else {
                if (err_len + (size_t)n + 1 > err_cap) {
                    while (err_len + (size_t)n + 1 > err_cap) {
                        err_cap *= 2;
                    }
                    err_buf = realloc(err_buf, err_cap);
                }
                memcpy(err_buf + err_len, chunk, (size_t)n);
                err_len += (size_t)n;
                err_buf[err_len] = '\0';
            }
        }
    }
    int status;
    waitpid(pid, &status, 0);
    long long code = WIFEXITED(status) ? (long long)WEXITSTATUS(status)
                    : WIFSIGNALED(status) ? (long long)(128 + WTERMSIG(status)) : -1;
    %[4]s result;
    result.%[5]s = rech_text_take(out_buf, out_len);
    result.%[6]s = rech_text_take(err_buf, err_len);
    result.%[7]s = code;
    return result;
#else
    SECURITY_ATTRIBUTES sa;
    ZeroMemory(&sa, sizeof(sa));
    sa.nLength = sizeof(sa);
    sa.bInheritHandle = TRUE;

    HANDLE out_read, out_write, err_read, err_write;
    if (!CreatePipe(&out_read, &out_write, &sa, 0) || !CreatePipe(&err_read, &err_write, &sa, 0)) {
        rech_panic("%[10]s");
    }
    SetHandleInformation(out_read, HANDLE_FLAG_INHERIT, 0);
    SetHandleInformation(err_read, HANDLE_FLAG_INHERIT, 0);

    STARTUPINFOA si;
    PROCESS_INFORMATION pi;
    ZeroMemory(&si, sizeof(si));
    si.cb = sizeof(si);
    si.dwFlags |= STARTF_USESTDHANDLES;
    si.hStdOutput = out_write;
    si.hStdError = err_write;
    si.hStdInput = GetStdHandle(STD_INPUT_HANDLE);
    ZeroMemory(&pi, sizeof(pi));

    size_t cmdline_len = strlen(cmd) + 16;
    char *cmdline = malloc(cmdline_len);
    snprintf(cmdline, cmdline_len, "cmd /C %%s", cmd);

    BOOL started = CreateProcessA(NULL, cmdline, NULL, NULL, TRUE, 0, NULL,
        dir[0] != '\0' ? dir : NULL, &si, &pi);
    CloseHandle(out_write);
    CloseHandle(err_write);
    if (!started) {
        CloseHandle(out_read);
        CloseHandle(err_read);
        rech_panic("%[10]s");
    }

    rech_pipe_reader_arg out_arg = {out_read, NULL};
    HANDLE reader = CreateThread(NULL, 0, rech_read_pipe_thread, &out_arg, 0, NULL);
    rech_pipe_reader_arg err_arg = {err_read, NULL};
    rech_read_pipe_thread(&err_arg);
    WaitForSingleObject(reader, INFINITE);
    CloseHandle(reader);
    CloseHandle(out_read);
    CloseHandle(err_read);

    WaitForSingleObject(pi.hProcess, INFINITE);
    DWORD code = 0;
    GetExitCodeProcess(pi.hProcess, &code);
    CloseHandle(pi.hProcess);
    CloseHandle(pi.hThread);

    %[4]s result;
    result.%[5]s = out_arg.result;
    result.%[6]s = err_arg.result;
    result.%[7]s = (long long)code;
    return result;
#endif
}
`, procType, splitResultC, pidField,
		resultType, outputField, errorField, exitCodeField,
		spawnFailedMsg, waitFailedMsg, runFailedMsg)
}
