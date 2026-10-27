//go:build linux || darwin

package rapidocr

import "github.com/ebitengine/purego"

// dlopen loads a shared library and returns its handle.
func dlopen(path string) (uintptr, error) {
	return purego.Dlopen(path, purego.RTLD_NOW|purego.RTLD_GLOBAL)
}

// registerLibFunc binds the named export to fn.
func registerLibFunc(fn any, handle uintptr, name string) {
	purego.RegisterLibFunc(fn, handle, name)
}

// lookupFunc returns the address of the named export, or 0 when missing.
func lookupFunc(handle uintptr, name string) uintptr {
	addr, err := purego.Dlsym(handle, name)
	if err != nil {
		return 0
	}
	return addr
}
