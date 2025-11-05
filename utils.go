package memstruct

import (
	"memcore"
	"unsafe"
)

const memmoveThreshold uint64 = 128

type setFn[T any] func(itemPtr unsafe.Pointer, value T)

//go:inline
//go:nosplit
func setByMove[T any](itemPtr unsafe.Pointer, value T) {
	srcPtr := unsafe.Pointer(&value)
	memcore.MemoryMoveNoHeapPointers(itemPtr, srcPtr, uintptr(memcore.SizeOf[T]()))
}

//go:inline
//go:nosplit
func setByAssign[T any](itemPtr unsafe.Pointer, value T) {
	*(*T)(itemPtr) = value
}

//go:inline
func getMovementFunc[T any](itemSizeBytes uint64) setFn[T] {
	if itemSizeBytes > memmoveThreshold {
		return setByMove[T]
	}

	return setByAssign[T]
}
