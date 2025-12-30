package memstruct

import (
	"fmt"
	"memcore"
)

/*
PriorityQueueCursorHeader caches the PriorityQueue header pointer for hot-path operations that bypass MarkRaw dereference overhead.

This cursor type stores a direct pointer to the PriorityQueue header, eliminating the need to dereference
MarkRaw on every operation. This is optimized for hot paths where memory movement is guaranteed
not to occur during the cursor's lifetime.

Use cases:
- High-performance loops with repeated priority queue operations
- Batch processing operations with many priority queue insert/extract operations
- Hot paths where memory stability is guaranteed
- Reducing overhead in performance-critical code sections

Time complexity: O(1) - structure initialization is constant-time
Space complexity: O(1) - fixed-size structure

Prerequisites:
- Must be created using PriorityQueueCursorHeaderCreate
- PriorityQueue memory must remain valid and unmoved for cursor lifetime
- Memory movement invalidates the cursor (undefined behavior if used after movement)

Edge cases:
- Cursor becomes invalid if priority queue memory is deallocated or unregistered
- Cursor becomes invalid if memory region is relocated
- No bounds checking in unsafe variants - caller must ensure queue is not empty/full

Additional notes:
- Cursor caches the header pointer for maximum performance
- Uses unsafe pointer operations for direct memory access
- Type safety is maintained through the generic parameter T
- Suitable for both sequential and random access patterns
- The cursor is a value type and can be copied, but shares the same underlying header
- Comparator function must be provided to each operation (not cached)
*/
type PriorityQueueCursorHeader[T any] struct {
	header *PriorityQueue[T]
}

/*
PriorityQueueCursorHeaderCreate creates a cursor that caches the PriorityQueue header pointer for hot-path operations.

This function performs a single MarkRaw dereference and caches the resulting header pointer,
enabling subsequent operations to bypass the dereference overhead. The cursor is optimized for
hot paths where memory movement is guaranteed not to occur during the cursor's lifetime.

Use cases:
- Setting up cursor-based priority queue operations
- High-performance priority queue manipulation patterns
- Batch processing operations
- Performance-critical code sections

Time complexity: O(1) - single dereference operation
Space complexity: O(1) - returns a fixed-size cursor structure

Prerequisites:
- queue must be a valid MarkRaw pointing to an initialized PriorityQueue[T]
- PriorityQueue must not have been deallocated or unregistered
- Memory must remain stable during cursor lifetime

Edge cases:
- Cursor becomes invalid if priority queue memory is deallocated or unregistered
- Cursor becomes invalid if memory region is relocated
- Returns cursor with nil header if queue is invalid (undefined behavior)

Additional notes:
- Cursor caches header pointer for maximum performance
- Uses unsafe pointer operations for direct memory access
- No header manipulation required after cursor creation
- Type safety is maintained through the generic parameter T
- The cursor is a value type and can be copied, but shares the same underlying header
- Comparator function must be provided to each operation (not cached in cursor)
*/
//
//go:inline
func PriorityQueueCursorHeaderCreate[T any](queue memcore.MarkRaw) PriorityQueueCursorHeader[T] {
	header := memcore.MemcoreMarkDereferenceObjectUnsafe[PriorityQueue[T]](queue)
	return PriorityQueueCursorHeader[T]{
		header: header,
	}
}

/*
PriorityQueueCursorHeaderInsert inserts an item into the priority queue and maintains heap order.

This function uses the cached header pointer to insert an item, bypassing MarkRaw dereference overhead.
It requires a comparator function 'less' that returns true if a < b. The item is inserted and the
heap property is restored.

Use cases:
- Cursor-based priority queue insert operations
- Batch processing operations
- Performance-critical code sections

Time complexity: O(log n) - heap sift-up operation
Space complexity: O(1) - no allocations

Prerequisites:
- cursor must be a valid PriorityQueueCursorHeader created with PriorityQueueCursorHeaderCreate
- PriorityQueue must not be at capacity
- less must be a valid comparator function
- PriorityQueue memory must remain valid and unmoved

Edge cases:
- Returns error if priority queue is at capacity
- Increments priority queue version on successful insert
- Undefined behavior if cursor is invalid or memory has moved

Additional notes:
- Uses cached header pointer for maximum performance
- Performs bounds checking before insert
- Maintains heap property using sift-up operation
- Type safety is maintained through the generic parameter T
*/
//
//go:inline
func PriorityQueueCursorHeaderInsert[T any](cursor PriorityQueueCursorHeader[T], item T, less func(a, b T) bool) error {
	if cursor.header.length >= cursor.header.capacity {
		return fmt.Errorf("priority queue overflow: capacity %d", cursor.header.capacity)
	}

	// Insert at the end
	ArraySetAtUnsafe(cursor.header.data, cursor.header.length, item)
	cursor.header.length++

	// Restore heap property
	pqSiftUp(cursor.header, cursor.header.length-1, less)
	cursor.header.version++
	return nil
}

/*
PriorityQueueCursorHeaderExtract removes and returns the minimum element (according to 'less').

This function uses the cached header pointer to extract the minimum element, bypassing MarkRaw
dereference overhead. It requires a comparator function 'less' that returns true if a < b.

Use cases:
- Cursor-based priority queue extract operations
- Batch processing operations
- Performance-critical code sections

Time complexity: O(log n) - heap sift-down operation
Space complexity: O(1) - no allocations

Prerequisites:
- cursor must be a valid PriorityQueueCursorHeader created with PriorityQueueCursorHeaderCreate
- PriorityQueue must not be empty
- less must be a valid comparator function
- PriorityQueue memory must remain valid and unmoved

Edge cases:
- Returns error if priority queue is empty
- Returns zero value and error if queue is empty
- Increments priority queue version on successful extract
- Undefined behavior if cursor is invalid or memory has moved

Additional notes:
- Uses cached header pointer for maximum performance
- Performs bounds checking before extract
- Maintains heap property using sift-down operation
- Type safety is maintained through the generic parameter T
*/
//
//go:inline
func PriorityQueueCursorHeaderExtract[T any](cursor PriorityQueueCursorHeader[T], less func(a, b T) bool) (T, error) {
	if cursor.header.length == 0 {
		var zero T
		return zero, fmt.Errorf("priority queue underflow: empty")
	}

	// 1. Read the root (min item)
	root := ArrayItemGetAtUnsafe[T](cursor.header.data, 0)

	// 2. Move the last item to the root position
	lastIdx := cursor.header.length - 1
	lastItem := ArrayItemGetAtUnsafe[T](cursor.header.data, lastIdx)
	ArraySetAtUnsafe(cursor.header.data, 0, lastItem)

	// 3. Decrease length (effectively deleting the last item)
	cursor.header.length--

	// 4. Restore heap property if elements remain
	if cursor.header.length > 0 {
		pqSiftDown(cursor.header, 0, less)
	}

	cursor.header.version++
	return root, nil
}

/*
PriorityQueueCursorHeaderLengthGet returns the current length of the priority queue.

This function uses the cached header pointer to access the priority queue length, bypassing MarkRaw
dereference overhead. The length represents the number of items currently in the priority queue.

Use cases:
- Cursor-based length queries
- Bounds checking before operations
- Loop iteration limits

Time complexity: O(1) - returns cached value
Space complexity: O(1) - no allocations

Prerequisites:
- cursor must be a valid PriorityQueueCursorHeader created with PriorityQueueCursorHeaderCreate
- PriorityQueue memory must remain valid and unmoved

Edge cases:
- Returns the priority queue's current length, which may be less than capacity
- Length does not change after cursor creation unless queue is modified
- Undefined behavior if cursor is invalid or memory has moved

Additional notes:
- Uses cached header pointer for maximum performance
- Length is stored in the priority queue header
- Type safety is maintained through the generic parameter T
*/
//
//go:inline
func PriorityQueueCursorHeaderLengthGet[T any](cursor PriorityQueueCursorHeader[T]) uint64 {
	return cursor.header.length
}

/*
PriorityQueueCursorHeaderIsEmpty returns whether the priority queue is empty.

This function uses the cached header pointer to check if the priority queue is empty, bypassing MarkRaw
dereference overhead.

Use cases:
- Cursor-based empty checks
- Conditional operations based on queue state
- Loop termination conditions

Time complexity: O(1) - simple comparison
Space complexity: O(1) - no allocations

Prerequisites:
- cursor must be a valid PriorityQueueCursorHeader created with PriorityQueueCursorHeaderCreate
- PriorityQueue memory must remain valid and unmoved

Edge cases:
- Returns true if priority queue length is 0
- Returns false if priority queue has any items
- Undefined behavior if cursor is invalid or memory has moved

Additional notes:
- Uses cached header pointer for maximum performance
- Equivalent to checking if length == 0
- Type safety is maintained through the generic parameter T
*/
//
//go:inline
func PriorityQueueCursorHeaderIsEmpty[T any](cursor PriorityQueueCursorHeader[T]) bool {
	return cursor.header.length == 0
}

/*
PriorityQueueCursorHeaderVersionGet returns the current version of the priority queue.

This function uses the cached header pointer to access the priority queue version, bypassing MarkRaw
dereference overhead. The version increments on every modification to the priority queue data, allowing
cache invalidation mechanisms to detect when cached values become stale.

Use cases:
- Cache invalidation checks
- Detecting priority queue modifications
- Version-based synchronization

Time complexity: O(1) - returns cached value
Space complexity: O(1) - no allocations

Prerequisites:
- cursor must be a valid PriorityQueueCursorHeader created with PriorityQueueCursorHeaderCreate
- PriorityQueue memory must remain valid and unmoved

Edge cases:
- Returns the priority queue's current version number
- Version increments on every modification operation
- Undefined behavior if cursor is invalid or memory has moved

Additional notes:
- Uses cached header pointer for maximum performance
- Version is stored in the priority queue header
- Version starts at 1 when priority queue is initialized
- Type safety is maintained through the generic parameter T
*/
//
//go:inline
func PriorityQueueCursorHeaderVersionGet[T any](cursor PriorityQueueCursorHeader[T]) uint64 {
	return cursor.header.version
}

