/* Extended Model Types - SVM, Logistic Regression, Neural Network
 *
 * Architecture:
 * - All models use fixed-point arithmetic
 * - Model-agnostic feature vector interface
 * - Unified action output (ALLOW/BLOCK/ALERT)
 */

#ifndef _KERNEL_ML_MODELS_H
#define _KERNEL_ML_MODELS_H

#include "ml_inference.h"

/* Model types */
enum model_type {
	MODEL_TYPE_RANDOM_FOREST = 0,
	MODEL_TYPE_SVM = 1,
	MODEL_TYPE_LOGISTIC_REGRESSION = 2,
	MODEL_TYPE_NEURAL_NETWORK = 3,
};

/* ===== SVM Model ===== */
#define SVM_MAX_SUPPORT_VECTORS 128

struct svm_model {
	u32 version;
	u32 num_support_vectors;
	u32 feature_dim;
	s64 bias;                               /* Fixed-point */
	s64 weights[FEATURE_DIM];               /* Linear kernel */
	s64 support_vectors[SVM_MAX_SUPPORT_VECTORS][FEATURE_DIM];
	s64 alphas[SVM_MAX_SUPPORT_VECTORS];
};

enum ml_action svm_inference(struct svm_model *model, struct feature_vector *fv);
int svm_model_load(struct svm_model *model, const void __user *data, size_t size);
void svm_model_free(struct svm_model *model);

/* ===== Logistic Regression ===== */
struct lr_model {
	u32 version;
	u32 feature_dim;
	s64 weights[FEATURE_DIM];
	s64 bias;
	s64 thresholds[2];  /* ALLOW/BLOCK, BLOCK/ALERT */
};

enum ml_action lr_inference(struct lr_model *model, struct feature_vector *fv);
int lr_model_load(struct lr_model *model, const void __user *data, size_t size);
void lr_model_free(struct lr_model *model);

/* ===== Neural Network (Single Hidden Layer MLP) ===== */
#define NN_MAX_HIDDEN 64

struct nn_model {
	u32 version;
	u32 input_dim;
	u32 hidden_dim;
	u32 output_dim;
	s64 weights_input[FEATURE_DIM * NN_MAX_HIDDEN];  /* input -> hidden */
	s64 bias_hidden[NN_MAX_HIDDEN];
	s64 weights_output[NN_MAX_HIDDEN * 3];           /* hidden -> 3 classes */
	s64 bias_output[3];
};

enum ml_action nn_inference(struct nn_model *model, struct feature_vector *fv);
int nn_model_load(struct nn_model *model, const void __user *data, size_t size);
void nn_model_free(struct nn_model *model);

/* ===== Activation Functions (Fixed-Point) ===== */

/* ReLU: max(0, x) */
static inline s64 relu(s64 x)
{
	return x > 0 ? x : 0;
}

/* Sigmoid approximation using lookup table (0.0 to 1.0 scaled by FLOAT_SCALE) */
static inline s64 sigmoid_approx(s64 x)
{
	/* Piecewise linear approximation:
	 * x < -6000: 0
	 * -6000 <= x <= 6000: linear interpolation
	 * x > 6000: 1000
	 */
	if (x < -6 * FLOAT_SCALE)
		return 0;
	if (x > 6 * FLOAT_SCALE)
		return FLOAT_SCALE;

	/* Linear: y = 0.5 + x/12 (scaled) */
	return (FLOAT_SCALE / 2) + (x / 12);
}

/* Softmax approximation: return argmax instead of full softmax */
static inline int argmax(const s64 *values, int n)
{
	int max_idx = 0;
	s64 max_val = values[0];
	int i;

	for (i = 1; i < n; i++) {
		if (values[i] > max_val) {
			max_val = values[i];
			max_idx = i;
		}
	}
	return max_idx;
}

/* ===== Unified Model Interface ===== */
struct unified_model {
	enum model_type type;
	union {
		struct ml_model rf;
		struct svm_model svm;
		struct lr_model lr;
		struct nn_model nn;
	} data;
};

enum ml_action unified_inference(struct unified_model *model, struct feature_vector *fv);
int unified_model_load(struct unified_model *model, enum model_type type,
                       const void __user *data, size_t size);
void unified_model_free(struct unified_model *model);

#endif /* _KERNEL_ML_MODELS_H */
