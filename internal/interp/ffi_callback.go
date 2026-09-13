// FFI: маршалинг колбэков ("Вызываемый(...)" параметром внешней функции) -
// та же модель, что и cbackend (internal/backend/cbackend/ffi_callback.go):
// один активный колбэк на СИГНАТУРУ, не на место вызова - регистрация
// нового замыкания заменяет предыдущее (тот же принцип, что signal()/
// atexit() в реальном C API, docs/SPEC.md, раздел 12). Трамплин - один на
// сигнатуру, создаётся лениво через purego.NewCallback и переиспользуется -
// сама реализация трамплина читает текущий callableValue из общего слота на
// каждый вызов, а не запоминает его на момент создания.
package interp

import (
	"fmt"
	"reflect"

	"github.com/ebitengine/purego"
)

// registerCallbackArg - v (значение типа "Вызываемый(...)") становится
// активным колбэком сигнатуры sigName (ir.Type.Entity параметра внешней
// функции) - заменяет предыдущий, если такой был. Возвращает трамплин
// (голый C-адрес) для передачи внешней функции вместо {fn, env} по
// значению.
func (in *Interp) registerCallbackArg(sigName string, v Value) (uintptr, error) {
	in.ffi.callbackSlots[sigName] = v.Callable
	return in.ensureCallbackTrampoline(sigName)
}

// ensureCallbackTrampoline - трамплин сигнатуры sigName, лениво (один раз на
// sigName, кэш in.ffi.callbackAddrs) - сама C-функция читает
// in.ffi.callbackSlots[sigName] на каждый вызов (не на момент создания
// трамплина), поэтому один и тот же адрес трамплина переживает
// registerCallbackArg выше сколько угодно раз подряд.
func (in *Interp) ensureCallbackTrampoline(sigName string) (uintptr, error) {
	if addr, ok := in.ffi.callbackAddrs[sigName]; ok {
		return addr, nil
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
		cb := in.ffi.callbackSlots[sigName]
		if cb == nil {
			panicRuntime("колбэк %q вызван из C, но ни одно Rech-замыкание для него сейчас не зарегистрировано", sigName)
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
	in.ffi.callbackAddrs[sigName] = addr
	return addr, nil
}
