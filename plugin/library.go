package plugin

var openNativeLibrary = openLibrary

// nativeLibrary abstracts the platform dynamic loader.
type nativeLibrary interface {
	call(symbol string, args ...uintptr) (uintptr, error)
	close() error
}

func readCString(ptr uintptr, max int) string {
	if ptr == 0 {
		return ""
	}
	buf := make([]byte, 0, 256)
	for i := 0; i < max; i++ {
		b := *(*byte)(ptrToUnsafe(ptr + uintptr(i)))
		if b == 0 {
			break
		}
		buf = append(buf, b)
	}
	return string(buf)
}
