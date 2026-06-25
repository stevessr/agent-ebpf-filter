package platform

import "math"

// MaxFloat64 returns the maximum of the given float64 values.
func MaxFloat64(values ...float64) float64 {
	if len(values) == 0 {
		return 0
	}
	max := values[0]
	for _, v := range values[1:] {
		if v > max {
			max = v
		}
	}
	return max
}

// BoolToFloat converts a bool to 1.0 or 0.0.
func BoolToFloat(b bool) float64 {
	if b {
		return 1.0
	}
	return 0.0
}

// ShannonEntropy computes Shannon entropy of a string, normalized to [0,1].
func ShannonEntropy(s string) float64 {
	if len(s) == 0 {
		return 0
	}
	counts := make(map[byte]int)
	for i := 0; i < len(s); i++ {
		counts[s[i]]++
	}
	var entropy float64
	n := float64(len(s))
	for _, c := range counts {
		p := float64(c) / n
		entropy -= p * math.Log2(p)
	}
	return entropy / 8.0
}
