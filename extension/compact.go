package extension

import (
	"fmt"
	"memcore"
	"memstruct"
	"sort"
	"unsafe"
)

/*
AllocationFn allocates a region of manual memory with the given size and alignment (e.g. memforge allocator Malloc).
Used by DenseLocalGridCreate to allocate key-space and value arrays.
*/
type AllocationFn func(sizeBytes, alignment uint64) memcore.MarkRaw

/*
IDExtractor maps a key to a uint64 identity used for deduplication and binary-search lookup in DenseLocalGrid key spaces.
*/
type IDExtractor[TKey any] func(key TKey) uint64

/*
DenseLocalGrid is a compact, per-cluster dense matrix built by locally reindexing sparse (xKey, yKey) pairs
into a small fully addressable rectangle. Key spaces store extracted IDs in sorted order for O(log n) lookup.
The runtime grid holds only data (key arrays, value array, optional validity bitmap); extractors are passed at call site.
*/
type DenseLocalGrid[TXKey, TYKey, TValue any] struct {
	xKeySpace    memcore.MarkRaw // Array[uint64]
	yKeySpace    memcore.MarkRaw // Array[uint64]
	valueSpace   memcore.MarkRaw // Array[TValue]
	validBits    memcore.MarkRaw // optional bitmap: (rows*cols+7)/8 bytes
	hasValidBits bool            // true when validBits is allocated (Sparse)
}

/*
Combination is a single (xKey, yKey, value) tuple used to build or reference a DenseLocalGrid cell.
*/
type Combination[TXKey, TYKey, TValue any] struct {
	XKey  TXKey
	YKey  TYKey
	Value TValue
}

/*
DenseLocalGridDuplicatePolicy controls how duplicate (xKey, yKey) pairs in combinations are handled during build.
Duplicate means the same (row, col) is written more than once.
*/
type DenseLocalGridDuplicatePolicy int

const (
	DenseLocalGridDuplicateLastWins  DenseLocalGridDuplicatePolicy = iota // overwrite with latest
	DenseLocalGridDuplicateKeepFirst                                      // ignore subsequent writes
	DenseLocalGridDuplicateError                                          // builder returns error on first duplicate
)

/*
DenseLocalGridCreateFull builds a dense local grid where every cell is considered valid (no validity bitmap).
Use for full relations. duplicatePolicy controls behavior when the same (xKey, yKey) appears more than once in combinations.
*/
func DenseLocalGridCreateFull[TXKey, TYKey, TValue any](
	allocationFn AllocationFn,
	xExtractor IDExtractor[TXKey],
	yExtractor IDExtractor[TYKey],
	combinations []Combination[TXKey, TYKey, TValue],
	duplicatePolicy DenseLocalGridDuplicatePolicy,
) (*DenseLocalGrid[TXKey, TYKey, TValue], error) {
	xIDSet := make(map[uint64]struct{})
	yIDSet := make(map[uint64]struct{})
	for _, c := range combinations {
		xIDSet[xExtractor(c.XKey)] = struct{}{}
		yIDSet[yExtractor(c.YKey)] = struct{}{}
	}
	uniqueX := make([]uint64, 0, len(xIDSet))
	for id := range xIDSet {
		uniqueX = append(uniqueX, id)
	}
	uniqueY := make([]uint64, 0, len(yIDSet))
	for id := range yIDSet {
		uniqueY = append(uniqueY, id)
	}
	sort.Slice(uniqueX, func(i, j int) bool { return uniqueX[i] < uniqueX[j] })
	sort.Slice(uniqueY, func(i, j int) bool { return uniqueY[i] < uniqueY[j] })
	rows := uint64(len(uniqueX))
	cols := uint64(len(uniqueY))

	valueCap := rows * cols
	if rows > 0 && cols > 0 && valueCap/rows != cols {
		return nil, fmt.Errorf("DenseLocalGridCreateFull: overflow rows*cols")
	}

	xKeySpaceSize := memstruct.ArrayRequiredBytesGet[uint64](rows)
	xKeySpaceAlign := memstruct.ArrayRequiredAlignmentGet[uint64]()
	xKeySpaceMark := allocationFn(xKeySpaceSize, xKeySpaceAlign)
	memstruct.ArrayInitializeAt[uint64](xKeySpaceMark, rows)

	yKeySpaceSize := memstruct.ArrayRequiredBytesGet[uint64](cols)
	yKeySpaceAlign := memstruct.ArrayRequiredAlignmentGet[uint64]()
	yKeySpaceMark := allocationFn(yKeySpaceSize, yKeySpaceAlign)
	memstruct.ArrayInitializeAt[uint64](yKeySpaceMark, cols)

	valueSpaceSize := memstruct.ArrayRequiredBytesGet[TValue](valueCap)
	valueSpaceAlign := memstruct.ArrayRequiredAlignmentGet[TValue]()
	valueSpaceMark := allocationFn(valueSpaceSize, valueSpaceAlign)
	memstruct.ArrayInitializeAt[TValue](valueSpaceMark, valueCap)

	for i, id := range uniqueX {
		_ = memstruct.ArraySetAt[uint64](xKeySpaceMark, uint64(i), id)
	}
	for j, id := range uniqueY {
		_ = memstruct.ArraySetAt[uint64](yKeySpaceMark, uint64(j), id)
	}

	xIDToRow := make(map[uint64]uint64, len(uniqueX))
	for row, id := range uniqueX {
		xIDToRow[id] = uint64(row)
	}
	yIDToCol := make(map[uint64]uint64, len(uniqueY))
	for col, id := range uniqueY {
		yIDToCol[id] = uint64(col)
	}

	var writtenBits []byte
	if duplicatePolicy == DenseLocalGridDuplicateError || duplicatePolicy == DenseLocalGridDuplicateKeepFirst {
		writtenBits = make([]byte, (valueCap+7)/8)
	}

	for _, c := range combinations {
		xID := xExtractor(c.XKey)
		yID := yExtractor(c.YKey)
		row, okX := xIDToRow[xID]
		if !okX {
			return nil, fmt.Errorf("DenseLocalGridCreateFull: xKey not in key space (extractor non-deterministic or builder bug)")
		}
		col, okY := yIDToCol[yID]
		if !okY {
			return nil, fmt.Errorf("DenseLocalGridCreateFull: yKey not in key space (extractor non-deterministic or builder bug)")
		}
		idx := row*cols + col
		if writtenBits != nil {
			byteIdx := idx / 8
			bitIdx := idx % 8
			mask := byte(1 << uint(bitIdx))
			if (writtenBits[byteIdx] & mask) != 0 {
				if duplicatePolicy == DenseLocalGridDuplicateError {
					return nil, fmt.Errorf("DenseLocalGridCreateFull: duplicate (xKey, yKey) at row=%d col=%d", row, col)
				}
				continue
			}
			writtenBits[byteIdx] |= mask
		}
		_ = memstruct.ArraySetAt[TValue](valueSpaceMark, idx, c.Value)
	}

	return &DenseLocalGrid[TXKey, TYKey, TValue]{
		xKeySpace:    xKeySpaceMark,
		yKeySpace:    yKeySpaceMark,
		valueSpace:   valueSpaceMark,
		validBits:    memcore.MarkRaw{},
		hasValidBits: false,
	}, nil
}

/*
DenseLocalGridCreateSparse builds a dense local grid with a validity bitmap; only cells written from combinations are valid.
Use for partial relations / automata. duplicatePolicy controls behavior when the same (xKey, yKey) appears more than once.
*/
func DenseLocalGridCreateSparse[TXKey, TYKey, TValue any](
	allocationFn AllocationFn,
	xExtractor IDExtractor[TXKey],
	yExtractor IDExtractor[TYKey],
	combinations []Combination[TXKey, TYKey, TValue],
	duplicatePolicy DenseLocalGridDuplicatePolicy,
) (*DenseLocalGrid[TXKey, TYKey, TValue], error) {
	xIDSet := make(map[uint64]struct{})
	yIDSet := make(map[uint64]struct{})
	for _, c := range combinations {
		xIDSet[xExtractor(c.XKey)] = struct{}{}
		yIDSet[yExtractor(c.YKey)] = struct{}{}
	}
	uniqueX := make([]uint64, 0, len(xIDSet))
	for id := range xIDSet {
		uniqueX = append(uniqueX, id)
	}
	uniqueY := make([]uint64, 0, len(yIDSet))
	for id := range yIDSet {
		uniqueY = append(uniqueY, id)
	}
	sort.Slice(uniqueX, func(i, j int) bool { return uniqueX[i] < uniqueX[j] })
	sort.Slice(uniqueY, func(i, j int) bool { return uniqueY[i] < uniqueY[j] })
	rows := uint64(len(uniqueX))
	cols := uint64(len(uniqueY))

	valueCap := rows * cols
	if rows > 0 && cols > 0 && valueCap/rows != cols {
		return nil, fmt.Errorf("DenseLocalGridCreateSparse: overflow rows*cols")
	}

	xKeySpaceSize := memstruct.ArrayRequiredBytesGet[uint64](rows)
	xKeySpaceAlign := memstruct.ArrayRequiredAlignmentGet[uint64]()
	xKeySpaceMark := allocationFn(xKeySpaceSize, xKeySpaceAlign)
	memstruct.ArrayInitializeAt[uint64](xKeySpaceMark, rows)

	yKeySpaceSize := memstruct.ArrayRequiredBytesGet[uint64](cols)
	yKeySpaceAlign := memstruct.ArrayRequiredAlignmentGet[uint64]()
	yKeySpaceMark := allocationFn(yKeySpaceSize, yKeySpaceAlign)
	memstruct.ArrayInitializeAt[uint64](yKeySpaceMark, cols)

	valueSpaceSize := memstruct.ArrayRequiredBytesGet[TValue](valueCap)
	valueSpaceAlign := memstruct.ArrayRequiredAlignmentGet[TValue]()
	valueSpaceMark := allocationFn(valueSpaceSize, valueSpaceAlign)
	memstruct.ArrayInitializeAt[TValue](valueSpaceMark, valueCap)

	validBitsSize := (valueCap + 7) / 8
	validBitsAlign := uint64(8)
	validBitsMark := allocationFn(validBitsSize, validBitsAlign)
	memcore.MemoryClearNoHeapPointers(memcore.MemcoreMarkDereference(validBitsMark), uintptr(validBitsSize))

	for i, id := range uniqueX {
		_ = memstruct.ArraySetAt[uint64](xKeySpaceMark, uint64(i), id)
	}
	for j, id := range uniqueY {
		_ = memstruct.ArraySetAt[uint64](yKeySpaceMark, uint64(j), id)
	}

	xIDToRow := make(map[uint64]uint64, len(uniqueX))
	for row, id := range uniqueX {
		xIDToRow[id] = uint64(row)
	}
	yIDToCol := make(map[uint64]uint64, len(uniqueY))
	for col, id := range uniqueY {
		yIDToCol[id] = uint64(col)
	}

	for _, c := range combinations {
		xID := xExtractor(c.XKey)
		yID := yExtractor(c.YKey)
		row, okX := xIDToRow[xID]
		if !okX {
			return nil, fmt.Errorf("DenseLocalGridCreateSparse: xKey not in key space (extractor non-deterministic or builder bug)")
		}
		col, okY := yIDToCol[yID]
		if !okY {
			return nil, fmt.Errorf("DenseLocalGridCreateSparse: yKey not in key space (extractor non-deterministic or builder bug)")
		}
		if duplicatePolicy == DenseLocalGridDuplicateError && denseLocalGridValidBitTest(validBitsMark, row, col, cols) {
			return nil, fmt.Errorf("DenseLocalGridCreateSparse: duplicate (xKey, yKey) at row=%d col=%d", row, col)
		}
		if duplicatePolicy == DenseLocalGridDuplicateKeepFirst && denseLocalGridValidBitTest(validBitsMark, row, col, cols) {
			continue
		}
		idx := row*cols + col
		_ = memstruct.ArraySetAt[TValue](valueSpaceMark, idx, c.Value)
		denseLocalGridValidBitSet(validBitsMark, row, col, cols)
	}

	return &DenseLocalGrid[TXKey, TYKey, TValue]{
		xKeySpace:    xKeySpaceMark,
		yKeySpace:    yKeySpaceMark,
		valueSpace:   valueSpaceMark,
		validBits:    validBitsMark,
		hasValidBits: true,
	}, nil
}

// denseLocalGridKeySpaceSearch returns the index of target in the sorted key-space array, or (0, false) if not found.
func denseLocalGridKeySpaceSearch(keySpace memcore.MarkRaw, capacity uint64, target uint64) (uint64, bool) {
	lo := uint64(0)
	hi := capacity
	for lo < hi {
		mid := lo + (hi-lo)/2
		v := memstruct.ArrayItemGetAtUnsafe[uint64](keySpace, mid)
		if v < target {
			lo = mid + 1
		} else if v > target {
			hi = mid
		} else {
			return mid, true
		}
	}
	return 0, false
}

func denseLocalGridValidBitSet(validBits memcore.MarkRaw, localRow, localCol, cols uint64) {
	idx := localRow*cols + localCol
	byteIdx := idx / 8
	bitIdx := idx % 8
	base := (*byte)(memcore.MemcoreMarkDereference(validBits))
	base = (*byte)(unsafe.Add(unsafe.Pointer(base), byteIdx))
	*base |= 1 << uint(bitIdx)
}

func denseLocalGridValidBitTest(validBits memcore.MarkRaw, localRow, localCol, cols uint64) bool {
	idx := localRow*cols + localCol
	byteIdx := idx / 8
	bitIdx := idx % 8
	base := (*byte)(memcore.MemcoreMarkDereference(validBits))
	base = (*byte)(unsafe.Add(unsafe.Pointer(base), byteIdx))
	return (*base & (1 << uint(bitIdx))) != 0
}

/*
DenseLocalGridLocalIndexXByID returns the local row index for xID, or (0, false) if not in the grid.
*/
func DenseLocalGridLocalIndexXByID[TXKey, TYKey, TValue any](grid *DenseLocalGrid[TXKey, TYKey, TValue], xID uint64) (localRow uint64, ok bool) {
	if grid == nil {
		return 0, false
	}
	capacity := memstruct.ArrayCapacityGet[uint64](grid.xKeySpace)
	if capacity == 0 {
		return 0, false
	}
	return denseLocalGridKeySpaceSearch(grid.xKeySpace, capacity, xID)
}

/*
DenseLocalGridLocalIndexYByID returns the local column index for yID, or (0, false) if not in the grid.
*/
func DenseLocalGridLocalIndexYByID[TXKey, TYKey, TValue any](grid *DenseLocalGrid[TXKey, TYKey, TValue], yID uint64) (localCol uint64, ok bool) {
	if grid == nil {
		return 0, false
	}
	capacity := memstruct.ArrayCapacityGet[uint64](grid.yKeySpace)
	if capacity == 0 {
		return 0, false
	}
	return denseLocalGridKeySpaceSearch(grid.yKeySpace, capacity, yID)
}

/*
DenseLocalGridLocalIndexXGet returns the local row index for xKey using the given extractor, or (0, false) if the key is not in the grid.
*/
func DenseLocalGridLocalIndexXGet[TXKey, TYKey, TValue any](grid *DenseLocalGrid[TXKey, TYKey, TValue], xExtractor IDExtractor[TXKey], xKey TXKey) (localRow uint64, ok bool) {
	return DenseLocalGridLocalIndexXByID(grid, xExtractor(xKey))
}

/*
DenseLocalGridLocalIndexYGet returns the local column index for yKey using the given extractor, or (0, false) if the key is not in the grid.
*/
func DenseLocalGridLocalIndexYGet[TXKey, TYKey, TValue any](grid *DenseLocalGrid[TXKey, TYKey, TValue], yExtractor IDExtractor[TYKey], yKey TYKey) (localCol uint64, ok bool) {
	return DenseLocalGridLocalIndexYByID(grid, yExtractor(yKey))
}

/*
DenseLocalGridValueGetByID returns the value at (xID, yID) and true if the cell exists and is valid (or grid is Full).
Returns (zero, false) if either ID is not in the grid or (Sparse grid) the cell was never set.
*/
func DenseLocalGridValueGetByID[TXKey, TYKey, TValue any](grid *DenseLocalGrid[TXKey, TYKey, TValue], xID, yID uint64) (TValue, bool) {
	var zero TValue
	if grid == nil {
		return zero, false
	}
	row, okX := DenseLocalGridLocalIndexXByID(grid, xID)
	if !okX {
		return zero, false
	}
	col, okY := DenseLocalGridLocalIndexYByID(grid, yID)
	if !okY {
		return zero, false
	}
	cols := DenseLocalGridColsGet(grid)
	if grid.hasValidBits && !denseLocalGridValidBitTest(grid.validBits, row, col, cols) {
		return zero, false
	}
	idx := row*cols + col
	val, err := memstruct.ArrayItemGetAt[TValue](grid.valueSpace, idx)
	if err != nil {
		return zero, false
	}
	return val, true
}

/*
DenseLocalGridValueSetByID writes value at (xID, yID). When the grid has a validity bitmap, the cell is marked valid.
*/
func DenseLocalGridValueSetByID[TXKey, TYKey, TValue any](grid *DenseLocalGrid[TXKey, TYKey, TValue], xID, yID uint64, value TValue) error {
	if grid == nil {
		return fmt.Errorf("DenseLocalGridValueSetByID: nil grid")
	}
	row, okX := DenseLocalGridLocalIndexXByID(grid, xID)
	if !okX {
		return fmt.Errorf("DenseLocalGridValueSetByID: xID not in grid")
	}
	col, okY := DenseLocalGridLocalIndexYByID(grid, yID)
	if !okY {
		return fmt.Errorf("DenseLocalGridValueSetByID: yID not in grid")
	}
	cols := DenseLocalGridColsGet(grid)
	idx := row*cols + col
	if err := memstruct.ArraySetAt[TValue](grid.valueSpace, idx, value); err != nil {
		return err
	}
	if grid.hasValidBits {
		denseLocalGridValidBitSet(grid.validBits, row, col, cols)
	}
	return nil
}

/*
DenseLocalGridRowsGet returns the number of rows (unique X keys). Returns 0 if grid is nil.
*/
func DenseLocalGridRowsGet[TXKey, TYKey, TValue any](grid *DenseLocalGrid[TXKey, TYKey, TValue]) uint64 {
	if grid == nil {
		return 0
	}
	return memstruct.ArrayCapacityGet[uint64](grid.xKeySpace)
}

/*
DenseLocalGridColsGet returns the number of columns (unique Y keys). Returns 0 if grid is nil.
*/
func DenseLocalGridColsGet[TXKey, TYKey, TValue any](grid *DenseLocalGrid[TXKey, TYKey, TValue]) uint64 {
	if grid == nil {
		return 0
	}
	return memstruct.ArrayCapacityGet[uint64](grid.yKeySpace)
}

/*
DenseLocalGridValueGet returns the value at (xKey, yKey) using the given extractors, and true if the cell exists and is valid.
*/
func DenseLocalGridValueGet[TXKey, TYKey, TValue any](grid *DenseLocalGrid[TXKey, TYKey, TValue], xExtractor IDExtractor[TXKey], yExtractor IDExtractor[TYKey], xKey TXKey, yKey TYKey) (TValue, bool) {
	return DenseLocalGridValueGetByID(grid, xExtractor(xKey), yExtractor(yKey))
}

/*
DenseLocalGridValueSet writes value at (xKey, yKey) using the given extractors.
*/
func DenseLocalGridValueSet[TXKey, TYKey, TValue any](grid *DenseLocalGrid[TXKey, TYKey, TValue], xExtractor IDExtractor[TXKey], yExtractor IDExtractor[TYKey], xKey TXKey, yKey TYKey, value TValue) error {
	return DenseLocalGridValueSetByID(grid, xExtractor(xKey), yExtractor(yKey), value)
}

/*
DenseLocalGridValueAtLocal returns the value at (localRow, localCol). Returns error if grid is nil, indices out of bounds, or (Sparse) cell not valid.
*/
func DenseLocalGridValueAtLocal[TXKey, TYKey, TValue any](grid *DenseLocalGrid[TXKey, TYKey, TValue], localRow, localCol uint64) (TValue, error) {
	var zero TValue
	if grid == nil {
		return zero, fmt.Errorf("DenseLocalGridValueAtLocal: nil grid")
	}
	rows := DenseLocalGridRowsGet(grid)
	cols := DenseLocalGridColsGet(grid)
	if localRow >= rows || localCol >= cols {
		return zero, fmt.Errorf("DenseLocalGridValueAtLocal: index out of bounds (row=%d, col=%d, grid=%dx%d)", localRow, localCol, rows, cols)
	}
	if grid.hasValidBits && !denseLocalGridValidBitTest(grid.validBits, localRow, localCol, cols) {
		return zero, fmt.Errorf("DenseLocalGridValueAtLocal: cell not set")
	}
	idx := localRow*cols + localCol
	return memstruct.ArrayItemGetAt[TValue](grid.valueSpace, idx)
}

/*
DenseLocalGridValueAtLocalUnsafe returns the value at (localRow, localCol) with no bounds or validity checks.
For Sparse grids, unset cells contain undefined storage: calling this without first checking DenseLocalGridCellValidAtLocal
may return garbage. Use only when the caller has guaranteed the cell is valid or accepts undefined behavior.
*/
func DenseLocalGridValueAtLocalUnsafe[TXKey, TYKey, TValue any](grid *DenseLocalGrid[TXKey, TYKey, TValue], localRow, localCol uint64) TValue {
	cols := DenseLocalGridColsGet(grid)
	idx := localRow*cols + localCol
	return memstruct.ArrayItemGetAtUnsafe[TValue](grid.valueSpace, idx)
}

/*
DenseLocalGridSetAtLocal writes value at (localRow, localCol). When the grid has a validity bitmap, the cell is marked valid.
*/
func DenseLocalGridSetAtLocal[TXKey, TYKey, TValue any](grid *DenseLocalGrid[TXKey, TYKey, TValue], localRow, localCol uint64, value TValue) error {
	if grid == nil {
		return fmt.Errorf("DenseLocalGridSetAtLocal: nil grid")
	}
	rows := DenseLocalGridRowsGet(grid)
	cols := DenseLocalGridColsGet(grid)
	if localRow >= rows || localCol >= cols {
		return fmt.Errorf("DenseLocalGridSetAtLocal: index out of bounds (row=%d, col=%d, grid=%dx%d)", localRow, localCol, rows, cols)
	}
	idx := localRow*cols + localCol
	if err := memstruct.ArraySetAt[TValue](grid.valueSpace, idx, value); err != nil {
		return err
	}
	if grid.hasValidBits {
		denseLocalGridValidBitSet(grid.validBits, localRow, localCol, cols)
	}
	return nil
}

/*
DenseLocalGridSetAtLocalUnsafe writes value at (localRow, localCol) with no bounds checks. When the grid has a validity bitmap, the cell is marked valid.
*/
func DenseLocalGridSetAtLocalUnsafe[TXKey, TYKey, TValue any](grid *DenseLocalGrid[TXKey, TYKey, TValue], localRow, localCol uint64, value TValue) {
	cols := DenseLocalGridColsGet(grid)
	idx := localRow*cols + localCol
	memstruct.ArraySetAtUnsafe[TValue](grid.valueSpace, idx, value)
	if grid.hasValidBits {
		denseLocalGridValidBitSet(grid.validBits, localRow, localCol, cols)
	}
}

/*
DenseLocalGridCellValidAtLocal returns whether the cell at (localRow, localCol) is valid (was set).
Full grids always return true for in-bounds indices; Sparse grids return true only when the cell was written.
*/
func DenseLocalGridCellValidAtLocal[TXKey, TYKey, TValue any](grid *DenseLocalGrid[TXKey, TYKey, TValue], localRow, localCol uint64) bool {
	if grid == nil {
		return false
	}
	rows := DenseLocalGridRowsGet(grid)
	cols := DenseLocalGridColsGet(grid)
	if localRow >= rows || localCol >= cols {
		return false
	}
	if !grid.hasValidBits {
		return true
	}
	return denseLocalGridValidBitTest(grid.validBits, localRow, localCol, cols)
}

/*
DenseLocalGridCellValidByID returns whether the cell at (xID, yID) is valid (was set).
*/
func DenseLocalGridCellValidByID[TXKey, TYKey, TValue any](grid *DenseLocalGrid[TXKey, TYKey, TValue], xID, yID uint64) bool {
	row, okX := DenseLocalGridLocalIndexXByID(grid, xID)
	if !okX {
		return false
	}
	col, okY := DenseLocalGridLocalIndexYByID(grid, yID)
	if !okY {
		return false
	}
	return DenseLocalGridCellValidAtLocal(grid, row, col)
}

/*
DenseLocalGridCellValidGet returns whether the cell at (xKey, yKey) is valid using the given extractors.
*/
func DenseLocalGridCellValidGet[TXKey, TYKey, TValue any](grid *DenseLocalGrid[TXKey, TYKey, TValue], xExtractor IDExtractor[TXKey], yExtractor IDExtractor[TYKey], xKey TXKey, yKey TYKey) bool {
	return DenseLocalGridCellValidByID(grid, xExtractor(xKey), yExtractor(yKey))
}

/*
DenseLocalGridXKeySpaceMarkGet returns the mark for the X key-space array (memstruct.Array[uint64], sorted by extracted ID).
Returns zero MarkRaw if grid is nil.
*/
func DenseLocalGridXKeySpaceMarkGet[TXKey, TYKey, TValue any](grid *DenseLocalGrid[TXKey, TYKey, TValue]) memcore.MarkRaw {
	if grid == nil {
		return memcore.MarkRaw{}
	}
	return grid.xKeySpace
}

/*
DenseLocalGridYKeySpaceMarkGet returns the mark for the Y key-space array (memstruct.Array[uint64], sorted by extracted ID).
Returns zero MarkRaw if grid is nil.
*/
func DenseLocalGridYKeySpaceMarkGet[TXKey, TYKey, TValue any](grid *DenseLocalGrid[TXKey, TYKey, TValue]) memcore.MarkRaw {
	if grid == nil {
		return memcore.MarkRaw{}
	}
	return grid.yKeySpace
}

/*
DenseLocalGridValueSpaceMarkGet returns the mark for the value matrix (memstruct.Array[TValue], row-major, rows*cols elements).
Returns zero MarkRaw if grid is nil.
*/
func DenseLocalGridValueSpaceMarkGet[TXKey, TYKey, TValue any](grid *DenseLocalGrid[TXKey, TYKey, TValue]) memcore.MarkRaw {
	if grid == nil {
		return memcore.MarkRaw{}
	}
	return grid.valueSpace
}
