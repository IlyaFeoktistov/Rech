package main

import (
	"fmt"
	"os"

	"github.com/IlyaFeoktistov/Rech/internal/protoimport"
)

// runProtoImport - "rech proto [-o <файл.rech>] <файл.proto>" - тем же
// принципом, что и "rech fmt" (fmt.go): без "-o" печатает результат в
// stdout, с "-o" пишет в указанный файл. См. internal/protoimport для
// того, что именно поддержано/не поддержано в самом ".proto".
func runProtoImport(args []string) bool {
	var out, target string
	for i := 0; i < len(args); i++ {
		switch args[i] {
		case "-o", "--output":
			if i+1 >= len(args) {
				fmt.Fprintln(os.Stderr, "ошибка: после -o ожидался путь к файлу")
				return false
			}
			i++
			out = args[i]
		default:
			if target != "" {
				fmt.Fprintf(os.Stderr, "ошибка: лишний аргумент %q\n", args[i])
				return false
			}
			target = args[i]
		}
	}
	if target == "" {
		fmt.Fprintln(os.Stderr, "использование: rech proto [-o <файл.rech>] <файл.proto>")
		return false
	}

	code, err := protoimport.Import(target)
	if err != nil {
		fmt.Fprintln(os.Stderr, "ошибка:", err)
		return false
	}

	if out == "" {
		fmt.Print(code)
		return true
	}
	if err := os.WriteFile(out, []byte(code), 0o644); err != nil {
		fmt.Fprintln(os.Stderr, "ошибка:", err)
		return false
	}
	return true
}
