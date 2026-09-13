//go:build darwin || freebsd || linux || netbsd

// ffiDlopen/ffiDlsym - реализация для Unix-подобных систем, где purego сама
// даёт dlopen/dlsym (dlfcn.go в самой purego собирается только под этот же
// набор GOOS - Windows ТУДА не входит, там своя реализация, см.
// ffi_dlopen_windows.go). Общий, платформонезависимый вызывающий код -
// internal/interp/ffi.go, tryFFISymbol.
package interp

import "github.com/ebitengine/purego"

func ffiDlopen(path string) (uintptr, error) {
	return purego.Dlopen(path, purego.RTLD_NOW|purego.RTLD_GLOBAL)
}

func ffiDlsym(handle uintptr, name string) (uintptr, error) {
	return purego.Dlsym(handle, name)
}
