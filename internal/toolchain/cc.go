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

// CompileC компилирует cPath в исполняемый файл binPath.
func (*CC) CompileC(cPath, binPath string, b compiler.CBuild) error {
	src, err := os.ReadFile(cPath)
	if err != nil {
		return fmt.Errorf("не удалось прочитать сгенерированный файл %q: %w", cPath, err)
	}
	return runCC(b, cPath, binPath, defaultLinkFlags(src, b))
}

// defaultLinkFlags - флаги линковки, которые компилятор ставит сам:
// автоматически найденные библиотеки (autoLinkFlags) плюс внешние
// библиотеки проекта (сборка.путь_библиотек/сборка.библиотеки).
func defaultLinkFlags(src []byte, b compiler.CBuild) []string {
	flags := autoLinkFlags(src, runtime.GOOS)
	flags = append(flags, libPathFlags(b.LibPaths)...)
	return append(flags, libFlags(b.Libs)...)
}

// libFlags превращает имена внешних библиотек ("curl", "ssl") в флаги
// линковщика ("-lcurl", "-lssl").
func libFlags(names []string) []string {
	flags := make([]string, len(names))
	for i, name := range names {
		flags[i] = "-l" + name
	}
	return flags
}

// libPathFlags - каталоги внешних библиотек: "-L<путь>" и, на Linux/macOS,
// "-Wl,-rpath,<путь>" - динамический загрузчик найдёт библиотеку при
// запуске без LD_LIBRARY_PATH. На Windows у DLL нет аналога rpath - DLL
// ищется рядом с .exe или в PATH.
func libPathFlags(paths []string) []string {
	var flags []string
	for _, p := range paths {
		flags = append(flags, "-L"+p)
		if runtime.GOOS != "windows" {
			flags = append(flags, "-Wl,-rpath,"+p)
		}
	}
	return flags
}

// AutoLinkLib - библиотека, которую компилятор линкует сам, если
// сгенерированный C её использует. Marker - текст в сгенерированном C,
// по которому это видно ("" - всегда); OS - "windows"/"!windows"/"" (любая);
// Purpose - для чего она (шаблон .rechproj и документация перечисляют
// эту же таблицу, см. TestTemplateListsAllAutoLinkFlags).
type AutoLinkLib struct {
	Flags   []string
	Marker  string
	OS      string
	Purpose string
}

// AutoLinkLibs - в порядке появления в командной строке (-lssl раньше
// -lcrypto: libssl зависит от libcrypto).
var AutoLinkLibs = []AutoLinkLib{
	{Flags: []string{"-lm"}, Purpose: "математика (корень, степень, округлить...) - всегда"},
	{Flags: []string{"-lgc"}, Marker: "#include <gc.h>", Purpose: "сборщик мусора Boehm GC - сущности, коллекции, строки"},
	{Flags: []string{"-pthread"}, Marker: "#include <pthread.h>", OS: "!windows", Purpose: "потоки - запустить, Блокировка, Семафор, пул БД"},
	{Flags: []string{"-lsqlite3"}, Marker: "sqlite3_open(", Purpose: "БД SQLite"},
	{Flags: []string{"-lpq"}, Marker: "PQconnectdb(", Purpose: "БД PostgreSQL"},
	{Flags: []string{"-lmysqlclient"}, Marker: "mysql_init(", Purpose: "БД MySQL"},
	{Flags: []string{"-lssl", "-lcrypto"}, Marker: "#include <openssl/ssl.h>", Purpose: "TLS (слушать_tcp_защищённо, https, grpcs)"},
	{Flags: []string{"-lcrypto"}, Marker: "#include <openssl/evp.h>", Purpose: "шифрование, хеши, подписи без TLS"},
	{Flags: []string{"-lnghttp2"}, Marker: "#include <nghttp2/nghttp2.h>", Purpose: "HTTP/2 и gRPC"},
	{Flags: []string{"-lz"}, Marker: "#include <zlib.h>", Purpose: "gzip-сжатие gRPC-сообщений"},
	{Flags: []string{"-lpng"}, Marker: "#include <png.h>", Purpose: "PNG-изображения"},
	{Flags: []string{"-ljpeg"}, Marker: "#include <jpeglib.h>", Purpose: "JPEG-изображения"},
	{Flags: []string{"-lshell32", "-lws2_32"}, OS: "windows", Purpose: "Windows: аргументы командной строки (CommandLineToArgvW) и сокеты (Winsock2) - всегда"},
}

// autoLinkFlags - флаги AutoLinkLibs, нужные программе src на goos, без
// повторов. Библиотека, которой программа не пользуется, не линкуется:
// иначе программа без БД требовала бы установленных клиентов всех БД.
func autoLinkFlags(src []byte, goos string) []string {
	var flags []string
	seen := map[string]bool{}
	for _, l := range AutoLinkLibs {
		switch {
		case l.OS == "windows" && goos != "windows", l.OS == "!windows" && goos == "windows":
			continue
		case l.Marker != "" && !bytes.Contains(src, []byte(l.Marker)):
			continue
		}
		for _, f := range l.Flags {
			if !seen[f] {
				seen[f] = true
				flags = append(flags, f)
			}
		}
	}
	return flags
}

// windowsGUIFlag - оконная подсистема Windows (MinGW/clang): исполняемый
// файл без консоли.
const windowsGUIFlag = "-mwindows"

// ccArgs - аргументы компилятора C для runCC (goos - целевая ОС).
func ccArgs(b compiler.CBuild, cPath, out string, defaultLink []string, goos string, info ccInfo) []string {
	args := cFlags(b, info)
	args = append(args, cPath, "-o", out)
	if b.LinkFlagsSet {
		args = append(args, b.LinkFlags...)
	} else {
		args = append(args, defaultLink...)
		if b.Release && info.family != "" {
			args = append(args, releaseLinkFlags(goos)...)
		}
	}
	// Подсистема - часть "сборка.тип", не флагов линковки: добавляется и
	// тогда, когда флаги_линковки заданы в проекте целиком.
	if b.Windowed && goos == "windows" {
		args = append(args, windowsGUIFlag)
	}
	return args
}

// runCC запускает компилятор C: "<флаги C> <cPath> -o <out> <флаги
// линковки>". Флаги проекта (b.CFlags/b.LinkFlags), если заданы, заменяют
// встроенные целиком (см. compiler.CBuild, cFlags).
func runCC(b compiler.CBuild, cPath, out string, defaultLink []string) error {
	path, err := findCCompiler(b.CCompiler)
	if err != nil {
		return err
	}
	info := identifyCC(path)
	if err := checkCCVersion(path, info); err != nil {
		return err
	}
	args := ccArgs(b, cPath, out, defaultLink, runtime.GOOS, info)
	// #nosec G204 -- пути приходят от локального пользователя, вызывающего свой же компилятор.
	cmd := exec.Command(path, args...)
	output, err := cmd.CombinedOutput()
	if err != nil {
		if hint := missingLibHint(string(output)); hint != "" {
			return fmt.Errorf("%s завершился с ошибкой: %w\n%s\n%s", path, err, string(output), hint)
		}
		return fmt.Errorf("%s завершился с ошибкой: %w\n%s", path, err, string(output))
	}
	return nil
}

// knownDBLibs - собственное имя (флаг -l<имя> в args) и человекочитаемое
// название СУБД, для которой оно нужно (missingLibHint ниже) - те же три
// библиотеки, что commonLinkFlags решает линковать по dbscheme.Set. Только
// они (не -lm/-lgc/-pthread) реально отсутствуют на типичной системе без
// отдельной установки ("libpq-dev/libmysqlclient-dev
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
	// -lnghttp2 (HTTP/2/gRPC-транспорт, http2.go) - тот же класс подсказки,
	// что SSL/PNG/JPEG выше - ПРОБЕЛ, найденный при аудите установщиков
	//: у missingLibHint были ветки для всех трёх новых
	// внешних библиотек, добавленных Шагами 93-107 (SSL/PNG/JPEG), кроме
	// именно этой - nghttp2 (Шаги 93-97, самый ранний из трёх) остался без
	// подсказки, хотя линковка того же вида ("cannot find -lnghttp2") уже
	// возможна с самого их появления.
	mentionsNghttp2 := strings.Contains(lower, "-lnghttp2") || strings.Contains(lower, "nghttp2.lib") || strings.Contains(lower, "nghttp2/nghttp2.h")
	if mentionsNghttp2 && (mentionsMissing || strings.Contains(lower, "no such file")) {
		fmt.Fprintf(&hint, "похоже, не хватает nghttp2 (HTTP/2/gRPC-транспорт): %s\n", installHintFor("nghttp2"))
	}
	// -lz (gzip для gRPC-сообщений, grpc.go) - тот же класс подсказки, что
	// nghttp2 выше - идёт вместе с ним (оба нужны, как только программа
	// использует gRPC вообще), но отдельным маркером/сообщением - разные
	// пакеты на всех трёх платформах.
	mentionsZlib := strings.Contains(lower, "-lz") || strings.Contains(lower, "zlib.lib") || strings.Contains(lower, "zlib.h")
	if mentionsZlib && (mentionsMissing || strings.Contains(lower, "no such file")) {
		fmt.Fprintf(&hint, "похоже, не хватает zlib (gzip для gRPC-сообщений): %s\n", installHintFor("zlib"))
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
		"nghttp2": {
			"linux":   "sudo apt install libnghttp2-dev (Debian/Ubuntu) или sudo dnf install libnghttp2-devel (Fedora)",
			"darwin":  "brew install nghttp2",
			"windows": "vcpkg install nghttp2 (или MSYS2: pacman -S mingw-w64-x86_64-libnghttp2)",
		},
		"zlib": {
			"linux":   "sudo apt install zlib1g-dev (Debian/Ubuntu) или sudo dnf install zlib-devel (Fedora)",
			"darwin":  "brew install zlib (обычно уже есть в системе)",
			"windows": "vcpkg install zlib (или MSYS2: pacman -S mingw-w64-x86_64-zlib)",
		},
	}
	if perOS, ok := hints[lib]; ok {
		if hint, ok := perOS[runtime.GOOS]; ok {
			return hint
		}
	}
	return "установите клиентскую библиотеку/dev-пакет для lib" + lib
}

// findCCompiler ищет компилятор C: сборка.компилятор_c (explicit), затем
// переменную окружения CC, затем стандартные имена в PATH.
func findCCompiler(explicit string) (string, error) {
	if explicit != "" {
		path, err := exec.LookPath(explicit)
		if err != nil {
			return "", fmt.Errorf("компилятор C %q из сборка.компилятор_c не найден: %w", explicit, err)
		}
		return path, nil
	}
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
	return "", fmt.Errorf("не найден C-компилятор (проверялись сборка.компилятор_c, $CC, cc, clang, gcc) - Rech транспилирует в C и требует системный компилятор для финальной сборки")
}
