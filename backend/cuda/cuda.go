package cuda

/*
#cgo LDFLAGS: -L${SRCDIR} -L/opt/cuda/lib64 -lmlcuda -lcudart -lcuda -lstdc++ -Wl,-rpath,/opt/cuda/lib64
#cgo CFLAGS: -I/opt/cuda/include

#include <stdlib.h>

int cuda_dev_count();
const char* cuda_dev_name(int d);
int cuda_dev_mem_mb(int d);

void knn_dist_launch(const float* q, const float* r, float* d, int nQ, int nR, int dim);
void knn_manh_launch(const float* q, const float* r, float* d, int nQ, int nR, int dim);
void logit_fwd_launch(const float* X, const float* W, float* P, int N, int D, int C);
void logit_grad_launch(const float* X, const float* P, const int* L, float* G, int N, int D, int C);
*/
import "C"
import (
	"fmt"
	"unsafe"
)

// Status holds CUDA capability info
type Status struct {
	Available bool   `json:"available"`
	Count     int    `json:"count"`
	Device    string `json:"device"`
	MemoryMB  int    `json:"memoryMb"`
}

var status Status

func init() {
	n := int(C.cuda_dev_count())
	if n == 0 {
		status = Status{Available: false}
		return
	}
	status = Status{
		Available: true,
		Count:     n,
		Device:    C.GoString(C.cuda_dev_name(0)),
		MemoryMB:  int(C.cuda_dev_mem_mb(0)),
	}
}

// GetStatus returns current CUDA status
func GetStatus() Status { return status }

// IsAvailable returns true if CUDA is usable
func IsAvailable() bool { return status.Available }

// DeviceInfo returns a display string
func DeviceInfo() string {
	if !status.Available {
		return "CUDA: not available"
	}
	return fmt.Sprintf("%s (%d MB)", status.Device, status.MemoryMB)
}

// KNNDistances computes pairwise distances (GPU or CPU fallback).
func KNNDistances(queries, refs []float32, nQ, nR, dim int, metric string) []float32 {
	out := make([]float32, nQ*nR)
	if !status.Available {
		return cpuKNNDistances(queries, refs, nQ, nR, dim, metric, out)
	}
	if metric == "manhattan" {
		C.knn_manh_launch(
			(*C.float)(unsafe.Pointer(&queries[0])),
			(*C.float)(unsafe.Pointer(&refs[0])),
			(*C.float)(unsafe.Pointer(&out[0])),
			C.int(nQ), C.int(nR), C.int(dim),
		)
	} else {
		C.knn_dist_launch(
			(*C.float)(unsafe.Pointer(&queries[0])),
			(*C.float)(unsafe.Pointer(&refs[0])),
			(*C.float)(unsafe.Pointer(&out[0])),
			C.int(nQ), C.int(nR), C.int(dim),
		)
	}
	return out
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
	// Newton's method for sqrt
	z := x
	for i := 0; i < 10; i++ {
		z = (z + x/z) / 2
	}
	return z
}

// LogisticForward computes softmax probabilities (GPU or CPU fallback).
func LogisticForward(X, W []float32, N, D, C int) []float32 {
	P := make([]float32, N*C)
	if !status.Available {
		return cpuLogisticForward(X, W, N, D, C, P)
	}
	C.logit_fwd_launch(
		(*C.float)(unsafe.Pointer(&X[0])),
		(*C.float)(unsafe.Pointer(&W[0])),
		(*C.float)(unsafe.Pointer(&P[0])),
		C.int(N), C.int(D), C.int(C),
	)
	return P
}

func cpuLogisticForward(X, W []float32, N, D, C int, P []float32) []float32 {
	for s := 0; s < N; s++ {
		logits := make([]float32, C)
		mx := float32(-1e30)
		for c := 0; c < C; c++ {
			dot := W[c*(D+1)+D] // bias
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
	// Taylor series approximation
	result := float32(1.0)
	term := float32(1.0)
	for i := 1; i < 15; i++ {
		term *= x / float32(i)
		result += term
	}
	return result
}

// LogisticGradient computes batched gradient (GPU or CPU fallback).
func LogisticGradient(X, P []float32, L []int32, G []float32, N, D, C int) {
	if !status.Available {
		cpuLogisticGradient(X, P, L, G, N, D, C)
		return
	}
	C.logit_grad_launch(
		(*C.float)(unsafe.Pointer(&X[0])),
		(*C.float)(unsafe.Pointer(&P[0])),
		(*C.int)(unsafe.Pointer(&L[0])),
		(*C.float)(unsafe.Pointer(&G[0])),
		C.int(N), C.int(D), C.int(C),
	)
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
