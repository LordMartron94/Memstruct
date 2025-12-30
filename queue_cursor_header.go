package memstruct

import (
	"fmt"
	"memcore"
)

/*
QueueCursorHeader caches the Queue header pointer for hot-path operations that bypass MarkRaw dereference overhead.

This cursor type stores a direct pointer to the Queue header, eliminating the need to dereference
MarkRaw on every operation. This is optimized for hot paths where memory movement is guaranteed
not to occur during the cursor's lifetime.

Use cases:
- High-performance loops with repeated queue operations
- Batch processing operations with many queue enqueue/dequeue operations
- Hot paths where memory stability is guaranteed
- Reducing overhead in performance-critical code sections

Time complexity: O(1) - structure initialization is constant-time
Space complexity: O(1) - fixed-size structure

Prerequisites:
- Must be created using QueueCursorHeaderCreate
- Queue memory must remain valid and unmoved for cursor lifetime
- Memory movement invalidates the cursor (undefined behavior if used after movement)

Edge cases:
- Cursor becomes invalid if queue memory is deallocated or unregistered
- Cursor becomes invalid if memory region is relocated
- No bounds checking in unsafe variants - caller must ensure queue is not empty/full

Additional notes:
- Cursor caches the header pointer for maximum performance
- Uses unsafe pointer operations for direct memory access
- Type safety is maintained through the generic parameter T
- Suitable for both sequential and random access patterns
- The cursor is a value type and can be copied, but shares the same underlying header
*/
type QueueCursorHeader[T any] struct {
	header *Queue[T]
}

/*
QueueCursorHeaderCreate creates a cursor that caches the Queue header pointer for hot-path operations.

This function performs a single MarkRaw dereference and caches the resulting header pointer,
enabling subsequent operations to bypass the dereference overhead. The cursor is optimized for
hot paths where memory movement is guaranteed not to occur during the cursor's lifetime.

Use cases:
- Setting up cursor-based queue operations
- High-performance queue manipulation patterns
- Batch processing operations
- Performance-critical code sections

Time complexity: O(1) - single dereference operation
Space complexity: O(1) - returns a fixed-size cursor structure

Prerequisites:
- queue must be a valid MarkRaw pointing to an initialized Queue[T]
- Queue must not have been deallocated or unregistered
- Memory must remain stable during cursor lifetime

Edge cases:
- Cursor becomes invalid if queue memory is deallocated or unregistered
- Cursor becomes invalid if memory region is relocated
- Returns cursor with nil header if queue is invalid (undefined behavior)

Additional notes:
- Cursor caches header pointer for maximum performance
- Uses unsafe pointer operations for direct memory access
- No header manipulation required after cursor creation
- Type safety is maintained through the generic parameter T
- The cursor is a value type and can be copied, but shares the same underlying header
*/
//
//go:inline
func QueueCursorHeaderCreate[T any](queue memcore.MarkRaw) QueueCursorHeader[T] {
	header := memcore.MemcoreMarkDereferenceObjectUnsafe[Queue[T]](queue)
	return QueueCursorHeader[T]{
		header: header,
	}
}

/*
QueueCursorHeaderEnqueue enqueues an item into the queue.

This function uses the cached header pointer to enqueue an item into the queue, bypassing MarkRaw
dereference overhead. It performs bounds checking and returns an error if the queue is full.

Use cases:
- Cursor-based queue enqueue operations with bounds checking
- Safe queue manipulation patterns
- Error-handling code paths

Time complexity: O(1) - constant time enqueue operation
Space complexity: O(1) - no allocations

Prerequisites:
- cursor must be a valid QueueCursorHeader created with QueueCursorHeaderCreate
- Queue must not be at capacity
- Queue memory must remain valid and unmoved

Edge cases:
- Returns error if queue is at capacity (queue overflow)
- Increments queue length on successful enqueue
- Undefined behavior if cursor is invalid or memory has moved

Additional notes:
- Uses cached header pointer for maximum performance
- Performs bounds checking before enqueue
- Implements circular buffer logic for wrap-around
- Type safety is maintained through the generic parameter T
*/
//
//go:inline
func QueueCursorHeaderEnqueue[T any](cursor QueueCursorHeader[T], item T) error {
	if cursor.header.length >= cursor.header.capacity {
		return fmt.Errorf("queue overflow: capacity %d", cursor.header.capacity)
	}

	ArraySetAtUnsafe(cursor.header.data, cursor.header.tail, item)
	cursor.header.tail = (cursor.header.tail + 1) % cursor.header.capacity
	cursor.header.length++
	return nil
}

/*
QueueCursorHeaderEnqueueUnsafe enqueues an item into the queue without bounds checking.

This function uses the cached header pointer to enqueue an item into the queue, bypassing MarkRaw
dereference overhead. It performs no bounds checking, so callers must ensure the queue is not full.

Use cases:
- High-performance hot paths with guaranteed queue capacity
- Cursor-based queue enqueue operations in tight loops
- Performance-critical code sections

Time complexity: O(1) - constant time enqueue operation
Space complexity: O(1) - no allocations

Prerequisites:
- cursor must be a valid QueueCursorHeader created with QueueCursorHeaderCreate
- Queue must not be at capacity (caller must validate)
- Queue memory must remain valid and unmoved

Edge cases:
- No bounds checking is performed - caller must ensure queue is not full
- Increments queue length on enqueue
- Implements circular buffer logic for wrap-around
- Undefined behavior if cursor is invalid or memory has moved

Additional notes:
- Uses cached header pointer for maximum performance
- No bounds checking for maximum speed
- Implements circular buffer logic for wrap-around
- Type safety is maintained through the generic parameter T
*/
//
//go:inline
func QueueCursorHeaderEnqueueUnsafe[T any](cursor QueueCursorHeader[T], item T) {
	ArraySetAtUnsafe(cursor.header.data, cursor.header.tail, item)
	cursor.header.tail = (cursor.header.tail + 1) % cursor.header.capacity
	cursor.header.length++
}

/*
QueueCursorHeaderDequeue dequeues an item from the queue.

This function uses the cached header pointer to dequeue an item from the queue, bypassing MarkRaw
dereference overhead. It performs bounds checking and returns an error if the queue is empty.

Use cases:
- Cursor-based queue dequeue operations with bounds checking
- Safe queue manipulation patterns
- Error-handling code paths

Time complexity: O(1) - constant time dequeue operation
Space complexity: O(1) - no allocations

Prerequisites:
- cursor must be a valid QueueCursorHeader created with QueueCursorHeaderCreate
- Queue must not be empty
- Queue memory must remain valid and unmoved

Edge cases:
- Returns error if queue is empty (queue underflow)
- Decrements queue length on successful dequeue
- Returns zero value and error if queue is empty
- Implements circular buffer logic for wrap-around
- Undefined behavior if cursor is invalid or memory has moved

Additional notes:
- Uses cached header pointer for maximum performance
- Performs bounds checking before dequeue
- Implements circular buffer logic for wrap-around
- Type safety is maintained through the generic parameter T
*/
//
//go:inline
func QueueCursorHeaderDequeue[T any](cursor QueueCursorHeader[T]) (T, error) {
	if cursor.header.length == 0 {
		var zero T
		return zero, fmt.Errorf("queue underflow: empty queue")
	}

	item := ArrayItemGetAtUnsafe[T](cursor.header.data, cursor.header.head)
	cursor.header.head = (cursor.header.head + 1) % cursor.header.capacity
	cursor.header.length--
	return item, nil
}

/*
QueueCursorHeaderDequeueUnsafe dequeues an item from the queue without bounds checking.

This function uses the cached header pointer to dequeue an item from the queue, bypassing MarkRaw
dereference overhead. It performs no bounds checking, so callers must ensure the queue is not empty.

Use cases:
- High-performance hot paths with guaranteed non-empty queue
- Cursor-based queue dequeue operations in tight loops
- Performance-critical code sections

Time complexity: O(1) - constant time dequeue operation
Space complexity: O(1) - no allocations

Prerequisites:
- cursor must be a valid QueueCursorHeader created with QueueCursorHeaderCreate
- Queue must not be empty (caller must validate)
- Queue memory must remain valid and unmoved

Edge cases:
- No bounds checking is performed - caller must ensure queue is not empty
- Decrements queue length on dequeue
- Implements circular buffer logic for wrap-around
- Undefined behavior if cursor is invalid or memory has moved

Additional notes:
- Uses cached header pointer for maximum performance
- No bounds checking for maximum speed
- Implements circular buffer logic for wrap-around
- Type safety is maintained through the generic parameter T
*/
//
//go:inline
func QueueCursorHeaderDequeueUnsafe[T any](cursor QueueCursorHeader[T]) T {
	item := ArrayItemGetAtUnsafe[T](cursor.header.data, cursor.header.head)
	cursor.header.head = (cursor.header.head + 1) % cursor.header.capacity
	cursor.header.length--
	return item
}

/*
QueueCursorHeaderLengthGet returns the current length of the queue.

This function uses the cached header pointer to access the queue length, bypassing MarkRaw
dereference overhead. The length represents the number of items currently in the queue.

Use cases:
- Cursor-based length queries
- Bounds checking before operations
- Loop iteration limits

Time complexity: O(1) - returns cached value
Space complexity: O(1) - no allocations

Prerequisites:
- cursor must be a valid QueueCursorHeader created with QueueCursorHeaderCreate
- Queue memory must remain valid and unmoved

Edge cases:
- Returns the queue's current length, which may be less than capacity
- Length does not change after cursor creation unless queue is modified
- Undefined behavior if cursor is invalid or memory has moved

Additional notes:
- Uses cached header pointer for maximum performance
- Length is stored in the queue header
- Type safety is maintained through the generic parameter T
*/
//
//go:inline
func QueueCursorHeaderLengthGet[T any](cursor QueueCursorHeader[T]) uint64 {
	return cursor.header.length
}

/*
QueueCursorHeaderIsEmpty returns whether the queue is empty.

This function uses the cached header pointer to check if the queue is empty, bypassing MarkRaw
dereference overhead.

Use cases:
- Cursor-based empty checks
- Conditional operations based on queue state
- Loop termination conditions

Time complexity: O(1) - simple comparison
Space complexity: O(1) - no allocations

Prerequisites:
- cursor must be a valid QueueCursorHeader created with QueueCursorHeaderCreate
- Queue memory must remain valid and unmoved

Edge cases:
- Returns true if queue length is 0
- Returns false if queue has any items
- Undefined behavior if cursor is invalid or memory has moved

Additional notes:
- Uses cached header pointer for maximum performance
- Equivalent to checking if length == 0
- Type safety is maintained through the generic parameter T
*/
//
//go:inline
func QueueCursorHeaderIsEmpty[T any](cursor QueueCursorHeader[T]) bool {
	return cursor.header.length == 0
}

/*
QueueCursorHeaderCapacityGet returns the capacity of the queue.

This function uses the cached header pointer to access the queue capacity, bypassing MarkRaw
dereference overhead. The capacity represents the maximum number of items that can be stored
in the queue.

Use cases:
- Cursor-based capacity queries
- Bounds checking before operations
- Memory allocation planning

Time complexity: O(1) - returns cached value
Space complexity: O(1) - no allocations

Prerequisites:
- cursor must be a valid QueueCursorHeader created with QueueCursorHeaderCreate
- Queue memory must remain valid and unmoved

Edge cases:
- Returns the queue's capacity, which is fixed at initialization
- Capacity does not change after cursor creation
- Undefined behavior if cursor is invalid or memory has moved

Additional notes:
- Uses cached header pointer for maximum performance
- Capacity is stored in the queue header
- Type safety is maintained through the generic parameter T
*/
//
//go:inline
func QueueCursorHeaderCapacityGet[T any](cursor QueueCursorHeader[T]) uint64 {
	return cursor.header.capacity
}
