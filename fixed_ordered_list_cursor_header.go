package memstruct

import (
	"fmt"
	"memcore"
)

/*
FixedOrderedListCursorHeader caches the FixedOrderedList header pointer for hot-path operations that bypass MarkRaw dereference overhead.

This cursor type stores a direct pointer to the FixedOrderedList header, eliminating the need to dereference
MarkRaw on every operation. This is optimized for hot paths where memory movement is guaranteed
not to occur during the cursor's lifetime.

Use cases:
- High-performance loops with repeated fixed ordered list operations
- Batch processing operations with many list insert/delete operations
- Hot paths where memory stability is guaranteed
- Reducing overhead in performance-critical code sections

Time complexity: O(1) - structure initialization is constant-time
Space complexity: O(1) - fixed-size structure

Prerequisites:
- Must be created using FixedOrderedListCursorHeaderCreate
- FixedOrderedList memory must remain valid and unmoved for cursor lifetime
- Memory movement invalidates the cursor (undefined behavior if used after movement)

Edge cases:
- Cursor becomes invalid if list memory is deallocated or unregistered
- Cursor becomes invalid if memory region is relocated
- No bounds checking in unsafe variants - caller must ensure indices are valid

Additional notes:
- Cursor caches the header pointer for maximum performance
- Uses unsafe pointer operations for direct memory access
- Type safety is maintained through the generic parameter T
- Suitable for both sequential and random access patterns
- The cursor is a value type and can be copied, but shares the same underlying header
*/
type FixedOrderedListCursorHeader[T any] struct {
	header *FixedOrderedList[T]
}

/*
FixedOrderedListCursorHeaderCreate creates a cursor that caches the FixedOrderedList header pointer for hot-path operations.

This function performs a single MarkRaw dereference and caches the resulting header pointer,
enabling subsequent operations to bypass the dereference overhead. The cursor is optimized for
hot paths where memory movement is guaranteed not to occur during the cursor's lifetime.

Use cases:
- Setting up cursor-based fixed ordered list operations
- High-performance list manipulation patterns
- Batch processing operations
- Performance-critical code sections

Time complexity: O(1) - single dereference operation
Space complexity: O(1) - returns a fixed-size cursor structure

Prerequisites:
- list must be a valid MarkRaw pointing to an initialized FixedOrderedList[T]
- FixedOrderedList must not have been deallocated or unregistered
- Memory must remain stable during cursor lifetime

Edge cases:
- Cursor becomes invalid if list memory is deallocated or unregistered
- Cursor becomes invalid if memory region is relocated
- Returns cursor with nil header if list is invalid (undefined behavior)

Additional notes:
- Cursor caches header pointer for maximum performance
- Uses unsafe pointer operations for direct memory access
- No header manipulation required after cursor creation
- Type safety is maintained through the generic parameter T
- The cursor is a value type and can be copied, but shares the same underlying header
*/
//
//go:inline
func FixedOrderedListCursorHeaderCreate[T any](list memcore.MarkRaw) FixedOrderedListCursorHeader[T] {
	header := memcore.MemcoreMarkDereferenceObjectUnsafe[FixedOrderedList[T]](list)
	return FixedOrderedListCursorHeader[T]{
		header: header,
	}
}

/*
FixedOrderedListCursorHeaderInsert inserts a value at a given logical index, shifting subsequent elements to the right.

This function uses the cached header pointer to insert an item at the specified index, bypassing MarkRaw
dereference overhead. It performs bounds checking and returns an error if the index is invalid or the list is full.

Use cases:
- Cursor-based list insert operations with bounds checking
- Safe list manipulation patterns
- Error-handling code paths

Time complexity: O(n) - shifting elements to the right
Space complexity: O(1) - no allocations

Prerequisites:
- cursor must be a valid FixedOrderedListCursorHeader created with FixedOrderedListCursorHeaderCreate
- idx must be in range [0, cursor.header.length] (can insert at end)
- FixedOrderedList must not be at capacity
- FixedOrderedList memory must remain valid and unmoved

Edge cases:
- Returns error if index is out of bounds
- Returns error if list is at capacity
- Increments list version on successful insert
- Undefined behavior if cursor is invalid or memory has moved

Additional notes:
- Uses cached header pointer for maximum performance
- Performs bounds checking before insert
- Shifts subsequent elements to the right
- Type safety is maintained through the generic parameter T
*/
//
//go:inline
func FixedOrderedListCursorHeaderInsert[T any](cursor FixedOrderedListCursorHeader[T], idx uint64, value T) error {
	if err := fixedListGuaranteeIdxInsertionValidity(cursor.header, idx); err != nil {
		return err
	}
	if cursor.header.length >= cursor.header.capacity {
		return fmt.Errorf("no space left in fixed list")
	}
	slot := StackPopUnsafe[uint64](cursor.header.freeList)
	ArraySetAtUnsafe(cursor.header.dataArray, slot, value)
	fixedListInsertIndex(cursor.header, idx, slot)
	cursor.header.length++
	cursor.header.version++
	return nil
}

/*
FixedOrderedListCursorHeaderDelete removes an element at logical index and shifts remaining elements left by one slot.

This function uses the cached header pointer to delete an item at the specified index, bypassing MarkRaw
dereference overhead. It performs bounds checking and returns an error if the index is invalid.

Use cases:
- Cursor-based list delete operations with bounds checking
- Safe list manipulation patterns
- Error-handling code paths

Time complexity: O(n) - shifting elements to the left
Space complexity: O(1) - no allocations

Prerequisites:
- cursor must be a valid FixedOrderedListCursorHeader created with FixedOrderedListCursorHeaderCreate
- idx must be in range [0, cursor.header.length)
- FixedOrderedList memory must remain valid and unmoved

Edge cases:
- Returns error if index is out of bounds
- Increments list version on successful delete
- Returns the slot to the freelist
- Undefined behavior if cursor is invalid or memory has moved

Additional notes:
- Uses cached header pointer for maximum performance
- Performs bounds checking before delete
- Shifts subsequent elements to the left
- Type safety is maintained through the generic parameter T
*/
//
//go:inline
func FixedOrderedListCursorHeaderDelete[T any](cursor FixedOrderedListCursorHeader[T], idx uint64) error {
	if err := fixedListGuaranteeIdxReadValidity(cursor.header, idx); err != nil {
		return err
	}
	slot := ArrayItemGetAtUnsafe[uint64](cursor.header.indices, idx)
	fixedListRemoveIndex(cursor.header, idx)
	StackPushUnsafe(cursor.header.freeList, slot)
	cursor.header.length--
	cursor.header.version++
	return nil
}

/*
FixedOrderedListCursorHeaderGetAt retrieves the element at logical index.

This function uses the cached header pointer to access list elements, bypassing MarkRaw
dereference overhead. It performs bounds checking and returns an error if the index is invalid.

Use cases:
- Cursor-based element access with bounds checking
- Safe random access patterns
- Error-handling code paths

Time complexity: O(1) - direct access via index table
Space complexity: O(1) - no allocations

Prerequisites:
- cursor must be a valid FixedOrderedListCursorHeader created with FixedOrderedListCursorHeaderCreate
- idx must be in range [0, cursor.header.length)
- FixedOrderedList memory must remain valid and unmoved

Edge cases:
- Returns error if index is out of bounds
- Returns zero value and error if index is invalid
- Undefined behavior if cursor is invalid or memory has moved

Additional notes:
- Uses cached header pointer for maximum performance
- Performs bounds checking before access
- Accesses element via index table lookup
- Type safety is maintained through the generic parameter T
*/
//
//go:inline
func FixedOrderedListCursorHeaderGetAt[T any](cursor FixedOrderedListCursorHeader[T], idx uint64) (T, error) {
	if err := fixedListGuaranteeIdxReadValidity(cursor.header, idx); err != nil {
		var zero T
		return zero, err
	}
	return fixedListGetElement(cursor.header, idx), nil
}

/*
FixedOrderedListCursorHeaderLengthGet returns the current length of the fixed ordered list.

This function uses the cached header pointer to access the list length, bypassing MarkRaw
dereference overhead. The length represents the number of items currently in the list.

Use cases:
- Cursor-based length queries
- Bounds checking before operations
- Loop iteration limits

Time complexity: O(1) - returns cached value
Space complexity: O(1) - no allocations

Prerequisites:
- cursor must be a valid FixedOrderedListCursorHeader created with FixedOrderedListCursorHeaderCreate
- FixedOrderedList memory must remain valid and unmoved

Edge cases:
- Returns the list's current length, which may be less than capacity
- Length does not change after cursor creation unless list is modified
- Undefined behavior if cursor is invalid or memory has moved

Additional notes:
- Uses cached header pointer for maximum performance
- Length is stored in the list header
- Type safety is maintained through the generic parameter T
*/
//
//go:inline
func FixedOrderedListCursorHeaderLengthGet[T any](cursor FixedOrderedListCursorHeader[T]) uint64 {
	return cursor.header.length
}

/*
FixedOrderedListCursorHeaderVersionGet returns the current version of the fixed ordered list.

This function uses the cached header pointer to access the list version, bypassing MarkRaw
dereference overhead. The version increments on every modification to the list data, allowing
cache invalidation mechanisms to detect when cached values become stale.

Use cases:
- Cache invalidation checks
- Detecting list modifications
- Version-based synchronization

Time complexity: O(1) - returns cached value
Space complexity: O(1) - no allocations

Prerequisites:
- cursor must be a valid FixedOrderedListCursorHeader created with FixedOrderedListCursorHeaderCreate
- FixedOrderedList memory must remain valid and unmoved

Edge cases:
- Returns the list's current version number
- Version increments on every modification operation
- Undefined behavior if cursor is invalid or memory has moved

Additional notes:
- Uses cached header pointer for maximum performance
- Version is stored in the list header
- Version starts at 1 when list is initialized
- Type safety is maintained through the generic parameter T
*/
//
//go:inline
func FixedOrderedListCursorHeaderVersionGet[T any](cursor FixedOrderedListCursorHeader[T]) uint64 {
	return cursor.header.version
}

