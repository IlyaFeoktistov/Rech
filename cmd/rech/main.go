// Command rech - компилятор языка Rech (v0.1).
//
// Использование:
//
//	rech build [--keep-c] <файл.rech>|<каталог>|-c "<код>"   - собрать бинарник
//	rech run [--keep-c] <файл.rech>|<каталог>|-c "<код>"     - собрать и выполнить
//	rech test <файл.rech>|<каталог>                          - найти и запустить тесты
//	rech debug                                               - сервер DAP по stdin/stdout (для VS Code)
//	rech lsp                                                 - сервер LSP по stdin/stdout (hover/автодополнение, для VS Code)
//	rech version                                             - напечатать версию
//
// <каталог> у build/run (например, "." - как "go build .") собирает все
// ".rech"-файлы прямо в нём (без подкаталогов) как одну единицу компиляции -
// это не система импортов (docs/ROADMAP.md, Шаг 11), а неявное объединение
// всего каталога, детали - internal/compiler.Compiler.parseDir.
//
// -c/--code "<код>" (как "python -c") собирает/запускает код прямо из
// аргумента командной строки, без файла - многострочный код передаётся
// обычной многострочной кавычкой шелла или heredoc'ом
// (rech run -c "$(cat <<'EOF' ... EOF)"), отдельной поддержки в самом rech
// для этого не нужно - строка приходит уже целиком.
//
// <каталог> у test обходится рекурсивно, ищет файлы с суффиксом
// "_тест.rech" и запускает каждый через internal/interp (без вызова
// системного cc) - детали internal/testrunner.
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
	"strings"

	"github.com/IlyaFeoktistov/Rech/internal/compiler"
	"github.com/IlyaFeoktistov/Rech/internal/debugadapter"
	"github.com/IlyaFeoktistov/Rech/internal/lsp"
	"github.com/IlyaFeoktistov/Rech/internal/project"
	"github.com/IlyaFeoktistov/Rech/internal/testrunner"
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
		name := ""
		if len(os.Args) >= 3 {
			name = os.Args[2]
		}
		if err := initProject(name); err != nil {
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
		if len(os.Args) < 3 {
			usage()
			os.Exit(2)
		}
		if !runTests(os.Args[2]) {
			os.Exit(1)
		}
		return
	}

	srcPath, code, keepC, hasCode, err := parseBuildArgs(os.Args[2:])
	if err != nil {
		fmt.Fprintln(os.Stderr, "ошибка:", err)
		os.Exit(2)
	}

	// Файл проекта (.rechproj) - либо явно передан вместо <файл.rech>|<каталог>,
	// либо (при полностью пустых аргументах) единственный в текущем каталоге -
	// см. cmd/rech/project.go. outDir/projName пусты, если проекта нет вовсе -
	// тогда ниже всё идёт как раньше, без переноса бинарника.
	var outDir, projName string
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
			srcPath = pf.MainPath()
			if !keepC {
				// --keep-c из командной строки не переопределяется "режимом"
				// файла проекта в обратную сторону - иначе значение по
				// умолчанию (false) неотличимо от явного выбора пользователя.
				keepC = pf.KeepC()
			}
			outDir, projName = pf.OutputDir(), pf.Имя
		}
	}

	if !hasCode && srcPath == "" {
		usage()
		os.Exit(2)
	}

	comp := InitializeCompiler()
	build := func() (string, error) { return comp.Build(srcPath, keepC) }
	if hasCode {
		build = func() (string, error) { return comp.BuildSource(code, keepC) }
	}

	switch cmd {
	case "build":
		binPath, err := build()
		if err != nil {
			fmt.Fprintln(os.Stderr, "ошибка:", err)
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
	case "run":
		binPath, err := build()
		if err != nil {
			fmt.Fprintln(os.Stderr, "ошибка:", err)
			os.Exit(1)
		}
		if outDir != "" {
			binPath, err = placeInOutputDir(binPath, outDir, projName)
			if err != nil {
				fmt.Fprintln(os.Stderr, "ошибка:", err)
				os.Exit(1)
			}
		}
		if err := runBinary(binPath); err != nil {
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

// parseBuildArgs разбирает аргументы после "build"/"run": флаг --keep-c (в
// любом месте), путь к .rech-файлу/каталогу ИЛИ -c/--code <код> (взаимно
// исключающие способы указать программу - hasCode отличает "код есть, но
// пустая строка" от "кода не было вовсе").
func parseBuildArgs(args []string) (srcPath, code string, keepC, hasCode bool, err error) {
	for i := 0; i < len(args); i++ {
		switch args[i] {
		case "--keep-c":
			keepC = true
		case "-c", "--code":
			if i+1 >= len(args) {
				return "", "", false, false, fmt.Errorf("после %q ожидался код программы", args[i])
			}
			i++
			code = args[i]
			hasCode = true
		default:
			srcPath = args[i]
		}
	}
	if hasCode && srcPath != "" {
		return "", "", false, false, fmt.Errorf("нельзя одновременно указать -c/--code и путь к файлу/каталогу (%q)", srcPath)
	}
	return srcPath, code, keepC, hasCode, nil
}

func usage() {
	fmt.Fprintln(os.Stderr, "использование: rech build|run [--keep-c] <файл.rech>|<каталог>|<файл.rechproj>|-c \"<код>\" | rech test <файл.rech>|<каталог> | rech debug | rech lsp | rech init [имя] | rech version")
}

// testDotsPerLine - сколько точек/"F" печатать в строке прогресса, прежде
// чем перейти на новую (как у PHPUnit/Composer) - чисто читаемость вывода
// при большом числе тестов, не влияет на подсчёт результатов.
const testDotsPerLine = 60

// runTests запускает testrunner.Run по path и печатает результат в стиле
// прогресс-точек (Composer/PHPUnit): "." за пройденный тест, "F" за
// непройденный - ни один упавший файл не прерывает прогон остальных
// (testrunner.RunFile изолирует ошибки каждого файла отдельно). После
// прогресса - список причин каждого провала, затем итоговая сводка.
// Возвращает false, если хотя бы один тест не прошёл.
func runTests(path string) bool {
	results, err := testrunner.Run(path)
	if err != nil {
		fmt.Fprintln(os.Stderr, "ошибка:", err)
		return false
	}
	if len(results) == 0 {
		fmt.Println("тестовых файлов не найдено")
		return true
	}

	var failed []testrunner.Result
	for i, r := range results {
		if r.Passed {
			fmt.Print(".")
		} else {
			fmt.Print("F")
			failed = append(failed, r)
		}
		if (i+1)%testDotsPerLine == 0 {
			fmt.Println()
		}
	}
	fmt.Println()
	fmt.Println()

	if len(failed) > 0 {
		fmt.Println("Провалившиеся тесты:")
		fmt.Println()
		for i, r := range failed {
			fmt.Printf("%d) %s\n%v\n\n", i+1, r.Path, r.Err)
		}
	}

	fmt.Printf("Тестов: %d, провалено: %d\n", len(results), len(failed))
	return len(failed) == 0
}

func runBinary(binPath string) error {
	abs, err := filepath.Abs(binPath)
	if err != nil {
		return err
	}
	// #nosec G204 -- запускается бинарник, только что собранный этим же процессом из локального файла.
	cmd := exec.Command(abs)
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	cmd.Stdin = os.Stdin
	return cmd.Run()
}
