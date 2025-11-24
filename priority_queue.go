package memstruct

import (
	"fmt"
	"memcore"
)

// --------------------------------------------------- MEMORY MANAGEMENT

func PriorityQueueRequiredBytesGet[T any](capacityElements uint64) uint64 {
	return ArrayRequiredBytesGet[T](capacityElements) + memcore.SizeOf[PriorityQueue[T]]()
}

func PriorityQueueRequiredAlignmentGet[T any]() uint64 {
	return max(ArrayRequiredAlignmentGet[T](), memcore.AlignOf[PriorityQueue[T]]())
}

// PriorityQueue is a highly efficient priority queue implementation.
type PriorityQueue[T any] struct {
	data     memcore.MarkRaw // Array[T]
	length   uint64
	capacity uint64
}

// PriorityQueueInitializeAt initializes a priority queue at the given address.
func PriorityQueueInitializeAt[T any](addr memcore.MarkRaw, capacityElements uint64) {
	header := memcore.MemcoreMarkDereferenceObject[PriorityQueue[T]](addr)

	offsetData := uintptr(memcore.SizeOf[PriorityQueue[T]]())
	dataMark := memcore.MemcoreMarkOffsetFrom(addr, offsetData)
	ArrayInitializeAt[T](dataMark, capacityElements)

	*header = PriorityQueue[T]{
		data:     dataMark,
		length:   0,
		capacity: capacityElements,
	}
}

// PriorityQueueInitializeFrom initializes a new priority queue at pqAddr using the contents of src.
// NewCapacity must be >= src.length.
func PriorityQueueInitializeFrom[T any](pqAddr memcore.MarkRaw, src memcore.MarkRaw, newCapacity uint64) error {
	PriorityQueueInitializeAt[T](pqAddr, newCapacity)
	return PriorityQueueCopyFrom[T](pqAddr, src)
}

// PriorityQueueCopyFrom copies the active elements from src into dest.
// Dest capacity must be >= src length.
// Dest length will be overwritten to match src length.
func PriorityQueueCopyFrom[T any](dest memcore.MarkRaw, src memcore.MarkRaw) error {
	destHeader := memcore.MemcoreMarkDereferenceObjectUnsafe[PriorityQueue[T]](dest)
	srcHeader := memcore.MemcoreMarkDereferenceObjectUnsafe[PriorityQueue[T]](src)

	// Check if destination can hold the active elements of source
	if destHeader.capacity < srcHeader.length {
		return fmt.Errorf(
			"PriorityQueueCopyFrom: insufficient capacity (srcLen=%d, destCap=%d)",
			srcHeader.length, destHeader.capacity,
		)
	}

	// We only copy the active range [0, length).
	// We use the Array primitives to handle the raw data movement.
	err := ArrayCopyFromRange[T](
		destHeader.data,  // Dest Array
		srcHeader.data,   // Source Array
		0,                // From (inclusive)
		srcHeader.length, // To (exclusive)
		0,                // Dest Start Index
	)

	if err != nil {
		return err
	}

	// Synchronize the length in the header
	destHeader.length = srcHeader.length

	return nil
}

// PriorityQueueSnapshotCreate creates a deep copy of a priority queue at a new memory location.
// It copies the PQ Header, the Array Header, and the Array Data in one block,
// then repoints the internal pointers.
func PriorityQueueSnapshotCreate[T any](dest memcore.MarkRaw, instance memcore.MarkRaw) memcore.MarkRaw {
	srcHeader := memcore.MemcoreMarkDereferenceObjectUnsafe[PriorityQueue[T]](instance)

	totalSize := PriorityQueueRequiredBytesGet[T](srcHeader.capacity)

	srcAddr := memcore.MemcoreMarkDereference(instance)
	dstAddr := memcore.MemcoreMarkDereference(dest)

	memcore.MemoryMoveNoHeapPointers(dstAddr, srcAddr, uintptr(totalSize))

	dstHeader := memcore.MemcoreMarkDereferenceObjectUnsafe[PriorityQueue[T]](dest)
	offsetData := uintptr(memcore.SizeOf[PriorityQueue[T]]())

	dstHeader.data = memcore.MemcoreMarkOffsetFrom(dest, offsetData)

	return dest
}

// PriorityQueueSnapshotRestore replaces the entire memory block of one queue
// with that of another. Capacities must match exactly.
func PriorityQueueSnapshotRestore[T any](dest, src memcore.MarkRaw) error {
	dstHeader := memcore.MemcoreMarkDereferenceObjectUnsafe[PriorityQueue[T]](dest)
	srcHeader := memcore.MemcoreMarkDereferenceObjectUnsafe[PriorityQueue[T]](src)

	if dstHeader.capacity != srcHeader.capacity {
		return fmt.Errorf("cannot restore snapshot: unequal capacities")
	}

	if dest == src {
		return nil
	}

	totalBytes := PriorityQueueRequiredBytesGet[T](dstHeader.capacity)

	dstAddr := memcore.MemcoreMarkDereferenceUnsafe(dest)
	srcAddr := memcore.MemcoreMarkDereferenceUnsafe(src)

	memcore.MemoryMoveNoHeapPointers(dstAddr, srcAddr, uintptr(totalBytes))

	offsetData := uintptr(memcore.SizeOf[PriorityQueue[T]]())
	dstHeader.data = memcore.MemcoreMarkOffsetFrom(dest, offsetData)

	return nil
}

// PriorityQueueClear clears the queue, allowing it to be re-used.
func PriorityQueueClear[T any](queue memcore.MarkRaw) {
	header := memcore.MemcoreMarkDereferenceObject[PriorityQueue[T]](queue)
	header.length = 0
}

// PriorityQueueClearAndZero clears the queue, allowing it to be re-used, and zeroes the underlying memory.
func PriorityQueueClearAndZero[T any](queue memcore.MarkRaw) {
	header := memcore.MemcoreMarkDereferenceObject[PriorityQueue[T]](queue)
	header.length = 0

	ArrayClear[T](header.data)
}

// --------------------------------------------------- CORE OPERATIONS

// PriorityQueuePush pushes an item into the queue and maintains heap order.
// Requires a comparator function 'less' that returns true if a < b.
//
//go:nosplit
func PriorityQueuePush[T any](queue memcore.MarkRaw, item T, less func(a, b T) bool) error {
	instance := memcore.MemcoreMarkDereferenceObject[PriorityQueue[T]](queue)

	if instance.length >= instance.capacity {
		return fmt.Errorf("priority queue overflow: capacity %d", instance.capacity)
	}

	// Insert at the end
	ArraySetAtUnsafe(instance.data, instance.length, item)
	instance.length++

	// Restore heap property
	pqSiftUp(instance, instance.length-1, less)
	return nil
}

// PriorityQueuePop removes and returns the minimum element (according to 'less').
// Requires a comparator function 'less' that returns true if a < b.
//
//go:nosplit
func PriorityQueuePop[T any](queue memcore.MarkRaw, less func(a, b T) bool) (T, error) {
	instance := memcore.MemcoreMarkDereferenceObject[PriorityQueue[T]](queue)

	if instance.length == 0 {
		var zero T
		return zero, fmt.Errorf("priority queue underflow: empty")
	}

	// 1. Read the root (min item)
	root := ArrayItemGetAtUnsafe[T](instance.data, 0)

	// 2. Move the last item to the root position
	lastIdx := instance.length - 1
	lastItem := ArrayItemGetAtUnsafe[T](instance.data, lastIdx)
	ArraySetAtUnsafe(instance.data, 0, lastItem)

	// 3. Decrease length (effectively deleting the last item)
	instance.length--

	// 4. Restore heap property if elements remain
	if instance.length > 0 {
		pqSiftDown(instance, 0, less)
	}

	return root, nil
}

// PriorityQueuePeek returns the minimum element without removing it.
//
//go:inline
//go:nosplit
func PriorityQueuePeek[T any](queue memcore.MarkRaw) (T, error) {
	instance := memcore.MemcoreMarkDereferenceObject[PriorityQueue[T]](queue)

	if instance.length == 0 {
		var zero T
		return zero, fmt.Errorf("priority queue empty")
	}

	return ArrayItemGetAtUnsafe[T](instance.data, 0), nil
}

// PriorityQueueIsEmpty checks if the queue is empty.
//
//go:inline
//go:nosplit
func PriorityQueueIsEmpty[T any](queue memcore.MarkRaw) bool {
	instance := memcore.MemcoreMarkDereferenceObject[PriorityQueue[T]](queue)
	return instance.length == 0
}

// PriorityQueueLengthGet returns the current number of items.
//
//go:inline
//go:nosplit
func PriorityQueueLengthGet[T any](queue memcore.MarkRaw) uint64 {
	instance := memcore.MemcoreMarkDereferenceObject[PriorityQueue[T]](queue)
	return instance.length
}

// PriorityQueueCapacityGet returns the current capacity.
//
//go:inline
//go:nosplit
func PriorityQueueCapacityGet[T any](queue memcore.MarkRaw) uint64 {
	instance := memcore.MemcoreMarkDereferenceObject[PriorityQueue[T]](queue)
	return instance.capacity
}

// --------------------------------------------------- PRIVATE ALGORITHMS

// pqSiftUp moves the element at idx up until the heap property is restored.
//
//go:nosplit
func pqSiftUp[T any](pq *PriorityQueue[T], idx uint64, less func(a, b T) bool) {
	currentIdx := idx

	for currentIdx > 0 {
		parentIdx := getParentIdx(currentIdx)

		currentVal := ArrayItemGetAtUnsafe[T](pq.data, currentIdx)
		parentVal := ArrayItemGetAtUnsafe[T](pq.data, parentIdx)

		// If parent is already smaller (or equal), we are done (Min Heap)
		if !less(currentVal, parentVal) {
			break
		}

		pqSwapUnsafe(pq.data, currentIdx, parentIdx, currentVal, parentVal)
		currentIdx = parentIdx
	}
}

// pqSiftDown moves the element at idx down until the heap property is restored.
//
//go:nosplit
func pqSiftDown[T any](pq *PriorityQueue[T], idx uint64, less func(a, b T) bool) {
	currentIdx := idx
	limit := pq.length

	for {
		leftIdx := getLeftChildIdx(currentIdx)
		rightIdx := getRightChildIdx(currentIdx)
		smallestIdx := currentIdx

		// Check Left
		if leftIdx < limit {
			valLeft := ArrayItemGetAtUnsafe[T](pq.data, leftIdx)
			valSmallest := ArrayItemGetAtUnsafe[T](pq.data, smallestIdx)
			if less(valLeft, valSmallest) {
				smallestIdx = leftIdx
			}
		}

		// Check Right
		if rightIdx < limit {
			valRight := ArrayItemGetAtUnsafe[T](pq.data, rightIdx)
			valSmallest := ArrayItemGetAtUnsafe[T](pq.data, smallestIdx)
			if less(valRight, valSmallest) {
				smallestIdx = rightIdx
			}
		}

		// If we are still the smallest, we are done
		if smallestIdx == currentIdx {
			return
		}

		// Perform swap
		valCurrent := ArrayItemGetAtUnsafe[T](pq.data, currentIdx)
		valSmallest := ArrayItemGetAtUnsafe[T](pq.data, smallestIdx)

		pqSwapUnsafe(pq.data, currentIdx, smallestIdx, valCurrent, valSmallest)
		currentIdx = smallestIdx
	}
}

// pqSwapUnsafe swaps values at two indices.
// It expects the values to be pre-fetched to avoid redundant lookups,
// but writes them to the opposing indices.
//
//go:inline
func pqSwapUnsafe[T any](data memcore.MarkRaw, idxA, idxB uint64, valA, valB T) {
	ArraySetAtUnsafe(data, idxA, valB)
	ArraySetAtUnsafe(data, idxB, valA)
}

// --------------------------------------------------- PRIVATE HELPERS

//go:inline
func getParentIdx(idx uint64) uint64 {
	return (idx - 1) / 2
}

//go:inline
func getLeftChildIdx(idx uint64) uint64 {
	return (2 * idx) + 1
}

//go:inline
func getRightChildIdx(idx uint64) uint64 {
	return (2 * idx) + 2
}
