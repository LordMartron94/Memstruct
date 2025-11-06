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

// VectorSumF32 computes the linear sum of the vector’s elements in float32 precision.
func VectorSumF32[T foundation.Numeric](vector memcore.MarkRaw) float32 {
	sum := float32(0)

	vectorUnaryReadOnlyExecute(vector, func(a T) {
		sum += float32(a)
	})

	return sum
}

// VectorSumF64 computes the linear sum of the vector’s elements in float64 precision.
func VectorSumF64[T foundation.Numeric](vector memcore.MarkRaw) float64 {
	sum := float64(0)

	vectorUnaryReadOnlyExecute(vector, func(a T) {
		sum += float64(a)
	})

	return sum
}

// VectorSumSquaredF32 computes the sum of squares (used in magnitude calculation) in float32 precision.
func VectorSumSquaredF32[T foundation.Numeric](vector memcore.MarkRaw) float32 {
	sqrSum := float32(0)

	vectorUnaryReadOnlyExecute(vector, func(a T) {
		sqrSum += float32(a) * float32(a)
	})

	return sqrSum
}

// VectorSumSquaredF64 computes the sum of squares (used in magnitude calculation) in float64 precision.
func VectorSumSquaredF64[T foundation.Numeric](vector memcore.MarkRaw) float64 {
	sqrSum := float64(0)

	vectorUnaryReadOnlyExecute(vector, func(a T) {
		sqrSum += float64(a) * float64(a)
	})

	return sqrSum
}

// VectorMagnitudeF32 computes the magnitude of a given vector in float32 precision.
func VectorMagnitudeF32[T foundation.Numeric](vector memcore.MarkRaw) float32 {
	sqrSum := float32(0)

	vectorUnaryReadOnlyExecute(vector, func(a T) {
		sqrSum += float32(a) * float32(a)
	})

	sqrRoot := foundation.Sqrt32(sqrSum)
	return sqrRoot
}

// VectorMagnitudeF64 computes the magnitude of a given vector in float64 precision.
func VectorMagnitudeF64[T foundation.Numeric](vector memcore.MarkRaw) float64 {
	sqrSum := float64(0)

	vectorUnaryReadOnlyExecute(vector, func(a T) {
		sqrSum += float64(a) * float64(a)
	})

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
	vectorUnaryExecute(currentVector, newVectorAddr, func(a T) float32 {
		return float32(a) * inv
	})
}

// VectorNormalizedF64 normalizes the vector with float64 precision such that its magnitude is 1.
func VectorNormalizedF64[T foundation.Numeric](
	currentVector memcore.MarkRaw,
	newVectorAddr memcore.MarkRaw,
) {
	magnitude := VectorMagnitudeF64[T](currentVector)
	inv := 1.0 / magnitude

	vectorUnaryExecute(currentVector, newVectorAddr, func(a T) float64 {
		return float64(a) * inv
	})
}

// VectorDotProductF32 computes the dot product between two vectors in float32 precision.
// T is the datatype of vector A, and U is the datatype of vector B.
func VectorDotProductF32[T, U foundation.Numeric](vectorAAddr, vectorBAddr memcore.MarkRaw) float32 {
	dotProduct := float32(0)

	vectorBinaryReadOnlyExecute(vectorAAddr, vectorBAddr, func(a T, b U) {
		dotProduct += float32(a) * float32(b)
	})

	return dotProduct
}

// VectorDotProductF64 computes the dot product between two vectors in float64 precision.
// T is the datatype of vector A, and U is the datatype of vector B.
func VectorDotProductF64[T, U foundation.Numeric](vectorAAddr, vectorBAddr memcore.MarkRaw) float64 {
	dotProduct := float64(0)

	vectorBinaryReadOnlyExecute(vectorAAddr, vectorBAddr, func(a T, b U) {
		dotProduct += float64(a) * float64(b)
	})

	return dotProduct
}

// VectorScaleF32 scales the vector with float32 precision such each value is multiplied by the scalar.
func VectorScaleF32[T, S foundation.Numeric](
	currentVectorAddr, newVectorAddr memcore.MarkRaw,
	scalar S,
) {
	vectorUnaryExecute(currentVectorAddr, newVectorAddr, func(a T) float32 {
		return float32(a) * float32(scalar)
	})
}

// VectorScaleF64 scales the vector with float64 precision such each value is multiplied by the scalar.
func VectorScaleF64[T, S foundation.Numeric](
	currentVectorAddr, newVectorAddr memcore.MarkRaw,
	scalar S,
) {
	vectorUnaryExecute(currentVectorAddr, newVectorAddr, func(a T) float64 {
		return float64(a) * float64(scalar)
	})
}

// VectorClamp transforms the vector in such a way that each element is between
// min and max.
func VectorClamp[T foundation.Numeric](
	vector memcore.MarkRaw,
	min, max T,
) {
	vectorUnaryExecute(vector, vector, func(a T) T {
		if a < min {
			return min
		} else if a > max {
			return max
		}

		return a
	})
}

// VectorAddF32 adds the values of Vector B to Vector A, resulting in Vector C at newVectorAddr.
// It does so in float32 precision.
func VectorAddF32[T, U foundation.Numeric](
	vectorAAddr, vectorBAddr, newVectorAddr memcore.MarkRaw,
) {
	vectorBinaryExecute(vectorAAddr, vectorBAddr, newVectorAddr, func(a T, b U) float32 {
		return float32(a) + float32(b)
	})
}

// VectorAddF64 adds the values of Vector B to Vector A, resulting in Vector C at newVectorAddr.
// It does so in float64 precision.
func VectorAddF64[T, U foundation.Numeric](
	vectorAAddr, vectorBAddr, newVectorAddr memcore.MarkRaw,
) {
	vectorBinaryExecute(vectorAAddr, vectorBAddr, newVectorAddr, func(a T, b U) float64 {
		return float64(a) + float64(b)
	})
}

// VectorSubtractF32 subtracts the values of Vector B from Vector A, resulting in Vector C at newVectorAddr.
// It does so in float32 precision.
func VectorSubtractF32[T, U foundation.Numeric](
	vectorAAddr, vectorBAddr, newVectorAddr memcore.MarkRaw,
) {
	vectorBinaryExecute(vectorAAddr, vectorBAddr, newVectorAddr, func(a T, b U) float32 {
		return float32(a) - float32(b)
	})
}

// VectorSubtractF64 subtracts the values of Vector B from Vector A, resulting in Vector C at newVectorAddr.
// It does so in float364precision.
func VectorSubtractF64[T, U foundation.Numeric](
	vectorAAddr, vectorBAddr, newVectorAddr memcore.MarkRaw,
) {
	vectorBinaryExecute(vectorAAddr, vectorBAddr, newVectorAddr, func(a T, b U) float64 {
		return float64(a) - float64(b)
	})
}

// -------------------------- PRIVATE HELPERS

//go:inline
func vectorUnaryReadOnlyExecute[T foundation.Numeric](
	vectorAddr memcore.MarkRaw,
	op func(a T),
) {
	srcBase, srcInstance := memcore.MemcoreMarkDereferenceObjectAlt[Vector[T]](vectorAddr)

	capacity := srcInstance.capacity
	srcItemSize := uintptr(srcInstance.itemSize)

	srcData := vectorComputeDataAddr(srcInstance, srcBase)

	i := uint64(0)
	for ; i+7 < capacity; i += 8 {
		vectorUnaryReadOnlyOp(srcData, srcItemSize, uintptr(i+0), op)
		vectorUnaryReadOnlyOp(srcData, srcItemSize, uintptr(i+1), op)
		vectorUnaryReadOnlyOp(srcData, srcItemSize, uintptr(i+2), op)
		vectorUnaryReadOnlyOp(srcData, srcItemSize, uintptr(i+3), op)
		vectorUnaryReadOnlyOp(srcData, srcItemSize, uintptr(i+4), op)
		vectorUnaryReadOnlyOp(srcData, srcItemSize, uintptr(i+5), op)
		vectorUnaryReadOnlyOp(srcData, srcItemSize, uintptr(i+6), op)
		vectorUnaryReadOnlyOp(srcData, srcItemSize, uintptr(i+7), op)
	}

	for ; i < capacity; i++ {
		vectorUnaryReadOnlyOp(srcData, srcItemSize, uintptr(i), op)
	}
}

//go:inline
func vectorUnaryExecute[T, P foundation.Numeric](
	vectorAddr, newVectorAddr memcore.MarkRaw,
	op func(a T) P,
) {
	srcBase, srcInstance := memcore.MemcoreMarkDereferenceObjectAlt[Vector[T]](vectorAddr)
	dstBase, dstInstance := memcore.MemcoreMarkDereferenceObjectAlt[Vector[P]](newVectorAddr)

	if srcInstance.capacity != dstInstance.capacity {
		panic(fmt.Errorf("cannot do unary operation with different capacities (src=%d,dest=%d)", srcInstance.capacity, dstInstance.capacity))
	}

	capacity := srcInstance.capacity
	srcItemSize := uintptr(srcInstance.itemSize)
	dstItemSize := uintptr(dstInstance.itemSize)

	srcData := vectorComputeDataAddr(srcInstance, srcBase)
	dstData := vectorComputeDataAddr(dstInstance, dstBase)

	i := uint64(0)
	for ; i+7 < capacity; i += 8 {
		vectorUnaryOp(srcData, dstData, srcItemSize, dstItemSize, uintptr(i+0), op)
		vectorUnaryOp(srcData, dstData, srcItemSize, dstItemSize, uintptr(i+1), op)
		vectorUnaryOp(srcData, dstData, srcItemSize, dstItemSize, uintptr(i+2), op)
		vectorUnaryOp(srcData, dstData, srcItemSize, dstItemSize, uintptr(i+3), op)
		vectorUnaryOp(srcData, dstData, srcItemSize, dstItemSize, uintptr(i+4), op)
		vectorUnaryOp(srcData, dstData, srcItemSize, dstItemSize, uintptr(i+5), op)
		vectorUnaryOp(srcData, dstData, srcItemSize, dstItemSize, uintptr(i+6), op)
		vectorUnaryOp(srcData, dstData, srcItemSize, dstItemSize, uintptr(i+7), op)
	}

	for ; i < capacity; i++ {
		vectorUnaryOp(srcData, dstData, srcItemSize, dstItemSize, uintptr(i), op)
	}
}

//go:inline
func vectorBinaryReadOnlyExecute[T, U foundation.Numeric](
	vectorAAddr, vectorBAddr memcore.MarkRaw,
	op func(a T, b U),
) {
	srcABase, srcAInstance := memcore.MemcoreMarkDereferenceObjectAlt[Vector[T]](vectorAAddr)
	srcBBase, srcBInstance := memcore.MemcoreMarkDereferenceObjectAlt[Vector[U]](vectorBAddr)

	if srcAInstance.capacity != srcBInstance.capacity {
		panic(fmt.Errorf("cannot do binary operation with different capacities (srcA=%d,srcB=%d)", srcAInstance.capacity, srcBInstance.capacity))
	}

	capacity := srcAInstance.capacity
	srcAItemSize := uintptr(srcAInstance.itemSize)
	srcBItemSize := uintptr(srcBInstance.itemSize)

	srcAData := vectorComputeDataAddr(srcAInstance, srcABase)
	srcBData := vectorComputeDataAddr(srcBInstance, srcBBase)

	i := uint64(0)
	for ; i+7 < capacity; i += 8 {
		vectorBinaryReadOnlyOp(srcAData, srcBData, srcAItemSize, srcBItemSize, uintptr(i+0), op)
		vectorBinaryReadOnlyOp(srcAData, srcBData, srcAItemSize, srcBItemSize, uintptr(i+1), op)
		vectorBinaryReadOnlyOp(srcAData, srcBData, srcAItemSize, srcBItemSize, uintptr(i+2), op)
		vectorBinaryReadOnlyOp(srcAData, srcBData, srcAItemSize, srcBItemSize, uintptr(i+3), op)
		vectorBinaryReadOnlyOp(srcAData, srcBData, srcAItemSize, srcBItemSize, uintptr(i+4), op)
		vectorBinaryReadOnlyOp(srcAData, srcBData, srcAItemSize, srcBItemSize, uintptr(i+5), op)
		vectorBinaryReadOnlyOp(srcAData, srcBData, srcAItemSize, srcBItemSize, uintptr(i+6), op)
		vectorBinaryReadOnlyOp(srcAData, srcBData, srcAItemSize, srcBItemSize, uintptr(i+7), op)
	}

	for ; i < capacity; i++ {
		vectorBinaryReadOnlyOp(srcAData, srcBData, srcAItemSize, srcBItemSize, uintptr(i), op)
	}
}

//go:inline
func vectorBinaryExecute[T, U, P foundation.Numeric](
	vectorAAddr, vectorBAddr, newVectorAddr memcore.MarkRaw,
	op func(a T, b U) P,
) {
	srcABase, srcAInstance := memcore.MemcoreMarkDereferenceObjectAlt[Vector[T]](vectorAAddr)
	srcBBase, srcBInstance := memcore.MemcoreMarkDereferenceObjectAlt[Vector[U]](vectorBAddr)
	dstBase, dstInstance := memcore.MemcoreMarkDereferenceObjectAlt[Vector[P]](newVectorAddr)

	if srcAInstance.capacity != dstInstance.capacity || srcAInstance.capacity != srcBInstance.capacity {
		panic(fmt.Errorf("cannot do binary operation with different capacities (srcA=%d,srcB=%d,dest=%d)", srcAInstance.capacity, srcBInstance.capacity, dstInstance.capacity))
	}

	capacity := srcAInstance.capacity
	srcAItemSize := uintptr(srcAInstance.itemSize)
	srcBItemSize := uintptr(srcBInstance.itemSize)
	dstItemSize := uintptr(dstInstance.itemSize)

	srcAData := vectorComputeDataAddr(srcAInstance, srcABase)
	srcBData := vectorComputeDataAddr(srcBInstance, srcBBase)
	dstData := vectorComputeDataAddr(dstInstance, dstBase)

	i := uint64(0)
	for ; i+7 < capacity; i += 8 {
		vectorBinaryOp(srcAData, srcBData, dstData, srcAItemSize, srcBItemSize, dstItemSize, uintptr(i+0), op)
		vectorBinaryOp(srcAData, srcBData, dstData, srcAItemSize, srcBItemSize, dstItemSize, uintptr(i+1), op)
		vectorBinaryOp(srcAData, srcBData, dstData, srcAItemSize, srcBItemSize, dstItemSize, uintptr(i+2), op)
		vectorBinaryOp(srcAData, srcBData, dstData, srcAItemSize, srcBItemSize, dstItemSize, uintptr(i+3), op)
		vectorBinaryOp(srcAData, srcBData, dstData, srcAItemSize, srcBItemSize, dstItemSize, uintptr(i+4), op)
		vectorBinaryOp(srcAData, srcBData, dstData, srcAItemSize, srcBItemSize, dstItemSize, uintptr(i+5), op)
		vectorBinaryOp(srcAData, srcBData, dstData, srcAItemSize, srcBItemSize, dstItemSize, uintptr(i+6), op)
		vectorBinaryOp(srcAData, srcBData, dstData, srcAItemSize, srcBItemSize, dstItemSize, uintptr(i+7), op)
	}

	for ; i < capacity; i++ {
		vectorBinaryOp(srcAData, srcBData, dstData, srcAItemSize, srcBItemSize, dstItemSize, uintptr(i), op)
	}
}

//go:inline
//go:nosplit
func vectorUnaryReadOnlyOp[T foundation.Numeric](
	srcData unsafe.Pointer,
	srcItemSize uintptr,
	idx uintptr,
	op func(a T),
) {
	v := (*(*T)(unsafe.Add(srcData, idx*srcItemSize)))
	op(v)
}

//go:inline
//go:nosplit
func vectorUnaryOp[T, P foundation.Numeric](
	srcData, dstData unsafe.Pointer,
	srcItemSize, dstItemSize uintptr,
	idx uintptr,
	op func(a T) P,
) {
	srcV := (*(*T)(unsafe.Add(srcData, idx*srcItemSize)))
	newV := op(srcV)
	*(*P)(unsafe.Add(dstData, idx*dstItemSize)) = newV
}

//go:inline
//go:nosplit
func vectorBinaryReadOnlyOp[T, U foundation.Numeric](
	srcAData, srcBData unsafe.Pointer,
	srcAItemSize, srcBItemSize uintptr,
	idx uintptr,
	op func(a T, b U),
) {
	srcAV := (*(*T)(unsafe.Add(srcAData, idx*srcAItemSize)))
	srcBV := (*(*U)(unsafe.Add(srcBData, idx*srcBItemSize)))
	op(srcAV, srcBV)
}

//go:inline
//go:nosplit
func vectorBinaryOp[T, U, P foundation.Numeric](
	srcAData, srcBData, dstData unsafe.Pointer,
	srcAItemSize, srcBItemSize, dstItemSize uintptr,
	idx uintptr,
	op func(a T, b U) P,
) {
	srcAV := (*(*T)(unsafe.Add(srcAData, idx*srcAItemSize)))
	srcBV := (*(*U)(unsafe.Add(srcBData, idx*srcBItemSize)))
	newV := op(srcAV, srcBV)
	*(*P)(unsafe.Add(dstData, idx*dstItemSize)) = newV
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
