package memstruct

import (
	"fmt"
	"memcore"
	"unsafe"
)

//
// ────────────────────────────────────────────────────────────────────────────────
//   STRUCTURE OVERVIEW
// ────────────────────────────────────────────────────────────────────────────────
//
// FixedOrderedList[T] is a manually managed, fixed-capacity sequential list
// built on top of Array[T] and Stack[uint64] primitives.
//
// Each element is stored in a contiguous Array[T], but logical ordering is
// maintained via an Array[uint64] index table. A freelist Stack[uint64] holds
// all unused slots, allowing O(1) slot reuse without heap allocations.
//
// All internal substructures (arrays, stacks, snapshots) live inside the same
// memcore namespace. Consequently, the list is fully relocatable and can be
// snapshotted or restored purely through pointer-offset reconstruction.
//
// Substructures are co-allocated in the same blob; the header caches *Array / *Stack
// headers and data bases so internal paths use *Fast without repeated mark deref.
//

// FixedOrderedList is a relocatable, manually-managed sequential container.
type FixedOrderedList[T any] struct {
	dataArray        *Array[T]
	dataArrayBase    unsafe.Pointer
	indices          *Array[uint64]
	indicesBase      unsafe.Pointer
	freeList         *Stack[uint64]
	freeListData     *Array[uint64]
	freeListDataBase unsafe.Pointer
	freeListSnapshot *Stack[uint64]
	snapshotData     *Array[uint64]
	snapshotDataBase unsafe.Pointer

	length   uint64 // number of logically occupied slots
	capacity uint64 // total available slots
	version  uint64
}

//
// ────────────────────────────────────────────────────────────────────────────────
//   SIZE & ALIGNMENT
// ────────────────────────────────────────────────────────────────────────────────
//

// FixedOrderedListRequiredBytes returns the total number of bytes required to
// store a FixedOrderedList[T] with a given element capacity.
//
// The calculation includes:
//   - One Array[T] data store
//   - One Array[uint64] index table
//   - Two Stack[uint64] freelists (active + snapshot)
//   - Header
//
// Use this function to determine how much manual memory must be allocated
// prior to calling FixedOrderedListInitializeAt.
func FixedOrderedListRequiredBytes[T any](capacity uint64) uint64 {
	sizeData := ArrayRequiredBytesGet[T](capacity)
	sizeIndices := ArrayRequiredBytesGet[uint64](capacity)
	sizeFreelist := StackRequiredBytesGet[uint64](capacity)
	return sizeData + sizeIndices + sizeFreelist + sizeFreelist + memcore.SizeOf[FixedOrderedList[T]]()
}

// FixedOrderedListRequiredAlignment returns the maximum alignment constraint
// among all internal components (data array, indices, stacks).
func FixedOrderedListRequiredAlignment[T any]() uint64 {
	return max(
		ArrayRequiredAlignmentGet[T](),
		ArrayRequiredAlignmentGet[uint64](),
		StackRequiredAlignmentGet[uint64](),
		memcore.AlignOf[FixedOrderedList[T]](),
	)
}

//
// ────────────────────────────────────────────────────────────────────────────────
//   INITIALIZATION / DESTRUCTION
// ────────────────────────────────────────────────────────────────────────────────
//

// FixedOrderedListInitializeAt constructs a FixedOrderedList[T] header and
// initializes its subcomponents in a pre-allocated, manually managed region.
//
// Preconditions:
//   - The memory region referenced by listAddr must belong to a registered
//     memcore namespace.
//   - The region must have at least FixedOrderedListRequiredBytes[T](capacity)
//     bytes available and satisfy FixedOrderedListRequiredAlignment[T].
//
// The function creates and registers four internal pointers within the same
// namespace (indices, freeList, freeListSnapshot, and dataArray), initializes
// all of them, fills the freelist, and snapshots it for future resets.
//
// Capacity is expressed in number of elements, not bytes.
func FixedOrderedListInitializeAt[T any](listAddr memcore.MarkRaw, capacity uint64) {
	// Compute offsets for internal components relative to list header
	sizeIndices := ArrayRequiredBytesGet[uint64](capacity)
	sizeFreelist := StackRequiredBytesGet[uint64](capacity)
	sizeFreelistSnapshot := sizeFreelist

	offsetIndices := uintptr(memcore.SizeOf[FixedOrderedList[T]]())
	offsetFreelist := offsetIndices + uintptr(sizeIndices)
	offsetFreelistSnapshot := offsetFreelist + uintptr(sizeFreelist)
	offsetData := offsetFreelistSnapshot + uintptr(sizeFreelistSnapshot)

	// Create subpointers within the same namespace
	indicesPtr := memcore.MemcoreMarkOffsetFrom(listAddr, offsetIndices)
	freeListPtr := memcore.MemcoreMarkOffsetFrom(listAddr, offsetFreelist)
	freeListSnapshotPtr := memcore.MemcoreMarkOffsetFrom(listAddr, offsetFreelistSnapshot)
	dataPtr := memcore.MemcoreMarkOffsetFrom(listAddr, offsetData)

	// Initialize substructures
	ArrayInitializeAt[uint64](indicesPtr, capacity)
	StackInitializeAt[uint64](freeListPtr, capacity)
	ArrayInitializeAt[T](dataPtr, capacity)

	dataBase, dataInst := memcore.MemcoreMarkDereferenceObjectAltUnsafe[Array[T]](dataPtr)
	indicesBase, indicesInst := memcore.MemcoreMarkDereferenceObjectAltUnsafe[Array[uint64]](indicesPtr)
	freeList := memcore.MemcoreMarkDereferenceObjectUnsafe[Stack[uint64]](freeListPtr)
	stackStorageWireAt(freeListPtr, freeList)
	freeListSnapshot := memcore.MemcoreMarkDereferenceObjectUnsafe[Stack[uint64]](freeListSnapshotPtr)

	// Populate freelist with all slots [0, capacity)
	for i := uint64(0); i < capacity; i++ {
		StackPushUnsafeFastAuto(freeList, i)
	}

	// Snapshot freelist for O(1) resets
	StackSnapshotCreate[uint64](freeListSnapshotPtr, freeListPtr)
	stackStorageWireAt(freeListSnapshotPtr, freeListSnapshot)

	list := memcore.MemcoreMarkDereferenceObject[FixedOrderedList[T]](listAddr)
	*list = FixedOrderedList[T]{
		dataArray:        dataInst,
		dataArrayBase:    dataBase,
		indices:          indicesInst,
		indicesBase:      indicesBase,
		freeList:         freeList,
		freeListData:     freeList.data,
		freeListDataBase: freeList.dataBase,
		freeListSnapshot: freeListSnapshot,
		snapshotData:     freeListSnapshot.data,
		snapshotDataBase: freeListSnapshot.dataBase,
		length:           0,
		capacity:         capacity,
		version:          1,
	}
}

//
// ────────────────────────────────────────────────────────────────────────────────
//   ELEMENT ACCESSORS
// ────────────────────────────────────────────────────────────────────────────────
//

// FixedOrderedListItemGetAt retrieves the element at logical index `idx`.
// Returns an error if `idx` is out of range.
func FixedOrderedListItemGetAt[T any](list memcore.MarkRaw, idx uint64) (T, error) {
	instance := memcore.MemcoreMarkDereferenceObject[FixedOrderedList[T]](list)
	if err := fixedListGuaranteeIdxReadValidity(instance, idx); err != nil {
		var zero T
		return zero, err
	}
	return fixedListGetElement(instance, idx), nil
}

// FixedOrderedListItemGetAtUnsafe retrieves an element without performing
// any bounds checks.
func FixedOrderedListItemGetAtUnsafe[T any](list memcore.MarkRaw, idx uint64) T {
	instance := memcore.MemcoreMarkDereferenceObject[FixedOrderedList[T]](list)
	return fixedListGetElement(instance, idx)
}

// FixedOrderedListItemPtrGetAt returns a pointer to the element at `idx`,
// validating that the index is within range.
func FixedOrderedListItemPtrGetAt[T any](list memcore.MarkRaw, idx uint64) (*T, error) {
	instance := memcore.MemcoreMarkDereferenceObject[FixedOrderedList[T]](list)
	if err := fixedListGuaranteeIdxReadValidity(instance, idx); err != nil {
		return nil, err
	}
	return fixedListGetElementPtr(instance, idx), nil
}

// FixedOrderedListItemPtrGetAtUnsafe returns a pointer to the element
// at `idx` without any safety checks.
func FixedOrderedListItemPtrGetAtUnsafe[T any](list memcore.MarkRaw, idx uint64) *T {
	instance := memcore.MemcoreMarkDereferenceObject[FixedOrderedList[T]](list)
	return fixedListGetElementPtr(instance, idx)
}

//
// ────────────────────────────────────────────────────────────────────────────────
//   INSERTION & DELETION
// ────────────────────────────────────────────────────────────────────────────────
//

// FixedOrderedListAppend appends a new element to the end of the list.
// Returns an error if the list is already full.
func FixedOrderedListAppend[T any](list memcore.MarkRaw, value T) error {
	instance := memcore.MemcoreMarkDereferenceObject[FixedOrderedList[T]](list)
	if instance.length >= instance.capacity {
		return fmt.Errorf("fixed list full: capacity %d", instance.capacity)
	}
	slot := StackPopUnsafeFast(instance.freeList, instance.freeListData, instance.freeListDataBase)
	ArraySetAtUnsafeFast(instance.indices, instance.indicesBase, instance.length, slot)
	ArraySetAtUnsafeFast(instance.dataArray, instance.dataArrayBase, slot, value)
	instance.length++
	fixedOrderedListIncrementVersion[T](list)
	return nil
}

// FixedOrderedListAppendUnsafe appends without capacity validation.
func FixedOrderedListAppendUnsafe[T any](list memcore.MarkRaw, value T) {
	instance := memcore.MemcoreMarkDereferenceObject[FixedOrderedList[T]](list)
	slot := StackPopUnsafeFast(instance.freeList, instance.freeListData, instance.freeListDataBase)
	ArraySetAtUnsafeFast(instance.indices, instance.indicesBase, instance.length, slot)
	ArraySetAtUnsafeFast(instance.dataArray, instance.dataArrayBase, slot, value)
	instance.length++
	fixedOrderedListIncrementVersion[T](list)
}

// FixedOrderedListInsertAt inserts a value at a given logical index, shifting
// subsequent elements to the right. Runs in O(n) time.
func FixedOrderedListInsertAt[T any](list memcore.MarkRaw, idx uint64, value T) error {
	instance := memcore.MemcoreMarkDereferenceObject[FixedOrderedList[T]](list)
	if err := fixedListGuaranteeIdxInsertionValidity(instance, idx); err != nil {
		return err
	}
	if instance.length >= instance.capacity {
		return fmt.Errorf("no space left in fixed list")
	}
	slot := StackPopUnsafeFast(instance.freeList, instance.freeListData, instance.freeListDataBase)
	ArraySetAtUnsafeFast(instance.dataArray, instance.dataArrayBase, slot, value)
	fixedListInsertIndex(instance, idx, slot)
	instance.length++
	fixedOrderedListIncrementVersion[T](list)
	return nil
}

// FixedOrderedListInsertAtUnsafe inserts without validation.
func FixedOrderedListInsertAtUnsafe[T any](list memcore.MarkRaw, idx uint64, value T) {
	instance := memcore.MemcoreMarkDereferenceObject[FixedOrderedList[T]](list)
	slot := StackPopUnsafeFast(instance.freeList, instance.freeListData, instance.freeListDataBase)
	ArraySetAtUnsafeFast(instance.dataArray, instance.dataArrayBase, slot, value)
	fixedListInsertIndex(instance, idx, slot)
	instance.length++
	fixedOrderedListIncrementVersion[T](list)
}

// FixedOrderedListDelete removes an element at logical index `idx` and
// shifts remaining elements left by one slot. Returns an error if `idx`
// is invalid.
func FixedOrderedListDelete[T any](list memcore.MarkRaw, idx uint64) error {
	instance := memcore.MemcoreMarkDereferenceObject[FixedOrderedList[T]](list)
	if err := fixedListGuaranteeIdxReadValidity(instance, idx); err != nil {
		return err
	}
	slot := ArrayItemGetAtUnsafeFast(instance.indices, instance.indicesBase, idx)
	fixedListRemoveIndex(instance, idx)
	StackPushUnsafeFast(instance.freeList, instance.freeListData, instance.freeListDataBase, slot)
	instance.length--
	fixedOrderedListIncrementVersion[T](list)
	return nil
}

// FixedOrderedListDeleteUnsafe deletes without validation.
func FixedOrderedListDeleteUnsafe[T any](list memcore.MarkRaw, idx uint64) {
	instance := memcore.MemcoreMarkDereferenceObject[FixedOrderedList[T]](list)
	slot := ArrayItemGetAtUnsafeFast(instance.indices, instance.indicesBase, idx)
	fixedListRemoveIndex(instance, idx)
	StackPushUnsafeFast(instance.freeList, instance.freeListData, instance.freeListDataBase, slot)
	instance.length--
	fixedOrderedListIncrementVersion[T](list)
}

//
// ────────────────────────────────────────────────────────────────────────────────
//   SEARCH UTILITIES
// ────────────────────────────────────────────────────────────────────────────────
//

// FixedOrderedListBinarySearch performs a binary search over the list using
// the provided comparator predicate. The list must be sorted according to
// the same predicate logic.
//
// The predicate must return:
//   - negative when item < target
//   - 0 when item == target
//   - positive when item > target
//
// Returns the index of the matching element or an error if not found.
func FixedOrderedListBinarySearch[T any](list memcore.MarkRaw, predicate func(item T) int8) (uint64, error) {
	instance := memcore.MemcoreMarkDereferenceObject[FixedOrderedList[T]](list)
	n := instance.length
	if n == 0 {
		return 0, fmt.Errorf("empty list")
	}

	lo := uint64(0)
	hi := n
	for lo < hi {
		mid := (lo + hi) >> 1
		item := fixedListGetElement(instance, mid)
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

// FixedOrderedListBinarySearchInterval locates the interval between two
// neighboring elements where a new value could be inserted to preserve order.
//
// Returns:
//   - (prevIdx, nextIdx): indices of the neighboring elements
//   - (^uint64(0), 0) if the list is empty
//
// The predicate rules are identical to BinarySearch but do not require equality.
func FixedOrderedListBinarySearchInterval[T any](list memcore.MarkRaw, predicate func(item T) int8) (uint64, uint64) {
	instance := memcore.MemcoreMarkDereferenceObject[FixedOrderedList[T]](list)
	n := instance.length
	if n == 0 {
		return ^uint64(0), 0
	}

	lo := uint64(0)
	hi := n
	for lo < hi {
		mid := (lo + hi) >> 1
		item := fixedListGetElement(instance, mid)
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

// FixedOrderedListBinarySearchInsertionPoint is a convenience wrapper
// around BinarySearchInterval that returns only the new insertion index.
func FixedOrderedListBinarySearchInsertionPoint[T any](list memcore.MarkRaw, predicate func(item T) int8) uint64 {
	instance := memcore.MemcoreMarkDereferenceObject[FixedOrderedList[T]](list)
	if instance.length == 0 {
		return 0
	}
	_, next := FixedOrderedListBinarySearchInterval(list, predicate)
	if next == ^uint64(0) {
		return 0
	}
	return next
}

//
// ────────────────────────────────────────────────────────────────────────────────
//   CLEAR / RESET
// ────────────────────────────────────────────────────────────────────────────────
//

// FixedOrderedListClear resets logical length to zero and restores the
// freelist from its snapshot, allowing instant reuse of all slots.
// The underlying memory contents remain untouched.
func FixedOrderedListClear[T any](list memcore.MarkRaw) {
	instance := memcore.MemcoreMarkDereferenceObject[FixedOrderedList[T]](list)
	fixedListFreelistRestore(instance)
	instance.length = 0
	fixedOrderedListIncrementVersion[T](list)
}

// FixedOrderedListClearAndZero behaves like Clear but additionally wipes
// all data array memory using zeroing semantics (useful for sensitive data).
func FixedOrderedListClearAndZero[T any](list memcore.MarkRaw) {
	instance := memcore.MemcoreMarkDereferenceObject[FixedOrderedList[T]](list)
	ArrayClearFast(instance.dataArray, instance.dataArrayBase)
	fixedListFreelistRestore(instance)
	instance.length = 0
	fixedOrderedListIncrementVersion[T](list)
}

//
// ────────────────────────────────────────────────────────────────────────────────
//   LENGTH / CAPACITY UTILITIES
// ────────────────────────────────────────────────────────────────────────────────
//

func FixedOrderedListLengthGet[T any](list memcore.MarkRaw) uint64 {
	instance := memcore.MemcoreMarkDereferenceObject[FixedOrderedList[T]](list)
	return instance.length
}

func FixedOrderedListCapacityGet[T any](list memcore.MarkRaw) uint64 {
	instance := memcore.MemcoreMarkDereferenceObject[FixedOrderedList[T]](list)
	return instance.capacity
}

func FixedOrderedListIsIdxValid[T any](list memcore.MarkRaw, idx uint64) bool {
	instance := memcore.MemcoreMarkDereferenceObject[FixedOrderedList[T]](list)
	return idx < instance.length
}

// FixedOrderedListVersionGet returns the current version of the fixed ordered list.
//
// Version increments on every modification to the list data, allowing cache invalidation
// mechanisms to detect when cached values become stale.
//
//go:inline
func FixedOrderedListVersionGet[T any](list memcore.MarkRaw) uint64 {
	instance := memcore.MemcoreMarkDereferenceObjectUnsafe[FixedOrderedList[T]](list)
	return instance.version
}

//
// ────────────────────────────────────────────────────────────────────────────────
//   PRIVATE INTERNALS
// ────────────────────────────────────────────────────────────────────────────────
//

func fixedListInsertIndex[T any](list *FixedOrderedList[T], logicalIdx, slot uint64) {
	n := list.length
	for i := n; i > logicalIdx; i-- {
		prev := ArrayItemGetAtUnsafeFast(list.indices, list.indicesBase, i-1)
		ArraySetAtUnsafeFast(list.indices, list.indicesBase, i, prev)
	}
	ArraySetAtUnsafeFast(list.indices, list.indicesBase, logicalIdx, slot)
}

func fixedListRemoveIndex[T any](list *FixedOrderedList[T], logicalIdx uint64) {
	n := list.length
	for i := logicalIdx; i+1 < n; i++ {
		next := ArrayItemGetAtUnsafeFast(list.indices, list.indicesBase, i+1)
		ArraySetAtUnsafeFast(list.indices, list.indicesBase, i, next)
	}
}

func fixedListGetPhysicalIdx[T any](list *FixedOrderedList[T], logicalIdx uint64) uint64 {
	return ArrayItemGetAtUnsafeFast(list.indices, list.indicesBase, logicalIdx)
}

func fixedListGetElement[T any](list *FixedOrderedList[T], logicalIdx uint64) T {
	physicalIdx := fixedListGetPhysicalIdx(list, logicalIdx)
	return ArrayItemGetAtUnsafeFast(list.dataArray, list.dataArrayBase, physicalIdx)
}

func fixedListGetElementPtr[T any](list *FixedOrderedList[T], logicalIdx uint64) *T {
	physicalIdx := fixedListGetPhysicalIdx(list, logicalIdx)
	return ArrayItemPtrGetAtUnsafeFast(list.dataArray, list.dataArrayBase, physicalIdx)
}

func fixedListFreelistRestore[T any](list *FixedOrderedList[T]) {
	if list.freeList.capacity != list.freeListSnapshot.capacity {
		panic(fmt.Errorf(
			"cannot restore stack snapshot: unequal capacities (%v vs %v)",
			list.freeList.capacity,
			list.freeListSnapshot.capacity,
		))
	}
	if err := arraySnapshotRestoreFast(
		list.freeListData,
		list.snapshotData,
		list.freeListDataBase,
		list.snapshotDataBase,
	); err != nil {
		panic(err)
	}
	list.freeList.length = list.freeListSnapshot.length
}

func fixedListGuaranteeIdxInsertionValidity[T any](list *FixedOrderedList[T], idx uint64) error {
	if idx >= list.capacity {
		return fmt.Errorf("index %v out of bounds (capacity %v)", idx, list.capacity)
	}
	if idx > list.length {
		return fmt.Errorf("invalid insertion index %v (0 ≤ idx ≤ %v)", idx, list.length)
	}
	return nil
}

func fixedListGuaranteeIdxReadValidity[T any](list *FixedOrderedList[T], idx uint64) error {
	if idx >= list.length {
		return fmt.Errorf("invalid read index %v (0 ≤ idx < %v)", idx, list.length)
	}
	return nil
}

//go:inline
func fixedOrderedListIncrementVersion[T any](list memcore.MarkRaw) {
	instance := memcore.MemcoreMarkDereferenceObjectUnsafe[FixedOrderedList[T]](list)
	instance.version++
}
