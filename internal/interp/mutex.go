// Синхронизация между потоками - "Блокировка" (см. package doc
// internal/backend/cbackend/mutex.go для полного описания примитива и
// решения дизайна - здесь только то, что специфично интерпретатору).
// Go sync.Mutex - естественный аналог нативного мьютекса ОС, поэтому в
// отличие от cbackend здесь не нужно ни собственного C-подобного
// представления, ни отдельной регистрации в internal/ir.Program.Entities -
// evalNewExpr перехватывает "новая Блокировка()" раньше обычного пути
// (in.entities[e.Entity]) и заводит sync.Mutex напрямую.
package interp

import (
	"fmt"
	"sync"

	"github.com/IlyaFeoktistov/Rech/internal/ir"
)

const mutexEntityName = "Блокировка"

// newMutexValue - "новая Блокировка()".
func newMutexValue() Value {
	return Value{Type: ir.EntityType(mutexEntityName), Obj: &entityObj{typeName: mutexEntityName, mutex: &sync.Mutex{}}}
}

// evalMutexMethod - "захватить"/"освободить" на значении "Блокировка" -
// recv.Obj.mutex != nil отличает эту ветку от обычного метода/generic-
// коллекции (см. evalMethodCallExpr). Оба метода без результата, как и в
// cbackend (funcSig без ReturnType, internal/semantic/mutex.go).
func evalMutexMethod(obj *entityObj, e *ir.MethodCallExpr) (Value, error) {
	switch e.Method {
	case "захватить":
		obj.mutex.Lock()
	case "освободить":
		obj.mutex.Unlock()
	default:
		return Value{}, fmt.Errorf("у сущности %q нет метода %q", mutexEntityName, e.Method)
	}
	return Value{}, nil
}
