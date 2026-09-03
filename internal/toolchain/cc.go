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
func (*CC) CompileC(cPath, binPath string, extraLibs []string) error {
	src, err := os.ReadFile(cPath)
	if err != nil {
		return fmt.Errorf("не удалось прочитать сгенерированный файл %q: %w", cPath, err)
	}
	args := append([]string{cPath, "-o", binPath}, commonLinkFlags(src)...)
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
	var missing []string
	for _, l := range knownDBLibs {
		mentionsLib := strings.Contains(lower, "-l"+l.lib) || strings.Contains(lower, l.lib+".lib")
		mentionsMissing := strings.Contains(lower, "find") || strings.Contains(lower, "found") || strings.Contains(lower, "cannot open")
		if mentionsLib && mentionsMissing {
			missing = append(missing, l.lib)
		}
	}
	if len(missing) == 0 {
		return ""
	}
	var sb strings.Builder
	sb.WriteString("похоже, не хватает клиентской библиотеки БД для линковки:\n")
	for _, lib := range missing {
		fmt.Fprintf(&sb, "  - %s: %s\n", lib, installHintFor(lib))
	}
	return strings.TrimRight(sb.String(), "\n")
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
