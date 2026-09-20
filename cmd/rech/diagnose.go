package main

import (
	"errors"
	"fmt"
	"os"

	"github.com/IlyaFeoktistov/Rech/internal/diagnostics"
)

// printCompileError печатает compileErrorText(err) на stderr - общая точка
// для build/run (build()/buildLib() в main.go), где ошибка сборки всегда
// фатальна для всей команды.
func printCompileError(err error) {
	fmt.Fprint(os.Stderr, compileErrorText(err))
}

// compileErrorText форматирует ошибку сборки (Frontend.Parse/semantic.
// Analyze) - если err несёт internal/diagnostics.Diagnostic/Bag (см.
// errors.As - проходит сквозь любые fmt.Errorf("...: %w", err)-обёртки
// выше по стеку, например internal/compiler.parseDir), возвращает код,
// позицию и сниппет исходной строки (Render/RenderAll); иначе, как и
// раньше - обычный текст ошибки (до-миграционный путь, ошибки самого CLI
// не про исходник программы, и рантайм-ошибки "rech test" - неудачный
// assert теста, а не ошибка сборки). Используется и для build/run
// (printCompileError), и для отчёта "rech test" о непрошедших тестах
// (cmd/rech - "Провалившиеся тесты") - тот же формат в обоих местах.
func compileErrorText(err error) string {
	var bag *diagnostics.Bag
	if errors.As(err, &bag) {
		diags := bag.Diagnostics()
		return diagnostics.RenderAll(diags, loadDiagnosticSources(diags))
	}
	var d *diagnostics.Diagnostic
	if errors.As(err, &d) {
		return diagnostics.Render(d, readSourceOrEmpty(d.Pos.File))
	}
	return fmt.Sprintf("ошибка: %v\n", err)
}

// loadDiagnosticSources читает КАЖДЫЙ уникальный Position.File среди diags
// ровно один раз - RenderAll не должен перечитывать общий файл под каждую
// диагностику из нескольких, найденных в нём.
func loadDiagnosticSources(diags []*diagnostics.Diagnostic) map[string]string {
	out := make(map[string]string, len(diags))
	for _, d := range diags {
		if d.Pos.File == "" {
			continue
		}
		if _, ok := out[d.Pos.File]; ok {
			continue
		}
		out[d.Pos.File] = readSourceOrEmpty(d.Pos.File)
	}
	return out
}

// readSourceOrEmpty - "" как для file == "" (код без файла-источника, "rech
// build -c"), так и при ошибке чтения (файл успел исчезнуть/переместиться
// между разбором и печатью диагностики) - Render тогда просто не печатает
// сниппет, не падает.
func readSourceOrEmpty(file string) string {
	if file == "" {
		return ""
	}
	b, err := os.ReadFile(file)
	if err != nil {
		return ""
	}
	return string(b)
}
