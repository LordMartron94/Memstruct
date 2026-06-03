package memstruct

import (
	"memcore"
	"testing"
	"unsafe"
)

func TestArrayItemGetAtMatchesFast(t *testing.T) {
	mem := make([]byte, 4096)
	region := memcore.MemcoreRegionRegister(uintptr(unsafe.Pointer(&mem[0])), uint64(len(mem)))
	defer memcore.MemcoreRegionUnregister(region)

	headerMark := memcore.MemcoreMarkCreate(region, 0)
	dataMark := memcore.MemcoreMarkCreate(region, 256)
	ArrayInitializeWithSeparatedHeaderAndData[int32](headerMark, dataMark, 4)

	for i := uint64(0); i < 4; i++ {
		if err := ArraySetAt[int32](headerMark, i, int32(i*10)); err != nil {
			t.Fatal(err)
		}
	}

	base, inst := memcore.MemcoreMarkDereferenceObjectAltUnsafe[Array[int32]](headerMark)
	for i := uint64(0); i < 4; i++ {
		gotMark, err := ArrayItemGetAt[int32](headerMark, i)
		if err != nil {
			t.Fatal(err)
		}
		gotFast, err := ArrayItemGetAtFast(inst, base, i)
		if err != nil {
			t.Fatal(err)
		}
		if gotMark != gotFast {
			t.Fatalf("idx %d: mark=%d fast=%d", i, gotMark, gotFast)
		}
	}
}

func TestArrayDataPtrGetFast(t *testing.T) {
	mem := make([]byte, 4096)
	region := memcore.MemcoreRegionRegister(uintptr(unsafe.Pointer(&mem[0])), uint64(len(mem)))
	defer memcore.MemcoreRegionUnregister(region)

	headerMark := memcore.MemcoreMarkCreate(region, 0)
	dataMark := memcore.MemcoreMarkCreate(region, 256)
	ArrayInitializeWithSeparatedHeaderAndData[uint8](headerMark, dataMark, 8)

	base, inst := memcore.MemcoreMarkDereferenceObjectAltUnsafe[Array[uint8]](headerMark)
	pMark := ArrayDataPtrGet[uint8](headerMark)
	pFast := ArrayDataPtrGetFast(inst, base)
	if pMark != pFast {
		t.Fatalf("data ptr mismatch: %p vs %p", pMark, pFast)
	}
}
