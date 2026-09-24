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

// Обратное направление - адрес C-функции, пришедший ОТ C (результат
// внешней функции, поле внешней структуры), становится значением
// Вызываемый, вызов которого зовёт этот адрес (purego.RegisterFunc) - тот
// же принцип, что у cbackend (ffi_callback.go, thunk сырого указателя).

// ffiFuncPointerSig - сигнатура указателя на функцию типа t: "Вызываемый"
// (nullable == false) или "?Вызываемый" (nullable == true).
func (in *Interp) ffiFuncPointerSig(t ir.Type) (sigName string, nullable, ok bool) {
	if t.Kind == ir.KindCallable {
		return t.Entity, false, true
	}
	if t.Kind == ir.KindNullable {
		if inner, found := in.nullableInner[t.Entity]; found && inner.Kind == ir.KindCallable {
			return inner.Entity, true, true
		}
	}
	return "", false, false
}

// ffiFuncPointerValue - значение типа t из адреса addr. NULL: "?Вызываемый"
// - Ничто; "Вызываемый" - ошибка с текстом nullMsg, а при nullMsg == ""
// (поле структуры, читается целиком) - значение без Callable, ошибка -
// при чтении самого поля (checkFFIFuncField).
func (in *Interp) ffiFuncPointerValue(t ir.Type, addr uintptr, nullMsg string) (Value, error) {
	sigName, nullable, _ := in.ffiFuncPointerSig(t)
	if addr == 0 {
		if nullable || nullMsg == "" {
			return Value{Type: t}, nil
		}
		return Value{}, fmt.Errorf("%s", nullMsg)
	}
	callableType := ir.Type{Kind: ir.KindCallable, Entity: sigName}
	v, err := in.callableFromRaw(callableType, addr)
	if err != nil {
		return Value{}, err
	}
	if nullable {
		return Value{Type: t, Nullable: &v}, nil
	}
	return v, nil
}

// callableFromRaw - Вызываемый сигнатуры t, зовущий C-функцию по адресу
// addr.
func (in *Interp) callableFromRaw(t ir.Type, addr uintptr) (Value, error) {
	sig, ok := in.callableSigs[t.Entity]
	if !ok {
		return Value{}, fmt.Errorf("внутренняя ошибка: сигнатура %q не найдена среди \"Вызываемый\" программы", t.Entity)
	}
	paramTypes := make([]reflect.Type, len(sig.Params))
	for i, p := range sig.Params {
		goType, err := in.ffiGoType(p)
		if err != nil {
			return Value{}, fmt.Errorf("указатель на функцию %s: параметр %d: %w", t.Entity, i+1, err)
		}
		paramTypes[i] = goType
	}
	var resultTypes []reflect.Type
	if sig.Result != nil {
		goType, err := in.ffiGoType(*sig.Result)
		if err != nil {
			return Value{}, fmt.Errorf("указатель на функцию %s: результат: %w", t.Entity, err)
		}
		resultTypes = []reflect.Type{goType}
	}
	fnPtr := reflect.New(reflect.FuncOf(paramTypes, resultTypes, false))
	purego.RegisterFunc(fnPtr.Interface(), addr)
	fn := fnPtr.Elem()
	return Value{Type: t, Callable: &callableValue{raw: addr, call: func(args []Value, _ *dbgThread) (Value, error) {
		argVals := make([]reflect.Value, len(args))
		for i, a := range args {
			argVals[i] = in.newFFIArgValue(a, paramTypes[i])
		}
		results := fn.Call(argVals)
		if sig.Result == nil {
			return Value{}, nil
		}
		return in.readFFIValue(results[0], *sig.Result), nil
	}}}, nil
}

// funcPointerArg - значение "Вызываемый"/"?Вызываемый" -> адрес для C:
// пришедшее от C уходит тем же адресом, Ничто - NULL, остальное занимает
// слот пула (registerCallbackArg).
func (in *Interp) funcPointerArg(v Value) (uintptr, error) {
	sigName, nullable, _ := in.ffiFuncPointerSig(v.Type)
	if nullable {
		if v.Nullable == nil {
			return 0, nil
		}
		v = *v.Nullable
	}
	if v.Callable != nil && v.Callable.raw != 0 {
		return v.Callable.raw, nil
	}
	return in.registerCallbackArg(sigName, v)
}

// checkFFIFuncField - чтение поля "Вызываемый" (без "?") внешней
// структуры, где C оставил NULL - паника с именем поля (cbackend паникует
// в той же точке).
func (in *Interp) checkFFIFuncField(recv Value, field string, v Value) {
	if v.Type.Kind != ir.KindCallable || v.Callable != nil {
		return
	}
	if ent, ok := in.entities[recv.Type.Entity]; ok && ent.IsExternStruct {
		panicRuntime(rtmsg.FFINullFuncPointerFieldTemplate, field, ent.ExternCName)
	}
}
