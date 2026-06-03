package memstruct

import (
	"fmt"
	"foundation"
	"unsafe"
)

// MatrixItemGetAtFast returns T at (row, col) using a pre-dereferenced matrix header and vector storage.
//
//go:nosplit
//go:inline
func MatrixItemGetAtFast[T foundation.Numeric](matrix *Matrix[T], dataInst *Array[T], dataBase unsafe.Pointer, row, col uint64) (T, error) {
	if row >= matrix.rows || col >= matrix.cols {
		var zero T
		return zero, fmt.Errorf("MatrixItemGetAtFast: index out of bounds (row=%d, col=%d, matrix=%dx%d)",
			row, col, matrix.rows, matrix.cols)
	}
	idx := matrixRowColToIdx(row, col, matrix.cols)
	if err := arrayGuaranteeIdxValidity(dataInst, idx); err != nil {
		var zero T
		return zero, err
	}
	return *(*T)(arrayGetPtrAtIdx(dataInst, dataBase, idx)), nil
}

// MatrixItemGetAtUnsafeFast returns T at (row, col) without bounds checks.
//
//go:inline
func MatrixItemGetAtUnsafeFast[T foundation.Numeric](matrix *Matrix[T], dataInst *Array[T], dataBase unsafe.Pointer, row, col uint64) T {
	idx := matrixRowColToIdx(row, col, matrix.cols)
	return VectorItemGetAtUnsafeFast(dataInst, dataBase, idx)
}

// MatrixItemPtrGetAtFast returns a pointer to T at (row, col).
//
//go:nosplit
//go:inline
func MatrixItemPtrGetAtFast[T foundation.Numeric](matrix *Matrix[T], dataInst *Array[T], dataBase unsafe.Pointer, row, col uint64) (*T, error) {
	if row >= matrix.rows || col >= matrix.cols {
		return nil, fmt.Errorf("MatrixItemPtrGetAtFast: index out of bounds (row=%d, col=%d, matrix=%dx%d)",
			row, col, matrix.rows, matrix.cols)
	}
	idx := matrixRowColToIdx(row, col, matrix.cols)
	if err := arrayGuaranteeIdxValidity(dataInst, idx); err != nil {
		return nil, err
	}
	return (*T)(arrayGetPtrAtIdx(dataInst, dataBase, idx)), nil
}

// MatrixItemPtrGetAtUnsafeFast returns a pointer at (row, col) without bounds checks.
//
//go:inline
func MatrixItemPtrGetAtUnsafeFast[T foundation.Numeric](matrix *Matrix[T], dataInst *Array[T], dataBase unsafe.Pointer, row, col uint64) *T {
	idx := matrixRowColToIdx(row, col, matrix.cols)
	return (*T)(arrayGetPtrAtIdx(dataInst, dataBase, idx))
}

// MatrixRowsGetFast returns row count from a pre-dereferenced matrix header.
//
//go:inline
func MatrixRowsGetFast[T foundation.Numeric](matrix *Matrix[T]) uint64 {
	return matrix.rows
}

// MatrixColsGetFast returns column count from a pre-dereferenced matrix header.
//
//go:inline
func MatrixColsGetFast[T foundation.Numeric](matrix *Matrix[T]) uint64 {
	return matrix.cols
}
