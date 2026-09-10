// Carbon-стиль (PHP) методов на встроенной "ДатаВремя" -
// "д.добавить_дни(5)"/"д.в_поясе(\"Europe/Moscow\")"/"д.разница_в_днях(б)"/
// "д.является_выходным()"/"д.в_читаемом_виде()" - пользователь явно попросил
// именно такой, метод-стиль API поверх уже существующих свободных функций
// (время_как_дата/дата_как_время/время_как_дата_в_поясе/...), а не голые
// функции наподобие "время_как_дата(дата_как_время(д) + 86400)".
//
// У "ДатаВремя" НЕТ ни одного настоящего ir.Method (methods["ДатаВремя"]
// нигде не заполняется) - каждый такой "метод" целиком разворачивается ЗДЕСЬ,
// на этапе семантики, в обычную композицию уже существующих ir.CallExpr/
// ir.BinaryExpr/ir.TernaryExpr узлов. Причина не эстетическая: сама
// "ДатаВремя" (как и "Запрос"/"Ответ"/...) безусловно попадает в
// ir.Program.Entities для КАЖДОЙ программы (analyzer.go, buildIR) - если бы
// эти 16 методов были настоящими ir.Method, привязанными к этой сущности,
// их C-тела эмитировались бы БЕЗУСЛОВНО тоже, а значит и флаги cbackend
// (usesDateTime/usesTZ), взводимые при обходе ИХ тел, стали бы безусловно
// истинны для абсолютно любой программы - ровно тот структурный риск,
// который уже один раз был найден и исправлен для "?Строка" (см. package
// doc registerOptionalStringOnCall, nullable.go: "по факту регистрации, не
// по факту использования"). После desugaring'а IR содержит ровно те же
// узлы, что и при вызове "время_как_дата(...)" вручную - существующий
// механизм простановки флагов (analyzeCallExpr/generateIRCallExpr) решает
// вопрос сам, без единой новой строчки в cbackend/interp.
package semantic

import (
	"fmt"

	"github.com/IlyaFeoktistov/Rech/internal/ast"
	"github.com/IlyaFeoktistov/Rech/internal/ir"
)

// dateTimeArithmeticSeconds - "добавить_дни"/"вычесть_дни"/... - число
// секунд ОДНОЙ единицы, со знаком (вычитание - то же добавление
// отрицательного количества, Carbon делает буквально то же самое:
// subDays(n) == addDays(-n)).
var dateTimeArithmeticSeconds = map[string]int64{
	"добавить_дни":     86400,
	"вычесть_дни":      -86400,
	"добавить_часы":    3600,
	"вычесть_часы":     -3600,
	"добавить_минуты":  60,
	"вычесть_минуты":   -60,
	"добавить_секунды": 1,
	"вычесть_секунды":  -1,
}

// dateTimeComparisonOps - "раньше"/"позже" - сравнение по unix-секундам.
var dateTimeComparisonOps = map[string]string{
	"раньше": "<",
	"позже":  ">",
}

// dateTimeDiffUnitSeconds - "разница_в_днях"/"разница_в_часах"/
// "разница_в_секундах" - модуль разницы unix-секунд, делённый на единицу
// (целочисленное усечение, не округление - тот же принцип, что у Carbon'а
// diffInDays).
var dateTimeDiffUnitSeconds = map[string]int64{
	"разница_в_днях":     86400,
	"разница_в_часах":    3600,
	"разница_в_секундах": 1,
}

// dateTimeSugarMethodNames - полный список имён, которые
// maybeDesugarDateTimeMethod распознаёт на получателе "ДатаВремя" - ЛЮБОЕ
// другое имя (в т.ч. одноимённый метод обычной пользовательской сущности)
// идёт обычным путём analyzeMethodCallExpr.
var dateTimeSugarMethodNames = func() map[string]bool {
	names := map[string]bool{"в_поясе": true, "является_выходным": true, "в_читаемом_виде": true}
	for k := range dateTimeArithmeticSeconds {
		names[k] = true
	}
	for k := range dateTimeComparisonOps {
		names[k] = true
	}
	for k := range dateTimeDiffUnitSeconds {
		names[k] = true
	}
	return names
}()

// maybeDesugarDateTimeMethod - вызывается из analyzeExpr (expr.go) ДО
// обычного analyzeMethodCallExpr. handled=false (без ошибки) - получатель
// анализируется здесь ещё раз (ast.Expr не несёт побочных эффектов на
// этапе семантики), не является "ДатаВремя", или сам e.Method - не одно из
// известных имён; тогда вызывающая сторона идёт обычным путём и получает
// свою обычную ошибку/результат.
func maybeDesugarDateTimeMethod(sc *ctx, e *ast.MethodCallExpr) (result ir.Expr, handled bool, err error) {
	if !dateTimeSugarMethodNames[e.Method] {
		return nil, false, nil
	}
	receiver, err := analyzeExpr(sc, e.Receiver)
	if err != nil {
		return nil, false, nil
	}
	receiver = maybeUnwrapNarrowed(sc, receiver)
	if ir.TypeOf(receiver) != ir.EntityType("ДатаВремя") {
		return nil, false, nil
	}

	asTime := func(v ir.Expr) ir.Expr {
		return &ir.CallExpr{Callee: "дата_как_время", Args: []ir.Expr{v}, Type: ir.TypeInt, HasReturn: true}
	}
	asDate := func(v ir.Expr) ir.Expr {
		return &ir.CallExpr{Callee: "время_как_дата", Args: []ir.Expr{v}, Type: ir.EntityType("ДатаВремя"), HasReturn: true}
	}

	if deltaSeconds, ok := dateTimeArithmeticSeconds[e.Method]; ok {
		n, aerr := dateTimeOneIntArg(sc, e)
		if aerr != nil {
			return nil, true, aerr
		}
		delta := &ir.BinaryExpr{Op: "*", Left: n, Right: &ir.IntLiteral{Value: deltaSeconds}, Type: ir.TypeInt}
		sum := &ir.BinaryExpr{Op: "+", Left: asTime(receiver), Right: delta, Type: ir.TypeInt}
		return asDate(sum), true, nil
	}

	if op, ok := dateTimeComparisonOps[e.Method]; ok {
		other, aerr := dateTimeOneArg(sc, e)
		if aerr != nil {
			return nil, true, aerr
		}
		return &ir.BinaryExpr{Op: op, Left: asTime(receiver), Right: asTime(other), Type: ir.TypeBool}, true, nil
	}

	if unitSeconds, ok := dateTimeDiffUnitSeconds[e.Method]; ok {
		other, aerr := dateTimeOneArg(sc, e)
		if aerr != nil {
			return nil, true, aerr
		}
		a, b := asTime(receiver), asTime(other)
		diffPos := &ir.BinaryExpr{Op: "-", Left: a, Right: b, Type: ir.TypeInt}
		diffNeg := &ir.BinaryExpr{Op: "-", Left: b, Right: a, Type: ir.TypeInt}
		cond := &ir.BinaryExpr{Op: ">", Left: a, Right: b, Type: ir.TypeBool}
		absDiff := &ir.TernaryExpr{Cond: cond, Then: diffPos, Else: diffNeg, Type: ir.TypeInt}
		// Целочисленное "/" - registerDivByZeroException, тем же принципом,
		// что и обычное "/" через analyzeBinaryExpr (expr.go) - desugaring
		// строит ir.BinaryExpr напрямую, в обход того пути, значит повторяет
		// его побочный эффект здесь явно (делитель здесь - буквальная
		// константа 86400/3600/1, никогда не 0, но C-рантайм всё равно
		// защищается общей проверкой rech_int_div - без регистрации entity
		// "ОшибкаДеленияНаНоль" её конструктор не был бы эмитирован).
		registerDivByZeroException(sc.entities)
		return &ir.BinaryExpr{Op: "/", Left: absDiff, Right: &ir.IntLiteral{Value: unitSeconds}, Type: ir.TypeInt}, true, nil
	}

	switch e.Method {
	case "в_поясе":
		if len(e.Args) != 1 {
			return nil, true, fmt.Errorf("строка %d: метод %q принимает один аргумент (Строка)", e.Line, e.Method)
		}
		zone, aerr := analyzeExpr(sc, e.Args[0])
		if aerr != nil {
			return nil, true, aerr
		}
		if ir.TypeOf(zone) != ir.TypeString {
			return nil, true, fmt.Errorf("строка %d: метод %q ожидает Строка, получено %s", e.Line, e.Method, ir.TypeOf(zone))
		}
		// "?ДатаВремя" - lazy-регистрация, тем же принципом, что и у прямого
		// вызова "время_как_дата_в_поясе" (expr.go, analyzeCallExpr) -
		// desugaring обходит analyzeCallExpr стороной, значит регистрация
		// здесь должна повториться явно.
		if rerr := registerOptionalDateTimeOnCall(sc.entities, sc.methods, sc.aliases, sc.interfaces, sc.enums); rerr != nil {
			return nil, true, fmt.Errorf("строка %d: %w", e.Line, rerr)
		}
		return &ir.CallExpr{Callee: "время_как_дата_в_поясе", Args: []ir.Expr{asTime(receiver), zone},
			Type: ir.NullableType(ir.EntityType("ДатаВремя")), HasReturn: true}, true, nil
	case "является_выходным":
		if len(e.Args) != 0 {
			return nil, true, fmt.Errorf("строка %d: метод %q не принимает аргументов", e.Line, e.Method)
		}
		wd := &ir.CallExpr{Callee: "день_недели", Args: []ir.Expr{receiver}, Type: ir.TypeInt, HasReturn: true}
		isSunday := &ir.BinaryExpr{Op: "==", Left: wd, Right: &ir.IntLiteral{Value: 0}, Type: ir.TypeBool}
		isSaturday := &ir.BinaryExpr{Op: "==", Left: wd, Right: &ir.IntLiteral{Value: 6}, Type: ir.TypeBool}
		return &ir.BinaryExpr{Op: "или", Left: isSunday, Right: isSaturday, Type: ir.TypeBool}, true, nil
	case "в_читаемом_виде":
		if len(e.Args) != 0 {
			return nil, true, fmt.Errorf("строка %d: метод %q не принимает аргументов", e.Line, e.Method)
		}
		return &ir.CallExpr{Callee: "дату_в_читаемом_виде", Args: []ir.Expr{receiver}, Type: ir.TypeString, HasReturn: true}, true, nil
	}
	return nil, false, nil
}

// dateTimeOneIntArg - один аргумент типа Целое (арифметические методы).
func dateTimeOneIntArg(sc *ctx, e *ast.MethodCallExpr) (ir.Expr, error) {
	if len(e.Args) != 1 {
		return nil, fmt.Errorf("строка %d: метод %q принимает один аргумент (Целое)", e.Line, e.Method)
	}
	v, err := analyzeExpr(sc, e.Args[0])
	if err != nil {
		return nil, err
	}
	v = maybeUnwrapNarrowed(sc, v)
	if ir.TypeOf(v) != ir.TypeInt {
		return nil, fmt.Errorf("строка %d: метод %q ожидает Целое, получено %s", e.Line, e.Method, ir.TypeOf(v))
	}
	return v, nil
}

// dateTimeOneArg - один аргумент типа ДатаВремя (сравнение/разница).
func dateTimeOneArg(sc *ctx, e *ast.MethodCallExpr) (ir.Expr, error) {
	if len(e.Args) != 1 {
		return nil, fmt.Errorf("строка %d: метод %q принимает один аргумент (ДатаВремя)", e.Line, e.Method)
	}
	v, err := analyzeExpr(sc, e.Args[0])
	if err != nil {
		return nil, err
	}
	v = maybeUnwrapNarrowed(sc, v)
	if ir.TypeOf(v) != ir.EntityType("ДатаВремя") {
		return nil, fmt.Errorf("строка %d: метод %q ожидает ДатаВремя, получено %s", e.Line, e.Method, ir.TypeOf(v))
	}
	return v, nil
}
