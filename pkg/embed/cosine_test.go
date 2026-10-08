package embed

import (
	"math"
	"testing"
)

func TestCosineSimilarity_identical(t *testing.T) {
	a := []float64{1, 0, 0}
	b := []float64{1, 0, 0}
	if CosineSimilarity(a, b) < 0.99 {
		t.Fatal("expected ~1")
	}
}

func TestCosineSimilarity_orthogonal(t *testing.T) {
	a := []float64{1, 0, 0}
	orth := []float64{0, 1, 0}
	if CosineSimilarity(a, orth) > 0.01 {
		t.Fatal("expected ~0")
	}
}

func TestCosineSimilarity_nilOrDimMismatch(t *testing.T) {
	b := []float64{1, 0}
	if CosineSimilarity(nil, b) != 0 {
		t.Fatal("nil")
	}
	if CosineSimilarity(b, []float64{1}) != 0 {
		t.Fatal("dim")
	}
}

func TestCosineSimilarity_nanInfZeroNorm(t *testing.T) {
	if CosineSimilarity([]float64{math.NaN()}, []float64{1}) != 0 {
		t.Fatal("nan")
	}
	if CosineSimilarity([]float64{0, 0}, []float64{1, 0}) != 0 {
		t.Fatal("zero norm")
	}
}

func TestBestMatch(t *testing.T) {
	q := []float64{1, 0}
	cands := [][]float64{{0, 1}, {1, 0}, {0.7, 0.7}}
	idx, sc := BestMatch(q, cands)
	if idx != 1 || sc < 0.99 {
		t.Fatalf("got %d %v", idx, sc)
	}
}
