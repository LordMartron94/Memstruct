package memstruct

import (
	"fmt"
	"memcore"
	"unsafe"
)

// String is a manually managed, immutable string representation stored in manual memory.
//
// Memory Layout:
// ┌───────────────────────────────┐
// │ String header (dataAddrOffset, length) │
// ├───────────────────────────────┤
// │ UTF-8 bytes (no null terminator)       │
// └───────────────────────────────┘
//
// All bytes live inside the same manual memory region.
// Safe for MemoryMoveNoHeapPointers, snapshots, and relocations.
type String struct {
	dataAddrOffset uintptr
	length         uint64
}

// StringRequiredBytesGet returns total required bytes for storing a given Go string.
//
//go:inline
func StringRequiredBytesGet(value string) uint64 {
	headerSize := memcore.SizeOf[String]()
	return headerSize + uint64(len(value))
}

//go:inline
func StringRequiredBytesGetLen(length uint64) uint64 {
	headerSize := memcore.SizeOf[String]()
	return headerSize + length
}

// StringRequiredAlignmentGet returns required alignment for the String header.
//
//go:inline
func StringRequiredAlignmentGet() uint64 {
	return memcore.AlignOf[String]()
}

// StringInitializeAt initializes a manually managed string at a memory location.
// Copies bytes from the Go string into manual memory.
//
// ⚠️ The capacity of the region must be >= StringRequiredBytesGet(len(value)).
//
//go:nosplit
//go:inline
func StringInitializeAt(strAddr memcore.MarkRaw, value string) {
	headerSize := memcore.SizeOf[String]()
	strPtr := memcore.MemcoreMarkDereferenceObject[String](strAddr)

	dataStart := unsafe.Add(memcore.MemcoreMarkDereference(strAddr), headerSize)
	dataLen := len(value)

	if dataLen == 0 {
		*strPtr = String{dataAddrOffset: uintptr(headerSize), length: 0}
		return
	}

	// Copy UTF-8 bytes directly into manual memory
	src := unsafe.StringData(value)
	memcore.MemoryMoveNoHeapPointers(dataStart, unsafe.Pointer(src), uintptr(dataLen))

	*strPtr = String{
		dataAddrOffset: uintptr(headerSize),
		length:         uint64(dataLen),
	}
}

// StringSnapshotCreate performs a deep copy from one manually managed String to another.
// Both source and destination must have sufficient allocated space.
//
//go:inline
func StringSnapshotCreate(dest, src memcore.MarkRaw) memcore.MarkRaw {
	srcPtr := memcore.MemcoreMarkDereferenceObject[String](src)
	totalBytes := StringRequiredBytesGetLen(srcPtr.length)

	memcore.MemoryMoveNoHeapPointers(
		memcore.MemcoreMarkDereference(dest),
		memcore.MemcoreMarkDereference(src),
		uintptr(totalBytes),
	)
	return dest
}

// StringSnapshotRestore replaces one String (header + data) with another.
// Both must have identical length or sufficient allocated bytes.
//
//go:inline
func StringSnapshotRestore(dest, src memcore.MarkRaw) error {
	dstPtr := memcore.MemcoreMarkDereferenceObject[String](dest)
	srcPtr := memcore.MemcoreMarkDereferenceObject[String](src)

	if dstPtr.length != srcPtr.length {
		return fmt.Errorf("cannot restore string snapshot: length mismatch (dst=%d, src=%d)", dstPtr.length, srcPtr.length)
	}

	totalBytes := StringRequiredBytesGetLen(dstPtr.length)
	memcore.MemoryMoveNoHeapPointers(
		memcore.MemcoreMarkDereference(dest),
		memcore.MemcoreMarkDereference(src),
		uintptr(totalBytes),
	)
	return nil
}

// StringHeaderClone clones only the header (no data movement).
//
//go:inline
func StringHeaderClone(dest, src memcore.MarkRaw) {
	dstAddr := memcore.MemcoreMarkDereferenceObject[String](dest)
	srcAddr := memcore.MemcoreMarkDereferenceObject[String](src)
	*dstAddr = *srcAddr
}

// StringLengthGet returns the string's length in bytes.
//
//go:inline
func StringLengthGet(str memcore.MarkRaw) uint64 {
	return memcore.MemcoreMarkDereferenceObject[String](str).length
}

// StringDataPtrGet returns pointer to the UTF-8 data section.
//
// ⚠️ Not stable across memory moves unless you re-fetch after region relocation.
//
//go:inline
func StringDataPtrGet(str memcore.MarkRaw) unsafe.Pointer {
	base := memcore.MemcoreMarkDereference(str)
	inst := memcore.MemcoreMarkDereferenceObject[String](str)
	return unsafe.Add(base, inst.dataAddrOffset)
}

// StringToGo converts a manually managed string to a Go string (zero-copy).
//
// ⚠️ Unsafe if the underlying manual memory is modified or freed afterward.
//
//go:inline
func StringToGo(str memcore.MarkRaw) string {
	inst := memcore.MemcoreMarkDereferenceObject[String](str)
	if inst.length == 0 {
		return ""
	}

	data := StringDataPtrGet(str)
	header := struct {
		Data unsafe.Pointer
		Len  int
	}{data, int(inst.length)}

	return *(*string)(unsafe.Pointer(&header))
}

// StringEquals compares two manually managed strings for equality.
//
//go:nosplit
//go:inline
func StringEquals(a, b memcore.MarkRaw) bool {
	aPtr := memcore.MemcoreMarkDereferenceObject[String](a)
	bPtr := memcore.MemcoreMarkDereferenceObject[String](b)

	if aPtr.length != bPtr.length {
		return false
	}

	aData := StringDataPtrGet(a)
	bData := StringDataPtrGet(b)

	return memcore.MemoryCompareNoHeapPointers(aData, bData, uintptr(aPtr.length))
}

// StringClear zeroes out the string data but preserves the header.
//
//go:nosplit
//go:inline
func StringClear(str memcore.MarkRaw) {
	inst := memcore.MemcoreMarkDereferenceObject[String](str)
	if inst.length == 0 {
		return
	}
	base := memcore.MemcoreMarkDereference(str)
	memcore.MemoryClearNoHeapPointers(
		unsafe.Add(base, inst.dataAddrOffset),
		uintptr(inst.length),
	)
}

// StringIsEmpty checks if string has zero length.
//
//go:inline
func StringIsEmpty(str memcore.MarkRaw) bool {
	return memcore.MemcoreMarkDereferenceObject[String](str).length == 0
}
