package memstruct

import (
	"memcore"
	"unsafe"
)

/*
ArrayCursor caches the Array header pointer for hot-path operations that bypass MarkRaw dereference overhead.

This cursor type stores a direct pointer to the Array header, eliminating the need to dereference
MarkRaw on every operation. This is optimized for hot paths where memory movement is guaranteed
not to occur during the cursor's lifetime.

Use cases:
- High-performance loops with repeated array access
- Batch processing operations with many array operations
- Hot paths where memory stability is guaranteed
- Reducing overhead in performance-critical code sections

Time complexity: O(1) - structure initialization is constant-time
Space complexity: O(1) - fixed-size structure

Prerequisites:
- Must be created using ArrayCursorCreate
- Array memory must remain valid and unmoved for cursor lifetime
- Memory movement invalidates the cursor (undefined behavior if used after movement)

Edge cases:
- Cursor becomes invalid if array memory is deallocated or unregistered
- Cursor becomes invalid if memory region is relocated
- No bounds checking in unsafe variants - caller must ensure indices are valid

Additional notes:
- Cursor caches the header pointer for maximum performance
- Uses unsafe pointer operations for direct memory access
- Type safety is maintained through the generic parameter T
- Suitable for both sequential and random access patterns
- The cursor is a value type and can be copied, but shares the same underlying header
*/
type ArrayCursor[T any] struct {
	header *Array[T]
}

/*
ArrayCursorCreate creates a cursor that caches the Array header pointer for hot-path operations.

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
- array must be a valid MarkRaw pointing to an initialized Array[T]
- Array must not have been deallocated or unregistered
- Memory must remain stable during cursor lifetime

Edge cases:
- Cursor becomes invalid if array memory is deallocated or unregistered
- Cursor becomes invalid if memory region is relocated
- Returns cursor with nil header if array is invalid (undefined behavior)

Additional notes:
- Cursor caches header pointer for maximum performance
- Uses unsafe pointer operations for direct memory access
- No header manipulation required after cursor creation
- Type safety is maintained through the generic parameter T
- The cursor is a value type and can be copied, but shares the same underlying header
*/
//
//go:inline
func ArrayCursorCreate[T any](array memcore.MarkRaw) ArrayCursor[T] {
	header := memcore.MemcoreMarkDereferenceObjectUnsafe[Array[T]](array)
	return ArrayCursor[T]{
		header: header,
	}
}

/*
ArrayCursorItemGetAt returns the element at the given index within the array.

This function uses the cached header pointer to access array elements, bypassing MarkRaw
dereference overhead. It performs bounds checking and returns an error if the index is invalid.

Use cases:
- Cursor-based element access with bounds checking
- Safe random access patterns
- Error-handling code paths

Time complexity: O(1) - pointer arithmetic and bounds check
Space complexity: O(1) - no allocations

Prerequisites:
- cursor must be a valid ArrayCursor created with ArrayCursorCreate
- idx must be in range [0, cursor.header.capacity)
- Array memory must remain valid and unmoved

Edge cases:
- Returns error if idx exceeds capacity
- Returns zero value and error if index is invalid
- Undefined behavior if cursor is invalid or memory has moved

Additional notes:
- Uses cached header pointer for maximum performance
- Performs bounds checking before access
- Type safety is maintained through the generic parameter T
*/
//
//go:inline
func ArrayCursorItemGetAt[T any](cursor ArrayCursor[T], idx uint64) (T, error) {
	if err := arrayGuaranteeIdxValidity(cursor.header, idx); err != nil {
		var zero T
		return zero, err
	}

	baseAddr := unsafe.Pointer(cursor.header)
	itemPtr := arrayGetPtrAtIdx(cursor.header, baseAddr, idx)
	return *(*T)(itemPtr), nil
}

/*
ArrayCursorItemGetAtUnsafe returns the element at the given index within the array.

This function uses the cached header pointer to access array elements, bypassing MarkRaw
dereference overhead. It performs no bounds checking, so callers must ensure the index is valid.

Use cases:
- High-performance hot paths with guaranteed valid indices
- Cursor-based element access in tight loops
- Performance-critical code sections

Time complexity: O(1) - pointer arithmetic only
Space complexity: O(1) - no allocations

Prerequisites:
- cursor must be a valid ArrayCursor created with ArrayCursorCreate
- idx must be in range [0, cursor.header.capacity) (caller must validate)
- Array memory must remain valid and unmoved

Edge cases:
- No bounds checking is performed - caller must ensure idx is valid
- Returns invalid value if idx exceeds capacity
- Undefined behavior if cursor is invalid or memory has moved

Additional notes:
- Uses cached header pointer for maximum performance
- No bounds checking for maximum speed
- Type safety is maintained through the generic parameter T
*/
//
//go:inline
func ArrayCursorItemGetAtUnsafe[T any](cursor ArrayCursor[T], idx uint64) T {
	baseAddr := unsafe.Pointer(cursor.header)
	itemPtr := arrayGetPtrAtIdx(cursor.header, baseAddr, idx)
	return *(*T)(itemPtr)
}

/*
ArrayCursorItemPtrGetAt returns a pointer to the element at the given index within the array.

This function uses the cached header pointer to access array elements, bypassing MarkRaw
dereference overhead. It performs bounds checking and returns an error if the index is invalid.

Use cases:
- Direct element manipulation through pointers
- Zero-copy element access
- Cursor-based pointer access with bounds checking

Time complexity: O(1) - pointer arithmetic and bounds check
Space complexity: O(1) - no allocations

Prerequisites:
- cursor must be a valid ArrayCursor created with ArrayCursorCreate
- idx must be in range [0, cursor.header.capacity)
- Array memory must remain valid and unmoved

Edge cases:
- Returns error if idx exceeds capacity
- Returns nil pointer and error if index is invalid
- Using the pointer after array modification is undefined behavior
- Undefined behavior if cursor is invalid or memory has moved

Additional notes:
- Uses cached header pointer for maximum performance
- Performs bounds checking before access
- Returns pointer that can be used for both reading and writing
- Type safety is maintained through the generic parameter T
*/
//
//go:inline
func ArrayCursorItemPtrGetAt[T any](cursor ArrayCursor[T], idx uint64) (*T, error) {
	if err := arrayGuaranteeIdxValidity(cursor.header, idx); err != nil {
		return nil, err
	}

	baseAddr := unsafe.Pointer(cursor.header)
	itemPtr := arrayGetPtrAtIdx(cursor.header, baseAddr, idx)
	return (*T)(itemPtr), nil
}

/*
ArrayCursorItemPtrGetAtUnsafe returns a pointer to the element at the given index within the array.

This function uses the cached header pointer to access array elements, bypassing MarkRaw
dereference overhead. It performs no bounds checking, so callers must ensure the index is valid.

Use cases:
- High-performance hot paths with guaranteed valid indices
- Direct element manipulation in tight loops
- Zero-copy element access in performance-critical code

Time complexity: O(1) - pointer arithmetic only
Space complexity: O(1) - no allocations

Prerequisites:
- cursor must be a valid ArrayCursor created with ArrayCursorCreate
- idx must be in range [0, cursor.header.capacity) (caller must validate)
- Array memory must remain valid and unmoved

Edge cases:
- No bounds checking is performed - caller must ensure idx is valid
- Returns invalid pointer if idx exceeds capacity
- Using the pointer after array modification is undefined behavior
- Undefined behavior if cursor is invalid or memory has moved

Additional notes:
- Uses cached header pointer for maximum performance
- No bounds checking for maximum speed
- Returns pointer that can be used for both reading and writing
- Type safety is maintained through the generic parameter T
*/
//
//go:inline
func ArrayCursorItemPtrGetAtUnsafe[T any](cursor ArrayCursor[T], idx uint64) *T {
	baseAddr := unsafe.Pointer(cursor.header)
	itemPtr := arrayGetPtrAtIdx(cursor.header, baseAddr, idx)
	return (*T)(itemPtr)
}

/*
ArrayCursorItemSetAt sets the element at the given index to the specified value.

This function uses the cached header pointer to modify array elements, bypassing MarkRaw
dereference overhead. It performs bounds checking and returns an error if the index is invalid.
The array version is incremented after a successful set operation.

Use cases:
- Cursor-based element modification with bounds checking
- Safe random access write patterns
- Error-handling code paths

Time complexity: O(1) - pointer arithmetic, bounds check, and set operation
Space complexity: O(1) - no allocations

Prerequisites:
- cursor must be a valid ArrayCursor created with ArrayCursorCreate
- idx must be in range [0, cursor.header.capacity)
- Array memory must remain valid and unmoved

Edge cases:
- Returns error if idx exceeds capacity
- Increments array version on successful set
- Undefined behavior if cursor is invalid or memory has moved

Additional notes:
- Uses cached header pointer for maximum performance
- Performs bounds checking before modification
- Uses registered set function for type-specific assignment
- Type safety is maintained through the generic parameter T
*/
//
//go:inline
func ArrayCursorItemSetAt[T any](cursor ArrayCursor[T], idx uint64, value T) error {
	if err := arrayGuaranteeIdxValidity(cursor.header, idx); err != nil {
		return err
	}

	baseAddr := unsafe.Pointer(cursor.header)
	itemPtr := arrayGetPtrAtIdx(cursor.header, baseAddr, idx)
	memcore.MemcoreFunctionRetrieveTyped[setFn[T]](cursor.header.setFnID)(itemPtr, value)

	cursor.header.version++
	return nil
}

/*
ArrayCursorItemSetAtUnsafe sets the element at the given index to the specified value.

This function uses the cached header pointer to modify array elements, bypassing MarkRaw
dereference overhead. It performs no bounds checking, so callers must ensure the index is valid.
The array version is incremented after the set operation.

Use cases:
- High-performance hot paths with guaranteed valid indices
- Cursor-based element modification in tight loops
- Performance-critical code sections

Time complexity: O(1) - pointer arithmetic and set operation
Space complexity: O(1) - no allocations

Prerequisites:
- cursor must be a valid ArrayCursor created with ArrayCursorCreate
- idx must be in range [0, cursor.header.capacity) (caller must validate)
- Array memory must remain valid and unmoved

Edge cases:
- No bounds checking is performed - caller must ensure idx is valid
- Increments array version on set
- Undefined behavior if cursor is invalid or memory has moved

Additional notes:
- Uses cached header pointer for maximum performance
- No bounds checking for maximum speed
- Uses registered set function for type-specific assignment
- Type safety is maintained through the generic parameter T
*/
//
//go:inline
func ArrayCursorItemSetAtUnsafe[T any](cursor ArrayCursor[T], idx uint64, value T) {
	baseAddr := unsafe.Pointer(cursor.header)
	itemPtr := arrayGetPtrAtIdx(cursor.header, baseAddr, idx)
	memcore.MemcoreFunctionRetrieveTyped[setFn[T]](cursor.header.setFnID)(itemPtr, value)

	cursor.header.version++
}

/*
ArrayCursorCapacityGet returns the capacity of the array.

This function uses the cached header pointer to access the array capacity, bypassing MarkRaw
dereference overhead. The capacity represents the maximum number of elements that can be stored
in the array.

Use cases:
- Cursor-based capacity queries
- Bounds checking before access
- Memory allocation planning

Time complexity: O(1) - returns cached value
Space complexity: O(1) - no allocations

Prerequisites:
- cursor must be a valid ArrayCursor created with ArrayCursorCreate
- Array memory must remain valid and unmoved

Edge cases:
- Returns the array's capacity, which is fixed at initialization
- Capacity does not change after cursor creation
- Undefined behavior if cursor is invalid or memory has moved

Additional notes:
- Uses cached header pointer for maximum performance
- Capacity is stored in the array header
- Type safety is maintained through the generic parameter T
*/
//
//go:inline
func ArrayCursorCapacityGet[T any](cursor ArrayCursor[T]) uint64 {
	return cursor.header.capacity
}

/*
ArrayCursorVersionGet returns the current version of the array.

This function uses the cached header pointer to access the array version, bypassing MarkRaw
dereference overhead. The version increments on every modification to the array data, allowing
cache invalidation mechanisms to detect when cached values become stale.

Use cases:
- Cache invalidation checks
- Detecting array modifications
- Version-based synchronization

Time complexity: O(1) - returns cached value
Space complexity: O(1) - no allocations

Prerequisites:
- cursor must be a valid ArrayCursor created with ArrayCursorCreate
- Array memory must remain valid and unmoved

Edge cases:
- Returns the array's current version number
- Version increments on every modification operation
- Undefined behavior if cursor is invalid or memory has moved

Additional notes:
- Uses cached header pointer for maximum performance
- Version is stored in the array header
- Version starts at 1 when array is initialized
- Type safety is maintained through the generic parameter T
*/
//
//go:inline
func ArrayCursorVersionGet[T any](cursor ArrayCursor[T]) uint64 {
	return cursor.header.version
}

/*
PtrAt returns a pointer to the element at the given index.

This method uses the cached header pointer to access array elements, bypassing MarkRaw
dereference overhead. It performs no bounds checking, so callers must ensure the index is valid.

Use cases:
- Direct element access in cursor-based iteration loops
- High-performance batch operations
- SIMD-optimized data processing
- Zero-copy read/write operations

Time complexity: O(1) - pointer arithmetic only
Space complexity: O(1) - no allocations

Prerequisites:
- cursor must be a valid ArrayCursor created with ArrayCursorCreate
- idx must be in range [0, cursor.header.capacity) (caller must validate)

Edge cases:
- No bounds checking is performed - caller must ensure idx is valid
- Returns invalid pointer if idx exceeds capacity
- Pointer becomes invalid if array memory is deallocated or unregistered
- Modifying elements through the pointer directly affects the array data

Additional notes:
- Uses cached header pointer for maximum performance
- Returns a pointer that can be used for both reading and writing
- Type safety is maintained through the generic parameter T
- The returned pointer remains valid as long as the array and cursor are valid
- This method provides the same functionality as ArrayCursorItemPtrGetAtUnsafe
*/
//
//go:inline
func (c *ArrayCursor[T]) PtrAt(idx uint64) *T {
	return ArrayCursorItemPtrGetAtUnsafe[T](*c, idx)
}

/*
Capacity returns the number of elements that can be stored in the array.

This method uses the cached header pointer to access the array capacity, bypassing MarkRaw
dereference overhead. The capacity represents the maximum number of elements that can be stored,
not the current length of valid elements in the array.

Use cases:
- Bounds checking before accessing elements
- Loop iteration limits
- Memory allocation planning
- Validation of index ranges

Time complexity: O(1) - returns cached value
Space complexity: O(1) - no allocations

Prerequisites:
- cursor must be a valid ArrayCursor created with ArrayCursorCreate

Edge cases:
- Returns the array's capacity, which may be greater than the number of valid elements
- Capacity does not change after cursor creation, even if the array is modified

Additional notes:
- Uses cached header pointer for maximum performance
- Capacity is stored in the array header
- The value reflects the array's capacity at the time the cursor was created
- Type safety is maintained through the generic parameter T
- This method provides the same functionality as ArrayCursorCapacityGet
*/
//
//go:inline
func (c *ArrayCursor[T]) Capacity() uint64 {
	return ArrayCursorCapacityGet[T](*c)
}
