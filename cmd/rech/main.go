// Command rech - компилятор языка Rech (v0.1).
//
// Использование:
//
//	rech build [--keep-c] [--release] <файл.rech>|<каталог>|-c "<код>"        - собрать бинарник
//	rech build [--keep-c] [--release] --lib <файл.rech>|<каталог>|-c "<код>"  - собрать разделяемую библиотеку (.so/.dylib/.dll) + C-заголовок (.h)
//	rech run [--keep-c] [--release] <файл.rech>|<каталог>|-c "<код>"          - собрать и выполнить
//	rech test [-j N] <файл.rech>|<каталог>                         - найти и запустить тесты (N процессов параллельно, по умолчанию - число ядер)
//	rech fmt [--write | --check] <файл.rech>|<каталог>             - канонический отступ, как gofmt; --check ничего не пишет, для CI
//	rech proto [-o <файл.rech>] <файл.proto>                       - импорт ".proto"-контракта в Rech-сущности с тегами `protobuf:N`
//	rech add <имя> <url> <версия>                                  - добавить внешнюю зависимость в .rechproj и сразу скачать её
//	rech install                                                   - скачать/обновить кеш внешних зависимостей (.rechproj, "зависимости:")
//	rech debug                                                     - сервер DAP по stdin/stdout (для VS Code)
//	rech lsp                                                       - сервер LSP по stdin/stdout (hover/автодополнение, для VS Code)
//	rech version                                                   - напечатать версию
//
// <каталог> у build/run (например, "." - как "go build .") собирает все
// ".rech"-файлы прямо в нём (без подкаталогов) как одну единицу компиляции -
// это не система импортов, а неявное объединение всего каталога, детали -
// internal/compiler.Compiler.parseDir.
//
// -c/--code "<код>" (как "python -c") собирает/запускает код прямо из
// аргумента командной строки, без файла - многострочный код передаётся
// обычной многострочной кавычкой шелла или heredoc'ом
// (rech run -c "$(cat <<'EOF' ... EOF)"), отдельной поддержки в самом rech
// для этого не нужно - строка приходит уже целиком.
//
// --release - релизная сборка (как "сборка.режим: релиз" в .rechproj):
// оптимизация, без отладочной информации и стека вызовов; по умолчанию -
// отладочная (#line на исходник Rech для gdb/lldb).
//
// <каталог> у test обходится рекурсивно, ищет файлы с суффиксом
// "_тест.rech" и запускает каждый через internal/interp (без вызова
// системного cc) в отдельном процессе - детали test.go, internal/testrunner.
//
// По умолчанию сгенерированный <файл>.c удаляется после успешной сборки -
// это техническая заготовка, а не то, что нужно смотреть. --keep-c
// оставляет его - пригодится, чтобы посмотреть, во что компилируется
// программа.
package main

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"

	"github.com/IlyaFeoktistov/Rech/internal/compiler"
	"github.com/IlyaFeoktistov/Rech/internal/debugadapter"
	"github.com/IlyaFeoktistov/Rech/internal/lsp"
	"github.com/IlyaFeoktistov/Rech/internal/project"
	"github.com/IlyaFeoktistov/Rech/internal/version"
)

func main() {
	if len(os.Args) < 2 {
		usage()
		os.Exit(2)
	}
	cmd := os.Args[1]

	if cmd == "version" {
		fmt.Println("rech", version.Version)
		return
	}

	if cmd == "init" {
		if err := initProject(os.Args[2:]); err != nil {
			fmt.Fprintln(os.Stderr, "ошибка:", err)
			os.Exit(1)
		}
		return
	}

	if cmd == "add" {
		if len(os.Args) < 5 {
			fmt.Fprintln(os.Stderr, "использование: rech add <имя> <url> <версия>")
			os.Exit(2)
		}
		if err := addDependency(os.Args[2], os.Args[3], os.Args[4]); err != nil {
			fmt.Fprintln(os.Stderr, "ошибка:", err)
			os.Exit(1)
		}
		return
	}

	if cmd == "install" {
		if err := installDeps(); err != nil {
			fmt.Fprintln(os.Stderr, "ошибка:", err)
			os.Exit(1)
		}
		return
	}

	if cmd == "debug" {
		// Без аргументов командной строки - в отличие от build/run/test,
		// цель отладки ("program") приходит не отсюда, а внутри самого
		// протокола DAP (запрос "launch"), VS Code запускает "rech debug"
		// как внешний процесс и говорит с ним через stdin/stdout сразу
		// после старта, до какого-либо файла. См. internal/debugadapter.
		if err := debugadapter.Run(os.Stdin, os.Stdout, strings.NewReader("")); err != nil {
			fmt.Fprintln(os.Stderr, "ошибка:", err)
			os.Exit(1)
		}
		return
	}

	if cmd == "lsp" {
		// Тем же принципом, что и "debug" выше - сервер протокола (LSP, не
		// DAP) по stdin/stdout, цель (какие файлы редактируются) приходит
		// внутри самого протокола (textDocument/didOpen), не как
		// CLI-аргумент. См. internal/lsp.
		if err := lsp.Run(os.Stdin, os.Stdout); err != nil {
			fmt.Fprintln(os.Stderr, "ошибка:", err)
			os.Exit(1)
		}
		return
	}

	if cmd == "test" {
		if len(os.Args) == 4 && os.Args[2] == testWorkerFlag {
			runTestWorker(os.Args[3])
			return
		}
		path, jobs, err := parseTestArgs(os.Args[2:])
		if err != nil {
			fmt.Fprintln(os.Stderr, "ошибка:", err)
			usage()
			os.Exit(2)
		}
		if !runTests(path, jobs) {
			os.Exit(1)
		}
		return
	}

	if cmd == "fmt" {
		if !runFmt(os.Args[2:]) {
			os.Exit(1)
		}
		return
	}

	if cmd == "proto" {
		if !runProtoImport(os.Args[2:]) {
			os.Exit(1)
		}
		return
	}

	args, err := parseBuildArgs(os.Args[2:])
	if err != nil {
		fmt.Fprintln(os.Stderr, "ошибка:", err)
		os.Exit(2)
	}
	srcPath, code, keepC, hasCode, isLib, release := args.srcPath, args.code, args.keepC, args.hasCode, args.isLib, args.release

	// Файл проекта (.rechproj) - либо явно передан вместо <файл.rech>|<каталог>,
	// либо (при полностью пустых аргументах) единственный в текущем каталоге -
	// см. cmd/rech/project.go. outDir/projName пусты, если проекта нет вовсе -
	// тогда ниже всё идёт как раньше, без переноса бинарника.
	var outDir, projName, libRoot string
	var extraLibs []string
	var extraLibPaths []string
	var extraHeaders []string
	var cBuild compiler.CBuild
	isService, serviceName := false, ""
	var optionsYAML string
	var projectDeps map[string]string
	var projectDepScopes map[string]map[string]string
	var runEnv map[string]string
	var dbPoolSize int
	var dbPoolTimeoutMs int
	var dbSQLiteBusyTimeoutMs int
	// poolShrinkEnabled по умолчанию true (пул воркер-потоков сжимается,
	// см. internal/project.File.PoolShrinkEnabled/Шаг 81) -
	// в отличие от dbPoolSize/dbPoolTimeoutMs (int, где 0 - естественный
	// сентинел "не задано"), у bool нет такого третьего состояния, поэтому
	// значение по умолчанию выставляется здесь явно, а не полагается на
	// нулевое значение Go - действует и без .rechproj вовсе (только
	// .rechproj с "потоки.сжатие_пула" ниже может его переопределить).
	poolShrinkEnabled := true
	var poolIdleTimeoutMs int
	var httpMaxRequestBytes int
	if !hasCode {
		projPath := srcPath
		if projPath == "" {
			p, ferr := project.Find(".")
			if ferr != nil {
				fmt.Fprintln(os.Stderr, "ошибка:", ferr)
				os.Exit(2)
			}
			projPath = p
		}
		if projPath != "" && filepath.Ext(projPath) == ".rechproj" {
			pf, perr := project.Load(projPath)
			if perr != nil {
				fmt.Fprintln(os.Stderr, "ошибка:", perr)
				os.Exit(1)
			}
			if perr = pf.CheckHostPlatform(); perr != nil {
				fmt.Fprintln(os.Stderr, "ошибка:", perr)
				os.Exit(1)
			}
			srcPath = pf.MainPath()
			// --keep-c/--lib/--release из командной строки файлом проекта не
			// отменяются: значение по умолчанию (false) неотличимо от явного
			// выбора пользователя.
			if !keepC {
				keepC = pf.KeepC()
			}
			if !isLib {
				isLib = pf.IsLibrary()
			}
			if !release {
				release = pf.IsRelease()
			}
			outDir, projName, libRoot = pf.OutputDir(), pf.Имя, pf.Dir
			runEnv = pf.Окружение
			if dbPoolSize, perr = pf.DBPoolSize(); perr != nil {
				fmt.Fprintln(os.Stderr, "ошибка:", perr)
				os.Exit(1)
			}
			if dbPoolTimeoutMs, perr = pf.DBPoolTimeoutMs(); perr != nil {
				fmt.Fprintln(os.Stderr, "ошибка:", perr)
				os.Exit(1)
			}
			if dbSQLiteBusyTimeoutMs, perr = pf.DBSQLiteBusyTimeoutMs(); perr != nil {
				fmt.Fprintln(os.Stderr, "ошибка:", perr)
				os.Exit(1)
			}
			if poolShrinkEnabled, perr = pf.PoolShrinkEnabled(); perr != nil {
				fmt.Fprintln(os.Stderr, "ошибка:", perr)
				os.Exit(1)
			}
			if poolIdleTimeoutMs, perr = pf.PoolIdleTimeoutMs(); perr != nil {
				fmt.Fprintln(os.Stderr, "ошибка:", perr)
				os.Exit(1)
			}
			if httpMaxRequestBytes, perr = pf.HTTPMaxRequestBytes(); perr != nil {
				fmt.Fprintln(os.Stderr, "ошибка:", perr)
				os.Exit(1)
			}
			// Библиотеки для линковки (-l<имя>) - только из файла проекта:
			// "внешняя функция" в .rech-коде не привязана к конкретной
			// библиотеке (см. package doc project.File.Библиотеки), значит без
			// .rechproj сборке неоткуда узнать список для линковки.
			extraLibs = pf.Библиотеки()
			extraLibPaths = pf.ПутиБиблиотек()
			extraHeaders = pf.Заголовки()
			cBuild.CCompiler = pf.КомпиляторC()
			cBuild.Windowed = pf.IsWindowed()
			cBuild.StaticLib = pf.IsStaticLibrary()
			isService = pf.IsService()
			serviceName = pf.ИмяСлужбы
			if cBuild.CFlags, cBuild.CFlagsSet, perr = pf.ФлагиC(); perr != nil {
				fmt.Fprintln(os.Stderr, "ошибка:", perr)
				os.Exit(1)
			}
			if cBuild.LinkFlags, cBuild.LinkFlagsSet, perr = pf.ФлагиЛинковки(); perr != nil {
				fmt.Fprintln(os.Stderr, "ошибка:", perr)
				os.Exit(1)
			}
			optionsYAML = pf.Опции
			if deps, scopes, depLibs, depLibPaths, derr := resolveDeps(pf); derr != nil {
				fmt.Fprintln(os.Stderr, "ошибка:", derr)
				os.Exit(1)
			} else {
				projectDeps = deps
				projectDepScopes = scopes
				// Библиотеки зависимостей (FFI-обёртки, сборка.библиотеки их
				// собственного .rechproj, включая закрытые (бинарные)
				// Rech-пакеты) - вместе с уже собранным списком САМОГО
				// проекта, без повторов (дедупликация ниже) - LibraryPaths
				// той же зависимости (её собственный каталог по умолчанию)
				// избавляет потребителя от ручного "путь_библиотек".
				extraLibs = mergeUnique(extraLibs, depLibs)
				extraLibPaths = mergeUnique(extraLibPaths, depLibPaths)
			}
		}
	}

	if !hasCode && srcPath == "" {
		usage()
		os.Exit(2)
	}

	comp := InitializeCompiler()
	comp.Warn = printWarning
	comp.Deps = projectDeps
	comp.DepScopes = projectDepScopes
	comp.DBPoolSize = dbPoolSize
	comp.DBPoolTimeoutMs = dbPoolTimeoutMs
	comp.DBSQLiteBusyTimeoutMs = dbSQLiteBusyTimeoutMs
	comp.PoolShrinkEnabled = poolShrinkEnabled
	comp.PoolIdleTimeoutMs = poolIdleTimeoutMs
	comp.HTTPMaxRequestBytes = httpMaxRequestBytes
	comp.ExternHeaders = extraHeaders
	comp.OptionsYAML = optionsYAML
	comp.CBuild = cBuild
	comp.Service = isService
	comp.Release = release
	build := func() (string, error) { return comp.Build(srcPath, keepC, extraLibs, extraLibPaths) }
	if outDir != "" {
		comp.LibName = projName // placeInOutputDir ниже переносит под этим именем
	}
	buildLib := func() (string, string, []string, error) {
		return comp.BuildLib(srcPath, libRoot, outDir, keepC, extraLibs, extraLibPaths)
	}
	if hasCode {
		build = func() (string, error) { return comp.BuildSource(code, keepC, extraLibs, extraLibPaths) }
		buildLib = func() (string, string, []string, error) {
			return comp.BuildLibSource(code, keepC, extraLibs, extraLibPaths)
		}
	}

	switch cmd {
	case "build":
		if isLib {
			libPath, hPath, rechiPaths, err := buildLib()
			if err != nil {
				printCompileError(err)
				os.Exit(1)
			}
			if outDir != "" {
				if libPath, err = placeInOutputDir(libPath, outDir, projName); err != nil {
					fmt.Fprintln(os.Stderr, "ошибка:", err)
					os.Exit(1)
				}
				if hPath, err = placeInOutputDir(hPath, outDir, projName); err != nil {
					fmt.Fprintln(os.Stderr, "ошибка:", err)
					os.Exit(1)
				}
				// rechiPaths уже лежат под outDir (comp.BuildLib получил его
				// как stubDir) - каждый по своему пути, зеркалящему структуру
				// исходников библиотеки (docs/SPEC.md, "Закрытые (бинарные)
				// пакеты") - переносить/переименовывать в projName, как
				// libPath/hPath выше, здесь бессмысленно (файлов может быть
				// много).
			}
			fmt.Println(libPath)
			fmt.Println(hPath)
			for _, p := range rechiPaths {
				fmt.Println(p)
			}
			return
		}
		binPath, err := build()
		if err != nil {
			printCompileError(err)
			os.Exit(1)
		}
		if outDir != "" {
			binPath, err = placeInOutputDir(binPath, outDir, projName)
			if err != nil {
				fmt.Fprintln(os.Stderr, "ошибка:", err)
				os.Exit(1)
			}
		}
		if hasCode || outDir != "" {
			// У -c/--code нет "рядом с исходником", а у файла проекта
			// бинарник переносится в каталог "выход" - в обоих случаях без
			// печати негде было бы догадаться, куда положен результат.
			fmt.Println(binPath)
		}
		if isService {
			paths, err := writeServiceFiles(binPath, projName, serviceName)
			if err != nil {
				fmt.Fprintln(os.Stderr, "ошибка:", err)
				os.Exit(1)
			}
			for _, p := range paths {
				fmt.Println(p)
			}
		}
	case "run":
		if isLib {
			fmt.Fprintln(os.Stderr, "ошибка: библиотека (--lib/сборка.тип: библиотека или статическая_библиотека) не исполняется как программа - соберите её через \"rech build --lib\" и подключите из внешнего кода")
			os.Exit(2)
		}
		binPath, err := build()
		if err != nil {
			printCompileError(err)
			os.Exit(1)
		}
		if outDir != "" {
			binPath, err = placeInOutputDir(binPath, outDir, projName)
			if err != nil {
				fmt.Fprintln(os.Stderr, "ошибка:", err)
				os.Exit(1)
			}
		}
		if err := runBinary(binPath, runEnv, extraLibPaths); err != nil {
			fmt.Fprintln(os.Stderr, "ошибка выполнения:", err)
			os.Exit(1)
		}
	default:
		usage()
		os.Exit(2)
	}
}

// placeInOutputDir переносит собранный бинарник binPath в outDir под именем
// проекта projName (см. поле "файлы.выход" .rechproj) - сохраняет расширение
// binPath (".exe" на Windows, иначе пусто, см. compiler.compileProgram).
func placeInOutputDir(binPath, outDir, projName string) (string, error) {
	if err := os.MkdirAll(outDir, 0o755); err != nil {
		return "", fmt.Errorf("не удалось создать каталог выхода %q: %w", outDir, err)
	}
	dst := filepath.Join(outDir, projName+filepath.Ext(binPath))
	if err := compiler.MoveFile(binPath, dst); err != nil {
		return "", fmt.Errorf("не удалось перенести собранный бинарник в %s: %w", dst, err)
	}
	return dst, nil
}

// buildArgs - аргументы "build"/"run": путь к .rech-файлу/каталогу/
// .rechproj ИЛИ -c/--code <код> (взаимно исключающие способы указать
// программу - hasCode отличает "код есть, но пустая строка" от "кода не было
// вовсе") и флаги --keep-c/--lib/--release (в любом месте; --lib имеет смысл
// только у "build" - проверяется вызывающим кодом).
type buildArgs struct {
	srcPath, code                  string
	keepC, hasCode, isLib, release bool
}

// parseBuildArgs разбирает аргументы после "build"/"run" (см. buildArgs).
func parseBuildArgs(args []string) (buildArgs, error) {
	var a buildArgs
	for i := 0; i < len(args); i++ {
		switch args[i] {
		case "--keep-c":
			a.keepC = true
		case "--lib":
			a.isLib = true
		case "--release":
			a.release = true
		case "-c", "--code":
			if i+1 >= len(args) {
				return buildArgs{}, fmt.Errorf("после %q ожидался код программы", args[i])
			}
			i++
			a.code = args[i]
			a.hasCode = true
		default:
			a.srcPath = args[i]
		}
	}
	if a.hasCode && a.srcPath != "" {
		return buildArgs{}, fmt.Errorf("нельзя одновременно указать -c/--code и путь к файлу/каталогу (%q)", a.srcPath)
	}
	return a, nil
}

func usage() {
	fmt.Fprintln(os.Stderr, "использование: rech build [--keep-c] [--lib] [--release] <файл.rech>|<каталог>|<файл.rechproj>|-c \"<код>\" | rech run [--keep-c] [--release] <файл.rech>|<каталог>|<файл.rechproj>|-c \"<код>\" | rech test [-j N] <файл.rech>|<каталог> | rech fmt [--write|-w | --check] <файл.rech>|<каталог> | rech proto [-o <файл.rech>] <файл.proto> | rech add <имя> <url> <версия> | rech install | rech debug | rech lsp | rech init [--console|--gui|--service|--library|--static-library] [имя] | rech version")
}

// runBinary запускает только что собранный binPath - env (project.File.
// Окружение, пусто у программ без файла проекта/без секции "окружение:")
// задаёт значения ПО УМОЛЧАНИЮ, не безусловные: уже заданная снаружи
// переменная с тем же именем (Docker "-e"/docker-compose "environment:"/CI/
// обычный шелл) побеждает - "окружение:" пишется в cmd.Env ПЕРВЫМ,
// os.Environ() - ПОСЛЕ (exec.Cmd.Env берёт ПОСЛЕДНЕЕ вхождение каждого
// имени), тот же принцип приоритета, что у 12-factor-конфигурации/
// docker-compose (внешнее окружение развёртывания важнее значения,
// зашитого в файл, который лежит в репозитории вместе с исходниками).
//
// libPaths - каталоги "путь_библиотек": на Linux/macOS путь к библиотекам
// уже зашит в программу (rpath), а Windows ищет DLL по PATH - каталоги
// дописываются в его начало (иначе 0xc0000135, "DLL не найдена").
func runBinary(binPath string, env map[string]string, libPaths []string) error {
	abs, err := filepath.Abs(binPath)
	if err != nil {
		return err
	}
	// #nosec G204 -- запускается бинарник, только что собранный этим же процессом из локального файла.
	cmd := exec.Command(abs)
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	cmd.Stdin = os.Stdin
	if len(env) > 0 {
		cmd.Env = make([]string, 0, len(env)+len(os.Environ()))
		for k, v := range env {
			cmd.Env = append(cmd.Env, k+"="+v)
		}
		cmd.Env = append(cmd.Env, os.Environ()...)
	}
	if runtime.GOOS == "windows" && len(libPaths) > 0 {
		if cmd.Env == nil {
			cmd.Env = os.Environ()
		}
		cmd.Env = withDLLSearchPath(cmd.Env, libPaths)
	}
	return cmd.Run()
}

// withDLLSearchPath - env с libPaths в начале PATH (имя переменной на
// Windows без учёта регистра - "Path"/"PATH").
func withDLLSearchPath(env, libPaths []string) []string {
	prefix := strings.Join(libPaths, string(os.PathListSeparator))
	out := make([]string, 0, len(env)+1)
	found := false
	for _, kv := range env {
		k, v, _ := strings.Cut(kv, "=")
		if strings.EqualFold(k, "PATH") {
			kv = k + "=" + prefix + string(os.PathListSeparator) + v
			found = true
		}
		out = append(out, kv)
	}
	if !found {
		out = append(out, "PATH="+prefix)
	}
	return out
}
