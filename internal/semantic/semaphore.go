// Синхронизация между потоками - "Семафор" (голый счётный семафор поверх
// нативного примитива ОС - обобщение "Блокировки", internal/semantic/
// mutex.go: "захватить"/"освободить" не 0/1, а от 0 до N одновременных
// захватов, N - начальное число разрешений, "что ещё нужно
// для настоящего HTTP highload" - ограничить число одновременно живых
// "запустить"-задач сверху).
//
// "захватить(таймаут_мс: Целое = -1)" - необязательный параметр
// (значения по умолчанию у параметров метода, Шаг 38) добавляет
// ограничение по времени поверх обычного "захватить" (admission control), не
// заводя вторую копию
// метода: по умолчанию (параметр не передан, -1) - ждать бесконечно,
// поведение БУКВАЛЬНО не меняется для всего существующего кода, зовущего
// "захватить()" без аргументов. Явный неотрицательный аргумент - ждать не
// дольше стольки миллисекунд (0 - не ждать вовсе, проверить немедленно) и,
// если не дождалось - паника (rtmsg.SemaphoreAcquireTimeout), а не
// бесконечное ожидание. Тот же принцип, что и таймаут пула БД (`бд.
// таймаут_пула_мс`/rtmsg.DBCheckoutTimeout, internal/backend/cbackend/
// db.go), но таймаут здесь - ОБЫЧНЫЙ аргумент вызова, не настройка
// `.rechproj`: у разных мест вызова одного и того же "Семафора" (или
// разных семафоров) может быть разный смысл ожидания, значение не одно на
// всю программу - отсюда и другой сентинел "ждать бесконечно" (-1, а не
// 0 - здесь 0 содержательно означает "немедленно", а не "бесконечно").
// Обслуживающий код (например, HTTP-сервер) ловит панику через `поймать
// (x: Паника)` и отвечает "мягким" 503 вместо того, чтобы либо ждать
// освобождения слота бесконечно, либо не иметь никакого предела вовсе.
//
// В отличие от "Блокировки" - конструктор принимает ОДИН аргумент
// (начальное число разрешений), поэтому не может быть заведён как совсем
// пустая entitySig (HasCtor: false, как у "Блокировки") - используется тот
// же приём, что и у systemExceptionCtor/registerDivByZeroException
// (internal/semantic/exception.go): HasCtor: true с явным CtorParams,
// синтетическое тело конструктора при этом не нужно вовсе (в отличие от
// "Исключение") - "Семафор", как и "Блокировка", не выражается ir.Stmt
// вообще, его C-представление целиком в internal/backend/cbackend/
// semaphore.go, здесь - только сигнатуры для проверки типов и связывание с
// internal/interp.
package semantic

import (
	"github.com/IlyaFeoktistov/Rech/internal/ast"
	"github.com/IlyaFeoktistov/Rech/internal/ir"
)

const semaphoreEntityName = "Семафор"

// semaphoreWaitForeverDefault - значение по умолчанию параметра
// "таймаут_мс" у "захватить" (-1 - ждать бесконечно, см. package doc
// выше).
var semaphoreWaitForeverDefault = &ast.IntLiteral{Value: -1}

// registerSemaphoreEntity - лениво регистрирует "Семафор" в
// entities/methods при первой встрече имени типа (тем же принципом, что
// registerMutexEntity).
func registerSemaphoreEntity(entities map[string]entitySig, methods map[string]map[string]funcSig) {
	if _, exists := entities[semaphoreEntityName]; exists {
		return
	}
	entities[semaphoreEntityName] = entitySig{
		IsSemaphore:    true,
		HasCtor:        true,
		CtorParams:     []ir.Type{ir.TypeInt},
		CtorParamNames: []string{"начальное_значение"},
	}
	methods[semaphoreEntityName] = map[string]funcSig{
		"захватить":  {Params: []ir.Type{ir.TypeInt}, ParamDefaults: []ast.Expr{semaphoreWaitForeverDefault}},
		"освободить": {},
	}
}

// semaphoreIRType - собственная сущность "Семафор" в готовом IR-виде,
// добавляется в ir.Program.Entities только если использована (см.
// internal/semantic/analyzer.go, buildIR) - Fields/Methods пусты, обе они
// синтезируются backend'ом напрямую по IsSemaphore, не по обычным
// ir.Method (см. package doc ir.Entity.IsSemaphore).
func semaphoreIRType() *ir.Entity {
	return &ir.Entity{Name: semaphoreEntityName, IsSemaphore: true}
}
