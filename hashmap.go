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
	value   TValue
	keyMark memcore.MarkRaw
}

func (k *KeyValuePair[TKey, TValue]) Value() TValue { return k.value }

// HashMap is a SwissTable-like open-addressing hashmap.
type HashMap[TKey, TValue any] struct {
	data, metaData        memcore.MarkRaw // backing arrays
	capacity, logicalBins uint64
	keyComparerID         memcore.FunctionID
	keyMarkRetrieverID    memcore.FunctionID
	groupMask             uint64
}

// KeyComparer must return true if *a and *b represent the same key.
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

	groupIDX, slot, found, hasFree := hashMapProbe(h, h1, h2, keyPtr, true)
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
	ctrlPtr := ArrayItemPtrGetAtUnsafe[ctrlGroup](h.metaData, groupIDX)
	*ctrlPtr = updateCtrlByte(*ctrlPtr, slot, ctrl(h2))
}

// HashMapItemGet retrieves the value for a key.
func HashMapItemGet[TKey, TValue any](instance memcore.MarkRaw, key TKey) (TValue, error) {
	h := memcore.MemcoreMarkDereferenceObjectUnsafe[HashMap[TKey, TValue]](instance)

	retrieve := memcore.MemcoreFunctionRetrieveTyped[KeyMarkRetriever[TKey]](h.keyMarkRetrieverID)
	keyMark := retrieve(key)
	keyPtr := memcore.MemcoreMarkDereferenceObjectUnsafe[TKey](keyMark)

	keyHash := hash.XXH3HasherHash64View[TKey](hasher, keyMark)
	h1, h2 := computeHashParts(keyHash)

	groupIDX, slot, found, _ := hashMapProbe(h, h1, h2, keyPtr, false)
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

	groupIDX, slot, found, _ := hashMapProbe(h, h1, h2, keyPtr, false)
	if !found {
		return nil, fmt.Errorf("key not found")
	}
	pair := ArrayItemPtrGetAtUnsafe[KeyValuePair[TKey, TValue]](h.data, groupIDX*8+slot)
	return &pair.value, nil
}

// HashMapItemDelete marks an entry as deleted.
func HashMapItemDelete[TKey, TValue any](instance memcore.MarkRaw, key TKey) {
	h := memcore.MemcoreMarkDereferenceObjectUnsafe[HashMap[TKey, TValue]](instance)

	retrieve := memcore.MemcoreFunctionRetrieveTyped[KeyMarkRetriever[TKey]](h.keyMarkRetrieverID)
	keyMark := retrieve(key)
	keyPtr := memcore.MemcoreMarkDereferenceObjectUnsafe[TKey](keyMark)

	keyHash := hash.XXH3HasherHash64View[TKey](hasher, keyMark)
	h1, h2 := computeHashParts(keyHash)

	groupIDX, slot, found, _ := hashMapProbe(h, h1, h2, keyPtr, false)
	if !found {
		return
	}

	ctrlPtr := ArrayItemPtrGetAtUnsafe[ctrlGroup](h.metaData, groupIDX)
	*ctrlPtr = updateCtrlByte(*ctrlPtr, slot, ctrlDeleted)
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
}

// HashMapClearAndZero fully zeroes metadata and data sections.
func HashMapClearAndZero[TKey, TValue any](instance memcore.MarkRaw) {
	h := memcore.MemcoreMarkDereferenceObjectUnsafe[HashMap[TKey, TValue]](instance)
	ArrayForEachUnsafe[ctrlGroup](h.metaData, func(p unsafe.Pointer, _ uint64) { *(*uint64)(p) = bitsetEmpty })
	ArrayClear[KeyValuePair[TKey, TValue]](h.data)
}

// ------------------------------------------------------------
// Private Helpers (inline hot-path operations)
// ------------------------------------------------------------

//go:nosplit
func hashMapProbe[TKey, TValue any](
	h *HashMap[TKey, TValue],
	h1 uint64, h2 uint8, key *TKey,
	forInsert bool,
) (groupIDX, slot uint64, found, hasFree bool) {
	const noIdx = ^uint64(0)
	groupIDX = h1 & h.groupMask
	availGroup, availSlot := noIdx, noIdx
	keyCmp := memcore.MemcoreFunctionRetrieveTyped[KeyComparer[TKey]](h.keyComparerID)

	dataCursor := ArrayCursorCreate[KeyValuePair[TKey, TValue]](h.data)
	metadataCursor := ArrayCursorCreate[ctrlGroup](h.metaData)

	for probe := uint64(0); probe < h.logicalBins; probe++ {
		ctrl := *metadataCursor.PtrAt(groupIDX)

		// 1. Match possible h2 candidates
		match := ctrlGroupMatchH2(ctrl, h2)
		for match != 0 {
			s := bitsetNextIndex(match)
			match &= match - 1
			pair := dataCursor.PtrAt(groupIDX*8 + s)
			pairKey := memcore.MemcoreMarkDereferenceObjectUnsafe[TKey](pair.keyMark)
			if keyCmp(pairKey, key) {
				return groupIDX, s, true, false
			}
		}

		// 2. Check available (empty or deleted)
		available := ctrlGroupMatchEmptyOrDeleted(ctrl)
		if available != 0 && availGroup == noIdx {
			empty := ctrlGroupMatchEmpty(ctrl)
			deleted := available &^ empty
			if deleted != 0 {
				availSlot = bitsetNextIndex(deleted)
			} else {
				availSlot = bitsetNextIndex(available)
			}
			availGroup = groupIDX
		}

		// 3. Determine if we should stop based on mode
		empty := ctrlGroupMatchEmpty(ctrl)
		if (forInsert && available != 0) || (!forInsert && empty != 0) {
			break
		}
		groupIDX = (groupIDX + 1) & h.groupMask
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
