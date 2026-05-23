package memstruct

import (
	"memcore"
	"unsafe"
)

/*
CStringRequiredBytesGet returns bytes required for a NUL-terminated UTF-8 C string in manual memory.

Layout is payload bytes followed by a single 0x00 byte. Suitable for C-ABI and FFI (*char).

Time complexity: O(1)
Space complexity: O(1)
*/
//
//go:inline
func CStringRequiredBytesGet(value string) uint64 {
	return uint64(len(value)) + 1
}

/*
CStringRequiredBytesGetLen returns required bytes for a C string with the given payload length (excludes NUL from length).

Time complexity: O(1)
Space complexity: O(1)
*/
//
//go:inline
func CStringRequiredBytesGetLen(length uint64) uint64 {
	return length + 1
}

/*
CStringRequiredAlignmentGet returns alignment for a NUL-terminated C string allocation.

Time complexity: O(1)
Space complexity: O(1)
*/
//
//go:inline
func CStringRequiredAlignmentGet() uint64 {
	return 1
}

/*
CStringInitializeAt copies value into manual memory at strAddr and appends a NUL terminator.

The entire allocation contains only bytes; no Go string header is stored. The returned
CStringPointerGet address is safe to pass across C-ABI boundaries while the region remains valid.

Pointers stored here must not target Go heap-managed memory (see GoString package notes in go_string.go).

Time complexity: O(n) where n is len(value)
Space complexity: O(1)
*/
//
//go:nosplit
//go:inline
func CStringInitializeAt(strAddr memcore.MarkRaw, value string) {
	base := memcore.MemcoreMarkDereference(strAddr)
	dataLen := len(value)

	if dataLen > 0 {
		src := unsafe.StringData(value)
		memcore.MemoryMoveNoHeapPointers(base, unsafe.Pointer(src), uintptr(dataLen))
	}

	*(*byte)(unsafe.Add(base, dataLen)) = 0
}

/*
CStringPointerGet returns *byte to the first byte of the NUL-terminated C string at strAddr.

The pointer targets manual memory only. It is valid for FFI while strAddr remains allocated.

Time complexity: O(1)
Space complexity: O(1)
*/
//
//go:inline
func CStringPointerGet(strAddr memcore.MarkRaw) *byte {
	return (*byte)(memcore.MemcoreMarkDereference(strAddr))
}

/*
CStringToGo reads the NUL-terminated C string at strAddr into a Go string (zero-copy view).

The Go string references manual memory and is only valid while strAddr remains allocated.

Time complexity: O(n) where n is strlen
Space complexity: O(1)
*/
//
//go:inline
func CStringToGo(strAddr memcore.MarkRaw) string {
	ptr := CStringPointerGet(strAddr)
	return cStringBytesToGo(ptr)
}

/*
CStringLengthGet returns the byte length of the C string payload (excluding the NUL terminator).

Time complexity: O(n) where n is strlen
Space complexity: O(1)
*/
//
//go:inline
func CStringLengthGet(strAddr memcore.MarkRaw) uint64 {
	ptr := CStringPointerGet(strAddr)
	return cStringPayloadLengthGet(ptr)
}

//go:nosplit
//go:inline
func cStringBytesToGo(ptr *byte) string {
	length := cStringPayloadLengthGet(ptr)
	if length == 0 {
		return ""
	}
	return unsafe.String(ptr, length)
}

//go:nosplit
//go:inline
func cStringPayloadLengthGet(ptr *byte) uint64 {
	if ptr == nil {
		return 0
	}
	start := unsafe.Pointer(ptr)
	var length uint64
	for {
		if *(*byte)(unsafe.Add(start, length)) == 0 {
			break
		}
		length++
	}
	return length
}
