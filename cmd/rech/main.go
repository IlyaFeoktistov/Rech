// Command rech - компилятор языка Rech (v0.1).
//
// Использование:
//
//	rech build [--keep-c] <файл.rech>|<каталог>|-c "<код>"   - собрать бинарник
//	rech run [--keep-c] <файл.rech>|<каталог>|-c "<код>"     - собрать и выполнить
//	rech test <файл.rech>|<каталог>                          - найти и запустить тесты
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
		if hasCode {
			// У -c/--code нет "рядом с исходником" - без файла негде было
			// бы догадаться, куда положен результат.
			fmt.Println(binPath)
		}
	case "run":
		binPath, err := build()
		if err != nil {
			fmt.Fprintln(os.Stderr, "ошибка:", err)
			os.Exit(1)
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
	fmt.Fprintln(os.Stderr, "использование: rech build|run [--keep-c] <файл.rech>|<каталог>|-c \"<код>\" | rech test <файл.rech>|<каталог> | rech version")
}

// runTests запускает testrunner.Run по path, печатает результат каждого
// файла и итоговую сводку. Возвращает false, если хотя бы один тест не
// прошёл (компилятор/семантика/"утверждать" - без разницы, всё это делает
// файл непройденным).
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
	passed := 0
	for _, r := range results {
		if r.Passed {
			passed++
			fmt.Printf("ок      %s\n", r.Path)
			continue
		}
		fmt.Printf("ОШИБКА  %s: %v\n", r.Path, r.Err)
	}
	fmt.Printf("Итого: %d пройдено, %d провалено\n", passed, len(results)-passed)
	return passed == len(results)
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
