/* Kernel ML Inference Engine - Random Forest Classifier
 *
 * Architecture:
 * - Fixed-point arithmetic (no FPU in kernel)
 * - Pre-compiled decision trees
 * - O(log N) inference per tree
 * - Action classification: ALLOW, BLOCK, ALERT
 *
 * Design rationale:
 * - Kernel space = low latency, direct syscall interception
 * - Fixed-point = kernel-safe, deterministic
 * - Decision trees = interpretable, fast (vs neural nets)
 */

#ifndef _KERNEL_ML_INFERENCE_H
#define _KERNEL_ML_INFERENCE_H

#ifdef __KERNEL__
#include <linux/types.h>
#else
#include <stddef.h>
#include <stdint.h>
typedef uint8_t u8;
typedef uint32_t u32;
typedef int32_t s32;
typedef uint64_t u64;
typedef int64_t s64;
#define __user
#endif

/* Model parameters */
#define FEATURE_DIM 128
#define DEFAULT_NUM_TREES 15
#define NUM_TREES 64
#define MAX_TREE_NODES 4095
#define ML_MAX_TREE_DEPTH 1024
#define ML_MAX_CLASSES 16
#define ML_DEFAULT_NUM_CLASSES 3
#define ML_DEFAULT_MAX_DEPTH 64

/* Fixed-point precision: 1000 = 0.001 resolution */
#define FLOAT_SCALE 1000
#define FLOAT_TO_FIXED(f) ((s64)((f) * FLOAT_SCALE))

/* Action types */
enum ml_action {
	ML_ACTION_ALLOW = 0,
	ML_ACTION_BLOCK = 1,
	ML_ACTION_ALERT = 2,
};

/* Tree node structure - fixed 32-byte UAPI layout.
 *
 * Keep this layout synchronized with model_loader.py and cuda_infer_helper.cu:
 *   u32 feature_idx
 *   u32 _pad0          // aligns threshold and preserves the historical
 *                      // Python native "IqiiiB3x" 32-byte encoding
 *   s64 threshold
 *   s32 left_child
 *   s32 right_child
 *   s32 leaf_value
 *   u8  is_leaf
 *   u8  _pad[3]
 */
struct tree_node {
	u32 feature_idx;    /* Which feature to test */
	u32 _pad0;
	s64 threshold;      /* Fixed-point threshold */
	s32 left_child;     /* -1 = leaf node */
	s32 right_child;
	s32 leaf_value;     /* Action if leaf */
	u8 is_leaf;
	u8 _pad[3];
} __attribute__((packed));

/* Feature vector for inference */
struct feature_vector {
	s64 features[FEATURE_DIM];
	u32 pid;
	u32 syscall_type;
	char comm[16];
};

/* Model metadata */
struct ml_model {
	u32 version;
	u32 num_trees;
	u32 feature_dim;
	u32 total_nodes;
	u32 num_classes;
	u32 max_depth;
	struct tree_node *trees[NUM_TREES];
	size_t tree_sizes[NUM_TREES];
};

/* Inference backend selection.
 *
 * CUDA is implemented as a kernel/userspace offload backend because Linux
 * kernel modules cannot link against or call the NVIDIA CUDA runtime directly.
 * The DKMS module exports a request/result ABI under /proc; the optional
 * userspace helper owns libcuda/libcudart and mirrors the loaded model.
 */
enum ml_backend_type {
	ML_BACKEND_KERNEL = 0,
	ML_BACKEND_CUDA = 1,
	ML_BACKEND_AUTO = 2,
};

#define ML_CUDA_REQUEST_VERSION 1
#define ML_CUDA_DEFAULT_TIMEOUT_MS 50

struct ml_cuda_request {
	u32 version;
	u32 reserved;
	u64 request_id;
	u64 model_generation;
	struct feature_vector features;
} __attribute__((packed));

struct ml_cuda_result {
	u32 version;
	u32 status;      /* 0 = success, otherwise errno-style helper failure */
	u64 request_id;
	u32 action;      /* enum ml_action when status == 0 */
	u32 reserved;
} __attribute__((packed));

/* Inference API */
enum ml_action ml_inference(struct ml_model *model, struct feature_vector *fv);
int ml_model_load(struct ml_model *model, const void __user *data, size_t size);
void ml_model_free(struct ml_model *model);

/* Feature extraction from syscall context */
void extract_features(struct feature_vector *fv,
                      u32 syscall_nr,
                      u32 pid,
                      const char *comm,
                      const long *args);

#endif /* _KERNEL_ML_INFERENCE_H */
