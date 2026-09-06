package cbackend

// consoleRuntimeCode - "установить_сырой_режим(Логическое)"/
// "ширина_терминала()"/"высота_терминала()" (docs/SPEC.md) - минимум,
// нужный полноэкранному консольному интерфейсу (TUI), которого не сделать
// чистым Rech-кодом (в отличие от курсора/цвета - те просто печатают ANSI-
// последовательности в stdout, обычные строки, без единого системного
// вызова): сырой режим терминала требует termios (POSIX)/режима консоли
// (Windows), размер терминала - ioctl(TIOCGWINSZ)/GetConsoleScreenBufferInfo.
//
// Сырой режим - отключает canonical mode (посимвольное чтение вместо
// построчного, до Enter) и echo (символ не печатается сам, программа сама
// решает, что показать) - "прочитать(Файл, Целое)" на "стандартный_ввод()"
// уже читает побайтово, не хватало только этого переключения. Исходный
// режим терминала запоминается ОДИН раз, при первом включении - "выключить"
// возвращает его как было, а не жёстко закодированные настройки (иначе
// программа, запущенная из чужого окружения с уже нестандартным термиос,
// испортила бы терминал вызывающей стороны после выхода).
//
// #ifdef _WIN32 - другой набор API (SetConsoleMode вместо termios,
// GetConsoleScreenBufferInfo вместо ioctl) - тот же принцип, что и у
// сетевого кода (winsock2 вместо BSD sockets).
func consoleRuntimeCode() string {
	return `#ifndef _WIN32
static struct termios rech_orig_termios;
static bool rech_raw_mode_saved = false;

static void rech_set_raw_mode(bool enable) {
    if (enable) {
        if (!rech_raw_mode_saved) {
            tcgetattr(STDIN_FILENO, &rech_orig_termios);
            rech_raw_mode_saved = true;
        }
        struct termios raw = rech_orig_termios;
        raw.c_lflag &= (tcflag_t)~(ECHO | ICANON);
        raw.c_cc[VMIN] = 1;
        raw.c_cc[VTIME] = 0;
        tcsetattr(STDIN_FILENO, TCSAFLUSH, &raw);
    } else if (rech_raw_mode_saved) {
        tcsetattr(STDIN_FILENO, TCSAFLUSH, &rech_orig_termios);
    }
}

static long long rech_terminal_width(void) {
    struct winsize w;
    if (ioctl(STDOUT_FILENO, TIOCGWINSZ, &w) == 0 && w.ws_col > 0) {
        return w.ws_col;
    }
    return 80;
}

static long long rech_terminal_height(void) {
    struct winsize w;
    if (ioctl(STDOUT_FILENO, TIOCGWINSZ, &w) == 0 && w.ws_row > 0) {
        return w.ws_row;
    }
    return 24;
}
#else
static DWORD rech_orig_console_mode;
static bool rech_console_mode_saved = false;

static void rech_set_raw_mode(bool enable) {
    HANDLE h = GetStdHandle(STD_INPUT_HANDLE);
    if (enable) {
        if (!rech_console_mode_saved) {
            GetConsoleMode(h, &rech_orig_console_mode);
            rech_console_mode_saved = true;
        }
        SetConsoleMode(h, rech_orig_console_mode & ~(DWORD)(ENABLE_LINE_INPUT | ENABLE_ECHO_INPUT | ENABLE_PROCESSED_INPUT));
    } else if (rech_console_mode_saved) {
        SetConsoleMode(h, rech_orig_console_mode);
    }
}

static long long rech_terminal_width(void) {
    CONSOLE_SCREEN_BUFFER_INFO info;
    if (GetConsoleScreenBufferInfo(GetStdHandle(STD_OUTPUT_HANDLE), &info)) {
        return info.srWindow.Right - info.srWindow.Left + 1;
    }
    return 80;
}

static long long rech_terminal_height(void) {
    CONSOLE_SCREEN_BUFFER_INFO info;
    if (GetConsoleScreenBufferInfo(GetStdHandle(STD_OUTPUT_HANDLE), &info)) {
        return info.srWindow.Bottom - info.srWindow.Top + 1;
    }
    return 24;
}
#endif

`
}
