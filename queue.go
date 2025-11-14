package memstruct

import (
	"fmt"
	"memcore"
)

// Queue is a custom queue implementation built on top of the array primitive.
type Queue[T any] struct {
	data     memcore.MarkRaw
	head     uint64
	tail     uint64
	length   uint64
	capacity uint64
}

// QueueRequiredBytesGet returns total bytes required for a queue of given capacity.
func QueueRequiredBytesGet[T any](capacity uint64) uint64 {
	queueHeaderSize := memcore.SizeOf[Queue[T]]()
	queueHeaderAlignment := memcore.AlignOf[Queue[T]]()
	queueHeaderTotalSize := memcore.AlignUp(queueHeaderSize, queueHeaderAlignment)

	return queueHeaderTotalSize + ArrayRequiredBytesGet[T](capacity)
}

// QueueRequiredAlignmentGet returns alignment requirement for the queue type.
func QueueRequiredAlignmentGet[T any]() uint64 {
	return max(memcore.AlignOf[Queue[T]](), ArrayRequiredAlignmentGet[T]())
}

// QueueInitializeAt initializes an instance of a queue for type T at a specific memory address.
// Ensure the address is properly aligned and has the right size.
//
// ⚠️ capacity is in elements, not bytes.
func QueueInitializeAt[T any](queueAddr memcore.MarkRaw, capacity uint64) {
	queueHeaderSize := memcore.SizeOf[Queue[T]]()
	queueHeaderAlignment := memcore.AlignOf[Queue[T]]()

	// Create pointer for nested array header
	arrayPtr, _ := memcore.MemcoreMarkAlignedOffsetFrom(
		queueAddr,
		uintptr(queueHeaderSize),
		queueHeaderAlignment,
	)

	// Initialize array header + data region
	ArrayInitializeAt[T](arrayPtr, capacity)

	// Initialize queue header itself
	queuePtr := memcore.MemcoreMarkDereferenceObject[Queue[T]](queueAddr)
	*queuePtr = Queue[T]{
		data:     arrayPtr,
		length:   0,
		head:     0,
		tail:     0,
		capacity: capacity,
	}
}

// QueueSnapshotCreate creates a deep snapshot of a queue at a new location.
func QueueSnapshotCreate[T any](dest memcore.MarkRaw, instance memcore.MarkRaw) memcore.MarkRaw {
	src := memcore.MemcoreMarkDereferenceObject[Queue[T]](instance)
	totalSize := QueueRequiredBytesGet[T](src.capacity)

	srcAddr := memcore.MemcoreMarkDereference(instance)
	dstAddr := memcore.MemcoreMarkDereference(dest)
	memcore.MemoryMoveNoHeapPointers(dstAddr, srcAddr, uintptr(totalSize))

	return dest
}

// QueueSnapshotRestore replaces the contents of one queue with another.
func QueueSnapshotRestore[T any](dest memcore.MarkRaw, src memcore.MarkRaw) {
	dstQueue := memcore.MemcoreMarkDereferenceObject[Queue[T]](dest)
	srcQueue := memcore.MemcoreMarkDereferenceObject[Queue[T]](src)

	if dstQueue.capacity != srcQueue.capacity {
		panic(fmt.Errorf("cannot restore queue snapshot: unequal capacities (%v vs %v)", dstQueue.capacity, srcQueue.capacity))
	}

	ArraySnapshotRestore[T](dstQueue.data, srcQueue.data)
	dstQueue.length = srcQueue.length
}

// QueuePush pushes an item into the queue.
//
//go:inline
//go:nosplit
func QueuePush[T any](queue memcore.MarkRaw, item T) error {
	instance := memcore.MemcoreMarkDereferenceObject[Queue[T]](queue)

	if instance.length >= instance.capacity {
		return fmt.Errorf("queue overflow: capacity %d", instance.capacity)
	}

	ArraySetAtUnsafe(instance.data, instance.tail, item)
	instance.tail = (instance.tail + 1) % instance.capacity
	instance.length++
	return nil
}

// QueuePushUnsafe pushes an item without boundary checks.
//
//go:inline
//go:nosplit
func QueuePushUnsafe[T any](queue memcore.MarkRaw, item T) {
	instance := memcore.MemcoreMarkDereferenceObject[Queue[T]](queue)

	ArraySetAtUnsafe(instance.data, instance.tail, item)
	instance.tail = (instance.tail + 1) % instance.capacity
	instance.length++
}

// QueuePop pops the top element with bounds checking.
//
//go:inline
//go:nosplit
func QueuePop[T any](queue memcore.MarkRaw) (T, error) {
	instance := memcore.MemcoreMarkDereferenceObject[Queue[T]](queue)

	if instance.length == 0 {
		var zero T
		return zero, fmt.Errorf("queue underflow: empty queue")
	}

	item := ArrayItemGetAtUnsafe[T](instance.data, instance.head)
	instance.head = (instance.head + 1) % instance.capacity
	instance.length--
	return item, nil
}

// QueuePopUnsafe pops without bounds checking.
//
//go:inline
//go:nosplit
func QueuePopUnsafe[T any](queue memcore.MarkRaw) T {
	instance := memcore.MemcoreMarkDereferenceObject[Queue[T]](queue)

	item := ArrayItemGetAtUnsafe[T](instance.data, instance.head)
	instance.head = (instance.head + 1) % instance.capacity
	instance.length--
	return item
}

// QueuePeek returns the top (head) element without removing it.
//
//go:inline
//go:nosplit
func QueuePeek[T any](queue memcore.MarkRaw) (T, error) {
	instance := memcore.MemcoreMarkDereferenceObject[Queue[T]](queue)

	if instance.length == 0 {
		var zero T
		return zero, fmt.Errorf("queue empty")
	}

	return ArrayItemGetAtUnsafe[T](instance.data, instance.head), nil
}

// QueuePeekUnsafe peeks without bounds checking.
//
//go:inline
//go:nosplit
func QueuePeekUnsafe[T any](queue memcore.MarkRaw) T {
	instance := memcore.MemcoreMarkDereferenceObject[Queue[T]](queue)
	return ArrayItemGetAtUnsafe[T](instance.data, instance.head)
}

// QueueClear resets logical length only (does not zero memory).
//
//go:inline
//go:nosplit
func QueueClear[T any](queue memcore.MarkRaw) {
	instance := memcore.MemcoreMarkDereferenceObject[Queue[T]](queue)
	instance.length = 0
	instance.head = 0
	instance.tail = 0
}

// QueueClearAndZero resets logical length and zeroes array memory.
//
//go:inline
//go:nosplit
func QueueClearAndZero[T any](queue memcore.MarkRaw) {
	instance := memcore.MemcoreMarkDereferenceObject[Queue[T]](queue)
	ArrayClear[T](instance.data)
	instance.length = 0
}

// QueueIsEmpty checks if queue is empty.
//
//go:inline
//go:nosplit
func QueueIsEmpty[T any](queue memcore.MarkRaw) bool {
	instance := memcore.MemcoreMarkDereferenceObject[Queue[T]](queue)
	return instance.length == 0
}
