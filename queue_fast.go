package memstruct

import (
	"fmt"
	"unsafe"
)

// QueuePushUnsafeFast enqueues without bounds checks.
//
//go:nosplit
func QueuePushUnsafeFast[T any](queue *Queue[T], dataInst *Array[T], dataBase unsafe.Pointer, item T) {
	ArraySetAtUnsafeFast(dataInst, dataBase, queue.tail, item)
	queue.tail = (queue.tail + 1) % queue.capacity
	queue.length++
}

// QueuePopUnsafeFast dequeues without bounds checks.
//
//go:nosplit
func QueuePopUnsafeFast[T any](queue *Queue[T], dataInst *Array[T], dataBase unsafe.Pointer) T {
	item := ArrayItemGetAtUnsafeFast(dataInst, dataBase, queue.head)
	queue.head = (queue.head + 1) % queue.capacity
	queue.length--
	return item
}

// QueuePopFast dequeues with bounds checking.
//
//go:nosplit
func QueuePopFast[T any](queue *Queue[T], dataInst *Array[T], dataBase unsafe.Pointer) (T, error) {
	if queue.length == 0 {
		var zero T
		return zero, fmt.Errorf("queue underflow: empty queue")
	}
	return QueuePopUnsafeFast(queue, dataInst, dataBase), nil
}

// QueueLengthGetFast returns current length.
//
//go:inline
func QueueLengthGetFast[T any](queue *Queue[T]) uint64 {
	return queue.length
}

// QueueClearFast resets head, tail, and length.
//
//go:inline
func QueueClearFast[T any](queue *Queue[T]) {
	queue.head = 0
	queue.tail = 0
	queue.length = 0
}
