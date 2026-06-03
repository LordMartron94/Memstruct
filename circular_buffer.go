package memstruct

import (
	"fmt"
	"memcore"
	"unsafe"
)

// CircularBuffer is a fixed-capacity circular buffer that maintains a sliding window.
// When full, adding a new element automatically removes the oldest element.
// Optimized for statistics operations that need to iterate over all elements.
type CircularBuffer[T any] struct {
	data     *Array[T]
	dataBase unsafe.Pointer
	startIdx uint64
	length   uint64
	capacity uint64
	version  uint64
}

// CircularBufferRequiredBytesGet returns total bytes required for a circular buffer of given capacity.
func CircularBufferRequiredBytesGet[T any](capacity uint64) uint64 {
	bufferHeaderSize := memcore.SizeOf[CircularBuffer[T]]()
	bufferHeaderAlignment := memcore.AlignOf[CircularBuffer[T]]()
	bufferHeaderTotalSize := memcore.AlignUp(bufferHeaderSize, bufferHeaderAlignment)

	return bufferHeaderTotalSize + ArrayRequiredBytesGet[T](capacity)
}

// CircularBufferRequiredAlignmentGet returns alignment requirement for the circular buffer type.
func CircularBufferRequiredAlignmentGet[T any]() uint64 {
	return max(memcore.AlignOf[CircularBuffer[T]](), ArrayRequiredAlignmentGet[T]())
}

// CircularBufferInitializeAt initializes an instance of a circular buffer for type T at a specific memory address.
// Ensure the address is properly aligned and has the right size.
//
// ⚠️ capacity is in elements, not bytes.
func CircularBufferInitializeAt[T any](bufferAddr memcore.MarkRaw, capacity uint64) {
	bufferHeaderSize := memcore.SizeOf[CircularBuffer[T]]()
	bufferHeaderAlignment := memcore.AlignOf[CircularBuffer[T]]()

	// Create pointer for nested array header
	arrayPtr, _ := memcore.MemcoreMarkAlignedOffsetFrom(
		bufferAddr,
		uintptr(bufferHeaderSize),
		bufferHeaderAlignment,
	)

	// Initialize array header + data region
	ArrayInitializeAt[T](arrayPtr, capacity)

	// Initialize circular buffer header itself
	bufferPtr := memcore.MemcoreMarkDereferenceObject[CircularBuffer[T]](bufferAddr)
	*bufferPtr = CircularBuffer[T]{
		startIdx: 0,
		length:   0,
		capacity: capacity,
		version:  1,
	}
	circularBufferStorageWireAt(bufferAddr, bufferPtr)
}

// CircularBufferSnapshotCreate creates a deep snapshot of a circular buffer at a new location.
func CircularBufferSnapshotCreate[T any](dest memcore.MarkRaw, instance memcore.MarkRaw) memcore.MarkRaw {
	src := memcore.MemcoreMarkDereferenceObject[CircularBuffer[T]](instance)
	totalSize := CircularBufferRequiredBytesGet[T](src.capacity)

	srcAddr := memcore.MemcoreMarkDereference(instance)
	dstAddr := memcore.MemcoreMarkDereference(dest)
	memcore.MemoryMoveNoHeapPointers(dstAddr, srcAddr, uintptr(totalSize))

	dstBuffer := memcore.MemcoreMarkDereferenceObject[CircularBuffer[T]](dest)
	circularBufferStorageWireAt(dest, dstBuffer)
	return dest
}

// CircularBufferSnapshotRestore replaces the contents of one circular buffer with another.
func CircularBufferSnapshotRestore[T any](dest memcore.MarkRaw, src memcore.MarkRaw) {
	dstBuffer := memcore.MemcoreMarkDereferenceObject[CircularBuffer[T]](dest)
	srcBuffer := memcore.MemcoreMarkDereferenceObject[CircularBuffer[T]](src)

	if dstBuffer.capacity != srcBuffer.capacity {
		panic(fmt.Errorf("cannot restore circular buffer snapshot: unequal capacities (%v vs %v)", dstBuffer.capacity, srcBuffer.capacity))
	}

	if err := arraySnapshotRestoreFast(dstBuffer.data, srcBuffer.data, dstBuffer.dataBase, srcBuffer.dataBase); err != nil {
		panic(err)
	}
	dstBuffer.startIdx = srcBuffer.startIdx
	dstBuffer.length = srcBuffer.length
	dstBuffer.version = srcBuffer.version
}

// CircularBufferPush adds a new item to the circular buffer.
// If the buffer is full, the oldest element is automatically removed.
// Version is incremented on every modification.
func CircularBufferPush[T any](buffer memcore.MarkRaw, item T) {
	instance := memcore.MemcoreMarkDereferenceObject[CircularBuffer[T]](buffer)

	if instance.length >= instance.capacity {
		// Buffer is full, overwrite oldest element
		ArraySetAtUnsafeFast(instance.data, instance.dataBase, instance.startIdx, item)
		instance.startIdx = (instance.startIdx + 1) % instance.capacity
	} else {
		// Buffer has space, add to end
		insertIdx := (instance.startIdx + instance.length) % instance.capacity
		ArraySetAtUnsafeFast(instance.data, instance.dataBase, insertIdx, item)
		instance.length++
	}

	instance.version++
}

// CircularBufferIsFull checks if the circular buffer is at capacity.
//
//go:inline
//go:nosplit
func CircularBufferIsFull[T any](buffer memcore.MarkRaw) bool {
	instance := memcore.MemcoreMarkDereferenceObject[CircularBuffer[T]](buffer)
	return instance.length >= instance.capacity
}

// CircularBufferLength returns the current number of elements in the buffer.
//
//go:inline
//go:nosplit
func CircularBufferLength[T any](buffer memcore.MarkRaw) uint64 {
	instance := memcore.MemcoreMarkDereferenceObject[CircularBuffer[T]](buffer)
	return instance.length
}

// CircularBufferCapacity returns the maximum capacity of the buffer.
//
//go:inline
//go:nosplit
func CircularBufferCapacity[T any](buffer memcore.MarkRaw) uint64 {
	instance := memcore.MemcoreMarkDereferenceObject[CircularBuffer[T]](buffer)
	return instance.capacity
}

// CircularBufferGetAt returns the element at logical index (0 = oldest, length-1 = newest).
// Panics if index is out of bounds.
func CircularBufferGetAt[T any](buffer memcore.MarkRaw, logicalIdx uint64) T {
	instance := memcore.MemcoreMarkDereferenceObject[CircularBuffer[T]](buffer)

	if logicalIdx >= instance.length {
		panic(fmt.Errorf("circular buffer index out of bounds: %d >= %d", logicalIdx, instance.length))
	}

	physicalIdx := (instance.startIdx + logicalIdx) % instance.capacity
	return ArrayItemGetAtUnsafeFast(instance.data, instance.dataBase, physicalIdx)
}

// CircularBufferGetAtUnsafe returns the element at logical index without bounds checking.
//
//go:inline
//go:nosplit
func CircularBufferGetAtUnsafe[T any](buffer memcore.MarkRaw, logicalIdx uint64) T {
	instance := memcore.MemcoreMarkDereferenceObject[CircularBuffer[T]](buffer)
	physicalIdx := (instance.startIdx + logicalIdx) % instance.capacity
	return ArrayItemGetAtUnsafeFast(instance.data, instance.dataBase, physicalIdx)
}

// CircularBufferClear resets the buffer to empty state (does not zero memory).
//
//go:inline
//go:nosplit
func CircularBufferClear[T any](buffer memcore.MarkRaw) {
	instance := memcore.MemcoreMarkDereferenceObject[CircularBuffer[T]](buffer)
	instance.length = 0
	instance.startIdx = 0
	instance.version++
}

// CircularBufferClearAndZero resets the buffer and zeroes array memory.
//
//go:inline
//go:nosplit
func CircularBufferClearAndZero[T any](buffer memcore.MarkRaw) {
	instance := memcore.MemcoreMarkDereferenceObject[CircularBuffer[T]](buffer)
	ArrayClearFast(instance.data, instance.dataBase)
	instance.length = 0
	instance.startIdx = 0
	instance.version++
}

// CircularBufferIsEmpty checks if the buffer is empty.
//
//go:inline
//go:nosplit
func CircularBufferIsEmpty[T any](buffer memcore.MarkRaw) bool {
	instance := memcore.MemcoreMarkDereferenceObject[CircularBuffer[T]](buffer)
	return instance.length == 0
}

// CircularBufferVersionGet returns the current version of the circular buffer.
// Version increments on every modification to enable cache invalidation.
//
//go:inline
//go:nosplit
func CircularBufferVersionGet[T any](buffer memcore.MarkRaw) uint64 {
	instance := memcore.MemcoreMarkDereferenceObject[CircularBuffer[T]](buffer)
	return instance.version
}

// CircularBufferItemPtrGetAt returns a pointer to T at logical index within the circular buffer.
// It returns an error if the logical index is invalid.
//
// Using this pointer after the buffer is modified is undefined behaviour.
// Use at your own discretion!
//
//go:nosplit
//go:inline
func CircularBufferItemPtrGetAt[T any](buffer memcore.MarkRaw, logicalIdx uint64) (*T, error) {
	instance := memcore.MemcoreMarkDereferenceObject[CircularBuffer[T]](buffer)

	if logicalIdx >= instance.length {
		return nil, fmt.Errorf("circular buffer index out of bounds: %d >= %d", logicalIdx, instance.length)
	}

	physicalIdx := (instance.startIdx + logicalIdx) % instance.capacity
	return ArrayItemPtrGetAtUnsafeFast(instance.data, instance.dataBase, physicalIdx), nil
}

// CircularBufferItemPtrGetAtUnsafe returns a pointer to T at logical index without bounds checking.
//
// Using this pointer after the buffer is modified is undefined behaviour.
// Use at your own discretion!
//
//go:inline
//go:nosplit
func CircularBufferItemPtrGetAtUnsafe[T any](buffer memcore.MarkRaw, logicalIdx uint64) *T {
	instance := memcore.MemcoreMarkDereferenceObject[CircularBuffer[T]](buffer)
	physicalIdx := (instance.startIdx + logicalIdx) % instance.capacity
	return ArrayItemPtrGetAtUnsafeFast(instance.data, instance.dataBase, physicalIdx)
}

// CircularBufferDataPtrGet returns the current pointer to the underlying array data storage in memory.
// This value CAN change if the underlying memory region changes.
// Not stable, so do not store.
//
//go:inline
func CircularBufferDataPtrGet[T any](buffer memcore.MarkRaw) unsafe.Pointer {
	instance := memcore.MemcoreMarkDereferenceObject[CircularBuffer[T]](buffer)
	return ArrayDataPtrGetFast(instance.data, instance.dataBase)
}

// CircularBufferByteOffsetGetAt returns the offset relative to the memory region for this logical index.
// Panics if the logical index is invalid.
//
//go:inline
func CircularBufferByteOffsetGetAt[T any](buffer memcore.MarkRaw, logicalIdx uint64) uintptr {
	instance := memcore.MemcoreMarkDereferenceObject[CircularBuffer[T]](buffer)

	if logicalIdx >= instance.length {
		panic(fmt.Errorf("circular buffer index out of bounds: %d >= %d", logicalIdx, instance.length))
	}

	physicalIdx := (instance.startIdx + logicalIdx) % instance.capacity
	return ArrayByteOffsetGetAtUnsafeFast(instance.data, physicalIdx)
}

// CircularBufferByteOffsetGetAtUnsafe returns the offset relative to the memory region for this logical index.
// Does no bounds checks.
//
//go:inline
func CircularBufferByteOffsetGetAtUnsafe[T any](buffer memcore.MarkRaw, logicalIdx uint64) uintptr {
	instance := memcore.MemcoreMarkDereferenceObject[CircularBuffer[T]](buffer)
	physicalIdx := (instance.startIdx + logicalIdx) % instance.capacity
	return ArrayByteOffsetGetAtUnsafeFast(instance.data, physicalIdx)
}

// CircularBufferIsIdxValid checks whether the given logical index is valid.
//
//go:inline
func CircularBufferIsIdxValid[T any](buffer memcore.MarkRaw, logicalIdx uint64) bool {
	instance := memcore.MemcoreMarkDereferenceObject[CircularBuffer[T]](buffer)
	return logicalIdx < instance.length
}

// circularBufferLogicalToPhysical maps a logical index to a physical index in the underlying array.
//
//go:inline
func circularBufferLogicalToPhysical[T any](instance *CircularBuffer[T], logicalIdx uint64) uint64 {
	return (instance.startIdx + logicalIdx) % instance.capacity
}

// circularBufferGetArrayPtrAtLogicalIdx returns a pointer to the element at the given logical index.
//
//go:inline
func circularBufferGetArrayPtrAtLogicalIdx[T any](instance *CircularBuffer[T], logicalIdx uint64) unsafe.Pointer {
	physicalIdx := circularBufferLogicalToPhysical[T](instance, logicalIdx)
	return arrayGetPtrAtIdx(instance.data, instance.dataBase, physicalIdx)
}

// CircularBufferForEachUnsafe calls a function for every element in the circular buffer.
// The function receives a pointer to the element and its logical index.
//
//go:inline
func CircularBufferForEachUnsafe[T any](buffer memcore.MarkRaw, fn func(ptr unsafe.Pointer, logicalIdx uint64)) {
	instance := memcore.MemcoreMarkDereferenceObject[CircularBuffer[T]](buffer)

	if instance.length == 0 {
		return
	}

	for logicalIdx := uint64(0); logicalIdx < instance.length; logicalIdx++ {
		ptr := circularBufferGetArrayPtrAtLogicalIdx[T](instance, logicalIdx)
		fn(ptr, logicalIdx)
	}
}

// CircularBufferStrideForEachUnsafe calls a function for every stride-th element in the circular buffer.
// It visits every stride-th element.
//
// For the tail it calls the tailFn which is supposed to process one element at once.
//
//go:inline
func CircularBufferStrideForEachUnsafe[T any](
	buffer memcore.MarkRaw,
	strideFn func(ptr unsafe.Pointer, logicalIdx uint64),
	tailFn func(ptr unsafe.Pointer, logicalIdx uint64),
	stride uint64,
) {
	instance := memcore.MemcoreMarkDereferenceObject[CircularBuffer[T]](buffer)

	if instance.length == 0 {
		return
	}

	logicalIdx := uint64(0)
	for ; logicalIdx+stride < instance.length; logicalIdx += stride {
		ptr := circularBufferGetArrayPtrAtLogicalIdx[T](instance, logicalIdx)
		strideFn(ptr, logicalIdx)
	}

	for ; logicalIdx < instance.length; logicalIdx++ {
		ptr := circularBufferGetArrayPtrAtLogicalIdx[T](instance, logicalIdx)
		tailFn(ptr, logicalIdx)
	}
}

// CircularBufferIterate allows you to iterate over the circular buffer efficiently.
// It provides a method to say which next element you need.
// The next function returns true when your requested n exceeds the length.
//
//go:inline
func CircularBufferIterate[T any](
	buffer memcore.MarkRaw,
	fn func(
		ptr unsafe.Pointer,
		logicalIdx uint64,
		next func(n uint64) (unsafe.Pointer, uint64, bool),
	),
) {
	instance := memcore.MemcoreMarkDereferenceObject[CircularBuffer[T]](buffer)

	if instance.length == 0 {
		return
	}

	logicalIdx := uint64(0)
	nextFn := func(n uint64) (unsafe.Pointer, uint64, bool) {
		logicalIdx += n

		if logicalIdx >= instance.length {
			return nil, logicalIdx, true
		}

		ptr := circularBufferGetArrayPtrAtLogicalIdx[T](instance, logicalIdx)
		return ptr, logicalIdx, false
	}

	ptr := circularBufferGetArrayPtrAtLogicalIdx[T](instance, logicalIdx)
	fn(ptr, logicalIdx, nextFn)
}

// CircularBufferIterateUnsafe allows you to iterate over the circular buffer efficiently.
// It provides a method to say which next element you need.
// The next function does no bounds checking. Safety must be guaranteed by the client.
//
//go:inline
func CircularBufferIterateUnsafe[T any](
	buffer memcore.MarkRaw,
	fn func(
		ptr unsafe.Pointer,
		logicalIdx uint64,
		next func(n uint64) (unsafe.Pointer, uint64),
	),
) {
	instance := memcore.MemcoreMarkDereferenceObject[CircularBuffer[T]](buffer)

	if instance.length == 0 {
		return
	}

	logicalIdx := uint64(0)
	nextFn := func(n uint64) (unsafe.Pointer, uint64) {
		logicalIdx += n
		ptr := circularBufferGetArrayPtrAtLogicalIdx[T](instance, logicalIdx)
		return ptr, logicalIdx
	}

	ptr := circularBufferGetArrayPtrAtLogicalIdx[T](instance, logicalIdx)
	fn(ptr, logicalIdx, nextFn)
}

// circularBufferUnrolledDispatch dispatches to stride-specific unrolled functions.
// Handles circular wrapping by processing in segments if needed.
//
//go:inline
func circularBufferUnrolledDispatch[T any](
	instance *CircularBuffer[T],
	stride uint64,
	apply func(logicalIdx uint64),
) {
	length := instance.length
	if length == 0 {
		return
	}

	// Check if the logical window wraps around the physical array
	endPhysicalIdx := (instance.startIdx + length) % instance.capacity
	wraps := instance.startIdx+length > instance.capacity || (instance.startIdx+length == instance.capacity && endPhysicalIdx != 0)

	if !wraps {
		// No wrapping - contiguous memory, use standard unrolled dispatch
		switch stride {
		case 8:
			circularBufferUnrolledStride8Contiguous[T](instance, length, apply)
		case 4:
			circularBufferUnrolledStride4Contiguous[T](instance, length, apply)
		case 2:
			circularBufferUnrolledStride2Contiguous[T](instance, length, apply)
		case 1:
			circularBufferUnrolledStride1Contiguous[T](instance, length, apply)
		default:
			circularBufferUnrolledGenericContiguous[T](instance, length, stride, apply)
		}
	} else {
		// Wrapping - process in two segments
		segment1Len := instance.capacity - instance.startIdx
		segment2Len := length - segment1Len

		switch stride {
		case 8:
			circularBufferUnrolledStride8Wrapped[T](instance, segment1Len, segment2Len, apply)
		case 4:
			circularBufferUnrolledStride4Wrapped[T](instance, segment1Len, segment2Len, apply)
		case 2:
			circularBufferUnrolledStride2Wrapped[T](instance, segment1Len, segment2Len, apply)
		case 1:
			circularBufferUnrolledStride1Wrapped[T](instance, segment1Len, segment2Len, apply)
		default:
			circularBufferUnrolledGenericWrapped[T](instance, segment1Len, segment2Len, stride, apply)
		}
	}
}

// Contiguous unrolled functions (no wrapping)

//go:inline
func circularBufferUnrolledStride1Contiguous[T any](instance *CircularBuffer[T], length uint64, apply func(logicalIdx uint64)) {
	for logicalIdx := uint64(0); logicalIdx < length; logicalIdx++ {
		apply(logicalIdx)
	}
}

//go:inline
func circularBufferUnrolledStride2Contiguous[T any](instance *CircularBuffer[T], length uint64, apply func(logicalIdx uint64)) {
	var logicalIdx uint64
	for ; logicalIdx+1 < length; logicalIdx += 2 {
		apply(logicalIdx + 0)
		apply(logicalIdx + 1)
	}
	for ; logicalIdx < length; logicalIdx++ {
		apply(logicalIdx)
	}
}

//go:inline
func circularBufferUnrolledStride4Contiguous[T any](instance *CircularBuffer[T], length uint64, apply func(logicalIdx uint64)) {
	var logicalIdx uint64
	for ; logicalIdx+3 < length; logicalIdx += 4 {
		apply(logicalIdx + 0)
		apply(logicalIdx + 1)
		apply(logicalIdx + 2)
		apply(logicalIdx + 3)
	}
	for ; logicalIdx < length; logicalIdx++ {
		apply(logicalIdx)
	}
}

//go:inline
func circularBufferUnrolledStride8Contiguous[T any](instance *CircularBuffer[T], length uint64, apply func(logicalIdx uint64)) {
	var logicalIdx uint64
	for ; logicalIdx+7 < length; logicalIdx += 8 {
		apply(logicalIdx + 0)
		apply(logicalIdx + 1)
		apply(logicalIdx + 2)
		apply(logicalIdx + 3)
		apply(logicalIdx + 4)
		apply(logicalIdx + 5)
		apply(logicalIdx + 6)
		apply(logicalIdx + 7)
	}
	for ; logicalIdx < length; logicalIdx++ {
		apply(logicalIdx)
	}
}

//go:inline
func circularBufferUnrolledGenericContiguous[T any](instance *CircularBuffer[T], length uint64, stride uint64, apply func(logicalIdx uint64)) {
	var logicalIdx uint64
	for ; logicalIdx+stride <= length; logicalIdx += stride {
		for j := uint64(0); j < stride; j++ {
			apply(logicalIdx + j)
		}
	}
	for ; logicalIdx < length; logicalIdx++ {
		apply(logicalIdx)
	}
}

// Wrapped unrolled functions (two segments)

//go:inline
func circularBufferUnrolledStride1Wrapped[T any](instance *CircularBuffer[T], segment1Len, segment2Len uint64, apply func(logicalIdx uint64)) {
	// Segment 1: from startIdx to end of array
	for logicalIdx := uint64(0); logicalIdx < segment1Len; logicalIdx++ {
		apply(logicalIdx)
	}
	// Segment 2: from 0 to segment2Len
	for logicalIdx := segment1Len; logicalIdx < segment1Len+segment2Len; logicalIdx++ {
		apply(logicalIdx)
	}
}

//go:inline
func circularBufferUnrolledStride2Wrapped[T any](instance *CircularBuffer[T], segment1Len, segment2Len uint64, apply func(logicalIdx uint64)) {
	// Segment 1
	var logicalIdx uint64
	for ; logicalIdx+1 < segment1Len; logicalIdx += 2 {
		apply(logicalIdx + 0)
		apply(logicalIdx + 1)
	}
	for ; logicalIdx < segment1Len; logicalIdx++ {
		apply(logicalIdx)
	}
	// Segment 2
	startIdx := segment1Len
	for ; startIdx+1 < segment1Len+segment2Len; startIdx += 2 {
		apply(startIdx + 0)
		apply(startIdx + 1)
	}
	for ; startIdx < segment1Len+segment2Len; startIdx++ {
		apply(startIdx)
	}
}

//go:inline
func circularBufferUnrolledStride4Wrapped[T any](instance *CircularBuffer[T], segment1Len, segment2Len uint64, apply func(logicalIdx uint64)) {
	// Segment 1
	var logicalIdx uint64
	for ; logicalIdx+3 < segment1Len; logicalIdx += 4 {
		apply(logicalIdx + 0)
		apply(logicalIdx + 1)
		apply(logicalIdx + 2)
		apply(logicalIdx + 3)
	}
	for ; logicalIdx < segment1Len; logicalIdx++ {
		apply(logicalIdx)
	}
	// Segment 2
	startIdx := segment1Len
	for ; startIdx+3 < segment1Len+segment2Len; startIdx += 4 {
		apply(startIdx + 0)
		apply(startIdx + 1)
		apply(startIdx + 2)
		apply(startIdx + 3)
	}
	for ; startIdx < segment1Len+segment2Len; startIdx++ {
		apply(startIdx)
	}
}

//go:inline
func circularBufferUnrolledStride8Wrapped[T any](instance *CircularBuffer[T], segment1Len, segment2Len uint64, apply func(logicalIdx uint64)) {
	// Segment 1
	var logicalIdx uint64
	for ; logicalIdx+7 < segment1Len; logicalIdx += 8 {
		apply(logicalIdx + 0)
		apply(logicalIdx + 1)
		apply(logicalIdx + 2)
		apply(logicalIdx + 3)
		apply(logicalIdx + 4)
		apply(logicalIdx + 5)
		apply(logicalIdx + 6)
		apply(logicalIdx + 7)
	}
	for ; logicalIdx < segment1Len; logicalIdx++ {
		apply(logicalIdx)
	}
	// Segment 2
	startIdx := segment1Len
	for ; startIdx+7 < segment1Len+segment2Len; startIdx += 8 {
		apply(startIdx + 0)
		apply(startIdx + 1)
		apply(startIdx + 2)
		apply(startIdx + 3)
		apply(startIdx + 4)
		apply(startIdx + 5)
		apply(startIdx + 6)
		apply(startIdx + 7)
	}
	for ; startIdx < segment1Len+segment2Len; startIdx++ {
		apply(startIdx)
	}
}

//go:inline
func circularBufferUnrolledGenericWrapped[T any](instance *CircularBuffer[T], segment1Len, segment2Len uint64, stride uint64, apply func(logicalIdx uint64)) {
	// Segment 1
	var logicalIdx uint64
	for ; logicalIdx+stride <= segment1Len; logicalIdx += stride {
		for j := uint64(0); j < stride; j++ {
			apply(logicalIdx + j)
		}
	}
	for ; logicalIdx < segment1Len; logicalIdx++ {
		apply(logicalIdx)
	}
	// Segment 2
	startIdx := segment1Len
	for ; startIdx+stride <= segment1Len+segment2Len; startIdx += stride {
		for j := uint64(0); j < stride; j++ {
			apply(startIdx + j)
		}
	}
	for ; startIdx < segment1Len+segment2Len; startIdx++ {
		apply(startIdx)
	}
}

// CircularBufferUnaryReadOnlyOp is an operation that executes over a single element and does not mutate.
type CircularBufferUnaryReadOnlyOp[T any] func(item T)

// CircularBufferUnaryOp is an operation that executes over a single element and mutates.
type CircularBufferUnaryOp[TInput, TOutput any] func(item TInput) TOutput

// CircularBufferBinaryReadOnlyOp is an operation that executes over two elements at the same logical index from different buffers.
// It does not mutate.
type CircularBufferBinaryReadOnlyOp[TInput1, TInput2 any] func(itemA TInput1, itemB TInput2)

// CircularBufferBinaryOp is an operation that executes over two elements at the same logical index from different buffers.
// It mutates.
type CircularBufferBinaryOp[TInput1, TInput2, TOutput any] func(itemA TInput1, itemB TInput2) TOutput

// CircularBufferUnaryExecute executes a stride of unary mutating operations,
// writing results from the source buffer into the destination buffer.
// This matches Vector's pattern where the non-readonly executor mutates a destination.
//
//go:inline
func CircularBufferUnaryExecute[T, P any](
	srcBuffer, dstBuffer memcore.MarkRaw,
	op CircularBufferUnaryOp[T, P],
	stride uint64,
) {
	srcInstance := memcore.MemcoreMarkDereferenceObject[CircularBuffer[T]](srcBuffer)
	dstInstance := memcore.MemcoreMarkDereferenceObject[CircularBuffer[P]](dstBuffer)

	if srcInstance.length != dstInstance.length {
		panic(fmt.Errorf("cannot perform unary op: length mismatch (src=%d, dst=%d)", srcInstance.length, dstInstance.length))
	}

	if srcInstance.length == 0 {
		return
	}

	srcArrayData := arrayComputeDataAddr(srcInstance.data, srcInstance.dataBase)
	srcItemSize := uintptr(srcInstance.data.itemSize)

	dstArrayData := arrayComputeDataAddr(dstInstance.data, dstInstance.dataBase)
	dstItemSize := uintptr(dstInstance.data.itemSize)

	circularBufferUnrolledDispatch[T](srcInstance, stride, func(logicalIdx uint64) {
		srcPhysicalIdx := circularBufferLogicalToPhysical[T](srcInstance, logicalIdx)
		dstPhysicalIdx := circularBufferLogicalToPhysical[P](dstInstance, logicalIdx)
		srcVal := *(*T)(unsafe.Add(srcArrayData, uintptr(srcPhysicalIdx)*srcItemSize))
		res := op(srcVal)
		*(*P)(unsafe.Add(dstArrayData, uintptr(dstPhysicalIdx)*dstItemSize)) = res
	})

	dstInstance.version++
}

// CircularBufferUnaryReadOnlyExecute executes a stride of unary readonly operations with stride unrolling optimization.
//
//go:inline
func CircularBufferUnaryReadOnlyExecute[T any](
	buffer memcore.MarkRaw,
	op CircularBufferUnaryReadOnlyOp[T],
	stride uint64,
) {
	instance := memcore.MemcoreMarkDereferenceObject[CircularBuffer[T]](buffer)

	if instance.length == 0 {
		return
	}

	arrayData := arrayComputeDataAddr(instance.data, instance.dataBase)
	itemSize := uintptr(instance.data.itemSize)

	circularBufferUnrolledDispatch[T](instance, stride, func(logicalIdx uint64) {
		physicalIdx := circularBufferLogicalToPhysical[T](instance, logicalIdx)
		val := *(*T)(unsafe.Add(arrayData, uintptr(physicalIdx)*itemSize))
		op(val)
	})
}

// CircularBufferBinaryReadOnlyExecute executes a stride of binary read-only operations
// between two circular buffers of equal length.
//
//go:inline
func CircularBufferBinaryReadOnlyExecute[T, U any](
	bufferA, bufferB memcore.MarkRaw,
	op CircularBufferBinaryReadOnlyOp[T, U],
	stride uint64,
) {
	instanceA := memcore.MemcoreMarkDereferenceObject[CircularBuffer[T]](bufferA)
	instanceB := memcore.MemcoreMarkDereferenceObject[CircularBuffer[U]](bufferB)

	if instanceA.length != instanceB.length {
		panic(fmt.Errorf("cannot perform binary op: length mismatch (a=%d, b=%d)", instanceA.length, instanceB.length))
	}

	if instanceA.length == 0 {
		return
	}

	arrayAData := arrayComputeDataAddr(instanceA.data, instanceA.dataBase)
	aItemSize := uintptr(instanceA.data.itemSize)

	arrayBData := arrayComputeDataAddr(instanceB.data, instanceB.dataBase)
	bItemSize := uintptr(instanceB.data.itemSize)

	circularBufferUnrolledDispatch[T](instanceA, stride, func(logicalIdx uint64) {
		physicalIdxA := circularBufferLogicalToPhysical[T](instanceA, logicalIdx)
		physicalIdxB := circularBufferLogicalToPhysical[U](instanceB, logicalIdx)
		aVal := *(*T)(unsafe.Add(arrayAData, uintptr(physicalIdxA)*aItemSize))
		bVal := *(*U)(unsafe.Add(arrayBData, uintptr(physicalIdxB)*bItemSize))
		op(aVal, bVal)
	})
}

// CircularBufferBinaryExecute executes a stride of binary mutating operations
// between two source buffers and writes results into a destination buffer.
//
//go:inline
func CircularBufferBinaryExecute[T, U, P any](
	bufferA, bufferB, destBuffer memcore.MarkRaw,
	op CircularBufferBinaryOp[T, U, P],
	stride uint64,
) {
	instanceA := memcore.MemcoreMarkDereferenceObject[CircularBuffer[T]](bufferA)
	instanceB := memcore.MemcoreMarkDereferenceObject[CircularBuffer[U]](bufferB)
	destInstance := memcore.MemcoreMarkDereferenceObject[CircularBuffer[P]](destBuffer)

	if instanceA.length != instanceB.length || instanceA.length != destInstance.length {
		panic(fmt.Errorf("cannot perform binary op: length mismatch (a=%d, b=%d, d=%d)", instanceA.length, instanceB.length, destInstance.length))
	}

	if instanceA.length == 0 {
		return
	}

	arrayAData := arrayComputeDataAddr(instanceA.data, instanceA.dataBase)
	aItemSize := uintptr(instanceA.data.itemSize)

	arrayBData := arrayComputeDataAddr(instanceB.data, instanceB.dataBase)
	bItemSize := uintptr(instanceB.data.itemSize)

	destArrayData := arrayComputeDataAddr(destInstance.data, destInstance.dataBase)
	destItemSize := uintptr(destInstance.data.itemSize)

	circularBufferUnrolledDispatch[T](instanceA, stride, func(logicalIdx uint64) {
		physicalIdxA := circularBufferLogicalToPhysical[T](instanceA, logicalIdx)
		physicalIdxB := circularBufferLogicalToPhysical[U](instanceB, logicalIdx)
		destPhysicalIdx := circularBufferLogicalToPhysical[P](destInstance, logicalIdx)
		aVal := *(*T)(unsafe.Add(arrayAData, uintptr(physicalIdxA)*aItemSize))
		bVal := *(*U)(unsafe.Add(arrayBData, uintptr(physicalIdxB)*bItemSize))
		*(*P)(unsafe.Add(destArrayData, uintptr(destPhysicalIdx)*destItemSize)) = op(aVal, bVal)
	})

	destInstance.version++
}

// CircularBufferSetAll sets all values within the logical window to value T.
//
//go:inline
func CircularBufferSetAll[T any](buffer memcore.MarkRaw, v T) {
	instance := memcore.MemcoreMarkDereferenceObject[CircularBuffer[T]](buffer)

	if instance.length == 0 {
		return
	}

	CircularBufferForEachUnsafe[T](buffer, func(ptr unsafe.Pointer, logicalIdx uint64) {
		*(*T)(ptr) = v
	})

	instance.version++
}

// CircularBufferZeroAll sets all values within the logical window to its zero value.
// This is different from clearing the memory to 0.
//
//go:inline
func CircularBufferZeroAll[T any](buffer memcore.MarkRaw) {
	var zero T
	CircularBufferSetAll[T](buffer, zero)
}

// CircularBufferCopyFrom copies the entire logical window from src buffer into dest buffer.
// Both buffers must have the same element type T and equal lengths.
// Returns an error if lengths don't match.
func CircularBufferCopyFrom[T any](destBuffer, srcBuffer memcore.MarkRaw) error {
	destInstance := memcore.MemcoreMarkDereferenceObject[CircularBuffer[T]](destBuffer)
	srcInstance := memcore.MemcoreMarkDereferenceObject[CircularBuffer[T]](srcBuffer)

	if destInstance.length != srcInstance.length {
		return fmt.Errorf("CircularBufferCopyFrom: length mismatch (dest=%d, src=%d)", destInstance.length, srcInstance.length)
	}

	if destInstance.length == 0 {
		return nil
	}

	// Copy element by element in logical order
	for logicalIdx := uint64(0); logicalIdx < destInstance.length; logicalIdx++ {
		srcPhysicalIdx := circularBufferLogicalToPhysical[T](srcInstance, logicalIdx)
		destPhysicalIdx := circularBufferLogicalToPhysical[T](destInstance, logicalIdx)
		item := ArrayItemGetAtUnsafeFast(srcInstance.data, srcInstance.dataBase, srcPhysicalIdx)
		ArraySetAtUnsafeFast(destInstance.data, destInstance.dataBase, destPhysicalIdx, item)
	}

	destInstance.version++
	return nil
}

// CircularBufferCopyFromRange copies a range from src buffer into dest buffer starting at destStartIdx.
// Both buffers must have the same element type T.
// Returns an error if the range is invalid or doesn't fit.
func CircularBufferCopyFromRange[T any](
	destBuffer, srcBuffer memcore.MarkRaw,
	srcFrom, srcTo, destStartIdx uint64,
) error {
	if srcFrom >= srcTo {
		return fmt.Errorf("CircularBufferCopyFromRange: srcFrom must be < srcTo")
	}

	destInstance := memcore.MemcoreMarkDereferenceObject[CircularBuffer[T]](destBuffer)
	srcInstance := memcore.MemcoreMarkDereferenceObject[CircularBuffer[T]](srcBuffer)

	if srcTo > srcInstance.length {
		return fmt.Errorf("CircularBufferCopyFromRange: srcTo exceeds src length")
	}

	count := srcTo - srcFrom
	if destStartIdx+count > destInstance.length {
		return fmt.Errorf("CircularBufferCopyFromRange: insufficient dest length")
	}

	if count == 0 {
		return nil
	}

	// Copy element by element
	for i := uint64(0); i < count; i++ {
		srcLogicalIdx := srcFrom + i
		destLogicalIdx := destStartIdx + i
		srcPhysicalIdx := circularBufferLogicalToPhysical[T](srcInstance, srcLogicalIdx)
		destPhysicalIdx := circularBufferLogicalToPhysical[T](destInstance, destLogicalIdx)
		item := ArrayItemGetAtUnsafeFast(srcInstance.data, srcInstance.dataBase, srcPhysicalIdx)
		ArraySetAtUnsafeFast(destInstance.data, destInstance.dataBase, destPhysicalIdx, item)
	}

	destInstance.version++
	return nil
}
