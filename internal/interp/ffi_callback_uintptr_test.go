package interp

import (
	"os"
	"path/filepath"
	"testing"
)

// TestCallbackUintptrABI - обёртка колбэка для Windows (все параметры и
// результат - uintptr) даёт тот же результат, что и прямой колбэк.
func TestCallbackUintptrABI(t *testing.T) {
	old := callbackNeedsUintptrABI
	callbackNeedsUintptrABI = true
	t.Cleanup(func() { callbackNeedsUintptrABI = old })
	src, err := os.ReadFile(filepath.Join("..", "..", "examples", "указатели", "массив_по_указателю.rech"))
	if err != nil {
		t.Fatal(err)
	}
	if got, want := runSrc(t, string(src)), "0 1 2 3 4 \n3 4\n30 30\n"; got != want {
		t.Fatalf("получено %q, ожидалось %q", got, want)
	}
}
