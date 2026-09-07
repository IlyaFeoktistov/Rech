// Package toolchain реализует compiler.Toolchain через системный
// C-компилятор (cc/clang/gcc), которому передаётся сгенерированный код.
package toolchain

import (
	"bytes"
	"fmt"
	"os"
	"os/exec"
	"runtime"
	"strings"

	"github.com/IlyaFeoktistov/Rech/internal/compiler"
)

// CC вызывает системный C-компилятор для сборки нативного бинарника или
// разделяемой библиотеки (CompileLib, lib.go).
type CC struct{}

// NewCC - провайдер для Wire.
func NewCC() compiler.Toolchain { return &CC{} }

// CompileC компилирует cPath в исполняемый файл binPath. extraLibs -
// внешние библиотеки для линковки (см. package doc CompileLib) - "" (или
// nil), если проект их не объявил (сборка.библиотеки, internal/project).
// extraLibPaths - см. libPathFlags.
func (*CC) CompileC(cPath, binPath string, extraLibs, extraLibPaths []string) error {
	src, err := os.ReadFile(cPath)
	if err != nil {
		return fmt.Errorf("не удалось прочитать сгенерированный файл %q: %w", cPath, err)
	}
	args := append([]string{cPath, "-o", binPath}, commonLinkFlags(src)...)
	args = append(args, libPathFlags(extraLibPaths)...)
	args = append(args, libFlags(extraLibs)...)
	return runCC(args)
}

// libFlags превращает имена внешних библиотек ("curl", "ssl") в флаги
// линковщика ("-lcurl", "-lssl") - то же соглашение об именах, что и у
// "-lm"/"-lgc" в commonLinkFlags, только имя приходит не отсюда, а из
// "сборка.библиотеки" файла проекта (Rech-код объявляет лишь сигнатуру
// функции "внешняя функция", без привязки к конкретной библиотеке - см.
// package doc ast.ExternFuncDecl).
func libFlags(names []string) []string {
	flags := make([]string, len(names))
	for i, name := range names {
		flags[i] = "-l" + name
	}
	return flags
}

// libPathFlags превращает локальные каталоги ("сборка.путь_библиотек"
// файла проекта, уже абсолютные - internal/project.File.ПутиБиблиотек) в
// флаги линковщика: "-L<путь>" (сама линковка ищет "-l<имя>" не только в
// системных путях) и, на Linux/macOS, дополнительно "-Wl,-rpath,<путь>" -
// чтобы уже СОБРАННЫЙ бинарник САМ нашёл разделяемую библиотеку в момент
// ЗАПУСКА, без ручного LD_LIBRARY_PATH (тот же принцип, которым настоящие
// C/C++-проекты решают "зависимость лежит рядом, не установлена в
// систему"). На Windows у DLL нет аналога rpath - только "-L" помогает
// самой линковке, поиск .dll при запуске - PATH/каталог рядом с .exe
// (известное ограничение, тот же класс, что и у остального FFI на
// Windows, docs/SPEC.md).
func libPathFlags(paths []string) []string {
	flags := make([]string, 0, len(paths)*2)
	for _, p := range paths {
		flags = append(flags, "-L"+p)
		if runtime.GOOS != "windows" {
			flags = append(flags, "-Wl,-rpath,"+p)
		}
	}
	return flags
}

// commonLinkFlags - флаги cc, общие для CompileC (исполняемый файл) и
// CompileLib (разделяемая библиотека, lib.go) - решение "нужен ли этот
// флаг" зависит только от содержимого сгенерированного C (src) и платформы,
// не от того, что именно из него в итоге собирается.
func commonLinkFlags(src []byte) []string {
	// -lm - математика (internal/builtins: корень/степень/округлить/
	// потолок/пол/модуль) может быть не слинкована без этого флага в libc,
	// не предоставляющей math.h-функции сама по себе; передаётся всегда, а
	// не только когда программа реально использует эти функции - линковка
	// неиспользуемой библиотеки безвредна, а решать здесь "нужна ли она"
	// означало бы протаскивать это знание из cbackend через весь конвейер
	// компиляции только ради этого одного флага.
	flags := []string{"-lm"}
	// -lgc - Boehm GC ("сущность"/"класс" - ссылочная семантика по
	// умолчанию, GC_MALLOC в сгенерированном C) - в отличие от -lm, НЕ
	// входит в состав стандартного C-тулчейна ни на одной платформе
	// (проверено эмпирически: ни на свежем Ubuntu, ни на MinGW-w64 её нет
	// без отдельной установки) - линковать безусловно, как -lm, означало
	// бы, что даже программа без единой сущности не соберётся без
	// libgc-dev/bdw-gc/bdwgc. cbackend подключает "#include <gc.h>" в
	// сгенерированном файле, только если в программе есть хотя бы одна
	// ссылочная сущность (ctx.usesGC, см. internal/backend/cbackend/
	// ir_generator.go) - этого же признака в самом файле достаточно, чтобы
	// решить, нужен ли -lgc, без протаскивания отдельного флага через
	// весь internal/compiler.Toolchain только ради этого.
	if bytes.Contains(src, []byte("#include <gc.h>")) {
		flags = append(flags, "-lgc")
	}
	// -pthread - потоки ОС ("запустить"/"дождаться" -
	// internal/backend/cbackend/concurrency.go) - только на POSIX, где
	// pthread_create и не является частью libc по умолчанию на всех
	// платформах (glibc сама по себе линкует его безусловно с недавних
	// версий, но не гарантированно на каждом POSIX-тулчейне) - на Windows
	// используется CreateThread (kernel32, уже линкуется по умолчанию),
	// поэтому здесь ограничено !windows, а не просто наличием
	// "#include <pthread.h>" в файле (тот присутствует в тексте всегда,
	// когда используются потоки, независимо от платформы - см. package doc
	// cbackend/ir_generator.go).
	if bytes.Contains(src, []byte("#include <pthread.h>")) && runtime.GOOS != "windows" {
		flags = append(flags, "-pthread")
	}
	// -lsqlite3/-lpq/-lmysqlclient - клиентские библиотеки БД
	// (internal/backend/cbackend/db.go) - тот же принцип, что и -lgc выше:
	// не входят в стандартный C-тулчейн, линкуются только когда программа
	// реально использует "подключиться_к_бд"/"запросить"/"выполнить"/
	// "закрыть_бд" (по маркеру в самом сгенерированном файле, а не по
	// отдельному протащенному флагу). В отличие от -lgc/-pthread выше -
	// ТРИ независимых маркера, не один: cbackend (internal/dbscheme.Set,
	// dbRuntimeCode) эмитирует C-код только для реально нужной(ых) СУБД
	// (по статическому анализу literal-схемы строки подключения) - линковать
	// все три библиотеки безусловно означало бы требовать
	// libsqlite3/libpq/libmysqlclient даже от программы, использующей
	// только одну (docs/ROADMAP.md, "toolchain линкует ... безусловно").
	// Маркер - имя функции подключения КОНКРЕТНОЙ библиотеки (не
	// "rech_db_connect(" - та эмитируется всегда при UsesDB, независимо от
	// того, какая именно СУБД внутри неё скомпилирована) - "sqlite3_open("/
	// "PQconnectdb("/"mysql_init(" присутствуют в сгенерированном файле
	// ТОЛЬКО когда dbRuntimeCode реально включила соответствующий блок.
	if bytes.Contains(src, []byte("sqlite3_open(")) {
		flags = append(flags, "-lsqlite3")
	}
	if bytes.Contains(src, []byte("PQconnectdb(")) {
		flags = append(flags, "-lpq")
	}
	if bytes.Contains(src, []byte("mysql_init(")) {
		flags = append(flags, "-lmysqlclient")
	}
	// -lssl -lcrypto - OpenSSL (TLS для "слушать_tcp_защищённо", internal/
	// backend/cbackend/tls.go) - тот же принцип, что и у -lgc/клиентов БД
	// выше: не входит в стандартный C-тулчейн, линкуется только когда
	// программа реально использует TLS (по маркеру "#include
	// <openssl/ssl.h>" в сгенерированном файле - тот эмитируется только при
	// ctx.usesTLS, см. ir_generator.go). -lssl ДО -lcrypto - libssl зависит
	// от libcrypto, порядок статической линковки на некоторых линковщиках
	// (GNU ld без --start-group) значим.
	usesOpenSSLSSL := bytes.Contains(src, []byte("#include <openssl/ssl.h>"))
	if usesOpenSSLSSL {
		flags = append(flags, "-lssl", "-lcrypto")
	}
	// -lcrypto (без -lssl) - AES-256-GCM ("зашифровать"/"расшифровать",
	// internal/backend/cbackend/aes.go) - только EVP/RAND (libcrypto), не
	// TLS-рукопожатие (libssl) - по маркеру "#include <openssl/evp.h>"
	// (ctx.usesAES). Если TLS уже добавил "-lcrypto" веткой выше - не
	// дублировать флаг.
	if !usesOpenSSLSSL && bytes.Contains(src, []byte("#include <openssl/evp.h>")) {
		flags = append(flags, "-lcrypto")
	}
	// -lnghttp2 - HTTP/2-транспорт (internal/backend/cbackend/http2.go) -
	// тот же принцип, что и OpenSSL выше: не входит в стандартный C-тулчейн,
	// линкуется только когда программа реально вызвала одну из
	// http2_*-функций (по маркеру "#include <nghttp2/nghttp2.h>", ctx.
	// usesHTTP2).
	if bytes.Contains(src, []byte("#include <nghttp2/nghttp2.h>")) {
		flags = append(flags, "-lnghttp2")
	}
	// -lpng - PNG-кодек ("png_декодировать_файл"/"png_кодировать_файл",
	// internal/backend/cbackend/png.go) - тот же принцип, что и OpenSSL/
	// nghttp2 выше, по маркеру "#include <png.h>" (ctx.usesPNG).
	if bytes.Contains(src, []byte("#include <png.h>")) {
		flags = append(flags, "-lpng")
	}
	// -ljpeg - JPEG-кодек ("jpeg_декодировать_файл"/"jpeg_кодировать_файл",
	// internal/backend/cbackend/jpeg.go) - по маркеру "#include
	// <jpeglib.h>" (ctx.usesJPEG).
	if bytes.Contains(src, []byte("#include <jpeglib.h>")) {
		flags = append(flags, "-ljpeg")
	}
	if runtime.GOOS == "windows" {
		// -lshell32 - CommandLineToArgvW (internal/backend/cbackend.
		// argFuncHelper/rech_init_args): argv, который сам даёт CRT на
		// Windows, - в ANSI-кодовой странице процесса, не UTF-8, поэтому
		// "аргумент"/"количество_аргументов" перечитывают его сами через
		// CommandLineToArgvW - без -lshell32 сборка не слинкуется. runtime.
		// GOOS здесь - платформа, на которой на самом деле запущен сам
		// rech (не кросс-компиляция C-вывода) - тот же компилятор cc и
		// собирает, и запускается на одной и той же ОС.
		// -lws2_32 - Winsock2 (internal/backend/cbackend.
		// socketHelpers): сокеты на Windows - отдельная библиотека, не
		// часть обычной libc, как BSD-сокеты на POSIX. Тот же принцип, что
		// и у -lm выше - всегда, безвредно при отсутствии реального
		// использования сокетов в программе.
		flags = append(flags, "-lshell32", "-lws2_32")
	}
	return flags
}

// runCC запускает системный компилятор с готовым списком аргументов -
// общий хвост CompileC/CompileLib.
func runCC(args []string) error {
	path, err := findCCompiler()
	if err != nil {
		return err
	}
	// #nosec G204 -- пути приходят от локального пользователя, вызывающего свой же компилятор.
	cmd := exec.Command(path, args...)
	out, err := cmd.CombinedOutput()
	if err != nil {
		if hint := missingLibHint(string(out)); hint != "" {
			return fmt.Errorf("%s завершился с ошибкой: %w\n%s\n%s", path, err, string(out), hint)
		}
		return fmt.Errorf("%s завершился с ошибкой: %w\n%s", path, err, string(out))
	}
	return nil
}

// knownDBLibs - собственное имя (флаг -l<имя> в args) и человекочитаемое
// название СУБД, для которой оно нужно (missingLibHint ниже) - те же три
// библиотеки, что commonLinkFlags решает линковать по dbscheme.Set. Только
// они (не -lm/-lgc/-pthread) реально отсутствуют на типичной системе без
// отдельной установки (docs/ROADMAP.md - "libpq-dev/libmysqlclient-dev
// часто не ставят, если нужен только SQLite").
var knownDBLibs = []struct{ lib, dbName string }{
	{"sqlite3", "SQLite"},
	{"pq", "PostgreSQL"},
	{"mysqlclient", "MySQL"},
}

// missingLibHint разбирает вывод линковщика ("cannot find -lX" у GNU ld,
// "library not found for -lX"/"library 'X' not found" у ld64 (macOS,
// clang), "cannot open file 'X.lib'"/"LNK1181"/"LNK1104" у link.exe (MSVC))
// на предмет упоминания одной из трёх клиентских библиотек БД - точный
// формат сообщения не парсится строго (три линковщика формулируют его
// по-разному и нестабильно между версиями), достаточно, что и имя
// библиотеки, и слово "find"/"found"/"open" встречаются рядом в тексте -
// ложное срабатывание здесь не опаснее лишней (верной) строки подсказки в
// уже показанном пользователю сыром выводе линковщика.
func missingLibHint(linkerOutput string) string {
	lower := strings.ToLower(linkerOutput)
	mentionsMissing := strings.Contains(lower, "find") || strings.Contains(lower, "found") || strings.Contains(lower, "cannot open")
	var missing []string
	for _, l := range knownDBLibs {
		mentionsLib := strings.Contains(lower, "-l"+l.lib) || strings.Contains(lower, l.lib+".lib")
		if mentionsLib && mentionsMissing {
			missing = append(missing, l.lib)
		}
	}
	var hint strings.Builder
	if len(missing) > 0 {
		hint.WriteString("похоже, не хватает клиентской библиотеки БД для линковки:\n")
		for _, lib := range missing {
			fmt.Fprintf(&hint, "  - %s: %s\n", lib, installHintFor(lib))
		}
	}
	// -lssl/-lcrypto (TLS, tls.go) - тот же класс подсказки, отдельно от
	// БД выше (не СУБД, другая формулировка сообщения) - libssl-dev
	// (заголовки) часто не установлен, даже если сама библиотека времени
	// исполнения (libssl3) уже есть в системе (apt ставит их отдельными
	// пакетами).
	mentionsSSL := strings.Contains(lower, "-lssl") || strings.Contains(lower, "ssl.lib") ||
		strings.Contains(lower, "-lcrypto") || strings.Contains(lower, "crypto.lib") ||
		strings.Contains(lower, "openssl/ssl.h")
	if mentionsSSL && (mentionsMissing || strings.Contains(lower, "no such file")) {
		fmt.Fprintf(&hint, "похоже, не хватает OpenSSL для TLS (\"слушать_tcp_защищённо\"): %s\n", installHintFor("ssl"))
	}
	// -lpng/-ljpeg (PNG/JPEG-кодек, png.go/jpeg.go) - тот же класс
	// подсказки, что и SSL выше - libpng-dev/libjpeg-dev (заголовки) часто
	// не установлены отдельно от рантайм-библиотек.
	mentionsPNG := strings.Contains(lower, "-lpng") || strings.Contains(lower, "png.lib") || strings.Contains(lower, "png.h")
	if mentionsPNG && (mentionsMissing || strings.Contains(lower, "no such file")) {
		fmt.Fprintf(&hint, "похоже, не хватает libpng (\"png_декодировать_файл\"/\"png_кодировать_файл\"): %s\n", installHintFor("png"))
	}
	mentionsJPEG := strings.Contains(lower, "-ljpeg") || strings.Contains(lower, "jpeg.lib") || strings.Contains(lower, "jpeglib.h")
	if mentionsJPEG && (mentionsMissing || strings.Contains(lower, "no such file")) {
		fmt.Fprintf(&hint, "похоже, не хватает libjpeg (\"jpeg_декодировать_файл\"/\"jpeg_кодировать_файл\"): %s\n", installHintFor("jpeg"))
	}
	return strings.TrimRight(hint.String(), "\n")
}

// installHintFor - подсказка по установке одной из трёх клиентских
// библиотек, для трёх основных платформ сразу (runtime.GOOS здесь -
// платформа, на которой запущен сам rech, см. commonLinkFlags) - каждая
// СУБД называется своим отдельным набором пакетов на Linux/macOS/Windows,
// общей команды нет.
func installHintFor(lib string) string {
	hints := map[string]map[string]string{
		"sqlite3": {
			"linux":   "sudo apt install libsqlite3-dev (Debian/Ubuntu) или sudo dnf install sqlite-devel (Fedora)",
			"darwin":  "brew install sqlite3",
			"windows": "vcpkg install sqlite3 (или скачайте sqlite3.lib/sqlite3.dll с sqlite.org)",
		},
		"pq": {
			"linux":   "sudo apt install libpq-dev (Debian/Ubuntu) или sudo dnf install libpq-devel (Fedora)",
			"darwin":  "brew install libpq (может понадобиться brew link --force libpq)",
			"windows": "vcpkg install libpq (входит в поставку PostgreSQL - libpq.lib из каталога lib установщика)",
		},
		"mysqlclient": {
			"linux":   "sudo apt install libmysqlclient-dev (Debian/Ubuntu) или sudo dnf install mysql-devel (Fedora)",
			"darwin":  "brew install mysql-client",
			"windows": "vcpkg install libmysql (или MySQL Connector/C из поставки MySQL Server)",
		},
		"ssl": {
			"linux":   "sudo apt install libssl-dev (Debian/Ubuntu) или sudo dnf install openssl-devel (Fedora)",
			"darwin":  "brew install openssl (может понадобиться -L/-I на путь brew --prefix openssl)",
			"windows": "vcpkg install openssl",
		},
		"png": {
			"linux":   "sudo apt install libpng-dev (Debian/Ubuntu) или sudo dnf install libpng-devel (Fedora)",
			"darwin":  "brew install libpng",
			"windows": "vcpkg install libpng",
		},
		"jpeg": {
			"linux":   "sudo apt install libjpeg-dev (Debian/Ubuntu) или sudo dnf install libjpeg-turbo-devel (Fedora)",
			"darwin":  "brew install jpeg",
			"windows": "vcpkg install libjpeg-turbo",
		},
	}
	if perOS, ok := hints[lib]; ok {
		if hint, ok := perOS[runtime.GOOS]; ok {
			return hint
		}
	}
	return "установите клиентскую библиотеку/dev-пакет для lib" + lib
}

// findCCompiler ищет системный компилятор C: сначала переменную окружения
// CC, затем стандартные имена в PATH.
func findCCompiler() (string, error) {
	if cc := os.Getenv("CC"); cc != "" {
		if path, err := exec.LookPath(cc); err == nil {
			return path, nil
		}
	}
	for _, candidate := range []string{"cc", "clang", "gcc"} {
		if path, err := exec.LookPath(candidate); err == nil {
			return path, nil
		}
	}
	return "", fmt.Errorf("не найден C-компилятор (проверялись $CC, cc, clang, gcc) - Rech v0.1 транспилирует в C и требует системный компилятор для финальной сборки")
}
