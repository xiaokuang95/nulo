package plugin

import "unsafe"

func ptrToUnsafe(ptr uintptr) unsafe.Pointer {
	return *(*unsafe.Pointer)(unsafe.Pointer(&ptr))
}

func bytesAt(ptr uintptr, n int) []byte {
	if ptr == 0 || n <= 0 {
		return nil
	}
	return unsafe.Slice((*byte)(ptrToUnsafe(ptr)), n)
}

func stringData(s string) (uintptr, int) {
	if len(s) == 0 {
		return 0, 0
	}
	return uintptr(unsafe.Pointer(unsafe.StringData(s))), len(s)
}
