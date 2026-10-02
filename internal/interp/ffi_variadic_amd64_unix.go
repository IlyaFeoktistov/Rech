//go:build amd64 && (darwin || freebsd || linux || netbsd)

package interp

import (
	"encoding/binary"
	"fmt"
	"sync"
	"syscall"
	"unsafe"
)

// System V amd64: вызывающий вариативной функции кладёт в AL верхнюю
// границу числа аргументов в регистрах xmm. purego всегда ставит AL = 0 -
// printf и подобные тогда не читают double-аргументы. Переходник "mov al,
// 8; jmp [rip+0]; <адрес>" ставит AL = 8 (все xmm-регистры аргументов) и
// передаёт управление настоящей функции, не трогая остальные регистры.

const varargThunkSize = 16

var varargThunks struct {
	sync.Mutex
	page []byte
	used int
	byFn map[uintptr]uintptr
}

// cVariadicEntry - адрес, по которому вызывать вариативную C-функцию fn.
func cVariadicEntry(fn uintptr) (uintptr, error) {
	varargThunks.Lock()
	defer varargThunks.Unlock()
	if t, ok := varargThunks.byFn[fn]; ok {
		return t, nil
	}
	pageSize := syscall.Getpagesize()
	if varargThunks.page == nil || varargThunks.used+varargThunkSize > pageSize {
		page, err := syscall.Mmap(-1, 0, pageSize, syscall.PROT_READ|syscall.PROT_WRITE, syscall.MAP_ANON|syscall.MAP_PRIVATE)
		if err != nil {
			return 0, fmt.Errorf("переходник вариативной C-функции: %w", err)
		}
		varargThunks.page, varargThunks.used = page, 0
		if varargThunks.byFn == nil {
			varargThunks.byFn = map[uintptr]uintptr{}
		}
	} else if err := syscall.Mprotect(varargThunks.page, syscall.PROT_READ|syscall.PROT_WRITE); err != nil {
		return 0, fmt.Errorf("переходник вариативной C-функции: %w", err)
	}
	code := varargThunks.page[varargThunks.used : varargThunks.used+varargThunkSize]
	copy(code, []byte{0xB0, 0x08, 0xFF, 0x25, 0, 0, 0, 0}) // mov al, 8; jmp [rip+0]
	binary.LittleEndian.PutUint64(code[8:], uint64(fn))
	if err := syscall.Mprotect(varargThunks.page, syscall.PROT_READ|syscall.PROT_EXEC); err != nil {
		return 0, fmt.Errorf("переходник вариативной C-функции: %w", err)
	}
	t := uintptr(unsafe.Pointer(&code[0]))
	varargThunks.used += varargThunkSize
	varargThunks.byFn[fn] = t
	return t, nil
}
