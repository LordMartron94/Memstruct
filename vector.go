package memstruct

import (
	"fmt"
	"foundation"
	"memcore"
	"strings"
	"unsafe"
)

// Vector is structurally equivalent to Array except specialized for numerics.
// All Array functions work as well for the Vector.
// For convenience, most, if not all, of these are wrapped under a facade with the Vector prefix.
type Vector[T foundation.Numeric] Array[T]

// VectorView represents a view around a vector.
// It can be readonly and/or a subset within the vector.
type VectorView[T foundation.Numeric] ArrayView[T]

func VectorRequiredBytesGet[T foundation.Numeric](capacity uint64) uint64 {
	return ArrayRequiredBytesGet[T](capacity)
}

func VectorRequiredAlignmentGet[T foundation.Numeric]() uint64 {
	return ArrayRequiredAlignmentGet[T]()
}

/*
VectorHeaderRequiredBytesGet returns the number of bytes required to store the Vector header structure.

This function wraps ArrayHeaderRequiredBytesGet for numeric types, calculating the size of the
Vector header only, excluding the data region. It is useful when allocating memory separately
for the header and data regions, or when calculating memory requirements for non-contiguous
memory layouts.

Use cases:
- Calculating memory requirements for separated header/data allocations
- Memory pool implementations that store headers separately
- Custom allocators that need precise header size information
- Memory layout planning and optimization

Time complexity: O(1) - compile-time constant evaluation
Space complexity: O(1) - no allocations

Prerequisites:
- Type T must be a numeric type (foundation.Numeric)

Edge cases:
- Returns the size of the Vector struct itself, which includes metadata fields
- Does not include padding or alignment considerations (use VectorHeaderRequiredAlignmentGet for alignment)
- Size is determined at compile time based on the Vector struct definition

Additional notes:
- The returned size is the exact size of the Vector header structure
- For total memory requirements including data, use VectorRequiredBytesGet
- Internally delegates to ArrayHeaderRequiredBytesGet
*/
func VectorHeaderRequiredBytesGet[T foundation.Numeric]() uint64 {
	return ArrayHeaderRequiredBytesGet[T]()
}

/*
VectorHeaderRequiredAlignmentGet returns the required memory alignment for the Vector header structure.

This function wraps ArrayHeaderRequiredAlignmentGet for numeric types. The alignment requirement
ensures that the Vector header is placed at a memory address that is a multiple of the returned
value. This is typically the alignment requirement of the element type T, which ensures optimal
memory access patterns and cache efficiency.

Use cases:
- Memory allocation alignment calculations
- Memory pool implementations requiring proper alignment
- Custom allocators that need alignment information
- Cache-optimized memory layout planning

Time complexity: O(1) - compile-time constant evaluation
Space complexity: O(1) - no allocations

Prerequisites:
- Type T must be a numeric type (foundation.Numeric)

Edge cases:
- Returns the alignment requirement of type T, which matches the header's alignment needs
- Alignment values are always powers of two
- Zero alignment is never returned (minimum alignment is 1)

Additional notes:
- The alignment is determined by the element type T, not the Vector struct itself
- This ensures that when the header is properly aligned, subsequent data access is also aligned
- For total alignment requirements including data, use VectorRequiredAlignmentGet
- Internally delegates to ArrayHeaderRequiredAlignmentGet
*/
func VectorHeaderRequiredAlignmentGet[T foundation.Numeric]() uint64 {
	return ArrayHeaderRequiredAlignmentGet[T]()
}

func (v *Vector[T]) String() string {
	if v == nil {
		return "<nil Vector>"
	}

	var sb strings.Builder
	fmt.Fprintf(&sb, "Vector[%T]{capacity=%d, itemSize=%d, data=[", *new(T), v.capacity, v.itemSize)

	baseAddr := unsafe.Pointer(v)
	dataAddr := unsafe.Add(baseAddr, v.dataAddrOffset)

	for i := uint64(0); i < v.capacity; i++ {
		if i > 0 {
			sb.WriteString(", ")
		}
		val := *(*T)(unsafe.Add(dataAddr, uintptr(i)*v.itemSize))
		fmt.Fprintf(&sb, "%v", val)
	}

	sb.WriteString("]}")
	return sb.String()
}

/*
VectorInitializeAt initializes a vector instance for numeric type T at a specific memory address,
assuming a contiguous memory layout where the header is immediately followed by the data region.

This function wraps ArrayInitializeAt for numeric types, setting up the vector header and calculating
the data address offset based on the assumption that data immediately follows the header in memory.
The memory at vectorAddr must be large enough to accommodate both the header and the data region.

Use cases:
- Standard vector initialization with contiguous memory layout
- Memory pool implementations with pre-allocated contiguous blocks
- Cache-optimized data structures requiring contiguous memory
- Simple vector creation when memory layout is not a concern

Time complexity: O(1) - constant time initialization
Space complexity: O(1) - only local variables used

Prerequisites:
- vectorAddr must point to a valid, properly aligned memory address
- The memory region must be large enough to hold header + capacity*sizeof(T) bytes
- Memory must be properly aligned according to VectorRequiredAlignmentGet
- capacity is specified in elements, not bytes

Edge cases:
- capacity of 0 is valid and creates a vector with no data region
- The data region starts immediately after the header (offset equals header size)
- Version is initialized to 1

Additional notes:
- This function assumes contiguous memory layout (header immediately followed by data)
- For non-contiguous layouts, use VectorInitializeWithSeparatedHeaderAndData
- The function registers a type-specific movement function for efficient element copying
- Internally delegates to ArrayInitializeAt
*/
func VectorInitializeAt[T foundation.Numeric](vectorAddr memcore.MarkRaw, capacity uint64) {
	ArrayInitializeAt[T](vectorAddr, capacity)
}

/*
VectorInitializeWithSeparatedHeaderAndData initializes a vector instance with the header and data
stored at separate, non-contiguous memory addresses.

This function wraps ArrayInitializeWithSeparatedHeaderAndData for numeric types, allowing vectors
to be initialized with flexible memory layouts where the header and data region are allocated
in different memory locations.

Use cases:
- Memory pools where headers are stored in a separate metadata region
- Custom allocators that manage header and data allocations independently
- Interleaved data structures where multiple headers share a common data region
- Memory-constrained environments requiring precise control over memory layout

Time complexity: O(1) - constant time initialization
Space complexity: O(1) - only local variables used

Prerequisites:
- headerAddr must point to a valid, properly aligned memory address for the Vector header
- dataAddr must point to a valid, properly aligned memory address for the data region
- The data region must have sufficient capacity for the specified number of elements
- Both addresses must be within memory managed by memcore

Edge cases:
- dataAddr can be located before or after headerAddr in memory (offset can be negative or positive)
- The offset is calculated as the difference between header and data addresses
- This method allows non-contiguous memory layouts, unlike VectorInitializeAt which assumes contiguous layout

Additional notes:
- The dataAddrOffset field stores the byte offset from the header address to the data address
- This offset can be negative if data is located before the header in memory
- After initialization, all standard Vector operations work identically regardless of memory layout
- Internally delegates to ArrayInitializeWithSeparatedHeaderAndData
*/
func VectorInitializeWithSeparatedHeaderAndData[T foundation.Numeric](headerAddr memcore.MarkRaw, dataAddr memcore.MarkRaw, capacity uint64) {
	ArrayInitializeWithSeparatedHeaderAndData[T](headerAddr, dataAddr, capacity)
}

/*
VectorInitializeFrom initializes a new vector at vectorAddr with the contents of the source vector.

This function wraps ArrayInitializeFrom for numeric types, creating a new vector and copying all
elements from the source vector into it. The new vector must have capacity greater than or equal
to the source vector's capacity.

Use cases:
- Creating a vector from an existing vector with different capacity
- Vector duplication and cloning operations
- Resizing vectors while preserving data
- Vector migration to new memory locations

Time complexity: O(n) - where n is the source vector capacity (initialization + copy)
Space complexity: O(1) - only local variables used (destination memory must be pre-allocated)

Prerequisites:
- vectorAddr must point to a valid, properly aligned memory address
- src must point to a valid Vector instance of the same type T
- newCapacity must be >= source vector capacity
- Both vectors must live in memory managed by memcore

Edge cases:
- Returns error if newCapacity is less than source capacity
- The new vector will have the same element values as the source
- Version is initialized to 1 in the new vector

Additional notes:
- This function first initializes the destination vector, then copies all elements
- The source vector is not modified
- Internally delegates to ArrayInitializeFrom
*/
func VectorInitializeFrom[T foundation.Numeric](vectorAddr memcore.MarkRaw, src memcore.MarkRaw, newCapacity uint64) error {
	return ArrayInitializeFrom[T](vectorAddr, src, newCapacity)
}

/*
VectorSnapshotCreate creates a deep copy of a vector at a new memory location, preserving
the memory layout (contiguous or non-contiguous) of the source vector.

This function wraps ArraySnapshotCreate for numeric types, creating a complete snapshot of
the vector by copying both the header and data region. For contiguous vectors, it copies
the header and data as a single block. For non-contiguous vectors, it copies the header
and data separately to maintain the same memory layout in the destination.

Use cases:
- Creating checkpoint/restore points for state management
- Deep copying vectors for backup purposes
- Implementing undo/redo functionality
- State serialization and deserialization

Time complexity: O(n) - where n is the total size of header + data region
Space complexity: O(1) - only local variables used (destination memory must be pre-allocated)

Prerequisites:
- dest must point to a valid, properly aligned memory address
- The destination memory must be large enough to accommodate the vector (header + data)
- Both dest and instance must be valid Vector instances of the same type T
- Both vectors must live in memory managed by memcore

Edge cases:
- Handles both contiguous and non-contiguous memory layouts automatically
- Preserves the exact memory layout of the source vector
- Returns dest on success
- The destination vector will have the same capacity and memory layout as the source

Additional notes:
- This function automatically detects whether the source vector uses contiguous or non-contiguous layout
- For contiguous vectors, copies header + data as a single block for efficiency
- For non-contiguous vectors, copies header and data separately to preserve layout
- The version number is copied but not reset (snapshot preserves state)
- Internally delegates to ArraySnapshotCreate
*/
func VectorSnapshotCreate[T foundation.Numeric](dest memcore.MarkRaw, instance memcore.MarkRaw) memcore.MarkRaw {
	return ArraySnapshotCreate[T](dest, instance)
}

/*
VectorSnapshotRestore replaces the entire memory content of one vector with that of another
vector, preserving the memory layout of both source and destination.

This function wraps ArraySnapshotRestore for numeric types, restoring a vector from a previously
created snapshot. It copies both the header and data region from the source to the destination.
The function automatically handles both contiguous and non-contiguous memory layouts, ensuring
the destination layout matches the source layout.

Use cases:
- Restoring vector state from checkpoints
- Implementing undo/redo functionality
- State deserialization from snapshots
- Rollback operations in transactional systems

Time complexity: O(n) - where n is the total size of header + data region
Space complexity: O(1) - only local variables used

Prerequisites:
- dest and src must point to valid, properly aligned memory addresses
- Both vectors must have the same capacity
- Both vectors must be of the same type T
- Both vectors must live in memory managed by memcore
- The destination memory must be large enough to accommodate the source vector

Edge cases:
- Returns nil error if dest == src (no-op)
- Returns error if capacities do not match
- Handles both contiguous and non-contiguous memory layouts automatically
- Preserves the exact memory layout of the source vector in the destination

Additional notes:
- This function automatically detects whether vectors use contiguous or non-contiguous layout
- For contiguous vectors, copies header + data as a single block for efficiency
- For non-contiguous vectors, copies header and data separately to preserve layout
- The destination layout will match the source layout after restoration
- Internally delegates to ArraySnapshotRestore
*/
func VectorSnapshotRestore[T foundation.Numeric](dest, src memcore.MarkRaw) error {
	return ArraySnapshotRestore[T](dest, src)
}

/*
VectorHeaderClone copies only the Vector header structure from source to destination,
without copying any data elements.

This function wraps ArrayHeaderClone for numeric types, cloning the metadata (capacity,
dataAddrOffset, itemSize, version, setFnID) from one vector to another, but does not
copy the actual data elements. This is useful when you want to initialize a new vector
with the same metadata as an existing vector, but with different or uninitialized data.

Use cases:
- Initializing vectors with the same metadata configuration
- Resetting vector metadata while preserving data
- Copying vector configuration for template-based initialization
- Metadata synchronization between vectors

Time complexity: O(1) - only struct field copy operations
Space complexity: O(1) - only local variables used

Prerequisites:
- dest and src must point to valid Vector instances of the same type T
- Both vectors must live in memory managed by memcore
- The destination vector's data region must be valid (if dataAddrOffset is set)

Edge cases:
- Only copies header fields; data elements are not touched
- The destination vector's dataAddrOffset will match the source, which may point to invalid
  data if the destination's data region is not properly set up
- Version number is copied, which may not reflect the actual state of destination data

Additional notes:
- This function does not validate that the destination's data region is valid
- After cloning, the destination vector will have the same capacity and metadata as source
- The data region is not copied or validated; caller must ensure data region is valid
- Useful for initializing vectors with the same configuration but different data
- Internally delegates to ArrayHeaderClone
*/
func VectorHeaderClone[T foundation.Numeric](dest, src memcore.MarkRaw) {
	ArrayHeaderClone[T](dest, src)
}

/*
VectorCopyFrom copies the entire contents of the source vector into the destination vector,
starting at the specified destination index.

This function wraps ArrayCopyFrom for numeric types, copying all elements from the source
vector (from index 0 to capacity-1) into the destination vector starting at destStartIdx.
Both vectors must have the same element type, and the destination must have sufficient
capacity to accommodate the copy operation.

Use cases:
- Copying vector contents to a different location
- Merging vectors by copying into a larger destination
- Initializing vectors from existing vector data
- Vector migration and data movement operations

Time complexity: O(n) - where n is the source vector capacity (number of elements copied)
Space complexity: O(1) - only local variables used

Prerequisites:
- dest and src must point to valid Vector instances of the same type T
- Both vectors must live in memory managed by memcore
- destStartIdx + srcCapacity must not exceed destCapacity
- Both vectors must have the same element type T

Edge cases:
- Returns error if destination capacity is insufficient
- Copies all elements from source (entire capacity, not just used elements)
- Example: copy src[0:srcCap] → dest[destStartIdx : destStartIdx+srcCap]
- Increments destination vector version after successful copy

Additional notes:
- This function works with both contiguous and non-contiguous vector layouts
- The copy operation uses efficient memory move operations
- The destination vector's version is incremented to indicate modification
- Source vector version is not modified
- Internally delegates to ArrayCopyFrom
*/
func VectorCopyFrom[T foundation.Numeric](dest memcore.MarkRaw, src memcore.MarkRaw, destStartIdx uint64) error {
	return ArrayCopyFrom[T](dest, src, destStartIdx)
}

/*
VectorCopyFromRange copies a contiguous range of elements from the source vector into the
destination vector, starting at the specified destination index.

This function wraps ArrayCopyFromRange for numeric types, copying elements from the source
vector in the range [from, to) (from inclusive, to exclusive) into the destination vector
starting at destStartIdx. Both vectors must have the same element type, and bounds are
validated before copying.

Use cases:
- Copying a subset of vector elements to another location
- Extracting and moving specific ranges of data
- Partial vector merging operations
- Selective data migration between vectors

Time complexity: O(n) - where n is the number of elements in the range (to - from)
Space complexity: O(1) - only local variables used

Prerequisites:
- dest and src must point to valid Vector instances of the same type T
- Both vectors must live in memory managed by memcore
- from must be less than to (range must be valid)
- to must not exceed source vector capacity
- destStartIdx + (to - from) must not exceed destination vector capacity

Edge cases:
- Returns error if from >= to (invalid range)
- Returns error if to exceeds source capacity
- Returns error if destination capacity is insufficient
- The range [from, to) is half-open (from inclusive, to exclusive)
- Increments destination vector version after successful copy

Additional notes:
- This function works with both contiguous and non-contiguous vector layouts
- The copy operation uses efficient memory move operations
- The destination vector's version is incremented to indicate modification
- Source vector version is not modified
- Internally delegates to ArrayCopyFromRange
*/
func VectorCopyFromRange[T foundation.Numeric](
	dest memcore.MarkRaw,
	src memcore.MarkRaw,
	from, to, destStartIdx uint64,
) error {
	return ArrayCopyFromRange[T](dest, src, from, to, destStartIdx)
}

/*
VectorHeaderSizeBytesGet returns the number of bytes required to store the Vector header structure.

This function wraps ArrayHeaderSizeBytesGet for numeric types, calculating the size of the
Vector header only, excluding the data region. It is useful when allocating memory separately
for the header and data regions, or when calculating memory requirements for non-contiguous
memory layouts.

Use cases:
- Calculating memory requirements for separated header/data allocations
- Memory pool implementations that store headers separately
- Custom allocators that need precise header size information
- Memory layout planning and optimization

Time complexity: O(1) - compile-time constant evaluation
Space complexity: O(1) - no allocations

Prerequisites:
- Type T must be a numeric type (foundation.Numeric)

Edge cases:
- Returns the size of the Vector struct itself, which includes metadata fields
- Does not include padding or alignment considerations (use VectorHeaderAlignmentGet for alignment)
- Size is determined at compile time based on the Vector struct definition

Additional notes:
- The returned size is the exact size of the Vector header structure
- For total memory requirements including data, use VectorRequiredBytesGet
- This is an alias for VectorHeaderRequiredBytesGet (both functions return the same value)
- Internally delegates to ArrayHeaderSizeBytesGet
*/
func VectorHeaderSizeBytesGet[T foundation.Numeric]() uint64 {
	return ArrayHeaderSizeBytesGet[T]()
}

/*
VectorHeaderAlignmentGet returns the required memory alignment for the Vector header structure.

This function wraps ArrayHeaderAlignmentGet for numeric types. The alignment requirement ensures
that the Vector header is placed at a memory address that is a multiple of the returned value.
This is typically the alignment requirement of the element type T, which ensures optimal memory
access patterns and cache efficiency.

Use cases:
- Memory allocation alignment calculations
- Memory pool implementations requiring proper alignment
- Custom allocators that need alignment information
- Cache-optimized memory layout planning

Time complexity: O(1) - compile-time constant evaluation
Space complexity: O(1) - no allocations

Prerequisites:
- Type T must be a numeric type (foundation.Numeric)

Edge cases:
- Returns the alignment requirement of type T, which matches the header's alignment needs
- Alignment values are always powers of two
- Zero alignment is never returned (minimum alignment is 1)

Additional notes:
- The alignment is determined by the element type T, not the Vector struct itself
- This ensures that when the header is properly aligned, subsequent data access is also aligned
- For total alignment requirements including data, use VectorRequiredAlignmentGet
- This is an alias for VectorHeaderRequiredAlignmentGet (both functions return the same value)
- Internally delegates to ArrayHeaderAlignmentGet
*/
func VectorHeaderAlignmentGet[T foundation.Numeric]() uint64 {
	return ArrayHeaderAlignmentGet[T]()
}

// VectorViewGet produces a view over a vector that is potentially a subset and/or readonly.
// It panics if from or to are invalid.
//
//go:inline
func VectorViewGet[T foundation.Numeric](array memcore.MarkRaw, from, to uint64, readonly bool) VectorView[T] {
	return VectorView[T](ArrayViewGet[T](array, from, to, readonly))
}

// VectorViewGetUnsafe produces a view over a vector that is potentially a subset and/or readonly.
// It does no validation of bounds.
//
//go:inline
func VectorViewGetUnsafe[T foundation.Numeric](array memcore.MarkRaw, from, to uint64, readonly bool) VectorView[T] {
	return VectorView[T](ArrayViewGetUnsafe[T](array, from, to, readonly))
}

// VectorCapacityGet returns the total amount of elements that can be stored.
//
//go:nosplit
//go:inline
func VectorCapacityGet[T foundation.Numeric](vector memcore.MarkRaw) uint64 {
	return ArrayCapacityGet[T](vector)
}

// VectorVersionGet returns the current version of the vector.
//
// Version increments on every modification to the vector data, allowing cache invalidation
// mechanisms to detect when cached values become stale.
//
//go:inline
func VectorVersionGet[T foundation.Numeric](vector memcore.MarkRaw) uint64 {
	return ArrayVersionGet[T](vector)
}

// VectorItemGetAt returns T at idx within the vector.
// It returns an error if the idx is invalid.
//
//go:nosplit
//go:inline
func VectorItemGetAt[T foundation.Numeric](vector memcore.MarkRaw, idx uint64) (T, error) {
	return ArrayItemGetAt[T](vector, idx)
}

// VectorItemGetAtUnsafe returns T at idx within the vector.
// It does no bounds checks.
//
//go:inline
func VectorItemGetAtUnsafe[T foundation.Numeric](vector memcore.MarkRaw, idx uint64) T {
	return ArrayItemGetAtUnsafe[T](vector, idx)
}

// VectorItemPtrGetAt returns a pointer to T at idx within the vector.
// It returns an error if the idx is invalid.
//
// Using this pointer after deletion or overwriting this idx is undefined behaviour.
// Use at your own discretion!
//
//go:nosplit
//go:inline
func VectorItemPtrGetAt[T foundation.Numeric](vector memcore.MarkRaw, idx uint64) (*T, error) {
	return ArrayItemPtrGetAt[T](vector, idx)
}

// VectorItemPtrGetAtUnsafe returns a pointer to T at idx within the vector.
// It does no bounds checks.
//
// Using this pointer after deletion or overwriting this idx is undefined behaviour.
// Use at your own discretion!
//
//go:inline
func VectorItemPtrGetAtUnsafe[T foundation.Numeric](vector memcore.MarkRaw, idx uint64) *T {
	return ArrayItemPtrGetAtUnsafe[T](vector, idx)
}

// VectorDataPtrGet returns the current pointer to the underlying data storage in memory.
// This value CAN change if the underlying memory region changes.
// Not stable, so do not store.
//
//go:inline
func VectorDataPtrGet[T foundation.Numeric](array memcore.MarkRaw) unsafe.Pointer {
	return ArrayDataPtrGet[T](array)
}

// VectorByteOffsetGetAt returns the offset relative to the memory region for this idx.
// Panics if the idx is invalid.
//
//go:inline
func VectorByteOffsetGetAt[T foundation.Numeric](array memcore.MarkRaw, idx uint64) uintptr {
	return ArrayByteOffsetGetAt[T](array, idx)
}

// VectorByteOffsetGetAtUnsafe returns the offset relative to the memory region for this idx.
// Does no bounds checks.
//
//go:inline
func VectorByteOffsetGetAtUnsafe[T foundation.Numeric](array memcore.MarkRaw, idx uint64) uintptr {
	return ArrayByteOffsetGetAtUnsafe[T](array, idx)
}

// VectorSetAt sets idx of vector to value T.
// It returns an error if the idx is invalid.
//
//go:nosplit
//go:inline
func VectorSetAt[T foundation.Numeric](vector memcore.MarkRaw, idx uint64, value T) error {
	return ArraySetAt(vector, idx, value)
}

// VectorSetAtUnsafe sets idx of vector to value T.
// It does no bounds checks.
//
//go:nosplit
//go:inline
func VectorSetAtUnsafe[T foundation.Numeric](vector memcore.MarkRaw, idx uint64, value T) {
	ArraySetAtUnsafe(vector, idx, value)
}

// VectorSetAll sets all values within the vector to value T.
//
//go:inline
func VectorSetAll[T foundation.Numeric](vector memcore.MarkRaw, v T) {
	ArraySetAll(vector, v)
}

// VectorZeroAll sets all values within the Vector to its zero value.
// This is different from clearing the memory to 0.
//
//go:inline
func VectorZeroAll[T foundation.Numeric](vector memcore.MarkRaw) {
	ArrayZeroAll[T](vector)
}

// VectorForEachUnsafe calls a function for every element in the vector.
//
//go:inline
func VectorForEachUnsafe[T foundation.Numeric](vector memcore.MarkRaw, fn func(ptr unsafe.Pointer, idx uint64)) {
	ArrayForEachUnsafe[T](vector, fn)
}

// VectorStrideForEachUnsafe calls a function for every element in the vector.
// It visits every stride-th element.
//
// For the tail it calls the tailFn which is supposed to process one element at once.
//
//go:inline
func VectorStrideForEachUnsafe[T foundation.Numeric](
	vector memcore.MarkRaw,
	fn func(ptr unsafe.Pointer, idx uint64),
	tailFn func(ptr unsafe.Pointer, idx uint64),
	stride uint64,
) {
	ArrayStrideForEachUnsafe[T](vector, fn, tailFn, stride)
}

// VectorIterate allows you to iterate over the vector efficiently.
// It provides a method to say which next element you need.
// The next function returns true when your requested n exceeds the capacity.
//
//go:inline
func VectorIterate[T foundation.Numeric](
	vector memcore.MarkRaw,
	fn func(ptr unsafe.Pointer, idx uint64, next func(n uint64) (unsafe.Pointer, uint64, bool)),
) {
	ArrayIterate[T](vector, fn)
}

// VectorIterateUnsafe allows you to iterate over the vector efficiently.
// It provides a method to say which next element you need.
// The next function does no bounds checking. Safety must be guaranteed by the client.
//
//go:inline
func VectorIterateUnsafe[T any](
	vector memcore.MarkRaw,
	fn func(ptr unsafe.Pointer, idx uint64, next func(n uint64) (unsafe.Pointer, uint64)),
) {
	ArrayIterateUnsafe[T](vector, fn)
}

// VectorReplaceInternal replaces srcIdx with the value at destIdx efficiently.
//
//go:inline
func VectorReplaceInternal[T foundation.Numeric](vector memcore.MarkRaw, srcIdx, destIdx uint64) error {
	return ArrayReplaceInternal[T](vector, srcIdx, destIdx)
}

// VectorReplaceInternalUnsafe replaces srcIdx with the value at destIdx efficiently.
//
// It does no bounds checks.
//
//go:inline
func VectorReplaceInternalUnsafe[T foundation.Numeric](vector memcore.MarkRaw, srcIdx, destIdx uint64) {
	ArrayReplaceInternalUnsafe[T](vector, srcIdx, destIdx)
}

// VectorShiftRight shifts a contiguous range of elements in the vector
// `count` positions to the right, preserving their order.
//
// Parameters:
//   - from:  starting index of the range to shift (inclusive)
//   - to:    ending index of the range to shift (inclusive)
//   - count: number of positions to shift the range to the right
//
// The data between [from, to] is moved rightward by `count` slots, such that the
// element originally at `to` ends up at index `to + count`. Any elements in the
// destination range [from+count, to+count] will be overwritten.
//
// Example:
//
//	Before: [A, B, C, D, E, F, G]
//	Call:   VectorShiftRightUnsafe(arr, from=2, to=4, count=1)
//	Moves:  C→D, D→E, E→F
//	After:  [A, B, _, C, D, E, G]
//
//go:nosplit
//go:inline
func VectorShiftRight[T foundation.Numeric](vector memcore.MarkRaw, from, to, count uint64) error {
	return ArrayShiftRight[T](vector, from, to, count)
}

// VectorShiftRightUnsafe shifts a contiguous range of elements in the vector
// `count` positions to the right, preserving their order. It performs no bounds
// checks, so callers must ensure valid indices.
//
// Parameters:
//   - from:  starting index of the range to shift (inclusive)
//   - to:    ending index of the range to shift (inclusive)
//   - count: number of positions to shift the range to the right
//
// The data between [from, to] is moved rightward by `count` slots, such that the
// element originally at `to` ends up at index `to + count`. Any elements in the
// destination range [from+count, to+count] will be overwritten.
//
// Example:
//
//	Before: [A, B, C, D, E, F, G]
//	Call:   VectorShiftRightUnsafe(arr, from=2, to=4, count=1)
//	Moves:  C→D, D→E, E→F
//	After:  [A, B, _, C, D, E, G]
//
//go:nosplit
//go:inline
func VectorShiftRightUnsafe[T foundation.Numeric](vector memcore.MarkRaw, from, to, count uint64) {
	ArrayShiftRightUnsafe[T](vector, from, to, count)
}

// VectorShiftLeft shifts a contiguous range of elements in the vector
// `count` positions to the left, preserving their order.
//
// Parameters:
//   - from:  starting index of the range to shift (inclusive)
//   - to:    ending index of the range to shift (inclusive)
//   - count: number of positions to shift the range to the left
//
// The data between [from, to] is moved leftward by `count` slots, such that the
// element originally at `from` ends up at index `from - count`. Any elements in
// the destination range [from-count, to-count] will be overwritten.
//
// Example:
//
//	Before: [A, B, C, D, E, F, G]
//	Call:   VectorShiftLeft(arr, from=3, to=5, count=1)
//	Moves:  D→C, E→D, F→E
//	After:  [A, B, C, D, E, _, G]
//
// Performs bounds checks and returns an error if the range exceeds capacity.
//
//go:nosplit
//go:inline
func VectorShiftLeft[T foundation.Numeric](vector memcore.MarkRaw, from, to, count uint64) error {
	return VectorShiftLeft[T](vector, from, to, count)
}

// VectorShiftLeftUnsafe shifts a contiguous range of elements in the vector
// `count` positions to the left, preserving their order. It performs no bounds
// checks, so callers must ensure valid indices.
//
// Parameters:
//   - from:  starting index of the range to shift (inclusive)
//   - to:    ending index of the range to shift (inclusive)
//   - count: number of positions to shift the range to the left
//
// The data between [from, to] is moved leftward by `count` slots, such that the
// element originally at `from` ends up at index `from - count`. Any elements in
// the destination range [from-count, to-count] will be overwritten.
//
// Example:
//
//	Before: [A, B, C, D, E, F, G]
//	Call:   VectorShiftLeft(arr, from=3, to=5, count=1)
//	Moves:  D→C, E→D, F→E
//	After:  [A, B, C, D, E, _, G]
//
//go:nosplit
//go:inline
func VectorShiftLeftUnsafe[T foundation.Numeric](vector memcore.MarkRaw, from, to, count uint64) {
	ArrayShiftLeftUnsafe[T](vector, from, to, count)
}

// VectorRangeCopy is a convenience wrapper around shift lift/shift right.
// It automatically determines which to use based on from and to.
//
//go:inline
//go:nosplit
func VectorRangeCopy[T foundation.Numeric](vector memcore.MarkRaw, from, to, count uint64) error {
	return ArrayRangeCopy[T](vector, from, to, count)
}

// VectorRangeCopyUnsafe is a convenience wrapper around shift lift/shift right unsafe.
// It automatically determines which to use based on from and to.
//
//go:inline
//go:nosplit
func VectorRangeCopyUnsafe[T foundation.Numeric](vector memcore.MarkRaw, from, to, count uint64) {
	ArrayRangeCopyUnsafe[T](vector, from, to, count)
}

// VectorDeleteAt resets memory to 0 at a given index, using pointers to this
// index gotten earlier is undefined behaviour.
// It returns an error if the index is invalid.
//
//go:nosplit
//go:inline
func VectorDeleteAt[T foundation.Numeric](vector memcore.MarkRaw, idx uint64) error {
	return ArrayDeleteAt[T](vector, idx)
}

// VectorDeleteAtUnsafe resets memory to 0 at a given index, using pointers to this
// index gotten earlier is undefined behaviour.
// It does no bounds checks.
//
//go:nosplit
//go:inline
func VectorDeleteAtUnsafe[T foundation.Numeric](vector memcore.MarkRaw, idx uint64) {
	ArrayDeleteAtUnsafe[T](vector, idx)
}

// VectorClear resets the entire vector's memory to 0, allowing it to be reused.
// Do NOT use pointers to previous vector items after this as that is undefined behaviour.
//
//go:nosplit
//go:inline
func VectorClear[T foundation.Numeric](vector memcore.MarkRaw) {
	ArrayClear[T](vector)
}

// VectorIsIdxValid checks whether the given index is valid.
//
//go:inline
func VectorIsIdxValid[T foundation.Numeric](vector memcore.MarkRaw, idx uint64) bool {
	return ArrayIsIdxValid[T](vector, idx)
}

// VectorSort sorts the vector in-place.
//
// This implementation uses an iterative Quicksort with Median-of-Three pivot
// selection to ensure O(n log n) performance and zero stack-overflow risk.
//
// Internally delegates to ArraySort since Vector is a constrained wrapper around Array.
func VectorSort[T foundation.Numeric](vector memcore.MarkRaw, cmp func(a, b T) int) {
	ArraySort[T](vector, cmp)
}

// VectorSorted returns a sorted variant of this vector.
//
// The comparison function should return:
// a < b : -1 (or negative)
// a == b : 0
// a > b : 1 (or positive)
func VectorSorted[T foundation.Numeric](
	vector memcore.MarkRaw,
	targetVectorAddr memcore.MarkRaw,
	cmp func(a, b T) int,
) {
	srcBase, srcInst := memcore.MemcoreMarkDereferenceObjectAltUnsafe[Vector[T]](vector)
	dstBase, dstInst := memcore.MemcoreMarkDereferenceObjectAltUnsafe[Vector[T]](targetVectorAddr)

	if srcInst.capacity != dstInst.capacity {
		panic("VectorSorted: capacity mismatch")
	}

	srcData := unsafe.Add(srcBase, srcInst.dataAddrOffset)
	dstData := unsafe.Add(dstBase, dstInst.dataAddrOffset)
	totalBytes := uintptr(srcInst.capacity) * uintptr(srcInst.itemSize)

	memcore.MemoryMoveNoHeapPointers(dstData, srcData, totalBytes)

	VectorSort[T](targetVectorAddr, cmp)
}

// ---------------------------------------------------- VECTOR VIEW

// VectorViewLengthGet returns the length of the current view.
//
//go:inline
func VectorViewLengthGet[T foundation.Numeric](vectorView VectorView[T]) uint64 {
	return ArrayViewLengthGet((ArrayView[T])(vectorView))
}

// VectorViewIsReadonly returns whether the current view is readonly.
//
//go:inline
func VectorViewIsReadonly[T foundation.Numeric](vectorView VectorView[T]) bool {
	return ArrayViewIsReadonly((ArrayView[T])(vectorView))
}

// VectorViewItemGetAt returns item at idx (as computed by view startIdx+relativeIdx)
//
//go:inline
func VectorViewItemGetAt[T foundation.Numeric](vectorView VectorView[T], relativeIdx uint64) (T, error) {
	return ArrayViewItemGetAt((ArrayView[T])(vectorView), relativeIdx)
}

// VectorViewItemPtrGetAt returns item pointer at idx (as computed by view startIdx+relativeIdx)
// Fails if the view is readonly (because getting a pointer would allow mutation)
//
//go:inline
func VectorViewItemPtrGetAt[T foundation.Numeric](vectorView VectorView[T], relativeIdx uint64) (*T, error) {
	return ArrayViewItemPtrGetAt((ArrayView[T])(vectorView), relativeIdx)
}

// VectorViewItemSetAt sets the item at idx (as computed by view startIdx+relativeIdx)
// Fails if the view is readonly.
//
//go:inline
func VectorViewItemSetAt[T foundation.Numeric](vectorView VectorView[T], relativeIdx uint64, v T) error {
	return ArrayViewItemSetAt((ArrayView[T])(vectorView), relativeIdx, v)
}

// VectorViewForEach calls a function for every element in the vector view.
// The indexes returned are the relative indexes.
//
//go:inline
func VectorViewForEach[T foundation.Numeric](vectorView VectorView[T], fn func(item T, idx uint64)) {
	ArrayViewForEach((ArrayView[T])(vectorView), fn)
}

// VectorViewForEachRaw calls a function for every element in the vector view.
// Not possible for readonly views.
// The indexes returned are the relative indexes.
//
//go:inline
func VectorViewForEachRaw[T foundation.Numeric](vectorView VectorView[T], fn func(ptr unsafe.Pointer, idx uint64)) error {
	return ArrayViewForEachRaw((ArrayView[T])(vectorView), fn)
}

// VectorViewStrideForEach calls a function for every element in the vector view.
// It visits every stride-th element.
// The indexes returned are the relative indexes.
//
//go:inline
func VectorViewStrideForEach[T foundation.Numeric](vectorView VectorView[T], fn func(item T, idx uint64), stride uint64) {
	ArrayViewStrideForEach((ArrayView[T])(vectorView), fn, stride)
}

// VectorViewStrideForEachRaw calls a function for every element in the vector view.
// It visits every stride-th element.
// Not possible for readonly views.
// The indexes returned are the relative indexes.
//
//go:inline
func VectorViewStrideForEachRaw[T foundation.Numeric](vectorView VectorView[T], fn func(ptr unsafe.Pointer, idx uint64), stride uint64) error {
	return ArrayViewStrideForEachRaw((ArrayView[T])(vectorView), fn, stride)
}

// VectorViewIterate allows you to iterate over the vector view efficiently.
// It provides a method to say which next element you need.
// The next function returns true when your requested n exceeds the view length.
// The indexes returned are the relative indexes.
//
//go:inline
func VectorViewIterate[T foundation.Numeric](
	vectorView VectorView[T],
	fn func(item T, idx uint64, next func(n uint64) (T, uint64, bool)),
) {
	ArrayViewIterate((ArrayView[T])(vectorView), fn)
}

// VectorViewIterateRaw allows you to iterate over the array view efficiently.
// It provides a method to say which next element you need.
// The next function returns true when your requested n exceeds the view length.
// Not possible for readonly views.
// The indexes returned are the relative indexes.
//
//go:inline
func VectorViewIterateRaw[T foundation.Numeric](
	vectorView VectorView[T],
	fn func(ptr unsafe.Pointer, idx uint64, next func(n uint64) (unsafe.Pointer, uint64, bool)),
) error {
	return ArrayViewIterateRaw((ArrayView[T])(vectorView), fn)
}

// VectorViewNestedGet creates a nested view inside an existing view.
// The indices [from:to) are relative to the current view.
// The readonly status is preserved.
//
//go:inline
func VectorViewNestedGet[T foundation.Numeric](vectorView VectorView[T], from, to uint64) VectorView[T] {
	return VectorView[T](ArrayViewNestedGet(ArrayView[T](vectorView), from, to))
}

// VectorUnaryReadOnlyOp is an operation that executes over a single element and does not mutate.
type VectorUnaryReadOnlyOp[T foundation.Numeric] func(item T)

// VectorUnaryOp is an operation that executes over a single element and mutates.
type VectorUnaryOp[TInput, TOutput foundation.Numeric] func(item TInput) TOutput

// VectorBinaryReadOnlyOp is an operation that executes over two elements at the same idx from different sources.
// It does not mutate.
type VectorBinaryReadOnlyOp[TInput1, TInput2 foundation.Numeric] func(itemA TInput1, itemB TInput2)

// VectorBinaryOp is an operation that executes over two elements at the same idx from different sources.
// It mutates.
type VectorBinaryOp[TInput1, TInput2, TOutput foundation.Numeric] func(itemA TInput1, itemB TInput2) TOutput

// VectorUnaryReadOnlyExecute executes a stride of unary readonly operations.
//
//go:inline
func VectorUnaryReadOnlyExecute[T foundation.Numeric](
	vectorAddr memcore.MarkRaw,
	op VectorUnaryReadOnlyOp[T],
	stride uint64,
) {
	base, inst := memcore.MemcoreMarkDereferenceObjectAlt[Vector[T]](vectorAddr)
	data := unsafe.Add(base, inst.dataAddrOffset)
	size := uintptr(inst.itemSize)
	cap := inst.capacity

	vectorUnrolledDispatch[T](cap, stride, func(idx uintptr) {
		v := *(*T)(unsafe.Add(data, idx*size))
		op(v)
	})
}

// VectorUnaryExecute executes a stride of unary mutating operations,
// writing results from the source vector into the destination vector.
//
//go:inline
func VectorUnaryExecute[T, P foundation.Numeric](
	srcAddr, dstAddr memcore.MarkRaw,
	op VectorUnaryOp[T, P],
	stride uint64,
) {
	srcBase, srcInst := memcore.MemcoreMarkDereferenceObjectAlt[Vector[T]](srcAddr)
	dstBase, dstInst := memcore.MemcoreMarkDereferenceObjectAlt[Vector[P]](dstAddr)

	if srcInst.capacity != dstInst.capacity {
		panic(fmt.Errorf("cannot perform unary op: capacity mismatch (src=%d, dst=%d)",
			srcInst.capacity, dstInst.capacity))
	}

	srcData := unsafe.Add(srcBase, srcInst.dataAddrOffset)
	dstData := unsafe.Add(dstBase, dstInst.dataAddrOffset)
	srcSize := uintptr(srcInst.itemSize)
	dstSize := uintptr(dstInst.itemSize)
	capacity := srcInst.capacity

	vectorUnrolledDispatch[T](capacity, stride, func(idx uintptr) {
		srcV := *(*T)(unsafe.Add(srcData, idx*srcSize))
		res := op(srcV)
		*(*P)(unsafe.Add(dstData, idx*dstSize)) = res
	})
}

// VectorBinaryReadOnlyExecute executes a stride of binary read-only operations
// between two vectors of equal capacity.
//
//go:inline
func VectorBinaryReadOnlyExecute[T, U foundation.Numeric](
	vectorAAddr, vectorBAddr memcore.MarkRaw,
	op VectorBinaryReadOnlyOp[T, U],
	stride uint64,
) {
	aBase, aInst := memcore.MemcoreMarkDereferenceObjectAlt[Vector[T]](vectorAAddr)
	bBase, bInst := memcore.MemcoreMarkDereferenceObjectAlt[Vector[U]](vectorBAddr)

	if aInst.capacity != bInst.capacity {
		panic(fmt.Errorf("cannot perform binary op: capacity mismatch (a=%d, b=%d)",
			aInst.capacity, bInst.capacity))
	}

	aData := unsafe.Add(aBase, aInst.dataAddrOffset)
	bData := unsafe.Add(bBase, bInst.dataAddrOffset)
	aSize := uintptr(aInst.itemSize)
	bSize := uintptr(bInst.itemSize)
	capacity := aInst.capacity

	vectorUnrolledDispatch[T](capacity, stride, func(idx uintptr) {
		aVal := *(*T)(unsafe.Add(aData, idx*aSize))
		bVal := *(*U)(unsafe.Add(bData, idx*bSize))
		op(aVal, bVal)
	})
}

// VectorBinaryExecute executes a stride of binary mutating operations
// between two source vectors and writes results into a destination vector.
//
//go:inline
func VectorBinaryExecute[T, U, P foundation.Numeric](
	vectorAAddr, vectorBAddr, destAddr memcore.MarkRaw,
	op VectorBinaryOp[T, U, P],
	stride uint64,
) {
	aBase, aInst := memcore.MemcoreMarkDereferenceObjectAlt[Vector[T]](vectorAAddr)
	bBase, bInst := memcore.MemcoreMarkDereferenceObjectAlt[Vector[U]](vectorBAddr)
	dBase, dInst := memcore.MemcoreMarkDereferenceObjectAlt[Vector[P]](destAddr)

	if aInst.capacity != bInst.capacity || aInst.capacity != dInst.capacity {
		panic(fmt.Errorf("cannot perform binary op: capacity mismatch (a=%d, b=%d, d=%d)",
			aInst.capacity, bInst.capacity, dInst.capacity))
	}

	aData := unsafe.Add(aBase, aInst.dataAddrOffset)
	bData := unsafe.Add(bBase, bInst.dataAddrOffset)
	dData := unsafe.Add(dBase, dInst.dataAddrOffset)

	aSize := uintptr(aInst.itemSize)
	bSize := uintptr(bInst.itemSize)
	dSize := uintptr(dInst.itemSize)
	capacity := aInst.capacity

	vectorUnrolledDispatch[T](capacity, stride, func(idx uintptr) {
		aVal := *(*T)(unsafe.Add(aData, idx*aSize))
		bVal := *(*U)(unsafe.Add(bData, idx*bSize))
		*(*P)(unsafe.Add(dData, idx*dSize)) = op(aVal, bVal)
	})
}

//go:inline
func vectorUnrolledDispatch[T foundation.Numeric](
	capacity uint64,
	stride uint64,
	apply func(idx uintptr),
) {
	switch stride {
	case 8:
		vectorUnrolledStride8[T](capacity, apply)
	case 4:
		vectorUnrolledStride4[T](capacity, apply)
	case 2:
		vectorUnrolledStride2[T](capacity, apply)
	case 1:
		vectorUnrolledStride1[T](capacity, apply)
	default:
		vectorUnrolledGeneric[T](capacity, stride, apply)
	}
}

//go:inline
func vectorUnrolledStride8[T foundation.Numeric](
	capacity uint64,
	apply func(idx uintptr),
) {
	var i uint64
	for ; i+7 < capacity; i += 8 {
		apply(uintptr(i + 0))
		apply(uintptr(i + 1))
		apply(uintptr(i + 2))
		apply(uintptr(i + 3))
		apply(uintptr(i + 4))
		apply(uintptr(i + 5))
		apply(uintptr(i + 6))
		apply(uintptr(i + 7))
	}

	for ; i < capacity; i++ {
		apply(uintptr(i))
	}
}

//go:inline
func vectorUnrolledStride4[T foundation.Numeric](capacity uint64, apply func(idx uintptr)) {
	var i uint64
	for ; i+3 < capacity; i += 4 {
		apply(uintptr(i + 0))
		apply(uintptr(i + 1))
		apply(uintptr(i + 2))
		apply(uintptr(i + 3))
	}
	for ; i < capacity; i++ {
		apply(uintptr(i))
	}
}

//go:inline
func vectorUnrolledStride2[T foundation.Numeric](capacity uint64, apply func(idx uintptr)) {
	var i uint64
	for ; i+1 < capacity; i += 2 {
		apply(uintptr(i + 0))
		apply(uintptr(i + 1))
	}
	for ; i < capacity; i++ {
		apply(uintptr(i))
	}
}

//go:inline
func vectorUnrolledStride1[T foundation.Numeric](capacity uint64, apply func(idx uintptr)) {
	for i := uint64(0); i < capacity; i++ {
		apply(uintptr(i))
	}
}

//go:inline
func vectorUnrolledGeneric[T foundation.Numeric](capacity uint64, stride uint64, apply func(idx uintptr)) {
	var i uint64
	for ; i+stride <= capacity; i += stride {
		for j := uint64(0); j < stride; j++ {
			apply(uintptr(i + j))
		}
	}
	for ; i < capacity; i++ {
		apply(uintptr(i))
	}
}
