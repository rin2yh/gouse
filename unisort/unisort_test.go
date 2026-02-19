package unisort_test

import (
	"reflect"
	"testing"

	"github.com/rin2yh/gouse/unisort"
)

func TestUniqueSortNaturalInts(t *testing.T) {
	tests := []struct {
		name string
		arr  []int
		want []int
	}{
		{
			name: "empty slice",
			arr:  []int{},
			want: []int{},
		},
		{
			name: "single element",
			arr:  []int{5},
			want: []int{5},
		},
		{
			name: "sorted unique elements",
			arr:  []int{1, 2, 3, 4, 5},
			want: []int{1, 2, 3, 4, 5},
		},
		{
			name: "unsorted unique elements",
			arr:  []int{5, 3, 1, 4, 2},
			want: []int{1, 2, 3, 4, 5},
		},
		{
			name: "with duplicates",
			arr:  []int{3, 1, 4, 1, 5, 9, 2, 6, 5},
			want: []int{1, 2, 3, 4, 5, 6, 9},
		},
		{
			name: "with zeros",
			arr:  []int{0, 0, 1, 0, 2, 3},
			want: []int{1, 2, 3},
		},
		{
			name: "with negative numbers",
			arr:  []int{-1, 2, 3, 4},
			want: []int{-1, 2, 3, 4}, // Function returns original sorted array when containing negatives
		},
		{
			name: "large numbers",
			arr:  []int{100, 50, 100, 25},
			want: []int{25, 50, 100},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := unisort.UniqueSortNaturalInts(tt.arr)
			if !reflect.DeepEqual(got, tt.want) {
				t.Errorf("UniqueSortNaturalInts() = %v, want %v", got, tt.want)
			}
		})
	}
}
