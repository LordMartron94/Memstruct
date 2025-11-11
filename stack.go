package memstruct

import (
	"fmt"
	"memcore"
)

// Stack is a custom stack implementation built on top of the array primitive.
type Stack[T any] struct {
	data     memcore.MarkRaw
	length   uint64
	capacity uint64
}

// StackRequiredBytesGet returns total bytes required for a stack of given capacity.
func StackRequiredBytesGet[T any](capacity uint64) uint64 {
	stackHeaderSize := memcore.SizeOf[Stack[T]]()
	stackHeaderAlignment := memcore.AlignOf[Stack[T]]()
	stackHeaderTotalSize := memcore.AlignUp(stackHeaderSize, stackHeaderAlignment)

	return stackHeaderTotalSize + ArrayRequiredBytesGet[T](capacity)
}

// StackRequiredAlignmentGet returns alignment requirement for the stack type.
func StackRequiredAlignmentGet[T any]() uint64 {
	return max(memcore.AlignOf[Stack[T]](), ArrayRequiredAlignmentGet[T]())
}

// StackInitializeAt initializes an instance of a stack for type T at a specific memory address.
// Ensure the address is properly aligned and has the right size.
//
// ⚠️ capacity is in elements, not bytes.
func StackInitializeAt[T any](stackAddr memcore.MarkRaw, capacity uint64) {
	stackHeaderSize := memcore.SizeOf[Stack[T]]()
	stackHeaderAlignment := memcore.AlignOf[Stack[T]]()

	// Create pointer for nested array header
	arrayPtr, _ := memcore.MemcoreMarkAlignedOffsetFrom(
		stackAddr,
		uintptr(stackHeaderSize),
		stackHeaderAlignment,
	)

	// Initialize array header + data region
	ArrayInitializeAt[T](arrayPtr, capacity)

	// Initialize stack header itself
	stackPtr := memcore.MemcoreMarkDereferenceObject[Stack[T]](stackAddr)
	*stackPtr = Stack[T]{
		data:     arrayPtr,
		length:   0,
		capacity: capacity,
	}
}

// StackSnapshotCreate creates a deep snapshot of a stack at a new location.
func StackSnapshotCreate[T any](dest memcore.MarkRaw, instance memcore.MarkRaw) memcore.MarkRaw {
	src := memcore.MemcoreMarkDereferenceObject[Stack[T]](instance)
	totalSize := StackRequiredBytesGet[T](src.capacity)

	srcAddr := memcore.MemcoreMarkDereference(instance)
	dstAddr := memcore.MemcoreMarkDereference(dest)
	memcore.MemoryMoveNoHeapPointers(dstAddr, srcAddr, uintptr(totalSize))

	return dest
}

// StackSnapshotRestore replaces the contents of one stack with another.
func StackSnapshotRestore[T any](dest memcore.MarkRaw, src memcore.MarkRaw) {
	dstStack := memcore.MemcoreMarkDereferenceObject[Stack[T]](dest)
	srcStack := memcore.MemcoreMarkDereferenceObject[Stack[T]](src)

	if dstStack.capacity != srcStack.capacity {
		panic(fmt.Errorf("cannot restore stack snapshot: unequal capacities (%v vs %v)", dstStack.capacity, srcStack.capacity))
	}

	ArraySnapshotRestore[T](dstStack.data, srcStack.data)
	dstStack.length = srcStack.length
}

// StackPush pushes an item into the stack.
//
//go:inline
//go:nosplit
func StackPush[T any](stack memcore.MarkRaw, item T) error {
	instance := memcore.MemcoreMarkDereferenceObject[Stack[T]](stack)

	if instance.length >= instance.capacity {
		return fmt.Errorf("stack overflow: capacity %d", instance.capacity)
	}

	ArraySetAtUnsafe(instance.data, instance.length, item)
	instance.length++
	return nil
}

// StackPushUnsafe pushes an item without boundary checks.
//
//go:inline
//go:nosplit
func StackPushUnsafe[T any](stack memcore.MarkRaw, item T) {
	instance := memcore.MemcoreMarkDereferenceObject[Stack[T]](stack)
	ArraySetAtUnsafe(instance.data, instance.length, item)
	instance.length++
}

// StackPop pops the top element with bounds checking.
//
//go:inline
//go:nosplit
func StackPop[T any](stack memcore.MarkRaw) (T, error) {
	instance := memcore.MemcoreMarkDereferenceObject[Stack[T]](stack)
	if instance.length == 0 {
		var zero T
		return zero, fmt.Errorf("stack underflow: empty stack")
	}

	item := ArrayItemGetAtUnsafe[T](instance.data, instance.length-1)
	instance.length--
	return item, nil
}

// StackPopUnsafe pops without bounds checking.
//
//go:inline
//go:nosplit
func StackPopUnsafe[T any](stack memcore.MarkRaw) T {
	instance := memcore.MemcoreMarkDereferenceObject[Stack[T]](stack)
	item := ArrayItemGetAtUnsafe[T](instance.data, instance.length-1)
	instance.length--
	return item
}

// StackPeek returns the top element without removing it.
//
//go:inline
//go:nosplit
func StackPeek[T any](stack memcore.MarkRaw) (T, error) {
	instance := memcore.MemcoreMarkDereferenceObject[Stack[T]](stack)
	if instance.length == 0 {
		var zero T
		return zero, fmt.Errorf("stack empty")
	}

	return ArrayItemGetAtUnsafe[T](instance.data, instance.length-1), nil
}

// StackPeekUnsafe peeks without bounds checking.
//
//go:inline
//go:nosplit
func StackPeekUnsafe[T any](stack memcore.MarkRaw) T {
	instance := memcore.MemcoreMarkDereferenceObject[Stack[T]](stack)
	return ArrayItemGetAtUnsafe[T](instance.data, instance.length-1)
}

// StackClear resets logical length only (does not zero memory).
//
//go:inline
//go:nosplit
func StackClear[T any](stack memcore.MarkRaw) {
	instance := memcore.MemcoreMarkDereferenceObject[Stack[T]](stack)
	instance.length = 0
}

// StackClearAndZero resets logical length and zeroes array memory.
//
//go:inline
//go:nosplit
func StackClearAndZero[T any](stack memcore.MarkRaw) {
	instance := memcore.MemcoreMarkDereferenceObject[Stack[T]](stack)
	ArrayClear[T](instance.data)
	instance.length = 0
}

// StackIsEmpty checks if stack is empty.
//
//go:inline
//go:nosplit
func StackIsEmpty[T any](stack memcore.MarkRaw) bool {
	instance := memcore.MemcoreMarkDereferenceObject[Stack[T]](stack)
	return instance.length == 0
}
