package memstruct

import (
	"foundation/hash"
	"memcore"
	"unsafe"
)

/*
HashMapCursorHeader caches the HashMap header pointer and function retrievers for hot-path operations that bypass MarkRaw dereference overhead.

This cursor type stores a direct pointer to the HashMap header and caches the function retrievers
for key comparison and key mark retrieval, eliminating the need to dereference MarkRaw and
retrieve functions on every operation. This is optimized for hot paths where memory movement is
guaranteed not to occur during the cursor's lifetime.

Use cases:
- High-performance loops with repeated hashmap operations
- Batch processing operations with many hashmap insert/lookup operations
- Hot paths where memory stability is guaranteed
- Reducing overhead in performance-critical code sections

Time complexity: O(1) - structure initialization is constant-time
Space complexity: O(1) - fixed-size structure

Prerequisites:
- Must be created using HashMapCursorHeaderCreate
- HashMap memory must remain valid and unmoved for cursor lifetime
- Memory movement invalidates the cursor (undefined behavior if used after movement)

Edge cases:
- Cursor becomes invalid if hashmap memory is deallocated or unregistered
- Cursor becomes invalid if memory region is relocated
- Function retrievers are cached at cursor creation time

Additional notes:
- Cursor caches the header pointer and function retrievers for maximum performance
- Uses unsafe pointer operations for direct memory access
- Type safety is maintained through the generic parameters TKey and TValue
- Suitable for both sequential and random access patterns
- The cursor is a value type and can be copied, but shares the same underlying header
*/
type HashMapCursorHeader[TKey, TValue any] struct {
	header            *HashMap[TKey, TValue]
	keyComparer       KeyComparer[TKey]
	keyMarkRetriever  KeyMarkRetriever[TKey]
}

/*
HashMapCursorHeaderCreate creates a cursor that caches the HashMap header pointer and function retrievers for hot-path operations.

This function performs a single MarkRaw dereference and caches the resulting header pointer and
function retrievers, enabling subsequent operations to bypass the dereference and function lookup
overhead. The cursor is optimized for hot paths where memory movement is guaranteed not to occur
during the cursor's lifetime.

Use cases:
- Setting up cursor-based hashmap operations
- High-performance hashmap manipulation patterns
- Batch processing operations
- Performance-critical code sections

Time complexity: O(1) - single dereference and function retrieval operations
Space complexity: O(1) - returns a fixed-size cursor structure

Prerequisites:
- hashmap must be a valid MarkRaw pointing to an initialized HashMap[TKey, TValue]
- HashMap must not have been deallocated or unregistered
- Memory must remain stable during cursor lifetime

Edge cases:
- Cursor becomes invalid if hashmap memory is deallocated or unregistered
- Cursor becomes invalid if memory region is relocated
- Returns cursor with nil header if hashmap is invalid (undefined behavior)

Additional notes:
- Cursor caches header pointer and function retrievers for maximum performance
- Uses unsafe pointer operations for direct memory access
- No header manipulation required after cursor creation
- Type safety is maintained through the generic parameters TKey and TValue
- The cursor is a value type and can be copied, but shares the same underlying header
*/
//
//go:inline
func HashMapCursorHeaderCreate[TKey, TValue any](hashmap memcore.MarkRaw) HashMapCursorHeader[TKey, TValue] {
	header := memcore.MemcoreMarkDereferenceObjectUnsafe[HashMap[TKey, TValue]](hashmap)
	keyComparer := memcore.MemcoreFunctionRetrieveTyped[KeyComparer[TKey]](header.keyComparerID)
	keyMarkRetriever := memcore.MemcoreFunctionRetrieveTyped[KeyMarkRetriever[TKey]](header.keyMarkRetrieverID)
	
	return HashMapCursorHeader[TKey, TValue]{
		header:           header,
		keyComparer:      keyComparer,
		keyMarkRetriever: keyMarkRetriever,
	}
}

/*
HashMapCursorHeaderItemAdd inserts or updates a key–value pair in the hashmap.

This function uses the cached header pointer and function retrievers to add or update a key–value
pair, bypassing MarkRaw dereference and function lookup overhead.

Use cases:
- Cursor-based hashmap insert/update operations
- Batch processing operations
- Performance-critical code sections

Time complexity: O(1) average case, O(n) worst case - hashmap probe operation
Space complexity: O(1) - no allocations

Prerequisites:
- cursor must be a valid HashMapCursorHeader created with HashMapCursorHeaderCreate
- HashMap must not be at capacity
- HashMap memory must remain valid and unmoved

Edge cases:
- Panics if hashmap is at capacity (no space left)
- Increments hashmap version on successful add/update
- Undefined behavior if cursor is invalid or memory has moved

Additional notes:
- Uses cached header pointer and function retrievers for maximum performance
- Implements SwissTable-style open addressing
- Type safety is maintained through the generic parameters TKey and TValue
*/
//
//go:inline
func HashMapCursorHeaderItemAdd[TKey, TValue any](cursor HashMapCursorHeader[TKey, TValue], key TKey, value TValue) {
	keyMark := cursor.keyMarkRetriever(key)
	keyPtr := memcore.MemcoreMarkDereferenceObjectUnsafe[TKey](keyMark)
	keyHash := hash.XXH3HasherHash64View[TKey](hasher, keyMark)
	h1, h2 := computeHashParts(keyHash)

	groupIDX, slot, found, hasFree := hashMapProbe(cursor.header, h1, h2, keyPtr, true,
		func(kvp *KeyValuePair[TKey, TValue]) *TKey {
			return memcore.MemcoreMarkDereferenceObjectUnsafe[TKey](kvp.keyMark)
		},
		cursor.keyComparer,
	)
	if found {
		ArrayItemPtrGetAtUnsafe[KeyValuePair[TKey, TValue]](cursor.header.data, groupIDX*8+slot).value = value
		cursor.header.version++
		return
	}
	if !hasFree {
		panic("HashMapCursorHeaderItemAdd: no space left")
	}

	pair := ArrayItemPtrGetAtUnsafe[KeyValuePair[TKey, TValue]](cursor.header.data, groupIDX*8+slot)
	pair.value = value
	pair.keyMark = keyMark
	pair.keyUnsafe = uintptr(unsafe.Pointer(keyPtr))
	ctrlPtr := ArrayItemPtrGetAtUnsafe[ctrlGroup](cursor.header.metaData, groupIDX)
	*ctrlPtr = updateCtrlByte(*ctrlPtr, slot, ctrl(h2))
	cursor.header.version++
}

/*
HashMapCursorHeaderItemGet retrieves the value for a key from the hashmap.

This function uses the cached header pointer and function retrievers to get a value by key,
bypassing MarkRaw dereference and function lookup overhead.

Use cases:
- Cursor-based hashmap lookup operations
- Batch processing operations
- Performance-critical code sections

Time complexity: O(1) average case, O(n) worst case - hashmap probe operation
Space complexity: O(1) - no allocations

Prerequisites:
- cursor must be a valid HashMapCursorHeader created with HashMapCursorHeaderCreate
- HashMap memory must remain valid and unmoved

Edge cases:
- Returns error if key is not found
- Returns zero value and error if key is not found
- Undefined behavior if cursor is invalid or memory has moved

Additional notes:
- Uses cached header pointer and function retrievers for maximum performance
- Implements SwissTable-style open addressing
- Type safety is maintained through the generic parameters TKey and TValue
*/
//
//go:inline
func HashMapCursorHeaderItemGet[TKey, TValue any](cursor HashMapCursorHeader[TKey, TValue], key TKey) (TValue, bool) {
	keyMark := cursor.keyMarkRetriever(key)
	keyPtr := memcore.MemcoreMarkDereferenceObjectUnsafe[TKey](keyMark)
	keyHash := hash.XXH3HasherHash64View[TKey](hasher, keyMark)
	h1, h2 := computeHashParts(keyHash)

	groupIDX, slot, found, _ := hashMapProbe(cursor.header, h1, h2, keyPtr, true,
		func(kvp *KeyValuePair[TKey, TValue]) *TKey {
			return memcore.MemcoreMarkDereferenceObject[TKey](kvp.keyMark)
		},
		cursor.keyComparer,
	)
	if !found {
		var zero TValue
		return zero, false
	}
	return ArrayItemPtrGetAtUnsafe[KeyValuePair[TKey, TValue]](cursor.header.data, groupIDX*8+slot).value, true
}

/*
HashMapCursorHeaderItemDelete marks an entry as deleted in the hashmap.

This function uses the cached header pointer and function retrievers to delete a key–value pair,
bypassing MarkRaw dereference and function lookup overhead.

Use cases:
- Cursor-based hashmap delete operations
- Batch processing operations
- Performance-critical code sections

Time complexity: O(1) average case, O(n) worst case - hashmap probe operation
Space complexity: O(1) - no allocations

Prerequisites:
- cursor must be a valid HashMapCursorHeader created with HashMapCursorHeaderCreate
- HashMap memory must remain valid and unmoved

Edge cases:
- Does nothing if key is not found
- Increments hashmap version on successful delete
- Undefined behavior if cursor is invalid or memory has moved

Additional notes:
- Uses cached header pointer and function retrievers for maximum performance
- Implements SwissTable-style open addressing
- Marks entry as deleted rather than removing it
- Type safety is maintained through the generic parameters TKey and TValue
*/
//
//go:inline
func HashMapCursorHeaderItemDelete[TKey, TValue any](cursor HashMapCursorHeader[TKey, TValue], key TKey) {
	keyMark := cursor.keyMarkRetriever(key)
	keyPtr := memcore.MemcoreMarkDereferenceObjectUnsafe[TKey](keyMark)
	keyHash := hash.XXH3HasherHash64View[TKey](hasher, keyMark)
	h1, h2 := computeHashParts(keyHash)

	groupIDX, slot, found, _ := hashMapProbe(cursor.header, h1, h2, keyPtr, true,
		func(kvp *KeyValuePair[TKey, TValue]) *TKey {
			return memcore.MemcoreMarkDereferenceObject[TKey](kvp.keyMark)
		},
		cursor.keyComparer,
	)
	if !found {
		return
	}

	ctrlPtr := ArrayItemPtrGetAtUnsafe[ctrlGroup](cursor.header.metaData, groupIDX)
	*ctrlPtr = updateCtrlByte(*ctrlPtr, slot, ctrlDeleted)
	cursor.header.version++
}

/*
HashMapCursorHeaderVersionGet returns the current version of the hashmap.

This function uses the cached header pointer to access the hashmap version, bypassing MarkRaw
dereference overhead. The version increments on every modification to the hashmap data, allowing
cache invalidation mechanisms to detect when cached values become stale.

Use cases:
- Cache invalidation checks
- Detecting hashmap modifications
- Version-based synchronization

Time complexity: O(1) - returns cached value
Space complexity: O(1) - no allocations

Prerequisites:
- cursor must be a valid HashMapCursorHeader created with HashMapCursorHeaderCreate
- HashMap memory must remain valid and unmoved

Edge cases:
- Returns the hashmap's current version number
- Version increments on every modification operation
- Undefined behavior if cursor is invalid or memory has moved

Additional notes:
- Uses cached header pointer for maximum performance
- Version is stored in the hashmap header
- Version starts at 1 when hashmap is initialized
- Type safety is maintained through the generic parameters TKey and TValue
*/
//
//go:inline
func HashMapCursorHeaderVersionGet[TKey, TValue any](cursor HashMapCursorHeader[TKey, TValue]) uint64 {
	return cursor.header.version
}

