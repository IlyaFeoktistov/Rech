package interp

import "errors"

// macOS arm64: аргументы "..." передаются через стек, а не в регистрах, как
// обычные - purego так вызывать не умеет.
func cVariadicEntry(uintptr) (uintptr, error) {
	return 0, errors.New("вариативную C-функцию интерпретатор на macOS arm64 не вызывает - соберите и запустите программу через `rech build`/`rech run`")
}
