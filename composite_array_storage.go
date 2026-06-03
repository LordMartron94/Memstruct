package memstruct

import (
	"fmt"
	"memcore"
	"unsafe"
)

// arrayStorageWireAtMark resolves co-located array header and data base from a child mark.
func arrayStorageWireAtMark[T any](arrayMark memcore.MarkRaw) (base unsafe.Pointer, inst *Array[T]) {
	return memcore.MemcoreMarkDereferenceObjectAltUnsafe[Array[T]](arrayMark)
}

// arraySnapshotRestoreFast copies array header and data between pre-wired storage.
func arraySnapshotRestoreFast[T any](dest, src *Array[T], destBase, srcBase unsafe.Pointer) error {
	if dest.capacity != src.capacity {
		return fmt.Errorf("cannot restore snapshot: unequal capacities (dest=%v, src=%v)", dest.capacity, src.capacity)
	}
	headerSize := memcore.SizeOf[Array[T]]()
	memcore.MemoryMoveNoHeapPointers(destBase, srcBase, uintptr(headerSize))
	dataSize := dest.capacity * uint64(dest.itemSize)
	destData := arrayComputeDataAddr(dest, destBase)
	srcData := arrayComputeDataAddr(src, srcBase)
	memcore.MemoryMoveNoHeapPointers(destData, srcData, uintptr(dataSize))
	return nil
}

func stackCoallocatedArrayMark[T any](stackAddr memcore.MarkRaw) memcore.MarkRaw {
	headerSize := memcore.SizeOf[Stack[T]]()
	align := memcore.AlignOf[Stack[T]]()
	arrayMark, _ := memcore.MemcoreMarkAlignedOffsetFrom(stackAddr, uintptr(headerSize), align)
	return arrayMark
}

func stackStorageWireAt[T any](stackAddr memcore.MarkRaw, stack *Stack[T]) {
	stack.dataBase, stack.data = arrayStorageWireAtMark[T](stackCoallocatedArrayMark[T](stackAddr))
}

func queueCoallocatedArrayMark[T any](queueAddr memcore.MarkRaw) memcore.MarkRaw {
	headerSize := memcore.SizeOf[Queue[T]]()
	align := memcore.AlignOf[Queue[T]]()
	arrayMark, _ := memcore.MemcoreMarkAlignedOffsetFrom(queueAddr, uintptr(headerSize), align)
	return arrayMark
}

func queueStorageWireAt[T any](queueAddr memcore.MarkRaw, queue *Queue[T]) {
	queue.dataBase, queue.data = arrayStorageWireAtMark[T](queueCoallocatedArrayMark[T](queueAddr))
}

func circularBufferCoallocatedArrayMark[T any](bufferAddr memcore.MarkRaw) memcore.MarkRaw {
	headerSize := memcore.SizeOf[CircularBuffer[T]]()
	align := memcore.AlignOf[CircularBuffer[T]]()
	arrayMark, _ := memcore.MemcoreMarkAlignedOffsetFrom(bufferAddr, uintptr(headerSize), align)
	return arrayMark
}

func circularBufferStorageWireAt[T any](bufferAddr memcore.MarkRaw, buffer *CircularBuffer[T]) {
	buffer.dataBase, buffer.data = arrayStorageWireAtMark[T](circularBufferCoallocatedArrayMark[T](bufferAddr))
}

func priorityQueueCoallocatedArrayMark[T any](queueAddr memcore.MarkRaw) memcore.MarkRaw {
	return memcore.MemcoreMarkOffsetFrom(queueAddr, uintptr(memcore.SizeOf[PriorityQueue[T]]()))
}

func priorityQueueStorageWireAt[T any](queueAddr memcore.MarkRaw, queue *PriorityQueue[T]) {
	queue.dataBase, queue.data = arrayStorageWireAtMark[T](priorityQueueCoallocatedArrayMark[T](queueAddr))
}

func hashMapStorageWireAt[TKey, TValue any](mapAddr memcore.MarkRaw, header *HashMap[TKey, TValue]) {
	headerSize := memcore.SizeOf[HashMap[TKey, TValue]]()
	metaAlign := ArrayRequiredAlignmentGet[ctrlGroup]()
	metaAddr, _ := memcore.MemcoreMarkAlignedOffsetFrom(mapAddr, uintptr(headerSize), metaAlign)
	header.metaBase, header.meta = arrayStorageWireAtMark[ctrlGroup](metaAddr)
	metaSize := ArrayRequiredBytesGet[ctrlGroup](header.logicalBins)
	afterMeta := memcore.MemcoreMarkOffsetFrom(metaAddr, uintptr(metaSize))
	dataAlign := ArrayRequiredAlignmentGet[KeyValuePair[TKey, TValue]]()
	dataAddr, _ := memcore.MemcoreMarkAlignedOffsetFrom(afterMeta, 0, dataAlign)
	header.dataBase, header.data = arrayStorageWireAtMark[KeyValuePair[TKey, TValue]](dataAddr)
}
