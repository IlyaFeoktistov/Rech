package cbackend

import "github.com/IlyaFeoktistov/Rech/internal/rtmsg"

// signalRuntimeCode - "получен_сигнал(сигнал: СигналОС): Логическое"
// (docs/SPEC.md) - обобщение rech_shutdown_requested (shutdown.go) на ЛЮБОЙ
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
// #ifdef _WIN32 - sigaction недоступен вовсе (не POSIX API); MinGW CRT
// определяет лишь малую часть перечисленных здесь имён сигналов (SIGINT/
// SIGTERM/SIGABRT - остальные POSIX-специфичны, SIGHUP/SIGQUIT/SIGUSR1/
// SIGUSR2/SIGPIPE/SIGALRM/SIGCHLD/SIGCONT/SIGSTOP/SIGTSTP/SIGWINCH на
// Windows не существуют) - урезанная ветка на старом signal() для трёх
// портируемых сигналов, для остальных ordinal - флаг всегда ложь (сигнал
// на этой платформе физически не может прийти).
func signalRuntimeCode() string {
	return `static volatile sig_atomic_t rech_sig_flags[15];

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
        rech_panic("` + escapeCString(rtmsg.SignalUncatchable) + `");
    }
    if (ordinal < 0 || ordinal >= 15) {
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

`
}
