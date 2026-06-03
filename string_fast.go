package memstruct

import (
	"unsafe"
)

// StringLengthGetFast returns byte length from a pre-dereferenced string header.
//
//go:inline
func StringLengthGetFast(str *String) uint64 {
	return str.length
}

// StringDataPtrGetFast returns the UTF-8 data pointer from a pre-dereferenced header.
//
//go:inline
func StringDataPtrGetFast(str *String) unsafe.Pointer {
	return stringDataPtrGetDirect(str)
}

// StringToGoFast converts to a Go string using a pre-dereferenced header.
func StringToGoFast(str *String) string {
	if str.length == 0 {
		return ""
	}
	data := stringDataPtrGetDirect(str)
	return unsafe.String((*byte)(data), str.length)
}

// StringIsEmptyFast checks empty length on a pre-dereferenced header.
//
//go:inline
func StringIsEmptyFast(str *String) bool {
	return str.length == 0
}
