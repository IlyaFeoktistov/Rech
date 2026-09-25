package process

// ServiceBodyName - C-имя тела "главная" у программы "сборка.тип: служба":
// настоящий main() - ServiceMainCode, он решает, как это тело запустить.
const ServiceBodyName = "rech_service_body"

// ServiceMainCode - main() службы. На Windows процесс, запущенный
// диспетчером служб (SCM), обязан вызвать StartServiceCtrlDispatcherW:
// SCM создаёт поток, в нём rech_service_main сообщает "служба работает",
// исполняет тело "главная" и сообщает "служба остановлена". Команды Stop/
// Shutdown взводят тот же флаг, что SIGINT/SIGTERM (ShutdownFlagCode) -
// программа видит их через "запрошена_остановка()". Запуск не из SCM
// (двойной щелчок, "rech run") - ERROR_FAILED_SERVICE_CONTROLLER_CONNECT,
// тело исполняется как у обычной консольной программы (Ctrl+C - тот же
// флаг). На Linux/macOS служба - обычный процесс под systemd/launchd,
// main() просто зовёт тело.
//
// Под SCM рабочий каталог процесса - System32; rech_service_main переходит
// в каталог самого .exe (как WorkingDirectory у юнита systemd, который
// пишет "rech build"), чтобы относительные пути службы значили одно и то же
// на обеих ОС.
//
// Поток службы создаёт SCM, не программа, - Boehm GC его не видел бы, но
// GC_INIT в начале тела ("главная") исполняется уже в этом потоке и
// регистрирует его сам; поток диспетчера (main) к куче GC не обращается.
const ServiceMainCode = `#ifdef _WIN32
static SERVICE_STATUS_HANDLE rech_service_handle;
static SERVICE_STATUS rech_service_status;
static int rech_service_argc;
static char **rech_service_argv;
static int rech_service_exit_code;

static void rech_service_report(DWORD state) {
    rech_service_status.dwCurrentState = state;
    SetServiceStatus(rech_service_handle, &rech_service_status);
}

static DWORD WINAPI rech_service_control(DWORD control, DWORD type, LPVOID data, LPVOID context) {
    (void)type;
    (void)data;
    (void)context;
    switch (control) {
    case SERVICE_CONTROL_STOP:
    case SERVICE_CONTROL_SHUTDOWN:
        rech_shutdown_flag = 1;
        rech_service_report(SERVICE_STOP_PENDING);
        return NO_ERROR;
    case SERVICE_CONTROL_INTERROGATE:
        return NO_ERROR;
    default:
        return ERROR_CALL_NOT_IMPLEMENTED;
    }
}

static void rech_service_chdir_to_exe(void) {
    wchar_t path[MAX_PATH];
    DWORD len = GetModuleFileNameW(NULL, path, MAX_PATH);
    if (len == 0 || len == MAX_PATH) {
        return;
    }
    while (len > 0 && path[len - 1] != L'\\' && path[len - 1] != L'/') {
        len--;
    }
    if (len > 0) {
        path[len - 1] = L'\0';
        SetCurrentDirectoryW(path);
    }
}

static void WINAPI rech_service_main(DWORD argc, LPWSTR *argv) {
    (void)argc;
    (void)argv;
    rech_service_chdir_to_exe();
    rech_service_handle = RegisterServiceCtrlHandlerExW(L"", rech_service_control, NULL);
    if (rech_service_handle == NULL) {
        rech_service_exit_code = 1;
        return;
    }
    rech_service_status.dwServiceType = SERVICE_WIN32_OWN_PROCESS;
    rech_service_status.dwControlsAccepted = SERVICE_ACCEPT_STOP | SERVICE_ACCEPT_SHUTDOWN;
    rech_service_report(SERVICE_RUNNING);
    rech_service_exit_code = ` + ServiceBodyName + `(rech_service_argc, rech_service_argv);
    rech_service_status.dwControlsAccepted = 0;
    rech_service_status.dwWin32ExitCode = rech_service_exit_code == 0 ? NO_ERROR : ERROR_SERVICE_SPECIFIC_ERROR;
    rech_service_status.dwServiceSpecificExitCode = (DWORD)rech_service_exit_code;
    rech_service_report(SERVICE_STOPPED);
}

int main(int argc, char **argv) {
    rech_service_argc = argc;
    rech_service_argv = argv;
    SERVICE_TABLE_ENTRYW table[] = {{L"", rech_service_main}, {NULL, NULL}};
    if (StartServiceCtrlDispatcherW(table)) {
        return rech_service_exit_code;
    }
    if (GetLastError() == ERROR_FAILED_SERVICE_CONTROLLER_CONNECT) {
        signal(SIGINT, rech_shutdown_signal_handler);
        return ` + ServiceBodyName + `(argc, argv);
    }
    return 1;
}
#else
int main(int argc, char **argv) {
    return ` + ServiceBodyName + `(argc, argv);
}
#endif

`
