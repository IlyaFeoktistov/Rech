package main

import (
	"errors"
	"fmt"
	"os"

	"github.com/IlyaFeoktistov/Rech/internal/diagnostics"
)

// printCompileError печатает ошибку сборки (Frontend.Parse/semantic.Analyze)
// на stderr - если err несёт internal/diagnostics.Diagnostic/Bag (см.
// errors.As - проходит сквозь любые fmt.Errorf("...: %w", err)-обёртки
// выше по стеку, например internal/compiler.parseDir), печатает код,
// позицию и сниппет исходной строки (Render/RenderAll); иначе, как и
// раньше - обычным текстом (до-миграционный путь и ошибки самого CLI,
// не связанные с исходником программы - "файл не найден" и т.п.). Общая
// точка для build/run (build()/buildLib() в main.go) - единственное
// место, решающее, КАК показать ошибку компиляции.
func printCompileError(err error) {
	var bag *diagnostics.Bag
	if errors.As(err, &bag) {
		diags := bag.Diagnostics()
		fmt.Fprint(os.Stderr, diagnostics.RenderAll(diags, loadDiagnosticSources(diags)))
		return
	}
	var d *diagnostics.Diagnostic
	if errors.As(err, &d) {
		fmt.Fprint(os.Stderr, diagnostics.Render(d, readSourceOrEmpty(d.Pos.File)))
		return
	}
	fmt.Fprintln(os.Stderr, "ошибка:", err)
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
