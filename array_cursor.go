package memstruct

import (
	"memcore"
	"unsafe"
)

/*
ArrayCursor represents a fast-access cursor for efficient random access to array elements.

This structure caches the base data pointer, element size, and capacity, enabling fast pointer
arithmetic without repeated header dereferencing. The cursor provides O(1) random access to
any element in the array through direct pointer arithmetic, avoiding the overhead of function
calls and header lookups in tight loops.

Use cases:
- High-performance random access to array elements
- Batch processing with non-sequential access patterns
- SIMD-optimized operations requiring direct pointer access
- Reducing overhead in performance-critical loops
- Zero-copy element access for read/write operations

Time complexity: O(1) - structure initialization is constant-time
Space complexity: O(1) - fixed-size structure

Prerequisites:
- Must be created using ArrayCursorCreate
- Array must remain valid for the lifetime of the cursor

Edge cases:
- Cursor becomes invalid if array memory is deallocated or unregistered
- No bounds checking is performed - caller must ensure indices are valid
- Capacity reflects the array's capacity, not its current length

Additional notes:
- Cursor caches the data base pointer and item size for maximum performance
- Uses unsafe pointer arithmetic for direct memory access
- Type safety is maintained through the generic parameter T
- Suitable for both sequential and random access patterns
*/
type ArrayCursor[T any] struct {
	dataBase unsafe.Pointer
	itemSize uintptr
	capacity uint64
}

/*
ArrayCursorCreate creates a fast-access cursor around an Array[T].

This function initializes a cursor that caches the base data pointer and element size, enabling
fast pointer arithmetic without dereferencing the array header each time. The cursor provides
O(1) random access to any element through direct pointer arithmetic, eliminating function call
overhead and header lookups in performance-critical code paths.

Use cases:
- Setting up cursor-based iteration loops
- High-performance random access patterns
- Batch processing operations
- SIMD-optimized data processing
- Zero-copy element manipulation

Time complexity: O(1) - calculates offset and creates cursor
Space complexity: O(1) - returns a fixed-size cursor structure

Prerequisites:
- array must be a valid MarkRaw pointing to an initialized Array[T]
- Array must not have been deallocated or unregistered

Edge cases:
- Cursor becomes invalid if array memory is deallocated or unregistered
- Data pointer points directly to the array's data region
- Capacity reflects the array's capacity, not its current length

Additional notes:
- Cursor caches data pointer and item size for maximum performance
- Uses unsafe pointer arithmetic for direct memory access
- No header manipulation required after cursor creation
- Type safety is maintained through the generic parameter T
- The cursor is a value type and can be copied, but shares the same underlying data
*/
//
//go:inline
func ArrayCursorCreate[T any](array memcore.MarkRaw) ArrayCursor[T] {
	base, instance := memcore.MemcoreMarkDereferenceObjectAltUnsafe[Array[T]](array)
	return ArrayCursor[T]{
		dataBase: unsafe.Add(base, instance.dataAddrOffset),
		itemSize: instance.itemSize,
		capacity: instance.capacity,
	}
}

/*
PtrAt returns a pointer to the element at the given index.

This function uses pointer arithmetic to calculate the address of the element at the specified
index, using the cached base pointer and item size. This avoids any header dereferencing or
function calls, providing maximum performance for random access patterns.

Use cases:
- Direct element access in cursor-based iteration loops
- High-performance batch operations
- SIMD-optimized data processing
- Zero-copy read/write operations

Time complexity: O(1) - pointer arithmetic only
Space complexity: O(1) - no allocations

Prerequisites:
- cursor must be a valid ArrayCursor created with ArrayCursorCreate
- idx must be in range [0, cursor.capacity)

Edge cases:
- No bounds checking is performed - caller must ensure idx is valid
- Returns invalid pointer if idx exceeds capacity
- Pointer becomes invalid if array memory is deallocated or unregistered
- Modifying elements through the pointer directly affects the array data

Additional notes:
- Uses cached base pointer and item size for maximum performance
- Returns a pointer that can be used for both reading and writing
- Type safety is maintained through the generic parameter T
- The returned pointer remains valid as long as the array and cursor are valid
*/
//
//go:inline
func (c *ArrayCursor[T]) PtrAt(idx uint64) *T {
	return (*T)(unsafe.Add(c.dataBase, uintptr(idx)*c.itemSize))
}

/*
Capacity returns the number of elements that can be stored in the array.

This function returns the capacity of the array that the cursor was created from. The capacity
represents the maximum number of elements that can be stored, not the current length of valid
elements in the array.

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
- Capacity is cached at cursor creation time
- The value reflects the array's capacity at the time the cursor was created
- Type safety is maintained through the generic parameter T
*/
//
//go:inline
func (c *ArrayCursor[T]) Capacity() uint64 {
	return c.capacity
}

/*
ArrayForwardCursor represents a forward-only iterator for efficient sequential iteration over array elements.

This structure caches the current position pointer, end pointer, and element size, enabling ultra-fast
sequential iteration with minimal overhead. The cursor uses simple pointer comparison for bounds checking
and pointer addition for advancement, avoiding multiplication operations in the hot loop.

Use cases:
- High-performance sequential iteration over all elements
- Batch processing of contiguous elements
- SIMD-optimized operations on sequential data
- Reducing overhead in tight iteration loops
- Forward-only traversal patterns

Time complexity: O(1) - structure initialization is constant-time
Space complexity: O(1) - fixed-size structure

Prerequisites:
- Must be created using ArrayForwardCursorCreate
- Array must remain valid for the lifetime of the cursor

Edge cases:
- Cursor becomes invalid if array memory is deallocated or unregistered
- Cursor can only iterate forward, not backward
- Iteration covers the array's capacity, not its current length

Additional notes:
- Cursor uses pointer arithmetic for maximum performance
- Bounds checking is a simple pointer comparison (no index calculations)
- Advancement uses only pointer addition (no multiplication in hot loop)
- Type safety is maintained through the generic parameter T
- Suitable only for forward sequential access patterns
*/
type ArrayForwardCursor[T any] struct {
	currentPtr unsafe.Pointer
	endPtr     unsafe.Pointer
	itemSize   uintptr
}

/*
ArrayForwardCursorCreate creates a forward-only iterator cursor for efficient sequential iteration.

This function initializes a cursor that caches the start and end pointers along with element size,
enabling ultra-fast sequential iteration. The cursor uses simple pointer comparison for bounds checking
and pointer addition for advancement, avoiding multiplication operations in the iteration loop.

Use cases:
- Setting up forward-only iteration loops
- High-performance sequential access patterns
- Batch processing operations
- SIMD-optimized sequential data processing

Time complexity: O(1) - calculates pointers and creates cursor
Space complexity: O(1) - returns a fixed-size cursor structure

Prerequisites:
- array must be a valid MarkRaw pointing to an initialized Array[T]
- Array must not have been deallocated or unregistered

Edge cases:
- Cursor becomes invalid if array memory is deallocated or unregistered
- Iteration covers the array's capacity, not its current length
- Cursor can only iterate forward, not backward

Additional notes:
- Cursor caches start and end pointers for maximum performance
- Uses pointer arithmetic for bounds checking and advancement
- No multiplication in the hot loop (only pointer addition)
- Type safety is maintained through the generic parameter T
- The cursor is a value type and can be copied, but shares the same underlying data
*/
func ArrayForwardCursorCreate[T any](array memcore.MarkRaw) ArrayForwardCursor[T] {
	base, instance := memcore.MemcoreMarkDereferenceObjectAltUnsafe[Array[T]](array)
	start := unsafe.Add(base, instance.dataAddrOffset)
	size := uintptr(instance.capacity) * instance.itemSize
	return ArrayForwardCursor[T]{
		currentPtr: start,
		endPtr:     unsafe.Add(start, size),
		itemSize:   instance.itemSize,
	}
}

/*
HasNext checks if there are more elements to iterate over.

This function performs a simple pointer comparison to determine if the cursor has reached the end
of the array. The comparison is extremely fast as it requires no index calculations or arithmetic
operations, only a direct pointer comparison.

Use cases:
- Loop condition for forward iteration
- Bounds checking before accessing elements
- Early termination of iteration loops

Time complexity: O(1) - simple pointer comparison
Space complexity: O(1) - no allocations

Prerequisites:
- cursor must be a valid ArrayForwardCursor created with ArrayForwardCursorCreate

Edge cases:
- Returns false when currentPtr reaches or exceeds endPtr
- Returns true as long as there are more elements to process
- Comparison is based on pointer addresses, not element count

Additional notes:
- Uses simple pointer comparison for maximum performance
- No arithmetic operations required
- Type safety is maintained through the generic parameter T
- Should be called before each Next() call in iteration loops
*/
//
//go:inline
func (c *ArrayForwardCursor[T]) HasNext() bool {
	return uintptr(c.currentPtr) < uintptr(c.endPtr)
}

/*
Next returns a pointer to the current element and advances the cursor to the next element.

This function returns a pointer to the current element and then advances the cursor by one element
using pointer addition. The advancement uses only pointer addition (no multiplication), making it
extremely efficient for sequential iteration. The returned pointer can be used for both reading and
writing the element value.

Use cases:
- Sequential element access in iteration loops
- High-performance batch operations
- SIMD-optimized data processing
- Zero-copy element manipulation

Time complexity: O(1) - pointer addition only
Space complexity: O(1) - no allocations

Prerequisites:
- cursor must be a valid ArrayForwardCursor created with ArrayForwardCursorCreate
- HasNext() should return true before calling Next()

Edge cases:
- No bounds checking is performed - caller must ensure HasNext() returns true
- Returns invalid pointer if called after reaching the end
- Modifying elements through the pointer directly affects the array data
- Cursor advances forward only, cannot go backward

Additional notes:
- Uses only pointer addition for advancement (no multiplication in hot loop)
- Returns a pointer that can be used for both reading and writing
- Type safety is maintained through the generic parameter T
- The returned pointer remains valid as long as the array and cursor are valid
- Should be called in a loop with HasNext() as the condition
*/
//
//go:inline
func (c *ArrayForwardCursor[T]) Next() *T {
	ptr := c.currentPtr
	c.currentPtr = unsafe.Add(c.currentPtr, c.itemSize)
	return (*T)(ptr)
}
