//go:build windows

package rapidocr

import (
	"syscall"

	"github.com/ebitengine/purego"
)

// dlopen loads a shared library and returns its handle.
func dlopen(path string) (uintptr, error) {
	h, err := syscall.LoadLibrary(path)
	return uintptr(h), err
}

// registerLibFunc binds the named export to fn.
func registerLibFunc(fn any, handle uintptr, name string) {
	addr, err := syscall.GetProcAddress(syscall.Handle(handle), name)
	if err != nil {
		panic("purego: GetProcAddress failed for " + name + ": " + err.Error())
	}
	purego.RegisterFunc(fn, addr)
}

// lookupFunc returns the address of the named export, or 0 when missing.
func lookupFunc(handle uintptr, name string) uintptr {
	addr, err := syscall.GetProcAddress(syscall.Handle(handle), name)
	if err != nil {
		return 0
	}
	return addr
}
