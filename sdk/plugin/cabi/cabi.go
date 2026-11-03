package cabi

/*
#include <stdlib.h>
*/
import "C"

import (
	"unsafe"

	pluginsdk "github.com/xiaokuang95/nulo/sdk/plugin"
)

// The functions below are the C ABI every nulo plugin library exports. They are
// compiled into the plugin when the main package imports this package and is
// built with -buildmode=c-shared. The host resolves them by name.

//export NuloPluginInit
func NuloPluginInit(info *C.char, infoLen C.int, hostCall C.uintptr_t) *C.char {
	text := C.GoStringN(info, infoLen)
	return C.CString(pluginsdk.Init(text, uintptr(hostCall)))
}

//export NuloPluginCall
func NuloPluginCall(request *C.char, requestLen C.int) *C.char {
	return C.CString(pluginsdk.Call(C.GoStringN(request, requestLen)))
}

//export NuloPluginFree
func NuloPluginFree(value *C.char) {
	C.free(unsafe.Pointer(value))
}
