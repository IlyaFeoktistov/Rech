package semantic

import (
	"fmt"

	"github.com/IlyaFeoktistov/Rech/internal/ir"
)

// checkVisibility - общая проверка приватности члена сущности (поле/
// константа/метод): доступ снаружи запрещён, если член приватный и текущий
// анализ не идёт внутри метода той же сущности (sc.thisEntity != entityName).
// kindWord/adjective/actionWord параметризуют разницу между полем/
// константой/методом (см. обёртки ниже) - kindWord - существительное для
// сообщения, adjective - форма "приватный", согласованная по роду с
// kindWord, actionWord - что запрещено ("доступ" у поля/константы, "вызов"
// у метода). См. одноимённый хелпер в cbackend/generator.go (AST-путь,
// проверка структурно одна и та же, но не вынесена в общий пакет - ir не
// должен знать про AST-путь, а cbackend не знает про internal/ir).
func checkVisibility(sc *ctx, kindWord, adjective, actionWord, entityName, memberName string, private bool, line int) error {
	if !private || sc.thisEntity == entityName {
		return nil
	}
	return fmt.Errorf("строка %d: %s %q сущности %q %s - %s только из методов самой сущности", line, kindWord, memberName, entityName, adjective, actionWord)
}

func checkFieldVisibility(sc *ctx, entityName, fieldName string, private bool, line int) error {
	return checkVisibility(sc, "поле", "приватное", "доступ", entityName, fieldName, private, line)
}

func checkConstVisibility(sc *ctx, entityName, constName string, private bool, line int) error {
	return checkVisibility(sc, "константа", "приватная", "доступ", entityName, constName, private, line)
}

// resolveEntityConstant возвращает значение константы entityName.constName.
// Значение вычисляется РОВНО один раз на весь Analyze: результат кладётся в
// sc.resolvedConsts (общая для всей программы карта, см. newCtx) и на
// повторных обращениях просто читается оттуда - без этого выражение
// константы (если это не литерал, а что-то посложнее) вычислялось бы заново
// на каждое обращение "Сущность.Константа"/"экземпляр.Константа".
//
// sc.resolvingConsts защищает от циклической ссылки константы саму на себя
// (напрямую или через цепочку других констант, в т.ч. чужих сущностей):
// ключ ставится перед рекурсивным analyzeExpr и снимается сразу после -
// если тот же ключ встретится повторно, пока он ещё не снят, значит
// вычисление зациклилось - без этой проверки analyzeExpr вызывал бы сам
// себя без остановки и падал стек-оверфлоу самого компилятора вместо
// понятной ошибки.
//
// Видимость (приватная/публичная) здесь не проверяется - это забота
// вызывающей стороны (обеих точек доступа в analyzeExpr выше): при
// построении ir.Entity.Constants для собственной сущности видимость
// проверять не нужно (сущность имеет право использовать собственную
// приватную константу для вычисления другой своей константы).
func resolveEntityConstant(sc *ctx, entityName, constName string, line int) (ir.Expr, error) {
	key := entityName + "." + constName
	if v, ok := sc.resolvedConsts[key]; ok {
		return v, nil
	}
	if sc.resolvingConsts[key] {
		return nil, fmt.Errorf("строка %d: константа %q.%q ссылается сама на себя (прямо или через другую константу) - вычислить значение невозможно", line, entityName, constName)
	}
	sig := sc.entities[entityName]
	idx := sig.constIndex(constName)
	sc.resolvingConsts[key] = true
	val, err := analyzeExpr(sc, sig.ConstExprs[idx])
	delete(sc.resolvingConsts, key)
	if err != nil {
		return nil, fmt.Errorf("строка %d: при вычислении константы %q.%q: %w", line, entityName, constName, err)
	}
	sc.resolvedConsts[key] = val
	return val, nil
}

// checkMethodVisibility - то же самое для вызова приватного метода.
func checkMethodVisibility(sc *ctx, entityName, methodName string, private bool, line int) error {
	return checkVisibility(sc, "метод", "приватный", "вызов", entityName, methodName, private, line)
}

// scope - символьная таблица одной области видимости: имя переменной ->
// тип.
