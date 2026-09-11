// Синхронизация между потоками - "Блокировка" (голый примитив поверх
// нативного мьютекса ОС, без встроенной проверки гонок - решение
// пользователя явно, "Синхронизация между потоками"):
// `новая Блокировка()`, методы `захватить()`/`освободить()` без параметров
// и результата. Не объявляется пользователем (нет ast.EntityDecl) - её
// C-представление и код методов целиком в internal/backend/cbackend/
// mutex.go (нативный примитив ОС не выразить инструкциями Rech), здесь -
// только сигнатуры для проверки типов и связывание с internal/interp
// (sync.Mutex).
package semantic

import "github.com/IlyaFeoktistov/Rech/internal/ir"

const mutexEntityName = "Блокировка"

// registerMutexEntity - лениво регистрирует "Блокировка" в entities/methods
// при первой встрече имени типа (тем же принципом, что resolveGenericTypeName
// для Список/Стопка/Очередь, types.go) - без параметра типа, поэтому
// регистрация фиксированная, не по шаблону.
func registerMutexEntity(entities map[string]entitySig, methods map[string]map[string]funcSig) {
	if _, exists := entities[mutexEntityName]; exists {
		return
	}
	entities[mutexEntityName] = entitySig{IsMutex: true}
	methods[mutexEntityName] = map[string]funcSig{
		"захватить":  {},
		"освободить": {},
	}
}

// mutexIRType - собственная сущность "Блокировка" в готовом IR-виде,
// добавляется в ir.Program.Entities только если использована (см.
// internal/semantic/analyzer.go, buildIR) - Fields/Methods пусты, обе они
// синтезируются backend'ом напрямую по IsMutex, не по обычным ir.Method
// (см. package doc ir.Entity.IsMutex).
func mutexIRType() *ir.Entity {
	return &ir.Entity{Name: mutexEntityName, IsMutex: true}
}
