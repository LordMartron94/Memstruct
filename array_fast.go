package memstruct

import (
	"fmt"
	"memcore"
	"unsafe"
)

// ArrayCapacityGetFast returns capacity using a pre-dereferenced array header.
//
//go:nosplit
//go:inline
func ArrayCapacityGetFast[T any](instance *Array[T]) uint64 {
	return instance.capacity
}

// ArrayDataStorageByteCountGetFast returns data region byte size from a pre-dereferenced header.
//
//go:inline
func ArrayDataStorageByteCountGetFast(instance *Array[byte]) uint64 {
	return instance.capacity * uint64(instance.itemSize)
}

// ArrayItemGetAtFast returns T at idx; requires stable (baseAddr, instance) from Alt deref.
//
//go:nosplit
//go:inline
func ArrayItemGetAtFast[T any](instance *Array[T], baseAddr unsafe.Pointer, idx uint64) (T, error) {
	if err := arrayGuaranteeIdxValidity(instance, idx); err != nil {
		var zero T
		return zero, err
	}
	return *(*T)(arrayGetPtrAtIdx(instance, baseAddr, idx)), nil
}

// ArrayItemGetAtUnsafeFast returns T at idx without bounds checks.
//
//go:inline
func ArrayItemGetAtUnsafeFast[T any](instance *Array[T], baseAddr unsafe.Pointer, idx uint64) T {
	return *(*T)(arrayGetPtrAtIdx(instance, baseAddr, idx))
}

// ArrayItemPtrGetAtFast returns a pointer to T at idx.
//
//go:nosplit
//go:inline
func ArrayItemPtrGetAtFast[T any](instance *Array[T], baseAddr unsafe.Pointer, idx uint64) (*T, error) {
	if err := arrayGuaranteeIdxValidity(instance, idx); err != nil {
		return nil, err
	}
	return (*T)(arrayGetPtrAtIdx(instance, baseAddr, idx)), nil
}

// ArrayItemPtrGetAtUnsafeFast returns a pointer to T at idx without bounds checks.
//
//go:inline
func ArrayItemPtrGetAtUnsafeFast[T any](instance *Array[T], baseAddr unsafe.Pointer, idx uint64) *T {
	return (*T)(arrayGetPtrAtIdx(instance, baseAddr, idx))
}

// ArrayDataPtrGetFast returns the current data storage pointer.
//
//go:inline
func ArrayDataPtrGetFast[T any](instance *Array[T], baseAddr unsafe.Pointer) unsafe.Pointer {
	return arrayComputeDataAddr(instance, baseAddr)
}

// ArrayDataAlignmentVerifyFast checks data alignment using pre-dereferenced header and base.
//
//go:inline
func ArrayDataAlignmentVerifyFast[T any](instance *Array[T], baseAddr unsafe.Pointer) bool {
	dataPtr := arrayComputeDataAddr(instance, baseAddr)
	if dataPtr == nil {
		return false
	}
	requiredAlign := ArrayDataRequiredAlignmentGet[T]()
	return uintptr(dataPtr)%uintptr(requiredAlign) == 0
}

// ArrayDataAlignmentGetFast returns the actual alignment of the data pointer.
//
//go:inline
func ArrayDataAlignmentGetFast[T any](instance *Array[T], baseAddr unsafe.Pointer) uint64 {
	dataPtr := arrayComputeDataAddr(instance, baseAddr)
	if dataPtr == nil {
		return 0
	}
	ptr := uintptr(dataPtr)
	if ptr == 0 {
		return 0
	}
	if (ptr & 127) == 0 {
		return 128
	}
	if (ptr & 63) == 0 {
		return 64
	}
	if (ptr & 31) == 0 {
		return 32
	}
	if (ptr & 15) == 0 {
		return 16
	}
	if (ptr & 7) == 0 {
		return 8
	}
	if (ptr & 3) == 0 {
		return 4
	}
	if (ptr & 1) == 0 {
		return 2
	}
	return 1
}

// ArrayByteOffsetGetAtFast returns the byte offset for idx; panics if invalid.
//
//go:inline
func ArrayByteOffsetGetAtFast[T any](instance *Array[T], idx uint64) uintptr {
	if err := arrayGuaranteeIdxValidity(instance, idx); err != nil {
		panic(err)
	}
	return instance.dataAddrOffset + (uintptr(idx) * instance.itemSize)
}

// ArrayByteOffsetGetAtUnsafeFast returns the byte offset for idx without bounds checks.
//
//go:inline
func ArrayByteOffsetGetAtUnsafeFast[T any](instance *Array[T], idx uint64) uintptr {
	return instance.dataAddrOffset + (uintptr(idx) * instance.itemSize)
}

// ArraySetAtFast sets idx to value; increments version on the header in place.
//
//go:nosplit
//go:inline
func ArraySetAtFast[T any](instance *Array[T], baseAddr unsafe.Pointer, idx uint64, value T) error {
	if err := arrayGuaranteeIdxValidity(instance, idx); err != nil {
		return err
	}
	itemPtr := arrayGetPtrAtIdx(instance, baseAddr, idx)
	memcore.MemcoreFunctionRetrieveTyped[setFn[T]](instance.setFnID)(itemPtr, value)
	arrayIncrementVersionDeref(instance)
	return nil
}

// ArraySetAtUnsafeFast sets idx without bounds checks.
//
//go:nosplit
//go:inline
func ArraySetAtUnsafeFast[T any](instance *Array[T], baseAddr unsafe.Pointer, idx uint64, value T) {
	itemPtr := arrayGetPtrAtIdx(instance, baseAddr, idx)
	memcore.MemcoreFunctionRetrieveTyped[setFn[T]](instance.setFnID)(itemPtr, value)
	arrayIncrementVersionDeref(instance)
}

// ArraySetAllFast sets every element to v.
//
//go:inline
func ArraySetAllFast[T any](instance *Array[T], baseAddr unsafe.Pointer, v T) {
	ArrayForEachUnsafeFast(instance, baseAddr, func(ptr unsafe.Pointer, idx uint64) {
		*(*T)(ptr) = v
	})
	arrayIncrementVersionDeref(instance)
}

// ArrayZeroAllFast sets every element to zero value.
//
//go:inline
func ArrayZeroAllFast[T any](instance *Array[T], baseAddr unsafe.Pointer) {
	var zero T
	ArrayForEachUnsafeFast(instance, baseAddr, func(ptr unsafe.Pointer, idx uint64) {
		*(*T)(ptr) = zero
	})
	arrayIncrementVersionDeref(instance)
}

// ArrayForEachUnsafeFast calls fn for every element.
//
//go:inline
func ArrayForEachUnsafeFast[T any](instance *Array[T], baseAddr unsafe.Pointer, fn func(ptr unsafe.Pointer, idx uint64)) {
	for idx := uint64(0); idx < instance.capacity; idx++ {
		ptr := arrayGetPtrAtIdx(instance, baseAddr, idx)
		fn(ptr, idx)
	}
}

// ArrayStrideForEachUnsafeFast visits every stride-th element.
//
//go:inline
func ArrayStrideForEachUnsafeFast[T any](
	instance *Array[T],
	baseAddr unsafe.Pointer,
	strideFn func(ptr unsafe.Pointer, idx uint64),
	tailFn func(ptr unsafe.Pointer, idx uint64),
	stride uint64,
) {
	idx := uint64(0)
	for ; idx+stride < instance.capacity; idx += stride {
		ptr := arrayGetPtrAtIdx(instance, baseAddr, idx)
		strideFn(ptr, idx)
	}
	for ; idx < instance.capacity; idx++ {
		ptr := arrayGetPtrAtIdx(instance, baseAddr, idx)
		tailFn(ptr, idx)
	}
}

// ArrayIterateFast iterates with a next function; stops when next exceeds capacity.
//
//go:inline
func ArrayIterateFast[T any](
	instance *Array[T],
	baseAddr unsafe.Pointer,
	fn func(ptr unsafe.Pointer, idx uint64, next func(n uint64) (unsafe.Pointer, uint64, bool)),
) {
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

// ArrayIterateUnsafeFast iterates without bounds checking in next.
//
//go:inline
func ArrayIterateUnsafeFast[T any](
	instance *Array[T],
	baseAddr unsafe.Pointer,
	fn func(ptr unsafe.Pointer, idx uint64, next func(n uint64) (unsafe.Pointer, uint64)),
) {
	idx := uint64(0)
	nextFn := func(n uint64) (unsafe.Pointer, uint64) {
		idx += n
		ptr := arrayGetPtrAtIdx(instance, baseAddr, idx)
		return ptr, idx
	}
	ptr := arrayGetPtrAtIdx(instance, baseAddr, idx)
	fn(ptr, idx, nextFn)
}

// ArrayReplaceInternalFast copies srcIdx to destIdx with bounds checks.
//
//go:inline
func ArrayReplaceInternalFast[T any](instance *Array[T], baseAddr unsafe.Pointer, srcIdx, destIdx uint64) error {
	if err := arrayGuaranteeIdxValidity(instance, srcIdx); err != nil {
		return err
	}
	if err := arrayGuaranteeIdxValidity(instance, destIdx); err != nil {
		return err
	}
	srcPtr := arrayGetPtrAtIdx(instance, baseAddr, srcIdx)
	dstPtr := arrayGetPtrAtIdx(instance, baseAddr, destIdx)
	memcore.MemoryMoveNoHeapPointers(dstPtr, srcPtr, instance.itemSize)
	return nil
}

// ArrayReplaceInternalUnsafeFast copies srcIdx to destIdx without bounds checks.
//
//go:inline
func ArrayReplaceInternalUnsafeFast[T any](instance *Array[T], baseAddr unsafe.Pointer, srcIdx, destIdx uint64) {
	srcPtr := arrayGetPtrAtIdx(instance, baseAddr, srcIdx)
	dstPtr := arrayGetPtrAtIdx(instance, baseAddr, destIdx)
	memcore.MemoryMoveNoHeapPointers(dstPtr, srcPtr, instance.itemSize)
	arrayIncrementVersionDeref(instance)
}

// ArrayShiftRightUnsafeFast shifts a range right by count positions.
//
//go:nosplit
//go:inline
func ArrayShiftRightUnsafeFast[T any](instance *Array[T], baseAddr unsafe.Pointer, from, to, count uint64) {
	elemSize := instance.itemSize
	srcPtr := arrayGetPtrAtIdx(instance, baseAddr, from)
	dstPtr := arrayGetPtrAtIdx(instance, baseAddr, from+count)
	memcore.MemoryMoveNoHeapPointers(dstPtr, srcPtr, uintptr((to-from+1)*uint64(elemSize)))
	arrayIncrementVersionDeref(instance)
}

// ArrayShiftLeftUnsafeFast shifts a range left by count positions.
//
//go:nosplit
//go:inline
func ArrayShiftLeftUnsafeFast[T any](instance *Array[T], baseAddr unsafe.Pointer, from, to, count uint64) {
	elemSize := instance.itemSize
	srcPtr := arrayGetPtrAtIdx(instance, baseAddr, from+count)
	dstPtr := arrayGetPtrAtIdx(instance, baseAddr, from)
	memcore.MemoryMoveNoHeapPointers(dstPtr, srcPtr, uintptr((to-from+1)*uint64(elemSize)))
}

// ArrayDeleteAtFast clears one element; increments version.
//
//go:nosplit
//go:inline
func ArrayDeleteAtFast[T any](instance *Array[T], baseAddr unsafe.Pointer, idx uint64) error {
	if err := arrayGuaranteeIdxValidity(instance, idx); err != nil {
		return err
	}
	currentPtr := arrayGetPtrAtIdx(instance, baseAddr, idx)
	memcore.MemoryClearNoHeapPointers(currentPtr, uintptr(instance.itemSize))
	arrayIncrementVersionDeref(instance)
	return nil
}

// ArrayDeleteAtUnsafeFast clears one element without bounds checks.
//
//go:nosplit
//go:inline
func ArrayDeleteAtUnsafeFast[T any](instance *Array[T], baseAddr unsafe.Pointer, idx uint64) {
	currentPtr := arrayGetPtrAtIdx(instance, baseAddr, idx)
	memcore.MemoryClearNoHeapPointers(currentPtr, uintptr(instance.itemSize))
	arrayIncrementVersionDeref(instance)
}

// ArrayClearFast clears the entire data region.
//
//go:nosplit
//go:inline
func ArrayClearFast[T any](instance *Array[T], baseAddr unsafe.Pointer) {
	memcore.MemoryClearNoHeapPointers(
		arrayComputeDataAddr(instance, baseAddr),
		uintptr(instance.capacity)*uintptr(instance.itemSize),
	)
	arrayIncrementVersionDeref(instance)
}

// ArrayIsIdxValidFast checks index validity from a pre-dereferenced header.
//
//go:inline
func ArrayIsIdxValidFast[T any](instance *Array[T], idx uint64) bool {
	return idx < instance.capacity
}

// ArraySortRangeFast sorts [from, to) in-place using pre-dereferenced base and header.
func ArraySortRangeFast[T any](base unsafe.Pointer, inst *Array[T], from, to uint64, cmp func(a, b T) int) {
	arraySortRangeDeref(base, inst, from, to, cmp)
}

// ArraySortFast sorts the full array in-place.
func ArraySortFast[T any](base unsafe.Pointer, inst *Array[T], cmp func(a, b T) int) {
	capacity := inst.capacity
	if capacity <= 1 {
		return
	}
	arraySortRangeDeref(base, inst, 0, capacity, cmp)
}

// ArrayCopyFromFast copies the full source array into dest starting at destStartIdx.
func ArrayCopyFromFast[T any](
	destInst *Array[T], destBase unsafe.Pointer,
	srcInst *Array[T], srcBase unsafe.Pointer,
	destStartIdx uint64,
) error {
	srcCap := srcInst.capacity
	destCap := destInst.capacity

	if destStartIdx+srcCap > destCap {
		return fmt.Errorf(
			"ArrayCopyFromFast: insufficient capacity (destStart=%d, srcCap=%d, destCap=%d)",
			destStartIdx, srcCap, destCap,
		)
	}

	srcPtr := arrayComputeDataAddr(srcInst, srcBase)
	dstPtr := unsafe.Add(arrayComputeDataAddr(destInst, destBase), uintptr(destStartIdx)*destInst.itemSize)
	totalBytes := uintptr(srcCap) * uintptr(srcInst.itemSize)
	memcore.MemoryMoveNoHeapPointers(dstPtr, srcPtr, totalBytes)
	arrayIncrementVersionDeref(destInst)
	return nil
}

// ArrayCopyFromRangeFast copies a source range into dest at destStartIdx.
func ArrayCopyFromRangeFast[T any](
	destInst *Array[T], destBase unsafe.Pointer,
	srcInst *Array[T], srcBase unsafe.Pointer,
	from, to, destStartIdx uint64,
) error {
	if from >= to {
		return fmt.Errorf("ArrayCopyFromRangeFast: from must be < to")
	}
	if to > srcInst.capacity {
		return fmt.Errorf("ArrayCopyFromRangeFast: 'to' exceeds src capacity")
	}
	count := to - from
	if destStartIdx+count > destInst.capacity {
		return fmt.Errorf("ArrayCopyFromRangeFast: insufficient dest capacity")
	}
	srcPtr := unsafe.Add(arrayComputeDataAddr(srcInst, srcBase), uintptr(from)*srcInst.itemSize)
	dstPtr := unsafe.Add(arrayComputeDataAddr(destInst, destBase), uintptr(destStartIdx)*destInst.itemSize)
	bytes := uintptr(count) * srcInst.itemSize
	memcore.MemoryMoveNoHeapPointers(dstPtr, srcPtr, bytes)
	arrayIncrementVersionDeref(destInst)
	return nil
}

// ArraySetFromSliceRangeFast copies a slice range into the array.
func ArraySetFromSliceRangeFast[T any](
	instance *Array[T], baseAddr unsafe.Pointer,
	slice []T,
	sliceStartIdx, sliceEndIdx, arrayStartIdx uint64,
) error {
	if sliceStartIdx >= sliceEndIdx {
		return fmt.Errorf("ArraySetFromSliceRangeFast: sliceStartIdx must be < sliceEndIdx")
	}
	sliceLen := uint64(len(slice))
	if sliceEndIdx > sliceLen {
		return fmt.Errorf("ArraySetFromSliceRangeFast: sliceEndIdx exceeds slice length")
	}
	if sliceLen == 0 {
		return nil
	}
	count := sliceEndIdx - sliceStartIdx
	if arrayStartIdx+count > instance.capacity {
		return fmt.Errorf("ArraySetFromSliceRangeFast: insufficient array capacity")
	}
	slicePtr := unsafe.Pointer(&slice[sliceStartIdx])
	dstPtr := unsafe.Add(arrayComputeDataAddr(instance, baseAddr), uintptr(arrayStartIdx)*instance.itemSize)
	bytes := uintptr(count) * instance.itemSize
	memcore.MemoryMoveNoHeapPointers(dstPtr, slicePtr, bytes)
	arrayIncrementVersionDeref(instance)
	return nil
}

// ArraySetFromSliceRangeUnsafeFast copies a slice range without bounds checks.
//
//go:nosplit
//go:inline
func ArraySetFromSliceRangeUnsafeFast[T any](
	instance *Array[T], baseAddr unsafe.Pointer,
	slice []T,
	sliceStartIdx, sliceEndIdx, arrayStartIdx uint64,
) {
	if sliceStartIdx >= sliceEndIdx {
		return
	}
	count := sliceEndIdx - sliceStartIdx
	slicePtr := unsafe.Pointer(&slice[sliceStartIdx])
	dstPtr := unsafe.Add(arrayComputeDataAddr(instance, baseAddr), uintptr(arrayStartIdx)*instance.itemSize)
	bytes := uintptr(count) * instance.itemSize
	memcore.MemoryMoveNoHeapPointers(dstPtr, slicePtr, bytes)
	arrayIncrementVersionDeref(instance)
}

// ArraySortedFast copies sorted data from src to dst (same capacity), then sorts dst.
func ArraySortedFast[T any](
	srcInst *Array[T], srcBase unsafe.Pointer,
	dstInst *Array[T], dstBase unsafe.Pointer,
	cmp func(a, b T) int,
) {
	if srcInst.capacity != dstInst.capacity {
		panic("ArraySortedFast: capacity mismatch")
	}
	srcData := unsafe.Add(srcBase, srcInst.dataAddrOffset)
	dstData := unsafe.Add(dstBase, dstInst.dataAddrOffset)
	totalBytes := uintptr(srcInst.capacity) * uintptr(srcInst.itemSize)
	memcore.MemoryMoveNoHeapPointers(dstData, srcData, totalBytes)
	ArraySortFast(dstBase, dstInst, cmp)
}

// ArrayVersionGetFast returns version from a pre-dereferenced header.
//
//go:inline
func ArrayVersionGetFast[T any](instance *Array[T]) uint64 {
	return instance.version
}

// ArrayViewGetFast produces a view using a pre-dereferenced header for bounds validation.
//
//go:inline
func ArrayViewGetFast[T any](array memcore.MarkRaw, instance *Array[T], from, to uint64, readonly bool) ArrayView[T] {
	if from >= to {
		panic("from must be smaller than to")
	}
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

// ArrayViewForEachFast iterates the view using pre-dereferenced array base and header.
//
//go:inline
func ArrayViewForEachFast[T any](
	arrayView ArrayView[T],
	instance *Array[T],
	baseAddr unsafe.Pointer,
	fn func(item T, idx uint64),
) {
	for idx := uint64(arrayView.startIdx); idx < arrayView.endIdx; idx++ {
		relIdx := idx - arrayView.startIdx
		ptr := arrayGetPtrAtIdx(instance, baseAddr, idx)
		item := *(*T)(ptr)
		fn(item, relIdx)
	}
}

// ArrayViewForEachRawFast iterates with raw pointers; fails on readonly views.
//
//go:inline
func ArrayViewForEachRawFast[T any](
	arrayView ArrayView[T],
	instance *Array[T],
	baseAddr unsafe.Pointer,
	fn func(ptr unsafe.Pointer, idx uint64),
) error {
	if arrayView.readonly {
		return fmt.Errorf("cannot mutate readonly view")
	}
	for idx := uint64(arrayView.startIdx); idx < arrayView.endIdx; idx++ {
		relIdx := idx - arrayView.startIdx
		ptr := arrayGetPtrAtIdx(instance, baseAddr, idx)
		fn(ptr, relIdx)
	}
	return nil
}
