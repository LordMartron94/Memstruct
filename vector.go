package memstruct

import (
	"fmt"
	"foundation"
	"memcore"
	"strings"
	"unsafe"
)

func VectorRequiredBytesGet[T foundation.Numeric](capacity uint64) uint64 {
	headerSize := memcore.SizeOf[Vector[T]]()
	itemSize := memcore.SizeOf[T]()
	return headerSize + itemSize*capacity
}

func VectorRequiredAlignmentGet[T foundation.Numeric]() uint64 {
	return max(memcore.AlignOf[T](), memcore.AlignOf[Vector[T]]())
}

// Vector is a custom vector implementation built on top of memcore.
type Vector[T foundation.Numeric] struct {
	dataAddrOffset uintptr
	capacity       uint64

	setFnID memcore.FunctionID

	itemSize        uint64
	itemSizeUintPtr uintptr
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
		val := *(*T)(unsafe.Add(dataAddr, uintptr(i*v.itemSize)))
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
	headerSize := memcore.SizeOf[Vector[T]]()

	itemSize := memcore.SizeOf[T]()
	vectorPtr := memcore.MemcoreMarkDereferenceObject[Vector[T]](vectorAddr)
	*vectorPtr = Vector[T]{
		dataAddrOffset:  uintptr(headerSize),
		capacity:        capacity,
		itemSize:        itemSize,
		itemSizeUintPtr: uintptr(itemSize),
	}

	vectorPtr.setFnID = memcore.MemcoreFunctionRegisterTyped(
		getMovementFunc[T](itemSize),
	)
}

// VectorSnapshotCreate creates a deep copy of an vector at a new memory location
// defined by the destination pointer (which points to the start of the new vector header).
// It copies both the header and the data that follow it, maintaining the same relative layout.
func VectorSnapshotCreate[T foundation.Numeric](dest memcore.MarkRaw, instance memcore.MarkRaw) memcore.MarkRaw {
	vectorPtr := memcore.MemcoreMarkDereferenceObject[Vector[T]](instance)
	totalSize := VectorRequiredBytesGet[T](vectorPtr.capacity)

	srcAddr := memcore.MemcoreMarkDereference(instance)
	dstAddr := memcore.MemcoreMarkDereference(dest)

	memcore.MemoryMoveNoHeapPointers(dstAddr, srcAddr, uintptr(totalSize))

	return dest
}

// VectorSnapshotRestore replaces the entire memory block of one vector
// (header + data) with that of another vector of the same type and capacity.
// Both vectors must live in manual memory managed by memcore.
func VectorSnapshotRestore[T foundation.Numeric](dest, src memcore.MarkRaw) error {
	dstHeader := memcore.MemcoreMarkDereferenceObject[Vector[T]](dest)
	srcHeader := memcore.MemcoreMarkDereferenceObject[Vector[T]](src)

	if dstHeader.capacity != srcHeader.capacity {
		return fmt.Errorf("cannot restore snapshot: unequal capacities (dest=%v, src=%v)", dstHeader.capacity, srcHeader.capacity)
	}

	if dest == src {
		return nil
	}

	totalBytes := VectorRequiredBytesGet[T](dstHeader.capacity)

	dstAddr := memcore.MemcoreMarkDereference(dest)
	srcAddr := memcore.MemcoreMarkDereference(src)

	memcore.MemoryMoveNoHeapPointers(dstAddr, srcAddr, uintptr(totalBytes))

	return nil
}

// VectorCapacityGet returns the total amount of elements that can be stored.
//
//go:nosplit
//go:inline
func VectorCapacityGet[T foundation.Numeric](vector memcore.MarkRaw) uint64 {
	instance := memcore.MemcoreMarkDereferenceObject[Vector[T]](vector)
	return instance.capacity
}

// VectorItemGetAt returns T at idx within the vector.
// It returns an error if the idx is invalid.
//
//go:nosplit
//go:inline
func VectorItemGetAt[T foundation.Numeric](vector memcore.MarkRaw, idx uint64) (T, error) {
	instance := memcore.MemcoreMarkDereferenceObject[Vector[T]](vector)
	baseAddr := memcore.MemcoreMarkDereference(vector)

	if error := vectorGuaranteeIdxValidity(instance, idx); error != nil {
		var zero T
		return zero, error
	}

	return *(*T)(vectorGetPtrAtIdx(instance, baseAddr, idx)), nil
}

// VectorItemGetAtUnsafe returns T at idx within the vector.
// It does no bounds checks.
//
//go:inline
func VectorItemGetAtUnsafe[T foundation.Numeric](vector memcore.MarkRaw, idx uint64) T {
	instance := memcore.MemcoreMarkDereferenceObject[Vector[T]](vector)
	baseAddr := memcore.MemcoreMarkDereference(vector)
	return *(*T)(vectorGetPtrAtIdx(instance, baseAddr, idx))
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
	instance := memcore.MemcoreMarkDereferenceObject[Vector[T]](vector)
	baseAddr := memcore.MemcoreMarkDereference(vector)
	if error := vectorGuaranteeIdxValidity(instance, idx); error != nil {
		return nil, error
	}

	return (*T)(vectorGetPtrAtIdx(instance, baseAddr, idx)), nil
}

// VectorItemPtrGetAtUnsafe returns a pointer to T at idx within the vector.
// It does no bounds checks.
//
// Using this pointer after deletion or overwriting this idx is undefined behaviour.
// Use at your own discretion!
//
//go:inline
func VectorItemPtrGetAtUnsafe[T foundation.Numeric](vector memcore.MarkRaw, idx uint64) *T {
	instance := memcore.MemcoreMarkDereferenceObject[Vector[T]](vector)
	baseAddr := memcore.MemcoreMarkDereference(vector)
	return (*T)(vectorGetPtrAtIdx(instance, baseAddr, idx))
}

// VectorSetAt sets idx of vector to value T.
// It returns an error if the idx is invalid.
//
//go:nosplit
//go:inline
func VectorSetAt[T foundation.Numeric](vector memcore.MarkRaw, idx uint64, value T) error {
	instance := memcore.MemcoreMarkDereferenceObject[Vector[T]](vector)
	if error := vectorGuaranteeIdxValidity(instance, idx); error != nil {
		return error
	}

	baseAddr := memcore.MemcoreMarkDereference(vector)
	itemPtr := vectorGetPtrAtIdx(instance, baseAddr, idx)

	memcore.MemcoreFunctionRetrieveTyped[setFn[T]](instance.setFnID)(itemPtr, value)

	return nil
}

// VectorSetAtUnsafe sets idx of vector to value T.
// It does no bounds checks.
//
//go:nosplit
//go:inline
func VectorSetAtUnsafe[T foundation.Numeric](vector memcore.MarkRaw, idx uint64, value T) {
	instance := memcore.MemcoreMarkDereferenceObject[Vector[T]](vector)

	baseAddr := memcore.MemcoreMarkDereference(vector)
	itemPtr := vectorGetPtrAtIdx(instance, baseAddr, idx)

	memcore.MemcoreFunctionRetrieveTyped[setFn[T]](instance.setFnID)(itemPtr, value)
}

// VectorReplaceInternal replaces srcIdx with the value at destIdx efficiently.
//
//go:inline
func VectorReplaceInternal[T foundation.Numeric](vector memcore.MarkRaw, srcIdx, destIdx uint64) error {
	instance := memcore.MemcoreMarkDereferenceObject[Vector[T]](vector)
	if error := vectorGuaranteeIdxValidity(instance, srcIdx); error != nil {
		return error
	}

	if error := vectorGuaranteeIdxValidity(instance, destIdx); error != nil {
		return error
	}

	baseAddr := memcore.MemcoreMarkDereference(vector)

	srcPtr := vectorGetPtrAtIdx(instance, baseAddr, srcIdx)
	dstPtr := vectorGetPtrAtIdx(instance, baseAddr, destIdx)

	memcore.MemoryMoveNoHeapPointers(dstPtr, srcPtr, instance.itemSizeUintPtr)

	return nil
}

// VectorReplaceInternalUnsafe replaces srcIdx with the value at destIdx efficiently.
//
// It does no bounds checks.
//
//go:inline
func VectorReplaceInternalUnsafe[T foundation.Numeric](vector memcore.MarkRaw, srcIdx, destIdx uint64) {
	instance := memcore.MemcoreMarkDereferenceObject[Vector[T]](vector)
	baseAddr := memcore.MemcoreMarkDereference(vector)
	srcPtr := vectorGetPtrAtIdx(instance, baseAddr, srcIdx)
	dstPtr := vectorGetPtrAtIdx(instance, baseAddr, destIdx)

	memcore.MemoryMoveNoHeapPointers(dstPtr, srcPtr, instance.itemSizeUintPtr)
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
	instance := memcore.MemcoreMarkDereferenceObject[Vector[T]](vector)
	if from >= instance.capacity || to >= instance.capacity {
		return fmt.Errorf("invalid range: from=%d to=%d capacity=%d", from, to, instance.capacity)
	}
	if count == 0 || from >= to {
		return nil
	}

	VectorShiftRightUnsafe[T](vector, from, to, count)
	return nil
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
	instance := memcore.MemcoreMarkDereferenceObject[Vector[T]](vector)
	baseAddr := memcore.MemcoreMarkDereference(vector)
	elemSize := instance.itemSizeUintPtr
	srcPtr := vectorGetPtrAtIdx(instance, baseAddr, from)
	dstPtr := vectorGetPtrAtIdx(instance, baseAddr, from+count)

	memcore.MemoryMoveNoHeapPointers(dstPtr, srcPtr, uintptr((to-from+1)*uint64(elemSize)))
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
	instance := memcore.MemcoreMarkDereferenceObject[Vector[T]](vector)
	if from >= instance.capacity || to >= instance.capacity {
		return fmt.Errorf("invalid range: from=%d to=%d capacity=%d", from, to, instance.capacity)
	}
	if count == 0 || from >= to {
		return nil
	}

	VectorShiftLeftUnsafe[T](vector, from, to, count)
	return nil
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
	instance := memcore.MemcoreMarkDereferenceObject[Vector[T]](vector)
	baseAddr := memcore.MemcoreMarkDereference(vector)

	elemSize := instance.itemSizeUintPtr
	srcPtr := vectorGetPtrAtIdx(instance, baseAddr, from+count)
	dstPtr := vectorGetPtrAtIdx(instance, baseAddr, from)

	memcore.MemoryMoveNoHeapPointers(dstPtr, srcPtr, uintptr((to-from+1)*uint64(elemSize)))
}

// VectorDeleteAt resets memory to 0 at a given index, using pointers to this
// index gotten earlier is undefined behaviour.
// It returns an error if the index is invalid.
//
//go:nosplit
//go:inline
func VectorDeleteAt[T foundation.Numeric](vector memcore.MarkRaw, idx uint64) error {
	instance := memcore.MemcoreMarkDereferenceObject[Vector[T]](vector)
	if error := vectorGuaranteeIdxValidity(instance, idx); error != nil {
		return error
	}

	baseAddr := memcore.MemcoreMarkDereference(vector)
	currentPtr := vectorGetPtrAtIdx(instance, baseAddr, idx)
	memcore.MemoryClearNoHeapPointers(currentPtr, uintptr(instance.itemSize))
	return nil
}

// VectorDeleteAtUnsafe resets memory to 0 at a given index, using pointers to this
// index gotten earlier is undefined behaviour.
// It does no bounds checks.
//
//go:nosplit
//go:inline
func VectorDeleteAtUnsafe[T foundation.Numeric](vector memcore.MarkRaw, idx uint64) {
	instance := memcore.MemcoreMarkDereferenceObject[Vector[T]](vector)
	baseAddr := memcore.MemcoreMarkDereference(vector)
	currentPtr := vectorGetPtrAtIdx(instance, baseAddr, idx)
	memcore.MemoryClearNoHeapPointers(currentPtr, uintptr(instance.itemSize))
}

// VectorClear resets the entire vector's memory to 0, allowing it to be reused.
// Do NOT use pointers to previous vector items after this as that is undefined behaviour.
//
//go:nosplit
//go:inline
func VectorClear[T foundation.Numeric](vector memcore.MarkRaw) {
	instance := memcore.MemcoreMarkDereferenceObject[Vector[T]](vector)
	baseAddr := memcore.MemcoreMarkDereference(vector)
	memcore.MemoryClearNoHeapPointers(vectorComputeDataAddr(instance, baseAddr), uintptr(instance.capacity)*uintptr(instance.itemSize))
}

// VectorIsIdxValid checks whether the given index is valid.
//
//go:inline
func VectorIsIdxValid[T foundation.Numeric](vector memcore.MarkRaw, idx uint64) bool {
	instance := memcore.MemcoreMarkDereferenceObject[Vector[T]](vector)
	return idx < instance.capacity
}

// VectorSum computes the linear sum of the vector’s elements.
func VectorSum[T foundation.Numeric](vector memcore.MarkRaw) float64 {
	baseAddr, instance := memcore.MemcoreMarkDereferenceObjectAlt[Vector[T]](vector)
	return vectorComputeSum(instance, baseAddr)
}

// VectorSumSquared computes the sum of squares (used in magnitude calculation).
func VectorSumSquared[T foundation.Numeric](vector memcore.MarkRaw) float64 {
	baseAddr, instance := memcore.MemcoreMarkDereferenceObjectAlt[Vector[T]](vector)
	return vectorComputeSquaredSum(instance, baseAddr)
}

// VectorMagnitudeF32 computes the magnitude of a given vector in float32 precision.
func VectorMagnitudeF32[T foundation.Numeric](vector memcore.MarkRaw) float32 {
	baseAddr, instance := memcore.MemcoreMarkDereferenceObjectAlt[Vector[T]](vector)
	sqrSum := vectorComputeSquaredSum(instance, baseAddr)
	sqrRoot := foundation.Sqrt32(sqrSum)
	return sqrRoot
}

// VectorMagnitudeF64 computes the magnitude of a given vector in float64 precision.
func VectorMagnitudeF64[T foundation.Numeric](vector memcore.MarkRaw) float64 {
	baseAddr, instance := memcore.MemcoreMarkDereferenceObjectAlt[Vector[T]](vector)
	sqrSum := vectorComputeSquaredSum(instance, baseAddr)
	sqrRoot := foundation.Sqrt64(sqrSum)
	return sqrRoot
}

// VectorNormalizedF32 normalizes the vector with float32 precision such that its magnitude is 1.
func VectorNormalizedF32[T foundation.Numeric](
	currentVector memcore.MarkRaw,
	newVectorAddr memcore.MarkRaw,
) {
	magnitude := VectorMagnitudeF32[T](currentVector)
	inv := 1.0 / magnitude
	vectorScale[T, float32, float32](currentVector, newVectorAddr, inv)
}

// VectorNormalizedF64 normalizes the vector with float64 precision such that its magnitude is 1.
func VectorNormalizedF64[T foundation.Numeric](
	currentVector memcore.MarkRaw,
	newVectorAddr memcore.MarkRaw,
) {
	magnitude := VectorMagnitudeF64[T](currentVector)
	inv := 1.0 / magnitude
	vectorScale[T, float64, float64](currentVector, newVectorAddr, inv)
}

// VectorDotProductF32 computes the dot product between two vectors in float32 precision.
// T is the datatype of vector A, and U is the datatype of vector B.
func VectorDotProductF32[T, U foundation.Numeric](vectorAAddr, vectorBAddr memcore.MarkRaw) float32 {
	return vectorDotProduct[T, U, float32](vectorAAddr, vectorBAddr)
}

// VectorDotProductF64 computes the dot product between two vectors in float64 precision.
// T is the datatype of vector A, and U is the datatype of vector B.
func VectorDotProductF64[T, U foundation.Numeric](vectorAAddr, vectorBAddr memcore.MarkRaw) float64 {
	return vectorDotProduct[T, U, float64](vectorAAddr, vectorBAddr)
}

// VectorScaleF32 scales the vector with float32 precision such each value is multiplied by the scalar.
func VectorScaleF32[T, S foundation.Numeric](
	currentVectorAddr, newVectorAddr memcore.MarkRaw,
	scalar S,
) {
	vectorScale[T, S, float32](currentVectorAddr, newVectorAddr, scalar)
}

// VectorScaleF64 scales the vector with float64 precision such each value is multiplied by the scalar.
func VectorScaleF64[T, S foundation.Numeric](
	currentVectorAddr, newVectorAddr memcore.MarkRaw,
	scalar S,
) {
	vectorScale[T, S, float64](currentVectorAddr, newVectorAddr, scalar)
}

// VectorClamp transforms the vector in such a way that each element is between
// min and max.
func VectorClamp[T foundation.Numeric](
	vector memcore.MarkRaw,
	min, max T,
) {
	vectorClamp(vector, min, max)
}

// VectorAddF32 adds the values of Vector B to Vector A, resulting in Vector C at newVectorAddr.
// It does so in float32 precision.
func VectorAddF32[T, U foundation.Numeric](
	vectorAAddr, vectorBAddr, newVectorAddr memcore.MarkRaw,
) {
	vectorAdd[T, U, float32](vectorAAddr, vectorBAddr, newVectorAddr)
}

// VectorAddF64 adds the values of Vector B to Vector A, resulting in Vector C at newVectorAddr.
// It does so in float64 precision.
func VectorAddF64[T, U foundation.Numeric](
	vectorAAddr, vectorBAddr, newVectorAddr memcore.MarkRaw,
) {
	vectorAdd[T, U, float64](vectorAAddr, vectorBAddr, newVectorAddr)
}

// -------------------------- PRIVATE HELPERS

//go:inline
func vectorAdd[T, U foundation.Numeric, P ~float32 | ~float64](
	vectorAAddr, vectorBAddr, newVectorAddr memcore.MarkRaw,
) {
	srcBase, srcInstance := memcore.MemcoreMarkDereferenceObjectAlt[Vector[T]](vectorAAddr)
	addBase, addInstance := memcore.MemcoreMarkDereferenceObjectAlt[Vector[U]](vectorBAddr)
	dstBase, dstInstance := memcore.MemcoreMarkDereferenceObjectAlt[Vector[P]](newVectorAddr)

	if !(srcInstance.capacity == dstInstance.capacity && srcInstance.capacity == addInstance.capacity && dstInstance.capacity == addInstance.capacity) {
		panic(fmt.Errorf("cannot add vectors with different capacities (src=%d,add=%d,dest=%d)", srcInstance.capacity, addInstance.capacity, dstInstance.capacity))
	}

	capacity := srcInstance.capacity
	srcItemSize := uintptr(srcInstance.itemSize)
	addItemSize := uintptr(addInstance.itemSize)
	dstItemSize := uintptr(dstInstance.itemSize)

	srcData := vectorComputeDataAddr(srcInstance, srcBase)
	addData := vectorComputeDataAddr(addInstance, addBase)
	dstData := vectorComputeDataAddr(dstInstance, dstBase)

	i := uint64(0)
	for ; i+7 < capacity; i += 8 {
		vectorAddAtIdx[T, U, P](srcData, addData, dstData, srcItemSize, addItemSize, dstItemSize, uintptr(i+0))
		vectorAddAtIdx[T, U, P](srcData, addData, dstData, srcItemSize, addItemSize, dstItemSize, uintptr(i+1))
		vectorAddAtIdx[T, U, P](srcData, addData, dstData, srcItemSize, addItemSize, dstItemSize, uintptr(i+2))
		vectorAddAtIdx[T, U, P](srcData, addData, dstData, srcItemSize, addItemSize, dstItemSize, uintptr(i+3))
		vectorAddAtIdx[T, U, P](srcData, addData, dstData, srcItemSize, addItemSize, dstItemSize, uintptr(i+4))
		vectorAddAtIdx[T, U, P](srcData, addData, dstData, srcItemSize, addItemSize, dstItemSize, uintptr(i+5))
		vectorAddAtIdx[T, U, P](srcData, addData, dstData, srcItemSize, addItemSize, dstItemSize, uintptr(i+6))
		vectorAddAtIdx[T, U, P](srcData, addData, dstData, srcItemSize, addItemSize, dstItemSize, uintptr(i+7))
	}

	for ; i < capacity; i++ {
		vectorAddAtIdx[T, U, P](srcData, addData, dstData, srcItemSize, addItemSize, dstItemSize, uintptr(i))
	}
}

//go:inline
//go:nosplit
func vectorAddAtIdx[T, U foundation.Numeric, P ~float32 | ~float64](
	srcData, addData, dstData unsafe.Pointer,
	srcItemSize, addItemSize, dstItemSize uintptr,
	idx uintptr,
) {
	v := P(float64(*(*T)(unsafe.Add(srcData, idx*srcItemSize))) + float64(*(*U)(unsafe.Add(addData, idx*addItemSize))))
	*(*P)(unsafe.Add(dstData, idx*dstItemSize)) = v
}

//go:inline
func vectorClamp[T foundation.Numeric](vectorAddr memcore.MarkRaw, min, max T) {
	vectorBase, vectorInstance := memcore.MemcoreMarkDereferenceObjectAlt[Vector[T]](vectorAddr)
	capacity := vectorInstance.capacity

	vectorData := vectorComputeDataAddr(vectorInstance, vectorBase)
	vectorItemSize := uintptr(vectorInstance.itemSize)

	i := uint64(0)
	for ; i+7 < capacity; i += 8 {
		vectorClampAtIdx(vectorData, vectorItemSize, uintptr(i+0), min, max)
		vectorClampAtIdx(vectorData, vectorItemSize, uintptr(i+1), min, max)
		vectorClampAtIdx(vectorData, vectorItemSize, uintptr(i+2), min, max)
		vectorClampAtIdx(vectorData, vectorItemSize, uintptr(i+3), min, max)
		vectorClampAtIdx(vectorData, vectorItemSize, uintptr(i+4), min, max)
		vectorClampAtIdx(vectorData, vectorItemSize, uintptr(i+5), min, max)
		vectorClampAtIdx(vectorData, vectorItemSize, uintptr(i+6), min, max)
		vectorClampAtIdx(vectorData, vectorItemSize, uintptr(i+7), min, max)
	}

	for ; i < capacity; i++ {
		vectorClampAtIdx(vectorData, vectorItemSize, uintptr(i), min, max)
	}
}

//go:inline
//go:nosplit
func vectorClampAtIdx[T foundation.Numeric](
	vectorData unsafe.Pointer,
	vectorItemSize uintptr,
	idx uintptr,
	min, max T,
) {
	itemPtr := unsafe.Add(vectorData, idx*vectorItemSize)
	val := *(*T)(itemPtr)

	if val < min {
		val = min
	} else if val > max {
		val = max
	}

	*(*T)(itemPtr) = val
}

//go:inline
func vectorScale[T, S foundation.Numeric, P ~float32 | ~float64](
	vectorAddr memcore.MarkRaw, newVectorAddr memcore.MarkRaw,
	scalar S,
) {
	srcBase, srcInstance := memcore.MemcoreMarkDereferenceObjectAlt[Vector[T]](vectorAddr)
	dstBase, dstInstance := memcore.MemcoreMarkDereferenceObjectAlt[Vector[P]](newVectorAddr)

	if srcInstance.capacity != dstInstance.capacity {
		panic(fmt.Errorf("cannot scale vectors with different capacities (src=%d,dest=%d)", srcInstance.capacity, dstInstance.capacity))
	}

	capacity := srcInstance.capacity
	srcItemSize := uintptr(srcInstance.itemSize)
	dstItemSize := uintptr(dstInstance.itemSize)

	srcData := vectorComputeDataAddr(srcInstance, srcBase)
	dstData := vectorComputeDataAddr(dstInstance, dstBase)

	i := uint64(0)
	for ; i+7 < capacity; i += 8 {
		vectorScaleAtIdx[T, S, P](srcData, dstData, srcItemSize, dstItemSize, uintptr(i+0), scalar)
		vectorScaleAtIdx[T, S, P](srcData, dstData, srcItemSize, dstItemSize, uintptr(i+1), scalar)
		vectorScaleAtIdx[T, S, P](srcData, dstData, srcItemSize, dstItemSize, uintptr(i+2), scalar)
		vectorScaleAtIdx[T, S, P](srcData, dstData, srcItemSize, dstItemSize, uintptr(i+3), scalar)
		vectorScaleAtIdx[T, S, P](srcData, dstData, srcItemSize, dstItemSize, uintptr(i+4), scalar)
		vectorScaleAtIdx[T, S, P](srcData, dstData, srcItemSize, dstItemSize, uintptr(i+5), scalar)
		vectorScaleAtIdx[T, S, P](srcData, dstData, srcItemSize, dstItemSize, uintptr(i+6), scalar)
		vectorScaleAtIdx[T, S, P](srcData, dstData, srcItemSize, dstItemSize, uintptr(i+7), scalar)
	}

	for ; i < capacity; i++ {
		vectorScaleAtIdx[T, S, P](srcData, dstData, srcItemSize, dstItemSize, uintptr(i), scalar)
	}
}

//go:inline
//go:nosplit
func vectorScaleAtIdx[T, S foundation.Numeric, P ~float32 | ~float64](
	srcData, dstData unsafe.Pointer,
	srcItemSize, dstItemSize uintptr,
	idx uintptr,
	scalar S,
) {
	v := P(float64(*(*T)(unsafe.Add(srcData, idx*srcItemSize))) * float64(scalar))
	*(*P)(unsafe.Add(dstData, idx*dstItemSize)) = v
}

//go:inline
func vectorDotProduct[T, U foundation.Numeric, P ~float32 | ~float64](
	vectorAAddr, vectorBAddr memcore.MarkRaw,
) P {
	vectorABaseAddr, vectorA := memcore.MemcoreMarkDereferenceObjectAlt[Vector[T]](vectorAAddr)
	vectorBBaseAddr, vectorB := memcore.MemcoreMarkDereferenceObjectAlt[Vector[U]](vectorBAddr)

	if vectorA.capacity != vectorB.capacity {
		panic(fmt.Errorf("cannot compute dot product of vectors with different capacities (a=%d,b=%d)", vectorA.capacity, vectorB.capacity))
	}

	capacity := vectorA.capacity

	vectorADataAddr := vectorComputeDataAddr(vectorA, vectorABaseAddr)
	vectorBDataAddr := vectorComputeDataAddr(vectorB, vectorBBaseAddr)

	vectorAItemSize := uintptr(vectorA.itemSize)
	vectorBItemSize := uintptr(vectorB.itemSize)

	dotProduct := float64(0)

	// Manually unrolled loop for performance benefits.
	i := uint64(0)
	for ; i+7 < capacity; i += 8 {
		dotProduct += vectorComputeDotProductAtIdx[T, U](vectorADataAddr, vectorBDataAddr, vectorAItemSize, vectorBItemSize, uintptr(i+0))
		dotProduct += vectorComputeDotProductAtIdx[T, U](vectorADataAddr, vectorBDataAddr, vectorAItemSize, vectorBItemSize, uintptr(i+1))
		dotProduct += vectorComputeDotProductAtIdx[T, U](vectorADataAddr, vectorBDataAddr, vectorAItemSize, vectorBItemSize, uintptr(i+2))
		dotProduct += vectorComputeDotProductAtIdx[T, U](vectorADataAddr, vectorBDataAddr, vectorAItemSize, vectorBItemSize, uintptr(i+3))
		dotProduct += vectorComputeDotProductAtIdx[T, U](vectorADataAddr, vectorBDataAddr, vectorAItemSize, vectorBItemSize, uintptr(i+4))
		dotProduct += vectorComputeDotProductAtIdx[T, U](vectorADataAddr, vectorBDataAddr, vectorAItemSize, vectorBItemSize, uintptr(i+5))
		dotProduct += vectorComputeDotProductAtIdx[T, U](vectorADataAddr, vectorBDataAddr, vectorAItemSize, vectorBItemSize, uintptr(i+6))
		dotProduct += vectorComputeDotProductAtIdx[T, U](vectorADataAddr, vectorBDataAddr, vectorAItemSize, vectorBItemSize, uintptr(i+7))
	}

	for ; i < capacity; i++ {
		dotProduct += vectorComputeDotProductAtIdx[T, U](vectorADataAddr, vectorBDataAddr, vectorAItemSize, vectorBItemSize, uintptr(i))
	}

	return P(dotProduct)
}

//go:inline
//go:nosplit
func vectorComputeDotProductAtIdx[T, U foundation.Numeric](
	vectorADataAddr, vectorBDataAddr unsafe.Pointer,
	vectorAItemSize, vectorBItemSize uintptr,
	idx uintptr,
) float64 {
	a := float64(*(*T)(unsafe.Add(vectorADataAddr, idx*vectorAItemSize)))
	b := float64(*(*U)(unsafe.Add(vectorBDataAddr, idx*vectorBItemSize)))
	return a * b
}

//go:inline
func vectorComputeSum[T foundation.Numeric](instance *Vector[T], baseAddr unsafe.Pointer) float64 {
	sum := float64(0)
	for i := uint64(0); i < instance.capacity; i++ {
		v := *(*T)(vectorGetPtrAtIdx(instance, baseAddr, i))
		sum += float64(v)
	}

	return sum
}

//go:inline
func vectorComputeSquaredSum[T foundation.Numeric](instance *Vector[T], baseAddr unsafe.Pointer) float64 {
	sum := float64(0)
	for i := uint64(0); i < instance.capacity; i++ {
		v := *(*T)(vectorGetPtrAtIdx(instance, baseAddr, i))
		sum += float64(v) * float64(v)
	}

	return sum
}

//go:inline
func vectorGetPtrAtIdx[T foundation.Numeric](instance *Vector[T], baseAddr unsafe.Pointer, idx uint64) unsafe.Pointer {
	return unsafe.Add(vectorComputeDataAddr(instance, baseAddr), idx*instance.itemSize)
}

//go:inline
func vectorGuaranteeIdxValidity[T foundation.Numeric](instance *Vector[T], idx uint64) error {
	idxValid := idx < instance.capacity

	if !idxValid {
		return fmt.Errorf("invalid index: %v, must be between 0 and %v (exclusive)", idx, instance.capacity)
	}

	return nil
}

//go:inline
func vectorComputeDataAddr[T foundation.Numeric](instance *Vector[T], baseAddr unsafe.Pointer) unsafe.Pointer {
	return unsafe.Add(baseAddr, instance.dataAddrOffset)
}
