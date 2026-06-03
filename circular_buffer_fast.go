package memstruct

import (
	"fmt"
	"unsafe"
)

// circularBufferPhysicalIdx maps a logical index to a physical array index.
//
//go:inline
func circularBufferPhysicalIdx[T any](buffer *CircularBuffer[T], logicalIdx uint64) uint64 {
	return (buffer.startIdx + logicalIdx) % buffer.capacity
}

// CircularBufferGetAtUnsafeFast returns an element at logical index without bounds checks.
//
//go:nosplit
func CircularBufferGetAtUnsafeFast[T any](
	buffer *CircularBuffer[T],
	dataInst *Array[T],
	dataBase unsafe.Pointer,
	logicalIdx uint64,
) T {
	physicalIdx := circularBufferPhysicalIdx(buffer, logicalIdx)
	return ArrayItemGetAtUnsafeFast(dataInst, dataBase, physicalIdx)
}

// CircularBufferGetAtFast returns an element at logical index with bounds checking.
func CircularBufferGetAtFast[T any](
	buffer *CircularBuffer[T],
	dataInst *Array[T],
	dataBase unsafe.Pointer,
	logicalIdx uint64,
) T {
	if logicalIdx >= buffer.length {
		panic(fmt.Errorf("circular buffer index out of bounds: %d >= %d", logicalIdx, buffer.length))
	}
	return CircularBufferGetAtUnsafeFast(buffer, dataInst, dataBase, logicalIdx)
}

// CircularBufferLengthFast returns current window length.
//
//go:inline
func CircularBufferLengthFast[T any](buffer *CircularBuffer[T]) uint64 {
	return buffer.length
}

// CircularBufferPushFast adds an item using pre-dereferenced buffer and backing array.
func CircularBufferPushFast[T any](
	buffer *CircularBuffer[T],
	dataInst *Array[T],
	dataBase unsafe.Pointer,
	item T,
) {
	if buffer.length >= buffer.capacity {
		ArraySetAtUnsafeFast(dataInst, dataBase, buffer.startIdx, item)
		buffer.startIdx = (buffer.startIdx + 1) % buffer.capacity
	} else {
		insertIdx := (buffer.startIdx + buffer.length) % buffer.capacity
		ArraySetAtUnsafeFast(dataInst, dataBase, insertIdx, item)
		buffer.length++
	}
	buffer.version++
}
