package memstruct

import (
	"foundation"
	"unsafe"
)

// VectorItemGetAtFast returns element at idx using pre-dereferenced vector (Array) header and base.
//
//go:nosplit
//go:inline
func VectorItemGetAtFast[T foundation.Numeric](instance *Array[T], baseAddr unsafe.Pointer, idx uint64) (T, error) {
	if err := arrayGuaranteeIdxValidity(instance, idx); err != nil {
		var zero T
		return zero, err
	}
	return *(*T)(arrayGetPtrAtIdx(instance, baseAddr, idx)), nil
}

// VectorItemGetAtUnsafeFast returns element at idx without bounds checks.
//
//go:inline
func VectorItemGetAtUnsafeFast[T foundation.Numeric](instance *Array[T], baseAddr unsafe.Pointer, idx uint64) T {
	return *(*T)(arrayGetPtrAtIdx(instance, baseAddr, idx))
}

// VectorItemPtrGetAtFast returns a pointer to the element at idx.
//
//go:nosplit
//go:inline
func VectorItemPtrGetAtFast[T foundation.Numeric](instance *Array[T], baseAddr unsafe.Pointer, idx uint64) (*T, error) {
	if err := arrayGuaranteeIdxValidity(instance, idx); err != nil {
		return nil, err
	}
	return (*T)(arrayGetPtrAtIdx(instance, baseAddr, idx)), nil
}

// VectorItemPtrGetAtUnsafeFast returns a pointer to the element at idx without bounds checks.
//
//go:inline
func VectorItemPtrGetAtUnsafeFast[T foundation.Numeric](instance *Array[T], baseAddr unsafe.Pointer, idx uint64) *T {
	return (*T)(arrayGetPtrAtIdx(instance, baseAddr, idx))
}

// VectorCapacityGetFast returns capacity from a pre-dereferenced vector header.
//
//go:inline
func VectorCapacityGetFast[T foundation.Numeric](instance *Array[T]) uint64 {
	return instance.capacity
}
