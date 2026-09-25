package signals

// RuntimeCode - "получен_сигнал(сигнал: СигналОС): Логическое" (docs/
// SPEC.md) - обобщение rech_shutdown_requested (shutdown.go) на ЛЮБОЙ
// именованный POSIX-сигнал, тем же принципом (volatile sig_atomic_t на
// сигнал, взводится один раз, не сбрасывается). Регистрация - через
// sigaction (не signal() - современный POSIX-интерфейс, sa_mask/sa_flags
// явно обнулены compound-литералом, а не отдельным memset), лениво, при
// каждом вызове rech_got_signal (идемпотентно - повторная регистрация того
// же обработчика ничего не меняет). ordinal - НЕ настоящий номер сигнала
// ОС (см. package doc internal/semantic/types.go, StandardEnums
// "СигналОС") - индекс, который здесь же переводится в реальное имя
// C-макроса через switch (единственное место всей программы, которое
// обязано знать это соответствие - интерпретатор держит свою копию,
// internal/interp/signals.go, синхронизировать оба при изменении списка).
// SIGKILL/SIGSTOP (ordinal 4/12) - паника (rtmsg.SignalUncatchable) вместо
// регистрации - POSIX не даёт перехватить ни один из двух ни при каких
// условиях, попытка sigaction на них либо не имеет эффекта, либо завершается
// ошибкой в зависимости от платформы - явный отказ понятнее тихого
// молчания.
//
// SIGILL/SIGTRAP/SIGBUS/SIGFPE/SIGSEGV/SIGSYS (ordinal 15-19, 30) - сигналы
// аппаратных ошибок, не событий "снаружи" - зарегистрированы тем же
// sigaction, но возврат из обработчика возобновляет выполнение прямо с
// ошибочной инструкции (обычно немедленный повторный сигнал) - надёжно
// ловятся, только если ПОСЛАНЫ другим процессом (см. "отправить_сигнал"),
// не когда вызваны настоящей аппаратной ошибкой самой программы.
//
// SIGSTKFLT/SIGPWR (ordinal 20/29) - дополнительно обёрнуты в "#ifdef
// __linux__" внутри POSIX-ветки (не просто "#ifndef _WIN32") - живой
// найденный баг: это расширения Linux/glibc, на macOS (BSD-сигналы) этих
// символов не существует вовсе - без этой границы файл не собирался на
// macOS (см. internal/interp/signals_darwin.go - там оба ordinal'а
// помечены sigUnsupported тем же принципом).
//
// #ifdef _WIN32 - sigaction недоступен вовсе (не POSIX API); MinGW CRT
// определяет лишь малую часть перечисленных здесь имён сигналов (SIGINT/
// SIGTERM/SIGABRT - остальные POSIX-специфичны, SIGHUP/SIGQUIT/SIGUSR1/
// SIGUSR2/SIGPIPE/SIGALRM/SIGCHLD/SIGCONT/SIGSTOP/SIGTSTP/SIGWINCH на
// Windows не существуют) - урезанная ветка на старом signal() для трёх
// портируемых сигналов, для остальных ordinal - флаг всегда ложь (сигнал
// на этой платформе физически не может прийти). uncatchableMsg/
// sendFailedMsg - уже экранированные (escapeCString - внутренняя,
// неэкспортированная инфраструктура cbackend, этот пакет от неё не
// зависит) сообщения об ошибке.
func RuntimeCode(uncatchableMsg, sendFailedMsg string) string {
	return `static volatile sig_atomic_t rech_sig_flags[31];

static void rech_sig_handler(int sig) {
#ifndef _WIN32
    switch (sig) {
    case SIGHUP: rech_sig_flags[0] = 1; break;
    case SIGINT: rech_sig_flags[1] = 1; break;
    case SIGQUIT: rech_sig_flags[2] = 1; break;
    case SIGABRT: rech_sig_flags[3] = 1; break;
    case SIGUSR1: rech_sig_flags[5] = 1; break;
    case SIGUSR2: rech_sig_flags[6] = 1; break;
    case SIGPIPE: rech_sig_flags[7] = 1; break;
    case SIGALRM: rech_sig_flags[8] = 1; break;
    case SIGTERM: rech_sig_flags[9] = 1; break;
    case SIGCHLD: rech_sig_flags[10] = 1; break;
    case SIGCONT: rech_sig_flags[11] = 1; break;
    case SIGTSTP: rech_sig_flags[13] = 1; break;
    case SIGWINCH: rech_sig_flags[14] = 1; break;
    case SIGILL: rech_sig_flags[15] = 1; break;
    case SIGTRAP: rech_sig_flags[16] = 1; break;
    case SIGBUS: rech_sig_flags[17] = 1; break;
    case SIGFPE: rech_sig_flags[18] = 1; break;
    case SIGSEGV: rech_sig_flags[19] = 1; break;
#ifdef __linux__
    case SIGSTKFLT: rech_sig_flags[20] = 1; break;
#endif
    case SIGTTIN: rech_sig_flags[21] = 1; break;
    case SIGTTOU: rech_sig_flags[22] = 1; break;
    case SIGURG: rech_sig_flags[23] = 1; break;
    case SIGXCPU: rech_sig_flags[24] = 1; break;
    case SIGXFSZ: rech_sig_flags[25] = 1; break;
    case SIGVTALRM: rech_sig_flags[26] = 1; break;
    case SIGPROF: rech_sig_flags[27] = 1; break;
    case SIGIO: rech_sig_flags[28] = 1; break;
#ifdef __linux__
    case SIGPWR: rech_sig_flags[29] = 1; break;
#endif
    case SIGSYS: rech_sig_flags[30] = 1; break;
    default: break;
    }
#else
    switch (sig) {
    case SIGINT: rech_sig_flags[1] = 1; break;
    case SIGABRT: rech_sig_flags[3] = 1; break;
    case SIGTERM: rech_sig_flags[9] = 1; break;
    default: break;
    }
#endif
}

static bool rech_got_signal(long long ordinal) {
    if (ordinal == 4 || ordinal == 12) {
        rech_panic("` + uncatchableMsg + `");
    }
    if (ordinal < 0 || ordinal >= 31) {
        return false;
    }
#ifndef _WIN32
    struct sigaction sa = {0};
    sa.sa_handler = rech_sig_handler;
    sigemptyset(&sa.sa_mask);
    switch (ordinal) {
    case 0: sigaction(SIGHUP, &sa, NULL); break;
    case 1: sigaction(SIGINT, &sa, NULL); break;
    case 2: sigaction(SIGQUIT, &sa, NULL); break;
    case 3: sigaction(SIGABRT, &sa, NULL); break;
    case 5: sigaction(SIGUSR1, &sa, NULL); break;
    case 6: sigaction(SIGUSR2, &sa, NULL); break;
    case 7: sigaction(SIGPIPE, &sa, NULL); break;
    case 8: sigaction(SIGALRM, &sa, NULL); break;
    case 9: sigaction(SIGTERM, &sa, NULL); break;
    case 10: sigaction(SIGCHLD, &sa, NULL); break;
    case 11: sigaction(SIGCONT, &sa, NULL); break;
    case 13: sigaction(SIGTSTP, &sa, NULL); break;
    case 14: sigaction(SIGWINCH, &sa, NULL); break;
    case 15: sigaction(SIGILL, &sa, NULL); break;
    case 16: sigaction(SIGTRAP, &sa, NULL); break;
    case 17: sigaction(SIGBUS, &sa, NULL); break;
    case 18: sigaction(SIGFPE, &sa, NULL); break;
    case 19: sigaction(SIGSEGV, &sa, NULL); break;
#ifdef __linux__
    case 20: sigaction(SIGSTKFLT, &sa, NULL); break;
#endif
    case 21: sigaction(SIGTTIN, &sa, NULL); break;
    case 22: sigaction(SIGTTOU, &sa, NULL); break;
    case 23: sigaction(SIGURG, &sa, NULL); break;
    case 24: sigaction(SIGXCPU, &sa, NULL); break;
    case 25: sigaction(SIGXFSZ, &sa, NULL); break;
    case 26: sigaction(SIGVTALRM, &sa, NULL); break;
    case 27: sigaction(SIGPROF, &sa, NULL); break;
    case 28: sigaction(SIGIO, &sa, NULL); break;
#ifdef __linux__
    case 29: sigaction(SIGPWR, &sa, NULL); break;
#endif
    case 30: sigaction(SIGSYS, &sa, NULL); break;
    default: break;
    }
#else
    switch (ordinal) {
    case 1: signal(SIGINT, rech_sig_handler); break;
    case 3: signal(SIGABRT, rech_sig_handler); break;
    case 9: signal(SIGTERM, rech_sig_handler); break;
    default: return false;
    }
#endif
    return rech_sig_flags[ordinal] != 0;
}

static void rech_send_signal(long long pid, long long ordinal) {
#ifndef _WIN32
    int sig;
    switch (ordinal) {
    case 0: sig = SIGHUP; break;
    case 1: sig = SIGINT; break;
    case 2: sig = SIGQUIT; break;
    case 3: sig = SIGABRT; break;
    case 4: sig = SIGKILL; break;
    case 5: sig = SIGUSR1; break;
    case 6: sig = SIGUSR2; break;
    case 7: sig = SIGPIPE; break;
    case 8: sig = SIGALRM; break;
    case 9: sig = SIGTERM; break;
    case 10: sig = SIGCHLD; break;
    case 11: sig = SIGCONT; break;
    case 12: sig = SIGSTOP; break;
    case 13: sig = SIGTSTP; break;
    case 14: sig = SIGWINCH; break;
    case 15: sig = SIGILL; break;
    case 16: sig = SIGTRAP; break;
    case 17: sig = SIGBUS; break;
    case 18: sig = SIGFPE; break;
    case 19: sig = SIGSEGV; break;
#ifdef __linux__
    case 20: sig = SIGSTKFLT; break;
#endif
    case 21: sig = SIGTTIN; break;
    case 22: sig = SIGTTOU; break;
    case 23: sig = SIGURG; break;
    case 24: sig = SIGXCPU; break;
    case 25: sig = SIGXFSZ; break;
    case 26: sig = SIGVTALRM; break;
    case 27: sig = SIGPROF; break;
    case 28: sig = SIGIO; break;
#ifdef __linux__
    case 29: sig = SIGPWR; break;
#endif
    case 30: sig = SIGSYS; break;
    default: rech_error_as(RECH_ERR_PROCESS, "` + sendFailedMsg + `");
    }
    if (kill((pid_t)pid, sig) != 0) {
        rech_error_as(RECH_ERR_PROCESS, "` + sendFailedMsg + `");
    }
#else
    /* Windows не знает POSIX-сигналов между процессами - лучшее доступное
       приближение: SIGINT через GenerateConsoleCtrlEvent (тот же механизм,
       что и Ctrl+C в реальном терминале, работает только с процессами
       собственной консольной группы), SIGKILL/SIGTERM - через
       TerminateProcess (безусловное завершение, ближайший аналог "убить
       процесс" на этой платформе). Остальные ordinal - ОшибкаПроцесса, POSIX-
       сигнал без аналога послать некому. */
    if (ordinal == 1) {
        if (!GenerateConsoleCtrlEvent(CTRL_C_EVENT, (DWORD)pid)) {
            rech_error_as(RECH_ERR_PROCESS, "` + sendFailedMsg + `");
        }
        return;
    }
    if (ordinal == 4 || ordinal == 9) {
        HANDLE h = OpenProcess(PROCESS_TERMINATE, FALSE, (DWORD)pid);
        if (h == NULL || !TerminateProcess(h, 1)) {
            if (h != NULL) CloseHandle(h);
            rech_error_as(RECH_ERR_PROCESS, "` + sendFailedMsg + `");
        }
        CloseHandle(h);
        return;
    }
    rech_error_as(RECH_ERR_PROCESS, "` + sendFailedMsg + `");
#endif
}

`
}
