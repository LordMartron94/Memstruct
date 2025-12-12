# memstruct

High-performance data structures built on manual memory management for Go.

## Overview

`memstruct` provides a comprehensive collection of data structures (arrays, vectors, matrices, stacks, queues, hash maps, etc.) that operate on manually managed memory, bypassing Go's garbage collector for maximum performance and predictable memory usage.

## Design Philosophy

- **Manual Memory**: All data structures live in memory allocated via `memcore`/`memforge`
- **No GC Overhead**: Zero garbage collection pauses
- **Memory Efficient**: Compact memory layouts with minimal overhead
- **Type Safe**: Generic data structures with compile-time type checking
- **Snapshot Support**: Deep copy and restore capabilities for checkpointing
- **View System**: Readonly and subset views without copying data

## Data Structures

### Array[T]

A fixed-capacity array for any type `T`.

**Features:**
- Fixed capacity, variable length
- Zero-copy views and cursors
- Snapshot/restore support
- Efficient element access and iteration

**Example:**
```go
// Initialize at a memory location
memcore.MemcoreMarkCreate(regionID, offset)
memstruct.ArrayInitializeAt[int](arrayMark, 100) // capacity=100
arrayPtr := memcore.MemcoreMarkDereferenceObject[memstruct.Array[int]](arrayMark)

// Set/get elements
memstruct.ArraySetAt[int](arrayMark, 0, 42)
value := memstruct.ArrayGetAt[int](arrayMark, 0)
```

### Vector[T]

A fixed-capacity array specialized for numeric types (`foundation.Numeric`).

**Features:**
- Same API as `Array` but optimized for numerics
- Used by numerical computing libraries like `blaze`
- Supports all array operations plus numeric-specific utilities

**Example:**
```go
memstruct.VectorInitializeAt[float64](vectorMark, 1000)
memstruct.VectorSetAt[float64](vectorMark, 0, 3.14)
```

### Matrix[T]

A 2D matrix structure for numeric types, stored in row-major order.

**Features:**
- Wraps a `Vector` internally
- Row and column access
- Matrix views for submatrices
- Efficient element indexing

**Example:**
```go
memstruct.MatrixInitializeAt[float32](matrixMark, 10, 20) // 10 rows, 20 cols
memstruct.MatrixSetAt[float32](matrixMark, 0, 0, 1.5) // Set element at row 0, col 0
value := memstruct.MatrixGetAt[float32](matrixMark, 0, 0)
```

### Stack[T]

A last-in-first-out (LIFO) stack.

**Features:**
- Fixed capacity
- O(1) push and pop
- Efficient memory usage

**Example:**
```go
memstruct.StackInitializeAt[int](stackMark, 100)
memstruct.StackPush[int](stackMark, 42)
value := memstruct.StackPop[int](stackMark)
isEmpty := memstruct.StackIsEmpty[int](stackMark)
```

### Queue[T]

A first-in-first-out (FIFO) queue implemented as a circular buffer.

**Features:**
- Fixed capacity
- O(1) enqueue and dequeue
- Efficient wrap-around logic

**Example:**
```go
memstruct.QueueInitializeAt[string](queueMark, 100)
memstruct.QueueEnqueue[string](queueMark, "hello")
value := memstruct.QueueDequeue[string](queueMark)
```

### PriorityQueue[T]

A priority queue (heap) for any comparable type.

**Features:**
- Fixed capacity
- O(log n) insert and extract
- Custom comparison function support

**Example:**
```go
compareFn := func(a, b int) bool { return a < b } // min-heap
memstruct.PriorityQueueInitializeAt[int](queueMark, 100, compareFn)
memstruct.PriorityQueueInsert[int](queueMark, 42)
min := memstruct.PriorityQueueExtract[int](queueMark)
```

### HashMap[TKey, TValue]

A hash map with open addressing.

**Features:**
- Fixed capacity (load factor determines effective size)
- O(1) average case insert/lookup
- Custom key comparison and hashing
- Supports any key and value types

**Example:**
```go
keyCompare := func(a, b string) bool { return a == b }
keyHash := func(key string) uint64 { /* hash function */ }
keyMark := func(key string) memcore.MarkRaw { /* convert to mark */ }

memstruct.HashMapInitializeAt[string, int](
    mapMark, 100, keyCompare, keyHash, keyMark,
)
memstruct.HashMapInsert[string, int](mapMark, "key", 42)
value, found := memstruct.HashMapGet[string, int](mapMark, "key")
```

### FixedOrderedList[T]

An ordered list that maintains insertion order.

**Features:**
- Fixed capacity
- Maintains order while allowing insertions and deletions
- Useful for priority queues or sorted lists with fixed size

### String

A manually managed, immutable string representation.

**Features:**
- UTF-8 byte storage
- No null terminator (length stored in header)
- Safe for `MemoryMoveNoHeapPointers`
- Compatible with snapshot/restore

**Example:**
```go
memstruct.StringInitializeAt(stringMark, "Hello, World!")
length := memstruct.StringLengthGet(stringMark)
bytes := memstruct.StringBytesGet(stringMark)
```

## Common Operations

### Initialization

All data structures require initialization at a memory location:

```go
// Calculate required size
sizeBytes := memstruct.ArrayRequiredBytesGet[int](capacity)
alignment := memstruct.ArrayRequiredAlignmentGet[int]()

// Allocate memory (using memforge or memcore)
mark := allocator.Malloc(sizeBytes, alignment)

// Initialize
memstruct.ArrayInitializeAt[int](mark, capacity)
arrayPtr := memcore.MemcoreMarkDereferenceObject[memstruct.Array[int]](mark)
```

### Views

Many structures support views (readonly or writable subsets):

```go
// Create a readonly view of array[10:20]
view := memstruct.ArrayViewCreate[int](arrayMark, 10, 20, true)

// Access view elements
value := memstruct.ArrayViewGetAt[int](view, 0) // Accesses array[10]
```

### Snapshots

Deep copy structures for checkpointing:

```go
// Create snapshot
snapshotMark := allocator.Malloc(sizeBytes, alignment)
memstruct.ArraySnapshotCreate[int](snapshotMark, arrayMark)

// Later, restore
memstruct.ArraySnapshotRestore[int](arrayMark, snapshotMark)
```

### Cursors

Iterate through arrays efficiently:

```go
cursor := memstruct.ArrayCursorCreate[int](arrayMark)
for memstruct.ArrayCursorHasNext[int](cursor) {
    value := memstruct.ArrayCursorNext[int](cursor)
    // Process value
}
```

## Memory Layout

All structures use a compact header + data layout:

```
┌─────────────────────┐
│ Structure Header    │  (capacity, offsets, metadata)
├─────────────────────┤
│ Data Region         │  (actual elements)
└─────────────────────┘
```

The header stores only essential metadata. All data is stored contiguously for cache efficiency.

## Size and Alignment Utilities

Every structure provides:

- **`RequiredBytesGet(capacity)`** - Calculate total byte size needed
- **`RequiredAlignmentGet()`** - Get alignment requirements

These are essential for proper allocation:

```go
size := memstruct.ArrayRequiredBytesGet[MyStruct](100)
align := memstruct.ArrayRequiredAlignmentGet[MyStruct]()
mark := allocator.Malloc(size, align)
```

## Safety Guidelines

⚠️ **Important:**

1. **No Go Pointers**: Never store Go pointers (slices, maps, channels, etc.) in these structures. Only store plain data types, structs without pointer fields, or manually managed handles.

2. **Lifetime Management**: Structures are invalid after their memory region is unmapped. Ensure allocators/regions outlive their structures.

3. **Bounds Checking**: Some operations perform bounds checking and may panic. Always verify indices/capacities.

4. **Type Consistency**: Never mix types in a generic structure instance. `Array[int]` must only contain `int` values.

5. **View Lifetime**: Views are invalid after the underlying structure is modified in ways that affect memory layout.

## Performance Characteristics

- **Array/Vector Access**: O(1) - direct pointer arithmetic
- **Matrix Access**: O(1) - row-major indexing
- **Stack Operations**: O(1)
- **Queue Operations**: O(1)
- **PriorityQueue**: O(log n) insert/extract
- **HashMap**: O(1) average case, O(n) worst case
- **Snapshots**: O(n) where n is structure size

All operations avoid heap allocations and GC pauses.

## Integration

`memstruct` works seamlessly with:

- **memcore** - Uses `MarkRaw` and memory utilities
- **memforge** - Uses allocators for memory management
- **memarch** - Factory layer for easy creation (recommended for most users)
- **blaze** - Uses `Vector` and `Matrix` for numerical computing

## Example: Complete Workflow

```go
import (
    "memcore"
    "memforge"
    "memstruct"
)

// Create allocator
allocator := memforge.FixedLinearAllocatorCreate(uint64(memcore.MegaByte))
defer memforge.FixedLinearAllocatorDestroy(allocator)

// Calculate requirements
size := memstruct.ArrayRequiredBytesGet[int](1000)
align := memstruct.ArrayRequiredAlignmentGet[int]()

// Allocate and initialize
mark := memforge.FixedLinearAllocatorMalloc(allocator, size, align)
memstruct.ArrayInitializeAt[int](mark, 1000)
array := memcore.MemcoreMarkDereferenceObject[memstruct.Array[int]](mark)

// Use the array
for i := uint64(0); i < 100; i++ {
    memstruct.ArraySetAt[int](mark, i, int(i)*2)
}
```

## Why memstruct?

Traditional Go data structures (slices, maps) are excellent for most use cases, but they:
- Trigger garbage collection
- Have unpredictable memory layout
- Cannot be snapshotted/restored efficiently
- Cannot be used in real-time systems

`memstruct` provides the same abstractions with:
- Zero GC overhead
- Predictable memory usage
- Snapshot/restore capabilities
- Real-time system compatibility
- Better cache locality

Use `memstruct` when you need deterministic performance and precise memory control.
