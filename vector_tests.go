package memstruct

import (
	"fmt"
	"foundation"
	foundationtesting "foundation/testing"
	"math/rand"
	"memcore"
	"sort"
	"testing"
)

type allocFn[T foundation.Numeric] func(capacity uint64) memcore.MarkRaw

func TestVectorSort(t *testing.T, alloc allocFn[int32]) {
	testSuite(t, alloc)
}

func testSuite(t *testing.T, alloc allocFn[int32]) {
	tests := []struct {
		name     string
		input    []int32
		expected []int32
	}{
		{
			name:     "Already Sorted",
			input:    []int32{1, 2, 3, 4, 5},
			expected: []int32{1, 2, 3, 4, 5},
		},
		{
			name:     "Reverse Sorted",
			input:    []int32{5, 4, 3, 2, 1},
			expected: []int32{1, 2, 3, 4, 5},
		},
		{
			name:     "All Identical",
			input:    []int32{7, 7, 7, 7, 7},
			expected: []int32{7, 7, 7, 7, 7},
		},
		{
			name:     "Negative and Positive",
			input:    []int32{10, -5, 20, 0, -100, 50},
			expected: []int32{-100, -5, 0, 10, 20, 50},
		},
		{
			name:     "Single Element",
			input:    []int32{42},
			expected: []int32{42},
		},
		{
			name:     "Empty",
			input:    []int32{},
			expected: []int32{},
		},
		{
			name:     "Unsorted Small",
			input:    []int32{15, 2, 88, 1, 9, 44, 3, 7, 10, 5, 2},
			expected: []int32{1, 2, 2, 3, 5, 7, 9, 10, 15, 44, 88},
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			capacity := uint64(len(tc.input))
			vector := alloc(capacity)

			if capacity > 0 {
				for i, val := range tc.input {
					VectorSetAtUnsafe(vector, uint64(i), val)
				}
			}

			VectorSort[int32](vector, int32Comparator)

			for i := uint64(0); i < capacity; i++ {
				actual := VectorItemGetAtUnsafe[int32](vector, i)
				expected := tc.expected[i]

				foundationtesting.Assert(
					actual == expected,
					fmt.Sprintf("%s: At index %d, expected %d but got %d", tc.name, i, expected, actual),
					fmt.Sprintf("%s: Index %d correct", tc.name, i),
					t,
				)
			}
		})
	}

	t.Run("Large Random Stress Test", func(t *testing.T) {
		const size = 10000
		input := make([]int32, size)
		expected := make([]int32, size)

		for i := 0; i < size; i++ {
			val := int32(rand.Int31n(2000000) - 1000000)
			input[i] = val
			expected[i] = val
		}

		// Sort the Go slice to use as ground truth
		sort.Slice(expected, func(i, j int) bool {
			return expected[i] < expected[j]
		})

		vector := alloc(uint64(size))
		for i, val := range input {
			VectorSetAtUnsafe(vector, uint64(i), val)
		}

		VectorSort[int32](vector, int32Comparator)

		for i := uint64(0); i < size; i++ {
			actual := VectorItemGetAtUnsafe[int32](vector, i)
			if actual != expected[i] {
				t.Fatalf("Random Stress: Mismatch at index %d (expected %d, got %d)", i, expected[i], actual)
			}
		}
		foundationtesting.Assert(true, "", "Large random sort verified against Go stdlib", t)
	})

	t.Run("Stability and Precision (Reverse 5k)", func(t *testing.T) {
		const size = 5000
		vector := alloc(size)
		for i := 0; i < size; i++ {
			VectorSetAtUnsafe(vector, uint64(i), int32(size-i))
		}

		VectorSort[int32](vector, int32Comparator)

		for i := uint64(0); i < size-1; i++ {
			curr := VectorItemGetAtUnsafe[int32](vector, i)
			next := VectorItemGetAtUnsafe[int32](vector, i+1)
			if curr > next {
				t.Fatalf("Monotonicity failure at index %d: %d > %d", i, curr, next)
			}
		}
		foundationtesting.Assert(true, "", "Reverse sorted vector correctly re-ordered", t)
	})
}

// int32Comparator is a standard comparator for testing.
func int32Comparator(a, b int32) int {
	if a < b {
		return -1
	}
	if a > b {
		return 1
	}
	return 0
}
