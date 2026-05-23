package memstruct

import (
	"memcore"
	"unsafe"
)

/*
GoStringRequiredBytesGet returns the total bytes required to store a native Go string in manual memory.

The layout is a Go string header followed immediately by the UTF-8 payload (no null terminator).
The header's data pointer is initialized to the payload offset within the same allocation.

Time complexity: O(1)
Space complexity: O(1)
*/
//
//go:inline
func GoStringRequiredBytesGet(value string) uint64 {
	return memcore.SizeOf[string]() + uint64(len(value))
}

/*
GoStringRequiredBytesGetLen returns required bytes for a native Go string with the given payload length.

Time complexity: O(1)
Space complexity: O(1)
*/
//
//go:inline
func GoStringRequiredBytesGetLen(length uint64) uint64 {
	return memcore.SizeOf[string]() + length
}

/*
GoStringRequiredAlignmentGet returns the alignment required for a native Go string allocation.

Time complexity: O(1)
Space complexity: O(1)
*/
//
//go:inline
func GoStringRequiredAlignmentGet() uint64 {
	return memcore.AlignOf[string]()
}

/*
GoStringInitializeAt writes a native Go string at strAddr using content copied from value.

The resulting string header and its data pointer both refer only to manual memory inside the
allocation at strAddr. The region must provide at least GoStringRequiredBytesGet(len(value)) bytes.

Unlike memstruct.String, this stores the runtime string type directly. If the containing memory
region is relocated, the string's data pointer must be updated to the new payload address.

Time complexity: O(n) where n is len(value)
Space complexity: O(1)
*/
//
//go:nosplit
//go:inline
func GoStringInitializeAt(strAddr memcore.MarkRaw, value string) {
	base := memcore.MemcoreMarkDereference(strAddr)
	out := (*string)(base)

	dataLen := len(value)
	if dataLen == 0 {
		*out = ""
		return
	}

	headerSize := memcore.SizeOf[string]()
	dataStart := unsafe.Add(base, headerSize)
	src := unsafe.StringData(value)
	memcore.MemoryMoveNoHeapPointers(dataStart, unsafe.Pointer(src), uintptr(dataLen))

	hdr := struct {
		Data uintptr
		Len  int
	}{uintptr(dataStart), dataLen}
	*out = *(*string)(unsafe.Pointer(&hdr))
}

/*
GoStringValueGet returns the native Go string stored at strAddr.

The returned string references manual memory and is only valid while that memory remains
allocated and unmodified.

Time complexity: O(1)
Space complexity: O(1)
*/
//
//go:inline
func GoStringValueGet(strAddr memcore.MarkRaw) string {
	return *memcore.MemcoreMarkDereferenceObject[string](strAddr)
}

/*
GoStringLengthGet returns the byte length of the native Go string at strAddr.

Time complexity: O(1)
Space complexity: O(1)
*/
//
//go:inline
func GoStringLengthGet(strAddr memcore.MarkRaw) uint64 {
	return uint64(len(*memcore.MemcoreMarkDereferenceObject[string](strAddr)))
}
