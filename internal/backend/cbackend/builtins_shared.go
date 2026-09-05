package cbackend

import (
	"fmt"
	"strings"

	"github.com/IlyaFeoktistov/Rech/internal/rtmsg"
)

// builtinFlags - какие runtime-хелперы понадобились из-за вызова встроенных
// функций (internal/builtins, шаг 8) - единственная таблица диспетчеризации
// (builtinCallCode) на весь backend, чтобы её не пришлось поддерживать
// отдельно на каждый путь кодогенерации.
type builtinFlags struct {
	// usesStrCmp - нужен ли strcmp (встроенная "содержит" - strstr тоже из
	// string.h, тот же заголовок; сравнение Строка "=="/"!=" вне builtin'ов
	// само по себе также использует этот флаг - тот же заголовок нужен по
	// той же причине).
	usesStrCmp bool
	// usesPanic - нужен ли rech_panic (helper аварийного завершения
	// процесса): встроенные generic-коллекции (выход за границы/операция на
	// пустой коллекции, шаг 4.2), "утверждать" (ложное условие),
	// целочисленное деление на ноль и большинство встроенных функций шага 8
	// (неверный формат ввода, ошибка открытия/чтения файла, индекс
	// аргумента вне диапазона) - все случаи используют один и тот же
	// helper.
	usesPanic bool
	// usesMath - нужен ли <math.h> (встроенные функции шага 8: корень,
	// степень, округлить, потолок, пол, модуль).
	usesMath bool
	// usesRandom - нужны ли rech_random/rech_random_int (встроенные функции
	// "случайное"/"случайное_целое", random.go) - arc4random/
	// arc4random_uniform из <stdlib.h> (glibc >= 2.36, BSD/macOS - без
	// ручного rand()/srand()), не math/rand-аналог: криптографически
	// стойкий источник нужен в первую очередь для токенов сессий (см.
	// examples/проекты/http_сервер).
	usesRandom bool
	// usesStringOps - нужны ли rech_string_length/rech_substring/
	// rech_string_find (встроенные функции "длина_строки"/"подстрока"/
	// "позиция_в_строке", string_ops.go) - индексация по code point'ам
	// Unicode, не байтам.
	usesStringOps bool
	// usesConsoleInput - нужны ли rech_read_line/rech_read_int/
	// rech_read_float (встроенные функции шага 8: прочитать_строку,
	// прочитать_целое, прочитать_число) - все три хелпера эмитятся вместе,
	// если использован хоть один (rech_read_int/rech_read_float построены
	// поверх rech_read_line). Подразумевает usesPanic (некорректный формат
	// ввода/конец ввода - тот же rech_panic, что и везде).
	usesConsoleInput bool
	// usesFileIO - нужны ли rech_open*/rech_read/rech_write/rech_close/
	// rech_read_line_from_file (встроенные функции шага 8: открыть*/
	// прочитать/записать/закрыть, "для X в Файл") - платформенно-ветвящийся
	// helper только в самом открытии файла (fopen против _wfopen), в
	// отличие от usesConsoleInput/usesMath - те везде используют один и тот
	// же стандартный C. Подразумевает usesPanic (ошибка открытия/чтения -
	// тот же rech_panic).
	usesFileIO bool
	// usesArgs - нужна ли rech_arg (встроенная функция "аргумент") -
	// rech_argc/rech_argv (argsGlobals) сами по себе всегда объявлены
	// безусловно (см. argsGlobals), этот флаг - только про саму индексную
	// функцию (нужен ей panicHelper).
	usesArgs bool
	// usesSockets - нужны ли rech_listen_tcp/rech_accept/rech_connect_tcp/
	// rech_connect_udp/rech_send/rech_recv/rech_close_socket (встроенные
	// функции шага 8: слушать_tcp/принять/подключиться_tcp/
	// подключиться_udp/отправить/получить/закрыть_сокет) - платформенно-
	// ветвящийся helper (BSD-сокеты против Winsock), плюс -lws2_32 на
	// Windows (internal/toolchain). Подразумевает usesPanic.
	usesSockets bool
	// usesUDPDatagram - нужны ли rech_recv_from/rech_send_to (встроенные
	// функции "получить_от"/"отправить_в", udp.go) - recvfrom/sendto над
	// udp-узлом, полученным "сетевой_узел" БЕЗ хоста - подразумевает
	// usesSockets (те же заголовки/rech_socket_t) и rechDupHelper
	// (rech_recv_from копирует адрес отправителя через rech_dup).
	usesUDPDatagram bool
	// usesHTTP - нужен ли rech_http_request/rech_recv_until_close/rech_dup
	// (встроенная функция шага 9: "запрос", HTTP-клиент типа curl) - строит
	// HTTP/1.1-запрос и парсит ответ поверх уже существующих rech_connect_tcp/
	// rech_send из socketHelpers, поэтому подразумевает usesSockets (и
	// usesPanic - через них).
	usesHTTP bool
	// usesHTTPServer - нужен ли rech_http_accept_request/rech_http_send_response
	// (встроенные функции "принять_http"/"отправить_http_ответ" - минимальный
	// HTTP-сервер поверх уже существующих сетевых примитивов, см.
	// httpServerHelpers) - подразумевает usesSockets.
	usesHTTPServer bool
	// usesQueryParams - нужен ли rech_query_params (встроенная функция
	// "параметры_пути" - разбор query-строки в Словарь<Строка, Строка>, см.
	// httpserver.go, queryParamsHelper).
	usesQueryParams bool
	// usesDateTime - нужны ли rech_time_now/rech_time_to_date/
	// rech_date_to_time/rech_date_to_string (datetime.go) - встроенные
	// функции "время"/"время_как_дата"/"дата_как_время"/"дату_в_строку".
	usesDateTime bool
	// usesCookies - нужны ли rech_cookie_value/rech_cookie_string
	// (cookies.go) - встроенные функции "значение_куки"/"строка_куки".
	usesCookies bool
	// usesEnv - нужен ли rech_getenv (env.go) - встроенная функция
	// "получить_переменную_окружения".
	usesEnv bool
	// usesCrypto - нужны ли rech_sha256_hex/rech_hmac_sha256_hex/
	// rech_base64url_encode/rech_base64url_decode (crypto.go) - встроенные
	// функции "хеш_sha256"/"подпись_hmac_sha256"/"закодировать_base64url"/
	// "декодировать_base64url" - не зависит от typedefs (только "const
	// char *"), тем же принципом, что и usesStringOps/usesRandom.
	usesCrypto bool
	// usesTLS - нужна ли "слушать_tcp_защищённо" (TLS для http_сервера,
	// tls.go) - переключает rech_accept/rech_send/rech_recv/
	// rech_close_socket на TLS-осведомлённые версии (см. package doc
	// tls.go) и подключает OpenSSL (#include <openssl/ssl.h>, -lssl
	// -lcrypto - internal/toolchain/cc.go) - В ОТЛИЧИЕ от usesCrypto выше
	// (SHA-256/HMAC - собственная реализация, без внешней библиотеки):
	// настоящий TLS-стек с нуля - риск для безопасности, не место для
	// принципа "не тянуть внешнюю библиотеку".
	usesTLS bool
	// usesShutdownFlag - нужен ли rech_shutdown_requested (shutdown.go) -
	// встроенная функция "запрошена_остановка" - регистрирует обработчик
	// SIGINT/SIGTERM лениво, при первом использовании программой (см. её
	// package doc).
	usesShutdownFlag bool
	// usesActiveTaskWait - нужен ли rech_wait_all_tasks (active_tasks.go) -
	// встроенная функция "дождаться_всех_задач" - отдельный от usesThreads
	// флаг (irGenCtx, не builtinFlags - builtinCallCode не видит ctx
	// целиком), хотя оба гейтят один и тот же счётчик: программа может
	// звать "дождаться_всех_задач" без "запустить" вообще (бессмысленно,
	// но не ошибка - счётчик всегда 0).
	usesActiveTaskWait bool
	// usesStringSplit - нужен ли rech_split_string (stringsplit.go) -
	// встроенная функция "разбить" (строка, разделитель): Список<Строка>.
	// Подразумевает usesPanic (пустой разделитель - ошибка, см.
	// rtmsg.SplitEmptySeparator).
	usesStringSplit bool
	// usesRegex - нужен ли rech_regex_find_all (regex.go) - встроенная
	// функция "регулярное" (строка, шаблон): Список<Совпадение> - гейтит
	// "#include <regex.h>"/"#include <locale.h>" (см. её package doc -
	// setlocale(LC_ALL, "C.UTF-8") обязателен для кириллицы в "\w"/классах
	// символов). Подразумевает usesPanic (невалидный шаблон - rech_panic).
	usesRegex bool
	// usesHTTPHeaders - нужен ли rech_header_value (httpheaders.go) -
	// встроенная функция "значение_заголовка" (заголовки, имя): ?Строка -
	// общий разбор строки заголовков Запрос/Ответ, не привязан к cookies.
	usesHTTPHeaders bool
}

// builtinCallCode генерирует C-вызов встроенной функции (internal/builtins,
// шаг 8) - имя и типы аргументов уже проверены до этого места (internal/
// semantic), здесь только выбор конкретного C-вызова и включение нужных
// runtime-флагов (математика - <math.h>, ввод - rech_read_*, тоже требует
// usesPanic - некорректный формат ввода/конец ввода паникуют). ok == false -
// name не встроенная функция, вызывающий код использует обычный путь
// (mangleName).
func builtinCallCode(flags *builtinFlags, name string, args []string) (string, bool) {
	switch name {
	case "корень":
		flags.usesMath = true
		return fmt.Sprintf("sqrt(%s)", args[0]), true
	case "степень":
		flags.usesMath = true
		return fmt.Sprintf("pow(%s, %s)", args[0], args[1]), true
	case "округлить":
		flags.usesMath = true
		return fmt.Sprintf("(long long)llround(%s)", args[0]), true
	case "потолок":
		flags.usesMath = true
		return fmt.Sprintf("(long long)ceil(%s)", args[0]), true
	case "пол":
		flags.usesMath = true
		return fmt.Sprintf("(long long)floor(%s)", args[0]), true
	case "модуль":
		flags.usesMath = true
		return fmt.Sprintf("fabs(%s)", args[0]), true
	case "экспонента":
		flags.usesMath = true
		return fmt.Sprintf("exp(%s)", args[0]), true
	case "синус":
		flags.usesMath = true
		return fmt.Sprintf("sin(%s)", args[0]), true
	case "косинус":
		flags.usesMath = true
		return fmt.Sprintf("cos(%s)", args[0]), true
	case "тангенс":
		flags.usesMath = true
		return fmt.Sprintf("tan(%s)", args[0]), true
	case "арксинус":
		flags.usesMath = true
		return fmt.Sprintf("asin(%s)", args[0]), true
	case "арккосинус":
		flags.usesMath = true
		return fmt.Sprintf("acos(%s)", args[0]), true
	case "арктангенс":
		flags.usesMath = true
		return fmt.Sprintf("atan(%s)", args[0]), true
	case "арктангенс2":
		flags.usesMath = true
		return fmt.Sprintf("atan2(%s, %s)", args[0], args[1]), true
	case "логарифм":
		flags.usesMath = true
		return fmt.Sprintf("log(%s)", args[0]), true
	case "логарифм2":
		flags.usesMath = true
		return fmt.Sprintf("log2(%s)", args[0]), true
	case "логарифм10":
		flags.usesMath = true
		return fmt.Sprintf("log10(%s)", args[0]), true
	case "минимум":
		flags.usesMath = true
		return fmt.Sprintf("fmin(%s, %s)", args[0], args[1]), true
	case "максимум":
		flags.usesMath = true
		return fmt.Sprintf("fmax(%s, %s)", args[0], args[1]), true
	case "пи":
		flags.usesMath = true
		return "3.14159265358979323846", true
	case "прочитать_строку":
		flags.usesConsoleInput = true
		flags.usesPanic = true
		return "rech_read_line()", true
	case "прочитать_целое":
		flags.usesConsoleInput = true
		flags.usesPanic = true
		return "rech_read_int()", true
	case "прочитать_число":
		flags.usesConsoleInput = true
		flags.usesPanic = true
		return "rech_read_float()", true
	case "открыть":
		flags.usesFileIO = true
		flags.usesPanic = true
		return fmt.Sprintf("rech_open(%s)", args[0]), true
	case "открыть_для_записи":
		flags.usesFileIO = true
		flags.usesPanic = true
		return fmt.Sprintf("rech_open_write(%s)", args[0]), true
	case "открыть_для_дозаписи":
		flags.usesFileIO = true
		flags.usesPanic = true
		return fmt.Sprintf("rech_open_append(%s)", args[0]), true
	case "прочитать":
		flags.usesFileIO = true
		flags.usesPanic = true
		return fmt.Sprintf("rech_read(%s, %s)", args[0], args[1]), true
	case "записать":
		flags.usesFileIO = true
		flags.usesPanic = true
		return fmt.Sprintf("rech_write(%s, %s)", args[0], args[1]), true
	case "закрыть":
		flags.usesFileIO = true
		return fmt.Sprintf("rech_close(%s)", args[0]), true
	case "прочитать_файл":
		flags.usesFileIO = true
		flags.usesPanic = true
		return fmt.Sprintf("rech_read_whole_file(%s)", args[0]), true
	case "записать_в_файл":
		flags.usesFileIO = true
		flags.usesPanic = true
		return fmt.Sprintf("rech_write_file(%s, %s)", args[0], args[1]), true
	case "дописать_в_файл":
		flags.usesFileIO = true
		flags.usesPanic = true
		return fmt.Sprintf("rech_append_file(%s, %s)", args[0], args[1]), true
	case "сетевой_узел":
		flags.usesSockets = true
		flags.usesPanic = true
		return fmt.Sprintf("rech_network_node(%s, %s, %s)", args[0], args[1], args[2]), true
	case "принять":
		flags.usesSockets = true
		flags.usesPanic = true
		return fmt.Sprintf("rech_accept(%s)", args[0]), true
	case "установить_таймаут":
		flags.usesSockets = true
		return fmt.Sprintf("rech_set_timeout(%s, %s)", args[0], args[1]), true
	case "запрошена_остановка":
		flags.usesShutdownFlag = true
		return "rech_shutdown_requested()", true
	case "дождаться_всех_задач":
		flags.usesActiveTaskWait = true
		return fmt.Sprintf("rech_wait_all_tasks(%s)", args[0]), true
	case "отправить":
		flags.usesSockets = true
		flags.usesPanic = true
		return fmt.Sprintf("rech_send(%s, %s)", args[0], args[1]), true
	case "получить":
		flags.usesSockets = true
		flags.usesPanic = true
		return fmt.Sprintf("rech_recv(%s, %s)", args[0], args[1]), true
	case "получить_от":
		flags.usesSockets = true
		flags.usesUDPDatagram = true
		flags.usesPanic = true
		return fmt.Sprintf("rech_recv_from(%s, %s)", args[0], args[1]), true
	case "отправить_в":
		flags.usesSockets = true
		flags.usesUDPDatagram = true
		flags.usesPanic = true
		return fmt.Sprintf("rech_send_to(%s, %s, %s, %s)", args[0], args[1], args[2], args[3]), true
	case "закрыть_сокет":
		flags.usesSockets = true
		return fmt.Sprintf("rech_close_socket(%s)", args[0]), true
	case "слушать_tcp_защищённо":
		flags.usesSockets = true
		flags.usesTLS = true
		flags.usesPanic = true
		return fmt.Sprintf("rech_listen_tcp_secure(%s, %s, %s)", args[0], args[1], args[2]), true
	case "запрос":
		flags.usesHTTP = true
		flags.usesSockets = true
		flags.usesPanic = true
		return fmt.Sprintf("rech_http_request(%s)", args[0]), true
	case "принять_http":
		flags.usesHTTPServer = true
		flags.usesSockets = true
		flags.usesPanic = true
		return fmt.Sprintf("rech_http_accept_request(%s)", args[0]), true
	case "отправить_http_ответ":
		flags.usesHTTPServer = true
		flags.usesSockets = true
		flags.usesPanic = true
		return fmt.Sprintf("rech_http_send_response(%s, %s)", args[0], args[1]), true
	case "параметры_пути":
		flags.usesQueryParams = true
		return fmt.Sprintf("rech_query_params(%s)", args[0]), true
	case "разбить":
		flags.usesStringSplit = true
		flags.usesPanic = true
		return fmt.Sprintf("rech_split_string(%s, %s)", args[0], args[1]), true
	case "регулярное":
		flags.usesRegex = true
		flags.usesPanic = true
		return fmt.Sprintf("rech_regex_find_all(%s, %s)", args[0], args[1]), true
	case "длина_строки":
		flags.usesStringOps = true
		return fmt.Sprintf("rech_string_length(%s)", args[0]), true
	case "подстрока":
		flags.usesStringOps = true
		flags.usesPanic = true
		return fmt.Sprintf("rech_substring(%s, %s, %s)", args[0], args[1], args[2]), true
	case "позиция_в_строке":
		flags.usesStringOps = true
		return fmt.Sprintf("rech_string_find(%s, %s)", args[0], args[1]), true
	case "случайное":
		flags.usesRandom = true
		return "rech_random()", true
	case "случайное_целое":
		flags.usesRandom = true
		flags.usesPanic = true
		return fmt.Sprintf("rech_random_int(%s, %s)", args[0], args[1]), true
	case "время":
		flags.usesDateTime = true
		return "rech_time_now()", true
	case "время_как_дата":
		flags.usesDateTime = true
		return fmt.Sprintf("rech_time_to_date(%s)", args[0]), true
	case "дата_как_время":
		flags.usesDateTime = true
		return fmt.Sprintf("rech_date_to_time(%s)", args[0]), true
	case "дату_в_строку":
		flags.usesDateTime = true
		return fmt.Sprintf("rech_date_to_string(%s)", args[0]), true
	case "значение_куки":
		flags.usesCookies = true
		return fmt.Sprintf("rech_cookie_value((%s).%s, %s)", args[0], mangleField("заголовки"), args[1]), true
	case "значение_заголовка":
		flags.usesHTTPHeaders = true
		return fmt.Sprintf("rech_header_value(%s, %s)", args[0], args[1]), true
	case "получить_переменную_окружения":
		flags.usesEnv = true
		return fmt.Sprintf("rech_getenv(%s)", args[0]), true
	case "строка_куки":
		flags.usesCookies = true
		return fmt.Sprintf("rech_cookie_string(%s, %s, %s)", args[0], args[1], args[2]), true
	case "подключиться_к_бд":
		flags.usesPanic = true
		return fmt.Sprintf("rech_db_connect(%s)", args[0]), true
	case "запросить":
		flags.usesPanic = true
		return fmt.Sprintf("rech_db_query(%s, %s, %s)", args[0], args[1], args[2]), true
	case "выполнить":
		flags.usesPanic = true
		return fmt.Sprintf("rech_db_exec(%s, %s, %s)", args[0], args[1], args[2]), true
	case "закрыть_бд":
		return fmt.Sprintf("rech_db_close(%s)", args[0]), true
	case "начать_транзакцию":
		flags.usesPanic = true
		return fmt.Sprintf("%s(%s, %s)", dbRunTransactionFunc, args[0], args[1]), true
	case "стандартный_ввод":
		flags.usesFileIO = true
		return "stdin", true
	case "стандартный_вывод":
		flags.usesFileIO = true
		return "stdout", true
	case "стандартный_вывод_ошибок":
		flags.usesFileIO = true
		return "stderr", true
	case "количество_аргументов":
		return "(rech_argc - 1)", true
	case "аргумент":
		flags.usesArgs = true
		flags.usesPanic = true
		return fmt.Sprintf("rech_arg(%s)", args[0]), true
	case "содержит":
		// usesStrCmp - тот же флаг, что и у "==" между Строка - оба нужны
		// исключительно за <string.h>, здесь - за strstr.
		flags.usesStrCmp = true
		return fmt.Sprintf("(strstr(%s, %s) != NULL)", args[0], args[1]), true
	case "хеш_sha256":
		flags.usesCrypto = true
		return fmt.Sprintf("rech_sha256_hex(%s)", args[0]), true
	case "подпись_hmac_sha256":
		flags.usesCrypto = true
		return fmt.Sprintf("rech_hmac_sha256_hex(%s, %s)", args[0], args[1]), true
	case "закодировать_base64url":
		flags.usesCrypto = true
		return fmt.Sprintf("rech_base64url_encode(%s)", args[0]), true
	case "декодировать_base64url":
		flags.usesCrypto = true
		flags.usesPanic = true
		return fmt.Sprintf("rech_base64url_decode(%s)", args[0]), true
	default:
		return "", false
	}
}

const strConcatHelper = `static char *rech_str_concat(const char *a, const char *b) {
    size_t la = strlen(a), lb = strlen(b);
    char *out = malloc(la + lb + 1);
    memcpy(out, a, la);
    memcpy(out + la, b, lb);
    out[la + lb] = '\0';
    return out;
}
`

// consoleInitHelper - на Windows консоль по умолчанию не в UTF-8 (легаси
// OEM-кодовая страница, например 866) - вывод/ввод кириллицы (все сообщения
// компилятора и рантайма Rech - на русском) без этого превращается в
// нечитаемый мусор, хотя сам код везде корректный UTF-8 (см. lexer.
// ValidateUTF8). rech_init_console вызывается один раз, первой инструкцией
// main() - переключает кодовую страницу консоли на UTF-8 для ввода и
// вывода сразу для всего процесса. На Linux/macOS терминал и так в UTF-8 -
// функция там пустая, а не отсутствующая (чтобы вызов из main() был
// безусловным, без #ifdef на стороне вызывающего кода).
const consoleInitHelper = `#ifdef _WIN32
static void rech_init_console(void) {
    SetConsoleOutputCP(CP_UTF8);
    SetConsoleCP(CP_UTF8);
}
#else
static void rech_init_console(void) {}
#endif
`

// argsGlobals - rech_argc/rech_argv + rech_init_args, всегда объявлены
// безусловно (не под usesArgs, в отличие от argFuncHelper ниже) - main()
// зовёт rech_init_args(argc, argv) на этапе генерации ЕЁ тела, то есть
// раньше, чем стало известно, использует ли программа "аргумент"/
// "количество_аргументов" где-либо вообще (эти флаги считаются по итогам
// генерации ВСЕХ функций, а преамбула, где обычно решается "включать или
// нет", печатается уже после этого - но сама main() пишется по ходу того
// же прохода).
//
// На Windows игнорирует свои же параметры и вместо этого перечитывает
// аргументы через CommandLineToArgvW (нужен -lshell32, internal/toolchain)
// - argv, который сам даёт CRT в main(int argc, char **argv), закодирован
// в активной ANSI-кодовой странице процесса, не UTF-8 (тот же класс
// проблемы, что и у путей к файлам в internal/compiler.compileProgram и у
// вывода кириллицы в rech_init_console выше - Windows-API вообще нигде не
// говорит по умолчанию на UTF-8, только через явные Wide-варианты).
const argsGlobals = `static int rech_argc = 0;
static char **rech_argv = NULL;

#ifdef _WIN32
static void rech_init_args(int argc, char **argv) {
    (void)argc;
    (void)argv;
    int wargc = 0;
    LPWSTR *wargv = CommandLineToArgvW(GetCommandLineW(), &wargc);
    rech_argc = wargc;
    rech_argv = malloc(sizeof(char *) * (size_t)wargc);
    for (int i = 0; i < wargc; i++) {
        int len = WideCharToMultiByte(CP_UTF8, 0, wargv[i], -1, NULL, 0, NULL, NULL);
        char *buf = malloc((size_t)len);
        WideCharToMultiByte(CP_UTF8, 0, wargv[i], -1, buf, len, NULL, NULL);
        rech_argv[i] = buf;
    }
    LocalFree(wargv);
}
#else
static void rech_init_args(int argc, char **argv) {
    rech_argc = argc;
    rech_argv = argv;
}
#endif
`

// argFuncHelper - "аргумент" (сама индексная функция, вызывает rech_panic
// при выходе за диапазон - в отличие от argsGlobals выше, требует
// panicHelper, поэтому под usesArgs, как и остальные необязательные
// helper'ы). Индексация с 0, как и у "получить" встроенных коллекций -
// rech_arg(i) читает argv[i+1]. Текст сообщения об ошибке должен совпадать
// байт-в-байт с rtmsg.ArgIndexOutOfRangeFormat - Go "%d" и C "%lld"
// печатают одно и то же десятичное число для любого int64, включая
// отрицательные.
const argFuncHelper = `static const char *rech_arg(long long i) {
    if (i < 0 || i >= (long long)rech_argc - 1) {
        char msg[128];
        snprintf(msg, sizeof(msg), "аргумент: индекс %lld вне диапазона", i);
        rech_panic(msg);
    }
    return rech_argv[i + 1];
}
`

// panicHelper - аварийное завершение процесса с понятным сообщением:
// снять/извлечь с пустой Стопки/Очереди, индекс вне границ у Списка -
// осознанно необрабатываемая ошибка (аналог паники в Go/Rust), не выброс
// исключения (internal/backend/cbackend/exceptions.go, rech_throw) -
// ошибка использования встроенной коллекции самой программой, не
// исключительная ситуация во внешних данных/окружении.
var panicHelper = fmt.Sprintf(`static void rech_panic(const char *msg) {
    fprintf(stderr, "%s%%s\n", msg);
    rech_trace_print();
    exit(1);
}
`, rtmsg.PanicPrefix)

// intWidthHelperSpec - C-тип и (для знаковых) минимальное представимое
// значение конкретной ширины rech_int_div<N>/rech_int_mod<N>/
// rech_uint_div<N>/rech_uint_mod<N> (см. intDivModHelperCode). min пустой у
// беззнаковых - у них нет случая переполнения при делении вовсе (см. ниже).
type intWidthHelperSpec struct {
	bits  int
	cType string
	min   string
}

var signedDivModWidths = []intWidthHelperSpec{
	{8, "int8_t", "INT8_MIN"},
	{16, "int16_t", "INT16_MIN"},
	{32, "int32_t", "INT32_MIN"},
	{64, "int64_t", "INT64_MIN"},
}

var unsignedDivModWidths = []intWidthHelperSpec{
	{8, "uint8_t", ""},
	{16, "uint16_t", ""},
	{32, "uint32_t", ""},
	{64, "uint64_t", ""},
}

// intDivModHelperCode строит текст ТОЛЬКО реально использованных (см. u,
// ir_generator.go.intDivModUsage) rech_int_div<N>/rech_int_mod<N>
// (signed=true) или rech_uint_div<N>/rech_uint_mod<N> (signed=false) -
// настоящий C-тип своей ширины на входе/выходе (int8_t/.../uint64_t), не
// общий "продвинуть до 64 бит и усечь обратно". Оба - с проверкой делителя
// на ноль (без него UB в C - обычно SIGFPE, а не понятная ошибка) -
// выбрасывают перехватываемое "ОшибкаДеленияНаНоль" через
// rech_divzero_throw (divZeroThrowHelperCode, json_extract.go), не
// rech_panic.
//
// Знаковое деление/остаток дополнительно проверяет "a == MIN && b == -1" -
// единственный случай, где ЗНАКОВОЕ деление целочисленного типа - тоже UB в
// C (частное не умещается в тип, даже если делитель не 0 - на x86 та же
// инструкция idiv может упасть по SIGFPE, хотя остаток от такого деления
// математически всегда 0 и прекрасно умещается). Явная проверка возвращает
// тот же результат, что естественным путём даёт two's-complement
// переполнение (и то же, что уже делает interp - см.
// internal/interp/widthints.go.wrapFixedWidth): деление - само MIN,
// остаток - 0. У беззнаковых такого случая нет вовсе - деление/остаток без
// знака никогда не переполняет.
func intDivModHelperCode(u intDivModUsage, signed bool) string {
	widths := unsignedDivModWidths
	prefix := "uint"
	if signed {
		widths = signedDivModWidths
		prefix = "int"
	}
	var sb strings.Builder
	for _, w := range widths {
		if u.wants(false, w.bits) {
			sb.WriteString(intDivHelperText(prefix, w))
		}
		if u.wants(true, w.bits) {
			sb.WriteString(intModHelperText(prefix, w))
		}
	}
	return sb.String()
}

func intDivHelperText(prefix string, w intWidthHelperSpec) string {
	if w.min == "" {
		return fmt.Sprintf(`static %[1]s rech_%[2]s_div%[3]d(%[1]s a, %[1]s b) {
    if (b == 0) {
        rech_divzero_throw("%[4]s");
    }
    return a / b;
}
`, w.cType, prefix, w.bits, rtmsg.DivisionByZero)
	}
	return fmt.Sprintf(`static %[1]s rech_%[2]s_div%[3]d(%[1]s a, %[1]s b) {
    if (b == 0) {
        rech_divzero_throw("%[4]s");
    }
    if (a == %[5]s && b == -1) {
        return %[5]s;
    }
    return a / b;
}
`, w.cType, prefix, w.bits, rtmsg.DivisionByZero, w.min)
}

func intModHelperText(prefix string, w intWidthHelperSpec) string {
	if w.min == "" {
		return fmt.Sprintf(`static %[1]s rech_%[2]s_mod%[3]d(%[1]s a, %[1]s b) {
    if (b == 0) {
        rech_divzero_throw("%[4]s");
    }
    return a %% b;
}
`, w.cType, prefix, w.bits, rtmsg.DivisionByZero)
	}
	return fmt.Sprintf(`static %[1]s rech_%[2]s_mod%[3]d(%[1]s a, %[1]s b) {
    if (b == 0) {
        rech_divzero_throw("%[4]s");
    }
    if (a == %[5]s && b == -1) {
        return 0;
    }
    return a %% b;
}
`, w.cType, prefix, w.bits, rtmsg.DivisionByZero, w.min)
}

// readHelpers - прочитать_строку/прочитать_целое/прочитать_число (шаг 8, без
// FFI - стандартный ввод есть у любой программы). rech_read_line
// намеренно не использует POSIX-only getline (шаг 8 явно требует
// кроссплатформенности, docs/ROADMAP.md) - растущий буфer поверх обычного
// fgets, доступного в любом C. Каждый вызов возвращает новый malloc'нутый
// буфер (как rech_str_concat) - статический/переиспользуемый буфер был бы
// багом: две переменные, каждая из своего вызова прочитать_строку, не
// должны делить память. rech_read_int/rech_read_float построены поверх
// rech_read_line - разбирают ровно всю строку (strtoll/strtod плюс
// проверка, что *end == '\0', как и в других местах разбора чисел) и
// паникуют с сообщением, включающим то, что реально ввёл пользователь -
// формат сообщения (буквальные "\"%%s\"") должен совпадать с
// rtmsg.ReadIntFormat/ReadFloatFormat (интерпретатор строит то же
// сообщение через fmt.Sprintf с тем же шаблоном - см. internal/interp).
var readHelpers = fmt.Sprintf(`static char *rech_read_line(void) {
    size_t cap = 128, len = 0;
    char *buf = malloc(cap);
    for (;;) {
        if (!fgets(buf + len, (int)(cap - len), stdin)) {
            if (len == 0) {
                rech_panic("%s");
            }
            break;
        }
        len += strlen(buf + len);
        if (len > 0 && buf[len - 1] == '\n') {
            break;
        }
        if (len + 1 >= cap) {
            cap *= 2;
            buf = realloc(buf, cap);
        }
    }
    while (len > 0 && (buf[len - 1] == '\n' || buf[len - 1] == '\r')) {
        buf[--len] = '\0';
    }
    return buf;
}

// rech_trim_line обрезает пробелы/табы с обоих концов line на месте -
// ровно тот же набор символов, что и strings.Trim(line, " \t") на стороне
// интерпретатора (internal/interp), чтобы "  42  " разбиралось одинаково в
// обоих путях (strtoll/strtod сами пропускают ведущие пробелы, но не
// завершающие - без этой обрезки "42 " с одним путём разошлось бы с другим).
static char *rech_trim_line(char *line) {
    while (*line == ' ' || *line == '\t') {
        line++;
    }
    size_t len = strlen(line);
    while (len > 0 && (line[len - 1] == ' ' || line[len - 1] == '\t')) {
        line[--len] = '\0';
    }
    return line;
}

static long long rech_read_int(void) {
    char *line = rech_trim_line(rech_read_line());
    char *end;
    long long v = strtoll(line, &end, 10);
    if (end == line || *end != '\0') {
        char msg[256];
        snprintf(msg, sizeof(msg), "прочитать_целое: \"%%s\" не является целым числом", line);
        rech_panic(msg);
    }
    return v;
}

static double rech_read_float(void) {
    char *line = rech_trim_line(rech_read_line());
    char *end;
    double v = strtod(line, &end);
    if (end == line || *end != '\0') {
        char msg[256];
        snprintf(msg, sizeof(msg), "прочитать_число: \"%%s\" не является числом", line);
        rech_panic(msg);
    }
    return v;
}
`, rtmsg.ReadEOF)

// fileHelpers - открыть*/прочитать/записать/закрыть и построчное чтение
// для "для X в Y" (шаг 8, файловый ввод-вывод). rech_file_t - всегда
// "FILE *" (не int/HANDLE, как в первом срезе этой фичи) - стандартный C
// stdio уже даёт буферизованное чтение/запись и построчный fgets
// одинаково на всех платформах, поэтому единственное, что реально
// расходится по платформе - сам ОТКРЫТИЕ файла: Rech "Строка" - UTF-8, а
// обычный узкий fopen на Windows интерпретирует путь через активную
// ANSI-кодовую страницу процесса (та же причина, по которой mingw
// gcc/ld ломают кириллицу в аргументах командной строки, см.
// internal/compiler.compileProgram) - значит нужен _wfopen с путём,
// перекодированным в UTF-16 (windows.h - тут только за
// MultiByteToWideChar, не за HANDLE/CreateFileW, как раньше). Сообщение
// об ошибке открытия строится в самом C-коде (snprintf с настоящими op/
// путём), должно совпадать байт-в-байт с rtmsg.OpenFailedFormat (см.
// internal/interp/builtins.go).
var fileHelpers = fmt.Sprintf(`typedef FILE *rech_file_t;

#ifdef _WIN32
static wchar_t *rech_utf8_to_wide(const char *s) {
    int wlen = MultiByteToWideChar(CP_UTF8, 0, s, -1, NULL, 0);
    wchar_t *w = malloc((size_t)wlen * sizeof(wchar_t));
    MultiByteToWideChar(CP_UTF8, 0, s, -1, w, wlen);
    return w;
}
#endif

static rech_file_t rech_open_mode(const char *path, const char *mode, const wchar_t *wmode, const char *op) {
#ifdef _WIN32
    wchar_t *wpath = rech_utf8_to_wide(path);
    FILE *f = _wfopen(wpath, wmode);
    free(wpath);
#else
    FILE *f = fopen(path, mode);
#endif
    if (!f) {
        char msg[512];
        snprintf(msg, sizeof(msg), "%%s: не удалось открыть файл \"%%s\"", op, path);
        rech_panic(msg);
    }
    return f;
}

static rech_file_t rech_open(const char *path) {
    return rech_open_mode(path, "rb", L"rb", "открыть");
}

static rech_file_t rech_open_write(const char *path) {
    return rech_open_mode(path, "wb", L"wb", "открыть_для_записи");
}

static rech_file_t rech_open_append(const char *path) {
    return rech_open_mode(path, "ab", L"ab", "открыть_для_дозаписи");
}

static char *rech_read(rech_file_t f, long long max) {
    if (max <= 0) {
        rech_panic("%[1]s");
    }
    char *buf = malloc((size_t)max + 1);
    size_t n = fread(buf, 1, (size_t)max, f);
    if (n == 0 && ferror(f)) {
        rech_panic("%[2]s");
    }
    buf[n] = '\0';
    return buf;
}

static void rech_write(rech_file_t f, const char *s) {
    size_t len = strlen(s);
    if (fwrite(s, 1, len, f) != len) {
        rech_panic("%[3]s");
    }
}

static void rech_close(rech_file_t f) {
    fclose(f);
}

// rech_read_line_from_file - "для <строка> в <Файл>". *ok == false -
// настоящий конец файла (ничего не прочитано вообще), а не просто пустая
// строка - пустая строка файла ("\n" сам по себе) - это *ok == true с
// buf == "", *ok различает их именно затем, что пустая Строка сама по себе
// не годится как признак конца файла (в отличие от "прочитать" выше, где
// пустая Строка - это и есть достаточный признак EOF, потому что при
// max > 0 обычное чтение никогда не возвращает "" иначе). Последняя строка
// файла без завершающего "\n" всё равно возвращается один раз
// (*ok == true) - цикл увидит конец файла только на СЛЕДУЮЩЕЙ попытке.
static char *rech_read_line_from_file(rech_file_t f, bool *ok) {
    size_t cap = 128, len = 0;
    char *buf = malloc(cap);
    int got_any = 0;
    for (;;) {
        if (!fgets(buf + len, (int)(cap - len), f)) {
            break;
        }
        got_any = 1;
        len += strlen(buf + len);
        if (len > 0 && buf[len - 1] == '\n') {
            break;
        }
        if (len + 1 >= cap) {
            cap *= 2;
            buf = realloc(buf, cap);
        }
    }
    if (!got_any) {
        free(buf);
        *ok = false;
        return NULL;
    }
    while (len > 0 && (buf[len - 1] == '\n' || buf[len - 1] == '\r')) {
        buf[--len] = '\0';
    }
    *ok = true;
    return buf;
}

// rech_read_whole_file/rech_write_file/rech_append_file - высокоуровневые
// обёртки без ручного открыть/прочитать(-в цикле)/закрыть (шаг 8) -
// открывают, делают ровно одну операцию на весь файл, закрывают.
// rech_write_whole_file - общий хвост записи/дозаписи, отличаются только
// режимом открытия (как rech_open/rech_open_write/rech_open_append выше).
static char *rech_read_whole_file(const char *path) {
    rech_file_t f = rech_open_mode(path, "rb", L"rb", "прочитать_файл");
    if (fseek(f, 0, SEEK_END) != 0 || ftell(f) < 0) {
        rech_panic("%[2]s");
    }
    long size = ftell(f);
    rewind(f);
    char *buf = malloc((size_t)size + 1);
    size_t n = fread(buf, 1, (size_t)size, f);
    if (n != (size_t)size && ferror(f)) {
        rech_panic("%[2]s");
    }
    buf[n] = '\0';
    fclose(f);
    return buf;
}

static void rech_write_whole_file(const char *path, const char *content, const char *mode, const wchar_t *wmode, const char *op) {
    rech_file_t f = rech_open_mode(path, mode, wmode, op);
    rech_write(f, content);
    fclose(f);
}

static void rech_write_file(const char *path, const char *content) {
    rech_write_whole_file(path, content, "wb", L"wb", "записать_в_файл");
}

static void rech_append_file(const char *path, const char *content) {
    rech_write_whole_file(path, content, "ab", L"ab", "дописать_в_файл");
}
`, rtmsg.ReadInvalidMax, rtmsg.ReadIOFailed, rtmsg.WriteIOFailed)

// socketHelpers - сетевые примитивы: TCP-сервер (rech_listen_tcp +
// rech_accept), UDP-сервер (rech_listen_udp - bind без connect/listen,
// дальше recvfrom/sendto - см. udpDatagramHelpers), TCP/UDP-клиент
// (rech_connect_tcp/rech_connect_udp - для UDP через connect() на
// datagram-сокете, что фиксирует адрес получателя и позволяет дальше
// использовать один и тот же send/recv, что и у TCP), rech_send/rech_recv/
// rech_close_socket. rech_network_node - единая точка входа
// ("сетевой_узел(режим, хост, порт)"): host[0] == '\0' решает "слушать"
// вместо "подключиться", mode строкой ("tcp"/"udp") - какой из четырёх
// вариантов. rech_socket_t - int на POSIX, SOCKET на Windows (оба -
// непрозрачные дескрипторы, как rech_file_t выше, только другого
// системного API).
// rechSocketTypedef - typedef rech_socket_t/RECH_INVALID_SOCKET - ОТДЕЛЬНО
// от остального socketTypeAndConnectCode (которое ссылается на этот тип,
// но само эмитируется намного позже, вперемешку с rech_accept - см.
// ir_generator.go, appendCodeIf(prog.UsesSockets...)) и эмитируется РАНЬШЕ
// nullablesCode (nullable.go) - тем же принципом, что и dbHandleTypedefCode
// (db.go): "?Сокет" ("принять" теперь возвращает nullable) несёт
// rech_socket_t как элемент своей обёртки (nullableCollectionCCode,
// elemC), значит rech_socket_t обязан существовать РАНЬШЕ первого
// упоминания "?Сокет", а rech_accept (использует ОБЁРТКУ "?Сокет") -
// заведомо позже самой обёртки. Три уровня зависимости в ряд (rech_socket_t
// -> обёртка "?Сокет" -> rech_accept), поэтому именно typedef выделен в
// свой самый ранний, самостоятельный кусок, а не весь socketTypeAndConnectCode.
//
// Эмитируется БЕЗУСЛОВНО (ir_generator.go зовёт её не через
// appendCodeIf(ctx.usesSockets, ...), как остальной сетевой рантайм, а
// всегда) - НАЙДЕННЫЙ БАГ: "?Сокет" (nullable-обёртка "принять") попадает
// в prog.Nullables БЕЗУСЛОВНО для ЛЮБОЙ программы (internal/semantic/
// analyzer.go строит prog.Nullables сканом ВСЕХ когда-либо
// зарегистрированных nullable-сущностей, а не только реально
// использованных - registerNetworkFuncs регистрирует funcSig "принять"
// безусловно, тем же принципом, что и остальные builtin'ы), значит
// nullablesCode всегда пытается эмитировать обёртку вокруг rech_socket_t,
// даже для программы, ни разу не тронувшей сети. "?Строка" (тот же
// безусловный принцип регистрации у "получить_переменную_окружения",
// registerEnvFuncs) с этим никогда не сталкивалась - "Строка" (const
// char *) не нуждается в собственном typedef, а rech_socket_t раньше
// был условным - настоящий cc падал "unknown type name rech_socket_t" на
// любой программе без единого сетевого вызова. Поэтому typedef здесь
// (в отличие от остального socketTypeAndConnectCode) самодостаточен - сам
// несёт свой #include <winsock2.h> на Windows (SOCKET нужен уже здесь, а
// не только там, где winsock2.h подключается условно, ниже).
const rechSocketTypedef = `#ifdef _WIN32
#include <winsock2.h>
typedef SOCKET rech_socket_t;
#define RECH_INVALID_SOCKET INVALID_SOCKET
#else
typedef int rech_socket_t;
#define RECH_INVALID_SOCKET (-1)
#endif

`

// socketTypeAndConnectCode - слушать(tcp/udp)/подключиться(tcp/udp)/
// rech_network_node - ОБЩАЯ часть между socketHelpers (обычная программа)
// и socketHelpersTLS (программа с "слушать_tcp_защищённо", tls.go) -
// установление соединения не зависит от TLS вообще (TLS оборачивает уже
// готовый TCP-сокет ПОСЛЕ rech_accept, см. package doc tls.go) - только
// rech_accept/rech_send/rech_recv/rech_close_socket ниже (plainSocketOpsCode)
// различаются между двумя вариантами.
//
// "signal(SIGPIPE, SIG_IGN)" (только POSIX - на Windows SIGPIPE не
// существует вовсе, Winsock просто возвращает SOCKET_ERROR) - без него
// запись ("отправить"/"отправить_http_ответ") в сокет, который ПИР уже
// закрыл (например, HTTP keep-alive соединение, где клиент отключился
// между запросами - examples/проекты/http_сервер) убивает ВЕСЬ процесс
// СРАЗУ (POSIX-умолчание для SIGPIPE - завершение процесса, не просто
// ошибка вызова) - живая находка (docs/ROADMAP.md, "HTTP keep-alive"):
// сервер падал молча, без единой строчки паники, при закрытии клиентом
// keep-alive-соединения. SIGPIPE - ГЛОБАЛЬНАЯ, на весь процесс, настройка
// (не per-сокет) - одного вызова где угодно ДО первой отправки достаточно
// на всю программу, поэтому он просто идемпотентно продублирован в каждой
// функции создания сокета (тем же приёмом, что и rech_shutdown_requested,
// shutdown.go - переустановка того же значения не накапливается и не
// вредит). rech_socket_t сам по себе - см.
// rechSocketTypedef выше, эмитируется отдельно и раньше.
var socketTypeAndConnectCode = fmt.Sprintf(`static rech_socket_t rech_listen_tcp(long long port) {
#ifndef _WIN32
    signal(SIGPIPE, SIG_IGN);
#endif
    rech_socket_t s = socket(AF_INET, SOCK_STREAM, 0);
    if (s == RECH_INVALID_SOCKET) {
        rech_panic("%[1]s");
    }
    int yes = 1;
    setsockopt(s, SOL_SOCKET, SO_REUSEADDR, (const char *)&yes, sizeof(yes));
    struct sockaddr_in addr;
    memset(&addr, 0, sizeof(addr));
    addr.sin_family = AF_INET;
    addr.sin_addr.s_addr = INADDR_ANY;
    addr.sin_port = htons((unsigned short)port);
    if (bind(s, (struct sockaddr *)&addr, sizeof(addr)) != 0) {
        rech_panic("%[2]s");
    }
    if (listen(s, SOMAXCONN) != 0) {
        rech_panic("%[3]s");
    }
    return s;
}

// rech_set_timeout - "установить_таймаут(сокет, миллисекунды)" - SO_RCVTIMEO
// (Windows принимает миллисекунды напрямую как DWORD, POSIX - struct
// timeval) - СТОЯЩАЯ (не разовая) настройка сокета, действует на КАЖДЫЙ
// следующий "получить"/"принять_http" (через rech_socket_read_raw - её
// TLS-осведомлённая версия, tls.go, тоже в конце концов читает тем же
// системным recv()/сокетом, таймаут не обходит TLS) и, на слушающем
// Сокете, на "принять" (rech_accept ниже/tls.go - ядро применяет
// SO_RCVTIMEO слушающего сокета и к accept(), не только к recv()) - ноль
// или отрицательное значение снимает таймаут (сокет снова блокируется
// бессрочно). Один и тот же вызов работает на обоих вариантах rech_accept
// (с TLS и без) - сам таймаут ниже уровня TLS, на голом сокете.
static void rech_set_timeout(rech_socket_t s, long long ms) {
#ifdef _WIN32
    DWORD tv = (DWORD)(ms > 0 ? ms : 0);
    setsockopt(s, SOL_SOCKET, SO_RCVTIMEO, (const char *)&tv, sizeof(tv));
#else
    struct timeval tv;
    tv.tv_sec = ms > 0 ? ms / 1000 : 0;
    tv.tv_usec = ms > 0 ? (ms %% 1000) * 1000 : 0;
    setsockopt(s, SOL_SOCKET, SO_RCVTIMEO, &tv, sizeof(tv));
#endif
}

// rech_listen_udp - bind БЕЗ listen()/connect() (UDP не различает
// "клиент"/"сервер" на уровне протокола, у datagram-сокета нет
// соединения вообще) - готовит сокет для recvfrom/sendto с адресом
// ПРОИЗВОЛЬНОГО отправителя (см. udpDatagramHelpers) вместо
// rech_connect_udp ниже, который фиксирует ОДНОГО адресата через connect().
static rech_socket_t rech_listen_udp(long long port) {
#ifndef _WIN32
    signal(SIGPIPE, SIG_IGN);
#endif
    rech_socket_t s = socket(AF_INET, SOCK_DGRAM, 0);
    if (s == RECH_INVALID_SOCKET) {
        rech_panic("%[1]s");
    }
    int yes = 1;
    setsockopt(s, SOL_SOCKET, SO_REUSEADDR, (const char *)&yes, sizeof(yes));
    struct sockaddr_in addr;
    memset(&addr, 0, sizeof(addr));
    addr.sin_family = AF_INET;
    addr.sin_addr.s_addr = INADDR_ANY;
    addr.sin_port = htons((unsigned short)port);
    if (bind(s, (struct sockaddr *)&addr, sizeof(addr)) != 0) {
        rech_panic("%[2]s");
    }
    return s;
}

static rech_socket_t rech_connect(const char *host, long long port, int type) {
#ifndef _WIN32
    signal(SIGPIPE, SIG_IGN);
#endif
    struct addrinfo hints, *res;
    memset(&hints, 0, sizeof(hints));
    hints.ai_family = AF_INET;
    hints.ai_socktype = type;
    char portbuf[16];
    snprintf(portbuf, sizeof(portbuf), "%%lld", port);
    if (getaddrinfo(host, portbuf, &hints, &res) != 0) {
        rech_panic("%[5]s");
    }
    rech_socket_t s = socket(res->ai_family, res->ai_socktype, res->ai_protocol);
    if (s == RECH_INVALID_SOCKET) {
        freeaddrinfo(res);
        rech_panic("%[6]s");
    }
    if (connect(s, res->ai_addr, (int)res->ai_addrlen) != 0) {
        freeaddrinfo(res);
        rech_panic("%[7]s");
    }
    freeaddrinfo(res);
    return s;
}

static rech_socket_t rech_connect_tcp(const char *host, long long port) {
    return rech_connect(host, port, SOCK_STREAM);
}

static rech_socket_t rech_connect_udp(const char *host, long long port) {
    return rech_connect(host, port, SOCK_DGRAM);
}

// rech_network_node - "сетевой_узел(режим, хост, порт)" - единая точка
// входа вместо отдельных rech_listen_tcp/rech_connect_tcp/rech_connect_udp/
// rech_listen_udp выше: host[0] == '\0' решает "слушать" (bind, для tcp -
// ещё и listen), непустой host - "подключиться" (connect). mode строкой -
// неизвестный режим (не "tcp"/"udp") - паника в рантайме, не ошибка
// компиляции (строковый параметр решается только здесь).
static rech_socket_t rech_network_node(const char *mode, const char *host, long long port) {
    bool listening = host[0] == '\0';
    if (strcmp(mode, "tcp") == 0) {
        return listening ? rech_listen_tcp(port) : rech_connect_tcp(host, port);
    }
    if (strcmp(mode, "udp") == 0) {
        return listening ? rech_listen_udp(port) : rech_connect_udp(host, port);
    }
    rech_panic("%[11]s");
    return RECH_INVALID_SOCKET;
}
`, rtmsg.SocketCreateFailed, rtmsg.SocketBindFailed, rtmsg.SocketListenFailed, rtmsg.SocketAcceptFailed,
	rtmsg.SocketResolveFailed, rtmsg.SocketConnectCreate, rtmsg.SocketConnectFailed, rtmsg.SocketSendFailed,
	rtmsg.SocketRecvInvalidMax, rtmsg.SocketRecvFailed, rtmsg.SocketUnknownMode)

// plainSocketOpsCode - rech_accept/rech_send/rech_recv/rech_close_socket
// БЕЗ TLS - вариант, эмитируемый вместо tlsSocketOpsCode (tls.go), когда
// программа не использует "слушать_tcp_защищённо" (см. package doc
// socketTypeAndConnectCode выше и ir_generator.go, выбор socketHelpers/
// socketHelpersTLS по ctx.usesTLS).
var plainSocketOpsCode = fmt.Sprintf(`static %[5]s rech_accept(rech_socket_t s) {
    rech_socket_t c = accept(s, NULL, NULL);
    if (c == RECH_INVALID_SOCKET) {
#ifdef _WIN32
        if (WSAGetLastError() == WSAETIMEDOUT) {
            return %[5]s_empty();
        }
#else
        // EINTR - accept() прерван доставкой сигнала (например,
        // rech_shutdown_signal_handler, shutdown.go) - настоящей ошибки нет,
        // просто соединения пока не было, тот же смысл, что у настоящего
        // таймаута (EAGAIN/EWOULDBLOCK): "принять" возвращает Ничто, цикл
        // сервера сам решает, продолжать ли ждать (пока не запрошена_
        // остановка) - именно так сигнал прерывает ожидание НЕМЕДЛЕННО, не
        // дожидаясь остатка установленного "установить_таймаут".
        if (errno == EAGAIN || errno == EWOULDBLOCK || errno == EINTR) {
            return %[5]s_empty();
        }
#endif
        rech_panic("%[1]s");
    }
    return %[5]s_wrap(c);
}

static void rech_send(rech_socket_t s, const char *data) {
    size_t len = strlen(data);
    size_t sent = 0;
    while (sent < len) {
        long long n = send(s, data + sent, (int)(len - sent), 0);
        if (n <= 0) {
            rech_panic("%[2]s");
        }
        sent += (size_t)n;
    }
}

// rech_socket_read_raw - ЕДИНСТВЕННАЯ точка настоящего чтения байт из
// потокового (TCP) сокета - и rech_recv ниже (публичная встроенная
// функция "получить"), и rech_recv_http_request (httpserver.go), и
// rech_recv_until_close (httpHelpers, HTTP-клиент "запрос") зовут ИМЕННО
// её, не recv() напрямую - иначе TLS-осведомлённая версия этой функции
// (tlsSocketOpsCode, tls.go) не покрывала бы чтение внутри "принять_http"/
// "запрос", а голый recv() там читал бы ещё зашифрованные байты TLS-
// записи вместо расшифрованного HTTP-текста (ровно так был найден баг -
// "принять_http" зависал в ожидании "\r\n\r\n", которого в шифротексте
// нет и не будет). Сигнатура/семантика возврата - как у recv() (число
// байт, 0 - собеседник закрыл соединение, отрицательное - ошибка).
static long long rech_socket_read_raw(rech_socket_t s, char *buf, long long max) {
    return recv(s, buf, (int)max, 0);
}

static char *rech_recv(rech_socket_t s, long long max) {
    if (max <= 0) {
        rech_panic("%[3]s");
    }
    char *buf = malloc((size_t)max + 1);
    long long n = rech_socket_read_raw(s, buf, max);
    if (n < 0) {
        free(buf);
        rech_panic("%[4]s");
    }
    buf[n] = '\0';
    return buf;
}

static void rech_close_socket(rech_socket_t s) {
#ifdef _WIN32
    closesocket(s);
#else
    close(s);
#endif
}
`, rtmsg.SocketAcceptFailed, rtmsg.SocketSendFailed, rtmsg.SocketRecvInvalidMax, rtmsg.SocketRecvFailed, mangleType("?Сокет"))

// socketHelpers - сетевые примитивы БЕЗ TLS: TCP-сервер (rech_listen_tcp +
// rech_accept), UDP-сервер (rech_listen_udp - bind без connect/listen,
// дальше recvfrom/sendto - см. udpDatagramHelpers), TCP/UDP-клиент
// (rech_connect_tcp/rech_connect_udp - для UDP через connect() на
// datagram-сокете, что фиксирует адрес получателя и позволяет дальше
// использовать один и тот же send/recv, что и у TCP), rech_send/rech_recv/
// rech_close_socket. rech_network_node - единая точка входа
// ("сетевой_узел(режим, хост, порт)"): host[0] == '\0' решает "слушать"
// вместо "подключиться", mode строкой ("tcp"/"udp") - какой из четырёх
// вариантов. rech_socket_t - int на POSIX, SOCKET на Windows (оба -
// непрозрачные дескрипторы, как rech_file_t выше, только другого
// системного API). См. socketHelpersTLS (tls.go) - вариант этого же текста
// для программы, использующей "слушать_tcp_защищённо".
var socketHelpers = socketTypeAndConnectCode + plainSocketOpsCode

// httpHelpers - "запрос" (шаг 9, HTTP-клиент типа curl): строит HTTP/1.1-
// запрос и парсит ответ поверх уже существующих rech_connect_tcp/rech_send/
// rech_close_socket (socketHelpers) - не отдельный сетевой механизм.
// "Запрос"/"Ответ" - фиксированные встроенные сущности (internal/semantic.
// registerHTTPEntity), их typedef уже эмитится общим циклом по
// Program.Entities раньше этого блока - здесь только ссылки на их
// mangleType/mangleField имена (вычисленные, не написанные вручную -
// исключает ошибку транслитерации). "Connection: close" в каждом запросе -
// вместо keep-alive/chunked transfer encoding, которые этот срез не
// разбирает: ответ читается до закрытия соединения сервером
// (rech_recv_until_close), не до предполагаемой длины - известное текущее
// ограничение (см. package doc internal/builtins.ReservedFuncNames и
// docs/ROADMAP.md) - сервер, отвечающий chunked без Content-Length,
// распарсится неверно (тело останется как есть, без де-чанкинга).
// rechDupHelper - rech_dup (копия C-строки в новый malloc-буфер) - общий
// для HTTP-клиента (httpHelpers) и HTTP-сервера (httpServerHelpers,
// httpserver.go) - эмитится один раз, когда нужен хотя бы один из них
// (usesHTTP || usesHTTPServer), см. ir_generator.go.
const rechDupHelper = `static char *rech_dup(const char *s) {
    size_t len = strlen(s);
    char *out = malloc(len + 1);
    memcpy(out, s, len + 1);
    return out;
}

`

var httpHelpers = fmt.Sprintf(`static char *rech_recv_until_close(rech_socket_t s) {
    size_t cap = 4096, len = 0;
    char *buf = malloc(cap);
    for (;;) {
        if (len + 1 >= cap) {
            cap *= 2;
            buf = realloc(buf, cap);
        }
        long long n = rech_socket_read_raw(s, buf + len, (long long)(cap - len - 1));
        if (n < 0) {
            free(buf);
            rech_panic("%[10]s");
        }
        if (n == 0) {
            break;
        }
        len += (size_t)n;
    }
    buf[len] = '\0';
    return buf;
}

static %[7]s rech_http_request(%[8]s req) {
    rech_socket_t s = rech_connect_tcp(req.%[3]s, req.%[5]s);
    size_t body_len = strlen(req.%[6]s);
    const char *hdr_sep = req.%[4]s[0] ? "\r\n" : "";
    int need = snprintf(NULL, 0, "%%s %%s HTTP/1.1\r\nHost: %%s\r\n%%s%%sConnection: close\r\nContent-Length: %%lld\r\n\r\n",
        req.%[1]s, req.%[2]s, req.%[3]s, req.%[4]s, hdr_sep, (long long)body_len);
    char *head = malloc((size_t)need + 1);
    snprintf(head, (size_t)need + 1, "%%s %%s HTTP/1.1\r\nHost: %%s\r\n%%s%%sConnection: close\r\nContent-Length: %%lld\r\n\r\n",
        req.%[1]s, req.%[2]s, req.%[3]s, req.%[4]s, hdr_sep, (long long)body_len);
    char *full = malloc((size_t)need + body_len + 1);
    memcpy(full, head, (size_t)need);
    memcpy(full + need, req.%[6]s, body_len);
    full[need + body_len] = '\0';
    free(head);
    rech_send(s, full);
    free(full);

    char *raw = rech_recv_until_close(s);
    rech_close_socket(s);

    %[7]s resp;
    long long code = 0;
    const char *sp1 = strchr(raw, ' ');
    if (sp1) {
        code = strtoll(sp1 + 1, NULL, 10);
    }
    resp.%[9]s = code;
    const char *body_sep = strstr(raw, "\r\n\r\n");
    if (body_sep) {
        size_t header_len = (size_t)(body_sep - raw);
        char *headers = malloc(header_len + 1);
        memcpy(headers, raw, header_len);
        headers[header_len] = '\0';
        resp.%[4]s = headers;
        resp.%[6]s = rech_dup(body_sep + 4);
    } else {
        resp.%[4]s = rech_dup("");
        resp.%[6]s = rech_dup("");
    }
    free(raw);
    return resp;
}
`, mangleField("метод"), mangleField("путь"), mangleField("адрес"), mangleField("заголовки"),
	mangleField("порт"), mangleField("тело"), mangleType("Ответ"), mangleType("Запрос"),
	mangleField("код"), rtmsg.HTTPRecvFailed)
