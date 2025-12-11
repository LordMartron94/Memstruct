package memstruct

import (
	"fmt"
	"foundation"
	"memcore"
	"strings"
	"unsafe"
)

// Matrix is a 2D matrix structure specialized for numerics.
// It wraps a Vector internally, storing data in row-major order.
// All Vector functions work on the underlying Vector.
type Matrix[T foundation.Numeric] struct {
	data memcore.MarkRaw // Vector[T] with capacity = rows * cols
	rows uint64
	cols uint64
}

// MatrixView represents a view around a matrix.
// It can be readonly and/or a subset within the matrix.
type MatrixView[T foundation.Numeric] struct {
	matrixHeader     memcore.MarkRaw
	startRow, endRow uint64
	startCol, endCol uint64
	readonly         bool
}

// matrixRowColToIdx converts (row, col) to linear index using row-major ordering.
//
//go:inline
func matrixRowColToIdx(row, col, cols uint64) uint64 {
	return row*cols + col
}

// matrixIdxToRowCol converts linear index to (row, col) using row-major ordering.
//
//go:inline
func matrixIdxToRowCol(idx, cols uint64) (uint64, uint64) {
	return idx / cols, idx % cols
}

// MatrixRequiredBytesGet returns the required bytes for a matrix with given dimensions.
func MatrixRequiredBytesGet[T foundation.Numeric](rows, cols uint64) uint64 {
	matrixHeaderSize := memcore.SizeOf[Matrix[T]]()
	vectorCapacity := rows * cols
	vectorSize := VectorRequiredBytesGet[T](vectorCapacity)
	return matrixHeaderSize + vectorSize
}

// MatrixRequiredAlignmentGet returns the required alignment for the Matrix header.
//
//go:inline
func MatrixRequiredAlignmentGet[T foundation.Numeric]() uint64 {
	return max(memcore.AlignOf[Matrix[T]](), VectorRequiredAlignmentGet[T]())
}

func (m *Matrix[T]) String() string {
	if m == nil {
		return "<nil Matrix>"
	}

	var sb strings.Builder
	fmt.Fprintf(&sb, "Matrix[%T]{rows=%d, cols=%d, data=[", *new(T), m.rows, m.cols)

	// Access the stored mark - if invalid, Vector functions will handle errors
	vectorAddr := m.data

	for row := uint64(0); row < m.rows; row++ {
		if row > 0 {
			sb.WriteString("; ")
		}
		sb.WriteString("[")
		for col := uint64(0); col < m.cols; col++ {
			if col > 0 {
				sb.WriteString(", ")
			}
			idx := matrixRowColToIdx(row, col, m.cols)
			val := VectorItemGetAtUnsafe[T](vectorAddr, idx)
			fmt.Fprintf(&sb, "%v", val)
		}
		sb.WriteString("]")
	}

	sb.WriteString("]}")
	return sb.String()
}

// MatrixInitializeAt initializes an instance of a matrix for type T at a specific memory address.
// Ensure the address is properly aligned and has the right size.
//
// ⚠️ rows and cols are in elements, not bytes.
func MatrixInitializeAt[T foundation.Numeric](matrixAddr memcore.MarkRaw, rows, cols uint64) {
	matrixPtr := memcore.MemcoreMarkDereferenceObject[Matrix[T]](matrixAddr)

	vectorCapacity := rows * cols
	vectorOffset := uintptr(memcore.SizeOf[Matrix[T]]())
	vectorMark := memcore.MemcoreMarkOffsetFrom(matrixAddr, vectorOffset)

	VectorInitializeAt[T](vectorMark, vectorCapacity)

	*matrixPtr = Matrix[T]{
		data: vectorMark,
		rows: rows,
		cols: cols,
	}
}

// MatrixInitializeFrom initializes a new matrix at matrixAddr with the contents of src.
// Rows and cols must match.
func MatrixInitializeFrom[T foundation.Numeric](matrixAddr memcore.MarkRaw, src memcore.MarkRaw, newRows, newCols uint64) error {
	srcMatrix := memcore.MemcoreMarkDereferenceObjectUnsafe[Matrix[T]](src)
	if srcMatrix.rows != newRows || srcMatrix.cols != newCols {
		return fmt.Errorf("MatrixInitializeFrom: dimension mismatch (src=%dx%d, new=%dx%d)",
			srcMatrix.rows, srcMatrix.cols, newRows, newCols)
	}

	MatrixInitializeAt[T](matrixAddr, newRows, newCols)
	return MatrixCopyFrom[T](matrixAddr, src, 0, 0)
}

// MatrixSnapshotCreate creates a deep copy of a matrix at a new memory location
// defined by the destination pointer (which points to the start of the new matrix header).
// It copies both the header and the data that follow it, maintaining the same relative layout.
func MatrixSnapshotCreate[T foundation.Numeric](dest memcore.MarkRaw, instance memcore.MarkRaw) memcore.MarkRaw {
	srcMatrix := memcore.MemcoreMarkDereferenceObjectUnsafe[Matrix[T]](instance)
	totalSize := MatrixRequiredBytesGet[T](srcMatrix.rows, srcMatrix.cols)

	srcAddr := memcore.MemcoreMarkDereference(instance)
	dstAddr := memcore.MemcoreMarkDereference(dest)

	memcore.MemoryMoveNoHeapPointers(dstAddr, srcAddr, uintptr(totalSize))

	dstMatrix := memcore.MemcoreMarkDereferenceObjectUnsafe[Matrix[T]](dest)
	offsetData := uintptr(memcore.SizeOf[Matrix[T]]())
	dstMatrix.data = memcore.MemcoreMarkOffsetFrom(dest, offsetData)

	return dest
}

// MatrixSnapshotRestore replaces the entire memory block of one matrix
// (header + data) with that of another matrix of the same type and dimensions.
// Both matrices must live in manual memory managed by memcore.
func MatrixSnapshotRestore[T foundation.Numeric](dest, src memcore.MarkRaw) error {
	dstMatrix := memcore.MemcoreMarkDereferenceObjectUnsafe[Matrix[T]](dest)
	srcMatrix := memcore.MemcoreMarkDereferenceObjectUnsafe[Matrix[T]](src)

	if dstMatrix.rows != srcMatrix.rows || dstMatrix.cols != srcMatrix.cols {
		return fmt.Errorf("cannot restore snapshot: dimension mismatch (dest=%dx%d, src=%dx%d)",
			dstMatrix.rows, dstMatrix.cols, srcMatrix.rows, srcMatrix.cols)
	}

	if dest == src {
		return nil
	}

	totalBytes := MatrixRequiredBytesGet[T](dstMatrix.rows, dstMatrix.cols)

	dstAddr := memcore.MemcoreMarkDereferenceUnsafe(dest)
	srcAddr := memcore.MemcoreMarkDereferenceUnsafe(src)

	memcore.MemoryMoveNoHeapPointers(dstAddr, srcAddr, uintptr(totalBytes))

	return nil
}

// MatrixHeaderClone clones the header to the matrix data.
// It will not move memory at all.
func MatrixHeaderClone[T foundation.Numeric](dest, src memcore.MarkRaw) {
	dstMatrix := memcore.MemcoreMarkDereferenceObjectUnsafe[Matrix[T]](dest)
	srcMatrix := memcore.MemcoreMarkDereferenceObjectUnsafe[Matrix[T]](src)

	*dstMatrix = *srcMatrix
}

// MatrixCopyFrom copies the entire contents of src matrix into dest matrix,
// starting at destStartRow, destStartCol. Both matrices must have the same element type T.
// Dimensions must allow the copy, else an error is returned.
func MatrixCopyFrom[T foundation.Numeric](dest memcore.MarkRaw, src memcore.MarkRaw, destStartRow, destStartCol uint64) error {
	dstMatrix := memcore.MemcoreMarkDereferenceObjectUnsafe[Matrix[T]](dest)
	srcMatrix := memcore.MemcoreMarkDereferenceObjectUnsafe[Matrix[T]](src)

	if destStartRow+srcMatrix.rows > dstMatrix.rows || destStartCol+srcMatrix.cols > dstMatrix.cols {
		return fmt.Errorf("MatrixCopyFrom: insufficient capacity (destStart=%d,%d, src=%dx%d, dest=%dx%d)",
			destStartRow, destStartCol, srcMatrix.rows, srcMatrix.cols, dstMatrix.rows, dstMatrix.cols)
	}

	// Copy row by row
	for row := uint64(0); row < srcMatrix.rows; row++ {
		srcStartIdx := matrixRowColToIdx(row, 0, srcMatrix.cols)
		dstStartIdx := matrixRowColToIdx(destStartRow+row, destStartCol, dstMatrix.cols)

		err := VectorCopyFromRange[T](dstMatrix.data, srcMatrix.data, srcStartIdx, srcStartIdx+srcMatrix.cols, dstStartIdx)
		if err != nil {
			return err
		}
	}

	return nil
}

// MatrixCopyFromRange copies src[startRow:endRow, startCol:endCol) into dest starting at destStartRow, destStartCol.
// Bounds are checked; both matrices must have same type T.
func MatrixCopyFromRange[T foundation.Numeric](
	dest memcore.MarkRaw,
	src memcore.MarkRaw,
	startRow, endRow, startCol, endCol, destStartRow, destStartCol uint64,
) error {
	if startRow >= endRow || startCol >= endCol {
		return fmt.Errorf("MatrixCopyFromRange: invalid range")
	}

	dstMatrix := memcore.MemcoreMarkDereferenceObjectUnsafe[Matrix[T]](dest)
	srcMatrix := memcore.MemcoreMarkDereferenceObjectUnsafe[Matrix[T]](src)

	if endRow > srcMatrix.rows || endCol > srcMatrix.cols {
		return fmt.Errorf("MatrixCopyFromRange: range exceeds src dimensions")
	}

	rows := endRow - startRow
	cols := endCol - startCol

	if destStartRow+rows > dstMatrix.rows || destStartCol+cols > dstMatrix.cols {
		return fmt.Errorf("MatrixCopyFromRange: insufficient dest capacity")
	}

	// Copy row by row
	for row := range rows {
		srcStartIdx := matrixRowColToIdx(startRow+row, startCol, srcMatrix.cols)
		dstStartIdx := matrixRowColToIdx(destStartRow+row, destStartCol, dstMatrix.cols)

		err := VectorCopyFromRange[T](dstMatrix.data, srcMatrix.data, srcStartIdx, srcStartIdx+cols, dstStartIdx)
		if err != nil {
			return err
		}
	}

	return nil
}

// MatrixHeaderSizeBytesGet returns the required bytes for the Matrix header.
//
//go:inline
func MatrixHeaderSizeBytesGet[T foundation.Numeric]() uint64 {
	return memcore.SizeOf[Matrix[T]]()
}

// MatrixHeaderAlignmentGet returns the required alignment for the Matrix header.
//
//go:inline
func MatrixHeaderAlignmentGet[T foundation.Numeric]() uint64 {
	return memcore.AlignOf[Matrix[T]]()
}

// MatrixRowsGet returns the number of rows in the matrix.
//
//go:nosplit
//go:inline
func MatrixRowsGet[T foundation.Numeric](matrix memcore.MarkRaw) uint64 {
	matrixPtr := memcore.MemcoreMarkDereferenceObjectUnsafe[Matrix[T]](matrix)
	return matrixPtr.rows
}

// MatrixColsGet returns the number of columns in the matrix.
//
//go:nosplit
//go:inline
func MatrixColsGet[T foundation.Numeric](matrix memcore.MarkRaw) uint64 {
	matrixPtr := memcore.MemcoreMarkDereferenceObjectUnsafe[Matrix[T]](matrix)
	return matrixPtr.cols
}

// MatrixCapacityGet returns the total amount of elements that can be stored (rows * cols).
//
//go:nosplit
//go:inline
func MatrixCapacityGet[T foundation.Numeric](matrix memcore.MarkRaw) uint64 {
	matrixPtr := memcore.MemcoreMarkDereferenceObjectUnsafe[Matrix[T]](matrix)
	return matrixPtr.rows * matrixPtr.cols
}

// MatrixItemGetAt returns T at (row, col) within the matrix.
// It returns an error if the row or col is invalid.
//
//go:nosplit
//go:inline
func MatrixItemGetAt[T foundation.Numeric](matrix memcore.MarkRaw, row, col uint64) (T, error) {
	matrixPtr := memcore.MemcoreMarkDereferenceObjectUnsafe[Matrix[T]](matrix)

	if row >= matrixPtr.rows || col >= matrixPtr.cols {
		var zero T
		return zero, fmt.Errorf("MatrixItemGetAt: index out of bounds (row=%d, col=%d, matrix=%dx%d)",
			row, col, matrixPtr.rows, matrixPtr.cols)
	}

	idx := matrixRowColToIdx(row, col, matrixPtr.cols)
	return VectorItemGetAt[T](matrixPtr.data, idx)
}

// MatrixItemGetAtUnsafe returns T at (row, col) within the matrix.
// It does no bounds checks.
//
//go:inline
func MatrixItemGetAtUnsafe[T foundation.Numeric](matrix memcore.MarkRaw, row, col uint64) T {
	matrixPtr := memcore.MemcoreMarkDereferenceObjectUnsafe[Matrix[T]](matrix)
	idx := matrixRowColToIdx(row, col, matrixPtr.cols)
	return VectorItemGetAtUnsafe[T](matrixPtr.data, idx)
}

// MatrixItemPtrGetAt returns a pointer to T at (row, col) within the matrix.
// It returns an error if the row or col is invalid.
//
// Using this pointer after deletion or overwriting this position is undefined behaviour.
// Use at your own discretion!
//
//go:nosplit
//go:inline
func MatrixItemPtrGetAt[T foundation.Numeric](matrix memcore.MarkRaw, row, col uint64) (*T, error) {
	matrixPtr := memcore.MemcoreMarkDereferenceObjectUnsafe[Matrix[T]](matrix)

	if row >= matrixPtr.rows || col >= matrixPtr.cols {
		return nil, fmt.Errorf("MatrixItemPtrGetAt: index out of bounds (row=%d, col=%d, matrix=%dx%d)",
			row, col, matrixPtr.rows, matrixPtr.cols)
	}

	idx := matrixRowColToIdx(row, col, matrixPtr.cols)
	return VectorItemPtrGetAt[T](matrixPtr.data, idx)
}

// MatrixItemPtrGetAtUnsafe returns a pointer to T at (row, col) within the matrix.
// It does no bounds checks.
//
// Using this pointer after deletion or overwriting this position is undefined behaviour.
// Use at your own discretion!
//
//go:inline
func MatrixItemPtrGetAtUnsafe[T foundation.Numeric](matrix memcore.MarkRaw, row, col uint64) *T {
	matrixPtr := memcore.MemcoreMarkDereferenceObjectUnsafe[Matrix[T]](matrix)
	idx := matrixRowColToIdx(row, col, matrixPtr.cols)
	return VectorItemPtrGetAtUnsafe[T](matrixPtr.data, idx)
}

// MatrixDataPtrGet returns the current pointer to the underlying data storage in memory.
// This value CAN change if the underlying memory region changes.
// Not stable, so do not store.
//
//go:inline
func MatrixDataPtrGet[T foundation.Numeric](matrix memcore.MarkRaw) unsafe.Pointer {
	matrixPtr := memcore.MemcoreMarkDereferenceObjectUnsafe[Matrix[T]](matrix)
	return VectorDataPtrGet[T](matrixPtr.data)
}

// MatrixByteOffsetGetAt returns the offset relative to the memory region for this (row, col).
// Panics if the row or col is invalid.
//
//go:inline
func MatrixByteOffsetGetAt[T foundation.Numeric](matrix memcore.MarkRaw, row, col uint64) uintptr {
	matrixPtr := memcore.MemcoreMarkDereferenceObjectUnsafe[Matrix[T]](matrix)
	idx := matrixRowColToIdx(row, col, matrixPtr.cols)
	return VectorByteOffsetGetAt[T](matrixPtr.data, idx)
}

// MatrixByteOffsetGetAtUnsafe returns the offset relative to the memory region for this (row, col).
// Does no bounds checks.
//
//go:inline
func MatrixByteOffsetGetAtUnsafe[T foundation.Numeric](matrix memcore.MarkRaw, row, col uint64) uintptr {
	matrixPtr := memcore.MemcoreMarkDereferenceObjectUnsafe[Matrix[T]](matrix)
	idx := matrixRowColToIdx(row, col, matrixPtr.cols)
	return VectorByteOffsetGetAtUnsafe[T](matrixPtr.data, idx)
}

// MatrixSetAt sets (row, col) of matrix to value T.
// It returns an error if the row or col is invalid.
//
//go:nosplit
//go:inline
func MatrixSetAt[T foundation.Numeric](matrix memcore.MarkRaw, row, col uint64, value T) error {
	matrixPtr := memcore.MemcoreMarkDereferenceObjectUnsafe[Matrix[T]](matrix)

	if row >= matrixPtr.rows || col >= matrixPtr.cols {
		return fmt.Errorf("MatrixSetAt: index out of bounds (row=%d, col=%d, matrix=%dx%d)",
			row, col, matrixPtr.rows, matrixPtr.cols)
	}

	idx := matrixRowColToIdx(row, col, matrixPtr.cols)
	return VectorSetAt(matrixPtr.data, idx, value)
}

// MatrixSetAtUnsafe sets (row, col) of matrix to value T.
// It does no bounds checks.
//
//go:nosplit
//go:inline
func MatrixSetAtUnsafe[T foundation.Numeric](matrix memcore.MarkRaw, row, col uint64, value T) {
	matrixPtr := memcore.MemcoreMarkDereferenceObjectUnsafe[Matrix[T]](matrix)
	idx := matrixRowColToIdx(row, col, matrixPtr.cols)
	VectorSetAtUnsafe(matrixPtr.data, idx, value)
}

// MatrixSetAll sets all values within the matrix to value T.
//
//go:inline
func MatrixSetAll[T foundation.Numeric](matrix memcore.MarkRaw, v T) {
	matrixPtr := memcore.MemcoreMarkDereferenceObjectUnsafe[Matrix[T]](matrix)
	VectorSetAll(matrixPtr.data, v)
}

// MatrixZeroAll sets all values within the Matrix to its zero value.
// This is different from clearing the memory to 0.
//
//go:inline
func MatrixZeroAll[T foundation.Numeric](matrix memcore.MarkRaw) {
	matrixPtr := memcore.MemcoreMarkDereferenceObjectUnsafe[Matrix[T]](matrix)
	VectorZeroAll[T](matrixPtr.data)
}

// MatrixForEachUnsafe calls a function for every element in the matrix.
// The function receives (row, col) indices.
//
//go:inline
func MatrixForEachUnsafe[T foundation.Numeric](matrix memcore.MarkRaw, fn func(ptr unsafe.Pointer, row, col uint64)) {
	matrixPtr := memcore.MemcoreMarkDereferenceObjectUnsafe[Matrix[T]](matrix)

	VectorForEachUnsafe[T](matrixPtr.data, func(ptr unsafe.Pointer, idx uint64) {
		row, col := matrixIdxToRowCol(idx, matrixPtr.cols)
		fn(ptr, row, col)
	})
}

// MatrixStrideForEachUnsafe calls a function for every element in the matrix.
// It visits every stride-th element.
//
// For the tail it calls the tailFn which is supposed to process one element at once.
// The function receives (row, col) indices.
//
//go:inline
func MatrixStrideForEachUnsafe[T foundation.Numeric](
	matrix memcore.MarkRaw,
	fn func(ptr unsafe.Pointer, row, col uint64),
	tailFn func(ptr unsafe.Pointer, row, col uint64),
	stride uint64,
) {
	matrixPtr := memcore.MemcoreMarkDereferenceObjectUnsafe[Matrix[T]](matrix)

	VectorStrideForEachUnsafe[T](matrixPtr.data,
		func(ptr unsafe.Pointer, idx uint64) {
			row, col := matrixIdxToRowCol(idx, matrixPtr.cols)
			fn(ptr, row, col)
		},
		func(ptr unsafe.Pointer, idx uint64) {
			row, col := matrixIdxToRowCol(idx, matrixPtr.cols)
			tailFn(ptr, row, col)
		},
		stride,
	)
}

// MatrixIterate allows you to iterate over the matrix efficiently.
// It provides a method to say which next element you need.
// The next function returns true when your requested n exceeds the capacity.
// The function receives (row, col) indices.
//
//go:inline
func MatrixIterate[T foundation.Numeric](
	matrix memcore.MarkRaw,
	fn func(ptr unsafe.Pointer, row, col uint64, next func(n uint64) (unsafe.Pointer, uint64, uint64, bool)),
) {
	matrixPtr := memcore.MemcoreMarkDereferenceObjectUnsafe[Matrix[T]](matrix)

	VectorIterate[T](matrixPtr.data, func(ptr unsafe.Pointer, idx uint64, next func(n uint64) (unsafe.Pointer, uint64, bool)) {
		row, col := matrixIdxToRowCol(idx, matrixPtr.cols)

		nextMatrix := func(n uint64) (unsafe.Pointer, uint64, uint64, bool) {
			nextPtr, nextIdx, ok := next(n)
			if !ok {
				return nil, 0, 0, false
			}
			nextRow, nextCol := matrixIdxToRowCol(nextIdx, matrixPtr.cols)
			return nextPtr, nextRow, nextCol, true
		}

		fn(ptr, row, col, nextMatrix)
	})
}

// MatrixIterateUnsafe allows you to iterate over the matrix efficiently.
// It provides a method to say which next element you need.
// The next function does no bounds checking. Safety must be guaranteed by the client.
// The function receives (row, col) indices.
//
//go:inline
func MatrixIterateUnsafe[T foundation.Numeric](
	matrix memcore.MarkRaw,
	fn func(ptr unsafe.Pointer, row, col uint64, next func(n uint64) (unsafe.Pointer, uint64, uint64)),
) {
	matrixPtr := memcore.MemcoreMarkDereferenceObjectUnsafe[Matrix[T]](matrix)

	VectorIterateUnsafe[T](matrixPtr.data, func(ptr unsafe.Pointer, idx uint64, next func(n uint64) (unsafe.Pointer, uint64)) {
		row, col := matrixIdxToRowCol(idx, matrixPtr.cols)

		nextMatrix := func(n uint64) (unsafe.Pointer, uint64, uint64) {
			nextPtr, nextIdx := next(n)
			nextRow, nextCol := matrixIdxToRowCol(nextIdx, matrixPtr.cols)
			return nextPtr, nextRow, nextCol
		}

		fn(ptr, row, col, nextMatrix)
	})
}

// MatrixReplaceInternal replaces srcRow,srcCol with the value at destRow,destCol efficiently.
//
//go:inline
func MatrixReplaceInternal[T foundation.Numeric](matrix memcore.MarkRaw, srcRow, srcCol, destRow, destCol uint64) error {
	matrixPtr := memcore.MemcoreMarkDereferenceObjectUnsafe[Matrix[T]](matrix)

	srcIdx := matrixRowColToIdx(srcRow, srcCol, matrixPtr.cols)
	destIdx := matrixRowColToIdx(destRow, destCol, matrixPtr.cols)

	return VectorReplaceInternal[T](matrixPtr.data, srcIdx, destIdx)
}

// MatrixReplaceInternalUnsafe replaces srcRow,srcCol with the value at destRow,destCol efficiently.
//
// It does no bounds checks.
//
//go:inline
func MatrixReplaceInternalUnsafe[T foundation.Numeric](matrix memcore.MarkRaw, srcRow, srcCol, destRow, destCol uint64) {
	matrixPtr := memcore.MemcoreMarkDereferenceObjectUnsafe[Matrix[T]](matrix)

	srcIdx := matrixRowColToIdx(srcRow, srcCol, matrixPtr.cols)
	destIdx := matrixRowColToIdx(destRow, destCol, matrixPtr.cols)

	VectorReplaceInternalUnsafe[T](matrixPtr.data, srcIdx, destIdx)
}

// MatrixRangeCopy is a convenience wrapper around shift left/shift right.
// It automatically determines which to use based on from and to.
// This operates on the underlying linear storage, so use with caution for 2D data.
//
//go:inline
//go:nosplit
func MatrixRangeCopy[T foundation.Numeric](matrix memcore.MarkRaw, fromRow, fromCol, toRow, toCol, count uint64) error {
	matrixPtr := memcore.MemcoreMarkDereferenceObjectUnsafe[Matrix[T]](matrix)

	fromIdx := matrixRowColToIdx(fromRow, fromCol, matrixPtr.cols)
	toIdx := matrixRowColToIdx(toRow, toCol, matrixPtr.cols)

	return VectorRangeCopy[T](matrixPtr.data, fromIdx, toIdx, count)
}

// MatrixRangeCopyUnsafe is a convenience wrapper around shift left/shift right unsafe.
// It automatically determines which to use based on from and to.
// This operates on the underlying linear storage, so use with caution for 2D data.
//
//go:inline
//go:nosplit
func MatrixRangeCopyUnsafe[T foundation.Numeric](matrix memcore.MarkRaw, fromRow, fromCol, toRow, toCol, count uint64) {
	matrixPtr := memcore.MemcoreMarkDereferenceObjectUnsafe[Matrix[T]](matrix)

	fromIdx := matrixRowColToIdx(fromRow, fromCol, matrixPtr.cols)
	toIdx := matrixRowColToIdx(toRow, toCol, matrixPtr.cols)

	VectorRangeCopyUnsafe[T](matrixPtr.data, fromIdx, toIdx, count)
}

// MatrixDeleteAt resets memory to 0 at a given (row, col), using pointers to this
// position gotten earlier is undefined behaviour.
// It returns an error if the row or col is invalid.
//
//go:nosplit
//go:inline
func MatrixDeleteAt[T foundation.Numeric](matrix memcore.MarkRaw, row, col uint64) error {
	matrixPtr := memcore.MemcoreMarkDereferenceObjectUnsafe[Matrix[T]](matrix)

	if row >= matrixPtr.rows || col >= matrixPtr.cols {
		return fmt.Errorf("MatrixDeleteAt: index out of bounds (row=%d, col=%d, matrix=%dx%d)",
			row, col, matrixPtr.rows, matrixPtr.cols)
	}

	idx := matrixRowColToIdx(row, col, matrixPtr.cols)
	return VectorDeleteAt[T](matrixPtr.data, idx)
}

// MatrixDeleteAtUnsafe resets memory to 0 at a given (row, col), using pointers to this
// position gotten earlier is undefined behaviour.
// It does no bounds checks.
//
//go:nosplit
//go:inline
func MatrixDeleteAtUnsafe[T foundation.Numeric](matrix memcore.MarkRaw, row, col uint64) {
	matrixPtr := memcore.MemcoreMarkDereferenceObjectUnsafe[Matrix[T]](matrix)
	idx := matrixRowColToIdx(row, col, matrixPtr.cols)
	VectorDeleteAtUnsafe[T](matrixPtr.data, idx)
}

// MatrixClear resets the entire matrix's memory to 0, allowing it to be reused.
// Do NOT use pointers to previous matrix items after this as that is undefined behaviour.
//
//go:nosplit
//go:inline
func MatrixClear[T foundation.Numeric](matrix memcore.MarkRaw) {
	matrixPtr := memcore.MemcoreMarkDereferenceObjectUnsafe[Matrix[T]](matrix)
	VectorClear[T](matrixPtr.data)
}

// MatrixIsIdxValid checks whether the given (row, col) is valid.
//
//go:inline
func MatrixIsIdxValid[T foundation.Numeric](matrix memcore.MarkRaw, row, col uint64) bool {
	matrixPtr := memcore.MemcoreMarkDereferenceObjectUnsafe[Matrix[T]](matrix)
	return row < matrixPtr.rows && col < matrixPtr.cols
}

// MatrixSort sorts the matrix in-place using the underlying linear storage.
// This may not preserve 2D structure, so use with caution.
//
// The comparison function should return:
// a < b : -1 (or negative)
// a == b : 0
// a > b : 1 (or positive)
func MatrixSort[T foundation.Numeric](matrix memcore.MarkRaw, cmp func(a, b T) int) {
	matrixPtr := memcore.MemcoreMarkDereferenceObjectUnsafe[Matrix[T]](matrix)
	VectorSort(matrixPtr.data, cmp)
}

// MatrixSorted returns a sorted variant of this matrix.
// This may not preserve 2D structure, so use with caution.
//
// The comparison function should return:
// a < b : -1 (or negative)
// a == b : 0
// a > b : 1 (or positive)
func MatrixSorted[T foundation.Numeric](
	matrix memcore.MarkRaw,
	targetMatrixAddr memcore.MarkRaw,
	cmp func(a, b T) int,
) {
	matrixPtr := memcore.MemcoreMarkDereferenceObjectUnsafe[Matrix[T]](matrix)
	targetMatrixPtr := memcore.MemcoreMarkDereferenceObjectUnsafe[Matrix[T]](targetMatrixAddr)

	VectorSorted(matrixPtr.data, targetMatrixPtr.data, cmp)
}

// ---------------------------------------------------- MATRIX VIEW

// MatrixViewGet produces a view over a matrix that is potentially a subset and/or readonly.
// It panics if fromRow/fromCol or toRow/toCol are invalid.
//
//go:inline
func MatrixViewGet[T foundation.Numeric](matrix memcore.MarkRaw, fromRow, toRow, fromCol, toCol uint64, readonly bool) MatrixView[T] {
	if fromRow >= toRow || fromCol >= toCol {
		panic("fromRow must be < toRow and fromCol must be < toCol")
	}

	matrixPtr := memcore.MemcoreMarkDereferenceObjectUnsafe[Matrix[T]](matrix)

	if fromRow >= matrixPtr.rows || toRow > matrixPtr.rows {
		panic(fmt.Errorf("MatrixViewGet: row range invalid (fromRow=%d, toRow=%d, rows=%d)",
			fromRow, toRow, matrixPtr.rows))
	}

	if fromCol >= matrixPtr.cols || toCol > matrixPtr.cols {
		panic(fmt.Errorf("MatrixViewGet: col range invalid (fromCol=%d, toCol=%d, cols=%d)",
			fromCol, toCol, matrixPtr.cols))
	}

	return MatrixView[T]{
		matrixHeader: matrix,
		startRow:     fromRow,
		endRow:       toRow,
		startCol:     fromCol,
		endCol:       toCol,
		readonly:     readonly,
	}
}

// MatrixViewGetUnsafe produces a view over a matrix that is potentially a subset and/or readonly.
// It does no validation of bounds.
//
//go:inline
func MatrixViewGetUnsafe[T foundation.Numeric](matrix memcore.MarkRaw, fromRow, toRow, fromCol, toCol uint64, readonly bool) MatrixView[T] {
	return MatrixView[T]{
		matrixHeader: matrix,
		startRow:     fromRow,
		endRow:       toRow,
		startCol:     fromCol,
		endCol:       toCol,
		readonly:     readonly,
	}
}

// MatrixViewRowsGet returns the number of rows in the current view.
//
//go:inline
func MatrixViewRowsGet[T foundation.Numeric](matrixView MatrixView[T]) uint64 {
	return matrixView.endRow - matrixView.startRow
}

// MatrixViewColsGet returns the number of columns in the current view.
//
//go:inline
func MatrixViewColsGet[T foundation.Numeric](matrixView MatrixView[T]) uint64 {
	return matrixView.endCol - matrixView.startCol
}

// MatrixViewIsReadonly returns whether the current view is readonly.
//
//go:inline
func MatrixViewIsReadonly[T foundation.Numeric](matrixView MatrixView[T]) bool {
	return matrixView.readonly
}

// MatrixViewItemGetAt returns item at (relativeRow, relativeCol) within the view.
//
//go:inline
func MatrixViewItemGetAt[T foundation.Numeric](matrixView MatrixView[T], relativeRow, relativeCol uint64) (T, error) {
	matrixPtr := memcore.MemcoreMarkDereferenceObjectUnsafe[Matrix[T]](matrixView.matrixHeader)

	viewRows := matrixView.endRow - matrixView.startRow
	viewCols := matrixView.endCol - matrixView.startCol

	if relativeRow >= viewRows || relativeCol >= viewCols {
		var zero T
		return zero, fmt.Errorf("MatrixViewItemGetAt: index out of bounds (relativeRow=%d, relativeCol=%d, view=%dx%d)",
			relativeRow, relativeCol, viewRows, viewCols)
	}

	row := matrixView.startRow + relativeRow
	col := matrixView.startCol + relativeCol
	idx := matrixRowColToIdx(row, col, matrixPtr.cols)

	return VectorItemGetAt[T](matrixPtr.data, idx)
}

// MatrixViewItemPtrGetAt returns item pointer at (relativeRow, relativeCol) within the view.
// Fails if the view is readonly (because getting a pointer would allow mutation)
//
//go:inline
func MatrixViewItemPtrGetAt[T foundation.Numeric](matrixView MatrixView[T], relativeRow, relativeCol uint64) (*T, error) {
	if matrixView.readonly {
		return nil, fmt.Errorf("MatrixViewItemPtrGetAt: view is readonly")
	}

	matrixPtr := memcore.MemcoreMarkDereferenceObjectUnsafe[Matrix[T]](matrixView.matrixHeader)

	viewRows := matrixView.endRow - matrixView.startRow
	viewCols := matrixView.endCol - matrixView.startCol

	if relativeRow >= viewRows || relativeCol >= viewCols {
		return nil, fmt.Errorf("MatrixViewItemPtrGetAt: index out of bounds (relativeRow=%d, relativeCol=%d, view=%dx%d)",
			relativeRow, relativeCol, viewRows, viewCols)
	}

	row := matrixView.startRow + relativeRow
	col := matrixView.startCol + relativeCol
	idx := matrixRowColToIdx(row, col, matrixPtr.cols)

	return VectorItemPtrGetAt[T](matrixPtr.data, idx)
}

// MatrixViewItemSetAt sets the item at (relativeRow, relativeCol) within the view.
// Fails if the view is readonly.
//
//go:inline
func MatrixViewItemSetAt[T foundation.Numeric](matrixView MatrixView[T], relativeRow, relativeCol uint64, v T) error {
	if matrixView.readonly {
		return fmt.Errorf("MatrixViewItemSetAt: view is readonly")
	}

	matrixPtr := memcore.MemcoreMarkDereferenceObjectUnsafe[Matrix[T]](matrixView.matrixHeader)

	viewRows := matrixView.endRow - matrixView.startRow
	viewCols := matrixView.endCol - matrixView.startCol

	if relativeRow >= viewRows || relativeCol >= viewCols {
		return fmt.Errorf("MatrixViewItemSetAt: index out of bounds (relativeRow=%d, relativeCol=%d, view=%dx%d)",
			relativeRow, relativeCol, viewRows, viewCols)
	}

	row := matrixView.startRow + relativeRow
	col := matrixView.startCol + relativeCol
	idx := matrixRowColToIdx(row, col, matrixPtr.cols)

	return VectorSetAt(matrixPtr.data, idx, v)
}

// MatrixViewForEach calls a function for every element in the matrix view.
// The indexes returned are the relative indexes.
//
//go:inline
func MatrixViewForEach[T foundation.Numeric](matrixView MatrixView[T], fn func(item T, row, col uint64)) {
	matrixPtr := memcore.MemcoreMarkDereferenceObjectUnsafe[Matrix[T]](matrixView.matrixHeader)

	viewRows := matrixView.endRow - matrixView.startRow
	viewCols := matrixView.endCol - matrixView.startCol

	for relativeRow := range viewRows {
		for relativeCol := range viewCols {
			row := matrixView.startRow + relativeRow
			col := matrixView.startCol + relativeCol
			idx := matrixRowColToIdx(row, col, matrixPtr.cols)
			val := VectorItemGetAtUnsafe[T](matrixPtr.data, idx)
			fn(val, relativeRow, relativeCol)
		}
	}
}

// MatrixViewForEachRaw calls a function for every element in the matrix view.
// Not possible for readonly views.
// The indexes returned are the relative indexes.
//
//go:inline
func MatrixViewForEachRaw[T foundation.Numeric](matrixView MatrixView[T], fn func(ptr unsafe.Pointer, row, col uint64)) error {
	if matrixView.readonly {
		return fmt.Errorf("MatrixViewForEachRaw: view is readonly")
	}

	matrixPtr := memcore.MemcoreMarkDereferenceObjectUnsafe[Matrix[T]](matrixView.matrixHeader)

	viewRows := matrixView.endRow - matrixView.startRow
	viewCols := matrixView.endCol - matrixView.startCol

	for relativeRow := range viewRows {
		for relativeCol := range viewCols {
			row := matrixView.startRow + relativeRow
			col := matrixView.startCol + relativeCol
			idx := matrixRowColToIdx(row, col, matrixPtr.cols)
			ptr := VectorItemPtrGetAtUnsafe[T](matrixPtr.data, idx)
			fn(unsafe.Pointer(ptr), relativeRow, relativeCol)
		}
	}

	return nil
}

// MatrixViewNestedGet creates a nested view inside an existing view.
// The indices [fromRow:toRow, fromCol:toCol) are relative to the current view.
// The readonly status is preserved.
//
//go:inline
func MatrixViewNestedGet[T foundation.Numeric](matrixView MatrixView[T], fromRow, toRow, fromCol, toCol uint64) MatrixView[T] {
	if fromRow >= toRow || fromCol >= toCol {
		panic("fromRow must be < toRow and fromCol must be < toCol")
	}

	viewRows := matrixView.endRow - matrixView.startRow
	viewCols := matrixView.endCol - matrixView.startCol

	if fromRow >= viewRows || toRow > viewRows {
		panic(fmt.Errorf("MatrixViewNestedGet: row range invalid (fromRow=%d, toRow=%d, viewRows=%d)",
			fromRow, toRow, viewRows))
	}

	if fromCol >= viewCols || toCol > viewCols {
		panic(fmt.Errorf("MatrixViewNestedGet: col range invalid (fromCol=%d, toCol=%d, viewCols=%d)",
			fromCol, toCol, viewCols))
	}

	return MatrixView[T]{
		matrixHeader: matrixView.matrixHeader,
		startRow:     matrixView.startRow + fromRow,
		endRow:       matrixView.startRow + toRow,
		startCol:     matrixView.startCol + fromCol,
		endCol:       matrixView.startCol + toCol,
		readonly:     matrixView.readonly,
	}
}

// MatrixUnaryReadOnlyOp is an operation that executes over a single element and does not mutate.
type MatrixUnaryReadOnlyOp[T foundation.Numeric] func(item T)

// MatrixUnaryOp is an operation that executes over a single element and mutates.
type MatrixUnaryOp[TInput, TOutput foundation.Numeric] func(item TInput) TOutput

// MatrixBinaryReadOnlyOp is an operation that executes over two elements at the same (row, col) from different sources.
// It does not mutate.
type MatrixBinaryReadOnlyOp[TInput1, TInput2 foundation.Numeric] func(itemA TInput1, itemB TInput2)

// MatrixBinaryOp is an operation that executes over two elements at the same (row, col) from different sources.
// It mutates.
type MatrixBinaryOp[TInput1, TInput2, TOutput foundation.Numeric] func(itemA TInput1, itemB TInput2) TOutput

// MatrixUnaryReadOnlyExecute executes a stride of unary readonly operations.
//
//go:inline
func MatrixUnaryReadOnlyExecute[T foundation.Numeric](
	matrixAddr memcore.MarkRaw,
	op MatrixUnaryReadOnlyOp[T],
	stride uint64,
) {
	matrixPtr := memcore.MemcoreMarkDereferenceObjectUnsafe[Matrix[T]](matrixAddr)
	VectorUnaryReadOnlyExecute(matrixPtr.data, VectorUnaryReadOnlyOp[T](op), stride)
}

// MatrixUnaryExecute executes a stride of unary mutating operations,
// writing results from the source matrix into the destination matrix.
//
//go:inline
func MatrixUnaryExecute[T, P foundation.Numeric](
	srcAddr, dstAddr memcore.MarkRaw,
	op MatrixUnaryOp[T, P],
	stride uint64,
) {
	srcMatrix := memcore.MemcoreMarkDereferenceObjectUnsafe[Matrix[T]](srcAddr)
	dstMatrix := memcore.MemcoreMarkDereferenceObjectUnsafe[Matrix[P]](dstAddr)

	if srcMatrix.rows != dstMatrix.rows || srcMatrix.cols != dstMatrix.cols {
		panic(fmt.Errorf("cannot perform unary op: dimension mismatch (src=%dx%d, dst=%dx%d)",
			srcMatrix.rows, srcMatrix.cols, dstMatrix.rows, dstMatrix.cols))
	}

	VectorUnaryExecute(srcMatrix.data, dstMatrix.data, VectorUnaryOp[T, P](op), stride)
}

// MatrixBinaryReadOnlyExecute executes a stride of binary read-only operations
// between two matrices of equal dimensions.
//
//go:inline
func MatrixBinaryReadOnlyExecute[T, U foundation.Numeric](
	matrixAAddr, matrixBAddr memcore.MarkRaw,
	op MatrixBinaryReadOnlyOp[T, U],
	stride uint64,
) {
	aMatrix := memcore.MemcoreMarkDereferenceObjectUnsafe[Matrix[T]](matrixAAddr)
	bMatrix := memcore.MemcoreMarkDereferenceObjectUnsafe[Matrix[U]](matrixBAddr)

	if aMatrix.rows != bMatrix.rows || aMatrix.cols != bMatrix.cols {
		panic(fmt.Errorf("cannot perform binary op: dimension mismatch (a=%dx%d, b=%dx%d)",
			aMatrix.rows, aMatrix.cols, bMatrix.rows, bMatrix.cols))
	}

	VectorBinaryReadOnlyExecute(aMatrix.data, bMatrix.data, VectorBinaryReadOnlyOp[T, U](op), stride)
}

// MatrixBinaryExecute executes a stride of binary mutating operations
// between two source matrices and writes results into a destination matrix.
//
//go:inline
func MatrixBinaryExecute[T, U, P foundation.Numeric](
	matrixAAddr, matrixBAddr, destAddr memcore.MarkRaw,
	op MatrixBinaryOp[T, U, P],
	stride uint64,
) {
	aMatrix := memcore.MemcoreMarkDereferenceObjectUnsafe[Matrix[T]](matrixAAddr)
	bMatrix := memcore.MemcoreMarkDereferenceObjectUnsafe[Matrix[U]](matrixBAddr)
	dMatrix := memcore.MemcoreMarkDereferenceObjectUnsafe[Matrix[P]](destAddr)

	if aMatrix.rows != bMatrix.rows || aMatrix.cols != bMatrix.cols ||
		aMatrix.rows != dMatrix.rows || aMatrix.cols != dMatrix.cols {
		panic(fmt.Errorf("cannot perform binary op: dimension mismatch (a=%dx%d, b=%dx%d, d=%dx%d)",
			aMatrix.rows, aMatrix.cols, bMatrix.rows, bMatrix.cols, dMatrix.rows, dMatrix.cols))
	}

	VectorBinaryExecute(aMatrix.data, bMatrix.data, dMatrix.data, VectorBinaryOp[T, U, P](op), stride)
}
