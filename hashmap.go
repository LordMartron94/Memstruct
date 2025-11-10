package memstruct

import (
	"fmt"
	"foundation/hash"
	"memcore"
	"unsafe"
)

var hasher *hash.XXH3Hasher

func init() {
	hasher = hash.XXH3HasherCreateWithSeed(42)
}

// KeyValuePair represents a pair between an arbitrary key and a value.
// It provides some utility functions to use in certain scenarios.
type KeyValuePair[TKey, TValue any] struct {
	key     TKey
	value   TValue
	keyHash uint64
	active  bool
}

func (k KeyValuePair[TKey, TValue]) Key() TKey {
	return k.key
}

func (k KeyValuePair[TKey, TValue]) Value() TValue {
	return k.value
}

func (k KeyValuePair[TKey, TValue]) KeyHash() uint64 {
	return k.keyHash
}

// HashMap is a custom map implementation.
type HashMap[TKey, TValue any] struct {
	data          memcore.MarkRaw // Array[KeyValuePair[TKey, TValue]]
	capacity      uint64
	keyComparerID memcore.FunctionID
}

func HashMapRequiredBytesGet[TKey, TValue any](capacityElements uint64) uint64 {
	headerSize := memcore.SizeOf[HashMap[TKey, TValue]]()
	dataSize := ArrayRequiredBytesGet[KeyValuePair[TKey, TValue]](capacityElements)
	return headerSize + dataSize
}

func HashMapRequiredAlignmentGet[TKey, TValue any]() uint64 {
	return max(memcore.AlignOf[HashMap[TKey, TValue]](), ArrayRequiredAlignmentGet[KeyValuePair[TKey, TValue]]())
}

// KeyComparer must return true when two keys are considered equal.
type KeyComparer[TKey any] func(a, b TKey) bool

// HashMapInitializeAt initializes an instance of a hashmap for type TKey and TValue at a specific memory address.
// Ensure the address is properly aligned and has the right size.
//
// ⚠️ capacity is in elements, not bytes.
func HashMapInitializeAt[TKey, TValue any](addr memcore.MarkRaw, capacityElements uint64, keyFn KeyComparer[TKey]) {
	headerSize := memcore.SizeOf[HashMap[TKey, TValue]]()
	headerAlignment := memcore.AlignOf[HashMap[TKey, TValue]]()

	dataAddr := memcore.MemcoreMarkAlignedOffsetFrom(addr, uintptr(headerSize), headerAlignment)
	ArrayInitializeAt[KeyValuePair[TKey, TValue]](dataAddr, capacityElements)

	if serializer := memcore.MemcoreRegisteredSerializerGet[TKey](); serializer == nil {
		panic("invalid TKey type: cannot serialize")
	}

	keyFnID := memcore.MemcoreFunctionRegisterTyped(keyFn)

	header := memcore.MemcoreMarkDereferenceObject[HashMap[TKey, TValue]](addr)
	*header = HashMap[TKey, TValue]{
		data:          dataAddr,
		capacity:      capacityElements,
		keyComparerID: keyFnID,
	}
}

// HashMapItemAdd adds an item into the map.
// It overwrites the given value if the key already exists in the map.
// It panics if there is no space left in the map.
// HashMapItemAdd inserts or overwrites a key-value pair.
// Uses open addressing with linear probing.
// Panics if the table is full.
func HashMapItemAdd[TKey, TValue any](instance memcore.MarkRaw, key TKey, value TValue) {
	header := memcore.MemcoreMarkDereferenceObject[HashMap[TKey, TValue]](instance)

	bytes, _ := memcore.MemcoreSerialize(&key)
	keyHash := hash.XXH3HasherHash64(hasher, bytes)

	kvp, found := hashMapFindSlot(header, keyHash, &key)
	if found {
		kvp.value = value
	} else {
		*kvp = KeyValuePair[TKey, TValue]{
			key:     key,
			value:   value,
			keyHash: keyHash,
			active:  true,
		}
	}
}

// HashMapKeyValuePairPtrGet retrieves the selected keyvalue pair as a pointer from the hashmap.
// If it is not existent within the map, it returns an error.
// This can not be stored inside manually managed memory, nor is it guaranteed to be stable when stored anywhere.
func HashMapKeyValuePairPtrGet[TKey, TValue any](instance memcore.MarkRaw, key TKey) (*KeyValuePair[TKey, TValue], error) {
	header := memcore.MemcoreMarkDereferenceObject[HashMap[TKey, TValue]](instance)

	bytes, _ := memcore.MemcoreSerialize(&key)
	keyHash := hash.XXH3HasherHash64(hasher, bytes)

	kvp, found := hashMapFindSlot(header, keyHash, &key)
	if !found || !kvp.active {
		return nil, fmt.Errorf("item with key %v not found", key)
	}
	return kvp, nil
}

// HashMapKeyValuePairGet retrieves the selected keyvalue pair from the hashmap.
// If it is not existent within the map, it returns an error.
func HashMapKeyValuePairGet[TKey, TValue any](instance memcore.MarkRaw, key TKey) (KeyValuePair[TKey, TValue], error) {
	if kvp, err := HashMapKeyValuePairPtrGet[TKey, TValue](instance, key); err != nil {
		var zero KeyValuePair[TKey, TValue]
		return zero, err
	} else {
		return *kvp, nil
	}
}

// HashMapItemGet retrieves the selected value from the hashmap.
// If it is not existent within the map, it returns an error.
func HashMapItemGet[TKey, TValue any](instance memcore.MarkRaw, key TKey) (TValue, error) {
	if kvp, err := HashMapKeyValuePairGet[TKey, TValue](instance, key); err != nil {
		var zero TValue
		return zero, err
	} else {
		return kvp.value, nil
	}
}

// HashMapItemPtrGet retrieves the selected value as a pointer from the hashmap.
// If it is not existent within the map, it returns an error.
// This can not be stored inside manually managed memory, nor is it guaranteed to be stable when stored anywhere.
func HashMapItemPtrGet[TKey, TValue any](instance memcore.MarkRaw, key TKey) (*TValue, error) {
	if kvp, err := HashMapKeyValuePairPtrGet[TKey, TValue](instance, key); err != nil {
		return nil, err
	} else {
		return &kvp.value, nil
	}
}

// HashMapItemDelete deletes an item from the map.
// If the item is not in the map, this does nothing (but still costs computation)
func HashMapItemDelete[TKey, TValue any](instance memcore.MarkRaw, key TKey) {
	header := memcore.MemcoreMarkDereferenceObject[HashMap[TKey, TValue]](instance)

	bytes, _ := memcore.MemcoreSerialize(&key)
	keyHash := hash.XXH3HasherHash64(hasher, bytes)

	kvp, found := hashMapFindSlot(header, keyHash, &key)
	if found {
		kvp.active = false
	}
}

// HashMapClear clears the map, allowing all keys to be inserted again.
// This only sets the metadata of the pairs to inactive, it does NOT zero memory.
func HashMapClear[TKey, TValue any](instance memcore.MarkRaw) {
	header := memcore.MemcoreMarkDereferenceObject[HashMap[TKey, TValue]](instance)

	itemSize := memcore.SizeOf[KeyValuePair[TKey, TValue]]()

	ArrayStrideForEachUnsafe[KeyValuePair[TKey, TValue]](
		header.data,
		func(ptr unsafe.Pointer, idx uint64) {
			(*KeyValuePair[TKey, TValue])(ptr).active = false
			(*KeyValuePair[TKey, TValue])(unsafe.Add(ptr, itemSize*1)).active = false
			(*KeyValuePair[TKey, TValue])(unsafe.Add(ptr, itemSize*2)).active = false
			(*KeyValuePair[TKey, TValue])(unsafe.Add(ptr, itemSize*3)).active = false
			(*KeyValuePair[TKey, TValue])(unsafe.Add(ptr, itemSize*4)).active = false
			(*KeyValuePair[TKey, TValue])(unsafe.Add(ptr, itemSize*5)).active = false
			(*KeyValuePair[TKey, TValue])(unsafe.Add(ptr, itemSize*6)).active = false
			(*KeyValuePair[TKey, TValue])(unsafe.Add(ptr, itemSize*7)).active = false
		},
		func(ptr unsafe.Pointer, idx uint64) {
			(*KeyValuePair[TKey, TValue])(ptr).active = false
		}, // tail fn
		8, // stride of 8
	)
}

// HashMapClearAndZero clears the map, allowing all keys to be inserted again.
// This zeroes the underlying memory.
func HashMapClearAndZero[TKey, TValue any](instance memcore.MarkRaw) {
	header := memcore.MemcoreMarkDereferenceObject[HashMap[TKey, TValue]](instance)
	ArrayClear[KeyValuePair[TKey, TValue]](header.data)
}

// -------------------------------------------------------- PRIVATE HELPERS

//go:inline
//go:nosplit
func hashMapFindSlot[TKey, TValue any](
	header *HashMap[TKey, TValue],
	keyHash uint64,
	key *TKey,
) (kvp *KeyValuePair[TKey, TValue], found bool) {
	comparer := memcore.MemcoreFunctionRetrieveTyped[KeyComparer[TKey]](header.keyComparerID)

	start := hashToArrayIDX(header, keyHash)
	idx := start

	for {
		kvp = ArrayItemPtrGetAtUnsafe[KeyValuePair[TKey, TValue]](header.data, idx)

		if !kvp.active {
			return kvp, false
		}

		if kvp.keyHash == keyHash {
			if comparer(kvp.key, *key) {
				return kvp, true
			}
		}

		idx++
		if idx == header.capacity {
			idx = 0
		}
		if idx == start {
			panic(fmt.Errorf("no space left in map with capacity: %d", header.capacity))
		}
	}
}

//go:inline
//go:nosplit
func hashToArrayIDX[TKey, TValue any](instance *HashMap[TKey, TValue], hash uint64) uint64 {
	return hash % instance.capacity
}
