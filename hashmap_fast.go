package memstruct

import "memcore"

// HashMapVersionGetFast returns version from a pre-dereferenced hashmap header.
//
//go:inline
func HashMapVersionGetFast[TKey, TValue any](h *HashMap[TKey, TValue]) uint64 {
	return h.version
}

// hashMapIncrementVersionFast increments version on a pre-dereferenced header.
//
//go:inline
func hashMapIncrementVersionFast[TKey, TValue any](h *HashMap[TKey, TValue]) {
	h.version++
}

// HashMapForEachFast iterates active pairs using a pre-dereferenced hashmap header.
func HashMapForEachFast[TKey, TValue any](
	h *HashMap[TKey, TValue],
	fn func(key *TKey, value *TValue) bool,
) {
	for g := uint64(0); g < h.logicalBins; g++ {
		ctrl := *ArrayItemPtrGetAtUnsafeFast(h.meta, h.metaBase, g)
		liveMask := ^ctrlGroupMatchEmptyOrDeleted(ctrl)

		if liveMask == 0 {
			continue
		}

		m := liveMask
		for m != 0 {
			s := bitsetNextIndex(m)
			m &= m - 1

			pair := ArrayItemPtrGetAtUnsafeFast(h.data, h.dataBase, g*8+s)
			key := memcore.MemcoreMarkDereferenceObjectUnsafe[TKey](pair.keyMark)
			if !fn(key, &pair.value) {
				return
			}
		}
	}
}
