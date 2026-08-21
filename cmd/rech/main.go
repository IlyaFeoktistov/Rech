// Command rech - компилятор языка Rech (v0.1).
//
// Использование:
//
//	rech build <файл.rech>   - сгенерировать <файл>.c и собрать нативный бинарник <файл>
//	rech run <файл.rech>     - собрать и сразу выполнить
//	rech version             - напечатать версию компилятора
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

	if len(os.Args) < 3 {
		usage()
		os.Exit(2)
	}
	srcPath := os.Args[2]

	comp := InitializeCompiler()

	switch cmd {
	case "build":
		if _, err := comp.Build(srcPath); err != nil {
			fmt.Fprintln(os.Stderr, "ошибка:", err)
			os.Exit(1)
		}
	case "run":
		binPath, err := comp.Build(srcPath)
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

func usage() {
	fmt.Fprintln(os.Stderr, "использование: rech build|run <файл.rech> | rech version")
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
