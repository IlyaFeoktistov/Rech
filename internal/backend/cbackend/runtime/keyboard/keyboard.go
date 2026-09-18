package keyboard

// RuntimeCode - "прочитать_код_клавиши(миллисекунды: Целое): Целое"
// (docs/SPEC.md, StandardEnums["КодКлавиши"] - internal/semantic/types.go) -
// физический код нажатой клавиши, НЕ зависящий от раскладки, в отличие от
// байтов, которые видит терминал через "прочитать(стандартный_ввод(), 1)"
// (те - уже ПОСЛЕ применения раскладки, см. package doc StandardEnums
// ["КодКлавиши"] и examples/проекты/змейка/исходники/ввод/клавиатурный_
// ввод.rech).
//
// Linux (#ifndef _WIN32) - /dev/input/eventN (evdev): единственный способ
// получить KEY_*-код ядра, независимый от раскладки, минуя весь слой
// X11/Wayland/tty, который её применяет. Устройство ищется один раз за
// жизнь процесса (rech_keyboard_fd_tried) - перебором /dev/input/event* с
// проверкой EVIOCGBIT(EV_KEY, ...) на бит KEY_A (power-button/consumer-
// control устройства тоже видны там, но не поддерживают алфавитные
// клавиши). Не находит ни одного (обычная ситуация без прав root/группы
// "input", ВСЕГДА так внутри WSL2/через SSH без доп. настройки) -
// rech_keyboard_fd остаётся -1 навсегда, "прочитать_код_клавиши" просто
// всегда возвращает -1, без паники.
//
// Windows (#else) - ReadConsoleInputW/wVirtualKeyCode: VK_A..VK_Z/VK_0..
// VK_9 у стандартных layout-DLL совпадают с ASCII-кодом буквы/цифры
// НЕЗАВИСИМО от текущей раскладки (та же причина, по которой
// GetAsyncKeyState(VK_W) в играх работает на любой раскладке) - не
// требует прав администратора, работает через обычный консольный
// хэндл stdin.
//
// macOS - не реализовано (отдельная задача, требует IOKit HID/
// NSEvent.keyCode) - собирается в ветке "#ifndef _WIN32" тем же кодом,
// что и Linux, но rech_open_keyboard там никогда не находит /dev/input
// (его попросту нет) - "прочитать_код_клавиши" возвращает -1, тем же
// путём, что и Linux без прав.
func RuntimeCode() string {
	return `#ifndef _WIN32
static int rech_keyboard_fd = -1;
static bool rech_keyboard_fd_tried = false;

static bool rech_evdev_is_keyboard(int fd) {
    unsigned char bits[KEY_MAX / 8 + 1];
    memset(bits, 0, sizeof(bits));
    if (ioctl(fd, EVIOCGBIT(EV_KEY, sizeof(bits)), bits) < 0) {
        return false;
    }
    return (bits[KEY_A / 8] & (1 << (KEY_A % 8))) != 0;
}

static void rech_open_keyboard(void) {
    rech_keyboard_fd_tried = true;
    DIR *dir = opendir("/dev/input");
    if (!dir) {
        return;
    }
    struct dirent *entry;
    while ((entry = readdir(dir)) != NULL) {
        if (strncmp(entry->d_name, "event", 5) != 0) {
            continue;
        }
        char path[320];
        snprintf(path, sizeof(path), "/dev/input/%s", entry->d_name);
        int fd = open(path, O_RDONLY | O_NONBLOCK);
        if (fd < 0) {
            continue;
        }
        if (rech_evdev_is_keyboard(fd)) {
            rech_keyboard_fd = fd;
            closedir(dir);
            return;
        }
        close(fd);
    }
    closedir(dir);
}

static long long rech_evdev_ordinal(unsigned short code) {
    static long long table[KEY_MAX + 1];
    static bool ready = false;
    if (!ready) {
        for (int i = 0; i <= KEY_MAX; i++) {
            table[i] = -1;
        }
        table[KEY_A] = 0; table[KEY_B] = 1; table[KEY_C] = 2; table[KEY_D] = 3;
        table[KEY_E] = 4; table[KEY_F] = 5; table[KEY_G] = 6; table[KEY_H] = 7;
        table[KEY_I] = 8; table[KEY_J] = 9; table[KEY_K] = 10; table[KEY_L] = 11;
        table[KEY_M] = 12; table[KEY_N] = 13; table[KEY_O] = 14; table[KEY_P] = 15;
        table[KEY_Q] = 16; table[KEY_R] = 17; table[KEY_S] = 18; table[KEY_T] = 19;
        table[KEY_U] = 20; table[KEY_V] = 21; table[KEY_W] = 22; table[KEY_X] = 23;
        table[KEY_Y] = 24; table[KEY_Z] = 25;
        table[KEY_0] = 26; table[KEY_1] = 27; table[KEY_2] = 28; table[KEY_3] = 29;
        table[KEY_4] = 30; table[KEY_5] = 31; table[KEY_6] = 32; table[KEY_7] = 33;
        table[KEY_8] = 34; table[KEY_9] = 35;
        table[KEY_UP] = 36; table[KEY_DOWN] = 37; table[KEY_LEFT] = 38; table[KEY_RIGHT] = 39;
        table[KEY_SPACE] = 40; table[KEY_ENTER] = 41; table[KEY_ESC] = 42; table[KEY_TAB] = 43;
        table[KEY_BACKSPACE] = 44;
        ready = true;
    }
    if (code > KEY_MAX) {
        return -1;
    }
    return table[code];
}

static long long rech_read_key_code(long long timeout_ms) {
    if (!rech_keyboard_fd_tried) {
        rech_open_keyboard();
    }
    if (rech_keyboard_fd < 0) {
        return -1;
    }

    bool has_deadline = timeout_ms >= 0;
    struct timespec deadline;
    if (has_deadline) {
        clock_gettime(CLOCK_MONOTONIC, &deadline);
        deadline.tv_sec += timeout_ms / 1000;
        deadline.tv_nsec += (timeout_ms % 1000) * 1000000LL;
        if (deadline.tv_nsec >= 1000000000LL) {
            deadline.tv_sec += 1;
            deadline.tv_nsec -= 1000000000LL;
        }
    }

    for (;;) {
        int remaining_ms = -1;
        if (has_deadline) {
            struct timespec now;
            clock_gettime(CLOCK_MONOTONIC, &now);
            long long remaining = (long long)(deadline.tv_sec - now.tv_sec) * 1000 +
                (deadline.tv_nsec - now.tv_nsec) / 1000000;
            if (remaining <= 0) {
                return -1;
            }
            remaining_ms = (int)remaining;
        }

        struct pollfd pfd;
        pfd.fd = rech_keyboard_fd;
        pfd.events = POLLIN;
        pfd.revents = 0;
        int rv = poll(&pfd, 1, remaining_ms);
        if (rv <= 0 || (pfd.revents & POLLIN) == 0) {
            return -1;
        }

        struct input_event ev;
        ssize_t n = read(rech_keyboard_fd, &ev, sizeof(ev));
        if (n != (ssize_t)sizeof(ev)) {
            return -1;
        }
        if (ev.type != EV_KEY || (ev.value != 1 && ev.value != 2)) {
            continue;
        }
        long long ord = rech_evdev_ordinal(ev.code);
        if (ord >= 0) {
            return ord;
        }
    }
}
#else
static long long rech_vk_ordinal(WORD vk) {
    static long long table[256];
    static bool ready = false;
    if (!ready) {
        for (int i = 0; i < 256; i++) {
            table[i] = -1;
        }
        for (int c = 'A'; c <= 'Z'; c++) {
            table[c] = c - 'A';
        }
        for (int c = '0'; c <= '9'; c++) {
            table[c] = 26 + (c - '0');
        }
        table[VK_UP] = 36; table[VK_DOWN] = 37; table[VK_LEFT] = 38; table[VK_RIGHT] = 39;
        table[VK_SPACE] = 40; table[VK_RETURN] = 41; table[VK_ESCAPE] = 42; table[VK_TAB] = 43;
        table[VK_BACK] = 44;
        ready = true;
    }
    return vk < 256 ? table[vk] : -1;
}

static long long rech_read_key_code(long long timeout_ms) {
    HANDLE h = GetStdHandle(STD_INPUT_HANDLE);
    bool has_deadline = timeout_ms >= 0;
    ULONGLONG deadline = has_deadline ? GetTickCount64() + (ULONGLONG)timeout_ms : 0;

    for (;;) {
        DWORD remaining = INFINITE;
        if (has_deadline) {
            ULONGLONG now = GetTickCount64();
            if (now >= deadline) {
                return -1;
            }
            remaining = (DWORD)(deadline - now);
        }

        DWORD wait = WaitForSingleObject(h, remaining);
        if (wait != WAIT_OBJECT_0) {
            return -1;
        }

        INPUT_RECORD rec;
        DWORD events_read = 0;
        if (!ReadConsoleInputW(h, &rec, 1, &events_read) || events_read == 0) {
            return -1;
        }
        if (rec.EventType != KEY_EVENT || !rec.Event.KeyEvent.bKeyDown) {
            continue;
        }
        long long ord = rech_vk_ordinal(rec.Event.KeyEvent.wVirtualKeyCode);
        if (ord >= 0) {
            return ord;
        }
    }
}
#endif

`
}
