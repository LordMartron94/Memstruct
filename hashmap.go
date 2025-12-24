package memstruct

import (
	"fmt"
	"foundation/hash"
	"math/bits"
	"memcore"
	"unsafe"
)

// Implementation inspired by Go's SwissTable map design:
// https://go.dev/blog/swisstable

type (
	ctrl      = uint8
	ctrlGroup = uint64
	bitset    = uint64
)

const (
	ctrlEmpty   ctrl = 0b10000000
	ctrlDeleted ctrl = 0b11111110

	bitsetLSB   bitset = 0x0101010101010101
	bitsetMSB   bitset = 0x8080808080808080
	bitsetEmpty bitset = bitsetLSB * uint64(ctrlEmpty)
	// bitsetDeleted bitset = bitsetLSB * uint64(ctrlDeleted)
)

var hasher = hash.XXH3HasherCreateWithSeed(42)

// KeyValuePair stores a key–value pair inside manual memory.
type KeyValuePair[TKey, TValue any] struct {
	value     TValue
	keyMark   memcore.MarkRaw
	keyUnsafe uintptr
}

func (k *KeyValuePair[TKey, TValue]) Value() TValue { return k.value }

// HashMap is a SwissTable-like open-addressing hashmap.
type HashMap[TKey, TValue any] struct {
	data, metaData        memcore.MarkRaw // backing arrays
	capacity, logicalBins uint64
	keyComparerID         memcore.FunctionID
	keyMarkRetrieverID    memcore.FunctionID
	groupMask             uint64
	version               uint64
}

// KeyComparer defines the contract when two keys are considered equal.
type KeyComparer[TKey any] func(a, b *TKey) bool

// KeyMarkRetriever must return the mark of the given key.
type KeyMarkRetriever[TKey any] func(key TKey) memcore.MarkRaw

// HashMapRequiredBytesGet returns the total size (in bytes) needed for allocation.
func HashMapRequiredBytesGet[TKey, TValue any](capacity uint64) uint64 {
	capacity = memcore.AlignUp(capacity, 8)
	numGroups := memcore.NextPowerOfTwo(capacity / 8)
	capacity = numGroups * 8

	header := memcore.SizeOf[HashMap[TKey, TValue]]()
	meta := ArrayRequiredBytesGet[ctrlGroup](numGroups)
	data := ArrayRequiredBytesGet[KeyValuePair[TKey, TValue]](capacity)

	offset := uint64(0)
	offset = memcore.AlignUp(offset, memcore.AlignOf[HashMap[TKey, TValue]]()) + header
	offset = memcore.AlignUp(offset, ArrayRequiredAlignmentGet[ctrlGroup]()) + meta
	offset = memcore.AlignUp(offset, ArrayRequiredAlignmentGet[KeyValuePair[TKey, TValue]]()) + data
	return offset
}

// HashMapRequiredAlignmentGet returns the required alignment for allocation.
func HashMapRequiredAlignmentGet[TKey, TValue any]() uint64 {
	return max(
		memcore.AlignOf[HashMap[TKey, TValue]](),
		ArrayRequiredAlignmentGet[ctrlGroup](),
		ArrayRequiredAlignmentGet[KeyValuePair[TKey, TValue]](),
	)
}

// HashMapInitializeAt constructs a new hashmap in-place at the given memory address.
// Note that the capacity might be increased (NEVER decreased) due to power of 8 and 2 requirements internally.
func HashMapInitializeAt[TKey, TValue any](
	addr memcore.MarkRaw,
	capacity uint64,
	keyCmp KeyComparer[TKey],
	keyMark KeyMarkRetriever[TKey],
) {
	capacity = memcore.AlignUp(capacity, 8)
	numGroups := memcore.NextPowerOfTwo(capacity / 8)
	capacity = numGroups * 8

	headerSize := memcore.SizeOf[HashMap[TKey, TValue]]()
	metaSize := ArrayRequiredBytesGet[ctrlGroup](numGroups)
	metaAlign := ArrayRequiredAlignmentGet[ctrlGroup]()
	dataAlign := ArrayRequiredAlignmentGet[KeyValuePair[TKey, TValue]]()

	metaAddr, _ := memcore.MemcoreMarkAlignedOffsetFrom(addr, uintptr(headerSize), metaAlign)
	ArrayInitializeAt[ctrlGroup](metaAddr, numGroups)
	ArrayForEachUnsafe[ctrlGroup](metaAddr, func(p unsafe.Pointer, _ uint64) { *(*uint64)(p) = bitsetEmpty })

	afterMeta := memcore.MemcoreMarkOffsetFrom(metaAddr, uintptr(metaSize))
	dataAddr, _ := memcore.MemcoreMarkAlignedOffsetFrom(afterMeta, 0, dataAlign)
	ArrayInitializeAt[KeyValuePair[TKey, TValue]](dataAddr, capacity)

	header := memcore.MemcoreMarkDereferenceObject[HashMap[TKey, TValue]](addr)
	*header = HashMap[TKey, TValue]{
		data:               dataAddr,
		metaData:           metaAddr,
		capacity:           capacity,
		logicalBins:        numGroups,
		keyComparerID:      memcore.MemcoreFunctionRegisterTyped(keyCmp),
		keyMarkRetrieverID: memcore.MemcoreFunctionRegisterTyped(keyMark),
		groupMask:          numGroups - 1,
		version:            1,
	}
}

// HashMapItemAdd inserts or updates a key–value pair.
func HashMapItemAdd[TKey, TValue any](instance memcore.MarkRaw, key TKey, value TValue) {
	h := memcore.MemcoreMarkDereferenceObjectUnsafe[HashMap[TKey, TValue]](instance)
	retrieve := memcore.MemcoreFunctionRetrieveTyped[KeyMarkRetriever[TKey]](h.keyMarkRetrieverID)
	keyMark := retrieve(key)
	keyPtr := memcore.MemcoreMarkDereferenceObjectUnsafe[TKey](keyMark)
	keyHash := hash.XXH3HasherHash64View[TKey](hasher, keyMark)
	h1, h2 := computeHashParts(keyHash)

	groupIDX, slot, found, hasFree := hashMapProbe(h, h1, h2, keyPtr, true,
		func(kvp *KeyValuePair[TKey, TValue]) *TKey {
			return memcore.MemcoreMarkDereferenceObjectUnsafe[TKey](kvp.keyMark)
		},
		memcore.MemcoreFunctionRetrieveTyped[KeyComparer[TKey]](h.keyComparerID),
	)
	if found {
		ArrayItemPtrGetAtUnsafe[KeyValuePair[TKey, TValue]](h.data, groupIDX*8+slot).value = value
		return
	}
	if !hasFree {
		panic("HashMapItemAdd: no space left")
	}

	pair := ArrayItemPtrGetAtUnsafe[KeyValuePair[TKey, TValue]](h.data, groupIDX*8+slot)
	pair.value = value
	pair.keyMark = keyMark
	pair.keyUnsafe = uintptr(unsafe.Pointer(keyPtr))
	ctrlPtr := ArrayItemPtrGetAtUnsafe[ctrlGroup](h.metaData, groupIDX)
	*ctrlPtr = updateCtrlByte(*ctrlPtr, slot, ctrl(h2))
}

// HashMapItemAddUnsafe inserts or updates a key–value pair.
// This variant assumes all keys inside the map are still accessible in their original memory address.
// This is therefore only "safe" when you can guarantee the underlying memory has not relocated.
// It is a bit faster.
func HashMapItemAddUnsafe[TKey, TValue any](instance memcore.MarkRaw, key TKey, value TValue) {
	h := memcore.MemcoreMarkDereferenceObjectUnsafe[HashMap[TKey, TValue]](instance)
	retrieve := memcore.MemcoreFunctionRetrieveTyped[KeyMarkRetriever[TKey]](h.keyMarkRetrieverID)
	keyMark := retrieve(key)
	keyPtr := memcore.MemcoreMarkDereferenceObjectUnsafe[TKey](keyMark)
	keyHash := hash.XXH3HasherHash64View[TKey](hasher, keyMark)
	h1, h2 := computeHashParts(keyHash)

	groupIDX, slot, found, hasFree := hashMapProbe(h, h1, h2, keyPtr, true,
		func(kvp *KeyValuePair[TKey, TValue]) *TKey {
			return (*TKey)(unsafe.Pointer(kvp.keyUnsafe))
		},
		memcore.MemcoreFunctionRetrieveTyped[KeyComparer[TKey]](h.keyComparerID),
	)
	if found {
		ArrayItemPtrGetAtUnsafe[KeyValuePair[TKey, TValue]](h.data, groupIDX*8+slot).value = value
		hashMapIncrementVersion[TKey, TValue](instance)
		return
	}
	if !hasFree {
		panic("HashMapItemAdd: no space left")
	}

	pair := ArrayItemPtrGetAtUnsafe[KeyValuePair[TKey, TValue]](h.data, groupIDX*8+slot)
	pair.value = value
	pair.keyMark = keyMark
	pair.keyUnsafe = uintptr(unsafe.Pointer(keyPtr))
	ctrlPtr := ArrayItemPtrGetAtUnsafe[ctrlGroup](h.metaData, groupIDX)
	*ctrlPtr = updateCtrlByte(*ctrlPtr, slot, ctrl(h2))
	hashMapIncrementVersion[TKey, TValue](instance)
}

// HashMapItemAddFast inserts or updates a key–value pair.
func HashMapItemAddFast[TKey, TValue any](
	instance memcore.MarkRaw,
	key TKey, value TValue,
	keyCmp KeyComparer[TKey],
	markRetriever KeyMarkRetriever[TKey],
) {
	h := memcore.MemcoreMarkDereferenceObjectUnsafe[HashMap[TKey, TValue]](instance)
	keyMark := markRetriever(key)
	keyPtr := memcore.MemcoreMarkDereferenceObjectUnsafe[TKey](keyMark)
	keyHash := hash.XXH3HasherHash64View[TKey](hasher, keyMark)
	h1, h2 := computeHashParts(keyHash)

	groupIDX, slot, found, hasFree := hashMapProbe(h, h1, h2, keyPtr, true,
		func(kvp *KeyValuePair[TKey, TValue]) *TKey {
			return memcore.MemcoreMarkDereferenceObjectUnsafe[TKey](kvp.keyMark)
		},
		keyCmp)
	if found {
		ArrayItemPtrGetAtUnsafe[KeyValuePair[TKey, TValue]](h.data, groupIDX*8+slot).value = value
		hashMapIncrementVersion[TKey, TValue](instance)
		return
	}
	if !hasFree {
		panic("HashMapItemAdd: no space left")
	}

	pair := ArrayItemPtrGetAtUnsafe[KeyValuePair[TKey, TValue]](h.data, groupIDX*8+slot)
	pair.value = value
	pair.keyMark = keyMark
	pair.keyUnsafe = uintptr(unsafe.Pointer(keyPtr))
	ctrlPtr := ArrayItemPtrGetAtUnsafe[ctrlGroup](h.metaData, groupIDX)
	*ctrlPtr = updateCtrlByte(*ctrlPtr, slot, ctrl(h2))
	hashMapIncrementVersion[TKey, TValue](instance)
}

// HashMapItemAddFastUnsafe inserts or updates a key–value pair.
// This variant assumes all keys inside the map are still accessible in their original memory address.
// This is therefore only "safe" when you can guarantee the underlying memory has not relocated.
// It is a bit faster.
func HashMapItemAddFastUnsafe[TKey, TValue any](
	instance memcore.MarkRaw,
	key TKey, value TValue,
	keyCmp KeyComparer[TKey],
	markRetriever KeyMarkRetriever[TKey],
) {
	h := memcore.MemcoreMarkDereferenceObjectUnsafe[HashMap[TKey, TValue]](instance)
	keyMark := markRetriever(key)
	keyPtr := memcore.MemcoreMarkDereferenceObjectUnsafe[TKey](keyMark)
	keyHash := hash.XXH3HasherHash64View[TKey](hasher, keyMark)
	h1, h2 := computeHashParts(keyHash)

	groupIDX, slot, found, hasFree := hashMapProbe(h, h1, h2, keyPtr, true,
		func(kvp *KeyValuePair[TKey, TValue]) *TKey {
			return (*TKey)(unsafe.Pointer(kvp.keyUnsafe))
		},
		keyCmp)
	if found {
		ArrayItemPtrGetAtUnsafe[KeyValuePair[TKey, TValue]](h.data, groupIDX*8+slot).value = value
		hashMapIncrementVersion[TKey, TValue](instance)
		return
	}
	if !hasFree {
		panic("HashMapItemAdd: no space left")
	}

	pair := ArrayItemPtrGetAtUnsafe[KeyValuePair[TKey, TValue]](h.data, groupIDX*8+slot)
	pair.value = value
	pair.keyMark = keyMark
	pair.keyUnsafe = uintptr(unsafe.Pointer(keyPtr))
	ctrlPtr := ArrayItemPtrGetAtUnsafe[ctrlGroup](h.metaData, groupIDX)
	*ctrlPtr = updateCtrlByte(*ctrlPtr, slot, ctrl(h2))
	hashMapIncrementVersion[TKey, TValue](instance)
}

// HashMapItemGet retrieves the value for a key.
func HashMapItemGet[TKey, TValue any](instance memcore.MarkRaw, key TKey) (TValue, error) {
	h := memcore.MemcoreMarkDereferenceObjectUnsafe[HashMap[TKey, TValue]](instance)

	retrieve := memcore.MemcoreFunctionRetrieveTyped[KeyMarkRetriever[TKey]](h.keyMarkRetrieverID)
	keyMark := retrieve(key)
	keyPtr := memcore.MemcoreMarkDereferenceObjectUnsafe[TKey](keyMark)

	keyHash := hash.XXH3HasherHash64View[TKey](hasher, keyMark)
	h1, h2 := computeHashParts(keyHash)

	groupIDX, slot, found, _ := hashMapProbe(h, h1, h2, keyPtr, true,
		func(kvp *KeyValuePair[TKey, TValue]) *TKey {
			return memcore.MemcoreMarkDereferenceObject[TKey](kvp.keyMark)
		},
		memcore.MemcoreFunctionRetrieveTyped[KeyComparer[TKey]](h.keyComparerID),
	)
	if !found {
		var zero TValue
		return zero, fmt.Errorf("key not found")
	}
	return ArrayItemPtrGetAtUnsafe[KeyValuePair[TKey, TValue]](h.data, groupIDX*8+slot).value, nil
}

// HashMapItemGetFast retrieves the value for a key.
func HashMapItemGetFast[TKey, TValue any](
	instance memcore.MarkRaw,
	key TKey,
	keyCmp KeyComparer[TKey],
	markRetriever KeyMarkRetriever[TKey],
) (TValue, error) {
	h := memcore.MemcoreMarkDereferenceObjectUnsafe[HashMap[TKey, TValue]](instance)

	keyMark := markRetriever(key)
	keyPtr := memcore.MemcoreMarkDereferenceObjectUnsafe[TKey](keyMark)

	keyHash := hash.XXH3HasherHash64View[TKey](hasher, keyMark)
	h1, h2 := computeHashParts(keyHash)

	groupIDX, slot, found, _ := hashMapProbe(h, h1, h2, keyPtr, true,
		func(kvp *KeyValuePair[TKey, TValue]) *TKey {
			return memcore.MemcoreMarkDereferenceObject[TKey](kvp.keyMark)
		},
		keyCmp,
	)
	if !found {
		var zero TValue
		return zero, fmt.Errorf("key not found")
	}
	return ArrayItemPtrGetAtUnsafe[KeyValuePair[TKey, TValue]](h.data, groupIDX*8+slot).value, nil
}

// HashMapItemGetUnsafe retrieves the value for a key.
// This variant assumes all keys inside the map are still accessible in their original memory address.
// This is therefore only "safe" when you can guarantee the underlying memory has not relocated.
// It is a bit faster.
func HashMapItemGetUnsafe[TKey, TValue any](instance memcore.MarkRaw, key TKey) (TValue, error) {
	h := memcore.MemcoreMarkDereferenceObjectUnsafe[HashMap[TKey, TValue]](instance)

	retrieve := memcore.MemcoreFunctionRetrieveTyped[KeyMarkRetriever[TKey]](h.keyMarkRetrieverID)
	keyMark := retrieve(key)
	keyPtr := memcore.MemcoreMarkDereferenceObjectUnsafe[TKey](keyMark)

	keyHash := hash.XXH3HasherHash64View[TKey](hasher, keyMark)
	h1, h2 := computeHashParts(keyHash)

	groupIDX, slot, found, _ := hashMapProbe(h, h1, h2, keyPtr, true,
		func(kvp *KeyValuePair[TKey, TValue]) *TKey {
			return (*TKey)(unsafe.Pointer(kvp.keyUnsafe))
		},
		memcore.MemcoreFunctionRetrieveTyped[KeyComparer[TKey]](h.keyComparerID),
	)
	if !found {
		var zero TValue
		return zero, fmt.Errorf("key not found")
	}
	return ArrayItemPtrGetAtUnsafe[KeyValuePair[TKey, TValue]](h.data, groupIDX*8+slot).value, nil
}

// HashMapItemGetFastUnsafe retrieves the value for a key.
// This variant assumes all keys inside the map are still accessible in their original memory address.
// This is therefore only "safe" when you can guarantee the underlying memory has not relocated.
// It is a bit faster.
func HashMapItemGetFastUnsafe[TKey, TValue any](
	instance memcore.MarkRaw,
	key TKey,
	keyCmp KeyComparer[TKey],
	markRetriever KeyMarkRetriever[TKey],
) (TValue, error) {
	h := memcore.MemcoreMarkDereferenceObjectUnsafe[HashMap[TKey, TValue]](instance)

	keyMark := markRetriever(key)
	keyPtr := memcore.MemcoreMarkDereferenceObjectUnsafe[TKey](keyMark)

	keyHash := hash.XXH3HasherHash64View[TKey](hasher, keyMark)
	h1, h2 := computeHashParts(keyHash)

	groupIDX, slot, found, _ := hashMapProbe(h, h1, h2, keyPtr, true,
		func(kvp *KeyValuePair[TKey, TValue]) *TKey {
			return (*TKey)(unsafe.Pointer(kvp.keyUnsafe))
		},
		keyCmp,
	)
	if !found {
		var zero TValue
		return zero, fmt.Errorf("key not found")
	}
	return ArrayItemPtrGetAtUnsafe[KeyValuePair[TKey, TValue]](h.data, groupIDX*8+slot).value, nil
}

// HashMapItemPtrGet returns a pointer to a value (not safe to persist).
func HashMapItemPtrGet[TKey, TValue any](instance memcore.MarkRaw, key TKey) (*TValue, error) {
	h := memcore.MemcoreMarkDereferenceObjectUnsafe[HashMap[TKey, TValue]](instance)

	retrieve := memcore.MemcoreFunctionRetrieveTyped[KeyMarkRetriever[TKey]](h.keyMarkRetrieverID)
	keyMark := retrieve(key)
	keyPtr := memcore.MemcoreMarkDereferenceObjectUnsafe[TKey](keyMark)

	keyHash := hash.XXH3HasherHash64View[TKey](hasher, keyMark)
	h1, h2 := computeHashParts(keyHash)

	groupIDX, slot, found, _ := hashMapProbe(h, h1, h2, keyPtr, true,
		func(kvp *KeyValuePair[TKey, TValue]) *TKey {
			return memcore.MemcoreMarkDereferenceObject[TKey](kvp.keyMark)
		},
		memcore.MemcoreFunctionRetrieveTyped[KeyComparer[TKey]](h.keyComparerID),
	)
	if !found {
		return nil, fmt.Errorf("key not found")
	}
	pair := ArrayItemPtrGetAtUnsafe[KeyValuePair[TKey, TValue]](h.data, groupIDX*8+slot)
	return &pair.value, nil
}

// HashMapItemPtrGetFast returns a pointer to a value (not safe to persist).
func HashMapItemPtrGetFast[TKey, TValue any](
	instance memcore.MarkRaw,
	key TKey,
	keyCmp KeyComparer[TKey],
	markRetriever KeyMarkRetriever[TKey],
) (*TValue, error) {
	h := memcore.MemcoreMarkDereferenceObjectUnsafe[HashMap[TKey, TValue]](instance)

	keyMark := markRetriever(key)
	keyPtr := memcore.MemcoreMarkDereferenceObjectUnsafe[TKey](keyMark)

	keyHash := hash.XXH3HasherHash64View[TKey](hasher, keyMark)
	h1, h2 := computeHashParts(keyHash)

	groupIDX, slot, found, _ := hashMapProbe(h, h1, h2, keyPtr, true,
		func(kvp *KeyValuePair[TKey, TValue]) *TKey {
			return memcore.MemcoreMarkDereferenceObject[TKey](kvp.keyMark)
		},
		keyCmp,
	)
	if !found {
		return nil, fmt.Errorf("key not found")
	}
	pair := ArrayItemPtrGetAtUnsafe[KeyValuePair[TKey, TValue]](h.data, groupIDX*8+slot)
	return &pair.value, nil
}

// HashMapItemPtrGetUnsafe retrieves the pointer of the value for a key.
// This variant assumes all keys inside the map are still accessible in their original memory address.
// This is therefore only "safe" when you can guarantee the underlying memory has not relocated.
// It is a bit faster.
func HashMapItemPtrGetUnsafe[TKey, TValue any](instance memcore.MarkRaw, key TKey) (*TValue, error) {
	h := memcore.MemcoreMarkDereferenceObjectUnsafe[HashMap[TKey, TValue]](instance)

	retrieve := memcore.MemcoreFunctionRetrieveTyped[KeyMarkRetriever[TKey]](h.keyMarkRetrieverID)
	keyMark := retrieve(key)
	keyPtr := memcore.MemcoreMarkDereferenceObjectUnsafe[TKey](keyMark)

	keyHash := hash.XXH3HasherHash64View[TKey](hasher, keyMark)
	h1, h2 := computeHashParts(keyHash)

	groupIDX, slot, found, _ := hashMapProbe(h, h1, h2, keyPtr, true,
		func(kvp *KeyValuePair[TKey, TValue]) *TKey {
			return (*TKey)(unsafe.Pointer(kvp.keyUnsafe))
		},
		memcore.MemcoreFunctionRetrieveTyped[KeyComparer[TKey]](h.keyComparerID),
	)
	if !found {
		return nil, fmt.Errorf("key not found")
	}
	return &ArrayItemPtrGetAtUnsafe[KeyValuePair[TKey, TValue]](h.data, groupIDX*8+slot).value, nil
}

// HashMapItemPtrGetFastUnsafe retrieves the pointer of the value for a key.
// This variant assumes all keys inside the map are still accessible in their original memory address.
// This is therefore only "safe" when you can guarantee the underlying memory has not relocated.
// It is a bit faster.
func HashMapItemPtrGetFastUnsafe[TKey, TValue any](
	instance memcore.MarkRaw,
	key TKey,
	keyCmp KeyComparer[TKey],
	markRetriever KeyMarkRetriever[TKey],
) (*TValue, error) {
	h := memcore.MemcoreMarkDereferenceObjectUnsafe[HashMap[TKey, TValue]](instance)

	keyMark := markRetriever(key)
	keyPtr := memcore.MemcoreMarkDereferenceObjectUnsafe[TKey](keyMark)

	keyHash := hash.XXH3HasherHash64View[TKey](hasher, keyMark)
	h1, h2 := computeHashParts(keyHash)

	groupIDX, slot, found, _ := hashMapProbe(h, h1, h2, keyPtr, true,
		func(kvp *KeyValuePair[TKey, TValue]) *TKey {
			return (*TKey)(unsafe.Pointer(kvp.keyUnsafe))
		},
		keyCmp,
	)
	if !found {
		return nil, fmt.Errorf("key not found")
	}
	return &ArrayItemPtrGetAtUnsafe[KeyValuePair[TKey, TValue]](h.data, groupIDX*8+slot).value, nil
}

// HashMapItemDelete marks an entry as deleted.
func HashMapItemDelete[TKey, TValue any](instance memcore.MarkRaw, key TKey) {
	h := memcore.MemcoreMarkDereferenceObjectUnsafe[HashMap[TKey, TValue]](instance)

	retrieve := memcore.MemcoreFunctionRetrieveTyped[KeyMarkRetriever[TKey]](h.keyMarkRetrieverID)
	keyMark := retrieve(key)
	keyPtr := memcore.MemcoreMarkDereferenceObjectUnsafe[TKey](keyMark)

	keyHash := hash.XXH3HasherHash64View[TKey](hasher, keyMark)
	h1, h2 := computeHashParts(keyHash)

	groupIDX, slot, found, _ := hashMapProbe(h, h1, h2, keyPtr, true,
		func(kvp *KeyValuePair[TKey, TValue]) *TKey {
			return memcore.MemcoreMarkDereferenceObject[TKey](kvp.keyMark)
		},
		memcore.MemcoreFunctionRetrieveTyped[KeyComparer[TKey]](h.keyComparerID),
	)
	if !found {
		return
	}

	ctrlPtr := ArrayItemPtrGetAtUnsafe[ctrlGroup](h.metaData, groupIDX)
	*ctrlPtr = updateCtrlByte(*ctrlPtr, slot, ctrlDeleted)
	hashMapIncrementVersion[TKey, TValue](instance)
}

// HashMapItemDeleteFast marks an entry as deleted.
func HashMapItemDeleteFast[TKey, TValue any](
	instance memcore.MarkRaw,
	key TKey,
	keyCmp KeyComparer[TKey],
	markRetriever KeyMarkRetriever[TKey],
) {
	h := memcore.MemcoreMarkDereferenceObjectUnsafe[HashMap[TKey, TValue]](instance)

	keyMark := markRetriever(key)
	keyPtr := memcore.MemcoreMarkDereferenceObjectUnsafe[TKey](keyMark)

	keyHash := hash.XXH3HasherHash64View[TKey](hasher, keyMark)
	h1, h2 := computeHashParts(keyHash)

	groupIDX, slot, found, _ := hashMapProbe(h, h1, h2, keyPtr, true,
		func(kvp *KeyValuePair[TKey, TValue]) *TKey {
			return memcore.MemcoreMarkDereferenceObject[TKey](kvp.keyMark)
		},
		keyCmp,
	)
	if !found {
		return
	}

	ctrlPtr := ArrayItemPtrGetAtUnsafe[ctrlGroup](h.metaData, groupIDX)
	*ctrlPtr = updateCtrlByte(*ctrlPtr, slot, ctrlDeleted)
	hashMapIncrementVersion[TKey, TValue](instance)
}

// HashMapItemDeleteUnsafe marks an entry as deleted.
// This variant assumes all keys inside the map are still accessible in their original memory address.
// This is therefore only "safe" when you can guarantee the underlying memory has not relocated.
// It is a bit faster.
func HashMapItemDeleteUnsafe[TKey, TValue any](instance memcore.MarkRaw, key TKey) {
	h := memcore.MemcoreMarkDereferenceObjectUnsafe[HashMap[TKey, TValue]](instance)

	retrieve := memcore.MemcoreFunctionRetrieveTyped[KeyMarkRetriever[TKey]](h.keyMarkRetrieverID)
	keyMark := retrieve(key)
	keyPtr := memcore.MemcoreMarkDereferenceObjectUnsafe[TKey](keyMark)

	keyHash := hash.XXH3HasherHash64View[TKey](hasher, keyMark)
	h1, h2 := computeHashParts(keyHash)

	groupIDX, slot, found, _ := hashMapProbe(h, h1, h2, keyPtr, true,
		func(kvp *KeyValuePair[TKey, TValue]) *TKey {
			return (*TKey)(unsafe.Pointer(kvp.keyUnsafe))
		},
		memcore.MemcoreFunctionRetrieveTyped[KeyComparer[TKey]](h.keyComparerID),
	)
	if !found {
		return
	}

	ctrlPtr := ArrayItemPtrGetAtUnsafe[ctrlGroup](h.metaData, groupIDX)
	*ctrlPtr = updateCtrlByte(*ctrlPtr, slot, ctrlDeleted)
	hashMapIncrementVersion[TKey, TValue](instance)
}

// HashMapItemDeleteFastUnsafe marks an entry as deleted.
// This variant assumes all keys inside the map are still accessible in their original memory address.
// This is therefore only "safe" when you can guarantee the underlying memory has not relocated.
// It is a bit faster.
func HashMapItemDeleteFastUnsafe[TKey, TValue any](
	instance memcore.MarkRaw,
	key TKey,
	keyCmp KeyComparer[TKey],
	markRetriever KeyMarkRetriever[TKey],
) {
	h := memcore.MemcoreMarkDereferenceObjectUnsafe[HashMap[TKey, TValue]](instance)

	keyMark := markRetriever(key)
	keyPtr := memcore.MemcoreMarkDereferenceObjectUnsafe[TKey](keyMark)

	keyHash := hash.XXH3HasherHash64View[TKey](hasher, keyMark)
	h1, h2 := computeHashParts(keyHash)

	groupIDX, slot, found, _ := hashMapProbe(h, h1, h2, keyPtr, true,
		func(kvp *KeyValuePair[TKey, TValue]) *TKey {
			return (*TKey)(unsafe.Pointer(kvp.keyUnsafe))
		},
		keyCmp,
	)
	if !found {
		return
	}

	ctrlPtr := ArrayItemPtrGetAtUnsafe[ctrlGroup](h.metaData, groupIDX)
	*ctrlPtr = updateCtrlByte(*ctrlPtr, slot, ctrlDeleted)
	hashMapIncrementVersion[TKey, TValue](instance)
}

// HashMapClear clears the map by marking all slots inactive and resetting control bytes.
// This does NOT zero memory, only resets metadata and active flags.
//
// It uses an unrolled loop for each stride of 8 elements (the Swiss group size).
// This design eliminates branches and allows better compiler optimization / pipelining.
func HashMapClear[TKey, TValue any](instance memcore.MarkRaw) {
	h := memcore.MemcoreMarkDereferenceObjectUnsafe[HashMap[TKey, TValue]](instance)

	// ─── Reset metadata control groups ───
	ArrayForEachUnsafe[ctrlGroup](h.metaData, func(ptr unsafe.Pointer, _ uint64) {
		*(*uint64)(ptr) = bitsetEmpty
	})
	hashMapIncrementVersion[TKey, TValue](instance)
}

// HashMapClearAndZero fully zeroes metadata and data sections.
func HashMapClearAndZero[TKey, TValue any](instance memcore.MarkRaw) {
	h := memcore.MemcoreMarkDereferenceObjectUnsafe[HashMap[TKey, TValue]](instance)
	ArrayForEachUnsafe[ctrlGroup](h.metaData, func(p unsafe.Pointer, _ uint64) { *(*uint64)(p) = bitsetEmpty })
	ArrayClear[KeyValuePair[TKey, TValue]](h.data)
	hashMapIncrementVersion[TKey, TValue](instance)
}

// HashMapForEach iterates over all active key–value pairs in the map.
// The callback receives (*TKey, *TValue). Returning false stops iteration early.
func HashMapForEach[TKey, TValue any](
	instance memcore.MarkRaw,
	fn func(key *TKey, value *TValue) bool,
) {
	h := memcore.MemcoreMarkDereferenceObjectUnsafe[HashMap[TKey, TValue]](instance)

	dataCur := ArrayCursorCreate[KeyValuePair[TKey, TValue]](h.data)
	metaCur := ArrayCursorCreate[ctrlGroup](h.metaData)

	for g := uint64(0); g < h.logicalBins; g++ {
		ctrl := *metaCur.PtrAt(g)
		liveMask := ^ctrlGroupMatchEmptyOrDeleted(ctrl)

		if liveMask == 0 {
			continue
		}

		m := liveMask
		for m != 0 {
			s := bitsetNextIndex(m)
			m &= m - 1

			pair := dataCur.PtrAt(g*8 + s)
			key := memcore.MemcoreMarkDereferenceObjectUnsafe[TKey](pair.keyMark)
			if !fn(key, &pair.value) {
				return
			}
		}
	}
}

// HashMapKeys returns a slice containing all keys in the hashmap.
// This is not guaranteed to be stable when stored.
func HashMapKeys[TKey, TValue any](instance memcore.MarkRaw) []*TKey {
	keys := make([]*TKey, 0)

	HashMapForEach(instance, func(key *TKey, _ *TValue) bool {
		keys = append(keys, key)
		return true
	})

	return keys
}

// HashMapValues returns a slice containing all values in the hashmap.
// This is not guaranteed to be stable when stored.
func HashMapValues[TKey, TValue any](instance memcore.MarkRaw) []*TValue {
	values := make([]*TValue, 0)

	HashMapForEach(instance, func(_ *TKey, value *TValue) bool {
		values = append(values, value)
		return true
	})

	return values
}

// HashMapVersionGet returns the current version of the hashmap.
//
// Version increments on every modification to the hashmap data, allowing cache invalidation
// mechanisms to detect when cached values become stale.
//
//go:inline
func HashMapVersionGet[TKey, TValue any](instance memcore.MarkRaw) uint64 {
	h := memcore.MemcoreMarkDereferenceObjectUnsafe[HashMap[TKey, TValue]](instance)
	return h.version
}

// ------------------------------------------------------------
// Private Helpers (inline hot-path operations)
// ------------------------------------------------------------

type hashMapPairKeyAccessor[TKey, TValue any] func(kvp *KeyValuePair[TKey, TValue]) *TKey

//go:inline
//go:nosplit
func hashMapProbe[TKey, TValue any](
	h *HashMap[TKey, TValue],
	h1 uint64, h2 uint8, key *TKey,
	forInsert bool,
	pairKeyAccessor hashMapPairKeyAccessor[TKey, TValue],
	keyCmp KeyComparer[TKey],
) (groupIDX, slot uint64, found, hasFree bool) {
	const noIdx = ^uint64(0)

	mask := h.groupMask
	groupStart := h1 & mask
	availGroup, availSlot := noIdx, noIdx

	dataCur := ArrayCursorCreate[KeyValuePair[TKey, TValue]](h.data)
	metaCur := ArrayCursorCreate[ctrlGroup](h.metaData)

	// Align start down to a block of 4 groups
	base := groupStart &^ 3

	for probed := uint64(0); probed < h.logicalBins; {
		remain := h.logicalBins - probed
		n := uint64(4)
		if remain < 4 {
			n = remain
		}

		// Prefetch up to 4 groups
		var ctrl [4]ctrlGroup
		for i := uint64(0); i < n; i++ {
			g := (base + i) & mask
			ctrl[i] = *metaCur.PtrAt(g)
		}

		// 1) Match candidates in order (base, base+1, base+2, base+3)
		for i := uint64(0); i < n; i++ {
			g := (base + i) & mask
			m := ctrlGroupMatchH2(ctrl[i], h2)
			for m != 0 {
				s := bitsetNextIndex(m)
				m &= m - 1

				pair := dataCur.PtrAt(g*8 + s)
				pairKey := pairKeyAccessor(pair)
				if keyCmp(pairKey, key) {
					return g, s, true, false
				}
			}
		}

		// 2) Track first available (empty or deleted) if we don't have one yet
		if availGroup == noIdx {
			for i := uint64(0); i < n; i++ {
				available := ctrlGroupMatchEmptyOrDeleted(ctrl[i])
				if available != 0 {
					empty := ctrlGroupMatchEmpty(ctrl[i])
					var s uint64
					if deleted := available &^ empty; deleted != 0 {
						s = bitsetNextIndex(deleted)
					} else {
						s = bitsetNextIndex(available)
					}
					availGroup, availSlot = (base+i)&mask, s
					break
				}
			}
		}

		stop := false
		for i := uint64(0); i < n; i++ {
			empty := ctrlGroupMatchEmpty(ctrl[i])
			if (!forInsert && empty != 0) || (forInsert && (ctrlGroupMatchEmptyOrDeleted(ctrl[i]) != 0)) {
				stop = true
				break
			}
		}

		probed += n
		if stop {
			break
		}

		base = (base + 4) & mask
	}

	if availGroup != noIdx {
		return availGroup, availSlot, false, true
	}
	return 0, 0, false, false
}

//go:nosplit
//go:inline
func computeHashParts(hash uint64) (uint64, uint8) {
	return hash >> 7, uint8(hash & 0x7F)
}

//go:nosplit
//go:inline
func ctrlGroupMatchH2(group ctrlGroup, h2 uint8) bitset {
	v := uint64(group) ^ (bitsetLSB * uint64(h2))
	return bitset(((v - bitsetLSB) &^ v) & bitsetMSB)
}

//go:nosplit
//go:inline
func ctrlGroupMatchEmpty(g ctrlGroup) bitset {
	v := uint64(g)
	return bitset((v &^ (v << 6)) & bitsetMSB)
}

//go:nosplit
//go:inline
func ctrlGroupMatchEmptyOrDeleted(g ctrlGroup) bitset {
	return bitset(uint64(g) & bitsetMSB)
}

//go:nosplit
//go:inline
func bitsetNextIndex(b bitset) uint64 {
	return uint64(bits.TrailingZeros64(uint64(b))) >> 3
}

//go:nosplit
//go:inline
func updateCtrlByte(group ctrlGroup, slot uint64, c ctrl) ctrlGroup {
	const byteMask = 0xFF
	shift := slot * 8
	mask := uint64(byteMask) << shift
	return ctrlGroup((uint64(group) &^ mask) | (uint64(c) << shift))
}

//go:inline
func hashMapIncrementVersion[TKey, TValue any](instance memcore.MarkRaw) {
	h := memcore.MemcoreMarkDereferenceObjectUnsafe[HashMap[TKey, TValue]](instance)
	h.version++
}
