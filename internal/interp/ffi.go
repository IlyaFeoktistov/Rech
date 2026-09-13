// FFI ("внешняя функция", docs/SPEC.md, раздел 12) для интерпретатора -
// в отличие от cbackend (настоящая компиляция в C и статическая линковка),
// у чистого Go-интерпретатора нет cgo, поэтому вызов реального C-символа
// идёт через стороннюю purego (github.com/ebitengine/purego, тоже без cgo) -
// dlopen/dlsym в рантайме + маршалинг аргументов по System V/Win64/AAPCS64
// соглашению средствами reflect.
//
// Поддержаны скаляры (isFFIScalar, internal/semantic/types.go), "внешняя
// сущность", "внешняя структура" по значению (поля - рекурсивно те же
// формы, включая "Указатель<T>"/"Вызываемый(...)" - см. ffiStructGoType),
// "Указатель<T>"/"Вызываемый(...)" параметром (out-параметр/колбэк, см.
// examples/основы/внешняя_функция_указатель.rech, ffi_callback.go) И
// "Указатель<T>" результатом внешней функции. "Указатель<T>", пришедший
// ИЗ C (результат внешней функции или поле структуры, читаемое из C) -
// "сырой" pointerTarget (четвёртый случай, package doc internal/interp/
// pointer.go) - чтение/запись через unsafe.Pointer/reflect
// (readRawPointerValue/setRawPointerValue ниже), не через Env/entityObj/
// collection. "Вызываемый(...)" полем "внешняя структура" (WNDCLASS.
// lpfnWndProc-стиль) - трамплин через registerCallbackArg (ffi_callback.go),
// тот же приём, что и параметром внешней функции - cbackend зеркалит это
// же поведение своей стороной (internal/backend/cbackend/ir_generator.go,
// generateIREntityStruct/generateIRNewExpr/FieldAssign - голый указатель
// на функцию в C-структуре, не {fn, env}).
//
// НЕ поддержано - запись "Указатель<T>", несущего RECH-хранилище (не
// "сырой", т. е. `&переменная`, не результат FFI), ПОЛЕМ внешней структуры
// (в отличие от параметра внешней функции, ffiPointerArg - там есть куда
// подвесить runtime.Pinner/writeback до конца ОДНОГО вызова; поле
// структуры может пережить вызов) - явная паника при попытке, не тихая
// порча. Чтение "Вызываемый(...)" ПОЛЯ, пришедшего из C, обратно в
// вызываемое Rech-значение - тоже не поддержано (обратный трамплин "raw C
// указатель -> Rech-значение" не реализован) - такое поле читается как
// обычный адрес (Value.I), просто не вызываемый (Callable == nil,
// evalCallValueExpr сам даёт понятную ошибку при попытке вызвать).
package interp

import (
	"fmt"
	"path/filepath"
	"reflect"
	"runtime"
	"unsafe"

	"github.com/IlyaFeoktistov/Rech/internal/ir"
	"github.com/ebitengine/purego"
)

// ffiRegistry - состояние FFI одного запуска интерпретатора: открытые
// (dlopen) библиотеки по имени файла (не открывать повторно ту же) и уже
// зарегистрированные (purego.RegisterFunc) вызываемые обёртки внешних
// функций по Alias (тот же ключ, что и ir.CallExpr.Callee для внешней
// функции) - обёртка строится один раз при первом вызове, дальше переиспользуется.
type ffiRegistry struct {
	libs map[string]uintptr
	fns  map[string]reflect.Value

	// callbackSlots/callbackAddrs - "Вызываемый(...)" параметром внешней
	// функции, по имени сигнатуры (ir.Type.Entity) - пул из ir.
	// FFICallbackPoolSize одновременно живых слотов на сигнатуру (индекс -
	// порядковый номер регистрации), см. package doc ffi_callback.go.
	callbackSlots map[string][]*callableValue
	callbackAddrs map[string][]uintptr

	// structTypes - "внешняя структура" по имени сущности -> уже построенный
	// (reflect.StructOf) Go-тип, см. ffiStructGoType.
	structTypes map[string]reflect.Type
}

// defaultFFILibraryNames - библиотеки, которые пробуются для резолва
// C-символа, если программа не подключает ничего своего (см.
// "сборка.библиотеки"/"путь_библиотек", ещё не проброшены в interp) -
// тот же набор, что cbackend всегда линкует безусловно (libc/libm, docs/
// SPEC.md - "-lm всегда линкуемый"). Известное упрощение - имена собраны
// под glibc/стандартный Windows/macOS рантайм, не musl и т. п. (тот же
// класс допущения, что уже есть у cbackend).
func defaultFFILibraryNames() []string {
	switch runtime.GOOS {
	case "windows":
		// kernel32.dll - как и libc/libm на других платформах, доступна любому
		// Windows-процессу без явного подключения (cbackend её тоже не
		// перечисляет отдельно - компоновщик MinGW/MSVC линкует её всегда).
		return []string{"ucrtbase.dll", "msvcrt.dll", "kernel32.dll"}
	case "darwin":
		return []string{"libSystem.dylib"}
	default:
		return []string{"libm.so.6", "libc.so.6"}
	}
}

// ffiScalarGoTypes - ir.TypeKind -> reflect.Type примитива Go, которым
// purego.RegisterFunc маршалит аргумент/результат на границе C ABI (см. её
// package doc, "Type Conversions (Go <=> C)"). Ровно те Kind, что разрешает
// isFFIScalar (internal/semantic/types.go) - остальные (Указатель/внешняя
// сущность/Вызываемый/внешняя структура) обрабатываются отдельно, позже.
var ffiScalarGoTypes = map[ir.TypeKind]reflect.Type{
	ir.KindInt:     reflect.TypeFor[int64](),
	ir.KindFloat:   reflect.TypeFor[float64](),
	ir.KindFloat32: reflect.TypeFor[float32](),
	ir.KindBool:    reflect.TypeFor[bool](),
	ir.KindByte:    reflect.TypeFor[uint8](),
	ir.KindChar:    reflect.TypeFor[int32](),
	ir.KindString:  reflect.TypeFor[string](),
}

// ffiGoType - Go-тип, которым будет маршалиться значение типа t на границе
// FFI, или ошибка, если t пока не поддержан интерпретатором (см. package
// doc). "Указатель<T>" - рекурсивно, *(Go-тип T) (in.pointerElemTypes,
// ir.Program.Pointers - T ещё не известен по одному только t.Entity).
func (in *Interp) ffiGoType(t ir.Type) (reflect.Type, error) {
	if goType, ok := ffiScalarGoTypes[t.Kind]; ok {
		return goType, nil
	}
	if t.Kind == ir.KindExternEntity {
		// "внешняя сущность" - непрозрачный указатель, Value несёт его как
		// есть в поле I (тот же приём, что и у Байт/Символ - сама структура
		// Value не даёт отдельного поля под каждый Kind).
		return reflect.TypeFor[uintptr](), nil
	}
	if t.Kind == ir.KindPointer {
		elem, ok := in.pointerElemTypes[t.Entity]
		if !ok {
			return nil, fmt.Errorf("внутренняя ошибка: инстанциация %q не найдена среди \"Указатель<T>\" программы", t.Entity)
		}
		elemGoType, err := in.ffiGoType(elem)
		if err != nil {
			return nil, fmt.Errorf("%s: %w", t.String(), err)
		}
		return reflect.PointerTo(elemGoType), nil
	}
	if t.Kind == ir.KindCallable {
		// "Вызываемый(...)" - на границе FFI это голый C-указатель на
		// функцию (трамплин), не {fn, env} по значению - см. ffi_callback.go.
		return reflect.TypeFor[uintptr](), nil
	}
	if t.Kind == ir.KindEntity {
		if ent, ok := in.entities[t.Entity]; ok && ent.IsExternStruct {
			return in.ffiStructGoType(ent)
		}
	}
	return nil, fmt.Errorf("тип %s пока не поддержан FFI интерпретатора (реализованы только примитивы: Целое/Действительное/Дробное/Логическое/Байт/Символ/Строка, \"внешняя сущность\", \"внешняя структура\", \"Указатель<T>\" параметром и \"Вызываемый(...)\" параметром) - соберите и запустите программу через `rech build`/`rech run`", t.String())
}

// ffiStructGoType - Go-структура (reflect.StructOf), которой маршалится
// "внешняя структура" ent по значению - поле за полем, в ТОМ ЖЕ порядке
// (порядок полей C-структуры важен для layout, см. docs/SPEC.md, раздел
// 12) - имена Go-полей ("F0", "F1", ...) не совпадают с Rech-именами
// (кириллица - не всегда валидный экспортируемый Go-идентификатор), но
// порядок/типы совпадают, что и определяет реальный layout для purego.
// "Указатель<T>"/"Вызываемый(...)" полем - тоже строятся (тем же ffiGoType,
// что и параметр) - оставшиеся ограничения на конкретные ЗНАЧЕНИЯ таких
// полей (не на сам тип) описаны в package doc файла и проверяются в
// рантайме (setFFIValue), не здесь.
func (in *Interp) ffiStructGoType(ent *ir.Entity) (reflect.Type, error) {
	if t, ok := in.ffi.structTypes[ent.Name]; ok {
		return t, nil
	}
	fields := make([]reflect.StructField, len(ent.Fields))
	for i, f := range ent.Fields {
		goType, err := in.ffiGoType(f.Type)
		if err != nil {
			return nil, fmt.Errorf("внешняя структура %q: поле %q: %w", ent.Name, f.Name, err)
		}
		fields[i] = reflect.StructField{Name: fmt.Sprintf("F%d", i), Type: goType}
	}
	t := reflect.StructOf(fields)
	in.ffi.structTypes[ent.Name] = t
	return t, nil
}

// setFFIValue - записывает v В уже адресуемый rv (типа, построенного
// in.ffiGoType(v.Type)) - общая часть newFFIArgValue (аргумент) и
// ffiPointerArg (буфер out-параметра) ниже. "внешняя структура" - рекурсивно,
// поле за полем, в том же порядке, что и ffiStructGoType (все формы полей
// там уже проверены заранее - здесь только маршалинг значения, без ошибок).
func (in *Interp) setFFIValue(rv reflect.Value, v Value) {
	if v.Type.Kind == ir.KindEntity {
		ent := in.entities[v.Type.Entity]
		for i, f := range ent.Fields {
			in.setFFIValue(rv.Field(i), v.Obj.getField(f.Name))
		}
		return
	}
	if v.Type.Kind == ir.KindPointer {
		if !v.Ptr.isRaw {
			// "Указатель<T>", несущий RECH-хранилище (не результат FFI) -
			// полем структуры/вложенно не поддержано, см. package doc файла
			// (в отличие от параметра внешней функции - там runtime.Pinner
			// подвешен на время ОДНОГО вызова, ffiPointerArg).
			panicRuntime("\"Указатель<T>\" полем внешней структуры/аргументом FFI поддержан только для адреса, уже пришедшего из C - не для &переменная")
		}
		rv.Set(reflect.NewAt(rv.Type().Elem(), v.Ptr.raw))
		return
	}
	if v.Type.Kind == ir.KindCallable {
		addr, err := in.registerCallbackArg(v.Type.Entity, v)
		if err != nil {
			panicRuntime("%v", err)
		}
		rv.SetUint(uint64(addr))
		return
	}
	switch rv.Kind() {
	case reflect.String:
		rv.SetString(v.S)
	case reflect.Bool:
		rv.SetBool(v.B)
	case reflect.Float32, reflect.Float64:
		rv.SetFloat(v.F)
	case reflect.Uint8, reflect.Uintptr:
		rv.SetUint(uint64(v.I))
	default: // int32 (Символ)/int64 (Целое)
		rv.SetInt(v.I)
	}
}

// readFFIValue - обратное направление setFFIValue, rv (того же строения) ->
// Value типа irType. "внешняя структура" - рекурсивно, собирает свежий
// entityObj полем за полем (тот же порядок, что и ffiStructGoType).
func (in *Interp) readFFIValue(rv reflect.Value, irType ir.Type) Value {
	if irType.Kind == ir.KindEntity {
		ent := in.entities[irType.Entity]
		obj := &entityObj{typeName: ent.Name, fields: map[string]Value{}}
		for i, f := range ent.Fields {
			obj.fields[f.Name] = in.readFFIValue(rv.Field(i), f.Type)
		}
		return Value{Type: irType, Obj: obj}
	}
	if irType.Kind == ir.KindPointer {
		// Адрес пришёл ИЗ C (результат внешней функции или поле структуры) -
		// "сырой" pointerTarget (package doc файла) - не Env/entityObj/
		// collection.
		elem := in.pointerElemTypes[irType.Entity]
		return rawPointerValue(irType, elem, rv.UnsafePointer(), in)
	}
	switch rv.Kind() {
	case reflect.String:
		return Value{Type: irType, S: rv.String()}
	case reflect.Bool:
		return Value{Type: irType, B: rv.Bool()}
	case reflect.Float32, reflect.Float64:
		return Value{Type: irType, F: rv.Float()}
	case reflect.Uint8, reflect.Uintptr:
		// "Вызываемый(...)", прочитанный из C (поле структуры/результат) -
		// попадает сюда тоже (Go-тип uintptr, package doc файла) - несёт
		// адрес в Value.I, но Callable остаётся nil (значение НЕ вызываемое -
		// обратный трамплин "raw-указатель -> Rech-значение" не реализован) -
		// evalCallValueExpr сам даёт понятную ошибку при попытке вызвать.
		return Value{Type: irType, I: int64(rv.Uint())}
	default:
		return Value{Type: irType, I: rv.Int()}
	}
}

// newFFIArgValue - Value -> reflect.Value типа goType, для передачи в
// fn.Call(...) ниже (аргумент-НЕ-указатель - см. ffiPointerArg для
// "Указатель<T>").
func (in *Interp) newFFIArgValue(v Value, goType reflect.Type) reflect.Value {
	rv := reflect.New(goType).Elem()
	in.setFFIValue(rv, v)
	return rv
}

// ffiPointerArg - "Указатель<T>" аргументом внешней функции.
//
// v.Ptr - "сырой" (isRaw, package doc файла - сам пришёл из C ранее,
// например результат предыдущего вызова) - адрес уже настоящий, писать
// некуда (значение принадлежит C, не Rech) - оборачивается как есть, без
// копии и без writeback.
//
// v.Ptr - обычный RECH-адрес (out-параметр в духе modf(x, &целая)) - НЕ
// настоящий устойчивый адрес (Go-карты неадресуемы), поэтому заводится
// временный, закреплённый от переезда GC (runtime.Pinner) Go-буфер нужного
// размера: текущее значение копируется В него ДО вызова, адрес буфера идёт
// в C, а после вызова (writeback) буфер копируется ОБРАТНО в v.Ptr - тот
// же приём, каким pure-Go FFI обычно обходится без cgo для out-параметров.
// elemType - T (уже известен из ir.Program.Pointers, см. ffiGoType),
// goType - *(Go-тип T).
func (in *Interp) ffiPointerArg(v Value, elemType ir.Type, goType reflect.Type) (arg reflect.Value, writeback func()) {
	if v.Ptr.isRaw {
		return reflect.NewAt(goType.Elem(), v.Ptr.raw), func() {}
	}

	buf := reflect.New(goType.Elem())
	in.setFFIValue(buf.Elem(), v.Ptr.get())

	var pinner runtime.Pinner
	pinner.Pin(buf.Interface())
	writeback = func() {
		v.Ptr.set(in.readFFIValue(buf.Elem(), elemType))
		pinner.Unpin()
	}
	return buf, writeback
}

// readRawPointerValue/setRawPointerValue - чтение/запись T по сырому чужому
// (C) адресу addr - unsafe.Pointer+reflect.NewAt (тот же приём, каким
// pure-Go FFI без cgo вообще получает доступ к памяти по known-типу) - см.
// package doc internal/interp/pointer.go, четвёртый ("raw") случай
// pointerTarget - это то, что вызывают "*указатель"/"*указатель = знач"
// (evalDerefExpr/execDerefAssignStmt) через v.Ptr.get()/set(), когда указатель
// сырой.
func (in *Interp) readRawPointerValue(addr unsafe.Pointer, elemType ir.Type) Value {
	goType, err := in.ffiGoType(elemType)
	if err != nil {
		panicRuntime("%v", err)
	}
	return in.readFFIValue(reflect.NewAt(goType, addr).Elem(), elemType)
}

func (in *Interp) setRawPointerValue(addr unsafe.Pointer, elemType ir.Type, v Value) {
	goType, err := in.ffiGoType(elemType)
	if err != nil {
		panicRuntime("%v", err)
	}
	in.setFFIValue(reflect.NewAt(goType, addr).Elem(), v)
}

// ensureExternFunc - находит (лениво, один раз на Alias, кэш in.ffi.fns) C-
// символ decl.CName среди defaultFFILibraryNames(), строит его Go-сигнатуру
// через reflect.FuncOf (по ffiGoType каждого параметра/результата) и
// регистрирует настоящий вызывающий stub через purego.RegisterFunc.
func (in *Interp) ensureExternFunc(decl *ir.ExternFuncDecl) (reflect.Value, error) {
	if fn, ok := in.ffi.fns[decl.Alias]; ok {
		return fn, nil
	}

	paramTypes := make([]reflect.Type, len(decl.Params))
	for i, p := range decl.Params {
		goType, err := in.ffiGoType(p)
		if err != nil {
			return reflect.Value{}, fmt.Errorf("внешняя функция %q: параметр %d: %w", decl.Alias, i+1, err)
		}
		paramTypes[i] = goType
	}
	var resultTypes []reflect.Type
	if decl.HasReturn {
		goType, err := in.ffiGoType(decl.ReturnType)
		if err != nil {
			return reflect.Value{}, fmt.Errorf("внешняя функция %q: результат: %w", decl.Alias, err)
		}
		resultTypes = []reflect.Type{goType}
	}

	addr, err := in.resolveFFISymbol(decl.CName)
	if err != nil {
		return reflect.Value{}, fmt.Errorf("внешняя функция %q (%s): %w", decl.Alias, decl.CName, err)
	}

	fnType := reflect.FuncOf(paramTypes, resultTypes, false)
	fnPtr := reflect.New(fnType)
	purego.RegisterFunc(fnPtr.Interface(), addr)

	fn := fnPtr.Elem()
	in.ffi.fns[decl.Alias] = fn
	return fn, nil
}

// ffiLibraryCandidates - имена ФАЙЛОВ библиотеки name, которые стоит
// попробовать dlopen'ом (в отличие от линкера, которому достаточно "-l
// <name>", dlopen ищет по имени файла, не по "-l"-имени) - неверсионированное
// имя (обычно даёт только dev-пакет - тот же практический гэп, что уже
// неявно есть у cbackend при линковке через "-l<name>" (docs/SPEC.md,
// раздел 12) - если "сборка.библиотеки: curl" уже собирается cbackend'ом
// на этой машине, значит неверсионированный файл уже есть, не новое
// требование именно для interp).
func ffiLibraryCandidates(name string) []string {
	switch runtime.GOOS {
	case "windows":
		return []string{name + ".dll", "lib" + name + ".dll"}
	case "darwin":
		return []string{"lib" + name + ".dylib"}
	default:
		return []string{"lib" + name + ".so"}
	}
}

// resolveFFISymbol - dlopen (лениво, кэш in.ffi.libs) по очереди: сначала
// каждая библиотека из "сборка.библиотеки" (in.ffiLibraries) - в каждом
// каталоге "путь_библиотек" (in.ffiLibraryPaths) явным путём, затем голым
// именем (обычные системные каталоги/rpath - тот же поиск, что и у
// линкованного cbackend-бинарника); затем defaultFFILibraryNames() (libc/
// libm, всегда доступны, тот же принцип, что и "-lm" cbackend'а). dlsym по
// cname в первой библиотеке, где символ нашёлся.
func (in *Interp) resolveFFISymbol(cname string) (uintptr, error) {
	var tried []string
	for _, name := range in.ffiLibraries {
		for _, candidate := range ffiLibraryCandidates(name) {
			for _, dir := range append(in.ffiLibraryPaths, "") {
				libPath := candidate
				if dir != "" {
					libPath = filepath.Join(dir, candidate)
				}
				tried = append(tried, libPath)
				if addr, ok := in.tryFFISymbol(libPath, cname); ok {
					return addr, nil
				}
			}
		}
	}
	for _, libName := range defaultFFILibraryNames() {
		tried = append(tried, libName)
		if addr, ok := in.tryFFISymbol(libName, cname); ok {
			return addr, nil
		}
	}
	return 0, fmt.Errorf("символ %q не найден ни в одной из %v", cname, tried)
}

// tryFFISymbol - dlopen(libPath) (лениво, кэш in.ffi.libs - не открывать
// повторно ту же библиотеку) + dlsym(cname); false вторым результатом -
// либо библиотека не открылась, либо в ней нет такого символа, в обоих
// случаях вызывающая сторона (resolveFFISymbol) просто пробует следующий
// кандидат, не различая эти два случая по отдельности.
func (in *Interp) tryFFISymbol(libPath, cname string) (uintptr, bool) {
	handle, ok := in.ffi.libs[libPath]
	if !ok {
		h, err := ffiDlopen(libPath)
		if err != nil {
			in.ffi.libs[libPath] = 0
			return 0, false
		}
		handle = h
		in.ffi.libs[libPath] = handle
	}
	if handle == 0 {
		return 0, false
	}
	addr, err := ffiDlsym(handle, cname)
	if err != nil {
		return 0, false
	}
	return addr, true
}

// callExternFunc - "внешняя функция" в позиции вызова (ir.CallExpr), см.
// eval.go, case *ir.CallExpr.
func (in *Interp) callExternFunc(decl *ir.ExternFuncDecl, args []Value) (Value, error) {
	fn, err := in.ensureExternFunc(decl)
	if err != nil {
		return Value{}, err
	}
	argVals := make([]reflect.Value, len(args))
	var writebacks []func()
	for i, a := range args {
		paramType := fn.Type().In(i)
		switch decl.Params[i].Kind {
		case ir.KindPointer:
			elemType := in.pointerElemTypes[decl.Params[i].Entity]
			arg, writeback := in.ffiPointerArg(a, elemType, paramType)
			argVals[i] = arg
			writebacks = append(writebacks, writeback)
		case ir.KindCallable:
			addr, err := in.registerCallbackArg(decl.Params[i].Entity, a)
			if err != nil {
				return Value{}, fmt.Errorf("внешняя функция %q: параметр %d: %w", decl.Alias, i+1, err)
			}
			argVals[i] = reflect.ValueOf(addr)
		default:
			argVals[i] = in.newFFIArgValue(a, paramType)
		}
	}
	results := fn.Call(argVals)
	for _, wb := range writebacks {
		wb()
	}
	if !decl.HasReturn {
		return Value{}, nil
	}
	return in.readFFIValue(results[0], decl.ReturnType), nil
}
