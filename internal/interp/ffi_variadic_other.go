//go:build !(amd64 && (darwin || freebsd || linux || netbsd)) && !(darwin && arm64)

package interp

// Windows и arm64 (кроме macOS): аргументы "..." передаются так же, как
// обычные, - переходник не нужен.
func cVariadicEntry(fn uintptr) (uintptr, error) { return fn, nil }
