package semantic

import (
	"github.com/IlyaFeoktistov/Rech/internal/ir"
	"github.com/IlyaFeoktistov/Rech/internal/ir/cfg"
)

// analyzeFlow строит граф потока управления (internal/ir/cfg) тела функции/
// метода/лямбды body ОДИН раз и возвращает обе проверки, которые из него
// следуют: unreachable - ошибка "недостижимый код" на первую (по номеру
// строки) физически недостижимую инструкцию, иначе - ошибка адреса
// элемента списка (list_element_pointer.go), nil если нет ни той, ни другой;
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
func analyzeFlow(body []ir.Stmt) (unreachable error, fallsThrough bool) {
	g := cfg.Build(body)
	if line, ok := g.UnreachableLine(); ok {
		unreachable = errAtLine(line, CodeFlow, "недостижимый код")
	} else if err := checkListElementPointers(g, body); err != nil {
		unreachable = err
	}
	fallsThrough = g.FallsThrough()
	return
}
