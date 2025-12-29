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
