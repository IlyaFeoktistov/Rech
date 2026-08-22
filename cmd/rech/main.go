// Command rech - компилятор языка Rech (v0.1).
//
// Использование:
//
//	rech build [--keep-c] <файл.rech>   - собрать нативный бинарник <файл>
//	rech run [--keep-c] <файл.rech>     - собрать и сразу выполнить
//	rech version                        - напечатать версию компилятора
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

	srcPath, keepC := parseBuildArgs(os.Args[2:])
	if srcPath == "" {
		usage()
		os.Exit(2)
	}

	comp := InitializeCompiler()

	switch cmd {
	case "build":
		if _, err := comp.Build(srcPath, keepC); err != nil {
			fmt.Fprintln(os.Stderr, "ошибка:", err)
			os.Exit(1)
		}
	case "run":
		binPath, err := comp.Build(srcPath, keepC)
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

// parseBuildArgs разбирает аргументы после "build"/"run": один флаг
// --keep-c (в любом месте) и ровно один путь к .rech-файлу.
func parseBuildArgs(args []string) (srcPath string, keepC bool) {
	for _, a := range args {
		if a == "--keep-c" {
			keepC = true
			continue
		}
		srcPath = a
	}
	return srcPath, keepC
}

func usage() {
	fmt.Fprintln(os.Stderr, "использование: rech build|run [--keep-c] <файл.rech> | rech version")
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
