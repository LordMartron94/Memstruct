package memstruct

import "fmt"

// FixedOrderedListLengthGetFast returns logical length from a pre-dereferenced header.
//
//go:inline
func FixedOrderedListLengthGetFast[T any](list *FixedOrderedList[T]) uint64 {
	return list.length
}

// FixedOrderedListItemGetAtUnsafeFast reads an element using a pre-dereferenced list header.
//
//go:inline
func FixedOrderedListItemGetAtUnsafeFast[T any](list *FixedOrderedList[T], idx uint64) T {
	physicalIdx := ArrayItemGetAtUnsafeFast(list.indices, list.indicesBase, idx)
	return ArrayItemGetAtUnsafeFast(list.dataArray, list.dataArrayBase, physicalIdx)
}

// FixedOrderedListItemGetAtFast reads with bounds validation.
func FixedOrderedListItemGetAtFast[T any](list *FixedOrderedList[T], idx uint64) (T, error) {
	if err := fixedListGuaranteeIdxReadValidity(list, idx); err != nil {
		var zero T
		return zero, err
	}
	return FixedOrderedListItemGetAtUnsafeFast(list, idx), nil
}

// FixedOrderedListAppendUnsafeFast appends using a pre-dereferenced list header.
func FixedOrderedListAppendUnsafeFast[T any](list *FixedOrderedList[T], value T) {
	slot := StackPopUnsafeFastAuto(list.freeList)
	ArraySetAtUnsafeFast(list.indices, list.indicesBase, list.length, slot)
	ArraySetAtUnsafeFast(list.dataArray, list.dataArrayBase, slot, value)
	list.length++
	list.version++
}

// FixedOrderedListClearFast clears logical state; freelist restore uses internal snapshot storage.
func FixedOrderedListClearFast[T any](list *FixedOrderedList[T]) {
	fixedListFreelistRestore(list)
	list.length = 0
	list.version++
}

// FixedOrderedListItemPtrGetAtUnsafeFast returns an element pointer using a pre-dereferenced header.
//
//go:inline
func FixedOrderedListItemPtrGetAtUnsafeFast[T any](list *FixedOrderedList[T], idx uint64) *T {
	return fixedListGetElementPtr(list, idx)
}

// FixedOrderedListDeleteUnsafeFast deletes without validation using a pre-dereferenced header.
func FixedOrderedListDeleteUnsafeFast[T any](list *FixedOrderedList[T], idx uint64) {
	slot := ArrayItemGetAtUnsafeFast(list.indices, list.indicesBase, idx)
	fixedListRemoveIndex(list, idx)
	StackPushUnsafeFast(list.freeList, list.freeListData, list.freeListDataBase, slot)
	list.length--
	list.version++
}

// FixedOrderedListInsertAtFast inserts with validation using a pre-dereferenced header.
func FixedOrderedListInsertAtFast[T any](list *FixedOrderedList[T], idx uint64, value T) error {
	if err := fixedListGuaranteeIdxInsertionValidity(list, idx); err != nil {
		return err
	}
	if list.length >= list.capacity {
		return fmt.Errorf("no space left in fixed list")
	}
	slot := StackPopUnsafeFastAuto(list.freeList)
	ArraySetAtUnsafeFast(list.dataArray, list.dataArrayBase, slot, value)
	fixedListInsertIndex(list, idx, slot)
	list.length++
	list.version++
	return nil
}

// FixedOrderedListBinarySearchFast searches using a pre-dereferenced header.
func FixedOrderedListBinarySearchFast[T any](list *FixedOrderedList[T], predicate func(item T) int8) (uint64, error) {
	n := list.length
	if n == 0 {
		return 0, fmt.Errorf("empty list")
	}
	lo := uint64(0)
	hi := n
	for lo < hi {
		mid := (lo + hi) >> 1
		item := fixedListGetElement(list, mid)
		cmp := predicate(item)
		if cmp < 0 {
			lo = mid + 1
		} else if cmp > 0 {
			hi = mid
		} else {
			return mid, nil
		}
	}
	return 0, fmt.Errorf("predicate not found")
}

// FixedOrderedListBinarySearchIntervalFast locates insertion interval using a pre-dereferenced header.
func FixedOrderedListBinarySearchIntervalFast[T any](list *FixedOrderedList[T], predicate func(item T) int8) (uint64, uint64) {
	n := list.length
	if n == 0 {
		return ^uint64(0), 0
	}
	lo := uint64(0)
	hi := n
	for lo < hi {
		mid := (lo + hi) >> 1
		item := fixedListGetElement(list, mid)
		if predicate(item) < 0 {
			lo = mid + 1
		} else {
			hi = mid
		}
	}
	switch lo {
	case 0:
		return ^uint64(0), 0
	case n:
		return n - 1, n
	default:
		return lo - 1, lo
	}
}

// FixedOrderedListBinarySearchInsertionPointFast returns insertion index from a pre-dereferenced header.
func FixedOrderedListBinarySearchInsertionPointFast[T any](list *FixedOrderedList[T], predicate func(item T) int8) uint64 {
	if list.length == 0 {
		return 0
	}
	_, next := FixedOrderedListBinarySearchIntervalFast(list, predicate)
	if next == ^uint64(0) {
		return 0
	}
	return next
}

// FixedOrderedListIsIdxValidFast reports whether idx is a valid logical index.
//
//go:inline
func FixedOrderedListIsIdxValidFast[T any](list *FixedOrderedList[T], idx uint64) bool {
	return idx < list.length
}
