package memstruct

import (
	"fmt"
	"memcore"
)

/*
StackCursorHeader caches the Stack header pointer for hot-path operations that bypass MarkRaw dereference overhead.

This cursor type stores a direct pointer to the Stack header, eliminating the need to dereference
MarkRaw on every operation. This is optimized for hot paths where memory movement is guaranteed
not to occur during the cursor's lifetime.

Use cases:
- High-performance loops with repeated stack operations
- Batch processing operations with many stack push/pop operations
- Hot paths where memory stability is guaranteed
- Reducing overhead in performance-critical code sections

Time complexity: O(1) - structure initialization is constant-time
Space complexity: O(1) - fixed-size structure

Prerequisites:
- Must be created using StackCursorHeaderCreate
- Stack memory must remain valid and unmoved for cursor lifetime
- Memory movement invalidates the cursor (undefined behavior if used after movement)

Edge cases:
- Cursor becomes invalid if stack memory is deallocated or unregistered
- Cursor becomes invalid if memory region is relocated
- No bounds checking in unsafe variants - caller must ensure stack is not empty/full

Additional notes:
- Cursor caches the header pointer for maximum performance
- Uses unsafe pointer operations for direct memory access
- Type safety is maintained through the generic parameter T
- Suitable for both sequential and random access patterns
- The cursor is a value type and can be copied, but shares the same underlying header
*/
type StackCursorHeader[T any] struct {
	header *Stack[T]
}

/*
StackCursorHeaderCreate creates a cursor that caches the Stack header pointer for hot-path operations.

This function performs a single MarkRaw dereference and caches the resulting header pointer,
enabling subsequent operations to bypass the dereference overhead. The cursor is optimized for
hot paths where memory movement is guaranteed not to occur during the cursor's lifetime.

Use cases:
- Setting up cursor-based stack operations
- High-performance stack manipulation patterns
- Batch processing operations
- Performance-critical code sections

Time complexity: O(1) - single dereference operation
Space complexity: O(1) - returns a fixed-size cursor structure

Prerequisites:
- stack must be a valid MarkRaw pointing to an initialized Stack[T]
- Stack must not have been deallocated or unregistered
- Memory must remain stable during cursor lifetime

Edge cases:
- Cursor becomes invalid if stack memory is deallocated or unregistered
- Cursor becomes invalid if memory region is relocated
- Returns cursor with nil header if stack is invalid (undefined behavior)

Additional notes:
- Cursor caches header pointer for maximum performance
- Uses unsafe pointer operations for direct memory access
- No header manipulation required after cursor creation
- Type safety is maintained through the generic parameter T
- The cursor is a value type and can be copied, but shares the same underlying header
*/
//
//go:inline
func StackCursorHeaderCreate[T any](stack memcore.MarkRaw) StackCursorHeader[T] {
	header := memcore.MemcoreMarkDereferenceObjectUnsafe[Stack[T]](stack)
	return StackCursorHeader[T]{
		header: header,
	}
}

/*
StackCursorHeaderPush pushes an item onto the stack.

This function uses the cached header pointer to push an item onto the stack, bypassing MarkRaw
dereference overhead. It performs bounds checking and returns an error if the stack is full.

Use cases:
- Cursor-based stack push operations with bounds checking
- Safe stack manipulation patterns
- Error-handling code paths

Time complexity: O(1) - constant time push operation
Space complexity: O(1) - no allocations

Prerequisites:
- cursor must be a valid StackCursorHeader created with StackCursorHeaderCreate
- Stack must not be at capacity
- Stack memory must remain valid and unmoved

Edge cases:
- Returns error if stack is at capacity (stack overflow)
- Increments stack length on successful push
- Undefined behavior if cursor is invalid or memory has moved

Additional notes:
- Uses cached header pointer for maximum performance
- Performs bounds checking before push
- Type safety is maintained through the generic parameter T
*/
//
//go:inline
func StackCursorHeaderPush[T any](cursor StackCursorHeader[T], item T) error {
	if cursor.header.length >= cursor.header.capacity {
		return fmt.Errorf("stack overflow: capacity %d", cursor.header.capacity)
	}

	ArraySetAtUnsafe(cursor.header.data, cursor.header.length, item)
	cursor.header.length++
	return nil
}

/*
StackCursorHeaderPushUnsafe pushes an item onto the stack without bounds checking.

This function uses the cached header pointer to push an item onto the stack, bypassing MarkRaw
dereference overhead. It performs no bounds checking, so callers must ensure the stack is not full.

Use cases:
- High-performance hot paths with guaranteed stack capacity
- Cursor-based stack push operations in tight loops
- Performance-critical code sections

Time complexity: O(1) - constant time push operation
Space complexity: O(1) - no allocations

Prerequisites:
- cursor must be a valid StackCursorHeader created with StackCursorHeaderCreate
- Stack must not be at capacity (caller must validate)
- Stack memory must remain valid and unmoved

Edge cases:
- No bounds checking is performed - caller must ensure stack is not full
- Increments stack length on push
- Undefined behavior if cursor is invalid or memory has moved

Additional notes:
- Uses cached header pointer for maximum performance
- No bounds checking for maximum speed
- Type safety is maintained through the generic parameter T
*/
//
//go:inline
func StackCursorHeaderPushUnsafe[T any](cursor StackCursorHeader[T], item T) {
	ArraySetAtUnsafe(cursor.header.data, cursor.header.length, item)
	cursor.header.length++
}

/*
StackCursorHeaderPop pops an item from the stack.

This function uses the cached header pointer to pop an item from the stack, bypassing MarkRaw
dereference overhead. It performs bounds checking and returns an error if the stack is empty.

Use cases:
- Cursor-based stack pop operations with bounds checking
- Safe stack manipulation patterns
- Error-handling code paths

Time complexity: O(1) - constant time pop operation
Space complexity: O(1) - no allocations

Prerequisites:
- cursor must be a valid StackCursorHeader created with StackCursorHeaderCreate
- Stack must not be empty
- Stack memory must remain valid and unmoved

Edge cases:
- Returns error if stack is empty (stack underflow)
- Decrements stack length on successful pop
- Returns zero value and error if stack is empty
- Undefined behavior if cursor is invalid or memory has moved

Additional notes:
- Uses cached header pointer for maximum performance
- Performs bounds checking before pop
- Type safety is maintained through the generic parameter T
*/
//
//go:inline
func StackCursorHeaderPop[T any](cursor StackCursorHeader[T]) (T, error) {
	if cursor.header.length == 0 {
		var zero T
		return zero, fmt.Errorf("stack underflow: empty stack")
	}

	item := ArrayItemGetAtUnsafe[T](cursor.header.data, cursor.header.length-1)
	cursor.header.length--
	return item, nil
}

/*
StackCursorHeaderPopUnsafe pops an item from the stack without bounds checking.

This function uses the cached header pointer to pop an item from the stack, bypassing MarkRaw
dereference overhead. It performs no bounds checking, so callers must ensure the stack is not empty.

Use cases:
- High-performance hot paths with guaranteed non-empty stack
- Cursor-based stack pop operations in tight loops
- Performance-critical code sections

Time complexity: O(1) - constant time pop operation
Space complexity: O(1) - no allocations

Prerequisites:
- cursor must be a valid StackCursorHeader created with StackCursorHeaderCreate
- Stack must not be empty (caller must validate)
- Stack memory must remain valid and unmoved

Edge cases:
- No bounds checking is performed - caller must ensure stack is not empty
- Decrements stack length on pop
- Undefined behavior if cursor is invalid or memory has moved

Additional notes:
- Uses cached header pointer for maximum performance
- No bounds checking for maximum speed
- Type safety is maintained through the generic parameter T
*/
//
//go:inline
func StackCursorHeaderPopUnsafe[T any](cursor StackCursorHeader[T]) T {
	item := ArrayItemGetAtUnsafe[T](cursor.header.data, cursor.header.length-1)
	cursor.header.length--
	return item
}

/*
StackCursorHeaderLengthGet returns the current length of the stack.

This function uses the cached header pointer to access the stack length, bypassing MarkRaw
dereference overhead. The length represents the number of items currently on the stack.

Use cases:
- Cursor-based length queries
- Bounds checking before operations
- Loop iteration limits

Time complexity: O(1) - returns cached value
Space complexity: O(1) - no allocations

Prerequisites:
- cursor must be a valid StackCursorHeader created with StackCursorHeaderCreate
- Stack memory must remain valid and unmoved

Edge cases:
- Returns the stack's current length, which may be less than capacity
- Length does not change after cursor creation unless stack is modified
- Undefined behavior if cursor is invalid or memory has moved

Additional notes:
- Uses cached header pointer for maximum performance
- Length is stored in the stack header
- Type safety is maintained through the generic parameter T
*/
//
//go:inline
func StackCursorHeaderLengthGet[T any](cursor StackCursorHeader[T]) uint64 {
	return cursor.header.length
}

/*
StackCursorHeaderIsEmpty returns whether the stack is empty.

This function uses the cached header pointer to check if the stack is empty, bypassing MarkRaw
dereference overhead.

Use cases:
- Cursor-based empty checks
- Conditional operations based on stack state
- Loop termination conditions

Time complexity: O(1) - simple comparison
Space complexity: O(1) - no allocations

Prerequisites:
- cursor must be a valid StackCursorHeader created with StackCursorHeaderCreate
- Stack memory must remain valid and unmoved

Edge cases:
- Returns true if stack length is 0
- Returns false if stack has any items
- Undefined behavior if cursor is invalid or memory has moved

Additional notes:
- Uses cached header pointer for maximum performance
- Equivalent to checking if length == 0
- Type safety is maintained through the generic parameter T
*/
//
//go:inline
func StackCursorHeaderIsEmpty[T any](cursor StackCursorHeader[T]) bool {
	return cursor.header.length == 0
}

/*
StackCursorHeaderCapacityGet returns the capacity of the stack.

This function uses the cached header pointer to access the stack capacity, bypassing MarkRaw
dereference overhead. The capacity represents the maximum number of items that can be stored
on the stack.

Use cases:
- Cursor-based capacity queries
- Bounds checking before operations
- Memory allocation planning

Time complexity: O(1) - returns cached value
Space complexity: O(1) - no allocations

Prerequisites:
- cursor must be a valid StackCursorHeader created with StackCursorHeaderCreate
- Stack memory must remain valid and unmoved

Edge cases:
- Returns the stack's capacity, which is fixed at initialization
- Capacity does not change after cursor creation
- Undefined behavior if cursor is invalid or memory has moved

Additional notes:
- Uses cached header pointer for maximum performance
- Capacity is stored in the stack header
- Type safety is maintained through the generic parameter T
*/
//
//go:inline
func StackCursorHeaderCapacityGet[T any](cursor StackCursorHeader[T]) uint64 {
	return cursor.header.capacity
}

