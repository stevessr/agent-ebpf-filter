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

#include <linux/types.h>

/* Model parameters */
#define FEATURE_DIM 128
#define NUM_TREES 15
#define MAX_TREE_NODES 127  /* Complete binary tree depth 7 */

/* Fixed-point precision: 1000 = 0.001 resolution */
#define FLOAT_SCALE 1000
#define FLOAT_TO_FIXED(f) ((s64)((f) * FLOAT_SCALE))

/* Action types */
enum ml_action {
	ML_ACTION_ALLOW = 0,
	ML_ACTION_BLOCK = 1,
	ML_ACTION_ALERT = 2,
};

/* Tree node structure - 32 bytes aligned */
struct tree_node {
	u32 feature_idx;    /* Which feature to test */
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
	struct tree_node *trees[NUM_TREES];
	size_t tree_sizes[NUM_TREES];
};

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
