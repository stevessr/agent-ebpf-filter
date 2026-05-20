//go:build !cuda

package cuda

// Status holds CUDA capability info.
type Status struct {
	Available bool   `json:"available"`
	Count     int    `json:"count"`
	Device    string `json:"device"`
	MemoryMB  int    `json:"memoryMb"`
}

var status = Status{Available: false}

// GetStatus returns current CUDA status.
func GetStatus() Status { return status }

// IsAvailable returns true if CUDA is usable.
func IsAvailable() bool { return false }

// DeviceInfo returns a display string.
func DeviceInfo() string { return "CUDA: not available" }

// MemUsedMB returns current GPU memory usage in MB.
func MemUsedMB() int { return 0 }

// MemTotalMB returns total GPU memory in MB.
func MemTotalMB() int { return 0 }

// RuntimeStatus returns live GPU status for display.
func RuntimeStatus() string { return "" }

// KNNDistances computes pairwise distances on CPU when CUDA is unavailable.
func KNNDistances(queries, refs []float32, nQ, nR, dim int, metric string) []float32 {
	out := make([]float32, nQ*nR)
	return cpuKNNDistances(queries, refs, nQ, nR, dim, metric, out)
}

func cpuKNNDistances(q, r []float32, nQ, nR, dim int, metric string, out []float32) []float32 {
	for qi := 0; qi < nQ; qi++ {
		for ri := 0; ri < nR; ri++ {
			var s float32
			for d := 0; d < dim; d++ {
				df := q[qi*dim+d] - r[ri*dim+d]
				if metric == "manhattan" {
					if df < 0 {
						df = -df
					}
					s += df
				} else {
					s += df * df
				}
			}
			if metric != "manhattan" {
				s = float32(fastSqrt(float64(s)))
			}
			out[qi*nR+ri] = s
		}
	}
	return out
}

func fastSqrt(x float64) float64 {
	if x <= 0 {
		return 0
	}
	z := x
	for i := 0; i < 10; i++ {
		z = (z + x/z) / 2
	}
	return z
}

// LogisticForward computes softmax probabilities on CPU when CUDA is unavailable.
func LogisticForward(X, W []float32, N, D, C int) []float32 {
	P := make([]float32, N*C)
	return cpuLogisticForward(X, W, N, D, C, P)
}

func cpuLogisticForward(X, W []float32, N, D, C int, P []float32) []float32 {
	for s := 0; s < N; s++ {
		logits := make([]float32, C)
		mx := float32(-1e30)
		for c := 0; c < C; c++ {
			dot := W[c*(D+1)+D]
			for d := 0; d < D; d++ {
				dot += W[c*(D+1)+d] * X[s*D+d]
			}
			logits[c] = dot
			if dot > mx {
				mx = dot
			}
		}
		sum := float32(0)
		for c := 0; c < C; c++ {
			v := expf32(logits[c] - mx)
			P[s*C+c] = v
			sum += v
		}
		for c := 0; c < C; c++ {
			P[s*C+c] /= sum
		}
	}
	return P
}

func expf32(x float32) float32 {
	if x < -20 {
		return 0
	}
	if x > 20 {
		return 1e9
	}
	result := float32(1.0)
	term := float32(1.0)
	for i := 1; i < 15; i++ {
		term *= x / float32(i)
		result += term
	}
	return result
}

// LogisticGradient computes batched gradient on CPU when CUDA is unavailable.
func LogisticGradient(X, P []float32, L []int32, G []float32, N, D, C int) {
	cpuLogisticGradient(X, P, L, G, N, D, C)
}

func cpuLogisticGradient(X, P []float32, L []int32, G []float32, N, D, C int) {
	for tid := 0; tid < C*(D+1); tid++ {
		c := tid / (D + 1)
		d := tid % (D + 1)
		var grad float32
		for s := 0; s < N; s++ {
			tgt := float32(0)
			if int(L[s]) == c {
				tgt = 1.0
			}
			err := P[s*C+c] - tgt
			if d == D {
				grad += err
			} else {
				grad += err * X[s*D+d]
			}
		}
		G[tid] += grad / float32(N)
	}
}
