// kernels.cu — CUDA kernels + host launchers for ML acceleration
// Compile: nvcc -c -o kernels.o kernels.cu && ar rcs libmlcuda.a kernels.o

#include <cuda_runtime.h>
#include <math.h>
#include <stdio.h>
#include <stdlib.h>

#define CUDA_CHECK(call) do { \
    cudaError_t _e = (call); \
    if (_e != cudaSuccess) { \
        fprintf(stderr, "[CUDA ERROR] %s:%d: %s\n", __FILE__, __LINE__, cudaGetErrorString(_e)); \
        return 1; \
    } \
} while(0)

#define KERNEL_CHECK() do { \
    cudaError_t _e = cudaGetLastError(); \
    if (_e != cudaSuccess) { \
        fprintf(stderr, "[CUDA KERNEL ERROR] %s:%d: %s\n", __FILE__, __LINE__, cudaGetErrorString(_e)); \
        return 1; \
    } \
} while(0)
#include <stdio.h>

// ── Device info ────────────────────────────────────────────────────

extern "C" int cuda_dev_count() {
    int n;
    cudaError_t err = cudaGetDeviceCount(&n);
    if (err != cudaSuccess) return 0;
    return n;
}

extern "C" const char* cuda_driver_version() {
    int v;
    if (cudaDriverGetVersion(&v) == cudaSuccess) {
        static char buf[32];
        snprintf(buf, sizeof(buf), "%d.%d", v/1000, (v%100)/10);
        return buf;
    }
    return "unknown";
}

extern "C" const char* cuda_dev_name(int d) {
    static char buf[256];
    cudaDeviceProp p;
    if (cudaGetDeviceProperties(&p, d) == cudaSuccess) {
        snprintf(buf, sizeof(buf), "%s", p.name);
        return buf;
    }
    return "unknown";
}

extern "C" int cuda_dev_mem_mb(int d) {
    cudaDeviceProp p;
    if (cudaGetDeviceProperties(&p, d) == cudaSuccess)
        return (int)(p.totalGlobalMem / (1024*1024));
    return 0;
}

extern "C" int cuda_mem_used_mb() {
    size_t free_bytes, total_bytes;
    if (cudaMemGetInfo(&free_bytes, &total_bytes) == cudaSuccess)
        return (int)((total_bytes - free_bytes) / (1024*1024));
    return 0;
}

extern "C" int cuda_mem_total_mb() {
    size_t free_bytes, total_bytes;
    if (cudaMemGetInfo(&free_bytes, &total_bytes) == cudaSuccess)
        return (int)(total_bytes / (1024*1024));
    return 0;
}

// ── KNN Euclidean Distance ─────────────────────────────────────────

__global__ void knn_dist_kernel(
    const float* queries, const float* refs, float* dists,
    int nQ, int nR, int dim
) {
    int q = blockIdx.x * blockDim.x + threadIdx.x;
    if (q >= nQ) return;
    for (int r = 0; r < nR; r++) {
        float s = 0;
        for (int d = 0; d < dim; d++) {
            float df = queries[q*dim+d] - refs[r*dim+d];
            s += df * df;
        }
        dists[q*nR + r] = sqrtf(s);
    }
}

extern "C" int knn_dist_launch(
    const float* q, const float* r, float* d,
    int nQ, int nR, int dim
) {
    float *dq=0, *dr=0, *dd=0;
    CUDA_CHECK(cudaMalloc(&dq, nQ*dim*4));
    CUDA_CHECK(cudaMemcpy(dq, q, nQ*dim*4, cudaMemcpyHostToDevice));
    CUDA_CHECK(cudaMalloc(&dr, nR*dim*4));
    CUDA_CHECK(cudaMemcpy(dr, r, nR*dim*4, cudaMemcpyHostToDevice));
    CUDA_CHECK(cudaMalloc(&dd, nQ*nR*4));
    int blk = (nQ + 255) / 256;
    knn_dist_kernel<<<blk, 256>>>(dq, dr, dd, nQ, nR, dim);
    KERNEL_CHECK();
    CUDA_CHECK(cudaDeviceSynchronize());
    CUDA_CHECK(cudaMemcpy(d, dd, nQ*nR*4, cudaMemcpyDeviceToHost));
    cudaFree(dq); cudaFree(dr); cudaFree(dd);
    return 0;
}

// ── KNN Manhattan Distance ─────────────────────────────────────────

__global__ void knn_manh_kernel(
    const float* queries, const float* refs, float* dists,
    int nQ, int nR, int dim
) {
    int q = blockIdx.x * blockDim.x + threadIdx.x;
    if (q >= nQ) return;
    for (int r = 0; r < nR; r++) {
        float s = 0;
        for (int d = 0; d < dim; d++)
            s += fabsf(queries[q*dim+d] - refs[r*dim+d]);
        dists[q*nR + r] = s;
    }
}

extern "C" int knn_manh_launch(
    const float* q, const float* r, float* d,
    int nQ, int nR, int dim
) {
    float *dq=0, *dr=0, *dd=0;
    CUDA_CHECK(cudaMalloc(&dq, nQ*dim*4));
    CUDA_CHECK(cudaMemcpy(dq, q, nQ*dim*4, cudaMemcpyHostToDevice));
    CUDA_CHECK(cudaMalloc(&dr, nR*dim*4));
    CUDA_CHECK(cudaMemcpy(dr, r, nR*dim*4, cudaMemcpyHostToDevice));
    CUDA_CHECK(cudaMalloc(&dd, nQ*nR*4));
    int blk = (nQ + 255) / 256;
    knn_manh_kernel<<<blk, 256>>>(dq, dr, dd, nQ, nR, dim);
    KERNEL_CHECK();
    CUDA_CHECK(cudaDeviceSynchronize());
    CUDA_CHECK(cudaMemcpy(d, dd, nQ*nR*4, cudaMemcpyDeviceToHost));
    cudaFree(dq); cudaFree(dr); cudaFree(dd);
    return 0;
}

// ── Logistic Softmax Forward ────────────────────────────────────────

__global__ void logit_fwd_kernel(
    const float* X, const float* W, float* P,
    int N, int D, int C
) {
    int s = blockIdx.x * blockDim.x + threadIdx.x;
    if (s >= N) return;
    float logits[8];
    float mx = -1e30f;
    for (int c = 0; c < C; c++) {
        float dot = W[c*(D+1)+D]; // bias
        for (int d = 0; d < D; d++)
            dot += W[c*(D+1)+d] * X[s*D+d];
        logits[c] = dot;
        if (dot > mx) mx = dot;
    }
    float sum = 0;
    for (int c = 0; c < C; c++) {
        float v = expf(logits[c] - mx);
        P[s*C+c] = v; sum += v;
    }
    for (int c = 0; c < C; c++) P[s*C+c] /= sum;
}

extern "C" int logit_fwd_launch(
    const float* X, const float* W, float* P,
    int N, int D, int C
) {
    float *dX=0, *dW=0, *dP=0;
    CUDA_CHECK(cudaMalloc(&dX, N*D*4));
    CUDA_CHECK(cudaMemcpy(dX, X, N*D*4, cudaMemcpyHostToDevice));
    CUDA_CHECK(cudaMalloc(&dW, C*(D+1)*4));
    CUDA_CHECK(cudaMemcpy(dW, W, C*(D+1)*4, cudaMemcpyHostToDevice));
    CUDA_CHECK(cudaMalloc(&dP, N*C*4));
    int blk = (N + 255) / 256;
    logit_fwd_kernel<<<blk, 256>>>(dX, dW, dP, N, D, C);
    KERNEL_CHECK();
    CUDA_CHECK(cudaDeviceSynchronize());
    CUDA_CHECK(cudaMemcpy(P, dP, N*C*4, cudaMemcpyDeviceToHost));
    cudaFree(dX); cudaFree(dW); cudaFree(dP);
    return 0;
}

// ── Logistic Gradient ───────────────────────────────────────────────

__global__ void logit_grad_kernel(
    const float* X, const float* P, const int* L,
    float* G, int N, int D, int C
) {
    int tid = blockIdx.x * blockDim.x + threadIdx.x;
    int total = C * (D + 1);
    if (tid >= total) return;
    int c = tid / (D + 1);
    int d = tid % (D + 1);

    float grad = 0;
    for (int s = 0; s < N; s++) {
        float tgt = (L[s] == c) ? 1.0f : 0.0f;
        float err = P[s*C+c] - tgt;
        if (d == D) {
            grad += err; // bias
        } else {
            grad += err * X[s*D+d];
        }
    }
    G[tid] += grad / (float)N;
}

extern "C" int logit_grad_launch(
    const float* X, const float* P, const int* L,
    float* G, int N, int D, int C
) {
    float *dX=0, *dP=0, *dG=0;
    int* dL=0;
    CUDA_CHECK(cudaMalloc(&dX, N*D*4));
    CUDA_CHECK(cudaMemcpy(dX, X, N*D*4, cudaMemcpyHostToDevice));
    CUDA_CHECK(cudaMalloc(&dP, N*C*4));
    CUDA_CHECK(cudaMemcpy(dP, P, N*C*4, cudaMemcpyHostToDevice));
    CUDA_CHECK(cudaMalloc(&dL, N*4));
    CUDA_CHECK(cudaMemcpy(dL, L, N*4, cudaMemcpyHostToDevice));
    CUDA_CHECK(cudaMalloc(&dG, C*(D+1)*4));
    CUDA_CHECK(cudaMemset(dG, 0, C*(D+1)*4));
    int total = C * (D + 1);
    int blk = (total + 255) / 256;
    logit_grad_kernel<<<blk, 256>>>(dX, dP, dL, dG, N, D, C);
    KERNEL_CHECK();
    CUDA_CHECK(cudaDeviceSynchronize());
    CUDA_CHECK(cudaMemcpy(G, dG, C*(D+1)*4, cudaMemcpyDeviceToHost));
    cudaFree(dX); cudaFree(dP); cudaFree(dL); cudaFree(dG);
    return 0;
}
