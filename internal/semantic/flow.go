package semantic

import (
	"fmt"

	"github.com/IlyaFeoktistov/Rech/internal/diagnostics"
	"github.com/IlyaFeoktistov/Rech/internal/ir"
	"github.com/IlyaFeoktistov/Rech/internal/ir/cfg"
)

// analyzeFlow строит граф потока управления (internal/ir/cfg) тела функции/
// метода/лямбды body ОДИН раз и возвращает проверки, которые из него
// следуют: err - ошибка адреса элемента списка (list_element_pointer.go),
// nil если её нет; unreachable - строки начала недостижимых кусков тела
// (cfg.Graph.UnreachableRegions) - предупреждения, не ошибки (warnUnreachable);
// fallsThrough - true, если хотя бы один живой путь тела проваливается в
// конец без "вернуть"/"выбросить" - решение о том, ошибка ли это (у функции/
// метода/лямбды с объявленным типом результата - да), и КАКОЙ именно текст
// ошибки писать, остаётся за каждым из трёх вызывающих мест (callable.go,
// analyzer.go) - у них разные формулировки ("лямбда"/просто текст,
// подставляемый в "метод %q: %w"/"функция %q: %w").
//
// Раньше "все пути возвращают значение" проверял endsWithReturnValue - узкая
// рекурсивная эвристика по ПОСЛЕДНЕЙ инструкции блока (см. историю git) -
// не распознавала, например, "пока истина {...}" без "прервать" как
// расходящийся путь, и не имела отдельной проверки недостижимого кода
// вовсе. internal/ir/cfg - настоящий граф базовых блоков, обе проверки
// сводятся к одной задаче (достижимость), см. его package doc.
func analyzeFlow(body []ir.Stmt) (err error, unreachable []int, fallsThrough bool) {
	g := cfg.Build(body)
	unreachable = g.UnreachableRegions(body)
	err = checkListElementPointers(g, body)
	fallsThrough = g.FallsThrough()
	return
}

// UnreachableMessage - текст предупреждения о недостижимом коде (после
// контекста "функция %q: ..."); LSP узнаёт по нему диагностику, которую
// рисует затенённой.
const UnreachableMessage = "недостижимый код - эта инструкция и всё после неё до конца блока не выполнятся"

// warnUnreachable - по предупреждению на каждый недостижимый кусок
// (строки lines) с контекстом format/args ("функция %q"). Один кусок -
// одно предупреждение, даже если тело анализируется несколько раз
// (generic-инстанциации).
func (a *Analyzer) warnUnreachable(lines []int, format string, args ...any) {
	for _, line := range lines {
		pos := diagnostics.Position{File: currentFile, Line: line}
		if a.warnedAt[pos] {
			continue
		}
		if a.warnedAt == nil {
			a.warnedAt = map[diagnostics.Position]bool{}
		}
		a.warnedAt[pos] = true
		d := errAtLine(line, CodeFlow, "%s: %s", fmt.Sprintf(format, args...), UnreachableMessage)
		d.Severity = diagnostics.Warning
		a.warnings = append(a.warnings, d)
	}
}
