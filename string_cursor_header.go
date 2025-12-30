package memstruct

import (
	"memcore"
	"unsafe"
)

/*
StringCursorHeader caches the String header pointer for hot-path operations that bypass MarkRaw dereference overhead.

This cursor type stores a direct pointer to the String header, eliminating the need to dereference
MarkRaw on every operation. This is optimized for hot paths where memory movement is guaranteed
not to occur during the cursor's lifetime.

Use cases:
- High-performance loops with repeated string operations
- Batch processing operations with many string access operations
- Hot paths where memory stability is guaranteed
- Reducing overhead in performance-critical code sections

Time complexity: O(1) - structure initialization is constant-time
Space complexity: O(1) - fixed-size structure

Prerequisites:
- Must be created using StringCursorHeaderCreate
- String memory must remain valid and unmoved for cursor lifetime
- Memory movement invalidates the cursor (undefined behavior if used after movement)

Edge cases:
- Cursor becomes invalid if string memory is deallocated or unregistered
- Cursor becomes invalid if memory region is relocated
- String data pointer is computed from cached header

Additional notes:
- Cursor caches the header pointer for maximum performance
- Uses unsafe pointer operations for direct memory access
- Suitable for both sequential and random access patterns
- The cursor is a value type and can be copied, but shares the same underlying header
*/
type StringCursorHeader struct {
	header *String
}

/*
StringCursorHeaderCreate creates a cursor that caches the String header pointer for hot-path operations.

This function performs a single MarkRaw dereference and caches the resulting header pointer,
enabling subsequent operations to bypass the dereference overhead. The cursor is optimized for
hot paths where memory movement is guaranteed not to occur during the cursor's lifetime.

Use cases:
- Setting up cursor-based string operations
- High-performance string manipulation patterns
- Batch processing operations
- Performance-critical code sections

Time complexity: O(1) - single dereference operation
Space complexity: O(1) - returns a fixed-size cursor structure

Prerequisites:
- str must be a valid MarkRaw pointing to an initialized String
- String must not have been deallocated or unregistered
- Memory must remain stable during cursor lifetime

Edge cases:
- Cursor becomes invalid if string memory is deallocated or unregistered
- Cursor becomes invalid if memory region is relocated
- Returns cursor with nil header if string is invalid (undefined behavior)

Additional notes:
- Cursor caches header pointer for maximum performance
- Uses unsafe pointer operations for direct memory access
- No header manipulation required after cursor creation
- The cursor is a value type and can be copied, but shares the same underlying header
*/
//
//go:inline
func StringCursorHeaderCreate(str memcore.MarkRaw) StringCursorHeader {
	header := memcore.MemcoreMarkDereferenceObjectUnsafe[String](str)
	return StringCursorHeader{
		header: header,
	}
}

/*
StringCursorHeaderLengthGet returns the string's length in bytes.

This function uses the cached header pointer to access the string length, bypassing MarkRaw
dereference overhead.

Use cases:
- Cursor-based length queries
- Bounds checking before operations
- Memory allocation planning

Time complexity: O(1) - returns cached value
Space complexity: O(1) - no allocations

Prerequisites:
- cursor must be a valid StringCursorHeader created with StringCursorHeaderCreate
- String memory must remain valid and unmoved

Edge cases:
- Returns the string's current length in bytes
- Length does not change after cursor creation unless string is modified
- Undefined behavior if cursor is invalid or memory has moved

Additional notes:
- Uses cached header pointer for maximum performance
- Length is stored in the string header
- Length represents UTF-8 byte count, not rune count
*/
//
//go:inline
func StringCursorHeaderLengthGet(cursor StringCursorHeader) uint64 {
	return cursor.header.length
}

/*
StringCursorHeaderBytesGet returns a slice of bytes containing the string's UTF-8 data.

This function uses the cached header pointer to access the string data, bypassing MarkRaw
dereference overhead. The returned slice is a view into the underlying memory and should not
be modified.

Use cases:
- Cursor-based byte access
- Zero-copy string data access
- Performance-critical string operations

Time complexity: O(1) - pointer computation only
Space complexity: O(1) - returns a slice view, no allocation

Prerequisites:
- cursor must be a valid StringCursorHeader created with StringCursorHeaderCreate
- String memory must remain valid and unmoved

Edge cases:
- Returns empty slice if string length is 0
- The returned slice is a view and should not be modified
- Undefined behavior if cursor is invalid or memory has moved

Additional notes:
- Uses cached header pointer for maximum performance
- Computes data pointer from header offset
- Returns a slice view into the underlying memory
- The slice should not be modified (string is immutable)
*/
//
//go:inline
func StringCursorHeaderBytesGet(cursor StringCursorHeader) []byte {
	if cursor.header.length == 0 {
		return nil
	}
	baseAddr := unsafe.Pointer(cursor.header)
	dataPtr := unsafe.Add(baseAddr, cursor.header.dataAddrOffset)
	return unsafe.Slice((*byte)(dataPtr), cursor.header.length)
}

/*
StringCursorHeaderToGo converts the manually managed string to a Go string (zero-copy).

This function uses the cached header pointer to convert the string to a Go string, bypassing MarkRaw
dereference overhead. The conversion is zero-copy and creates a Go string that references the
underlying manual memory.

Use cases:
- Cursor-based string conversion
- Zero-copy string access
- Integration with Go string APIs

Time complexity: O(1) - pointer computation only
Space complexity: O(1) - returns a Go string view, no allocation

Prerequisites:
- cursor must be a valid StringCursorHeader created with StringCursorHeaderCreate
- String memory must remain valid and unmoved

Edge cases:
- Returns empty string if string length is 0
- The returned Go string references the underlying manual memory
- Undefined behavior if cursor is invalid or memory has moved
- Unsafe if the underlying manual memory is modified or freed afterward

Additional notes:
- Uses cached header pointer for maximum performance
- Computes data pointer from header offset
- Creates a Go string that references the underlying memory
- The Go string becomes invalid if the underlying memory is modified or freed
*/
//
//go:inline
func StringCursorHeaderToGo(cursor StringCursorHeader) string {
	if cursor.header.length == 0 {
		return ""
	}
	baseAddr := unsafe.Pointer(cursor.header)
	dataPtr := unsafe.Add(baseAddr, cursor.header.dataAddrOffset)
	header := struct {
		Data uintptr
		Len  int
	}{uintptr(dataPtr), int(cursor.header.length)}
	return *(*string)(unsafe.Pointer(&header))
}

