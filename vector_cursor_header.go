package memstruct

import (
	"foundation"
	"memcore"
)

/*
VectorCursorHeader caches the Vector header pointer for hot-path operations that bypass MarkRaw dereference overhead.

This cursor type stores a direct pointer to the Vector header, eliminating the need to dereference
MarkRaw on every operation. Since Vector is structurally equivalent to Array, this cursor wraps
ArrayCursorHeader functionality for numeric types.

Use cases:
- High-performance loops with repeated vector access
- Batch processing operations with many vector operations
- Hot paths where memory stability is guaranteed
- Reducing overhead in performance-critical code sections

Time complexity: O(1) - structure initialization is constant-time
Space complexity: O(1) - fixed-size structure

Prerequisites:
- Must be created using VectorCursorHeaderCreate
- Vector memory must remain valid and unmoved for cursor lifetime
- Memory movement invalidates the cursor (undefined behavior if used after movement)

Edge cases:
- Cursor becomes invalid if vector memory is deallocated or unregistered
- Cursor becomes invalid if memory region is relocated
- No bounds checking in unsafe variants - caller must ensure indices are valid

Additional notes:
- Cursor caches the header pointer for maximum performance
- Uses unsafe pointer operations for direct memory access
- Type safety is maintained through the generic parameter T (must be foundation.Numeric)
- Suitable for both sequential and random access patterns
- The cursor is a value type and can be copied, but shares the same underlying header
*/
type VectorCursorHeader[T foundation.Numeric] struct {
	header *Vector[T]
}

/*
VectorCursorHeaderCreate creates a cursor that caches the Vector header pointer for hot-path operations.

This function performs a single MarkRaw dereference and caches the resulting header pointer,
enabling subsequent operations to bypass the dereference overhead. The cursor is optimized for
hot paths where memory movement is guaranteed not to occur during the cursor's lifetime.

Use cases:
- Setting up cursor-based iteration loops
- High-performance random access patterns
- Batch processing operations
- Performance-critical code sections

Time complexity: O(1) - single dereference operation
Space complexity: O(1) - returns a fixed-size cursor structure

Prerequisites:
- vector must be a valid MarkRaw pointing to an initialized Vector[T]
- Vector must not have been deallocated or unregistered
- Memory must remain stable during cursor lifetime

Edge cases:
- Cursor becomes invalid if vector memory is deallocated or unregistered
- Cursor becomes invalid if memory region is relocated
- Returns cursor with nil header if vector is invalid (undefined behavior)

Additional notes:
- Cursor caches header pointer for maximum performance
- Uses unsafe pointer operations for direct memory access
- No header manipulation required after cursor creation
- Type safety is maintained through the generic parameter T (must be foundation.Numeric)
- The cursor is a value type and can be copied, but shares the same underlying header
*/
//
//go:inline
func VectorCursorHeaderCreate[T foundation.Numeric](vector memcore.MarkRaw) VectorCursorHeader[T] {
	header := memcore.MemcoreMarkDereferenceObjectUnsafe[Vector[T]](vector)
	return VectorCursorHeader[T]{
		header: header,
	}
}

/*
VectorCursorHeaderItemGetAt returns the element at the given index within the vector.

This function uses the cached header pointer to access vector elements, bypassing MarkRaw
dereference overhead. It performs bounds checking and returns an error if the index is invalid.

Use cases:
- Cursor-based element access with bounds checking
- Safe random access patterns
- Error-handling code paths

Time complexity: O(1) - pointer arithmetic and bounds check
Space complexity: O(1) - no allocations

Prerequisites:
- cursor must be a valid VectorCursorHeader created with VectorCursorHeaderCreate
- idx must be in range [0, cursor.header.capacity)
- Vector memory must remain valid and unmoved

Edge cases:
- Returns error if idx exceeds capacity
- Returns zero value and error if index is invalid
- Undefined behavior if cursor is invalid or memory has moved

Additional notes:
- Uses cached header pointer for maximum performance
- Performs bounds checking before access
- Type safety is maintained through the generic parameter T (must be foundation.Numeric)
- Wraps ArrayCursorHeaderItemGetAt functionality
*/
//
//go:inline
func VectorCursorHeaderItemGetAt[T foundation.Numeric](cursor VectorCursorHeader[T], idx uint64) (T, error) {
	arrayCursor := ArrayCursorHeader[T]{
		header: (*Array[T])(cursor.header),
	}
	return ArrayCursorHeaderItemGetAt[T](arrayCursor, idx)
}

/*
VectorCursorHeaderItemGetAtUnsafe returns the element at the given index within the vector.

This function uses the cached header pointer to access vector elements, bypassing MarkRaw
dereference overhead. It performs no bounds checking, so callers must ensure the index is valid.

Use cases:
- High-performance hot paths with guaranteed valid indices
- Cursor-based element access in tight loops
- Performance-critical code sections

Time complexity: O(1) - pointer arithmetic only
Space complexity: O(1) - no allocations

Prerequisites:
- cursor must be a valid VectorCursorHeader created with VectorCursorHeaderCreate
- idx must be in range [0, cursor.header.capacity) (caller must validate)
- Vector memory must remain valid and unmoved

Edge cases:
- No bounds checking is performed - caller must ensure idx is valid
- Returns invalid value if idx exceeds capacity
- Undefined behavior if cursor is invalid or memory has moved

Additional notes:
- Uses cached header pointer for maximum performance
- No bounds checking for maximum speed
- Type safety is maintained through the generic parameter T (must be foundation.Numeric)
- Wraps ArrayCursorHeaderItemGetAtUnsafe functionality
*/
//
//go:inline
func VectorCursorHeaderItemGetAtUnsafe[T foundation.Numeric](cursor VectorCursorHeader[T], idx uint64) T {
	arrayCursor := ArrayCursorHeader[T]{
		header: (*Array[T])(cursor.header),
	}
	return ArrayCursorHeaderItemGetAtUnsafe[T](arrayCursor, idx)
}

/*
VectorCursorHeaderItemPtrGetAt returns a pointer to the element at the given index within the vector.

This function uses the cached header pointer to access vector elements, bypassing MarkRaw
dereference overhead. It performs bounds checking and returns an error if the index is invalid.

Use cases:
- Direct element manipulation through pointers
- Zero-copy element access
- Cursor-based pointer access with bounds checking

Time complexity: O(1) - pointer arithmetic and bounds check
Space complexity: O(1) - no allocations

Prerequisites:
- cursor must be a valid VectorCursorHeader created with VectorCursorHeaderCreate
- idx must be in range [0, cursor.header.capacity)
- Vector memory must remain valid and unmoved

Edge cases:
- Returns error if idx exceeds capacity
- Returns nil pointer and error if index is invalid
- Using the pointer after vector modification is undefined behavior
- Undefined behavior if cursor is invalid or memory has moved

Additional notes:
- Uses cached header pointer for maximum performance
- Performs bounds checking before access
- Returns pointer that can be used for both reading and writing
- Type safety is maintained through the generic parameter T (must be foundation.Numeric)
- Wraps ArrayCursorHeaderItemPtrGetAt functionality
*/
//
//go:inline
func VectorCursorHeaderItemPtrGetAt[T foundation.Numeric](cursor VectorCursorHeader[T], idx uint64) (*T, error) {
	arrayCursor := ArrayCursorHeader[T]{
		header: (*Array[T])(cursor.header),
	}
	return ArrayCursorHeaderItemPtrGetAt[T](arrayCursor, idx)
}

/*
VectorCursorHeaderItemPtrGetAtUnsafe returns a pointer to the element at the given index within the vector.

This function uses the cached header pointer to access vector elements, bypassing MarkRaw
dereference overhead. It performs no bounds checking, so callers must ensure the index is valid.

Use cases:
- High-performance hot paths with guaranteed valid indices
- Direct element manipulation in tight loops
- Zero-copy element access in performance-critical code

Time complexity: O(1) - pointer arithmetic only
Space complexity: O(1) - no allocations

Prerequisites:
- cursor must be a valid VectorCursorHeader created with VectorCursorHeaderCreate
- idx must be in range [0, cursor.header.capacity) (caller must validate)
- Vector memory must remain valid and unmoved

Edge cases:
- No bounds checking is performed - caller must ensure idx is valid
- Returns invalid pointer if idx exceeds capacity
- Using the pointer after vector modification is undefined behavior
- Undefined behavior if cursor is invalid or memory has moved

Additional notes:
- Uses cached header pointer for maximum performance
- No bounds checking for maximum speed
- Returns pointer that can be used for both reading and writing
- Type safety is maintained through the generic parameter T (must be foundation.Numeric)
- Wraps ArrayCursorHeaderItemPtrGetAtUnsafe functionality
*/
//
//go:inline
func VectorCursorHeaderItemPtrGetAtUnsafe[T foundation.Numeric](cursor VectorCursorHeader[T], idx uint64) *T {
	arrayCursor := ArrayCursorHeader[T]{
		header: (*Array[T])(cursor.header),
	}
	return ArrayCursorHeaderItemPtrGetAtUnsafe[T](arrayCursor, idx)
}

/*
VectorCursorHeaderItemSetAt sets the element at the given index to the specified value.

This function uses the cached header pointer to modify vector elements, bypassing MarkRaw
dereference overhead. It performs bounds checking and returns an error if the index is invalid.
The vector version is incremented after a successful set operation.

Use cases:
- Cursor-based element modification with bounds checking
- Safe random access write patterns
- Error-handling code paths

Time complexity: O(1) - pointer arithmetic, bounds check, and set operation
Space complexity: O(1) - no allocations

Prerequisites:
- cursor must be a valid VectorCursorHeader created with VectorCursorHeaderCreate
- idx must be in range [0, cursor.header.capacity)
- Vector memory must remain valid and unmoved

Edge cases:
- Returns error if idx exceeds capacity
- Increments vector version on successful set
- Undefined behavior if cursor is invalid or memory has moved

Additional notes:
- Uses cached header pointer for maximum performance
- Performs bounds checking before modification
- Uses registered set function for type-specific assignment
- Type safety is maintained through the generic parameter T (must be foundation.Numeric)
- Wraps ArrayCursorHeaderItemSetAt functionality
*/
//
//go:inline
func VectorCursorHeaderItemSetAt[T foundation.Numeric](cursor VectorCursorHeader[T], idx uint64, value T) error {
	arrayCursor := ArrayCursorHeader[T]{
		header: (*Array[T])(cursor.header),
	}
	return ArrayCursorHeaderItemSetAt[T](arrayCursor, idx, value)
}

/*
VectorCursorHeaderItemSetAtUnsafe sets the element at the given index to the specified value.

This function uses the cached header pointer to modify vector elements, bypassing MarkRaw
dereference overhead. It performs no bounds checking, so callers must ensure the index is valid.
The vector version is incremented after the set operation.

Use cases:
- High-performance hot paths with guaranteed valid indices
- Cursor-based element modification in tight loops
- Performance-critical code sections

Time complexity: O(1) - pointer arithmetic and set operation
Space complexity: O(1) - no allocations

Prerequisites:
- cursor must be a valid VectorCursorHeader created with VectorCursorHeaderCreate
- idx must be in range [0, cursor.header.capacity) (caller must validate)
- Vector memory must remain valid and unmoved

Edge cases:
- No bounds checking is performed - caller must ensure idx is valid
- Increments vector version on set
- Undefined behavior if cursor is invalid or memory has moved

Additional notes:
- Uses cached header pointer for maximum performance
- No bounds checking for maximum speed
- Uses registered set function for type-specific assignment
- Type safety is maintained through the generic parameter T (must be foundation.Numeric)
- Wraps ArrayCursorHeaderItemSetAtUnsafe functionality
*/
//
//go:inline
func VectorCursorHeaderItemSetAtUnsafe[T foundation.Numeric](cursor VectorCursorHeader[T], idx uint64, value T) {
	arrayCursor := ArrayCursorHeader[T]{
		header: (*Array[T])(cursor.header),
	}
	ArrayCursorHeaderItemSetAtUnsafe[T](arrayCursor, idx, value)
}

/*
VectorCursorHeaderCapacityGet returns the capacity of the vector.

This function uses the cached header pointer to access the vector capacity, bypassing MarkRaw
dereference overhead. The capacity represents the maximum number of elements that can be stored
in the vector.

Use cases:
- Cursor-based capacity queries
- Bounds checking before access
- Memory allocation planning

Time complexity: O(1) - returns cached value
Space complexity: O(1) - no allocations

Prerequisites:
- cursor must be a valid VectorCursorHeader created with VectorCursorHeaderCreate
- Vector memory must remain valid and unmoved

Edge cases:
- Returns the vector's capacity, which is fixed at initialization
- Capacity does not change after cursor creation
- Undefined behavior if cursor is invalid or memory has moved

Additional notes:
- Uses cached header pointer for maximum performance
- Capacity is stored in the vector header
- Type safety is maintained through the generic parameter T (must be foundation.Numeric)
- Wraps ArrayCursorHeaderCapacityGet functionality
*/
//
//go:inline
func VectorCursorHeaderCapacityGet[T foundation.Numeric](cursor VectorCursorHeader[T]) uint64 {
	arrayCursor := ArrayCursorHeader[T]{
		header: (*Array[T])(cursor.header),
	}
	return ArrayCursorHeaderCapacityGet[T](arrayCursor)
}

/*
VectorCursorHeaderVersionGet returns the current version of the vector.

This function uses the cached header pointer to access the vector version, bypassing MarkRaw
dereference overhead. The version increments on every modification to the vector data, allowing
cache invalidation mechanisms to detect when cached values become stale.

Use cases:
- Cache invalidation checks
- Detecting vector modifications
- Version-based synchronization

Time complexity: O(1) - returns cached value
Space complexity: O(1) - no allocations

Prerequisites:
- cursor must be a valid VectorCursorHeader created with VectorCursorHeaderCreate
- Vector memory must remain valid and unmoved

Edge cases:
- Returns the vector's current version number
- Version increments on every modification operation
- Undefined behavior if cursor is invalid or memory has moved

Additional notes:
- Uses cached header pointer for maximum performance
- Version is stored in the vector header
- Version starts at 1 when vector is initialized
- Type safety is maintained through the generic parameter T (must be foundation.Numeric)
- Wraps ArrayCursorHeaderVersionGet functionality
*/
//
//go:inline
func VectorCursorHeaderVersionGet[T foundation.Numeric](cursor VectorCursorHeader[T]) uint64 {
	arrayCursor := ArrayCursorHeader[T]{
		header: (*Array[T])(cursor.header),
	}
	return ArrayCursorHeaderVersionGet[T](arrayCursor)
}

