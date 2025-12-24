package memstruct

import (
	"fmt"
	"foundation"
	"memcore"
	"strings"
	"unsafe"
)

// Vector is structurally equivalent to Array except specialized for numerics.
// All Array functions work as well for the Vector.
// For convenience, most, if not all, of these are wrapped under a facade with the Vector prefix.
type Vector[T foundation.Numeric] Array[T]

// VectorView represents a view around a vector.
// It can be readonly and/or a subset within the vector.
type VectorView[T foundation.Numeric] ArrayView[T]

func VectorRequiredBytesGet[T foundation.Numeric](capacity uint64) uint64 {
	return ArrayRequiredBytesGet[T](capacity)
}

func VectorRequiredAlignmentGet[T foundation.Numeric]() uint64 {
	return ArrayRequiredAlignmentGet[T]()
}

func (v *Vector[T]) String() string {
	if v == nil {
		return "<nil Vector>"
	}

	var sb strings.Builder
	fmt.Fprintf(&sb, "Vector[%T]{capacity=%d, itemSize=%d, data=[", *new(T), v.capacity, v.itemSize)

	baseAddr := unsafe.Pointer(v)
	dataAddr := unsafe.Add(baseAddr, v.dataAddrOffset)

	for i := uint64(0); i < v.capacity; i++ {
		if i > 0 {
			sb.WriteString(", ")
		}
		val := *(*T)(unsafe.Add(dataAddr, uintptr(i)*v.itemSize))
		fmt.Fprintf(&sb, "%v", val)
	}

	sb.WriteString("]}")
	return sb.String()
}

// VectorInitializeAt initializes an instance of an vector for type T at a specific memory address.
// Ensure the address is properly aligned and has the right size.
//
// ⚠️ capacity is in elements, not bytes.
func VectorInitializeAt[T foundation.Numeric](vectorAddr memcore.MarkRaw, capacity uint64) {
	ArrayInitializeAt[T](vectorAddr, capacity)
}

// VectorInitializeFrom initializes a new vector at vectorAddr with the contents of src.
// Capacity must be >= src capacity.
func VectorInitializeFrom[T foundation.Numeric](vectorAddr memcore.MarkRaw, src memcore.MarkRaw, newCapacity uint64) error {
	return VectorInitializeFrom[T](vectorAddr, src, newCapacity)
}

// VectorSnapshotCreate creates a deep copy of an vector at a new memory location
// defined by the destination pointer (which points to the start of the new vector header).
// It copies both the header and the data that follow it, maintaining the same relative layout.
func VectorSnapshotCreate[T foundation.Numeric](dest memcore.MarkRaw, instance memcore.MarkRaw) memcore.MarkRaw {
	return ArraySnapshotCreate[T](dest, instance)
}

// VectorSnapshotRestore replaces the entire memory block of one vector
// (header + data) with that of another vector of the same type and capacity.
// Both vectors must live in manual memory managed by memcore.
func VectorSnapshotRestore[T foundation.Numeric](dest, src memcore.MarkRaw) error {
	return ArraySnapshotRestore[T](dest, src)
}

// VectorHeaderClone clones the header to the vector data.
// It will not move memory at all.
func VectorHeaderClone[T foundation.Numeric](dest, src memcore.MarkRaw) {
	ArrayHeaderClone[T](dest, src)
}

// VectorCopyFrom copies the entire contents of src vector into dest vector,
// starting at destStartIdx. Both arrays must have the same element type T.
// Capacity must allow the copy, else an error is returned.
//
// Example: copy src[0:srcCap] → dest[destStartIdx : destStartIdx+srcCap]
func VectorCopyFrom[T foundation.Numeric](dest memcore.MarkRaw, src memcore.MarkRaw, destStartIdx uint64) error {
	return ArrayCopyFrom[T](dest, src, destStartIdx)
}

// VectorCopyFromRange copies src[from:to) into dest starting at destStartIdx.
// Bounds are checked; both arrays must have same type T.
func VectorCopyFromRange[T foundation.Numeric](
	dest memcore.MarkRaw,
	src memcore.MarkRaw,
	from, to, destStartIdx uint64,
) error {
	return ArrayCopyFromRange[T](dest, src, from, to, destStartIdx)
}

// VectorHeaderSizeBytesGet returns the required bytes for the Vector header.
//
//go:inline
func VectorHeaderSizeBytesGet[T foundation.Numeric]() uint64 {
	return ArrayHeaderSizeBytesGet[T]()
}

// VectorHeaderAlignmentGet returns the required alignment for the Vector header.
//
//go:inline
func VectorHeaderAlignmentGet[T foundation.Numeric]() uint64 {
	return ArrayHeaderAlignmentGet[T]()
}

// VectorViewGet produces a view over a vector that is potentially a subset and/or readonly.
// It panics if from or to are invalid.
//
//go:inline
func VectorViewGet[T foundation.Numeric](array memcore.MarkRaw, from, to uint64, readonly bool) VectorView[T] {
	return VectorView[T](ArrayViewGet[T](array, from, to, readonly))
}

// VectorViewGetUnsafe produces a view over a vector that is potentially a subset and/or readonly.
// It does no validation of bounds.
//
//go:inline
func VectorViewGetUnsafe[T foundation.Numeric](array memcore.MarkRaw, from, to uint64, readonly bool) VectorView[T] {
	return VectorView[T](ArrayViewGetUnsafe[T](array, from, to, readonly))
}

// VectorCapacityGet returns the total amount of elements that can be stored.
//
//go:nosplit
//go:inline
func VectorCapacityGet[T foundation.Numeric](vector memcore.MarkRaw) uint64 {
	return ArrayCapacityGet[T](vector)
}

// VectorItemGetAt returns T at idx within the vector.
// It returns an error if the idx is invalid.
//
//go:nosplit
//go:inline
func VectorItemGetAt[T foundation.Numeric](vector memcore.MarkRaw, idx uint64) (T, error) {
	return ArrayItemGetAt[T](vector, idx)
}

// VectorItemGetAtUnsafe returns T at idx within the vector.
// It does no bounds checks.
//
//go:inline
func VectorItemGetAtUnsafe[T foundation.Numeric](vector memcore.MarkRaw, idx uint64) T {
	return ArrayItemGetAtUnsafe[T](vector, idx)
}

// VectorItemPtrGetAt returns a pointer to T at idx within the vector.
// It returns an error if the idx is invalid.
//
// Using this pointer after deletion or overwriting this idx is undefined behaviour.
// Use at your own discretion!
//
//go:nosplit
//go:inline
func VectorItemPtrGetAt[T foundation.Numeric](vector memcore.MarkRaw, idx uint64) (*T, error) {
	return ArrayItemPtrGetAt[T](vector, idx)
}

// VectorItemPtrGetAtUnsafe returns a pointer to T at idx within the vector.
// It does no bounds checks.
//
// Using this pointer after deletion or overwriting this idx is undefined behaviour.
// Use at your own discretion!
//
//go:inline
func VectorItemPtrGetAtUnsafe[T foundation.Numeric](vector memcore.MarkRaw, idx uint64) *T {
	return ArrayItemPtrGetAtUnsafe[T](vector, idx)
}

// VectorDataPtrGet returns the current pointer to the underlying data storage in memory.
// This value CAN change if the underlying memory region changes.
// Not stable, so do not store.
//
//go:inline
func VectorDataPtrGet[T foundation.Numeric](array memcore.MarkRaw) unsafe.Pointer {
	return ArrayDataPtrGet[T](array)
}

// VectorByteOffsetGetAt returns the offset relative to the memory region for this idx.
// Panics if the idx is invalid.
//
//go:inline
func VectorByteOffsetGetAt[T foundation.Numeric](array memcore.MarkRaw, idx uint64) uintptr {
	return ArrayByteOffsetGetAt[T](array, idx)
}

// VectorByteOffsetGetAtUnsafe returns the offset relative to the memory region for this idx.
// Does no bounds checks.
//
//go:inline
func VectorByteOffsetGetAtUnsafe[T foundation.Numeric](array memcore.MarkRaw, idx uint64) uintptr {
	return ArrayByteOffsetGetAtUnsafe[T](array, idx)
}

// VectorSetAt sets idx of vector to value T.
// It returns an error if the idx is invalid.
//
//go:nosplit
//go:inline
func VectorSetAt[T foundation.Numeric](vector memcore.MarkRaw, idx uint64, value T) error {
	return ArraySetAt(vector, idx, value)
}

// VectorSetAtUnsafe sets idx of vector to value T.
// It does no bounds checks.
//
//go:nosplit
//go:inline
func VectorSetAtUnsafe[T foundation.Numeric](vector memcore.MarkRaw, idx uint64, value T) {
	ArraySetAtUnsafe(vector, idx, value)
}

// VectorSetAll sets all values within the vector to value T.
//
//go:inline
func VectorSetAll[T foundation.Numeric](vector memcore.MarkRaw, v T) {
	ArraySetAll(vector, v)
}

// VectorZeroAll sets all values within the Vector to its zero value.
// This is different from clearing the memory to 0.
//
//go:inline
func VectorZeroAll[T foundation.Numeric](vector memcore.MarkRaw) {
	ArrayZeroAll[T](vector)
}

// VectorForEachUnsafe calls a function for every element in the vector.
//
//go:inline
func VectorForEachUnsafe[T foundation.Numeric](vector memcore.MarkRaw, fn func(ptr unsafe.Pointer, idx uint64)) {
	ArrayForEachUnsafe[T](vector, fn)
}

// VectorStrideForEachUnsafe calls a function for every element in the vector.
// It visits every stride-th element.
//
// For the tail it calls the tailFn which is supposed to process one element at once.
//
//go:inline
func VectorStrideForEachUnsafe[T foundation.Numeric](
	vector memcore.MarkRaw,
	fn func(ptr unsafe.Pointer, idx uint64),
	tailFn func(ptr unsafe.Pointer, idx uint64),
	stride uint64,
) {
	ArrayStrideForEachUnsafe[T](vector, fn, tailFn, stride)
}

// VectorIterate allows you to iterate over the vector efficiently.
// It provides a method to say which next element you need.
// The next function returns true when your requested n exceeds the capacity.
//
//go:inline
func VectorIterate[T foundation.Numeric](
	vector memcore.MarkRaw,
	fn func(ptr unsafe.Pointer, idx uint64, next func(n uint64) (unsafe.Pointer, uint64, bool)),
) {
	ArrayIterate[T](vector, fn)
}

// VectorIterateUnsafe allows you to iterate over the vector efficiently.
// It provides a method to say which next element you need.
// The next function does no bounds checking. Safety must be guaranteed by the client.
//
//go:inline
func VectorIterateUnsafe[T any](
	vector memcore.MarkRaw,
	fn func(ptr unsafe.Pointer, idx uint64, next func(n uint64) (unsafe.Pointer, uint64)),
) {
	ArrayIterateUnsafe[T](vector, fn)
}

// VectorReplaceInternal replaces srcIdx with the value at destIdx efficiently.
//
//go:inline
func VectorReplaceInternal[T foundation.Numeric](vector memcore.MarkRaw, srcIdx, destIdx uint64) error {
	return ArrayReplaceInternal[T](vector, srcIdx, destIdx)
}

// VectorReplaceInternalUnsafe replaces srcIdx with the value at destIdx efficiently.
//
// It does no bounds checks.
//
//go:inline
func VectorReplaceInternalUnsafe[T foundation.Numeric](vector memcore.MarkRaw, srcIdx, destIdx uint64) {
	ArrayReplaceInternalUnsafe[T](vector, srcIdx, destIdx)
}

// VectorShiftRight shifts a contiguous range of elements in the vector
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
//	Call:   VectorShiftRightUnsafe(arr, from=2, to=4, count=1)
//	Moves:  C→D, D→E, E→F
//	After:  [A, B, _, C, D, E, G]
//
//go:nosplit
//go:inline
func VectorShiftRight[T foundation.Numeric](vector memcore.MarkRaw, from, to, count uint64) error {
	return ArrayShiftRight[T](vector, from, to, count)
}

// VectorShiftRightUnsafe shifts a contiguous range of elements in the vector
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
//	Call:   VectorShiftRightUnsafe(arr, from=2, to=4, count=1)
//	Moves:  C→D, D→E, E→F
//	After:  [A, B, _, C, D, E, G]
//
//go:nosplit
//go:inline
func VectorShiftRightUnsafe[T foundation.Numeric](vector memcore.MarkRaw, from, to, count uint64) {
	ArrayShiftRightUnsafe[T](vector, from, to, count)
}

// VectorShiftLeft shifts a contiguous range of elements in the vector
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
//	Call:   VectorShiftLeft(arr, from=3, to=5, count=1)
//	Moves:  D→C, E→D, F→E
//	After:  [A, B, C, D, E, _, G]
//
// Performs bounds checks and returns an error if the range exceeds capacity.
//
//go:nosplit
//go:inline
func VectorShiftLeft[T foundation.Numeric](vector memcore.MarkRaw, from, to, count uint64) error {
	return VectorShiftLeft[T](vector, from, to, count)
}

// VectorShiftLeftUnsafe shifts a contiguous range of elements in the vector
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
//	Call:   VectorShiftLeft(arr, from=3, to=5, count=1)
//	Moves:  D→C, E→D, F→E
//	After:  [A, B, C, D, E, _, G]
//
//go:nosplit
//go:inline
func VectorShiftLeftUnsafe[T foundation.Numeric](vector memcore.MarkRaw, from, to, count uint64) {
	ArrayShiftLeftUnsafe[T](vector, from, to, count)
}

// VectorRangeCopy is a convenience wrapper around shift lift/shift right.
// It automatically determines which to use based on from and to.
//
//go:inline
//go:nosplit
func VectorRangeCopy[T foundation.Numeric](vector memcore.MarkRaw, from, to, count uint64) error {
	return ArrayRangeCopy[T](vector, from, to, count)
}

// VectorRangeCopyUnsafe is a convenience wrapper around shift lift/shift right unsafe.
// It automatically determines which to use based on from and to.
//
//go:inline
//go:nosplit
func VectorRangeCopyUnsafe[T foundation.Numeric](vector memcore.MarkRaw, from, to, count uint64) {
	ArrayRangeCopyUnsafe[T](vector, from, to, count)
}

// VectorDeleteAt resets memory to 0 at a given index, using pointers to this
// index gotten earlier is undefined behaviour.
// It returns an error if the index is invalid.
//
//go:nosplit
//go:inline
func VectorDeleteAt[T foundation.Numeric](vector memcore.MarkRaw, idx uint64) error {
	return ArrayDeleteAt[T](vector, idx)
}

// VectorDeleteAtUnsafe resets memory to 0 at a given index, using pointers to this
// index gotten earlier is undefined behaviour.
// It does no bounds checks.
//
//go:nosplit
//go:inline
func VectorDeleteAtUnsafe[T foundation.Numeric](vector memcore.MarkRaw, idx uint64) {
	ArrayDeleteAtUnsafe[T](vector, idx)
}

// VectorClear resets the entire vector's memory to 0, allowing it to be reused.
// Do NOT use pointers to previous vector items after this as that is undefined behaviour.
//
//go:nosplit
//go:inline
func VectorClear[T foundation.Numeric](vector memcore.MarkRaw) {
	ArrayClear[T](vector)
}

// VectorIsIdxValid checks whether the given index is valid.
//
//go:inline
func VectorIsIdxValid[T foundation.Numeric](vector memcore.MarkRaw, idx uint64) bool {
	return ArrayIsIdxValid[T](vector, idx)
}

// VectorSort sorts the vector in-place.
//
// This implementation uses an iterative Quicksort with Median-of-Three pivot
// selection to ensure O(n log n) performance and zero stack-overflow risk.
//
// Internally delegates to ArraySort since Vector is a constrained wrapper around Array.
func VectorSort[T foundation.Numeric](vector memcore.MarkRaw, cmp func(a, b T) int) {
	ArraySort[T](vector, cmp)
}

// VectorSorted returns a sorted variant of this vector.
//
// The comparison function should return:
// a < b : -1 (or negative)
// a == b : 0
// a > b : 1 (or positive)
func VectorSorted[T foundation.Numeric](
	vector memcore.MarkRaw,
	targetVectorAddr memcore.MarkRaw,
	cmp func(a, b T) int,
) {
	srcBase, srcInst := memcore.MemcoreMarkDereferenceObjectAltUnsafe[Vector[T]](vector)
	dstBase, dstInst := memcore.MemcoreMarkDereferenceObjectAltUnsafe[Vector[T]](targetVectorAddr)

	if srcInst.capacity != dstInst.capacity {
		panic("VectorSorted: capacity mismatch")
	}

	srcData := unsafe.Add(srcBase, srcInst.dataAddrOffset)
	dstData := unsafe.Add(dstBase, dstInst.dataAddrOffset)
	totalBytes := uintptr(srcInst.capacity) * uintptr(srcInst.itemSize)

	memcore.MemoryMoveNoHeapPointers(dstData, srcData, totalBytes)

	VectorSort[T](targetVectorAddr, cmp)
}

// ---------------------------------------------------- VECTOR VIEW

// VectorViewLengthGet returns the length of the current view.
//
//go:inline
func VectorViewLengthGet[T foundation.Numeric](vectorView VectorView[T]) uint64 {
	return ArrayViewLengthGet((ArrayView[T])(vectorView))
}

// VectorViewIsReadonly returns whether the current view is readonly.
//
//go:inline
func VectorViewIsReadonly[T foundation.Numeric](vectorView VectorView[T]) bool {
	return ArrayViewIsReadonly((ArrayView[T])(vectorView))
}

// VectorViewItemGetAt returns item at idx (as computed by view startIdx+relativeIdx)
//
//go:inline
func VectorViewItemGetAt[T foundation.Numeric](vectorView VectorView[T], relativeIdx uint64) (T, error) {
	return ArrayViewItemGetAt((ArrayView[T])(vectorView), relativeIdx)
}

// VectorViewItemPtrGetAt returns item pointer at idx (as computed by view startIdx+relativeIdx)
// Fails if the view is readonly (because getting a pointer would allow mutation)
//
//go:inline
func VectorViewItemPtrGetAt[T foundation.Numeric](vectorView VectorView[T], relativeIdx uint64) (*T, error) {
	return ArrayViewItemPtrGetAt((ArrayView[T])(vectorView), relativeIdx)
}

// VectorViewItemSetAt sets the item at idx (as computed by view startIdx+relativeIdx)
// Fails if the view is readonly.
//
//go:inline
func VectorViewItemSetAt[T foundation.Numeric](vectorView VectorView[T], relativeIdx uint64, v T) error {
	return ArrayViewItemSetAt((ArrayView[T])(vectorView), relativeIdx, v)
}

// VectorViewForEach calls a function for every element in the vector view.
// The indexes returned are the relative indexes.
//
//go:inline
func VectorViewForEach[T foundation.Numeric](vectorView VectorView[T], fn func(item T, idx uint64)) {
	ArrayViewForEach((ArrayView[T])(vectorView), fn)
}

// VectorViewForEachRaw calls a function for every element in the vector view.
// Not possible for readonly views.
// The indexes returned are the relative indexes.
//
//go:inline
func VectorViewForEachRaw[T foundation.Numeric](vectorView VectorView[T], fn func(ptr unsafe.Pointer, idx uint64)) error {
	return ArrayViewForEachRaw((ArrayView[T])(vectorView), fn)
}

// VectorViewStrideForEach calls a function for every element in the vector view.
// It visits every stride-th element.
// The indexes returned are the relative indexes.
//
//go:inline
func VectorViewStrideForEach[T foundation.Numeric](vectorView VectorView[T], fn func(item T, idx uint64), stride uint64) {
	ArrayViewStrideForEach((ArrayView[T])(vectorView), fn, stride)
}

// VectorViewStrideForEachRaw calls a function for every element in the vector view.
// It visits every stride-th element.
// Not possible for readonly views.
// The indexes returned are the relative indexes.
//
//go:inline
func VectorViewStrideForEachRaw[T foundation.Numeric](vectorView VectorView[T], fn func(ptr unsafe.Pointer, idx uint64), stride uint64) error {
	return ArrayViewStrideForEachRaw((ArrayView[T])(vectorView), fn, stride)
}

// VectorViewIterate allows you to iterate over the vector view efficiently.
// It provides a method to say which next element you need.
// The next function returns true when your requested n exceeds the view length.
// The indexes returned are the relative indexes.
//
//go:inline
func VectorViewIterate[T foundation.Numeric](
	vectorView VectorView[T],
	fn func(item T, idx uint64, next func(n uint64) (T, uint64, bool)),
) {
	ArrayViewIterate((ArrayView[T])(vectorView), fn)
}

// VectorViewIterateRaw allows you to iterate over the array view efficiently.
// It provides a method to say which next element you need.
// The next function returns true when your requested n exceeds the view length.
// Not possible for readonly views.
// The indexes returned are the relative indexes.
//
//go:inline
func VectorViewIterateRaw[T foundation.Numeric](
	vectorView VectorView[T],
	fn func(ptr unsafe.Pointer, idx uint64, next func(n uint64) (unsafe.Pointer, uint64, bool)),
) error {
	return ArrayViewIterateRaw((ArrayView[T])(vectorView), fn)
}

// VectorViewNestedGet creates a nested view inside an existing view.
// The indices [from:to) are relative to the current view.
// The readonly status is preserved.
//
//go:inline
func VectorViewNestedGet[T foundation.Numeric](vectorView VectorView[T], from, to uint64) VectorView[T] {
	return VectorView[T](ArrayViewNestedGet(ArrayView[T](vectorView), from, to))
}

// VectorUnaryReadOnlyOp is an operation that executes over a single element and does not mutate.
type VectorUnaryReadOnlyOp[T foundation.Numeric] func(item T)

// VectorUnaryOp is an operation that executes over a single element and mutates.
type VectorUnaryOp[TInput, TOutput foundation.Numeric] func(item TInput) TOutput

// VectorBinaryReadOnlyOp is an operation that executes over two elements at the same idx from different sources.
// It does not mutate.
type VectorBinaryReadOnlyOp[TInput1, TInput2 foundation.Numeric] func(itemA TInput1, itemB TInput2)

// VectorBinaryOp is an operation that executes over two elements at the same idx from different sources.
// It mutates.
type VectorBinaryOp[TInput1, TInput2, TOutput foundation.Numeric] func(itemA TInput1, itemB TInput2) TOutput

// VectorUnaryReadOnlyExecute executes a stride of unary readonly operations.
//
//go:inline
func VectorUnaryReadOnlyExecute[T foundation.Numeric](
	vectorAddr memcore.MarkRaw,
	op VectorUnaryReadOnlyOp[T],
	stride uint64,
) {
	base, inst := memcore.MemcoreMarkDereferenceObjectAlt[Vector[T]](vectorAddr)
	data := unsafe.Add(base, inst.dataAddrOffset)
	size := uintptr(inst.itemSize)
	cap := inst.capacity

	vectorUnrolledDispatch[T](cap, stride, func(idx uintptr) {
		v := *(*T)(unsafe.Add(data, idx*size))
		op(v)
	})
}

// VectorUnaryExecute executes a stride of unary mutating operations,
// writing results from the source vector into the destination vector.
//
//go:inline
func VectorUnaryExecute[T, P foundation.Numeric](
	srcAddr, dstAddr memcore.MarkRaw,
	op VectorUnaryOp[T, P],
	stride uint64,
) {
	srcBase, srcInst := memcore.MemcoreMarkDereferenceObjectAlt[Vector[T]](srcAddr)
	dstBase, dstInst := memcore.MemcoreMarkDereferenceObjectAlt[Vector[P]](dstAddr)

	if srcInst.capacity != dstInst.capacity {
		panic(fmt.Errorf("cannot perform unary op: capacity mismatch (src=%d, dst=%d)",
			srcInst.capacity, dstInst.capacity))
	}

	srcData := unsafe.Add(srcBase, srcInst.dataAddrOffset)
	dstData := unsafe.Add(dstBase, dstInst.dataAddrOffset)
	srcSize := uintptr(srcInst.itemSize)
	dstSize := uintptr(dstInst.itemSize)
	capacity := srcInst.capacity

	vectorUnrolledDispatch[T](capacity, stride, func(idx uintptr) {
		srcV := *(*T)(unsafe.Add(srcData, idx*srcSize))
		res := op(srcV)
		*(*P)(unsafe.Add(dstData, idx*dstSize)) = res
	})
}

// VectorBinaryReadOnlyExecute executes a stride of binary read-only operations
// between two vectors of equal capacity.
//
//go:inline
func VectorBinaryReadOnlyExecute[T, U foundation.Numeric](
	vectorAAddr, vectorBAddr memcore.MarkRaw,
	op VectorBinaryReadOnlyOp[T, U],
	stride uint64,
) {
	aBase, aInst := memcore.MemcoreMarkDereferenceObjectAlt[Vector[T]](vectorAAddr)
	bBase, bInst := memcore.MemcoreMarkDereferenceObjectAlt[Vector[U]](vectorBAddr)

	if aInst.capacity != bInst.capacity {
		panic(fmt.Errorf("cannot perform binary op: capacity mismatch (a=%d, b=%d)",
			aInst.capacity, bInst.capacity))
	}

	aData := unsafe.Add(aBase, aInst.dataAddrOffset)
	bData := unsafe.Add(bBase, bInst.dataAddrOffset)
	aSize := uintptr(aInst.itemSize)
	bSize := uintptr(bInst.itemSize)
	capacity := aInst.capacity

	vectorUnrolledDispatch[T](capacity, stride, func(idx uintptr) {
		aVal := *(*T)(unsafe.Add(aData, idx*aSize))
		bVal := *(*U)(unsafe.Add(bData, idx*bSize))
		op(aVal, bVal)
	})
}

// VectorBinaryExecute executes a stride of binary mutating operations
// between two source vectors and writes results into a destination vector.
//
//go:inline
func VectorBinaryExecute[T, U, P foundation.Numeric](
	vectorAAddr, vectorBAddr, destAddr memcore.MarkRaw,
	op VectorBinaryOp[T, U, P],
	stride uint64,
) {
	aBase, aInst := memcore.MemcoreMarkDereferenceObjectAlt[Vector[T]](vectorAAddr)
	bBase, bInst := memcore.MemcoreMarkDereferenceObjectAlt[Vector[U]](vectorBAddr)
	dBase, dInst := memcore.MemcoreMarkDereferenceObjectAlt[Vector[P]](destAddr)

	if aInst.capacity != bInst.capacity || aInst.capacity != dInst.capacity {
		panic(fmt.Errorf("cannot perform binary op: capacity mismatch (a=%d, b=%d, d=%d)",
			aInst.capacity, bInst.capacity, dInst.capacity))
	}

	aData := unsafe.Add(aBase, aInst.dataAddrOffset)
	bData := unsafe.Add(bBase, bInst.dataAddrOffset)
	dData := unsafe.Add(dBase, dInst.dataAddrOffset)

	aSize := uintptr(aInst.itemSize)
	bSize := uintptr(bInst.itemSize)
	dSize := uintptr(dInst.itemSize)
	capacity := aInst.capacity

	vectorUnrolledDispatch[T](capacity, stride, func(idx uintptr) {
		aVal := *(*T)(unsafe.Add(aData, idx*aSize))
		bVal := *(*U)(unsafe.Add(bData, idx*bSize))
		*(*P)(unsafe.Add(dData, idx*dSize)) = op(aVal, bVal)
	})
}

//go:inline
func vectorUnrolledDispatch[T foundation.Numeric](
	capacity uint64,
	stride uint64,
	apply func(idx uintptr),
) {
	switch stride {
	case 8:
		vectorUnrolledStride8[T](capacity, apply)
	case 4:
		vectorUnrolledStride4[T](capacity, apply)
	case 2:
		vectorUnrolledStride2[T](capacity, apply)
	case 1:
		vectorUnrolledStride1[T](capacity, apply)
	default:
		vectorUnrolledGeneric[T](capacity, stride, apply)
	}
}

//go:inline
func vectorUnrolledStride8[T foundation.Numeric](
	capacity uint64,
	apply func(idx uintptr),
) {
	var i uint64
	for ; i+7 < capacity; i += 8 {
		apply(uintptr(i + 0))
		apply(uintptr(i + 1))
		apply(uintptr(i + 2))
		apply(uintptr(i + 3))
		apply(uintptr(i + 4))
		apply(uintptr(i + 5))
		apply(uintptr(i + 6))
		apply(uintptr(i + 7))
	}

	for ; i < capacity; i++ {
		apply(uintptr(i))
	}
}

//go:inline
func vectorUnrolledStride4[T foundation.Numeric](capacity uint64, apply func(idx uintptr)) {
	var i uint64
	for ; i+3 < capacity; i += 4 {
		apply(uintptr(i + 0))
		apply(uintptr(i + 1))
		apply(uintptr(i + 2))
		apply(uintptr(i + 3))
	}
	for ; i < capacity; i++ {
		apply(uintptr(i))
	}
}

//go:inline
func vectorUnrolledStride2[T foundation.Numeric](capacity uint64, apply func(idx uintptr)) {
	var i uint64
	for ; i+1 < capacity; i += 2 {
		apply(uintptr(i + 0))
		apply(uintptr(i + 1))
	}
	for ; i < capacity; i++ {
		apply(uintptr(i))
	}
}

//go:inline
func vectorUnrolledStride1[T foundation.Numeric](capacity uint64, apply func(idx uintptr)) {
	for i := uint64(0); i < capacity; i++ {
		apply(uintptr(i))
	}
}

//go:inline
func vectorUnrolledGeneric[T foundation.Numeric](capacity uint64, stride uint64, apply func(idx uintptr)) {
	var i uint64
	for ; i+stride <= capacity; i += stride {
		for j := uint64(0); j < stride; j++ {
			apply(uintptr(i + j))
		}
	}
	for ; i < capacity; i++ {
		apply(uintptr(i))
	}
}
