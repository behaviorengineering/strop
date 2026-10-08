package embed

import "math"

// CosineSimilarity returns dot(a,b)/(||a||*||b||). Zero norms yield 0.
func CosineSimilarity(a, b []float64) float64 {
	if len(a) == 0 || len(b) == 0 || len(a) != len(b) {
		return 0
	}
	var dot, na, nb float64
	for i := range a {
		if math.IsNaN(a[i]) || math.IsNaN(b[i]) || math.IsInf(a[i], 0) || math.IsInf(b[i], 0) {
			return 0
		}
		dot += a[i] * b[i]
		na += a[i] * a[i]
		nb += b[i] * b[i]
	}
	if na == 0 || nb == 0 {
		return 0
	}
	return dot / (math.Sqrt(na) * math.Sqrt(nb))
}

// BestMatch returns the index and score of the candidate vector most similar to query.
func BestMatch(query []float64, candidates [][]float64) (index int, score float64) {
	if len(candidates) == 0 {
		return 0, 0
	}
	best := 0
	bestCos := CosineSimilarity(query, candidates[0])
	for i := 1; i < len(candidates); i++ {
		c := CosineSimilarity(query, candidates[i])
		if c > bestCos {
			bestCos = c
			best = i
		}
	}
	return best, bestCos
}
