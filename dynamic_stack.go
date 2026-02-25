package memstruct

import (
	"fmt"
	"memcore"
)

/*
DynamicStackAllocationFn allocates a region of manual memory with the given size and alignment.
Clients can pass an allocator (e.g. memforge DynamicLinearAllocatorMalloc wrapper) so that
DynamicStack can allocate new buffers when it grows.
*/
type DynamicStackAllocationFn func(sizeBytes, alignment uint64) memcore.MarkRaw

/*
DynamicStackGrowthPolicy determines how the stack expands when full.
Same signature as memforge.GrowthStrategy: given currentCap and neededCap (e.g. currentCap+1),
returns the new capacity. The returned value must be >= neededCap or the code panics.

Use cases:
- Double growth: func(currentCap, neededCap uint64) uint64 { n := currentCap * 2; if n < neededCap { n = neededCap }; return n }
- Fixed bump: func(currentCap, neededCap uint64) uint64 { n := currentCap + 64; if n < neededCap { n = neededCap }; return n }
*/
type DynamicStackGrowthPolicy func(currentCap, neededCap uint64) uint64

/*
DynamicStackFreeFn is called with the old stack mark when the stack grows and the buffer is replaced.
If nil, the client is responsible for the old buffer (e.g. arena that does not free, or custom tracking).
*/
type DynamicStackFreeFn func(mark memcore.MarkRaw)

/*
DynamicStack is a thin wrapper around Stack that automatically grows capacity when full.
It holds the current stack mark, an allocation function, a growth policy, and an optional free function.
After any DynamicStackPush that triggers growth, the previous stack mark is invalid; do not store it.
*/
type DynamicStack[T any] struct {
	stackMark    memcore.MarkRaw
	alloc        DynamicStackAllocationFn
	growthPolicy DynamicStackGrowthPolicy
	freeFn       DynamicStackFreeFn
}

type dynamicStackGrowthPolicyViolation struct {
	prevCapacity uint64
	required     uint64
	returned     uint64
}

func (e dynamicStackGrowthPolicyViolation) Error() string {
	return fmt.Sprintf(
		"dynamic stack growth policy violation: returned %d < required %d (previous capacity %d)",
		e.returned, e.required, e.prevCapacity,
	)
}

/*
DynamicStackCreate initializes a DynamicStack with the given allocator, growth policy, and initial capacity.
The initial stack is allocated via alloc and initialized; freeFn is not set (client manages old buffers if needed).
*/
func DynamicStackCreate[T any](
	alloc DynamicStackAllocationFn,
	growthPolicy DynamicStackGrowthPolicy,
	initialCapacity uint64,
) *DynamicStack[T] {
	mark := alloc(StackRequiredBytesGet[T](initialCapacity), StackRequiredAlignmentGet[T]())
	StackInitializeAt[T](mark, initialCapacity)
	return &DynamicStack[T]{
		stackMark:    mark,
		alloc:        alloc,
		growthPolicy: growthPolicy,
		freeFn:       nil,
	}
}

/*
DynamicStackCreateWithFree is like DynamicStackInitialize but sets an optional free function
called with the old mark when the stack grows. Use when the allocator requires per-pointer free.
*/
func DynamicStackCreateWithFree[T any](
	ds *DynamicStack[T],
	alloc DynamicStackAllocationFn,
	growthPolicy DynamicStackGrowthPolicy,
	freeFn DynamicStackFreeFn,
	initialCapacity uint64,
) *DynamicStack[T] {
	mark := alloc(StackRequiredBytesGet[T](initialCapacity), StackRequiredAlignmentGet[T]())
	StackInitializeAt[T](mark, initialCapacity)
	return &DynamicStack[T]{
		stackMark:    mark,
		alloc:        alloc,
		growthPolicy: growthPolicy,
		freeFn:       freeFn,
	}
}

/*
DynamicStackPush ensures there is space (growing if needed via the growth policy), then pushes the item.
It does not rely on error checking: it checks length >= capacity first and grows before pushing.
Returns nil on success; allocation or growth policy violation panics.
*/
func DynamicStackPush[T any](ds *DynamicStack[T], item T) error {
	length := StackLengthGet[T](ds.stackMark)
	capacity := StackCapacityGet[T](ds.stackMark)
	if length >= capacity {
		currentCap := capacity
		neededCap := currentCap + 1
		newCap := ds.growthPolicy(currentCap, neededCap)
		if newCap < neededCap {
			panic(dynamicStackGrowthPolicyViolation{
				prevCapacity: currentCap,
				required:     neededCap,
				returned:     newCap,
			})
		}
		newMark := ds.alloc(StackRequiredBytesGet[T](newCap), StackRequiredAlignmentGet[T]())
		StackInitializeAt[T](newMark, newCap)
		if err := StackContentsCopy[T](newMark, ds.stackMark); err != nil {
			panic(err)
		}
		if ds.freeFn != nil {
			ds.freeFn(ds.stackMark)
		}
		ds.stackMark = newMark
	}
	StackPushUnsafe(ds.stackMark, item)
	return nil
}

/*
DynamicStackPop pops the top element with bounds checking. Delegates to StackPop.
*/
func DynamicStackPop[T any](ds *DynamicStack[T]) (T, error) {
	return StackPop[T](ds.stackMark)
}

/*
DynamicStackPopUnsafe pops without bounds checking. Delegates to StackPopUnsafe.
*/
func DynamicStackPopUnsafe[T any](ds *DynamicStack[T]) T {
	return StackPopUnsafe[T](ds.stackMark)
}

/*
DynamicStackPeek returns the top element without removing it. Delegates to StackPeek.
*/
func DynamicStackPeek[T any](ds *DynamicStack[T]) (T, error) {
	return StackPeek[T](ds.stackMark)
}

/*
DynamicStackPeekUnsafe peeks without bounds checking. Delegates to StackPeekUnsafe.
*/
func DynamicStackPeekUnsafe[T any](ds *DynamicStack[T]) T {
	return StackPeekUnsafe[T](ds.stackMark)
}

/*
DynamicStackClear resets logical length only (does not zero memory). Delegates to StackClear.
*/
func DynamicStackClear[T any](ds *DynamicStack[T]) {
	StackClear[T](ds.stackMark)
}

/*
DynamicStackClearAndZero resets logical length and zeroes array memory. Delegates to StackClearAndZero.
*/
func DynamicStackClearAndZero[T any](ds *DynamicStack[T]) {
	StackClearAndZero[T](ds.stackMark)
}

/*
DynamicStackIsEmpty returns whether the stack has no elements. Delegates to StackIsEmpty.
*/
func DynamicStackIsEmpty[T any](ds *DynamicStack[T]) bool {
	return StackIsEmpty[T](ds.stackMark)
}

/*
DynamicStackLengthGet returns the current number of elements on the stack. Delegates to StackLengthGet.
*/
func DynamicStackLengthGet[T any](ds *DynamicStack[T]) uint64 {
	return StackLengthGet[T](ds.stackMark)
}

/*
DynamicStackCapacityGet returns the current stack capacity (can change after a growing push). Delegates to StackCapacityGet.
*/
func DynamicStackCapacityGet[T any](ds *DynamicStack[T]) uint64 {
	return StackCapacityGet[T](ds.stackMark)
}

/*
DynamicStackStackMarkGet returns the current stack mark for advanced use.
This mark may change after any DynamicStackPush that triggers growth; do not store long-term.
*/
func DynamicStackStackMarkGet[T any](ds *DynamicStack[T]) memcore.MarkRaw {
	return ds.stackMark
}
