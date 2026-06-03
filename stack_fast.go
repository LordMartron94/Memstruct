package memstruct

import (
	"fmt"
	"unsafe"
)

// StackPopUnsafeFast pops the top element using pre-dereferenced stack and backing array storage.
//
//go:nosplit
func StackPopUnsafeFast[T any](stack *Stack[T], dataInst *Array[T], dataBase unsafe.Pointer) T {
	item := ArrayItemGetAtUnsafeFast(dataInst, dataBase, stack.length-1)
	stack.length--
	return item
}

// StackPushUnsafeFast pushes without bounds checks.
//
//go:nosplit
func StackPushUnsafeFast[T any](stack *Stack[T], dataInst *Array[T], dataBase unsafe.Pointer, item T) {
	ArraySetAtUnsafeFast(dataInst, dataBase, stack.length, item)
	stack.length++
}

// StackPopFast pops with bounds checking.
//
//go:nosplit
func StackPopFast[T any](stack *Stack[T], dataInst *Array[T], dataBase unsafe.Pointer) (T, error) {
	if stack.length == 0 {
		var zero T
		return zero, fmt.Errorf("stack underflow: empty stack")
	}
	return StackPopUnsafeFast(stack, dataInst, dataBase), nil
}

// StackPeekUnsafeFast returns the top element without removing it.
//
//go:inline
func StackPeekUnsafeFast[T any](stack *Stack[T], dataInst *Array[T], dataBase unsafe.Pointer) T {
	return ArrayItemGetAtUnsafeFast(dataInst, dataBase, stack.length-1)
}

// StackLengthGetFast returns current length from a pre-dereferenced header.
//
//go:inline
func StackLengthGetFast[T any](stack *Stack[T]) uint64 {
	return stack.length
}

// StackClearFast resets logical length only.
//
//go:inline
func StackClearFast[T any](stack *Stack[T]) {
	stack.length = 0
}

// StackPushUnsafeFastAuto pushes using wired storage on the stack header.
//
//go:nosplit
func StackPushUnsafeFastAuto[T any](stack *Stack[T], item T) {
	StackPushUnsafeFast(stack, stack.data, stack.dataBase, item)
}

// StackPopUnsafeFastAuto pops using wired storage on the stack header.
//
//go:nosplit
func StackPopUnsafeFastAuto[T any](stack *Stack[T]) T {
	return StackPopUnsafeFast(stack, stack.data, stack.dataBase)
}

// StackPopFastAuto pops with bounds checking using wired storage.
//
//go:nosplit
func StackPopFastAuto[T any](stack *Stack[T]) (T, error) {
	return StackPopFast(stack, stack.data, stack.dataBase)
}
