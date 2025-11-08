package memstruct

import (
	"fmt"
	"memcore"
	"unsafe"
)

func ArrayRequiredBytesGet[T any](capacity uint64) uint64 {
	headerSize := memcore.SizeOf[Array[T]]()
	itemSize := memcore.SizeOf[T]()
	return headerSize + itemSize*capacity
}

func ArrayRequiredAlignmentGet[T any]() uint64 {
	return max(memcore.AlignOf[T](), memcore.AlignOf[Array[T]]())
}

// Array is a custom array implementation built on top of memcore.
type Array[T any] struct {
	dataAddrOffset uintptr
	capacity       uint64

	setFnID memcore.FunctionID

	itemSize uintptr
}

// ArrayView represents a view around an array.
// It can be readonly and/or a subset within the array.
type ArrayView[T any] struct {
	arrayHeader      memcore.MarkRaw
	startIdx, endIdx uint64
	readonly         bool
}

// ArrayInitializeAt initializes an instance of an array for type T at a specific memory address.
// Ensure the address is properly aligned and has the right size.
//
// ⚠️ capacity is in elements, not bytes.
func ArrayInitializeAt[T any](arrayAddr memcore.MarkRaw, capacity uint64) {
	headerSize := memcore.SizeOf[Array[T]]()

	itemSize := memcore.SizeOf[T]()
	arrayPtr := memcore.MemcoreMarkDereferenceObject[Array[T]](arrayAddr)
	*arrayPtr = Array[T]{
		dataAddrOffset: uintptr(headerSize),
		capacity:       capacity,
		itemSize:       uintptr(itemSize),
	}

	arrayPtr.setFnID = memcore.MemcoreFunctionRegisterTyped(
		getMovementFunc[T](itemSize),
	)
}

// ArraySnapshotCreate creates a deep copy of an array at a new memory location
// defined by the destination pointer (which points to the start of the new array header).
// It copies both the header and the data that follow it, maintaining the same relative layout.
func ArraySnapshotCreate[T any](dest memcore.MarkRaw, instance memcore.MarkRaw) memcore.MarkRaw {
	arrayPtr := memcore.MemcoreMarkDereferenceObject[Array[T]](instance)
	totalSize := ArrayRequiredBytesGet[T](arrayPtr.capacity)

	srcAddr := memcore.MemcoreMarkDereference(instance)
	dstAddr := memcore.MemcoreMarkDereference(dest)

	memcore.MemoryMoveNoHeapPointers(dstAddr, srcAddr, uintptr(totalSize))

	return dest
}

// ArraySnapshotRestore replaces the entire memory block of one array
// (header + data) with that of another array of the same type and capacity.
// Both arrays must live in manual memory managed by memcore.
func ArraySnapshotRestore[T any](dest, src memcore.MarkRaw) error {
	dstHeader := memcore.MemcoreMarkDereferenceObject[Array[T]](dest)
	srcHeader := memcore.MemcoreMarkDereferenceObject[Array[T]](src)

	if dstHeader.capacity != srcHeader.capacity {
		return fmt.Errorf("cannot restore snapshot: unequal capacities (dest=%v, src=%v)", dstHeader.capacity, srcHeader.capacity)
	}

	if dest == src {
		return nil
	}

	totalBytes := ArrayRequiredBytesGet[T](dstHeader.capacity)

	dstAddr := memcore.MemcoreMarkDereference(dest)
	srcAddr := memcore.MemcoreMarkDereference(src)

	memcore.MemoryMoveNoHeapPointers(dstAddr, srcAddr, uintptr(totalBytes))

	return nil
}

// ArrayHeaderClone clones the header to the array data.
// It will not move memory at all.
func ArrayHeaderClone[T any](dest, src memcore.MarkRaw) {
	dstHeader := memcore.MemcoreMarkDereferenceObject[Array[T]](dest)
	srcHeader := memcore.MemcoreMarkDereferenceObject[Array[T]](src)

	*dstHeader = *srcHeader
}

// ArrayHeaderSizeBytesGet returns the required bytes for the Array header.
//
//go:inline
func ArrayHeaderSizeBytesGet[T any]() uint64 {
	return memcore.SizeOf[Array[T]]()
}

// ArrayHeaderAlignmentGet returns the required alignment for the Array header.
//
//go:inline
func ArrayHeaderAlignmentGet[T any]() uint64 {
	return memcore.AlignOf[Array[T]]()
}

// ArrayViewGet produces a view over an array that is potentially a subset and/or readonly.
// It panics if from or to are invalid.
//
//go:inline
func ArrayViewGet[T any](array memcore.MarkRaw, from, to uint64, readonly bool) ArrayView[T] {
	if from >= to {
		panic("from must be smaller than to")
	}

	instance := memcore.MemcoreMarkDereferenceObject[Array[T]](array)

	if err := arrayGuaranteeIdxValidity(instance, from); err != nil {
		panic(fmt.Errorf("from invalid: %w", err))
	}

	if err := arrayGuaranteeIdxValidity(instance, to); err != nil {
		panic(fmt.Errorf("to invalid: %w", err))
	}

	return ArrayView[T]{
		arrayHeader: array,
		startIdx:    from,
		endIdx:      to,
		readonly:    readonly,
	}
}

// ArrayViewGetUnsafe produces a view over an array that is potentially a subset and/or readonly.
// It does no validation of bounds.
//
//go:inline
func ArrayViewGetUnsafe[T any](array memcore.MarkRaw, from, to uint64, readonly bool) ArrayView[T] {
	return ArrayView[T]{
		arrayHeader: array,
		startIdx:    from,
		endIdx:      to,
		readonly:    readonly,
	}
}

// ArrayCapacityGet returns the total amount of elements that can be stored.
//
//go:nosplit
//go:inline
func ArrayCapacityGet[T any](array memcore.MarkRaw) uint64 {
	instance := memcore.MemcoreMarkDereferenceObject[Array[T]](array)
	return instance.capacity
}

// ArrayItemGetAt returns T at idx within the array.
// It returns an error if the idx is invalid.
//
//go:nosplit
//go:inline
func ArrayItemGetAt[T any](array memcore.MarkRaw, idx uint64) (T, error) {
	baseAddr, instance := memcore.MemcoreMarkDereferenceObjectAlt[Array[T]](array)

	if error := arrayGuaranteeIdxValidity(instance, idx); error != nil {
		var zero T
		return zero, error
	}

	return *(*T)(arrayGetPtrAtIdx(instance, baseAddr, idx)), nil
}

// ArrayItemGetAtUnsafe returns T at idx within the array.
// It does no bounds checks.
//
//go:inline
func ArrayItemGetAtUnsafe[T any](array memcore.MarkRaw, idx uint64) T {
	baseAddr, instance := memcore.MemcoreMarkDereferenceObjectAlt[Array[T]](array)
	return *(*T)(arrayGetPtrAtIdx(instance, baseAddr, idx))
}

// ArrayItemPtrGetAt returns a pointer to T at idx within the array.
// It returns an error if the idx is invalid.
//
// Using this pointer after deletion or overwriting this idx is undefined behaviour.
// Use at your own discretion!
//
//go:nosplit
//go:inline
func ArrayItemPtrGetAt[T any](array memcore.MarkRaw, idx uint64) (*T, error) {
	baseAddr, instance := memcore.MemcoreMarkDereferenceObjectAlt[Array[T]](array)
	if error := arrayGuaranteeIdxValidity(instance, idx); error != nil {
		return nil, error
	}

	return (*T)(arrayGetPtrAtIdx(instance, baseAddr, idx)), nil
}

// ArrayItemPtrGetAtUnsafe returns a pointer to T at idx within the array.
// It does no bounds checks.
//
// Using this pointer after deletion or overwriting this idx is undefined behaviour.
// Use at your own discretion!
//
//go:inline
func ArrayItemPtrGetAtUnsafe[T any](array memcore.MarkRaw, idx uint64) *T {
	baseAddr, instance := memcore.MemcoreMarkDereferenceObjectAlt[Array[T]](array)
	return (*T)(arrayGetPtrAtIdx(instance, baseAddr, idx))
}

// ArrayDataPtrGet returns the current pointer to the underlying data storage in memory.
// This value CAN change if the underlying memory region changes.
// Not stable, so do not store.
//
//go:inline
func ArrayDataPtrGet[T any](array memcore.MarkRaw) unsafe.Pointer {
	baseAddr, instance := memcore.MemcoreMarkDereferenceObjectAlt[Array[T]](array)
	return arrayComputeDataAddr(instance, baseAddr)
}

// ArrayByteOffsetGetAt returns the offset relative to the memory region for this idx.
// Panics if the idx is invalid.
//
//go:inline
func ArrayByteOffsetGetAt[T any](array memcore.MarkRaw, idx uint64) uintptr {
	instance := memcore.MemcoreMarkDereferenceObject[Array[T]](array)
	if err := arrayGuaranteeIdxValidity(instance, idx); err != nil {
		panic(err)
	}

	return instance.dataAddrOffset + (uintptr(idx) * instance.itemSize)
}

// ArrayByteOffsetGetAtUnsafe returns the offset relative to the memory region for this idx.
// Does no bounds checks.
//
//go:inline
func ArrayByteOffsetGetAtUnsafe[T any](array memcore.MarkRaw, idx uint64) uintptr {
	instance := memcore.MemcoreMarkDereferenceObject[Array[T]](array)
	return instance.dataAddrOffset + (uintptr(idx) * instance.itemSize)
}

// ArraySetAt sets idx of array to value T.
// It returns an error if the idx is invalid.
//
//go:nosplit
//go:inline
func ArraySetAt[T any](array memcore.MarkRaw, idx uint64, value T) error {
	baseAddr, instance := memcore.MemcoreMarkDereferenceObjectAlt[Array[T]](array)
	if error := arrayGuaranteeIdxValidity(instance, idx); error != nil {
		return error
	}

	itemPtr := arrayGetPtrAtIdx(instance, baseAddr, idx)

	memcore.MemcoreFunctionRetrieveTyped[setFn[T]](instance.setFnID)(itemPtr, value)

	return nil
}

// ArraySetAtUnsafe sets idx of array to value T.
// It does no bounds checks.
//
//go:nosplit
//go:inline
func ArraySetAtUnsafe[T any](array memcore.MarkRaw, idx uint64, value T) {
	baseAddr, instance := memcore.MemcoreMarkDereferenceObjectAlt[Array[T]](array)
	itemPtr := arrayGetPtrAtIdx(instance, baseAddr, idx)

	memcore.MemcoreFunctionRetrieveTyped[setFn[T]](instance.setFnID)(itemPtr, value)
}

// ArraySetAll sets all values within the array to value T.
//
//go:inline
func ArraySetAll[T any](array memcore.MarkRaw, v T) {
	ArrayForEachUnsafe[T](array, func(ptr unsafe.Pointer, idx uint64) {
		*(*T)(ptr) = v
	})
}

// ArrayZeroAll sets all values within the array to its zero value.
// This is different from clearing the memory to 0.
//
//go:inline
func ArrayZeroAll[T any](array memcore.MarkRaw) {
	var zero T
	ArrayForEachUnsafe[T](array, func(ptr unsafe.Pointer, idx uint64) {
		*(*T)(ptr) = zero
	})
}

// ArrayForEachUnsafe calls a function for every element in the array.
//
//go:inline
func ArrayForEachUnsafe[T any](array memcore.MarkRaw, fn func(ptr unsafe.Pointer, idx uint64)) {
	baseAddr, instance := memcore.MemcoreMarkDereferenceObjectAlt[Array[T]](array)

	for idx := uint64(0); idx < instance.capacity; idx++ {
		ptr := arrayGetPtrAtIdx(instance, baseAddr, idx)
		fn(ptr, idx)
	}
}

// ArrayStrideForEachUnsafe calls a function for every element in the array.
// It visits every stride-th element.
//
//go:inline
func ArrayStrideForEachUnsafe[T any](array memcore.MarkRaw, fn func(ptr unsafe.Pointer, idx uint64), stride uint64) {
	baseAddr, instance := memcore.MemcoreMarkDereferenceObjectAlt[Array[T]](array)

	for idx := uint64(0); idx < instance.capacity; idx++ {
		if idx%stride != 0 {
			continue
		}

		ptr := arrayGetPtrAtIdx(instance, baseAddr, idx)
		fn(ptr, idx)
	}
}

// ArrayIterate allows you to iterate over the array efficiently.
// It provides a method to say which next element you need.
// The next function returns true when your requested n exceeds the capacity.
//
//go:inline
func ArrayIterate[T any](
	array memcore.MarkRaw,
	fn func(
		ptr unsafe.Pointer,
		idx uint64,
		next func(n uint64) (unsafe.Pointer, uint64, bool),
	),
) {
	baseAddr, instance := memcore.MemcoreMarkDereferenceObjectAlt[Array[T]](array)

	idx := uint64(0)
	nextFn := func(n uint64) (unsafe.Pointer, uint64, bool) {
		idx += n

		if err := arrayGuaranteeIdxValidity(instance, idx); err != nil {
			return nil, idx, true
		}

		ptr := arrayGetPtrAtIdx(instance, baseAddr, idx)
		return ptr, idx, false
	}

	ptr := arrayGetPtrAtIdx(instance, baseAddr, idx)
	fn(ptr, idx, nextFn)
}

// ArrayIterateUnsafe allows you to iterate over the array efficiently.
// It provides a method to say which next element you need.
// The next function does no bounds checking. Safety must be guaranteed by the client.
//
//go:inline
func ArrayIterateUnsafe[T any](
	array memcore.MarkRaw,
	fn func(
		ptr unsafe.Pointer,
		idx uint64,
		next func(n uint64) (unsafe.Pointer, uint64),
	),
) {
	baseAddr, instance := memcore.MemcoreMarkDereferenceObjectAlt[Array[T]](array)

	idx := uint64(0)
	nextFn := func(n uint64) (unsafe.Pointer, uint64) {
		idx += n

		ptr := arrayGetPtrAtIdx(instance, baseAddr, idx)
		return ptr, idx
	}

	ptr := arrayGetPtrAtIdx(instance, baseAddr, idx)
	fn(ptr, idx, nextFn)
}

// ArrayReplaceInternal replaces srcIdx with the value at destIdx efficiently.
//
//go:inline
func ArrayReplaceInternal[T any](array memcore.MarkRaw, srcIdx, destIdx uint64) error {
	baseAddr, instance := memcore.MemcoreMarkDereferenceObjectAlt[Array[T]](array)
	if error := arrayGuaranteeIdxValidity(instance, srcIdx); error != nil {
		return error
	}

	if error := arrayGuaranteeIdxValidity(instance, destIdx); error != nil {
		return error
	}

	srcPtr := arrayGetPtrAtIdx(instance, baseAddr, srcIdx)
	dstPtr := arrayGetPtrAtIdx(instance, baseAddr, destIdx)

	memcore.MemoryMoveNoHeapPointers(dstPtr, srcPtr, instance.itemSize)

	return nil
}

// ArrayReplaceInternalUnsafe replaces srcIdx with the value at destIdx efficiently.
//
// It does no bounds checks.
//
//go:inline
func ArrayReplaceInternalUnsafe[T any](array memcore.MarkRaw, srcIdx, destIdx uint64) {
	baseAddr, instance := memcore.MemcoreMarkDereferenceObjectAlt[Array[T]](array)
	srcPtr := arrayGetPtrAtIdx(instance, baseAddr, srcIdx)
	dstPtr := arrayGetPtrAtIdx(instance, baseAddr, destIdx)

	memcore.MemoryMoveNoHeapPointers(dstPtr, srcPtr, instance.itemSize)
}

// ArrayShiftRight shifts a contiguous range of elements in the array
// `count` positions to the right, preserving their order.
//
// Parameters:
//   - from:  starting index of the range to shift (inclusive)
//   - to:    ending index of the range to shift (inclusive)
//   - count: number of positions to shift the range to the right
//
// The data between [from, to] is moved rightward by `count` slots, such that the
// element originally at `to` ends up at index `to + count`. Any elements in the
// destination range [from+count, to+count] will be overwritten.
//
// Example:
//
//	Before: [A, B, C, D, E, F, G]
//	Call:   ArrayShiftRightUnsafe(arr, from=2, to=4, count=1)
//	Moves:  C→D, D→E, E→F
//	After:  [A, B, _, C, D, E, G]
//
//go:nosplit
//go:inline
func ArrayShiftRight[T any](array memcore.MarkRaw, from, to, count uint64) error {
	instance := memcore.MemcoreMarkDereferenceObject[Array[T]](array)
	if from >= instance.capacity || to >= instance.capacity {
		return fmt.Errorf("invalid range: from=%d to=%d capacity=%d", from, to, instance.capacity)
	}
	if count == 0 || from >= to {
		return nil
	}

	ArrayShiftRightUnsafe[T](array, from, to, count)
	return nil
}

// ArrayShiftRightUnsafe shifts a contiguous range of elements in the array
// `count` positions to the right, preserving their order. It performs no bounds
// checks, so callers must ensure valid indices.
//
// Parameters:
//   - from:  starting index of the range to shift (inclusive)
//   - to:    ending index of the range to shift (inclusive)
//   - count: number of positions to shift the range to the right
//
// The data between [from, to] is moved rightward by `count` slots, such that the
// element originally at `to` ends up at index `to + count`. Any elements in the
// destination range [from+count, to+count] will be overwritten.
//
// Example:
//
//	Before: [A, B, C, D, E, F, G]
//	Call:   ArrayShiftRightUnsafe(arr, from=2, to=4, count=1)
//	Moves:  C→D, D→E, E→F
//	After:  [A, B, _, C, D, E, G]
//
//go:nosplit
//go:inline
func ArrayShiftRightUnsafe[T any](array memcore.MarkRaw, from, to, count uint64) {
	baseAddr, instance := memcore.MemcoreMarkDereferenceObjectAlt[Array[T]](array)
	elemSize := instance.itemSize
	srcPtr := arrayGetPtrAtIdx(instance, baseAddr, from)
	dstPtr := arrayGetPtrAtIdx(instance, baseAddr, from+count)

	memcore.MemoryMoveNoHeapPointers(dstPtr, srcPtr, uintptr((to-from+1)*uint64(elemSize)))
}

// ArrayShiftLeft shifts a contiguous range of elements in the array
// `count` positions to the left, preserving their order.
//
// Parameters:
//   - from:  starting index of the range to shift (inclusive)
//   - to:    ending index of the range to shift (inclusive)
//   - count: number of positions to shift the range to the left
//
// The data between [from, to] is moved leftward by `count` slots, such that the
// element originally at `from` ends up at index `from - count`. Any elements in
// the destination range [from-count, to-count] will be overwritten.
//
// Example:
//
//	Before: [A, B, C, D, E, F, G]
//	Call:   ArrayShiftLeft(arr, from=3, to=5, count=1)
//	Moves:  D→C, E→D, F→E
//	After:  [A, B, C, D, E, _, G]
//
// Performs bounds checks and returns an error if the range exceeds capacity.
//
//go:nosplit
//go:inline
func ArrayShiftLeft[T any](array memcore.MarkRaw, from, to, count uint64) error {
	instance := memcore.MemcoreMarkDereferenceObject[Array[T]](array)
	if from >= instance.capacity || to >= instance.capacity {
		return fmt.Errorf("invalid range: from=%d to=%d capacity=%d", from, to, instance.capacity)
	}
	if count == 0 || from >= to {
		return nil
	}

	ArrayShiftLeftUnsafe[T](array, from, to, count)
	return nil
}

// ArrayShiftLeftUnsafe shifts a contiguous range of elements in the array
// `count` positions to the left, preserving their order. It performs no bounds
// checks, so callers must ensure valid indices.
//
// Parameters:
//   - from:  starting index of the range to shift (inclusive)
//   - to:    ending index of the range to shift (inclusive)
//   - count: number of positions to shift the range to the left
//
// The data between [from, to] is moved leftward by `count` slots, such that the
// element originally at `from` ends up at index `from - count`. Any elements in
// the destination range [from-count, to-count] will be overwritten.
//
// Example:
//
//	Before: [A, B, C, D, E, F, G]
//	Call:   ArrayShiftLeft(arr, from=3, to=5, count=1)
//	Moves:  D→C, E→D, F→E
//	After:  [A, B, C, D, E, _, G]
//
//go:nosplit
//go:inline
func ArrayShiftLeftUnsafe[T any](array memcore.MarkRaw, from, to, count uint64) {
	baseAddr, instance := memcore.MemcoreMarkDereferenceObjectAlt[Array[T]](array)

	elemSize := instance.itemSize
	srcPtr := arrayGetPtrAtIdx(instance, baseAddr, from+count)
	dstPtr := arrayGetPtrAtIdx(instance, baseAddr, from)

	memcore.MemoryMoveNoHeapPointers(dstPtr, srcPtr, uintptr((to-from+1)*uint64(elemSize)))
}

// ArrayRangeCopy is a convenience wrapper around shift lift/shift right.
// It automatically determines which to use based on from and to.
//
//go:inline
//go:nosplit
func ArrayRangeCopy[T any](array memcore.MarkRaw, from, to, count uint64) error {
	if from < to {
		return ArrayShiftRight[T](array, from, to, count)
	}

	return ArrayShiftLeft[T](array, from, to, count)
}

// ArrayRangeCopyUnsafe is a convenience wrapper around shift lift/shift right unsafe.
// It automatically determines which to use based on from and to.
//
//go:inline
//go:nosplit
func ArrayRangeCopyUnsafe[T any](array memcore.MarkRaw, from, to, count uint64) {
	if from < to {
		ArrayShiftRightUnsafe[T](array, from, to, count)
		return
	}

	ArrayShiftLeftUnsafe[T](array, from, to, count)
}

// ArrayDeleteAt resets memory to 0 at a given index, using pointers to this
// index gotten earlier is undefined behaviour.
// It returns an error if the index is invalid.
//
//go:nosplit
//go:inline
func ArrayDeleteAt[T any](array memcore.MarkRaw, idx uint64) error {
	baseAddr, instance := memcore.MemcoreMarkDereferenceObjectAlt[Array[T]](array)
	if error := arrayGuaranteeIdxValidity(instance, idx); error != nil {
		return error
	}

	currentPtr := arrayGetPtrAtIdx(instance, baseAddr, idx)
	memcore.MemoryClearNoHeapPointers(currentPtr, uintptr(instance.itemSize))
	return nil
}

// ArrayDeleteAtUnsafe resets memory to 0 at a given index, using pointers to this
// index gotten earlier is undefined behaviour.
// It does no bounds checks.
//
//go:nosplit
//go:inline
func ArrayDeleteAtUnsafe[T any](array memcore.MarkRaw, idx uint64) {
	baseAddr, instance := memcore.MemcoreMarkDereferenceObjectAlt[Array[T]](array)
	currentPtr := arrayGetPtrAtIdx(instance, baseAddr, idx)
	memcore.MemoryClearNoHeapPointers(currentPtr, uintptr(instance.itemSize))
}

// ArrayClear resets the entire array's memory to 0, allowing it to be reused.
// Do NOT use pointers to previous array items after this as that is undefined behaviour.
//
//go:nosplit
//go:inline
func ArrayClear[T any](array memcore.MarkRaw) {
	baseAddr, instance := memcore.MemcoreMarkDereferenceObjectAlt[Array[T]](array)
	memcore.MemoryClearNoHeapPointers(arrayComputeDataAddr(instance, baseAddr), uintptr(instance.capacity)*uintptr(instance.itemSize))
}

// ArrayIsIdxValid checks whether the given index is valid.
//
//go:inline
func ArrayIsIdxValid[T any](array memcore.MarkRaw, idx uint64) bool {
	instance := memcore.MemcoreMarkDereferenceObject[Array[T]](array)
	return idx < instance.capacity
}

// -------------------------- ARRAY VIEW

// ArrayViewLengthGet returns the length of the current view.
//
//go:inline
func ArrayViewLengthGet[T any](arrayView ArrayView[T]) uint64 {
	return arrayView.endIdx - arrayView.startIdx
}

// ArrayViewIsReadonly returns whether the current view is readonly.
//
//go:inline
func ArrayViewIsReadonly[T any](arrayView ArrayView[T]) bool {
	return arrayView.readonly
}

// ArrayViewItemGetAt returns item at idx (as computed by view startIdx+relativeIdx)
//
//go:inline
func ArrayViewItemGetAt[T any](arrayView ArrayView[T], relativeIdx uint64) (T, error) {
	idx := arrayView.startIdx + relativeIdx
	if err := arrayViewGuaranteeIdxValidity(arrayView, idx); err != nil {
		var zero T
		return zero, err
	}

	return ArrayItemGetAtUnsafe[T](arrayView.arrayHeader, idx), nil
}

// ArrayViewItemPtrGetAt returns item pointer at idx (as computed by view startIdx+relativeIdx)
// Fails if the view is readonly (because getting a pointer would allow mutation)
//
//go:inline
func ArrayViewItemPtrGetAt[T any](arrayView ArrayView[T], relativeIdx uint64) (*T, error) {
	if arrayView.readonly {
		return nil, fmt.Errorf("cannot mutate readonly view")
	}

	idx := arrayView.startIdx + relativeIdx

	if err := arrayViewGuaranteeIdxValidity(arrayView, idx); err != nil {
		return nil, err
	}

	return ArrayItemPtrGetAtUnsafe[T](arrayView.arrayHeader, idx), nil
}

// ArrayViewItemSetAt sets the item at idx (as computed by view startIdx+relativeIdx)
// Fails if the view is readonly.
//
//go:inline
func ArrayViewItemSetAt[T any](arrayView ArrayView[T], relativeIdx uint64, v T) error {
	if arrayView.readonly {
		return fmt.Errorf("cannot mutate readonly view")
	}

	idx := arrayView.startIdx + relativeIdx

	if err := arrayViewGuaranteeIdxValidity(arrayView, idx); err != nil {
		return err
	}

	ArraySetAtUnsafe(arrayView.arrayHeader, idx, v)
	return nil
}

// ArrayViewForEach calls a function for every element in the array view.
// The indexes returned are the relative indexes.
//
//go:inline
func ArrayViewForEach[T any](arrayView ArrayView[T], fn func(item T, idx uint64)) {
	baseAddr, instance := memcore.MemcoreMarkDereferenceObjectAlt[Array[T]](arrayView.arrayHeader)

	for idx := uint64(arrayView.startIdx); idx < arrayView.endIdx; idx++ {
		relIdx := idx - arrayView.startIdx
		ptr := arrayGetPtrAtIdx(instance, baseAddr, idx)
		item := *(*T)(ptr)
		fn(item, relIdx)
	}
}

// ArrayViewForEachRaw calls a function for every element in the array view.
// Not possible for readonly views.
// The indexes returned are the relative indexes.
//
//go:inline
func ArrayViewForEachRaw[T any](arrayView ArrayView[T], fn func(ptr unsafe.Pointer, idx uint64)) error {
	if arrayView.readonly {
		return fmt.Errorf("cannot mutate readonly view")
	}

	baseAddr, instance := memcore.MemcoreMarkDereferenceObjectAlt[Array[T]](arrayView.arrayHeader)

	for idx := uint64(arrayView.startIdx); idx < arrayView.endIdx; idx++ {
		relIdx := idx - arrayView.startIdx
		ptr := arrayGetPtrAtIdx(instance, baseAddr, idx)
		fn(ptr, relIdx)
	}

	return nil
}

// ArrayViewStrideForEach calls a function for every element in the array view.
// It visits every stride-th element.
// The indexes returned are the relative indexes.
//
//go:inline
func ArrayViewStrideForEach[T any](arrayView ArrayView[T], fn func(item T, idx uint64), stride uint64) {
	baseAddr, instance := memcore.MemcoreMarkDereferenceObjectAlt[Array[T]](arrayView.arrayHeader)

	for idx := uint64(arrayView.startIdx); idx < arrayView.endIdx; idx++ {
		relIdx := idx - arrayView.startIdx
		if relIdx%stride != 0 {
			continue
		}

		ptr := arrayGetPtrAtIdx(instance, baseAddr, relIdx)
		item := *(*T)(ptr)
		fn(item, idx)
	}
}

// ArrayViewStrideForEachRaw calls a function for every element in the array view.
// It visits every stride-th element.
// Not possible for readonly views.
// The indexes returned are the relative indexes.
//
//go:inline
func ArrayViewStrideForEachRaw[T any](arrayView ArrayView[T], fn func(ptr unsafe.Pointer, idx uint64), stride uint64) error {
	if arrayView.readonly {
		return fmt.Errorf("cannot mutate readonly view")
	}

	baseAddr, instance := memcore.MemcoreMarkDereferenceObjectAlt[Array[T]](arrayView.arrayHeader)

	for idx := uint64(arrayView.startIdx); idx < arrayView.endIdx; idx++ {
		relIdx := idx - arrayView.startIdx
		if relIdx%stride != 0 {
			continue
		}

		ptr := arrayGetPtrAtIdx(instance, baseAddr, relIdx)
		fn(ptr, idx)
	}

	return nil
}

// ArrayViewIterate allows you to iterate over the array view efficiently.
// It provides a method to say which next element you need.
// The next function returns true when your requested n exceeds the view length.
// The indexes returned are the relative indexes.
//
//go:inline
func ArrayViewIterate[T any](
	arrayView ArrayView[T],
	fn func(item T, idx uint64, next func(n uint64) (T, uint64, bool)),
) {
	baseAddr, instance := memcore.MemcoreMarkDereferenceObjectAlt[Array[T]](arrayView.arrayHeader)

	idx := arrayView.startIdx
	nextFn := func(n uint64) (T, uint64, bool) {
		idx += n
		relIdx := idx - arrayView.startIdx

		if err := arrayViewGuaranteeIdxValidity(arrayView, idx); err != nil {
			var zero T
			return zero, idx, true
		}

		ptr := arrayGetPtrAtIdx(instance, baseAddr, idx)
		item := *(*T)(ptr)
		return item, relIdx, false
	}

	ptr := arrayGetPtrAtIdx(instance, baseAddr, idx)
	item := *(*T)(ptr)
	fn(item, 0, nextFn)
	idx += 1
}

// ArrayViewIterateRaw allows you to iterate over the array view efficiently.
// It provides a method to say which next element you need.
// The next function returns true when your requested n exceeds the view length.
// Not possible for readonly views.
// The indexes returned are the relative indexes.
//
//go:inline
func ArrayViewIterateRaw[T any](
	arrayView ArrayView[T],
	fn func(ptr unsafe.Pointer, idx uint64, next func(n uint64) (unsafe.Pointer, uint64, bool)),
) error {
	if arrayView.readonly {
		return fmt.Errorf("cannot mutate readonly view")
	}

	baseAddr, instance := memcore.MemcoreMarkDereferenceObjectAlt[Array[T]](arrayView.arrayHeader)

	idx := arrayView.startIdx
	nextFn := func(n uint64) (unsafe.Pointer, uint64, bool) {
		idx += n
		relIdx := idx - arrayView.startIdx

		if err := arrayViewGuaranteeIdxValidity(arrayView, idx); err != nil {
			return nil, idx, true
		}

		ptr := arrayGetPtrAtIdx(instance, baseAddr, idx)
		return ptr, relIdx, false
	}

	ptr := arrayGetPtrAtIdx(instance, baseAddr, idx)
	fn(ptr, 0, nextFn)
	idx += 1
	return nil
}

// ArrayViewNestedGet creates a nested view inside an existing view.
// The indices [from:to) are relative to the current view.
// The readonly status is preserved.
func ArrayViewNestedGet[T any](view ArrayView[T], from, to uint64) ArrayView[T] {
	length := view.endIdx - view.startIdx

	if from >= to {
		panic("from must be smaller than to")
	}

	if to > length {
		panic(fmt.Errorf("invalid index (to): %v, must be between 0 and %v (exclusive)", to, length))
	}

	return ArrayView[T]{
		arrayHeader: view.arrayHeader,
		startIdx:    view.startIdx + from,
		endIdx:      view.startIdx + to,
		readonly:    view.readonly,
	}
}

// -------------------------- PRIVATE HELPERS

//go:inline
func arrayViewGuaranteeIdxValidity[T any](view ArrayView[T], globalIdx uint64) error {
	idxValid := globalIdx >= view.startIdx && globalIdx < view.endIdx

	if !idxValid {
		return fmt.Errorf("invalid index: %v, must be between %v and %v (exclusive)", globalIdx, view.startIdx, view.endIdx)
	}

	return nil
}

//go:inline
func arrayGetPtrAtIdx[T any](instance *Array[T], baseAddr unsafe.Pointer, idx uint64) unsafe.Pointer {
	return unsafe.Add(arrayComputeDataAddr(instance, baseAddr), idx*uint64(instance.itemSize))
}

//go:inline
func arrayGuaranteeIdxValidity[T any](instance *Array[T], idx uint64) error {
	idxValid := idx < instance.capacity

	if !idxValid {
		return fmt.Errorf("invalid index: %v, must be between 0 and %v (exclusive)", idx, instance.capacity)
	}

	return nil
}

//go:inline
func arrayComputeDataAddr[T any](instance *Array[T], baseAddr unsafe.Pointer) unsafe.Pointer {
	return unsafe.Add(baseAddr, instance.dataAddrOffset)
}
