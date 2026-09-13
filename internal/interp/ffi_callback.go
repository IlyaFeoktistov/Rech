// FFI: маршалинг колбэков ("Вызываемый(...)" параметром внешней функции) -
// та же модель, что и cbackend (internal/backend/cbackend/ffi_callback.go):
// пул из ir.FFICallbackPoolSize (internal/ir/decl.go, общая константа с
// cbackend) одновременно живых слотов НА СИГНАТУРУ - каждая регистрация
// занимает СЛЕДУЮЩИЙ свободный слот, не заменяет предыдущий (раньше был
// единственный слот, "один активный колбэк", заменён пулом по прямому
// запросу поддержать реалистичный случай нескольких window class-подобных
// замыканий одной сигнатуры одновременно - см. docs/SPEC.md, раздел 12).
// Трамплин - свой на КАЖДЫЙ слот (purego.NewCallback может породить сколько
// угодно РАЗНЫХ C-адресов в рантайме - в отличие от cbackend, здесь не
// нужен компилятор C/статический пул трамплинов, но предел
// ir.FFICallbackPoolSize всё равно применяется, чтобы оба backend'а
// исчерпывали пул на одной и той же по счёту регистрации - иначе
// parity-тесты (TestRunAllExamplesMatchCompiledOutput) разошлись бы) -
// создаётся лениво, реализация трамплина читает СВОЙ индекс из общего
// слайса слотов на каждый вызов, а не запоминает callableValue на момент
// создания.
package interp

import (
	"fmt"
	"reflect"

	"github.com/IlyaFeoktistov/Rech/internal/ir"
	"github.com/IlyaFeoktistov/Rech/internal/rtmsg"
	"github.com/ebitengine/purego"
)

// registerCallbackArg - v (значение типа "Вызываемый(...)") занимает
// следующий свободный слот пула сигнатуры sigName (ir.Type.Entity
// параметра внешней функции) - паникует явно (возвращает ошибку), если пул
// уже исчерпан. Возвращает трамплин (голый C-адрес, свой на этот слот) для
// передачи внешней функции вместо {fn, env} по значению.
func (in *Interp) registerCallbackArg(sigName string, v Value) (uintptr, error) {
	slots := in.ffi.callbackSlots[sigName]
	if len(slots) >= ir.FFICallbackPoolSize {
		return 0, fmt.Errorf(rtmsg.FFICallbackPoolExhaustedTemplate, sigName, ir.FFICallbackPoolSize)
	}
	idx := len(slots)
	in.ffi.callbackSlots[sigName] = append(slots, v.Callable)
	return in.ensureCallbackTrampoline(sigName, idx)
}

// ensureCallbackTrampoline - трамплин слота idx сигнатуры sigName, лениво
// (один раз на пару (sigName, idx), кэш in.ffi.callbackAddrs) - сама
// C-функция читает in.ffi.callbackSlots[sigName][idx] на каждый вызов (не
// на момент создания трамплина, но ИМЕННО свой idx, не последний
// зарегистрированный), поэтому один и тот же адрес трамплина переживает
// дальнейшие регистрации ДРУГИХ слотов той же сигнатуры, не теряя свою
// собственную привязку.
func (in *Interp) ensureCallbackTrampoline(sigName string, idx int) (uintptr, error) {
	if addrs, ok := in.ffi.callbackAddrs[sigName]; ok && idx < len(addrs) {
		return addrs[idx], nil
	}
	sig, ok := in.callableSigs[sigName]
	if !ok {
		return 0, fmt.Errorf("внутренняя ошибка: сигнатура колбэка %q не найдена среди \"Вызываемый\" программы", sigName)
	}

	paramTypes := make([]reflect.Type, len(sig.Params))
	for i, p := range sig.Params {
		goType, err := in.ffiGoType(p)
		if err != nil {
			return 0, fmt.Errorf("колбэк %q: параметр %d: %w", sigName, i+1, err)
		}
		paramTypes[i] = goType
	}
	var resultTypes []reflect.Type
	if sig.Result != nil {
		goType, err := in.ffiGoType(*sig.Result)
		if err != nil {
			return 0, fmt.Errorf("колбэк %q: результат: %w", sigName, err)
		}
		resultTypes = []reflect.Type{goType}
	}

	fnType := reflect.FuncOf(paramTypes, resultTypes, false)
	impl := reflect.MakeFunc(fnType, func(callArgs []reflect.Value) []reflect.Value {
		cb := in.ffi.callbackSlots[sigName][idx]
		if cb == nil {
			panicRuntime("колбэк %q (слот %d) вызван из C, но замыкание для него сейчас не зарегистрировано", sigName, idx)
		}
		args := make([]Value, len(callArgs))
		for i, rv := range callArgs {
			args[i] = in.readFFIValue(rv, sig.Params[i])
		}
		result, err := cb.call(args, nil)
		if err != nil {
			panicRuntime("колбэк %q: %v", sigName, err)
		}
		if sig.Result == nil {
			return nil
		}
		return []reflect.Value{in.newFFIArgValue(result, resultTypes[0])}
	})

	addr := purego.NewCallback(impl.Interface())
	in.ffi.callbackAddrs[sigName] = append(in.ffi.callbackAddrs[sigName], addr)
	return addr, nil
}
