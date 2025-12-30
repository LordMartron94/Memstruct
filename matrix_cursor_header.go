package memstruct

import (
	"fmt"
	"foundation"
	"memcore"
)

/*
MatrixCursorHeader caches the Matrix header pointer for hot-path operations that bypass MarkRaw dereference overhead.

This cursor type stores a direct pointer to the Matrix header, eliminating the need to dereference
MarkRaw on every operation. This is optimized for hot paths where memory movement is guaranteed
not to occur during the cursor's lifetime.

Use cases:
- High-performance loops with repeated matrix access
- Batch processing operations with many matrix operations
- Hot paths where memory stability is guaranteed
- Reducing overhead in performance-critical code sections

Time complexity: O(1) - structure initialization is constant-time
Space complexity: O(1) - fixed-size structure

Prerequisites:
- Must be created using MatrixCursorHeaderCreate
- Matrix memory must remain valid and unmoved for cursor lifetime
- Memory movement invalidates the cursor (undefined behavior if used after movement)

Edge cases:
- Cursor becomes invalid if matrix memory is deallocated or unregistered
- Cursor becomes invalid if memory region is relocated
- No bounds checking in unsafe variants - caller must ensure indices are valid

Additional notes:
- Cursor caches the header pointer for maximum performance
- Uses unsafe pointer operations for direct memory access
- Type safety is maintained through the generic parameter T (must be foundation.Numeric)
- Suitable for both sequential and random access patterns
- The cursor is a value type and can be copied, but shares the same underlying header
*/
type MatrixCursorHeader[T foundation.Numeric] struct {
	header *Matrix[T]
}

/*
MatrixCursorHeaderCreate creates a cursor that caches the Matrix header pointer for hot-path operations.

This function performs a single MarkRaw dereference and caches the resulting header pointer,
enabling subsequent operations to bypass the dereference overhead. The cursor is optimized for
hot paths where memory movement is guaranteed not to occur during the cursor's lifetime.

Use cases:
- Setting up cursor-based matrix operations
- High-performance matrix manipulation patterns
- Batch processing operations
- Performance-critical code sections

Time complexity: O(1) - single dereference operation
Space complexity: O(1) - returns a fixed-size cursor structure

Prerequisites:
- matrix must be a valid MarkRaw pointing to an initialized Matrix[T]
- Matrix must not have been deallocated or unregistered
- Memory must remain stable during cursor lifetime

Edge cases:
- Cursor becomes invalid if matrix memory is deallocated or unregistered
- Cursor becomes invalid if memory region is relocated
- Returns cursor with nil header if matrix is invalid (undefined behavior)

Additional notes:
- Cursor caches header pointer for maximum performance
- Uses unsafe pointer operations for direct memory access
- No header manipulation required after cursor creation
- Type safety is maintained through the generic parameter T (must be foundation.Numeric)
- The cursor is a value type and can be copied, but shares the same underlying header
*/
//
//go:inline
func MatrixCursorHeaderCreate[T foundation.Numeric](matrix memcore.MarkRaw) MatrixCursorHeader[T] {
	header := memcore.MemcoreMarkDereferenceObjectUnsafe[Matrix[T]](matrix)
	return MatrixCursorHeader[T]{
		header: header,
	}
}

/*
MatrixCursorHeaderGetAt returns the element at the given (row, col) position within the matrix.

This function uses the cached header pointer to access matrix elements, bypassing MarkRaw
dereference overhead. It performs bounds checking and returns an error if the indices are invalid.

Use cases:
- Cursor-based element access with bounds checking
- Safe random access patterns
- Error-handling code paths

Time complexity: O(1) - pointer arithmetic and bounds check
Space complexity: O(1) - no allocations

Prerequisites:
- cursor must be a valid MatrixCursorHeader created with MatrixCursorHeaderCreate
- row must be in range [0, cursor.header.rows)
- col must be in range [0, cursor.header.cols)
- Matrix memory must remain valid and unmoved

Edge cases:
- Returns error if row or col exceeds matrix dimensions
- Returns zero value and error if indices are invalid
- Undefined behavior if cursor is invalid or memory has moved

Additional notes:
- Uses cached header pointer for maximum performance
- Performs bounds checking before access
- Converts (row, col) to linear index using row-major ordering
- Type safety is maintained through the generic parameter T (must be foundation.Numeric)
*/
//
//go:inline
func MatrixCursorHeaderGetAt[T foundation.Numeric](cursor MatrixCursorHeader[T], row, col uint64) (T, error) {
	if row >= cursor.header.rows || col >= cursor.header.cols {
		var zero T
		return zero, fmt.Errorf("MatrixCursorHeaderGetAt: index out of bounds (row=%d, col=%d, matrix=%dx%d)",
			row, col, cursor.header.rows, cursor.header.cols)
	}

	idx := matrixRowColToIdx(row, col, cursor.header.cols)
	return VectorItemGetAt[T](cursor.header.data, idx)
}

/*
MatrixCursorHeaderGetAtUnsafe returns the element at the given (row, col) position within the matrix.

This function uses the cached header pointer to access matrix elements, bypassing MarkRaw
dereference overhead. It performs no bounds checking, so callers must ensure the indices are valid.

Use cases:
- High-performance hot paths with guaranteed valid indices
- Cursor-based element access in tight loops
- Performance-critical code sections

Time complexity: O(1) - pointer arithmetic only
Space complexity: O(1) - no allocations

Prerequisites:
- cursor must be a valid MatrixCursorHeader created with MatrixCursorHeaderCreate
- row must be in range [0, cursor.header.rows) (caller must validate)
- col must be in range [0, cursor.header.cols) (caller must validate)
- Matrix memory must remain valid and unmoved

Edge cases:
- No bounds checking is performed - caller must ensure indices are valid
- Returns invalid value if indices exceed matrix dimensions
- Undefined behavior if cursor is invalid or memory has moved

Additional notes:
- Uses cached header pointer for maximum performance
- No bounds checking for maximum speed
- Converts (row, col) to linear index using row-major ordering
- Type safety is maintained through the generic parameter T (must be foundation.Numeric)
*/
//
//go:inline
func MatrixCursorHeaderGetAtUnsafe[T foundation.Numeric](cursor MatrixCursorHeader[T], row, col uint64) T {
	idx := matrixRowColToIdx(row, col, cursor.header.cols)
	return VectorItemGetAtUnsafe[T](cursor.header.data, idx)
}

/*
MatrixCursorHeaderSetAt sets the element at the given (row, col) position to the specified value.

This function uses the cached header pointer to modify matrix elements, bypassing MarkRaw
dereference overhead. It performs bounds checking and returns an error if the indices are invalid.

Use cases:
- Cursor-based element modification with bounds checking
- Safe random access write patterns
- Error-handling code paths

Time complexity: O(1) - pointer arithmetic, bounds check, and set operation
Space complexity: O(1) - no allocations

Prerequisites:
- cursor must be a valid MatrixCursorHeader created with MatrixCursorHeaderCreate
- row must be in range [0, cursor.header.rows)
- col must be in range [0, cursor.header.cols)
- Matrix memory must remain valid and unmoved

Edge cases:
- Returns error if row or col exceeds matrix dimensions
- Undefined behavior if cursor is invalid or memory has moved

Additional notes:
- Uses cached header pointer for maximum performance
- Performs bounds checking before modification
- Converts (row, col) to linear index using row-major ordering
- Type safety is maintained through the generic parameter T (must be foundation.Numeric)
*/
//
//go:inline
func MatrixCursorHeaderSetAt[T foundation.Numeric](cursor MatrixCursorHeader[T], row, col uint64, value T) error {
	if row >= cursor.header.rows || col >= cursor.header.cols {
		return fmt.Errorf("MatrixCursorHeaderSetAt: index out of bounds (row=%d, col=%d, matrix=%dx%d)",
			row, col, cursor.header.rows, cursor.header.cols)
	}

	idx := matrixRowColToIdx(row, col, cursor.header.cols)
	return VectorSetAt(cursor.header.data, idx, value)
}

/*
MatrixCursorHeaderSetAtUnsafe sets the element at the given (row, col) position to the specified value.

This function uses the cached header pointer to modify matrix elements, bypassing MarkRaw
dereference overhead. It performs no bounds checking, so callers must ensure the indices are valid.

Use cases:
- High-performance hot paths with guaranteed valid indices
- Cursor-based element modification in tight loops
- Performance-critical code sections

Time complexity: O(1) - pointer arithmetic and set operation
Space complexity: O(1) - no allocations

Prerequisites:
- cursor must be a valid MatrixCursorHeader created with MatrixCursorHeaderCreate
- row must be in range [0, cursor.header.rows) (caller must validate)
- col must be in range [0, cursor.header.cols) (caller must validate)
- Matrix memory must remain valid and unmoved

Edge cases:
- No bounds checking is performed - caller must ensure indices are valid
- Undefined behavior if cursor is invalid or memory has moved

Additional notes:
- Uses cached header pointer for maximum performance
- No bounds checking for maximum speed
- Converts (row, col) to linear index using row-major ordering
- Type safety is maintained through the generic parameter T (must be foundation.Numeric)
*/
//
//go:inline
func MatrixCursorHeaderSetAtUnsafe[T foundation.Numeric](cursor MatrixCursorHeader[T], row, col uint64, value T) {
	idx := matrixRowColToIdx(row, col, cursor.header.cols)
	VectorSetAtUnsafe(cursor.header.data, idx, value)
}

/*
MatrixCursorHeaderRowsGet returns the number of rows in the matrix.

This function uses the cached header pointer to access the matrix row count, bypassing MarkRaw
dereference overhead.

Use cases:
- Cursor-based row count queries
- Bounds checking before operations
- Loop iteration limits

Time complexity: O(1) - returns cached value
Space complexity: O(1) - no allocations

Prerequisites:
- cursor must be a valid MatrixCursorHeader created with MatrixCursorHeaderCreate
- Matrix memory must remain valid and unmoved

Edge cases:
- Returns the matrix's row count, which is fixed at initialization
- Row count does not change after cursor creation
- Undefined behavior if cursor is invalid or memory has moved

Additional notes:
- Uses cached header pointer for maximum performance
- Row count is stored in the matrix header
- Type safety is maintained through the generic parameter T (must be foundation.Numeric)
*/
//
//go:inline
func MatrixCursorHeaderRowsGet[T foundation.Numeric](cursor MatrixCursorHeader[T]) uint64 {
	return cursor.header.rows
}

/*
MatrixCursorHeaderColsGet returns the number of columns in the matrix.

This function uses the cached header pointer to access the matrix column count, bypassing MarkRaw
dereference overhead.

Use cases:
- Cursor-based column count queries
- Bounds checking before operations
- Loop iteration limits

Time complexity: O(1) - returns cached value
Space complexity: O(1) - no allocations

Prerequisites:
- cursor must be a valid MatrixCursorHeader created with MatrixCursorHeaderCreate
- Matrix memory must remain valid and unmoved

Edge cases:
- Returns the matrix's column count, which is fixed at initialization
- Column count does not change after cursor creation
- Undefined behavior if cursor is invalid or memory has moved

Additional notes:
- Uses cached header pointer for maximum performance
- Column count is stored in the matrix header
- Type safety is maintained through the generic parameter T (must be foundation.Numeric)
*/
//
//go:inline
func MatrixCursorHeaderColsGet[T foundation.Numeric](cursor MatrixCursorHeader[T]) uint64 {
	return cursor.header.cols
}

