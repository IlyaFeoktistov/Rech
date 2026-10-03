package interp

import (
	"errors"
	"reflect"
	"runtime"
	"unsafe"
)

// На Windows purego.NewCallback - это syscall.NewCallback: параметры и
// результат колбэка обязаны быть размером в uintptr (целые и указатели
// приходят в целочисленных регистрах). Колбэк объявленной сигнатуры
// оборачивается функцией "все uintptr", которая приводит их к объявленным
// типам. Дробные параметры так не передаются (они в xmm-регистрах) -
// явная ошибка.
var callbackNeedsUintptrABI = runtime.GOOS == "windows"

var errCallbackFloatOnWindows = errors.New("на Windows интерпретатор не передаёт в колбэк дробные параметры и результат - соберите и запустите программу через `rech build`/`rech run`")

var uintptrType = reflect.TypeFor[uintptr]()

// uintptrCallback - impl (сигнатуры paramTypes -> resultTypes) в виде
// функции, у которой все параметры и результат - uintptr.
func uintptrCallback(impl reflect.Value, paramTypes, resultTypes []reflect.Type) (reflect.Value, error) {
	for _, t := range append(append([]reflect.Type{}, paramTypes...), resultTypes...) {
		if t.Kind() == reflect.Float32 || t.Kind() == reflect.Float64 || t.Kind() == reflect.Struct {
			return reflect.Value{}, errCallbackFloatOnWindows
		}
	}
	params := make([]reflect.Type, len(paramTypes))
	for i := range params {
		params[i] = uintptrType
	}
	var results []reflect.Type
	if len(resultTypes) > 0 {
		results = []reflect.Type{uintptrType}
	}
	fnType := reflect.FuncOf(params, results, false)
	return reflect.MakeFunc(fnType, func(raw []reflect.Value) []reflect.Value {
		args := make([]reflect.Value, len(raw))
		for i, rv := range raw {
			args[i] = fromUintptr(uintptr(rv.Uint()), paramTypes[i])
		}
		out := impl.Call(args)
		if len(results) == 0 {
			return nil
		}
		return []reflect.Value{reflect.ValueOf(toUintptr(out[0]))}
	}), nil
}

func fromUintptr(u uintptr, t reflect.Type) reflect.Value {
	switch t.Kind() {
	case reflect.Pointer:
		return reflect.NewAt(t.Elem(), addrPointer(u))
	case reflect.Bool:
		return reflect.ValueOf(u != 0)
	case reflect.String:
		return reflect.ValueOf(cString(u))
	}
	return reflect.ValueOf(u).Convert(t)
}

func toUintptr(v reflect.Value) uintptr {
	switch v.Kind() {
	case reflect.Pointer, reflect.UnsafePointer:
		return v.Pointer()
	case reflect.Bool:
		if v.Bool() {
			return 1
		}
		return 0
	case reflect.Int, reflect.Int8, reflect.Int16, reflect.Int32, reflect.Int64:
		return uintptr(v.Int())
	}
	return uintptr(v.Uint())
}

// cString - C-строка по адресу u (до нулевого байта).
func cString(u uintptr) string {
	if u == 0 {
		return ""
	}
	p := addrPointer(u)
	n := 0
	for *(*byte)(unsafe.Add(p, n)) != 0 {
		n++
	}
	return string(unsafe.Slice((*byte)(p), n))
}

// addrPointer - адрес из C (в регистре колбэка) как unsafe.Pointer; память
// принадлежит C, сборщик Go её не двигает.
func addrPointer(u uintptr) unsafe.Pointer {
	return *(*unsafe.Pointer)(unsafe.Pointer(&u))
}
