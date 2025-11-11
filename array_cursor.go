package memstruct

import (
	"memcore"
	"unsafe"
)

type ArrayCursor[T any] struct {
	dataBase unsafe.Pointer
	itemSize uintptr
	capacity uint64
}

// ArrayCursorCreate creates a fast-access cursor around an Array[T].
// It caches the base pointer and element size so that pointer arithmetic
// can be done without dereferencing the header each time.
//
//go:inline
func ArrayCursorCreate[T any](array memcore.MarkRaw) ArrayCursor[T] {
	base, instance := memcore.MemcoreMarkDereferenceObjectAltUnsafe[Array[T]](array)
	return ArrayCursor[T]{
		dataBase: unsafe.Add(base, instance.dataAddrOffset),
		itemSize: instance.itemSize,
		capacity: instance.capacity,
	}
}

// PtrAt returns a pointer to the element at the given index.
//
//go:inline
func (c *ArrayCursor[T]) PtrAt(idx uint64) *T {
	return (*T)(unsafe.Add(c.dataBase, uintptr(idx)*c.itemSize))
}

// Capacity returns the number of elements that can be stored.
//
//go:inline
func (c *ArrayCursor[T]) Capacity() uint64 {
	return c.capacity
}
