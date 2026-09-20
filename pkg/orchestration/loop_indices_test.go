package orchestration

import (
	"reflect"
	"testing"
)

func TestNormalizePerItemIndices(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name      string
		requested []int
		n         int
		want      []int
	}{
		{
			name:      "nil requested processes all indices",
			requested: nil,
			n:         3,
			want:      []int{0, 1, 2},
		},
		{
			name:      "empty requested processes all indices",
			requested: []int{},
			n:         2,
			want:      []int{0, 1},
		},
		{
			name:      "deduplicates and sorts",
			requested: []int{2, 0, 2, 1},
			n:         3,
			want:      []int{0, 1, 2},
		},
		{
			name:      "drops out of range indices",
			requested: []int{-1, 3, 1},
			n:         3,
			want:      []int{1},
		},
		{
			name:      "n zero returns nil",
			requested: []int{0},
			n:         0,
			want:      nil,
		},
	}
	for _, tt := range tests {
		tt := tt
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			got := normalizePerItemIndices(tt.requested, tt.n)
			if !reflect.DeepEqual(got, tt.want) {
				t.Fatalf("normalizePerItemIndices(%v, %d) = %#v, want %#v", tt.requested, tt.n, got, tt.want)
			}
		})
	}
}
