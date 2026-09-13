//go:build windows

// ffiDlopen/ffiDlsym - реализация для Windows: purego сама НЕ даёт dlopen/
// dlsym здесь (её dlfcn.go собирается только под Unix-подобные GOOS - живая
// находка на настоящей Windows, не просто кросс-компиляция - "purego.
// Dlopen" неопределён при сборке под windows/amd64) - используется
// стандартный syscall.LoadLibrary/GetProcAddress (тот же смысл: открыть
// библиотеку по имени файла/резолвить адрес символа по имени, просто под
// другим названием) - возвращаемый адрес дальше передаётся в ту же самую
// purego.RegisterFunc/NewCallback (они кросс-платформенны, см. func.go).
// Общий, платформонезависимый вызывающий код - internal/interp/ffi.go,
// tryFFISymbol.
package interp

import "syscall"

func ffiDlopen(path string) (uintptr, error) {
	h, err := syscall.LoadLibrary(path)
	if err != nil {
		return 0, err
	}
	return uintptr(h), nil
}

func ffiDlsym(handle uintptr, name string) (uintptr, error) {
	return syscall.GetProcAddress(syscall.Handle(handle), name)
}
