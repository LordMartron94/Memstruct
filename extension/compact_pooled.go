package extension

import (
	"fmt"
	"memcore"
	"sort"
	"unsafe"
)

/*
DenseLocalGridDescriptor describes a single logical grid's view into a DenseLocalGridPool.
Offsets and lengths index into the pool's raw regions (bytes, not element counts).
Field order: largest (uint64) first, bool last for minimal padding.
*/
type DenseLocalGridDescriptor struct {
	xOff         uint64
	xLen         uint64
	yOff         uint64
	yLen         uint64
	valOff       uint64
	valLen       uint64
	bitOff       uint64
	bitLen       uint64
	hasValidBits bool
}

/*
DenseLocalGridPool holds four global pools (X keys, Y keys, values, validity bitmap)
and a slice of descriptors. Each descriptor defines one logical grid as a segment
of those pools. Logically many grids; physically one allocation per pool.
*/
type DenseLocalGridPool[TXKey, TYKey, TValue any] struct {
	poolX       memcore.MarkRaw // raw uint64 data, all X keyspaces concatenated
	poolY       memcore.MarkRaw // raw uint64 data, all Y keyspaces concatenated
	poolValues  memcore.MarkRaw // raw TValue data, all value matrices concatenated
	poolBits    memcore.MarkRaw // raw byte data, all validity bitmaps concatenated
	descriptors []DenseLocalGridDescriptor
}

/*
DenseLocalGridPoolGridSpec specifies one grid to be built into a DenseLocalGridPool:
combinations, whether it uses a validity bitmap (Sparse), and duplicate policy.
*/
type DenseLocalGridPoolGridSpec[TXKey, TYKey, TValue any] struct {
	Combinations    []Combination[TXKey, TYKey, TValue]
	Sparse          bool
	DuplicatePolicy DenseLocalGridDuplicatePolicy
}

/*
DenseLocalGridPoolBuild builds a DenseLocalGridPool from a list of grid specs.
One allocation per pool is performed; each spec becomes one descriptor and one logical grid.
Returns the pool and nil, or nil and an error if sizing or duplicate policy fails.

Prerequisites:
- allocationFn must return valid marks for the requested sizes and alignments.
- xExtractor and yExtractor must be deterministic and consistent for each spec's combinations.

Edge cases:
- Empty specs returns a non-nil pool with zero grids and no pool allocations.
- Sparse grids use a validity bitmap; Full grids do not.
- DuplicatePolicy is applied per grid independently.
*/
func DenseLocalGridPoolBuild[TXKey, TYKey, TValue any](
	allocationFn AllocationFn,
	xExtractor IDExtractor[TXKey],
	yExtractor IDExtractor[TYKey],
	specs []DenseLocalGridPoolGridSpec[TXKey, TYKey, TValue],
) (*DenseLocalGridPool[TXKey, TYKey, TValue], error) {
	if len(specs) == 0 {
		return &DenseLocalGridPool[TXKey, TYKey, TValue]{
			descriptors: nil,
		}, nil
	}

	var totalX, totalY, totalVal, totalBits uint64
	descriptors := make([]DenseLocalGridDescriptor, 0, len(specs))

	for si, spec := range specs {
		xIDSet := make(map[uint64]struct{})
		yIDSet := make(map[uint64]struct{})
		for _, c := range spec.Combinations {
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
			return nil, fmt.Errorf("DenseLocalGridPoolBuild: overflow rows*cols at grid %d", si)
		}
		bitLen := uint64(0)
		if spec.Sparse {
			bitLen = (valueCap + 7) / 8
		}

		desc := DenseLocalGridDescriptor{
			xOff:         totalX,
			xLen:         rows,
			yOff:         totalY,
			yLen:         cols,
			valOff:       totalVal,
			valLen:       valueCap,
			bitOff:       totalBits,
			bitLen:       bitLen,
			hasValidBits: spec.Sparse,
		}
		descriptors = append(descriptors, desc)

		totalX += rows
		totalY += cols
		totalVal += valueCap
		totalBits += bitLen
	}

	poolXBytes := totalX * 8
	poolYBytes := totalY * 8
	valueSize := memcore.SizeOf[TValue]()
	poolValBytes := totalVal * valueSize
	poolBitsAlign := uint64(8)
	poolValAlign := memcore.AlignOf[TValue]()
	if poolValAlign < 8 {
		poolValAlign = 8
	}

	poolXMark := allocationFn(poolXBytes, 8)
	poolYMark := allocationFn(poolYBytes, 8)
	poolValMark := allocationFn(poolValBytes, poolValAlign)
	var poolBitsMark memcore.MarkRaw
	if totalBits > 0 {
		poolBitsMark = allocationFn(totalBits, poolBitsAlign)
		memcore.MemoryClearNoHeapPointers(memcore.MemcoreMarkDereference(poolBitsMark), uintptr(totalBits))
	}

	cursorX := uint64(0)
	cursorY := uint64(0)
	cursorVal := uint64(0)
	cursorBit := uint64(0)

	for si, spec := range specs {
		desc := &descriptors[si]
		xIDSet := make(map[uint64]struct{})
		yIDSet := make(map[uint64]struct{})
		for _, c := range spec.Combinations {
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
		rows := desc.xLen
		cols := desc.yLen
		valueCap := rows * cols

		baseX := memcore.MemcoreMarkDereference(poolXMark)
		for i, id := range uniqueX {
			*denseLocalGridPooledU64PtrAt(baseX, cursorX+uint64(i)) = id
		}
		baseY := memcore.MemcoreMarkDereference(poolYMark)
		for j, id := range uniqueY {
			*denseLocalGridPooledU64PtrAt(baseY, cursorY+uint64(j)) = id
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
		if spec.DuplicatePolicy == DenseLocalGridDuplicateError || spec.DuplicatePolicy == DenseLocalGridDuplicateKeepFirst {
			writtenBits = make([]byte, (valueCap+7)/8)
		}

		for _, c := range spec.Combinations {
			xID := xExtractor(c.XKey)
			yID := yExtractor(c.YKey)
			row, okX := xIDToRow[xID]
			if !okX {
				return nil, fmt.Errorf("DenseLocalGridPoolBuild: xKey not in key space at grid %d", si)
			}
			col, okY := yIDToCol[yID]
			if !okY {
				return nil, fmt.Errorf("DenseLocalGridPoolBuild: yKey not in key space at grid %d", si)
			}
			idx := denseLocalGridPooledCellIndex(cols, row, col)
			if writtenBits != nil {
				byteIdx := idx / 8
				bitIdx := idx % 8
				mask := byte(1 << uint(bitIdx))
				if (writtenBits[byteIdx] & mask) != 0 {
					if spec.DuplicatePolicy == DenseLocalGridDuplicateError {
						return nil, fmt.Errorf("DenseLocalGridPoolBuild: duplicate (xKey, yKey) at grid %d row=%d col=%d", si, row, col)
					}
					continue
				}
				writtenBits[byteIdx] |= mask
			}
			valueIndex := denseLocalGridPooledValueIndex(cursorVal, cols, row, col)
			ptr := denseLocalGridPooledValuePtrAt[TValue](poolValMark, valueIndex)
			*ptr = c.Value
			if spec.Sparse {
				denseLocalGridPooledValidBitSet(poolBitsMark, uintptr(cursorBit), row, col, cols)
			}
		}

		if spec.Sparse {
			cursorBit += (valueCap + 7) / 8
		}

		cursorX += rows
		cursorY += cols
		cursorVal += valueCap
	}

	pool := &DenseLocalGridPool[TXKey, TYKey, TValue]{
		poolX:       poolXMark,
		poolY:       poolYMark,
		poolValues:  poolValMark,
		poolBits:    poolBitsMark,
		descriptors: descriptors,
	}
	return pool, nil
}

/*
DenseLocalGridPoolNumGrids returns the number of logical grids (descriptors) in the pool.
Returns 0 if pool is nil.
*/
func DenseLocalGridPoolNumGrids[TXKey, TYKey, TValue any](pool *DenseLocalGridPool[TXKey, TYKey, TValue]) uint64 {
	if pool == nil {
		return 0
	}
	return uint64(len(pool.descriptors))
}

/*
DenseLocalGridPoolDescriptorAt returns a pointer to the descriptor at the given index, or nil if pool is nil or index is out of range.
The returned pointer is valid for the lifetime of the pool; do not modify the descriptor's fields.
*/
func DenseLocalGridPoolDescriptorAt[TXKey, TYKey, TValue any](pool *DenseLocalGridPool[TXKey, TYKey, TValue], index uint64) *DenseLocalGridDescriptor {
	if pool == nil || index >= uint64(len(pool.descriptors)) {
		return nil
	}
	return &pool.descriptors[index]
}

/*
DenseLocalGridPooledLocalIndexXByID returns the local row index for xID in the grid described by desc, or (0, false) if not in the grid.
*/
func DenseLocalGridPooledLocalIndexXByID[TXKey, TYKey, TValue any](
	pool *DenseLocalGridPool[TXKey, TYKey, TValue],
	desc *DenseLocalGridDescriptor,
	xID uint64,
) (localRow uint64, ok bool) {
	if pool == nil || desc == nil || desc.xLen == 0 {
		return 0, false
	}
	return denseLocalGridPooledKeySpaceSearch(pool.poolX, desc.xOff, desc.xLen, xID)
}

/*
DenseLocalGridPooledLocalIndexYByID returns the local column index for yID in the grid described by desc, or (0, false) if not in the grid.
*/
func DenseLocalGridPooledLocalIndexYByID[TXKey, TYKey, TValue any](
	pool *DenseLocalGridPool[TXKey, TYKey, TValue],
	desc *DenseLocalGridDescriptor,
	yID uint64,
) (localCol uint64, ok bool) {
	if pool == nil || desc == nil || desc.yLen == 0 {
		return 0, false
	}
	return denseLocalGridPooledKeySpaceSearch(pool.poolY, desc.yOff, desc.yLen, yID)
}

/*
DenseLocalGridPooledLocalIndexXGet returns the local row index for xKey using the given extractor.
*/
func DenseLocalGridPooledLocalIndexXGet[TXKey, TYKey, TValue any](
	pool *DenseLocalGridPool[TXKey, TYKey, TValue],
	desc *DenseLocalGridDescriptor,
	xExtractor IDExtractor[TXKey],
	xKey TXKey,
) (localRow uint64, ok bool) {
	return DenseLocalGridPooledLocalIndexXByID(pool, desc, xExtractor(xKey))
}

/*
DenseLocalGridPooledLocalIndexYGet returns the local column index for yKey using the given extractor.
*/
func DenseLocalGridPooledLocalIndexYGet[TXKey, TYKey, TValue any](
	pool *DenseLocalGridPool[TXKey, TYKey, TValue],
	desc *DenseLocalGridDescriptor,
	yExtractor IDExtractor[TYKey],
	yKey TYKey,
) (localCol uint64, ok bool) {
	return DenseLocalGridPooledLocalIndexYByID(pool, desc, yExtractor(yKey))
}

/*
DenseLocalGridPooledRowsGet returns the number of rows (unique X keys) for the grid described by desc.
Returns 0 if desc is nil.
*/
func DenseLocalGridPooledRowsGet[TXKey, TYKey, TValue any](_ *DenseLocalGridPool[TXKey, TYKey, TValue], desc *DenseLocalGridDescriptor) uint64 {
	if desc == nil {
		return 0
	}
	return desc.xLen
}

/*
DenseLocalGridPooledXKeyAtLocal returns the X key (as uint64 ID) at the given local row index.
Returns (0, false) if pool or desc is nil or localRow is out of range.
*/
func DenseLocalGridPooledXKeyAtLocal[TXKey, TYKey, TValue any](
	pool *DenseLocalGridPool[TXKey, TYKey, TValue],
	desc *DenseLocalGridDescriptor,
	localRow uint64,
) (uint64, bool) {
	if pool == nil || desc == nil || localRow >= desc.xLen {
		return 0, false
	}
	base := memcore.MemcoreMarkDereference(pool.poolX)
	ptr := denseLocalGridPooledU64PtrAt(base, desc.xOff+localRow)
	return *ptr, true
}

/*
DenseLocalGridPooledYKeyAtLocal returns the Y key (as uint64 ID) at the given local column index.
Returns (0, false) if pool or desc is nil or localCol is out of range.
*/
func DenseLocalGridPooledYKeyAtLocal[TXKey, TYKey, TValue any](
	pool *DenseLocalGridPool[TXKey, TYKey, TValue],
	desc *DenseLocalGridDescriptor,
	localCol uint64,
) (uint64, bool) {
	if pool == nil || desc == nil || localCol >= desc.yLen {
		return 0, false
	}
	base := memcore.MemcoreMarkDereference(pool.poolY)
	ptr := denseLocalGridPooledU64PtrAt(base, desc.yOff+localCol)
	return *ptr, true
}

/*
DenseLocalGridPooledIterateValid calls fn for each valid cell in the grid described by desc.
For Sparse grids only cells that were set are visited; for Full grids all (row,col) pairs are visited.
Keys and value are passed as (xID, yID, value). If fn returns false, iteration stops.
Order is row-major: localRow 0..rows-1, for each localCol 0..cols-1 (when valid).
*/
func DenseLocalGridPooledIterateValid[TXKey, TYKey, TValue any](
	pool *DenseLocalGridPool[TXKey, TYKey, TValue],
	desc *DenseLocalGridDescriptor,
	fn func(xID, yID uint64, value TValue) bool,
) {
	if pool == nil || desc == nil || fn == nil {
		return
	}
	rows := desc.xLen
	cols := desc.yLen
	if rows == 0 || cols == 0 {
		return
	}
	for localRow := uint64(0); localRow < rows; localRow++ {
		xID, okX := DenseLocalGridPooledXKeyAtLocal(pool, desc, localRow)
		if !okX {
			continue
		}
		for localCol := uint64(0); localCol < cols; localCol++ {
			if desc.hasValidBits && !denseLocalGridPooledValidBitTest(pool.poolBits, uintptr(desc.bitOff), localRow, localCol, cols) {
				continue
			}
			yID, okY := DenseLocalGridPooledYKeyAtLocal(pool, desc, localCol)
			if !okY {
				continue
			}
			valueIndex := denseLocalGridPooledValueIndex(desc.valOff, cols, localRow, localCol)
			ptr := denseLocalGridPooledValuePtrAt[TValue](pool.poolValues, valueIndex)
			if !fn(xID, yID, *ptr) {
				return
			}
		}
	}
}

/*
DenseLocalGridPooledColsGet returns the number of columns (unique Y keys) for the grid described by desc.
Returns 0 if desc is nil.
*/
func DenseLocalGridPooledColsGet[TXKey, TYKey, TValue any](_ *DenseLocalGridPool[TXKey, TYKey, TValue], desc *DenseLocalGridDescriptor) uint64 {
	if desc == nil {
		return 0
	}
	return desc.yLen
}

/*
DenseLocalGridPooledValueGetByID returns the value at (xID, yID) and true if the cell exists and is valid (or grid is Full).
Returns (zero, false) if either ID is not in the grid or (Sparse) the cell was never set.
*/
func DenseLocalGridPooledValueGetByID[TXKey, TYKey, TValue any](
	pool *DenseLocalGridPool[TXKey, TYKey, TValue],
	desc *DenseLocalGridDescriptor,
	xID, yID uint64,
) (TValue, bool) {
	var zero TValue
	if pool == nil || desc == nil {
		return zero, false
	}
	row, okX := DenseLocalGridPooledLocalIndexXByID(pool, desc, xID)
	if !okX {
		return zero, false
	}
	col, okY := DenseLocalGridPooledLocalIndexYByID(pool, desc, yID)
	if !okY {
		return zero, false
	}
	cols := desc.yLen
	if desc.hasValidBits && !denseLocalGridPooledValidBitTest(pool.poolBits, uintptr(desc.bitOff), row, col, cols) {
		return zero, false
	}
	valueIndex := denseLocalGridPooledValueIndex(desc.valOff, cols, row, col)
	ptr := denseLocalGridPooledValuePtrAt[TValue](pool.poolValues, valueIndex)
	return *ptr, true
}

/*
DenseLocalGridPooledValueSetByID writes value at (xID, yID). When the grid has a validity bitmap, the cell is marked valid.
*/
func DenseLocalGridPooledValueSetByID[TXKey, TYKey, TValue any](
	pool *DenseLocalGridPool[TXKey, TYKey, TValue],
	desc *DenseLocalGridDescriptor,
	xID, yID uint64,
	value TValue,
) error {
	if pool == nil || desc == nil {
		return fmt.Errorf("DenseLocalGridPooledValueSetByID: nil pool or descriptor")
	}
	row, okX := DenseLocalGridPooledLocalIndexXByID(pool, desc, xID)
	if !okX {
		return fmt.Errorf("DenseLocalGridPooledValueSetByID: xID not in grid")
	}
	col, okY := DenseLocalGridPooledLocalIndexYByID(pool, desc, yID)
	if !okY {
		return fmt.Errorf("DenseLocalGridPooledValueSetByID: yID not in grid")
	}
	cols := desc.yLen
	valueIndex := denseLocalGridPooledValueIndex(desc.valOff, cols, row, col)
	ptr := denseLocalGridPooledValuePtrAt[TValue](pool.poolValues, valueIndex)
	*ptr = value
	if desc.hasValidBits {
		denseLocalGridPooledValidBitSet(pool.poolBits, uintptr(desc.bitOff), row, col, cols)
	}
	return nil
}

/*
DenseLocalGridPooledValueGet returns the value at (xKey, yKey) using the given extractors.
*/
func DenseLocalGridPooledValueGet[TXKey, TYKey, TValue any](
	pool *DenseLocalGridPool[TXKey, TYKey, TValue],
	desc *DenseLocalGridDescriptor,
	xExtractor IDExtractor[TXKey],
	yExtractor IDExtractor[TYKey],
	xKey TXKey,
	yKey TYKey,
) (TValue, bool) {
	return DenseLocalGridPooledValueGetByID(pool, desc, xExtractor(xKey), yExtractor(yKey))
}

/*
DenseLocalGridPooledValueSet writes value at (xKey, yKey) using the given extractors.
*/
func DenseLocalGridPooledValueSet[TXKey, TYKey, TValue any](
	pool *DenseLocalGridPool[TXKey, TYKey, TValue],
	desc *DenseLocalGridDescriptor,
	xExtractor IDExtractor[TXKey],
	yExtractor IDExtractor[TYKey],
	xKey TXKey,
	yKey TYKey,
	value TValue,
) error {
	return DenseLocalGridPooledValueSetByID(pool, desc, xExtractor(xKey), yExtractor(yKey), value)
}

/*
DenseLocalGridPooledValueAtLocal returns the value at (localRow, localCol). Returns error if indices out of bounds or (Sparse) cell not valid.
*/
func DenseLocalGridPooledValueAtLocal[TXKey, TYKey, TValue any](
	pool *DenseLocalGridPool[TXKey, TYKey, TValue],
	desc *DenseLocalGridDescriptor,
	localRow, localCol uint64,
) (TValue, error) {
	var zero TValue
	if pool == nil || desc == nil {
		return zero, fmt.Errorf("DenseLocalGridPooledValueAtLocal: nil pool or descriptor")
	}
	if err := denseLocalGridPooledGuaranteeLocalBounds(desc, localRow, localCol); err != nil {
		return zero, fmt.Errorf("DenseLocalGridPooledValueAtLocal: %w", err)
	}
	cols := desc.yLen
	if desc.hasValidBits && !denseLocalGridPooledValidBitTest(pool.poolBits, uintptr(desc.bitOff), localRow, localCol, cols) {
		return zero, fmt.Errorf("DenseLocalGridPooledValueAtLocal: cell not set")
	}
	valueIndex := denseLocalGridPooledValueIndex(desc.valOff, cols, localRow, localCol)
	ptr := denseLocalGridPooledValuePtrAt[TValue](pool.poolValues, valueIndex)
	return *ptr, nil
}

/*
DenseLocalGridPooledValueAtLocalUnsafe returns the value at (localRow, localCol) with no bounds or validity checks.
For Sparse grids, unset cells contain undefined storage.
*/
func DenseLocalGridPooledValueAtLocalUnsafe[TXKey, TYKey, TValue any](
	pool *DenseLocalGridPool[TXKey, TYKey, TValue],
	desc *DenseLocalGridDescriptor,
	localRow, localCol uint64,
) TValue {
	cols := desc.yLen
	valueIndex := denseLocalGridPooledValueIndex(desc.valOff, cols, localRow, localCol)
	ptr := denseLocalGridPooledValuePtrAt[TValue](pool.poolValues, valueIndex)
	return *ptr
}

/*
DenseLocalGridPooledSetAtLocal writes value at (localRow, localCol). When the grid has a validity bitmap, the cell is marked valid.
*/
func DenseLocalGridPooledSetAtLocal[TXKey, TYKey, TValue any](
	pool *DenseLocalGridPool[TXKey, TYKey, TValue],
	desc *DenseLocalGridDescriptor,
	localRow, localCol uint64,
	value TValue,
) error {
	if pool == nil || desc == nil {
		return fmt.Errorf("DenseLocalGridPooledSetAtLocal: nil pool or descriptor")
	}
	if err := denseLocalGridPooledGuaranteeLocalBounds(desc, localRow, localCol); err != nil {
		return fmt.Errorf("DenseLocalGridPooledSetAtLocal: %w", err)
	}
	cols := desc.yLen
	valueIndex := denseLocalGridPooledValueIndex(desc.valOff, cols, localRow, localCol)
	ptr := denseLocalGridPooledValuePtrAt[TValue](pool.poolValues, valueIndex)
	*ptr = value
	if desc.hasValidBits {
		denseLocalGridPooledValidBitSet(pool.poolBits, uintptr(desc.bitOff), localRow, localCol, cols)
	}
	return nil
}

/*
DenseLocalGridPooledSetAtLocalUnsafe writes value at (localRow, localCol) with no bounds checks.
*/
func DenseLocalGridPooledSetAtLocalUnsafe[TXKey, TYKey, TValue any](
	pool *DenseLocalGridPool[TXKey, TYKey, TValue],
	desc *DenseLocalGridDescriptor,
	localRow, localCol uint64,
	value TValue,
) {
	cols := desc.yLen
	valueIndex := denseLocalGridPooledValueIndex(desc.valOff, cols, localRow, localCol)
	ptr := denseLocalGridPooledValuePtrAt[TValue](pool.poolValues, valueIndex)
	*ptr = value
	if desc.hasValidBits {
		denseLocalGridPooledValidBitSet(pool.poolBits, uintptr(desc.bitOff), localRow, localCol, cols)
	}
}

/*
DenseLocalGridPooledCellValidAtLocal returns whether the cell at (localRow, localCol) is valid (was set).
Full grids always return true for in-bounds indices; Sparse grids return true only when the cell was written.
*/
func DenseLocalGridPooledCellValidAtLocal[TXKey, TYKey, TValue any](
	pool *DenseLocalGridPool[TXKey, TYKey, TValue],
	desc *DenseLocalGridDescriptor,
	localRow, localCol uint64,
) bool {
	if desc == nil {
		return false
	}
	if err := denseLocalGridPooledGuaranteeLocalBounds(desc, localRow, localCol); err != nil {
		return false
	}
	if !desc.hasValidBits {
		return true
	}
	if pool == nil {
		return false
	}
	return denseLocalGridPooledValidBitTest(pool.poolBits, uintptr(desc.bitOff), localRow, localCol, desc.yLen)
}

/*
DenseLocalGridPooledCellValidByID returns whether the cell at (xID, yID) is valid (was set).
*/
func DenseLocalGridPooledCellValidByID[TXKey, TYKey, TValue any](
	pool *DenseLocalGridPool[TXKey, TYKey, TValue],
	desc *DenseLocalGridDescriptor,
	xID, yID uint64,
) bool {
	row, okX := DenseLocalGridPooledLocalIndexXByID(pool, desc, xID)
	if !okX {
		return false
	}
	col, okY := DenseLocalGridPooledLocalIndexYByID(pool, desc, yID)
	if !okY {
		return false
	}
	return DenseLocalGridPooledCellValidAtLocal(pool, desc, row, col)
}

/*
DenseLocalGridPooledCellValidGet returns whether the cell at (xKey, yKey) is valid using the given extractors.
*/
func DenseLocalGridPooledCellValidGet[TXKey, TYKey, TValue any](
	pool *DenseLocalGridPool[TXKey, TYKey, TValue],
	desc *DenseLocalGridDescriptor,
	xExtractor IDExtractor[TXKey],
	yExtractor IDExtractor[TYKey],
	xKey TXKey,
	yKey TYKey,
) bool {
	return DenseLocalGridPooledCellValidByID(pool, desc, xExtractor(xKey), yExtractor(yKey))
}

// denseLocalGridPooledKeySpaceSearch returns the local index of target in the sorted
// uint64 segment [segmentOff, segmentOff+length) in the pool, or (0, false) if not found.
func denseLocalGridPooledKeySpaceSearch(poolMark memcore.MarkRaw, segmentOff, length uint64, target uint64) (uint64, bool) {
	if length == 0 {
		return 0, false
	}
	base := memcore.MemcoreMarkDereference(poolMark)
	lo := uint64(0)
	hi := length
	for lo < hi {
		mid := lo + (hi-lo)/2
		ptr := denseLocalGridPooledU64PtrAt(base, segmentOff+mid)
		v := *ptr
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

// denseLocalGridPooledCellIndex returns the linear cell index for (localRow, localCol) in a grid with cols columns.
// Single choke point for row-major index: localRow*cols + localCol.
//
//go:inline
func denseLocalGridPooledCellIndex(cols, localRow, localCol uint64) uint64 {
	return localRow*cols + localCol
}

// denseLocalGridPooledValueIndex returns the pool-relative value index for the cell at (localRow, localCol).
// Equals valOff + denseLocalGridPooledCellIndex(cols, localRow, localCol).
//
//go:inline
func denseLocalGridPooledValueIndex(valOff, cols, localRow, localCol uint64) uint64 {
	return valOff + denseLocalGridPooledCellIndex(cols, localRow, localCol)
}

// denseLocalGridPooledU64PtrAt returns a pointer to the uint64 at element index i in the pool (base is pool start).
//
//go:inline
func denseLocalGridPooledU64PtrAt(base unsafe.Pointer, i uint64) *uint64 {
	return (*uint64)(unsafe.Add(base, i*8))
}

// denseLocalGridPooledValuePtrAt returns a pointer to the T at valueIndex in the values pool.
// Single choke point for value pool indexing.
//
//go:inline
func denseLocalGridPooledValuePtrAt[T any](poolValues memcore.MarkRaw, valueIndex uint64) *T {
	base := memcore.MemcoreMarkDereference(poolValues)
	itemSize := memcore.SizeOf[T]()
	return (*T)(unsafe.Add(base, uintptr(valueIndex*itemSize)))
}

// denseLocalGridPooledGuaranteeLocalBounds returns an error if (localRow, localCol) is out of bounds for desc.
//
//go:inline
func denseLocalGridPooledGuaranteeLocalBounds(desc *DenseLocalGridDescriptor, localRow, localCol uint64) error {
	if desc == nil {
		return fmt.Errorf("nil descriptor")
	}
	rows := desc.xLen
	cols := desc.yLen
	if localRow >= rows || localCol >= cols {
		return fmt.Errorf("index out of bounds (row=%d, col=%d, grid=%dx%d)", localRow, localCol, rows, cols)
	}
	return nil
}

func denseLocalGridPooledValidBitSet(poolBits memcore.MarkRaw, bitOff uintptr, localRow, localCol, cols uint64) {
	idx := denseLocalGridPooledCellIndex(cols, localRow, localCol)
	byteIdx := bitOff + uintptr(idx/8)
	bitIdx := idx % 8
	base := (*byte)(unsafe.Add(memcore.MemcoreMarkDereference(poolBits), byteIdx))
	*base |= 1 << uint(bitIdx)
}

func denseLocalGridPooledValidBitTest(poolBits memcore.MarkRaw, bitOff uintptr, localRow, localCol, cols uint64) bool {
	idx := denseLocalGridPooledCellIndex(cols, localRow, localCol)
	byteIdx := bitOff + uintptr(idx/8)
	bitIdx := idx % 8
	base := (*byte)(unsafe.Add(memcore.MemcoreMarkDereference(poolBits), byteIdx))
	return (*base & (1 << uint(bitIdx))) != 0
}
