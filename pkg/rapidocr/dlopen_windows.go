//go:build windows

package rapidocr

import (
	"os"
	"path/filepath"
	"syscall"
	"unsafe"

	"github.com/ebitengine/purego"
)

// loadLibraryExFlags searches the library's own directory for its
// dependencies (onnxruntime.dll, opencv dlls) so a local build is not
// shadowed by older copies in System32.
const loadLibrarySearchDirs = 0x00000100 | // LOAD_LIBRARY_SEARCH_DLL_LOAD_DIR
	0x00001000 | // LOAD_LIBRARY_SEARCH_APPLICATION_DIR
	0x00002000 | // LOAD_LIBRARY_SEARCH_USER_DIRS
	0x00004000 // LOAD_LIBRARY_SEARCH_SYSTEM32

var (
	modkernel32         = syscall.NewLazyDLL("kernel32.dll")
	procLoadLibraryExW  = modkernel32.NewProc("LoadLibraryExW")
	procAddDllDirectory = modkernel32.NewProc("AddDllDirectory")
)

// siblingDeps are loaded by absolute path before the engine library itself:
// once a module name is in the loaded-module table, the loader binds the
// engine's imports to it instead of searching System32.
var siblingDeps = []string{"onnxruntime.dll", "onnxruntime-1.20.dll"}

// dlopen loads a shared library and returns its handle. Sibling dependency
// libraries next to path are preloaded so a local build is not shadowed by
// older copies already present on the system (e.g. System32).
func dlopen(path string) (uintptr, error) {
	dir, ferr := filepath.Abs(filepath.Dir(path))
	if ferr == nil {
		if dirPtr, perr := syscall.UTF16PtrFromString(dir); perr == nil {
			procAddDllDirectory.Call(uintptr(unsafe.Pointer(dirPtr)))
		}
		for _, dep := range siblingDeps {
			depPath := filepath.Join(dir, dep)
			if _, serr := os.Stat(depPath); serr == nil {
				syscall.LoadLibrary(depPath)
			}
		}
	}

	// LOAD_LIBRARY_SEARCH_DLL_LOAD_DIR requires a fully qualified path;
	// a relative lpFileName fails with ERROR_INVALID_PARAMETER.
	absPath, err := filepath.Abs(path)
	if err == nil {
		path = absPath
	}
	pathPtr, err := syscall.UTF16PtrFromString(path)
	if err != nil {
		return 0, err
	}
	h, _, errno := procLoadLibraryExW.Call(
		uintptr(unsafe.Pointer(pathPtr)), 0, loadLibrarySearchDirs)
	if h == 0 {
		// Older Windows without AddDllDirectory support: plain load.
		h2, lerr := syscall.LoadLibrary(path)
		if lerr != nil {
			if errno != syscall.Errno(0) {
				return 0, errno
			}
			return 0, lerr
		}
		return uintptr(h2), nil
	}
	return h, nil
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
