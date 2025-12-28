package memstruct

import (
	"fmt"
	"memcore"
	"unsafe"
)

/*
ArrayHeaderRequiredBytesGet returns the number of bytes required to store the Array header structure.

This function calculates the size of the Array header only, excluding the data region.
It is useful when allocating memory separately for the header and data regions, or when
calculating memory requirements for non-contiguous memory layouts.

Use cases:
- Calculating memory requirements for separated header/data allocations
- Memory pool implementations that store headers separately
- Custom allocators that need precise header size information
- Memory layout planning and optimization

Time complexity: O(1) - compile-time constant evaluation
Space complexity: O(1) - no allocations

Prerequisites:
- Type T must be a valid Go type

Edge cases:
- Returns the size of the Array struct itself, which includes metadata fields
- Does not include padding or alignment considerations (use ArrayHeaderRequiredAlignmentGet for alignment)
- Size is determined at compile time based on the Array struct definition

Additional notes:
- The returned size is the exact size of the Array header structure
- For total memory requirements including data, use ArrayRequiredBytesGet
*/
func ArrayHeaderRequiredBytesGet[T any]() uint64 {
	headerSize := memcore.SizeOf[Array[T]]()
	return headerSize
}

/*
ArrayRequiredBytesGet returns the total number of bytes required for an array with the specified capacity,
assuming a contiguous memory layout where the header is immediately followed by the data region.

This function calculates the combined size of the Array header and the data region for a contiguous
memory layout. For non-contiguous layouts (where header and data are separated), calculate header
and data sizes separately using ArrayHeaderRequiredBytesGet and item size calculations.

Use cases:
- Calculating memory requirements for contiguous array allocations
- Memory pool sizing and capacity planning
- Allocator implementations that need total size information
- Memory layout optimization for cache efficiency

Time complexity: O(1) - simple arithmetic operations
Space complexity: O(1) - only local variables used

Prerequisites:
- Type T must be a valid Go type
- capacity must be a valid non-negative integer

Edge cases:
- Returns headerSize + itemSize*capacity for contiguous layouts
- Does not account for alignment padding between header and data (handled by alignment requirements)
- For non-contiguous layouts, this calculation is incorrect; use ArrayHeaderRequiredBytesGet separately

Additional notes:
- This function assumes contiguous memory layout (header immediately followed by data)
- For separated header/data layouts, calculate sizes separately
- The actual allocated size may need to account for alignment requirements (use ArrayRequiredAlignmentGet)
*/
func ArrayRequiredBytesGet[T any](capacity uint64) uint64 {
	headerSize := memcore.SizeOf[Array[T]]()
	itemSize := memcore.SizeOf[T]()
	return headerSize + itemSize*capacity
}

/*
ArrayHeaderRequiredAlignmentGet returns the required memory alignment for the Array header structure.

The alignment requirement ensures that the Array header is placed at a memory address that is
a multiple of the returned value. This is typically the alignment requirement of the element
type T, which ensures optimal memory access patterns and cache efficiency.

Use cases:
- Memory allocation alignment calculations
- Memory pool implementations requiring proper alignment
- Custom allocators that need alignment information
- Cache-optimized memory layout planning

Time complexity: O(1) - compile-time constant evaluation
Space complexity: O(1) - no allocations

Prerequisites:
- Type T must be a valid Go type

Edge cases:
- Returns the alignment requirement of type T, which matches the header's alignment needs
- Alignment values are always powers of two
- Zero alignment is never returned (minimum alignment is 1)

Additional notes:
- The alignment is determined by the element type T, not the Array struct itself
- This ensures that when the header is properly aligned, subsequent data access is also aligned
- For total alignment requirements including data, use ArrayRequiredAlignmentGet
*/
func ArrayHeaderRequiredAlignmentGet[T any]() uint64 {
	return memcore.AlignOf[T]()
}

/*
ArrayRequiredAlignmentGet returns the required memory alignment for an array allocation,
accounting for both the header and data region alignment requirements.

The returned alignment is the maximum of the element type alignment and the Array header
alignment, ensuring that both the header and all data elements are properly aligned for
optimal memory access patterns and cache efficiency.

Use cases:
- Memory allocation alignment calculations for contiguous arrays
- Allocator implementations requiring proper alignment
- Cache-optimized memory layout planning
- SIMD operations requiring specific alignment

Time complexity: O(1) - simple comparison operation
Space complexity: O(1) - only local variables used

Prerequisites:
- Type T must be a valid Go type

Edge cases:
- Returns the maximum of element type alignment and Array header alignment
- Alignment values are always powers of two
- Minimum returned alignment is 1 (never zero)

Additional notes:
- This function assumes contiguous memory layout
- The alignment ensures both header and data elements are properly aligned
- For separated header/data layouts, use ArrayHeaderRequiredAlignmentGet for header alignment
*/
func ArrayRequiredAlignmentGet[T any]() uint64 {
	return max(memcore.AlignOf[T](), memcore.AlignOf[Array[T]]())
}

/*
ArrayDataRequiredBytesGet returns the number of bytes required to store the data region
of an array with the specified capacity, excluding the header structure.

This function calculates the size of the data region only (capacity * sizeof(T)), which
is useful when allocating memory separately for the header and data regions, or when
calculating memory requirements for non-contiguous memory layouts.

Use cases:
- Calculating memory requirements for separated header/data allocations
- Memory pool implementations that store headers separately
- Custom allocators that need precise data region size information
- Memory layout planning and optimization for data-only regions

Time complexity: O(1) - simple arithmetic operation
Space complexity: O(1) - no allocations

Prerequisites:
- Type T must be a valid Go type
- capacity must be a valid non-negative integer

Edge cases:
- Returns 0 if capacity is 0 (no data region needed)
- Does not include header size (use ArrayHeaderRequiredBytesGet for header size)
- Does not account for alignment padding (use ArrayDataRequiredAlignmentGet for alignment)

Additional notes:
- The returned size is the exact size needed for capacity elements of type T
- For total memory requirements including header, use ArrayRequiredBytesGet
- This function is useful when header and data are allocated separately
*/
func ArrayDataRequiredBytesGet[T any](capacity uint64) uint64 {
	itemSize := memcore.SizeOf[T]()
	return itemSize * capacity
}

/*
ArrayDataRequiredAlignmentGet returns the required memory alignment for the Array data region.

The alignment requirement ensures that the data region is placed at a memory address that is
a multiple of the returned value. This is the alignment requirement of the element type T,
ensuring optimal memory access patterns and cache efficiency for data elements.

Use cases:
- Memory allocation alignment calculations for data-only regions
- Memory pool implementations requiring proper data alignment
- Custom allocators that need data region alignment information
- SIMD operations requiring specific data alignment
- Cache-optimized memory layout planning for data regions

Time complexity: O(1) - compile-time constant evaluation
Space complexity: O(1) - no allocations

Prerequisites:
- Type T must be a valid Go type

Edge cases:
- Returns the alignment requirement of type T (the element type)
- Alignment values are always powers of two
- Zero alignment is never returned (minimum alignment is 1)

Additional notes:
- The alignment is determined by the element type T
- This ensures that all data elements are properly aligned for efficient access
- For total alignment requirements including header, use ArrayRequiredAlignmentGet
- This function is useful when header and data are allocated separately
*/
func ArrayDataRequiredAlignmentGet[T any]() uint64 {
	return memcore.AlignOf[T]()
}

// Array is a custom array implementation built on top of memcore.
type Array[T any] struct {
	dataAddrOffset uintptr
	capacity       uint64

	setFnID memcore.FunctionID

	itemSize uintptr
	version  uint64
}

// ArrayView represents a view around an array.
// It can be readonly and/or a subset within the array.
type ArrayView[T any] struct {
	arrayHeader      memcore.MarkRaw
	startIdx, endIdx uint64
	readonly         bool
}

/*
ArrayInitializeAt initializes an array instance for type T at a specific memory address,
assuming a contiguous memory layout where the header is immediately followed by the data region.

This function sets up the array header and calculates the data address offset based on the
assumption that data immediately follows the header in memory. The memory at arrayAddr must
be large enough to accommodate both the header and the data region.

Use cases:
- Standard array initialization with contiguous memory layout
- Memory pool implementations with pre-allocated contiguous blocks
- Cache-optimized data structures requiring contiguous memory
- Simple array creation when memory layout is not a concern

Time complexity: O(1) - constant time initialization
Space complexity: O(1) - only local variables used

Prerequisites:
- arrayAddr must point to a valid, properly aligned memory address
- The memory region must be large enough to hold header + capacity*sizeof(T) bytes
- Memory must be properly aligned according to ArrayRequiredAlignmentGet
- capacity is specified in elements, not bytes

Edge cases:
- capacity of 0 is valid and creates an array with no data region
- The data region starts immediately after the header (offset equals header size)
- Version is initialized to 1

Additional notes:
- This function assumes contiguous memory layout (header immediately followed by data)
- For non-contiguous layouts, use ArrayInitializeWithSeparatedHeaderAndData
- The function registers a type-specific movement function for efficient element copying
*/
func ArrayInitializeAt[T any](arrayAddr memcore.MarkRaw, capacity uint64) {
	headerSize := memcore.SizeOf[Array[T]]()

	itemSize := memcore.SizeOf[T]()
	arrayPtr := memcore.MemcoreMarkDereferenceObject[Array[T]](arrayAddr)
	*arrayPtr = Array[T]{
		dataAddrOffset: uintptr(headerSize),
		capacity:       capacity,
		itemSize:       uintptr(itemSize),
		version:        1,
	}

	arrayPtr.setFnID = memcore.MemcoreFunctionRegisterTyped(
		getMovementFunc[T](itemSize),
	)
}

/*
ArrayInitializeWithSeparatedHeaderAndData initializes an array instance with the header and data
stored at separate, non-contiguous memory addresses.

This function is used when the array header and data region are allocated in different memory
locations, allowing for flexible memory layouts such as memory pools, custom allocators, or
interleaved data structures where headers and data are stored separately.

Use cases:
- Memory pools where headers are stored in a separate metadata region
- Custom allocators that manage header and data allocations independently
- Interleaved data structures where multiple headers share a common data region
- Memory-constrained environments requiring precise control over memory layout

Time complexity: O(1) - constant time initialization
Space complexity: O(1) - only local variables used

Prerequisites:
- headerAddr must point to a valid, properly aligned memory address for the Array header
- dataAddr must point to a valid, properly aligned memory address for the data region
- The data region must have sufficient capacity for the specified number of elements
- Both addresses must be within memory managed by memcore

Edge cases:
- dataAddr can be located before or after headerAddr in memory (offset can be negative or positive)
- The offset is calculated as the difference between header and data addresses
- This method allows non-contiguous memory layouts, unlike ArrayInitializeAt which assumes contiguous layout

Additional notes:
- The dataAddrOffset field stores the byte offset from the header address to the data address
- This offset can be negative if data is located before the header in memory
- After initialization, all standard Array operations work identically regardless of memory layout
*/
func ArrayInitializeWithSeparatedHeaderAndData[T any](headerAddr memcore.MarkRaw, dataAddr memcore.MarkRaw, capacity uint64) {
	itemSize := memcore.SizeOf[T]()
	arrayPtr := memcore.MemcoreMarkDereferenceObject[Array[T]](headerAddr)

	headerPtr := uintptr(unsafe.Pointer(arrayPtr))
	dataPtr := uintptr(memcore.MemcoreMarkDereference(dataAddr))

	// Calculate offset: dataPtr - headerPtr so that headerPtr + offset = dataPtr
	// Previous calculation was reversed (headerPtr - dataPtr), which was incorrect
	dataAddrOffset := dataPtr - headerPtr

	*arrayPtr = Array[T]{
		dataAddrOffset: dataAddrOffset,
		capacity:       capacity,
		itemSize:       uintptr(itemSize),
		version:        1,
	}

	arrayPtr.setFnID = memcore.MemcoreFunctionRegisterTyped(
		getMovementFunc[T](itemSize),
	)
}

// ArrayInitializeFrom initializes a new array at arrayAddr with the contents of src.
// Capacity must be >= src capacity.
func ArrayInitializeFrom[T any](arrayAddr memcore.MarkRaw, src memcore.MarkRaw, newCapacity uint64) error {
	ArrayInitializeAt[T](arrayAddr, newCapacity)
	return ArrayCopyFrom[T](arrayAddr, src, 0)
}

/*
ArraySnapshotCreate creates a deep copy of an array at a new memory location, preserving
the memory layout (contiguous or non-contiguous) of the source array.

This function creates a complete snapshot of the array, copying both the header and data
region. For contiguous arrays, it copies the header and data as a single block. For
non-contiguous arrays, it copies the header and data separately to maintain the same
memory layout in the destination.

Use cases:
- Creating checkpoint/restore points for state management
- Deep copying arrays for backup purposes
- Implementing undo/redo functionality
- State serialization and deserialization

Time complexity: O(n) - where n is the total size of header + data region
Space complexity: O(1) - only local variables used (destination memory must be pre-allocated)

Prerequisites:
- dest must point to a valid, properly aligned memory address
- The destination memory must be large enough to accommodate the array (header + data)
- Both dest and instance must be valid Array instances of the same type T
- Both arrays must live in memory managed by memcore

Edge cases:
- Handles both contiguous and non-contiguous memory layouts automatically
- Preserves the exact memory layout of the source array
- Returns dest on success
- The destination array will have the same capacity and memory layout as the source

Additional notes:
- This function automatically detects whether the source array uses contiguous or non-contiguous layout
- For contiguous arrays, copies header + data as a single block for efficiency
- For non-contiguous arrays, copies header and data separately to preserve layout
- The version number is copied but not reset (snapshot preserves state)
*/
func ArraySnapshotCreate[T any](dest memcore.MarkRaw, instance memcore.MarkRaw) memcore.MarkRaw {
	arrayPtr := memcore.MemcoreMarkDereferenceObjectUnsafe[Array[T]](instance)

	if arrayIsContiguous(arrayPtr) {
		// Contiguous layout: copy header + data as single block
		totalSize := ArrayRequiredBytesGet[T](arrayPtr.capacity)
		srcAddr := memcore.MemcoreMarkDereference(instance)
		dstAddr := memcore.MemcoreMarkDereference(dest)
		memcore.MemoryMoveNoHeapPointers(dstAddr, srcAddr, uintptr(totalSize))
	} else {
		// Non-contiguous layout: copy header and data separately
		headerSize := memcore.SizeOf[Array[T]]()
		dataSize := uintptr(arrayPtr.capacity) * arrayPtr.itemSize

		// Copy header
		srcHeaderAddr := memcore.MemcoreMarkDereference(instance)
		dstHeaderAddr := memcore.MemcoreMarkDereference(dest)
		memcore.MemoryMoveNoHeapPointers(dstHeaderAddr, srcHeaderAddr, uintptr(headerSize))

		// Copy data (preserve the same offset in destination)
		srcBaseAddr := memcore.MemcoreMarkDereference(instance)
		dstBaseAddr := memcore.MemcoreMarkDereference(dest)
		srcDataAddr := arrayComputeDataAddr(arrayPtr, srcBaseAddr)
		dstDataAddr := unsafe.Add(dstBaseAddr, arrayPtr.dataAddrOffset)
		memcore.MemoryMoveNoHeapPointers(dstDataAddr, srcDataAddr, dataSize)
	}

	return dest
}

/*
ArraySnapshotRestore replaces the entire memory content of one array with that of another
array, preserving the memory layout of both source and destination.

This function restores an array from a previously created snapshot. It copies both the
header and data region from the source to the destination. The function automatically
handles both contiguous and non-contiguous memory layouts, ensuring the destination
layout matches the source layout.

Use cases:
- Restoring array state from checkpoints
- Implementing undo/redo functionality
- State deserialization from snapshots
- Rollback operations in transactional systems

Time complexity: O(n) - where n is the total size of header + data region
Space complexity: O(1) - only local variables used

Prerequisites:
- dest and src must point to valid, properly aligned memory addresses
- Both arrays must have the same capacity
- Both arrays must be of the same type T
- Both arrays must live in memory managed by memcore
- The destination memory must be large enough to accommodate the source array

Edge cases:
- Returns nil error if dest == src (no-op)
- Returns error if capacities do not match
- Handles both contiguous and non-contiguous memory layouts automatically
- Preserves the exact memory layout of the source array in the destination

Additional notes:
- This function automatically detects whether arrays use contiguous or non-contiguous layout
- For contiguous arrays, copies header + data as a single block for efficiency
- For non-contiguous arrays, copies header and data separately to preserve layout
- The destination layout will match the source layout after restoration
*/
func ArraySnapshotRestore[T any](dest, src memcore.MarkRaw) error {
	dstHeader := memcore.MemcoreMarkDereferenceObjectUnsafe[Array[T]](dest)
	srcHeader := memcore.MemcoreMarkDereferenceObjectUnsafe[Array[T]](src)

	if dstHeader.capacity != srcHeader.capacity {
		return fmt.Errorf("cannot restore snapshot: unequal capacities (dest=%v, src=%v)", dstHeader.capacity, srcHeader.capacity)
	}

	if dest == src {
		return nil
	}

	if arrayIsContiguous(srcHeader) {
		// Contiguous layout: copy header + data as single block
		totalBytes := ArrayRequiredBytesGet[T](dstHeader.capacity)
		dstAddr := memcore.MemcoreMarkDereferenceUnsafe(dest)
		srcAddr := memcore.MemcoreMarkDereferenceUnsafe(src)
		memcore.MemoryMoveNoHeapPointers(dstAddr, srcAddr, uintptr(totalBytes))
	} else {
		// Non-contiguous layout: copy header and data separately
		headerSize := memcore.SizeOf[Array[T]]()
		dataSize := uintptr(srcHeader.capacity) * srcHeader.itemSize

		// Copy header
		dstHeaderAddr := memcore.MemcoreMarkDereferenceUnsafe(dest)
		srcHeaderAddr := memcore.MemcoreMarkDereferenceUnsafe(src)
		memcore.MemoryMoveNoHeapPointers(dstHeaderAddr, srcHeaderAddr, uintptr(headerSize))

		// Copy data (preserve the same offset in destination as source)
		dstBaseAddr := memcore.MemcoreMarkDereferenceUnsafe(dest)
		srcBaseAddr := memcore.MemcoreMarkDereferenceUnsafe(src)
		srcDataAddr := arrayComputeDataAddr(srcHeader, srcBaseAddr)
		dstDataAddr := unsafe.Add(dstBaseAddr, srcHeader.dataAddrOffset)
		memcore.MemoryMoveNoHeapPointers(dstDataAddr, srcDataAddr, dataSize)
	}

	return nil
}

/*
ArrayHeaderClone copies only the Array header structure from source to destination,
without copying any data elements.

This function clones the metadata (capacity, dataAddrOffset, itemSize, version, setFnID)
from one array to another, but does not copy the actual data elements. This is useful
when you want to initialize a new array with the same metadata as an existing array,
but with different or uninitialized data.

Use cases:
- Initializing arrays with the same metadata configuration
- Resetting array metadata while preserving data
- Copying array configuration for template-based initialization
- Metadata synchronization between arrays

Time complexity: O(1) - only struct field copy operations
Space complexity: O(1) - only local variables used

Prerequisites:
- dest and src must point to valid Array instances of the same type T
- Both arrays must live in memory managed by memcore
- The destination array's data region must be valid (if dataAddrOffset is set)

Edge cases:
  - Only copies header fields; data elements are not touched
  - The destination array's dataAddrOffset will match the source, which may point to invalid
    data if the destination's data region is not properly set up
  - Version number is copied, which may not reflect the actual state of destination data

Additional notes:
- This function does not validate that the destination's data region is valid
- After cloning, the destination array will have the same capacity and metadata as source
- The data region is not copied or validated; caller must ensure data region is valid
- Useful for initializing arrays with the same configuration but different data
*/
func ArrayHeaderClone[T any](dest, src memcore.MarkRaw) {
	dstHeader := memcore.MemcoreMarkDereferenceObjectUnsafe[Array[T]](dest)
	srcHeader := memcore.MemcoreMarkDereferenceObjectUnsafe[Array[T]](src)

	*dstHeader = *srcHeader
}

/*
ArrayCopyFrom copies the entire contents of the source array into the destination array,
starting at the specified destination index.

This function copies all elements from the source array (from index 0 to capacity-1) into
the destination array starting at destStartIdx. Both arrays must have the same element
type, and the destination must have sufficient capacity to accommodate the copy operation.

Use cases:
- Copying array contents to a different location
- Merging arrays by copying into a larger destination
- Initializing arrays from existing array data
- Array migration and data movement operations

Time complexity: O(n) - where n is the source array capacity (number of elements copied)
Space complexity: O(1) - only local variables used

Prerequisites:
- dest and src must point to valid Array instances of the same type T
- Both arrays must live in memory managed by memcore
- destStartIdx + srcCapacity must not exceed destCapacity
- Both arrays must have the same element type T

Edge cases:
- Returns error if destination capacity is insufficient
- Copies all elements from source (entire capacity, not just used elements)
- Example: copy src[0:srcCap] → dest[destStartIdx : destStartIdx+srcCap]
- Increments destination array version after successful copy

Additional notes:
- This function works with both contiguous and non-contiguous array layouts
- The copy operation uses efficient memory move operations
- The destination array's version is incremented to indicate modification
- Source array version is not modified
*/
func ArrayCopyFrom[T any](dest memcore.MarkRaw, src memcore.MarkRaw, destStartIdx uint64) error {
	destBase, destHeader := memcore.MemcoreMarkDereferenceObjectAltUnsafe[Array[T]](dest)
	srcBase, srcHeader := memcore.MemcoreMarkDereferenceObjectAltUnsafe[Array[T]](src)

	srcCap := srcHeader.capacity
	destCap := destHeader.capacity

	if destStartIdx+srcCap > destCap {
		return fmt.Errorf(
			"ArrayCopyFrom: insufficient capacity (destStart=%d, srcCap=%d, destCap=%d)",
			destStartIdx, srcCap, destCap,
		)
	}

	srcPtr := arrayComputeDataAddr(srcHeader, srcBase)
	dstPtr := unsafe.Add(arrayComputeDataAddr(destHeader, destBase), uintptr(destStartIdx)*destHeader.itemSize)

	totalBytes := uintptr(srcCap) * uintptr(srcHeader.itemSize)
	memcore.MemoryMoveNoHeapPointers(dstPtr, srcPtr, totalBytes)

	arrayIncrementVersion[T](dest)
	return nil
}

/*
ArrayCopyFromRange copies a contiguous range of elements from the source array into the
destination array, starting at the specified destination index.

This function copies elements from the source array in the range [from, to) (from inclusive,
to exclusive) into the destination array starting at destStartIdx. Both arrays must have
the same element type, and bounds are validated before copying.

Use cases:
- Copying a subset of array elements to another location
- Extracting and moving specific ranges of data
- Partial array merging operations
- Selective data migration between arrays

Time complexity: O(n) - where n is the number of elements in the range (to - from)
Space complexity: O(1) - only local variables used

Prerequisites:
- dest and src must point to valid Array instances of the same type T
- Both arrays must live in memory managed by memcore
- from must be less than to (range must be valid)
- to must not exceed source array capacity
- destStartIdx + (to - from) must not exceed destination array capacity

Edge cases:
- Returns error if from >= to (invalid range)
- Returns error if to exceeds source capacity
- Returns error if destination capacity is insufficient
- The range [from, to) is half-open (from inclusive, to exclusive)
- Increments destination array version after successful copy

Additional notes:
- This function works with both contiguous and non-contiguous array layouts
- The copy operation uses efficient memory move operations
- The destination array's version is incremented to indicate modification
- Source array version is not modified
*/
func ArrayCopyFromRange[T any](
	dest memcore.MarkRaw,
	src memcore.MarkRaw,
	from, to, destStartIdx uint64,
) error {
	if from >= to {
		return fmt.Errorf("ArrayCopyFromRange: from must be < to")
	}

	srcBase, srcHeader := memcore.MemcoreMarkDereferenceObjectAltUnsafe[Array[T]](src)
	destBase, destHeader := memcore.MemcoreMarkDereferenceObjectAltUnsafe[Array[T]](dest)

	srcCap := srcHeader.capacity
	if to > srcCap {
		return fmt.Errorf("ArrayCopyFromRange: 'to' exceeds src capacity")
	}

	count := to - from

	if destStartIdx+count > destHeader.capacity {
		return fmt.Errorf("ArrayCopyFromRange: insufficient dest capacity")
	}

	srcPtr := unsafe.Add(arrayComputeDataAddr(srcHeader, srcBase), uintptr(from)*srcHeader.itemSize)
	dstPtr := unsafe.Add(arrayComputeDataAddr(destHeader, destBase), uintptr(destStartIdx)*destHeader.itemSize)

	bytes := uintptr(count) * srcHeader.itemSize
	memcore.MemoryMoveNoHeapPointers(dstPtr, srcPtr, bytes)

	arrayIncrementVersion[T](dest)
	return nil
}

// ArrayHeaderSizeBytesGet returns the required bytes for the Array header.
//
//go:inline
func ArrayHeaderSizeBytesGet[T any]() uint64 {
	return memcore.SizeOf[Array[T]]()
}

// ArrayHeaderAlignmentGet returns the required alignment for the Array header.
//
//go:inline
func ArrayHeaderAlignmentGet[T any]() uint64 {
	return memcore.AlignOf[Array[T]]()
}

// ArrayViewGet produces a view over an array that is potentially a subset and/or readonly.
// It panics if from or to are invalid.
//
//go:inline
func ArrayViewGet[T any](array memcore.MarkRaw, from, to uint64, readonly bool) ArrayView[T] {
	if from >= to {
		panic("from must be smaller than to")
	}

	instance := memcore.MemcoreMarkDereferenceObjectUnsafe[Array[T]](array)

	if err := arrayGuaranteeIdxValidity(instance, from); err != nil {
		panic(fmt.Errorf("from invalid: %w", err))
	}

	if err := arrayGuaranteeIdxValidity(instance, to); err != nil {
		panic(fmt.Errorf("to invalid: %w", err))
	}

	return ArrayView[T]{
		arrayHeader: array,
		startIdx:    from,
		endIdx:      to,
		readonly:    readonly,
	}
}

// ArrayViewGetUnsafe produces a view over an array that is potentially a subset and/or readonly.
// It does no validation of bounds.
//
//go:inline
func ArrayViewGetUnsafe[T any](array memcore.MarkRaw, from, to uint64, readonly bool) ArrayView[T] {
	return ArrayView[T]{
		arrayHeader: array,
		startIdx:    from,
		endIdx:      to,
		readonly:    readonly,
	}
}

// ArrayCapacityGet returns the total amount of elements that can be stored.
//
//go:nosplit
//go:inline
func ArrayCapacityGet[T any](array memcore.MarkRaw) uint64 {
	instance := memcore.MemcoreMarkDereferenceObjectUnsafe[Array[T]](array)
	return instance.capacity
}

// ArrayItemGetAt returns T at idx within the array.
// It returns an error if the idx is invalid.
//
//go:nosplit
//go:inline
func ArrayItemGetAt[T any](array memcore.MarkRaw, idx uint64) (T, error) {
	baseAddr, instance := memcore.MemcoreMarkDereferenceObjectAltUnsafe[Array[T]](array)

	if error := arrayGuaranteeIdxValidity(instance, idx); error != nil {
		var zero T
		return zero, error
	}

	return *(*T)(arrayGetPtrAtIdx(instance, baseAddr, idx)), nil
}

// ArrayItemGetAtUnsafe returns T at idx within the array.
// It does no bounds checks.
//
//go:inline
func ArrayItemGetAtUnsafe[T any](array memcore.MarkRaw, idx uint64) T {
	baseAddr, instance := memcore.MemcoreMarkDereferenceObjectAltUnsafe[Array[T]](array)
	return *(*T)(arrayGetPtrAtIdx(instance, baseAddr, idx))
}

// ArrayItemPtrGetAt returns a pointer to T at idx within the array.
// It returns an error if the idx is invalid.
//
// Using this pointer after deletion or overwriting this idx is undefined behaviour.
// Use at your own discretion!
//
//go:nosplit
//go:inline
func ArrayItemPtrGetAt[T any](array memcore.MarkRaw, idx uint64) (*T, error) {
	baseAddr, instance := memcore.MemcoreMarkDereferenceObjectAltUnsafe[Array[T]](array)
	if error := arrayGuaranteeIdxValidity(instance, idx); error != nil {
		return nil, error
	}

	return (*T)(arrayGetPtrAtIdx(instance, baseAddr, idx)), nil
}

// ArrayItemPtrGetAtUnsafe returns a pointer to T at idx within the array.
// It does no bounds checks.
//
// Using this pointer after deletion or overwriting this idx is undefined behaviour.
// Use at your own discretion!
//
//go:inline
func ArrayItemPtrGetAtUnsafe[T any](array memcore.MarkRaw, idx uint64) *T {
	baseAddr, instance := memcore.MemcoreMarkDereferenceObjectAltUnsafe[Array[T]](array)
	return (*T)(arrayGetPtrAtIdx(instance, baseAddr, idx))
}

// ArrayDataPtrGet returns the current pointer to the underlying data storage in memory.
// This value CAN change if the underlying memory region changes.
// Not stable, so do not store.
//
//go:inline
func ArrayDataPtrGet[T any](array memcore.MarkRaw) unsafe.Pointer {
	baseAddr, instance := memcore.MemcoreMarkDereferenceObjectAltUnsafe[Array[T]](array)
	return arrayComputeDataAddr(instance, baseAddr)
}

// ArrayByteOffsetGetAt returns the offset relative to the memory region for this idx.
// Panics if the idx is invalid.
//
//go:inline
func ArrayByteOffsetGetAt[T any](array memcore.MarkRaw, idx uint64) uintptr {
	instance := memcore.MemcoreMarkDereferenceObject[Array[T]](array)
	if err := arrayGuaranteeIdxValidity(instance, idx); err != nil {
		panic(err)
	}

	return instance.dataAddrOffset + (uintptr(idx) * instance.itemSize)
}

// ArrayByteOffsetGetAtUnsafe returns the offset relative to the memory region for this idx.
// Does no bounds checks.
//
//go:inline
func ArrayByteOffsetGetAtUnsafe[T any](array memcore.MarkRaw, idx uint64) uintptr {
	instance := memcore.MemcoreMarkDereferenceObject[Array[T]](array)
	return instance.dataAddrOffset + (uintptr(idx) * instance.itemSize)
}

// ArraySetAt sets idx of array to value T.
// It returns an error if the idx is invalid.
//
//go:nosplit
//go:inline
func ArraySetAt[T any](array memcore.MarkRaw, idx uint64, value T) error {
	baseAddr, instance := memcore.MemcoreMarkDereferenceObjectAltUnsafe[Array[T]](array)
	if error := arrayGuaranteeIdxValidity(instance, idx); error != nil {
		return error
	}

	itemPtr := arrayGetPtrAtIdx(instance, baseAddr, idx)

	memcore.MemcoreFunctionRetrieveTyped[setFn[T]](instance.setFnID)(itemPtr, value)

	arrayIncrementVersion[T](array)
	return nil
}

// ArraySetAtUnsafe sets idx of array to value T.
// It does no bounds checks.
//
//go:nosplit
//go:inline
func ArraySetAtUnsafe[T any](array memcore.MarkRaw, idx uint64, value T) {
	baseAddr, instance := memcore.MemcoreMarkDereferenceObjectAltUnsafe[Array[T]](array)
	itemPtr := arrayGetPtrAtIdx(instance, baseAddr, idx)

	memcore.MemcoreFunctionRetrieveTyped[setFn[T]](instance.setFnID)(itemPtr, value)
	arrayIncrementVersion[T](array)
}

// ArraySetAll sets all values within the array to value T.
//
//go:inline
func ArraySetAll[T any](array memcore.MarkRaw, v T) {
	ArrayForEachUnsafe[T](array, func(ptr unsafe.Pointer, idx uint64) {
		*(*T)(ptr) = v
	})
	arrayIncrementVersion[T](array)
}

// ArrayZeroAll sets all values within the array to its zero value.
// This is different from clearing the memory to 0.
//
//go:inline
func ArrayZeroAll[T any](array memcore.MarkRaw) {
	var zero T
	ArrayForEachUnsafe[T](array, func(ptr unsafe.Pointer, idx uint64) {
		*(*T)(ptr) = zero
	})
	arrayIncrementVersion[T](array)
}

// ArrayForEachUnsafe calls a function for every element in the array.
//
//go:inline
func ArrayForEachUnsafe[T any](array memcore.MarkRaw, fn func(ptr unsafe.Pointer, idx uint64)) {
	baseAddr, instance := memcore.MemcoreMarkDereferenceObjectAltUnsafe[Array[T]](array)

	for idx := uint64(0); idx < instance.capacity; idx++ {
		ptr := arrayGetPtrAtIdx(instance, baseAddr, idx)
		fn(ptr, idx)
	}
}

// ArrayStrideForEachUnsafe calls a function for every element in the array.
// It visits every stride-th element.
//
// For the tail it calls the tailFn which is supposed to process one element at once.
//
//go:inline
func ArrayStrideForEachUnsafe[T any](
	array memcore.MarkRaw,
	strideFn func(ptr unsafe.Pointer, idx uint64),
	tailFn func(ptr unsafe.Pointer, idx uint64),
	stride uint64,
) {
	baseAddr, instance := memcore.MemcoreMarkDereferenceObjectAltUnsafe[Array[T]](array)

	idx := uint64(0)
	for ; idx+stride < instance.capacity; idx += stride {
		ptr := arrayGetPtrAtIdx(instance, baseAddr, idx)
		strideFn(ptr, idx)
	}

	for ; idx < instance.capacity; idx++ {
		ptr := arrayGetPtrAtIdx(instance, baseAddr, idx)
		tailFn(ptr, idx)
	}
}

// ArrayIterate allows you to iterate over the array efficiently.
// It provides a method to say which next element you need.
// The next function returns true when your requested n exceeds the capacity.
//
//go:inline
func ArrayIterate[T any](
	array memcore.MarkRaw,
	fn func(
		ptr unsafe.Pointer,
		idx uint64,
		next func(n uint64) (unsafe.Pointer, uint64, bool),
	),
) {
	baseAddr, instance := memcore.MemcoreMarkDereferenceObjectAltUnsafe[Array[T]](array)

	idx := uint64(0)
	nextFn := func(n uint64) (unsafe.Pointer, uint64, bool) {
		idx += n

		if err := arrayGuaranteeIdxValidity(instance, idx); err != nil {
			return nil, idx, true
		}

		ptr := arrayGetPtrAtIdx(instance, baseAddr, idx)
		return ptr, idx, false
	}

	ptr := arrayGetPtrAtIdx(instance, baseAddr, idx)
	fn(ptr, idx, nextFn)
}

// ArrayIterateUnsafe allows you to iterate over the array efficiently.
// It provides a method to say which next element you need.
// The next function does no bounds checking. Safety must be guaranteed by the client.
//
//go:inline
func ArrayIterateUnsafe[T any](
	array memcore.MarkRaw,
	fn func(
		ptr unsafe.Pointer,
		idx uint64,
		next func(n uint64) (unsafe.Pointer, uint64),
	),
) {
	baseAddr, instance := memcore.MemcoreMarkDereferenceObjectAltUnsafe[Array[T]](array)

	idx := uint64(0)
	nextFn := func(n uint64) (unsafe.Pointer, uint64) {
		idx += n

		ptr := arrayGetPtrAtIdx(instance, baseAddr, idx)
		return ptr, idx
	}

	ptr := arrayGetPtrAtIdx(instance, baseAddr, idx)
	fn(ptr, idx, nextFn)
}

// ArrayReplaceInternal replaces srcIdx with the value at destIdx efficiently.
//
//go:inline
func ArrayReplaceInternal[T any](array memcore.MarkRaw, srcIdx, destIdx uint64) error {
	baseAddr, instance := memcore.MemcoreMarkDereferenceObjectAltUnsafe[Array[T]](array)
	if error := arrayGuaranteeIdxValidity(instance, srcIdx); error != nil {
		return error
	}

	if error := arrayGuaranteeIdxValidity(instance, destIdx); error != nil {
		return error
	}

	srcPtr := arrayGetPtrAtIdx(instance, baseAddr, srcIdx)
	dstPtr := arrayGetPtrAtIdx(instance, baseAddr, destIdx)

	memcore.MemoryMoveNoHeapPointers(dstPtr, srcPtr, instance.itemSize)

	return nil
}

// ArrayReplaceInternalUnsafe replaces srcIdx with the value at destIdx efficiently.
//
// It does no bounds checks.
//
//go:inline
func ArrayReplaceInternalUnsafe[T any](array memcore.MarkRaw, srcIdx, destIdx uint64) {
	baseAddr, instance := memcore.MemcoreMarkDereferenceObjectAltUnsafe[Array[T]](array)
	srcPtr := arrayGetPtrAtIdx(instance, baseAddr, srcIdx)
	dstPtr := arrayGetPtrAtIdx(instance, baseAddr, destIdx)

	memcore.MemoryMoveNoHeapPointers(dstPtr, srcPtr, instance.itemSize)
	arrayIncrementVersion[T](array)
}

// ArrayShiftRight shifts a contiguous range of elements in the array
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
//	Call:   ArrayShiftRightUnsafe(arr, from=2, to=4, count=1)
//	Moves:  C→D, D→E, E→F
//	After:  [A, B, _, C, D, E, G]
//
//go:nosplit
//go:inline
func ArrayShiftRight[T any](array memcore.MarkRaw, from, to, count uint64) error {
	instance := memcore.MemcoreMarkDereferenceObject[Array[T]](array)
	if from >= instance.capacity || to >= instance.capacity {
		return fmt.Errorf("invalid range: from=%d to=%d capacity=%d", from, to, instance.capacity)
	}
	if count == 0 || from >= to {
		return nil
	}

	ArrayShiftRightUnsafe[T](array, from, to, count)
	return nil
}

// ArrayShiftRightUnsafe shifts a contiguous range of elements in the array
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
//	Call:   ArrayShiftRightUnsafe(arr, from=2, to=4, count=1)
//	Moves:  C→D, D→E, E→F
//	After:  [A, B, _, C, D, E, G]
//
//go:nosplit
//go:inline
func ArrayShiftRightUnsafe[T any](array memcore.MarkRaw, from, to, count uint64) {
	baseAddr, instance := memcore.MemcoreMarkDereferenceObjectAltUnsafe[Array[T]](array)
	elemSize := instance.itemSize
	srcPtr := arrayGetPtrAtIdx(instance, baseAddr, from)
	dstPtr := arrayGetPtrAtIdx(instance, baseAddr, from+count)

	memcore.MemoryMoveNoHeapPointers(dstPtr, srcPtr, uintptr((to-from+1)*uint64(elemSize)))
	arrayIncrementVersion[T](array)
}

// ArrayShiftLeft shifts a contiguous range of elements in the array
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
//	Call:   ArrayShiftLeft(arr, from=3, to=5, count=1)
//	Moves:  D→C, E→D, F→E
//	After:  [A, B, C, D, E, _, G]
//
// Performs bounds checks and returns an error if the range exceeds capacity.
//
//go:nosplit
//go:inline
func ArrayShiftLeft[T any](array memcore.MarkRaw, from, to, count uint64) error {
	instance := memcore.MemcoreMarkDereferenceObject[Array[T]](array)
	if from >= instance.capacity || to >= instance.capacity {
		return fmt.Errorf("invalid range: from=%d to=%d capacity=%d", from, to, instance.capacity)
	}
	if count == 0 || from >= to {
		return nil
	}

	ArrayShiftLeftUnsafe[T](array, from, to, count)
	return nil
}

// ArrayShiftLeftUnsafe shifts a contiguous range of elements in the array
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
//	Call:   ArrayShiftLeft(arr, from=3, to=5, count=1)
//	Moves:  D→C, E→D, F→E
//	After:  [A, B, C, D, E, _, G]
//
//go:nosplit
//go:inline
func ArrayShiftLeftUnsafe[T any](array memcore.MarkRaw, from, to, count uint64) {
	baseAddr, instance := memcore.MemcoreMarkDereferenceObjectAltUnsafe[Array[T]](array)

	elemSize := instance.itemSize
	srcPtr := arrayGetPtrAtIdx(instance, baseAddr, from+count)
	dstPtr := arrayGetPtrAtIdx(instance, baseAddr, from)

	memcore.MemoryMoveNoHeapPointers(dstPtr, srcPtr, uintptr((to-from+1)*uint64(elemSize)))
}

// ArrayRangeCopy is a convenience wrapper around shift lift/shift right.
// It automatically determines which to use based on from and to.
//
//go:inline
//go:nosplit
func ArrayRangeCopy[T any](array memcore.MarkRaw, from, to, count uint64) error {
	if from < to {
		return ArrayShiftRight[T](array, from, to, count)
	}

	return ArrayShiftLeft[T](array, from, to, count)
}

// ArrayRangeCopyUnsafe is a convenience wrapper around shift lift/shift right unsafe.
// It automatically determines which to use based on from and to.
//
//go:inline
//go:nosplit
func ArrayRangeCopyUnsafe[T any](array memcore.MarkRaw, from, to, count uint64) {
	if from < to {
		ArrayShiftRightUnsafe[T](array, from, to, count)
		return
	}

	ArrayShiftLeftUnsafe[T](array, from, to, count)
}

// ArrayDeleteAt resets memory to 0 at a given index, using pointers to this
// index gotten earlier is undefined behaviour.
// It returns an error if the index is invalid.
//
//go:nosplit
//go:inline
func ArrayDeleteAt[T any](array memcore.MarkRaw, idx uint64) error {
	baseAddr, instance := memcore.MemcoreMarkDereferenceObjectAltUnsafe[Array[T]](array)
	if error := arrayGuaranteeIdxValidity(instance, idx); error != nil {
		return error
	}

	currentPtr := arrayGetPtrAtIdx(instance, baseAddr, idx)
	memcore.MemoryClearNoHeapPointers(currentPtr, uintptr(instance.itemSize))
	arrayIncrementVersion[T](array)
	return nil
}

// ArrayDeleteAtUnsafe resets memory to 0 at a given index, using pointers to this
// index gotten earlier is undefined behaviour.
// It does no bounds checks.
//
//go:nosplit
//go:inline
func ArrayDeleteAtUnsafe[T any](array memcore.MarkRaw, idx uint64) {
	baseAddr, instance := memcore.MemcoreMarkDereferenceObjectAltUnsafe[Array[T]](array)
	currentPtr := arrayGetPtrAtIdx(instance, baseAddr, idx)
	memcore.MemoryClearNoHeapPointers(currentPtr, uintptr(instance.itemSize))
	arrayIncrementVersion[T](array)
}

// ArrayClear resets the entire array's memory to 0, allowing it to be reused.
// Do NOT use pointers to previous array items after this as that is undefined behaviour.
//
//go:nosplit
//go:inline
func ArrayClear[T any](array memcore.MarkRaw) {
	baseAddr, instance := memcore.MemcoreMarkDereferenceObjectAltUnsafe[Array[T]](array)
	memcore.MemoryClearNoHeapPointers(arrayComputeDataAddr(instance, baseAddr), uintptr(instance.capacity)*uintptr(instance.itemSize))
	arrayIncrementVersion[T](array)
}

// ArrayIsIdxValid checks whether the given index is valid.
//
//go:inline
func ArrayIsIdxValid[T any](array memcore.MarkRaw, idx uint64) bool {
	instance := memcore.MemcoreMarkDereferenceObject[Array[T]](array)
	return idx < instance.capacity
}

// ArraySort sorts the array in-place.
//
// This implementation uses an iterative Quicksort with Median-of-Three pivot
// selection to ensure O(n log n) performance and zero stack-overflow risk.
//
// The comparison function should return:
// a < b : -1 (or negative)
// a == b : 0
// a > b : 1 (or positive)
func ArraySort[T any](array memcore.MarkRaw, cmp func(a, b T) int) {
	base, inst := memcore.MemcoreMarkDereferenceObjectAltUnsafe[Array[T]](array)

	capacity := inst.capacity
	if capacity <= 1 {
		return
	}

	dataAddr := unsafe.Add(base, inst.dataAddrOffset)
	elemSize := uintptr(inst.itemSize)

	// Manual stack for partitioning bounds. 128 elements can handle
	// up to 2^64 elements in the worst case.
	var stack [128]int64
	top := -1

	// Push initial bounds
	top++
	stack[top] = 0
	top++
	stack[top] = int64(capacity - 1)

	for top >= 0 {
		high := stack[top]
		top--
		low := stack[top]
		top--

		if low < high {
			// Median-of-Three pivot selection
			mid := low + (high-low)/2

			// Sort low, mid, and high pointers to find the median
			valLow := *(*T)(unsafe.Add(dataAddr, uintptr(low)*elemSize))
			valMid := *(*T)(unsafe.Add(dataAddr, uintptr(mid)*elemSize))
			valHigh := *(*T)(unsafe.Add(dataAddr, uintptr(high)*elemSize))

			if cmp(valMid, valLow) < 0 {
				arraySwapRaw[T](dataAddr, low, mid, elemSize)
			}
			if cmp(valHigh, valLow) < 0 {
				arraySwapRaw[T](dataAddr, low, high, elemSize)
			}
			if cmp(valHigh, valMid) < 0 {
				arraySwapRaw[T](dataAddr, mid, high, elemSize)
			}

			// Place pivot at high-1 for partitioning
			arraySwapRaw[T](dataAddr, mid, high, elemSize)

			pivotIdx := arrayPartitionRaw(dataAddr, low, high, elemSize, cmp)

			// Push larger partition first to keep manual stack depth O(log n)
			if pivotIdx-low > high-pivotIdx {
				if pivotIdx-1 > low {
					top++
					stack[top] = low
					top++
					stack[top] = pivotIdx - 1
				}
				if pivotIdx+1 < high {
					top++
					stack[top] = pivotIdx + 1
					top++
					stack[top] = high
				}
			} else {
				if pivotIdx+1 < high {
					top++
					stack[top] = pivotIdx + 1
					top++
					stack[top] = high
				}
				if pivotIdx-1 > low {
					top++
					stack[top] = low
					top++
					stack[top] = pivotIdx - 1
				}
			}
		}
	}
	arrayIncrementVersion[T](array)
}

// ArraySorted returns a sorted variant of this array.
//
// The comparison function should return:
// a < b : -1 (or negative)
// a == b : 0
// a > b : 1 (or positive)
func ArraySorted[T any](
	array memcore.MarkRaw,
	targetArrayAddr memcore.MarkRaw,
	cmp func(a, b T) int,
) {
	srcBase, srcInst := memcore.MemcoreMarkDereferenceObjectAltUnsafe[Array[T]](array)
	dstBase, dstInst := memcore.MemcoreMarkDereferenceObjectAltUnsafe[Array[T]](targetArrayAddr)

	if srcInst.capacity != dstInst.capacity {
		panic("ArraySorted: capacity mismatch")
	}

	srcData := unsafe.Add(srcBase, srcInst.dataAddrOffset)
	dstData := unsafe.Add(dstBase, dstInst.dataAddrOffset)
	totalBytes := uintptr(srcInst.capacity) * uintptr(srcInst.itemSize)

	memcore.MemoryMoveNoHeapPointers(dstData, srcData, totalBytes)

	ArraySort[T](targetArrayAddr, cmp)
	// Note: ArraySort already increments version, so no need to increment here
}

// -------------------------- ARRAY VIEW

// ArrayViewLengthGet returns the length of the current view.
//
//go:inline
func ArrayViewLengthGet[T any](arrayView ArrayView[T]) uint64 {
	return arrayView.endIdx - arrayView.startIdx
}

// ArrayViewIsReadonly returns whether the current view is readonly.
//
//go:inline
func ArrayViewIsReadonly[T any](arrayView ArrayView[T]) bool {
	return arrayView.readonly
}

// ArrayViewItemGetAt returns item at idx (as computed by view startIdx+relativeIdx)
//
//go:inline
func ArrayViewItemGetAt[T any](arrayView ArrayView[T], relativeIdx uint64) (T, error) {
	idx := arrayView.startIdx + relativeIdx
	if err := arrayViewGuaranteeIdxValidity(arrayView, idx); err != nil {
		var zero T
		return zero, err
	}

	return ArrayItemGetAtUnsafe[T](arrayView.arrayHeader, idx), nil
}

// ArrayViewItemPtrGetAt returns item pointer at idx (as computed by view startIdx+relativeIdx)
// Fails if the view is readonly (because getting a pointer would allow mutation)
//
//go:inline
func ArrayViewItemPtrGetAt[T any](arrayView ArrayView[T], relativeIdx uint64) (*T, error) {
	if arrayView.readonly {
		return nil, fmt.Errorf("cannot mutate readonly view")
	}

	idx := arrayView.startIdx + relativeIdx

	if err := arrayViewGuaranteeIdxValidity(arrayView, idx); err != nil {
		return nil, err
	}

	return ArrayItemPtrGetAtUnsafe[T](arrayView.arrayHeader, idx), nil
}

// ArrayViewItemSetAt sets the item at idx (as computed by view startIdx+relativeIdx)
// Fails if the view is readonly.
//
//go:inline
func ArrayViewItemSetAt[T any](arrayView ArrayView[T], relativeIdx uint64, v T) error {
	if arrayView.readonly {
		return fmt.Errorf("cannot mutate readonly view")
	}

	idx := arrayView.startIdx + relativeIdx

	if err := arrayViewGuaranteeIdxValidity(arrayView, idx); err != nil {
		return err
	}

	ArraySetAtUnsafe(arrayView.arrayHeader, idx, v)
	return nil
}

// ArrayViewForEach calls a function for every element in the array view.
// The indexes returned are the relative indexes.
//
//go:inline
func ArrayViewForEach[T any](arrayView ArrayView[T], fn func(item T, idx uint64)) {
	baseAddr, instance := memcore.MemcoreMarkDereferenceObjectAltUnsafe[Array[T]](arrayView.arrayHeader)

	for idx := uint64(arrayView.startIdx); idx < arrayView.endIdx; idx++ {
		relIdx := idx - arrayView.startIdx
		ptr := arrayGetPtrAtIdx(instance, baseAddr, idx)
		item := *(*T)(ptr)
		fn(item, relIdx)
	}
}

// ArrayViewForEachRaw calls a function for every element in the array view.
// Not possible for readonly views.
// The indexes returned are the relative indexes.
//
//go:inline
func ArrayViewForEachRaw[T any](arrayView ArrayView[T], fn func(ptr unsafe.Pointer, idx uint64)) error {
	if arrayView.readonly {
		return fmt.Errorf("cannot mutate readonly view")
	}

	baseAddr, instance := memcore.MemcoreMarkDereferenceObjectAltUnsafe[Array[T]](arrayView.arrayHeader)

	for idx := uint64(arrayView.startIdx); idx < arrayView.endIdx; idx++ {
		relIdx := idx - arrayView.startIdx
		ptr := arrayGetPtrAtIdx(instance, baseAddr, idx)
		fn(ptr, relIdx)
	}

	return nil
}

// ArrayViewStrideForEach calls a function for every element in the array view.
// It visits every stride-th element.
// The indexes returned are the relative indexes.
//
//go:inline
func ArrayViewStrideForEach[T any](arrayView ArrayView[T], fn func(item T, idx uint64), stride uint64) {
	baseAddr, instance := memcore.MemcoreMarkDereferenceObjectAltUnsafe[Array[T]](arrayView.arrayHeader)

	for idx := uint64(arrayView.startIdx); idx < arrayView.endIdx; idx++ {
		relIdx := idx - arrayView.startIdx
		if relIdx%stride != 0 {
			continue
		}

		ptr := arrayGetPtrAtIdx(instance, baseAddr, relIdx)
		item := *(*T)(ptr)
		fn(item, idx)
	}
}

// ArrayViewStrideForEachRaw calls a function for every element in the array view.
// It visits every stride-th element.
// Not possible for readonly views.
// The indexes returned are the relative indexes.
//
//go:inline
func ArrayViewStrideForEachRaw[T any](arrayView ArrayView[T], fn func(ptr unsafe.Pointer, idx uint64), stride uint64) error {
	if arrayView.readonly {
		return fmt.Errorf("cannot mutate readonly view")
	}

	baseAddr, instance := memcore.MemcoreMarkDereferenceObjectAltUnsafe[Array[T]](arrayView.arrayHeader)

	for idx := uint64(arrayView.startIdx); idx < arrayView.endIdx; idx++ {
		relIdx := idx - arrayView.startIdx
		if relIdx%stride != 0 {
			continue
		}

		ptr := arrayGetPtrAtIdx(instance, baseAddr, relIdx)
		fn(ptr, idx)
	}

	return nil
}

// ArrayViewIterate allows you to iterate over the array view efficiently.
// It provides a method to say which next element you need.
// The next function returns true when your requested n exceeds the view length.
// The indexes returned are the relative indexes.
//
//go:inline
func ArrayViewIterate[T any](
	arrayView ArrayView[T],
	fn func(item T, idx uint64, next func(n uint64) (T, uint64, bool)),
) {
	baseAddr, instance := memcore.MemcoreMarkDereferenceObjectAltUnsafe[Array[T]](arrayView.arrayHeader)

	idx := arrayView.startIdx
	nextFn := func(n uint64) (T, uint64, bool) {
		idx += n
		relIdx := idx - arrayView.startIdx

		if err := arrayViewGuaranteeIdxValidity(arrayView, idx); err != nil {
			var zero T
			return zero, idx, true
		}

		ptr := arrayGetPtrAtIdx(instance, baseAddr, idx)
		item := *(*T)(ptr)
		return item, relIdx, false
	}

	ptr := arrayGetPtrAtIdx(instance, baseAddr, idx)
	item := *(*T)(ptr)
	fn(item, 0, nextFn)
	idx += 1
}

// ArrayViewIterateRaw allows you to iterate over the array view efficiently.
// It provides a method to say which next element you need.
// The next function returns true when your requested n exceeds the view length.
// Not possible for readonly views.
// The indexes returned are the relative indexes.
//
//go:inline
func ArrayViewIterateRaw[T any](
	arrayView ArrayView[T],
	fn func(ptr unsafe.Pointer, idx uint64, next func(n uint64) (unsafe.Pointer, uint64, bool)),
) error {
	if arrayView.readonly {
		return fmt.Errorf("cannot mutate readonly view")
	}

	baseAddr, instance := memcore.MemcoreMarkDereferenceObjectAltUnsafe[Array[T]](arrayView.arrayHeader)

	idx := arrayView.startIdx
	nextFn := func(n uint64) (unsafe.Pointer, uint64, bool) {
		idx += n
		relIdx := idx - arrayView.startIdx

		if err := arrayViewGuaranteeIdxValidity(arrayView, idx); err != nil {
			return nil, idx, true
		}

		ptr := arrayGetPtrAtIdx(instance, baseAddr, idx)
		return ptr, relIdx, false
	}

	ptr := arrayGetPtrAtIdx(instance, baseAddr, idx)
	fn(ptr, 0, nextFn)
	idx += 1
	return nil
}

// ArrayViewNestedGet creates a nested view inside an existing view.
// The indices [from:to) are relative to the current view.
// The readonly status is preserved.
func ArrayViewNestedGet[T any](view ArrayView[T], from, to uint64) ArrayView[T] {
	length := view.endIdx - view.startIdx

	if from >= to {
		panic("from must be smaller than to")
	}

	if to > length {
		panic(fmt.Errorf("invalid index (to): %v, must be between 0 and %v (exclusive)", to, length))
	}

	return ArrayView[T]{
		arrayHeader: view.arrayHeader,
		startIdx:    view.startIdx + from,
		endIdx:      view.startIdx + to,
		readonly:    view.readonly,
	}
}

// -------------------------- PRIVATE HELPERS

//go:inline
func arrayViewGuaranteeIdxValidity[T any](view ArrayView[T], globalIdx uint64) error {
	idxValid := globalIdx >= view.startIdx && globalIdx < view.endIdx

	if !idxValid {
		return fmt.Errorf("invalid index: %v, must be between %v and %v (exclusive)", globalIdx, view.startIdx, view.endIdx)
	}

	return nil
}

//go:inline
func arrayGetPtrAtIdx[T any](instance *Array[T], baseAddr unsafe.Pointer, idx uint64) unsafe.Pointer {
	return unsafe.Add(arrayComputeDataAddr(instance, baseAddr), idx*uint64(instance.itemSize))
}

//go:inline
func arrayGuaranteeIdxValidity[T any](instance *Array[T], idx uint64) error {
	idxValid := idx < instance.capacity

	if !idxValid {
		return fmt.Errorf("invalid index: %v, must be between 0 and %v (exclusive)", idx, instance.capacity)
	}

	return nil
}

//go:inline
func arrayComputeDataAddr[T any](instance *Array[T], baseAddr unsafe.Pointer) unsafe.Pointer {
	return unsafe.Add(baseAddr, instance.dataAddrOffset)
}

//go:inline
func arrayIsContiguous[T any](instance *Array[T]) bool {
	headerSize := memcore.SizeOf[Array[T]]()
	return instance.dataAddrOffset == uintptr(headerSize)
}

/*
ArrayUpdateDataAddrOffset updates the data address offset of an array to point to a new data location.

This function allows rebinding an array header to a different data region without reinitializing
the entire array. The offset is calculated as the difference between the new data address and
the header address, enabling efficient cursor-based iteration where a single header is reused
and its data pointer is updated for each element.

Use cases:
- Cursor-based iteration with reusable array headers
- Rebinding arrays to different data regions
- Efficient sequential access patterns
- Avoiding header reallocation in tight loops

Time complexity: O(1) - single field update
Space complexity: O(1) - no allocations

Prerequisites:
- arrayAddr must point to a valid Array instance
- dataAddr must point to a valid memory address for the data region
- The data region must have sufficient capacity for the array's capacity
- Both addresses must be within memory managed by memcore

Edge cases:
- The offset can be negative if data is located before the header in memory
- No validation is performed on data capacity or alignment
- The array's capacity and itemSize remain unchanged

Additional notes:
- This function only updates the dataAddrOffset field, preserving all other array metadata
- Useful for cursor-based iteration where the header is reused and data pointer is updated
- The offset calculation follows the same pattern as ArrayInitializeWithSeparatedHeaderAndData
- Type safety is maintained through the generic parameter T
*/
func ArrayUpdateDataAddrOffset[T any](arrayAddr memcore.MarkRaw, dataAddr memcore.MarkRaw) {
	arrayPtr := memcore.MemcoreMarkDereferenceObject[Array[T]](arrayAddr)

	headerPtr := uintptr(unsafe.Pointer(arrayPtr))
	dataPtr := uintptr(memcore.MemcoreMarkDereference(dataAddr))

	// Calculate offset: dataPtr - headerPtr so that headerPtr + offset = dataPtr
	dataAddrOffset := dataPtr - headerPtr
	arrayPtr.dataAddrOffset = dataAddrOffset
}

// ArrayVersionGet returns the current version of the array.
//
// Version increments on every modification to the array data, allowing cache invalidation
// mechanisms to detect when cached values become stale.
//
//go:inline
func ArrayVersionGet[T any](array memcore.MarkRaw) uint64 {
	instance := memcore.MemcoreMarkDereferenceObjectUnsafe[Array[T]](array)
	return instance.version
}

//go:inline
func arrayIncrementVersion[T any](array memcore.MarkRaw) {
	instance := memcore.MemcoreMarkDereferenceObjectUnsafe[Array[T]](array)
	instance.version++
}

//go:inline
func arrayPartitionRaw[T any](
	data unsafe.Pointer,
	low, high int64,
	size uintptr,
	cmp func(a, b T) int,
) int64 {
	pivotPtr := unsafe.Add(data, uintptr(high)*size)
	pivot := *(*T)(pivotPtr)

	i := low - 1
	for j := low; j < high; j++ {
		currentPtr := unsafe.Add(data, uintptr(j)*size)
		currentVal := *(*T)(currentPtr)

		if cmp(currentVal, pivot) < 0 {
			i++
			arraySwapRaw[T](data, i, j, size)
		}
	}
	arraySwapRaw[T](data, i+1, high, size)
	return i + 1
}

//go:inline
func arraySwapRaw[T any](data unsafe.Pointer, i, j int64, size uintptr) {
	if i == j {
		return
	}
	ptrI := (*T)(unsafe.Add(data, uintptr(i)*size))
	ptrJ := (*T)(unsafe.Add(data, uintptr(j)*size))

	tmp := *ptrI
	*ptrI = *ptrJ
	*ptrJ = tmp
}
