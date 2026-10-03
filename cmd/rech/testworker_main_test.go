package main

import (
	"os"
	"testing"
)

// runTests запускает тесты рабочими процессами os.Executable() - под
// "go test" это тестовый бинарник: в режиме "test --worker" он ведёт себя
// как rech.
func TestMain(m *testing.M) {
	if len(os.Args) == 4 && os.Args[1] == "test" && os.Args[2] == testWorkerFlag {
		runTestWorker(os.Args[3])
		os.Exit(0)
	}
	os.Exit(m.Run())
}
