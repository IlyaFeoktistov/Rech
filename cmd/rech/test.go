package main

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"os/exec"
	"runtime"
	"strconv"
	"strings"
	"sync"

	"github.com/IlyaFeoktistov/Rech/internal/testrunner"
)

// "rech test": каждый тестовый файл исполняется в отдельном процессе -
// "rech test --worker <файл>" - до jobs процессов одновременно. Отдельный
// процесс изолирует то, что у интерпретатора общее на процесс: обработчики
// сигналов, режим консоли, "выйти()" (завершает только свой тест, а не весь
// прогон) - тесты не мешают друг другу и могут идти параллельно.
//
// Рабочий процесс печатает результат одной строкой JSON (testReport) с уже
// отформатированными ошибкой и предупреждениями: сниппеты исходника строит
// он сам.

// testWorkerFlag - скрытый режим "rech test": один файл, результат - JSON.
const testWorkerFlag = "--worker"

// testDotsPerLine - сколько точек/"F" печатать в строке прогресса, прежде
// чем перейти на новую (как у PHPUnit/Composer) - чисто читаемость вывода
// при большом числе тестов, не влияет на подсчёт результатов.
const testDotsPerLine = 60

// testReport - результат одного тестового файла от рабочего процесса.
type testReport struct {
	Passed   bool     `json:"passed"`
	Failure  string   `json:"failure,omitempty"`
	Warnings []string `json:"warnings,omitempty"`
}

// parseTestArgs - "[-j N] <путь>"; jobs по умолчанию - число ядер.
func parseTestArgs(args []string) (path string, jobs int, err error) {
	jobs = runtime.NumCPU()
	for i := 0; i < len(args); i++ {
		switch {
		case args[i] == "-j":
			if i+1 >= len(args) {
				return "", 0, fmt.Errorf("после -j ожидалось число процессов")
			}
			i++
			if jobs, err = strconv.Atoi(args[i]); err != nil || jobs < 1 {
				return "", 0, fmt.Errorf("-j %q: ожидалось целое число больше нуля", args[i])
			}
		case path == "":
			path = args[i]
		default:
			return "", 0, fmt.Errorf("лишний аргумент %q", args[i])
		}
	}
	if path == "" {
		return "", 0, fmt.Errorf("не указан файл или каталог с тестами")
	}
	return path, jobs, nil
}

// runTestWorker - режим рабочего процесса: path исполняется, результат -
// строкой JSON в stdout.
func runTestWorker(path string) {
	r := testrunner.RunFile(path)
	rep := testReport{Passed: r.Passed}
	if r.Err != nil {
		rep.Failure = compileErrorText(r.Err)
	}
	for _, w := range r.Warnings {
		rep.Warnings = append(rep.Warnings, compileErrorText(w))
	}
	if err := json.NewEncoder(os.Stdout).Encode(rep); err != nil {
		fmt.Fprintln(os.Stderr, "ошибка:", err)
		os.Exit(1)
	}
}

// runTestProcess исполняет file в рабочем процессе. stdin - ввод теста
// (nil - пустой: при параллельном прогоне общий ввод делить не на кого).
func runTestProcess(file string, stdin io.Reader) testReport {
	exe, err := os.Executable()
	if err != nil {
		return testReport{Failure: fmt.Sprintf("ошибка: %v\n", err)}
	}
	// #nosec G204 -- запускается сам rech с путём тестового файла пользователя.
	cmd := exec.Command(exe, "test", testWorkerFlag, file)
	var stdout, stderr bytes.Buffer
	cmd.Stdin = stdin
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr
	runErr := cmd.Run()
	// Результат - последняя строка: до неё в stdout мог писать сам тест
	// мимо перехвата (C-функция через FFI).
	out := strings.TrimRight(stdout.String(), "\n")
	last := out[strings.LastIndex(out, "\n")+1:]
	var rep testReport
	if err := json.Unmarshal([]byte(last), &rep); err == nil && (rep.Passed || rep.Failure != "") {
		return rep
	}
	// Процесс завершился, не сообщив результат: "выйти(N)" в тесте,
	// аварийное завершение FFI-кода, сигнал.
	code := -1
	if cmd.ProcessState != nil {
		code = cmd.ProcessState.ExitCode()
	} else if runErr != nil {
		return testReport{Failure: fmt.Sprintf("ошибка: %v\n", runErr)}
	}
	msg := fmt.Sprintf("ошибка: тест завершил процесс до конца \"главная\" (код выхода %d)\n", code)
	if s := strings.TrimSpace(stderr.String()); s != "" {
		msg += s + "\n"
	}
	return testReport{Failure: msg}
}

// runTests находит тесты по path, исполняет их (до jobs процессов
// одновременно) и печатает результат в стиле прогресс-точек
// (Composer/PHPUnit): "." за пройденный тест, "F" за непройденный, по мере
// завершения; ни один упавший файл не прерывает прогон остальных. После
// прогресса - предупреждения и причины провалов в порядке файлов, затем
// итоговая сводка. false - хотя бы один тест не прошёл.
func runTests(path string, jobs int) bool {
	files, err := testrunner.Discover(path)
	if err != nil {
		fmt.Fprintln(os.Stderr, "ошибка:", err)
		return false
	}
	if len(files) == 0 {
		fmt.Println("тестовых файлов не найдено")
		return true
	}

	var stdin io.Reader
	if jobs == 1 {
		stdin = os.Stdin
	}
	reports := make([]testReport, len(files))
	next := make(chan int)
	done := make(chan int)
	var wg sync.WaitGroup
	for range min(jobs, len(files)) {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for i := range next {
				reports[i] = runTestProcess(files[i], stdin)
				done <- i
			}
		}()
	}
	go func() {
		for i := range files {
			next <- i
		}
		close(next)
		wg.Wait()
		close(done)
	}()

	printed := 0
	for i := range done {
		if reports[i].Passed {
			fmt.Print(".")
		} else {
			fmt.Print("F")
		}
		if printed++; printed%testDotsPerLine == 0 {
			fmt.Println()
		}
	}
	fmt.Println()
	fmt.Println()

	for _, r := range reports {
		for _, w := range r.Warnings {
			fmt.Fprintln(os.Stderr, w)
		}
	}

	failed := 0
	for i, r := range reports {
		if r.Passed {
			continue
		}
		if failed == 0 {
			fmt.Println("Провалившиеся тесты:")
			fmt.Println()
		}
		failed++
		fmt.Printf("%d) %s\n%s\n", failed, files[i], r.Failure)
	}

	fmt.Printf("Тестов: %d, провалено: %d\n", len(files), failed)
	return failed == 0
}
