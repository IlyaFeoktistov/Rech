// Синхронизация между потоками - "Семафор" (голый счётный семафор поверх
// нативного примитива ОС - обобщение "Блокировки", internal/semantic/
// mutex.go: "захватить"/"освободить" не 0/1, а от 0 до N одновременных
// захватов, N - начальное число разрешений, docs/ROADMAP.md, "что ещё нужно
// для настоящего HTTP highload" - ограничить число одновременно живых
// "запустить"-задач сверху).
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

import "github.com/IlyaFeoktistov/Rech/internal/ir"

const semaphoreEntityName = "Семафор"

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
		"захватить":  {},
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
